package database

// Config holds configuration for a single database instance within a cluster.
type Config struct {
	ClusterName    string // Docker network name and container name prefix
	Name           string // Instance name; used to form container/image names: <ClusterName>-<Name>
	ProxyServer    string // Teleport proxy server address
	BinariesDir    string // Persistent dir: ~/.teleport-dbtest/<cluster>/teleport-binaries/
	WorkDir        string // Persistent dir: ~/.teleport-dbtest/<cluster>/<name>/
	ServiceWorkDir string // Persistent dir: ~/.teleport-dbtest/<cluster>/<name>-database-service/
}

// Database represents a test database with an associated Teleport database service.
type Database interface {
	// Up creates the image, container, and any associated files/data for the database
	// and its Teleport database service.
	Up() error

	// Down removes the image, container, and any associated files/data for the database
	// and its Teleport database service. This method is idempotent.
	Down() error
}
