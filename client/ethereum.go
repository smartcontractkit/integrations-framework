package client

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	storage "integrations-framework/storage"
)

// Etherum client that wraps the go-ethereum client and adds some helper methods
type EthereumClient struct {
	Client *ethclient.Client
}

// Builds a new ethereum client based on a connection string
func NewEthereumClient(rpcConnectionString string) EthereumClient {
	cl, err := ethclient.Dial(rpcConnectionString)
	if err != nil {
		log.Fatal(err)
	}

	return EthereumClient{Client: cl}
}

// Creates a default contract (need to parameterize this)
func (clientWrapper EthereumClient) CreateContract() (contractAddress string, err error) {
	privateKey, err := crypto.HexToECDSA("HexPrivateKey")
	if err != nil {
		log.Fatal(err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("cannot assert type: publicKey is not of type *ecdsa.PublicKey")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	nonce, err := clientWrapper.Client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		log.Fatal(err)
	}

	gasPrice, err := clientWrapper.Client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	auth := bind.NewKeyedTransactor(privateKey)
	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)     // in wei
	auth.GasLimit = uint64(300000) // in units
	auth.GasPrice = gasPrice

	address, tx, instance, err := storage.DeployStorage(auth, clientWrapper.Client, "1.0")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(address.Hex())
	fmt.Println(tx.Hash().Hex())

	_ = instance
	return "", nil
}
