package reports

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// TestReport reports on the parameters and results of one to many test runs
type TestReport struct {
	ID        string `json:"id,omitempty"`
	GoProject string `json:"go_project"`
	// Current branch name
	BranchName           string       `json:"branch_name,omitempty"`
	DefaultBranchName    string       `json:"default_branch_name,omitempty"`
	BaseBranchName       string       `json:"base_branch_name,omitempty"`
	HeadSHA              string       `json:"head_sha,omitempty"`
	BaseSHA              string       `json:"base_sha,omitempty"`
	RepoURL              string       `json:"repo_url,omitempty"`
	GitHubWorkflowName   string       `json:"github_workflow_name,omitempty"`
	GitHubWorkflowRunURL string       `json:"github_workflow_run_url,omitempty"`
	SummaryData          *SummaryData `json:"summary_data"`
	RaceDetection        bool         `json:"race_detection"`
	ExcludedTests        []string     `json:"excluded_tests,omitempty"`
	SelectedTests        []string     `json:"selected_tests,omitempty"`
	Results              []TestResult `json:"results,omitempty"`
	// MaxPassRatio is the maximum flakiness ratio allowed for a test to be considered not flaky
	MaxPassRatio float64 `json:"max_pass_ratio,omitempty"`
}

// GenerateSummaryData generates a summary of a report's test results
func (testReport *TestReport) GenerateSummaryData() {
	var runs, testRunCount, passes, fails, skips, panickedTests, racedTests, flakyTests int

	// Map to hold unique test names that were entirely skipped
	uniqueSkippedTestsMap := make(map[string]struct{})

	for _, result := range testReport.Results {
		runs += result.Runs
		if result.Runs > testRunCount {
			testRunCount = result.Runs
		}
		passes += result.Successes
		fails += result.Failures
		skips += result.Skips

		if result.Runs == 0 && result.Skipped {
			uniqueSkippedTestsMap[result.TestName] = struct{}{}
		}

		if result.Panic {
			panickedTests++
			flakyTests++
		} else if result.Race {
			racedTests++
			flakyTests++
		} else if !result.Skipped && result.Runs > 0 && result.PassRatio < testReport.MaxPassRatio {
			flakyTests++
		}
	}

	// Calculate the unique count of skipped tests
	uniqueSkippedTestCount := len(uniqueSkippedTestsMap)

	// Calculate the raw pass ratio
	passRatio := passRatio(passes, runs)

	// Calculate the raw flake ratio
	totalTests := len(testReport.Results)
	flakeRatio := flakeRatio(flakyTests, totalTests)

	passRatioStr := formatRatio(passRatio)
	flakeTestRatioStr := formatRatio(flakeRatio)

	testReport.SummaryData = &SummaryData{
		UniqueTestsRun:         totalTests,
		UniqueSkippedTestCount: uniqueSkippedTestCount,
		TestRunCount:           testRunCount,
		PanickedTests:          panickedTests,
		RacedTests:             racedTests,
		FlakyTests:             flakyTests,
		FlakyTestPercent:       flakeTestRatioStr,

		TotalRuns:   runs,
		PassedRuns:  passes,
		FailedRuns:  fails,
		SkippedRuns: skips,
		PassPercent: passRatioStr,
	}
}

// TestResult contains the results and outputs of a single test
type TestResult struct {
	// ReportID is the ID of the report this test result belongs to
	// used mostly for Splunk logging
	ReportID       string              `json:"report_id"`
	TestName       string              `json:"test_name"`
	TestPackage    string              `json:"test_package"`
	PackagePanic   bool                `json:"package_panic"`
	Panic          bool                `json:"panic"`
	Timeout        bool                `json:"timeout"`
	Race           bool                `json:"race"`
	Skipped        bool                `json:"skipped"`
	PassRatio      float64             `json:"pass_ratio"`
	Runs           int                 `json:"runs"`
	Failures       int                 `json:"failures"`
	Successes      int                 `json:"successes"`
	Skips          int                 `json:"skips"`
	Outputs        map[string][]string `json:"-"`                        // Temporary storage for outputs during test run
	PassedOutputs  map[string][]string `json:"passed_outputs,omitempty"` // Outputs for passed runs
	FailedOutputs  map[string][]string `json:"failed_outputs,omitempty"` // Outputs for failed runs
	Durations      []time.Duration     `json:"durations"`
	PackageOutputs []string            `json:"package_outputs,omitempty"`
	TestPath       string              `json:"test_path"`
	CodeOwners     []string            `json:"code_owners"`
}

// SummaryData contains aggregated data from a set of test results
type SummaryData struct {
	// Overall test run stats
	// UniqueTestsRun tracks how many unique tests were run
	UniqueTestsRun int `json:"unique_tests_run"`
	// UniqueSkippedTestCount tracks how many unique tests were entirely skipped
	UniqueSkippedTestCount int `json:"unique_skipped_test_count"`
	// TestRunCount tracks the max amount of times the tests were run, giving an idea of how many times flakeguard was executed
	// e.g. if TestA was run 5 times, and TestB was run 10 times, UniqueTestsRun == 2 and TestRunCount == 10
	TestRunCount int `json:"test_run_count"`
	// PanickedTests tracks how many tests panicked
	PanickedTests int `json:"panicked_tests"`
	// RacedTests tracks how many tests raced
	RacedTests int `json:"raced_tests"`
	// FlakyTests tracks how many tests are considered flaky
	FlakyTests int `json:"flaky_tests"`
	// FlakyTestPercent is the human-readable percentage of tests that are considered flaky
	FlakyTestPercent string `json:"flaky_test_percent"`

	// Individual test run counts
	// TotalRuns tracks how many total test runs were executed
	// e.g. if TestA was run 5 times, and TestB was run 10 times, TotalRuns would be 15
	TotalRuns int `json:"total_runs"`
	// PassedRuns tracks how many test runs passed
	PassedRuns int `json:"passed_runs"`
	// FailedRuns tracks how many test runs failed
	FailedRuns int `json:"failed_runs"`
	// SkippedRuns tracks how many test runs were skipped
	SkippedRuns int `json:"skipped_runs"`
	// PassPercent is the human-readable percentage of test runs that passed
	PassPercent string `json:"pass_percent"`
}

// SplunkType represents what type of data is being sent to Splunk, e.g. a report or a result.
// This is a custom field to help us distinguish what kind of data we're sending.
type SplunkType string

const (
	Report SplunkType = "report"
	Result SplunkType = "result"

	// https://docs.splunk.com/Splexicon:Sourcetype
	SplunkSourceType = "flakeguard_json"
	// https://docs.splunk.com/Splexicon:Index
	SplunkIndex = "github_flakeguard_runs"
)

// SplunkTestReport is the full wrapper structure sent to Splunk for the full test report (sans results)
type SplunkTestReport struct {
	Event      SplunkTestReportEvent `json:"event"`      // https://docs.splunk.com/Splexicon:Event
	SourceType string                `json:"sourcetype"` // https://docs.splunk.com/Splexicon:Sourcetype
	Index      string                `json:"index"`      // https://docs.splunk.com/Splexicon:Index
}

// SplunkTestReportEvent contains the actual meat of the Splunk test report event
type SplunkTestReportEvent struct {
	Event string     `json:"event"`
	Type  SplunkType `json:"type"`
	Data  TestReport `json:"data"`
	// Incomplete indicates that there were issues uploading test results and the report is incomplete
	Incomplete bool `json:"incomplete"`
}

// SplunkTestResult is the full wrapper structure sent to Splunk for a single test result
type SplunkTestResult struct {
	Event      SplunkTestResultEvent `json:"event"`      // https://docs.splunk.com/Splexicon:Event
	SourceType string                `json:"sourcetype"` // https://docs.splunk.com/Splexicon:Sourcetype
	Index      string                `json:"index"`      // https://docs.splunk.com/Splexicon:Index
}

// SplunkTestResultEvent contains the actual meat of the Splunk test result event
type SplunkTestResultEvent struct {
	Event string     `json:"event"`
	Type  SplunkType `json:"type"`
	Data  TestResult `json:"data"`
}

// Data Processing Functions

func FilterResults(report *TestReport, maxPassRatio float64) *TestReport {
	filteredResults := FilterTests(report.Results, func(tr TestResult) bool {
		return !tr.Skipped && tr.PassRatio < maxPassRatio
	})
	report.Results = filteredResults
	return report
}

func FilterTests(results []TestResult, predicate func(TestResult) bool) []TestResult {
	var filtered []TestResult
	for _, result := range results {
		if predicate(result) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func mergeTestResults(a, b TestResult) TestResult {
	a.Runs += b.Runs
	a.Durations = append(a.Durations, b.Durations...)
	if a.PassedOutputs == nil {
		a.PassedOutputs = make(map[string][]string)
	}
	if a.FailedOutputs == nil {
		a.FailedOutputs = make(map[string][]string)
	}
	for runID, outputs := range b.PassedOutputs {
		a.PassedOutputs[runID] = append(a.PassedOutputs[runID], outputs...)
	}
	for runID, outputs := range b.FailedOutputs {
		a.FailedOutputs[runID] = append(a.FailedOutputs[runID], outputs...)
	}
	a.PackageOutputs = append(a.PackageOutputs, b.PackageOutputs...)
	a.Successes += b.Successes
	a.Failures += b.Failures
	a.Panic = a.Panic || b.Panic
	a.Race = a.Race || b.Race
	a.Skips += b.Skips
	a.Skipped = a.Skipped && b.Skipped

	if a.Runs > 0 {
		a.PassRatio = float64(a.Successes) / float64(a.Runs)
	} else {
		a.PassRatio = -1.0 // Indicate undefined pass ratio for skipped tests
	}

	return a
}

func sortTestResults(results []TestResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].TestPackage != results[j].TestPackage {
			return results[i].TestPackage < results[j].TestPackage
		}
		iParts := strings.Split(results[i].TestName, "/")
		jParts := strings.Split(results[j].TestName, "/")
		for k := 0; k < len(iParts) && k < len(jParts); k++ {
			if iParts[k] != jParts[k] {
				return iParts[k] < jParts[k]
			}
		}
		if len(iParts) != len(jParts) {
			return len(iParts) < len(jParts)
		}
		return results[i].PassRatio < results[j].PassRatio
	})
}

func avgDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

// passRatio calculates the pass percentage in statistical terms (0-1)
func passRatio(successes, runs int) float64 {
	passRatio := 1.0
	if runs > 0 {
		passRatio = (float64(successes) / float64(runs))
	}
	return passRatio
}

// flakeRatio calculates the flake percentage in statistical terms (0-1)
func flakeRatio(flakyTests, totalTests int) float64 {
	flakeRatio := 0.0
	if totalTests > 0 {
		flakeRatio = (float64(flakyTests) / float64(totalTests))
	}
	return flakeRatio
}

// formatRatio converts a float ratio (0.0-1.0) into a human-readable string (0.00%-100.00%)
func formatRatio(ratio float64) string {
	ratio *= 100
	// Format with 4 decimal places
	s := fmt.Sprintf("%.4f", ratio)
	// Trim trailing zeros
	s = strings.TrimRight(s, "0")
	// Trim trailing '.' if needed (in case we have an integer)
	s = strings.TrimRight(s, ".")
	return s + "%"
}
