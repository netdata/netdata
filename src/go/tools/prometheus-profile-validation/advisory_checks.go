// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	commonmodel "github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"
)

// addProfileMatchHeuristics checks whether the auto-selection signature also
// accepts common runtime/instrumentation families. Those families can be
// charted, but one generic hit is enough to select the entire profile.
func addProfileMatchHeuristics(expression string, r *report) {
	m, err := matcher.NewSimplePatternsMatcher(expression)
	if err != nil {
		return // The strict profile loader reports the authoritative syntax error.
	}

	probes := []struct {
		class string
		name  string
	}{
		{class: "go_*", name: "go_memstats_alloc_bytes"},
		{class: "http_*", name: "http_requests_total"},
		{class: "process_*", name: "process_cpu_seconds_total"},
		{class: "python_*", name: "python_gc_collections_total"},
	}
	var matched []string
	for _, probe := range probes {
		if m.MatchString(probe.name) {
			matched = append(matched, probe.class)
		}
	}
	if len(matched) == 0 {
		return
	}

	r.addWarning(
		"generic_profile_match",
		"profile.match",
		fmt.Sprintf("profile detection also accepts generic family classes %v", matched),
		"Automatic selection needs only one matching scraped family. Generic runtime and instrumentation families can be charted without participating in detection; keep match exporter-specific unless broad selection is deliberate.",
	)
}

// addJobDenyReview reports observed impact, not a policy verdict. A deny can be
// correct, but its diagnostic loss must be a conscious dashboard decision.
func addJobDenyReview(
	expr promselector.Expr,
	batch prompkg.SampleBatch,
	rawFamilies []rawFamilyReport,
	r *report,
) {
	if len(expr.Allow) == 0 && len(expr.Deny) == 0 {
		return
	}

	eligible := make(map[string]struct{})
	for _, family := range rawFamilies {
		switch {
		case family.Shape == "info_suffix":
			continue
		case family.Shape == "summary_without_quantiles":
			continue
		case family.Shape == "histogram_without_buckets":
			continue
		}
		switch commonmodel.MetricType(family.Type) {
		case commonmodel.MetricTypeGauge,
			commonmodel.MetricTypeCounter,
			commonmodel.MetricTypeHistogram,
			commonmodel.MetricTypeSummary:
			eligible[family.Name] = struct{}{}
		}
	}

	allow, err := (promselector.Expr{Allow: expr.Allow}).Parse()
	if err != nil {
		return // Collector.Init reports the authoritative selector error.
	}
	effective, err := expr.Parse()
	if err == nil && effective != nil {
		allIdentities := make(map[string]struct{})
		excludedIdentities := make(map[string]struct{})
		excludedFamilies := make(map[string]struct{})
		excludedRawSeries := 0
		for _, sample := range batch.Samples {
			family := sampleSourceFamilyName(sample)
			if _, ok := eligible[family]; !ok {
				continue
			}
			identity := logicalSampleIdentityKey(family, sample)
			allIdentities[identity] = struct{}{}
			if effective.Matches(sampleLabelsWithName(sample)) {
				continue
			}
			excludedIdentities[identity] = struct{}{}
			excludedFamilies[family] = struct{}{}
			excludedRawSeries++
		}
		if len(excludedIdentities) > 0 {
			r.addWarning(
				"job_policy_exclusion_summary",
				"selector",
				fmt.Sprintf(
					"job selector removes %d of %d observed writer-capable logical identities across %d families (%d raw exposition series) before profile coverage is measured",
					len(excludedIdentities),
					len(allIdentities),
					len(excludedFamilies),
					excludedRawSeries,
				),
				"A mechanical PASS covers only the post-policy denominator. Use hierarchy and priority for dashboard focus; filtering distinct writer-capable diagnostics merely makes the gate easier by deleting evidence.",
			)
		}
	}
	if len(expr.Allow) > 0 {
		families := make(map[string]struct{})
		logicalIdentities := make(map[string]struct{})
		rawSeries := 0
		for _, sample := range batch.Samples {
			family := sampleSourceFamilyName(sample)
			if _, ok := eligible[family]; !ok {
				continue
			}
			if allow != nil && allow.Matches(sampleLabelsWithName(sample)) {
				continue
			}
			families[family] = struct{}{}
			logicalIdentities[logicalSampleIdentityKey(family, sample)] = struct{}{}
			rawSeries++
		}
		if len(logicalIdentities) > 0 {
			r.addWarning(
				"job_allow_exclusion_review",
				"selector.allow",
				fmt.Sprintf(
					"allow expressions exclude %d observed logical identities across %d otherwise writer-capable families (%d raw exposition series)",
					len(logicalIdentities),
					len(families),
					rawSeries,
				),
				"An allow list defines the dashboard's raw evidence boundary. Confirm that excluded exporter/runtime surfaces are intentionally delegated or discarded; post-policy coverage cannot recover them.",
			)
		}
	}
	for i, expression := range expr.Deny {
		deny, err := promselector.Parse(expression)
		if err != nil {
			continue // Collector.Init reports the authoritative selector error.
		}

		families := make(map[string]struct{})
		logicalIdentities := make(map[string]struct{})
		rawSeries := 0
		for _, sample := range batch.Samples {
			family := sampleSourceFamilyName(sample)
			if _, ok := eligible[family]; !ok {
				continue
			}
			labels := sampleLabelsWithName(sample)
			if (allow != nil && !allow.Matches(labels)) || deny == nil || !deny.Matches(labels) {
				continue
			}
			families[family] = struct{}{}
			logicalIdentities[logicalSampleIdentityKey(family, sample)] = struct{}{}
			rawSeries++
		}
		if len(logicalIdentities) == 0 {
			continue
		}

		r.addWarning(
			"job_deny_review",
			fmt.Sprintf("selector.deny[%d]", i),
			fmt.Sprintf(
				"deny expression matches %d observed logical identities across %d otherwise writer-capable families (%d raw exposition series)",
				len(logicalIdentities),
				len(families),
				rawSeries,
			),
			"An exclusion can be correct, but current zero/constant values or similar names do not prove redundancy. Confirm authoritative semantics and state which operator question is lost.",
		)
	}
}

func sampleSourceFamilyName(sample prompkg.Sample) string {
	switch sample.Kind {
	case prompkg.SampleKindHistogramBucket:
		return strings.TrimSuffix(sample.Name, "_bucket")
	case prompkg.SampleKindHistogramCount, prompkg.SampleKindSummaryCount:
		return strings.TrimSuffix(sample.Name, "_count")
	case prompkg.SampleKindHistogramSum, prompkg.SampleKindSummarySum:
		return strings.TrimSuffix(sample.Name, "_sum")
	default:
		return sample.Name
	}
}

func sampleLabelsWithName(sample prompkg.Sample) promlabels.Labels {
	out := make(promlabels.Labels, 0, len(sample.Labels)+1)
	out = append(out, promlabels.Label{Name: promlabels.MetricName, Value: sample.Name})
	out = append(out, sample.Labels...)
	return out
}

func logicalSampleIdentityKey(family string, sample prompkg.Sample) string {
	excluded := ""
	switch sample.Kind {
	case prompkg.SampleKindHistogramBucket:
		excluded = "le"
	case prompkg.SampleKindSummaryQuantile:
		excluded = "quantile"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d:%s;", len(family), family)
	for _, label := range sample.Labels {
		if label.Name == excluded {
			continue
		}
		fmt.Fprintf(&b, "%d:%s=%d:%s;", len(label.Name), label.Name, len(label.Value), label.Value)
	}
	return b.String()
}

type observedLabelDimension struct {
	spec     charttpl.Dimension
	selector metrixselector.Compiled
}

type observedLabelAggregation struct {
	keys   []string
	paths  []string
	titles []string
}

// addObservedLabelAggregationHeuristics identifies labels that selected series
// carry but the authored chart does not use for identity, dimensions, promoted
// metadata, selector routing, or explicit by_labels exclusion. Aggregation can
// be intentional; the warning makes the lost comparison/filter explicit.
func addObservedLabelAggregationHeuristics(spec *charttpl.Spec, reader metrix.Reader, r *report) {
	if spec == nil {
		return
	}

	grouped := make(map[string]*observedLabelAggregation)
	for _, ref := range enumerateChartRefs(spec) {
		handled, wildcard := handledChartLabels(ref.chart)
		dimensions := make([]observedLabelDimension, 0, len(ref.chart.Dimensions))
		for _, dimension := range ref.chart.Dimensions {
			compiled, err := metrixselector.ParseCompiled(dimension.Selector)
			if err != nil {
				continue // The authoritative compiler reports selector errors.
			}
			for _, key := range compiled.Meta().ConstrainedLabelKeys {
				handled[strings.TrimSpace(key)] = struct{}{}
			}
			if key := strings.TrimSpace(dimension.NameFromLabel); key != "" {
				handled[key] = struct{}{}
			}
			dimensions = append(dimensions, observedLabelDimension{spec: dimension, selector: compiled})
		}
		if wildcard {
			continue
		}

		aggregated := make(map[string]struct{})
		reader.ForEachSeriesIdentity(func(
			_ metrix.SeriesIdentity,
			meta metrix.SeriesMeta,
			name string,
			labels metrix.LabelView,
			_ metrix.SampleValue,
		) {
			matched := false
			for _, dimension := range dimensions {
				if dimension.selector.Matches(name, labels) &&
					dimensionNameMaterializes(dimension.spec, name, labels, meta) {
					matched = true
					break
				}
			}
			if !matched {
				return
			}
			labels.Range(func(key, _ string) bool {
				key = strings.TrimSpace(key)
				if key == "" || key == metrix.HistogramBucketLabel || key == metrix.SummaryQuantileLabel {
					return true
				}
				if _, ok := handled[key]; !ok {
					aggregated[key] = struct{}{}
				}
				return true
			})
		})
		if len(aggregated) == 0 {
			continue
		}

		keys := slices.Sorted(maps.Keys(aggregated))
		groupKey := strings.Join(keys, "\x00")
		group := grouped[groupKey]
		if group == nil {
			group = &observedLabelAggregation{keys: keys}
			grouped[groupKey] = group
		}
		group.paths = append(group.paths, ref.path)
		group.titles = append(group.titles, ref.chart.Title)
	}

	groupKeys := slices.Sorted(maps.Keys(grouped))
	for _, groupKey := range groupKeys {
		group := grouped[groupKey]
		path := "template"
		message := fmt.Sprintf(
			"%d charts aggregate observed label keys %v without an authored role",
			len(group.paths),
			group.keys,
		)
		if len(group.paths) == 1 {
			path = group.paths[0]
			message = fmt.Sprintf(
				"chart %q aggregates observed label keys %v without an authored role",
				group.titles[0],
				group.keys,
			)
		} else {
			exampleCount := min(3, len(group.titles))
			examples := make([]string, 0, exampleCount)
			for _, title := range group.titles[:exampleCount] {
				examples = append(examples, fmt.Sprintf("%q", title))
			}
			message += " (examples: " + strings.Join(examples, ", ") + ")"
		}
		r.addWarning(
			"observed_label_aggregation",
			path,
			message,
			"An omitted label removes that comparison and may merge distinct entities when new values appear. Aggregation can be correct, but explain the lost filtering/comparison and expected cardinality; do not add identity or promotion merely to silence the warning.",
		)
	}
}

func handledChartLabels(chart charttpl.Chart) (map[string]struct{}, bool) {
	handled := make(map[string]struct{})
	for _, key := range chart.LabelPromoted {
		if key = strings.TrimSpace(key); key != "" {
			handled[key] = struct{}{}
		}
	}

	wildcard := false
	if chart.Instances == nil {
		return handled, wildcard
	}
	for _, token := range chart.Instances.ByLabels {
		token = strings.TrimSpace(token)
		switch {
		case token == "*":
			wildcard = true
		case strings.HasPrefix(token, "!"):
			if key := strings.TrimSpace(strings.TrimPrefix(token, "!")); key != "" {
				handled[key] = struct{}{}
			}
		case token != "":
			handled[token] = struct{}{}
		}
	}
	return handled, wildcard
}

type metricDeclaration struct {
	name string
	path string
	used bool
}

// addAuthoredProfileHeuristics checks source intent before collector merge or
// compiler defaulting can hide it. Every result remains an advisory warning.
func addAuthoredProfileHeuristics(root charttpl.Group, r *report) {
	var declarations []*metricDeclaration
	var walk func(group charttpl.Group, path string, active map[string]*metricDeclaration)
	walk = func(group charttpl.Group, path string, active map[string]*metricDeclaration) {
		scoped := maps.Clone(active)
		for i, name := range group.Metrics {
			decl := &metricDeclaration{
				name: strings.TrimSpace(name),
				path: fmt.Sprintf("%s.metrics[%d]", path, i),
			}
			declarations = append(declarations, decl)
			scoped[decl.name] = decl
		}

		for i, chart := range group.Charts {
			chartPath := fmt.Sprintf("%s.charts[%d]", path, i)
			reviewAuthoredChart(chart, chartPath, scoped, r)
		}
		for i, child := range group.Groups {
			walk(
				child,
				fmt.Sprintf("%s.groups[%d](%s)", path, i, child.Family),
				scoped,
			)
		}
	}
	walk(root, "template", make(map[string]*metricDeclaration))

	for _, decl := range declarations {
		if decl.name == "" || decl.used {
			continue
		}
		r.addWarning(
			"unused_metric_declaration",
			decl.path,
			fmt.Sprintf("metric %q is declared but no authored dimension in its scope selects it", decl.name),
			"A metrics declaration only authorizes selector scope; it does not keep, drop, or chart data. Unused declarations obscure ownership and can leave stale denied families looking intentionally covered.",
		)
	}
}

func reviewAuthoredChart(
	chart charttpl.Chart,
	path string,
	active map[string]*metricDeclaration,
	r *report,
) {
	hasBucket := false
	for _, dimension := range chart.Dimensions {
		compiled, err := metrixselector.ParseCompiled(dimension.Selector)
		if err != nil {
			continue // Authoritative template/compiler validation reports errors.
		}
		for _, name := range compiled.Meta().MetricNames {
			if decl := active[name]; decl != nil {
				decl.used = true
			}
			if strings.HasSuffix(name, "_bucket") {
				hasBucket = true
			}
		}
	}

	if hasBucket && !strings.EqualFold(strings.TrimSpace(chart.Type), "heatmap") {
		authoredType := strings.TrimSpace(chart.Type)
		if authoredType == "" {
			authoredType = "<default line>"
		}
		r.addWarning(
			"histogram_type_runtime_override",
			path,
			fmt.Sprintf("chart %q selects histogram buckets but declares type %q", chart.Title, authoredType),
			"The compiler forces bucket charts to heatmap. Declare heatmap explicitly so the authored design states the UI that actually runs.",
		)
	}

	chartType := strings.ToLower(strings.TrimSpace(chart.Type))
	if (chartType == "area" || chartType == "stacked") && rateLikeUnits(chart.Units) {
		r.addWarning(
			"rate_filled_type_review",
			path,
			fmt.Sprintf("chart %q uses %s for rate-like units %q", chart.Title, chartType, chart.Units),
			"Additive event, token, request, count, or time rates do not become physical volume merely because their dimensions sum. Use line unless the units represent physical volume, space, bandwidth, or I/O with meaningful fill.",
		)
	} else if (chartType == "area" || chartType == "stacked") && !physicalVolumeUnits(chart.Units) {
		r.addWarning(
			"nonvolume_filled_type_review",
			path,
			fmt.Sprintf("chart %q uses %s for non-volume units %q", chart.Title, chartType, chart.Units),
			"Filled/stacked presentation is reserved for meaningful physical volume, space, bandwidth, or I/O. Counts, states, and arbitrary additive categories should normally remain lines.",
		)
	}
}

func rateLikeUnits(units string) bool {
	units = strings.ToLower(strings.TrimSpace(units))
	return strings.Contains(units, "/s") ||
		strings.Contains(units, "/sec") ||
		strings.Contains(units, "per second")
}

func physicalVolumeUnits(units string) bool {
	units = strings.ToLower(strings.TrimSpace(units))
	for _, token := range []string{
		"byte",
		"bit/s",
		"bits/s",
		"bandwidth",
		"i/o",
		"io/s",
		"space",
	} {
		if strings.Contains(units, token) {
			return true
		}
	}
	return false
}

func addObservedDistributionHeuristics(
	root charttpl.Group,
	rawFamilies []rawFamilyReport,
	r *report,
) {
	familyTypes := make(map[string]commonmodel.MetricType)
	for _, family := range rawFamilies {
		typ := commonmodel.MetricType(family.Type)
		if typ == commonmodel.MetricTypeHistogram || typ == commonmodel.MetricTypeSummary {
			familyTypes[family.Name] = typ
		}
	}

	var walk func(group charttpl.Group, path string)
	walk = func(group charttpl.Group, path string) {
		for i, chart := range group.Charts {
			chartPath := fmt.Sprintf("%s.charts[%d]", path, i)
			rolesByFamily := make(map[string]map[string]struct{})
			for _, dimension := range chart.Dimensions {
				compiled, err := metrixselector.ParseCompiled(dimension.Selector)
				if err != nil {
					continue
				}
				for _, name := range compiled.Meta().MetricNames {
					family, role, ok := distributionRole(name, familyTypes)
					if !ok {
						continue
					}
					roles := rolesByFamily[family]
					if roles == nil {
						roles = make(map[string]struct{})
						rolesByFamily[family] = roles
					}
					roles[role] = struct{}{}
				}
			}
			for _, roles := range rolesByFamily {
				if len(roles) < 2 {
					continue
				}
				names := make([]string, 0, len(roles))
				for role := range roles {
					names = append(names, role)
				}
				slices.Sort(names)
				r.addWarning(
					"distribution_role_mixing",
					chartPath,
					fmt.Sprintf("chart %q mixes distribution roles %v from one source family", chart.Title, names),
					"Buckets/quantiles describe distribution shape, count describes observations, and sum carries observed units. One chart unit and axis cannot make those roles semantically interchangeable.",
				)
			}
		}
		for i, child := range group.Groups {
			walk(child, fmt.Sprintf("%s.groups[%d](%s)", path, i, child.Family))
		}
	}
	walk(root, "template")
}

func distributionRole(
	name string,
	familyTypes map[string]commonmodel.MetricType,
) (string, string, bool) {
	if base := strings.TrimSuffix(name, "_bucket"); base != name &&
		familyTypes[base] == commonmodel.MetricTypeHistogram {
		return base, "bucket", true
	}
	if base := strings.TrimSuffix(name, "_count"); base != name {
		if typ := familyTypes[base]; typ == commonmodel.MetricTypeHistogram || typ == commonmodel.MetricTypeSummary {
			return base, "count", true
		}
	}
	if base := strings.TrimSuffix(name, "_sum"); base != name {
		if typ := familyTypes[base]; typ == commonmodel.MetricTypeHistogram || typ == commonmodel.MetricTypeSummary {
			return base, "sum", true
		}
	}
	if familyTypes[name] == commonmodel.MetricTypeSummary {
		return name, "quantile", true
	}
	return "", "", false
}

type chartScaleMeta struct {
	title     string
	absolute  bool
	heatmap   bool
	dimension map[string]dimensionScale
}

type dimensionScale struct {
	multiplier int
	divisor    int
	hidden     bool
}

// addObservedScaleHeuristics uses the exact values already routed by the
// planner. It reports only a ratio and fingerprints the chart ID so observed
// label-derived identities and dynamic dimension names remain private.
func addObservedScaleHeuristics(plan chartengine.Plan, r *report) {
	charts := make(map[string]*chartScaleMeta)
	for _, action := range plan.Actions {
		switch item := action.(type) {
		case chartengine.CreateChartAction:
			charts[item.ChartID] = &chartScaleMeta{
				title:     item.Meta.Title,
				absolute:  string(item.Meta.Algorithm) == "absolute",
				heatmap:   string(item.Meta.Type) == "heatmap",
				dimension: make(map[string]dimensionScale),
			}
		case chartengine.CreateDimensionAction:
			meta := charts[item.ChartID]
			if meta == nil {
				continue
			}
			meta.dimension[item.Name] = dimensionScale{
				multiplier: item.Multiplier,
				divisor:    item.Divisor,
				hidden:     item.Hidden,
			}
		}
	}

	warned := make(map[string]struct{})
	for _, action := range plan.Actions {
		update, ok := action.(chartengine.UpdateChartAction)
		if !ok {
			continue
		}
		meta := charts[update.ChartID]
		if meta == nil || !meta.absolute || meta.heatmap {
			continue
		}

		var magnitudes []float64
		for _, value := range update.Values {
			if value.IsEmpty {
				continue
			}
			v := float64(value.Int64)
			if value.IsFloat {
				v = value.Float64
			}
			scale := meta.dimension[value.Name]
			if scale.hidden {
				continue
			}
			multiplier, divisor := scale.multiplier, scale.divisor
			if multiplier == 0 {
				multiplier = 1
			}
			if divisor == 0 {
				divisor = 1
			}
			v = math.Abs(v * float64(multiplier) / float64(divisor))
			if v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) {
				magnitudes = append(magnitudes, v)
			}
		}
		if len(magnitudes) < 2 {
			continue
		}
		minimum, maximum := slices.Min(magnitudes), slices.Max(magnitudes)
		ratio := maximum / minimum
		if ratio < 20 {
			continue
		}
		if _, ok := warned[update.ChartID]; ok {
			continue
		}
		warned[update.ChartID] = struct{}{}
		r.addWarning(
			"observed_scale_gap",
			fingerprintID(update.ChartID),
			fmt.Sprintf("chart %q has non-zero absolute dimensions differing by about %.0fx in the supplied dump", meta.title, ratio),
			"A shared axis can flatten the smaller signal. Split dimensions, normalize a meaningful ratio, or explain why the capacity/composition comparison remains useful.",
		)
	}
}
