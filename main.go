package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"dbtest/cluster"
	"dbtest/database"
	"dbtest/docker"
	"dbtest/teleport"
)

func main() {
	app := &cli.Command{
		Name:  "dbtest",
		Usage: "Spin up test databases with Teleport database services",
		Commands: []*cli.Command{
			clusterCommand(),
			dbCommand(),
		},
	}
	suppressUsageOnError(app)
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

// suppressUsageOnError sets OnUsageError on cmd and all subcommands so that
// parse errors print only the error message, not the full usage text.
func suppressUsageOnError(cmd *cli.Command) {
	cmd.OnUsageError = func(ctx context.Context, cmd *cli.Command, err error, isSubcommand bool) error {
		return err
	}
	for _, sub := range cmd.Commands {
		suppressUsageOnError(sub)
	}
}

// clusterCommand returns the top-level "cluster" command group.
func clusterCommand() *cli.Command {
	return &cli.Command{
		Name:  "cluster",
		Usage: "Manage test clusters",
		Commands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "Create a new cluster",
				ArgsUsage: "<name>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "proxy-server",
						Usage:    "Teleport proxy server address (host:port)",
						Required: true,
						Sources:  cli.EnvVars("TELEPORT_PROXY_SERVER"),
					},
					&cli.StringFlag{
						Name:     "binaries-dir",
						Usage:    "Path to a local directory containing Linux Teleport binaries (teleport, tsh, tctl)",
						Required: true,
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("cluster name is required")
					}
					binDir := cmd.String("binaries-dir")
					if err := validateBinariesDir(binDir); err != nil {
						return fmt.Errorf("--binaries-dir: %w", err)
					}
					s := &cluster.State{
						Name:        name,
						ProxyServer: cmd.String("proxy-server"),
					}
					if err := cluster.Create(s); err != nil {
						return err
					}
					binariesDir, err := cluster.BinariesDir(name)
					if err != nil {
						cluster.Remove(name)
						return err
					}
					if err := os.MkdirAll(binariesDir, 0755); err != nil {
						cluster.Remove(name)
						return fmt.Errorf("creating binaries dir: %w", err)
					}
					fmt.Printf("copying teleport binaries...\n")
					for _, binary := range []string{"teleport", "tsh", "tctl"} {
						src := filepath.Join(binDir, binary)
						dst := filepath.Join(binariesDir, binary)
						if err := copyBinary(src, dst); err != nil {
							cluster.Remove(name)
							return fmt.Errorf("copying %s: %w", binary, err)
						}
					}
					docker.NetworkCreate(name)
					dir, _ := cluster.Dir(name)
					fmt.Printf("created cluster %q (directory: %s)\n", name, dir)
					return nil
				},
			},
			{
				Name:  "list",
				Usage: "List all clusters",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					clusters, err := cluster.List()
					if err != nil {
						return err
					}
					if len(clusters) == 0 {
						fmt.Println("no clusters found")
						return nil
					}
					for _, s := range clusters {
						fmt.Printf("%s  %s\n", s.Name, s.ProxyServer)
					}
					return nil
				},
			},
			{
				Name:      "get",
				Usage:     "Get details of a specific cluster",
				ArgsUsage: "<name>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("cluster name is required")
					}
					s, err := cluster.Load(name)
					if err != nil {
						return err
					}
					dir, _ := cluster.Dir(name)
					binDir, _ := cluster.BinariesDir(name)
					fmt.Printf("Name:         %s\n", s.Name)
					fmt.Printf("Proxy Server: %s\n", s.ProxyServer)
					fmt.Printf("Binaries:     %s\n", binDir)
					fmt.Printf("Directory:    %s\n", dir)
					return nil
				},
			},
			{
				Name:      "remove",
				Usage:     "Remove a cluster and wipe all associated Docker resources",
				ArgsUsage: "<name>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("cluster name is required")
					}
					if _, err := cluster.Load(name); err != nil {
						return err
					}
					docker.WipeContainers(name)
					docker.WipeImages(name)
					docker.NetworkRemove(name)
					return cluster.Remove(name)
				},
			},
		},
	}
}

// dbCommand returns the top-level "db" command group.
func dbCommand() *cli.Command {
	return &cli.Command{
		Name:  "db",
		Usage: "Manage databases within a cluster",
		Commands: []*cli.Command{
			{
				Name:  "up",
				Usage: "Start a test database and its Teleport database service",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "cluster",
						Usage:    "Cluster name",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "name",
						Usage:    "Database instance name (forms the container name: <cluster>-<name>)",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "type",
						Usage:    "Database type (postgres, mysql, mariadb, mongodb, cockroachdb, redis, redis-cluster)",
						Required: true,
					},
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Print output of docker/tsh/tctl commands",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					clusterName := cmd.String("cluster")
					dbName := cmd.String("name")
					dbType := cmd.String("type")
					setCmdOutput(cmd.Bool("debug"))

					login, err := teleport.CheckLogin()
					if err != nil {
						return err
					}

					s, err := cluster.Load(clusterName)
					if err != nil {
						return fmt.Errorf("loading cluster %q: %w", clusterName, err)
					}

					if want, got := proxyHost(s.ProxyServer), login.ProxyHost; want != got {
						return fmt.Errorf("logged into wrong cluster: tsh is connected to %q but cluster %q uses %q (run 'tsh login --proxy=%s')", got, clusterName, want, want)
					}
					fmt.Printf("logged in as %s on cluster %s\n", login.Username, login.Cluster)

					cfg, err := configFromCluster(s, clusterName, dbName)
					if err != nil {
						return err
					}

					db, err := getDatabase(dbType, cfg)
					if err != nil {
						return err
					}

					return db.Up()
				},
			},
			{
				Name:  "down",
				Usage: "Stop and remove a test database and its Teleport database service",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "cluster",
						Usage:    "Cluster name",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "name",
						Usage:    "Database instance name",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "type",
						Usage:    "Database type (postgres, mysql, mariadb, mongodb, cockroachdb, redis, redis-cluster)",
						Required: true,
					},
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Print output of docker/tsh/tctl commands",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					clusterName := cmd.String("cluster")
					dbName := cmd.String("name")
					dbType := cmd.String("type")
					setCmdOutput(cmd.Bool("debug"))

					s, err := cluster.Load(clusterName)
					if err != nil {
						return fmt.Errorf("loading cluster %q: %w", clusterName, err)
					}

					cfg, err := configFromCluster(s, clusterName, dbName)
					if err != nil {
						return err
					}

					db, err := getDatabase(dbType, cfg)
					if err != nil {
						return err
					}

					if err := db.Down(); err != nil {
						return err
					}

					fmt.Printf("cleaning up files for %s...\n", dbName)
					os.RemoveAll(cfg.WorkDir)
					os.RemoveAll(cfg.ServiceWorkDir)
					return nil
				},
			},
		},
	}
}

// configFromCluster builds a database.Config from cluster state and the
// given database instance name.
func configFromCluster(s *cluster.State, clusterName, dbName string) (database.Config, error) {
	workDir, err := cluster.DBDir(clusterName, dbName)
	if err != nil {
		return database.Config{}, err
	}
	svcDir, err := cluster.DBServiceDir(clusterName, dbName)
	if err != nil {
		return database.Config{}, err
	}
	binDir, err := cluster.BinariesDir(clusterName)
	if err != nil {
		return database.Config{}, err
	}
	return database.Config{
		ClusterName:    s.Name,
		Name:           dbName,
		ProxyServer:    proxyAddr(s.ProxyServer),
		BinariesDir:    binDir,
		WorkDir:        workDir,
		ServiceWorkDir: svcDir,
	}, nil
}

// proxyAddr ensures the proxy server address includes a port, defaulting to 443.
func proxyAddr(addr string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, "443")
	}
	return addr
}

// proxyHost returns just the hostname from a proxy server address (strips port).
func proxyHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func getDatabase(dbType string, cfg database.Config) (database.Database, error) {
	switch dbType {
	case "postgres":
		return &database.Postgres{Config: cfg}, nil
	case "mysql":
		return &database.MySQL{Config: cfg}, nil
	case "mariadb":
		return &database.MariaDB{Config: cfg}, nil
	case "mongodb":
		return &database.MongoDB{Config: cfg}, nil
	case "cockroachdb":
		return &database.CockroachDB{Config: cfg}, nil
	case "redis":
		return &database.Redis{Config: cfg}, nil
	case "redis-cluster":
		return &database.RedisCluster{Config: cfg}, nil
	default:
		return nil, fmt.Errorf("invalid --type %q (valid: postgres, mysql, mariadb, mongodb, cockroachdb, redis, redis-cluster)", dbType)
	}
}

// setCmdOutput configures the output writer for subcommands in the docker and
// teleport packages. When debug is false, output is discarded.
func setCmdOutput(debug bool) {
	w := io.Writer(os.Stdout)
	if !debug {
		w = io.Discard
	}
	docker.Output = w
	teleport.Output = w
}

// validateBinariesDir checks that dir contains teleport, tsh, and tctl as
// regular files.
func validateBinariesDir(dir string) error {
	for _, name := range []string{"teleport", "tsh", "tctl"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("binary %q not found in %s: %w", name, dir, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%q is not a regular file", path)
		}
	}
	return nil
}

// copyBinary copies a file from src to dst with executable permissions (0755).
func copyBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
