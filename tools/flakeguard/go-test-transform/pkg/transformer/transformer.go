package transformer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// TestEvent represents a single event from go test -json
type TestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test,omitempty"`
	Elapsed float64   `json:"Elapsed,omitempty"`
	Output  string    `json:"Output,omitempty"`
}

// TestNode represents a node in the test tree
type TestNode struct {
	Name      string               // Test name
	Package   string               // Package name
	IsPackage bool                 // Is this a package node
	Failed    bool                 // Did this test fail
	Skipped   bool                 // Is this test skipped
	Ignored   bool                 // Should this failure be ignored
	Children  map[string]*TestNode // Child tests
	Parent    *TestNode            // Parent test
	IsSubtest bool                 // Is this a subtest
	// Store output messages for direct failure detection
	OutputMessages []string
	// Flag to track if this node has any failing subtests
	HasFailingSubtests bool
}

// TransformJSON transforms go test -json output according to the options
func TransformJSON(input io.Reader, output io.Writer, opts *Options) error {
	// Create scanner for JSON input
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max buffer

	// First pass: collect all events
	var events []TestEvent
	for scanner.Scan() {
		var event TestEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return fmt.Errorf("failed to parse JSON: %v", err)
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %v", err)
	}

	// Build test tree
	testTree := buildTestTree(events)

	// Track output messages for direct failure detection
	captureOutputMessages(events, testTree)

	// Mark parents with failing subtests
	markParentsWithFailingSubtests(testTree)

	// Identify which tests should be passed
	identifyTestsToIgnore(testTree, opts)

	// Transform events
	transformedEvents, _ := transformEvents(events, testTree)

	// Output transformed events
	for _, event := range transformedEvents {
		eventJSON, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %v", err)
		}
		fmt.Fprintln(output, string(eventJSON))
	}

	return nil
}

// buildTestTree builds a tree of tests from the events
func buildTestTree(events []TestEvent) map[string]*TestNode {
	tree := make(map[string]*TestNode)

	// First create nodes for all packages and tests
	for _, event := range events {
		if event.Package != "" {
			// Create package node if it doesn't exist
			if _, exists := tree[event.Package]; !exists {
				tree[event.Package] = &TestNode{
					Name:      event.Package,
					Package:   event.Package,
					IsPackage: true,
					Children:  make(map[string]*TestNode),
				}
			}

			// Create test node if applicable and it doesn't exist
			if event.Test != "" {
				packageNode := tree[event.Package]

				// Initialize current to packageNode to start traversal
				current := packageNode

				// Split test path into components
				components := strings.Split(event.Test, "/")

				// Build path component by component
				path := ""
				for i, component := range components {
					if i > 0 {
						path += "/"
					}
					path += component

					if _, exists := current.Children[component]; !exists {
						// Create new node
						newNode := &TestNode{
							Name:           path,
							Package:        event.Package,
							IsSubtest:      i > 0, // First level is not a subtest
							Children:       make(map[string]*TestNode),
							Parent:         current,
							OutputMessages: []string{},
						}
						current.Children[component] = newNode
					}

					// Move to next level
					current = current.Children[component]
				}
			}
		}
	}

	// Now mark failing tests
	for _, event := range events {
		if event.Action == "fail" {
			if event.Test == "" {
				// Package failure
				if node, exists := tree[event.Package]; exists {
					node.Failed = true
				}
			} else {
				// Test failure - need to find the node
				if packageNode, exists := tree[event.Package]; exists {
					// Traverse to find the test node
					current := packageNode
					components := strings.Split(event.Test, "/")

					found := true
					for i, component := range components {
						if child, exists := current.Children[component]; exists {
							current = child

							// If this is the last component, mark as failed
							if i == len(components)-1 {
								current.Failed = true
							}
						} else {
							found = false
							break
						}
					}

					if !found {
						fmt.Printf("Warning: Could not find node for test %s\n", event.Test)
					}
				}
			}
		}
	}

	return tree
}

// captureOutputMessages captures output messages for each test
func captureOutputMessages(events []TestEvent, tree map[string]*TestNode) {
	for _, event := range events {
		if event.Action == "output" && event.Output != "" {
			// Find the node
			var node *TestNode
			if event.Test == "" {
				node = tree[event.Package]
			} else {
				packageNode := tree[event.Package]
				if packageNode == nil {
					continue
				}

				components := strings.Split(event.Test, "/")
				current := packageNode

				found := true
				for i, component := range components {
					if child, exists := current.Children[component]; exists {
						current = child

						// If this is the last component, we found the node
						if i == len(components)-1 {
							node = current
						}
					} else {
						found = false
						break
					}
				}

				if !found {
					continue
				}
			}

			// Store the output message
			if node != nil {
				node.OutputMessages = append(node.OutputMessages, event.Output)
			}
		}
	}
}

// markParentsWithFailingSubtests marks parent nodes that have any failing subtests
func markParentsWithFailingSubtests(tree map[string]*TestNode) {
	// Process each package
	for _, pkgNode := range tree {
		// Bottom-up traversal to mark parents with failing subtests
		var markParents func(*TestNode) bool
		markParents = func(node *TestNode) bool {
			// Check if any child is failing or has failing subtests
			hasFailingSubtests := false

			for _, child := range node.Children {
				// Process child first (depth-first)
				childHasFailingSubtests := markParents(child)

				// Check if child is failing or has failing subtests
				if child.Failed || childHasFailingSubtests {
					hasFailingSubtests = true
				}
			}

			// Update the node's status
			node.HasFailingSubtests = hasFailingSubtests

			// Return whether this node has failing subtests
			return hasFailingSubtests
		}

		markParents(pkgNode)
	}
}

// identifyTestsToIgnore identifies which tests should be converted to pass
func identifyTestsToIgnore(tree map[string]*TestNode, opts *Options) {
	// Process each package
	for _, pkgNode := range tree {
		// Process all nodes in the tree and set ignore status
		var processNode func(*TestNode)
		processNode = func(node *TestNode) {
			// Process children first
			for _, child := range node.Children {
				processNode(child)
			}

			// If this node has failing subtests and it's failing itself,
			// mark it as ignored (so it will be converted to PASS)
			if node.HasFailingSubtests && node.Failed {
				node.Ignored = true
			}
		}

		processNode(pkgNode)
	}
}

// transformEvents applies transformations to events based on the test tree
func transformEvents(events []TestEvent, tree map[string]*TestNode) ([]TestEvent, bool) {
	transformedEvents := make([]TestEvent, len(events))
	anyRemainingFailures := false

	// Helper function to find a node
	findNode := func(pkg, test string) *TestNode {
		if test == "" {
			// This is a package event
			return tree[pkg]
		}

		// Find the node for this test
		packageNode := tree[pkg]
		if packageNode == nil {
			return nil
		}

		if test == "" {
			return packageNode
		}

		// Traverse to find the test node
		current := packageNode
		components := strings.Split(test, "/")

		for i, component := range components {
			if child, exists := current.Children[component]; exists {
				current = child
				// If this is the last component, we found the node
				if i == len(components)-1 {
					return current
				}
			} else {
				return nil
			}
		}

		return nil
	}

	for i, event := range events {
		// Make a copy of the event
		transformedEvents[i] = event

		// Check if we need to transform this event
		if event.Action == "fail" {
			node := findNode(event.Package, event.Test)
			if node != nil && node.Ignored {
				// Convert this fail to a pass
				transformedEvents[i].Action = "pass"
			} else {
				// We're keeping this as a failure
				anyRemainingFailures = true
			}
		} else if event.Action == "skip" {
			// Preserve skip events without transformation
			// This ensures skipped tests remain in the output
		} else if event.Action == "output" {
			node := findNode(event.Package, event.Test)
			if node != nil && node.Failed && node.Ignored {
				// Transform output text for passed tests
				transformedEvents[i].Output = transformOutputText(event.Output)
			}
		}
	}

	return transformedEvents, anyRemainingFailures
}

// transformOutputText transforms failure text in output to success text
func transformOutputText(output string) string {
	// Replace === FAIL with === PASS
	output = strings.Replace(output, "=== FAIL", "=== PASS", -1)

	// Replace --- FAIL with --- PASS
	output = strings.Replace(output, "--- FAIL", "--- PASS", -1)

	// Handle standalone FAIL appropriately
	if output == "FAIL\n" {
		return "PASS\n"
	}

	// Replace other forms of FAIL
	output = strings.Replace(output, "\nFAIL\n", "\nPASS\n", -1)
	output = strings.Replace(output, "\nFAIL ", "\nPASS ", -1)
	output = strings.Replace(output, " FAIL\n", " PASS\n", -1)

	return output
}
