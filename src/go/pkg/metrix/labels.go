// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"sort"
	"strings"
)

type compiledLabelSet struct {
	owner meterBackend // owning backend; used to reject foreign handles
	items []Label      // canonical sorted labels
	key   string       // canonical packed key for fast identity joins
}

type labelView struct {
	items []Label
}

func (v labelView) Len() int { return len(v.items) }

func (v labelView) Get(key string) (string, bool) {
	for _, l := range v.items {
		if l.Key == key {
			return l.Value, true
		}
	}
	return "", false
}

func (v labelView) Range(fn func(key, value string) bool) {
	for _, l := range v.items {
		if !fn(l.Key, l.Value) {
			return
		}
	}
}

func (v labelView) CloneMap() map[string]string {
	m := make(map[string]string, len(v.items))
	for _, l := range v.items {
		m[l.Key] = l.Value
	}
	return m
}

func canonicalizeLabels(input map[string]string) ([]Label, string, error) {
	if len(input) == 0 {
		return nil, "", nil
	}

	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	labels := make([]Label, 0, len(keys))
	var b strings.Builder
	for _, k := range keys {
		if k == "" {
			return nil, "", errInvalidLabelKey
		}
		v := input[k]
		labels = append(labels, Label{Key: k, Value: v})
		b.WriteString(k)
		b.WriteByte('\xff')
		b.WriteString(v)
		b.WriteByte('\xff')
	}
	return labels, b.String(), nil
}

// labelsFromSet merges precompiled label sets and validates ownership/duplicates.
func labelsFromSet(sets []LabelSet, owner meterBackend) ([]Label, string, error) {
	if len(sets) == 0 {
		return nil, "", nil
	}

	merged := make(map[string]string)
	for _, ls := range sets {
		if ls.set == nil {
			return nil, "", errInvalidLabelSet
		}
		if ls.set.owner != owner {
			return nil, "", errForeignLabelSet
		}
		for _, l := range ls.set.items {
			if _, ok := merged[l.Key]; ok {
				return nil, "", errDuplicateLabelKey
			}
			merged[l.Key] = l.Value
		}
	}

	return canonicalizeLabels(merged)
}

func appendLabelSets(base []LabelSet, extra []LabelSet) []LabelSet {
	if len(extra) == 0 {
		return base
	}
	if len(base) == 0 {
		return extra
	}
	out := make([]LabelSet, 0, len(base)+len(extra))
	out = append(out, base...)
	out = append(out, extra...)
	return out
}
