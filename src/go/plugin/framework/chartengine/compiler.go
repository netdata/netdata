// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sort"
	"strings"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// Compile converts a validated chart template spec into immutable chartengine IR.
func Compile(spec *charttpl.Spec, revision uint64) (*program.Program, error) {
	if spec == nil {
		return nil, fmt.Errorf("chartengine: nil template spec")
	}
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("chartengine: invalid template spec: %w", err)
	}

	c := compiler{
		metricsSet: make(map[string]struct{}),
	}

	rootCtx := normalizeOptional(spec.ContextNamespace)
	for i := range spec.Groups {
		groupPath := []int{i}
		if err := c.compileGroup(spec.Groups[i], compileScope{
			metrics:      make(map[string]struct{}),
			familyParts:  nil,
			contextParts: rootCtx,
		}, groupPath); err != nil {
			return nil, err
		}
	}

	return program.New(spec.Version, revision, c.metricNames(), c.charts)
}

type compiler struct {
	charts     []program.Chart
	metricsSet map[string]struct{}
}

type compileScope struct {
	metrics      map[string]struct{}
	familyParts  []string
	contextParts []string
}

func (c *compiler) compileGroup(group charttpl.Group, parent compileScope, groupPath []int) error {
	scope := compileScope{
		metrics:      cloneStringSet(parent.metrics),
		familyParts:  append(append([]string(nil), parent.familyParts...), strings.TrimSpace(group.Family)),
		contextParts: append(append([]string(nil), parent.contextParts...), normalizeOptional(group.ContextNamespace)...),
	}

	for _, metric := range group.Metrics {
		name := strings.TrimSpace(metric)
		scope.metrics[name] = struct{}{}
		c.metricsSet[name] = struct{}{}
	}

	for i := range group.Charts {
		templateID := buildTemplateID(groupPath, i)
		compiled, err := c.compileChart(group.Charts[i], scope, templateID)
		if err != nil {
			return fmt.Errorf("chartengine: compile group[%s] chart[%d]: %w", pathIndexes(groupPath), i, err)
		}
		c.charts = append(c.charts, compiled)
	}

	for i := range group.Groups {
		nextPath := append(append([]int(nil), groupPath...), i)
		if err := c.compileGroup(group.Groups[i], scope, nextPath); err != nil {
			return err
		}
	}
	return nil
}

func (c *compiler) compileChart(chart charttpl.Chart, scope compileScope, templateID string) (program.Chart, error) {
	dimensions := make([]program.Dimension, 0, len(chart.Dimensions))
	selectorKeySet := make(map[string]struct{})
	dynamicDimensionKeys := make(map[string]struct{})

	metricKinds := make(map[string]bool)
	for i := range chart.Dimensions {
		compiledDim, err := compileDimension(chart.Dimensions[i], scope.metrics)
		if err != nil {
			return program.Chart{}, fmt.Errorf("dimension[%d]: %w", i, err)
		}
		dimensions = append(dimensions, compiledDim.dimension)

		for _, key := range compiledDim.selectorKeys {
			selectorKeySet[key] = struct{}{}
		}
		for _, key := range compiledDim.dynamicLabelKeys {
			dynamicDimensionKeys[key] = struct{}{}
		}
		for _, kind := range compiledDim.metricKinds {
			metricKinds[kind] = true
		}
	}

	algorithm, err := resolveAlgorithm(chart.Algorithm, metricKinds)
	if err != nil {
		return program.Chart{}, err
	}

	chartType, err := resolveChartType(chart.Type)
	if err != nil {
		return program.Chart{}, err
	}

	contextParts := append(append([]string(nil), scope.contextParts...), strings.TrimSpace(chart.Context))
	context := strings.Join(filterEmpty(contextParts), ".")

	baseID := strings.TrimSpace(chart.ID)
	if baseID == "" {
		// Phase-1 default when id is omitted: derive from chart context.
		baseID = strings.ReplaceAll(context, ".", "_")
	}

	idTemplate, err := parseTemplate(baseID)
	if err != nil {
		return program.Chart{}, fmt.Errorf("id: %w", err)
	}

	instanceByLabels, err := compileInstanceByLabels(chart.Instances)
	if err != nil {
		return program.Chart{}, fmt.Errorf("instances.by_labels: %w", err)
	}

	labelMode := program.PromotionModeAutoIntersection
	promote := normalizeUnique(chart.LabelPromoted)
	if len(promote) > 0 {
		labelMode = program.PromotionModeExplicitIntersection
	}

	identity := program.ChartIdentity{
		IDTemplate:       idTemplate,
		InstanceByLabels: instanceByLabels,
		ContextNamespace: append([]string(nil), scope.contextParts...),
		Static:           len(instanceByLabels) == 0,
	}
	metaFamily := composeFamily(scope.familyParts, chart.Family)

	out := program.Chart{
		TemplateID: templateID,
		Meta: program.ChartMeta{
			Title:     strings.TrimSpace(chart.Title),
			Family:    metaFamily,
			Context:   context,
			Units:     strings.TrimSpace(chart.Units),
			Algorithm: algorithm,
			Type:      chartType,
			Priority:  chart.Priority,
		},
		Identity: identity,
		Labels: program.LabelPolicy{
			Mode:        labelMode,
			PromoteKeys: promote,
			Exclusions: program.LabelExclusions{
				SelectorConstrainedKeys: mapKeysSorted(selectorKeySet),
				DimensionKeyLabels:      mapKeysSorted(dynamicDimensionKeys),
			},
			Precedence: program.DefaultLabelPrecedence(),
		},
		Lifecycle:       compileLifecycle(chart.Lifecycle),
		Dimensions:      dimensions,
		CollisionReduce: program.ReduceSum,
	}

	return out, nil
}

type compiledDimension struct {
	dimension        program.Dimension
	selectorKeys     []string
	dynamicLabelKeys []string
	metricKinds      []string
}

func compileDimension(dim charttpl.Dimension, visibleMetrics map[string]struct{}) (compiledDimension, error) {
	compiledSel, err := metrixselector.ParseCompiled(dim.Selector)
	if err != nil {
		return compiledDimension{}, fmt.Errorf("selector: %w", err)
	}

	meta := compiledSel.Meta()
	if len(visibleMetrics) > 0 {
		for _, metricName := range meta.MetricNames {
			if _, ok := visibleMetrics[metricName]; !ok {
				return compiledDimension{}, fmt.Errorf("selector: metric %q is not visible in current group scope", metricName)
			}
		}
	}

	name := strings.TrimSpace(dim.Name)
	nameFromLabel := strings.TrimSpace(dim.NameFromLabel)
	// If dimension naming is omitted, runtime planner resolves dynamic key source
	// from series origin metadata (metrix.Reader.SeriesMeta on flattened series).
	inferFromSeriesMeta := name == "" && nameFromLabel == ""
	if inferFromSeriesMeta && !supportsRuntimeInferredDimension(meta) {
		return compiledDimension{}, fmt.Errorf(
			"name inference requires inferable selector (histogram bucket/summary quantile/stateset-like metric); set name or name_from_label",
		)
	}

	nameTemplate := program.Template{}
	dynamicLabelKeys := make([]string, 0, 1)
	if name != "" {
		nameTemplate, err = parseTemplate(name)
		if err != nil {
			return compiledDimension{}, fmt.Errorf("name: %w", err)
		}
	} else if nameFromLabel != "" {
		dynamicLabelKeys = append(dynamicLabelKeys, nameFromLabel)
	}

	metricKinds := metricKindsFromNames(meta.MetricNames)
	options := compileDimensionOptions(dim.Options)

	return compiledDimension{
		dimension: program.Dimension{
			Selector: program.SelectorBinding{
				Expression:           strings.TrimSpace(dim.Selector),
				Matcher:              selectorMatcher{compiled: compiledSel},
				MetricNames:          append([]string(nil), meta.MetricNames...),
				ConstrainedLabelKeys: append([]string(nil), meta.ConstrainedLabelKeys...),
			},
			NameTemplate:            nameTemplate,
			NameFromLabel:           nameFromLabel,
			InferNameFromSeriesMeta: inferFromSeriesMeta,
			Hidden:                  options.hidden,
			Multiplier:              options.multiplier,
			Divisor:                 options.divisor,
			Float:                   options.float,
			Dynamic:                 inferFromSeriesMeta || nameFromLabel != "",
		},
		selectorKeys:     append([]string(nil), meta.ConstrainedLabelKeys...),
		dynamicLabelKeys: normalizeUnique(dynamicLabelKeys),
		metricKinds:      metricKinds,
	}, nil
}

type compiledDimensionOptions struct {
	hidden     bool
	multiplier int
	divisor    int
	float      bool
}

func compileDimensionOptions(in *charttpl.DimensionOptions) compiledDimensionOptions {
	out := compiledDimensionOptions{
		multiplier: 1,
		divisor:    1,
	}
	if in == nil {
		return out
	}
	out.hidden = in.Hidden
	out.float = in.Float
	if in.Multiplier != 0 {
		out.multiplier = in.Multiplier
	}
	if in.Divisor != 0 {
		out.divisor = in.Divisor
	}
	return out
}

func resolveAlgorithm(raw string, metricKinds map[string]bool) (program.Algorithm, error) {
	normalized := strings.TrimSpace(raw)
	if normalized != "" {
		switch normalized {
		case string(program.AlgorithmAbsolute):
			return program.AlgorithmAbsolute, nil
		case string(program.AlgorithmIncremental):
			return program.AlgorithmIncremental, nil
		default:
			return "", fmt.Errorf("invalid algorithm %q", raw)
		}
	}

	// Inference baseline:
	// - counter-like selectors => incremental
	// - gauge-like selectors => absolute
	// - mixed inferred kinds must be explicit.
	if metricKinds["counter_like"] && metricKinds["gauge_like"] {
		return "", fmt.Errorf("algorithm inference is ambiguous for mixed metric kinds; set algorithm explicitly")
	}
	if metricKinds["counter_like"] {
		return program.AlgorithmIncremental, nil
	}
	return program.AlgorithmAbsolute, nil
}

func resolveChartType(raw string) (program.ChartType, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return program.ChartTypeLine, nil
	}
	switch normalized {
	case string(program.ChartTypeLine):
		return program.ChartTypeLine, nil
	case string(program.ChartTypeArea):
		return program.ChartTypeArea, nil
	case string(program.ChartTypeStacked):
		return program.ChartTypeStacked, nil
	case string(program.ChartTypeHeatmap):
		return program.ChartTypeHeatmap, nil
	default:
		return "", fmt.Errorf("invalid chart type %q", raw)
	}
}

func compileLifecycle(in *charttpl.Lifecycle) program.LifecyclePolicy {
	out := defaultChartLifecyclePolicyCopy()
	if in == nil {
		return out
	}
	out.MaxInstances = in.MaxInstances
	if in.ExpireAfterCycles > 0 {
		out.ExpireAfterCycles = in.ExpireAfterCycles
	}
	if in.Dimensions != nil {
		out.Dimensions.MaxDims = in.Dimensions.MaxDims
		out.Dimensions.ExpireAfterCycles = in.Dimensions.ExpireAfterCycles
	}
	return out
}

func compileInstanceByLabels(instances *charttpl.Instances) ([]program.InstanceLabelSelector, error) {
	if instances == nil {
		return nil, nil
	}
	out := make([]program.InstanceLabelSelector, 0, len(instances.ByLabels))
	for _, token := range instances.ByLabels {
		t := strings.TrimSpace(token)
		switch {
		case t == "*":
			out = append(out, program.InstanceLabelSelector{IncludeAll: true})
		case strings.HasPrefix(t, "!"):
			key := strings.TrimSpace(strings.TrimPrefix(t, "!"))
			if key == "" {
				return nil, fmt.Errorf("exclude token must include label key")
			}
			out = append(out, program.InstanceLabelSelector{Exclude: true, Key: key})
		default:
			out = append(out, program.InstanceLabelSelector{Key: t})
		}
	}
	return out, nil
}

func metricKindsFromNames(names []string) []string {
	seen := make(map[string]struct{})
	for _, name := range names {
		switch {
		case strings.HasSuffix(name, "_total"):
			seen["counter_like"] = struct{}{}
		case strings.HasSuffix(name, "_count"):
			seen["counter_like"] = struct{}{}
		case strings.HasSuffix(name, "_sum"):
			seen["counter_like"] = struct{}{}
		case strings.HasSuffix(name, "_bucket"):
			seen["counter_like"] = struct{}{}
		default:
			seen["gauge_like"] = struct{}{}
		}
	}
	return mapKeysSorted(seen)
}

func supportsRuntimeInferredDimension(meta metrixselector.Meta) bool {
	for _, key := range meta.ConstrainedLabelKeys {
		switch key {
		case histogramBucketLabel, summaryQuantileLabel:
			return true
		}
	}
	for _, name := range meta.MetricNames {
		name = strings.TrimSpace(name)
		switch {
		case strings.HasSuffix(name, "_bucket"):
			return true
		case strings.HasSuffix(name, "_state"):
			return true
		case strings.HasSuffix(name, "_status"):
			return true
		case strings.HasSuffix(name, "_mode"):
			return true
		}
		if idx := strings.LastIndexByte(name, '.'); idx >= 0 && idx < len(name)-1 {
			name = name[idx+1:]
		}
		if name == "state" || name == "status" || name == "mode" {
			return true
		}
	}
	return false
}

func composeFamily(parts []string, leaf string) string {
	out := make([]string, 0, len(parts)+1)
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if leaf = strings.TrimSpace(leaf); leaf != "" {
		out = append(out, leaf)
	}
	return strings.Join(out, "/")
}

func buildTemplateID(groupPath []int, chartIndex int) string {
	return fmt.Sprintf("g%s.c%d", pathIndexes(groupPath), chartIndex)
}

func pathIndexes(path []int) string {
	if len(path) == 0 {
		return "root"
	}
	var b strings.Builder
	for i, idx := range path {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(fmt.Sprintf("%d", idx))
	}
	return b.String()
}

func (c *compiler) metricNames() []string {
	return mapKeysSorted(c.metricsSet)
}

func normalizeOptional(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func filterEmpty(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func cloneStringSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func normalizeUnique(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return mapKeysSorted(set)
}

func mapKeysSorted(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// selectorMatcher adapts prometheus selector API to chartengine selector binding.
type selectorMatcher struct {
	compiled metrixselector.Compiled
}

func (m selectorMatcher) Matches(metricName string, lbs program.SelectorLabels) bool {
	return m.compiled.Matches(metricName, selectorLabelView{labels: lbs})
}

type selectorLabelView struct {
	labels program.SelectorLabels
}

func (v selectorLabelView) Len() int {
	if v.labels == nil {
		return 0
	}
	return v.labels.Len()
}

func (v selectorLabelView) Get(key string) (string, bool) {
	if v.labels == nil {
		return "", false
	}
	return v.labels.Get(key)
}

func (v selectorLabelView) Range(fn func(key, value string) bool) {
	if v.labels == nil {
		return
	}
	v.labels.Range(fn)
}

func (v selectorLabelView) CloneMap() map[string]string {
	if v.labels == nil {
		return nil
	}
	out := make(map[string]string, v.labels.Len())
	v.labels.Range(func(key, value string) bool {
		out[key] = value
		return true
	})
	return out
}
