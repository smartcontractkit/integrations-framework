package blockchain

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-testing-framework/framework"
)

var (
	DefaultAptosAccount    = "0xa337b42bd0eecf8fb59ee5929ea4541904b3c35a642040223f3d26ab57f59d6e"
	DefaultAptosPrivateKey = "0xd477c65f88ed9e6d4ec6e2014755c3cfa3e0c44e521d0111a02868c5f04c41d4"
)

func defaultAptos(in *Input) {
	if in.Image == "" {
		in.Image = "aptoslabs/tools:aptos-node-v1.27.1"
	}
	if in.Port != "" {
		framework.L.Warn().Msg("'port' field is set but only default port can be used: 8080")
	}
	in.Port = "8080"
}

func newAptos(in *Input) (*Output, error) {
	defaultAptos(in)
	ctx := context.Background()
	containerName := framework.DefaultTCName("blockchain-node")

	absPath, err := filepath.Abs(in.ContractsDir)
	if err != nil {
		return nil, err
	}

	bindPort := fmt.Sprintf("%s/tcp", in.Port)

	req := testcontainers.ContainerRequest{
		Image:        in.Image,
		ExposedPorts: []string{in.Port},
		WaitingFor:   wait.ForLog("Faucet is ready"),
		Name:         containerName,
		Labels:       framework.DefaultTCLabels(),
		Networks:     []string{framework.DefaultNetworkName},
		NetworkAliases: map[string][]string{
			framework.DefaultNetworkName: {containerName},
		},
		HostConfigModifier: func(h *container.HostConfig) {
			h.PortBindings = framework.MapTheSamePort(bindPort)
			framework.ResourceLimitsFunc(h, in.ContainerResources)
		},
		ImagePlatform: "linux/amd64",
		Cmd: []string{
			"aptos",
			"node",
			"run-local-testnet",
			"--with-faucet",
			"--force-restart",
			"--bind-to",
			"0.0.0.0",
		},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      absPath,
				ContainerFilePath: "/",
			},
		},
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}
	host, err := c.Host(ctx)
	if err != nil {
		return nil, err
	}
	cmdStr := []string{"aptos", "init", "--network=local", "--assume-yes", fmt.Sprintf("--private-key=%s", DefaultAptosPrivateKey)}
	_, err = framework.ExecContainer(containerName, cmdStr)
	if err != nil {
		return nil, err
	}
	fundCmd := []string{"aptos", "account", "fund-with-faucet", "--account", DefaultAptosAccount, "--amount", "1000000000000"}
	_, err = framework.ExecContainer(containerName, fundCmd)
	if err != nil {
		return nil, err
	}
	return &Output{
		UseCache:      true,
		Family:        "aptos",
		ContainerName: containerName,
		Nodes: []*Node{
			{
				HostHTTPUrl:           fmt.Sprintf("http://%s:%s", host, in.Port),
				DockerInternalHTTPUrl: fmt.Sprintf("http://%s:%s", containerName, in.Port),
			},
		},
	}, nil
}
