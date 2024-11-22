package simple_node_set

import (
	"fmt"
	"time"

	"github.com/smartcontractkit/chainlink-testing-framework/framework/chaos"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/blockchain"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/postgres"
)

// UpgradeNodeSet updates nodes configuration TOML files
// this API is discouraged, however, you can use it if nodes require restart or configuration updates, temporarily!
func UpgradeNodeSet(in *Input, bc *blockchain.Output, wait time.Duration) (*Output, error) {
	clNodeContainerNameRegex := fmt.Sprintf("%s.*", CLNodeContainerLabel)
	clNodeDBContainerNameRegex := fmt.Sprintf("%s-%s.*", postgres.DBContainerLabel, CLNodeContainerLabel)
	_, err := chaos.ExecPumba(fmt.Sprintf("rm --volumes=false re2:%s|%s", clNodeContainerNameRegex, clNodeDBContainerNameRegex), wait)
	if err != nil {
		return nil, err
	}
	in.Out = nil
	out, err := NewSharedDBNodeSet(in, bc)
	in.Out = out
	return out, err
}
