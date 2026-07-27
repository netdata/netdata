// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	commonmodel "github.com/prometheus/common/model"
)

const validationTimeout = 30 * time.Second

func validateProfile(opts validationOptions) report {
	r := newReport()
	defer sortReport(&r)

	if err := validateInputFile(opts.profilePath); err != nil {
		r.addError("profile_input", opts.profilePath, err.Error(), "The validator must read one regular profile file.")
		return r
	}
	if err := validateInputFile(opts.dumpPath); err != nil {
		r.addError("dump_input", opts.dumpPath, err.Error(), "The evidence dump must be a readable regular file.")
		return r
	}
	policy, err := loadJobPolicy(opts.jobPath)
	if err != nil {
		r.addError("job_policy", opts.jobPath, err.Error(), "Only the documented shaping fields are accepted, with strict YAML keys.")
		return r
	}
	if opts.jobPath == "" {
		r.addWarning(
			"default_validation_job",
			"",
			"no structured job policy was supplied; collector defaults are being validated",
			"A deliverable profile must be tested with the same selector, relabeling, fallback types, application identity, and series limits intended for deployment.",
		)
	}

	isolated, cleanup, err := stageIsolatedCatalog(opts.profilePath, opts.dumpPath)
	if err != nil {
		r.addError("profile_load", opts.profilePath, err.Error(), "The real strict profile catalog and template decoder must accept the candidate.")
		return r
	}
	defer cleanup()

	catalog, err := promprofiles.DefaultCatalog()
	if err != nil {
		r.addError("profile_catalog", opts.profilePath, err.Error(), "The collector must resolve the candidate through its real runtime catalog.")
		return r
	}
	profile, ok := catalog.Get(isolated.profileName)
	if !ok {
		r.addError("profile_catalog", opts.profilePath, "candidate missing from isolated runtime catalog", "Exact profile selection cannot succeed without the candidate.")
		return r
	}
	r.Profile = profileReport{Name: profile.Name, Match: profile.Match, App: profile.App}
	addProfileMatchHeuristics(profile.Match, &r)

	authored, err := inspectAuthoredCharts(profile, &r)
	if err != nil {
		r.addError("profile_template", opts.profilePath, err.Error(), "Priority policy is checked on the authored profile before collector merge/defaulting.")
		return r
	}
	r.Counts.AuthoredCharts = len(authored)

	ctx, cancel := context.WithTimeout(context.Background(), validationTimeout)
	defer cancel()

	rawFamilies, err := scrapeRawFamilies(ctx, isolated.fileURL)
	if err != nil {
		r.addError("dump_parse", opts.dumpPath, err.Error(), "The real Prometheus parser must accept and assemble the supplied exposition.")
		return r
	}
	rawSamples, err := scrapeRawSamples(ctx, isolated.fileURL)
	if err != nil {
		r.addError("dump_parse", opts.dumpPath, err.Error(), "The real Prometheus sample parser must accept the supplied exposition before job shaping.")
		return r
	}
	r.RawFamilies, r.Counts.RawLogicalSeries = inventoryRawFamilies(rawFamilies)
	r.Counts.RawFamilies = len(r.RawFamilies)
	authoredTemplate, err := profile.Template()
	if err != nil {
		r.addError("profile_template", opts.profilePath, err.Error(), "Observed semantic prompts require the same strictly decoded authored template.")
		return r
	}
	addObservedDistributionHeuristics(authoredTemplate, r.RawFamilies, &r)

	coll := promcollector.New()
	r.Job = applyJobPolicy(coll, policy, isolated.fileURL, isolated.profileName)
	if err := coll.Init(ctx); err != nil {
		r.addError("collector_init", opts.jobPath, err.Error(), "Selector, relabeling, fallback typing, limits, and exact profile selection are validated by the real collector.")
		return r
	}
	defer coll.Cleanup(context.Background())
	addJobDenyReview(policy.Selector, rawSamples, r.RawFamilies, &r)
	if err := coll.Check(ctx); err != nil {
		r.addError("collector_check", opts.dumpPath, err.Error(), "The candidate must match the post-policy scrape and pass the collector startup gates.")
		return r
	}

	managed, ok := metrix.AsCycleManagedStore(coll.MetricStore())
	if !ok {
		r.addError("collector_store", "", "collector store does not expose cycle control", "Validation must drive Collect with the same begin/commit contract as the framework.")
		return r
	}
	controller := managed.CycleController()
	controller.BeginCycle()
	if err := coll.Collect(ctx); err != nil {
		controller.AbortCycle()
		r.addError("collector_collect", opts.dumpPath, err.Error(), "The real writer pipeline must complete a collection cycle.")
		return r
	}
	if err := controller.CommitCycleSuccess(); err != nil {
		r.addError("collector_commit", "", err.Error(), "Only successfully committed series are valid chartengine evidence.")
		return r
	}

	reader := coll.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	materializedFamilies := inventoryWriterSeries(reader)
	r.Counts.WriterSeries = materializedFamilies.series
	r.PipelineExcluded = explainExcludedRawFamilies(r.RawFamilies, materializedFamilies, r.Job)
	r.Counts.PipelineExcluded = len(r.PipelineExcluded)

	templateYAML := coll.ChartTemplateYAML()
	if strings.TrimSpace(templateYAML) == "" {
		r.addError("collector_template", "", "collector returned an empty chart template", "Check must select and merge the exact candidate profile.")
		return r
	}
	merged, err := charttpl.DecodeYAML([]byte(templateYAML))
	if err != nil {
		r.addError("template_decode", "", err.Error(), "The collector's merged template must round-trip through the authoritative decoder.")
		return r
	}
	addDashboardHeuristics(merged, &r)
	addObservedLabelAggregationHeuristics(merged, reader, &r)

	engine, err := chartengine.New()
	if err != nil {
		r.addError("engine_init", "", err.Error(), "The authoritative planner must initialize.")
		return r
	}
	if err := engine.LoadYAML([]byte(templateYAML), 1); err != nil {
		r.addError("engine_load", "", err.Error(), "The authoritative compiler must accept the exact merged template served by the collector.")
		return r
	}
	attempt, err := engine.PreparePlan(reader)
	if err != nil {
		r.addError("engine_plan", "", err.Error(), "The real chartengine must route the committed collector series.")
		return r
	}
	plan := attempt.Plan()
	if err := attempt.Commit(); err != nil {
		r.addError("engine_commit", "", err.Error(), "A successful plan must satisfy chartengine's lifecycle transaction.")
		return r
	}
	addObservedScaleHeuristics(plan, &r)

	r.Charts = materializeCharts(plan)
	for _, chart := range r.Charts {
		r.Counts.ChartDimensions += len(chart.DimensionFingerprints)
		if chart.Autogen {
			r.Counts.AutogenCharts++
		} else {
			r.Counts.CuratedCharts++
		}
	}
	emitted, err := inspectEmittedPlan(plan)
	if err != nil {
		r.addError(
			"chart_emit",
			"",
			err.Error(),
			"The public chart emitter is the final ID/length contract before the Netdata wire protocol; a plan that cannot emit is not runtime-valid.",
		)
	} else {
		r.ChartWireCollisions = emitted.chartCollisions
		r.ContextCollisions = emitted.contextCollisions
		r.DimensionCollisions = emitted.dimensionCollisions
		if emitted.emittedCharts != emitted.plannedCharts {
			r.addError(
				"chart_wire_emission_loss",
				"",
				fmt.Sprintf(
					"chartengine planned %d new charts but the public emitter produced %d CHART commands",
					emitted.plannedCharts,
					emitted.emittedCharts,
				),
				"Every distinct planned chart must survive normalization into one public wire chart.",
			)
		}
		if len(emitted.emptyChartIDs) > 0 {
			r.addError(
				"chart_wire_id_empty",
				"",
				fmt.Sprintf(
					"%d planned chart IDs normalized to an empty public wire ID (raw ID fingerprints: %s)",
					len(emitted.emptyChartIDs),
					strings.Join(emitted.emptyChartIDs, ", "),
				),
				"A chart whose ID disappears at wire normalization cannot have a stable runtime identity.",
			)
		}
		if len(emitted.chartCollisions) > 0 {
			r.addError(
				"chart_wire_id_collision_observed",
				"",
				fmt.Sprintf("%d public wire chart IDs each represent more than one planned chart", len(emitted.chartCollisions)),
				"Planner-distinct chart IDs can normalize to the same wire ID and overwrite or merge each other.",
			)
		}
		if len(emitted.emptyContexts) > 0 {
			r.addError(
				"context_wire_emission_loss",
				"",
				fmt.Sprintf(
					"%d effective contexts normalized to an empty public wire value (chart ID fingerprints: %s)",
					len(emitted.emptyContexts),
					strings.Join(emitted.emptyContexts, ", "),
				),
				"A chart without its intended context cannot participate in the designed NIDL dashboard.",
			)
		}
		if len(emitted.contextCollisions) > 0 {
			r.addError(
				"context_wire_collision_observed",
				"",
				fmt.Sprintf("%d public wire contexts each represent multiple distinct effective contexts", len(emitted.contextCollisions)),
				"Intentional reuse of one raw context is valid, but distinct contexts must not become indistinguishable after wire normalization.",
			)
		}
		if emitted.emittedDimensions != emitted.plannedDimensions {
			r.addError(
				"dimension_wire_emission_loss",
				"",
				fmt.Sprintf(
					"chartengine planned %d new dimensions but the public emitter produced %d DIMENSION commands",
					emitted.plannedDimensions,
					emitted.emittedDimensions,
				),
				"A dynamic dimension name can sanitize to an empty wire ID and disappear even though chartengine materialized it.",
			)
		}
	}
	runtimeCountersOK := true
	for suffix, destination := range map[string]*int{
		"series_scanned_total":         &r.Counts.SeriesScanned,
		"series_autogen_matched_total": &r.Counts.SeriesAutogen,
		"series_unmatched_total":       &r.Counts.SeriesUnmatched,
	} {
		value, err := runtimeMetricInt(engine, suffix)
		if err != nil {
			runtimeCountersOK = false
			r.addError(
				"runtime_metrics",
				suffix,
				err.Error(),
				"Coverage counters are authoritative evidence; absence or an invalid value cannot be interpreted as zero.",
			)
			continue
		}
		*destination = value
	}

	refs := enumerateChartRefs(merged)
	unavailableIdentities, identityAuditErrs := inspectUnavailableInstanceIdentities(refs, reader)
	for _, item := range identityAuditErrs {
		r.addError(
			"instance_identity_audit",
			item.path,
			item.err.Error(),
			"The validator must resolve each authored dimension against the effective chart instance identity.",
		)
	}
	for _, item := range unavailableIdentities {
		r.addError(
			"instance_identity_label_unavailable",
			item.path,
			fmt.Sprintf(
				"chart %q effective instance identity requires labels %v, but selector %q matched %d writer series missing those labels",
				item.chartTitle,
				item.missingLabels,
				item.selector,
				item.series,
			),
			"Every explicit instance label must exist on every selected series. Move the chart to the correct entity boundary or override instances.by_labels with labels the series actually carries; never add a nonexistent label merely to satisfy the hierarchy.",
		)
	}
	dead, deadDimensions, dimensionLosses, collisions, instanceLosses, isolationErrs := inspectChartsInIsolation(merged, refs, reader)
	r.DeadCharts = dead
	r.DeadDimensions = deadDimensions
	r.DimensionLosses = dimensionLosses
	r.Collisions = collisions
	r.InstanceLosses = instanceLosses
	for _, item := range isolationErrs {
		r.addError("isolated_chart_plan", item.path, item.err.Error(), "Every authored chart is planned alone to expose dead selectors and rendered IDs hidden by whole-plan collision suppression.")
	}

	if runtimeCountersOK && r.Counts.SeriesAutogen != 0 {
		r.addError(
			"unexpected_autogen",
			"",
			fmt.Sprintf("%d series produced %d fallback charts", r.Counts.SeriesAutogen, r.Counts.AutogenCharts),
			"Passing profiles must curate every series that survives the job/writer pipeline; autogen is evidence of a coverage gap.",
		)
	}
	if runtimeCountersOK && r.Counts.SeriesUnmatched != 0 {
		r.addError(
			"unmatched_series",
			"",
			fmt.Sprintf("%d writer series matched neither a curated chart nor autogen", r.Counts.SeriesUnmatched),
			"Unmatched series are silently absent from the dashboard and therefore violate complete post-policy coverage.",
		)
	}
	for _, item := range r.DeadCharts {
		r.addError(
			"dead_chart",
			item.Path,
			fmt.Sprintf("chart %q did not materialize for the supplied dump", item.Title),
			"A declared chart with no evidence is either based on an invalid assumption or needs a different dump; it must not be presented as validated.",
		)
	}
	for _, item := range r.DeadDimensions {
		r.addError(
			"dead_dimension",
			item.Path,
			fmt.Sprintf("dimension selector %q did not materialize for the supplied dump", item.Selector),
			"A live sibling dimension can hide a selector or label-routing defect; every authored dimension needs observed evidence.",
		)
	}
	for _, item := range r.DimensionLosses {
		r.addError(
			"dimension_materialization_loss",
			item.Path,
			fmt.Sprintf(
				"%d observed per-instance dimension identities produced only %d planned dimensions",
				item.ObservedDimensions,
				item.PlannedDimensions,
			),
			"Lifecycle caps and planner normalization must not silently discard series present in the validation evidence.",
		)
	}
	for _, item := range r.Collisions {
		r.addError(
			"rendered_id_collision",
			strings.Join(item.Charts, ", "),
			fmt.Sprintf("authored charts render the same chart ID fingerprint %q", item.RenderedIDFingerprint),
			"The full planner keeps only one owner for a rendered ID, so another chart can disappear without a schema error.",
		)
	}
	for _, item := range r.InstanceLosses {
		message := fmt.Sprintf(
			"%d observed raw instance identities produced only %d distinct rendered chart IDs",
			item.ObservedIdentities,
			item.RenderedIDs,
		)
		if item.Cause == "lifecycle_limit_or_rendered_id_collapse" {
			r.addError(
				"instance_materialization_loss_observed",
				item.Path,
				message,
				"The configured max_instances limit is below observed cardinality, or rendered IDs collapsed; either case merges or omits observed entities.",
			)
		} else {
			r.addError(
				"instance_id_collision_observed",
				item.Path,
				message,
				"Different label values can normalize to the same chart ID; the planner then sums them into one chart instead of preserving separate entities.",
			)
		}
	}
	for _, item := range r.DimensionCollisions {
		r.addError(
			"dimension_id_collision_observed",
			item.ChartIDFingerprint,
			fmt.Sprintf(
				"%d authored dimensions emit the same wire ID fingerprint %q",
				item.Occurrences,
				item.DimensionIDFingerprint,
			),
			"Different dynamic dimension names can normalize to the same Netdata wire ID, causing one dimension to overwrite or merge another.",
		)
	}

	if r.Counts.WriterSeries != r.Counts.SeriesScanned {
		r.addWarning(
			"planner_series_count",
			"",
			fmt.Sprintf("writer exposed %d flattened series while chartengine scanned %d", r.Counts.WriterSeries, r.Counts.SeriesScanned),
			"An engine selector or reader-level rule may intentionally filter series, but the difference must be understood rather than treated as profile coverage.",
		)
	}
	return r
}

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

func scrapeRawFamilies(ctx context.Context, fileURL string) (prompkg.MetricFamilies, error) {
	client := prompkg.New(http.DefaultClient, web.RequestConfig{URL: fileURL})
	return client.ScrapeContext(ctx)
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
			Help:   family.Help(),
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
	sourceSeries map[string]map[string]struct{}
	series       int
}

func inventoryWriterSeries(reader metrix.Reader) writerInventory {
	out := writerInventory{sourceSeries: make(map[string]map[string]struct{})}
	reader.ForEachSeriesIdentity(func(_ metrix.SeriesIdentity, meta metrix.SeriesMeta, name string, labels metrix.LabelView, _ metrix.SampleValue) {
		out.series++
		family := sourceFamilyName(name, meta)
		identities := out.sourceSeries[family]
		if identities == nil {
			identities = make(map[string]struct{})
			out.sourceSeries[family] = identities
		}
		identities[sourceIdentityKey(labels, meta)] = struct{}{}
	})
	return out
}

func (i writerInventory) sourceSeriesCount(name string) int {
	return len(i.sourceSeries[name])
}

func sourceIdentityKey(labels metrix.LabelView, meta metrix.SeriesMeta) string {
	exclude := ""
	switch meta.FlattenRole {
	case metrix.FlattenRoleHistogramBucket:
		exclude = metrix.HistogramBucketLabel
	case metrix.FlattenRoleSummaryQuantile:
		exclude = metrix.SummaryQuantileLabel
	}

	var keys []string
	labels.Range(func(key, _ string) bool {
		if key != exclude {
			keys = append(keys, key)
		}
		return true
	})
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		value, _ := labels.Get(key)
		fmt.Fprintf(&b, "%d:%s=%d:%s;", len(key), key, len(value), value)
	}
	return b.String()
}

func sourceFamilyName(name string, meta metrix.SeriesMeta) string {
	switch meta.SourceKind {
	case metrix.MetricKindHistogram:
		switch meta.FlattenRole {
		case metrix.FlattenRoleHistogramBucket:
			return strings.TrimSuffix(name, "_bucket")
		case metrix.FlattenRoleHistogramCount:
			return strings.TrimSuffix(name, "_count")
		case metrix.FlattenRoleHistogramSum:
			return strings.TrimSuffix(name, "_sum")
		}
	case metrix.MetricKindSummary:
		switch meta.FlattenRole {
		case metrix.FlattenRoleSummaryCount:
			return strings.TrimSuffix(name, "_count")
		case metrix.FlattenRoleSummarySum:
			return strings.TrimSuffix(name, "_sum")
		}
	}
	return name
}

func explainExcludedRawFamilies(raw []rawFamilyReport, writer writerInventory, job effectiveJobReport) []pipelineExcludedReport {
	var out []pipelineExcludedReport
	ambiguousJobPolicy := len(job.SelectorAllow) > 0 || len(job.SelectorDeny) > 0 || job.RelabelBlocks > 0
	for _, family := range raw {
		writerSeries := writer.sourceSeriesCount(family.Name)
		if writerSeries >= family.Series {
			continue
		}
		category := "not_materialized_after_job_policy_or_writer"
		if writerSeries > 0 {
			if ambiguousJobPolicy {
				category = "partially_not_materialized_after_job_policy_or_writer"
			} else {
				category = "writer_partially_materialized_family"
			}
		} else if !ambiguousJobPolicy {
			switch {
			case family.Shape == "info_suffix":
				category = "writer_policy_skips_info_suffix"
			case job.MaxSeriesPerMetric > 0 && family.Series > job.MaxSeriesPerMetric:
				category = "writer_policy_series_limit"
			case family.Shape == "summary_without_quantiles":
				category = "writer_requires_summary_quantiles"
			case family.Shape == "histogram_without_buckets":
				category = "writer_requires_histogram_buckets"
			case family.Shape == "untyped" && !strings.HasSuffix(family.Name, "_total"):
				category = "untyped_requires_matching_fallback_type"
			}
		}
		out = append(out, pipelineExcludedReport{
			Name:               family.Name,
			Type:               family.Type,
			Shape:              family.Shape,
			Category:           category,
			RawLogicalSeries:   family.Series,
			WriterSourceSeries: writerSeries,
		})
	}
	return out
}

func runtimeMetricInt(engine *chartengine.Engine, suffix string) (int, error) {
	if engine == nil || engine.RuntimeStore() == nil {
		return 0, fmt.Errorf("chartengine runtime store is unavailable")
	}
	name := "netdata.go.plugin.framework.chartengine." + suffix
	value, ok := engine.RuntimeStore().Read(metrix.ReadRaw()).Value(name, nil)
	if !ok {
		return 0, fmt.Errorf("required runtime metric %q is absent", name)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || math.Trunc(value) != value {
		return 0, fmt.Errorf("required runtime metric %q has invalid counter value %v", name, value)
	}
	maxInt := int(^uint(0) >> 1)
	if value > float64(maxInt) {
		return 0, fmt.Errorf("required runtime metric %q exceeds the platform int range", name)
	}
	return int(value), nil
}

func materializeCharts(plan chartengine.Plan) []materializedChart {
	byID := make(map[string]*materializedChart)
	for _, action := range plan.Actions {
		switch item := action.(type) {
		case chartengine.CreateChartAction:
			byID[item.ChartID] = &materializedChart{
				TemplateID:    item.ChartTemplateID,
				IDFingerprint: fingerprintID(item.ChartID),
				Context:       item.Meta.Context,
				Title:         item.Meta.Title,
				Family:        item.Meta.Family,
				Units:         item.Meta.Units,
				Priority:      item.Meta.Priority,
				Autogen:       strings.HasPrefix(item.ChartTemplateID, "__autogen__:"),
			}
		case chartengine.CreateDimensionAction:
			if chart := byID[item.ChartID]; chart != nil {
				chart.DimensionFingerprints = append(chart.DimensionFingerprints, fingerprintID(item.Name))
			}
		}
	}
	out := make([]materializedChart, 0, len(byID))
	for _, chart := range byID {
		slices.Sort(chart.DimensionFingerprints)
		out = append(out, *chart)
	}
	return out
}

const validationWireTypeID = "prometheus.profile_validation"

type emittedPlanInspection struct {
	plannedCharts       int
	emittedCharts       int
	plannedDimensions   int
	emittedDimensions   int
	emptyChartIDs       []string
	emptyContexts       []string
	chartCollisions     []wireChartCollisionReport
	contextCollisions   []wireContextCollisionReport
	dimensionCollisions []dimensionCollisionReport
}

func inspectEmittedPlan(plan chartengine.Plan) (emittedPlanInspection, error) {
	var result emittedPlanInspection
	createByID := make(map[string]chartengine.CreateChartAction)
	emissionPlan := chartengine.Plan{}
	for _, action := range plan.Actions {
		switch item := action.(type) {
		case chartengine.CreateChartAction:
			result.plannedCharts++
			createByID[item.ChartID] = item
			emissionPlan.Actions = append(emissionPlan.Actions, action)
		case chartengine.CreateDimensionAction:
			result.plannedDimensions++
			emissionPlan.Actions = append(emissionPlan.Actions, action)
		}
	}

	orderedCreates := make([]chartengine.CreateChartAction, 0, len(createByID))
	for _, item := range createByID {
		orderedCreates = append(orderedCreates, item)
	}
	slices.SortFunc(orderedCreates, func(a, b chartengine.CreateChartAction) int {
		return strings.Compare(a.ChartID, b.ChartID)
	})

	var wire bytes.Buffer
	if err := chartemit.ApplyPlan(netdataapi.New(&wire), emissionPlan, chartemit.EmitEnv{
		TypeID:      validationWireTypeID,
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "prometheus",
		JobName:     "profile_validation",
	}); err != nil {
		return result, safePublicEmitterError(err)
	}

	type dimensionCount struct {
		fingerprint string
		count       int
	}
	wireChartCounts := make(map[string]int)
	rawContextsByWire := make(map[string]map[string]struct{})
	perChart := make(map[string]map[string]*dimensionCount)
	currentChart := ""
	createIndex := 0
	scanner := bufio.NewScanner(bytes.NewReader(wire.Bytes()))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "CHART "):
			fields, ok := wireQuotedFields(line)
			if !ok || len(fields) != 12 {
				return result, fmt.Errorf("parse emitted CHART line")
			}
			wireChartID := fields[0]
			wireContext := fields[5]
			result.emittedCharts++
			wireChartCounts[wireChartID]++
			currentChart = fingerprintID(wireChartID)
			if perChart[currentChart] == nil {
				perChart[currentChart] = make(map[string]*dimensionCount)
			}

			if createIndex < len(orderedCreates) {
				raw := orderedCreates[createIndex]
				createIndex++
				wireIDPrefix := validationWireTypeID + "."
				if !strings.HasPrefix(wireChartID, wireIDPrefix) {
					return result, fmt.Errorf("emitted CHART ID has an unexpected type prefix")
				}
				if strings.TrimPrefix(wireChartID, wireIDPrefix) == "" {
					result.emptyChartIDs = append(result.emptyChartIDs, fingerprintID(raw.ChartID))
				}
				if wireContext == "" {
					result.emptyContexts = append(result.emptyContexts, fingerprintID(raw.ChartID))
				}
				rawContexts := rawContextsByWire[wireContext]
				if rawContexts == nil {
					rawContexts = make(map[string]struct{})
					rawContextsByWire[wireContext] = rawContexts
				}
				rawContexts[raw.Meta.Context] = struct{}{}
			}
		case strings.HasPrefix(line, "DIMENSION "):
			if currentChart == "" {
				return result, fmt.Errorf("emitted DIMENSION before CHART")
			}
			fields, ok := wireQuotedFields(line)
			if !ok || len(fields) != 6 {
				return result, fmt.Errorf("parse emitted DIMENSION line")
			}
			value := fields[0]
			result.emittedDimensions++
			item := perChart[currentChart][value]
			if item == nil {
				item = &dimensionCount{fingerprint: fingerprintID(value)}
				perChart[currentChart][value] = item
			}
			item.count++
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("scan emitted chart protocol: %w", err)
	}

	slices.Sort(result.emptyChartIDs)
	slices.Sort(result.emptyContexts)
	for wireID, count := range wireChartCounts {
		if count < 2 {
			continue
		}
		result.chartCollisions = append(result.chartCollisions, wireChartCollisionReport{
			WireIDFingerprint: fingerprintID(wireID),
			Occurrences:       count,
		})
	}
	for wireContext, rawContexts := range rawContextsByWire {
		if len(rawContexts) < 2 {
			continue
		}
		rawFingerprints := make([]string, 0, len(rawContexts))
		for rawContext := range rawContexts {
			rawFingerprints = append(rawFingerprints, fingerprintID(rawContext))
		}
		slices.Sort(rawFingerprints)
		result.contextCollisions = append(result.contextCollisions, wireContextCollisionReport{
			WireContextFingerprint: fingerprintID(wireContext),
			RawContextFingerprints: rawFingerprints,
		})
	}
	for chartFingerprint, dimensions := range perChart {
		for _, item := range dimensions {
			if item.count < 2 {
				continue
			}
			result.dimensionCollisions = append(result.dimensionCollisions, dimensionCollisionReport{
				ChartIDFingerprint:     chartFingerprint,
				DimensionIDFingerprint: item.fingerprint,
				Occurrences:            item.count,
			})
		}
	}
	return result, nil
}

func safePublicEmitterError(err error) error {
	message := "public chart emitter rejected the plan"
	if strings.Contains(err.Error(), "type.id exceeds max length") {
		message = "public chart emitter rejected a type.id that exceeds the maximum length"
	}
	return fmt.Errorf("%s (error fingerprint %s)", message, fingerprintID(err.Error()))
}

func wireQuotedFields(line string) ([]string, bool) {
	start := strings.IndexByte(line, '\'')
	if start < 0 || !strings.HasSuffix(line, "'") {
		return nil, false
	}
	return strings.Split(line[start+1:len(line)-1], "' '"), true
}

func fingerprintID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + fmt.Sprintf("%x", sum[:8])
}

type authoredChart struct {
	path     string
	priority int
}

func inspectAuthoredCharts(profile promprofiles.Profile, r *report) ([]authoredChart, error) {
	root, err := profile.Template()
	if err != nil {
		return nil, err
	}
	addAuthoredProfileHeuristics(root, r)
	var charts []authoredChart
	seenPriorities := make(map[int]string)
	var previous *authoredChart
	var walk func(group charttpl.Group, path string)
	walk = func(group charttpl.Group, path string) {
		for i, chart := range group.Charts {
			chartPath := fmt.Sprintf("%s.charts[%d]", path, i)
			current := authoredChart{path: chartPath, priority: chart.Priority}
			charts = append(charts, current)
			switch {
			case chart.Priority <= 0:
				r.addError(
					"priority_missing",
					chartPath,
					fmt.Sprintf("chart %q has no explicit positive priority", chart.Title),
					"Missing and zero priorities collapse to 70000; the author must decide the operator-facing presentation order.",
				)
			default:
				if first, ok := seenPriorities[chart.Priority]; ok {
					r.addWarning(
						"priority_duplicate",
						chartPath,
						fmt.Sprintf("priority %d is already used by %s", chart.Priority, first),
						"Unique priorities express a deterministic total order; a tie can be deliberate, but otherwise dashboard placement falls back to unrelated chart-ID ordering.",
					)
				} else {
					seenPriorities[chart.Priority] = chartPath
				}
				if previous != nil && chart.Priority < previous.priority {
					r.addError(
						"priority_source_order",
						chartPath,
						fmt.Sprintf("priority %d does not follow %d from %s", chart.Priority, previous.priority, previous.path),
						"YAML family/chart order must mirror dashboard presentation order so the authored operator journey is reviewable. Reorder the source or correct the priorities; deliberate ties remain available when a total order is unnecessary.",
					)
				}
			}
			previous = &current
		}
		for i, child := range group.Groups {
			childPath := fmt.Sprintf("%s.groups[%d](%s)", path, i, filepath.Base(child.Family))
			walk(child, childPath)
		}
	}
	walk(root, "template")
	return charts, nil
}
