package test_env

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-testing-framework/docker"
)

const (
	PRYSM_QUERY_RPC_PORT = "3500"
	PRYSM_NODE_RPC_PORT  = "4000"
	PRYSM_IMAGE_TAG      = "v4.1.1-debug"
)

type PrysmBeaconChain struct {
	EnvComponent
	InternalBeaconRpcProvider string
	InternalQueryRpcUrl       string
	ExternalBeaconRpcProvider string
	ExternalQueryRpcUrl       string
	generatedDataHostDir      string
	gethInternalExecutionURL  string
	beaconChainConfig         EthereumChainConfig
	l                         zerolog.Logger
}

type PrysmValidator struct {
	EnvComponent
	internalBeaconRpcProvider string
	valKeysDir                string
	generatedDataHostDir      string
	beaconChainConfig         EthereumChainConfig
	l                         zerolog.Logger
}

func NewPrysmBeaconChain(networks []string, beaconChainConfig EthereumChainConfig, customConfigDataDir, gethExecutionURL string, opts ...EnvComponentOption) *PrysmBeaconChain {
	g := &PrysmBeaconChain{
		EnvComponent: EnvComponent{
			ContainerName: fmt.Sprintf("%s-%s", "prysm-beacon-chain", uuid.NewString()[0:8]),
			Networks:      networks,
		},
		beaconChainConfig:        beaconChainConfig,
		generatedDataHostDir:     customConfigDataDir,
		gethInternalExecutionURL: gethExecutionURL,
		l:                        log.Logger,
	}
	for _, opt := range opts {
		opt(&g.EnvComponent)
	}
	return g
}

func (g *PrysmBeaconChain) WithLogger(l zerolog.Logger) *PrysmBeaconChain {
	g.l = l
	return g
}

func (g *PrysmBeaconChain) StartContainer() error {
	r, err := g.getContainerRequest(g.Networks)
	if err != nil {
		return err
	}

	ct, err := docker.StartContainerWithRetry(g.l, tc.GenericContainerRequest{
		ContainerRequest: *r,
		Reuse:            true,
		Started:          true,
		Logger:           &g.l,
	})
	if err != nil {
		return errors.Wrapf(err, "cannot start prysm beacon chain container")
	}

	host, err := GetHost(context.Background(), ct)
	if err != nil {
		return err
	}
	queryPort, err := ct.MappedPort(context.Background(), NatPort(PRYSM_QUERY_RPC_PORT))
	if err != nil {
		return err
	}

	externalRcpPort, err := ct.MappedPort(context.Background(), NatPort(PRYSM_NODE_RPC_PORT))
	if err != nil {
		return err
	}

	_ = externalRcpPort

	g.Container = ct
	g.InternalBeaconRpcProvider = fmt.Sprintf("%s:%s", g.ContainerName, PRYSM_NODE_RPC_PORT)
	g.InternalQueryRpcUrl = fmt.Sprintf("%s:%s", g.ContainerName, PRYSM_QUERY_RPC_PORT)
	g.ExternalBeaconRpcProvider = FormatHttpUrl(host, externalRcpPort.Port())
	g.ExternalQueryRpcUrl = FormatHttpUrl(host, queryPort.Port())

	g.l.Info().Str("containerName", g.ContainerName).
		Msg("Started Prysm Beacon Chain container")

	return nil
}

func (g *PrysmBeaconChain) getContainerRequest(networks []string) (*tc.ContainerRequest, error) {
	return &tc.ContainerRequest{
		Name:          g.ContainerName,
		Image:         fmt.Sprintf("gcr.io/prysmaticlabs/prysm/beacon-chain:%s", PRYSM_IMAGE_TAG),
		ImagePlatform: "linux/amd64",
		Networks:      networks,
		WaitingFor: tcwait.ForAll(
			tcwait.ForLog("Starting beacon node").
				WithStartupTimeout(g.beaconChainConfig.GetDefaultWaitDuration()).
				WithPollInterval(2 * time.Second),
		),
		Cmd: []string{
			"--accept-terms-of-use",
			"--datadir=/consensus-data",
			fmt.Sprintf("--chain-config-file=%s/config.yaml", GENERATED_DATA_DIR_INSIDE_CONTAINER),
			fmt.Sprintf("--genesis-state=%s/genesis.ssz", GENERATED_DATA_DIR_INSIDE_CONTAINER),
			fmt.Sprintf("--execution-endpoint=%s", g.gethInternalExecutionURL),
			"--rpc-host=0.0.0.0",
			"--grpc-gateway-host=0.0.0.0",
			"--grpc-gateway-corsdomain=*",
			"--suggested-fee-recipient=0x8943545177806ED17B9F23F0a21ee5948eCaa776",
			"--subscribe-all-subnets=true",
			fmt.Sprintf("--jwt-secret=%s/jwtsecret", GENERATED_DATA_DIR_INSIDE_CONTAINER),
			// mine, modify when running multi-node
			"--minimum-peers-per-subnet=0",
			"--min-sync-peers=0",
		},
		ExposedPorts: []string{NatPortFormat(PRYSM_NODE_RPC_PORT), NatPortFormat(PRYSM_QUERY_RPC_PORT)},
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

func NewPrysmValidator(networks []string, beaconChainConfig EthereumChainConfig, generatedDataHostDir, valKeysDir, internalBeaconRpcProvider string, opts ...EnvComponentOption) *PrysmValidator {
	g := &PrysmValidator{
		EnvComponent: EnvComponent{
			ContainerName: fmt.Sprintf("%s-%s", "prysm-validator", uuid.NewString()[0:8]),
			Networks:      networks,
		},
		generatedDataHostDir:      generatedDataHostDir,
		valKeysDir:                valKeysDir,
		internalBeaconRpcProvider: internalBeaconRpcProvider,
		l:                         log.Logger,
	}
	for _, opt := range opts {
		opt(&g.EnvComponent)
	}
	return g
}

func (g *PrysmValidator) WithLogger(l zerolog.Logger) *PrysmValidator {
	g.l = l
	return g
}

func (g *PrysmValidator) StartContainer() error {
	r, err := g.getContainerRequest(g.Networks)
	if err != nil {
		return err
	}

	ct, err := docker.StartContainerWithRetry(g.l, tc.GenericContainerRequest{
		ContainerRequest: *r,
		Reuse:            true,
		Started:          true,
		Logger:           &g.l,
	})
	if err != nil {
		return errors.Wrapf(err, "cannot start prysm validator container")
	}

	g.Container = ct

	g.l.Info().Str("containerName", g.ContainerName).
		Msg("Started Prysm Validator container")

	return nil
}

func (g *PrysmValidator) getContainerRequest(networks []string) (*tc.ContainerRequest, error) {
	return &tc.ContainerRequest{
		Name:          g.ContainerName,
		Image:         fmt.Sprintf("gcr.io/prysmaticlabs/prysm/validator:%s", PRYSM_IMAGE_TAG),
		Networks:      networks,
		ImagePlatform: "linux/x86_64",
		WaitingFor: tcwait.ForAll(
			tcwait.ForLog("Beacon chain started").
				WithStartupTimeout(200 * time.Second).
				WithPollInterval(2 * time.Second),
		),
		Cmd: []string{
			"--accept-terms-of-use",
			fmt.Sprintf("--chain-config-file=%s/config.yaml", GENERATED_DATA_DIR_INSIDE_CONTAINER),
			fmt.Sprintf("--beacon-rpc-provider=%s", g.internalBeaconRpcProvider),
			"--datadir=/consensus-data",
			"--suggested-fee-recipient=0x8943545177806ED17B9F23F0a21ee5948eCaa776",
			fmt.Sprintf("--wallet-dir=%s/prysm", NODE_0_DIR_INSIDE_CONTAINER),
			fmt.Sprintf("--wallet-password-file=%s", VALIDATOR_WALLET_PASSWORD_FILE_INSIDE_CONTAINER),
		},
		Mounts: tc.ContainerMounts{
			tc.ContainerMount{
				Source: tc.GenericBindMountSource{
					HostPath: g.valKeysDir,
				},
				Target: tc.ContainerMountTarget(GENERATED_VALIDATOR_KEYS_DIR_INSIDE_CONTAINER),
			},
			tc.ContainerMount{
				Source: tc.GenericBindMountSource{
					HostPath: g.generatedDataHostDir,
				},
				Target: tc.ContainerMountTarget(GENERATED_DATA_DIR_INSIDE_CONTAINER),
			},
		},
	}, nil
}
