package database

import (
	"fmt"
	"os"
	"path/filepath"

	"dbtest/docker"
	"dbtest/teleport"
)

const mongodbDockerfile = `FROM mongo:5

COPY mongodb.cas /certs/mongodb.cas
COPY mongodb.crt /certs/mongodb.crt
COPY mongod.conf /etc/mongo/mongod.conf
COPY init.js /docker-entrypoint-initdb.d/init.js
RUN chown -R mongodb:mongodb \
    /certs \
    /etc/mongo/mongod.conf \
    /docker-entrypoint-initdb.d

EXPOSE 27017

CMD [ "--config", "/etc/mongo/mongod.conf" ]
`

const mongodbMongodConf = `net:
  tls:
    mode: requireTLS
    certificateKeyFile: /certs/mongodb.crt
    CAFile: /certs/mongodb.cas

security:
  authorization: enabled
  clusterAuthMode: x509
`

const mongodbInitJS = `db.getSiblingDB("admin").runCommand({
  createRole: "teleport-admin-role",
  privileges: [{ resource: { anyResource: true }, actions: ["anyAction"] }],
  roles: [],
});
db.getSiblingDB("$external").runCommand({
  createUser: "CN=teleport-admin",
  roles: [{ role: "teleport-admin-role", db: "admin" }],
});

const appDb = db.getSiblingDB("app");
appDb.users.insertMany([
  { id: 1, name: "Alice",   email: "alice@example.com"   },
  { id: 2, name: "Bob",     email: "bob@example.com"     },
  { id: 3, name: "Charlie", email: "charlie@example.com" },
]);
appDb.products.insertMany([
  { id: 1, name: "Widget",    price: 9  },
  { id: 2, name: "Gadget",    price: 42 },
  { id: 3, name: "Doohickey", price: 7  },
]);
appDb.runCommand({
  createRole: "writer",
  privileges: [],
  roles: [
    { role: "readWrite", db: "app" },
    { role: "dbAdmin",   db: "app" },
  ],
});
appDb.runCommand({
  createRole: "reader",
  privileges: [],
  roles: [{ role: "read", db: "app" }],
});

const archiveDb = db.getSiblingDB("archive");
archiveDb.events.insertMany([
  { id: 1, name: "launch",      description: "Product launch"        },
  { id: 2, name: "update",      description: "Software update"       },
  { id: 3, name: "maintenance", description: "Scheduled maintenance" },
]);
archiveDb.snapshots.insertMany([
  { id: 1, data: "snapshot-a" },
  { id: 2, data: "snapshot-b" },
]);
`

type MongoDB struct {
	Config Config
}

func (m *MongoDB) Up() error {
	imageName := m.Config.ClusterName + "-" + m.Config.Name
	dir := m.Config.WorkDir

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	for name, content := range map[string]string{
		"Dockerfile":  mongodbDockerfile,
		"mongod.conf": mongodbMongodConf,
		"init.js":     mongodbInitJS,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	fmt.Println("generating mongodb certificates...")
	if err := teleport.AuthSign(dir, "mongodb", m.Config.ClusterName+"-"+m.Config.Name, "mongodb", "2160h"); err != nil {
		return fmt.Errorf("generating mongodb certificates: %w", err)
	}

	fmt.Println("building mongodb image...")
	if err := docker.BuildImage(imageName, dir); err != nil {
		return fmt.Errorf("building mongodb image: %w", err)
	}

	fmt.Println("starting mongodb container...")
	if err := docker.RunContainer(docker.RunOptions{
		Network: m.Config.ClusterName,
		Name:    imageName,
		Image:   imageName,
		User:    "mongodb",
	}); err != nil {
		return fmt.Errorf("starting mongodb container: %w", err)
	}

	return ensureDatabaseService(m.Config, "mongodb", nil)
}

func (m *MongoDB) Down() error {
	imageName := m.Config.ClusterName + "-" + m.Config.Name

	wipeDatabaseService(m.Config)

	fmt.Println("wiping mongodb containers...")
	docker.WipeContainers(imageName)
	fmt.Println("wiping mongodb images...")
	docker.WipeImages(imageName)

	return nil
}
