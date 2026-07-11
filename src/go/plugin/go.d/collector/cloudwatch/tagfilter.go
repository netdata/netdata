// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"slices"
	"strconv"
	"strings"
)

// resourceTagFilter is the canonical compiled form of one exact AWS tag
// predicate. Filters are sorted by key and their values are sorted so equivalent
// user configurations share one stable fetch signature.
type resourceTagFilter struct {
	key    string
	values []string
}

func compileResourceTagFilters(config []ResourceTagFilterConfig) []resourceTagFilter {
	filters := make([]resourceTagFilter, len(config))
	for i, filter := range config {
		filters[i] = resourceTagFilter{key: filter.Key, values: slices.Clone(filter.Values)}
		slices.Sort(filters[i].values)
	}
	slices.SortFunc(filters, func(a, b resourceTagFilter) int { return strings.Compare(a.key, b.key) })
	return filters
}

func resourceTagFilterSignature(filters []resourceTagFilter) string {
	var b strings.Builder
	for _, filter := range filters {
		writeSignaturePart(&b, filter.key)
		b.WriteByte(':')
		for _, value := range filter.values {
			writeSignaturePart(&b, value)
		}
		b.WriteByte(';')
	}
	return b.String()
}

func writeSignaturePart(b *strings.Builder, value string) {
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
}

func resourceMatchesFilters(tags map[string]string, filters []resourceTagFilter) bool {
	for _, filter := range filters {
		value, ok := tags[filter.key]
		if !ok || !slices.Contains(filter.values, value) {
			return false
		}
	}
	return true
}
