package docker

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Output is the writer for subcommand stdout/stderr. Set to io.Discard to suppress output.
var Output io.Writer = os.Stdout

type ContainerInfo struct {
	ID    string `json:"ID"`
	Image string `json:"Image"`
	Names string `json:"Names"`
}

type ImageInfo struct {
	ID         string `json:"ID"`
	Repository string `json:"Repository"`
}

type RunOptions struct {
	Network string
	Name    string
	Image   string
	User    string
	Cmd     []string
}

func run(args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = Output
	cmd.Stderr = Output
	return cmd.Run()
}

func NetworkCreate(name string) {
	fmt.Printf("creating docker network %q...\n", name)
	run("network", "create", "--driver", "bridge", name)
}

func NetworkRemove(name string) {
	fmt.Printf("removing docker network %q...\n", name)
	run("network", "rm", name)
}

func BuildImage(tag, contextDir string) error {
	return run("build", "--tag", tag, contextDir)
}

func RunContainer(opts RunOptions) error {
	args := []string{"run", "--detach", "--network", opts.Network, "--name", opts.Name}
	if opts.User != "" {
		args = append(args, "--user", opts.User)
	}
	args = append(args, opts.Image)
	args = append(args, opts.Cmd...)
	return run(args...)
}

func RunContainerWithVolume(network, name, image, volume string, cmd ...string) error {
	args := []string{"run", "--rm", "--network", network, "--name", name, "-v", volume}
	args = append(args, image)
	args = append(args, cmd...)
	return run(args...)
}

func WipeContainers(namePattern string) {
	out, err := exec.Command("docker", "container", "ls", "--all", "--format", "json").Output()
	if err != nil {
		return
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var c ContainerInfo
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		if strings.Contains(c.Image, namePattern) || strings.Contains(c.Names, namePattern) {
			run("container", "stop", c.ID)
			run("container", "rm", c.ID)
		}
	}
}

func WipeImages(namePattern string) {
	out, err := exec.Command("docker", "image", "ls", "--format", "json").Output()
	if err != nil {
		return
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var img ImageInfo
		if err := json.Unmarshal([]byte(line), &img); err != nil {
			continue
		}
		if strings.Contains(img.Repository, namePattern) {
			run("image", "rm", img.ID)
		}
	}
}
