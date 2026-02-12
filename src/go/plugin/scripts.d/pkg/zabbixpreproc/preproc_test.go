package zabbixpreproc

import (
	"fmt"
	"testing"
)

func TestZabbixTestSuite(t *testing.T) {
	// Load ALL official Zabbix test suites
	testCases, err := LoadAllTestCases()
	if err != nil {
		t.Fatalf("Failed to load test cases: %v", err)
	}

	passed := 0
	failed := 0
	skipped := 0

	for _, tc := range testCases {
		// Capture tc in closure for t.Run
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			// Create a fresh preprocessor for each test to isolate state
			preprocessor := NewPreprocessor("test-shard")
			// Parse test input
			value, err := ValueFromTestInput(tc)
			if err != nil {
				t.Skipf("Failed to parse input: %v", err)
				skipped++
				return
			}

			step, err := StepFromTestInput(tc)
			if err != nil {
				t.Skipf("Failed to parse step: %v", err)
				skipped++
				return
			}

			// Load history value if provided (for stateful operations like delta, throttle)
			historyVal := HistoryValueFromTestInput(tc)
			itemID := "test-item" // Use consistent itemID for tests
			if historyVal != nil {
				// Pre-populate state through public API (no direct state manipulation)
				if err := preprocessor.PreloadState(itemID, step.Type, historyVal.Data, historyVal.Timestamp); err != nil {
					// Not a stateful operation, ignore
					t.Logf("Note: history value provided but step type %d is not stateful", step.Type)
				}
			}

			// Execute preprocessing with itemID
			result, err := preprocessor.Execute(itemID, value, step)

			// Check expected outcome
			if tc.Out.Return == "SUCCEED" {
				if err != nil {
					t.Errorf("Expected success but got error: %v", err)
					failed++
					return
				}

				// Extract result value from first metric
				if len(result.Metrics) == 0 {
					t.Errorf("No metrics returned")
					failed++
					return
				}
				resultData := result.Metrics[0].Value

				expected, _ := ExpectedOutputFromTestCase(tc)
				if resultData != expected {
					// Debug: print input details for trim tests
					if step.Type == StepTypeRTrim || step.Type == StepTypeLTrim || step.Type == StepTypeTrim {
						paramsDesc := ""
						for i, ch := range step.Params {
							if i > 0 {
								paramsDesc += ", "
							}
							paramsDesc += fmt.Sprintf("%c(0x%x)", ch, ch)
						}
						t.Logf("DEBUG: input='%s' params='%s'[%s] (len=%d) expected='%s' (len=%d) got='%s' (len=%d)", value.Data, step.Params, paramsDesc, len(step.Params), expected, len(expected), resultData, len(resultData))
					}
					t.Errorf("Expected '%s' but got '%s'", expected, resultData)
					failed++
					return
				}

				passed++
			} else { // FAIL expected
				if err == nil {
					resultData := ""
					if len(result.Metrics) > 0 {
						resultData = result.Metrics[0].Value
					}
					t.Errorf("Expected error but succeeded with '%s'", resultData)
					failed++
					return
				}

				passed++
			}
		})
	}

	fmt.Printf("\n=== Test Results ===\n")
	fmt.Printf("Passed:  %d\n", passed)
	fmt.Printf("Failed:  %d\n", failed)
	fmt.Printf("Skipped: %d\n", skipped)
	fmt.Printf("Total:   %d\n", passed+failed+skipped)
	if passed+failed > 0 {
		fmt.Printf("Pass Rate: %.1f%%\n", float64(passed)*100/float64(passed+failed))
	}

	if failed > 0 {
		t.Fatalf("%d tests failed", failed)
	}
}
