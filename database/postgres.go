package database

import (
	"fmt"
	"os"
	"path/filepath"

	"dbtest/docker"
	"dbtest/teleport"
)

const postgresDockerfile = `FROM postgres:16

ENV POSTGRES_PASSWORD=password

COPY postgres.cas /pg-certs/postgres.cas
COPY postgres.crt /pg-certs/postgres.crt
COPY postgres.key /pg-certs/postgres.key
RUN chown postgres:postgres /pg-certs/postgres.cas /pg-certs/postgres.crt /pg-certs/postgres.key \
    && chmod 600 /pg-certs/postgres.key

COPY pg_hba.conf /etc/postgresql/pg_hba.conf

EXPOSE 5432
`

const postgresHBAConf = `# TYPE  DATABASE  USER  ADDRESS    METHOD
local   all       all              trust
hostssl all       all   ::/0       cert
hostssl all       all   0.0.0.0/0  cert
`

type Postgres struct {
	Config Config
}

func (p *Postgres) Up() error {
	imageName := p.Config.ClusterName + "-" + p.Config.Name
	dir := p.Config.WorkDir

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(postgresDockerfile), 0644); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "pg_hba.conf"), []byte(postgresHBAConf), 0644); err != nil {
		return fmt.Errorf("writing pg_hba.conf: %w", err)
	}

	fmt.Println("generating postgres certificates...")
	if err := teleport.AuthSign(dir, "db", p.Config.ClusterName+"-"+p.Config.Name, "postgres", "2160h"); err != nil {
		return fmt.Errorf("generating postgres certificates: %w", err)
	}

	fmt.Println("building postgres image...")
	if err := docker.BuildImage(imageName, dir); err != nil {
		return fmt.Errorf("building postgres image: %w", err)
	}

	fmt.Println("starting postgres container...")
	if err := docker.RunContainer(docker.RunOptions{
		Network: p.Config.ClusterName,
		Name:    imageName,
		Image:   imageName,
		Cmd: []string{
			"postgres",
			"-c", "ssl=on",
			"-c", "ssl_cert_file=/pg-certs/postgres.crt",
			"-c", "ssl_key_file=/pg-certs/postgres.key",
			"-c", "ssl_ca_file=/pg-certs/postgres.cas",
			"-c", "hba_file=/etc/postgresql/pg_hba.conf",
			"-c", "listen_addresses=*",
		},
	}); err != nil {
		return fmt.Errorf("starting postgres container: %w", err)
	}

	return ensureDatabaseService(p.Config, "postgres", nil)
}

func (p *Postgres) Down() error {
	imageName := p.Config.ClusterName + "-" + p.Config.Name

	wipeDatabaseService(p.Config)

	fmt.Println("wiping postgres containers...")
	docker.WipeContainers(imageName)
	fmt.Println("wiping postgres images...")
	docker.WipeImages(imageName)

	return nil
}
