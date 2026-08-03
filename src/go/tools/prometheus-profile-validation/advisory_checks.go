// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	promrelabel "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
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

type relabelDiscardAudit struct {
	action            promrelabel.Action
	blockMatch        string
	families          map[string]struct{}
	metricNames       map[string]struct{}
	logicalIdentities map[string]struct{}
	rawSeries         int
}

type relabelDiscardRuleKey struct {
	block int
	rule  int
}

type relabelNameRewriteAudit struct {
	metricNames          map[string]struct{}
	blockInputLabelNames map[string]struct{}
}

type relabelIdentityCollapseAudit struct {
	finalFamilies    []string
	finalIdentities  int
	sourceIdentities int
}

type relabelInvalidNameDropAudit struct {
	blocks            map[int]struct{}
	families          map[string]struct{}
	logicalIdentities map[string]struct{}
	rawSeries         int
}

type relabelPolicyAudits struct {
	discards         map[relabelDiscardRuleKey]*relabelDiscardAudit
	nameRewrites     map[relabelDiscardRuleKey]*relabelNameRewriteAudit
	identityCollapse relabelIdentityCollapseAudit
	invalidNameDrops relabelInvalidNameDropAudit
}

// addRelabelPolicyReview replays the validated job rules over the captured
// samples so every discard and bounded name rewrite has an evidence boundary.
func addRelabelPolicyReview(
	expr promselector.Expr,
	blocks []promcollector.RelabelBlock,
	fallback fallbackTypePolicy,
	batch prompkg.SampleBatch,
	r *report,
) relabelPolicyAudits {
	if len(blocks) == 0 {
		return relabelPolicyAudits{}
	}

	effectiveSelector, err := expr.Parse()
	if err != nil {
		return relabelPolicyAudits{} // Collector.Init reports the authoritative selector error.
	}

	type compiledBlock struct {
		match            matcher.Matcher
		proc             *promrelabel.Processor
		ruleInput        map[int]*promrelabel.Processor
		nameRewriteRules map[int]promrelabel.Config
	}
	compiled := make([]compiledBlock, 0, len(blocks))
	audits := relabelPolicyAudits{
		discards:     make(map[relabelDiscardRuleKey]*relabelDiscardAudit),
		nameRewrites: make(map[relabelDiscardRuleKey]*relabelNameRewriteAudit),
		invalidNameDrops: relabelInvalidNameDropAudit{
			blocks:            make(map[int]struct{}),
			families:          make(map[string]struct{}),
			logicalIdentities: make(map[string]struct{}),
		},
	}
	for blockIndex, block := range blocks {
		m, err := matcher.NewSimplePatternsMatcher(block.Match)
		if err != nil {
			return relabelPolicyAudits{} // Collector.Init reports the authoritative block error.
		}
		proc, err := promrelabel.New(block.MetricRelabelConfigs)
		if err != nil {
			return relabelPolicyAudits{} // Collector.Init reports the authoritative rule error.
		}
		item := compiledBlock{
			match:            m,
			proc:             proc,
			ruleInput:        make(map[int]*promrelabel.Processor),
			nameRewriteRules: make(map[int]promrelabel.Config),
		}
		compiled = append(compiled, item)
		for ruleIndex, config := range block.MetricRelabelConfigs {
			action, ok := sampleDiscardingRelabelAction(config.Action)
			if ok {
				audits.discards[relabelDiscardRuleKey{block: blockIndex, rule: ruleIndex}] = &relabelDiscardAudit{
					action:            action,
					blockMatch:        block.Match,
					families:          make(map[string]struct{}),
					metricNames:       make(map[string]struct{}),
					logicalIdentities: make(map[string]struct{}),
				}
			}
			isNameRewrite := normalizedRelabelAction(config.Action) == promrelabel.Replace &&
				len(config.SourceLabels) == 1 && config.SourceLabels[0] == promlabels.MetricName &&
				config.TargetLabel == promlabels.MetricName
			if isNameRewrite {
				key := relabelDiscardRuleKey{block: blockIndex, rule: ruleIndex}
				audits.nameRewrites[key] = &relabelNameRewriteAudit{
					metricNames:          make(map[string]struct{}),
					blockInputLabelNames: make(map[string]struct{}),
				}
				compiled[blockIndex].nameRewriteRules[ruleIndex] = config
			}
			if ruleIndex > 0 && (ok || isNameRewrite) {
				prefix, err := promrelabel.New(block.MetricRelabelConfigs[:ruleIndex])
				if err != nil {
					return relabelPolicyAudits{} // Collector.Init reports the authoritative rule error.
				}
				compiled[blockIndex].ruleInput[ruleIndex] = prefix
			}
		}
	}

	type relabeledIdentity struct {
		family           string
		sourceIdentities map[string]struct{}
	}
	finalIdentities := make(map[string]*relabeledIdentity)
	fallbackGauge := fallbackTypeMatcher(fallback.Gauge)
	fallbackCounter := fallbackTypeMatcher(fallback.Counter)

	for _, raw := range batch.Samples {
		if effectiveSelector != nil && !effectiveSelector.Matches(sampleLabelsWithName(raw)) {
			continue
		}
		sample := raw
		sampleDropped := false
		for blockIndex, block := range compiled {
			if !block.match.MatchString(sample.Name) {
				continue
			}
			for ruleIndex, config := range block.nameRewriteRules {
				input := sample
				if prefix := block.ruleInput[ruleIndex]; prefix != nil {
					processed, prefixDrop := prefix.Apply(sample)
					if prefixDrop.Dropped() {
						continue
					}
					input = processed
				}
				if config.Regex.MatchString(input.Name) {
					audit := audits.nameRewrites[relabelDiscardRuleKey{block: blockIndex, rule: ruleIndex}]
					audit.metricNames[input.Name] = struct{}{}
					for _, label := range sample.Labels {
						audit.blockInputLabelNames[label.Name] = struct{}{}
					}
				}
			}
			out, drop := block.proc.Apply(sample)
			if drop.Dropped() {
				sampleDropped = true
				if drop.Reason == promrelabel.DropReasonInvalidMetricName {
					family := sampleSourceFamilyName(raw)
					audits.invalidNameDrops.blocks[blockIndex] = struct{}{}
					audits.invalidNameDrops.families[family] = struct{}{}
					audits.invalidNameDrops.logicalIdentities[logicalSampleIdentityKey(family, raw)] = struct{}{}
					audits.invalidNameDrops.rawSeries++
				}
				key := relabelDiscardRuleKey{block: blockIndex, rule: drop.RuleIndex}
				if audit := audits.discards[key]; audit != nil {
					input := sample
					if prefix := block.ruleInput[drop.RuleIndex]; prefix != nil {
						if processed, prefixDrop := prefix.Apply(sample); !prefixDrop.Dropped() {
							input = processed
						}
					}
					family := sampleSourceFamilyName(raw)
					audit.families[family] = struct{}{}
					audit.metricNames[input.Name] = struct{}{}
					audit.logicalIdentities[logicalSampleIdentityKey(family, raw)] = struct{}{}
					audit.rawSeries++
				}
				break
			}
			sample = out
		}
		if !sampleDropped && sampleCanReachWriter(sample, fallbackGauge, fallbackCounter) {
			family := sampleSourceFamilyName(sample)
			key := logicalSampleIdentityKey(family, sample)
			identity := finalIdentities[key]
			if identity == nil {
				identity = &relabeledIdentity{
					family:           family,
					sourceIdentities: make(map[string]struct{}),
				}
				finalIdentities[key] = identity
			}
			identity.sourceIdentities[logicalSampleIdentityKey(sampleSourceFamilyName(raw), raw)] = struct{}{}
		}
	}

	collapsedSources := make(map[string]struct{})
	collapsedFamilies := make(map[string]struct{})
	for _, identity := range finalIdentities {
		if len(identity.sourceIdentities) < 2 {
			continue
		}
		audits.identityCollapse.finalIdentities++
		collapsedFamilies[identity.family] = struct{}{}
		for source := range identity.sourceIdentities {
			collapsedSources[source] = struct{}{}
		}
	}
	audits.identityCollapse.finalFamilies = slices.Sorted(maps.Keys(collapsedFamilies))
	audits.identityCollapse.sourceIdentities = len(collapsedSources)

	keys := slices.SortedFunc(maps.Keys(audits.discards), func(a, b relabelDiscardRuleKey) int {
		if a.block != b.block {
			return a.block - b.block
		}
		return a.rule - b.rule
	})
	for _, key := range keys {
		audit := audits.discards[key]
		path := fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d]", key.block, key.rule)
		message := fmt.Sprintf(
			"sample-discarding relabel action %q in block match %q dropped no samples in the supplied dump",
			audit.action,
			audit.blockMatch,
		)
		if audit.rawSeries > 0 {
			message = fmt.Sprintf(
				"sample-discarding relabel action %q in block match %q removes %d observed logical identities across %d source families (%d raw exposition series)",
				audit.action,
				audit.blockMatch,
				len(audit.logicalIdentities),
				len(audit.families),
				audit.rawSeries,
			)
		}
		r.addWarning(
			"job_relabel_discard_review",
			path,
			message,
			"Drop, keep, dropequal, and keepequal change the evidence denominator before profile coverage is measured. Confirm authoritative semantics and state which operator question is lost; zero observed drops do not prove the rule is harmless for unseen values.",
		)
	}
	return audits
}

func fallbackTypeMatcher(patterns []string) matcher.Matcher {
	m := matcher.FALSE()
	for _, pattern := range patterns {
		item, err := matcher.NewGlobMatcher(pattern)
		if err != nil {
			return matcher.FALSE() // Collector.Init reports the authoritative pattern error.
		}
		m = matcher.Or(m, item)
	}
	return m
}

func sampleCanReachWriter(sample prompkg.Sample, fallbackGauge, fallbackCounter matcher.Matcher) bool {
	family := sampleSourceFamilyName(sample)
	if strings.HasSuffix(family, "_info") {
		return false
	}
	if sample.Kind == prompkg.SampleKindScalar && (math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0)) {
		return false
	}
	switch sample.FamilyType {
	case commonmodel.MetricTypeGauge,
		commonmodel.MetricTypeCounter,
		commonmodel.MetricTypeHistogram,
		commonmodel.MetricTypeSummary:
		return true
	case commonmodel.MetricTypeUnknown:
		return strings.HasSuffix(family, "_total") ||
			(fallbackGauge != nil && fallbackGauge.MatchString(family)) ||
			(fallbackCounter != nil && fallbackCounter.MatchString(family))
	default:
		return false
	}
}

func normalizedRelabelAction(action promrelabel.Action) promrelabel.Action {
	action = promrelabel.Action(strings.ToLower(strings.TrimSpace(string(action))))
	if action == "" {
		return promrelabel.Replace
	}
	return action
}

func sampleDiscardingRelabelAction(action promrelabel.Action) (promrelabel.Action, bool) {
	action = normalizedRelabelAction(action)
	switch action {
	case promrelabel.Drop, promrelabel.DropEqual, promrelabel.Keep, promrelabel.KeepEqual:
		return action, true
	default:
		return "", false
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
	for _, identityToken := range chart.Instances.ByLabels {
		identityToken = strings.TrimSpace(identityToken)
		switch {
		case identityToken == "*":
			wildcard = true
		case strings.HasPrefix(identityToken, "!"):
			if key := strings.TrimSpace(strings.TrimPrefix(identityToken, "!")); key != "" {
				handled[key] = struct{}{}
			}
		case identityToken != "":
			handled[identityToken] = struct{}{}
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
// compiler defaulting can hide it. It reports objective presentation failures
// as errors and leaves judgment-dependent findings as warnings.
func addAuthoredProfileHeuristics(root charttpl.Group, rawFamilies []rawFamilyReport, r *report) {
	familyTypes := observedDistributionTypes(rawFamilies)
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
			reviewAuthoredChart(chart, chartPath, scoped, familyTypes, r)
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
	familyTypes map[string]commonmodel.MetricType,
	r *report,
) {
	hasBucket := false
	hasVisibleDimension := false
	for _, dimension := range chart.Dimensions {
		if dimension.Options == nil || !dimension.Options.Hidden {
			hasVisibleDimension = true
		}
		compiled, err := metrixselector.ParseCompiled(dimension.Selector)
		if err != nil {
			continue // Authoritative template/compiler validation reports errors.
		}
		for _, name := range compiled.Meta().MetricNames {
			if decl := active[name]; decl != nil {
				decl.used = true
			}
			if _, role, ok := distributionRole(name, familyTypes); ok && role == "bucket" {
				hasBucket = true
			}
		}
	}

	if !hasVisibleDimension {
		r.addError(
			"all_dimensions_hidden",
			path,
			fmt.Sprintf("chart %q hides every authored dimension", chart.Title),
			"A chart with no visible dimensions cannot answer an operator question. Keep at least one dimension visible; hidden dimensions may support a visible comparison but cannot replace it.",
		)
	}

	if hasBucket {
		if !strings.EqualFold(strings.TrimSpace(chart.Type), "heatmap") {
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
		if strings.TrimSpace(chart.Units) != "observations/s" {
			r.addError(
				"histogram_bucket_units",
				path,
				fmt.Sprintf("chart %q selects histogram buckets but declares units %q", chart.Title, chart.Units),
				"Metrix exposes non-overlapping bucket counters, so the heatmap intensity is an observation rate. Use units \"observations/s\"; the bucket boundaries already carry the observed value's unit.",
			)
		}
		if algorithm := strings.TrimSpace(chart.Algorithm); algorithm != "" && algorithm != "incremental" {
			r.addError(
				"histogram_bucket_algorithm",
				path,
				fmt.Sprintf("chart %q selects histogram buckets but declares algorithm %q", chart.Title, chart.Algorithm),
				"Histogram bucket values are counter-like totals after flattening and must render as change per second. Omit the algorithm for suffix inference or declare \"incremental\".",
			)
		}
	}

	chartType := strings.ToLower(strings.TrimSpace(chart.Type))
	if chartType == "area" || chartType == "stacked" {
		switch {
		case physicalVolumeUnits(chart.Units):
			if rateLikeUnits(chart.Units) {
				r.addWarning(
					"rate_filled_type_review",
					path,
					fmt.Sprintf("chart %q uses %s for physical rate units %q", chart.Title, chartType, chart.Units),
					"Bandwidth and I/O can justify meaningful fill, but confirm that the dimensions represent physical flow or volume rather than unrelated rates sharing a unit.",
				)
			}
		case unambiguouslyNonVolumeUnits(chart.Units):
			r.addError(
				"filled_nonvolume_units",
				path,
				fmt.Sprintf("chart %q uses %s for non-volume units %q", chart.Title, chartType, chart.Units),
				"Event, token, request, count, state, and time values must use line. Additive categories do not become physical volume merely because they sum.",
			)
		default:
			r.addWarning(
				"nonvolume_filled_type_review",
				path,
				fmt.Sprintf("chart %q uses %s for units %q whose fill semantics are not mechanically clear", chart.Title, chartType, chart.Units),
				"Use filled presentation only when the area represents physical volume, space, bandwidth, or I/O. Otherwise use line and preserve model judgment in the chart composition.",
			)
		}
	}
}

func rateLikeUnits(units string) bool {
	units = strings.ToLower(strings.TrimSpace(units))
	return strings.Contains(units, "/s") ||
		strings.Contains(units, "/sec") ||
		strings.Contains(units, "per second")
}

// addIncrementalUnitHeuristics uses the compiler-resolved runtime algorithm,
// avoiding a second implementation of chartengine's selector-kind inference.
func addIncrementalUnitHeuristics(charts []materializedChart, r *report) {
	type templateSummary struct {
		title     string
		units     string
		instances int
	}
	templates := make(map[string]*templateSummary)
	for _, chart := range charts {
		if chart.Autogen || chart.Algorithm != "incremental" {
			continue
		}
		item := templates[chart.TemplateID]
		if item == nil {
			item = &templateSummary{title: chart.Title, units: chart.Units}
			templates[chart.TemplateID] = item
		}
		item.instances++
	}
	for _, templateID := range slices.Sorted(maps.Keys(templates)) {
		item := templates[templateID]
		if rateLikeUnits(item.units) || incrementalRateEquivalentUnits(item.units) {
			continue
		}
		r.addWarning(
			"incremental_units_review",
			templateID,
			fmt.Sprintf(
				"incremental chart %q materializes %d instance(s) with non-rate units %q",
				item.title,
				item.instances,
				item.units,
			),
			"Netdata renders incremental values as per-second deltas. Use rate-bearing units that preserve the measured object, or document a truthful derived equivalent such as CPU cores or utilization; plain count/quantity units hide the temporal denominator.",
		)
	}
}

func incrementalRateEquivalentUnits(units string) bool {
	units = strings.ToLower(strings.TrimSpace(units))
	switch units {
	case "%", "percent", "percentage", "ratio", "utilization",
		"core", "cores", "concurrent", "concurrency", "in-flight", "in flight":
		return true
	default:
		return false
	}
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
	for _, field := range strings.FieldsFunc(units, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		switch field {
		case "b", "kb", "kib", "mb", "mib", "gb", "gib", "tb", "tib", "pb", "pib":
			return true
		}
	}
	return false
}

func unambiguouslyNonVolumeUnits(units string) bool {
	units = strings.ToLower(strings.TrimSpace(units))
	for _, token := range []string{
		"allocation",
		"choice",
		"collection",
		"connection",
		"core",
		"count",
		"cycle",
		"descriptor",
		"draft",
		"error",
		"event",
		"file",
		"hit",
		"item",
		"invocation",
		"latency",
		"miss",
		"object",
		"observation",
		"operation",
		"percent",
		"preemption",
		"process",
		"query",
		"queue",
		"ratio",
		"record",
		"request",
		"response",
		"retry",
		"sample",
		"second",
		"session",
		"state",
		"step",
		"task",
		"thread",
		"time",
		"token",
		"utilization",
		"worker",
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
	familyTypes := observedDistributionTypes(rawFamilies)

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

func observedDistributionTypes(rawFamilies []rawFamilyReport) map[string]commonmodel.MetricType {
	familyTypes := make(map[string]commonmodel.MetricType)
	for _, family := range rawFamilies {
		typ := commonmodel.MetricType(family.Type)
		if typ == commonmodel.MetricTypeHistogram || typ == commonmodel.MetricTypeSummary {
			familyTypes[family.Name] = typ
		}
	}
	return familyTypes
}

func distributionRole(
	name string,
	familyTypes map[string]commonmodel.MetricType,
) (string, string, bool) {
	if base := strings.TrimSuffix(name, "_bucket"); base != name &&
		familyTypes[base] == commonmodel.MetricTypeHistogram {
		return base, "bucket", true
	}
	if base, ok := strings.CutSuffix(name, "_count"); ok {
		if typ := familyTypes[base]; typ == commonmodel.MetricTypeHistogram || typ == commonmodel.MetricTypeSummary {
			return base, "count", true
		}
	}
	if base, ok := strings.CutSuffix(name, "_sum"); ok {
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
