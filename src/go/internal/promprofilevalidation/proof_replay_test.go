// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofileproof"
	"github.com/netdata/netdata/go/plugins/internal/promtestdata"
)

func TestStockProfileProofsPass(t *testing.T) {
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
		descriptor := bundle.Descriptor
		t.Run(descriptor.Profile.Name, func(t *testing.T) {
			profilePath := filepath.Join(repoRoot, filepath.FromSlash(descriptor.Profile.Path))
			jobPath := filepath.Join(repoRoot, filepath.FromSlash(descriptor.Validation.Job))
			dumpPath := promtestdata.Require(t, descriptor.External.Fixture)
			result := runValidationFiles(t, profilePath, dumpPath, jobPath)
			if result.exitCode != 0 || result.report.Verdict != verdictPass {
				t.Fatalf("stock profile proof failed\nexit code: %d\nstderr:\n%s\nreport:\n%s",
					result.exitCode, result.stderr, result.stdout)
			}
			want := descriptor.Validation.Expected
			got := promprofileproof.ExpectedFacts{
				Verdict:          result.report.Verdict,
				RawFamilies:      result.report.Counts.RawFamilies,
				RawLogicalSeries: result.report.Counts.RawLogicalSeries,
				WriterSeries:     result.report.Counts.WriterSeries,
				SeriesScanned:    result.report.Counts.SeriesScanned,
				SeriesAutogen:    result.report.Counts.SeriesAutogen,
				SeriesUnmatched:  result.report.Counts.SeriesUnmatched,
				AuthoredCharts:   result.report.Counts.AuthoredCharts,
				RuntimeCharts:    result.report.Counts.CuratedCharts,
				AutogenCharts:    result.report.Counts.AutogenCharts,
				ChartDimensions:  result.report.Counts.ChartDimensions,
				PipelineExcluded: result.report.Counts.PipelineExcluded,
				PipelineRenamed:  result.report.Counts.PipelineRenamed,
			}
			if got != want {
				t.Fatalf("stock profile counts differ from %s: got %+v, want %+v", bundle.Path, got, want)
			}
		})
	}
}
