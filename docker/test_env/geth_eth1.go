package test_env

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/google/uuid"
	"github.com/smartcontractkit/chainlink-testing-framework/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/mirror"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/templates"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
)

func NewGethEth1(networks []string, chainConfig *EthereumChainConfig, opts ...EnvComponentOption) *Geth {
	parts := strings.Split(defaultGethEth1Image, ":")
	g := &Geth{
		EnvComponent: EnvComponent{
			ContainerName:    fmt.Sprintf("%s-%s", "geth-eth1", uuid.NewString()[0:8]),
			Networks:         networks,
			ContainerImage:   parts[0],
			ContainerVersion: parts[1],
		},
		chainConfig: chainConfig,
		l:           logging.GetTestLogger(nil),
	}
	g.SetDefaultHooks()
	for _, opt := range opts {
		opt(&g.EnvComponent)
	}
	// if the internal docker repo is set then add it to the version
	g.EnvComponent.ContainerImage = mirror.AddMirrorToImageIfSet(g.EnvComponent.ContainerImage)
	return g
}

func (g *Geth) getEth1ContainerRequest() (*tc.ContainerRequest, error) {
	initScriptFile, err := os.CreateTemp("", "init_script")
	if err != nil {
		return nil, err
	}
	_, err = initScriptFile.WriteString(templates.InitGethScript)
	if err != nil {
		return nil, err
	}
	keystoreDir, err := os.MkdirTemp("", "keystore")
	if err != nil {
		return nil, err
	}

	generatedData, err := generateKeystoreAndExtraData(keystoreDir)
	if err != nil {
		return nil, err
	}

	genesisJsonStr, err := templates.GenesisJsonTemplate{
		ChainId:     fmt.Sprintf("%d", g.chainConfig.ChainID),
		AccountAddr: []string{generatedData.minerAccount.Address.Hex(), RootFundingAddr},
		Consensus:   templates.GethGenesisConsensus_Clique,
		ExtraData:   fmt.Sprintf("0x%s", hex.EncodeToString(generatedData.extraData)),
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
	configDir, err := os.MkdirTemp("", "config")
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(configDir+"/password.txt", []byte(""), 0600)
	if err != nil {
		return nil, err
	}

	entrypoint, err := g.getEntryPointAndKeystoreLocation(generatedData.minerAccount.Address.Hex())
	if err != nil {
		return nil, err
	}

	websocketMsg, err := g.getWebsocketEnabledMessage()
	if err != nil {
		return nil, err
	}

	return &tc.ContainerRequest{
		Name:            g.ContainerName,
		AlwaysPullImage: true,
		Image:           g.GetImageWithVersion(),
		ExposedPorts:    []string{NatPortFormat(DEFAULT_EVM_NODE_HTTP_PORT), NatPortFormat(DEFAULT_EVM_NODE_WS_PORT)},
		Networks:        g.Networks,
		WaitingFor: tcwait.ForAll(
			NewHTTPStrategy("/", NatPort(DEFAULT_EVM_NODE_HTTP_PORT)),
			tcwait.ForLog(websocketMsg),
			tcwait.ForLog("Started P2P networking").
				WithStartupTimeout(120*time.Second).
				WithPollInterval(1*time.Second),
			NewWebSocketStrategy(NatPort(DEFAULT_EVM_NODE_WS_PORT), g.l),
		),
		Entrypoint: entrypoint,
		Files: []tc.ContainerFile{
			{
				HostFilePath:      initScriptFile.Name(),
				ContainerFilePath: "/root/init.sh",
				FileMode:          0644,
			},
			{
				HostFilePath:      genesisFile.Name(),
				ContainerFilePath: "/root/genesis.json",
				FileMode:          0644,
			},
		},
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   keystoreDir,
				Target:   "/root/.ethereum/devchain/keystore/",
				ReadOnly: false,
			}, mount.Mount{
				Type:     mount.TypeBind,
				Source:   configDir,
				Target:   "/root/config/",
				ReadOnly: false,
			})
		},
		LifecycleHooks: []tc.ContainerLifecycleHooks{
			{
				PostStarts: g.PostStartsHooks,
				PostStops:  g.PostStopsHooks,
			},
		},
	}, nil
}
