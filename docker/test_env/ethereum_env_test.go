package test_env

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pelletier/go-toml/v2"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"
	"github.com/smartcontractkit/chainlink-testing-framework/logging"
)

func TestEth2CustomConfig(t *testing.T) {
	l := logging.GetTestLogger(t)

	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithConsensusType(EthereumVersion_Eth2_Legacy).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithEthereumChainConfig(EthereumChainConfig{
			SecondsPerSlot: 6,
			SlotsPerEpoch:  2,
		}).
		Build()
	require.NoError(t, err, "Builder validation failed")

	net, _, err := cfg.Start()
	require.NoError(t, err, "Couldn't start PoS network")

	c, err := blockchain.ConnectEVMClient(net, l)
	require.NoError(t, err, "Couldn't connect to the evm client")
	err = c.Close()
	require.NoError(t, err, "Couldn't close the client")
}

func TestEth2ExtraFunding(t *testing.T) {
	l := logging.GetTestLogger(t)

	addressToFund := "0x14dc79964da2c08b23698b3d3cc7ca32193d9955"

	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithConsensusType(EthereumVersion_Eth2_Legacy).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithEthereumChainConfig(EthereumChainConfig{
			AddressesToFund: []string{addressToFund},
		}).
		Build()
	require.NoError(t, err, "Builder validation failed")

	net, _, err := cfg.Start()
	require.NoError(t, err, "Couldn't start PoS network")

	c, err := blockchain.ConnectEVMClient(net, l)
	require.NoError(t, err, "Couldn't connect to the evm client")

	balance, err := c.BalanceAt(context.Background(), common.HexToAddress(addressToFund))
	require.NoError(t, err, "Couldn't get balance")
	require.Equal(t, "11515845246265065472", fmt.Sprintf("%d", balance.Uint64()), "Balance is not correct")

	err = c.Close()
	require.NoError(t, err, "Couldn't close the client")
}

func TestEth2WithPrysmAndGethReuseNetwork(t *testing.T) {
	l := logging.GetTestLogger(t)

	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithConsensusType(EthereumVersion_Eth2_Legacy).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		Build()
	require.NoError(t, err, "Builder validation failed")

	_, _, err = cfg.Start()
	require.NoError(t, err, "Couldn't start PoS network")

	newBuilder := NewEthereumNetworkBuilder()
	reusedCfg, err := newBuilder.
		WithExistingConfig(cfg).
		Build()
	require.NoError(t, err, "Builder validation failed")

	net, _, err := reusedCfg.Start()
	require.NoError(t, err, "Couldn't reuse PoS network")

	c, err := blockchain.ConnectEVMClient(net, l)
	require.NoError(t, err, "Couldn't connect to the evm client")
	err = c.Close()
	require.NoError(t, err, "Couldn't close the client")
}

func TestEth2WithPrysmAndGethReuseFromEnv(t *testing.T) {
	t.Skip("for demonstration purposes only")
	l := logging.GetTestLogger(t)

	os.Setenv(CONFIG_ENV_VAR_NAME, "change-me-to-the-path-of-your-config-file")
	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WihtExistingConfigFromEnvVar().
		Build()
	require.NoError(t, err, "Builder validation failed")

	net, _, err := cfg.Start()
	require.NoError(t, err, "Couldn't start PoS network")

	c, err := blockchain.ConnectEVMClient(net, l)
	require.NoError(t, err, "Couldn't connect to the evm client")
	err = c.Close()
	require.NoError(t, err, "Couldn't close the client")
}

func TestEth2ExecClientFromToml(t *testing.T) {
	toml := `
	[EthereumNetwork]
	consensus_type="pos"
	consensus_layer="prysm"
	execution_layer="besu"
	wait_for_finalization=false

	[EthereumNetwork.EthereumChainConfig]
	seconds_per_slot=12
	slots_per_epoch=2
	genesis_delay=20
	validator_count=8
	chain_id=1234
	addresses_to_fund=["0x742d35Cc6634C0532925a3b844Bc454e4438f44e", "0x742d35Cc6634C0532925a3b844Bc454e4438f44f"]
	`

	tomlCfg, err := readEthereumNetworkConfig(toml)
	require.NoError(t, err, "Couldn't read config")

	tomlCfg.EthereumChainConfig.GenerateGenesisTimestamp()

	err = tomlCfg.Validate()
	require.NoError(t, err, "Couldn't validate TOML config")

	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithExistingConfig(tomlCfg).
		Build()
	require.NoError(t, err, "Builder validation failed")
	require.Equal(t, ExecutionLayer_Besu, *cfg.ExecutionLayer, "Execution layer should be Besu")
	require.NotNil(t, cfg.ConsensusLayer, "Consensus layer should not be nil")
	require.Equal(t, ConsensusLayer_Prysm, *cfg.ConsensusLayer, "Consensus layer should be Prysm")
	require.Equal(t, EthereumVersion_Eth2_Legacy, *cfg.EthereumVersion, "Consensus type should be PoS")
	require.NotNil(t, cfg.WaitForFinalization, "Wait for finalization should not be nil")
	require.False(t, *cfg.WaitForFinalization, "Wait for finalization should be false")
	require.Equal(t, 2, len(cfg.EthereumChainConfig.AddressesToFund), "Should have 2 addresses to fund")
	require.Equal(t, 12, cfg.EthereumChainConfig.SecondsPerSlot, "Seconds per slot should be 12")
	require.Equal(t, 2, cfg.EthereumChainConfig.SlotsPerEpoch, "Slots per epoch should be 2")
	require.Equal(t, 20, cfg.EthereumChainConfig.GenesisDelay, "Genesis delay should be 20")
	require.Equal(t, 8, cfg.EthereumChainConfig.ValidatorCount, "Validator count should be 8")
	require.Equal(t, 1234, cfg.EthereumChainConfig.ChainID, "Chain ID should be 1234")
}

func TestCustomDockerImagesFromToml(t *testing.T) {
	toml := `
	[EthereumNetwork]
	consensus_type="pos"
	consensus_layer="prysm"
	execution_layer="geth"
	wait_for_finalization=false

	[EthereumNetwork.EthereumChainConfig]
	seconds_per_slot=12
	slots_per_epoch=2
	genesis_delay=20
	validator_count=8
	chain_id=1234
	addresses_to_fund=["0x742d35Cc6634C0532925a3b844Bc454e4438f44e", "0x742d35Cc6634C0532925a3b844Bc454e4438f44f"]

	[EthereumNetwork.CustomDockerImages]
	geth="i-dont-exist:tag-me"
	`

	tomlCfg, err := readEthereumNetworkConfig(toml)
	require.NoError(t, err, "Couldn't read config")

	tomlCfg.EthereumChainConfig.GenerateGenesisTimestamp()

	err = tomlCfg.Validate()
	require.NoError(t, err, "Couldn't validate TOML config")

	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithExistingConfig(tomlCfg).
		Build()
	require.NoError(t, err, "Builder validation failed")

	_, _, err = cfg.Start()
	require.Error(t, err, "Could start PoS network using incorrect image")
}

type ethereumNetworkWrapper struct {
	EthereumNetwork *EthereumNetwork `toml:"EthereumNetwork"`
}

func readEthereumNetworkConfig(configDecoded string) (EthereumNetwork, error) {
	var net ethereumNetworkWrapper
	err := toml.Unmarshal([]byte(configDecoded), &net)
	if err != nil {
		return EthereumNetwork{}, fmt.Errorf("error unmarshaling ethereum network config: %w", err)
	}

	return *net.EthereumNetwork, nil
}

func TestEth2CustomDockerNetworks(t *testing.T) {
	networks := []string{"test-network"}

	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithConsensusType(EthereumVersion_Eth2_Legacy).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithDockerNetworks(networks).
		Build()
	require.NoError(t, err, "Builder validation failed")
	require.Equal(t, networks, cfg.DockerNetworkNames, "Incorrect docker networks in config")
}

func TestEth2CustomImages(t *testing.T) {
	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithConsensusType(EthereumVersion_Eth2_Legacy).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithCustomDockerImages(map[ContainerType]string{
			ContainerType_Geth: "i-dont-exist:tag-me"}).
		Build()
	require.NoError(t, err, "Builder validation failed")

	_, _, err = cfg.Start()
	require.Error(t, err, "Could start PoS network using incorrect image")
}

func TestEth2DenebHardFork(t *testing.T) {
	l := logging.GetTestLogger(t)

	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithConsensusType(EthereumVersion_Eth2_Legacy).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithEthereumChainConfig(EthereumChainConfig{
			HardForkEpochs: map[string]int{"Deneb": 1},
		}).
		Build()
	require.NoError(t, err, "Builder validation failed")

	net, _, err := cfg.Start()
	require.NoError(t, err, "Couldn't start PoS network")

	c, err := blockchain.ConnectEVMClient(net, l)
	require.NoError(t, err, "Couldn't connect to the evm client")
	balance, err := c.BalanceAt(context.Background(), common.HexToAddress("0x14dc79964da2c08b23698b3d3cc7ca32193d9955"))
	require.NoError(t, err, "Couldn't get balance")
	require.Equal(t, "0", fmt.Sprintf("%d", balance.Uint64()), "Balance is not correct")

	err = c.Close()
	require.NoError(t, err, "Couldn't close the client")
}

func TestEth2InvalidHardForks(t *testing.T) {
	builder := NewEthereumNetworkBuilder()
	_, err := builder.
		WithConsensusType(EthereumVersion_Eth2_Legacy).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithEthereumChainConfig(EthereumChainConfig{
			HardForkEpochs: map[string]int{"Deneb": 0},
		}).
		Build()
	require.Error(t, err, "Builder validation failed")
	require.Contains(t, err.Error(), "hard fork Deneb epoch must be >= 1")

	builder = NewEthereumNetworkBuilder()
	_, err = builder.
		WithConsensusType(EthereumVersion_Eth2_Legacy).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithEthereumChainConfig(EthereumChainConfig{
			HardForkEpochs: map[string]int{"Electra": 1},
		}).
		Build()
	require.Error(t, err, "Builder validation failed")
	require.Contains(t, err.Error(), UnsopportedForkErr)

	builder = NewEthereumNetworkBuilder()
	_, err = builder.
		WithConsensusType(EthereumVersion_Eth2_Legacy).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithEthereumChainConfig(EthereumChainConfig{
			HardForkEpochs: map[string]int{"Electra": 1, "Deneb": 1},
		}).
		Build()
	require.Error(t, err, "Builder validation failed")
	require.Contains(t, err.Error(), UnsopportedForkErr)
}

func TestVersionDependentConsensusPoWMinor(t *testing.T) {
	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithExecutionLayer(ExecutionLayer_Nethermind).
		WithCustomDockerImages(map[ContainerType]string{
			ContainerType_Geth: "nethermind/nethermind:1.13.2"}).
		Build()
	require.NoError(t, err, "Builder validation failed")
	require.Equal(t, EthereumVersion_Eth1_Legacy, *cfg.EthereumVersion, "Consensus type should be PoW")
	require.Nil(t, cfg.ConsensusLayer, "Consensus layer should be nil")
}

func TestVersionDependentConsensusPoSMinor(t *testing.T) {
	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithExecutionLayer(ExecutionLayer_Nethermind).
		WithCustomDockerImages(map[ContainerType]string{
			ContainerType_Geth: "nethermind/nethermind:1.14.0"}).
		Build()
	require.NoError(t, err, "Builder validation failed")
	require.Equal(t, EthereumVersion_Eth2_Legacy, *cfg.EthereumVersion, "Consensus type should be PoS")
	require.Equal(t, ConsensusLayer_Prysm, *cfg.ConsensusLayer, "Consensus layer should be Prysm")
}

func TestVersionDependentConsensusRc(t *testing.T) {
	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithConsensusType(EthereumVersion_Auto).
		WithExecutionLayer(ExecutionLayer_Nethermind).
		WithCustomDockerImages(map[ContainerType]string{
			ContainerType_Geth: "nethermind/nethermind:1.17.0-RC2"}).
		Build()
	require.NoError(t, err, "Builder validation failed")
	require.Equal(t, EthereumVersion_Eth2_Legacy, *cfg.EthereumVersion, "Consensus type should be PoS")
	require.Equal(t, ConsensusLayer_Prysm, *cfg.ConsensusLayer, "Consensus layer should be Prysm")
}

func TestVersionDependentConsensusWithV(t *testing.T) {
	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithConsensusType(EthereumVersion_Auto).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithCustomDockerImages(map[ContainerType]string{
			ContainerType_Geth: "ethereum/client-go:v1.13.10"}).
		Build()
	require.NoError(t, err, "Builder validation failed")
	require.Equal(t, EthereumVersion_Eth2_Legacy, *cfg.EthereumVersion, "Consensus type should be PoS")
	require.Equal(t, ConsensusLayer_Prysm, *cfg.ConsensusLayer, "Consensus layer should be Prysm")
}

func TestVersionDependentConsensusOnlyMajor(t *testing.T) {
	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithCustomDockerImages(map[ContainerType]string{
			ContainerType_Geth: "ethereum/client-go:v1.13"}).
		Build()
	require.NoError(t, err, "Builder validation failed")
	require.Equal(t, EthereumVersion_Eth2_Legacy, *cfg.EthereumVersion, "Consensus type should be PoS")
	require.Equal(t, ConsensusLayer_Prysm, *cfg.ConsensusLayer, "Consensus layer should be Prysm")
}

func TestLatestVersionFromGithub(t *testing.T) {
	builder := NewEthereumNetworkBuilder()
	cfg, err := builder.
		WithCustomDockerImages(map[ContainerType]string{
			ContainerType_Besu: fmt.Sprintf("hyperledger/besu:%s", AUTOMATIC_STABLE_LATEST_TAG)}).
		Build()
	require.NoError(t, err, "Builder validation failed")
	require.Equal(t, EthereumVersion_Eth2_Legacy, *cfg.EthereumVersion, "Consensus type should be PoS")
	require.Equal(t, ConsensusLayer_Prysm, *cfg.ConsensusLayer, "Consensus layer should be Prysm")
	require.NotContains(t, cfg.CustomDockerImages[ContainerType_Besu], AUTOMATIC_STABLE_LATEST_TAG, "Automatic tag should be replaced")
}

func TestMischmachedExecutionClient(t *testing.T) {
	builder := NewEthereumNetworkBuilder()
	_, err := builder.
		WithCustomDockerImages(map[ContainerType]string{
			ContainerType_Geth: fmt.Sprintf("hyperledger/besu:%s", AUTOMATIC_LATEST_TAG)}).
		Build()
	require.Error(t, err, "Builder validation succeeded")
	require.Equal(t, ErrMismatchedExecutionClient.Error(), err.Error(), "Error message is not correct")
}
