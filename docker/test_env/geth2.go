package test_env

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"
	"github.com/smartcontractkit/chainlink-testing-framework/docker"
)

const (
	GO_CLIENT_IMAGE_TAG = "v1.13.4"
)

type Geth2 struct {
	EnvComponent
	ExternalHttpUrl      string
	InternalHttpUrl      string
	ExternalWsUrl        string
	InternalWsUrl        string
	InternalExecutionURL string
	ExternalExecutionURL string
	generatedDataHostDir string
	consensusLayer       ConsensusLayer
	l                    zerolog.Logger
}

func NewGeth2(networks []string, generatedDataHostDir string, consensusLayer ConsensusLayer, opts ...EnvComponentOption) *Geth2 {
	g := &Geth2{
		EnvComponent: EnvComponent{
			ContainerName: fmt.Sprintf("%s-%s", "geth2", uuid.NewString()[0:8]),
			Networks:      networks,
		},
		generatedDataHostDir: generatedDataHostDir,
		consensusLayer:       consensusLayer,
		l:                    log.Logger,
	}
	for _, opt := range opts {
		opt(&g.EnvComponent)
	}
	return g
}

func (g *Geth2) WithLogger(l zerolog.Logger) *Geth2 {
	g.l = l
	return g
}

func (g *Geth2) StartContainer() (blockchain.EVMNetwork, error) {
	r, err := g.getContainerRequest(g.Networks)
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}

	ct, err := docker.StartContainerWithRetry(g.l, tc.GenericContainerRequest{
		ContainerRequest: *r,
		Reuse:            true,
		Started:          true,
		Logger:           &g.l,
	})
	if err != nil {
		return blockchain.EVMNetwork{}, errors.Wrapf(err, "cannot start geth container")
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
	executionPort, err := ct.MappedPort(context.Background(), NatPort(ETH2_EXECUTION_PORT))
	if err != nil {
		return blockchain.EVMNetwork{}, err
	}

	g.Container = ct
	g.ExternalHttpUrl = FormatHttpUrl(host, httpPort.Port())
	g.InternalHttpUrl = FormatHttpUrl(g.ContainerName, TX_GETH_HTTP_PORT)
	g.ExternalWsUrl = FormatWsUrl(host, wsPort.Port())
	g.InternalWsUrl = FormatWsUrl(g.ContainerName, TX_GETH_WS_PORT)
	g.InternalExecutionURL = FormatHttpUrl(g.ContainerName, ETH2_EXECUTION_PORT)
	g.ExternalExecutionURL = FormatHttpUrl(host, executionPort.Port())

	networkConfig := blockchain.SimulatedEVMNetwork
	networkConfig.Name = fmt.Sprintf("Simulated Eth2 (Geth %s)", g.consensusLayer)
	networkConfig.Name = fmt.Sprintf("geth-eth2-%s", g.consensusLayer)
	networkConfig.URLs = []string{g.ExternalWsUrl}
	networkConfig.HTTPURLs = []string{g.ExternalHttpUrl}

	g.l.Info().Str("containerName", g.ContainerName).
		Msg("Started Geth2 container")

	return networkConfig, nil
}

func (g *Geth2) GetInternalExecutionURL() string {
	return g.InternalExecutionURL
}

func (g *Geth2) GetExternalExecutionURL() string {
	return g.ExternalExecutionURL
}

func (g *Geth2) GetInternalHttpUrl() string {
	return g.InternalHttpUrl
}

func (g *Geth2) GetInternalWsUrl() string {
	return g.InternalWsUrl
}

func (g *Geth2) GetExternalHttpUrl() string {
	return g.ExternalHttpUrl
}

func (g *Geth2) GetExternalWsUrl() string {
	return g.ExternalWsUrl
}

func (g *Geth2) GetContainerName() string {
	return g.ContainerName
}

func (g *Geth2) GetContainer() *tc.Container {
	return &g.Container
}

func (g *Geth2) getContainerRequest(networks []string) (*tc.ContainerRequest, error) {
	initFile, err := os.CreateTemp("", "init.sh")
	if err != nil {
		return nil, err
	}

	initScriptContent, err := buildInitScript()
	if err != nil {
		return nil, err
	}

	_, err = initFile.WriteString(initScriptContent)
	if err != nil {
		return nil, err
	}

	return &tc.ContainerRequest{
		Name:          g.ContainerName,
		Image:         fmt.Sprintf("ethereum/client-go:%s", GO_CLIENT_IMAGE_TAG),
		Networks:      networks,
		ImagePlatform: "linux/x86_64",
		ExposedPorts:  []string{NatPortFormat(TX_GETH_HTTP_PORT), NatPortFormat(TX_GETH_WS_PORT), NatPortFormat(ETH2_EXECUTION_PORT)},
		WaitingFor: tcwait.ForAll(
			tcwait.ForLog("WebSocket enabled").
				WithStartupTimeout(120 * time.Second).
				WithPollInterval(1 * time.Second),
		),
		Entrypoint: []string{
			"sh",
			"/init.sh",
		},
		Files: []tc.ContainerFile{
			{
				HostFilePath:      initFile.Name(),
				ContainerFilePath: "/init.sh",
				FileMode:          0744,
			},
		},
		Mounts: tc.ContainerMounts{
			tc.ContainerMount{
				Source: tc.GenericBindMountSource{
					HostPath: g.generatedDataHostDir,
				},
				Target: tc.ContainerMountTarget(GENERATED_DATA_DIR_INSIDE_CONTAINER),
			},
		},
	}, nil
}

func (g Geth2) WaitUntilChainIsReady(waitTime time.Duration) error {
	waitForFirstBlock := tcwait.NewLogStrategy("Chain head was updated").WithPollInterval(1 * time.Second).WithStartupTimeout(waitTime)
	return waitForFirstBlock.WaitUntilReady(context.Background(), *g.GetContainer())
}

// echo "" > /execution/password.txt && \

func buildInitScript() (string, error) {
	initTemplate := `#!/bin/bash
	mkdir -p /execution/keystore && \
	echo "2e0834786285daccd064ca17f1654f67b4aef298acbb82cef9ec422fb4975622" > /execution/sk.json && \
	echo '{"address":"123463a4b065722e99115d6c222f267d9cabb524","crypto":{"cipher":"aes-128-ctr","ciphertext":"93b90389b855889b9f91c89fd15b9bd2ae95b06fe8e2314009fc88859fc6fde9","cipherparams":{"iv":"9dc2eff7967505f0e6a40264d1511742"},"kdf":"scrypt","kdfparams":{"dklen":32,"n":262144,"p":1,"r":8,"salt":"c07503bb1b66083c37527cd8f06f8c7c1443d4c724767f625743bd47ae6179a4"},"mac":"6d359be5d6c432d5bbb859484009a4bf1bd71b76e89420c380bd0593ce25a817"},"id":"622df904-0bb1-4236-b254-f1b8dfdff1ec","version":3}' > /execution/keystore/key1 && \
	geth init --state.scheme=path --datadir=/execution {{.GeneratedDataDir}}/genesis.json && \
	geth --http --http.api=eth,net,web3,debug --http.addr=0.0.0.0 --http.corsdomain=* \
		--http.vhosts=* --http.port={{.HttpPort}} --ws --ws.api=admin,debug,web3,eth,txpool,net \
		--ws.addr=0.0.0.0 --ws.origins=* --ws.port={{.WsPort}} --authrpc.vhosts=* \
		--authrpc.addr=0.0.0.0 --authrpc.jwtsecret={{.JwtFileLocation}} --datadir=/execution \
		--rpc.allow-unprotected-txs --rpc.txfeecap=0 --allow-insecure-unlock \
		--password={{.PasswordFileLocation}} --nodiscover --syncmode=full --networkid={{.ChainID}} \
		--graphql --graphql.corsdomain=* --unlock=0x123463a4b065722e99115d6c222f267d9cabb524`

	data := struct {
		HttpPort             string
		WsPort               string
		ChainID              string
		GeneratedDataDir     string
		JwtFileLocation      string
		PasswordFileLocation string
	}{
		HttpPort:             TX_GETH_HTTP_PORT,
		WsPort:               TX_GETH_WS_PORT,
		ChainID:              "1337",
		GeneratedDataDir:     GENERATED_DATA_DIR_INSIDE_CONTAINER,
		JwtFileLocation:      JWT_SECRET_LOCATION_INSIDE_CONTAINER,
		PasswordFileLocation: EL_ACCOUNT_PASSWORD_FILE_INSIDE_CONTAINER,
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
