// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func writeJSONReport(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func writeTextReport(w io.Writer, r Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "VERDICT: %s\n", r.Verdict)
	if r.Profiles.Candidate.Name != "" {
		fmt.Fprintf(&b, "Candidate profile: %s (match=%q, app=%q)\n", r.Profiles.Candidate.Name, r.Profiles.Candidate.Match, r.Profiles.Candidate.App)
	}
	for _, support := range r.Profiles.Supports {
		fmt.Fprintf(&b, "Supporting profile: %s (match=%q, app=%q)\n", support.Name, support.Match, support.App)
	}
	if len(r.Profiles.Selected) > 0 {
		fmt.Fprintf(&b, "Selected profiles: %s\n", strings.Join(r.Profiles.Selected, ", "))
	}
	if r.Profiles.Candidate.FutureRawProbe != "" {
		fmt.Fprintf(&b, "First future raw probe: %s (isolated forward-compatibility run)\n", r.Profiles.Candidate.FutureRawProbe)
	}
	for _, support := range r.Profiles.Supports {
		if support.FutureRawProbe != "" {
			fmt.Fprintf(&b, "First future raw probe for supporting profile %s: %s (isolated forward-compatibility run)\n", support.Name, support.FutureRawProbe)
		}
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
	fmt.Fprintf(
		&b,
		"Pipeline: excluded_families=%d, renamed_families=%d\n",
		r.Counts.PipelineExcluded,
		r.Counts.PipelineRenamed,
	)

	if len(r.PipelineExcluded) > 0 {
		fmt.Fprintln(&b, "\nRaw families wholly or partly absent after the real collector/writer pipeline:")
		for _, item := range r.PipelineExcluded {
			shape := ""
			if item.Shape != "" {
				shape = " (" + item.Shape + ")"
			}
			fmt.Fprintf(
				&b,
				"  - %s: %s%s; logical_series raw=%d writer=%d policy=%s\n",
				item.Name,
				item.Category,
				shape,
				item.RawLogicalSeries,
				item.WriterSourceSeries,
				formatPolicyPaths(item.PolicyPaths),
			)
		}
	}
	if len(r.PipelineRenamed) > 0 {
		fmt.Fprintln(&b, "\nRaw families successfully normalized by job/profile relabeling:")
		for _, item := range r.PipelineRenamed {
			fmt.Fprintf(
				&b,
				"  - %s -> %s; logical_series raw=%d normalized_and_materialized=%d policy=%s\n",
				item.RawName,
				strings.Join(item.FinalNames, ","),
				item.RawLogicalSeries,
				item.MaterializedLogicalSeries,
				formatPolicyPaths(item.PolicyPaths),
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
			owner := ""
			if chart.Path != "" {
				owner = " path=" + chart.Path
			}
			if chart.Profile != "" {
				owner += " profile=" + chart.Profile
			}
			fmt.Fprintf(
				&b,
				"  - [%s] %s%s context=%s units=%q algorithms=%s priority=%d dims=%s\n",
				kind,
				chart.IDFingerprint,
				owner,
				chart.Context,
				chart.Units,
				strings.Join(chart.Algorithms, ","),
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
			fmt.Fprintf(&b, "  - id=%s occurrences=%d%s\n", item.WireIDFingerprint, item.Occurrences, formatOwnerPaths(item.Paths))
		}
	}
	if len(r.ContextCollisions) > 0 {
		fmt.Fprintln(&b, "\nPublic wire context collisions:")
		for _, item := range r.ContextCollisions {
			fmt.Fprintf(
				&b,
				"  - context=%s raw_contexts=%s%s\n",
				item.WireContextFingerprint,
				strings.Join(item.RawContextFingerprints, ","),
				formatOwnerPaths(item.Paths),
			)
		}
	}
	if len(r.DimensionCollisions) > 0 {
		fmt.Fprintln(&b, "\nPublic wire dimension ID collisions:")
		for _, item := range r.DimensionCollisions {
			fmt.Fprintf(
				&b,
				"  - chart=%s dimension=%s occurrences=%d%s\n",
				item.ChartIDFingerprint,
				item.DimensionIDFingerprint,
				item.Occurrences,
				formatOwnerPaths(item.Paths),
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

func formatPolicyPaths(paths []string) string {
	if len(paths) == 0 {
		return "<none>"
	}
	return strings.Join(paths, ",")
}

func formatOwnerPaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return " paths=" + strings.Join(paths, ",")
}
