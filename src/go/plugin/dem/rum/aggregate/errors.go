// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"sort"
	"unicode/utf8"
)

const (
	maxErrorGroups   = 200
	errMaxStackBytes = 4096
	errorGroupsTopN  = 10
)

type errorKey struct {
	fingerprint string
	other       bool
}
type errorPopulation struct {
	count   uint64
	message string
}
type ErrorChartGroup struct {
	Fingerprint, Message string
	Other                bool
	Count, Lost          uint64
}

func truncateStack(s string) string {
	if len(s) <= errMaxStackBytes {
		return s
	}
	n := errMaxStackBytes
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (r *windowRead) addError(v *activityObservation) {
	fp := v.fingerprint
	if fp == "" {
		fp = unknownValue
	}
	key := errorKey{
		fingerprint: fp,
	}
	g := r.errorGroups[key]
	if g == nil {
		if len(r.errorGroups) >= maxErrorGroups {
			key = errorKey{
				fingerprint: Other,
				other:       true,
			}
		}
		g = r.errorGroups[key]
		if g == nil {
			g = &errorPopulation{
				message: v.message,
			}
			r.errorGroups[key] = g
		}
	}
	g.count += v.errors
}
func (r *windowRead) rankErrorGroups() []ErrorChartGroup {
	var keys []errorKey
	for key := range r.errorGroups {
		if !key.other {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := r.errorGroups[keys[i]], r.errorGroups[keys[j]]
		if a.count != b.count {
			return a.count > b.count
		}
		return keys[i].fingerprint < keys[j].fingerprint
	})
	n := min(errorGroupsTopN, len(keys))
	var out []ErrorChartGroup
	for _, key := range keys[:n] {
		g := r.errorGroups[key]
		out = append(out, ErrorChartGroup{
			Fingerprint: key.fingerprint,
			Message:     g.message,
			Count:       g.count,
			Lost:        r.windowLost,
		})
	}
	var count uint64
	if g := r.errorGroups[errorKey{
		fingerprint: Other,
		other:       true,
	}]; g != nil {
		count = g.count
	}
	for _, key := range keys[n:] {
		count += r.errorGroups[key].count
	}
	if count > 0 {
		out = append(out, ErrorChartGroup{
			Fingerprint: Other,
			Other:       true,
			Count:       count,
			Lost:        r.windowLost,
		})
	}
	return out
}
