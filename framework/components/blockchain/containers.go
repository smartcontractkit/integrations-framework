package blockchain

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-testing-framework/framework"
)

func baseRequest(in *Input) testcontainers.ContainerRequest {
	containerName := framework.DefaultTCName("blockchain-node")
	bindPort := fmt.Sprintf("%s/tcp", in.Port)

	return testcontainers.ContainerRequest{
		Labels:       framework.DefaultTCLabels(),
		Name:         containerName,
		ExposedPorts: []string{bindPort},
		HostConfigModifier: func(h *container.HostConfig) {
			h.PortBindings = framework.MapTheSamePort(bindPort)
			framework.ResourceLimitsFunc(h, in.ContainerResources)
		},
		Networks: []string{framework.DefaultNetworkName},
		NetworkAliases: map[string][]string{
			framework.DefaultNetworkName: {containerName},
		},
		WaitingFor: wait.ForListeningPort(nat.Port(in.Port)).WithStartupTimeout(10 * time.Second).WithPollInterval(200 * time.Millisecond),
	}
}

func createGenericEvmContainer(in *Input, req testcontainers.ContainerRequest) (*Output, error) {
	ctx := context.Background()

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	host, err := framework.GetHost(c)
	if err != nil {
		return nil, err
	}

	bindPort := req.ExposedPorts[0]
	mp, err := c.MappedPort(ctx, nat.Port(bindPort))
	if err != nil {
		return nil, err
	}

	containerName := req.Name

	return &Output{
		UseCache:      true,
		Family:        "evm",
		ChainID:       in.ChainID,
		ContainerName: containerName,
		Container:     c,
		Nodes: []*Node{
			{
				HostWSUrl:             fmt.Sprintf("ws://%s:%s", host, mp.Port()),
				HostHTTPUrl:           fmt.Sprintf("http://%s:%s", host, mp.Port()),
				DockerInternalWSUrl:   fmt.Sprintf("ws://%s:%s", containerName, in.Port),
				DockerInternalHTTPUrl: fmt.Sprintf("http://%s:%s", containerName, in.Port),
			},
		},
	}, nil
}
