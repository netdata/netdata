// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"maps"
	"sort"
	"strings"
)

// HostScope identifies the target host/vnode partition for metric series.
//
// The zero value is the default scope and is equivalent to unscoped writes.
// Non-default scopes must have a stable ScopeKey plus host metadata.
type HostScope struct {
	ScopeKey string
	GUID     string
	Hostname string
	Labels   map[string]string
}

// IsDefault reports whether this scope is the default unscoped partition.
func (s HostScope) IsDefault() bool {
	return strings.TrimSpace(s.ScopeKey) == ""
}

func normalizeHostScope(scope HostScope) (HostScope, error) {
	out := HostScope{
		ScopeKey: strings.TrimSpace(scope.ScopeKey),
		GUID:     strings.TrimSpace(scope.GUID),
		Hostname: strings.TrimSpace(scope.Hostname),
	}
	if strings.ContainsAny(out.ScopeKey, "\xfe\xff") {
		return HostScope{}, fmt.Errorf("metrix: host scope key contains reserved separator")
	}
	if out.ScopeKey == "" {
		if out.GUID != "" || out.Hostname != "" || len(scope.Labels) > 0 {
			return HostScope{}, fmt.Errorf("metrix: default host scope cannot carry vnode metadata")
		}
		return HostScope{}, nil
	}
	if out.GUID == "" {
		return HostScope{}, fmt.Errorf("metrix: host scope guid is required")
	}
	if out.Hostname == "" {
		return HostScope{}, fmt.Errorf("metrix: host scope hostname is required")
	}
	if len(scope.Labels) > 0 {
		out.Labels = make(map[string]string, len(scope.Labels))
		for key, value := range scope.Labels {
			k := strings.TrimSpace(key)
			if k == "" {
				return HostScope{}, fmt.Errorf("metrix: host scope label key is required")
			}
			if _, ok := out.Labels[k]; ok {
				return HostScope{}, fmt.Errorf("metrix: duplicate host scope label key %q", k)
			}
			out.Labels[k] = strings.TrimSpace(value)
		}
	}
	return out, nil
}

func mustNormalizeHostScope(scope HostScope) HostScope {
	out, err := normalizeHostScope(scope)
	if err != nil {
		panic(err)
	}
	return out
}

func cloneHostScope(scope HostScope) HostScope {
	if scope.Labels != nil {
		scope.Labels = maps.Clone(scope.Labels)
	}
	return scope
}

func hostScopeEqual(a, b HostScope) bool {
	if a.ScopeKey != b.ScopeKey || a.GUID != b.GUID || a.Hostname != b.Hostname {
		return false
	}
	return maps.Equal(a.Labels, b.Labels)
}

func sortedHostScopes(scopes map[string]HostScope) []HostScope {
	if len(scopes) == 0 {
		return nil
	}
	keys := make([]string, 0, len(scopes))
	for key := range scopes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]HostScope, 0, len(keys))
	if scope, ok := scopes[""]; ok {
		out = append(out, cloneHostScope(scope))
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		out = append(out, cloneHostScope(scopes[key]))
	}
	return out
}
