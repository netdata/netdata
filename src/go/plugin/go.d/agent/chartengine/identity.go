// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
)

func renderChartInstanceID(identity program.ChartIdentity, labels map[string]string) (string, bool, error) {
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

func renderTemplate(tpl program.Template, labels map[string]string) (string, bool, error) {
	if len(tpl.Parts) == 0 {
		return tpl.Raw, true, nil
	}

	var b strings.Builder
	for _, part := range tpl.Parts {
		if part.PlaceholderKey == "" {
			b.WriteString(part.Literal)
			continue
		}

		value, ok := labels[part.PlaceholderKey]
		if !ok || strings.TrimSpace(value) == "" {
			return "", false, nil
		}

		for _, transform := range part.Transforms {
			var err error
			value, err = applyTemplateTransform(value, transform)
			if err != nil {
				return "", false, err
			}
		}

		if strings.TrimSpace(value) == "" {
			return "", false, nil
		}
		b.WriteString(value)
	}
	return b.String(), true, nil
}

func applyTemplateTransform(value string, transform program.TemplateTransform) (string, error) {
	switch transform.Name {
	case "":
		return value, nil
	case "lower":
		return strings.ToLower(value), nil
	case "upper":
		return strings.ToUpper(value), nil
	case "trim":
		return strings.TrimSpace(value), nil
	case "replace":
		if len(transform.Args) != 2 {
			return "", fmt.Errorf("chartengine: replace transform expects 2 args, got %d", len(transform.Args))
		}
		return strings.ReplaceAll(value, transform.Args[0], transform.Args[1]), nil
	default:
		return "", fmt.Errorf("chartengine: unsupported template transform %q", transform.Name)
	}
}

func renderInstanceSuffix(identity program.ChartIdentity, labels map[string]string) (string, bool, error) {
	if len(identity.InstanceByLabels) == 0 {
		return "", true, nil
	}

	placeholderSet := make(map[string]struct{}, len(identity.IDPlaceholders))
	for _, key := range identity.IDPlaceholders {
		placeholderSet[key] = struct{}{}
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
			if _, skip := placeholderSet[token.Key]; skip {
				continue
			}
			if _, excluded := excludeSet[token.Key]; excluded {
				continue
			}
			if _, exists := seenKeys[token.Key]; exists {
				continue
			}
			if _, ok := labels[token.Key]; !ok {
				// Explicit instance key is required to materialize one instance.
				return "", false, nil
			}
			seenKeys[token.Key] = struct{}{}
			keys = append(keys, token.Key)
		}
	}

	if includeAll {
		all := make([]string, 0, len(labels))
		for key := range labels {
			if _, skip := placeholderSet[key]; skip {
				continue
			}
			if _, excluded := excludeSet[key]; excluded {
				continue
			}
			if _, exists := seenKeys[key]; exists {
				continue
			}
			all = append(all, key)
		}
		sort.Strings(all)
		for _, key := range all {
			seenKeys[key] = struct{}{}
			keys = append(keys, key)
		}
	}

	if len(keys) == 0 {
		return "", true, nil
	}

	values := make([]string, 0, len(keys))
	for _, key := range keys {
		value, ok := labels[key]
		if !ok || strings.TrimSpace(value) == "" {
			return "", false, nil
		}
		values = append(values, sanitizeIDComponent(value))
	}
	return "_" + strings.Join(values, "_"), true, nil
}

func sanitizeIDComponent(value string) string {
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "'", "")
	return value
}
