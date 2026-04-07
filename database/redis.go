package database

import (
	"fmt"
	"os"
	"path/filepath"

	"dbtest/docker"
	"dbtest/teleport"
)

const redisDockerfile = `FROM redis:7.2.3

USER root

COPY redis.conf users.acl /usr/local/etc/redis/
COPY redis.cas /usr/local/etc/redis/certs/redis.cas
COPY redis.crt /usr/local/etc/redis/certs/redis.crt
COPY redis.key /usr/local/etc/redis/certs/redis.key
RUN chown -R redis:redis /usr/local/etc/redis/

USER redis

CMD [ "redis-server", "/usr/local/etc/redis/redis.conf" ]
`

const redisConf = `tls-port 6379
port 0
aclfile /usr/local/etc/redis/users.acl
tls-ca-cert-file /usr/local/etc/redis/certs/redis.cas
tls-cert-file /usr/local/etc/redis/certs/redis.crt
tls-key-file /usr/local/etc/redis/certs/redis.key
tls-protocols "TLSv1.2 TLSv1.3"
`

const redisUsersACL = `user alice on #42a9798b99d4afcec9995e47a1d246b98ebc96be7a732323eee39d924006ee1d allcommands allkeys
user default off
`

type Redis struct {
	Config Config
}

func (r *Redis) Up() error {
	imageName := r.Config.ClusterName + "-" + r.Config.Name
	dir := r.Config.WorkDir

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	for name, content := range map[string]string{
		"Dockerfile": redisDockerfile,
		"redis.conf": redisConf,
		"users.acl":  redisUsersACL,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	fmt.Println("generating redis certificates...")
	if err := teleport.AuthSign(dir, "redis", r.Config.ClusterName+"-"+r.Config.Name, "redis", "2160h"); err != nil {
		return fmt.Errorf("generating redis certificates: %w", err)
	}

	fmt.Println("building redis image...")
	if err := docker.BuildImage(imageName, dir); err != nil {
		return fmt.Errorf("building redis image: %w", err)
	}

	fmt.Println("starting redis container...")
	if err := docker.RunContainer(docker.RunOptions{
		Network: r.Config.ClusterName,
		Name:    imageName,
		Image:   imageName,
	}); err != nil {
		return fmt.Errorf("starting redis container: %w", err)
	}

	return ensureDatabaseService(r.Config, "redis", nil)
}

func (r *Redis) Down() error {
	imageName := r.Config.ClusterName + "-" + r.Config.Name

	wipeDatabaseService(r.Config)

	fmt.Println("wiping redis containers...")
	docker.WipeContainers(imageName)
	fmt.Println("wiping redis images...")
	docker.WipeImages(imageName)

	return nil
}
