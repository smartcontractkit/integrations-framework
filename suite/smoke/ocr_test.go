package smoke

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/helmenv/tools"
	"github.com/smartcontractkit/integrations-framework/actions"
	"github.com/smartcontractkit/integrations-framework/client"
	"github.com/smartcontractkit/integrations-framework/contracts"
	"github.com/smartcontractkit/integrations-framework/utils"
	"path/filepath"
)

var _ = Describe("OCR Feed @ocr", func() {
	var (
		err               error
		env               *environment.Environment
		networks          *client.Networks
		contractDeployer  contracts.ContractDeployer
		linkTokenContract contracts.LinkToken
		chainlinkNodes    []client.Chainlink
		mockserver        *client.MockserverClient
	)
	ocrInstances := make([]contracts.OffchainAggregator, 1)
	BeforeEach(func() {
		By("Deploying the environment", func() {
			env, err = environment.DeployOrLoadEnvironment(
				environment.NewChainlinkConfig(environment.ChainlinkReplicas(6, nil)),
				tools.ChartsRoot,
			)
			Expect(err).ShouldNot(HaveOccurred())
			err = env.ConnectAll()
			Expect(err).ShouldNot(HaveOccurred())
		})
		By("Getting the clients", func() {
			networkRegistry := client.NewNetworkRegistry()
			var err error
			networks, err = networkRegistry.GetNetworks(filepath.Join(utils.ProjectRoot, "networks.yaml"), env)
			Expect(err).ShouldNot(HaveOccurred())
			contractDeployer, err = contracts.NewContractDeployer(networks.Default)
			Expect(err).ShouldNot(HaveOccurred())
			chainlinkNodes, err = client.NewChainlinkClients(env)
			Expect(err).ShouldNot(HaveOccurred())
			mockserver, err = client.NewMockServerClientFromEnv(env)
			Expect(err).ShouldNot(HaveOccurred())
			networks.Default.ParallelTransactions(true)
			Expect(err).ShouldNot(HaveOccurred())
			linkTokenContract, err = contractDeployer.DeployLinkTokenContract()
			Expect(err).ShouldNot(HaveOccurred())
		})
		By("Funding Chainlink nodes", actions.FundNodes(networks, chainlinkNodes))
		By("Deploying OCR contracts",
			actions.DeployOCRContracts(ocrInstances, linkTokenContract, contractDeployer, chainlinkNodes, networks))
		By("Creating OCR jobs", actions.CreateOCRJobs(ocrInstances, chainlinkNodes, mockserver))
	})

	Describe("with OCR job", func() {
		It("performs two rounds", func() {
			By("setting adapter responses",
				actions.SetAdapterResponses([]int{5, 5, 5, 5, 5}, ocrInstances, chainlinkNodes, mockserver))
			By("starting new round", actions.StartNewRound(1, ocrInstances, networks))

			answer, err := ocrInstances[0].GetLatestAnswer(context.Background())
			Expect(err).ShouldNot(HaveOccurred())
			Expect(answer.Int64()).Should(Equal(int64(5)), "latest answer from OCR is not as expected")

			By("setting adapter responses",
				actions.SetAdapterResponses([]int{10, 10, 10, 10, 10}, ocrInstances, chainlinkNodes, mockserver))
			By("starting new round", actions.StartNewRound(2, ocrInstances, networks))

			answer, err = ocrInstances[0].GetLatestAnswer(context.Background())
			Expect(err).ShouldNot(HaveOccurred())
			Expect(answer.Int64()).Should(Equal(int64(10)), "latest answer from OCR is not as expected")
		})
	})

	AfterEach(func() {
		By("Printing gas stats", func() {
			networks.Default.GasStats().PrintStats()
		})
		By("Tearing down the environment", func() {
			err = actions.TeardownSuite(env, networks)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})
