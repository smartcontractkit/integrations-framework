package test_env

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"
	"github.com/smartcontractkit/chainlink-testing-framework/docker"
	"github.com/smartcontractkit/chainlink-testing-framework/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/mirror"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/templates"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/testcontext"
)

const defaultErigonPowImage = "thorax/erigon:v2.40.0"

type ErigonPow struct {
	EnvComponent
	ExternalHttpUrl string
	InternalHttpUrl string
	ExternalWsUrl   string
	InternalWsUrl   string
	chainConfg      *EthereumChainConfig
	l               zerolog.Logger
	t               *testing.T
}

func NewErigonPow(networks []string, chainConfg *EthereumChainConfig, opts ...EnvComponentOption) (*ErigonPow, error) {
	parts := strings.Split(defaultErigonPowImage, ":")
	g := &ErigonPow{
		EnvComponent: EnvComponent{
			ContainerName:    fmt.Sprintf("%s-%s", "erigon-pow", uuid.NewString()[0:8]),
			Networks:         networks,
			ContainerImage:   parts[0],
			ContainerVersion: parts[1],
		},
		chainConfg: chainConfg,
		l:          logging.GetTestLogger(nil),
	}
	g.SetDefaultHooks()
	for _, opt := range opts {
		opt(&g.EnvComponent)
	}
	// if the internal docker repo is set then add it to the version
	g.EnvComponent.ContainerImage = mirror.AddMirrorToImageIfSet(g.EnvComponent.ContainerImage)
	return g, nil
}

func (g *ErigonPow) WithTestInstance(t *testing.T) ExecutionClient {
	g.l = logging.GetTestLogger(t)
	g.t = t
	return g
}

func (g *ErigonPow) StartContainer() (blockchain.EVMNetwork, error) {
	r, err := g.getContainerRequest()
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}

	l := logging.GetTestContainersGoTestLogger(g.t)
	ct, err := docker.StartContainerWithRetry(g.l, tc.GenericContainerRequest{
		ContainerRequest: *r,
		Reuse:            true,
		Started:          true,
		Logger:           l,
	})
	if err != nil {
		return blockchain.EVMNetwork{}, fmt.Errorf("cannot start erigon container: %w", err)
	}

	host, err := GetHost(testcontext.Get(g.t), ct)
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}
	httpPort, err := ct.MappedPort(testcontext.Get(g.t), NatPort(TX_GETH_HTTP_PORT))
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}

	g.Container = ct
	g.ExternalHttpUrl = FormatHttpUrl(host, httpPort.Port())
	g.InternalHttpUrl = FormatHttpUrl(g.ContainerName, TX_GETH_HTTP_PORT)
	g.ExternalWsUrl = FormatWsUrl(host, httpPort.Port())
	g.InternalWsUrl = FormatWsUrl(g.ContainerName, TX_GETH_HTTP_PORT)

	networkConfig := blockchain.SimulatedEVMNetwork
	networkConfig.Name = "Simulated Ethereum-PoW (erigon)"
	networkConfig.URLs = []string{g.ExternalWsUrl}
	networkConfig.HTTPURLs = []string{g.ExternalHttpUrl}

	g.l.Info().Str("containerName", g.ContainerName).
		Msg("Started Erigon container")

	return networkConfig, nil
}

func (g *ErigonPow) GetInternalExecutionURL() string {
	panic("not supported")
}

func (g *ErigonPow) GetExternalExecutionURL() string {
	panic("not supported")
}

func (g *ErigonPow) GetInternalHttpUrl() string {
	return g.InternalHttpUrl
}

func (g *ErigonPow) GetInternalWsUrl() string {
	return g.InternalWsUrl
}

func (g *ErigonPow) GetExternalHttpUrl() string {
	return g.ExternalHttpUrl
}

func (g *ErigonPow) GetExternalWsUrl() string {
	return g.ExternalWsUrl
}

func (g *ErigonPow) GetContainerName() string {
	return g.ContainerName
}

func (g *ErigonPow) GetContainer() *tc.Container {
	return &g.Container
}

func (g *ErigonPow) getContainerRequest() (*tc.ContainerRequest, error) {
	initFile, err := os.CreateTemp("", "init.sh")
	if err != nil {
		return nil, err
	}

	initScriptContent, err := g.buildInitScript()
	if err != nil {
		return nil, err
	}

	_, err = initFile.WriteString(initScriptContent)
	if err != nil {
		return nil, err
	}

	keystoreDir, err := os.MkdirTemp("", "keystore")
	if err != nil {
		return nil, err
	}

	genesisJsonStr, err := templates.GethPoWGenesisJsonTemplate{
		ChainId:     fmt.Sprintf("%d", g.chainConfg.ChainID),
		AccountAddr: RootFundingAddr,
	}.String()
	if err != nil {
		return nil, err
	}
	genesisFile, err := os.CreateTemp("", "genesis_json")
	if err != nil {
		return nil, err
	}
	_, err = genesisFile.WriteString(genesisJsonStr)
	if err != nil {
		return nil, err
	}
	key1File, err := os.CreateTemp(keystoreDir, "key1")
	if err != nil {
		return nil, err
	}
	_, err = key1File.WriteString(RootFundingWallet)
	if err != nil {
		return nil, err
	}

	return &tc.ContainerRequest{
		Name:          g.ContainerName,
		Image:         g.GetImageWithVersion(),
		Networks:      g.Networks,
		ImagePlatform: "linux/x86_64",
		ExposedPorts:  []string{NatPortFormat(TX_GETH_HTTP_PORT)},
		WaitingFor: tcwait.ForAll(
			tcwait.ForLog("Started P2P networking").
				WithStartupTimeout(120*time.Second).
				WithPollInterval(1*time.Second),
			NewWebSocketStrategy(NatPort(TX_GETH_HTTP_PORT), g.l),
		),
		User: "0:0",
		Entrypoint: []string{
			"sh",
			"/home/erigon/init.sh",
		},
		Files: []tc.ContainerFile{
			{
				HostFilePath:      initFile.Name(),
				ContainerFilePath: "/root/init.sh",
				FileMode:          0644,
			},
			{
				HostFilePath:      genesisFile.Name(),
				ContainerFilePath: "/root/genesis.json",
				FileMode:          0644,
			},
			{
				HostFilePath:      initFile.Name(),
				ContainerFilePath: "/home/erigon/init.sh",
				FileMode:          0744,
			},
		},
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   keystoreDir,
				Target:   "/root/.local/share/erigon/keystore/",
				ReadOnly: false,
			},
			)
		},
		LifecycleHooks: []tc.ContainerLifecycleHooks{
			{
				PostStarts: g.PostStartsHooks,
				PostStops:  g.PostStopsHooks,
			},
		},
	}, nil
}

func (g *ErigonPow) WaitUntilChainIsReady(ctx context.Context, waitTime time.Duration) error {
	return nil
}

func (g *ErigonPow) buildInitScript() (string, error) {
	initTemplate := `#!/bin/bash
	echo "Running erigon init"
	erigon init /root/genesis.json
	exit_code=$?
	if [ $exit_code -ne 0 ]; then
		echo "Erigon init failed with exit code $exit_code"
		exit 1
	fi

	echo "Starting Erigon..."
	erigon --http --http.api=eth,erigon,engine,web3,net,debug,trace,txpool,admin --http.addr=0.0.0.0 --http.corsdomain=* --http.vhosts=* --http.port={{.HttpPort}} --ws \
	--allow-insecure-unlock  --nodiscover --networkid={{.ChainID}} --fakepow --mine \
	--miner.etherbase {{.RootFundingAddr}}`

	data := struct {
		HttpPort        string
		ChainID         int
		RootFundingAddr string
	}{
		HttpPort:        TX_GETH_HTTP_PORT,
		ChainID:         g.chainConfg.ChainID,
		RootFundingAddr: RootFundingAddr,
	}

	t, err := template.New("init").Parse(initTemplate)
	if err != nil {
		fmt.Println("Error parsing template:", err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, data)

	return buf.String(), err

}

func (g *ErigonPow) GetContainerType() ContainerType {
	return ContainerType_Erigon
}
