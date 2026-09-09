// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	"github.com/prometheus/prometheus/model/labels"
)

// PipelineValueIdentity is an opaque identity for one scalar value occurrence.
type PipelineValueIdentity [32]byte

// PipelineDecision identifies one observable boundary in the production
// selector, relabel, assembly, writer, or profile-selection path.
type PipelineDecision string

const (
	PipelineRawAccepted          PipelineDecision = "raw_accepted"
	PipelineRawSelectorRejected  PipelineDecision = "raw_selector_rejected"
	PipelineRelabelBlockEntered  PipelineDecision = "relabel_block_entered"
	PipelineRelabelRuleEvaluated PipelineDecision = "relabel_rule_evaluated"
	PipelineRelabelDropped       PipelineDecision = "relabel_dropped"
	PipelineRelabelOutput        PipelineDecision = "relabel_output"
	PipelineTypedFamilyRejected  PipelineDecision = "typed_family_rejected"
	PipelineWriterFamilyRejected PipelineDecision = "writer_family_rejected"
	PipelineWriterSeriesRejected PipelineDecision = "writer_series_rejected"
	PipelineWriterSeriesAccepted PipelineDecision = "writer_series_accepted"
	PipelineProfileSelected      PipelineDecision = "profile_selected"
)

// PipelineRelabelStage identifies the owner of one relabel-pipeline fact.
type PipelineRelabelStage string

const (
	PipelineRelabelStageJob     PipelineRelabelStage = "job"
	PipelineRelabelStageProfile PipelineRelabelStage = "profile"
)

// PipelineLabel is one detached label snapshot from an opt-in pipeline fact.
type PipelineLabel struct {
	Name  string
	Value string
}

// PipelineReason is a stable production reason for a rejected pipeline item.
type PipelineReason string

const (
	PipelineReasonSelectorDenied          PipelineReason = "selector_denied"
	PipelineReasonTypedFamilyCorruption   PipelineReason = "typed_family_corruption"
	PipelineReasonInfoFamily              PipelineReason = "info_family"
	PipelineReasonSeriesLimit             PipelineReason = "series_limit"
	PipelineReasonUnsupportedType         PipelineReason = "unsupported_type"
	PipelineReasonInvalidFamilySchema     PipelineReason = "invalid_family_schema"
	PipelineReasonInvalidSeriesSchema     PipelineReason = "invalid_series_schema"
	PipelineReasonFamilyTypeDrift         PipelineReason = "family_type_drift"
	PipelineReasonDistributionSchemaDrift PipelineReason = "distribution_schema_drift"
	PipelineReasonInvalidSeriesValue      PipelineReason = "invalid_series_value"
)

// PipelineDiagnostic is a detached fact from one real collector pipeline run.
// Slice fields are owned by the callback. Label values are exposed only on the
// explicitly enabled diagnostic path and are never logged by the collector.
type PipelineDiagnostic struct {
	Decision               PipelineDecision
	Reason                 PipelineReason
	RawIdentity            prompkg.RawSampleIdentity
	DestinationRawIdentity prompkg.RawSampleIdentity
	Source                 prompkg.SampleSeriesIdentity
	Destination            prompkg.SampleSeriesIdentity
	ValueIdentity          PipelineValueIdentity
	ScalarValue            bool
	MetricName             string
	InputMetricName        string
	OutputMetricName       string
	InputLabelNames        []string
	InputLabels            []PipelineLabel
	OutputLabels           []PipelineLabel
	BlockIndex             int
	RuleIndex              int
	RelabelAction          relabel.Action
	RelabelRuleMatched     bool
	RelabelRuleDropped     bool
	RelabelDrop            relabel.DropInfo
	RelabelStage           PipelineRelabelStage
	ProfileName            string
}

// PipelineDiagnosticObserver receives synchronous production pipeline facts.
// An observer must return promptly and must not call Collector lifecycle methods.
type PipelineDiagnosticObserver func(PipelineDiagnostic)

func (c *Collector) observePipeline(fact PipelineDiagnostic) {
	if c == nil || c.pipelineObserver == nil {
		return
	}
	c.pipelineObserver(fact)
}

func pipelineScalarValueIdentity(value float64) PipelineValueIdentity {
	var raw [8]byte
	binary.LittleEndian.PutUint64(raw[:], math.Float64bits(value))
	return PipelineValueIdentity(sha256.Sum256(raw[:]))
}

func pipelineLabels(lbs labels.Labels) []PipelineLabel {
	if lbs == nil {
		return nil
	}
	out := make([]PipelineLabel, 0, len(lbs))
	for _, label := range lbs {
		if label.Name == labels.MetricName {
			continue
		}
		out = append(out, PipelineLabel{Name: label.Name, Value: label.Value})
	}
	return out
}

type pipelineObservingSelector struct {
	next      promselector.Selector
	collector *Collector
}

func (s pipelineObservingSelector) Matches(lbs labels.Labels) bool {
	if s.next.Matches(lbs) {
		return true
	}
	name := ""
	for _, label := range lbs {
		if label.Name == labels.MetricName {
			name = label.Value
			break
		}
	}
	s.collector.observePipeline(PipelineDiagnostic{
		Decision:    PipelineRawSelectorRejected,
		Reason:      PipelineReasonSelectorDenied,
		RawIdentity: prompkg.IdentifyRawSample(name, lbs),
		MetricName:  name,
	})
	return false
}
