package zabbixpreproc

import (
	"fmt"
	"testing"
	"time"
)

// TestPrometheusMultiMetric tests that prometheusToJSONMulti() returns multiple metrics
func TestPrometheusMultiMetric(t *testing.T) {
	input := `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET"} 100
http_requests_total{method="POST"} 50
memory_usage_bytes 1024
`

	result, err := prometheusToJSONMulti(Value{Data: input, Type: ValueTypeStr}, "")
	if err != nil {
		t.Fatalf("prometheusToJSONMulti() error: %v", err)
	}

	// Should return 3 metrics
	if len(result.Metrics) != 3 {
		t.Fatalf("Expected 3 metrics, got %d", len(result.Metrics))
	}

	// Verify first metric
	m0 := result.Metrics[0]
	if m0.Name != "http_requests_total" {
		t.Errorf("Metric 0: expected name 'http_requests_total', got %q", m0.Name)
	}
	if m0.Value != "100" {
		t.Errorf("Metric 0: expected value '100', got %q", m0.Value)
	}
	if m0.Type != ValueTypeStr {
		t.Errorf("Metric 0: expected type ValueTypeStr, got %v", m0.Type)
	}
	if m0.Labels == nil {
		t.Fatal("Metric 0: labels should not be nil")
	}
	if m0.Labels["method"] != "GET" {
		t.Errorf("Metric 0: expected label method=GET, got %q", m0.Labels["method"])
	}

	// Verify second metric
	m1 := result.Metrics[1]
	if m1.Name != "http_requests_total" {
		t.Errorf("Metric 1: expected name 'http_requests_total', got %q", m1.Name)
	}
	if m1.Value != "50" {
		t.Errorf("Metric 1: expected value '50', got %q", m1.Value)
	}
	if m1.Labels["method"] != "POST" {
		t.Errorf("Metric 1: expected label method=POST, got %q", m1.Labels["method"])
	}

	// Verify third metric (no labels)
	m2 := result.Metrics[2]
	if m2.Name != "memory_usage_bytes" {
		t.Errorf("Metric 2: expected name 'memory_usage_bytes', got %q", m2.Name)
	}
	if m2.Value != "1024" {
		t.Errorf("Metric 2: expected value '1024', got %q", m2.Value)
	}
	if len(m2.Labels) != 0 {
		t.Errorf("Metric 2: expected no labels, got %v", m2.Labels)
	}
}

// TestPrometheusMultiMetric_EmptyInput tests empty input handling
func TestPrometheusMultiMetric_EmptyInput(t *testing.T) {
	input := ""

	result, err := prometheusToJSONMulti(Value{Data: input, Type: ValueTypeStr}, "")
	if err != nil {
		t.Fatalf("prometheusToJSONMulti() error on empty input: %v", err)
	}

	// Empty input should return zero metrics
	if len(result.Metrics) != 0 {
		t.Errorf("Expected 0 metrics for empty input, got %d", len(result.Metrics))
	}
}

// TestPrometheusMultiMetric_InvalidInput tests error handling
func TestPrometheusMultiMetric_InvalidInput(t *testing.T) {
	input := "invalid prometheus data here"

	_, err := prometheusToJSONMulti(Value{Data: input, Type: ValueTypeStr}, "")
	if err == nil {
		t.Fatal("Expected error for invalid Prometheus data, got nil")
	}
}

// TestPrometheusMultiMetric_SingleMetric tests single metric returns single-element array
func TestPrometheusMultiMetric_SingleMetric(t *testing.T) {
	input := "cpu_usage 42.5"

	result, err := prometheusToJSONMulti(Value{Data: input, Type: ValueTypeStr}, "")
	if err != nil {
		t.Fatalf("prometheusToJSONMulti() error: %v", err)
	}

	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(result.Metrics))
	}

	m := result.Metrics[0]
	if m.Name != "cpu_usage" {
		t.Errorf("Expected name 'cpu_usage', got %q", m.Name)
	}
	if m.Value != "42.5" {
		t.Errorf("Expected value '42.5', got %q", m.Value)
	}
}

// TestPrometheusMultiMetric_ViaPreprocessor tests multi-metric through Preprocessor.Execute()
func TestPrometheusMultiMetric_ViaPreprocessor(t *testing.T) {
	input := `http_requests_total{method="GET"} 100
http_requests_total{method="POST"} 50
memory_usage_bytes 1024
`

	p := NewPreprocessor("test-shard")
	result, err := p.Execute("item1",
		Value{Data: input, Type: ValueTypeStr, Timestamp: time.Now()},
		Step{Type: StepTypePrometheusToJSONMulti, Params: ""})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Should return 3 metrics
	if len(result.Metrics) != 3 {
		t.Fatalf("Expected 3 metrics, got %d", len(result.Metrics))
	}

	// Verify metric names and values
	expected := []struct {
		name  string
		value string
	}{
		{"http_requests_total", "100"},
		{"http_requests_total", "50"},
		{"memory_usage_bytes", "1024"},
	}

	for i, exp := range expected {
		if result.Metrics[i].Name != exp.name {
			t.Errorf("Metric %d: expected name %q, got %q", i, exp.name, result.Metrics[i].Name)
		}
		if result.Metrics[i].Value != exp.value {
			t.Errorf("Metric %d: expected value %q, got %q", i, exp.value, result.Metrics[i].Value)
		}
	}
}

// TestZabbixCompatibility_PrometheusToJSON tests that old step type still works
func TestZabbixCompatibility_PrometheusToJSON(t *testing.T) {
	input := `http_requests_total{method="GET"} 100
http_requests_total{method="POST"} 50
`

	p := NewPreprocessor("test-shard")
	result, err := p.Execute("item1",
		Value{Data: input, Type: ValueTypeStr, Timestamp: time.Now()},
		Step{Type: StepTypePrometheusToJSON, Params: ""}) // OLD step type

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// OLD behavior: Should return SINGLE metric with JSON array string
	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric (Zabbix compatibility), got %d", len(result.Metrics))
	}

	// Value should be a JSON array string
	value := result.Metrics[0].Value
	if value[0] != '[' || value[len(value)-1] != ']' {
		t.Errorf("Expected JSON array string, got: %s", value)
	}

	// Verify it contains both metrics in the JSON array
	if !contains(value, `"name":"http_requests_total"`) {
		t.Errorf("JSON array missing http_requests_total metric")
	}
	if !contains(value, `"method":"GET"`) || !contains(value, `"method":"POST"`) {
		t.Errorf("JSON array missing expected labels")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// JSONPath Multi-Metric Tests
// ============================================================================

// TestJSONPathMulti_Array tests JSONPath with array result
func TestJSONPathMulti_Array(t *testing.T) {
	input := `{"items": [{"name": "a", "value": 10}, {"name": "b", "value": 20}]}`
	jsonpath := "$.items[*]"

	result, err := jsonPathMulti(Value{Data: input, Type: ValueTypeStr}, jsonpath)
	if err != nil {
		t.Fatalf("jsonPathMulti() error: %v", err)
	}

	// Should return 2 metrics (one per array element)
	if len(result.Metrics) != 2 {
		t.Fatalf("Expected 2 metrics, got %d", len(result.Metrics))
	}

	// Verify first metric
	m0 := result.Metrics[0]
	if m0.Name != "item" {
		t.Errorf("Metric 0: expected name 'item', got %q", m0.Name)
	}
	if m0.Labels["index"] != "0" {
		t.Errorf("Metric 0: expected index label '0', got %q", m0.Labels["index"])
	}
	if !contains(m0.Value, `"name":"a"`) {
		t.Errorf("Metric 0: expected to contain name:a, got %q", m0.Value)
	}

	// Verify second metric
	m1 := result.Metrics[1]
	if m1.Labels["index"] != "1" {
		t.Errorf("Metric 1: expected index label '1', got %q", m1.Labels["index"])
	}
}

// TestJSONPathMulti_SingleValue tests JSONPath with single value result
func TestJSONPathMulti_SingleValue(t *testing.T) {
	input := `{"name": "test", "value": 42}`
	jsonpath := "$.value"

	result, err := jsonPathMulti(Value{Data: input, Type: ValueTypeStr}, jsonpath)
	if err != nil {
		t.Fatalf("jsonPathMulti() error: %v", err)
	}

	// Should return 1 metric
	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(result.Metrics))
	}

	m := result.Metrics[0]
	if m.Value != "42" {
		t.Errorf("Expected value '42', got %q", m.Value)
	}
	if len(m.Labels) != 0 {
		t.Errorf("Single value should have no labels, got %v", m.Labels)
	}
}

// TestJSONPathMulti_StringValue tests JSONPath extracting string
func TestJSONPathMulti_StringValue(t *testing.T) {
	input := `{"message": "hello world"}`
	jsonpath := "$.message"

	result, err := jsonPathMulti(Value{Data: input, Type: ValueTypeStr}, jsonpath)
	if err != nil {
		t.Fatalf("jsonPathMulti() error: %v", err)
	}

	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(result.Metrics))
	}

	// String should be returned directly without JSON encoding
	m := result.Metrics[0]
	if m.Value != "hello world" {
		t.Errorf("Expected value 'hello world', got %q", m.Value)
	}
}

// TestJSONPathMulti_ArrayOfStrings tests array of primitive values
func TestJSONPathMulti_ArrayOfStrings(t *testing.T) {
	input := `{"tags": ["red", "green", "blue"]}`
	jsonpath := "$.tags[*]"

	result, err := jsonPathMulti(Value{Data: input, Type: ValueTypeStr}, jsonpath)
	if err != nil {
		t.Fatalf("jsonPathMulti() error: %v", err)
	}

	// Should return 3 metrics
	if len(result.Metrics) != 3 {
		t.Fatalf("Expected 3 metrics, got %d", len(result.Metrics))
	}

	// Verify values
	expected := []string{"red", "green", "blue"}
	for i, exp := range expected {
		if result.Metrics[i].Value != exp {
			t.Errorf("Metric %d: expected value %q, got %q", i, exp, result.Metrics[i].Value)
		}
		if result.Metrics[i].Labels["index"] != fmt.Sprintf("%d", i) {
			t.Errorf("Metric %d: expected index %d, got %q", i, i, result.Metrics[i].Labels["index"])
		}
	}
}

// TestJSONPathMulti_ViaPreprocessor tests full integration
func TestJSONPathMulti_ViaPreprocessor(t *testing.T) {
	input := `{"servers": [{"name": "web1", "cpu": 45}, {"name": "web2", "cpu": 60}]}`
	jsonpath := "$.servers[*]"

	p := NewPreprocessor("test-shard")
	result, err := p.Execute("item1",
		Value{Data: input, Type: ValueTypeStr, Timestamp: time.Now()},
		Step{Type: StepTypeJSONPathMulti, Params: jsonpath})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if len(result.Metrics) != 2 {
		t.Fatalf("Expected 2 metrics, got %d", len(result.Metrics))
	}

	// Verify both metrics have index labels
	for i := 0; i < 2; i++ {
		if result.Metrics[i].Labels["index"] != fmt.Sprintf("%d", i) {
			t.Errorf("Metric %d missing correct index label", i)
		}
	}
}

// TestJSONPathMulti_EmptyResult tests error on no matches
func TestJSONPathMulti_EmptyResult(t *testing.T) {
	input := `{"name": "test"}`
	jsonpath := "$.nonexistent"

	_, err := jsonPathMulti(Value{Data: input, Type: ValueTypeStr}, jsonpath)
	if err == nil {
		t.Fatal("Expected error for no matches, got nil")
	}
}

// TestZabbixCompatibility_JSONPath tests old step type still works
func TestZabbixCompatibility_JSONPath(t *testing.T) {
	input := `{"items": [1, 2, 3]}`
	jsonpath := "$.items"

	p := NewPreprocessor("test-shard")
	result, err := p.Execute("item1",
		Value{Data: input, Type: ValueTypeStr, Timestamp: time.Now()},
		Step{Type: StepTypeJSONPath, Params: jsonpath}) // OLD step type

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// OLD behavior: Should return SINGLE metric with JSON array string
	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric (Zabbix compatibility), got %d", len(result.Metrics))
	}

	// Value should be a JSON array
	value := result.Metrics[0].Value
	if value[0] != '[' {
		t.Errorf("Expected JSON array, got: %s", value)
	}
}

// ============================================================================
// SNMP Walk to JSON Multi-Metric Tests
// ============================================================================

// TestSNMPWalkMulti_BasicDiscovery tests basic SNMP discovery with multiple items
func TestSNMPWalkMulti_BasicDiscovery(t *testing.T) {
	params := `{#IFNAME}
.1.3.6.1.2.1.31.1.1.1.1
0
{#IFDESCR}
.1.3.6.1.2.1.2.2.1.2
0`

	input := `.1.3.6.1.2.1.31.1.1.1.1.1 = STRING: "eth0"
.1.3.6.1.2.1.31.1.1.1.1.2 = STRING: "eth1"
.1.3.6.1.2.1.2.2.1.2.1 = STRING: "Ethernet Interface 0"
.1.3.6.1.2.1.2.2.1.2.2 = STRING: "Ethernet Interface 1"
`

	result, err := snmpWalkToJSONMulti(Value{Data: input, Type: ValueTypeStr}, params, NoopLogger{})
	if err != nil {
		t.Fatalf("snmpWalkToJSONMulti() error: %v", err)
	}

	// Should return 2 metrics (one per index: 2, 1 in descending order)
	if len(result.Metrics) != 2 {
		t.Fatalf("Expected 2 metrics, got %d", len(result.Metrics))
	}

	// Verify first metric (index 2 - descending order)
	m0 := result.Metrics[0]
	if m0.Name != "snmp_discovery" {
		t.Errorf("Metric 0: expected name 'snmp_discovery', got %q", m0.Name)
	}
	if m0.Labels["index"] != "2" {
		t.Errorf("Metric 0: expected index label '2', got %q", m0.Labels["index"])
	}
	if !contains(m0.Value, `"{#SNMPINDEX}":"2"`) {
		t.Errorf("Metric 0: expected SNMPINDEX=2, got %q", m0.Value)
	}
	if !contains(m0.Value, `"{#IFNAME}":"eth1"`) {
		t.Errorf("Metric 0: expected IFNAME=eth1, got %q", m0.Value)
	}
	if !contains(m0.Value, `"{#IFDESCR}":"Ethernet Interface 1"`) {
		t.Errorf("Metric 0: expected IFDESCR, got %q", m0.Value)
	}

	// Verify second metric (index 1)
	m1 := result.Metrics[1]
	if m1.Labels["index"] != "1" {
		t.Errorf("Metric 1: expected index label '1', got %q", m1.Labels["index"])
	}
	if !contains(m1.Value, `"{#IFNAME}":"eth0"`) {
		t.Errorf("Metric 1: expected IFNAME=eth0, got %q", m1.Value)
	}
}

// TestSNMPWalkMulti_EmptyInput tests empty input handling
func TestSNMPWalkMulti_EmptyInput(t *testing.T) {
	params := `{#IFNAME}
.1.3.6.1.2.1.31.1.1.1.1
0`

	result, err := snmpWalkToJSONMulti(Value{Data: "", Type: ValueTypeStr}, params, NoopLogger{})
	if err != nil {
		t.Fatalf("snmpWalkToJSONMulti() error on empty input: %v", err)
	}

	// Empty input should return zero metrics
	if len(result.Metrics) != 0 {
		t.Errorf("Expected 0 metrics for empty input, got %d", len(result.Metrics))
	}
}

// TestSNMPWalkMulti_NoMacros tests behavior with no macros defined
func TestSNMPWalkMulti_NoMacros(t *testing.T) {
	params := "" // No macros

	input := `.1.3.6.1.2.1.31.1.1.1.1.1 = STRING: "eth0"`

	result, err := snmpWalkToJSONMulti(Value{Data: input, Type: ValueTypeStr}, params, NoopLogger{})
	if err != nil {
		t.Fatalf("snmpWalkToJSONMulti() error: %v", err)
	}

	// No macros should return empty metrics array
	if len(result.Metrics) != 0 {
		t.Errorf("Expected 0 metrics with no macros, got %d", len(result.Metrics))
	}
}

// TestSNMPWalkMulti_SingleItem tests single discovery item
func TestSNMPWalkMulti_SingleItem(t *testing.T) {
	params := `{#IFNAME}
.1.3.6.1.2.1.31.1.1.1.1
0`

	input := `.1.3.6.1.2.1.31.1.1.1.1.1 = STRING: "lo"`

	result, err := snmpWalkToJSONMulti(Value{Data: input, Type: ValueTypeStr}, params, NoopLogger{})
	if err != nil {
		t.Fatalf("snmpWalkToJSONMulti() error: %v", err)
	}

	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(result.Metrics))
	}

	m := result.Metrics[0]
	if m.Name != "snmp_discovery" {
		t.Errorf("Expected name 'snmp_discovery', got %q", m.Name)
	}
	if m.Labels["index"] != "1" {
		t.Errorf("Expected index label '1', got %q", m.Labels["index"])
	}
	if !contains(m.Value, `"{#IFNAME}":"lo"`) {
		t.Errorf("Expected IFNAME=lo, got %q", m.Value)
	}
}

// TestSNMPWalkMulti_ViaPreprocessor tests full integration
func TestSNMPWalkMulti_ViaPreprocessor(t *testing.T) {
	params := `{#IFNAME}
.1.3.6.1.2.1.31.1.1.1.1
0
{#IFTYPE}
.1.3.6.1.2.1.2.2.1.3
0`

	input := `.1.3.6.1.2.1.31.1.1.1.1.1 = STRING: "eth0"
.1.3.6.1.2.1.31.1.1.1.1.2 = STRING: "wlan0"
.1.3.6.1.2.1.2.2.1.3.1 = INTEGER: 6
.1.3.6.1.2.1.2.2.1.3.2 = INTEGER: 71
`

	p := NewPreprocessor("test-shard")
	result, err := p.Execute("item1",
		Value{Data: input, Type: ValueTypeStr, Timestamp: time.Now()},
		Step{Type: StepTypeSNMPWalkToJSONMulti, Params: params})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Should return 2 metrics (indices 2, 1 in descending order)
	if len(result.Metrics) != 2 {
		t.Fatalf("Expected 2 metrics, got %d", len(result.Metrics))
	}

	// Verify metric structure and labels
	for i, m := range result.Metrics {
		if m.Name != "snmp_discovery" {
			t.Errorf("Metric %d: expected name 'snmp_discovery', got %q", i, m.Name)
		}
		if m.Type != ValueTypeStr {
			t.Errorf("Metric %d: expected type ValueTypeStr, got %v", i, m.Type)
		}
		if m.Labels["index"] == "" {
			t.Errorf("Metric %d: missing index label", i)
		}
		// Verify JSON structure
		if !contains(m.Value, `"{#SNMPINDEX}"`) {
			t.Errorf("Metric %d: missing SNMPINDEX field", i)
		}
		if !contains(m.Value, `"{#IFNAME}"`) {
			t.Errorf("Metric %d: missing IFNAME field", i)
		}
		if !contains(m.Value, `"{#IFTYPE}"`) {
			t.Errorf("Metric %d: missing IFTYPE field", i)
		}
	}
}

// TestSNMPWalkMulti_InvalidData tests error handling
func TestSNMPWalkMulti_InvalidData(t *testing.T) {
	params := `{#IFNAME}
.1.3.6.1.2.1.31.1.1.1.1
0`

	input := `not valid SNMP walk data`

	_, err := snmpWalkToJSONMulti(Value{Data: input, Type: ValueTypeStr}, params, NoopLogger{})
	if err == nil {
		t.Fatal("Expected error for invalid SNMP data, got nil")
	}
}

// TestZabbixCompatibility_SNMPWalkToJSON tests that old step type still works
func TestZabbixCompatibility_SNMPWalkToJSON(t *testing.T) {
	params := `{#IFNAME}
.1.3.6.1.2.1.31.1.1.1.1
0`

	input := `.1.3.6.1.2.1.31.1.1.1.1.1 = STRING: "eth0"
.1.3.6.1.2.1.31.1.1.1.1.2 = STRING: "eth1"
`

	p := NewPreprocessor("test-shard")
	result, err := p.Execute("item1",
		Value{Data: input, Type: ValueTypeStr, Timestamp: time.Now()},
		Step{Type: StepTypeSNMPWalkToJSON, Params: params}) // OLD step type

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// OLD behavior: Should return SINGLE metric with JSON array string
	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric (Zabbix compatibility), got %d", len(result.Metrics))
	}

	// Value should be a JSON array string
	value := result.Metrics[0].Value
	if value[0] != '[' || value[len(value)-1] != ']' {
		t.Errorf("Expected JSON array string, got: %s", value)
	}

	// Verify it contains both discovery items in the JSON array
	if !contains(value, `"{#SNMPINDEX}":"2"`) {
		t.Errorf("JSON array missing index 2")
	}
	if !contains(value, `"{#SNMPINDEX}":"1"`) {
		t.Errorf("JSON array missing index 1")
	}
	if !contains(value, `"{#IFNAME}":"eth0"`) {
		t.Errorf("JSON array missing eth0")
	}
	if !contains(value, `"{#IFNAME}":"eth1"`) {
		t.Errorf("JSON array missing eth1")
	}
}

// ============================================================================
// CSV to JSON Multi-Metric Tests
// ============================================================================

// TestCSVMulti_WithHeader tests CSV with header row
func TestCSVMulti_WithHeader(t *testing.T) {
	params := ",\n\"\n1" // comma delimiter, double quote, has header

	input := `name,age,city
Alice,30,NYC
Bob,25,LA
`

	result, err := csvToJSONMulti(Value{Data: input, Type: ValueTypeStr}, params)
	if err != nil {
		t.Fatalf("csvToJSONMulti() error: %v", err)
	}

	// Should return 3 rows (2 data + 1 empty from trailing newline)
	if len(result.Metrics) != 3 {
		t.Fatalf("Expected 3 metrics, got %d", len(result.Metrics))
	}

	// Verify first row
	m0 := result.Metrics[0]
	if m0.Name != "csv_row" {
		t.Errorf("Metric 0: expected name 'csv_row', got %q", m0.Name)
	}
	if m0.Labels["row"] != "1" {
		t.Errorf("Metric 0: expected row label '1', got %q", m0.Labels["row"])
	}
	if !contains(m0.Value, `"name":"Alice"`) {
		t.Errorf("Metric 0: expected name=Alice, got %q", m0.Value)
	}
	if !contains(m0.Value, `"age":"30"`) {
		t.Errorf("Metric 0: expected age=30, got %q", m0.Value)
	}
	if !contains(m0.Value, `"city":"NYC"`) {
		t.Errorf("Metric 0: expected city=NYC, got %q", m0.Value)
	}

	// Verify second row
	m1 := result.Metrics[1]
	if m1.Labels["row"] != "2" {
		t.Errorf("Metric 1: expected row label '2', got %q", m1.Labels["row"])
	}
	if !contains(m1.Value, `"name":"Bob"`) {
		t.Errorf("Metric 1: expected name=Bob, got %q", m1.Value)
	}

	// Verify third row (empty trailing row from newline)
	m2 := result.Metrics[2]
	if m2.Labels["row"] != "3" {
		t.Errorf("Metric 2: expected row label '3', got %q", m2.Labels["row"])
	}
	// Empty row should have all empty fields
	if !contains(m2.Value, `"name":""`) {
		t.Errorf("Metric 2: expected empty name field, got %q", m2.Value)
	}
}

// TestCSVMulti_NoHeader tests CSV without header row
func TestCSVMulti_NoHeader(t *testing.T) {
	params := ",\n\"\n0" // comma delimiter, double quote, no header

	input := `10,20,30
40,50,60`

	result, err := csvToJSONMulti(Value{Data: input, Type: ValueTypeStr}, params)
	if err != nil {
		t.Fatalf("csvToJSONMulti() error: %v", err)
	}

	// Should return 2 rows
	if len(result.Metrics) != 2 {
		t.Fatalf("Expected 2 metrics, got %d", len(result.Metrics))
	}

	// Verify first row (1-based column indices)
	m0 := result.Metrics[0]
	if m0.Labels["row"] != "1" {
		t.Errorf("Metric 0: expected row label '1', got %q", m0.Labels["row"])
	}
	if !contains(m0.Value, `"1":"10"`) {
		t.Errorf("Metric 0: expected column 1=10, got %q", m0.Value)
	}
	if !contains(m0.Value, `"2":"20"`) {
		t.Errorf("Metric 0: expected column 2=20, got %q", m0.Value)
	}
	if !contains(m0.Value, `"3":"30"`) {
		t.Errorf("Metric 0: expected column 3=30, got %q", m0.Value)
	}

	// Verify second row
	m1 := result.Metrics[1]
	if m1.Labels["row"] != "2" {
		t.Errorf("Metric 1: expected row label '2', got %q", m1.Labels["row"])
	}
	if !contains(m1.Value, `"1":"40"`) {
		t.Errorf("Metric 1: expected column 1=40, got %q", m1.Value)
	}
}

// TestCSVMulti_EmptyInput tests empty input handling
func TestCSVMulti_EmptyInput(t *testing.T) {
	params := ",\n\"\n1"

	result, err := csvToJSONMulti(Value{Data: "", Type: ValueTypeStr}, params)
	if err != nil {
		t.Fatalf("csvToJSONMulti() error on empty input: %v", err)
	}

	// Empty input should return zero metrics
	if len(result.Metrics) != 0 {
		t.Errorf("Expected 0 metrics for empty input, got %d", len(result.Metrics))
	}
}

// TestCSVMulti_SingleRow tests single data row
func TestCSVMulti_SingleRow(t *testing.T) {
	params := ",\n\"\n1"

	input := `name,value
test,42`

	result, err := csvToJSONMulti(Value{Data: input, Type: ValueTypeStr}, params)
	if err != nil {
		t.Fatalf("csvToJSONMulti() error: %v", err)
	}

	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(result.Metrics))
	}

	m := result.Metrics[0]
	if m.Name != "csv_row" {
		t.Errorf("Expected name 'csv_row', got %q", m.Name)
	}
	if m.Labels["row"] != "1" {
		t.Errorf("Expected row label '1', got %q", m.Labels["row"])
	}
	if !contains(m.Value, `"name":"test"`) {
		t.Errorf("Expected name=test, got %q", m.Value)
	}
	if !contains(m.Value, `"value":"42"`) {
		t.Errorf("Expected value=42, got %q", m.Value)
	}
}

// TestCSVMulti_ViaPreprocessor tests full integration
func TestCSVMulti_ViaPreprocessor(t *testing.T) {
	params := ",\n\"\n1"

	input := `id,status
1,active
2,inactive
3,pending`

	p := NewPreprocessor("test-shard")
	result, err := p.Execute("item1",
		Value{Data: input, Type: ValueTypeStr, Timestamp: time.Now()},
		Step{Type: StepTypeCSVToJSONMulti, Params: params})

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Should return 3 metrics (one per data row)
	if len(result.Metrics) != 3 {
		t.Fatalf("Expected 3 metrics, got %d", len(result.Metrics))
	}

	// Verify all metrics have correct structure
	for i, m := range result.Metrics {
		if m.Name != "csv_row" {
			t.Errorf("Metric %d: expected name 'csv_row', got %q", i, m.Name)
		}
		if m.Type != ValueTypeStr {
			t.Errorf("Metric %d: expected type ValueTypeStr, got %v", i, m.Type)
		}
		if m.Labels["row"] == "" {
			t.Errorf("Metric %d: missing row label", i)
		}
		// Verify JSON structure
		if !contains(m.Value, `"id"`) {
			t.Errorf("Metric %d: missing id field", i)
		}
		if !contains(m.Value, `"status"`) {
			t.Errorf("Metric %d: missing status field", i)
		}
	}
}

// TestCSVMulti_InvalidParams tests error handling
func TestCSVMulti_InvalidParams(t *testing.T) {
	params := "," // Missing required params

	input := `a,b,c`

	_, err := csvToJSONMulti(Value{Data: input, Type: ValueTypeStr}, params)
	if err == nil {
		t.Fatal("Expected error for invalid params, got nil")
	}
}

// TestZabbixCompatibility_CSVToJSON tests that old step type still works
func TestZabbixCompatibility_CSVToJSON(t *testing.T) {
	params := ",\n\"\n1"

	input := `name,value
foo,10
bar,20`

	p := NewPreprocessor("test-shard")
	result, err := p.Execute("item1",
		Value{Data: input, Type: ValueTypeStr, Timestamp: time.Now()},
		Step{Type: StepTypeCSVToJSON, Params: params}) // OLD step type

	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// OLD behavior: Should return SINGLE metric with JSON array string
	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric (Zabbix compatibility), got %d", len(result.Metrics))
	}

	// Value should be a JSON array string
	value := result.Metrics[0].Value
	if value[0] != '[' || value[len(value)-1] != ']' {
		t.Errorf("Expected JSON array string, got: %s", value)
	}

	// Verify it contains both rows in the JSON array
	if !contains(value, `"name":"foo"`) {
		t.Errorf("JSON array missing foo row")
	}
	if !contains(value, `"name":"bar"`) {
		t.Errorf("JSON array missing bar row")
	}
	if !contains(value, `"value":"10"`) {
		t.Errorf("JSON array missing value 10")
	}
	if !contains(value, `"value":"20"`) {
		t.Errorf("JSON array missing value 20")
	}
}
