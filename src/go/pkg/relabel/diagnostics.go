// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import "github.com/prometheus/prometheus/model/labels"

// RuleDiagnostic is one rule evaluation from the production relabel processor.
// Metric names and detached label sets are snapshots at the rule boundary.
type RuleDiagnostic struct {
	RuleIndex        int
	Action           Action
	InputMetricName  string
	OutputMetricName string
	InputLabels      labels.Labels
	OutputLabels     labels.Labels
	Matched          bool
	Dropped          bool
}

// RuleDiagnosticObserver receives synchronous rule facts. The callback must not
// call the same Processor recursively.
type RuleDiagnosticObserver func(RuleDiagnostic)

// BlockDiagnostic is one entered block from the production relabel pipeline.
// InputLabelNames contains names only; label values are deliberately excluded.
type BlockDiagnostic struct {
	BlockIndex      int
	InputMetricName string
	InputLabelNames []string
}

// BlockDiagnosticObserver receives synchronous block-entry facts. The callback
// owns InputLabelNames and must not call the same Pipeline recursively.
type BlockDiagnosticObserver func(BlockDiagnostic)

// PipelineRuleDiagnosticObserver receives a rule fact with its containing block.
type PipelineRuleDiagnosticObserver func(blockIndex int, fact RuleDiagnostic)
