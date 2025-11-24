package zabbixpreproc

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Numeric parsing constants
const (
	parseBase10    = 10 // Decimal base for integer parsing
	parseBase8     = 8  // Octal base for oct2dec
	parseBase16    = 16 // Hexadecimal base for hex2dec
	parseBitSize64 = 64 // 64-bit precision for float/int parsing
)

// formatFloat formats a float for output, avoiding scientific notation
func formatFloat(f float64) string {
	// If it's a whole number, format as integer
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	// Otherwise use %g which will choose reasonable formatting
	return fmt.Sprintf("%g", f)
}

// multiplyValue multiplies a numeric value by a multiplier.
func multiplyValue(value Value, paramStr string) (Value, error) {
	multiplier, err := strconv.ParseFloat(paramStr, parseBitSize64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid multiplier: %w", err)
	}

	var numValue float64
	// For UINT64 type, parse as integer first (truncate any decimals)
	if value.Type == ValueTypeUint64 {
		intVal, err := strconv.ParseInt(value.Data, 10, 64)
		if err != nil {
			// Try float, then truncate to uint64
			floatVal, err := strconv.ParseFloat(value.Data, 64)
			if err != nil {
				return Value{}, fmt.Errorf("invalid numeric value: %w", err)
			}
			intVal = int64(floatVal)
		}
		numValue = float64(intVal)
	} else {
		var err error
		numValue, err = strconv.ParseFloat(value.Data, 64)
		if err != nil {
			return Value{}, fmt.Errorf("invalid numeric value: %w", err)
		}
	}

	result := numValue * multiplier

	// Convert back based on value type
	switch value.Type {
	case ValueTypeUint64:
		// Banker's rounding (round half to even) like Zabbix
		floor := int64(result)
		frac := result - float64(floor)
		var rounded int64
		if frac < 0.5 {
			rounded = floor
		} else if frac > 0.5 {
			rounded = floor + 1
		} else {
			// Exactly 0.5 - round to even
			if floor%2 == 0 {
				rounded = floor
			} else {
				rounded = floor + 1
			}
		}
		return Value{Data: fmt.Sprintf("%d", rounded), Type: ValueTypeUint64}, nil
	case ValueTypeFloat:
		return Value{Data: formatFloat(result), Type: ValueTypeFloat}, nil
	default:
		// String type - return result as string
		if result == float64(int64(result)) {
			return Value{Data: fmt.Sprintf("%d", int64(result)), Type: ValueTypeStr}, nil
		}
		return Value{Data: formatFloat(result), Type: ValueTypeStr}, nil
	}
}

// trimValue trims specified characters from a value.
func trimValue(value Value, paramStr string, direction string) (Value, error) {
	chars := paramStr
	if chars == "" {
		chars = " \t\r\n" // Default whitespace
	} else {
		// Handle Zabbix escape sequences in params
		chars = interpretEscapeSequences(chars)
	}

	// Trim the value data
	result := value.Data
	switch direction {
	case "left":
		result = strings.TrimLeft(result, chars)
	case "right":
		result = strings.TrimRight(result, chars)
	case "both":
		result = strings.Trim(result, chars)
	}

	return Value{Data: result, Type: value.Type}, nil
}

// interpretEscapeSequences handles Zabbix escape sequences in trim/ltrim/rtrim params
// Handles both single backslash (\s) and double backslash (\\s) formats
func interpretEscapeSequences(s string) string {
	var result strings.Builder
	result.Grow(len(s)) // Pre-allocate capacity for efficiency

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]

			// Check for double-backslash escape sequences (\\s, \\n, etc.)
			// This is only for trim operations where \\s means full whitespace class
			if next == '\\' && i+2 < len(s) {
				third := s[i+2]
				switch third {
				case 's':
					// \\s means whitespace class: space, tab, newline, carriage return, and backslash
					result.WriteString(" \t\r\n\\")
					i += 2
					continue
				}
			}

			// Single backslash escape sequences
			switch next {
			case '\\':
				// Backslash-backslash is a literal backslash (consume both backslashes)
				result.WriteByte('\\')
				i++
				continue
			case 'n':
				result.WriteByte('\n')
				i++
				continue
			case 'r':
				result.WriteByte('\r')
				i++
				continue
			case 't':
				result.WriteByte('\t')
				i++
				continue
			case 's':
				result.WriteByte(' ')
				i++
				continue
			case ' ':
				// Backslash-space is an escape for space
				result.WriteByte(' ')
				i++
				continue
			default:
				// Not an escape sequence, just add the backslash
				result.WriteByte(s[i])
			}
		} else {
			result.WriteByte(s[i])
		}
	}
	return result.String()
}

// regexSubstitute performs regex find and replace.
func regexSubstitute(value Value, paramStr string) (Value, error) {
	// paramStr format: "pattern\noutput"
	parts := strings.SplitN(paramStr, "\n", 2)
	if len(parts) != 2 {
		return Value{}, fmt.Errorf("regex substitution requires pattern and output")
	}

	pattern := parts[0]
	output := parts[1]

	// Debug: log the parsed parts
	//fmt.Printf("DEBUG regsub: pattern='%s' output='%s'\n", pattern, output)

	// Compile regex with safety checks and caching
	re, err := compileRegexSafe(pattern, 0)
	if err != nil {
		return Value{}, fmt.Errorf("invalid regex pattern: %w", err)
	}

	// Check if there's a match - if no match, return error (with timeout protection)
	matched, err := matchWithTimeout(re, value.Data, defaultRegexTimeout)
	if err != nil {
		return Value{}, fmt.Errorf("regex match failed: %w", err)
	}
	if !matched {
		return Value{}, fmt.Errorf("regex pattern does not match")
	}

	// Find match with capture groups (with timeout protection)
	// Zabbix regsub returns only the replacement string (not modified original)
	matches, err := findStringSubmatchIndexWithTimeout(re, value.Data, defaultRegexTimeout)
	if err != nil {
		return Value{}, fmt.Errorf("regex substitution failed: %w", err)
	}
	if matches == nil {
		return Value{}, fmt.Errorf("regex pattern does not match")
	}

	// Convert Zabbix-style backreferences (\1, \2) to Go-style ($1, $2)
	// Zabbix uses \N syntax, but Go's regexp.Expand() uses $N syntax
	// Must preserve escaped backslashes (\\1 should become literal \1, not $1)
	goOutput := convertBackreferences(output)

	// Use regexp.Expand() for proper capture group expansion
	// This correctly handles:
	// - Multi-digit backreferences (${10}, ${11}, etc.)
	// - Escaped backslashes
	// - All edge cases (unmatched groups, nested references, etc.)
	var replacement []byte
	replacement = re.Expand(replacement, []byte(goOutput), []byte(value.Data), matches)

	// Return the replacement string (not the modified original string)
	return Value{Data: string(replacement), Type: value.Type}, nil
}

// convertBackreferences converts Zabbix-style \N to Go-style ${N}
// Preserves escaped backslashes: \\1 remains \1 (literal)
// Uses ${N} instead of $N to avoid ambiguity (e.g., $1y vs ${1}y)
func convertBackreferences(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '\\' {
			if s[i+1] == '\\' {
				// Escaped backslash: \\1 → \1 (literal)
				result.WriteByte('\\')
				i += 2
				continue
			} else if s[i+1] >= '0' && s[i+1] <= '9' {
				// Backreference: \1 → ${1}, \10 → ${10}, etc.
				result.WriteString("${")
				// Handle multi-digit backreferences (\10, \11, etc.)
				j := i + 1
				for j < len(s) && s[j] >= '0' && s[j] <= '9' {
					result.WriteByte(s[j])
					j++
				}
				result.WriteByte('}')
				i = j
				continue
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

// bool2Dec converts boolean values to decimal.
func bool2Dec(value Value) (Value, error) {
	// Empty input is an error
	if value.Data == "" {
		return Value{}, fmt.Errorf("empty input for bool2dec")
	}

	v := strings.ToLower(strings.TrimSpace(value.Data))

	// Zabbix considers these as true (full words and single letters)
	trueValues := map[string]bool{
		"1": true, "true": true, "yes": true, "on": true,
		"t": true, "y": true, "ok": true,
	}

	// Zabbix considers these as false (full words and single letters)
	falseValues := map[string]bool{
		"0": true, "false": true, "no": true, "off": true,
		"f": true, "n": true, "err": true, "error": true,
	}

	var result int64
	if trueValues[v] {
		result = 1
	} else if falseValues[v] {
		result = 0
	} else {
		// Try to parse as number
		num, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			// If not a recognized value and not a number, return error
			return Value{}, fmt.Errorf("unrecognized boolean value: %s", v)
		}
		if num != 0 {
			result = 1
		}
	}

	return Value{Data: fmt.Sprintf("%d", result), Type: ValueTypeUint64}, nil
}

// oct2Dec converts octal string to decimal.
func oct2Dec(value Value) (Value, error) {
	// Parse as octal
	num, err := strconv.ParseInt(strings.TrimSpace(value.Data), parseBase8, parseBitSize64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid octal value: %w", err)
	}

	return Value{Data: fmt.Sprintf("%d", num), Type: ValueTypeUint64}, nil
}

// hex2Dec converts hexadecimal string to decimal.
func hex2Dec(value Value) (Value, error) {
	// Parse as hexadecimal (with or without 0x prefix)
	data := strings.TrimSpace(value.Data)
	if strings.HasPrefix(data, "0x") || strings.HasPrefix(data, "0X") {
		data = data[2:]
	}

	// Remove spaces for parsing
	data = strings.ReplaceAll(data, " ", "")

	num, err := strconv.ParseInt(data, parseBase16, parseBitSize64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid hexadecimal value: %w", err)
	}

	return Value{Data: fmt.Sprintf("%d", num), Type: ValueTypeUint64}, nil
}

// deltaValue calculates the difference between current and previous value.
// Thread-safe for concurrent use. State isolated per shard and item.
func (p *Preprocessor) deltaValue(itemID string, value Value, paramStr string) (Value, error) {
	// State key: shardID:itemID:operation
	stateKey := fmt.Sprintf("%s:%s:delta_value", p.shardID, itemID)

	// Parse numeric value before acquiring lock
	numValue, err := strconv.ParseFloat(value.Data, 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid numeric value: %w", err)
	}

	// Acquire exclusive lock for read-modify-write operation
	p.mu.Lock()
	defer p.mu.Unlock()

	state, hasState := p.state[stateKey]

	if !hasState {
		// First call - no previous value
		p.state[stateKey] = &OperationState{
			LastValue:     value.Data,
			LastTimestamp: value.Timestamp,
			LastAccess:    time.Now(),
		}
		return Value{Data: "0", Type: ValueTypeStr}, nil
	}

	// Update access time
	state.LastAccess = time.Now()

	// Calculate delta
	prevValue, err := strconv.ParseFloat(state.LastValue, 64)
	if err != nil {
		// If we can't parse previous, reset and return 0
		p.state[stateKey] = &OperationState{
			LastValue:     value.Data,
			LastTimestamp: value.Timestamp,
			LastAccess:    time.Now(),
		}
		return Value{Data: "0", Type: ValueTypeStr}, nil
	}

	delta := numValue - prevValue

	// Update state
	p.state[stateKey] = &OperationState{
		LastValue:     value.Data,
		LastTimestamp: value.Timestamp,
		LastAccess:    time.Now(),
	}

	// Discard negative deltas (value decreased)
	if delta < 0 {
		return Value{Data: "", Type: ValueTypeStr}, nil
	}

	// Return delta
	if delta == float64(int64(delta)) {
		return Value{Data: fmt.Sprintf("%d", int64(delta)), Type: ValueTypeStr}, nil
	}
	return Value{Data: fmt.Sprintf("%g", delta), Type: ValueTypeStr}, nil
}

// deltaSpeed calculates the speed (delta per second).
// Thread-safe for concurrent use. State isolated per shard and item.
func (p *Preprocessor) deltaSpeed(itemID string, value Value, paramStr string) (Value, error) {
	// State key: shardID:itemID:operation
	stateKey := fmt.Sprintf("%s:%s:delta_speed", p.shardID, itemID)

	// Parse numeric value before acquiring lock
	numValue, err := strconv.ParseFloat(value.Data, 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid numeric value: %w", err)
	}

	// Acquire exclusive lock for read-modify-write operation
	p.mu.Lock()
	defer p.mu.Unlock()

	state, hasState := p.state[stateKey]

	if !hasState {
		// First call - no previous value
		p.state[stateKey] = &OperationState{
			LastValue:     value.Data,
			LastValueTime: value.Timestamp,
			LastTimestamp: value.Timestamp,
			LastAccess:    time.Now(),
		}
		return Value{Data: "0", Type: ValueTypeStr}, nil
	}

	// Update access time
	state.LastAccess = time.Now()

	// Calculate delta
	prevValue, err := strconv.ParseFloat(state.LastValue, 64)
	if err != nil {
		// If we can't parse previous, reset and return 0
		p.state[stateKey] = &OperationState{
			LastValue:     value.Data,
			LastValueTime: value.Timestamp,
			LastTimestamp: value.Timestamp,
			LastAccess:    time.Now(),
		}
		return Value{Data: "0", Type: ValueTypeStr}, nil
	}

	delta := numValue - prevValue
	timeDiff := value.Timestamp.Sub(state.LastValueTime).Seconds()

	// Update state
	p.state[stateKey] = &OperationState{
		LastValue:     value.Data,
		LastValueTime: value.Timestamp,
		LastTimestamp: value.Timestamp,
		LastAccess:    time.Now(),
	}

	// Discard if time went backwards or stayed the same
	if timeDiff <= 0 {
		return Value{Data: "", Type: ValueTypeStr}, nil
	}

	// Discard if value decreased (negative delta)
	if delta < 0 {
		return Value{Data: "", Type: ValueTypeStr}, nil
	}

	speed := delta / timeDiff

	// Format based on input value type
	if value.Type == ValueTypeUint64 {
		// For UINT64, truncate to integer
		return Value{Data: fmt.Sprintf("%d", int64(speed)), Type: ValueTypeStr}, nil
	}

	// For other types, use float formatting
	if speed == float64(int64(speed)) {
		return Value{Data: fmt.Sprintf("%d", int64(speed)), Type: ValueTypeStr}, nil
	}
	return Value{Data: fmt.Sprintf("%g", speed), Type: ValueTypeStr}, nil
}

// stringReplace replaces one string with another.
func stringReplace(value Value, paramStr string) (Value, error) {
	// paramStr format: "search\nreplace"
	parts := strings.SplitN(paramStr, "\n", 2)
	if len(parts) != 2 {
		return Value{}, fmt.Errorf("string replace requires search and replace parameters")
	}

	// Interpret escape sequences in search and replace strings
	search := interpretEscapeSequences(parts[0])
	replace := interpretEscapeSequences(parts[1])

	// Empty search string is not allowed
	if search == "" {
		return Value{}, fmt.Errorf("search string cannot be empty")
	}

	result := strings.ReplaceAll(value.Data, search, replace)
	return Value{Data: result, Type: value.Type}, nil
}

// validateRange validates that a value is within a numeric range.
func validateRange(value Value, paramStr string) (Value, error) {
	// paramStr format: "min\nmax"
	parts := strings.SplitN(paramStr, "\n", 2)
	if len(parts) != 2 {
		return Value{}, fmt.Errorf("validate range requires min and max parameters")
	}

	minStr := strings.TrimSpace(parts[0])
	maxStr := strings.TrimSpace(parts[1])

	numValue, err := strconv.ParseFloat(value.Data, 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid numeric value: %w", err)
	}

	min, err := strconv.ParseFloat(minStr, 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid minimum value: %w", err)
	}

	max, err := strconv.ParseFloat(maxStr, 64)
	if err != nil {
		return Value{}, fmt.Errorf("invalid maximum value: %w", err)
	}

	if numValue < min || numValue > max {
		return Value{}, fmt.Errorf("value %g is out of range [%g, %g]", numValue, min, max)
	}

	return value, nil
}

// validateRegex validates that a value matches a regex pattern.
func validateRegex(value Value, paramStr string) (Value, error) {
	pattern := strings.TrimSpace(paramStr)
	if pattern == "" {
		return Value{}, fmt.Errorf("validate regex requires a pattern")
	}

	re, err := compileRegexSafe(pattern, 0)
	if err != nil {
		return Value{}, fmt.Errorf("invalid regex pattern: %w", err)
	}

	matched, err := matchWithTimeout(re, value.Data, defaultRegexTimeout)
	if err != nil {
		return Value{}, fmt.Errorf("regex validation failed: %w", err)
	}
	if !matched {
		return Value{}, fmt.Errorf("value does not match regex pattern: %s", pattern)
	}

	return value, nil
}

// validateNotRegex validates that a value does NOT match a regex pattern.
func validateNotRegex(value Value, paramStr string) (Value, error) {
	pattern := strings.TrimSpace(paramStr)
	if pattern == "" {
		return Value{}, fmt.Errorf("validate not regex requires a pattern")
	}

	re, err := compileRegexSafe(pattern, 0)
	if err != nil {
		return Value{}, fmt.Errorf("invalid regex pattern: %w", err)
	}

	matched, err := matchWithTimeout(re, value.Data, defaultRegexTimeout)
	if err != nil {
		return Value{}, fmt.Errorf("regex validation failed: %w", err)
	}
	if matched {
		return Value{}, fmt.Errorf("value matches regex pattern but should not: %s", pattern)
	}

	return value, nil
}

// validateNotSupported checks if an error from a previous step is "not supported".
// - If input is a VALUE (not an error): pass through unchanged (succeed)
// - If input is an ERROR:
//   - If params is empty: fail (any error is considered "supported")
//   - If params has pattern: check if error matches pattern
//   - Matches: fail (error is "supported"/expected)
//   - Doesn't match: succeed (error is "not supported"/unexpected)
func validateNotSupported(value Value, paramStr string) (Value, error) {
	// If input is not an error, pass through unchanged
	if !value.IsError {
		return value, nil
	}

	// Input is an error - validate it
	paramStr = strings.TrimSpace(paramStr)

	// If no params provided, any error is considered "supported" - fail
	if paramStr == "" {
		return Value{}, fmt.Errorf("value is not supported")
	}

	// Parse params: "mode\npattern"
	parts := strings.SplitN(paramStr, "\n", 2)
	if len(parts) != 2 {
		// Invalid params format, treat as "any error is supported"
		return Value{}, fmt.Errorf("value is not supported")
	}

	// mode := strings.TrimSpace(parts[0]) // Mode not currently used
	pattern := strings.TrimSpace(parts[1])

	// Empty pattern means any error is supported
	if pattern == "" {
		return Value{}, fmt.Errorf("value is not supported")
	}

	// Check if error message matches pattern (regex match)
	re, err := compileRegexSafe(pattern, 0)
	if err != nil {
		// Invalid regex pattern - treat as validation failure
		return Value{}, fmt.Errorf("invalid pattern in validation: %w", err)
	}

	matched, err := matchWithTimeout(re, value.Data, defaultRegexTimeout)
	if err != nil {
		// Regex timeout or error
		return Value{}, fmt.Errorf("pattern matching failed: %w", err)
	}

	if matched {
		// Error matches pattern - it's "supported"/expected, so fail
		return Value{}, fmt.Errorf("value is not supported")
	}

	// Error doesn't match pattern - it's "not supported"/unexpected, so succeed
	// Pass through the error data unchanged
	return value, nil
}
