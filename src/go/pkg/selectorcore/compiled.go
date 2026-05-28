// SPDX-License-Identifier: GPL-3.0-or-later

package selectorcore

import (
	"fmt"
	"slices"
	"strings"
)

// Meta contains selector metadata needed by template compilers/engines.
type Meta struct {
	MetricNames          []string
	ConstrainedLabelKeys []string
}

// Compiled is a selector with stable metadata.
type Compiled interface {
	Selector
	Meta() Meta
}

type compiledSelector struct {
	Selector
	meta Meta
}

func (c compiledSelector) Meta() Meta {
	return Meta{
		MetricNames:          append([]string(nil), c.meta.MetricNames...),
		ConstrainedLabelKeys: append([]string(nil), c.meta.ConstrainedLabelKeys...),
	}
}

// ParseCompiled parses a selector expression and returns matcher + metadata.
func ParseCompiled(expr string) (Compiled, error) {
	expanded := unsugarExpr(expr)
	terms, err := splitSelectorTerms(expanded)
	if err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return nil, fmt.Errorf("invalid selector syntax: %q", expr)
	}

	parts := make([]Selector, 0, len(terms))
	metricNames := map[string]struct{}{}
	keys := map[string]struct{}{}

	for _, term := range terms {
		sel, err := parseSelector(term)
		if err != nil {
			return nil, err
		}
		parts = append(parts, sel)

		sub := reLV.FindStringSubmatch(strings.TrimSpace(term))
		if sub == nil {
			return nil, fmt.Errorf("invalid selector syntax: %q", term)
		}
		name, op, pattern := sub[1], sub[2], strings.Trim(sub[3], "\"")
		if name == MetricNameLabel {
			for _, mn := range metricNameCandidates(op, pattern) {
				metricNames[mn] = struct{}{}
			}
			continue
		}
		keys[name] = struct{}{}
	}

	out := compiledSelector{Selector: parts[0]}
	if len(parts) > 1 {
		out.Selector = And(parts[0], parts[1], parts[2:]...)
	}
	out.meta = Meta{
		MetricNames:          mapKeysSorted(metricNames),
		ConstrainedLabelKeys: mapKeysSorted(keys),
	}
	return out, nil
}

func metricNameCandidates(op, pattern string) []string {
	switch op {
	case OpEqual:
		return []string{pattern}
	case OpSimplePatterns:
		if strings.ContainsAny(pattern, "*?[]") {
			return nil
		}
		return []string{pattern}
	default:
		return nil
	}
}

func mapKeysSorted(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}
