package database

import (
	"fmt"
	"os"
	"path/filepath"

	"dbtest/docker"
	"dbtest/teleport"
)

const mysqlDockerfile = `FROM mysql:8.3

COPY mysql.cas /certs/mysql.cas
COPY mysql.crt /certs/mysql.crt
COPY mysql.key /certs/mysql.key
COPY my.cnf /etc/mysql/conf.d/my.cnf
COPY init.sql /docker-entrypoint-initdb.d/init.sql
RUN chown -R mysql:mysql \
    /certs \
    /etc/mysql/conf.d/my.cnf \
    /docker-entrypoint-initdb.d

EXPOSE 3306

ENV MYSQL_ROOT_PASSWORD=root
CMD [ "--default-authentication-plugin=mysql_native_password" ]
`

const mysqlMyCnf = `[mysqld]
require_secure_transport=ON
ssl_ca=/certs/mysql.cas
ssl_cert=/certs/mysql.crt
ssl_key=/certs/mysql.key
`

const mysqlInitSQL = `CREATE DATABASE teleport;
CREATE DATABASE app;
CREATE DATABASE archive;

CREATE TABLE app.users    (id INT, name TEXT, email TEXT);
CREATE TABLE app.products (id INT, name TEXT, price INT);
CREATE TABLE archive.events    (id INT, name TEXT, description TEXT);
CREATE TABLE archive.snapshots (id INT, data TEXT);

INSERT INTO app.users    VALUES (1,'Alice','alice@example.com'),(2,'Bob','bob@example.com'),(3,'Charlie','charlie@example.com');
INSERT INTO app.products VALUES (1,'Widget',9),(2,'Gadget',42),(3,'Doohickey',7);
INSERT INTO archive.events    VALUES (1,'launch','Product launch'),(2,'update','Software update'),(3,'maintenance','Scheduled maintenance');
INSERT INTO archive.snapshots VALUES (1,'snapshot-a'),(2,'snapshot-b');

CREATE ROLE 'writer';
GRANT CREATE, SELECT, INSERT, UPDATE, DELETE ON app.* TO 'writer';

CREATE ROLE 'reader';
GRANT SELECT ON app.* TO 'reader';

CREATE USER 'teleport-admin'@'%' REQUIRE SUBJECT '/CN=teleport-admin';
GRANT ALL PRIVILEGES ON *.* TO 'teleport-admin'@'%' WITH GRANT OPTION;
`

type MySQL struct {
	Config Config
}

func (m *MySQL) Up() error {
	imageName := m.Config.ClusterName + "-" + m.Config.Name
	dir := m.Config.WorkDir

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	for name, content := range map[string]string{
		"Dockerfile": mysqlDockerfile,
		"my.cnf":     mysqlMyCnf,
		"init.sql":   mysqlInitSQL,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	fmt.Println("generating mysql certificates...")
	if err := teleport.AuthSign(dir, "db", m.Config.ClusterName+"-"+m.Config.Name, "mysql", "2160h"); err != nil {
		return fmt.Errorf("generating mysql certificates: %w", err)
	}

	fmt.Println("building mysql image...")
	if err := docker.BuildImage(imageName, dir); err != nil {
		return fmt.Errorf("building mysql image: %w", err)
	}

	fmt.Println("starting mysql container...")
	if err := docker.RunContainer(docker.RunOptions{
		Network: m.Config.ClusterName,
		Name:    imageName,
		Image:   imageName,
		User:    "mysql",
	}); err != nil {
		return fmt.Errorf("starting mysql container: %w", err)
	}

	return ensureDatabaseService(m.Config, "mysql", nil)
}

func (m *MySQL) Down() error {
	imageName := m.Config.ClusterName + "-" + m.Config.Name

	wipeDatabaseService(m.Config)

	fmt.Println("wiping mysql containers...")
	docker.WipeContainers(imageName)
	fmt.Println("wiping mysql images...")
	docker.WipeImages(imageName)

	return nil
}
