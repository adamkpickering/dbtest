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
COPY init.sql /docker-entrypoint-initdb.d/init.sql

EXPOSE 5432
`

const postgresInitSQL = `CREATE ROLE writer;
CREATE ROLE reader;

CREATE SCHEMA app;
CREATE SCHEMA archive;

CREATE TABLE app.users    (id INT, name TEXT, email TEXT);
CREATE TABLE app.products (id INT, name TEXT, price INT);
CREATE TABLE archive.events    (id INT, name TEXT, description TEXT);
CREATE TABLE archive.snapshots (id INT, data TEXT);

INSERT INTO app.users    VALUES (1,'Alice','alice@example.com'),(2,'Bob','bob@example.com'),(3,'Charlie','charlie@example.com');
INSERT INTO app.products VALUES (1,'Widget',9),(2,'Gadget',42),(3,'Doohickey',7);
INSERT INTO archive.events    VALUES (1,'launch','Product launch'),(2,'update','Software update'),(3,'maintenance','Scheduled maintenance');
INSERT INTO archive.snapshots VALUES (1,'snapshot-a'),(2,'snapshot-b');

GRANT USAGE, CREATE ON SCHEMA app TO writer;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA app TO writer;

GRANT USAGE ON SCHEMA app TO reader;
GRANT SELECT ON ALL TABLES IN SCHEMA app TO reader;

CREATE USER "teleport-admin" SUPERUSER;
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

	if err := os.WriteFile(filepath.Join(dir, "init.sql"), []byte(postgresInitSQL), 0644); err != nil {
		return fmt.Errorf("writing init.sql: %w", err)
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
