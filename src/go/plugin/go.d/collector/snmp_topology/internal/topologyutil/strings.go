// SPDX-License-Identifier: GPL-3.0-or-later

package topologyutil

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
)

func FirstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func FirstNonEmptySlice[T any](values ...[]T) []T {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func FirstNonEmptyMap[K comparable, V any](values ...map[K]V) map[K]V {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func FirstNonZeroInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func FirstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func SortedMapKeys[T any](m map[string]T) []string {
	keys, _ := worklimit.SortedStringKeys(nil, m)
	return keys
}

func JoinKeyParts(parts ...string) string {
	var b strings.Builder
	for _, part := range parts {
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
}

func DeduplicateSortedStrings(values []string) []string {
	out, _ := DeduplicateSortedStringsWithLimiter(nil, values)
	return out
}

func DeduplicateSortedStringsWithLimiter(limiter worklimit.Limiter, values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if err := worklimit.ChargeStrings(limiter, values); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if err := worklimit.SortStrings(limiter, out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
