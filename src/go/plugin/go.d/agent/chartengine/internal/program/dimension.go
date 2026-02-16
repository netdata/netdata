// SPDX-License-Identifier: GPL-3.0-or-later

package program

import "fmt"

// SelectorMatcher is the runtime predicate compiled from selector expressions.
//
// Implementations are expected to be immutable and safe for concurrent reads.
type SelectorMatcher interface {
	Matches(metricName string, labels SelectorLabels) bool
}

// SelectorLabels is the minimal read-only label view required by selector
// matchers. metrix.LabelView satisfies this interface.
type SelectorLabels interface {
	Len() int
	Get(key string) (string, bool)
	Range(fn func(key, value string) bool)
}

// SelectorBinding stores selector expression + compiled runtime matcher metadata.
type SelectorBinding struct {
	// Expression is original selector text (for diagnostics/debugging).
	Expression string
	// Matcher is compiled selector predicate.
	Matcher SelectorMatcher
	// MetricNames are normalized metric-name constraints extracted from selector.
	MetricNames []string
	// ConstrainedLabelKeys are matcher keys (sorted/unique) used by label policies.
	ConstrainedLabelKeys []string
}

// Dimension is one compiled dimension mapping rule for a chart.
type Dimension struct {
	Selector SelectorBinding

	// NameTemplate is used for static/dynamic dimension rendering.
	NameTemplate Template
	// NameFromLabel is explicit dynamic name source in placeholder-free mode.
	NameFromLabel string
	// InferNameFromSeriesMeta requests runtime inference based on series origin
	// metadata (e.g. histogram/state-set flatten semantics).
	InferNameFromSeriesMeta bool

	// Hidden maps to DIMENSION hidden option.
	Hidden bool
	// Multiplier maps to DIMENSION multiplier option.
	Multiplier int
	// Divisor maps to DIMENSION divisor option.
	Divisor int
	// Dynamic is compile-derived and true when rendering can fan out by labels.
	Dynamic bool
}

func validateDimension(dimension Dimension) error {
	if dimension.Selector.Expression == "" {
		return fmt.Errorf("selector expression is required")
	}
	if dimension.Selector.Matcher == nil {
		return fmt.Errorf("selector matcher is required")
	}
	if dimension.NameTemplate.Raw == "" && dimension.NameFromLabel == "" && !dimension.InferNameFromSeriesMeta {
		return fmt.Errorf("dimension name is required (name template or name_from_label)")
	}
	return nil
}

func (d Dimension) clone() Dimension {
	out := d
	out.Selector = d.Selector.clone()
	out.NameTemplate = d.NameTemplate.clone()
	return out
}

func (s SelectorBinding) clone() SelectorBinding {
	out := s
	// Matcher instance is immutable compiled program and can be shared.
	out.MetricNames = append([]string(nil), s.MetricNames...)
	out.ConstrainedLabelKeys = append([]string(nil), s.ConstrainedLabelKeys...)
	return out
}
