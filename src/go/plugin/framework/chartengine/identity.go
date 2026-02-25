// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

type instanceLabelValue struct {
	Key   string
	Value string
}

type labelAccessor interface {
	Get(key string) (string, bool)
	Range(fn func(key, value string) bool)
}

type mapLabelAccessor map[string]string

func (m mapLabelAccessor) Get(key string) (string, bool) {
	value, ok := m[key]
	return value, ok
}

func (m mapLabelAccessor) Range(fn func(key, value string) bool) {
	for key, value := range m {
		if !fn(key, value) {
			return
		}
	}
}

type labelViewAccessor struct {
	view metrix.LabelView
}

func (l labelViewAccessor) Get(key string) (string, bool) {
	return l.view.Get(key)
}

func (l labelViewAccessor) Range(fn func(key, value string) bool) {
	l.view.Range(fn)
}

func renderChartInstanceID(identity program.ChartIdentity, labels map[string]string) (string, bool, error) {
	return renderChartInstanceIDWithAccessor(identity, mapLabelAccessor(labels))
}

func renderChartInstanceIDFromView(identity program.ChartIdentity, labels metrix.LabelView) (string, bool, error) {
	return renderChartInstanceIDWithAccessor(identity, labelViewAccessor{view: labels})
}

func renderChartInstanceIDWithAccessor(identity program.ChartIdentity, labels labelAccessor) (string, bool, error) {
	baseID, ok, err := renderTemplate(identity.IDTemplate, labels)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}

	suffix, ok, err := renderInstanceSuffix(identity, labels)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	if suffix == "" {
		return baseID, true, nil
	}
	return baseID + suffix, true, nil
}

func renderTemplate(tpl program.Template, _ labelAccessor) (string, bool, error) {
	return tpl.Raw, true, nil
}

func renderInstanceSuffix(identity program.ChartIdentity, labels labelAccessor) (string, bool, error) {
	values, ok, err := resolveInstanceLabelValues(identity, labels)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	if len(values) == 0 {
		return "", true, nil
	}

	parts := make([]string, 0, len(values))
	hasNonEmpty := false
	for _, item := range values {
		part := sanitizeIDComponent(item.Value)
		if strings.TrimSpace(part) != "" {
			hasNonEmpty = true
		}
		parts = append(parts, part)
	}
	if !hasNonEmpty {
		// Keep base chart ID when every instance label value is empty.
		return "", true, nil
	}
	for i := range parts {
		if strings.TrimSpace(parts[i]) == "" {
			parts[i] = "empty"
		}
	}
	return "_" + strings.Join(parts, "_"), true, nil
}

func resolveInstanceLabelValues(identity program.ChartIdentity, labels labelAccessor) ([]instanceLabelValue, bool, error) {
	if len(identity.InstanceByLabels) == 0 {
		return nil, true, nil
	}

	excludeSet := make(map[string]struct{})
	seenKeys := make(map[string]struct{})
	keys := make([]string, 0, len(identity.InstanceByLabels))
	includeAll := false
	for _, token := range identity.InstanceByLabels {
		switch {
		case token.Exclude:
			if token.Key != "" {
				excludeSet[token.Key] = struct{}{}
			}
		case token.IncludeAll:
			includeAll = true
		case token.Key != "":
			if _, excluded := excludeSet[token.Key]; excluded {
				continue
			}
			if _, exists := seenKeys[token.Key]; exists {
				continue
			}
			if _, ok := labels.Get(token.Key); !ok {
				// Explicit instance key is required to materialize one instance.
				return nil, false, nil
			}
			seenKeys[token.Key] = struct{}{}
			keys = append(keys, token.Key)
		}
	}

	if includeAll {
		all := make([]string, 0)
		labels.Range(func(key, _ string) bool {
			if _, excluded := excludeSet[key]; excluded {
				return true
			}
			if _, exists := seenKeys[key]; exists {
				return true
			}
			all = append(all, key)
			return true
		})
		sort.Strings(all)
		for _, key := range all {
			seenKeys[key] = struct{}{}
			keys = append(keys, key)
		}
	}

	out := make([]instanceLabelValue, 0, len(keys))
	for _, key := range keys {
		value, ok := labels.Get(key)
		if !ok {
			return nil, false, nil
		}
		out = append(out, instanceLabelValue{
			Key:   key,
			Value: value,
		})
	}
	return out, true, nil
}

func sanitizeIDComponent(value string) string {
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "'", "")
	return value
}
