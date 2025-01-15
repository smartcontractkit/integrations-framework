package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/block-vision/sui-go-sdk/models"
	"github.com/docker/docker/api/types/container"
	"github.com/go-resty/resty/v2"
	"github.com/smartcontractkit/chainlink-testing-framework/framework"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"path/filepath"
	"strings"
	"time"
)

type SuiKeyInfo struct {
	Alias           *string `json:"alias"`           // Alias key name, usually "null"
	Flag            int     `json:"flag"`            // Flag is an integer
	KeyScheme       string  `json:"keyScheme"`       // Key scheme is a string
	Mnemonic        string  `json:"mnemonic"`        // Mnemonic is a string
	PeerId          string  `json:"peerId"`          // Peer ID is a string
	PublicBase64Key string  `json:"publicBase64Key"` // Public key in Base64 format
	SuiAddress      string  `json:"suiAddress"`      // Sui address is a 0x prefixed hex string
}

func fundAccount(url string, address string) error {
	r := resty.New().SetBaseURL(url).EnableTrace().SetDebug(true)
	b := &models.FaucetRequest{
		FixedAmountRequest: &models.FaucetFixedAmountRequest{
			Recipient: address,
		},
	}
	resp, err := r.R().SetBody(b).SetHeader("Content-Type", "application/json").Post("/gas")
	if err != nil {
		return err
	}
	framework.L.Info().Any("Resp", resp).Msg("Address is funded!")
	return nil
}

func generateKeyData(containerName string, keyCipherType string) (*SuiKeyInfo, error) {
	cmdStr := []string{"sui", "keytool", "generate", keyCipherType, "--json"}
	keyOut, err := framework.ExecContainer(containerName, cmdStr)
	if err != nil {
		return nil, err
	}
	cleanKey := strings.ReplaceAll(keyOut, "\x00", "")
	cleanKey = strings.ReplaceAll(cleanKey, "\x01", "")
	cleanKey = strings.ReplaceAll(cleanKey, "\x02", "")
	cleanKey = strings.ReplaceAll(cleanKey, "\n", "")
	var key *SuiKeyInfo
	if err := json.Unmarshal([]byte("{"+cleanKey[2:]), &key); err != nil {
		return nil, err
	}
	framework.L.Info().Interface("Key", key).Msg("Test key")
	return key, nil
}

func defaultSui(in *Input) {
	if in.Image == "" {
		in.Image = "mysten/sui-tools:devnet"
	}
	if in.Port != "" {
		framework.L.Warn().Msg("'port' field is set but only default port can be used: 9000")
	}
	in.Port = "9000"
}

func newSui(in *Input) (*Output, error) {
	defaultSui(in)
	ctx := context.Background()
	containerName := framework.DefaultTCName("blockchain-node")

	absPath, err := filepath.Abs(in.ContractsDir)
	if err != nil {
		return nil, err
	}

	bindPort := fmt.Sprintf("%s/tcp", in.Port)

	req := testcontainers.ContainerRequest{
		Image:        in.Image,
		ExposedPorts: []string{in.Port, "9123/tcp"},
		Name:         containerName,
		Labels:       framework.DefaultTCLabels(),
		Networks:     []string{framework.DefaultNetworkName},
		NetworkAliases: map[string][]string{
			framework.DefaultNetworkName: {containerName},
		},
		HostConfigModifier: func(h *container.HostConfig) {
			h.PortBindings = framework.MapTheSamePort(bindPort, "9123/tcp")
		},
		ImagePlatform: "linux/amd64",
		Env: map[string]string{
			"RUST_LOG": "off,sui_node=info",
		},
		Cmd: []string{
			"sui",
			"start",
			"--force-regenesis",
			"--with-faucet",
		},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      absPath,
				ContainerFilePath: "/",
			},
		},
		// we need faucet for funding
		WaitingFor: wait.ForListeningPort("9123/tcp").WithStartupTimeout(10 * time.Second).WithPollInterval(200 * time.Millisecond),
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}
	host, err := c.Host(ctx)
	if err != nil {
		return nil, err
	}
	keyData, err := generateKeyData(containerName, "ed25519")
	if err != nil {
		return nil, err
	}
	err = fundAccount(fmt.Sprintf("http://%s:%s", "127.0.0.1", "9123"), keyData.SuiAddress)
	if err != nil {
		return nil, err
	}
	return &Output{
		UseCache:      true,
		Family:        "sui",
		ContainerName: containerName,
		GeneratedData: &GeneratedData{Mnemonic: keyData.Mnemonic},
		Nodes: []*Node{
			{
				HostHTTPUrl:           fmt.Sprintf("http://%s:%s", host, in.Port),
				DockerInternalHTTPUrl: fmt.Sprintf("http://%s:%s", containerName, in.Port),
			},
		},
	}, nil
}
