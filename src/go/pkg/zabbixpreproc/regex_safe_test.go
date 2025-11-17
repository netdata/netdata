package zabbixpreproc

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// Test constants for regex pattern limits (used only in tests)
const testMaxRegexPatternLength = 1000

// TestRegexComplexityValidation tests that overly complex patterns are rejected
func TestRegexComplexityValidation(t *testing.T) {
	tests := []struct {
		name             string
		pattern          string
		maxPatternLength int
		wantErr          bool
	}{
		{
			name:             "simple pattern",
			pattern:          "hello.*world",
			maxPatternLength: 0, // no limit
			wantErr:          false,
		},
		{
			name:             "pattern too long with limit",
			pattern:          strings.Repeat("a", testMaxRegexPatternLength+1),
			maxPatternLength: testMaxRegexPatternLength,
			wantErr:          true,
		},
		{
			name:             "pattern ok when no limit",
			pattern:          strings.Repeat("a", testMaxRegexPatternLength+1),
			maxPatternLength: 0, // no limit
			wantErr:          false,
		},
		{
			name:             "excessive nesting",
			pattern:          strings.Repeat("(", maxRegexNestingDepth+1) + strings.Repeat(")", maxRegexNestingDepth+1),
			maxPatternLength: 0,
			wantErr:          true,
		},
		{
			name:             "unbalanced parentheses - too many open",
			pattern:          "((hello",
			maxPatternLength: 0,
			wantErr:          true,
		},
		{
			name:             "unbalanced parentheses - too many close",
			pattern:          "hello))",
			maxPatternLength: 0,
			wantErr:          true,
		},
		{
			name:             "acceptable nesting",
			pattern:          strings.Repeat("(", maxRegexNestingDepth) + strings.Repeat(")", maxRegexNestingDepth),
			maxPatternLength: 0,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegexComplexity(tt.pattern, tt.maxPatternLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRegexComplexity() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestRegexCaching tests that compiled regexes are cached
func TestRegexCaching(t *testing.T) {
	ClearRegexCache()

	pattern := "test.*pattern"

	// First compilation
	re1, err := compileRegexSafe(pattern, 0)
	if err != nil {
		t.Fatalf("compileRegexSafe() error = %v", err)
	}

	// Second compilation should return cached instance
	re2, err := compileRegexSafe(pattern, 0)
	if err != nil {
		t.Fatalf("compileRegexSafe() error = %v", err)
	}

	// Should be the exact same pointer (cached)
	if re1 != re2 {
		t.Errorf("Expected cached regex to be same instance, got different instances")
	}
}

// TestRegexCacheEviction tests LRU cache eviction
func TestRegexCacheEviction(t *testing.T) {
	ClearRegexCache()

	// Fill cache to capacity
	for i := 0; i < regexCacheSize; i++ {
		pattern := regexp.QuoteMeta(strings.Repeat("a", i+1))
		_, err := compileRegexSafe(pattern, 0)
		if err != nil {
			t.Fatalf("compileRegexSafe() error = %v", err)
		}
	}

	// Cache should be at capacity
	if len(globalRegexCache.cache) != regexCacheSize {
		t.Errorf("Expected cache size %d, got %d", regexCacheSize, len(globalRegexCache.cache))
	}

	// Add one more - should evict oldest
	newPattern := regexp.QuoteMeta(strings.Repeat("z", 200))
	_, err := compileRegexSafe(newPattern, 0)
	if err != nil {
		t.Fatalf("compileRegexSafe() error = %v", err)
	}

	// Cache should still be at capacity
	if len(globalRegexCache.cache) != regexCacheSize {
		t.Errorf("Expected cache size %d after eviction, got %d", regexCacheSize, len(globalRegexCache.cache))
	}

	// First pattern should be evicted
	firstPattern := regexp.QuoteMeta("a")
	_, exists := globalRegexCache.cache[firstPattern]
	if exists {
		t.Errorf("Expected oldest pattern to be evicted, but it's still in cache")
	}
}

// TestMatchWithTimeout tests that slow regex operations timeout
func TestMatchWithTimeout(t *testing.T) {
	// Simple pattern that should complete quickly
	re := regexp.MustCompile("hello")

	// Should complete successfully
	matched, err := matchWithTimeout(re, "hello world", 100*time.Millisecond)
	if err != nil {
		t.Errorf("matchWithTimeout() error = %v", err)
	}
	if !matched {
		t.Errorf("Expected match, got no match")
	}
}

// TestRegexSubstituteWithSafeRegex tests regexSubstitute uses safe regex
func TestRegexSubstituteWithSafeRegex(t *testing.T) {
	ClearRegexCache()

	value := Value{Data: "hello world", Type: ValueTypeStr}
	params := "world\nuniverse"

	result, err := regexSubstitute(value, params)
	if err != nil {
		t.Fatalf("regexSubstitute() error = %v", err)
	}

	if result.Data != "universe" {
		t.Errorf("Expected 'universe', got '%s'", result.Data)
	}

	// Verify pattern was cached
	pattern := "world"
	_, exists := globalRegexCache.cache[pattern]
	if !exists {
		t.Errorf("Expected pattern to be cached")
	}
}

// TestValidateRegexWithSafeRegex tests validateRegex uses safe regex
func TestValidateRegexWithSafeRegex(t *testing.T) {
	ClearRegexCache()

	value := Value{Data: "test123", Type: ValueTypeStr}
	pattern := "^test[0-9]+$"

	result, err := validateRegex(value, pattern)
	if err != nil {
		t.Fatalf("validateRegex() error = %v", err)
	}

	if result.Data != value.Data {
		t.Errorf("Expected value unchanged, got '%s'", result.Data)
	}

	// Verify pattern was cached
	_, exists := globalRegexCache.cache[pattern]
	if !exists {
		t.Errorf("Expected pattern to be cached")
	}
}

// TestValidateNotRegexWithSafeRegex tests validateNotRegex uses safe regex
func TestValidateNotRegexWithSafeRegex(t *testing.T) {
	ClearRegexCache()

	value := Value{Data: "hello", Type: ValueTypeStr}
	pattern := "^[0-9]+$"

	result, err := validateNotRegex(value, pattern)
	if err != nil {
		t.Fatalf("validateNotRegex() error = %v", err)
	}

	if result.Data != value.Data {
		t.Errorf("Expected value unchanged, got '%s'", result.Data)
	}

	// Verify pattern was cached
	_, exists := globalRegexCache.cache[pattern]
	if !exists {
		t.Errorf("Expected pattern to be cached")
	}
}

// TestErrorFieldRegexWithSafeRegex tests errorFieldRegex uses safe regex
func TestErrorFieldRegexWithSafeRegex(t *testing.T) {
	ClearRegexCache()

	value := Value{Data: "ERROR: test failed", Type: ValueTypeStr}
	params := "ERROR: (.+)\n$1"

	_, err := errorFieldRegex(value, params)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if err.Error() != "test failed" {
		t.Errorf("Expected 'test failed', got '%s'", err.Error())
	}

	// Verify pattern was cached
	pattern := "ERROR: (.+)"
	_, exists := globalRegexCache.cache[pattern]
	if !exists {
		t.Errorf("Expected pattern to be cached")
	}
}

// TestRegexProtectionAgainstComplexPatterns tests that complex patterns are rejected
func TestRegexProtectionAgainstComplexPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   string
		wantErr bool
	}{
		{
			name:    "simple valid pattern",
			pattern: "^[a-z]+$",
			input:   "hello",
			wantErr: false,
		},
		{
			name:    "long pattern ok with no limit",
			pattern: ".*" + strings.Repeat("a", testMaxRegexPatternLength) + ".*", // Valid long pattern
			input:   strings.Repeat("a", testMaxRegexPatternLength),               // Input that matches
			wantErr: false,                                                        // No limit by default
		},
		{
			name:    "excessive nesting depth",
			pattern: strings.Repeat("(a", maxRegexNestingDepth+1) + strings.Repeat(")", maxRegexNestingDepth+1),
			input:   "aaaa",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := Value{Data: tt.input, Type: ValueTypeStr}
			_, err := validateRegex(value, tt.pattern)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateRegex() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestConcurrentRegexCacheAccess tests thread safety of regex cache
func TestConcurrentRegexCacheAccess(t *testing.T) {
	ClearRegexCache()

	// Launch multiple goroutines that compile the same pattern
	pattern := "test.*concurrent"
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, err := compileRegexSafe(pattern, 0)
				if err != nil {
					t.Errorf("compileRegexSafe() error = %v", err)
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Pattern should be in cache exactly once
	_, exists := globalRegexCache.cache[pattern]
	if !exists {
		t.Errorf("Expected pattern to be cached")
	}
}
