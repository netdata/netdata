package zabbixpreproc

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/theory/jsonpath"
)

// JSON Unicode escape sequence constants
const (
	unicodeEscapeLen      = 6      // Length of \uXXXX sequence
	surrogatePairLen      = 12     // Length of surrogate pair \uXXXX\uYYYY
	unicodeHexDigits      = 4      // Number of hex digits in \uXXXX
	unicodeHexBase        = 16     // Base for parsing hex codes
	unicodeHexBitSize     = 16     // Bit size for ParseUint
	unicodeSurrogateStart = 0xD800 // Start of surrogate range
	unicodeSurrogateEnd   = 0xDFFF // End of surrogate range
	unicodeHighSurroStart = 0xD800 // Start of high surrogate range
	unicodeHighSurroEnd   = 0xDBFF // End of high surrogate range
	unicodeLowSurroStart  = 0xDC00 // Start of low surrogate range
	unicodeLowSurroEnd    = 0xDFFF // End of low surrogate range
)

// validateSurrogatePairs checks for unpaired UTF-16 surrogates in JSON string.
// Returns error if any unpaired surrogates are found.
func validateSurrogatePairs(jsonStr string) error {
	i := 0
	for i < len(jsonStr) {
		// Find next \u escape sequence
		if jsonStr[i] == '\\' && i+unicodeEscapeLen-1 < len(jsonStr) && jsonStr[i+1] == 'u' {
			// Parse the 4-digit hex code
			hexCode := jsonStr[i+2 : i+2+unicodeHexDigits]
			code, err := strconv.ParseUint(hexCode, unicodeHexBase, unicodeHexBitSize)
			if err != nil {
				// Invalid hex code, but let json.Unmarshal handle it
				i++
				continue
			}

			// Check if this is a surrogate
			if code >= unicodeSurrogateStart && code <= unicodeSurrogateEnd {
				// This is a surrogate
				if code >= unicodeHighSurroStart && code <= unicodeHighSurroEnd {
					// High surrogate - must be followed by low surrogate
					if i+surrogatePairLen < len(jsonStr) && jsonStr[i+6:i+8] == "\\u" {
						nextHex := jsonStr[i+8 : i+8+unicodeHexDigits]
						nextCode, err := strconv.ParseUint(nextHex, unicodeHexBase, unicodeHexBitSize)
						if err == nil && nextCode >= unicodeLowSurroStart && nextCode <= unicodeLowSurroEnd {
							// Valid surrogate pair, skip both
							i += surrogatePairLen
							continue
						}
					}
					// High surrogate without low surrogate = invalid
					return fmt.Errorf("invalid JSON: unpaired high surrogate")
				} else {
					// Low surrogate without preceding high surrogate = invalid
					return fmt.Errorf("invalid JSON: unpaired low surrogate")
				}
			}

			i += unicodeEscapeLen // Skip this \uXXXX sequence
		} else {
			i++
		}
	}
	return nil
}

// jsonpathExtract extracts values from JSON using JSONPath expression.
// jsonPathMulti extracts values using JSONPath and returns multiple metrics for arrays
func jsonPathMulti(value Value, paramStr string) (Result, error) {
	expr := strings.TrimSpace(paramStr)
	if expr == "" {
		err := fmt.Errorf("jsonpath expression is required")
		return Result{Error: err}, err
	}

	// Validate surrogate pairs before parsing
	if err := validateSurrogatePairs(value.Data); err != nil {
		return Result{Error: err}, err
	}

	// Parse JSON
	var data interface{}
	err := json.Unmarshal([]byte(value.Data), &data)
	if err != nil {
		err = fmt.Errorf("invalid JSON: %w", err)
		return Result{Error: err}, err
	}

	// Compile and execute JSONPath expression
	jp, err := jsonpath.Parse(expr)
	if err != nil {
		err = fmt.Errorf("invalid jsonpath expression: %w", err)
		return Result{Error: err}, err
	}

	results := jp.Select(data)
	if len(results) == 0 {
		err = fmt.Errorf("jsonpath expression did not match any values")
		return Result{Error: err}, err
	}

	// If single result and not an array, return single metric
	if len(results) == 1 {
		// Check if the single result is an array
		if arr, ok := results[0].([]interface{}); ok {
			// It's an array - return each element as separate metric
			metrics := make([]Metric, 0, len(arr))
			for i, item := range arr {
				metrics = append(metrics, Metric{
					Name:   "item",
					Value:  marshalJSONValue(item),
					Type:   ValueTypeStr,
					Labels: map[string]string{"index": fmt.Sprintf("%d", i)},
				})
			}
			return Result{Metrics: metrics}, nil
		}
		// Single non-array result - return as single metric
		return Result{
			Metrics: []Metric{{
				Name:  "",
				Value: marshalJSONValue(results[0]),
				Type:  ValueTypeStr,
			}},
		}, nil
	}

	// Multiple results from JSONPath - return each as separate metric
	metrics := make([]Metric, 0, len(results))
	for i, result := range results {
		metrics = append(metrics, Metric{
			Name:   "item",
			Value:  marshalJSONValue(result),
			Type:   ValueTypeStr,
			Labels: map[string]string{"index": fmt.Sprintf("%d", i)},
		})
	}

	return Result{Metrics: metrics}, nil
}

// marshalJSONValue converts a value to JSON string, handling strings specially
func marshalJSONValue(v interface{}) string {
	// If it's already a string, return it directly without JSON encoding
	if s, ok := v.(string); ok {
		return s
	}
	// For non-string values, encode as JSON
	b, _ := json.Marshal(v)
	return string(b)
}

func jsonpathExtract(value Value, paramStr string) (Value, error) {
	expr := strings.TrimSpace(paramStr)
	if expr == "" {
		return Value{}, fmt.Errorf("jsonpath expression is required")
	}

	// Validate surrogate pairs before parsing
	if err := validateSurrogatePairs(value.Data); err != nil {
		return Value{}, err
	}

	// Parse JSON
	var data interface{}
	err := json.Unmarshal([]byte(value.Data), &data)
	if err != nil {
		return Value{}, fmt.Errorf("invalid JSON: %w", err)
	}

	// Compile and execute JSONPath expression
	jp, err := jsonpath.Parse(expr)
	if err != nil {
		return Value{}, fmt.Errorf("invalid jsonpath expression: %w", err)
	}

	results := jp.Select(data)
	if len(results) == 0 {
		return Value{}, fmt.Errorf("jsonpath expression did not match any values")
	}

	// Return the first result
	result := results[0]

	// If result is a string, return it directly without JSON encoding
	if s, ok := result.(string); ok {
		return Value{Data: s, Type: ValueTypeStr}, nil
	}

	// For non-string results, encode as JSON
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return Value{}, fmt.Errorf("failed to marshal result: %w", err)
	}

	return Value{Data: string(resultBytes), Type: ValueTypeStr}, nil
}
