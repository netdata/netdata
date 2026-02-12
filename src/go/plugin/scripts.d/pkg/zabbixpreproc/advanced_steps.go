package zabbixpreproc

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/antchfx/xmlquery"
	"github.com/theory/jsonpath"
)

// errorFieldJSON extracts error message from JSON response.
func errorFieldJSON(value Value, paramStr string) (Value, error) {
	expr := strings.TrimSpace(paramStr)
	if expr == "" {
		return Value{}, fmt.Errorf("jsonpath expression required for error field extraction")
	}

	var data interface{}
	err := json.Unmarshal([]byte(value.Data), &data)
	if err != nil {
		// Invalid JSON - return input unchanged
		return value, nil
	}

	jp, err := jsonpath.Parse(expr)
	if err != nil {
		return Value{}, fmt.Errorf("invalid jsonpath: %w", err)
	}

	results := jp.Select(data)
	if len(results) == 0 {
		// No error field found - return input unchanged
		return value, nil
	}

	// Error field exists - return error with the field's value
	errValue := fmt.Sprintf("%v", results[0])
	return Value{}, fmt.Errorf("%s", errValue)
}

// errorFieldXML extracts error message from XML response.
func errorFieldXML(value Value, paramStr string) (Value, error) {
	expr := strings.TrimSpace(paramStr)
	if expr == "" {
		return Value{}, fmt.Errorf("xpath expression required for error field extraction")
	}

	doc, err := xmlquery.Parse(strings.NewReader(value.Data))
	if err != nil {
		// Invalid XML - return input unchanged
		return value, nil
	}

	nodes, err := xmlquery.QueryAll(doc, expr)
	if err != nil {
		return Value{}, fmt.Errorf("invalid xpath: %w", err)
	}

	if len(nodes) == 0 {
		// No error field found - return input unchanged
		return value, nil
	}

	// Error field exists - return error with the field's text value
	errText := nodes[0].InnerText()
	return Value{}, fmt.Errorf("%s", errText)
}

// errorFieldRegex extracts error message from regex match.
func errorFieldRegex(value Value, paramStr string) (Value, error) {
	// paramStr format: "pattern\noutput"
	parts := strings.SplitN(paramStr, "\n", 2)
	if len(parts) < 1 {
		return Value{}, fmt.Errorf("regex pattern required for error field extraction")
	}

	pattern := parts[0]
	output := ""
	if len(parts) > 1 {
		output = parts[1]
	}

	re, err := compileRegexSafe(pattern, 0)
	if err != nil {
		return Value{}, fmt.Errorf("invalid regex: %w", err)
	}

	matches, err := findStringSubmatchWithTimeout(re, value.Data, defaultRegexTimeout)
	if err != nil {
		return Value{}, fmt.Errorf("regex error field extraction failed: %w", err)
	}
	if len(matches) == 0 {
		// No match - no error
		return value, nil
	}

	// Match found - return error
	errMsg := output
	if errMsg == "" {
		errMsg = matches[0]
	}

	// Replace capture groups in output
	for i, match := range matches {
		placeholder := fmt.Sprintf("$%d", i)
		errMsg = strings.ReplaceAll(errMsg, placeholder, match)
	}

	return Value{}, fmt.Errorf("%s", errMsg)
}

// throttleValue returns the value only if it changed from the previous value.
// Thread-safe for concurrent use. State isolated per shard and item.
func (p *Preprocessor) throttleValue(itemID string, value Value, paramStr string) (Value, error) {
	// State key: shardID:itemID:operation
	stateKey := fmt.Sprintf("%s:%s:throttle_value", p.shardID, itemID)

	// Acquire exclusive lock for read-modify-write operation
	p.mu.Lock()
	defer p.mu.Unlock()

	state, hasState := p.state[stateKey]

	// Always update state before returning
	defer func() {
		p.state[stateKey] = &OperationState{
			LastValue:     value.Data,
			LastTimestamp: value.Timestamp,
			LastAccess:    time.Now(),
		}
	}()

	if !hasState {
		// First call
		return value, nil
	}

	// Update access time
	state.LastAccess = time.Now()

	// Check if value changed
	if value.Data != state.LastValue {
		// Value changed - return it
		return value, nil
	}

	// Value didn't change - discard it (return empty value without error)
	return Value{Data: "", Type: value.Type}, nil
}

// throttleTimedValue returns the value only if it changed or enough time has passed.
// Thread-safe for concurrent use. State isolated per shard and item.
func (p *Preprocessor) throttleTimedValue(itemID string, value Value, paramStr string) (Value, error) {
	// State key: shardID:itemID:operation
	stateKey := fmt.Sprintf("%s:%s:throttle_timed", p.shardID, itemID)

	// Parse timeout parameter before acquiring lock
	timeoutSeconds := parseDuration(paramStr)

	// Acquire exclusive lock for read-modify-write operation
	p.mu.Lock()
	defer p.mu.Unlock()

	state, hasState := p.state[stateKey]

	if !hasState {
		// First call - record state and return value
		p.state[stateKey] = &OperationState{
			LastValue:     value.Data,
			LastValueTime: value.Timestamp,
			LastAccess:    time.Now(),
		}
		return value, nil
	}

	// Update access time
	state.LastAccess = time.Now()

	// Check if value changed
	if value.Data != state.LastValue {
		// Value changed - return immediately and update state
		p.state[stateKey] = &OperationState{
			LastValue:     value.Data,
			LastValueTime: value.Timestamp,
			LastAccess:    time.Now(),
		}
		return value, nil
	}

	// Value unchanged - check if enough time has passed
	timeDiff := value.Timestamp.Sub(state.LastValueTime).Seconds()

	if timeDiff >= timeoutSeconds {
		// Enough time passed - return value and update timestamp
		p.state[stateKey] = &OperationState{
			LastValue:     value.Data,
			LastValueTime: value.Timestamp,
			LastAccess:    time.Now(),
		}
		return value, nil
	}

	// Value unchanged and not enough time passed - discard
	return Value{Data: "", Type: value.Type}, nil
}

// parseDuration parses Zabbix duration format (60, 1m, 1h, 1d, 1w) to seconds
func parseDuration(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 60 // Default 1 minute
	}

	// Check for suffix
	var multiplier float64 = 1
	if len(s) > 0 {
		lastChar := s[len(s)-1]
		switch lastChar {
		case 's':
			multiplier = 1
			s = s[:len(s)-1]
		case 'm':
			multiplier = 60
			s = s[:len(s)-1]
		case 'h':
			multiplier = 3600
			s = s[:len(s)-1]
		case 'd':
			multiplier = 86400
			s = s[:len(s)-1]
		case 'w':
			multiplier = 604800
			s = s[:len(s)-1]
		}
	}

	// Parse numeric value
	var value float64
	fmt.Sscanf(s, "%f", &value)
	if value == 0 {
		return 60 // Default if parsing failed
	}

	return value * multiplier
}
