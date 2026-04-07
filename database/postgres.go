package database

import (
	"fmt"
	"os"
	"path/filepath"

	"dbtest/docker"
	"dbtest/teleport"
)

const postgresDockerfile = `FROM ubuntu:24.04

# Installing curl is critical because it sets up a directory we
# later use for certificates
RUN apt-get update && apt-get install -y curl vim postgresql

COPY postgres.cas /pg-certs/postgres.cas
COPY postgres.crt /pg-certs/postgres.crt
COPY postgres.key /pg-certs/postgres.key
RUN chown postgres /pg-certs/postgres.cas /pg-certs/postgres.crt /pg-certs/postgres.key

RUN sed -i "/^# configuration parameter, or via the -i or -h command line switches./a\hostssl all             all             ::/0                    cert\nhostssl all             all             0.0.0.0/0               cert" /etc/postgresql/16/main/pg_hba.conf
RUN sed -i "\$a\ssl = on\nssl_cert_file = '/pg-certs/postgres.crt'\nssl_key_file = '/pg-certs/postgres.key'\nssl_ca_file = '/pg-certs/postgres.cas'" /etc/postgresql/16/main/postgresql.conf
RUN sed -i "/^#listen_addresses = 'localhost'/i\listen_addresses = '*'" /etc/postgresql/16/main/postgresql.conf

EXPOSE 5432
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
		User:    "postgres",
		Cmd:     []string{"/usr/lib/postgresql/16/bin/postgres", "--config-file=/etc/postgresql/16/main/postgresql.conf"},
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
