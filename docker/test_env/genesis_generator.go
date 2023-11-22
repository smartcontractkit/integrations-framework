package test_env

import (
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/smartcontractkit/chainlink-testing-framework/docker"
)

type EthGenesisGeneretor struct {
	EnvComponent
	beaconChainConfig    EthereumChainConfig
	l                    zerolog.Logger
	generatedDataHostDir string
}

func NewEthGenesisGenerator(beaconChainConfig EthereumChainConfig, generatedDataHostDir string, opts ...EnvComponentOption) *EthGenesisGeneretor {
	g := &EthGenesisGeneretor{
		EnvComponent: EnvComponent{
			ContainerName: fmt.Sprintf("%s-%s", "eth-genesis-generator", uuid.NewString()[0:8]),
		},
		beaconChainConfig:    beaconChainConfig,
		generatedDataHostDir: generatedDataHostDir,
		l:                    log.Logger,
	}
	for _, opt := range opts {
		opt(&g.EnvComponent)
	}
	return g
}

func (g *EthGenesisGeneretor) WithLogger(l zerolog.Logger) *EthGenesisGeneretor {
	g.l = l
	return g
}

func (g *EthGenesisGeneretor) StartContainer() error {
	r, err := g.getContainerRequest(g.Networks)
	if err != nil {
		return err
	}

	_, err = docker.StartContainerWithRetry(g.l, tc.GenericContainerRequest{
		ContainerRequest: *r,
		Reuse:            true,
		Started:          true,
		Logger:           &g.l,
	})
	if err != nil {
		return errors.Wrapf(err, "cannot start eth genesis generation container")
	}

	g.l.Info().Str("containerName", g.ContainerName).
		Msg("Started Eth Genesis container")

	return nil
}

func (g *EthGenesisGeneretor) getContainerRequest(networks []string) (*tc.ContainerRequest, error) {
	valuesEnv, err := os.CreateTemp("", "values.env")
	if err != nil {
		return nil, err
	}

	bc, err := generateEnvValues(&g.beaconChainConfig)
	if err != nil {
		return nil, err
	}
	_, err = valuesEnv.WriteString(bc)
	if err != nil {
		return nil, err
	}

	elGenesisFile, err := os.CreateTemp("", "genesis-config.yaml")
	if err != nil {
		return nil, err
	}
	_, err = elGenesisFile.WriteString(elGenesisConfig)
	if err != nil {
		return nil, err
	}

	clGenesisFile, err := os.CreateTemp("", "config.yaml")
	if err != nil {
		return nil, err
	}
	_, err = clGenesisFile.WriteString(clGenesisConfig)
	if err != nil {
		return nil, err
	}

	mnemonicsFile, err := os.CreateTemp("", "mnemonics.yaml")
	if err != nil {
		return nil, err
	}
	_, err = mnemonicsFile.WriteString(mnemonics)
	if err != nil {
		return nil, err
	}

	jwtSecretFile, err := os.CreateTemp("/tmp", "jwtsecret")
	if err != nil {
		return nil, err
	}
	_, err = jwtSecretFile.WriteString("0xfad2709d0bb03bf0e8ba3c99bea194575d3e98863133d1af638ed056d1d59345")
	if err != nil {
		return nil, err
	}

	return &tc.ContainerRequest{
		Name:          g.ContainerName,
		Image:         "tofelb/ethereum-genesis-generator:2.0.4-slots-per-epoch",
		ImagePlatform: "linux/x86_64",
		Networks:      networks,
		WaitingFor: tcwait.ForAll(
			tcwait.ForLog("+ terminalTotalDifficulty=0"),
			tcwait.ForLog("+ sed -i 's/TERMINAL_TOTAL_DIFFICULTY:.*/TERMINAL_TOTAL_DIFFICULTY: 0/' /data/custom_config_data/config.yaml").
				WithStartupTimeout(20*time.Second).
				WithPollInterval(1*time.Second),
		),
		Cmd: []string{"all"},
		Files: []tc.ContainerFile{
			{
				HostFilePath:      valuesEnv.Name(),
				ContainerFilePath: "/config/values.env",
				FileMode:          0644,
			},
			{
				HostFilePath:      elGenesisFile.Name(),
				ContainerFilePath: "/config/el/genesis-config.yaml",
				FileMode:          0644,
			},
			{
				HostFilePath:      clGenesisFile.Name(),
				ContainerFilePath: "/config/cl/config.yaml",
				FileMode:          0644,
			},
			{
				HostFilePath:      mnemonicsFile.Name(),
				ContainerFilePath: "/config/cl/mnemonics.yaml",
				FileMode:          0644,
			},
			{
				HostFilePath:      jwtSecretFile.Name(),
				ContainerFilePath: JWT_SECRET_LOCATION_INSIDE_CONTAINER,
				FileMode:          0644,
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
