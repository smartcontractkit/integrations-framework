package client

import (
	"integrations-framework/config"

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
		networkID BlockchainNetworkID,
		privateKeyString string,
		address string,
	) {
		networkConfig, err := initFunc(conf, networkID)
		Expect(err).ShouldNot(HaveOccurred())
		wallets, err := networkConfig.Wallets()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(wallets.Default().PrivateKey()).To(Equal(privateKeyString))
		Expect(address).To(Equal(wallets.Default().Address()))
	},
		Entry("on Ethereum Hardhat", NewEthereumNetwork, EthereumHardhatID,
			"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
			"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"),
	)
})
