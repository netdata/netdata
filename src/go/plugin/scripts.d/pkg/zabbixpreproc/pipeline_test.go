package zabbixpreproc

import (
	"strings"
	"testing"
	"time"
)

// TestPipeline_BasicChaining tests that values flow correctly between steps
func TestPipeline_BasicChaining(t *testing.T) {
	p := NewPreprocessor("test-shard")

	tests := []struct {
		name     string
		input    string
		steps    []Step
		expected string
	}{
		{
			name:  "JSONPath → Multiplier → Range",
			input: `{"cpu": 0.45}`,
			steps: []Step{
				{Type: StepTypeJSONPath, Params: "$.cpu"},       // Extract 0.45
				{Type: StepTypeMultiplier, Params: "100"},       // Multiply by 100 → 45
				{Type: StepTypeValidateRange, Params: "0\n100"}, // Validate 0-100
			},
			expected: "45",
		},
		{
			name:  "Trim → Regex → Multiplier",
			input: "  Temperature: 25.5 C  ",
			steps: []Step{
				{Type: StepTypeTrim, Params: " "},                                             // Remove spaces
				{Type: StepTypeRegexSubstitution, Params: "Temperature:\\s+([0-9.]+).*\n\\1"}, // Extract number
				{Type: StepTypeMultiplier, Params: "1.8"},                                     // Celsius to Fahrenheit delta
			},
			expected: "45.9",
		},
		{
			name:  "XPath → Trim → Hex2Dec",
			input: `<data><value>  0xFF  </value></data>`,
			steps: []Step{
				{Type: StepTypeXPath, Params: "//value/text()"}, // Extract "  0xFF  "
				{Type: StepTypeTrim, Params: " "},               // Remove spaces → "0xFF"
				{Type: StepTypeHex2Dec},                         // Convert to decimal
			},
			expected: "255",
		},
		{
			name:  "Prometheus → Multiplier → Validate",
			input: "http_requests{method=\"GET\"} 100\nhttp_requests{method=\"POST\"} 50",
			steps: []Step{
				{Type: StepTypePrometheusPattern, Params: `http_requests{method="GET"}`}, // Extract 100
				{Type: StepTypeMultiplier, Params: "0.01"},                               // Convert to percentage
				{Type: StepTypeValidateRange, Params: "0\n10"},                           // Must be 0-10%
			},
			expected: "1",
		},
		{
			name:  "String Replace → Trim → Bool2Dec",
			input: "Status: TRUE ",
			steps: []Step{
				{Type: StepTypeStringReplace, Params: "Status: \n"}, // Remove prefix
				{Type: StepTypeTrim, Params: " "},                   // Remove spaces
				{Type: StepTypeBool2Dec},                            // TRUE → 1
			},
			expected: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{Data: tt.input, Type: ValueTypeStr}
			result, err := p.ExecutePipeline("test-item", value, tt.steps)

			if err != nil {
				t.Errorf("Pipeline failed: %v", err)
				return
			}

			if len(result.Metrics) == 0 {
				t.Error("Pipeline produced no metrics")
				return
			}

			got := result.Metrics[0].Value
			if got != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, got)
			}
		})
	}
}

// TestPipeline_ErrorPropagation tests that errors stop pipeline execution
func TestPipeline_ErrorPropagation(t *testing.T) {
	p := NewPreprocessor("test-shard")

	tests := []struct {
		name        string
		input       string
		steps       []Step
		expectError bool
		errorMatch  string
	}{
		{
			name:  "Invalid JSONPath stops pipeline",
			input: `{"value": 100}`,
			steps: []Step{
				{Type: StepTypeJSONPath, Params: "$.nonexistent"}, // Error: path not found
				{Type: StepTypeMultiplier, Params: "2"},           // Should NOT execute
			},
			expectError: true,
			errorMatch:  "step 0",
		},
		{
			name:  "Invalid multiplier stops pipeline",
			input: "abc",
			steps: []Step{
				{Type: StepTypeMultiplier, Params: "10"},        // Error: not a number
				{Type: StepTypeValidateRange, Params: "0\n100"}, // Should NOT execute
			},
			expectError: true,
			errorMatch:  "step 0",
		},
		{
			name:  "Failed validation stops pipeline",
			input: "150",
			steps: []Step{
				{Type: StepTypeValidateRange, Params: "0\n100"}, // Error: out of range
				{Type: StepTypeMultiplier, Params: "2"},         // Should NOT execute
			},
			expectError: true,
			errorMatch:  "step 0",
		},
		{
			name:  "Invalid regex stops pipeline",
			input: "test",
			steps: []Step{
				{Type: StepTypeRegexSubstitution, Params: "([0-9]+\n\\1"}, // Error: unclosed group
				{Type: StepTypeTrim, Params: " "},                         // Should NOT execute
			},
			expectError: true,
			errorMatch:  "step 0",
		},
		{
			name:  "Error in middle step stops pipeline",
			input: `{"value": "abc"}`,
			steps: []Step{
				{Type: StepTypeJSONPath, Params: "$.value"}, // Extract "abc"
				{Type: StepTypeMultiplier, Params: "10"},    // Error: not a number
				{Type: StepTypeTrim, Params: " "},           // Should NOT execute
			},
			expectError: true,
			errorMatch:  "step 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{Data: tt.input, Type: ValueTypeStr}
			result, err := p.ExecutePipeline("test-item", value, tt.steps)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.errorMatch) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorMatch, err)
				}
				if result.Error == nil {
					t.Error("Expected Result.Error to be set")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestPipeline_ErrorHandlers tests error handler behavior in pipelines
func TestPipeline_ErrorHandlers(t *testing.T) {
	p := NewPreprocessor("test-shard")

	tests := []struct {
		name     string
		input    string
		steps    []Step
		expected string
	}{
		{
			name:  "DISCARD error allows pipeline to continue with empty value",
			input: `{"value": 100}`,
			steps: []Step{
				{
					Type:   StepTypeJSONPath,
					Params: "$.nonexistent",
					ErrorHandler: ErrorHandler{
						Action: ErrorActionDiscard, // Return empty string on error
					},
				},
				{Type: StepTypeTrim, Params: " "}, // Processes empty string → empty string
			},
			expected: "",
		},
		{
			name:  "SET_VALUE error allows pipeline to continue with fallback",
			input: "not-a-number",
			steps: []Step{
				{
					Type:   StepTypeMultiplier,
					Params: "10",
					ErrorHandler: ErrorHandler{
						Action: ErrorActionSetValue,
						Params: "0", // Fallback value
					},
				},
				{Type: StepTypeMultiplier, Params: "2"}, // Processes "0" → "0"
			},
			expected: "0",
		},
		{
			name:  "Multiple error handlers in sequence",
			input: `{"data": "invalid"}`,
			steps: []Step{
				{
					Type:   StepTypeJSONPath,
					Params: "$.missing",
					ErrorHandler: ErrorHandler{
						Action: ErrorActionSetValue,
						Params: "100",
					},
				},
				{
					Type:   StepTypeMultiplier,
					Params: "abc", // Invalid multiplier
					ErrorHandler: ErrorHandler{
						Action: ErrorActionSetValue,
						Params: "50",
					},
				},
				{Type: StepTypeMultiplier, Params: "2"}, // Processes "50" → "100"
			},
			expected: "100",
		},
		{
			name:  "Error handler only catches specific step errors",
			input: "150",
			steps: []Step{
				{
					Type:   StepTypeValidateRange,
					Params: "0\n100",
					ErrorHandler: ErrorHandler{
						Action: ErrorActionSetValue,
						Params: "100", // Clamp to max
					},
				},
				{Type: StepTypeMultiplier, Params: "0.5"}, // Processes "100" → "50"
			},
			expected: "50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{Data: tt.input, Type: ValueTypeStr}
			result, err := p.ExecutePipeline("test-item", value, tt.steps)

			if err != nil {
				t.Errorf("Pipeline failed: %v", err)
				return
			}

			if len(result.Metrics) == 0 {
				t.Error("Pipeline produced no metrics")
				return
			}

			got := result.Metrics[0].Value
			if got != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, got)
			}
		})
	}
}

// TestPipeline_ErrorHandlerSetError tests that SET_ERROR stops pipeline
func TestPipeline_ErrorHandlerSetError(t *testing.T) {
	p := NewPreprocessor("test-shard")

	steps := []Step{
		{
			Type:   StepTypeMultiplier,
			Params: "abc", // Invalid
			ErrorHandler: ErrorHandler{
				Action: ErrorActionSetError,
				Params: "Custom error message",
			},
		},
		{Type: StepTypeMultiplier, Params: "2"}, // Should NOT execute
	}

	value := Value{Data: "100", Type: ValueTypeStr}
	result, err := p.ExecutePipeline("test-item", value, steps)

	if err == nil {
		t.Error("Expected error, got nil")
		return
	}

	if !strings.Contains(err.Error(), "Custom error message") {
		t.Errorf("Expected custom error message, got: %v", err)
	}

	if result.Error == nil {
		t.Error("Expected Result.Error to be set")
	}
}

// TestPipeline_StatefulOperations tests that stateful steps work in pipelines
func TestPipeline_StatefulOperations(t *testing.T) {
	p := NewPreprocessor("test-shard")

	steps := []Step{
		{Type: StepTypeJSONPath, Params: "$.counter"},
		{Type: StepTypeDeltaValue},
		{Type: StepTypeMultiplier, Params: "10"}, // Scale delta
	}

	// First call: baseline (delta = 0)
	value1 := Value{
		Data:      `{"counter": 100}`,
		Type:      ValueTypeStr,
		Timestamp: time.Now(),
	}
	result1, err := p.ExecutePipeline("item1", value1, steps)
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}
	if result1.Metrics[0].Value != "0" {
		t.Errorf("First delta should be 0, got %s", result1.Metrics[0].Value)
	}

	// Second call: delta = 50, scaled = 500
	value2 := Value{
		Data:      `{"counter": 150}`,
		Type:      ValueTypeStr,
		Timestamp: time.Now(),
	}
	result2, err := p.ExecutePipeline("item1", value2, steps)
	if err != nil {
		t.Fatalf("Second call failed: %v", err)
	}
	if result2.Metrics[0].Value != "500" {
		t.Errorf("Expected 500, got %s", result2.Metrics[0].Value)
	}
}

// TestPipeline_EmptyPipeline tests edge case of zero steps
func TestPipeline_EmptyPipeline(t *testing.T) {
	p := NewPreprocessor("test-shard")

	value := Value{Data: "test", Type: ValueTypeStr}
	result, err := p.ExecutePipeline("test-item", value, []Step{})

	if err != nil {
		t.Errorf("Empty pipeline should succeed, got error: %v", err)
	}

	if len(result.Metrics) == 0 {
		t.Error("Empty pipeline should return input as metric")
		return
	}

	if result.Metrics[0].Value != "test" {
		t.Errorf("Expected input unchanged, got %q", result.Metrics[0].Value)
	}
}

// TestPipeline_SingleStep tests edge case of single step
func TestPipeline_SingleStep(t *testing.T) {
	p := NewPreprocessor("test-shard")

	steps := []Step{
		{Type: StepTypeMultiplier, Params: "10"},
	}

	value := Value{Data: "5", Type: ValueTypeFloat}
	result, err := p.ExecutePipeline("test-item", value, steps)

	if err != nil {
		t.Errorf("Single step pipeline failed: %v", err)
	}

	if len(result.Metrics) == 0 {
		t.Error("Pipeline produced no metrics")
		return
	}

	if result.Metrics[0].Value != "50" {
		t.Errorf("Expected 50, got %s", result.Metrics[0].Value)
	}
}

// TestPipeline_ValueTypePreservation tests that type flows through pipeline
func TestPipeline_ValueTypePreservation(t *testing.T) {
	p := NewPreprocessor("test-shard")

	tests := []struct {
		name         string
		inputType    ValueType
		steps        []Step
		expectedType ValueType
	}{
		{
			name:      "Float through multiplier stays float",
			inputType: ValueTypeFloat,
			steps: []Step{
				{Type: StepTypeMultiplier, Params: "2"},
			},
			expectedType: ValueTypeFloat,
		},
		{
			name:      "String through trim stays string",
			inputType: ValueTypeStr,
			steps: []Step{
				{Type: StepTypeTrim, Params: " "},
			},
			expectedType: ValueTypeStr,
		},
		{
			name:      "String through regex substitution stays string",
			inputType: ValueTypeStr,
			steps: []Step{
				{Type: StepTypeRegexSubstitution, Params: "([0-9]+)\n\\1"},
			},
			expectedType: ValueTypeStr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{
				Data:      "100",
				Type:      tt.inputType,
				Timestamp: time.Now(),
			}

			result, err := p.ExecutePipeline("test-item", value, tt.steps)
			if err != nil {
				t.Errorf("Pipeline failed: %v", err)
				return
			}

			if len(result.Metrics) == 0 {
				t.Error("Pipeline produced no metrics")
				return
			}

			if result.Metrics[0].Type != tt.expectedType {
				t.Errorf("Expected type %v, got %v", tt.expectedType, result.Metrics[0].Type)
			}
		})
	}
}
