// SPDX-License-Identifier: GPL-3.0-or-later

package ipmifunc

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

const (
	paramState = "state"
	paramQuery = "query"
	stateAll   = "all"
)

type selQuery struct {
	state string // empty or stateAll selects every state
	text  matcher.Matcher
}

// parseSELQuery reads the request inputs with the framework's precedence:
// arguments, then payload selections, then top-level payload fields. Other
// inputs, such as the dashboard's after and before, are ignored.
func parseSELQuery(req funcapi.RawMethodRequest) (selQuery, error) {
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
