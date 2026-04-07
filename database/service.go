package database

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"dbtest/docker"
	"dbtest/teleport"
)

const databaseServiceDockerfile = `FROM ubuntu:24.04

# Installing curl is critical because it sets up a directory we
# later use for certificates
RUN apt-get update && apt-get install -y curl

COPY teleport /usr/local/bin/teleport
COPY tsh /usr/local/bin/tsh
COPY tctl /usr/local/bin/tctl
COPY teleport.yaml /etc/teleport.yaml
COPY ca.crt /tls/ca.crt

CMD ["teleport", "start"]
`

type serviceParams struct {
	DBName      string // instance name: used for nodename and database registration
	DBType      string // engine type: used to determine protocol
	JoinToken   string
	ProxyServer string
	Protocol    string
	URI         string
	CACertFile  string
}

func generateTeleportYAML(p serviceParams) string {
	yaml := fmt.Sprintf(`version: v3
teleport:
  nodename: %s-database-service
  join_params:
    token_name: %s
    method: token
  proxy_server: %s
  log:
    output: stderr
    severity: DEBUG
    format:
      output: text
auth_service:
  enabled: false
proxy_service:
  enabled: false
ssh_service:
  enabled: true
db_service:
  enabled: true
  databases:
    - name: %s
      protocol: %s
      uri: %s
`, p.DBName, p.JoinToken, p.ProxyServer, p.DBName, p.Protocol, p.URI)

	if p.CACertFile != "" {
		yaml += fmt.Sprintf(`      tls:
        ca_cert_file: %s
`, p.CACertFile)
	}

	yaml += `      admin_user:
        name: teleport-admin
`
	return yaml
}

// ensureDatabaseService generates a join token, templates teleport.yaml,
// copies Teleport binaries from the cluster's stored binaries directory,
// builds a database service image, and runs it.
// Persistent config files (teleport.yaml, ca.crt, Dockerfile) are written to
// cfg.ServiceWorkDir. Teleport binaries are copied into an ephemeral temp
// dir and discarded after the image is built.
// caCertContent is the content to write to ca.crt; pass nil for databases that
// don't need a custom CA cert.
func ensureDatabaseService(cfg Config, dbType string, caCertContent []byte) error {
	imageName := cfg.ClusterName + "-" + cfg.Name + "-database-service"

	// Persistent directory for config files that are useful to inspect later.
	svcDir := cfg.ServiceWorkDir
	if err := os.MkdirAll(svcDir, 0755); err != nil {
		return fmt.Errorf("creating service work dir: %w", err)
	}

	fmt.Printf("generating join token for %s database service...\n", cfg.Name)
	joinToken, err := teleport.TokensAdd("db,node", "100h")
	if err != nil {
		return fmt.Errorf("generating join token: %w", err)
	}

	protocol := dbType
	switch dbType {
	case "mariadb":
		protocol = "mysql"
	case "redis-cluster":
		protocol = "redis"
	}

	// The database container is named <clusterName>-<instanceName>. For
	// redis-cluster the service connects to node-1 of the cluster.
	var uri string
	if dbType == "redis-cluster" {
		uri = fmt.Sprintf("%s-%s-node-1:6379", cfg.ClusterName, cfg.Name)
	} else {
		ports := map[string]string{
			"postgres":    "5432",
			"mysql":       "3306",
			"mariadb":     "3306",
			"mongodb":     "27017",
			"cockroachdb": "26257",
			"redis":       "6379",
		}
		uri = fmt.Sprintf("%s-%s:%s", cfg.ClusterName, cfg.Name, ports[dbType])
	}

	var caCertFile string
	if caCertContent != nil {
		caCertFile = "/tls/ca.crt"
	}

	params := serviceParams{
		DBName:      cfg.Name,
		DBType:      dbType,
		JoinToken:   joinToken,
		ProxyServer: cfg.ProxyServer,
		Protocol:    protocol,
		URI:         uri,
		CACertFile:  caCertFile,
	}

	fmt.Printf("templating out teleport config for %s database service...\n", dbType)
	teleportYAML := generateTeleportYAML(params)

	if caCertContent == nil {
		caCertContent = []byte{}
	}

	// Write persistent config files to svcDir.
	for name, content := range map[string][]byte{
		"Dockerfile":    []byte(databaseServiceDockerfile),
		"teleport.yaml": []byte(teleportYAML),
		"ca.crt":        caCertContent,
	} {
		if err := os.WriteFile(filepath.Join(svcDir, name), content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	// Create an ephemeral build context. The Teleport binaries are large; we
	// copy them in from the cluster's stored binaries directory and discard
	// them after the image is built.
	buildDir, err := os.MkdirTemp("", "teldev-svc-build-*")
	if err != nil {
		return fmt.Errorf("creating build temp dir: %w", err)
	}
	defer os.RemoveAll(buildDir)

	fmt.Printf("copying teleport binaries for %s database service...\n", cfg.Name)
	for _, name := range []string{"teleport", "tsh", "tctl"} {
		src := filepath.Join(cfg.BinariesDir, name)
		dst := filepath.Join(buildDir, name)
		if err := copyExecutable(src, dst); err != nil {
			return fmt.Errorf("copying binary %s to build dir: %w", name, err)
		}
	}

	// Copy persistent config files into the ephemeral build context so that
	// docker build has access to everything in one directory.
	for _, name := range []string{"Dockerfile", "teleport.yaml", "ca.crt"} {
		if err := copyFile(filepath.Join(svcDir, name), filepath.Join(buildDir, name)); err != nil {
			return fmt.Errorf("copying %s to build dir: %w", name, err)
		}
	}

	fmt.Printf("building %s database service image...\n", dbType)
	if err := docker.BuildImage(imageName, buildDir); err != nil {
		return fmt.Errorf("building database service image: %w", err)
	}

	fmt.Printf("starting %s database service container...\n", dbType)
	return docker.RunContainer(docker.RunOptions{
		Network: cfg.ClusterName,
		Name:    imageName,
		Image:   imageName,
	})
}

func wipeDatabaseService(cfg Config) {
	imageName := cfg.ClusterName + "-" + cfg.Name + "-database-service"
	fmt.Printf("wiping %s database service containers...\n", cfg.Name)
	docker.WipeContainers(imageName)
	fmt.Printf("wiping %s database service images...\n", cfg.Name)
	docker.WipeImages(imageName)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyExecutable(src, dst string) error {
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
