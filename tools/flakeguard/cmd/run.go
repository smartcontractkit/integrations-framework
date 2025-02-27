package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-testing-framework/tools/flakeguard/reports"
	"github.com/smartcontractkit/chainlink-testing-framework/tools/flakeguard/runner"
	"github.com/spf13/cobra"
)

const (
	// FlakyTestsExitCode indicates that Flakeguard ran correctly and was able to identify flaky tests
	FlakyTestsExitCode = 1
	// ErrorExitCode indicates that Flakeguard ran into an error and was not able to complete operation
	ErrorExitCode = 2
)

var RunTestsCmd = &cobra.Command{
	Use:   "run",
	Short: "Run tests to check if they are flaky",
	Run: func(cmd *cobra.Command, args []string) {
		// Retrieve flags
		projectPath, _ := cmd.Flags().GetString("project-path")
		testPackagesJson, _ := cmd.Flags().GetString("test-packages-json")
		testPackagesArg, _ := cmd.Flags().GetStringSlice("test-packages")
		testCmdStrings, _ := cmd.Flags().GetStringArray("test-cmd")
		runCount, _ := cmd.Flags().GetInt("run-count")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		tags, _ := cmd.Flags().GetStringArray("tags")
		useRace, _ := cmd.Flags().GetBool("race")
		outputPath, _ := cmd.Flags().GetString("output-json")
		maxPassRatio, _ := cmd.Flags().GetFloat64("max-pass-ratio")
		skipTests, _ := cmd.Flags().GetStringSlice("skip-tests")
		selectTests, _ := cmd.Flags().GetStringSlice("select-tests")
		useShuffle, _ := cmd.Flags().GetBool("shuffle")
		shuffleSeed, _ := cmd.Flags().GetString("shuffle-seed")
		omitOutputsOnSuccess, _ := cmd.Flags().GetBool("omit-test-outputs-on-success")
		ignoreParentFailuresOnSubtests, _ := cmd.Flags().GetBool("ignore-parent-failures-on-subtests")

		outputDir := filepath.Dir(outputPath)
		initialDirSize, err := getDirSize(outputDir)
		if err != nil {
			log.Error().Err(err).Str("path", outputDir).Msg("Error getting initial directory size")
			// intentionally don't exit here, as we can still proceed with the run
		}

		if maxPassRatio < 0 || maxPassRatio > 1 {
			log.Error().Float64("max pass ratio", maxPassRatio).Msg("Error: max pass ratio must be between 0 and 1")
			os.Exit(ErrorExitCode)
		}

		// Check if project dependencies are correctly set up
		if err := checkDependencies(projectPath); err != nil {
			log.Error().Err(err).Msg("Error checking project dependencies")
			os.Exit(ErrorExitCode)
		}

		// Determine test packages
		var testPackages []string
		if len(testCmdStrings) == 0 {
			if testPackagesJson != "" {
				if err := json.Unmarshal([]byte(testPackagesJson), &testPackages); err != nil {
					log.Error().Err(err).Msg("Error decoding test packages JSON")
					os.Exit(ErrorExitCode)
				}
			} else if len(testPackagesArg) > 0 {
				testPackages = testPackagesArg
			} else {
				log.Error().Msg("Error: must specify either --test-packages-json or --test-packages")
				os.Exit(ErrorExitCode)
			}
		}

		// Initialize the runner
		testRunner := runner.Runner{
			ProjectPath:                    projectPath,
			Verbose:                        true,
			RunCount:                       runCount,
			Timeout:                        timeout,
			Tags:                           tags,
			UseRace:                        useRace,
			SkipTests:                      skipTests,
			SelectTests:                    selectTests,
			UseShuffle:                     useShuffle,
			ShuffleSeed:                    shuffleSeed,
			OmitOutputsOnSuccess:           omitOutputsOnSuccess,
			MaxPassRatio:                   maxPassRatio,
			IgnoreParentFailuresOnSubtests: ignoreParentFailuresOnSubtests,
		}

		// Run the tests
		var (
			testReport *reports.TestReport
		)

		if len(testCmdStrings) > 0 {
			testReport, err = testRunner.RunTestCmd(testCmdStrings)
			if err != nil {
				log.Fatal().Err(err).Msg("Error running custom test command")
				os.Exit(ErrorExitCode)
			}
		} else {
			testReport, err = testRunner.RunTestPackages(testPackages)
			if err != nil {
				log.Fatal().Err(err).Msg("Error running test packages")
				os.Exit(ErrorExitCode)
			}
		}

		// Save the test results in JSON format
		if outputPath != "" && len(testReport.Results) > 0 {
			jsonData, err := json.MarshalIndent(testReport, "", "  ")
			if err != nil {
				log.Error().Err(err).Msg("Error marshaling test results to JSON")
				os.Exit(ErrorExitCode)
			}
			if err := os.WriteFile(outputPath, jsonData, 0600); err != nil {
				log.Error().Err(err).Msg("Error writing test results to file")
				os.Exit(ErrorExitCode)
			}
			log.Info().Str("path", outputPath).Msg("Test results saved")
		}

		if len(testReport.Results) == 0 {
			log.Warn().Msg("No tests were run for the specified packages")
			return
		}

		// Filter flaky tests using FilterTests
		flakyTests := reports.FilterTests(testReport.Results, func(tr reports.TestResult) bool {
			return !tr.Skipped && tr.PassRatio < maxPassRatio
		})

		finalDirSize, err := getDirSize(outputDir)
		if err != nil {
			log.Error().Err(err).Str("path", outputDir).Msg("Error getting initial directory size")
			// intentionally don't exit here, as we can still proceed with the run
		}
		diskSpaceUsed := byteCountSI(finalDirSize - initialDirSize)

		if len(flakyTests) > 0 {
			log.Info().Str("disk space used", diskSpaceUsed).Int("count", len(flakyTests)).Str("pass ratio threshold", fmt.Sprintf("%.2f%%", maxPassRatio*100)).Msg("Found flaky tests")
		} else {
			log.Info().Str("disk space used", diskSpaceUsed).Msg("No flaky tests found")
		}

		fmt.Printf("\nFlakeguard Summary\n")
		reports.RenderResults(os.Stdout, testReport, false, false)

		if len(flakyTests) > 0 {
			// Exit with error code if there are flaky tests
			os.Exit(FlakyTestsExitCode)
		}
	},
}

func init() {
	RunTestsCmd.Flags().StringP("project-path", "r", ".", "The path to the Go project. Default is the current directory. Useful for subprojects")
	RunTestsCmd.Flags().String("test-packages-json", "", "JSON-encoded string of test packages")
	RunTestsCmd.Flags().StringSlice("test-packages", nil, "Comma-separated list of test packages to run")
	RunTestsCmd.Flags().StringArray("test-cmd", nil,
		"Optional custom test command (e.g. 'go test -json github.com/smartcontractkit/chainlink/integration-tests/smoke -v -run TestForwarderOCR2Basic'), which must produce go test -json output.",
	)
	RunTestsCmd.Flags().Bool("run-all-packages", false, "Run all test packages in the project. This flag overrides --test-packages and --test-packages-json")
	RunTestsCmd.Flags().IntP("run-count", "c", 1, "Number of times to run the tests")
	RunTestsCmd.Flags().Duration("timeout", 0, "Passed on to the 'go test' command as the -timeout flag")
	RunTestsCmd.Flags().StringArray("tags", nil, "Passed on to the 'go test' command as the -tags flag")
	RunTestsCmd.Flags().Bool("race", false, "Enable the race detector")
	RunTestsCmd.Flags().Bool("shuffle", false, "Enable test shuffling")
	RunTestsCmd.Flags().String("shuffle-seed", "", "Set seed for test shuffling. Must be used with --shuffle")
	RunTestsCmd.Flags().Bool("fail-fast", false, "Stop on the first test failure")
	RunTestsCmd.Flags().String("output-json", "", "Path to output the test results in JSON format")
	RunTestsCmd.Flags().StringSlice("skip-tests", nil, "Comma-separated list of test names to skip from running")
	RunTestsCmd.Flags().StringSlice("select-tests", nil, "Comma-separated list of test names to specifically run")
	RunTestsCmd.Flags().Float64("max-pass-ratio", 1.0, "The maximum pass ratio threshold for a test to be considered flaky. Any tests below this pass rate will be considered flaky.")
	RunTestsCmd.Flags().Bool("omit-test-outputs-on-success", true, "Omit test outputs and package outputs for tests that pass")
	RunTestsCmd.Flags().Bool("ignore-parent-failures-on-subtests", false, "Ignore failures in parent tests when only subtests fail")
}

func checkDependencies(projectPath string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = projectPath

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dependency check failed: %v\n%s\nPlease run 'go mod tidy' to fix missing or unused dependencies", err, out.String())
	}

	return nil
}
