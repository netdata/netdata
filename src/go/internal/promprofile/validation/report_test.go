// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestReportResultSnapshot(t *testing.T) {
	report := newReport()
	report.addError("objective", "", "message", "why")
	report.addWarning("warning", "", "message", "why")

	snapshot := report.ResultSnapshot()
	if snapshot.Errors["objective"] != 1 {
		t.Fatalf("errors = %v", snapshot.Errors)
	}
	if len(snapshot.UnsupportedFindingSeverities) != 0 {
		t.Fatalf("unsupported severities = %v", snapshot.UnsupportedFindingSeverities)
	}
}

func TestWriteTextReportReturnsWriterError(t *testing.T) {
	if err := writeTextReport(errorWriter{}, newReport()); err == nil {
		t.Fatal("expected output writer failure")
	}
}

func TestWriteTextReportIncludesAuthoredMapping(t *testing.T) {
	r := newReport()
	r.AuthoredMapping = []authoredChartMappingReport{{
		Path:             "groups[0](Service).charts[0]",
		DisplayedFamily:  "Service/Requests",
		Title:            "Requests",
		Context:          "requests",
		Units:            "requests/s",
		Priority:         100,
		Type:             "line",
		InstanceByLabels: []string{"instance"},
		Dimensions: []authoredDimensionMappingReport{{
			Selector: "service_requests_total",
			Name:     "requests",
		}},
	}}
	var out bytes.Buffer
	if err := writeTextReport(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Authored selector-to-display mapping (source order):",
		`family="Service/Requests"`,
		`selector="service_requests_total"`,
		"algorithm=\"<inferred>\"",
		"identity=[instance]",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text report missing %q:\n%s", want, out.String())
		}
	}
}

func TestWriteTextReportSeparatesPipelineLossFromRenames(t *testing.T) {
	r := newReport()
	r.Counts.PipelineExcluded = 1
	r.Counts.PipelineRenamed = 1
	r.PipelineExcluded = []pipelineExcludedReport{{
		Name:               "app_dropped",
		Type:               "gauge",
		Category:           "not_materialized_after_pipeline_policy_or_writer",
		RawLogicalSeries:   1,
		WriterSourceSeries: 0,
	}}
	r.PipelineRenamed = []pipelineRenamedReport{{
		RawName:                   "app_worker_alpha_temperature",
		FinalNames:                []string{"app_temperature"},
		RawLogicalSeries:          1,
		MaterializedLogicalSeries: 1,
	}}

	var out bytes.Buffer
	if err := writeTextReport(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Pipeline: excluded_families=1, renamed_families=1",
		"app_dropped: not_materialized_after_pipeline_policy_or_writer",
		"app_worker_alpha_temperature -> app_temperature",
		"normalized_and_materialized=1",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text report missing %q:\n%s", want, out.String())
		}
	}
}

func TestWriteTextReportIncludesDetailedDiagnostics(t *testing.T) {
	r := newReport()
	r.Charts = []materializedChart{{
		TemplateID:    "g1.c0",
		Profile:       "app",
		Path:          "profiles[app].template.charts[0]",
		IDFingerprint: "sha256:chart-id",
		Context:       "app.value",
		Units:         "values",
	}}
	r.DeadCharts = []deadChartReport{{
		Path:     "template.charts[0]",
		Title:    "Dead chart",
		Context:  "dead",
		Priority: 100,
	}}
	r.DeadDimensions = []deadDimensionReport{{
		Path:     "template.charts[1].dimensions[0]",
		Selector: "app_missing",
		Name:     "missing",
	}}
	r.DimensionLosses = []dimensionMaterializationLossReport{{
		Path:               "template.charts[2]",
		ObservedDimensions: 3,
		PlannedDimensions:  2,
		Cause:              "dimension lifecycle cap",
	}}
	r.Collisions = []collisionReport{{
		RenderedIDFingerprint: "sha256:rendered",
		Charts:                []string{"template.charts[3]", "template.charts[4]"},
	}}
	r.InstanceLosses = []instanceMaterializationLossReport{{
		Path:               "template.charts[5]",
		ObservedIdentities: 2,
		RenderedIDs:        1,
		Cause:              "rendered IDs collapsed",
	}}
	r.ChartWireCollisions = []wireChartCollisionReport{{
		WireIDFingerprint: "sha256:wire-chart",
		Occurrences:       2,
		Paths:             []string{"profiles[app].template.charts[0]", "profiles[runtime].template.charts[0]"},
	}}
	r.ContextCollisions = []wireContextCollisionReport{{
		WireContextFingerprint: "sha256:wire-context",
		RawContextFingerprints: []string{"sha256:raw-a", "sha256:raw-b"},
		Paths:                  []string{"profiles[app].template.charts[0]", "profiles[runtime].template.charts[0]"},
	}}
	r.DimensionCollisions = []dimensionCollisionReport{{
		ChartIDFingerprint:     "sha256:chart",
		DimensionIDFingerprint: "sha256:dimension",
		Occurrences:            2,
		Paths:                  []string{"profiles[app].template.charts[0]"},
	}}

	var out bytes.Buffer
	if err := writeTextReport(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Materialized charts:",
		"path=profiles[app].template.charts[0] profile=app",
		"Dead authored charts:",
		`template.charts[0] title="Dead chart"`,
		"Dead authored dimensions:",
		`selector="app_missing" name="missing"`,
		"Dimension materialization losses:",
		`observed=3 planned=2 cause="dimension lifecycle cap"`,
		"Rendered chart ID collisions:",
		"id=sha256:rendered charts=template.charts[3],template.charts[4]",
		"Chart instance materialization losses:",
		`observed=2 rendered=1 cause="rendered IDs collapsed"`,
		"Public wire chart ID collisions:",
		"id=sha256:wire-chart occurrences=2 paths=profiles[app].template.charts[0],profiles[runtime].template.charts[0]",
		"Public wire context collisions:",
		"context=sha256:wire-context raw_contexts=sha256:raw-a,sha256:raw-b paths=profiles[app].template.charts[0],profiles[runtime].template.charts[0]",
		"Public wire dimension ID collisions:",
		"chart=sha256:chart dimension=sha256:dimension occurrences=2 paths=profiles[app].template.charts[0]",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text report missing %q:\n%s", want, out.String())
		}
	}
}

type errorWriter struct{}

func (errorWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}
