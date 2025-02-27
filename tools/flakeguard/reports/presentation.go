package reports

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// GenerateFlakyTestsTable generates a table of flaky tests from the given test report to print to the console or markdown.
func GenerateFlakyTestsTable(
	testReport *TestReport,
	markdown bool,
) [][]string {
	p := message.NewPrinter(language.English)

	// Headers in the requested order
	headers := []string{
		"Name",
		"Pass Ratio",
		"Panicked?",
		"Timed Out?",
		"Race?",
		"Runs",
		"Successes",
		"Failures",
		"Skips",
		"Package",
		"Package Panicked?",
		"Avg Duration",
		"Code Owners",
	}

	// Format headers for Markdown if needed
	if markdown {
		for i, header := range headers {
			headers[i] = fmt.Sprintf("**%s**", header)
		}
	}

	// Initialize the table with headers
	table := [][]string{headers}

	for _, result := range testReport.Results {
		// Exclude skipped tests and only include tests below the expected pass ratio
		if !result.Skipped && result.PassRatio < testReport.MaxPassRatio {
			row := []string{
				result.TestName,
				formatRatio(result.PassRatio),
				fmt.Sprintf("%t", result.Panic),
				fmt.Sprintf("%t", result.Timeout),
				fmt.Sprintf("%t", result.Race),
				p.Sprintf("%d", result.Runs),
				p.Sprintf("%d", result.Successes),
				p.Sprintf("%d", result.Failures),
				p.Sprintf("%d", result.Skips),
				result.TestPackage,
				fmt.Sprintf("%t", result.PackagePanic),
				avgDuration(result.Durations).String(),
			}

			// Add code owners
			owners := "Unknown"
			if len(result.CodeOwners) > 0 {
				owners = strings.Join(result.CodeOwners, ", ")
			}
			row = append(row, owners)

			table = append(table, row)
		}
	}
	return table
}

// GenerateGitHubSummaryMarkdown generates a markdown summary of the test results for a GitHub workflow summary
func GenerateGitHubSummaryMarkdown(w io.Writer, testReport *TestReport, maxPassRatio float64, artifactName, artifactLink string) {
	fmt.Fprint(w, "# Flakeguard Summary\n\n")

	if len(testReport.Results) == 0 {
		fmt.Fprintln(w, "No tests were executed.")
		return
	}

	settingsTable := buildSettingsTable(testReport, maxPassRatio)
	printTable(w, settingsTable, false)
	fmt.Fprintln(w)

	if testReport.SummaryData.FlakyTests > 0 {
		fmt.Fprintln(w, "## Found Flaky Tests :x:")
	} else {
		fmt.Fprintln(w, "## No Flakes Found :white_check_mark:")
	}
	fmt.Fprintln(w)

	RenderResults(w, testReport, true, false)

	if artifactLink != "" {
		renderArtifactSection(w, artifactName, artifactLink)
	}

	if testReport.SummaryData.FlakyTests > 0 {
		renderTroubleshootingSection(w)
	}
}

// GeneratePRCommentMarkdown generates a markdown summary of the test results for a GitHub PR comment.
func GeneratePRCommentMarkdown(
	w io.Writer,
	testReport *TestReport,
	maxPassRatio float64,
	baseBranch, currentBranch, currentCommitSHA, repoURL, actionRunID, artifactName, artifactLink string,
) {
	fmt.Fprint(w, "# Flakeguard Summary\n\n")

	if len(testReport.Results) == 0 {
		fmt.Fprintln(w, "No tests were executed.")
		return
	}

	// Construct additional info
	additionalInfo := fmt.Sprintf(
		"Ran new or updated tests between `%s` and %s (`%s`).",
		baseBranch,
		currentCommitSHA,
		currentBranch,
	)

	// Construct the links
	viewDetailsLink := fmt.Sprintf("[View Flaky Detector Details](%s/actions/runs/%s)", repoURL, actionRunID)
	compareChangesLink := fmt.Sprintf("[Compare Changes](%s/compare/%s...%s#files_bucket)", repoURL, baseBranch, currentCommitSHA)
	linksLine := fmt.Sprintf("%s | %s", viewDetailsLink, compareChangesLink)

	// Include additional information
	fmt.Fprintln(w, additionalInfo)
	fmt.Fprintln(w) // Add an extra newline for formatting

	// Include the links
	fmt.Fprintln(w, linksLine)
	fmt.Fprintln(w) // Add an extra newline for formatting

	// Add the flaky tests section
	if testReport.SummaryData.FlakyTests > 0 {
		fmt.Fprintln(w, "## Found Flaky Tests :x:")
	} else {
		fmt.Fprintln(w, "## No Flakes Found :white_check_mark:")
	}

	resultsTable := GenerateFlakyTestsTable(testReport, true)
	renderTestResultsTable(w, resultsTable, true)

	if artifactLink != "" {
		renderArtifactSection(w, artifactName, artifactLink)
	}
}

func buildSettingsTable(testReport *TestReport, maxPassRatio float64) [][]string {
	rows := [][]string{
		{"**Setting**", "**Value**"},
	}

	if testReport.GoProject != "" {
		rows = append(rows, []string{"Project", testReport.GoProject})
	}

	rows = append(rows, []string{"Max Pass Ratio", fmt.Sprintf("%.2f%%", maxPassRatio*100)})
	rows = append(rows, []string{"Test Run Count", fmt.Sprintf("%d", testReport.SummaryData.TestRunCount)})
	rows = append(rows, []string{"Race Detection", fmt.Sprintf("%t", testReport.RaceDetection)})

	if len(testReport.ExcludedTests) > 0 {
		rows = append(rows, []string{"Excluded Tests", strings.Join(testReport.ExcludedTests, ", ")})
	}
	if len(testReport.SelectedTests) > 0 {
		rows = append(rows, []string{"Selected Tests", strings.Join(testReport.SelectedTests, ", ")})
	}

	return rows
}

func RenderError(
	w io.Writer,
	err error,
) {
	fmt.Fprintln(w, ":x: Error Running Flakeguard :x:")
}

// RenderResults renders the test results into a console or markdown format.
// If in markdown mode, the table results can also be made collapsible.
func RenderResults(
	w io.Writer,
	testReport *TestReport,
	markdown bool,
	collapsible bool,
) {
	resultsTable := GenerateFlakyTestsTable(testReport, markdown)
	renderSummaryTable(w, testReport.SummaryData, markdown, false, testReport.RaceDetection) // Don't make the summary collapsible
	renderTestResultsTable(w, resultsTable, collapsible)
}

// renderSummaryTable renders a summary table with the given data into a console or markdown format.
// If in markdown mode, the table can also be made collapsible.
func renderSummaryTable(w io.Writer, summary *SummaryData, markdown bool, collapsible bool, raceDetection bool) {
	summaryData := [][]string{
		{"Category", "Total"},
		{"Unique Tests", fmt.Sprintf("%d", summary.UniqueTestsRun)},
		{"Unique Flaky Tests", fmt.Sprintf("%d (%s)", summary.FlakyTests, summary.FlakyTestPercent)},
		{"Unique Skipped Tests", fmt.Sprintf("%d", summary.UniqueSkippedTestCount)},
		{"Unique Panicked Tests", fmt.Sprintf("%d", summary.PanickedTests)},
		{"Total Test Runs", fmt.Sprintf("%d", summary.TotalRuns)},
		{"Passed Test Runs", fmt.Sprintf("%d (%s)", summary.PassedRuns, summary.PassPercent)},
	}
	// Only include "Raced Tests" row if race detection is enabled.
	if raceDetection {
		summaryData = append(summaryData, []string{"Raced Tests", fmt.Sprintf("%d", summary.RacedTests)})
	}

	if markdown {
		for i, row := range summaryData {
			if i == 0 {
				summaryData[i] = []string{"**Category**", "**Total**"}
			} else {
				summaryData[i] = []string{fmt.Sprintf("**%s**", row[0]), row[1]}
			}
		}
	}
	printTable(w, summaryData, collapsible && markdown)
	fmt.Fprintln(w)
}

func renderTestResultsTable(w io.Writer, table [][]string, collapsible bool) {
	if len(table) <= 1 {
		fmt.Fprintln(w, "No tests found under the specified pass ratio threshold.")
		return
	}
	printTable(w, table, collapsible)
}

func renderArtifactSection(w io.Writer, artifactName, artifactLink string) {
	if artifactLink != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Artifacts")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "For detailed logs of the failed tests, please refer to the artifact [%s](%s).\n", artifactName, artifactLink)
	}
}

// renderTroubleshootingSection appends a troubleshooting section with a link to the README
func renderTroubleshootingSection(w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Troubleshooting Flaky Tests 🔍")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "For guidance on diagnosing and resolving E2E test flakiness, refer to the [Finding the Root Cause of Test Flakes](https://github.com/smartcontractkit/chainlink-testing-framework/blob/main/tools/flakeguard/e2e-flaky-test-guide.md) guide.")
}

// printTable prints a markdown table to the given writer in a pretty format.
func printTable(w io.Writer, table [][]string, collapsible bool) {
	colWidths := calculateColumnWidths(table)
	separator := buildSeparator(colWidths)

	if collapsible {
		numResults := len(table) - 1
		fmt.Fprintln(w, "<details>")
		fmt.Fprintf(w, "<summary>%d Results</summary>\n\n", numResults)
	}

	for i, row := range table {
		printRow(w, row, colWidths)
		if i == 0 {
			fmt.Fprintln(w, separator)
		}
	}

	if collapsible {
		fmt.Fprintln(w, "</details>")
	}
}

func calculateColumnWidths(table [][]string) []int {
	colWidths := make([]int, len(table[0]))
	for _, row := range table {
		for i, cell := range row {
			if len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}
	return colWidths
}

func buildSeparator(colWidths []int) string {
	var buffer bytes.Buffer
	for _, width := range colWidths {
		buffer.WriteString("|-")
		buffer.WriteString(strings.Repeat("-", width))
		buffer.WriteString("-")
	}
	buffer.WriteString("|")
	return buffer.String()
}

func printRow(w io.Writer, row []string, colWidths []int) {
	var buffer bytes.Buffer
	for i, cell := range row {
		buffer.WriteString(fmt.Sprintf("| %-*s ", colWidths[i], cell))
	}
	buffer.WriteString("|")
	fmt.Fprintln(w, buffer.String())
}
