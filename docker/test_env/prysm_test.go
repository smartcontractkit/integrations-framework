package test_env

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"
	"github.com/smartcontractkit/chainlink-testing-framework/logging"
)

func TestEth2WithPrysmAndGethDefaultConfig(t *testing.T) {
	l := logging.GetTestLogger(t)

	builder := NewEthereumNetworkBuilder(t)
	_, eth2, err := builder.
		WithConsensusType(ConsensusType_PoS).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		Start()
	require.NoError(t, err)

	ns := blockchain.SimulatedEVMNetwork
	ns.URLs = eth2.PublicWsUrsl()
	c, err := blockchain.ConnectEVMClient(ns, l)
	require.NoError(t, err, "Couldn't connect to the evm client")
	err = c.Close()
	require.NoError(t, err, "Couldn't close the client")
}

func TestEth2WithPrysmAndGethCustomConfig(t *testing.T) {
	l := logging.GetTestLogger(t)

	builder := NewEthereumNetworkBuilder(t)
	err := builder.
		WithConsensusType(ConsensusType_PoS).
		WithConsensusLayer(ConsensusLayer_Prysm).
		WithExecutionLayer(ExecutionLayer_Geth).
		WithBeaconChainConfig(BeaconChainConfig{
			SecondsPerSlot: 4,
			SlotsPerEpoch:  2,
		}).
		Build()
	require.NoError(t, err, "Builder validation failed")

	_, eth2, err := builder.Start()
	require.NoError(t, err, "Couldn't start PoS network")

	ns := blockchain.SimulatedEVMNetwork
	ns.URLs = eth2.PublicWsUrsl()
	c, err := blockchain.ConnectEVMClient(ns, l)
	require.NoError(t, err, "Couldn't connect to the evm client")
	err = c.Close()
	require.NoError(t, err, "Couldn't close the client")
}

func TestEth1WithGeth(t *testing.T) {
	l := logging.GetTestLogger(t)

	builder := NewEthereumNetworkBuilder(t)
	err := builder.
		WithConsensusType(ConsensusType_PoW).
		WithExecutionLayer(ExecutionLayer_Geth).
		Build()
	require.NoError(t, err, "Builder validation failed")

	_, eth2, err := builder.Start()
	require.NoError(t, err, "Couldn't start PoW network")

	ns := blockchain.SimulatedEVMNetwork
	ns.URLs = eth2.PublicWsUrsl()
	c, err := blockchain.ConnectEVMClient(ns, l)
	require.NoError(t, err, "Couldn't connect to the evm client")
	err = c.Close()
	require.NoError(t, err, "Couldn't close the client")
}
