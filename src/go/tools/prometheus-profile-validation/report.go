// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	verdictPass = "PASS"
	verdictFail = "FAIL"
)

type report struct {
	Verdict             string                               `json:"verdict"`
	Profile             profileReport                        `json:"profile"`
	Job                 effectiveJobReport                   `json:"job"`
	Counts              countReport                          `json:"counts"`
	RawFamilies         []rawFamilyReport                    `json:"raw_families,omitempty"`
	PipelineExcluded    []pipelineExcludedReport             `json:"pipeline_excluded,omitempty"`
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
}

type profileReport struct {
	Name                 string   `json:"name,omitempty"`
	Match                string   `json:"match,omitempty"`
	App                  string   `json:"app,omitempty"`
	AutogenSelectorAllow []string `json:"autogen_selector_allow,omitempty"`
	AutogenSelectorDeny  []string `json:"autogen_selector_deny,omitempty"`
}

type effectiveJobReport struct {
	Name               string   `json:"name"`
	App                string   `json:"app,omitempty"`
	SelectorAllow      []string `json:"selector_allow,omitempty"`
	SelectorDeny       []string `json:"selector_deny,omitempty"`
	RelabelBlocks      int      `json:"relabel_blocks"`
	FallbackGauge      []string `json:"fallback_gauge,omitempty"`
	FallbackCounter    []string `json:"fallback_counter,omitempty"`
	ExpectedPrefix     string   `json:"expected_prefix,omitempty"`
	MaxTimeSeries      int      `json:"max_time_series"`
	MaxSeriesPerMetric int      `json:"max_time_series_per_metric"`
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
	Name               string `json:"name"`
	Type               string `json:"type"`
	Shape              string `json:"shape,omitempty"`
	Category           string `json:"category"`
	RawLogicalSeries   int    `json:"raw_logical_series"`
	WriterSourceSeries int    `json:"writer_source_series"`
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
	IDFingerprint         string   `json:"id_fingerprint"`
	Context               string   `json:"context"`
	Title                 string   `json:"title"`
	Family                string   `json:"family"`
	Units                 string   `json:"units"`
	Algorithm             string   `json:"algorithm"`
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
	WireIDFingerprint string `json:"wire_id_fingerprint"`
	Occurrences       int    `json:"occurrences"`
}

type wireContextCollisionReport struct {
	WireContextFingerprint string   `json:"wire_context_fingerprint"`
	RawContextFingerprints []string `json:"raw_context_fingerprints"`
}

type dimensionCollisionReport struct {
	ChartIDFingerprint     string `json:"chart_id_fingerprint"`
	DimensionIDFingerprint string `json:"dimension_id_fingerprint"`
	Occurrences            int    `json:"occurrences"`
}

type finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
	Why      string `json:"why,omitempty"`
}

func newReport() report {
	return report{
		Verdict: verdictPass,
		EvidenceLimits: []string{
			"Validation proves behavior for the supplied dump and structured job policy, not metrics or label values absent from that evidence.",
			"Observed planner and public-wire chart/context/dimension collisions are checked; possible collisions from unseen future values cannot be proven from one dump.",
			"A lifecycle cap that accommodates this dump may still omit entities or dimensions in a larger configuration.",
			"Exact candidate validation does not prove that profile.match uniquely auto-selects this exporter against unrelated endpoints.",
			"Dashboard meaning, functional hierarchy, and operator usefulness require model judgment and review; this tool does not score taste.",
		},
	}
}

func (r *report) addError(code, path, message, why string) {
	r.Verdict = verdictFail
	r.Findings = append(r.Findings, finding{
		Severity: "error",
		Code:     code,
		Path:     path,
		Message:  message,
		Why:      why,
	})
}

func (r *report) addWarning(code, path, message, why string) {
	r.Findings = append(r.Findings, finding{
		Severity: "warning",
		Code:     code,
		Path:     path,
		Message:  message,
		Why:      why,
	})
}

func writeJSONReport(w io.Writer, r report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func writeTextReport(w io.Writer, r report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "VERDICT: %s\n", r.Verdict)
	if r.Profile.Name != "" {
		fmt.Fprintf(&b, "Profile: %s (match=%q, app=%q)\n", r.Profile.Name, r.Profile.Match, r.Profile.App)
	}
	fmt.Fprintf(
		&b,
		"Evidence: raw=%d families/%d logical series, writer=%d flattened series, planner=%d scanned\n",
		r.Counts.RawFamilies,
		r.Counts.RawLogicalSeries,
		r.Counts.WriterSeries,
		r.Counts.SeriesScanned,
	)
	fmt.Fprintf(
		&b,
		"Charts: authored=%d, curated=%d, autogen=%d, dimensions=%d\n",
		r.Counts.AuthoredCharts,
		r.Counts.CuratedCharts,
		r.Counts.AutogenCharts,
		r.Counts.ChartDimensions,
	)
	fmt.Fprintf(
		&b,
		"Routing: autogen_series=%d, unmatched_series=%d, dead_charts=%d, dead_dimensions=%d, dimension_losses=%d, collisions=%d\n",
		r.Counts.SeriesAutogen,
		r.Counts.SeriesUnmatched,
		len(r.DeadCharts),
		len(r.DeadDimensions),
		len(r.DimensionLosses),
		len(r.Collisions)+len(r.InstanceLosses)+len(r.ChartWireCollisions)+len(r.ContextCollisions)+len(r.DimensionCollisions),
	)

	if len(r.PipelineExcluded) > 0 {
		fmt.Fprintln(&b, "\nRaw families wholly or partly absent after the real job/writer pipeline:")
		for _, item := range r.PipelineExcluded {
			shape := ""
			if item.Shape != "" {
				shape = " (" + item.Shape + ")"
			}
			fmt.Fprintf(
				&b,
				"  - %s: %s%s; logical_series raw=%d writer=%d\n",
				item.Name,
				item.Category,
				shape,
				item.RawLogicalSeries,
				item.WriterSourceSeries,
			)
		}
	}
	if len(r.AuthoredMapping) > 0 {
		fmt.Fprintln(&b, "\nAuthored selector-to-display mapping (source order):")
		for _, chart := range r.AuthoredMapping {
			algorithm := chart.Algorithm
			if algorithm == "" {
				algorithm = "<inferred>"
			}
			identity := "<global>"
			if len(chart.InstanceByLabels) > 0 {
				identity = strings.Join(chart.InstanceByLabels, ",")
			}
			fmt.Fprintf(
				&b,
				"  - %s family=%q title=%q context=%q units=%q algorithm=%q type=%q priority=%d identity=[%s]\n",
				chart.Path,
				chart.DisplayedFamily,
				chart.Title,
				chart.Context,
				chart.Units,
				algorithm,
				chart.Type,
				chart.Priority,
				identity,
			)
			for _, dimension := range chart.Dimensions {
				name := "<runtime>"
				switch {
				case dimension.Name != "":
					name = "name:" + dimension.Name
				case dimension.NameFromLabel != "":
					name = "name_from_label:" + dimension.NameFromLabel
				}
				visibility := "visible"
				if dimension.Hidden {
					visibility = "hidden"
				}
				fmt.Fprintf(&b, "      selector=%q %s %s\n", dimension.Selector, name, visibility)
			}
		}
	}
	if len(r.Charts) > 0 {
		fmt.Fprintln(&b, "\nMaterialized charts:")
		for _, chart := range r.Charts {
			kind := "curated"
			if chart.Autogen {
				kind = "AUTOGEN"
			}
			fmt.Fprintf(
				&b,
				"  - [%s] %s context=%s units=%q algorithm=%s priority=%d dims=%s\n",
				kind,
				chart.IDFingerprint,
				chart.Context,
				chart.Units,
				chart.Algorithm,
				chart.Priority,
				strings.Join(chart.DimensionFingerprints, ","),
			)
		}
	}
	if len(r.DeadCharts) > 0 {
		fmt.Fprintln(&b, "\nDead authored charts:")
		for _, item := range r.DeadCharts {
			fmt.Fprintf(
				&b,
				"  - %s title=%q context=%q priority=%d\n",
				item.Path,
				item.Title,
				item.Context,
				item.Priority,
			)
		}
	}
	if len(r.DeadDimensions) > 0 {
		fmt.Fprintln(&b, "\nDead authored dimensions:")
		for _, item := range r.DeadDimensions {
			fmt.Fprintf(&b, "  - %s selector=%q name=%q\n", item.Path, item.Selector, item.Name)
		}
	}
	if len(r.DimensionLosses) > 0 {
		fmt.Fprintln(&b, "\nDimension materialization losses:")
		for _, item := range r.DimensionLosses {
			fmt.Fprintf(
				&b,
				"  - %s observed=%d planned=%d cause=%q\n",
				item.Path,
				item.ObservedDimensions,
				item.PlannedDimensions,
				item.Cause,
			)
		}
	}
	if len(r.Collisions) > 0 {
		fmt.Fprintln(&b, "\nRendered chart ID collisions:")
		for _, item := range r.Collisions {
			fmt.Fprintf(
				&b,
				"  - id=%s charts=%s\n",
				item.RenderedIDFingerprint,
				strings.Join(item.Charts, ","),
			)
		}
	}
	if len(r.InstanceLosses) > 0 {
		fmt.Fprintln(&b, "\nChart instance materialization losses:")
		for _, item := range r.InstanceLosses {
			fmt.Fprintf(
				&b,
				"  - %s observed=%d rendered=%d cause=%q\n",
				item.Path,
				item.ObservedIdentities,
				item.RenderedIDs,
				item.Cause,
			)
		}
	}
	if len(r.ChartWireCollisions) > 0 {
		fmt.Fprintln(&b, "\nPublic wire chart ID collisions:")
		for _, item := range r.ChartWireCollisions {
			fmt.Fprintf(&b, "  - id=%s occurrences=%d\n", item.WireIDFingerprint, item.Occurrences)
		}
	}
	if len(r.ContextCollisions) > 0 {
		fmt.Fprintln(&b, "\nPublic wire context collisions:")
		for _, item := range r.ContextCollisions {
			fmt.Fprintf(
				&b,
				"  - context=%s raw_contexts=%s\n",
				item.WireContextFingerprint,
				strings.Join(item.RawContextFingerprints, ","),
			)
		}
	}
	if len(r.DimensionCollisions) > 0 {
		fmt.Fprintln(&b, "\nPublic wire dimension ID collisions:")
		for _, item := range r.DimensionCollisions {
			fmt.Fprintf(
				&b,
				"  - chart=%s dimension=%s occurrences=%d\n",
				item.ChartIDFingerprint,
				item.DimensionIDFingerprint,
				item.Occurrences,
			)
		}
	}
	if len(r.Findings) > 0 {
		fmt.Fprintln(&b, "\nFindings:")
		for _, item := range r.Findings {
			location := ""
			if item.Path != "" {
				location = " [" + item.Path + "]"
			}
			fmt.Fprintf(&b, "  - %s %s%s: %s\n", strings.ToUpper(item.Severity), item.Code, location, item.Message)
			if item.Why != "" {
				fmt.Fprintf(&b, "    Why: %s\n", item.Why)
			}
		}
	}
	if len(r.EvidenceLimits) > 0 {
		fmt.Fprintln(&b, "\nEvidence limits:")
		for _, item := range r.EvidenceLimits {
			fmt.Fprintf(&b, "  - %s\n", item)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func sortReport(r *report) {
	sort.Slice(r.RawFamilies, func(i, j int) bool { return r.RawFamilies[i].Name < r.RawFamilies[j].Name })
	sort.Slice(r.PipelineExcluded, func(i, j int) bool { return r.PipelineExcluded[i].Name < r.PipelineExcluded[j].Name })
	sort.Slice(r.Charts, func(i, j int) bool { return r.Charts[i].IDFingerprint < r.Charts[j].IDFingerprint })
	sort.Slice(r.DeadCharts, func(i, j int) bool { return r.DeadCharts[i].Path < r.DeadCharts[j].Path })
	sort.Slice(r.DeadDimensions, func(i, j int) bool { return r.DeadDimensions[i].Path < r.DeadDimensions[j].Path })
	sort.Slice(r.DimensionLosses, func(i, j int) bool {
		return r.DimensionLosses[i].Path < r.DimensionLosses[j].Path
	})
	sort.Slice(r.Collisions, func(i, j int) bool {
		return r.Collisions[i].RenderedIDFingerprint < r.Collisions[j].RenderedIDFingerprint
	})
	sort.Slice(r.InstanceLosses, func(i, j int) bool {
		return r.InstanceLosses[i].Path < r.InstanceLosses[j].Path
	})
	sort.Slice(r.ChartWireCollisions, func(i, j int) bool {
		return r.ChartWireCollisions[i].WireIDFingerprint < r.ChartWireCollisions[j].WireIDFingerprint
	})
	sort.Slice(r.ContextCollisions, func(i, j int) bool {
		return r.ContextCollisions[i].WireContextFingerprint < r.ContextCollisions[j].WireContextFingerprint
	})
	sort.Slice(r.DimensionCollisions, func(i, j int) bool {
		if r.DimensionCollisions[i].ChartIDFingerprint != r.DimensionCollisions[j].ChartIDFingerprint {
			return r.DimensionCollisions[i].ChartIDFingerprint < r.DimensionCollisions[j].ChartIDFingerprint
		}
		return r.DimensionCollisions[i].DimensionIDFingerprint < r.DimensionCollisions[j].DimensionIDFingerprint
	})
	sort.SliceStable(r.Findings, func(i, j int) bool {
		if r.Findings[i].Severity != r.Findings[j].Severity {
			return r.Findings[i].Severity == "error"
		}
		if r.Findings[i].Code != r.Findings[j].Code {
			return r.Findings[i].Code < r.Findings[j].Code
		}
		return r.Findings[i].Path < r.Findings[j].Path
	})
}
