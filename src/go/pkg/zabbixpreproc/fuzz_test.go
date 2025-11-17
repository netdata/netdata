package zabbixpreproc

import (
	"strings"
	"testing"
	"time"
)

// Fuzzing tests to find edge cases and crashes
// Run with: go test -fuzz=Fuzz

// FuzzJSONPath fuzzes JSONPath extraction
func FuzzJSONPath(f *testing.F) {
	// Seed corpus with known good inputs
	f.Add(`{"key": "value"}`, "$.key")
	f.Add(`{"a": {"b": 123}}`, "$.a.b")
	f.Add(`[1,2,3]`, "$[0]")

	f.Fuzz(func(t *testing.T, jsonData, path string) {
		// Skip empty path (required parameter)
		if path == "" {
			return
		}

		value := Value{Data: jsonData, Type: ValueTypeStr}
		_, _ = jsonpathExtract(value, path)
		// Don't check error - we're looking for crashes, not correctness
	})
}

// FuzzXPath fuzzes XPath extraction
func FuzzXPath(f *testing.F) {
	// Seed corpus
	f.Add(`<root><item>value</item></root>`, "//item/text()")
	f.Add(`<data><a>1</a><b>2</b></data>`, "//a")

	f.Fuzz(func(t *testing.T, xmlData, path string) {
		// Skip empty path (required parameter)
		if path == "" {
			return
		}

		value := Value{Data: xmlData, Type: ValueTypeStr}
		_, _ = xpathExtract(value, path)
		// Don't check error - we're looking for crashes
	})
}

// FuzzRegexSubstitution fuzzes regex pattern replacement
func FuzzRegexSubstitution(f *testing.F) {
	// Seed corpus
	f.Add("hello world", "world\nuniverse")
	f.Add("test123", "[0-9]+\nNUM")

	f.Fuzz(func(t *testing.T, input, params string) {
		// Skip invalid params format (need newline separator)
		if !strings.Contains(params, "\n") {
			return
		}

		value := Value{Data: input, Type: ValueTypeStr}
		_, _ = regexSubstitute(value, params)
		// Don't check error - we're looking for crashes
	})
}

// FuzzMultiplier fuzzes numeric multiplication
func FuzzMultiplier(f *testing.F) {
	// Seed corpus
	f.Add("100", "2.5")
	f.Add("3.14", "10")

	f.Fuzz(func(t *testing.T, input, multiplier string) {
		value := Value{Data: input, Type: ValueTypeFloat}
		_, _ = multiplyValue(value, multiplier)
		// Don't check error - we're looking for crashes
	})
}

// FuzzCSVToJSON fuzzes CSV parsing
func FuzzCSVToJSON(f *testing.F) {
	// Seed corpus
	f.Add("a,b,c\n1,2,3", ",\n\"\n1")
	f.Add("x;y\n10;20", ";\n\"\n1")

	f.Fuzz(func(t *testing.T, csvData, params string) {
		// Skip invalid params (need at least 3 newlines)
		if strings.Count(params, "\n") < 2 {
			return
		}

		value := Value{Data: csvData, Type: ValueTypeStr}
		_, _ = csvToJSON(value, params)
		// Don't check error - we're looking for crashes
	})
}

// FuzzPrometheusPattern fuzzes Prometheus metric extraction
func FuzzPrometheusPattern(f *testing.F) {
	// Seed corpus
	f.Add(`http_requests{method="GET"} 100`, `http_requests{method="GET"}`)
	f.Add(`cpu_usage 45.2`, `cpu_usage`)

	f.Fuzz(func(t *testing.T, promData, pattern string) {
		// Skip empty pattern
		if pattern == "" {
			return
		}

		value := Value{Data: promData, Type: ValueTypeStr}
		_, _ = prometheusPattern(value, pattern)
		// Don't check error - we're looking for crashes
	})
}

// FuzzSNMPWalkToJSON fuzzes SNMP walk parsing
func FuzzSNMPWalkToJSON(f *testing.F) {
	// Seed corpus
	f.Add(`.1.3.6.1.2.1.1.1.0 = STRING: "Linux"`, "{#NAME}\n.1.3.6.1.2.1.1\n1")
	f.Add(`.1.3.6.1.4.1.2021.10.1.3.1 = INTEGER: 50`, "{#CPU}\n.1.3.6.1.4.1.2021.10.1.3\n1")

	f.Fuzz(func(t *testing.T, snmpData, params string) {
		// Skip invalid params (need multiple of 3 newlines)
		if strings.Count(params, "\n")%3 != 2 {
			return
		}

		value := Value{Data: snmpData, Type: ValueTypeStr}
		logger := NoopLogger{}
		_, _ = snmpWalkToJSON(value, params, logger)
		// Don't check error - we're looking for crashes
	})
}

// FuzzTrim fuzzes string trimming
func FuzzTrim(f *testing.F) {
	// Seed corpus
	f.Add("  hello  ", " ")
	f.Add("\t\ntest\r\n", "\t\n\r")

	f.Fuzz(func(t *testing.T, input, chars string) {
		value := Value{Data: input, Type: ValueTypeStr}
		_, _ = trimValue(value, chars, "both")
		_, _ = trimValue(value, chars, "left")
		_, _ = trimValue(value, chars, "right")
		// Don't check error - we're looking for crashes
	})
}

// FuzzDelta fuzzes delta value calculation
func FuzzDelta(f *testing.F) {
	// Seed corpus
	f.Add("100", "")
	f.Add("50", "")

	p := NewPreprocessor("fuzz-shard")

	f.Fuzz(func(t *testing.T, input, params string) {
		value := Value{
			Data:      input,
			Type:      ValueTypeUint64,
			Timestamp: time.Now(),
		}
		_, _ = p.deltaValue("fuzz-item", value, params)
		// Don't check error - we're looking for crashes
	})
}

// FuzzValidateRange fuzzes range validation
func FuzzValidateRange(f *testing.F) {
	// Seed corpus
	f.Add("50", "0\n100")
	f.Add("3.14", "-10\n10")

	f.Fuzz(func(t *testing.T, input, params string) {
		value := Value{Data: input, Type: ValueTypeFloat}
		_, _ = validateRange(value, params)
		// Don't check error - we're looking for crashes
	})
}

// FuzzInterpretEscapeSequences fuzzes escape sequence interpretation
func FuzzInterpretEscapeSequences(f *testing.F) {
	// Seed corpus
	f.Add("hello\\nworld")
	f.Add("tab\\there")
	f.Add("back\\\\slash")

	f.Fuzz(func(t *testing.T, input string) {
		_ = interpretEscapeSequences(input)
		// Don't check result - we're looking for crashes
	})
}
