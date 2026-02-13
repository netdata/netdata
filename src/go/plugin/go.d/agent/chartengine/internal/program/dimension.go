// SPDX-License-Identifier: GPL-3.0-or-later

package program

import "fmt"

// SelectorMatcher is the runtime predicate compiled from selector expressions.
//
// Implementations are expected to be immutable and safe for concurrent reads.
type SelectorMatcher interface {
	Matches(metricName string, labels map[string]string) bool
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

	// Hidden maps to DIMENSION hidden option.
	Hidden bool
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
	if dimension.NameTemplate.Raw == "" && dimension.NameFromLabel == "" {
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
