package runner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-testing-framework/tools/flakeguard/go-test-transform/pkg/transformer"
	"github.com/smartcontractkit/chainlink-testing-framework/tools/flakeguard/reports"
)

var (
	startPanicRe = regexp.MustCompile(`^panic:`)
	startRaceRe  = regexp.MustCompile(`^WARNING: DATA RACE`)
)

// Runner describes the test run parameters and raw test outputs
type Runner struct {
	ProjectPath                    string        // Path to the Go project directory.
	prettyProjectPath              string        // Go project package path, formatted for pretty printing.
	Verbose                        bool          // If true, provides detailed logging.
	RunCount                       int           // Number of times to run the tests.
	UseRace                        bool          // Enable race detector.
	Timeout                        time.Duration // Test timeout
	Tags                           []string      // Build tags.
	UseShuffle                     bool          // Enable test shuffling. -shuffle=on flag.
	ShuffleSeed                    string        // Set seed for test shuffling -shuffle={seed} flag. Must be used with UseShuffle.
	FailFast                       bool          // Stop on first test failure.
	SkipTests                      []string      // Test names to exclude.
	SelectTests                    []string      // Test names to include.
	CollectRawOutput               bool          // Set to true to collect test output for later inspection.
	OmitOutputsOnSuccess           bool          // Set to true to omit test outputs on success.
	MaxPassRatio                   float64       // Maximum pass ratio threshold for a test to be considered flaky.
	IgnoreParentFailuresOnSubtests bool          // Ignore failures in parent tests when only subtests fail.
	rawOutputs                     map[string]*bytes.Buffer
}

// RunTestPackages executes the tests for each provided package and aggregates all results.
// It returns all test results and any error encountered during testing.
func (r *Runner) RunTestPackages(packages []string) (*reports.TestReport, error) {
	var jsonFilePaths []string
	for _, p := range packages {
		for i := 0; i < r.RunCount; i++ {
			if r.CollectRawOutput { // Collect raw output for debugging
				if r.rawOutputs == nil {
					r.rawOutputs = make(map[string]*bytes.Buffer)
				}
				if _, exists := r.rawOutputs[p]; !exists {
					r.rawOutputs[p] = &bytes.Buffer{}
				}
				separator := strings.Repeat("-", 80)
				r.rawOutputs[p].WriteString(fmt.Sprintf("Run %d\n%s\n", i+1, separator))
			}
			jsonFilePath, passed, err := r.runTestPackage(p)
			if err != nil {
				return nil, fmt.Errorf("failed to run tests in package %s: %w", p, err)
			}
			jsonFilePaths = append(jsonFilePaths, jsonFilePath)
			if !passed && r.FailFast {
				break
			}
		}
	}

	results, err := r.parseTestResults(jsonFilePaths)
	if err != nil {
		return nil, fmt.Errorf("failed to parse test results: %w", err)
	}
	report := &reports.TestReport{
		GoProject:     r.prettyProjectPath,
		RaceDetection: r.UseRace,
		ExcludedTests: r.SkipTests,
		SelectedTests: r.SelectTests,
		Results:       results,
		MaxPassRatio:  r.MaxPassRatio,
	}
	report.GenerateSummaryData()

	return report, nil
}

// RunTestCmd runs an arbitrary command testCmd (like ["go", "run", "my_test.go", ...])
// that produces the same JSON lines that 'go test -json' would produce on stdout.
// It captures those lines in a temp file, then parses them for pass/fail/panic/race data.
func (r *Runner) RunTestCmd(testCmd []string) (*reports.TestReport, error) {
	var jsonFilePaths []string

	// Run the command r.RunCount times
	for i := 0; i < r.RunCount; i++ {
		jsonFilePath, passed, err := r.runCmd(testCmd, i)
		if err != nil {
			return nil, fmt.Errorf("failed to run test command: %w", err)
		}
		jsonFilePaths = append(jsonFilePaths, jsonFilePath)
		if !passed && r.FailFast {
			break
		}
	}

	results, err := r.parseTestResults(jsonFilePaths)
	if err != nil {
		return nil, fmt.Errorf("failed to parse test results: %w", err)
	}

	report := &reports.TestReport{
		GoProject:     r.prettyProjectPath,
		RaceDetection: r.UseRace,
		ExcludedTests: r.SkipTests,
		SelectedTests: r.SelectTests,
		Results:       results,
		MaxPassRatio:  r.MaxPassRatio,
	}
	report.GenerateSummaryData()

	return report, nil
}

// RawOutputs retrieves the raw output from the test runs, if CollectRawOutput enabled.
// packageName : raw output
func (r *Runner) RawOutputs() map[string]*bytes.Buffer {
	return r.rawOutputs
}

type exitCoder interface {
	ExitCode() int
}

// runTestPackage runs the tests for a given package and returns the path to the output file.
func (r *Runner) runTestPackage(packageName string) (string, bool, error) {
	args := []string{"test", packageName, "-json", "-count=1"}
	if r.UseRace {
		args = append(args, "-race")
	}
	if r.Timeout > 0 {
		args = append(args, fmt.Sprintf("-timeout=%s", r.Timeout.String()))
	}
	if len(r.Tags) > 0 {
		args = append(args, fmt.Sprintf("-tags=%s", strings.Join(r.Tags, ",")))
	}
	if r.UseShuffle {
		if r.ShuffleSeed != "" {
			args = append(args, fmt.Sprintf("-shuffle=%s", r.ShuffleSeed))
		} else {
			args = append(args, "-shuffle=on")
		}
	}
	if len(r.SkipTests) > 0 {
		skipPattern := strings.Join(r.SkipTests, "|")
		args = append(args, fmt.Sprintf("-skip=%s", skipPattern))
	}
	if len(r.SelectTests) > 0 {
		selectPattern := strings.Join(r.SelectTests, "$|^")
		args = append(args, fmt.Sprintf("-run=^%s$", selectPattern))
	}

	if r.Verbose {
		log.Info().Str("command", fmt.Sprintf("go %s\n", strings.Join(args, " "))).Msg("Running command")
	}

	// Create a temporary file to store the output
	tmpFile, err := os.CreateTemp("", "test-output-*.json")
	if err != nil {
		return "", false, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	r.prettyProjectPath, err = prettyProjectPath(r.ProjectPath)
	if err != nil {
		r.prettyProjectPath = r.ProjectPath
		log.Warn().Err(err).Str("projectPath", r.ProjectPath).Msg("Failed to get pretty project path")
	}
	// Run the command with output directed to the file
	cmd := exec.Command("go", args...)
	cmd.Dir = r.ProjectPath
	if r.CollectRawOutput {
		cmd.Stdout = io.MultiWriter(tmpFile, r.rawOutputs[packageName])
	} else {
		cmd.Stdout = tmpFile
	}

	err = cmd.Run()
	if err != nil {
		var exErr exitCoder
		// Check if the error is due to a non-zero exit code
		if errors.As(err, &exErr) && exErr.ExitCode() == 0 {
			return "", false, fmt.Errorf("test command failed at %s: %w", packageName, err)
		}
		return tmpFile.Name(), false, nil // Test failed
	}

	return tmpFile.Name(), true, nil // Test succeeded
}

// runCmd runs the user-supplied command once, captures its JSON output,
// and returns the temp file path, whether the test passed, and an error if any.
func (r *Runner) runCmd(testCmd []string, runIndex int) (tempFilePath string, passed bool, err error) {
	// Create temp file for JSON output
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("test-output-cmd-run%d-*.json", runIndex+1))
	if err != nil {
		err = fmt.Errorf("failed to create temp file: %w", err)
		return
	}
	defer tmpFile.Close()

	if r.Verbose {
		log.Info().Msgf("Running custom test command (%d/%d): %s", runIndex+1, r.RunCount, strings.Join(testCmd, " "))
	}

	cmd := exec.Command(testCmd[0], testCmd[1:]...)
	cmd.Dir = r.ProjectPath

	// If collecting raw output, write to both file & buffer
	if r.CollectRawOutput {
		if r.rawOutputs == nil {
			r.rawOutputs = make(map[string]*bytes.Buffer)
		}
		key := fmt.Sprintf("customCmd-run%d", runIndex+1)
		if _, exists := r.rawOutputs[key]; !exists {
			r.rawOutputs[key] = &bytes.Buffer{}
		}
		cmd.Stdout = io.MultiWriter(tmpFile, r.rawOutputs[key])
	} else {
		cmd.Stdout = tmpFile
	}
	cmd.Stderr = os.Stderr

	err = cmd.Run()

	tempFilePath = tmpFile.Name()

	// Determine pass/fail from exit code
	type exitCoder interface {
		ExitCode() int
	}
	var ec exitCoder
	if errors.As(err, &ec) {
		// Non-zero exit code => test failure
		passed = ec.ExitCode() == 0
		err = nil // Clear error since we handled it
		return
	} else if err != nil {
		// Some other error that doesn't implement ExitCode() => real error
		tempFilePath = ""
		err = fmt.Errorf("error running test command: %w", err)
		return
	}

	// Otherwise, test passed
	passed = true
	return
}

type entry struct {
	Action  string  `json:"Action"`
	Test    string  `json:"Test"`
	Package string  `json:"Package"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"` // Decimal value in seconds
}

func (e entry) String() string {
	return fmt.Sprintf("Action: %s, Test: %s, Package: %s, Output: %s, Elapsed: %f", e.Action, e.Test, e.Package, e.Output, e.Elapsed)
}

// parseTestResults reads the test output Go test json output files and returns processed TestResults.
//
// Go test results have a lot of edge cases and strange behavior, especially when running in parallel,
// and any panic throws the whole thing into disarray.
// If any test in packageA panics, all tests in packageA will stop running and never report their results.
// The test that panicked will report its panic in Go test output, but will often misattribute the panic to a different test.
// It will also sometimes mark the test with both a panic and a failure, double-counting the test run.
// It's possible to properly attribute panics to the test that caused them, but it's not possible to distinguish between
// panics and failures at that point.
// Subtests add more complexity, as panics in subtests are only reported in their parent's output,
// and cannot be accurately attributed to the subtest that caused them.
func (r *Runner) parseTestResults(filePaths []string) ([]reports.TestResult, error) {
	// If the option is enabled, transform each JSON output file before parsing.
	if r.IgnoreParentFailuresOnSubtests {
		transformedPaths, err := r.transformTestOutputFiles(filePaths)
		if err != nil {
			return nil, err
		}
		filePaths = transformedPaths
	}

	var (
		testDetails         = make(map[string]*reports.TestResult) // Holds run, pass counts, and other details for each test
		panickedPackages    = map[string]struct{}{}                // Packages with tests that panicked
		racePackages        = map[string]struct{}{}                // Packages with tests that raced
		packageLevelOutputs = map[string][]string{}                // Package-level outputs
		testsWithSubTests   = map[string][]string{}                // Parent tests that have subtests
		panicDetectionMode  = false
		raceDetectionMode   = false
		detectedEntries     = []entry{} // race or panic entries
		expectedRuns        = r.RunCount
	)

	runNumber := 0
	// Process each file
	for _, filePath := range filePaths {
		runNumber++
		runID := fmt.Sprintf("run%d", runNumber)
		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open test output file: %w", err)
		}

		scanner := bufio.NewScanner(file)
		var precedingLines []string // Store preceding lines for context
		var followingLines []string // To collect lines after an error

		for scanner.Scan() {
			line := scanner.Text()
			precedingLines = append(precedingLines, line)

			// Limit precedingLines to the last 15 lines
			if len(precedingLines) > 15 {
				precedingLines = precedingLines[1:]
			}

			var entryLine entry
			if err := json.Unmarshal(scanner.Bytes(), &entryLine); err != nil {
				// Collect 15 lines after the error for more context
				for scanner.Scan() && len(followingLines) < 15 {
					followingLines = append(followingLines, scanner.Text())
				}

				// Combine precedingLines and followingLines to provide 15 lines before and after
				context := append(precedingLines, followingLines...)
				return nil, fmt.Errorf("failed to parse json test output near lines:\n%s\nerror: %w", strings.Join(context, "\n"), err)
			}

			var result *reports.TestResult
			if entryLine.Test != "" {
				// If it's a subtest, associate it with its parent for easier processing of panics later
				key := fmt.Sprintf("%s/%s", entryLine.Package, entryLine.Test)
				parentTestName, subTestName := parseSubTest(entryLine.Test)
				if subTestName != "" {
					parentTestKey := fmt.Sprintf("%s/%s", entryLine.Package, parentTestName)
					testsWithSubTests[parentTestKey] = append(testsWithSubTests[parentTestKey], subTestName)
				}

				if _, exists := testDetails[key]; !exists {
					testDetails[key] = &reports.TestResult{
						TestName:       entryLine.Test,
						TestPackage:    entryLine.Package,
						PassRatio:      0,
						PassedOutputs:  make(map[string][]string),
						FailedOutputs:  make(map[string][]string),
						PackageOutputs: []string{},
					}
				}
				result = testDetails[key]
			}

			if entryLine.Output != "" {
				if panicDetectionMode || raceDetectionMode { // currently collecting panic or race output
					detectedEntries = append(detectedEntries, entryLine)
					continue
				} else if startPanicRe.MatchString(entryLine.Output) { // found a panic, start collecting output
					panickedPackages[entryLine.Package] = struct{}{}
					detectedEntries = append(detectedEntries, entryLine)
					panicDetectionMode = true
					continue // Don't process this entry further
				} else if startRaceRe.MatchString(entryLine.Output) {
					racePackages[entryLine.Package] = struct{}{}
					detectedEntries = append(detectedEntries, entryLine)
					raceDetectionMode = true
					continue // Don't process this entry further
				} else if entryLine.Test != "" && entryLine.Action == "output" {
					// Collect outputs regardless of pass or fail
					if result.Outputs == nil {
						result.Outputs = make(map[string][]string)
					}
					result.Outputs[runID] = append(result.Outputs[runID], entryLine.Output)
				} else if entryLine.Test == "" {
					if _, exists := packageLevelOutputs[entryLine.Package]; !exists {
						packageLevelOutputs[entryLine.Package] = []string{}
					}
					packageLevelOutputs[entryLine.Package] = append(packageLevelOutputs[entryLine.Package], entryLine.Output)
				} else {
					// Collect outputs per run, per test action
					switch entryLine.Action {
					case "pass":
						result.PassedOutputs[runID] = append(result.PassedOutputs[runID], entryLine.Output)
					case "fail":
						result.FailedOutputs[runID] = append(result.FailedOutputs[runID], entryLine.Output)
					default:
						// Handle other actions if necessary
					}
				}
			}

			if (panicDetectionMode || raceDetectionMode) && entryLine.Action == "fail" { // End of panic or race output
				if panicDetectionMode {
					panicTest, timeout, err := attributePanicToTest(entryLine.Package, detectedEntries)
					if err != nil {
						return nil, err
					}
					panicTestKey := fmt.Sprintf("%s/%s", entryLine.Package, panicTest)

					// Ensure the test exists in testDetails
					result, exists := testDetails[panicTestKey]
					if !exists {
						// Create a new TestResult if it doesn't exist
						result = &reports.TestResult{
							TestName:       panicTest,
							TestPackage:    entryLine.Package,
							PassRatio:      0,
							PassedOutputs:  make(map[string][]string),
							FailedOutputs:  make(map[string][]string),
							PackageOutputs: []string{},
						}
						testDetails[panicTestKey] = result
					}

					result.Panic = true
					result.Timeout = timeout
					result.Failures++
					result.Runs++

					// Handle outputs
					for _, entry := range detectedEntries {
						if entry.Test == "" {
							result.PackageOutputs = append(result.PackageOutputs, entry.Output)
						} else {
							runID := fmt.Sprintf("run%d", runNumber)
							result.FailedOutputs[runID] = append(result.FailedOutputs[runID], entry.Output)
						}
					}
				} else if raceDetectionMode {
					raceTest, err := attributeRaceToTest(entryLine.Package, detectedEntries)
					if err != nil {
						return nil, err
					}
					raceTestKey := fmt.Sprintf("%s/%s", entryLine.Package, raceTest)

					// Ensure the test exists in testDetails
					result, exists := testDetails[raceTestKey]
					if !exists {
						// Create a new TestResult if it doesn't exist
						result = &reports.TestResult{
							TestName:       raceTest,
							TestPackage:    entryLine.Package,
							PassRatio:      0,
							PassedOutputs:  make(map[string][]string),
							FailedOutputs:  make(map[string][]string),
							PackageOutputs: []string{},
						}
						testDetails[raceTestKey] = result
					}

					result.Race = true
					result.Failures++
					result.Runs++

					// Handle outputs
					for _, entry := range detectedEntries {
						if entry.Test == "" {
							result.PackageOutputs = append(result.PackageOutputs, entry.Output)
						} else {
							runID := fmt.Sprintf("run%d", runNumber)
							result.FailedOutputs[runID] = append(result.FailedOutputs[runID], entry.Output)
						}
					}
				}

				detectedEntries = []entry{}
				panicDetectionMode = false
				raceDetectionMode = false
				continue
			}

			switch entryLine.Action {
			case "pass":
				if entryLine.Test != "" {
					duration, err := time.ParseDuration(strconv.FormatFloat(entryLine.Elapsed, 'f', -1, 64) + "s")
					if err != nil {
						return nil, fmt.Errorf("failed to parse duration: %w", err)
					}
					result.Durations = append(result.Durations, duration)
					result.Successes++

					// Move outputs to PassedOutputs
					if result.PassedOutputs == nil {
						result.PassedOutputs = make(map[string][]string)
					}
					result.PassedOutputs[runID] = result.Outputs[runID]
					// Clear temporary outputs
					delete(result.Outputs, runID)
				}
			case "fail":
				if entryLine.Test != "" {
					duration, err := time.ParseDuration(strconv.FormatFloat(entryLine.Elapsed, 'f', -1, 64) + "s")
					if err != nil {
						return nil, fmt.Errorf("failed to parse duration: %w", err)
					}
					result.Durations = append(result.Durations, duration)
					result.Failures++

					// Move outputs to FailedOutputs
					if result.FailedOutputs == nil {
						result.FailedOutputs = make(map[string][]string)
					}
					result.FailedOutputs[runID] = result.Outputs[runID]
					// Clear temporary outputs
					delete(result.Outputs, runID)
				}
			case "skip":
				if entryLine.Test != "" {
					result.Skipped = true
					result.Skips++
				}
			case "output":
				// Handled above when entryLine.Test is not empty
			}
			if entryLine.Test != "" {
				result.Runs = result.Successes + result.Failures
				if result.Runs > 0 {
					result.PassRatio = float64(result.Successes) / float64(result.Runs)
				} else {
					result.PassRatio = 1
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading test output file: %w", err)
		}
		// Clean up file after parsing
		if err = file.Close(); err != nil {
			log.Warn().Err(err).Str("file", filePath).Msg("failed to close file")
		}
		if err = os.Remove(filePath); err != nil {
			log.Warn().Err(err).Str("file", filePath).Msg("failed to delete file")
		}
	}

	var results []reports.TestResult
	// Check through parent tests for panics, and bubble possible panics down
	for parentTestKey, subTests := range testsWithSubTests {
		if parentTestResult, exists := testDetails[parentTestKey]; exists {
			if parentTestResult.Panic {
				for _, subTest := range subTests {
					// Include parent test name in subTestKey
					subTestKey := fmt.Sprintf("%s/%s/%s", parentTestResult.TestPackage, parentTestResult.TestName, subTest)
					if subTestResult, exists := testDetails[subTestKey]; exists {
						if subTestResult.Failures > 0 {
							subTestResult.Panic = true
							// Initialize Outputs map if nil
							if subTestResult.FailedOutputs == nil {
								subTestResult.FailedOutputs = make(map[string][]string)
							}
							// Add the message to each run's output
							for runID := range subTestResult.FailedOutputs {
								subTestResult.FailedOutputs[runID] = append(subTestResult.FailedOutputs[runID], "Panic in parent test")
							}
						}
					} else {
						log.Warn().Str("expected subtest", subTestKey).Str("parent test", parentTestKey).Msg("expected subtest not found in parent test")
					}
				}
			}
		} else {
			log.Warn().Str("parent test", parentTestKey).Msg("expected parent test not found")
		}
	}
	for _, result := range testDetails {
		if result.Runs > expectedRuns { // Panics can introduce double-counting test failures, this is a correction for it
			if result.Panic {
				result.Failures = expectedRuns
				result.Runs = expectedRuns
			} else {
				log.Warn().Str("test", result.TestName).Int("actual runs", result.Runs).Int("expected runs", expectedRuns).Msg("unexpected test runs")
			}
		}
		// If a package panicked, all tests in that package will be marked as panicking
		if _, panicked := panickedPackages[result.TestPackage]; panicked {
			result.PackagePanic = true
		}
		if outputs, exists := packageLevelOutputs[result.TestPackage]; exists {
			result.PackageOutputs = outputs
		}
		results = append(results, *result)
	}

	// Omit success outputs if requested
	if r.OmitOutputsOnSuccess {
		for i := range results {
			results[i].PassedOutputs = make(map[string][]string)
			results[i].Outputs = make(map[string][]string)
		}
	}

	return results, nil
}

// transformTestOutputFiles transforms the test output JSON files to ignore parent failures when only subtests fail.
// It returns the paths to the transformed files.
func (r *Runner) transformTestOutputFiles(filePaths []string) ([]string, error) {
	transformedPaths := make([]string, len(filePaths))
	for i, origPath := range filePaths {
		inFile, err := os.Open(origPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open original file %s: %w", origPath, err)
		}
		// Create a temporary file for the transformed output.
		outFile, err := os.CreateTemp("", "transformed-output-*.json")
		if err != nil {
			inFile.Close()
			return nil, fmt.Errorf("failed to create transformed temp file: %w", err)
		}
		// Transform the JSON output.
		// The transformer option is set to ignore parent failures when only subtests fail.
		err = transformer.TransformJSON(inFile, outFile, transformer.NewOptions(true))
		inFile.Close()
		outFile.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to transform output file %s: %v", origPath, err)
		}
		// Use the transformed file path.
		transformedPaths[i] = outFile.Name()
		os.Remove(origPath)
	}
	return transformedPaths, nil
}

// attributePanicToTest properly attributes panics to the test that caused them.
func attributePanicToTest(panicPackage string, panicEntries []entry) (test string, timeout bool, err error) {
	regexSanitizePanicPackage := filepath.Base(panicPackage)
	panicAttributionRe := regexp.MustCompile(fmt.Sprintf(`%s\.(Test[^\.\(]+)`, regexSanitizePanicPackage))
	timeoutAttributionRe := regexp.MustCompile(`(Test.*?)\W+\(.*\)`)
	entriesOutputs := []string{}
	for _, entry := range panicEntries {
		entriesOutputs = append(entriesOutputs, entry.Output)
		if matches := panicAttributionRe.FindStringSubmatch(entry.Output); len(matches) > 1 {
			testName := strings.TrimSpace(matches[1])
			return testName, false, nil
		}
		if matches := timeoutAttributionRe.FindStringSubmatch(entry.Output); len(matches) > 1 {
			testName := strings.TrimSpace(matches[1])
			return testName, true, nil
		}
	}
	return "", false, fmt.Errorf("failed to attribute panic to test, using regex %s on these strings:\n%s", panicAttributionRe.String(), strings.Join(entriesOutputs, ""))
}

// attributeRaceToTest properly attributes races to the test that caused them.
func attributeRaceToTest(racePackage string, raceEntries []entry) (string, error) {
	regexSanitizeRacePackage := filepath.Base(racePackage)
	raceAttributionRe := regexp.MustCompile(fmt.Sprintf(`%s\.(Test[^\.\(]+)`, regexSanitizeRacePackage))
	entriesOutputs := []string{}
	for _, entry := range raceEntries {
		entriesOutputs = append(entriesOutputs, entry.Output)
		if matches := raceAttributionRe.FindStringSubmatch(entry.Output); len(matches) > 1 {
			testName := strings.TrimSpace(matches[1])
			return testName, nil
		}
	}
	return "", fmt.Errorf("failed to attribute race to test, using regex: %s on these strings:\n%s", raceAttributionRe.String(), strings.Join(entriesOutputs, ""))
}

// parseSubTest checks if a test name is a subtest and returns the parent and sub names.
func parseSubTest(testName string) (parentTestName, subTestName string) {
	parts := strings.SplitN(testName, "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// prettyProjectPath returns the project path formatted for pretty printing in results.
func prettyProjectPath(projectPath string) (string, error) {
	// Walk up the directory structure to find go.mod
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	dir := absPath
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir { // Reached the root without finding go.mod
			return "", fmt.Errorf("go.mod not found in project path, started at %s, ended at %s", projectPath, dir)
		}
		dir = parent
	}

	// Read go.mod to extract the module path
	goModPath := filepath.Join(dir, "go.mod")
	goModData, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("failed to read go.mod: %w", err)
	}

	for _, line := range strings.Split(string(goModData), "\n") {
		if strings.HasPrefix(line, "module ") {
			goProject := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			relativePath := strings.TrimPrefix(projectPath, dir)
			relativePath = strings.TrimLeft(relativePath, string(os.PathSeparator))
			return filepath.Join(goProject, relativePath), nil
		}
	}

	return "", fmt.Errorf("module path not found in go.mod")
}
