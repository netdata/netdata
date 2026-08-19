// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"io"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

const (
	verdictPass = "PASS"
	verdictFail = "FAIL"
)

// Report is the deterministic machine-readable result of Validate.
type Report struct {
	Verdict             string                               `json:"verdict"`
	Profiles            profileCompositionReport             `json:"profiles"`
	Job                 effectiveJobReport                   `json:"job"`
	Counts              countReport                          `json:"counts"`
	RawFamilies         []rawFamilyReport                    `json:"raw_families,omitempty"`
	PipelineExcluded    []pipelineExcludedReport             `json:"pipeline_excluded,omitempty"`
	PipelineRenamed     []pipelineRenamedReport              `json:"pipeline_renamed,omitempty"`
	AuthoredMapping     []authoredChartMappingReport         `json:"authored_mapping,omitempty"`
	Charts              []materializedChart                  `json:"charts,omitempty"`
	DeadCharts          []deadChartReport                    `json:"dead_charts,omitempty"`
	DeadDimensions      []deadDimensionReport                `json:"dead_dimensions,omitempty"`
	DimensionLosses     []dimensionMaterializationLossReport `json:"dimension_materialization_losses,omitempty"`
	Collisions          []collisionReport                    `json:"collisions,omitempty"`
	InstanceLosses      []instanceMaterializationLossReport  `json:"instance_materialization_losses,omitempty"`
	ChartWireCollisions []wireChartCollisionReport           `json:"chart_wire_collisions,omitempty"`
	ContextCollisions   []wireContextCollisionReport         `json:"context_wire_collisions,omitempty"`
	DimensionCollisions []dimensionCollisionReport           `json:"dimension_collisions,omitempty"`
	Findings            []finding                            `json:"findings,omitempty"`
	EvidenceLimits      []string                             `json:"evidence_limits,omitempty"`
	semantics           *promreplay.SemanticSnapshot
}

// Passed reports whether validation found no errors.
func (r Report) Passed() bool { return r.Verdict == verdictPass }

// WriteJSON writes the stable JSON report representation.
func WriteJSON(w io.Writer, r Report) error { return writeJSONReport(w, r) }

// WriteText writes the human-readable report representation.
func WriteText(w io.Writer, r Report) error { return writeTextReport(w, r) }

// ResultSnapshot returns the stable facts used by stock-proof replay without
// requiring callers to decode the report's JSON representation.
func (r Report) ResultSnapshot() promreplay.Snapshot {
	result := promreplay.Snapshot{}
	result.Semantics = promreplay.CloneSemanticSnapshot(r.semantics)
	for _, finding := range r.Findings {
		switch finding.Severity {
		case "error":
			if result.Errors == nil {
				result.Errors = make(map[string]int)
			}
			result.Errors[finding.Code]++
		case "warning":
			// Warnings are standalone authoring guidance, not objective proof facts.
		default:
			if result.UnsupportedFindingSeverities == nil {
				result.UnsupportedFindingSeverities = make(map[string]int)
			}
			result.UnsupportedFindingSeverities[finding.Severity]++
		}
	}
	return result
}

type profileReport struct {
	Name                 string   `json:"name,omitempty"`
	Match                string   `json:"match,omitempty"`
	App                  string   `json:"app,omitempty"`
	FutureRawProbe       string   `json:"future_raw_probe,omitempty"`
	AutogenSelectorAllow []string `json:"autogen_selector_allow,omitempty"`
	AutogenSelectorDeny  []string `json:"autogen_selector_deny,omitempty"`
}

type profileCompositionReport struct {
	Candidate profileReport   `json:"candidate"`
	Supports  []profileReport `json:"supports,omitempty"`
	Selected  []string        `json:"selected,omitempty"`
}

type effectiveJobReport struct {
	Name                 string   `json:"name"`
	App                  string   `json:"app,omitempty"`
	SelectorAllow        []string `json:"selector_allow,omitempty"`
	SelectorDeny         []string `json:"selector_deny,omitempty"`
	RelabelBlocks        int      `json:"relabel_blocks"`
	FallbackGauge        []string `json:"fallback_gauge,omitempty"`
	FallbackCounter      []string `json:"fallback_counter,omitempty"`
	ExpectedPrefix       string   `json:"expected_prefix,omitempty"`
	MaxTimeSeries        int      `json:"max_time_series"`
	MaxSeriesPerMetric   int      `json:"max_time_series_per_metric"`
	DeclaredFutureInputs int      `json:"declared_future_inputs"`
}

type countReport struct {
	RawFamilies      int `json:"raw_families"`
	RawLogicalSeries int `json:"raw_logical_series"`
	WriterSeries     int `json:"writer_series"`
	SeriesScanned    int `json:"series_scanned"`
	SeriesAutogen    int `json:"series_autogen"`
	SeriesUnmatched  int `json:"series_unmatched"`
	CuratedCharts    int `json:"curated_charts"`
	AutogenCharts    int `json:"autogen_charts"`
	ChartDimensions  int `json:"chart_dimensions"`
	AuthoredCharts   int `json:"authored_charts"`
	PipelineExcluded int `json:"pipeline_excluded"`
	PipelineRenamed  int `json:"pipeline_renamed"`
}

type rawFamilyReport struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Series    int    `json:"series"`
	Shape     string `json:"shape,omitempty"`
	Help      string `json:"help,omitempty"`
	Quantiles int    `json:"quantiles,omitempty"`
	Buckets   int    `json:"buckets,omitempty"`
}

type pipelineExcludedReport struct {
	Name               string   `json:"name"`
	Type               string   `json:"type"`
	Shape              string   `json:"shape,omitempty"`
	Category           string   `json:"category"`
	RawLogicalSeries   int      `json:"raw_logical_series"`
	WriterSourceSeries int      `json:"writer_source_series"`
	PolicyPaths        []string `json:"policy_paths,omitempty"`
}

type pipelineRenamedReport struct {
	RawName                   string   `json:"raw_name"`
	FinalNames                []string `json:"final_names"`
	RawLogicalSeries          int      `json:"raw_logical_series"`
	MaterializedLogicalSeries int      `json:"materialized_logical_series"`
	PolicyPaths               []string `json:"policy_paths,omitempty"`
}

type authoredChartMappingReport struct {
	Path             string                           `json:"path"`
	DisplayedFamily  string                           `json:"displayed_family"`
	Title            string                           `json:"title"`
	Context          string                           `json:"context"`
	Units            string                           `json:"units"`
	Algorithm        string                           `json:"algorithm"`
	Type             string                           `json:"type"`
	Priority         int                              `json:"priority"`
	InstanceByLabels []string                         `json:"instance_by_labels"`
	Dimensions       []authoredDimensionMappingReport `json:"dimensions"`
}

type authoredDimensionMappingReport struct {
	Selector      string `json:"selector"`
	Name          string `json:"name,omitempty"`
	NameFromLabel string `json:"name_from_label,omitempty"`
	Hidden        bool   `json:"hidden,omitempty"`
}

type materializedChart struct {
	TemplateID            string   `json:"template_id"`
	Profile               string   `json:"profile,omitempty"`
	Path                  string   `json:"path,omitempty"`
	IDFingerprint         string   `json:"id_fingerprint"`
	Context               string   `json:"context"`
	Title                 string   `json:"title"`
	Family                string   `json:"family"`
	Units                 string   `json:"units"`
	Algorithms            []string `json:"algorithms"`
	Priority              int      `json:"priority"`
	Autogen               bool     `json:"autogen"`
	DimensionFingerprints []string `json:"dimension_fingerprints,omitempty"`
}

type deadChartReport struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Context  string `json:"context"`
	Priority int    `json:"priority"`
}

type deadDimensionReport struct {
	Path     string `json:"path"`
	Selector string `json:"selector"`
	Name     string `json:"name,omitempty"`
}

type dimensionMaterializationLossReport struct {
	Path               string `json:"path"`
	ObservedDimensions int    `json:"observed_dimensions"`
	PlannedDimensions  int    `json:"planned_dimensions"`
	Cause              string `json:"cause"`
}

type collisionReport struct {
	RenderedIDFingerprint string   `json:"rendered_id_fingerprint"`
	Charts                []string `json:"charts"`
}

type instanceMaterializationLossReport struct {
	Path               string `json:"path"`
	ObservedIdentities int    `json:"observed_identities"`
	RenderedIDs        int    `json:"rendered_ids"`
	Cause              string `json:"cause"`
}

type wireChartCollisionReport struct {
	WireIDFingerprint string   `json:"wire_id_fingerprint"`
	Occurrences       int      `json:"occurrences"`
	Paths             []string `json:"paths,omitempty"`
}

type wireContextCollisionReport struct {
	WireContextFingerprint string   `json:"wire_context_fingerprint"`
	RawContextFingerprints []string `json:"raw_context_fingerprints"`
	Paths                  []string `json:"paths,omitempty"`
}

type dimensionCollisionReport struct {
	ChartIDFingerprint     string   `json:"chart_id_fingerprint"`
	DimensionIDFingerprint string   `json:"dimension_id_fingerprint"`
	Occurrences            int      `json:"occurrences"`
	Paths                  []string `json:"paths,omitempty"`
}

type finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
	Why      string `json:"why,omitempty"`
}

func newReport() Report {
	return Report{
		Verdict: verdictPass,
		EvidenceLimits: []string{
			"Validation proves behavior for the supplied dump and structured job policy, not metrics or label values absent from that evidence.",
			"The isolated future-input run proves the declared or derived raw probes traverse current selector, relabel, writer, profile, and fallback behavior; it cannot establish every future metric's labels, semantics, or cardinality.",
			"Observed planner and public-wire chart/context/dimension collisions are checked; possible collisions from unseen future values cannot be proven from one dump.",
			"A lifecycle cap that accommodates this dump may still omit entities or dimensions in a larger configuration.",
			"Exact candidate validation does not prove that profile.match uniquely auto-selects this exporter against unrelated endpoints.",
			"Dashboard meaning, functional hierarchy, and operator usefulness require model judgment and review; this tool does not score taste.",
		},
	}
}

func (r *Report) addError(code, path, message, why string) {
	r.Verdict = verdictFail
	r.Findings = append(r.Findings, finding{
		Severity: "error",
		Code:     code,
		Path:     path,
		Message:  message,
		Why:      why,
	})
}

func (r *Report) addWarning(code, path, message, why string) {
	r.Findings = append(r.Findings, finding{
		Severity: "warning",
		Code:     code,
		Path:     path,
		Message:  message,
		Why:      why,
	})
}

func (r *Report) addDeferredError(deferToSemanticCoverage bool, code, path, message, why string) {
	if deferToSemanticCoverage {
		r.addWarning(code, path, message, why)
		return
	}
	r.addError(code, path, message, why)
}
