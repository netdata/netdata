# Zabbix Preprocessing Library - Integration Guide

Pure Go implementation of Zabbix preprocessing. 100% compatible with Zabbix official test suite (362/362 tests passing). No CGO dependencies.

**Package**: `github.com/netdata/zabbix-preproc`

---

## Quick Start

```go
import zp "github.com/netdata/zabbix-preproc"

// Create preprocessor instance (one per shard)
p := zp.NewPreprocessor("shard-1")

// Define input value
value := zp.Value{
    Data:      "42.5",
    Type:      zp.ValueTypeStr,
    Timestamp: time.Now(),
}

// Define preprocessing step
step := zp.Step{
    Type:   zp.StepTypeMultiplier,
    Params: "10",
}

// Execute
result, err := p.Execute("item-123", value, step)
if err != nil {
    log.Fatal(err)
}

// Get result
fmt.Println(result.Metrics[0].Value) // "425"
```

---

## Core Types

### Value
```go
type Value struct {
    Data      string    // Raw string data
    Type      ValueType // ValueTypeStr (0), ValueTypeFloat (1), ValueTypeUint64 (3)
    Timestamp time.Time // Required for stateful operations (delta, throttle)
    IsError   bool      // True if value represents error from previous step
}

const (
    ValueTypeStr    ValueType = 0
    ValueTypeFloat  ValueType = 1
    ValueTypeUint64 ValueType = 3
)
```

### Step
```go
type Step struct {
    Type         StepType     // One of 30 preprocessing types (see below)
    Params       string       // Type-specific parameters
    ErrorHandler ErrorHandler // Optional error handling
}
```

### Result
```go
type Result struct {
    Metrics []Metric // Array of extracted metrics
    Logs    []string // Optional log entries
    Error   error    // Overall error
}

type Metric struct {
    Name   string            // Metric name
    Value  string            // Metric value as string
    Type   ValueType         // Value type
    Labels map[string]string // Optional labels (for Prometheus, etc.)
}
```

---

## All 30 Preprocessing Types

### Arithmetic Operations

**Type 1: Multiplier**
```go
step := zp.Step{Type: zp.StepTypeMultiplier, Params: "10"}
// Input: "42.5" → Output: "425"
```

**Type 9: Delta Value** (stateful - requires timestamp)
```go
// First call: value=100, timestamp=T1 → returns nothing (needs baseline)
// Second call: value=150, timestamp=T2 → returns "50" (150-100)
step := zp.Step{Type: zp.StepTypeDeltaValue}
```

**Type 10: Delta Speed** (stateful - requires timestamp)
```go
// Returns (current - previous) / time_diff_seconds
step := zp.Step{Type: zp.StepTypeDeltaSpeed}
```

### String Operations

**Type 2: RTrim**
```go
step := zp.Step{Type: zp.StepTypeRTrim, Params: " \t\n"}
// Input: "hello  \n" → Output: "hello"
```

**Type 3: LTrim**
```go
step := zp.Step{Type: zp.StepTypeLTrim, Params: " \t"}
// Input: "  hello" → Output: "hello"
```

**Type 4: Trim**
```go
step := zp.Step{Type: zp.StepTypeTrim, Params: " "}
// Input: "  hello  " → Output: "hello"
```

**Type 5: Regex Substitution**
```go
// Params: "pattern\nreplacement"
step := zp.Step{
    Type:   zp.StepTypeRegexSubstitution,
    Params: "\\d+\nNUMBER",
}
// Input: "error code 123" → Output: "error code NUMBER"

// With capture groups:
step := zp.Step{
    Type:   zp.StepTypeRegexSubstitution,
    Params: "(\\d+)-(\\d+)\n\\2-\\1",
}
// Input: "123-456" → Output: "456-123"
```

**Type 25: String Replace**
```go
// Params: "search\nreplace"
step := zp.Step{
    Type:   zp.StepTypeStringReplace,
    Params: "old\nnew",
}
// Input: "old value old" → Output: "new value new"
```

### Type Conversions

**Type 6: Bool to Decimal**
```go
step := zp.Step{Type: zp.StepTypeBool2Dec}
// "true", "yes", "on", "up", "running", "enabled", "available", non-zero → "1"
// "false", "no", "off", "down", "unused", "disabled", "unavailable", "0" → "0"
```

**Type 7: Octal to Decimal**
```go
step := zp.Step{Type: zp.StepTypeOct2Dec}
// Input: "755" → Output: "493"
// Input: "0755" → Output: "493"
```

**Type 8: Hex to Decimal**
```go
step := zp.Step{Type: zp.StepTypeHex2Dec}
// Input: "FF" → Output: "255"
// Input: "0xFF" → Output: "255"
// Input: "0x1A2B" → Output: "6699"
```

### Data Extraction

**Type 11: XPath**
```go
step := zp.Step{
    Type:   zp.StepTypeXPath,
    Params: "//server/status/text()",
}
// Input: "<server><status>OK</status></server>"
// Output: "OK"
```

**Type 12: JSONPath**
```go
step := zp.Step{
    Type:   zp.StepTypeJSONPath,
    Params: "$.store.book[0].price",
}
// Input: `{"store":{"book":[{"price":8.95}]}}`
// Output: "8.95"

// Array extraction:
step := zp.Step{
    Type:   zp.StepTypeJSONPath,
    Params: "$.store.book[*].price",
}
// Output: "[8.95,12.99,8.99]"
```

**Type 22: Prometheus Pattern**
```go
// Params: "metric_name\nlabel_name\nfunction"
// Functions: value, label, sum, min, max, avg, count

// Get metric value:
step := zp.Step{
    Type:   zp.StepTypePrometheusPattern,
    Params: "cpu_usage\n\nvalue",
}
// Input: `cpu_usage{host="server1"} 85.5`
// Output: "85.5"

// Get label value:
step := zp.Step{
    Type:   zp.StepTypePrometheusPattern,
    Params: "http_requests\nmethod\nlabel",
}
// Input: `http_requests{method="GET"} 100`
// Output: "GET"

// Aggregate:
step := zp.Step{
    Type:   zp.StepTypePrometheusPattern,
    Params: "cpu_usage\n\nsum",
}
// Multiple metrics → sum of all values
```

**Type 23: Prometheus to JSON**
```go
step := zp.Step{Type: zp.StepTypePrometheusToJSON}
// Input: `metric_name{label="value"} 123`
// Output: `[{"name":"metric_name","value":"123","labels":{"label":"value"}}]`
```

**Type 24: CSV to JSON**
```go
// Params: "delimiter\nquote_char\nwith_header\n"
// delimiter: single char (default: ,)
// quote_char: single char (default: ")
// with_header: 1 = first row is header, 0 = no header

step := zp.Step{
    Type:   zp.StepTypeCSVToJSON,
    Params: ",\n\"\n1\n",
}
// Input:
// name,age,city
// Alice,30,NYC
// Bob,25,LA

// Output:
// [{"name":"Alice","age":"30","city":"NYC"},{"name":"Bob","age":"25","city":"LA"}]
```

**Type 27: XML to JSON**
```go
step := zp.Step{Type: zp.StepTypeXMLToJSON}
// Input: `<root><foo bar="BAR">BAZ</foo></root>`
// Output: `{"root":{"foo":{"@bar":"BAR","#text":"BAZ"}}}`

// Zabbix XML serialization rules:
// 1. Attributes → "@attr": "value"
// 2. Self-closing → null
// 3. Empty attributes → "@attr": ""
// 4. Repeated elements → arrays
// 5. Simple text → direct string
// 6. Text + attributes → "#text": "value"
```

### Validation

**Type 13: Validate Range**
```go
// Params: "min\nmax"
step := zp.Step{
    Type:   zp.StepTypeValidateRange,
    Params: "0\n100",
}
// Input: "50" → Output: "50" (passes validation)
// Input: "150" → Error: value out of range
```

**Type 14: Validate Regex**
```go
step := zp.Step{
    Type:   zp.StepTypeValidateRegex,
    Params: "^\\d+$",
}
// Input: "12345" → passes
// Input: "abc123" → Error: does not match pattern
```

**Type 15: Validate Not Regex**
```go
step := zp.Step{
    Type:   zp.StepTypeValidateNotRegex,
    Params: "error|fail",
}
// Input: "success" → passes
// Input: "error occurred" → Error: matches forbidden pattern
```

**Type 26: Validate Not Supported**
```go
step := zp.Step{Type: zp.StepTypeValidateNotSupported}
// Always returns error: "not supported"
// Used to mark items as not supported in preprocessing chain
```

### Error Field Extraction

**Type 16: Error Field JSON**
```go
step := zp.Step{
    Type:   zp.StepTypeErrorFieldJSON,
    Params: "$.error.message",
}
// Input: `{"error":{"message":"timeout"}}`
// Output: Error with message "timeout"
```

**Type 17: Error Field XML**
```go
step := zp.Step{
    Type:   zp.StepTypeErrorFieldXML,
    Params: "//error/text()",
}
// Input: `<response><error>Not found</error></response>`
// Output: Error with message "Not found"
```

**Type 18: Error Field Regex**
```go
step := zp.Step{
    Type:   zp.StepTypeErrorFieldRegex,
    Params: "ERROR: (.+)",
}
// Input: "ERROR: Connection refused"
// Output: Error with message "Connection refused"
```

### Throttling (Stateful)

**Type 19: Throttle Value**
```go
step := zp.Step{Type: zp.StepTypeThrottleValue}
// Only passes through if value changed from previous
// Same value → returns empty result (discarded)
```

**Type 20: Throttle Timed Value**
```go
// Params: "seconds"
step := zp.Step{
    Type:   zp.StepTypeThrottleTimedValue,
    Params: "60",
}
// Passes through if:
// - Value changed from previous, OR
// - More than 60 seconds since last pass
```

### JavaScript

**Type 21: JavaScript**
```go
step := zp.Step{
    Type:   zp.StepTypeJavaScript,
    Params: "return parseFloat(value) * 2;",
}
// Input: "21" → Output: "42"

// Complex transformation:
step := zp.Step{
    Type:   zp.StepTypeJavaScript,
    Params: `
        var obj = JSON.parse(value);
        return obj.temperature * 9/5 + 32;
    `,
}
// Input: `{"temperature": 20}` → Output: "68"
```

### SNMP Operations

**Type 28: SNMP Walk Value**
```go
// Params: "oid\nformat"
// format: 0=unchanged, 1=UTF-8, 2=MAC, 3=Int, 4=Hex, 5=IP
step := zp.Step{
    Type:   zp.StepTypeSNMPWalkValue,
    Params: ".1.3.6.1.2.1.2.2.1.2.1\n0",
}
// Extracts specific OID value from SNMP walk output
```

**Type 29: SNMP Walk to JSON**
```go
// Params: "field_name\noid_to_extract\nformat"
step := zp.Step{
    Type:   zp.StepTypeSNMPWalkToJSON,
    Params: "interfaces\n.1.3.6.1.2.1.2.2.1.2\n0",
}
// Converts SNMP walk table to JSON structure
```

**Type 30: SNMP Get Value**
```go
// Params: "format"
// format: 0=unchanged, 1=UTF-8, 2=MAC, 3=Int, 4=Hex, 5=IP
step := zp.Step{
    Type:   zp.StepTypeSNMPGetValue,
    Params: "2", // MAC address format
}
// Input: "0x001122334455" → Output: "00:11:22:33:44:55"
```

---

## Error Handling

```go
step := zp.Step{
    Type:   zp.StepTypeMultiplier,
    Params: "10",
    ErrorHandler: zp.ErrorHandler{
        Action: zp.ErrorActionSetValue,
        Params: "0", // Default to 0 on error
    },
}

// Error actions:
// ErrorActionDefault (0)  - Return error to caller
// ErrorActionDiscard (1)  - Discard value (empty result)
// ErrorActionSetValue (2) - Set custom value
// ErrorActionSetError (3) - Set custom error message
```

---

## Pipeline Execution

```go
steps := []zp.Step{
    {Type: zp.StepTypeJSONPath, Params: "$.temperature"},
    {Type: zp.StepTypeMultiplier, Params: "1.8"},
    {Type: zp.StepTypeValidateRange, Params: "-50\n150"},
}

result, err := p.ExecutePipeline("item-123", value, steps)
```

---

## Stateful Operations (Delta, Throttle)

```go
// Create preprocessor for shard
p := zp.NewPreprocessor("shard-1")

// Delta/Throttle operations require:
// 1. Consistent itemID across calls
// 2. Accurate timestamps

value1 := zp.Value{Data: "100", Timestamp: time.Now()}
value2 := zp.Value{Data: "150", Timestamp: time.Now().Add(time.Second)}

step := zp.Step{Type: zp.StepTypeDeltaValue}

// First call establishes baseline
r1, _ := p.Execute("cpu-counter", value1, step)
// r1.Metrics is empty (needs baseline)

// Second call computes delta
r2, _ := p.Execute("cpu-counter", value2, step)
// r2.Metrics[0].Value = "50"
```

---

## State Management

### Enable Automatic Cleanup
```go
p := zp.NewPreprocessor("shard-1")

// Clean up state entries unused for 1 hour, check every 10 minutes
stopCleanup := p.EnableStateCleanup(time.Hour, 10*time.Minute)

// When done
stopCleanup()
```

### With Config
```go
cfg := zp.Config{
    Logger:               myLogger,
    StateTTL:             time.Hour,
    StateCleanupInterval: 10 * time.Minute,
}
p := zp.NewPreprocessorWithConfig("shard-1", cfg)
```

---

## Logging

```go
// Implement Logger interface
type Logger interface {
    Logf(format string, args ...interface{})
}

// Use your logger
p.SetLogger(myLogger)

// Or with config
cfg := zp.Config{Logger: myLogger}
p := zp.NewPreprocessorWithConfig("shard-1", cfg)

// Default: NoopLogger (zero overhead)
```

---

## Multi-Metric Extensions (Non-Zabbix)

```go
// Types 60-63 return multiple metrics directly (not JSON string)
step := zp.Step{
    Type:   zp.StepTypePrometheusToJSONMulti,
    Params: "",
}

result, _ := p.Execute("prom", value, step)
for _, metric := range result.Metrics {
    fmt.Printf("%s=%s labels=%v\n", metric.Name, metric.Value, metric.Labels)
}
```

---

## Thread Safety

```go
// Single Preprocessor instance is thread-safe
p := zp.NewPreprocessor("shard-1")

// Safe for concurrent use
go func() {
    p.Execute("item-1", val1, step1)
}()
go func() {
    p.Execute("item-2", val2, step2)
}()
```

---

## Complete Example: Netdata Integration

```go
package main

import (
    "fmt"
    "time"
    zp "github.com/netdata/zabbix-preproc"
)

func main() {
    // One preprocessor per shard
    p := zp.NewPreprocessorWithConfig("netdata-agent-1", zp.Config{
        StateTTL:             24 * time.Hour,
        StateCleanupInterval: time.Hour,
    })

    // Example: Process CPU metric from Prometheus format
    promValue := zp.Value{
        Data: `
node_cpu_seconds_total{cpu="0",mode="idle"} 12345.67
node_cpu_seconds_total{cpu="0",mode="system"} 1234.56
node_cpu_seconds_total{cpu="0",mode="user"} 9876.54
`,
        Type:      zp.ValueTypeStr,
        Timestamp: time.Now(),
    }

    // Extract idle CPU metric
    step := zp.Step{
        Type:   zp.StepTypePrometheusPattern,
        Params: "node_cpu_seconds_total\nmode=idle\nvalue",
    }

    result, err := p.Execute("cpu.idle", promValue, step)
    if err != nil {
        panic(err)
    }

    fmt.Printf("CPU idle seconds: %s\n", result.Metrics[0].Value)

    // Example: JSON extraction pipeline
    jsonValue := zp.Value{
        Data:      `{"system":{"cpu":{"usage":85.5},"memory":{"used":4096}}}`,
        Type:      zp.ValueTypeStr,
        Timestamp: time.Now(),
    }

    pipeline := []zp.Step{
        {Type: zp.StepTypeJSONPath, Params: "$.system.cpu.usage"},
        {Type: zp.StepTypeValidateRange, Params: "0\n100"},
    }

    result, err = p.ExecutePipeline("system.cpu.usage", jsonValue, pipeline)
    if err != nil {
        panic(err)
    }

    fmt.Printf("CPU usage: %s%%\n", result.Metrics[0].Value)

    // Example: Delta calculation (rate of change)
    counter1 := zp.Value{
        Data:      "1000",
        Type:      zp.ValueTypeStr,
        Timestamp: time.Now(),
    }
    counter2 := zp.Value{
        Data:      "1100",
        Type:      zp.ValueTypeStr,
        Timestamp: time.Now().Add(time.Minute),
    }

    deltaStep := zp.Step{Type: zp.StepTypeDeltaSpeed}

    // First call establishes baseline
    p.Execute("network.bytes", counter1, deltaStep)

    // Second call computes rate (bytes/second)
    result, _ = p.Execute("network.bytes", counter2, deltaStep)
    fmt.Printf("Network rate: %s bytes/sec\n", result.Metrics[0].Value)
}
```

---

## Dependencies

- `gopkg.in/yaml.v3` - YAML parsing (test harness)
- `github.com/ohler55/ojg` - JSONPath (RFC 9535 compliant)
- `github.com/antchfx/xmlquery` - XPath
- `github.com/dop251/goja` - JavaScript ES5.1 runtime
- `github.com/prometheus/common/expfmt` - Prometheus text format

All pure Go - no CGO dependencies.
