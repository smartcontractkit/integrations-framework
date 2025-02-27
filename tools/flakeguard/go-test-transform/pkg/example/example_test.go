package example

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func init() {
	// Seed random number generator
	rand.Seed(time.Now().UnixNano())
}

// Simple function to simulate a flaky operation
func simulateFlaky() bool {
	// Returns true 50% of the time, false 50% of the time
	return rand.Intn(2) == 0
}

// Simple function to potentially timeout
func simulateTimeout() bool {
	// Returns true 30% of the time (signaling a timeout), false 70% of the time
	return rand.Intn(10) < 3
}

// Regular tests that always pass
func TestAlwaysPass(t *testing.T) {
	t.Run("FirstSubtest", func(t *testing.T) {
		// This test always passes
	})

	t.Run("SecondSubtest", func(t *testing.T) {
		// This test always passes too
	})
}

// Tests with predictable failures
func TestPredictableFailure(t *testing.T) {
	t.Run("PassingSubtest", func(t *testing.T) {
		// This subtest passes
	})

	t.Run("FailingSubtest", func(t *testing.T) {
		// This subtest always fails
		t.Error("This is a predictable failure")
	})
}

// Flaky tests that sometimes fail
func TestFlakyTests(t *testing.T) {
	t.Run("FlakyTest1", func(t *testing.T) {
		if !simulateFlaky() {
			t.Error("This test failed due to flakiness")
		}
	})

	t.Run("FlakyTest2", func(t *testing.T) {
		if !simulateFlaky() {
			t.Error("Another flaky test failure")
		}
	})
}

// Test that sometimes times out
func TestTimeoutBehavior(t *testing.T) {
	t.Run("MightTimeout", func(t *testing.T) {
		t.Parallel()

		if simulateTimeout() {
			// Simulate a timeout by just failing
			t.Error("Test timed out")
			return
		}

		// Test passes otherwise
	})
}

// Error with specific message pattern
func TestSpecificErrorMessages(t *testing.T) {
	t.Run("ConnectionError", func(t *testing.T) {
		err := errors.New("connection timeout: couldn't connect to server")
		if err != nil {
			t.Errorf("Test failed with connection timeout: %v", err)
		}
	})

	t.Run("OtherError", func(t *testing.T) {
		err := errors.New("some other error occurred")
		if err != nil {
			t.Errorf("Test failed with error: %v", err)
		}
	})
}

// Nested tests to demonstrate hierarchy
func TestNestedStructure(t *testing.T) {
	t.Run("Level1", func(t *testing.T) {
		t.Run("Level2A", func(t *testing.T) {
			// This test passes
		})

		t.Run("Level2B", func(t *testing.T) {
			t.Run("Level3A_Flaky", func(t *testing.T) {
				t.Error("Level3A_Flaky test failed")
			})

			t.Run("Level3B", func(t *testing.T) {
				// This test passes
			})

			// This test will fail, causing Level2B to fail
			t.Error("Level2B has a direct failure")
		})
	})
}

// Test with many subtests to test large number handling
func TestManySubtests(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run(fmt.Sprintf("Subtest%d", i), func(t *testing.T) {
			if i%5 == 0 {
				// Make every 5th test fail
				t.Errorf("Test %d is meant to fail", i)
			}
		})
	}
}

// TestNestedWithLogs demonstrates a nested structure with logs in parent tests
func TestNestedWithLogs(t *testing.T) {
	t.Run("Level1", func(t *testing.T) {
		// Log in parent test - this should not cause a failure
		t.Log("This is just a log message in Level1, not an error")

		t.Run("Level2A", func(t *testing.T) {
			// This test passes
		})

		t.Run("Level2B", func(t *testing.T) {
			// Log in parent test - this should not cause a failure
			t.Log("This is just a log message in Level2B, not an error")

			t.Run("Level3A_Flaky", func(t *testing.T) {
				// This test sometimes fails
				t.Error("Level3A_Flaky test failed")
			})

			t.Run("Level3B", func(t *testing.T) {
				// This test passes
			})

			// No direct failure in Level2B, only logs
		})
	})
}

// TestSkippedTests demonstrates skipped tests
func TestSkippedTests(t *testing.T) {
	t.Run("SkippedTest", func(t *testing.T) {
		t.Skip("This test is skipped intentionally")
	})

	t.Run("ConditionallySkipped", func(t *testing.T) {
		if simulateFlaky() {
			t.Skip("This test is conditionally skipped")
		}
		// This test passes if not skipped
	})
}

// TestMixedResults demonstrates a test with mixed results (pass, fail, skip)
func TestMixedResults(t *testing.T) {
	t.Run("Passing", func(t *testing.T) {
		// This test passes
	})

	t.Run("Failing", func(t *testing.T) {
		t.Error("This test fails")
	})

	t.Run("Skipped", func(t *testing.T) {
		t.Skip("This test is skipped")
	})

	t.Run("Flaky", func(t *testing.T) {
		if !simulateFlaky() {
			t.Error("This test is flaky")
		}
	})
}

// TestDeepNesting demonstrates deeply nested tests
func TestDeepNesting(t *testing.T) {
	t.Run("Level1", func(t *testing.T) {
		t.Run("Level2", func(t *testing.T) {
			t.Run("Level3", func(t *testing.T) {
				t.Run("Level4", func(t *testing.T) {
					t.Run("Level5", func(t *testing.T) {
						t.Error("Deep nested test failed")
					})
				})
			})
		})
	})
}

// TestWithSetupTeardown demonstrates tests with setup and teardown
func TestWithSetupTeardown(t *testing.T) {
	// Setup
	t.Log("Setting up test resources")

	// Run subtests
	t.Run("TestWithSetup1", func(t *testing.T) {
		// This test passes
	})

	t.Run("TestWithSetup2", func(t *testing.T) {
		t.Error("This test fails despite setup")
	})

	// Teardown (always executed)
	t.Log("Tearing down test resources")
}

// TestWithHelperFunctions demonstrates using helper functions
func TestWithHelperFunctions(t *testing.T) {
	// Helper function
	assertCondition := func(t *testing.T, name string, condition bool) {
		t.Helper() // Mark as helper
		if !condition {
			t.Errorf("Assertion failed for %s", name)
		}
	}

	t.Run("HelperPass", func(t *testing.T) {
		assertCondition(t, "passing condition", true)
	})

	t.Run("HelperFail", func(t *testing.T) {
		assertCondition(t, "failing condition", false)
	})
}

// TestWithPanicRecovery demonstrates tests with panic recovery
func TestWithPanicRecovery(t *testing.T) {
	t.Run("Panicking", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Recovered from panic: %v", r)
				// Don't fail the test - we expected the panic
			}
		}()

		if simulateFlaky() {
			panic("simulated panic in test")
		}
	})

	t.Run("NotPanicking", func(t *testing.T) {
		// This test passes
	})
}

// TestWithCustomNames demonstrates tests with special characters in names
func TestWithCustomNames(t *testing.T) {
	specialNames := []string{
		"Test with spaces",
		"Test/with/slashes",
		"Test.with.dots",
		"Test-with-hyphens",
		"Test_with_underscores",
		"Test with (parentheses)",
		"Test with [brackets]",
		"Test with {braces}",
	}

	for _, name := range specialNames {
		t.Run(name, func(t *testing.T) {
			if simulateFlaky() {
				t.Errorf("Test %q failed", name)
			}
		})
	}
}

// TestWithConcurrency demonstrates concurrent test execution
func TestWithConcurrency(t *testing.T) {
	// Run 5 concurrent subtests
	for i := 0; i < 5; i++ {
		i := i // Capture loop variable
		t.Run(fmt.Sprintf("ConcurrentTest%d", i), func(t *testing.T) {
			t.Parallel() // Mark as parallel

			// Simulate variable-length test
			time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

			if i%2 == 0 && !simulateFlaky() {
				t.Errorf("Concurrent test %d failed", i)
			}
		})
	}
}

// TestWithTableDrivenTests demonstrates table-driven tests
func TestWithTableDrivenTests(t *testing.T) {
	tests := []struct {
		name       string
		input      int
		expected   bool
		shouldFail bool
	}{
		{"Zero", 0, true, false},
		{"Positive", 5, true, false},
		{"Negative", -5, false, false},
		{"FailingCase", 42, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input >= 0 // Simple check if input is non-negative

			if result != tt.expected || tt.shouldFail {
				t.Errorf("Test %s failed: got %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

// TestWithSubtestReuse demonstrates reusing the same subtest multiple times
func TestWithSubtestReuse(t *testing.T) {
	testFunc := func(t *testing.T, name string, shouldFail bool) {
		t.Run(name, func(t *testing.T) {
			if shouldFail {
				t.Errorf("Subtest %s intentionally failed", name)
			}
		})
	}

	// Run the same test function with different parameters
	testFunc(t, "First", false)
	testFunc(t, "Second", true)
	testFunc(t, "Third", false)
	testFunc(t, "Fourth", true)
}

// See what happens if you include slashes in the subtest name
func TestSubTestNameWithSlashes(t *testing.T) {
	t.Parallel()

	t.Run("sub/test/name/with/slashes", func(t *testing.T) {
		t.Log("This subtest always passes")
	})
}

// Account for fuzz tests with a corpus, so they run as normal unit tests
func FuzzTestWithCorpus(f *testing.F) {
	f.Add("some")
	f.Add("corpus")
	f.Add("values")

	f.Fuzz(func(t *testing.T, input string) {
		t.Logf("Fuzzing with input: %s", input)
	})
}
