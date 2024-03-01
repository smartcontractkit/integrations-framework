package test_env

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"
	"github.com/smartcontractkit/chainlink-testing-framework/docker"
	"github.com/smartcontractkit/chainlink-testing-framework/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/mirror"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/templates"
)

const defaultNethermindPowImage = "nethermind/nethermind:1.13.0"

type NethermindPow struct {
	EnvComponent
	ExternalHttpUrl string
	InternalHttpUrl string
	ExternalWsUrl   string
	InternalWsUrl   string
	chainConfg      *EthereumChainConfig
	l               zerolog.Logger
	t               *testing.T
}

func NewNethermindPow(networks []string, chainConfg *EthereumChainConfig, opts ...EnvComponentOption) (*NethermindPow, error) {
	parts := strings.Split(defaultNethermindPowImage, ":")
	g := &NethermindPow{
		EnvComponent: EnvComponent{
			ContainerName:    fmt.Sprintf("%s-%s", "nethermind-pow", uuid.NewString()[0:8]),
			Networks:         networks,
			ContainerImage:   parts[0],
			ContainerVersion: parts[1],
		},
		chainConfg: chainConfg,
		l:          logging.GetTestLogger(nil),
	}
	for _, opt := range opts {
		opt(&g.EnvComponent)
	}
	// if the internal docker repo is set then add it to the version
	g.EnvComponent.ContainerImage = mirror.AddMirrorToImageIfSet(g.EnvComponent.ContainerImage)
	return g, nil
}

func (g *NethermindPow) WithTestInstance(t *testing.T) ExecutionClient {
	g.l = logging.GetTestLogger(t)
	g.t = t
	return g
}

func (g *NethermindPow) StartContainer() (blockchain.EVMNetwork, error) {
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
		return blockchain.EVMNetwork{}, fmt.Errorf("cannot start nethermind container: %w", err)
	}

	host, err := GetHost(context.Background(), ct)
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}
	httpPort, err := ct.MappedPort(context.Background(), NatPort(TX_GETH_HTTP_PORT))
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}
	wsPort, err := ct.MappedPort(context.Background(), NatPort(TX_GETH_WS_PORT))
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}

	g.Container = ct
	g.ExternalHttpUrl = FormatHttpUrl(host, httpPort.Port())
	g.InternalHttpUrl = FormatHttpUrl(g.ContainerName, TX_GETH_HTTP_PORT)
	g.ExternalWsUrl = FormatWsUrl(host, wsPort.Port())
	g.InternalWsUrl = FormatWsUrl(g.ContainerName, TX_GETH_WS_PORT)

	networkConfig := blockchain.SimulatedEVMNetwork
	networkConfig.Name = "Simulated Ethereum-PoW (nethermind)"
	networkConfig.URLs = []string{g.ExternalWsUrl}
	networkConfig.HTTPURLs = []string{g.ExternalHttpUrl}
	networkConfig.GasEstimationBuffer = 100_000_000_000

	g.l.Info().Str("containerName", g.ContainerName).
		Msg("Started Nethermind container")

	return networkConfig, nil
}

func (g *NethermindPow) GetInternalExecutionURL() string {
	panic("not supported")
}

func (g *NethermindPow) GetExternalExecutionURL() string {
	panic("not supported")
}

func (g *NethermindPow) GetInternalHttpUrl() string {
	return g.InternalHttpUrl
}

func (g *NethermindPow) GetInternalWsUrl() string {
	return g.InternalWsUrl
}

func (g *NethermindPow) GetExternalHttpUrl() string {
	return g.ExternalHttpUrl
}

func (g *NethermindPow) GetExternalWsUrl() string {
	return g.ExternalWsUrl
}

func (g *NethermindPow) GetContainerName() string {
	return g.ContainerName
}

func (g *NethermindPow) GetContainer() *tc.Container {
	return &g.Container
}

func (g *NethermindPow) getContainerRequest() (*tc.ContainerRequest, error) {
	keystoreDir, err := os.MkdirTemp("", "keystore")
	if err != nil {
		return nil, err
	}

	// Create keystore and ethereum account
	ks := keystore.NewKeyStore(keystoreDir, keystore.StandardScryptN, keystore.StandardScryptP)
	account, err := ks.NewAccount("")
	if err != nil {
		return nil, err
	}

	addr := strings.Replace(account.Address.Hex(), "0x", "", 1)
	FundingAddresses[addr] = ""
	signerBytes, err := hex.DecodeString(addr)
	if err != nil {
		fmt.Println("Error decoding signer address:", err)
		return nil, err
	}

	zeroBytes := make([]byte, 32)                      // Create 32 zero bytes
	extradata := append(zeroBytes, signerBytes...)     // Concatenate zero bytes and signer address
	extradata = append(extradata, make([]byte, 65)...) // Concatenate 65 more zero bytes

	genesisJsonStr, err := templates.NethermindPoWGenesisJsonTemplate{
		ChainId:     fmt.Sprintf("%d", g.chainConfg.ChainID),
		AccountAddr: RootFundingAddr,
		ExtraData:   fmt.Sprintf("0x%s", hex.EncodeToString(extradata)),
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

	// create empty cfg file since if we don't pass any
	// default mainnet.cfg will be used
	noneCfg, err := os.CreateTemp("", "none.cfg")
	if err != nil {
		return nil, err
	}

	_, err = noneCfg.WriteString("{}")
	if err != nil {
		return nil, err
	}

	passFile, err := os.CreateTemp("", "password.txt")
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(passFile.Name(), []byte(""), 0600)
	if err != nil {
		return nil, err
	}

	rootFile, err := os.CreateTemp(keystoreDir, RootFundingAddr)
	if err != nil {
		return nil, err
	}
	_, err = rootFile.WriteString(RootFundingWallet)
	if err != nil {
		return nil, err
	}

	return &tc.ContainerRequest{
		Name:            g.ContainerName,
		Image:           g.GetImageWithVersion(),
		Networks:        g.Networks,
		AlwaysPullImage: true,
		// ImagePlatform: "linux/x86_64",  //don't even try this on Apple Silicon, the node won't start due to .NET error
		ExposedPorts: []string{NatPortFormat(TX_GETH_HTTP_PORT), NatPortFormat(TX_GETH_WS_PORT)},
		WaitingFor: tcwait.ForAll(
			tcwait.ForLog("Nethermind initialization completed").
				WithStartupTimeout(120 * time.Second).
				WithPollInterval(1 * time.Second),
		),
		Cmd: []string{
			"--config=/none.cfg",
			"--Init.ChainSpecPath=/chainspec.json",
			"--Init.DiscoveryEnabled=false",
			"--Init.WebSocketsEnabled=true",
			fmt.Sprintf("--JsonRpc.WebSocketsPort=%s", TX_GETH_WS_PORT),
			"--JsonRpc.Enabled=true",
			"--JsonRpc.EnabledModules=net,consensus,eth,subscribe,web3,admin,trace,txpool",
			"--JsonRpc.Host=0.0.0.0",
			fmt.Sprintf("--JsonRpc.Port=%s", TX_GETH_HTTP_PORT),
			"--KeyStore.KeyStoreDirectory=/keystore",
			fmt.Sprintf("--KeyStore.BlockAuthorAccount=%s", account.Address.Hex()),
			fmt.Sprintf("--KeyStore.UnlockAccounts=%s", account.Address.Hex()),
			"--KeyStore.PasswordFiles=/password.txt",
			"--Mining.Enabled=true",
			// "--Init.IsMining=true",
			"--Init.PeerManagerEnabled=false",
			"--HealthChecks.Enabled=true", // default slug /health
		},
		Files: []tc.ContainerFile{
			{
				HostFilePath:      genesisFile.Name(),
				ContainerFilePath: "/chainspec.json",
				FileMode:          0644,
			},
			{
				HostFilePath:      noneCfg.Name(),
				ContainerFilePath: "/none.cfg",
				FileMode:          0644,
			},
			{
				HostFilePath:      passFile.Name(),
				ContainerFilePath: "/password.txt",
				FileMode:          0644,
			},
		},
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   keystoreDir,
				Target:   "/keystore",
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

func (g *NethermindPow) WaitUntilChainIsReady(ctx context.Context, waitTime time.Duration) error {
	return nil
}

func (g *NethermindPow) GetContainerType() ContainerType {
	return ContainerType_Nethermind
}
