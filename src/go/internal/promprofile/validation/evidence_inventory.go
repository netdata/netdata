// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	commonmodel "github.com/prometheus/common/model"
)

func validateInputFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", path)
	}
	return nil
}

func scrapeRawSamples(ctx context.Context, fileURL string) (prompkg.SampleBatch, error) {
	client := prompkg.New(http.DefaultClient, web.RequestConfig{URL: fileURL})
	return client.ScrapeSamples(ctx)
}

func inventoryRawFamilies(families prompkg.MetricFamilies) ([]rawFamilyReport, int) {
	out := make([]rawFamilyReport, 0, len(families))
	series := 0
	for _, family := range families {
		item := rawFamilyReport{
			Name:   family.Name(),
			Type:   string(family.Type()),
			Series: len(family.Metrics()),
			// HELP is semantic source evidence. Reports stay with the private input dump;
			// committed fixtures remain responsible for sanitizing deployment data.
			Help: family.Help(),
		}
		series += item.Series
		switch family.Type() {
		case commonmodel.MetricTypeSummary:
			for i := range family.Metrics() {
				if summary := family.Metrics()[i].Summary(); summary != nil {
					item.Quantiles = max(item.Quantiles, len(summary.Quantiles()))
				}
			}
			if item.Quantiles == 0 {
				item.Shape = "summary_without_quantiles"
			}
		case commonmodel.MetricTypeHistogram:
			for i := range family.Metrics() {
				if histogram := family.Metrics()[i].Histogram(); histogram != nil {
					item.Buckets = max(item.Buckets, len(histogram.Buckets()))
				}
			}
			if item.Buckets == 0 {
				item.Shape = "histogram_without_buckets"
			}
		case commonmodel.MetricTypeUnknown:
			item.Shape = "untyped"
		}
		if strings.HasSuffix(item.Name, "_info") {
			item.Shape = "info_suffix"
		}
		out = append(out, item)
	}
	slices.SortFunc(out, func(a, b rawFamilyReport) int { return strings.Compare(a.Name, b.Name) })
	return out, series
}

type writerInventory struct {
	series int
}

func inventoryWriterSeries(reader metrix.Reader) writerInventory {
	var out writerInventory
	reader.ForEachSeriesIdentity(func(_ metrix.SeriesIdentity, _ metrix.SeriesMeta, _ string, _ metrix.LabelView, _ metrix.SampleValue) {
		out.series++
	})
	return out
}

func reconcileRawFamilies(
	raw []rawFamilyReport,
	job effectiveJobReport,
	pipeline *pipelineDiagnosticSummary,
) ([]pipelineExcludedReport, []pipelineRenamedReport) {
	var excluded []pipelineExcludedReport
	var renamed []pipelineRenamedReport
	ambiguousPipelinePolicy := len(job.SelectorAllow) > 0 || len(job.SelectorDeny) > 0 ||
		job.RelabelBlocks > 0 || pipeline.profileRelabelBlocks > 0
	for _, family := range raw {
		policyPaths := pipeline.relabelPolicyPaths(pipeline.sourcesByFamily[family.Name])
		writerSeries := 0
		renamedSeries := 0
		finalNames := make(map[string]struct{})
		for source := range pipeline.sourcesByFamily[family.Name] {
			destinations := pipeline.audits.provenance[source]
			materialized := false
			materializedRename := false
			for destination := range destinations {
				if _, accepted := pipeline.writerAccepted[destination]; !accepted {
					continue
				}
				materialized = true
				if destination.series.Family != family.Name {
					materializedRename = true
					finalNames[destination.series.Family] = struct{}{}
				}
			}
			if materialized {
				writerSeries++
			}
			if materializedRename {
				renamedSeries++
			}
		}
		if len(finalNames) > 0 {
			renamed = append(renamed, pipelineRenamedReport{
				RawName:                   family.Name,
				FinalNames:                slices.Sorted(maps.Keys(finalNames)),
				RawLogicalSeries:          family.Series,
				MaterializedLogicalSeries: renamedSeries,
				PolicyPaths:               policyPaths,
			})
		}
		if writerSeries >= family.Series {
			continue
		}
		category := "not_materialized_after_pipeline_policy_or_writer"
		if writerSeries > 0 {
			if ambiguousPipelinePolicy {
				category = "partially_not_materialized_after_pipeline_policy_or_writer"
			} else {
				category = "writer_partially_materialized_family"
			}
		} else if !ambiguousPipelinePolicy {
			switch {
			case family.Shape == "info_suffix":
				category = "writer_policy_skips_info_suffix"
			case pipeline.writerFamilyRejects[family.Name] == promcollector.PipelineReasonSeriesLimit:
				category = "writer_policy_series_limit"
			case family.Shape == "histogram_without_buckets":
				category = "writer_requires_histogram_buckets"
			case family.Shape == "untyped" && !strings.HasSuffix(family.Name, "_total"):
				category = "untyped_requires_matching_fallback_type"
			case pipeline.writerFamilyRejects[family.Name] == promcollector.PipelineReasonInvalidFamilySchema:
				category = "writer_invalid_family_schema"
			case pipeline.writerFamilyRejects[family.Name] == promcollector.PipelineReasonUnsupportedType:
				category = "writer_unsupported_type"
			}
		}
		excluded = append(excluded, pipelineExcludedReport{
			Name:               family.Name,
			Type:               family.Type,
			Shape:              family.Shape,
			Category:           category,
			RawLogicalSeries:   family.Series,
			WriterSourceSeries: writerSeries,
			PolicyPaths:        policyPaths,
		})
	}
	return excluded, renamed
}
