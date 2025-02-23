package framework

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/docker/docker/api/types/container"
	dfilter "github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	tc "github.com/testcontainers/testcontainers-go"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

const (
	DefaultCTFLogsDir = "logs/docker"
)

func IsDockerRunning() bool {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return false
	}
	defer cli.Close()

	_, err = cli.Ping(context.Background())
	return err == nil
}

func GetHost(container tc.Container) (string, error) {
	host, err := container.Host(context.Background())
	if err != nil {
		return "", err
	}
	// if localhost then force it to ipv4 localhost
	if host == "localhost" {
		host = "127.0.0.1"
	}
	return host, nil
}

func MapTheSamePort(ports ...string) nat.PortMap {
	portMap := nat.PortMap{}
	for _, port := range ports {
		// need to split off /tcp or /udp
		onlyPort := strings.SplitN(port, "/", 2)
		portMap[nat.Port(port)] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: onlyPort[0],
			},
		}
	}
	return portMap
}

func DefaultTCLabels() map[string]string {
	return map[string]string{
		"framework": "ctf",
		"logging":   "promtail",
	}
}

func DefaultTCName(name string) string {
	return fmt.Sprintf("%s-%s", name, uuid.NewString()[0:5])
}

// runCommand executes a command and prints the output.
func runCommand(name string, args ...string) error {
	L.Info().Str("Command", name).Strs("Args", args).Msg("Executing os command")
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunCommandDir executes a command in some directory and prints the output
func RunCommandDir(dir, name string, args ...string) error {
	L.Info().Str("Command", name).Strs("Args", args).Msg("Executing os command")
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.Run()
}

// DockerClient wraps a Docker API client and provides convenience methods
type DockerClient struct {
	cli *client.Client
}

// NewDockerClient creates a new instance of DockerClient
func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &DockerClient{cli: cli}, nil
}

// CopyFile copies a file into a container by name
func (dc *DockerClient) CopyFile(containerName, sourceFile, targetPath string) error {
	ctx := context.Background()
	containerID, err := dc.findContainerIDByName(ctx, containerName)
	if err != nil {
		return fmt.Errorf("failed to find container ID by name: %s", containerName)
	}
	return dc.copyToContainer(containerID, sourceFile, targetPath)
}

// findContainerIDByName finds a container ID by its name
func (dc *DockerClient) findContainerIDByName(ctx context.Context, containerName string) (string, error) {
	containers, err := dc.cli.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}
	for _, c := range containers {
		for _, name := range c.Names {
			if name == "/"+containerName {
				return c.ID, nil
			}
		}
	}
	return "", fmt.Errorf("container with name %s not found", containerName)
}

// copyToContainer copies a file into a container
func (dc *DockerClient) copyToContainer(containerID, sourceFile, targetPath string) error {
	ctx := context.Background()
	src, err := os.Open(sourceFile)
	if err != nil {
		return fmt.Errorf("could not open source file: %w", err)
	}
	defer src.Close()

	// Create a tar archive containing the file
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("could not stat source file: %w", err)
	}

	// Add file to tar
	header := &tar.Header{
		Name: info.Name(),
		Size: info.Size(),
		Mode: int64(info.Mode()),
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("could not write tar header: %w", err)
	}
	if _, err := io.Copy(tw, src); err != nil {
		return fmt.Errorf("could not write file to tar archive: %w", err)
	}
	tw.Close()

	// Copy the tar archive to the container
	err = dc.cli.CopyToContainer(ctx, containerID, targetPath, &buf, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
	})
	if err != nil {
		return fmt.Errorf("could not copy file to container: %w", err)
	}
	return nil
}

// SearchLogFile searches logfile using regex and return matches or error
func SearchLogFile(fp string, regex string) ([]string, error) {
	file, err := os.Open(fp)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	re, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}
	matches := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if re.MatchString(line) {
			L.Info().Str("Regex", regex).Msg("Log match found")
			matches = append(matches, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return matches, err
	}
	return matches, nil
}

func SaveAndCheckLogs(t *testing.T) error {
	_, err := SaveContainerLogs(fmt.Sprintf("%s-%s", DefaultCTFLogsDir, t.Name()))
	if err != nil {
		return err
	}
	err = CheckCLNodeContainerErrors()
	if err != nil {
		return err
	}
	return nil
}

// SaveContainerLogs writes all Docker container logs to some directory
func SaveContainerLogs(dir string) ([]string, error) {
	L.Info().Msg("Writing Docker containers logs")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	provider, err := tc.NewDockerProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker provider: %w", err)
	}
	containers, err := provider.Client().ContainerList(context.Background(), container.ListOptions{
		All: true,
		Filters: dfilter.NewArgs(dfilter.KeyValuePair{
			Key:   "label",
			Value: "framework=ctf",
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list Docker containers: %w", err)
	}

	eg := &errgroup.Group{}
	logFilePaths := make([]string, 0)

	for _, containerInfo := range containers {
		eg.Go(func() error {
			containerName := containerInfo.Names[0]
			L.Debug().Str("Container", containerName).Msg("Collecting logs")
			logOptions := container.LogsOptions{ShowStdout: true, ShowStderr: true}
			logs, err := provider.Client().ContainerLogs(context.Background(), containerInfo.ID, logOptions)
			if err != nil {
				L.Error().Err(err).Str("Container", containerName).Msg("failed to fetch logs for container")
				return err
			}
			logFilePath := filepath.Join(dir, fmt.Sprintf("%s.log", containerName))
			logFile, err := os.Create(logFilePath)
			if err != nil {
				L.Error().Err(err).Str("Container", containerName).Msg("failed to create container log file")
				return err
			}
			logFilePaths = append(logFilePaths, logFilePath)
			// Parse and write logs
			header := make([]byte, 8) // Docker stream header is 8 bytes
			for {
				_, err := io.ReadFull(logs, header)
				if err == io.EOF {
					break
				}
				if err != nil {
					L.Error().Err(err).Str("Container", containerName).Msg("failed to read log stream header")
					break
				}

				// Extract log message size
				msgSize := binary.BigEndian.Uint32(header[4:8])

				// Read the log message
				msg := make([]byte, msgSize)
				_, err = io.ReadFull(logs, msg)
				if err != nil {
					L.Error().Err(err).Str("Container", containerName).Msg("failed to read log message")
					break
				}

				// Write the log message to the file
				if _, err := logFile.Write(msg); err != nil {
					L.Error().Err(err).Str("Container", containerName).Msg("failed to write log message to file")
					break
				}
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return logFilePaths, nil
}

func BuildImageOnce(once *sync.Once, dctx, dfile, nameAndTag string) error {
	var err error
	once.Do(func() {
		err = BuildImage(dctx, dfile, nameAndTag)
		if err != nil {
			err = fmt.Errorf("failed to build Docker image: %w", err)
		}
	})
	return err
}

func BuildImage(dctx, dfile, nameAndTag string) error {
	dfilePath := filepath.Join(dctx, dfile)
	if os.Getenv("CTF_CLNODE_DLV") == "true" {
		return runCommand("docker", "build", "--build-arg", `GO_GCFLAGS=all=-N -l`, "-t", nameAndTag, "-f", dfilePath, dctx)
	} else {
		return runCommand("docker", "build", "-t", nameAndTag, "-f", dfilePath, dctx)
	}
}

// ExecContainer executes a command inside a running container by name and returns the combined stdout/stderr.
func ExecContainer(containerName string, command []string) (string, error) {
	L.Info().Strs("Command", command).Str("ContainerName", containerName).Msg("Executing command")
	p, err := tc.NewDockerProvider()
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	containers, err := p.Client().ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}
	var containerID string
	for _, cont := range containers {
		for _, name := range cont.Names {
			if name == "/"+containerName {
				containerID = cont.ID
				break
			}
		}
	}
	if containerID == "" {
		return "", fmt.Errorf("container with name '%s' not found", containerName)
	}

	execConfig := container.ExecOptions{
		Cmd:          command,
		AttachStdout: true,
		AttachStderr: true,
	}
	execID, err := p.Client().ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec instance: %w", err)
	}
	resp, err := p.Client().ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec instance: %w", err)
	}
	defer resp.Close()
	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to read exec output: %w", err)
	}
	L.Info().Str("Output", string(output)).Msg("Command output")
	return string(output), nil
}

type ContainerResources struct {
	CPUs     float64 `toml:"cpus" validate:"gte=0"`
	MemoryMb uint    `toml:"memory_mb"`
}

// ResourceLimitsFunc returns a function to configure container resources based on the human-readable CPUs and memory in Mb
func ResourceLimitsFunc(h *container.HostConfig, resources *ContainerResources) {
	if resources == nil {
		return
	}
	if resources.MemoryMb > 0 {
		h.Memory = int64(resources.MemoryMb) * 1024 * 1024            // Memory in Mb
		h.MemoryReservation = int64(resources.MemoryMb) * 1024 * 1024 // Total memory that can be reserved (soft) in Mb
		// https://docs.docker.com/engine/containers/resource_constraints/ if both values are equal swap is off, read the docs
		h.MemorySwap = h.Memory
	}
	if resources.CPUs > 0 {
		// Set CPU limits using CPUQuota and CPUPeriod
		// we don't use runtime.NumCPU or docker API to get CPUs because h.CPUShares is relative to amount of containers you run
		// CPUPeriod and CPUQuota are absolute and easier to control
		h.CPUPeriod = 100000                        // Default period (100ms)
		h.CPUQuota = int64(resources.CPUs * 100000) // Quota in microseconds (e.g., 0.5 CPUs = 50000)
	}
}
