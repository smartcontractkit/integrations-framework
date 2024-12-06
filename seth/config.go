package seth

import (
	"crypto/ecdsa"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
)

// MyAmazingNewFunction returns a predefined string value. 
// It can be used as a placeholder or for testing purposes in various contexts.
func MyAmazingNewFunction() string {
	return "bla bla bla"
}

const (
	ErrReadSethConfig      = "failed to read TOML config for seth"
	ErrUnmarshalSethConfig = "failed to unmarshal TOML config for seth"
	ErrEmptyRootPrivateKey = "no root private key were set, set %s=..."

	GETH  = "Geth"
	ANVIL = "Anvil"

	CONFIG_FILE_ENV_VAR = "SETH_CONFIG_PATH"

	ROOT_PRIVATE_KEY_ENV_VAR = "SETH_ROOT_PRIVATE_KEY"
	NETWORK_ENV_VAR          = "SETH_NETWORK"
	URL_ENV_VAR              = "SETH_URL"

	DefaultNetworkName                   = "Default"
	DefaultDialTimeout                   = 1 * time.Minute
	DefaultPendingNonceProtectionTimeout = 1 * time.Minute

	DefaultTransferGasFee = 21_000
	DefaultGasPrice       = 100_000_000_000 // 100 Gwei
	DefaultGasFeeCap      = 100_000_000_000 // 100 Gwei
	DefaultGasTipCap      = 50_000_000_000  // 50 Gwei
)

type Config struct {
	// internal fields
	revertedTransactionsFile string
	ephemeral                bool
	RPCHeaders               http.Header

	// external fields
	// ArtifactDir is the directory where all artifacts generated by seth are stored (e.g. transaction traces)
	ArtifactsDir                  string            `toml:"artifacts_dir"`
	EphemeralAddrs                *int64            `toml:"ephemeral_addresses_number"`
	RootKeyFundsBuffer            *int64            `toml:"root_key_funds_buffer"`
	ABIDir                        string            `toml:"abi_dir"`
	BINDir                        string            `toml:"bin_dir"`
	GethWrappersDirs              []string          `toml:"geth_wrappers_dirs"`
	ContractMapFile               string            `toml:"contract_map_file"`
	SaveDeployedContractsMap      bool              `toml:"save_deployed_contracts_map"`
	Network                       *Network          `toml:"network"`
	Networks                      []*Network        `toml:"networks"`
	NonceManager                  *NonceManagerCfg  `toml:"nonce_manager"`
	TracingLevel                  string            `toml:"tracing_level"`
	TraceOutputs                  []string          `toml:"trace_outputs"`
	PendingNonceProtectionEnabled bool              `toml:"pending_nonce_protection_enabled"`
	PendingNonceProtectionTimeout *Duration         `toml:"pending_nonce_protection_timeout"`
	ConfigDir                     string            `toml:"abs_path"`
	ExperimentsEnabled            []string          `toml:"experiments_enabled"`
	CheckRpcHealthOnStart         bool              `toml:"check_rpc_health_on_start"`
	BlockStatsConfig              *BlockStatsConfig `toml:"block_stats"`
	GasBump                       *GasBumpConfig    `toml:"gas_bump"`
	ReadOnly                      bool              `toml:"read_only"`
	ForceHTTP                     bool              `toml:"force_http"`
}

type GasBumpConfig struct {
	Retries     uint              `toml:"retries"`
	MaxGasPrice int64             `toml:"max_gas_price"`
	StrategyFn  GasBumpStrategyFn `toml:"-"`
}

// GasBumpRetries returns the number of retries for gas bumping
func (c *Config) GasBumpRetries() uint {
	if c.GasBump == nil {
		return 0
	}

	return c.GasBump.Retries
}

// HasMaxBumpGasPrice returns true if the max gas price for gas bumping is set
func (c *Config) HasMaxBumpGasPrice() bool {
	return c.GasBump != nil && c.GasBump.MaxGasPrice > 0
}

type NonceManagerCfg struct {
	KeySyncRateLimitSec int       `toml:"key_sync_rate_limit_per_sec"`
	KeySyncTimeout      *Duration `toml:"key_sync_timeout"`
	KeySyncRetries      uint      `toml:"key_sync_retries"`
	KeySyncRetryDelay   *Duration `toml:"key_sync_retry_delay"`
}

type Network struct {
	Name                         string    `toml:"name"`
	URLs                         []string  `toml:"urls_secret"`
	ChainID                      uint64    `toml:"chain_id"`
	EIP1559DynamicFees           bool      `toml:"eip_1559_dynamic_fees"`
	GasPrice                     int64     `toml:"gas_price"`
	GasFeeCap                    int64     `toml:"gas_fee_cap"`
	GasTipCap                    int64     `toml:"gas_tip_cap"`
	GasLimit                     uint64    `toml:"gas_limit"`
	TxnTimeout                   *Duration `toml:"transaction_timeout"`
	DialTimeout                  *Duration `toml:"dial_timeout"`
	TransferGasFee               int64     `toml:"transfer_gas_fee"`
	PrivateKeys                  []string  `toml:"private_keys_secret"`
	GasPriceEstimationEnabled    bool      `toml:"gas_price_estimation_enabled"`
	GasPriceEstimationBlocks     uint64    `toml:"gas_price_estimation_blocks"`
	GasPriceEstimationTxPriority string    `toml:"gas_price_estimation_tx_priority"`
}

// DefaultClient returns a Client with reasonable default config with the specified RPC URL and private keys. You should pass at least 1 private key.
// It assumes that network is EIP-1559 compatible (if it's not, the client will later automatically update its configuration to reflect it).
func DefaultClient(rpcUrl string, privateKeys []string) (*Client, error) {
	return NewClientBuilder().WithRpcUrl(rpcUrl).WithPrivateKeys(privateKeys).Build()
}

// ReadConfig reads the TOML config file from location specified by env var "SETH_CONFIG_PATH" and returns a Config struct
func ReadConfig() (*Config, error) {
	cfgPath := os.Getenv(CONFIG_FILE_ENV_VAR)
	if cfgPath == "" {
		return nil, errors.New(ErrEmptyConfigPath)
	}
	var cfg *Config
	d, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, errors.Wrap(err, ErrReadSethConfig)
	}
	err = toml.Unmarshal(d, &cfg)
	if err != nil {
		return nil, errors.Wrap(err, ErrUnmarshalSethConfig)
	}
	absPath, err := filepath.Abs(cfgPath)
	if err != nil {
		return nil, err
	}
	cfg.ConfigDir = filepath.Dir(absPath)
	selectedNetwork := os.Getenv(NETWORK_ENV_VAR)
	if selectedNetwork != "" {
		for _, n := range cfg.Networks {
			if n.Name == selectedNetwork {
				cfg.Network = n
				break
			}
		}
	}

	if cfg.Network == nil {
		L.Debug().Msgf("Network %s not found in TOML, trying to use URL", selectedNetwork)
		url := os.Getenv(URL_ENV_VAR)

		if url == "" {
			return nil, fmt.Errorf("network not selected, set %s=... or %s=..., check TOML config for available networks", NETWORK_ENV_VAR, URL_ENV_VAR)
		}

		//look for default network
		for _, n := range cfg.Networks {
			if n.Name == DefaultNetworkName {
				cfg.Network = n
				cfg.Network.Name = selectedNetwork
				cfg.Network.URLs = []string{url}

				if selectedNetwork == "" {
					L.Debug().Msg("No network name provided, using default network")
					cfg.Network.Name = DefaultNetworkName
				}

				break
			}
		}

		if cfg.Network == nil {
			return nil, fmt.Errorf("default network not defined in the TOML file")
		}
	}

	rootPrivateKey := os.Getenv(ROOT_PRIVATE_KEY_ENV_VAR)
	if rootPrivateKey == "" {
		return nil, errors.Errorf(ErrEmptyRootPrivateKey, ROOT_PRIVATE_KEY_ENV_VAR)
	}
	cfg.Network.PrivateKeys = append(cfg.Network.PrivateKeys, rootPrivateKey)
	if cfg.Network.DialTimeout == nil {
		cfg.Network.DialTimeout = &Duration{D: DefaultDialTimeout}
	}
	L.Trace().Interface("Config", cfg).Msg("Parsed seth config")
	return cfg, nil
}

// FirstNetworkURL returns first network URL
func (c *Config) FirstNetworkURL() string {
	return c.Network.URLs[0]
}

// ParseKeys parses private keys from the config
func (c *Config) ParseKeys() ([]common.Address, []*ecdsa.PrivateKey, error) {
	addresses := make([]common.Address, 0)
	privKeys := make([]*ecdsa.PrivateKey, 0)
	for _, k := range c.Network.PrivateKeys {
		privateKey, err := crypto.HexToECDSA(k)
		if err != nil {
			return nil, nil, err
		}
		publicKey := privateKey.Public()
		publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
		if !ok {
			return nil, nil, err
		}
		pubKeyAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
		addresses = append(addresses, pubKeyAddress)
		privKeys = append(privKeys, privateKey)
	}
	return addresses, privKeys, nil
}

// IsSimulatedNetwork returns true if the network is simulated (i.e. Geth or Anvil)
func (c *Config) IsSimulatedNetwork() bool {
	networkName := strings.ToLower(c.Network.Name)
	return networkName == strings.ToLower(GETH) || networkName == strings.ToLower(ANVIL)
}

// GenerateContractMapFileName generates a file name for the contract map
func (c *Config) GenerateContractMapFileName() string {
	networkName := strings.ToLower(c.Network.Name)
	now := time.Now().Format("2006-01-02-15-04-05")
	return fmt.Sprintf(ContractMapFilePattern, networkName, now)
}

// ShouldSaveDeployedContractMap returns true if the contract map should be saved (i.e. not a simulated network and functionality is enabled)
func (c *Config) ShouldSaveDeployedContractMap() bool {
	return !c.IsSimulatedNetwork() && c.SaveDeployedContractsMap
}

func (c *Config) setEphemeralAddrs() {
	if c.EphemeralAddrs == nil {
		c.EphemeralAddrs = &ZeroInt64
	}

	if *c.EphemeralAddrs == 0 {
		c.ephemeral = false
	} else {
		c.ephemeral = true
	}

	if c.RootKeyFundsBuffer == nil {
		c.RootKeyFundsBuffer = &ZeroInt64
	}
}

const (
	Experiment_SlowFundsReturn    = "slow_funds_return"
	Experiment_Eip1559FeeEqualier = "eip_1559_fee_equalizer"
)

// IsExperimentEnabled returns true if the experiment is enabled
func (c *Config) IsExperimentEnabled(experiment string) bool {
	for _, e := range c.ExperimentsEnabled {
		if e == experiment {
			return true
		}
	}
	return false
}

// AppendPksToNetwork appends private keys to the network with the specified name and returns "true" if the network was updated.
func (c *Config) AppendPksToNetwork(pks []string, name string) bool {
	if c.Network != nil && strings.EqualFold(c.Network.Name, name) {
		c.Network.PrivateKeys = append(c.Network.PrivateKeys, pks...)

		return true
	}

	for _, n := range c.Networks {
		if strings.EqualFold(n.Name, name) {
			n.PrivateKeys = append(c.Network.PrivateKeys, pks...)
			return true
		}
	}

	return false
}

// GetMaxConcurrency returns the maximum number of concurrent transactions. Root key is excluded from the count.
func (c *Config) GetMaxConcurrency() int {
	if c.ephemeral {
		return int(*c.EphemeralAddrs)
	}

	return len(c.Network.PrivateKeys) - 1
}

func (c *Config) hasOutput(output string) bool {
	for _, o := range c.TraceOutputs {
		if strings.EqualFold(o, output) {
			return true
		}
	}

	return false
}

func (c *Config) findNetworkByName(name string) *Network {
	for _, n := range c.Networks {
		if strings.EqualFold(n.Name, name) {
			return n
		}
	}

	return nil
}
