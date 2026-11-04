// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

// Raw-request input names read by RawParams.TimeRange and RawParams.QueryMatcher.
// Methods advertise them with FunctionConfig.AcceptedParams.
const (
	ParamAfter  = "after"
	ParamBefore = "before"
	ParamQuery  = "query"
)

// Accepted Unix seconds: years 1 to 9999, the RFC 3339 range. The bound also keeps
// relative-time arithmetic far from int64 overflow.
const (
	minUnixSeconds = -62135596800 // 0001-01-01T00:00:00Z
	maxUnixSeconds = 253402300799 // 9999-12-31T23:59:59Z
)

// RawParams holds the named inputs of a raw request.
type RawParams struct {
	values map[string]json.RawMessage
}

// ParseRawParams collects the named inputs of a raw request. A name takes its value from the first source
// that has it: "key:value" or "key=value" arguments (the last duplicate wins), then the payload "selections"
// object, then top-level payload fields. Arguments without a separator, such as "info", are ignored.
// A non-empty payload must be a JSON object.
func ParseRawParams(req RawMethodRequest) (RawParams, error) {
	values := make(map[string]json.RawMessage)
	for _, arg := range req.Args {
		key, value, ok := strings.Cut(arg, ":")
		if !ok {
			key, value, ok = strings.Cut(arg, "=")
		}
		if ok {
			values[key], _ = json.Marshal(value)
		}
	}
	if len(req.Payload) != 0 {
		var payload map[string]json.RawMessage
		if json.Unmarshal(req.Payload, &payload) != nil || payload == nil {
			return RawParams{}, errors.New("expected a JSON object")
		}
		if raw, ok := payload["selections"]; ok {
			var selections map[string]json.RawMessage
			if json.Unmarshal(raw, &selections) != nil {
				return RawParams{}, errors.New("invalid selections")
			}
			for k, v := range selections {
				if _, exists := values[k]; !exists {
					values[k] = v
				}
			}
		}
		for k, v := range payload {
			if _, exists := values[k]; !exists {
				values[k] = v
			}
		}
	}
	return RawParams{values: values}, nil
}

// String returns the string value of key, or "" when key is absent.
// A one-element string array, the form of a single selection, is accepted.
func (p RawParams) String(key string) (string, error) {
	raw, ok := p.values[key]
	if !ok {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil && string(raw) != "null" {
		return text, nil
	}
	var selected []string
	if json.Unmarshal(raw, &selected) == nil && len(selected) == 1 {
		return selected[0], nil
	}
	return "", fmt.Errorf("%s requires one string value", key)
}

// TimeRange returns the after and before inputs as Unix seconds, each nil when absent.
// Inputs are integer seconds, as JSON numbers or strings. Relative values follow the query API convention:
// before <= 0 is relative to now, and a negative after is relative to before (or to now without before).
func (p RawParams) TimeRange(now time.Time) (after, before *int64, err error) {
	if after, err = p.unixSeconds(ParamAfter); err != nil {
		return nil, nil, err
	}
	if before, err = p.unixSeconds(ParamBefore); err != nil {
		return nil, nil, err
	}
	if before != nil && *before <= 0 {
		*before += now.Unix()
	}
	if after != nil && *after < 0 {
		end := now.Unix()
		if before != nil {
			end = *before
		}
		*after += end
	}
	if after != nil && before != nil && *after > *before {
		return nil, nil, errors.New("after must not exceed before")
	}
	return after, before, nil
}

// QueryMatcher returns a matcher for the query input in simple-pattern syntax,
// or nil when the query is absent or blank.
func (p RawParams) QueryMatcher() (matcher.Matcher, error) {
	pattern, err := p.String(ParamQuery)
	if err != nil || strings.TrimSpace(pattern) == "" {
		return nil, err
	}
	m, err := matcher.NewSimplePatternsMatcher(pattern)
	if err != nil {
		return nil, errors.New("invalid simple-pattern query")
	}
	return m, nil
}

func (p RawParams) unixSeconds(key string) (*int64, error) {
	raw, ok := p.values[key]
	if !ok {
		return nil, nil
	}
	text := string(raw)
	if strings.HasPrefix(text, `"`) {
		if json.Unmarshal(raw, &text) != nil {
			return nil, fmt.Errorf("invalid %s time", key)
		}
	}
	seconds, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be integer Unix seconds or relative seconds", key)
	}
	if seconds < minUnixSeconds || seconds > maxUnixSeconds {
		return nil, fmt.Errorf("%s is outside the supported timestamp range", key)
	}
	return &seconds, nil
}
