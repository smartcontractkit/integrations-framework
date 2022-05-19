package soak_runner

//revive:disable:dot-imports
import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-testing-framework/actions"
	"github.com/smartcontractkit/chainlink-testing-framework/config"
	"github.com/smartcontractkit/chainlink-testing-framework/utils"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/stretchr/testify/require"
)

// Builds the go tests to run, and returns a path to it, along with remote config options
func buildGoTests(t *testing.T) string {
	exePath := filepath.Join(utils.ProjectRoot, "remote.test")
	soakTestsPath := filepath.Join(utils.SoakRoot, "tests")
	compileCmd := exec.Command("go", "test", "-c", soakTestsPath, "-o", exePath) // #nosec G204
	compileCmd.Env = os.Environ()
	compileCmd.Env = append(compileCmd.Env, "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")

	log.Info().Str("Test Directory", soakTestsPath).Msg("Compiling tests")
	compileOut, err := compileCmd.Output()
	log.Debug().
		Str("Output", string(compileOut)).
		Str("Command", compileCmd.String()).
		Msg("Ran command")
	require.NoError(t, err, fmt.Sprintf("Env: %s\nCommand: %s\nCommand Output: %s", compileCmd.Env, compileCmd.String(), compileOut))

	_, err = os.Stat(exePath)
	require.NoError(t, err, fmt.Sprintf("Expected '%s' to exist", exePath))
	return exePath
}

// runs a soak test based on the tag, launching as many chainlink nodes as necessary
func runSoakTest(t *testing.T, testTag, namespacePrefix string, chainlinkReplicas int) {
	actions.LoadConfigs()
	exePath := buildGoTests(t)

	env, err := environment.DeployRemoteRunnerEnvironment(
		environment.NewChainlinkConfig(
			environment.ChainlinkReplicas(chainlinkReplicas, config.ChainlinkVals()),
			namespacePrefix,
			config.GethNetworks()...,
		),
		testTag, // Name of the test to run
		config.ProjectConfig.RemoteRunnerConfig.SlackAPIKey,  // API key to use to upload artifacts to slack
		config.ProjectConfig.RemoteRunnerConfig.SlackChannel, // Slack Channel to upload test artifacts to
		config.ProjectConfig.RemoteRunnerConfig.SlackUserID,  // Slack user to notify on completion
		filepath.Join(utils.SuiteRoot, "framework.yaml"),     // Path of the framework config
		filepath.Join(utils.SuiteRoot, "networks.yaml"),      // Path to the networks config
		exePath, // Path to the executable test file
	)
	require.NoError(t, err, "Error launching soak test environment")
	log.Info().Str("Namespace", env.Namespace).
		Str("Environment File", fmt.Sprintf("%s.%s", env.Namespace, "yaml")).
		Msg("Soak Test Successfully Launched. Save the environment file to collect logs when test is done.")
}
