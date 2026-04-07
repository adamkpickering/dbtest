// Package cluster manages the state of test clusters in ~/.teleport-dbtest/.
// A cluster is a named Docker network plus a set of test databases (and
// potentially other Teleport services) that are associated with it.
// This package is purely a state-management layer: it reads and writes
// cluster.json files but has no knowledge of Docker, Teleport, or databases.
package cluster

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const baseDir = ".teleport-dbtest"

// State is the persisted state for a single cluster, stored in
// ~/.teleport-dbtest/<name>/cluster.json.
type State struct {
	Name        string `json:"name"`
	ProxyServer string `json:"proxy_server"`
}

// Dir returns the absolute path to the cluster's state directory.
func Dir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, baseDir, name), nil
}

// BaseDir returns the absolute path to ~/.teleport-dbtest/.
func BaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, baseDir), nil
}

// DBDir returns the absolute path to the per-database subdirectory within
// the cluster's state directory.
func DBDir(clusterName, dbType string) (string, error) {
	base, err := Dir(clusterName)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, dbType), nil
}

// DBServiceDir returns the absolute path to the database service subdirectory
// for a given database type within the cluster's state directory.
func DBServiceDir(clusterName, dbType string) (string, error) {
	base, err := Dir(clusterName)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, dbType+"-database-service"), nil
}

// BinariesDir returns the absolute path to the stored Teleport binaries
// directory for the cluster (~/.teleport-dbtest/<cluster>/teleport-binaries/).
func BinariesDir(clusterName string) (string, error) {
	base, err := Dir(clusterName)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "teleport-binaries"), nil
}

// Create initializes a new cluster by creating its state directory and writing
// an initial cluster.json. Returns an error if a cluster with that name already exists.
func Create(s *State) error {
	dir, err := Dir(s.Name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("cluster %q already exists at %s", s.Name, dir)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating cluster directory: %w", err)
	}
	if err := save(dir, s); err != nil {
		os.RemoveAll(dir)
		return err
	}
	return nil
}

// Load reads and returns the state for the named cluster.
// Returns an error if the cluster does not exist.
func Load(name string) (*State, error) {
	dir, err := Dir(name)
	if err != nil {
		return nil, err
	}
	return loadFromDir(dir)
}

func loadFromDir(dir string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(dir, "cluster.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cluster not found at %s", dir)
		}
		return nil, fmt.Errorf("reading cluster state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing cluster state: %w", err)
	}
	return &s, nil
}

// List returns the state of all clusters found under ~/.teleport-dbtest/.
// Directories that don't contain a valid cluster.json are silently skipped.
func List() ([]*State, error) {
	base, err := BaseDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state directory: %w", err)
	}
	var clusters []*State
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		s, err := loadFromDir(filepath.Join(base, entry.Name()))
		if err != nil {
			continue
		}
		clusters = append(clusters, s)
	}
	return clusters, nil
}

// Remove deletes the cluster's state directory entirely.
// Callers are responsible for tearing down all Docker resources before calling this.
func Remove(name string) error {
	dir, err := Dir(name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing cluster directory: %w", err)
	}
	return nil
}

func save(dir string, s *State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing cluster state: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cluster.json"), data, 0644); err != nil {
		return fmt.Errorf("writing cluster state: %w", err)
	}
	return nil
}
