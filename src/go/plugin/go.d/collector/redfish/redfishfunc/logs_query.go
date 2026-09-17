// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

type logQuery struct {
	service, severity string
	after, before     *int64
	text              matcher.Matcher
}

func parseLogQuery(req funcapi.RawMethodRequest, now time.Time) (logQuery, error) {
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
			return logQuery{}, errors.New("expected a JSON object")
		}
		if raw, ok := payload["selections"]; ok {
			var selections map[string]json.RawMessage
			if json.Unmarshal(raw, &selections) != nil {
				return logQuery{}, errors.New("invalid selections")
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
	var q logQuery
	var err error
	if q.service, err = logString(values, "service"); err != nil {
		return q, err
	}
	if q.service == "" {
		return q, errors.New("select a log service")
	}
	if q.severity, err = logString(values, "severity"); err != nil {
		return q, err
	}
	switch q.severity {
	case "", "all", "OK", "Warning", "Critical", "Unknown":
	default:
		return q, errors.New("invalid severity selection")
	}
	if q.after, err = logSeconds(values, "after"); err != nil {
		return q, err
	}
	if q.before, err = logSeconds(values, "before"); err != nil {
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
	pattern, err := logString(values, "query")
	if err != nil {
		return q, err
	}
	if strings.TrimSpace(pattern) != "" {
		q.text, err = matcher.NewSimplePatternsMatcher(pattern)
		if err != nil {
			return q, errors.New("invalid simple-pattern query")
		}
	}
	return q, nil
}

func logString(values map[string]json.RawMessage, key string) (string, error) {
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

func logSeconds(values map[string]json.RawMessage, key string) (*int64, error) {
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
	// RFC3339 source times have four-digit years; keep relative arithmetic in this range too.
	if seconds < -62135596800 || seconds > 253402300799 {
		return nil, fmt.Errorf("%s is outside the supported timestamp range", key)
	}
	return &seconds, nil
}
