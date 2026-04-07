package database

import (
	"fmt"
	"os"
	"path/filepath"

	"dbtest/docker"
	"dbtest/teleport"
)

const mariadbDockerfile = `FROM mariadb:11

COPY mariadb.cas /certs/mariadb.cas
COPY mariadb.crt /certs/mariadb.crt
COPY mariadb.key /certs/mariadb.key
COPY my.cnf /etc/mysql/conf.d/my.cnf
COPY init.sql /docker-entrypoint-initdb.d/init.sql
RUN chown -R mysql:mysql \
    /certs \
    /etc/mysql/conf.d/my.cnf \
    /docker-entrypoint-initdb.d

EXPOSE 3306

ENV MARIADB_ROOT_PASSWORD=root
`

const mariadbMyCnf = `[mariadb]
require_secure_transport=ON
ssl-ca=/certs/mariadb.cas
ssl-cert=/certs/mariadb.crt
ssl-key=/certs/mariadb.key
`

const mariadbInitSQL = `CREATE DATABASE ` + "`teleport`" + `;
CREATE DATABASE ` + "`public`" + `;

CREATE USER 'teleport-admin'@'%' REQUIRE SUBJECT '/CN=teleport-admin';
GRANT PROCESS, CREATE USER ON *.* TO 'teleport-admin';
GRANT SELECT ON mysql.roles_mapping TO 'teleport-admin';
GRANT UPDATE ON mysql.* TO 'teleport-admin';
GRANT SELECT ON *.* TO 'teleport-admin';
GRANT ALL ON ` + "`teleport`" + `.* TO 'teleport-admin' WITH GRANT OPTION;

CREATE ROLE creator WITH ADMIN 'teleport-admin';
GRANT CREATE ON ` + "`public`" + `.* TO creator;
GRANT SELECT ON ` + "`public`" + `.* TO creator;

CREATE TABLE public.example_table (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);
`

type MariaDB struct {
	Config Config
}

func (m *MariaDB) Up() error {
	imageName := m.Config.ClusterName + "-" + m.Config.Name
	dir := m.Config.WorkDir

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	for name, content := range map[string]string{
		"Dockerfile": mariadbDockerfile,
		"my.cnf":     mariadbMyCnf,
		"init.sql":   mariadbInitSQL,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	fmt.Println("generating mariadb certificates...")
	if err := teleport.AuthSign(dir, "db", m.Config.ClusterName+"-"+m.Config.Name, "mariadb", "2160h"); err != nil {
		return fmt.Errorf("generating mariadb certificates: %w", err)
	}

	fmt.Println("building mariadb image...")
	if err := docker.BuildImage(imageName, dir); err != nil {
		return fmt.Errorf("building mariadb image: %w", err)
	}

	fmt.Println("starting mariadb container...")
	if err := docker.RunContainer(docker.RunOptions{
		Network: m.Config.ClusterName,
		Name:    imageName,
		Image:   imageName,
		User:    "mysql",
	}); err != nil {
		return fmt.Errorf("starting mariadb container: %w", err)
	}

	return ensureDatabaseService(m.Config, "mariadb", nil)
}

func (m *MariaDB) Down() error {
	imageName := m.Config.ClusterName + "-" + m.Config.Name

	wipeDatabaseService(m.Config)

	fmt.Println("wiping mariadb containers...")
	docker.WipeContainers(imageName)
	fmt.Println("wiping mariadb images...")
	docker.WipeImages(imageName)

	return nil
}
