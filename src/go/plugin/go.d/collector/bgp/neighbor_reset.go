// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

func parseNeighborResetDetails(neighbor frrNeighbor) neighborResetDetails {
	reset := neighborResetDetails{}

	lastReset := strings.TrimSpace(neighbor.LastReset)
	if lastReset != "" {
		reset.HasState = true
		reset.Never = strings.EqualFold(lastReset, "never")
	}
	if neighbor.LastHardReset != nil {
		reset.HasState = true
		reset.Hard = *neighbor.LastHardReset
	}
	if neighbor.DownLastResetAge != nil {
		reset.AgeSecs = maxInt64(*neighbor.DownLastResetAge, 0)
		reset.HasState = true
	} else if neighbor.LastResetTimer != nil {
		reset.AgeSecs = maxInt64(*neighbor.LastResetTimer/1000, 0)
		reset.HasState = true
	}

	if code, ok := parseJSONInt(neighbor.DownLastResetCode); ok {
		reset.ResetCode = code
		reset.HasResetCode = true
		reset.HasState = true
	} else if code, ok := parseJSONInt(neighbor.LastResetCode); ok {
		reset.ResetCode = code
		reset.HasResetCode = true
		reset.HasState = true
	}

	if code, subcode, okCode, okSub := parseLastErrorCodeSubcode(neighbor.LastError); okCode || okSub {
		reset.ErrorCode = code
		reset.ErrorSubcode = subcode
		reset.HasErrorCode = okCode
		reset.HasErrorSub = okSub
		reset.HasState = true
	}

	return reset
}

func parseLastErrorCodeSubcode(raw json.RawMessage) (code, subcode int64, okCode, okSub bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0, 0, false, false
	}

	var numeric []int64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		if len(numeric) > 0 {
			code, okCode = numeric[0], true
		}
		if len(numeric) > 1 {
			subcode, okSub = numeric[1], true
		}
		return code, subcode, okCode, okSub
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return parseCodePair(text)
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		if v, ok := object["code"]; ok {
			code, okCode = parseJSONInt(v)
		}
		if v, ok := object["subcode"]; ok {
			subcode, okSub = parseJSONInt(v)
		}
		if !okCode {
			if v, ok := object["errorCode"]; ok {
				code, okCode = parseJSONInt(v)
			}
		}
		if !okSub {
			if v, ok := object["errorSubcode"]; ok {
				subcode, okSub = parseJSONInt(v)
			}
		}
		return code, subcode, okCode, okSub
	}

	if value, ok := parseJSONInt(raw); ok {
		return value, 0, true, false
	}

	return 0, 0, false, false
}

func parseCodePair(value string) (code, subcode int64, okCode, okSub bool) {
	var numbers []int64
	var current strings.Builder

	flush := func() {
		if current.Len() == 0 {
			return
		}
		if n, err := strconv.ParseInt(current.String(), 10, 64); err == nil {
			numbers = append(numbers, n)
		}
		current.Reset()
	}

	for _, r := range value {
		if r >= '0' && r <= '9' {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()

	if len(numbers) > 0 {
		code, okCode = numbers[0], true
	}
	if len(numbers) > 1 {
		subcode, okSub = numbers[1], true
	}
	return code, subcode, okCode, okSub
}

func parseJSONInt(raw json.RawMessage) (int64, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0, false
	}

	var integer int64
	if err := json.Unmarshal(raw, &integer); err == nil {
		return integer, true
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			return n, true
		}
	}

	return 0, false
}
