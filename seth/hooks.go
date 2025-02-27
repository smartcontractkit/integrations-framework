package seth

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
)

type Hooks struct {
	ContractDeployment ContractDeploymentHooks
	Decode             DecodeHooks
}

type PreDeployHook func(auth *bind.TransactOpts, name string, abi abi.ABI, bytecode []byte, params ...interface{}) error
type PostDeployHook func(client *Client, tx *types.Transaction) error

type ContractDeploymentHooks struct {
	Pre  PreDeployHook
	Post PostDeployHook
}

type PreDecodeHook func(client *Client) error
type PostDecodeHook func(client *Client, decodedTx *DecodedTransaction, decodedErr error) error

type DecodeHooks struct {
	Pre  PreDecodeHook
	Post PostDecodeHook
}
