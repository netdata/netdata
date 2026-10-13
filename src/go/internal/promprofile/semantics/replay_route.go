// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

type expectedSemanticEdge struct {
	sourceIndex        int
	sourceProfile      string
	signal             string
	component          string
	occurrenceID       string
	destinationProfile string
	context            string
	fullContext        string
	view               string
	input              string
	renderedRole       string
	metricName         string
	family             string
	identityLabels     []string
	chartLabels        []promreplay.SemanticLabel
	dimensionName      string
	dimensionKeyLabel  string
	promotionMode      string
	promotionModeAlias string
	promotedLabels     []string
	algorithm          string
	seriesKind         string
	aggregation        string
	units              string
	multiplier         int64
	divisor            int64
	presentation       string
}

type matchedSemanticEdge struct {
	actual promreplay.SemanticRoute
}

type semanticRouteFingerprint struct {
	profile          string
	metricName       string
	context          string
	family           string
	identity         string
	chartLabels      string
	chartLabelValues string
	dimensionName    string
	dimensionKey     string
	promotionMode    string
	promotedLabels   string
	algorithm        string
	seriesKind       string
	aggregation      string
	units            string
	multiplier       int64
	divisor          int64
	presentation     string
}

// ReconcileProductionRoutes derives the active semantic edge multiset and
// compares it bidirectionally with authored routes emitted by production.
func (c *CompiledSemanticCase) ReconcileProductionRoutes(
	ctx context.Context,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) error {
	if err := checkSemanticContext(ctx, "before production route reconciliation"); err != nil {
		return err
	}
	if c == nil || c.root == nil {
		return fmt.Errorf("production route reconciliation: compiled semantic case is nil")
	}
	if snapshot == nil || reconciled == nil {
		return fmt.Errorf("production route reconciliation: snapshot and reconciled sources must be present")
	}
	if len(reconciled.Sources) != len(snapshot.Sources) {
		return fmt.Errorf("production route reconciliation: source join has %d entries for %d production sources",
			len(reconciled.Sources), len(snapshot.Sources))
	}

	reconciled.Edges = nil
	reconciled.Exclusions = nil
	terminalNormalizations, err := terminalNormalizationsBySource(reconciled.Normalizations, len(snapshot.Sources))
	if err != nil {
		return fmt.Errorf("production route reconciliation: %w", err)
	}
	autogenDenies := productionAutogenDenies(snapshot.Profiles)
	var matches []matchedSemanticEdge
	var errs []error
	for _, binding := range reconciled.Sources {
		if err := checkSemanticContext(ctx, "during production route reconciliation"); err != nil {
			return err
		}
		if binding.SourceIndex < 0 || binding.SourceIndex >= len(snapshot.Sources) ||
			binding.program == nil || binding.occurrence == nil {
			errs = append(errs, fmt.Errorf("source join contains an invalid binding at index %d", binding.SourceIndex))
			continue
		}
		source := snapshot.Sources[binding.SourceIndex]
		writerIneligible, err := reconcileWriterIneligibleSource(binding, source)
		if err != nil {
			errs = append(errs, fmt.Errorf("raw occurrence %s (%s/%s): %w",
				source.OccurrenceID, source.MetricName, source.Component, err))
			continue
		}
		if writerIneligible {
			continue
		}
		if terminal, ok := terminalNormalizations[binding.SourceIndex]; ok {
			if err := validateNormalizationTerminalDisposition(source, terminal); err != nil {
				errs = append(errs, fmt.Errorf("raw occurrence %s (%s/%s): %w",
					source.OccurrenceID, source.MetricName, source.Component, err))
			}
			continue
		}
		if binding.occurrence.terminalExclusion != "" {
			errs = append(errs, fmt.Errorf(
				"raw occurrence %s (%s/%s) is owned by terminal normalization %q but has no reconciled terminal fact",
				source.OccurrenceID, source.MetricName, source.Component, binding.occurrence.terminalExclusion,
			))
			continue
		}
		exclusion, err := c.activeDesignExclusion(binding, source)
		if err != nil {
			errs = append(errs, fmt.Errorf("raw occurrence %s (%s/%s): %w",
				source.OccurrenceID, source.MetricName, source.Component, err))
			continue
		}
		if exclusion != nil {
			fact, err := reconcileDesignExclusion(binding, source, exclusion, autogenDenies[binding.Profile])
			if err != nil {
				errs = append(errs, fmt.Errorf("raw occurrence %s (%s/%s): %w",
					source.OccurrenceID, source.MetricName, source.Component, err))
				continue
			}
			reconciled.Exclusions = append(reconciled.Exclusions, fact)
		}
		expected := []expectedSemanticEdge(nil)
		if exclusion == nil {
			expected, err = c.expectedSemanticEdges(binding, source, snapshot.ContextRoot)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("raw occurrence %s (%s/%s): %w",
				source.OccurrenceID, source.MetricName, source.Component, err))
			continue
		}
		if exclusion == nil {
			if len(expected) == 0 {
				errs = append(errs, fmt.Errorf(
					"raw occurrence %s (%s/%s) has no expected authored destination or intentional terminal exclusion",
					source.OccurrenceID, source.MetricName, source.Component,
				))
			}
			if err := validateRoutedSourceDisposition(source, len(expected)); err != nil {
				errs = append(errs, fmt.Errorf("raw occurrence %s (%s/%s): %w",
					source.OccurrenceID, source.MetricName, source.Component, err))
			}
		}
		actual := source.Routes
		used := make([]bool, len(actual))
		actualByFingerprint := make(map[semanticRouteFingerprint][]int, len(actual))
		for index, route := range actual {
			fingerprint := actualSemanticRouteFingerprint(route)
			actualByFingerprint[fingerprint] = append(actualByFingerprint[fingerprint], index)
		}
		for _, want := range expected {
			fingerprint := expectedSemanticRouteFingerprint(want)
			candidates := actualByFingerprint[fingerprint]
			if len(candidates) == 0 && want.promotionModeAlias != "" {
				fingerprint.promotionMode = want.promotionModeAlias
				candidates = actualByFingerprint[fingerprint]
			}
			found := -1
			if len(candidates) != 0 {
				found = candidates[len(candidates)-1]
				actualByFingerprint[fingerprint] = candidates[:len(candidates)-1]
			}
			if found < 0 {
				closest := closestSemanticRouteMismatch(actual, used, want)
				errs = append(errs, fmt.Errorf(
					"semantic edge %s/%s/%s -> %s/%s#%s/%s has no exact production route%s",
					want.sourceProfile, want.signal, want.component,
					want.destinationProfile, want.context, want.input, want.renderedRole,
					formatSemanticRouteMismatch(closest),
				))
				continue
			}
			used[found] = true
			got := actual[found]
			if got.TemplatePath == "" || got.ChartID == "" || got.DimensionName == "" {
				errs = append(errs, fmt.Errorf(
					"semantic edge %s/%s#%s has empty production template/chart/dimension identity",
					want.destinationProfile, want.context, want.input))
				continue
			}
			matches = append(matches, matchedSemanticEdge{actual: got})
			reconciled.Edges = append(reconciled.Edges, ReconciledSemanticEdge{
				SourceIndex:        want.sourceIndex,
				SourceProfile:      want.sourceProfile,
				Signal:             want.signal,
				Component:          want.component,
				OccurrenceID:       want.occurrenceID,
				DestinationProfile: want.destinationProfile,
				Context:            want.context,
				View:               want.view,
				Input:              want.input,
				RenderedRole:       want.renderedRole,
				TemplatePath:       got.TemplatePath,
				ChartID:            got.ChartID,
				DimensionIndex:     got.DimensionIndex,
				DimensionName:      got.DimensionName,
			})
		}
		for index, got := range actual {
			if !used[index] {
				errs = append(errs, fmt.Errorf(
					"raw occurrence %s has unexpected production route profile=%q context=%q chart=%q dimension=%q",
					source.OccurrenceID, got.Profile, got.Context, got.ChartID, got.DimensionName,
				))
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if err := reconcileSemanticContributorCounts(matches); err != nil {
		return err
	}
	slices.SortFunc(reconciled.Edges, compareReconciledSemanticEdges)
	slices.SortFunc(reconciled.Exclusions, func(left, right ReconciledSemanticExclusion) int {
		if left.SourceIndex != right.SourceIndex {
			return left.SourceIndex - right.SourceIndex
		}
		return strings.Compare(left.Exclusion, right.Exclusion)
	})
	return nil
}

func terminalNormalizationsBySource(
	facts []ReconciledSemanticNormalization,
	sourceCount int,
) (map[int]ReconciledSemanticNormalization, error) {
	terminals := make(map[int]ReconciledSemanticNormalization)
	for _, fact := range facts {
		if !fact.Terminal {
			continue
		}
		if fact.SourceIndex < 0 || fact.SourceIndex >= sourceCount {
			return nil, fmt.Errorf("terminal normalization %q has invalid source index %d",
				fact.Normalization, fact.SourceIndex)
		}
		if previous, ok := terminals[fact.SourceIndex]; ok {
			return nil, fmt.Errorf("source index %d has multiple terminal normalizations %q and %q",
				fact.SourceIndex, previous.Normalization, fact.Normalization)
		}
		terminals[fact.SourceIndex] = fact
	}
	return terminals, nil
}

func validateNormalizationTerminalDisposition(
	source promreplay.SemanticSource,
	fact ReconciledSemanticNormalization,
) error {
	if source.Terminal == nil || source.Terminal.Disposition != "profile_excluded" ||
		source.Terminal.Profile != fact.Profile {
		return fmt.Errorf("normalization %q expects profile exclusion, got %#v", fact.Normalization, source.Terminal)
	}
	if len(source.Routes) != 0 || source.WriterSeries != 0 || source.AutogenSeries != 0 ||
		source.UnmatchedSeries != 0 || len(source.AutogenSuppressions) != 0 {
		return fmt.Errorf(
			"normalization %q terminal reached route/writer state routes=%d writer=%d autogen=%d unmatched=%d suppressed=%d",
			fact.Normalization, len(source.Routes), source.WriterSeries, source.AutogenSeries,
			source.UnmatchedSeries, len(source.AutogenSuppressions),
		)
	}
	return nil
}

func reconcileWriterIneligibleSource(
	binding ReconciledSemanticSource,
	source promreplay.SemanticSource,
) (bool, error) {
	if binding.entry.registration.prometheus.Shape != "info" {
		if source.Terminal != nil && source.Terminal.Disposition == "writer_ineligible" {
			return false, fmt.Errorf("source-declared writable shape %q was rejected by writer with reason %q",
				binding.entry.registration.prometheus.Shape, source.Terminal.WriterReason)
		}
		return false, nil
	}
	if source.Terminal == nil || source.Terminal.Disposition != "writer_ineligible" ||
		source.Terminal.WriterReason != "info_family" {
		return false, fmt.Errorf("source-declared info family requires writer rejection reason info_family, got %#v",
			source.Terminal)
	}
	if len(source.Routes) != 0 || source.WriterSeries != 0 || source.AutogenSeries != 0 || source.UnmatchedSeries != 0 ||
		len(source.AutogenSuppressions) != 0 {
		return false, fmt.Errorf(
			"source-declared info family reached route/writer state routes=%d writer=%d autogen=%d unmatched=%d suppressed=%d",
			len(source.Routes), source.WriterSeries, source.AutogenSeries, source.UnmatchedSeries,
			len(source.AutogenSuppressions))
	}
	return true, nil
}

func (c *CompiledSemanticCase) expectedSemanticEdges(
	binding ReconciledSemanticSource,
	source promreplay.SemanticSource,
	contextRoot string,
) ([]expectedSemanticEdge, error) {
	labels := semanticPipelineLabelMap(source.FinalLabels)
	var out []expectedSemanticEdge
	for _, destinationProfile := range c.ActiveProfiles() {
		destination := c.programs[destinationProfile]
		for _, contextID := range sortedMapKeys(destination.views) {
			view := destination.views[contextID]
			for _, inputID := range sortedMapKeys(view.inputs) {
				input := view.inputs[inputID]
				for _, occurrence := range input.occurrences {
					if occurrence.program != binding.program || occurrence.occurrence != binding.occurrence ||
						!c.viewOccurrenceActive(destination, view, occurrence) ||
						!replayLabelConditionMatches(input.definition.Where, labels) {
						continue
					}
					edge, err := expectedSemanticRoute(
						binding, source, destination, contextID, view, input, occurrence, contextRoot, labels,
					)
					if err != nil {
						return nil, fmt.Errorf("view %q input %q: %w", contextID, inputID, err)
					}
					out = append(out, edge)
				}
			}
		}
	}
	return out, nil
}

func (c *CompiledSemanticCase) viewOccurrenceActive(
	destination *CompiledSemanticContract,
	view *compiledView,
	occurrence compiledViewOccurrence,
) bool {
	sourceAssignment, sourceActive := c.assignments[occurrence.sourceProfile]
	destinationAssignment, destinationActive := c.assignments[destination.profile]
	return sourceActive && destinationActive &&
		occurrence.occurrence.availability.evaluate(occurrence.program.environment.axes, sourceAssignment) &&
		occurrence.destinationAvailability.evaluate(view.destinationAxes, destinationAssignment)
}

func (c *CompiledSemanticCase) activeDesignExclusion(
	binding ReconciledSemanticSource,
	source promreplay.SemanticSource,
) (*compiledDesignExclusion, error) {
	assignment := c.assignments[binding.Profile]
	labels := semanticPipelineLabelMap(source.FinalLabels)
	var found *compiledDesignExclusion
	for _, exclusion := range binding.program.exclusions {
		if !exclusion.availability.evaluate(binding.program.environment.axes, assignment) ||
			!replayLabelConditionMatches(exclusion.definition.Source.Where, labels) {
			continue
		}
		for _, occurrence := range exclusion.occurrences {
			if occurrence.occurrence == binding.occurrence &&
				occurrence.availability.evaluate(binding.program.environment.axes, assignment) {
				if found != nil {
					return nil, fmt.Errorf("matches design exclusions %q and %q", found.id, exclusion.id)
				}
				found = exclusion
			}
		}
	}
	return found, nil
}

func reconcileDesignExclusion(
	binding ReconciledSemanticSource,
	source promreplay.SemanticSource,
	exclusion *compiledDesignExclusion,
	autogenDenies map[string]struct{},
) (ReconciledSemanticExclusion, error) {
	fact := ReconciledSemanticExclusion{
		SourceIndex: binding.SourceIndex,
		Profile:     binding.Profile,
		Exclusion:   exclusion.id,
		Outcome:     exclusion.definition.Outcome,
	}
	if exclusion.definition.Reason == "metadata_only" && source.Value != 1 {
		return fact, fmt.Errorf("design exclusion %q metadata carrier value is %v, want 1",
			exclusion.id, source.Value)
	}
	if len(source.Routes) != 0 {
		return fact, fmt.Errorf("design exclusion %q has %d authored production routes",
			exclusion.id, len(source.Routes))
	}
	switch exclusion.definition.Outcome {
	case "drop_before_writer":
		if source.Terminal == nil || source.Terminal.Disposition != "profile_excluded" ||
			source.Terminal.Profile != binding.Profile || source.Terminal.RuntimePath == "" {
			return fact, fmt.Errorf("design exclusion %q expects profile drop before writer, got %#v",
				exclusion.id, source.Terminal)
		}
		if source.WriterSeries != 0 || source.AutogenSeries != 0 || source.UnmatchedSeries != 0 ||
			len(source.AutogenSuppressions) != 0 {
			return fact, fmt.Errorf(
				"design exclusion %q reaches writer/autogen state writer=%d autogen=%d unmatched=%d suppressed=%d",
				exclusion.id, source.WriterSeries, source.AutogenSeries, source.UnmatchedSeries,
				len(source.AutogenSuppressions))
		}
		if !slices.ContainsFunc(source.RelabelRules, func(rule promreplay.SemanticRelabelOccurrence) bool {
			return rule.Profile == binding.Profile && rule.RuntimePath == source.Terminal.RuntimePath &&
				rule.Action == "drop" && rule.Matched && rule.Dropped
		}) {
			return fact, fmt.Errorf("design exclusion %q terminal path %q is not an exercised profile drop",
				exclusion.id, source.Terminal.RuntimePath)
		}
		fact.RuntimePath = source.Terminal.RuntimePath
	case "retain_writable_unrendered":
		if source.Terminal != nil {
			return fact, fmt.Errorf("design exclusion %q must remain writable, got terminal %#v",
				exclusion.id, source.Terminal)
		}
		if source.WriterSeries == 0 || source.AutogenSeries != 0 || source.UnmatchedSeries != 0 {
			return fact, fmt.Errorf(
				"design exclusion %q retained state got writer=%d autogen=%d unmatched=%d, want writer>0 autogen=0 unmatched=0",
				exclusion.id, source.WriterSeries, source.AutogenSeries, source.UnmatchedSeries)
		}
		family, ok := replaySourceFamily(source.FinalMetricName, source.Component)
		if !ok {
			return fact, fmt.Errorf("design exclusion %q cannot derive final writer family from %q/%q",
				exclusion.id, source.FinalMetricName, source.Component)
		}
		if _, ok := autogenDenies[family]; !ok {
			return fact, fmt.Errorf("design exclusion %q retained family %q has no exact autogen deny in profile %q",
				exclusion.id, family, binding.Profile)
		}
		if len(source.AutogenSuppressions) == 0 {
			return fact, fmt.Errorf("design exclusion %q retained family %q is not suppressed from autogen",
				exclusion.id, family)
		}
		for _, suppression := range source.AutogenSuppressions {
			if suppression.Profile != binding.Profile {
				return fact, fmt.Errorf("design exclusion %q is suppressed by profile %q, want owner %q",
					exclusion.id, suppression.Profile, binding.Profile)
			}
			if suppression.Family != family {
				return fact, fmt.Errorf("design exclusion %q suppresses family %q, want %q",
					exclusion.id, suppression.Family, family)
			}
		}
		fact.AutogenFamily = family
		fact.SuppressedSeries = len(source.AutogenSuppressions)
	default:
		panic("validated design exclusion outcome has no replay implementation")
	}
	return fact, nil
}

func validateRoutedSourceDisposition(source promreplay.SemanticSource, expectedEdges int) error {
	if expectedEdges == 0 {
		return nil
	}
	if source.Terminal != nil {
		return fmt.Errorf("expected authored destination reached terminal %#v", source.Terminal)
	}
	if source.WriterSeries == 0 {
		return fmt.Errorf("expected authored destination has no writer series")
	}
	if source.AutogenSeries != 0 || source.UnmatchedSeries != 0 {
		return fmt.Errorf("expected authored destination also reaches generic fallback autogen=%d unmatched=%d",
			source.AutogenSeries, source.UnmatchedSeries)
	}
	if len(source.AutogenSuppressions) != 0 {
		return fmt.Errorf("expected authored destination is also suppressed from autogen by %v", source.AutogenSuppressions)
	}
	return nil
}

func productionAutogenDenies(profiles []promreplay.SemanticProfile) map[string]map[string]struct{} {
	result := make(map[string]map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		families := make(map[string]struct{}, len(profile.AutogenSelectorDeny))
		for _, family := range profile.AutogenSelectorDeny {
			families[family] = struct{}{}
		}
		result[profile.Name] = families
	}
	return result
}

func expectedSemanticRoute(
	binding ReconciledSemanticSource,
	source promreplay.SemanticSource,
	destination *CompiledSemanticContract,
	contextID string,
	view *compiledView,
	input *compiledViewInput,
	occurrence compiledViewOccurrence,
	contextRoot string,
	labels map[string]string,
) (expectedSemanticEdge, error) {
	identity, err := resolveExpectedIdentity(view.entity, labels)
	if err != nil {
		return expectedSemanticEdge{}, err
	}
	dimensionName, dimensionKey, err := resolveExpectedDimension(view, input, occurrence, source, labels)
	if err != nil {
		return expectedSemanticEdge{}, err
	}
	promotionMode, promotionModeAlias := expectedPromotionMode(view)
	promoted := slices.Clone(view.labels.Promote)
	slices.Sort(promoted)
	chartLabels, err := expectedChartLabels(identity, promoted, labels)
	if err != nil {
		return expectedSemanticEdge{}, err
	}
	multiplier := view.scale.multiplier
	if input.definition.Direction != nil {
		multiplier = -multiplier
	}
	aggregation := "sum"
	if view.reduction != nil {
		aggregation = view.reduction.Reducer
	}
	relativeContext := expectedRelativeContext(destination.header.namespace, contextID, contextRoot)
	return expectedSemanticEdge{
		sourceIndex:        binding.SourceIndex,
		sourceProfile:      binding.Profile,
		signal:             binding.Signal,
		component:          binding.Component,
		occurrenceID:       source.OccurrenceID,
		destinationProfile: destination.profile,
		context:            relativeContext,
		fullContext:        joinSemanticContext(contextRoot, relativeContext),
		view:               contextID,
		input:              input.id,
		renderedRole:       input.renderedRole,
		metricName:         source.FinalMetricName,
		family:             view.definition.Family,
		identityLabels:     identity,
		chartLabels:        chartLabels,
		dimensionName:      dimensionName,
		dimensionKeyLabel:  dimensionKey,
		promotionMode:      promotionMode,
		promotionModeAlias: promotionModeAlias,
		promotedLabels:     promoted,
		algorithm:          occurrence.algorithm,
		seriesKind:         expectedSeriesKind(binding, occurrence),
		aggregation:        aggregation,
		units:              view.unit,
		multiplier:         multiplier,
		divisor:            view.scale.divisor,
		presentation:       view.presentation,
	}, nil
}

func resolveExpectedIdentity(entity EntityDefinition, labels map[string]string) ([]string, error) {
	out := make([]string, 0, len(entityIdentityLabels(entity.Identity)))
	for _, label := range entity.Identity.Required {
		if strings.TrimSpace(labels[label]) == "" {
			return nil, fmt.Errorf("required identity label %q is absent or blank", label)
		}
		out = append(out, label)
	}
	if len(entity.Identity.Alternatives) != 0 {
		selected, err := selectedLabelAlternative(entity.Identity.Alternatives, labels)
		if err != nil {
			return nil, fmt.Errorf("identity alternatives: %w", err)
		}
		out = append(out, entity.Identity.Alternatives[selected]...)
	}
	for _, label := range entity.Identity.Optional {
		if strings.TrimSpace(labels[label]) != "" {
			out = append(out, label)
		}
	}
	return out, nil
}

func resolveExpectedDimension(
	view *compiledView,
	input *compiledViewInput,
	occurrence compiledViewOccurrence,
	source promreplay.SemanticSource,
	labels map[string]string,
) (string, string, error) {
	structural := structuralLabelForComponent(occurrence.component.source.WireRole)
	applicable := make([]string, 0, len(view.labels.Dimensions))
	for _, label := range sortedMapKeys(view.labels.Dimensions) {
		if _, ok := occurrence.occurrence.labels[label]; ok {
			applicable = append(applicable, label)
		}
	}
	if structural != "" && len(applicable) != 0 {
		return "", "", fmt.Errorf("structural component also has semantic dimension labels %v", applicable)
	}
	if len(applicable) > 1 {
		return "", "", fmt.Errorf("source occurrence has multiple semantic dimension labels %v", applicable)
	}
	if structural != "" {
		value := source.WriterStructuralValue
		if value != "" && source.WriterStructuralLabel != structural {
			return "", "", fmt.Errorf(
				"writer structural label got %q, want %q",
				source.WriterStructuralLabel, structural,
			)
		}
		if value == "" {
			value = labels[structural]
		}
		if value == "" {
			value = semanticLabelMap(source.Labels)[structural]
		}
		if value == "" {
			return "", "", fmt.Errorf("structural dimension label %q is absent or blank", structural)
		}
		return value, structural, nil
	}
	if len(applicable) == 0 {
		return input.renderedRole, "", nil
	}
	label := applicable[0]
	switch view.labels.Dimensions[label].Render {
	case "label_value":
		value := labels[label]
		if value == "" {
			return "", "", fmt.Errorf("dimension label %q is absent or blank", label)
		}
		return value, label, nil
	case "input_role":
		return input.renderedRole, "", nil
	default:
		panic("validated dimension rendering has no replay implementation")
	}
}

func expectedPromotionMode(view *compiledView) (string, string) {
	if len(view.labels.Promote) != 0 {
		return "allowlist", ""
	}
	if len(view.labels.Omit) != 0 {
		return "identity_only", ""
	}
	// With no non-identity label contract, explicit identity-only promotion is
	// observationally identical to automatic promotion. This occurs when a
	// child template must override an inherited allowlist that is inapplicable
	// to all of the child's source occurrences.
	return "automatic", "identity_only"
}

func expectedChartLabels(
	identity []string,
	promoted []string,
	labels map[string]string,
) ([]promreplay.SemanticLabel, error) {
	out := make([]promreplay.SemanticLabel, 0, len(identity)+len(promoted))
	seen := make(map[string]struct{}, len(identity)+len(promoted))
	for _, name := range identity {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		value := labels[name]
		if value == "" {
			return nil, fmt.Errorf("chart label %q is absent or blank", name)
		}
		out = append(out, promreplay.SemanticLabel{Name: name, Value: value})
	}
	for _, name := range promoted {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if value := labels[name]; value != "" {
			out = append(out, promreplay.SemanticLabel{Name: name, Value: value})
		}
	}
	slices.SortFunc(out, compareSemanticLabelValues)
	return out, nil
}

func expectedRelativeContext(namespace, contextID, contextRoot string) string {
	last := contextRoot
	if index := strings.LastIndexByte(last, '.'); index >= 0 {
		last = last[index+1:]
	}
	if last == namespace {
		return contextID
	}
	return namespace + "." + contextID
}

func joinSemanticContext(root, relative string) string {
	if root == "" {
		return relative
	}
	return root + "." + relative
}

func expectedSeriesKind(binding ReconciledSemanticSource, occurrence compiledViewOccurrence) string {
	return expectedSeriesKindForComponent(binding.entry.registration.prometheus, occurrence.component.source)
}

func expectedSeriesKindForComponent(prometheus PrometheusContract, component Component) string {
	switch component.WireRole {
	case "histogram_bucket", "histogram_count", "histogram_sum", "summary_count", "summary_sum":
		return "counter"
	case "summary_quantile":
		return "gauge"
	case "scalar":
		kind := prometheus.Type
		if kind == "untyped" {
			kind = prometheus.Classification
		}
		return kind
	default:
		panic("validated wire role has no replay kind")
	}
}

func compareSemanticRoute(got promreplay.SemanticRoute, want expectedSemanticEdge) string {
	gotChartLabels := slices.Clone(got.ChartLabels)
	slices.Sort(gotChartLabels)
	gotChartLabelValues := slices.Clone(got.ChartLabelValues)
	slices.SortFunc(gotChartLabelValues, compareSemanticLabelValues)
	gotPromotedLabels := slices.Clone(got.PromotedLabels)
	slices.Sort(gotPromotedLabels)
	switch {
	case got.MetricName != want.metricName:
		return fmt.Sprintf("metric got %q, want %q", got.MetricName, want.metricName)
	case got.Context != want.fullContext:
		return fmt.Sprintf("context got %q, want %q", got.Context, want.fullContext)
	case got.DisplayedFamily != want.family:
		return fmt.Sprintf("family got %q, want %q", got.DisplayedFamily, want.family)
	case !slices.Equal(got.IdentityLabels, want.identityLabels):
		return fmt.Sprintf("identity got %v, want %v", got.IdentityLabels, want.identityLabels)
	case !slices.Equal(gotChartLabels, semanticLabelNames(want.chartLabels)):
		return fmt.Sprintf("chart labels got %v, want %v", got.ChartLabels, semanticLabelNames(want.chartLabels))
	case !slices.EqualFunc(gotChartLabelValues, want.chartLabels, equalSemanticLabelValue):
		return fmt.Sprintf("chart label values got %v, want %v", got.ChartLabelValues, want.chartLabels)
	case got.DimensionName != want.dimensionName || got.DimensionKeyLabel != want.dimensionKeyLabel:
		return fmt.Sprintf("dimension got %q key %q, want %q key %q",
			got.DimensionName, got.DimensionKeyLabel, want.dimensionName, want.dimensionKeyLabel)
	case got.PromotionMode != want.promotionMode && got.PromotionMode != want.promotionModeAlias:
		return fmt.Sprintf("promotion mode got %q, want %q", got.PromotionMode, want.promotionMode)
	case !slices.Equal(gotPromotedLabels, want.promotedLabels):
		return fmt.Sprintf("promoted labels got %v, want %v", got.PromotedLabels, want.promotedLabels)
	case got.Algorithm != want.algorithm:
		return fmt.Sprintf("algorithm got %q, want %q", got.Algorithm, want.algorithm)
	case got.SeriesKind != want.seriesKind:
		return fmt.Sprintf("series kind got %q, want %q", got.SeriesKind, want.seriesKind)
	case got.Aggregation != want.aggregation:
		return fmt.Sprintf("aggregation got %q, want %q", got.Aggregation, want.aggregation)
	case got.Units != want.units:
		return fmt.Sprintf("units got %q, want %q", got.Units, want.units)
	case got.Multiplier != want.multiplier || got.Divisor != want.divisor:
		return fmt.Sprintf("scale got %d/%d, want %d/%d",
			got.Multiplier, got.Divisor, want.multiplier, want.divisor)
	case got.Presentation != want.presentation:
		return fmt.Sprintf("presentation got %q, want %q", got.Presentation, want.presentation)
	default:
		return ""
	}
}

func closestSemanticRouteMismatch(
	actual []promreplay.SemanticRoute,
	used []bool,
	want expectedSemanticEdge,
) string {
	for index, got := range actual {
		if !used[index] && got.Profile == want.destinationProfile {
			return compareSemanticRoute(got, want)
		}
	}
	return ""
}

func actualSemanticRouteFingerprint(route promreplay.SemanticRoute) semanticRouteFingerprint {
	chartLabels := slices.Clone(route.ChartLabels)
	slices.Sort(chartLabels)
	chartLabelValues := slices.Clone(route.ChartLabelValues)
	slices.SortFunc(chartLabelValues, compareSemanticLabelValues)
	promotedLabels := slices.Clone(route.PromotedLabels)
	slices.Sort(promotedLabels)
	return semanticRouteFingerprint{
		profile:          route.Profile,
		metricName:       route.MetricName,
		context:          route.Context,
		family:           route.DisplayedFamily,
		identity:         encodeSemanticStrings(route.IdentityLabels),
		chartLabels:      encodeSemanticStrings(chartLabels),
		chartLabelValues: encodeSemanticLabels(chartLabelValues),
		dimensionName:    route.DimensionName,
		dimensionKey:     route.DimensionKeyLabel,
		promotionMode:    route.PromotionMode,
		promotedLabels:   encodeSemanticStrings(promotedLabels),
		algorithm:        route.Algorithm,
		seriesKind:       route.SeriesKind,
		aggregation:      route.Aggregation,
		units:            route.Units,
		multiplier:       route.Multiplier,
		divisor:          route.Divisor,
		presentation:     route.Presentation,
	}
}

func expectedSemanticRouteFingerprint(edge expectedSemanticEdge) semanticRouteFingerprint {
	return semanticRouteFingerprint{
		profile:          edge.destinationProfile,
		metricName:       edge.metricName,
		context:          edge.fullContext,
		family:           edge.family,
		identity:         encodeSemanticStrings(edge.identityLabels),
		chartLabels:      encodeSemanticStrings(semanticLabelNames(edge.chartLabels)),
		chartLabelValues: encodeSemanticLabels(edge.chartLabels),
		dimensionName:    edge.dimensionName,
		dimensionKey:     edge.dimensionKeyLabel,
		promotionMode:    edge.promotionMode,
		promotedLabels:   encodeSemanticStrings(edge.promotedLabels),
		algorithm:        edge.algorithm,
		seriesKind:       edge.seriesKind,
		aggregation:      edge.aggregation,
		units:            edge.units,
		multiplier:       edge.multiplier,
		divisor:          edge.divisor,
		presentation:     edge.presentation,
	}
}

func encodeSemanticStrings(values []string) string {
	var out strings.Builder
	for _, value := range values {
		fmt.Fprintf(&out, "%d:%s", len(value), value)
	}
	return out.String()
}

func encodeSemanticLabels(values []promreplay.SemanticLabel) string {
	var out strings.Builder
	for _, value := range values {
		fmt.Fprintf(&out, "%d:%s%d:%s", len(value.Name), value.Name, len(value.Value), value.Value)
	}
	return out.String()
}

func reconcileSemanticContributorCounts(matches []matchedSemanticEdge) error {
	counts := make(map[string]int)
	observed := make(map[string]int)
	inconsistent := make(map[string]struct{})
	for _, match := range matches {
		key := match.actual.ChartID + "\x00" + match.actual.DimensionName
		counts[key]++
		if previous, ok := observed[key]; ok && previous != match.actual.ContributorCount {
			inconsistent[key] = struct{}{}
		} else {
			observed[key] = match.actual.ContributorCount
		}
	}
	var errs []error
	for _, key := range sortedMapKeys(counts) {
		chartID, dimensionName, _ := strings.Cut(key, "\x00")
		if _, ok := inconsistent[key]; ok {
			errs = append(errs, fmt.Errorf(
				"chart %q dimension %q has inconsistent production contributor counts", chartID, dimensionName))
			continue
		}
		if observed[key] != counts[key] {
			errs = append(errs, fmt.Errorf(
				"chart %q dimension %q contributor count got %d, want %d from semantic edges",
				chartID, dimensionName, observed[key], counts[key],
			))
		}
	}
	return errors.Join(errs...)
}

func semanticLabelNames(labels []promreplay.SemanticLabel) []string {
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		out = append(out, label.Name)
	}
	return out
}

func compareSemanticLabelValues(left, right promreplay.SemanticLabel) int {
	if result := strings.Compare(left.Name, right.Name); result != 0 {
		return result
	}
	return strings.Compare(left.Value, right.Value)
}

func equalSemanticLabelValue(left, right promreplay.SemanticLabel) bool {
	return left.Name == right.Name && left.Value == right.Value
}

func formatSemanticRouteMismatch(mismatch string) string {
	if mismatch == "" {
		return ""
	}
	return "; closest route differs: " + mismatch
}

func compareReconciledSemanticEdges(left, right ReconciledSemanticEdge) int {
	if left.SourceIndex != right.SourceIndex {
		return left.SourceIndex - right.SourceIndex
	}
	for _, pair := range [][2]string{
		{left.DestinationProfile, right.DestinationProfile},
		{left.Context, right.Context},
		{left.View, right.View},
		{left.Input, right.Input},
		{left.RenderedRole, right.RenderedRole},
		{left.TemplatePath, right.TemplatePath},
		{left.ChartID, right.ChartID},
		{left.DimensionName, right.DimensionName},
		{left.OccurrenceID, right.OccurrenceID},
	} {
		if result := strings.Compare(pair[0], pair[1]); result != 0 {
			return result
		}
	}
	if left.DimensionIndex != right.DimensionIndex {
		return left.DimensionIndex - right.DimensionIndex
	}
	return 0
}
