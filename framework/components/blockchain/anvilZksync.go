package blockchain

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/testcontainers/testcontainers-go"

	"github.com/smartcontractkit/chainlink-testing-framework/framework"
)

func defaultAnvilZksync(in *Input) {
	if in.ChainID == "" {
		in.ChainID = "260"
	}
	if in.Port == "" {
		in.Port = "8011"
	}
}

const dockerFile = `FROM ubuntu:latest
RUN apt update
RUN apt install -y curl git
RUN curl -L https://raw.githubusercontent.com/matter-labs/foundry-zksync/main/install-foundry-zksync | bash`

// anvil-zskync has no public image builds. this method will build the image from source
// creating a dockerfile in a temporary directory with the necessary commands to install
// foundry-zksync.
// see: https://foundry-book.zksync.io/getting-started/installation#using-foundry-with-docker
func newAnvilZksync(in *Input) (*Output, error) {
	defaultAnvilZksync(in)
	req := baseRequest(in)

	tempDir, err := os.MkdirTemp(".", "anvil-zksync-dockercontext-")
	if err != nil {
		return nil, err
	}

	dockerfilePath := filepath.Join(tempDir, "anvilZksync.Dockerfile")

	if err := os.WriteFile(dockerfilePath, []byte(dockerFile), 0644); err != nil {
		return nil, err
	}
	req.FromDockerfile = testcontainers.FromDockerfile{
		Context:    tempDir,
		Dockerfile: "anvilZksync.Dockerfile",
		KeepImage:  true,
	}

	req.Entrypoint = []string{
		"/bin/sh",
		"-c",
		"/root/.foundry/bin/anvil-zksync --offline --chain-id " + in.ChainID + " --port " + in.Port + " run",
	}

	framework.L.Info().Any("Cmd", strings.Join(req.Entrypoint, " ")).Msg("Creating anvil with command")

	output, err := createGenericEvmContainer(in, req)
	if err != nil {
		return nil, err
	}

	if err := os.RemoveAll(tempDir); err != nil {
		return nil, err
	}

	return output, nil
}
