// SPDX-License-Identifier: GPL-3.0-or-later

package ipmifunc

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

const (
	paramState  = "state"
	paramAfter  = "after"
	paramBefore = "before"
	paramQuery  = "query"
	stateAll    = "all"
)

type selQuery struct {
	state         string // empty or stateAll selects every state
	after, before *int64 // Unix seconds, inclusive
	text          matcher.Matcher
}

// parseSELQuery reads the request inputs with the framework's precedence:
// arguments, then payload selections, then top-level payload fields.
func parseSELQuery(req funcapi.RawMethodRequest, now time.Time) (selQuery, error) {
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
			return selQuery{}, errors.New("expected a JSON object")
		}
		if raw, ok := payload["selections"]; ok {
			var selections map[string]json.RawMessage
			if json.Unmarshal(raw, &selections) != nil {
				return selQuery{}, errors.New("invalid selections")
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

	var q selQuery
	var err error
	if q.state, err = stringValue(values, paramState); err != nil {
		return q, err
	}
	if q.state != "" && q.state != stateAll && !slices.Contains(selStates, q.state) {
		return q, errors.New("invalid state selection")
	}
	if q.after, err = secondsValue(values, paramAfter); err != nil {
		return q, err
	}
	if q.before, err = secondsValue(values, paramBefore); err != nil {
		return q, err
	}
	// Function relative times are seconds: before is relative to now, after to before.
	if q.before != nil && *q.before <= 0 {
		*q.before += now.Unix()
	}
	if q.after != nil && *q.after < 0 {
		end := now.Unix()
		if q.before != nil {
			end = *q.before
		}
		*q.after += end
	}
	if q.after != nil && q.before != nil && *q.after > *q.before {
		return q, errors.New("after must not exceed before")
	}
	pattern, err := stringValue(values, paramQuery)
	if err != nil {
		return q, err
	}
	if strings.TrimSpace(pattern) != "" {
		if q.text, err = matcher.NewSimplePatternsMatcher(pattern); err != nil {
			return q, errors.New("invalid simple-pattern query")
		}
	}
	return q, nil
}

func stringValue(values map[string]json.RawMessage, key string) (string, error) {
	raw, ok := values[key]
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

func secondsValue(values map[string]json.RawMessage, key string) (*int64, error) {
	raw, ok := values[key]
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
	// SEL timestamps are 32-bit; this range keeps relative arithmetic from overflowing.
	if seconds < -(1<<33) || seconds > 1<<33 {
		return nil, fmt.Errorf("%s is outside the supported timestamp range", key)
	}
	return &seconds, nil
}
