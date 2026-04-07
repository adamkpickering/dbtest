package database

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"dbtest/docker"
	"dbtest/teleport"
)

const redisClusterDockerfile = `FROM redis:7.2.3

USER root

COPY redis.conf users.acl /usr/local/etc/redis/
COPY combined.crt /usr/local/etc/redis/certs/combined.crt
COPY node.crt /usr/local/etc/redis/certs/node.crt
COPY node.key /usr/local/etc/redis/certs/node.key
RUN chown -R redis:redis /usr/local/etc/redis/

USER redis

CMD [ "redis-server", "/usr/local/etc/redis/redis.conf" ]
`

const redisClusterConf = `bind 0.0.0.0
port 0
tls-port 6379

tls-cert-file /usr/local/etc/redis/certs/node.crt
tls-key-file /usr/local/etc/redis/certs/node.key
tls-ca-cert-file /usr/local/etc/redis/certs/combined.crt

tls-cluster yes
tls-replication yes

cluster-enabled yes
cluster-config-file nodes.conf
cluster-node-timeout 5000

aclfile /usr/local/etc/redis/users.acl
`

const redisClusterUsersACL = `user alice on #9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 allcommands allkeys
user default off
`

const redisClusterSSLConfTpl = `[default]
SANS = DNS:localhost,IP:127.0.0.1

[req]
default_bits        = 2048
distinguished_name  = req_distinguished_name
string_mask         = utf8only
default_md          = sha256
x509_extensions     = v3_ca

[req_distinguished_name]
countryName                     = Country Name (2 letter code)
stateOrProvinceName             = State or Province Name
localityName                    = Locality Name
0.organizationName              = Organization Name
organizationalUnitName          = Organizational Unit Name
commonName                      = Common Name
emailAddress                    = Email Address

countryName_default             = US
stateOrProvinceName_default     = USA
localityName_default            =
0.organizationName_default      = Teleport
commonName_default              = localhost

[v3_ca]
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer
basicConstraints = critical, CA:true, pathlen: 0
keyUsage = critical, cRLSign, keyCertSign

[client_cert]
basicConstraints = CA:FALSE
nsCertType = client
nsComment = "OpenSSL Generated Client Certificate"
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
keyUsage = critical, nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
subjectAltName = SANS_SUBSTITUTE

[server_and_client_cert]
basicConstraints = CA:FALSE
nsCertType = server, client
nsComment = "OpenSSL Generated Server/Client Certificate"
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer:always
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = SANS_SUBSTITUTE
`

type RedisCluster struct {
	Config Config
}

func openssl(dir string, args ...string) {
	cmd := exec.Command("openssl", args...)
	cmd.Dir = dir
	// suppress openssl output to keep things tidy, like the nushell script does
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Run()
}

func (r *RedisCluster) Up() error {
	imageBaseName := r.Config.ClusterName + "-" + r.Config.Name
	nodeNames := make([]string, 6)
	for i := range nodeNames {
		nodeNames[i] = fmt.Sprintf("%s-node-%d", imageBaseName, i+1)
	}

	dir := r.Config.WorkDir
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating work dir: %w", err)
	}

	// Generate root CA cert for redis-cluster
	fmt.Println("generating root CA cert for redis-cluster...")
	caConf := strings.ReplaceAll(redisClusterSSLConfTpl, "SANS_SUBSTITUTE", "DNS:localhost,IP:127.0.0.1")
	if err := os.WriteFile(filepath.Join(dir, "ssl-ca.conf"), []byte(caConf), 0644); err != nil {
		return fmt.Errorf("writing ssl-ca.conf: %w", err)
	}
	openssl(dir, "genrsa", "-out", "ca.key", "2048")
	openssl(dir, "req", "-config", "ssl-ca.conf", "-key", "ca.key", "-new", "-x509",
		"-days", "365", "-sha256", "-extensions", "v3_ca", "-subj", "/CN=ca", "-out", "ca.crt")

	// Export teleport db-client CA cert and combine with our CA
	fmt.Println("exporting teleport db_client CA cert...")
	dbClientCA, err := teleport.AuthExport("db-client")
	if err != nil {
		return fmt.Errorf("exporting db-client CA: %w", err)
	}
	caCRT, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		return fmt.Errorf("reading ca.crt: %w", err)
	}
	combinedCRT := append(caCRT, []byte(dbClientCA)...)
	if err := os.WriteFile(filepath.Join(dir, "combined.crt"), combinedCRT, 0644); err != nil {
		return fmt.Errorf("writing combined.crt: %w", err)
	}

	// Set up each node
	for _, nodeName := range nodeNames {
		fmt.Printf("copying common files for %s...\n", nodeName)
		nodeDir := filepath.Join(dir, nodeName)
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			return fmt.Errorf("creating node dir for %s: %w", nodeName, err)
		}

		nodeConf := strings.ReplaceAll(redisClusterSSLConfTpl, "SANS_SUBSTITUTE",
			fmt.Sprintf("DNS:%s,DNS:localhost,IP:127.0.0.1", nodeName))

		for name, content := range map[string]string{
			"Dockerfile": redisClusterDockerfile,
			"redis.conf": redisClusterConf,
			"users.acl":  redisClusterUsersACL,
			"ssl.conf":   nodeConf,
		} {
			if err := os.WriteFile(filepath.Join(nodeDir, name), []byte(content), 0644); err != nil {
				return fmt.Errorf("writing %s for %s: %w", name, nodeName, err)
			}
		}
		if err := copyFile(filepath.Join(dir, "combined.crt"), filepath.Join(nodeDir, "combined.crt")); err != nil {
			return fmt.Errorf("copying combined.crt for %s: %w", nodeName, err)
		}

		fmt.Printf("setting up certs for %s...\n", nodeName)
		openssl(nodeDir, "genrsa", "-out", "node.key", "2048")
		openssl(nodeDir, "req", "-config", "ssl.conf", "-subj", "/CN="+nodeName, "-key", "node.key", "-new", "-out", "node.csr")
		openssl(nodeDir, "x509", "-req", "-in", "node.csr",
			"-CA", "../ca.crt", "-CAkey", "../ca.key", "-CAcreateserial",
			"-days", "365", "-out", "node.crt",
			"-extfile", "ssl.conf", "-extensions", "server_and_client_cert")

		fmt.Printf("building image for %s...\n", nodeName)
		if err := docker.BuildImage(nodeName, nodeDir); err != nil {
			return fmt.Errorf("building image for %s: %w", nodeName, err)
		}

		fmt.Printf("starting container for %s...\n", nodeName)
		if err := docker.RunContainer(docker.RunOptions{
			Network: r.Config.ClusterName,
			Name:    nodeName,
			Image:   nodeName,
		}); err != nil {
			return fmt.Errorf("starting container for %s: %w", nodeName, err)
		}
	}

	// Give redis nodes some time to start up
	fmt.Println("waiting for redis cluster nodes to start...")
	time.Sleep(5 * time.Second)

	// Generate client certificate for redis-cli
	fmt.Println("generating client certificate for redis-cli...")
	bootstrapName := imageBaseName + "-bootstrap"
	clientConf := strings.ReplaceAll(redisClusterSSLConfTpl, "SANS_SUBSTITUTE",
		fmt.Sprintf("DNS:%s,DNS:localhost,IP:127.0.0.1", bootstrapName))
	if err := os.WriteFile(filepath.Join(dir, "ssl-client.conf"), []byte(clientConf), 0644); err != nil {
		return fmt.Errorf("writing ssl-client.conf: %w", err)
	}
	openssl(dir, "genrsa", "-out", "client.key", "2048")
	openssl(dir, "req", "-config", "ssl-client.conf", "-subj", "/CN=redis-cli-client", "-key", "client.key", "-new", "-out", "client.csr")
	openssl(dir, "x509", "-req", "-in", "client.csr",
		"-CA", "ca.crt", "-CAkey", "ca.key", "-CAcreateserial",
		"-days", "365", "-out", "client.crt",
		"-extfile", "ssl-client.conf", "-extensions", "client_cert")

	// Build node address list for cluster create
	nodeAddrs := make([]string, len(nodeNames))
	for i, name := range nodeNames {
		nodeAddrs[i] = name + ":6379"
	}

	fmt.Println("running cluster create command...")
	clusterCreateCmd := append(
		[]string{
			"redis-cli", "--tls",
			"--cacert", "/tls/ca.crt",
			"--cert", "/tls/client.crt",
			"--key", "/tls/client.key",
			"--user", "alice", "-a", "test",
			"--cluster", "create",
		},
		append(nodeAddrs, "--cluster-replicas", "1", "--cluster-yes")...,
	)
	if err := docker.RunContainerWithVolume(
		r.Config.ClusterName,
		bootstrapName,
		"redis:7.2.3",
		dir+":/tls:ro",
		clusterCreateCmd...,
	); err != nil {
		return fmt.Errorf("running cluster create: %w", err)
	}

	return ensureDatabaseService(r.Config, "redis-cluster", combinedCRT)
}

func (r *RedisCluster) Down() error {
	imageBaseName := r.Config.ClusterName + "-" + r.Config.Name

	wipeDatabaseService(r.Config)

	fmt.Println("wiping redis-cluster containers...")
	docker.WipeContainers(imageBaseName)
	fmt.Println("wiping redis-cluster images...")
	docker.WipeImages(imageBaseName)

	return nil
}
