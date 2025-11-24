# Zabbix Preprocessing Library

Pure Go implementation of Zabbix preprocessing with 100% compatibility.

## Features

- ✅ **100% Zabbix Compatible** - Passes all 362 official Zabbix test cases
- ✅ **Pure Go** - No CGO, no external C libraries
- ✅ **Thread-Safe** - Concurrent processing with per-shard state isolation
- ✅ **High Performance** - VM pooling, regex caching, optimized parsers
- ✅ **Multi-Metric Support** - Extract multiple metrics from single values (Prometheus, SNMP, CSV, JSONPath)

## Installation

```bash
go get github.com/netdata/zabbix-preproc
```

## Quick Start

```go
package main

import (
	"fmt"
	preproc "github.com/netdata/zabbix-preproc"
)

func main() {
	// Create preprocessor for a shard
	p := preproc.NewPreprocessor("shard1")

	// Define preprocessing pipeline
	steps := []preproc.Step{
		{Type: preproc.StepTypeJSONPath, Params: "$.cpu"},
		{Type: preproc.StepTypeMultiplier, Params: "100"},
	}

	// Process value
	value := preproc.Value{Data: `{"cpu": 0.45}`}
	result, err := p.ExecutePipeline("cpu_metric", value, steps)
	if err != nil {
		panic(err)
	}

	// Get result
	fmt.Println(result.Metrics[0].Value) // Output: 45
}
```

## Validation

Validate steps before execution to catch configuration errors early:

```go
// Validate a single step
step := preproc.Step{Type: preproc.StepTypeMultiplier, Params: "2.5"}
if err := preproc.ValidateStep(step); err != nil {
	// Handle validation error
	fmt.Printf("Invalid step: %v\n", err)
}

// Validate entire pipeline
steps := []preproc.Step{
	{Type: preproc.StepTypeJSONPath, Params: "$.cpu"},
	{Type: preproc.StepTypeMultiplier, Params: "100"},
}
if err := preproc.ValidatePipeline(steps); err != nil {
	// Pipeline has invalid configuration
	fmt.Printf("Invalid pipeline: %v\n", err)
}
```

**Benefits of early validation:**
- Catch configuration errors before processing data
- Especially useful for multi-step pipelines (fail fast)
- Clear error messages indicating which parameter is missing/invalid
- Validates step type, required parameters, parameter format, and error handlers

## Common Use Cases

### 1. Data Extraction

**JSONPath** - Extract values from JSON:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeJSONPath, Params: "$.server.memory"},
}
// Input:  {"server": {"memory": 8192}}
// Output: 8192
```

**XPath** - Extract values from XML:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeXPath, Params: "//temperature/text()"},
}
// Input:  <data><temperature>25.5</temperature></data>
// Output: 25.5
```

**Prometheus** - Extract specific metrics:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypePrometheusPattern, Params: `http_requests{method="GET"}`},
}
// Input:  http_requests{method="GET"} 1027\nhttp_requests{method="POST"} 50
// Output: 1027
```

### 2. Data Transformation

**Multiply** - Scale values:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeMultiplier, Params: "1000"},
}
// Input:  1.5
// Output: 1500
```

**Regex Substitution** - Transform text:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeRegexSubstitution, Params: "([0-9]+)\n$1 units"},
}
// Input:  Temperature: 25
// Output: Temperature: 25 units
```

**Trim** - Remove whitespace:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeTrim, Params: " \t\n"},
}
// Input:  "  hello  "
// Output: "hello"
```

### 3. Stateful Operations

**Delta Value** - Calculate differences:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeDeltaValue},
}
// First call:  100 → 0 (baseline)
// Second call: 150 → 50 (delta)
```

**Delta Speed** - Calculate rate per second:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeDeltaSpeed},
}
// Call 1 (t=0s):  100 bytes → 0
// Call 2 (t=10s): 600 bytes → 50 bytes/sec
```

**Throttling** - Suppress duplicate values:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeThrottleValue, Params: "300"}, // 5 minutes
}
// Only processes value if changed or 5 min elapsed
```

### 4. Multi-Metric Extraction

**Prometheus to Multiple Metrics**:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypePrometheusToJSONMulti},
}

prometheusData := `
http_requests{method="GET"} 100
http_requests{method="POST"} 50
memory_bytes 8192
`

result, _ := p.ExecutePipeline("metrics", preproc.Value{Data: prometheusData}, steps)

for _, metric := range result.Metrics {
	fmt.Printf("%s: %s (labels: %v)\n", metric.Name, metric.Value, metric.Labels)
}
// Output:
// http_requests: 100 (labels: map[method:GET])
// http_requests: 50 (labels: map[method:POST])
// memory_bytes: 8192 (labels: map[])
```

**SNMP Walk to Multiple Metrics**:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeSNMPWalkToJSONMulti, Params: ".1.3.6.1.2.1.2.2.1.2"},
}

snmpData := `
.1.3.6.1.2.1.2.2.1.2.1 = STRING: "eth0"
.1.3.6.1.2.1.2.2.1.2.2 = STRING: "eth1"
`

result, _ := p.ExecutePipeline("interfaces", preproc.Value{Data: snmpData}, steps)
// Returns multiple metrics with OID indices as labels
```

**JSONPath to Multiple Values**:
```go
steps := []preproc.Step{
	{Type: preproc.StepTypeJSONPathMulti, Params: "$.servers[*].cpu"},
}

jsonData := `{"servers": [{"cpu": 45}, {"cpu": 23}, {"cpu": 78}]}`

result, _ := p.ExecutePipeline("cpus", preproc.Value{Data: jsonData}, steps)
// Returns 3 metrics with values: 45, 23, 78
```

### 5. Error Handling

**Discard on Error**:
```go
steps := []preproc.Step{
	{
		Type:   preproc.StepTypeJSONPath,
		Params: "$.nonexistent",
		ErrorHandler: preproc.ErrorHandler{
			Action: preproc.ErrorActionDiscard,
		},
	},
}
// Returns empty result instead of error
```

**Set Custom Value on Error**:
```go
steps := []preproc.Step{
	{
		Type:   preproc.StepTypeValidateRange,
		Params: "0\n100",
		ErrorHandler: preproc.ErrorHandler{
			Action: preproc.ErrorActionSetValue,
			Params: "-1", // Default value
		},
	},
}
// Returns -1 if value out of range
```

### 6. Complex Pipelines

Chain multiple steps together:

```go
steps := []preproc.Step{
	// 1. Extract JSON value
	{Type: preproc.StepTypeJSONPath, Params: "$.temperature"},

	// 2. Remove whitespace
	{Type: preproc.StepTypeTrim, Params: " "},

	// 3. Extract number (remove unit)
	{Type: preproc.StepTypeRegexSubstitution, Params: "([0-9.]+).*\n\\1"},

	// 4. Convert Celsius to Fahrenheit
	{Type: preproc.StepTypeMultiplier, Params: "1.8"},
}

value := preproc.Value{Data: `{"temperature": " 25.0 C "}`}
result, _ := p.ExecutePipeline("temp", value, steps)
// Output: 45
```

## Supported Preprocessing Types

### Arithmetic
- `StepTypeMultiplier` - Multiply by constant
- `StepTypeDeltaValue` - Calculate simple delta
- `StepTypeDeltaSpeed` - Calculate rate per second

### String Operations
- `StepTypeTrim`, `StepTypeRTrim`, `StepTypeLTrim` - Remove characters
- `StepTypeRegexSubstitution` - Regex find/replace
- `StepTypeStringReplace` - Simple string replace

### Conversions
- `StepTypeBool2Dec` - Boolean to 0/1
- `StepTypeOct2Dec` - Octal to decimal
- `StepTypeHex2Dec` - Hexadecimal to decimal

### Data Extraction (Single-Value)
- `StepTypeJSONPath` - JSONPath query
- `StepTypeXPath` - XPath query
- `StepTypePrometheusPattern` - Prometheus metric extraction
- `StepTypePrometheusToJSON` - Prometheus to JSON
- `StepTypeCSVToJSON` - CSV to JSON
- `StepTypeSNMPWalkValue` - Extract specific SNMP OID
- `StepTypeSNMPGetValue` - SNMP get value
- `StepTypeSNMPWalkToJSON` - SNMP walk to JSON

### Data Extraction (Multi-Metric)
- `StepTypePrometheusToJSONMulti` - Extract all Prometheus metrics
- `StepTypeJSONPathMulti` - Extract array values as separate metrics
- `StepTypeSNMPWalkToJSONMulti` - SNMP walk discovery
- `StepTypeCSVToJSONMulti` - CSV rows as separate metrics

### Validation
- `StepTypeValidateRange` - Check numeric range
- `StepTypeValidateRegex` - Validate with regex
- `StepTypeValidateNotRegex` - Inverse regex validation
- `StepTypeValidateNotSupported` - Check for "not supported" errors

### Error Field Extraction
- `StepTypeErrorFieldJSON` - Extract error from JSON
- `StepTypeErrorFieldXML` - Extract error from XML
- `StepTypeErrorFieldRegex` - Extract error with regex

### Throttling
- `StepTypeThrottleValue` - Suppress duplicate values
- `StepTypeThrottleTimedValue` - Time-based throttling

### Advanced
- `StepTypeJavaScript` - Custom JavaScript preprocessing

## Architecture

### Per-Shard State Isolation

Each preprocessor instance maintains independent state:

```go
// Shard A - processing metrics from server1
p1 := preproc.NewPreprocessor("shard-server1")

// Shard B - processing metrics from server2
p2 := preproc.NewPreprocessor("shard-server2")

// Each maintains separate delta/throttle history
// No state leakage between shards
```

### Thread Safety

All operations are thread-safe:
- Concurrent preprocessing across different items/shards
- VM pooling for JavaScript execution
- Read-only MIB-to-OID mappings
- RWMutex protection for stateful operations

### Performance Optimizations

- **VM Pooling**: JavaScript VMs reused, not recreated
- **Program Caching**: Compiled JavaScript programs cached
- **Regex Caching**: Patterns compiled once at package init
- **strings.Builder**: Efficient string construction in loops

## Testing

```bash
# Run all tests (362 Zabbix compatibility tests + unit tests)
go test

# Run with coverage report
go test -cover

# Run with race detector
go test -race

# Run specific test suite
go test -run TestZabbixTestSuite

# Run fuzzing tests (finds edge cases via random input generation)
go test -fuzz=FuzzJSONPath -fuzztime=30s     # Fuzz JSONPath extraction
go test -fuzz=FuzzXPath -fuzztime=30s        # Fuzz XPath extraction
go test -fuzz=FuzzRegexSubstitution -fuzztime=30s  # Fuzz regex replacement
go test -fuzz=FuzzMultiplier -fuzztime=30s   # Fuzz numeric operations
go test -fuzz=FuzzCSVToJSON -fuzztime=30s    # Fuzz CSV parsing
go test -fuzz=FuzzPrometheusPattern -fuzztime=30s  # Fuzz Prometheus parsing
go test -fuzz=FuzzSNMPWalkToJSON -fuzztime=30s     # Fuzz SNMP parsing
# Note: Fuzzing runs indefinitely if -fuzztime not specified

# Benchmark
go test -bench=.
```

## Compatibility

This library passes 100% of official Zabbix preprocessing tests:
- ✅ zbx_item_preproc.yaml (362 test cases)
- ✅ item_preproc_xpath.yaml (12 XPath tests)
- ✅ item_preproc_csv_to_json.yaml (4 CSV tests)

Zabbix version compatibility: **7.x** (backward compatible with 6.x)

## License

Apache 2.0

## Contributing

Contributions welcome! Please ensure:
- All Zabbix tests still pass (`go test`)
- No CGO dependencies added
- Code follows existing patterns
- New features have test coverage
