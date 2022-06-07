package smoke

//revive:disable:dot-imports
import (
	"context"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"github.com/smartcontractkit/chainlink-env/environment"
	"github.com/smartcontractkit/chainlink-env/pkg/cdk8s/blockscout"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/chainlink"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/mockserver"
	mockservercfg "github.com/smartcontractkit/chainlink-env/pkg/helm/mockserver-cfg"
	"github.com/smartcontractkit/chainlink-env/pkg/helm/reorg"
	"github.com/smartcontractkit/chainlink-testing-framework/actions"
	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"
	"github.com/smartcontractkit/chainlink-testing-framework/client"
	"github.com/smartcontractkit/chainlink-testing-framework/config"
	"github.com/smartcontractkit/chainlink-testing-framework/contracts"
	"github.com/smartcontractkit/chainlink-testing-framework/suite/chaos"
	"github.com/smartcontractkit/chainlink-testing-framework/utils"
)

var _ = Describe("Direct request suite @direct-request", func() {
	var (
		err            error
		nets           *blockchain.Networks
		cd             contracts.ContractDeployer
		chainlinkNodes []client.Chainlink
		oracle         contracts.Oracle
		consumer       contracts.APIConsumer
		jobUUID        uuid.UUID
		ms             *client.MockserverClient
		e              *environment.Environment
	)
	reorgBlocks := 50
	minIncomingConfirmations := "200"
	EVMFinalityDepth := "200"
	EVMTrackerHistoryDepth := "400"
	timeout := "15m"
	interval := "2s"
	BeforeEach(func() {
		By("Deploying the environment", func() {
			e = environment.New(&environment.Config{TTL: 48 * time.Hour})
			err := e.
				AddHelm(mockservercfg.New(nil)).
				AddHelm(mockserver.New(nil)).
				AddChart(blockscout.New(&blockscout.Props{
					WsURL:   "ws://geth-ethereum-geth:8546",
					HttpURL: "http://geth-ethereum-geth:8544",
				})).
				AddHelm(reorg.New(&reorg.Props{
					NetworkName: "geth",
					NetworkType: "geth-reorg",
					Values: map[string]interface{}{
						"geth": map[string]interface{}{
							"genesis": map[string]interface{}{
								"networkId": "1337",
							},
						},
					},
				})).
				Run()
			Expect(err).ShouldNot(HaveOccurred(), "Environment deployment shouldn't fail")
			// related https://app.shortcut.com/chainlinklabs/story/38295/creating-an-evm-chain-via-cli-or-api-immediately-polling-the-nodes-and-returning-an-error
			// node must work and reconnect even if network is not working
			time.Sleep(90 * time.Second)
			err = e.AddHelm(chainlink.New(map[string]interface{}{
				"env": map[string]interface{}{
					"eth_url":                        "ws://geth-ethereum-geth:8546",
					"eth_http_url":                   "http://geth-ethereum-geth:8544",
					"eth_chain_id":                   "1337",
					"ETH_FINALITY_DEPTH":             EVMFinalityDepth,
					"ETH_HEAD_TRACKER_HISTORY_DEPTH": EVMTrackerHistoryDepth,
				},
			})).Run()
			Expect(err).ShouldNot(HaveOccurred(), "Environment deployment shouldn't fail")
		})

		By("Connecting to launched resources", func() {
			networkRegistry := blockchain.NewDefaultNetworkRegistry()
			nets, err = networkRegistry.GetNetworks(e)
			Expect(err).ShouldNot(HaveOccurred(), "Connecting to blockchain nodes shouldn't fail")
			cd, err = contracts.NewContractDeployer(nets.Default)
			Expect(err).ShouldNot(HaveOccurred(), "Deploying contracts shouldn't fail")
			chainlinkNodes, err = client.ConnectChainlinkNodes(e)
			Expect(err).ShouldNot(HaveOccurred(), "Connecting to chainlink nodes shouldn't fail")
			ms, err = client.ConnectMockServer(e)
			Expect(err).ShouldNot(HaveOccurred())
		})

		By("Funding Chainlink nodes", func() {
			err = actions.FundChainlinkNodes(chainlinkNodes, nets.Default, big.NewFloat(10))
			Expect(err).ShouldNot(HaveOccurred(), "Funding chainlink nodes with ETH shouldn't fail")
		})

		By("Deploying contracts", func() {
			lt, err := cd.DeployLinkTokenContract()
			Expect(err).ShouldNot(HaveOccurred(), "Deploying Link Token Contract shouldn't fail")
			oracle, err = cd.DeployOracle(lt.Address())
			Expect(err).ShouldNot(HaveOccurred(), "Deploying Oracle Contract shouldn't fail")
			consumer, err = cd.DeployAPIConsumer(lt.Address())
			Expect(err).ShouldNot(HaveOccurred(), "Deploying Consumer Contract shouldn't fail")
			err = nets.Default.SetDefaultWallet(0)
			Expect(err).ShouldNot(HaveOccurred(), "Setting default wallet shouldn't fail")
			err = lt.Transfer(consumer.Address(), big.NewInt(2e18))
			Expect(err).ShouldNot(HaveOccurred(), "Transferring %d to consumer contract shouldn't fail", big.NewInt(2e18))
		})

		By("Creating directrequest job", func() {
			err = ms.SetValuePath("/variable", 5)
			Expect(err).ShouldNot(HaveOccurred(), "Setting mockserver value path shouldn't fail")

			jobUUID = uuid.NewV4()

			bta := client.BridgeTypeAttributes{
				Name: fmt.Sprintf("five-%s", jobUUID.String()),
				URL:  fmt.Sprintf("%s/variable", ms.Config.ClusterURL),
			}
			err = chainlinkNodes[0].CreateBridge(&bta)
			Expect(err).ShouldNot(HaveOccurred(), "Creating bridge shouldn't fail")

			os := &client.DirectRequestTxPipelineSpec{
				BridgeTypeAttributes: bta,
				DataPath:             "data,result",
			}
			ost, err := os.String()
			Expect(err).ShouldNot(HaveOccurred(), "Building observation source spec shouldn't fail")

			_, err = chainlinkNodes[0].CreateJob(&client.DirectRequestJobSpec{
				Name:                     "direct_request",
				MinIncomingConfirmations: minIncomingConfirmations,
				ContractAddress:          oracle.Address(),
				ExternalJobID:            jobUUID.String(),
				ObservationSource:        ost,
			})
			Expect(err).ShouldNot(HaveOccurred(), "Creating direct_request job shouldn't fail")
		})

		By("creating reorg for 10 blocks", func() {
			rc, err := chaos.NewReorgController(
				&chaos.ReorgConfig{
					FromPodLabel:            reorg.TXNodesAppLabel,
					ToPodLabel:              reorg.MinerNodesAppLabel,
					Network:                 nets.Default,
					Env:                     e,
					BlockConsensusThreshold: 3,
					Timeout:                 1800 * time.Second,
				},
			)
			Expect(err).ShouldNot(HaveOccurred())
			rc.ReOrg(reorgBlocks)
			rc.WaitReorgStarted()

			jobUUIDReplaces := strings.Replace(jobUUID.String(), "-", "", 4)
			var jobID [32]byte
			copy(jobID[:], jobUUIDReplaces)
			err = consumer.CreateRequestTo(
				oracle.Address(),
				jobID,
				big.NewInt(1e18),
				fmt.Sprintf("%s/variable", ms.Config.ClusterURL),
				"data,result",
				big.NewInt(100),
			)
			Expect(err).ShouldNot(HaveOccurred(), "Calling oracle contract shouldn't fail")

			err = rc.WaitDepthReached()
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Describe("with DirectRequest job", func() {
		It("receives API call data on-chain", func() {
			Eventually(func(g Gomega) {
				d, err := consumer.Data(context.Background())
				g.Expect(err).ShouldNot(HaveOccurred(), "Getting data from consumer contract shouldn't fail")
				g.Expect(d).ShouldNot(BeNil(), "Expected the initial on chain data to be nil")
				log.Debug().Int64("Data", d.Int64()).Msg("Found on chain")
				g.Expect(d.Int64()).Should(BeNumerically("==", 5), "Expected the on-chain data to be 5, but found %d", d.Int64())
			}, timeout, interval).Should(Succeed())

			testFilename := strings.Split(ginkgo.CurrentSpecReport().FileName(), ".")[0]
			_, testName := filepath.Split(testFilename)
			logsPath := filepath.Join(config.ProjectConfigDirectory, "logs", fmt.Sprintf("%s-%d", testName, time.Now().Unix()))
			err = e.Artifacts.DumpTestResult(logsPath, "chainlink")
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	AfterEach(func() {
		By("Tearing down the environment", func() {
			nets.Default.GasStats().PrintStats()
			err = actions.TeardownSuite(e, nets, utils.ProjectRoot, chainlinkNodes, nil)
			Expect(err).ShouldNot(HaveOccurred(), "Environment teardown shouldn't fail")
		})
	})
})
