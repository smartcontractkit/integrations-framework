package transformer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestEvent alias for readability in tests
type testEvent = TestEvent

// createEvent is a helper to create test events
func createEvent(action, pkg, test string, elapsed float64, output string) testEvent {
	return testEvent{
		Time:    time.Now(),
		Action:  action,
		Package: pkg,
		Test:    test,
		Elapsed: elapsed,
		Output:  output,
	}
}

// transformAndVerify is a helper function to transform events and verify the results
func transformAndVerify(t *testing.T, events []testEvent, opts *Options, expectedActions map[string]string) {
	// Convert test events to JSON
	var input bytes.Buffer
	for _, event := range events {
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		input.WriteString(string(eventJSON) + "\n")
	}

	// Transform the events
	var output bytes.Buffer
	err := TransformJSON(&input, &output, opts)
	if err != nil {
		t.Fatalf("Failed to transform JSON: %v", err)
	}

	// Parse the output
	scanner := bufio.NewScanner(&output)
	var resultEvents []TestEvent
	for scanner.Scan() {
		var event TestEvent
		if err := json.Unmarshal([]byte(scanner.Text()), &event); err != nil {
			t.Fatalf("Failed to unmarshal result event: %v", err)
		}
		resultEvents = append(resultEvents, event)
	}

	// Extract final actions for each test
	finalActions := make(map[string]string)
	for _, event := range resultEvents {
		if event.Action == "pass" || event.Action == "fail" || event.Action == "skip" {
			testID := fmt.Sprintf("%s/%s", event.Package, event.Test)
			finalActions[testID] = event.Action
		}
	}

	// Verify expected actions
	for testID, expectedAction := range expectedActions {
		actualAction, found := finalActions[testID]
		if !found {
			t.Errorf("Test %q not found in output", testID)
		} else if actualAction != expectedAction {
			t.Errorf("Expected action %q for test %q, got %q", expectedAction, testID, actualAction)
		}
	}
}

// TestIgnoreAllSubtests tests ignoring all subtests
func TestIgnoreAllSubtests(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestParent", 0, ""),
		createEvent("run", "example", "TestParent/SubTest1", 0, ""),
		createEvent("pass", "example", "TestParent/SubTest1", 0.001, ""),
		createEvent("run", "example", "TestParent/SubTest2", 0, ""),
		createEvent("output", "example", "TestParent/SubTest2", 0, "    test.go:10: SubTest2 failed\n"),
		createEvent("fail", "example", "TestParent/SubTest2", 0.001, ""),
		createEvent("output", "example", "TestParent", 0, "=== FAIL: TestParent (0.002s)\n"),
		createEvent("fail", "example", "TestParent", 0.002, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.003, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	expectedActions := map[string]string{
		"example/TestParent/SubTest2": "fail",
		"example/TestParent":          "pass",
		"example/":                    "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestNestedSubtests tests handling nested subtests
func TestNestedSubtests(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestParent", 0, ""),
		createEvent("run", "example", "TestParent/SubTest1", 0, ""),
		createEvent("pass", "example", "TestParent/SubTest1", 0.001, ""),
		createEvent("run", "example", "TestParent/SubTest2", 0, ""),
		createEvent("run", "example", "TestParent/SubTest2/NestedPass", 0, ""),
		createEvent("pass", "example", "TestParent/SubTest2/NestedPass", 0.001, ""),
		createEvent("run", "example", "TestParent/SubTest2/NestedFail", 0, ""),
		createEvent("output", "example", "TestParent/SubTest2/NestedFail", 0, "    test.go:20: NestedFail failed\n"),
		createEvent("fail", "example", "TestParent/SubTest2/NestedFail", 0.001, ""),
		createEvent("output", "example", "TestParent/SubTest2", 0, "=== FAIL: TestParent/SubTest2 (0.002s)\n"),
		createEvent("fail", "example", "TestParent/SubTest2", 0.002, ""),
		createEvent("output", "example", "TestParent", 0, "=== FAIL: TestParent (0.003s)\n"),
		createEvent("fail", "example", "TestParent", 0.003, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.004, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	expectedActions := map[string]string{
		"example/TestParent/SubTest2/NestedFail": "fail",
		"example/TestParent/SubTest2":            "pass",
		"example/TestParent":                     "pass",
		"example/":                               "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestParallelTestsEventOrdering tests handling of parallel tests
func TestParallelTestsEventOrdering(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestParallel", 0, ""),
		createEvent("run", "example", "TestParallel/Sub1", 0, ""),
		createEvent("output", "example", "TestParallel/Sub1", 0, "=== PAUSE TestParallel/Sub1\n"),
		createEvent("run", "example", "TestParallel/Sub2", 0, ""),
		createEvent("output", "example", "TestParallel/Sub2", 0, "=== PAUSE TestParallel/Sub2\n"),
		createEvent("output", "example", "TestParallel/Sub1", 0, "=== CONT  TestParallel/Sub1\n"),
		createEvent("pass", "example", "TestParallel/Sub1", 0.001, ""),
		createEvent("output", "example", "TestParallel/Sub2", 0, "=== CONT  TestParallel/Sub2\n"),
		createEvent("output", "example", "TestParallel/Sub2", 0, "    test.go:15: Sub2 failed\n"),
		createEvent("fail", "example", "TestParallel/Sub2", 0.001, ""),
		createEvent("output", "example", "TestParallel", 0, "=== FAIL: TestParallel (0.002s)\n"),
		createEvent("fail", "example", "TestParallel", 0.002, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.003, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	expectedActions := map[string]string{
		"example/TestParallel/Sub2": "fail",
		"example/TestParallel":      "pass",
		"example/":                  "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestOutputTransformation tests transformation of output text
func TestOutputTransformation(t *testing.T) {
	input := "=== FAIL: TestSomething (0.001s)\nFAIL\n--- FAIL: TestSomething/SubTest (0.001s)\n"
	expected := "=== PASS: TestSomething (0.001s)\nPASS\n--- PASS: TestSomething/SubTest (0.001s)\n"

	actual := transformOutputText(input)

	if actual != expected {
		t.Errorf("Expected transformed output:\n%s\nGot:\n%s", expected, actual)
	}
}

// TestEmptyInput tests handling empty input
func TestEmptyInput(t *testing.T) {
	opts := DefaultOptions()

	var input, output bytes.Buffer
	err := TransformJSON(&input, &output, opts)

	if err != nil {
		t.Errorf("Expected no error for empty input, got: %v", err)
	}

	if output.Len() != 0 {
		t.Errorf("Expected empty output for empty input, got: %s", output.String())
	}
}

// TestMalformedJSON tests handling malformed JSON input
func TestMalformedJSON(t *testing.T) {
	opts := DefaultOptions()

	input := strings.NewReader("This is not JSON\n{\"Action\":\"fail\"")
	var output bytes.Buffer

	err := TransformJSON(input, &output, opts)

	if err == nil {
		t.Error("Expected error for malformed JSON, got nil")
	}
}

func TestParentWithDirectFailureAndFailingSubtest(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestParent", 0, ""),
		createEvent("run", "example", "TestParent/SubTest1", 0, ""),
		createEvent("pass", "example", "TestParent/SubTest1", 0.001, ""),
		createEvent("run", "example", "TestParent/SubTest2", 0, ""),
		// SubTest2 has both a direct failure and a failing subtest
		createEvent("run", "example", "TestParent/SubTest2/NestedTest", 0, ""),
		createEvent("output", "example", "TestParent/SubTest2/NestedTest", 0, "    test.go:15: NestedTest failed\n"),
		createEvent("fail", "example", "TestParent/SubTest2/NestedTest", 0.001, ""),
		// Direct failure in SubTest2
		createEvent("output", "example", "TestParent/SubTest2", 0, "    test.go:20: SubTest2 has a direct failure\n"),
		createEvent("output", "example", "TestParent/SubTest2", 0, "=== FAIL: TestParent/SubTest2 (0.002s)\n"),
		createEvent("fail", "example", "TestParent/SubTest2", 0.002, ""),
		createEvent("output", "example", "TestParent", 0, "=== FAIL: TestParent (0.003s)\n"),
		createEvent("fail", "example", "TestParent", 0.003, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.004, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// With the updated behavior:
	// - The leaf subtest (NestedTest) should remain as "fail"
	// - The parent subtest (SubTest2) should be changed to "pass" because it has a failing subtest,
	//   even though it also has a direct failure
	// - The top-level test (TestParent) should be changed to "pass" because it has a failing subtest
	// - The package should be changed to "pass"
	expectedActions := map[string]string{
		"example/TestParent/SubTest2/NestedTest": "fail",
		"example/TestParent/SubTest2":            "pass",
		"example/TestParent":                     "pass",
		"example/":                               "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

func TestParentWithOnlyDirectFailure(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestParent", 0, ""),
		createEvent("run", "example", "TestParent/SubTest1", 0, ""),
		createEvent("pass", "example", "TestParent/SubTest1", 0.001, ""),
		createEvent("run", "example", "TestParent/SubTest2", 0, ""),
		createEvent("pass", "example", "TestParent/SubTest2", 0.001, ""),
		// Direct failure in the parent test
		createEvent("output", "example", "TestParent", 0, "    test.go:20: TestParent has a direct failure\n"),
		createEvent("output", "example", "TestParent", 0, "=== FAIL: TestParent (0.003s)\n"),
		createEvent("fail", "example", "TestParent", 0.003, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.004, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// In this case:
	// - All subtests pass
	// - The parent test has a direct failure but no failing subtests, so it should remain as "fail"
	// - The package should remain as "fail" because the parent test fails
	expectedActions := map[string]string{
		"example/TestParent/SubTest1": "pass",
		"example/TestParent/SubTest2": "pass",
		"example/TestParent":          "fail",
		"example/":                    "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestMultiLevelNestedFailures tests multiple levels of nested failures
func TestMultiLevelNestedFailures(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestParent", 0, ""),
		createEvent("run", "example", "TestParent/Level1", 0, ""),
		createEvent("run", "example", "TestParent/Level1/Level2A", 0, ""),
		createEvent("pass", "example", "TestParent/Level1/Level2A", 0.001, ""),
		createEvent("run", "example", "TestParent/Level1/Level2B", 0, ""),
		createEvent("run", "example", "TestParent/Level1/Level2B/Level3A_Flaky", 0, ""),
		createEvent("output", "example", "TestParent/Level1/Level2B/Level3A_Flaky", 0, "    test.go:25: Level3A_Flaky test failed\n"),
		createEvent("fail", "example", "TestParent/Level1/Level2B/Level3A_Flaky", 0.001, ""),
		createEvent("run", "example", "TestParent/Level1/Level2B/Level3B", 0, ""),
		createEvent("pass", "example", "TestParent/Level1/Level2B/Level3B", 0.001, ""),
		// Direct failure in Level2B
		createEvent("output", "example", "TestParent/Level1/Level2B", 0, "    test.go:30: Level2B has a direct failure\n"),
		createEvent("output", "example", "TestParent/Level1/Level2B", 0, "=== FAIL: TestParent/Level1/Level2B (0.002s)\n"),
		createEvent("fail", "example", "TestParent/Level1/Level2B", 0.002, ""),
		createEvent("output", "example", "TestParent/Level1", 0, "=== FAIL: TestParent/Level1 (0.003s)\n"),
		createEvent("fail", "example", "TestParent/Level1", 0.003, ""),
		createEvent("output", "example", "TestParent", 0, "=== FAIL: TestParent (0.004s)\n"),
		createEvent("fail", "example", "TestParent", 0.004, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.005, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// With IgnoreAllSubtestFailures=true:
	// - The leaf subtest (Level3A_Flaky) should remain as "fail"
	// - All parent tests should be changed to "pass"
	expectedActions := map[string]string{
		"example/TestParent/Level1/Level2B/Level3A_Flaky": "fail",
		"example/TestParent/Level1/Level2B":               "pass",
		"example/TestParent/Level1":                       "pass",
		"example/TestParent":                              "pass",
		"example/":                                        "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestOnlyDirectFailures tests a scenario where there are no failing subtests, only direct failures
func TestOnlyDirectFailures(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestParent", 0, ""),
		createEvent("run", "example", "TestParent/Level1", 0, ""),
		createEvent("run", "example", "TestParent/Level1/Level2A", 0, ""),
		createEvent("pass", "example", "TestParent/Level1/Level2A", 0.001, ""),
		createEvent("run", "example", "TestParent/Level1/Level2B", 0, ""),
		createEvent("run", "example", "TestParent/Level1/Level2B/Level3A", 0, ""),
		createEvent("pass", "example", "TestParent/Level1/Level2B/Level3A", 0.001, ""),
		createEvent("run", "example", "TestParent/Level1/Level2B/Level3B", 0, ""),
		createEvent("pass", "example", "TestParent/Level1/Level2B/Level3B", 0.001, ""),
		// Direct failure in Level2B (no subtest failures)
		createEvent("output", "example", "TestParent/Level1/Level2B", 0, "    test.go:30: Level2B has a direct failure\n"),
		createEvent("output", "example", "TestParent/Level1/Level2B", 0, "=== FAIL: TestParent/Level1/Level2B (0.002s)\n"),
		createEvent("fail", "example", "TestParent/Level1/Level2B", 0.002, ""),
		createEvent("output", "example", "TestParent/Level1", 0, "=== FAIL: TestParent/Level1 (0.003s)\n"),
		createEvent("fail", "example", "TestParent/Level1", 0.003, ""),
		createEvent("output", "example", "TestParent", 0, "=== FAIL: TestParent (0.004s)\n"),
		createEvent("fail", "example", "TestParent", 0.004, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.005, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// With IgnoreAllSubtestFailures=true:
	// - Level2B should remain as "fail" because it has a direct failure
	// - Level1 should be changed to "pass" because it only fails due to Level2B
	// - TestParent should be changed to "pass" because it only fails due to Level1
	// - The package should remain as "fail" because there are still failing tests
	expectedActions := map[string]string{
		"example/TestParent/Level1/Level2B": "fail", // Direct failure, should remain fail
		"example/TestParent/Level1":         "pass", // Only fails because of Level2B
		"example/TestParent":                "pass", // Only fails because of Level1
		"example/":                          "pass", // Still fails because Level2B has a direct failure
	}

	// Exit code should be 1 because there are still failing tests
	transformAndVerify(t, events, opts, expectedActions)
}

// TestLogMessagesNotDirectFailures tests that log messages are not treated as direct failures
func TestLogMessagesNotDirectFailures(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestWithLogs", 0, ""),
		createEvent("run", "example", "TestWithLogs/Parent", 0, ""),
		// Log message in parent test - should not be treated as a direct failure
		createEvent("output", "example", "TestWithLogs/Parent", 0, "    test.go:10: This is just a log message, not an error\n"),
		createEvent("run", "example", "TestWithLogs/Parent/Child", 0, ""),
		// Error in child test
		createEvent("output", "example", "TestWithLogs/Parent/Child", 0, "    test.go:15: Child test failed\n"),
		createEvent("output", "example", "TestWithLogs/Parent/Child", 0, "--- FAIL: TestWithLogs/Parent/Child (0.001s)\n"),
		createEvent("fail", "example", "TestWithLogs/Parent/Child", 0.001, ""),
		// Parent fails because child failed
		createEvent("output", "example", "TestWithLogs/Parent", 0, "--- FAIL: TestWithLogs/Parent (0.002s)\n"),
		createEvent("fail", "example", "TestWithLogs/Parent", 0.002, ""),
		createEvent("output", "example", "TestWithLogs", 0, "--- FAIL: TestWithLogs (0.003s)\n"),
		createEvent("fail", "example", "TestWithLogs", 0.003, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.004, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// With IgnoreAllSubtestFailures=true:
	// - The child test should remain as "fail"
	// - The parent test should be changed to "pass" because it only has a log message, not a direct failure
	// - The top-level test should be changed to "pass"
	// - The package should be changed to "pass"
	expectedActions := map[string]string{
		"example/TestWithLogs/Parent/Child": "fail",
		"example/TestWithLogs/Parent":       "pass",
		"example/TestWithLogs":              "pass",
		"example/":                          "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestNestedLogsAllPassing tests that log messages in passing tests are handled correctly
func TestNestedLogsAllPassing(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestWithPassingLogs", 0, ""),
		createEvent("run", "example", "TestWithPassingLogs/Parent", 0, ""),
		// Log message in parent test
		createEvent("output", "example", "TestWithPassingLogs/Parent", 0, "    test.go:10: This is just a log message in parent, not an error\n"),
		createEvent("run", "example", "TestWithPassingLogs/Parent/Child", 0, ""),
		// Log message in child test
		createEvent("output", "example", "TestWithPassingLogs/Parent/Child", 0, "    test.go:15: This is just a log message in child, not an error\n"),
		createEvent("output", "example", "TestWithPassingLogs/Parent/Child", 0, "--- PASS: TestWithPassingLogs/Parent/Child (0.001s)\n"),
		createEvent("pass", "example", "TestWithPassingLogs/Parent/Child", 0.001, ""),
		// Parent passes
		createEvent("output", "example", "TestWithPassingLogs/Parent", 0, "--- PASS: TestWithPassingLogs/Parent (0.002s)\n"),
		createEvent("pass", "example", "TestWithPassingLogs/Parent", 0.002, ""),
		createEvent("output", "example", "TestWithPassingLogs", 0, "--- PASS: TestWithPassingLogs (0.003s)\n"),
		createEvent("pass", "example", "TestWithPassingLogs", 0.003, ""),
		createEvent("output", "example", "", 0, "PASS\n"),
		createEvent("pass", "example", "", 0.004, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// All tests should remain as "pass"
	expectedActions := map[string]string{
		"example/TestWithPassingLogs/Parent/Child": "pass",
		"example/TestWithPassingLogs/Parent":       "pass",
		"example/TestWithPassingLogs":              "pass",
		"example/":                                 "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestSkippedTests tests handling of skipped tests
func TestSkippedTests(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestSkippedTests", 0, ""),
		createEvent("output", "example", "TestSkippedTests", 0, "=== RUN   TestSkippedTests\n"),
		createEvent("run", "example", "TestSkippedTests/SkippedTest", 0, ""),
		createEvent("output", "example", "TestSkippedTests/SkippedTest", 0, "=== RUN   TestSkippedTests/SkippedTest\n"),
		createEvent("output", "example", "TestSkippedTests/SkippedTest", 0, "    example_test.go:163: This test is skipped intentionally\n"),
		createEvent("output", "example", "TestSkippedTests/SkippedTest", 0, "--- SKIP: TestSkippedTests/SkippedTest (0.00s)\n"),
		createEvent("skip", "example", "TestSkippedTests/SkippedTest", 0, ""),
		createEvent("run", "example", "TestSkippedTests/ConditionallySkipped", 0, ""),
		createEvent("output", "example", "TestSkippedTests/ConditionallySkipped", 0, "=== RUN   TestSkippedTests/ConditionallySkipped\n"),
		createEvent("output", "example", "TestSkippedTests/ConditionallySkipped", 0, "--- PASS: TestSkippedTests/ConditionallySkipped (0.00s)\n"),
		createEvent("pass", "example", "TestSkippedTests/ConditionallySkipped", 0, ""),
		createEvent("output", "example", "TestSkippedTests", 0, "--- PASS: TestSkippedTests (0.00s)\n"),
		createEvent("pass", "example", "TestSkippedTests", 0, ""),
	}

	opts := &Options{} // Use default options

	// The transformer should preserve the original actions
	expectedActions := map[string]string{
		"example/TestSkippedTests/SkippedTest":          "skip",
		"example/TestSkippedTests/ConditionallySkipped": "pass",
		"example/TestSkippedTests":                      "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestDeepNesting tests handling of deeply nested test hierarchies
func TestDeepNesting(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestDeepNesting", 0, ""),
		createEvent("run", "example", "TestDeepNesting/Level1", 0, ""),
		createEvent("run", "example", "TestDeepNesting/Level1/Level2", 0, ""),
		createEvent("run", "example", "TestDeepNesting/Level1/Level2/Level3", 0, ""),
		createEvent("run", "example", "TestDeepNesting/Level1/Level2/Level3/Level4", 0, ""),
		createEvent("run", "example", "TestDeepNesting/Level1/Level2/Level3/Level4/Level5", 0, ""),
		createEvent("output", "example", "TestDeepNesting/Level1/Level2/Level3/Level4/Level5", 0, "    example_test.go:65: Deep nested test failed\n"),
		createEvent("output", "example", "TestDeepNesting/Level1/Level2/Level3/Level4/Level5", 0, "--- FAIL: TestDeepNesting/Level1/Level2/Level3/Level4/Level5 (0.001s)\n"),
		createEvent("fail", "example", "TestDeepNesting/Level1/Level2/Level3/Level4/Level5", 0.001, ""),
		createEvent("output", "example", "TestDeepNesting/Level1/Level2/Level3/Level4", 0, "--- FAIL: TestDeepNesting/Level1/Level2/Level3/Level4 (0.001s)\n"),
		createEvent("fail", "example", "TestDeepNesting/Level1/Level2/Level3/Level4", 0.001, ""),
		createEvent("output", "example", "TestDeepNesting/Level1/Level2/Level3", 0, "--- FAIL: TestDeepNesting/Level1/Level2/Level3 (0.001s)\n"),
		createEvent("fail", "example", "TestDeepNesting/Level1/Level2/Level3", 0.001, ""),
		createEvent("output", "example", "TestDeepNesting/Level1/Level2", 0, "--- FAIL: TestDeepNesting/Level1/Level2 (0.001s)\n"),
		createEvent("fail", "example", "TestDeepNesting/Level1/Level2", 0.001, ""),
		createEvent("output", "example", "TestDeepNesting/Level1", 0, "--- FAIL: TestDeepNesting/Level1 (0.001s)\n"),
		createEvent("fail", "example", "TestDeepNesting/Level1", 0.001, ""),
		createEvent("output", "example", "TestDeepNesting", 0, "--- FAIL: TestDeepNesting (0.001s)\n"),
		createEvent("fail", "example", "TestDeepNesting", 0.001, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.002, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// With IgnoreAllSubtestFailures=true:
	// - The leaf test should remain as "fail"
	// - All parent tests should be changed to "pass"
	expectedActions := map[string]string{
		"example/TestDeepNesting/Level1/Level2/Level3/Level4/Level5": "fail",
		"example/TestDeepNesting/Level1/Level2/Level3/Level4":        "pass",
		"example/TestDeepNesting/Level1/Level2/Level3":               "pass",
		"example/TestDeepNesting/Level1/Level2":                      "pass",
		"example/TestDeepNesting/Level1":                             "pass",
		"example/TestDeepNesting":                                    "pass",
		"example/":                                                   "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestConcurrentTests tests handling of concurrent test execution
func TestConcurrentTests(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestWithConcurrency", 0, ""),
		// First subtest starts
		createEvent("run", "example", "TestWithConcurrency/ConcurrentTest0", 0, ""),
		createEvent("output", "example", "TestWithConcurrency/ConcurrentTest0", 0, "=== RUN   TestWithConcurrency/ConcurrentTest0\n"),
		createEvent("output", "example", "TestWithConcurrency/ConcurrentTest0", 0, "=== PAUSE TestWithConcurrency/ConcurrentTest0\n"),
		createEvent("pause", "example", "TestWithConcurrency/ConcurrentTest0", 0, ""),
		// Second subtest starts
		createEvent("run", "example", "TestWithConcurrency/ConcurrentTest1", 0, ""),
		createEvent("output", "example", "TestWithConcurrency/ConcurrentTest1", 0, "=== RUN   TestWithConcurrency/ConcurrentTest1\n"),
		createEvent("output", "example", "TestWithConcurrency/ConcurrentTest1", 0, "=== PAUSE TestWithConcurrency/ConcurrentTest1\n"),
		createEvent("pause", "example", "TestWithConcurrency/ConcurrentTest1", 0, ""),
		// Continue tests
		createEvent("cont", "example", "TestWithConcurrency/ConcurrentTest0", 0, ""),
		createEvent("output", "example", "TestWithConcurrency/ConcurrentTest0", 0, "=== CONT  TestWithConcurrency/ConcurrentTest0\n"),
		createEvent("cont", "example", "TestWithConcurrency/ConcurrentTest1", 0, ""),
		createEvent("output", "example", "TestWithConcurrency/ConcurrentTest1", 0, "=== CONT  TestWithConcurrency/ConcurrentTest1\n"),
		// Test 0 fails
		createEvent("output", "example", "TestWithConcurrency/ConcurrentTest0", 0, "    example_test.go:303: Concurrent test 0 failed\n"),
		createEvent("output", "example", "TestWithConcurrency/ConcurrentTest0", 0, "--- FAIL: TestWithConcurrency/ConcurrentTest0 (0.05s)\n"),
		createEvent("fail", "example", "TestWithConcurrency/ConcurrentTest0", 0.05, ""),
		// Test 1 passes
		createEvent("output", "example", "TestWithConcurrency/ConcurrentTest1", 0, "--- PASS: TestWithConcurrency/ConcurrentTest1 (0.02s)\n"),
		createEvent("pass", "example", "TestWithConcurrency/ConcurrentTest1", 0.02, ""),
		// Parent test fails
		createEvent("output", "example", "TestWithConcurrency", 0, "--- FAIL: TestWithConcurrency (0.00s)\n"),
		createEvent("fail", "example", "TestWithConcurrency", 0, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.07, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// With IgnoreAllSubtestFailures=true:
	// - The failing subtest should remain as "fail"
	// - The passing subtest should remain as "pass"
	// - The parent test should be changed to "pass"
	expectedActions := map[string]string{
		"example/TestWithConcurrency/ConcurrentTest0": "fail",
		"example/TestWithConcurrency/ConcurrentTest1": "pass",
		"example/TestWithConcurrency":                 "pass",
		"example/":                                    "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestTableDrivenTests tests handling of table-driven tests
func TestTableDrivenTests(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestWithTableDrivenTests", 0, ""),
		// First case passes
		createEvent("run", "example", "TestWithTableDrivenTests/Zero", 0, ""),
		createEvent("output", "example", "TestWithTableDrivenTests/Zero", 0, "--- PASS: TestWithTableDrivenTests/Zero (0.00s)\n"),
		createEvent("pass", "example", "TestWithTableDrivenTests/Zero", 0, ""),
		// Second case passes
		createEvent("run", "example", "TestWithTableDrivenTests/Positive", 0, ""),
		createEvent("output", "example", "TestWithTableDrivenTests/Positive", 0, "--- PASS: TestWithTableDrivenTests/Positive (0.00s)\n"),
		createEvent("pass", "example", "TestWithTableDrivenTests/Positive", 0, ""),
		// Third case passes
		createEvent("run", "example", "TestWithTableDrivenTests/Negative", 0, ""),
		createEvent("output", "example", "TestWithTableDrivenTests/Negative", 0, "--- PASS: TestWithTableDrivenTests/Negative (0.00s)\n"),
		createEvent("pass", "example", "TestWithTableDrivenTests/Negative", 0, ""),
		// Fourth case fails
		createEvent("run", "example", "TestWithTableDrivenTests/FailingCase", 0, ""),
		createEvent("output", "example", "TestWithTableDrivenTests/FailingCase", 0, "    example_test.go:328: Test FailingCase failed: got true, want true\n"),
		createEvent("output", "example", "TestWithTableDrivenTests/FailingCase", 0, "--- FAIL: TestWithTableDrivenTests/FailingCase (0.00s)\n"),
		createEvent("fail", "example", "TestWithTableDrivenTests/FailingCase", 0, ""),
		// Parent test fails
		createEvent("output", "example", "TestWithTableDrivenTests", 0, "--- FAIL: TestWithTableDrivenTests (0.00s)\n"),
		createEvent("fail", "example", "TestWithTableDrivenTests", 0, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.001, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// With IgnoreAllSubtestFailures=true:
	// - The failing case should remain as "fail"
	// - The passing cases should remain as "pass"
	// - The parent test should be changed to "pass"
	expectedActions := map[string]string{
		"example/TestWithTableDrivenTests/Zero":        "pass",
		"example/TestWithTableDrivenTests/Positive":    "pass",
		"example/TestWithTableDrivenTests/Negative":    "pass",
		"example/TestWithTableDrivenTests/FailingCase": "fail",
		"example/TestWithTableDrivenTests":             "pass",
		"example/":                                     "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestSubtestReuse tests handling of tests that reuse the same subtest function
func TestSubtestReuse(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestWithSubtestReuse", 0, ""),
		// First subtest passes
		createEvent("run", "example", "TestWithSubtestReuse/First", 0, ""),
		createEvent("output", "example", "TestWithSubtestReuse/First", 0, "--- PASS: TestWithSubtestReuse/First (0.00s)\n"),
		createEvent("pass", "example", "TestWithSubtestReuse/First", 0, ""),
		// Second subtest fails
		createEvent("run", "example", "TestWithSubtestReuse/Second", 0, ""),
		createEvent("output", "example", "TestWithSubtestReuse/Second", 0, "    example_test.go:339: Subtest Second intentionally failed\n"),
		createEvent("output", "example", "TestWithSubtestReuse/Second", 0, "--- FAIL: TestWithSubtestReuse/Second (0.00s)\n"),
		createEvent("fail", "example", "TestWithSubtestReuse/Second", 0, ""),
		// Third subtest passes
		createEvent("run", "example", "TestWithSubtestReuse/Third", 0, ""),
		createEvent("output", "example", "TestWithSubtestReuse/Third", 0, "--- PASS: TestWithSubtestReuse/Third (0.00s)\n"),
		createEvent("pass", "example", "TestWithSubtestReuse/Third", 0, ""),
		// Fourth subtest fails
		createEvent("run", "example", "TestWithSubtestReuse/Fourth", 0, ""),
		createEvent("output", "example", "TestWithSubtestReuse/Fourth", 0, "    example_test.go:339: Subtest Fourth intentionally failed\n"),
		createEvent("output", "example", "TestWithSubtestReuse/Fourth", 0, "--- FAIL: TestWithSubtestReuse/Fourth (0.00s)\n"),
		createEvent("fail", "example", "TestWithSubtestReuse/Fourth", 0, ""),
		// Parent test fails
		createEvent("output", "example", "TestWithSubtestReuse", 0, "--- FAIL: TestWithSubtestReuse (0.00s)\n"),
		createEvent("fail", "example", "TestWithSubtestReuse", 0, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.001, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// With IgnoreAllSubtestFailures=true:
	// - The failing subtests should remain as "fail"
	// - The passing subtests should remain as "pass"
	// - The parent test should be changed to "pass"
	expectedActions := map[string]string{
		"example/TestWithSubtestReuse/First":  "pass",
		"example/TestWithSubtestReuse/Second": "fail",
		"example/TestWithSubtestReuse/Third":  "pass",
		"example/TestWithSubtestReuse/Fourth": "fail",
		"example/TestWithSubtestReuse":        "pass",
		"example/":                            "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

// TestSpecialCharactersInTestNames tests handling of tests with special characters in names
func TestSpecialCharactersInTestNames(t *testing.T) {
	events := []testEvent{
		createEvent("run", "example", "TestWithSpecialNames", 0, ""),
		// Test with spaces
		createEvent("run", "example", "TestWithSpecialNames/Test with spaces", 0, ""),
		createEvent("output", "example", "TestWithSpecialNames/Test with spaces", 0, "--- PASS: TestWithSpecialNames/Test with spaces (0.00s)\n"),
		createEvent("pass", "example", "TestWithSpecialNames/Test with spaces", 0, ""),
		// Test with symbols
		createEvent("run", "example", "TestWithSpecialNames/Test-with-hyphens", 0, ""),
		createEvent("output", "example", "TestWithSpecialNames/Test-with-hyphens", 0, "    example_test.go:250: Test with hyphens failed\n"),
		createEvent("output", "example", "TestWithSpecialNames/Test-with-hyphens", 0, "--- FAIL: TestWithSpecialNames/Test-with-hyphens (0.00s)\n"),
		createEvent("fail", "example", "TestWithSpecialNames/Test-with-hyphens", 0, ""),
		// Test with braces
		createEvent("run", "example", "TestWithSpecialNames/Test with {braces}", 0, ""),
		createEvent("output", "example", "TestWithSpecialNames/Test with {braces}", 0, "--- PASS: TestWithSpecialNames/Test with {braces} (0.00s)\n"),
		createEvent("pass", "example", "TestWithSpecialNames/Test with {braces}", 0, ""),
		// Parent test fails
		createEvent("output", "example", "TestWithSpecialNames", 0, "--- FAIL: TestWithSpecialNames (0.00s)\n"),
		createEvent("fail", "example", "TestWithSpecialNames", 0, ""),
		createEvent("output", "example", "", 0, "FAIL\n"),
		createEvent("fail", "example", "", 0.001, ""),
	}

	opts := &Options{
		IgnoreAllSubtestFailures: true,
	}

	// With IgnoreAllSubtestFailures=true:
	// - The failing subtest should remain as "fail"
	// - The passing subtests should remain as "pass"
	// - The parent test should be changed to "pass"
	expectedActions := map[string]string{
		"example/TestWithSpecialNames/Test with spaces":   "pass",
		"example/TestWithSpecialNames/Test-with-hyphens":  "fail",
		"example/TestWithSpecialNames/Test with {braces}": "pass",
		"example/TestWithSpecialNames":                    "pass",
		"example/":                                        "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

func TestSubTestNameWithSlashes(t *testing.T) {
	opts := &Options{
		IgnoreAllSubtestFailures: false,
	}

	events := []testEvent{
		createEvent("start", "example", "", 0.0, ""),
		createEvent("run", "example", "TestSubTestNameWithSlashes", 0.0, "=== RUN   TestSubTestNameWithSlashes\n"),
		createEvent("output", "example", "TestSubTestNameWithSlashes", 0.0, "=== PAUSE TestSubTestNameWithSlashes\n"),
		createEvent("pause", "example", "TestSubTestNameWithSlashes", 0.0, ""),
		createEvent("cont", "example", "TestSubTestNameWithSlashes", 0.0, "=== CONT  TestSubTestNameWithSlashes\n"),
		createEvent("run", "example", "TestSubTestNameWithSlashes/sub/test/name/with/slashes", 0.0, "=== RUN   TestSubTestNameWithSlashes/sub/test/name/with/slashes\n"),
		createEvent("output", "example", "TestSubTestNameWithSlashes/sub/test/name/with/slashes", 0.0, "    example_test.go:356: This subtest always passes\n"),
		createEvent("output", "example", "TestSubTestNameWithSlashes/sub/test/name/with/slashes", 0.0, "--- PASS: TestSubTestNameWithSlashes/sub/test/name/with/slashes (0.00s)\n"),
		createEvent("pass", "example", "TestSubTestNameWithSlashes/sub/test/name/with/slashes", 0.0, ""),
		createEvent("output", "example", "TestSubTestNameWithSlashes", 0.0, "--- PASS: TestSubTestNameWithSlashes (0.00s)\n"),
		createEvent("pass", "example", "TestSubTestNameWithSlashes", 0.0, ""),
		createEvent("output", "example", "", 0.0, "PASS\n"),
		createEvent("output", "example", "", 0.0, "ok  \texample\t0.188s\n"),
		createEvent("pass", "example", "", 0.188, ""),
	}

	expectedActions := map[string]string{
		"example/TestSubTestNameWithSlashes/sub/test/name/with/slashes": "pass",
		"example/TestSubTestNameWithSlashes":                            "pass",
		"example/":                                                      "pass",
	}

	transformAndVerify(t, events, opts, expectedActions)
}

func TestFuzzTestWithCorpus(t *testing.T) {
	opts := &Options{
		IgnoreAllSubtestFailures: false,
	}

	events := []testEvent{
		createEvent("start", "example", "", 0.0, ""),
		createEvent("run", "example", "FuzzTestWithCorpus", 0.0, "=== RUN   FuzzTestWithCorpus\n"),

		createEvent("run", "example", "FuzzTestWithCorpus/seed#0", 0.0, "=== RUN   FuzzTestWithCorpus/seed#0\n"),
		createEvent("output", "example", "FuzzTestWithCorpus/seed#0", 0.0, "    example_test.go:367: Fuzzing with input: some\n"),
		createEvent("output", "example", "FuzzTestWithCorpus/seed#0", 0.0, "--- PASS: FuzzTestWithCorpus/seed#0 (0.00s)\n"),
		createEvent("pass", "example", "FuzzTestWithCorpus/seed#0", 0.0, ""),

		createEvent("run", "example", "FuzzTestWithCorpus/seed#1", 0.0, "=== RUN   FuzzTestWithCorpus/seed#1\n"),
		createEvent("output", "example", "FuzzTestWithCorpus/seed#1", 0.0, "    example_test.go:367: Fuzzing with input: corpus\n"),
		createEvent("output", "example", "FuzzTestWithCorpus/seed#1", 0.0, "--- PASS: FuzzTestWithCorpus/seed#1 (0.00s)\n"),
		createEvent("pass", "example", "FuzzTestWithCorpus/seed#1", 0.0, ""),

		createEvent("run", "example", "FuzzTestWithCorpus/seed#2", 0.0, "=== RUN   FuzzTestWithCorpus/seed#2\n"),
		createEvent("output", "example", "FuzzTestWithCorpus/seed#2", 0.0, "    example_test.go:367: Fuzzing with input: values\n"),
		createEvent("output", "example", "FuzzTestWithCorpus/seed#2", 0.0, "--- PASS: FuzzTestWithCorpus/seed#2 (0.00s)\n"),
		createEvent("pass", "example", "FuzzTestWithCorpus/seed#2", 0.0, ""),

		createEvent("output", "example", "FuzzTestWithCorpus", 0.0, "--- PASS: FuzzTestWithCorpus (0.00s)\n"),
		createEvent("pass", "example", "FuzzTestWithCorpus", 0.0, ""),

		createEvent("output", "example", "", 0.0, "PASS\n"),
		createEvent("output", "example", "", 0.0, "ok  \texample\t0.231s\n"),
		createEvent("pass", "example", "", 0.231, ""),
	}

	// All fuzz seeds pass, so the fuzz test and package pass as well
	expectedActions := map[string]string{
		"example/FuzzTestWithCorpus/seed#0": "pass",
		"example/FuzzTestWithCorpus/seed#1": "pass",
		"example/FuzzTestWithCorpus/seed#2": "pass",
		"example/FuzzTestWithCorpus":        "pass",
		"example/":                          "pass",
	}

	// transformAndVerify should yield exit code 0 since everything passes
	transformAndVerify(t, events, opts, expectedActions)
}
