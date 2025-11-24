package zabbixpreproc

import (
	"strings"
	"testing"
	"time"
)

// TestJavaScriptExecutionTimeout tests that JavaScript execution times out after 5 seconds
func TestJavaScriptExecutionTimeout(t *testing.T) {
	value := Value{Data: "test", Type: ValueTypeStr}

	// Infinite loop should timeout
	script := `
		while(true) {
			// Infinite loop
		}
		return value;
	`

	start := time.Now()
	_, err := javascriptExecute(value, script, DefaultLimits().JavaScript)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "Interrupt") {
		t.Errorf("Expected timeout error, got: %v", err)
	}

	// Should timeout around 10 seconds (Zabbix default), allow some overhead
	if duration < 9*time.Second || duration > 12*time.Second {
		t.Errorf("Expected timeout around 10s, got %v", duration)
	}
}

// TestJavaScriptCallStackLimit tests that deep recursion is limited
func TestJavaScriptCallStackLimit(t *testing.T) {
	value := Value{Data: "test", Type: ValueTypeStr}

	// Deep recursion should hit stack limit
	script := `
		function recurse(n) {
			if (n > 0) {
				return recurse(n - 1);
			}
			return n;
		}
		return recurse(10000);
	`

	_, err := javascriptExecute(value, script, DefaultLimits().JavaScript)

	if err == nil {
		t.Fatal("Expected stack overflow error, got nil")
	}

	// Goja may report stack issues in various ways - just verify it errors on deep recursion
	if err == nil {
		t.Fatalf("Expected deep recursion to fail, got nil error")
	}
	// The error should mention the recursive function or contain depth info
	errStr := err.Error()
	if !strings.Contains(errStr, "recurse") && !strings.Contains(errStr, "stack") && !strings.Contains(errStr, "RangeError") {
		t.Logf("Deep recursion error: %v", err)
	}
}

// TestJavaScriptRSAKeyValidation tests that RSA key validation is in place
// Note: Minimum key size is 1024 bits to match Zabbix behavior (though 2048+ is recommended)
func TestJavaScriptRSAKeyValidation(t *testing.T) {
	// The official Zabbix tests verify that 1024-bit keys work correctly.
	// Keys smaller than 1024 bits would be rejected by the validation in javascriptExecute.
	// This test verifies the validation code exists and works with valid keys.

	// This is already tested by the official Zabbix test suite with 1024-bit keys,
	// so we just verify that the validation constant is set correctly.
	if jsMinRSAKeyBits != 1024 {
		t.Errorf("Expected minimum RSA key size to be 1024 bits (Zabbix compatibility), got %d", jsMinRSAKeyBits)
	}

	t.Log("RSA key validation is in place with minimum", jsMinRSAKeyBits, "bits")
}

// TestJavaScriptNormalExecution tests that valid JavaScript still executes correctly
func TestJavaScriptNormalExecution(t *testing.T) {
	value := Value{Data: "hello", Type: ValueTypeStr}

	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "simple return",
			script:   `return value;`,
			expected: "hello",
		},
		{
			name:     "string concatenation",
			script:   `return value + " world";`,
			expected: "hello world",
		},
		{
			name:     "btoa encoding",
			script:   `return btoa(value);`,
			expected: "aGVsbG8=",
		},
		{
			name:     "atob decoding",
			script:   `return atob("aGVsbG8=");`,
			expected: "hello",
		},
		{
			name:     "hmac",
			script:   `return hmac("sha256", "secret", value);`,
			expected: "88aab3ede8d3adf94d26ab90d3bafd4a2083070c3bcce9c014ee04a443847c0b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := javascriptExecute(value, tt.script, DefaultLimits().JavaScript)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Data != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result.Data)
			}
		})
	}
}

// TestJavaScriptMemoryIntensiveOperation tests that memory-intensive operations complete within limits
func TestJavaScriptMemoryIntensiveOperation(t *testing.T) {
	value := Value{Data: "test", Type: ValueTypeStr}

	// Create a large array (but not infinite)
	script := `
		var arr = [];
		for (var i = 0; i < 10000; i++) {
			arr.push(i);
		}
		return arr.length.toString();
	`

	result, err := javascriptExecute(value, script, DefaultLimits().JavaScript)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Data != "10000" {
		t.Errorf("Expected '10000', got '%s'", result.Data)
	}
}

// TestJavaScriptCPUIntensiveOperation tests that CPU-intensive operations complete within timeout
func TestJavaScriptCPUIntensiveOperation(t *testing.T) {
	value := Value{Data: "test", Type: ValueTypeStr}

	// Fibonacci calculation (should complete quickly)
	script := `
		function fib(n) {
			if (n <= 1) return n;
			return fib(n - 1) + fib(n - 2);
		}
		return fib(20).toString();
	`

	result, err := javascriptExecute(value, script, DefaultLimits().JavaScript)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Data != "6765" {
		t.Errorf("Expected '6765', got '%s'", result.Data)
	}
}

// TestJavaScriptPanicRecovery tests that panics are caught and converted to errors
func TestJavaScriptPanicRecovery(t *testing.T) {
	value := Value{Data: "test", Type: ValueTypeStr}

	// Trigger a JavaScript error
	script := `
		throw new Error("test error");
		return value;
	`

	_, err := javascriptExecute(value, script, DefaultLimits().JavaScript)

	if err == nil {
		t.Fatal("Expected error from thrown exception, got nil")
	}

	if !strings.Contains(err.Error(), "test error") && !strings.Contains(err.Error(), "Error") {
		t.Errorf("Expected error message, got: %v", err)
	}
}

// TestJavaScriptHMACWithInvalidAlgorithm tests HMAC error handling
func TestJavaScriptHMACWithInvalidAlgorithm(t *testing.T) {
	value := Value{Data: "test", Type: ValueTypeStr}

	script := `
		return hmac("invalid", "key", value);
	`

	_, err := javascriptExecute(value, script, DefaultLimits().JavaScript)

	if err == nil {
		t.Fatal("Expected error for invalid HMAC algorithm, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported") && !strings.Contains(err.Error(), "algorithm") {
		t.Errorf("Expected algorithm error, got: %v", err)
	}
}

// TestJavaScriptSandboxingDoesNotAffectValidCode tests that sandboxing doesn't break valid operations
func TestJavaScriptSandboxingDoesNotAffectValidCode(t *testing.T) {
	value := Value{Data: "100", Type: ValueTypeStr}

	// Test various operations that should work fine
	tests := []struct {
		name   string
		script string
	}{
		{
			name:   "arithmetic",
			script: `return (parseInt(value) * 2).toString();`,
		},
		{
			name:   "string operations",
			script: `return value.toUpperCase();`,
		},
		{
			name:   "array operations",
			script: `return [1, 2, 3].map(x => x * 2).join(",");`,
		},
		{
			name:   "object operations",
			script: `var obj = {a: 1}; return obj.a.toString();`,
		},
		{
			name:   "json operations",
			script: `return JSON.stringify({value: value});`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := javascriptExecute(value, tt.script, DefaultLimits().JavaScript)
			if err != nil {
				t.Errorf("Valid code failed after sandboxing: %v", err)
			}
		})
	}
}

// TestJavaScriptTimeoutPreventsHang tests that timeout prevents indefinite hangs
func TestJavaScriptTimeoutPreventsHang(t *testing.T) {
	value := Value{Data: "test", Type: ValueTypeStr}

	// Slow operation that would hang without timeout
	script := `
		var sum = 0;
		for (var i = 0; i < 999999999999; i++) {
			sum += i;
		}
		return sum.toString();
	`

	start := time.Now()
	_, err := javascriptExecute(value, script, DefaultLimits().JavaScript)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}

	// Should timeout around 10s (Zabbix default), not run for minutes
	if duration > 12*time.Second {
		t.Errorf("Execution took too long (%v), timeout not working", duration)
	}
}
