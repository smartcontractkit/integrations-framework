package client

import (
	"integrations-framework/config"
	"math/big"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client", func() {
	var conf *config.Config

	BeforeEach(func() {
		var err error
		conf, err = config.NewConfigWithPath(config.LocalConfig, "../config")
		Expect(err).ShouldNot(HaveOccurred())
	})

	DescribeTable("create new wallet configurations", func(
		initFunc BlockchainNetworkInit,
		privateKey string,
		address string,
	) {
		networkConfig := initFunc(conf)
		wallets, err := networkConfig.Wallets()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(privateKey).To(Equal(wallets.Default().PrivateKey()))
		Expect(address).To(Equal(wallets.Default().Address()))
	},
		Entry("Ethereum Hardhat", NewEthereumHardhat, "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", "WRONG0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"),
	)

	DescribeTable("deploy the storage contract", func(initFunc BlockchainNetworkInit) {
		networkConfig := initFunc(conf)
		client, err := NewBlockchainClient(networkConfig)
		Expect(err).ShouldNot(HaveOccurred())
		wallets, err := networkConfig.Wallets()
		Expect(err).ShouldNot(HaveOccurred())
		err = client.DeployStorageContract(wallets.Default())
		Expect(err).ShouldNot(HaveOccurred())
	},
		Entry("Ethereum Hardhat", NewEthereumHardhat),
	)

	DescribeTable("create new wallet configurations", func(
		initFunc BlockchainNetworkInit,
		privateKey string,
		address string,
	) {
		networkConfig := initFunc(conf)
		wallets, err := networkConfig.Wallets()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(privateKey).To(Equal(wallets.Default().PrivateKey()))
		Expect(address).To(Equal(wallets.Default().Address()))
	},
		Entry("Ethereum Hardhat", NewEthereumHardhat,
			"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
			"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"),
	)

	DescribeTable("send basic ETH transactions", func(
		initFunc BlockchainNetworkInit,
	) {
		networkConfig := initFunc(conf)
		wallets, err := networkConfig.Wallets()
		Expect(err).ShouldNot(HaveOccurred())
		client, err := NewBlockchainClient(networkConfig)
		Expect(err).ShouldNot(HaveOccurred())

		toWallet, err := wallets.Wallet(1)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = client.SendNativeTransaction(wallets.Default(), toWallet.Address(), big.NewInt(500))
		Expect(err).ShouldNot(HaveOccurred())

		// Get latest block and make sure our transaction went through
		// This can be tricky, there's lots to potentially parameterize around this
		// Current idea I'm working with is to include a txTimeout config variable
		// There's more potential issues I thought of but forgot
	},
		Entry("Ethereum Hardhat", NewEthereumHardhat),
	)
})

func TestTransaction(t *testing.T) {

}
