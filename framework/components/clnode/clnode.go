package clnode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/smartcontractkit/chainlink-testing-framework/framework"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/postgres"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	DefaultHTTPPort     = "6688"
	DefaultP2PPort      = "6690"
	DefaultDebuggerPort = 40000
	TmpImageName        = "chainlink-tmp:latest"
	CustomPortSeparator = ":"
)

var (
	once = &sync.Once{}
)

// Input represents Chainlink node input
type Input struct {
	DbInput *postgres.Input `toml:"db" validate:"required"`
	Node    *NodeInput      `toml:"node" validate:"required"`
	Out     *Output         `toml:"out"`
}

// NodeInput is CL nod container inputs
type NodeInput struct {
	Image                   string                        `toml:"image" validate:"required"`
	Name                    string                        `toml:"name"`
	DockerFilePath          string                        `toml:"docker_file"`
	DockerContext           string                        `toml:"docker_ctx"`
	PullImage               bool                          `toml:"pull_image"`
	CapabilitiesBinaryPaths []string                      `toml:"capabilities"`
	CapabilityContainerDir  string                        `toml:"capabilities_container_dir"`
	TestConfigOverrides     string                        `toml:"test_config_overrides"`
	UserConfigOverrides     string                        `toml:"user_config_overrides"`
	TestSecretsOverrides    string                        `toml:"test_secrets_overrides"`
	UserSecretsOverrides    string                        `toml:"user_secrets_overrides"`
	HTTPPort                int                           `toml:"port"`
	P2PPort                 int                           `toml:"p2p_port"`
	CustomPorts             []string                      `toml:"custom_ports"`
	DebuggerPort            int                           `toml:"debugger_port"`
	ContainerResources      *framework.ContainerResources `toml:"resources"`
}

// Output represents Chainlink node output, nodes and databases connection URLs
type Output struct {
	UseCache   bool             `toml:"use_cache"`
	Node       *NodeOut         `toml:"node"`
	PostgreSQL *postgres.Output `toml:"postgresql"`
}

// NodeOut is CL node container output, URLs to connect
type NodeOut struct {
	APIAuthUser     string `toml:"api_auth_user"`
	APIAuthPassword string `toml:"api_auth_password"`
	ContainerName   string `toml:"container_name"`
	HostURL         string `toml:"url"`
	DockerURL       string `toml:"docker_internal_url"`
	DockerP2PUrl    string `toml:"p2p_docker_internal_url"`
	InternalIP      string `toml:"internal_ip"`
}

// NewNodeWithDB create a new Chainlink node with some image:tag and one or several configs
// see config params: TestConfigOverrides, UserConfigOverrides, etc
func NewNodeWithDB(in *Input) (*Output, error) {
	if in.Out != nil && in.Out.UseCache {
		return in.Out, nil
	}
	pgOut, err := postgres.NewPostgreSQL(in.DbInput)
	if err != nil {
		return nil, err
	}
	nodeOut, err := newNode(in, pgOut)
	if err != nil {
		return nil, err
	}
	out := &Output{
		UseCache:   true,
		Node:       nodeOut,
		PostgreSQL: pgOut,
	}
	in.Out = out
	return out, nil
}

func NewNode(in *Input, pgOut *postgres.Output) (*Output, error) {
	if in.Out != nil && in.Out.UseCache {
		return in.Out, nil
	}
	nodeOut, err := newNode(in, pgOut)
	if err != nil {
		return nil, err
	}
	out := &Output{
		UseCache:   true,
		Node:       nodeOut,
		PostgreSQL: pgOut,
	}
	in.Out = out
	return out, nil
}

func generateEntryPoint() []string {
	entrypoint := []string{
		"/bin/sh", "-c",
	}
	if os.Getenv("CTF_CLNODE_DLV") == "true" {
		entrypoint = append(entrypoint, "dlv  exec /usr/local/bin/chainlink --continue --listen=0.0.0.0:40000 --headless=true --api-version=2 --accept-multiclient -- -c /config/config -c /config/overrides -c /config/user-overrides -s /config/secrets -s /config/secrets-overrides -s /config/user-secrets-overrides node start -d -p /config/node_password -a /config/apicredentials")
	} else {
		entrypoint = append(entrypoint, "chainlink -c /config/config -c /config/overrides -c /config/user-overrides -s /config/secrets -s /config/secrets-overrides -s /config/user-secrets-overrides node start -d -p /config/node_password -a /config/apicredentials")
	}
	return entrypoint
}

// generatePortBindings generates exposed ports and port bindings
// exposes default CL node port
// exposes custom_ports in format "host:docker" or map 1-to-1 if only "host" port is provided
func generatePortBindings(in *Input) ([]string, nat.PortMap, error) {
	httpPort := fmt.Sprintf("%s/tcp", DefaultHTTPPort)
	exposedPorts := []string{httpPort}
	portBindings := nat.PortMap{
		nat.Port(httpPort): []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: strconv.Itoa(in.Node.HTTPPort),
			},
		},
	}
	if os.Getenv("CTF_CLNODE_DLV") == "true" {
		innerDebuggerPort := fmt.Sprintf("%d/tcp", DefaultDebuggerPort)
		portBindings[nat.Port(innerDebuggerPort)] = append(portBindings[nat.Port(innerDebuggerPort)], nat.PortBinding{
			HostIP:   "0.0.0.0",
			HostPort: strconv.Itoa(in.Node.DebuggerPort),
		})
		exposedPorts = append(exposedPorts, strconv.Itoa(DefaultDebuggerPort))
	}
	customPorts := make([]string, 0)
	for _, p := range in.Node.CustomPorts {
		if strings.Contains(p, CustomPortSeparator) {
			pp := strings.Split(p, CustomPortSeparator)
			if len(pp) != 2 {
				return nil, nil, errors.New("custom_ports has ':' but you must provide both ports")
			}
			customPorts = append(customPorts, fmt.Sprintf("%s/tcp", pp[1]))

			dockerPort := nat.Port(fmt.Sprintf("%s/tcp", pp[1]))
			hostPort := pp[0]
			portBindings[dockerPort] = []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: hostPort,
				},
			}
		} else {
			customPorts = append(customPorts, fmt.Sprintf("%s/tcp", p))

			dockerPort := nat.Port(fmt.Sprintf("%s/tcp", p))
			hostPort := p
			portBindings[dockerPort] = []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: hostPort,
				},
			}
		}
	}
	exposedPorts = append(exposedPorts, customPorts...)
	return exposedPorts, portBindings, nil
}

func newNode(in *Input, pgOut *postgres.Output) (*NodeOut, error) {
	ctx := context.Background()

	passwordPath, err := WriteTmpFile(DefaultPasswordTxt, "password.txt")
	if err != nil {
		return nil, err
	}
	apiCredentialsPath, err := WriteTmpFile(DefaultAPICredentials, "apicredentials")
	if err != nil {
		return nil, err
	}
	cfgPath, err := writeDefaultConfig()
	if err != nil {
		return nil, err
	}
	secretsPath, err := writeDefaultSecrets(pgOut)
	if err != nil {
		return nil, err
	}
	overridesFile, err := WriteTmpFile(in.Node.TestConfigOverrides, "overrides.toml")
	if err != nil {
		return nil, err
	}
	secretsOverridesFile, err := WriteTmpFile(in.Node.TestSecretsOverrides, "secrets-overrides.toml")
	if err != nil {
		return nil, err
	}
	userOverridesFile, err := WriteTmpFile(in.Node.UserConfigOverrides, "user-overrides.toml")
	if err != nil {
		return nil, err
	}
	userSecretsOverridesFile, err := WriteTmpFile(in.Node.UserSecretsOverrides, "user-secrets-overrides.toml")
	if err != nil {
		return nil, err
	}

	var containerName string
	if in.Node.Name != "" {
		containerName = in.Node.Name
	} else {
		containerName = framework.DefaultTCName("node")
	}

	exposedPorts, portBindings, err := generatePortBindings(in)
	if err != nil {
		return nil, err
	}
	req := tc.ContainerRequest{
		AlwaysPullImage: in.Node.PullImage,
		Image:           in.Node.Image,
		Name:            containerName,
		Labels:          framework.DefaultTCLabels(),
		Networks:        []string{framework.DefaultNetworkName},
		NetworkAliases: map[string][]string{
			framework.DefaultNetworkName: {containerName},
		},
		ExposedPorts: exposedPorts,
		Entrypoint:   generateEntryPoint(),
		WaitingFor: wait.ForHTTP("/").
			WithPort(DefaultHTTPPort).
			WithStartupTimeout(1 * time.Minute).
			WithPollInterval(200 * time.Millisecond),
	}
	if in.Node.HTTPPort != 0 && in.Node.P2PPort != 0 {
		req.HostConfigModifier = func(h *container.HostConfig) {
			h.PortBindings = portBindings
			framework.ResourceLimitsFunc(h, in.Node.ContainerResources)
		}
	}
	files := []tc.ContainerFile{
		{
			HostFilePath:      cfgPath.Name(),
			ContainerFilePath: "/config/config",
			FileMode:          0644,
		},
		{
			HostFilePath:      secretsPath.Name(),
			ContainerFilePath: "/config/secrets",
			FileMode:          0644,
		},
		{
			HostFilePath:      overridesFile.Name(),
			ContainerFilePath: "/config/overrides",
			FileMode:          0644,
		},
		{
			HostFilePath:      userOverridesFile.Name(),
			ContainerFilePath: "/config/user-overrides",
			FileMode:          0644,
		},
		{
			HostFilePath:      secretsOverridesFile.Name(),
			ContainerFilePath: "/config/secrets-overrides",
			FileMode:          0644,
		},
		{
			HostFilePath:      userSecretsOverridesFile.Name(),
			ContainerFilePath: "/config/user-secrets-overrides",
			FileMode:          0644,
		},
		{
			HostFilePath:      passwordPath.Name(),
			ContainerFilePath: "/config/node_password",
			FileMode:          0644,
		},
		{
			HostFilePath:      apiCredentialsPath.Name(),
			ContainerFilePath: "/config/apicredentials",
			FileMode:          0644,
		},
	}
	if in.Node.CapabilityContainerDir == "" {
		in.Node.CapabilityContainerDir = "/home/capabilities"
	}
	for _, cp := range in.Node.CapabilitiesBinaryPaths {
		cpPath := filepath.Base(cp)
		framework.L.Info().Any("Path", cpPath).Str("Binary", cpPath).Msg("Copying capability binary")
		files = append(files, tc.ContainerFile{
			HostFilePath:      cp,
			ContainerFilePath: filepath.Join(in.Node.CapabilityContainerDir, cpPath),
			FileMode:          0777,
		})
	}
	req.Files = append(req.Files, files...)
	if req.Image == "" {
		if in.Node.DockerFilePath != "" || in.Node.DockerContext != "" {
			return nil, errors.New("image field is empty but there are no 'docker_ctx' or 'docker_file' set in TOML config")
		}
		req.Image = TmpImageName
		if err := framework.BuildImageOnce(once, in.Node.DockerContext, in.Node.DockerFilePath, req.Image); err != nil {
			return nil, err
		}
		req.KeepImage = false
	}
	c, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}
	ip, err := c.ContainerIP(ctx)
	if err != nil {
		return nil, err
	}
	host, err := framework.GetHost(c)
	if err != nil {
		return nil, err
	}

	mp := nat.Port(fmt.Sprintf("%d/tcp", in.Node.HTTPPort))

	return &NodeOut{
		APIAuthUser:     DefaultAPIUser,
		APIAuthPassword: DefaultAPIPassword,
		ContainerName:   containerName,
		HostURL:         fmt.Sprintf("http://%s:%s", host, mp.Port()),
		DockerURL:       fmt.Sprintf("http://%s:%s", containerName, DefaultHTTPPort),
		DockerP2PUrl:    fmt.Sprintf("http://%s:%s", containerName, DefaultP2PPort),
		InternalIP:      ip,
	}, nil
}

type DefaultCLNodeConfig struct {
	HTTPPort      string
	SecureCookies bool
}

func generateDefaultConfig() (string, error) {
	config := DefaultCLNodeConfig{
		HTTPPort:      DefaultHTTPPort,
		SecureCookies: false,
	}
	tmpl, err := template.New("toml").Parse(defaultConfigTmpl)
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	err = tmpl.Execute(&output, config)
	if err != nil {
		return "", err
	}
	return output.String(), nil
}

type DefaultSecretsConfig struct {
	DatabaseURL string
	Keystore    string
}

func generateSecretsConfig(connString, password string) (string, error) {
	// Create the configuration with example values
	config := DefaultSecretsConfig{
		DatabaseURL: connString,
		Keystore:    password,
	}
	tmpl, err := template.New("toml").Parse(dbTmpl)
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	err = tmpl.Execute(&output, config)
	if err != nil {
		return "", err
	}
	return output.String(), nil
}

func writeDefaultSecrets(pgOut *postgres.Output) (*os.File, error) {
	secretsOverrides, err := generateSecretsConfig(pgOut.DockerInternalURL, DefaultTestKeystorePassword)
	if err != nil {
		return nil, err
	}
	return WriteTmpFile(secretsOverrides, "secrets.toml")
}

func writeDefaultConfig() (*os.File, error) {
	cfg, err := generateDefaultConfig()
	if err != nil {
		return nil, err
	}
	return WriteTmpFile(cfg, "config.toml")
}

// WriteTmpFile writes the provided data string to a specified filepath and returns the file and any error encountered.
func WriteTmpFile(data, filePath string) (*os.File, error) {
	file, err := os.CreateTemp("", filePath)
	if err != nil {
		return nil, err
	}
	_, err = file.WriteString(data)
	if err != nil {
		return nil, err
	}
	return file, nil
}
