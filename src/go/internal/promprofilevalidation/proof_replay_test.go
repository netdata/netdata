// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofileproof"
	"github.com/netdata/netdata/go/plugins/internal/promtestdata"
)

func TestStockProfileProofsReplay(t *testing.T) {
	goRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join(goRoot, "../.."))
	if err != nil {
		t.Fatal(err)
	}
	bundles, err := promprofileproof.Discover(repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	for _, bundle := range bundles {
		t.Run(bundle.Descriptor.Profile, func(t *testing.T) {
			for _, validationCase := range bundle.Descriptor.Validation.Cases {
				t.Run(validationCase.Name, func(t *testing.T) {
					profilePath := filepath.Join(repoRoot, filepath.FromSlash(bundle.ProfilePath()))
					dumpPath := promtestdata.Require(t, bundle.FixturePath(validationCase))
					jobPath := ""
					if validationCase.Job == "validation" {
						jobPath = filepath.Join(repoRoot, filepath.FromSlash(bundle.ValidationJobPath()))
					}

					result := runValidationFiles(t, profilePath, dumpPath, jobPath)
					assertExpectedResult(t, bundle, validationCase, result)
					if validationCase.Kind == "source_complete" {
						assertSourceCompleteInventory(t, bundle, result.report)
					}
				})
			}
		})
	}
}

func assertExpectedResult(
	t *testing.T,
	bundle promprofileproof.Bundle,
	validationCase promprofileproof.ValidationCase,
	result validationResult,
) {
	t.Helper()

	want := validationCase.Expected
	wantExitCode := 0
	if want.Verdict == verdictFail {
		wantExitCode = 1
	}
	if result.exitCode != wantExitCode || result.report.Verdict != want.Verdict {
		t.Fatalf("proof verdict differs from %s case %q: exit=%d verdict=%s, want exit=%d verdict=%s\nreport:\n%s",
			bundle.Path, validationCase.Name, result.exitCode, result.report.Verdict, wantExitCode, want.Verdict, result.stdout)
	}

	gotCounts := promprofileproof.ExpectedCounts{
		RawFamilies:         result.report.Counts.RawFamilies,
		RawLogicalSeries:    result.report.Counts.RawLogicalSeries,
		WriterSeries:        result.report.Counts.WriterSeries,
		SeriesScanned:       result.report.Counts.SeriesScanned,
		SeriesAutogen:       result.report.Counts.SeriesAutogen,
		SeriesUnmatched:     result.report.Counts.SeriesUnmatched,
		AuthoredCharts:      result.report.Counts.AuthoredCharts,
		RuntimeCharts:       result.report.Counts.CuratedCharts,
		AutogenCharts:       result.report.Counts.AutogenCharts,
		ChartDimensions:     result.report.Counts.ChartDimensions,
		PipelineExcluded:    result.report.Counts.PipelineExcluded,
		PipelineRenamed:     result.report.Counts.PipelineRenamed,
		DeadCharts:          len(result.report.DeadCharts),
		DeadDimensions:      len(result.report.DeadDimensions),
		DimensionLosses:     len(result.report.DimensionLosses),
		InstanceLosses:      len(result.report.InstanceLosses),
		Collisions:          len(result.report.Collisions),
		ChartWireCollisions: len(result.report.ChartWireCollisions),
		ContextCollisions:   len(result.report.ContextCollisions),
		DimensionCollisions: len(result.report.DimensionCollisions),
	}
	if gotCounts != want.Counts {
		t.Fatalf("proof counts differ from %s case %q:\n got: %+v\nwant: %+v",
			bundle.Path, validationCase.Name, gotCounts, want.Counts)
	}

	var gotErrors, gotWarnings map[string]int
	for _, finding := range result.report.Findings {
		switch finding.Severity {
		case "error":
			if gotErrors == nil {
				gotErrors = make(map[string]int)
			}
			gotErrors[finding.Code]++
		case "warning":
			if gotWarnings == nil {
				gotWarnings = make(map[string]int)
			}
			gotWarnings[finding.Code]++
		default:
			t.Fatalf("proof emitted unsupported finding severity %q for code %q", finding.Severity, finding.Code)
		}
	}
	if !equalIntMap(gotErrors, want.Errors) {
		t.Fatalf("proof error findings differ from %s case %q: got %v, want %v",
			bundle.Path, validationCase.Name, gotErrors, want.Errors)
	}
	if !equalIntMap(gotWarnings, want.Warnings) {
		t.Fatalf("proof warning findings differ from %s case %q: got %v, want %v",
			bundle.Path, validationCase.Name, gotWarnings, want.Warnings)
	}
}

func equalIntMap(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func assertSourceCompleteInventory(t *testing.T, bundle promprofileproof.Bundle, report Report) {
	t.Helper()

	inventoryPath := promtestdata.Require(t, bundle.SourceInventoryPath())
	inventory, err := promprofileproof.LoadSourceInventory(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.VerifyExpected(bundle.Descriptor.Inventory); err != nil {
		t.Fatal(err)
	}

	rawFamilies := make(map[string]struct{}, len(report.RawFamilies))
	for _, family := range report.RawFamilies {
		rawFamilies[family.Name] = struct{}{}
	}
	assertStringSetEqual(t, "source families", rawFamilies, inventory.SourceFamilies)

	authoredSelectors := make(map[string]struct{})
	for _, chart := range report.AuthoredMapping {
		for _, dimension := range chart.Dimensions {
			authoredSelectors[dimension.Selector] = struct{}{}
		}
	}
	assertStringSetEqual(t, "authored selectors", authoredSelectors, inventory.AuthoredSelectors)
}

func assertStringSetEqual(t *testing.T, name string, got, want map[string]struct{}) {
	t.Helper()
	var missing, unexpected []string
	for value := range want {
		if _, ok := got[value]; !ok {
			missing = append(missing, value)
		}
	}
	for value := range got {
		if _, ok := want[value]; !ok {
			unexpected = append(unexpected, value)
		}
	}
	slices.Sort(missing)
	slices.Sort(unexpected)
	if len(missing) != 0 || len(unexpected) != 0 {
		t.Fatalf("source inventory %s differ: missing=%v unexpected=%v", name, missing, unexpected)
	}
}
