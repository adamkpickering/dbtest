package database

import (
	"fmt"
	"os"
	"path/filepath"

	"dbtest/docker"
	"dbtest/teleport"
)

const cockroachdbDockerfile = `FROM cockroachdb/cockroach:v26.1.1

COPY --chmod=600 certs /certs/certs

# root won't be able to connect unless we provide a client.root.[crt|key].
# Therefore, we create a custom client CA using ` + "`cockroach cert create-client-ca`" + `.
# That command, with --overwrite, will prepend the custom client CA to client-ca.crt.
# This way, root can auth and init will succeed.
RUN /cockroach/cockroach cert create-client-ca \
    --certs-dir=/certs/certs \
    --ca-key=/certs/certs/ca-client.key \
    --overwrite

# make client.root.[crt|key]
RUN /cockroach/cockroach cert create-client root \
    --certs-dir=/certs/certs \
    --ca-key=/certs/certs/ca-client.key

# make client.node.[crt|key]
RUN /cockroach/cockroach cert create-client node \
    --certs-dir=/certs/certs \
    --ca-key=/certs/certs/ca-client.key

EXPOSE 26257

CMD [ "start-single-node", "--certs-dir=/certs/certs" ]
`

type CockroachDB struct {
	Config Config
}

func (c *CockroachDB) Up() error {
	imageName := c.Config.ClusterName + "-" + c.Config.Name
	dir := c.Config.WorkDir

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(cockroachdbDockerfile), 0644); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}

	// tctl auth sign --format=cockroachdb --out=certs creates a certs/ directory
	if err := os.MkdirAll(filepath.Join(dir, "certs"), 0755); err != nil {
		return fmt.Errorf("creating certs dir: %w", err)
	}

	fmt.Println("generating cockroachdb certificates...")
	if err := teleport.AuthSign(dir, "cockroachdb", c.Config.ClusterName+"-"+c.Config.Name+",127.0.0.1", "certs", "2160h"); err != nil {
		return fmt.Errorf("generating cockroachdb certificates: %w", err)
	}

	fmt.Println("building cockroachdb image...")
	if err := docker.BuildImage(imageName, dir); err != nil {
		return fmt.Errorf("building cockroachdb image: %w", err)
	}

	fmt.Println("starting cockroachdb container...")
	if err := docker.RunContainer(docker.RunOptions{
		Network: c.Config.ClusterName,
		Name:    imageName,
		Image:   imageName,
	}); err != nil {
		return fmt.Errorf("starting cockroachdb container: %w", err)
	}

	return ensureDatabaseService(c.Config, "cockroachdb", nil)
}

func (c *CockroachDB) Down() error {
	imageName := c.Config.ClusterName + "-" + c.Config.Name

	wipeDatabaseService(c.Config)

	fmt.Println("wiping cockroachdb containers...")
	docker.WipeContainers(imageName)
	fmt.Println("wiping cockroachdb images...")
	docker.WipeImages(imageName)

	return nil
}
