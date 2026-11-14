// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import "sort"

const errorGroupsTopN = 10

type errorPopulation struct {
	count   uint64
	message string
}
type ErrorChartGroup struct {
	Fingerprint, Message string
	Other                bool
	Count                uint64
}

func (r *windowRead) addError(v *activityObservation) {
	fp := v.fingerprint
	if fp == "" {
		fp = unknownValue
	}
	g := r.errorGroups[fp]
	if g == nil {
		g = &errorPopulation{message: v.message}
		r.errorGroups[fp] = g
	}
	g.count += v.errors
}
func (r *windowRead) rankErrorGroups() []ErrorChartGroup {
	var keys []string
	for key := range r.errorGroups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := r.errorGroups[keys[i]], r.errorGroups[keys[j]]
		if a.count != b.count {
			return a.count > b.count
		}
		return keys[i] < keys[j]
	})
	n := min(errorGroupsTopN, len(keys))
	var out []ErrorChartGroup
	for _, key := range keys[:n] {
		g := r.errorGroups[key]
		out = append(out, ErrorChartGroup{
			Fingerprint: key,
			Message:     g.message,
			Count:       g.count,
		})
	}
	var count uint64
	for _, key := range keys[n:] {
		count += r.errorGroups[key].count
	}
	if count > 0 {
		out = append(out, ErrorChartGroup{
			Fingerprint: Other,
			Other:       true,
			Count:       count,
		})
	}
	return out
}
