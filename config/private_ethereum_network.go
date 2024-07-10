package config

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog"
	tc "github.com/testcontainers/testcontainers-go"

	"github.com/smartcontractkit/chainlink-testing-framework/logging"
	"github.com/smartcontractkit/chainlink-testing-framework/utils/slice"
)

var (
	ErrMissingEthereumVersion = errors.New("ethereum version is required")
	ErrMissingExecutionLayer  = errors.New("execution layer is required")
	Eth1NotSupportedByRethMsg = "eth1 is not supported by Reth, please use eth2"
	DefaultNodeLogLevel       = "info"
)

type EthereumNetworkConfig struct {
	ConsensusType        *EthereumVersion          `toml:"consensus_type"`
	EthereumVersion      *EthereumVersion          `toml:"ethereum_version"`
	ConsensusLayer       *ConsensusLayer           `toml:"consensus_layer"`
	ExecutionLayer       *ExecutionLayer           `toml:"execution_layer"`
	DockerNetworkNames   []string                  `toml:"docker_network_names"`
	Containers           EthereumNetworkContainers `toml:"containers"`
	WaitForFinalization  *bool                     `toml:"wait_for_finalization"`
	GeneratedDataHostDir *string                   `toml:"generated_data_host_dir"`
	ValKeysDir           *string                   `toml:"val_keys_dir"`
	EthereumChainConfig  *EthereumChainConfig      `toml:"EthereumChainConfig"`
	CustomDockerImages   map[ContainerType]string  `toml:"CustomDockerImages"`
	NodeLogLevel         *string                   `toml:"node_log_level,omitempty"`
}

func (en *EthereumNetworkConfig) Validate() error {
	l := logging.GetTestLogger(nil)

	// logically it doesn't belong here, but placing it here guarantees it will always run without changing API
	if en.EthereumVersion != nil && en.ConsensusType != nil {
		l.Warn().Msg("Both EthereumVersion and ConsensusType are set. ConsensusType as a _deprecated_ field will be ignored")
	}

	if en.EthereumVersion == nil && en.ConsensusType != nil {
		l.Debug().Msg("Using _deprecated_ ConsensusType as EthereumVersion")
		tempEthVersion := en.ConsensusType
		switch *tempEthVersion {
		case EthereumVersion_Eth1, EthereumVersion_Eth1_Legacy:
			*tempEthVersion = EthereumVersion_Eth1
		case EthereumVersion_Eth2, EthereumVersion_Eth2_Legacy:
			*tempEthVersion = EthereumVersion_Eth2
		default:
			return fmt.Errorf("unknown ethereum version (consensus type): %s", *en.ConsensusType)
		}

		en.EthereumVersion = tempEthVersion
	}

	if (en.EthereumVersion == nil || *en.EthereumVersion == "") && len(en.CustomDockerImages) == 0 {
		return ErrMissingEthereumVersion
	}

	if (en.ExecutionLayer == nil || *en.ExecutionLayer == "") && len(en.CustomDockerImages) == 0 {
		return ErrMissingExecutionLayer
	}

	if (en.EthereumVersion != nil && (*en.EthereumVersion == EthereumVersion_Eth2_Legacy || *en.EthereumVersion == EthereumVersion_Eth2)) && (en.ConsensusLayer == nil || *en.ConsensusLayer == "") {
		l.Warn().Msg("Consensus layer is not set, but is required for PoS. Defaulting to Prysm")
		en.ConsensusLayer = &ConsensusLayer_Prysm
	}

	if (en.EthereumVersion != nil && (*en.EthereumVersion == EthereumVersion_Eth1_Legacy || *en.EthereumVersion == EthereumVersion_Eth1)) && (en.ConsensusLayer != nil && *en.ConsensusLayer != "") {
		l.Warn().Msg("Consensus layer is set, but is not allowed for PoW. Ignoring")
		en.ConsensusLayer = nil
	}

	if en.NodeLogLevel == nil {
		en.NodeLogLevel = &DefaultNodeLogLevel
	}

	if *en.EthereumVersion == EthereumVersion_Eth1 && *en.ExecutionLayer == ExecutionLayer_Reth {
		msg := `%s

If you are using builder to create the network, please change the EthereumVersion to EthereumVersion_Eth2 by calling this method:
WithEthereumVersion(config.EthereumVersion_Eth2).

If you are using a TOML file, please change the EthereumVersion to "eth2" in the TOML file:
[PrivateEthereumNetwork]
ethereum_version="eth2"
`
		return fmt.Errorf(msg, Eth1NotSupportedByRethMsg)
	}

	switch strings.ToLower(*en.NodeLogLevel) {
	case "trace", "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid node log level: %s", *en.NodeLogLevel)
	}

	if en.EthereumChainConfig == nil {
		return errors.New("ethereum chain config is required")
	}

	return en.EthereumChainConfig.Validate(l, en.EthereumVersion)
}

func (en *EthereumNetworkConfig) ApplyOverrides(from *EthereumNetworkConfig) error {
	if from == nil {
		return nil
	}
	if from.ConsensusLayer != nil {
		en.ConsensusLayer = from.ConsensusLayer
	}
	if from.ExecutionLayer != nil {
		en.ExecutionLayer = from.ExecutionLayer
	}
	if from.EthereumVersion != nil {
		en.EthereumVersion = from.EthereumVersion
	}
	if from.WaitForFinalization != nil {
		en.WaitForFinalization = from.WaitForFinalization
	}

	if from.EthereumChainConfig != nil {
		if en.EthereumChainConfig == nil {
			en.EthereumChainConfig = from.EthereumChainConfig
		} else {
			err := en.EthereumChainConfig.ApplyOverrides(from.EthereumChainConfig)
			if err != nil {
				return fmt.Errorf("error applying overrides from network config file to config: %w", err)
			}
		}
	}

	return nil
}

func (en *EthereumNetworkConfig) Describe() string {
	cL := "prysm"
	if en.ConsensusLayer == nil {
		cL = "(none)"
	}
	return fmt.Sprintf("ethereum version: %s, execution layer: %s, consensus layer: %s", *en.EthereumVersion, *en.ExecutionLayer, cL)
}

type EthereumNetworkContainer struct {
	ContainerName string        `toml:"container_name"`
	ContainerType ContainerType `toml:"container_type"`
	Container     *tc.Container `toml:"-"`
}

// Deprecated: use EthereumVersion instead
type ConsensusType string

const (
	// Deprecated: use EthereumVersion_Eth2 instead
	ConsensusType_PoS ConsensusType = "pos"
	// Deprecated: use EthereumVersion_Eth1 instead
	ConsensusType_PoW ConsensusType = "pow"
)

type EthereumVersion string

const (
	EthereumVersion_Eth2 EthereumVersion = "eth2"
	// Deprecated: use EthereumVersion_Eth2 instead
	EthereumVersion_Eth2_Legacy EthereumVersion = "pos"
	EthereumVersion_Eth1        EthereumVersion = "eth1"
	// Deprecated: use EthereumVersion_Eth1 instead
	EthereumVersion_Eth1_Legacy EthereumVersion = "pow"
)

type ExecutionLayer string

const (
	ExecutionLayer_Geth       ExecutionLayer = "geth"
	ExecutionLayer_Nethermind ExecutionLayer = "nethermind"
	ExecutionLayer_Erigon     ExecutionLayer = "erigon"
	ExecutionLayer_Besu       ExecutionLayer = "besu"
	ExecutionLayer_Reth       ExecutionLayer = "reth"
)

type ConsensusLayer string

var ConsensusLayer_Prysm ConsensusLayer = "prysm"

type EthereumNetworkContainers []EthereumNetworkContainer

type ContainerType string

const (
	ContainerType_ExecutionLayer     ContainerType = "execution_layer"
	ContainerType_ConsensusLayer     ContainerType = "consensus_layer"
	ContainerType_ConsensusValidator ContainerType = "consensus_validator"
	ContainerType_GenesisGenerator   ContainerType = "genesis_generator"
	ContainerType_ValKeysGenerator   ContainerType = "val_keys_generator"
)

const (
	UnsopportedForkErr = "only 'Electra' and 'EOF' hard forks are supported"
)

type EthereumChainConfig struct {
	SecondsPerSlot   int            `json:"seconds_per_slot" toml:"seconds_per_slot"`
	SlotsPerEpoch    int            `json:"slots_per_epoch" toml:"slots_per_epoch"`
	GenesisDelay     int            `json:"genesis_delay" toml:"genesis_delay"`
	ValidatorCount   int            `json:"validator_count" toml:"validator_count"`
	ChainID          int            `json:"chain_id" toml:"chain_id"`
	GenesisTimestamp int            // this is not serialized
	AddressesToFund  []string       `json:"addresses_to_fund" toml:"addresses_to_fund"`
	HardForkEpochs   map[string]int `json:"HardForkEpochs" toml:"HardForkEpochs"`
}

//go:embed tomls/default_ethereum_env.toml
var defaultEthereumChainConfig []byte

func (c *EthereumChainConfig) Default() error {
	wrapper := struct {
		EthereumNetwork *EthereumNetworkConfig `toml:"PrivateEthereumNetwork"`
	}{}
	if err := toml.Unmarshal(defaultEthereumChainConfig, &wrapper); err != nil {
		return fmt.Errorf("error unmarshalling ethereum network config: %w", err)
	}

	if wrapper.EthereumNetwork == nil {
		return errors.New("[EthereumNetwork] was not present in default TOML file")
	}

	*c = *wrapper.EthereumNetwork.EthereumChainConfig

	if c.GenesisTimestamp == 0 {
		c.GenerateGenesisTimestamp()
	}

	return nil
}

func GetDefaultChainConfig() EthereumChainConfig {
	config := EthereumChainConfig{}
	if err := config.Default(); err != nil {
		panic(err)
	}
	return config
}

func (c *EthereumChainConfig) Validate(logger zerolog.Logger, ethereumVersion *EthereumVersion) error {
	if c.ChainID < 1 {
		return fmt.Errorf("chain id must be >= 0")
	}

	// don't like it 100% but in cases where we load private ethereum network config from TOML it might be incomplete
	// until we pass it to ethereum network builder that will fill in defaults
	if ethereumVersion == nil || (*ethereumVersion == EthereumVersion_Eth1_Legacy || *ethereumVersion == EthereumVersion_Eth1) {
		return nil
	}

	if c.ValidatorCount < 4 {
		return fmt.Errorf("validator count must be >= 4")
	}
	if c.SecondsPerSlot < 3 {
		return fmt.Errorf("seconds per slot must be >= 3")
	}
	if c.SlotsPerEpoch < 2 {
		return fmt.Errorf("slots per epoch must be >= 1")
	}
	if c.GenesisDelay < 10 {
		return fmt.Errorf("genesis delay must be >= 10")
	}
	if c.GenesisTimestamp == 0 {
		return fmt.Errorf("genesis timestamp must be generated by calling GenerateGenesisTimestamp()")
	}

	if err := c.ValidateHardForks(); err != nil {
		return err
	}

	var err error
	var hadDuplicates bool
	// we need to deduplicate addresses to fund, because if present they will crash the genesis
	c.AddressesToFund, hadDuplicates, err = slice.ValidateAndDeduplicateAddresses(c.AddressesToFund)
	if err != nil {
		return err
	}
	if hadDuplicates {
		logger.Warn().Msg("Duplicate addresses found in addresses_to_fund. Removed them. You might want to review your configuration.")
	}

	return nil
}

func (c *EthereumChainConfig) ValidateHardForks() error {
	if len(c.HardForkEpochs) == 0 {
		return nil
	}

	// currently Prysm Beacon Chain doesn't support any fork (Electra is coming in 2025)
	c.HardForkEpochs = map[string]int{}

	return nil
}

func (c *EthereumChainConfig) ApplyOverrides(from *EthereumChainConfig) error {
	if from == nil {
		return nil
	}
	if from.ValidatorCount != 0 {
		c.ValidatorCount = from.ValidatorCount
	}
	if from.SecondsPerSlot != 0 {
		c.SecondsPerSlot = from.SecondsPerSlot
	}
	if from.SlotsPerEpoch != 0 {
		c.SlotsPerEpoch = from.SlotsPerEpoch
	}
	if from.GenesisDelay != 0 {
		c.GenesisDelay = from.GenesisDelay
	}
	if from.ChainID != 0 {
		c.ChainID = from.ChainID
	}
	if len(from.AddressesToFund) != 0 {
		c.AddressesToFund = append([]string{}, from.AddressesToFund...)
	}
	return nil
}

func (c *EthereumChainConfig) FillInMissingValuesWithDefault() {
	defaultConfig := GetDefaultChainConfig()
	if c.ValidatorCount == 0 {
		c.ValidatorCount = defaultConfig.ValidatorCount
	}
	if c.SecondsPerSlot == 0 {
		c.SecondsPerSlot = defaultConfig.SecondsPerSlot
	}
	if c.SlotsPerEpoch == 0 {
		c.SlotsPerEpoch = defaultConfig.SlotsPerEpoch
	}
	if c.GenesisDelay == 0 {
		c.GenesisDelay = defaultConfig.GenesisDelay
	}
	if c.ChainID == 0 {
		c.ChainID = defaultConfig.ChainID
	}
	if len(c.AddressesToFund) == 0 {
		c.AddressesToFund = append([]string{}, defaultConfig.AddressesToFund...)
	} else {
		c.AddressesToFund = append(append([]string{}, c.AddressesToFund...), defaultConfig.AddressesToFund...)
	}

	if len(c.HardForkEpochs) == 0 {
		c.HardForkEpochs = defaultConfig.HardForkEpochs
	}
}

func (c *EthereumChainConfig) GetValidatorBasedGenesisDelay() int {
	return c.ValidatorCount * 5
}

func (c *EthereumChainConfig) GenerateGenesisTimestamp() {
	c.GenesisTimestamp = int(time.Now().Unix()) + c.GetValidatorBasedGenesisDelay()
}

func (c *EthereumChainConfig) GetDefaultWaitDuration() time.Duration {
	return time.Duration((c.GenesisDelay+c.GetValidatorBasedGenesisDelay())*2) * time.Second
}

func (c *EthereumChainConfig) GetDefaultFinalizationWaitDuration() time.Duration {
	return 5 * time.Minute
}
