// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"sort"
)

func sortReport(r *Report) {
	sort.Slice(r.RawFamilies, func(i, j int) bool { return r.RawFamilies[i].Name < r.RawFamilies[j].Name })
	sort.Slice(r.PipelineExcluded, func(i, j int) bool { return r.PipelineExcluded[i].Name < r.PipelineExcluded[j].Name })
	sort.Slice(r.PipelineRenamed, func(i, j int) bool { return r.PipelineRenamed[i].RawName < r.PipelineRenamed[j].RawName })
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
		if r.Findings[i].Path != r.Findings[j].Path {
			return r.Findings[i].Path < r.Findings[j].Path
		}
		return r.Findings[i].Message < r.Findings[j].Message
	})
}
