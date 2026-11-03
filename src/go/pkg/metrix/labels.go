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
	i := sort.Search(len(v.items), func(i int) bool { return v.items[i].Key >= key })
	if i < len(v.items) && v.items[i].Key == key {
		return v.items[i].Value, true
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

// appendCanonicalLabel appends base with extra inserted or replaced, in canonical key
// order, to dst and writes the matching canonical labels key to key. base is not
// modified. An empty extra key is invalid and appends nothing.
func appendCanonicalLabel(dst []Label, key *strings.Builder, base []Label, extra Label) ([]Label, bool) {
	if extra.Key == "" {
		return dst, false
	}
	inserted := false
	for _, label := range base {
		if !inserted && extra.Key <= label.Key {
			dst = appendLabelKeyed(dst, key, extra)
			inserted = true
			if extra.Key == label.Key {
				continue
			}
		}
		dst = appendLabelKeyed(dst, key, label)
	}
	if !inserted {
		dst = appendLabelKeyed(dst, key, extra)
	}
	return dst, true
}

func appendLabelKeyed(dst []Label, key *strings.Builder, label Label) []Label {
	key.WriteString(label.Key)
	key.WriteByte('\xff')
	key.WriteString(label.Value)
	key.WriteByte('\xff')
	return append(dst, label)
}

// labelsFromSet merges precompiled label sets and validates ownership/duplicates.
func labelsFromSet(sets []LabelSet, owner meterBackend) ([]Label, string, error) {
	if len(sets) == 0 {
		return nil, "", nil
	}

	if len(sets) == 1 {
		set, err := validatedLabelSet(sets[0], owner)
		if err != nil {
			return nil, "", err
		}
		// Reuse immutable precompiled labels directly on the single-set path.
		return set.items, set.key, nil
	}

	if len(sets) == 2 {
		left, err := validatedLabelSet(sets[0], owner)
		if err != nil {
			return nil, "", err
		}
		right, err := validatedLabelSet(sets[1], owner)
		if err != nil {
			return nil, "", err
		}
		return mergeTwoCompiledLabelSets(left, right)
	}

	merged := make(map[string]string)
	for _, ls := range sets {
		set, err := validatedLabelSet(ls, owner)
		if err != nil {
			return nil, "", err
		}
		for _, l := range set.items {
			if _, ok := merged[l.Key]; ok {
				return nil, "", errDuplicateLabelKey
			}
			merged[l.Key] = l.Value
		}
	}

	return canonicalizeLabels(merged)
}

func validatedLabelSet(ls LabelSet, owner meterBackend) (*compiledLabelSet, error) {
	if ls.set == nil {
		return nil, errInvalidLabelSet
	}
	if ls.set.owner != owner {
		return nil, errForeignLabelSet
	}
	return ls.set, nil
}

func mergeTwoCompiledLabelSets(left, right *compiledLabelSet) ([]Label, string, error) {
	if len(left.items) == 0 {
		// Reuse immutable precompiled labels directly.
		return right.items, right.key, nil
	}
	if len(right.items) == 0 {
		// Reuse immutable precompiled labels directly.
		return left.items, left.key, nil
	}

	out := make([]Label, 0, len(left.items)+len(right.items))
	var b strings.Builder

	i := 0
	j := 0
	for i < len(left.items) && j < len(right.items) {
		li := left.items[i]
		rj := right.items[j]
		if li.Key == rj.Key {
			return nil, "", errDuplicateLabelKey
		}
		if li.Key < rj.Key {
			out = append(out, li)
			b.WriteString(li.Key)
			b.WriteByte('\xff')
			b.WriteString(li.Value)
			b.WriteByte('\xff')
			i++
			continue
		}
		out = append(out, rj)
		b.WriteString(rj.Key)
		b.WriteByte('\xff')
		b.WriteString(rj.Value)
		b.WriteByte('\xff')
		j++
	}

	for ; i < len(left.items); i++ {
		li := left.items[i]
		out = append(out, li)
		b.WriteString(li.Key)
		b.WriteByte('\xff')
		b.WriteString(li.Value)
		b.WriteByte('\xff')
	}
	for ; j < len(right.items); j++ {
		rj := right.items[j]
		out = append(out, rj)
		b.WriteString(rj.Key)
		b.WriteByte('\xff')
		b.WriteString(rj.Value)
		b.WriteByte('\xff')
	}
	return out, b.String(), nil
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
