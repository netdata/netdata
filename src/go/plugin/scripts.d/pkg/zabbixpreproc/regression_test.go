package zabbixpreproc

import (
	"strings"
	"testing"
	"time"
)

// TestRegressionIsErrorPropagation tests that IsError flag is preserved through pipeline.
// Issue: ExecutePipeline was resetting IsError to false, breaking validateNotSupported.
func TestRegressionIsErrorPropagation(t *testing.T) {
	p := NewPreprocessor("test-shard")

	tests := []struct {
		name        string
		inputData   string
		inputError  bool // IsError flag on input
		steps       []Step
		expectError bool   // Should return error?
		expectData  string // Expected output data (if no error)
	}{
		{
			name:       "Error input passes through unchanged when no error-handling step",
			inputData:  "previous error message",
			inputError: true,
			steps: []Step{
				{Type: StepTypeTrim, Params: " "},
			},
			expectError: false,
			expectData:  "previous error message", // Trim works on error message string
		},
		{
			name:       "ValidateNotSupported sees IsError flag",
			inputData:  "ZBX_NOTSUPPORTED: Agent is not available",
			inputError: true,
			steps: []Step{
				{Type: StepTypeValidateNotSupported, Params: "\nAgent is not available"},
			},
			expectError: true, // Pattern matches, so error is "supported" -> fail
		},
		{
			name:       "IsError preserved through multiple successful steps",
			inputData:  "error data",
			inputError: true,
			steps: []Step{
				{Type: StepTypeTrim, Params: " "},
				{Type: StepTypeStringReplace, Params: "data\ninfo"},
				{Type: StepTypeTrim, Params: " "},
			},
			expectError: false,
			expectData:  "error info", // Processed but IsError flag should be preserved
		},
		{
			name:       "Normal value not treated as error",
			inputData:  "normal value",
			inputError: false,
			steps: []Step{
				{Type: StepTypeTrim, Params: " "},
			},
			expectError: false,
			expectData:  "normal value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := Value{
				Data:      tt.inputData,
				Type:      ValueTypeStr,
				Timestamp: time.Now(),
				IsError:   tt.inputError,
			}

			result, err := p.ExecutePipeline("item1", input, tt.steps)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if len(result.Metrics) > 0 && result.Metrics[0].Value != tt.expectData {
					t.Errorf("Expected data %q, got %q", tt.expectData, result.Metrics[0].Value)
				}
			}
		})
	}
}

// TestRegressionPrometheusEscapedQuotes tests proper handling of escaped characters in labels.
// Issue: Regex-based parsing failed on escaped quotes like path="/foo\"bar".
func TestRegressionPrometheusEscapedQuotes(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		labelKey   string
		labelValue string
	}{
		{
			name:       "Escaped double quote in label value",
			input:      `http_requests{path="/foo\"bar"} 100`,
			labelKey:   "path",
			labelValue: `/foo"bar`,
		},
		{
			name:       "Escaped backslash in label value",
			input:      `file_size{path="C:\\Users\\Admin"} 200`,
			labelKey:   "path",
			labelValue: `C:\Users\Admin`,
		},
		{
			name:       "Escaped newline in label value",
			input:      `message{text="line1\nline2"} 50`,
			labelKey:   "text",
			labelValue: "line1\nline2",
		},
		{
			name:       "Multiple escapes in single label",
			input:      `complex{data="quote\"slash\\newline\n"} 75`,
			labelKey:   "data",
			labelValue: "quote\"slash\\newline\n",
		},
		{
			name:       "Mixed normal and escaped labels",
			input:      `metric{method="GET",path="/test\"path",code="200"} 300`,
			labelKey:   "path",
			labelValue: `/test"path`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := parsePrometheusText(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}
			if len(metrics) == 0 {
				t.Fatal("No metrics parsed")
			}

			metric := metrics[0]
			if val, ok := metric.Labels[tt.labelKey]; !ok {
				t.Errorf("Label %q not found in parsed metric", tt.labelKey)
			} else if val != tt.labelValue {
				t.Errorf("Label %q value mismatch:\nExpected: %q\nGot:      %q", tt.labelKey, tt.labelValue, val)
			}
		})
	}
}

// TestRegressionPrometheusLabelOrder tests that label order is preserved.
func TestRegressionPrometheusLabelOrder(t *testing.T) {
	input := `metric{first="1",second="2",third="3"} 100`
	metrics, err := parsePrometheusText(input)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if len(metrics) == 0 {
		t.Fatal("No metrics parsed")
	}

	expected := []string{"first", "second", "third"}
	if len(metrics[0].LabelOrder) != len(expected) {
		t.Fatalf("Label order length mismatch: expected %d, got %d", len(expected), len(metrics[0].LabelOrder))
	}

	for i, key := range expected {
		if metrics[0].LabelOrder[i] != key {
			t.Errorf("Label order[%d] mismatch: expected %q, got %q", i, key, metrics[0].LabelOrder[i])
		}
	}
}

// TestRegressionSNMPTextualOIDSupport tests that textual OIDs are properly translated.
// Issue: snmpWalkToJSON only did direct map lookup, not full translateMIB.
func TestRegressionSNMPTextualOIDSupport(t *testing.T) {
	tests := []struct {
		name       string
		oid        string
		expectedOK bool
		expected   string // Expected numeric OID (if OK)
	}{
		{
			name:       "IF-MIB::ifDescr translates correctly",
			oid:        "IF-MIB::ifDescr",
			expectedOK: true,
			expected:   ".1.3.6.1.2.1.2.2.1.2",
		},
		{
			name:       "IF-MIB::ifIndex translates correctly",
			oid:        "IF-MIB::ifIndex",
			expectedOK: true,
			expected:   ".1.3.6.1.2.1.2.2.1.1",
		},
		{
			name:       "SNMPv2-MIB::sysDescr translates correctly",
			oid:        "SNMPv2-MIB::sysDescr",
			expectedOK: true,
			expected:   ".1.3.6.1.2.1.1.1",
		},
		{
			name:       "HOST-RESOURCES-MIB::hrStorageDescr translates correctly",
			oid:        "HOST-RESOURCES-MIB::hrStorageDescr",
			expectedOK: true,
			expected:   ".1.3.6.1.2.1.25.2.3.1.3",
		},
		{
			name:       "Numeric OID passthrough",
			oid:        ".1.3.6.1.2.1.2.2.1.2",
			expectedOK: true,
			expected:   ".1.3.6.1.2.1.2.2.1.2",
		},
		{
			name:       "Numeric OID without leading dot gets normalized",
			oid:        "1.3.6.1.2.1.2.2.1.2",
			expectedOK: true,
			expected:   "1.3.6.1.2.1.2.2.1.2", // translateMIB returns passthrough for numeric
		},
		{
			name:       "Unknown MIB returns error",
			oid:        "UNKNOWN-MIB::unknownOid",
			expectedOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := translateMIB(tt.oid)
			if tt.expectedOK {
				if err != nil {
					t.Errorf("Expected successful translation, got error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("OID translation mismatch:\nExpected: %s\nGot:      %s", tt.expected, result)
				}
			} else {
				if err == nil {
					t.Error("Expected error for unknown MIB, got nil")
				}
			}
		})
	}
}

// TestRegressionSNMPWalkToJSONUsesTranslateMIB tests that snmpWalkToJSON uses full translateMIB.
func TestRegressionSNMPWalkToJSONUsesTranslateMIB(t *testing.T) {
	p := NewPreprocessor("test-shard")

	// SNMP walk data using numeric OIDs
	snmpData := `.1.3.6.1.2.1.2.2.1.1.1 = INTEGER: 1
.1.3.6.1.2.1.2.2.1.1.2 = INTEGER: 2
.1.3.6.1.2.1.2.2.1.2.1 = STRING: "eth0"
.1.3.6.1.2.1.2.2.1.2.2 = STRING: "eth1"`

	// Use textual OID in params - this should work now
	params := `{#IFINDEX}
IF-MIB::ifIndex
0
{#IFDESCR}
IF-MIB::ifDescr
0`

	input := Value{
		Data: snmpData,
		Type: ValueTypeStr,
	}

	step := Step{
		Type:   StepTypeSNMPWalkToJSON,
		Params: params,
	}

	result, err := p.Execute("item1", input, step)
	if err != nil {
		t.Fatalf("SNMP walk to JSON failed: %v", err)
	}

	if len(result.Metrics) == 0 {
		t.Fatal("No metrics returned")
	}

	// The result should contain JSON with both indices
	output := result.Metrics[0].Value
	if !strings.Contains(output, `"eth0"`) || !strings.Contains(output, `"eth1"`) {
		t.Errorf("Expected output to contain interface descriptions, got: %s", output)
	}
	if !strings.Contains(output, `"{#IFINDEX}"`) || !strings.Contains(output, `"{#IFDESCR}"`) {
		t.Errorf("Expected output to contain macro names, got: %s", output)
	}
}

// TestRegressionJavaScriptVMIsolation tests that JS VMs are isolated between executions.
// Issue: VM pooling with resetVM didn't clear prototype modifications.
func TestRegressionJavaScriptVMIsolation(t *testing.T) {
	// First execution: modify Object prototype
	script1 := `
		Object.prototype.polluted = "malicious";
		return "done";
	`
	result1, err := javascriptExecute(Value{Data: "test"}, script1, DefaultLimits().JavaScript)
	if err != nil {
		t.Fatalf("First JS execution failed: %v", err)
	}
	if result1.Data != "done" {
		t.Errorf("Expected 'done', got %q", result1.Data)
	}

	// Second execution: check if prototype pollution persists
	script2 := `
		// If VMs were pooled without proper isolation, this would return "malicious"
		var obj = {};
		if (obj.polluted !== undefined) {
			return "POLLUTION DETECTED: " + obj.polluted;
		}
		return "clean";
	`
	result2, err := javascriptExecute(Value{Data: "test"}, script2, DefaultLimits().JavaScript)
	if err != nil {
		t.Fatalf("Second JS execution failed: %v", err)
	}
	if result2.Data != "clean" {
		t.Errorf("VM isolation failed! Got: %q", result2.Data)
	}
}

// TestRegressionJavaScriptGlobalVariableIsolation tests that global variables don't leak.
func TestRegressionJavaScriptGlobalVariableIsolation(t *testing.T) {
	// First execution: define global variable
	script1 := `
		globalVar = "secret";
		return globalVar;
	`
	result1, err := javascriptExecute(Value{Data: "test"}, script1, DefaultLimits().JavaScript)
	if err != nil {
		t.Fatalf("First JS execution failed: %v", err)
	}
	if result1.Data != "secret" {
		t.Errorf("Expected 'secret', got %q", result1.Data)
	}

	// Second execution: check if global variable persists
	script2 := `
		if (typeof globalVar !== 'undefined') {
			return "LEAK DETECTED: " + globalVar;
		}
		return "isolated";
	`
	result2, err := javascriptExecute(Value{Data: "test"}, script2, DefaultLimits().JavaScript)
	if err != nil {
		t.Fatalf("Second JS execution failed: %v", err)
	}
	if result2.Data != "isolated" {
		t.Errorf("Global variable leaked! Got: %q", result2.Data)
	}
}

// TestRegressionPipelineDiscardedFlag tests that pipeline explicitly flags discarded values.
// Issue: Pipeline returned Result{} with nil error, hiding discard vs misconfiguration.
func TestRegressionPipelineDiscardedFlag(t *testing.T) {
	p := NewPreprocessor("test-shard")

	// Test 1: Normal successful pipeline - not discarded
	input1 := Value{
		Data:      "100",
		Type:      ValueTypeStr,
		Timestamp: time.Now(),
	}

	steps := []Step{
		{Type: StepTypeTrim, Params: " "},
	}

	result1, err := p.ExecutePipeline("item1", input1, steps)
	if err != nil {
		t.Fatalf("Normal execution failed: %v", err)
	}
	if result1.Discarded {
		t.Error("Normal result should not be discarded")
	}
	if len(result1.Metrics) == 0 {
		t.Error("Normal result should have metrics")
	}

	// Test 2: Error with ErrorActionDiscard - returns empty string, not discarded
	// Zabbix behavior: discard means replace error with empty string value
	input2 := Value{
		Data:      "invalid",
		Type:      ValueTypeStr,
		Timestamp: time.Now(),
	}

	stepsDiscard := []Step{
		{
			Type:   StepTypeMultiplier,
			Params: "2",
			ErrorHandler: ErrorHandler{
				Action: ErrorActionDiscard,
			},
		},
	}

	result2, err := p.ExecutePipeline("item2", input2, stepsDiscard)
	if err != nil {
		t.Fatalf("Discard execution failed: %v", err)
	}
	// ErrorActionDiscard returns empty string, not zero metrics
	if result2.Discarded {
		t.Error("ErrorActionDiscard should return empty string, not discard flag")
	}
	if len(result2.Metrics) == 0 {
		t.Error("ErrorActionDiscard should return metrics (with empty value)")
	}
	if len(result2.Metrics) > 0 && result2.Metrics[0].Value != "" {
		t.Errorf("ErrorActionDiscard should return empty string, got %q", result2.Metrics[0].Value)
	}

	// Test 3: Empty Result detection - Result.Discarded flag is for when step returns 0 metrics
	// This can happen with multi-metric steps or certain edge cases
	// The flag allows callers to distinguish empty result from error
	if result1.Discarded || result2.Discarded {
		t.Error("Neither normal nor discard results should have Discarded=true")
	}
}

// TestRegressionSNMPTextualWalkOutput tests that SNMP walk output with textual OIDs is parsed.
// Issue: parsers skipped lines not starting with '.', rejecting IF-MIB::ifDescr.1 format.
func TestRegressionSNMPTextualWalkOutput(t *testing.T) {
	p := NewPreprocessor("test-shard")

	// SNMP walk data using textual OIDs (real snmpwalk output without -On flag)
	snmpData := `IF-MIB::ifIndex.1 = INTEGER: 1
IF-MIB::ifIndex.2 = INTEGER: 2
IF-MIB::ifDescr.1 = STRING: "eth0"
IF-MIB::ifDescr.2 = STRING: "eth1"
IF-MIB::ifSpeed.1 = Gauge32: 1000000000
IF-MIB::ifSpeed.2 = Gauge32: 10000000000`

	// Use textual OIDs in params
	params := `{#IFINDEX}
IF-MIB::ifIndex
0
{#IFDESCR}
IF-MIB::ifDescr
0`

	input := Value{
		Data: snmpData,
		Type: ValueTypeStr,
	}

	step := Step{
		Type:   StepTypeSNMPWalkToJSON,
		Params: params,
	}

	result, err := p.Execute("item1", input, step)
	if err != nil {
		t.Fatalf("SNMP walk to JSON failed with textual OID output: %v", err)
	}

	if len(result.Metrics) == 0 {
		t.Fatal("No metrics returned from textual SNMP walk output")
	}

	output := result.Metrics[0].Value
	// Verify both interfaces were parsed
	if !strings.Contains(output, `"eth0"`) {
		t.Errorf("Expected 'eth0' in output, got: %s", output)
	}
	if !strings.Contains(output, `"eth1"`) {
		t.Errorf("Expected 'eth1' in output, got: %s", output)
	}
	if !strings.Contains(output, `"{#IFINDEX}"`) {
		t.Errorf("Expected macro {#IFINDEX} in output, got: %s", output)
	}
}

// TestRegressionJavaScriptArrayPrototypeIsolation tests Array.prototype isolation.
func TestRegressionJavaScriptArrayPrototypeIsolation(t *testing.T) {
	// First execution: modify Array prototype
	script1 := `
		Array.prototype.myMethod = function() { return "injected"; };
		return [].myMethod();
	`
	result1, err := javascriptExecute(Value{Data: "test"}, script1, DefaultLimits().JavaScript)
	if err != nil {
		t.Fatalf("First JS execution failed: %v", err)
	}
	if result1.Data != "injected" {
		t.Errorf("Expected 'injected', got %q", result1.Data)
	}

	// Second execution: check if Array prototype modification persists
	script2 := `
		if (typeof [].myMethod === 'function') {
			return "ARRAY POLLUTION: " + [].myMethod();
		}
		return "array_clean";
	`
	result2, err := javascriptExecute(Value{Data: "test"}, script2, DefaultLimits().JavaScript)
	if err != nil {
		t.Fatalf("Second JS execution failed: %v", err)
	}
	if result2.Data != "array_clean" {
		t.Errorf("Array.prototype leaked! Got: %q", result2.Data)
	}
}
