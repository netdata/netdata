// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"sort"

	"github.com/prometheus/prometheus/model/labels"
)

type (
	// SeriesSample is a pair of label set and value
	SeriesSample struct {
		Labels labels.Labels
		Value  float64
	}

	// Series is a list of SeriesSample
	Series []SeriesSample
)

// Name the __name__ label value
func (s SeriesSample) Name() string {
	return s.Labels[0].Value
}

// Add appends a metric.
func (s *Series) Add(kv SeriesSample) {
	*s = append(*s, kv)
}

// Reset resets the buffer to be empty,
// but it retains the underlying storage for use by future writes.
func (s *Series) Reset() {
	*s = (*s)[:0]
}

// Sort sorts data.
func (s Series) Sort() {
	sort.Sort(s)
}

// Len returns metric length.
func (s Series) Len() int {
	return len(s)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (s Series) Less(i, j int) bool {
	return s[i].Name() < s[j].Name()
}

// Swap swaps the elements with indexes i and j.
func (s Series) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// FindByName finds metrics where it's __name__ label matches given name.
// It expects the metrics is sorted.
// Complexity: O(log(N))
func (s Series) FindByName(name string) Series {
	from := sort.Search(len(s), func(i int) bool {
		return s[i].Name() >= name
	})
	if from == len(s) || s[from].Name() != name { // not found
		return Series{}
	}
	until := from + 1
	for until < len(s) && s[until].Name() == name {
		until++
	}
	return s[from:until]
}

// FindByNames finds metrics where it's __name__ label matches given any of names.
// It expects the metrics is sorted.
// Complexity: O(log(N))
func (s Series) FindByNames(names ...string) Series {
	switch len(names) {
	case 0:
		return Series{}
	case 1:
		return s.FindByName(names[0])
	}
	var result Series
	for _, name := range names {
		result = append(result, s.FindByName(name)...)
	}
	return result
}

// Max returns the max value.
// It does NOT expect the metrics is sorted.
// Complexity: O(N)
func (s Series) Max() float64 {
	switch len(s) {
	case 0:
		return 0
	case 1:
		return s[0].Value
	}
	maxv := s[0].Value
	for _, kv := range s[1:] {
		if maxv < kv.Value {
			maxv = kv.Value
		}
	}
	return maxv
}
