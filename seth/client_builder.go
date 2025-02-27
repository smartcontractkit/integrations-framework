package seth

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/pkg/errors"
)

const (
	NoPkForRpcHealthCheckErr  = "you need to provide at least one private key to check the RPC health"
	NoPkForNonceProtection    = "you need to provide at least one private key to enable nonce protection"
	NoPkForEphemeralKeys      = "you need to provide at least one private key to generate and fund ephemeral addresses"
	NoPkForGasPriceEstimation = "you need to provide at least one private key to enable gas price estimations"
	EthClientAndUrlsSet       = "you cannot set both EthClient and RPC URLs"
)

type ClientBuilder struct {
	config   *Config
	readonly bool
	errors   []error
}

// NewClientBuilder creates a new ClientBuilder with reasonable default values. You only need to pass private key(s) and RPC URL to build a usable config.
func NewClientBuilder() *ClientBuilder {
	network := &Network{
		Name:                           DefaultNetworkName,
		EIP1559DynamicFees:             true,
		TxnTimeout:                     MustMakeDuration(5 * time.Minute),
		DialTimeout:                    MustMakeDuration(DefaultDialTimeout),
		TransferGasFee:                 DefaultTransferGasFee,
		GasPriceEstimationEnabled:      true,
		GasPriceEstimationBlocks:       200,
		GasPriceEstimationTxPriority:   Priority_Standard,
		GasPrice:                       DefaultGasPrice,
		GasFeeCap:                      DefaultGasFeeCap,
		GasTipCap:                      DefaultGasTipCap,
		GasPriceEstimationAttemptCount: DefaultGasPriceEstimationsAttemptCount,
	}

	return &ClientBuilder{
		config: &Config{
			ArtifactsDir:          "seth_artifacts",
			EphemeralAddrs:        &ZeroInt64,
			RootKeyFundsBuffer:    &ZeroInt64,
			Network:               network,
			Networks:              []*Network{network},
			TracingLevel:          TracingLevel_Reverted,
			TraceOutputs:          []string{TraceOutput_Console, TraceOutput_DOT},
			ExperimentsEnabled:    []string{},
			CheckRpcHealthOnStart: true,
			BlockStatsConfig:      &BlockStatsConfig{RPCRateLimit: 10},
			NonceManager:          &NonceManagerCfg{KeySyncRateLimitSec: 10, KeySyncRetries: 3, KeySyncTimeout: MustMakeDuration(60 * time.Second), KeySyncRetryDelay: MustMakeDuration(5 * time.Second)},
			GasBump: &GasBumpConfig{
				Retries: 0, // bumping disabled by default
			},
		},
	}
}

// NewClientBuilderWithConfig creates a new ClientBuilder with a provided config. If it doesn't have the network set, remember to set it with `UseNetworkWithName(name string)`
// or `WithSelectedNetworkWithChainId(chainId uint64)`, before calling any of the methods that modify the Network.
func NewClientBuilderWithConfig(config *Config) *ClientBuilder {
	return &ClientBuilder{
		config: config,
	}
}

// WithRpcUrl sets the RPC URL for the config.
// Default value is an empty string (which is an incorrect value).
func (c *ClientBuilder) WithRpcUrl(url string) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}

	c.config.Network.URLs = []string{url}
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.URLs = []string{url}
	}
	return c
}

// UseNetworkWithName sets the network to use by name. If the network with the provided name is not found in the `Networks` slice, config will fail on build.
// There is no default value.
func (c *ClientBuilder) UseNetworkWithName(name string) *ClientBuilder {
	for _, network := range c.config.Networks {
		if network.Name == name {
			c.config.Network = network
			return c
		}
	}

	// if the network is not found, we will try to use the default network
	for _, network := range c.config.Networks {
		if network.Name == DefaultNetworkName {
			c.config.Network = network
			return c
		}
	}

	c.errors = append(c.errors, fmt.Errorf("network with name '%s' not found", name))
	return c
}

// UseNetworkWithChainId sets the network to use by chain ID. If the network with the provided chain ID is not found in the `Networks` slice, config will fail on build.
// There is no default value.
func (c *ClientBuilder) UseNetworkWithChainId(chainId uint64) *ClientBuilder {
	for _, network := range c.config.Networks {
		if network.ChainID == chainId {
			c.config.Network = network
			return c
		}
	}

	// if the network is not found, we will try to use the default network
	for _, network := range c.config.Networks {
		if network.Name == DefaultNetworkName {
			c.config.Network = network
			return c
		}
	}

	c.errors = append(c.errors, fmt.Errorf("network with chainId '%d' not found", chainId))
	return c
}

// WithPrivateKeys sets the private keys for the config. At least one is required to build a valid config.
// Default value is an empty slice (which is an incorrect value).
func (c *ClientBuilder) WithPrivateKeys(pks []string) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.PrivateKeys = pks
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.PrivateKeys = pks
	}
	return c
}

// WithNetworkName sets the network name, useful mostly for debugging and logging.
// Default value is "default".
func (c *ClientBuilder) WithNetworkName(name string) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.Name = name
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.Name = name
	}
	return c
}

// WithNetworkChainId sets the network chainID. If no value is set, we will ask the RPC node for the chainID.
// There is no default value.
func (c *ClientBuilder) WithNetworkChainId(chainId uint64) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.ChainID = chainId
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.ChainID = chainId
	}
	return c
}

// WithGasPriceEstimations enables or disables gas price estimations, sets the number of blocks to use for estimation or transaction priority.
// Even with estimations enabled you should still either set legacy gas price with `WithLegacyGasPrice()` or EIP-1559 dynamic fees with `WithDynamicGasPrices()`
// ss they will be used as fallback values, if the estimations fail.
// To disable gas price estimations, set the enabled parameter to false. Setting attemptCount to 0 won't disable it, it will be treated as "no value" and default to 1.
// Following priorities are supported: "slow", "standard" and "fast"
// Default values are true for enabled, 200 blocks for estimation, "standard" for priority and 1 attempt.
func (c *ClientBuilder) WithGasPriceEstimations(enabled bool, estimationBlocks uint64, txPriority string, attemptCount uint) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.GasPriceEstimationEnabled = enabled
	c.config.Network.GasPriceEstimationBlocks = estimationBlocks
	c.config.Network.GasPriceEstimationTxPriority = txPriority
	c.config.Network.GasPriceEstimationAttemptCount = attemptCount
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.GasPriceEstimationEnabled = enabled
		net.GasPriceEstimationBlocks = estimationBlocks
		net.GasPriceEstimationTxPriority = txPriority
		net.GasPriceEstimationAttemptCount = attemptCount
	}
	return c
}

// WithEIP1559DynamicFees enables or disables EIP-1559 dynamic fees. If enabled, you should set gas fee cap and gas tip cap with `WithDynamicGasPrices()`
// Default value is true.
func (c *ClientBuilder) WithEIP1559DynamicFees(enabled bool) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.EIP1559DynamicFees = enabled
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.EIP1559DynamicFees = enabled
	}
	return c
}

// WithLegacyGasPrice sets the gas price for legacy transactions that will be used only if EIP-1559 dynamic fees are disabled.
// Default value is 1 gwei.
func (c *ClientBuilder) WithLegacyGasPrice(gasPrice int64) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.GasPrice = gasPrice
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.GasPrice = gasPrice
	}
	return c
}

// WithDynamicGasPrices sets the gas fee cap and gas tip cap for EIP-1559 dynamic fees. These values will be used only if EIP-1559 dynamic fees are enabled.
// Default values are 150 gwei for gas fee cap and 50 gwei for gas tip cap.
func (c *ClientBuilder) WithDynamicGasPrices(gasFeeCap, gasTipCap int64) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.GasFeeCap = gasFeeCap
	c.config.Network.GasTipCap = gasTipCap
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.GasFeeCap = gasFeeCap
		net.GasTipCap = gasTipCap
	}
	return c
}

// WithTransferGasFee sets the gas fee for transfer transactions. This value is used, when sending funds to ephemeral keys or returning funds to root private key.
// Default value is 21_000 wei.
func (c *ClientBuilder) WithTransferGasFee(transferGasFee int64) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.TransferGasFee = transferGasFee
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.TransferGasFee = transferGasFee
	}
	return c
}

// WithGasBumping sets the number of retries for gas bumping and max gas price. You can also provide a custom bumping strategy. If the transaction is not mined within this number of retries, it will be considered failed.
// If the gas price is bumped to a value higher than max gas price, no more gas bumping will be attempted and previous gas price will be used by all subsequent attempts. If set to 0 max price is not checked.
// Default value is 0 retries. If you want to use default bumping strategy (where gas increase % based on gas_price_estimation_tx_priority), pass `nil` as the customBumpingStrategy.
func (c *ClientBuilder) WithGasBumping(retries uint, maxGasPrice int64, customBumpingStrategy GasBumpStrategyFn) *ClientBuilder {
	c.config.GasBump = &GasBumpConfig{
		Retries:     retries,
		MaxGasPrice: maxGasPrice,
		StrategyFn:  customBumpingStrategy,
	}
	return c
}

// WithTransactionTimeout sets the timeout for transactions. If the transaction is not mined within this time, it will be considered failed.
// Default value is 5 minutes.
func (c *ClientBuilder) WithTransactionTimeout(timeout time.Duration) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.TxnTimeout = MustMakeDuration(timeout)
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.TxnTimeout = MustMakeDuration(timeout)
	}
	return c
}

// WithRpcDialTimeout sets the timeout for dialing the RPC server. If the connection is not established within this time, it will be considered failed.
// Default value is 1 minute.
func (c *ClientBuilder) WithRpcDialTimeout(timeout time.Duration) *ClientBuilder {
	if !c.checkIfNetworkIsSet() {
		return c
	}
	c.config.Network.DialTimeout = MustMakeDuration(timeout)
	// defensive programming
	if len(c.config.Networks) == 0 {
		c.config.Networks = append(c.config.Networks, c.config.Network)
	} else if net := c.config.findNetworkByName(c.config.Network.Name); net != nil {
		net.DialTimeout = MustMakeDuration(timeout)
	}
	return c
}

// WithEphemeralAddresses sets the number of ephemeral addresses to generate and the amount of funds to keep in the root private key.
// Default values are 0 for ephemeral addresses and 0 for root key funds buffer.
func (c *ClientBuilder) WithEphemeralAddresses(ephemeralAddressCount, rootKeyBufferAmount int64) *ClientBuilder {
	c.config.EphemeralAddrs = &ephemeralAddressCount
	c.config.RootKeyFundsBuffer = &rootKeyBufferAmount

	return c
}

// WithTracing sets the tracing level and outputs. Tracing level can be one of: "all", "reverted", "none". Outputs can be one or more of: "console", "dot" or "json".
// Default values are "reverted" and ["console", "dot"].
func (c *ClientBuilder) WithTracing(level string, outputs []string) *ClientBuilder {
	c.config.TracingLevel = level
	c.config.TraceOutputs = outputs
	return c
}

// WithProtections enables or disables nonce protection (fails, when key has a pending transaction, and you try to submit another one) and node health check on startup.
// Default values are false for nonce protection, true for node health check and 1 minute timeout.
func (c *ClientBuilder) WithProtections(pendingNonceProtectionEnabled, nodeHealthStartupCheck bool, pendingNonceProtectionTimeout *Duration) *ClientBuilder {
	c.config.PendingNonceProtectionEnabled = pendingNonceProtectionEnabled
	c.config.CheckRpcHealthOnStart = nodeHealthStartupCheck
	c.config.PendingNonceProtectionTimeout = pendingNonceProtectionTimeout
	return c
}

// WithArtifactsFolder sets the folder where the Seth artifacts such as DOT graphs or JSON will be saved.
// Default value is "seth_artifacts".
func (c *ClientBuilder) WithArtifactsFolder(folder string) *ClientBuilder {
	c.config.ArtifactsDir = folder
	return c
}

// WithGethWrappersFolders sets list of folders where the Geth wrappers are stored. Seth will load ABIs from all wrappers it finds in theses folders (including subfolders).
// Default value is an empty string (= loading disabled).
func (c *ClientBuilder) WithGethWrappersFolders(folders []string) *ClientBuilder {
	c.config.GethWrappersDirs = folders
	return c
}

// WithNonceManager sets the rate limit for key sync, number of retries, timeout and retry delay.
// Default values are 10 calls per second, 3 retires, 60s timeout and 5s retry delay.
func (c *ClientBuilder) WithNonceManager(rateLimitSec int, retries uint, timeout, retryDelay time.Duration) *ClientBuilder {
	c.config.NonceManager = &NonceManagerCfg{
		KeySyncRateLimitSec: rateLimitSec,
		KeySyncRetries:      retries,
		KeySyncTimeout:      MustMakeDuration(timeout),
		KeySyncRetryDelay:   MustMakeDuration(retryDelay),
	}

	return c
}

// WithEthClient sets the ethclient to use. It means that the URL you pass will be ignored and the client will use the provided ethclient,
// but what it allows you is to use Geth's Simulated Backend or similar implementations for testing.
// Default value is nil.
func (c *ClientBuilder) WithEthClient(ethclient simulated.Client) *ClientBuilder {
	c.config.ethclient = ethclient
	return c
}

func (c *ClientBuilder) WithHooks(hooks Hooks) *ClientBuilder {
	c.config.Hooks = &hooks
	return c
}

// WithReadOnlyMode sets the client to read-only mode. It removes all private keys from all Networks and disables nonce protection and ephemeral addresses.
func (c *ClientBuilder) WithReadOnlyMode() *ClientBuilder {
	c.readonly = true
	return c
}

// Build creates a new Client from the builder.
func (c *ClientBuilder) Build() (*Client, error) {
	config, err := c.BuildConfig()
	if err != nil {
		return nil, err
	}
	return NewClientWithConfig(config)
}

// BuildConfig returns the config from the builder.
func (c *ClientBuilder) BuildConfig() (*Config, error) {
	c.handleReadOnlyMode()
	c.validateConfig()
	if len(c.errors) > 0 {
		var concatenatedErrors string
		for _, err := range c.errors {
			concatenatedErrors = fmt.Sprintf("%s\n%s", concatenatedErrors, err.Error())
		}
		return nil, fmt.Errorf("errors occurred during building the config:%s", concatenatedErrors)
	}
	return c.config, nil
}

func (c *ClientBuilder) handleReadOnlyMode() {
	if c.readonly {
		c.config.PendingNonceProtectionEnabled = false
		c.config.CheckRpcHealthOnStart = false
		c.config.EphemeralAddrs = nil
		c.readonly = true
		if c.config.Network != nil {
			c.config.Network.GasPriceEstimationEnabled = false
			c.config.Network.PrivateKeys = []string{}
		}

		for i := range c.config.Networks {
			c.config.Networks[i].PrivateKeys = []string{}
		}
	}
}

func (c *ClientBuilder) validateConfig() {
	if c.config.Network != nil {
		if len(c.config.Network.PrivateKeys) == 0 && c.config.CheckRpcHealthOnStart {
			c.errors = append(c.errors, errors.New(NoPkForRpcHealthCheckErr))
		}
		if len(c.config.Network.PrivateKeys) == 0 && c.config.PendingNonceProtectionEnabled {
			c.errors = append(c.errors, errors.New(NoPkForNonceProtection))
		}
		if len(c.config.Network.PrivateKeys) == 0 && c.config.EphemeralAddrs != nil && *c.config.EphemeralAddrs > 0 {
			c.errors = append(c.errors, errors.New(NoPkForEphemeralKeys))
		}
		if len(c.config.Network.PrivateKeys) == 0 && c.config.Network.GasPriceEstimationEnabled {
			c.errors = append(c.errors, errors.New(NoPkForGasPriceEstimation))
		}
		if len(c.config.Network.URLs) > 0 && c.config.ethclient != nil {
			c.errors = append(c.errors, errors.New(EthClientAndUrlsSet))
		}
	}
}

func (c *ClientBuilder) checkIfNetworkIsSet() bool {
	if c.config.Network == nil {
		c.errors = append(c.errors, errors.New("at least one method that required network to be set was called, but network is nil"))
		return false
	}
	return true
}
