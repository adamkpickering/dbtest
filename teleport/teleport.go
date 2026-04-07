package teleport

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// Output is the writer for subcommand stdout/stderr. Set to io.Discard to suppress output.
var Output io.Writer = os.Stdout

// LoginInfo holds the active Teleport session details.
type LoginInfo struct {
	Username  string
	Cluster   string
	ProxyHost string // hostname of the proxy (no port), for cluster matching
}

// CheckLogin verifies that the user is logged into a Teleport cluster via tsh
// and returns the active session details.
func CheckLogin() (LoginInfo, error) {
	out, err := exec.Command("tsh", "status", "--format=json").Output()
	if err != nil {
		return LoginInfo{}, fmt.Errorf("not logged into a Teleport cluster (run 'tsh login'): %w", err)
	}
	var status struct {
		Active struct {
			ProfileURL string `json:"profile_url"`
			Username   string `json:"username"`
			Cluster    string `json:"cluster"`
		} `json:"active"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return LoginInfo{}, fmt.Errorf("parsing tsh status output: %w", err)
	}
	if status.Active.Username == "" {
		return LoginInfo{}, fmt.Errorf("not logged into a Teleport cluster (run 'tsh login')")
	}
	proxyHost := status.Active.Cluster
	if u, err := url.Parse(status.Active.ProfileURL); err == nil && u.Hostname() != "" {
		proxyHost = u.Hostname()
	}
	return LoginInfo{
		Username:  status.Active.Username,
		Cluster:   status.Active.Cluster,
		ProxyHost: proxyHost,
	}, nil
}

// AuthSign runs tctl auth sign in the given directory.
// The --out flag specifies a filename prefix; tctl will create <out>.crt, <out>.key, <out>.cas
// (or a directory for cockroachdb format) relative to dir.
func AuthSign(dir, format, host, out, ttl string) error {
	cmd := exec.Command("tctl", "auth", "sign",
		"--format="+format,
		"--host="+host,
		"--out="+out,
		"--ttl="+ttl)
	cmd.Dir = dir
	cmd.Stdout = Output
	cmd.Stderr = Output
	return cmd.Run()
}

// AuthExport runs tctl auth export and returns the output.
func AuthExport(exportType string) (string, error) {
	out, err := exec.Command("tctl", "auth", "export", "--type="+exportType).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// TokensAdd creates a join token and returns it as a string.
func TokensAdd(types, ttl string) (string, error) {
	out, err := exec.Command("tctl", "tokens", "add",
		"--type="+types,
		"--ttl="+ttl,
		"--format=text").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
