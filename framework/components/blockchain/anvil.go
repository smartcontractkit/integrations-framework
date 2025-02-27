package blockchain

import (
	"fmt"
	"strings"

	"github.com/smartcontractkit/chainlink-testing-framework/framework"
)

const (
	DefaultAnvilPrivateKey = `ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80`
)

func defaultAnvil(in *Input) {
	if in.Image == "" {
		in.Image = "f4hrenh9it/foundry:latest"
	}
	if in.ChainID == "" {
		in.ChainID = "1337"
	}
	if in.Port == "" {
		in.Port = "8545"
	}
}

// newAnvil deploy foundry anvil node
func newAnvil(in *Input) (*Output, error) {
	defaultAnvil(in)
	req := baseRequest(in)

	req.Image = fmt.Sprintf("%s", in.Image)
	req.AlwaysPullImage = in.PullImage

	entryPoint := []string{"anvil"}
	defaultCmd := []string{"--host", "0.0.0.0", "--port", in.Port, "--chain-id", in.ChainID}
	entryPoint = append(entryPoint, defaultCmd...)
	entryPoint = append(entryPoint, in.DockerCmdParamsOverrides...)
	req.Entrypoint = entryPoint

	framework.L.Info().Any("Cmd", strings.Join(entryPoint, " ")).Msg("Creating anvil with command")

	return createGenericEvmContainer(in, req)
}
