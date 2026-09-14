// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

// ReconciledSemanticSource binds one production raw occurrence to its one
// active source-semantic owner. Production paths and chart IDs are deliberately
// not semantic identity.
type ReconciledSemanticSource struct {
	SourceIndex  int
	Profile      string
	Signal       string
	Component    string
	Registration string

	program    *CompiledSemanticContract
	occurrence *compiledOccurrence
	entry      compiledSourceEntry
}

// ReconciledSemanticNormalization records one semantic normalization branch
// exercised by production replay. Runtime paths are production evidence, not
// authored semantic identity.
type ReconciledSemanticNormalization struct {
	SourceIndex   int
	Profile       string
	Normalization string
	Branch        string
	RuntimePaths  []string
	Terminal      bool
}

// ReconciledSemanticEdge binds one source-semantic occurrence to one
// operator-facing destination. Runtime IDs are retained as production facts,
// but do not participate in the semantic edge identity.
type ReconciledSemanticEdge struct {
	SourceIndex        int
	SourceProfile      string
	Signal             string
	Component          string
	OccurrenceID       string
	DestinationProfile string
	Context            string
	View               string
	Input              string
	RenderedRole       string
	TemplatePath       string
	ChartID            string
	DimensionIndex     int
	DimensionName      string
}

// ReconciledSemanticExclusion records one active design exclusion and its
// production disposition for coverage and later proof observations.
type ReconciledSemanticExclusion struct {
	SourceIndex      int
	Profile          string
	Exclusion        string
	Outcome          string
	RuntimePath      string
	AutogenFamily    string
	SuppressedSeries int
}

// ReconciledSemanticClaim records a bounded relationship or state-encoding
// witness validated from production source values.
type ReconciledSemanticClaim struct {
	Profile string
	Kind    string
	ID      string
}

// ReconciledSemanticCase is the bounded per-snapshot source join consumed by
// later edge, normalization, and observation reconciliation.
type ReconciledSemanticCase struct {
	Sources        []ReconciledSemanticSource
	Normalizations []ReconciledSemanticNormalization
	Edges          []ReconciledSemanticEdge
	Exclusions     []ReconciledSemanticExclusion
	Claims         []ReconciledSemanticClaim
}

type sourceReplayCandidate struct {
	program *CompiledSemanticContract
	entry   compiledSourceEntry
	exact   bool
}

// ReconcileProductionSources validates the selected production profile set and
// assigns every raw occurrence to exactly one active semantic source owner.
func (c *CompiledSemanticCase) ReconcileProductionSources(
	ctx context.Context,
	snapshot *promreplay.SemanticSnapshot,
) (*ReconciledSemanticCase, error) {
	if err := checkSemanticContext(ctx, "before production source reconciliation"); err != nil {
		return nil, err
	}
	if c == nil || c.root == nil {
		return nil, fmt.Errorf("production source reconciliation: compiled semantic case is nil")
	}
	if snapshot == nil {
		return nil, fmt.Errorf("production source reconciliation: semantic snapshot is nil")
	}
	if err := c.reconcileProductionHeader(snapshot); err != nil {
		return nil, fmt.Errorf("production source reconciliation: %w", err)
	}

	result := &ReconciledSemanticCase{Sources: make([]ReconciledSemanticSource, 0, len(snapshot.Sources))}
	var errs []error
	for index, source := range snapshot.Sources {
		if err := checkSemanticContext(ctx, "during production source reconciliation"); err != nil {
			return nil, err
		}
		candidate, err := c.matchProductionSource(source)
		if err != nil {
			errs = append(errs, fmt.Errorf("raw occurrence %s (%s/%s): %w",
				source.OccurrenceID, source.MetricName, source.Component, err))
			continue
		}
		if candidate.entry.owner.kind == "source_exclusion" {
			errs = append(errs, fmt.Errorf(
				"raw occurrence %s (%s/%s) emitted from source-excluded registration %q (%s)",
				source.OccurrenceID, source.MetricName, source.Component,
				candidate.entry.registration.key, candidate.entry.owner.id,
			))
			continue
		}
		if err := c.validateProductionSourceLabels(candidate, source); err != nil {
			errs = append(errs, fmt.Errorf("raw occurrence %s (%s/%s): %w",
				source.OccurrenceID, source.MetricName, source.Component, err))
			continue
		}
		result.Sources = append(result.Sources, ReconciledSemanticSource{
			SourceIndex:  index,
			Profile:      candidate.program.profile,
			Signal:       candidate.entry.occurrence.signal,
			Component:    candidate.entry.occurrence.component,
			Registration: candidate.entry.registration.key,
			program:      candidate.program,
			occurrence:   candidate.entry.occurrence,
			entry:        candidate.entry,
		})
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *CompiledSemanticCase) reconcileProductionHeader(snapshot *promreplay.SemanticSnapshot) error {
	got := slices.Clone(snapshot.SelectedProfiles)
	slices.Sort(got)
	if want := c.ActiveProfiles(); !slices.Equal(got, want) {
		return fmt.Errorf("selected profiles %v differ from active semantic profiles %v", got, want)
	}
	if snapshot.Job.HasSelector || snapshot.Job.HasRelabeling || snapshot.Job.HasFallbackType ||
		snapshot.Job.HasApp || snapshot.Job.HasProfiles {
		return fmt.Errorf(
			"stock proof job contains profile-owned policy selector=%t relabeling=%t fallback_type=%t app=%t profiles=%t",
			snapshot.Job.HasSelector,
			snapshot.Job.HasRelabeling,
			snapshot.Job.HasFallbackType,
			snapshot.Job.HasApp,
			snapshot.Job.HasProfiles,
		)
	}
	profiles := make(map[string]promreplay.SemanticProfile, len(snapshot.Profiles))
	for _, profile := range snapshot.Profiles {
		if _, exists := profiles[profile.Name]; exists {
			return fmt.Errorf("production profile facts duplicate %q", profile.Name)
		}
		profiles[profile.Name] = profile
	}
	for _, name := range c.ActiveProfiles() {
		profile, ok := profiles[name]
		if !ok {
			return fmt.Errorf("active profile %q has no production profile facts", name)
		}
		if err := c.programs[name].ValidateProductionProfileHeader(profile); err != nil {
			return err
		}
	}
	return nil
}

func (c *CompiledSemanticCase) matchProductionSource(
	source promreplay.SemanticSource,
) (sourceReplayCandidate, error) {
	family, ok := replaySourceFamily(source.MetricName, source.Component)
	if !ok {
		return sourceReplayCandidate{}, fmt.Errorf("cannot derive source family")
	}
	var candidates []sourceReplayCandidate
	for _, name := range c.ActiveProfiles() {
		program := c.programs[name]
		key := sourceFamilyComponentKey{family: family, component: source.Component}
		for _, entry := range program.sourceIndex.exact[key] {
			if c.sourceEntryActive(program, entry) && entry.registration.prometheus.Type == source.PrometheusType {
				candidates = append(candidates, sourceReplayCandidate{program: program, entry: entry, exact: true})
			}
		}
		for _, length := range program.sourceIndex.prefixLengths {
			if len(family) <= length {
				continue
			}
			prefix := family[:length]
			for _, entry := range program.sourceIndex.embeddedByPrefix[prefix] {
				if entry.wireRole != source.Component ||
					!embeddedFamilyMatches(family, *entry.embedded) ||
					!c.sourceEntryActive(program, entry) ||
					entry.registration.prometheus.Type != source.PrometheusType {
					continue
				}
				candidates = append(candidates, sourceReplayCandidate{program: program, entry: entry})
			}
		}
	}
	candidates = selectSourceCandidates(candidates)
	if len(candidates) == 0 {
		return sourceReplayCandidate{}, fmt.Errorf(
			"family %q type %q has no active semantic registration for component %q",
			family, source.PrometheusType, source.Component,
		)
	}
	if len(candidates) != 1 {
		owners := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			owners = append(owners, candidate.program.profile+"/"+candidate.entry.registration.key+"/"+candidate.entry.owner.id)
		}
		slices.Sort(owners)
		return sourceReplayCandidate{}, fmt.Errorf("family %q has ambiguous active semantic owners %v", family, owners)
	}
	return candidates[0], nil
}

func (c *CompiledSemanticCase) sourceEntryActive(
	program *CompiledSemanticContract,
	entry compiledSourceEntry,
) bool {
	assignment, ok := c.assignments[program.profile]
	return ok && entry.availability.evaluate(program.environment.axes, assignment)
}

func selectSourceCandidates(candidates []sourceReplayCandidate) []sourceReplayCandidate {
	hasExact := slices.ContainsFunc(candidates, func(candidate sourceReplayCandidate) bool {
		return candidate.exact
	})
	if hasExact {
		filtered := candidates[:0]
		for _, candidate := range candidates {
			if candidate.exact {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}

	best := make(map[string]int)
	for _, candidate := range candidates {
		grammar := candidate.entry.grammar
		if grammar == "" || candidate.entry.interpretation != "longest_known_suffix" {
			continue
		}
		rank := 0
		if candidate.exact {
			rank = int(^uint(0) >> 1)
		} else if candidate.entry.embedded != nil {
			rank = len(candidate.entry.embedded.Separator + candidate.entry.embedded.Suffix)
		}
		key := candidate.program.profile + "\x00" + grammar
		best[key] = max(best[key], rank)
	}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		grammar := candidate.entry.grammar
		if grammar != "" && candidate.entry.interpretation == "longest_known_suffix" {
			rank := 0
			if candidate.exact {
				rank = int(^uint(0) >> 1)
			} else if candidate.entry.embedded != nil {
				rank = len(candidate.entry.embedded.Separator + candidate.entry.embedded.Suffix)
			}
			if rank != best[candidate.program.profile+"\x00"+grammar] {
				continue
			}
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func embeddedFamilyMatches(family string, embedded GrammarEmbedded) bool {
	if !embeddedFamilyNamespaceMatches(family, embedded) {
		return false
	}
	tail := embedded.Separator + embedded.Suffix
	if !strings.HasSuffix(family, tail) {
		return false
	}
	return len(family) > len(embedded.Prefix)+len(tail)
}

func embeddedFamilyNamespaceMatches(family string, embedded GrammarEmbedded) bool {
	return strings.HasPrefix(family, embedded.Prefix) && !embeddedPrefixExcluded(embedded, family)
}

func replaySourceFamily(metric, component string) (string, bool) {
	suffix := ""
	switch component {
	case "scalar", "summary_quantile":
		return metric, metric != ""
	case "histogram_bucket":
		suffix = "_bucket"
	case "histogram_sum", "summary_sum":
		suffix = "_sum"
	case "histogram_count", "summary_count":
		suffix = "_count"
	default:
		return "", false
	}
	if !strings.HasSuffix(metric, suffix) || len(metric) == len(suffix) {
		return "", false
	}
	return strings.TrimSuffix(metric, suffix), true
}

func (c *CompiledSemanticCase) validateProductionSourceLabels(
	candidate sourceReplayCandidate,
	source promreplay.SemanticSource,
) error {
	occurrence := candidate.entry.occurrence
	labels := semanticLabelMap(source.Labels)
	observed := maps.Clone(labels)
	structural := replayStructuralLabel(source.Component)
	if structural != "" {
		value, ok := observed[structural]
		if !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("structural label %q is missing or blank", structural)
		}
		delete(observed, structural)
	}
	assignment := c.assignments[candidate.program.profile]
	for _, name := range slices.Sorted(maps.Keys(occurrence.sourceLabels)) {
		schema := occurrence.sourceLabels[name]
		value, present := observed[name]
		requiredNonblank := schema.Presence.Kind == "required"
		requiredPresent := schema.Presence.keyIsAlwaysPresent()
		forbidden := false
		if schema.Presence.Kind == "" {
			condition, err := candidate.program.environment.resolve(schema.Presence.When)
			if err != nil {
				return fmt.Errorf("label %q presence: %w", name, err)
			}
			requiredNonblank = condition.evaluate(candidate.program.environment.axes, assignment)
			requiredPresent = requiredNonblank
			forbidden = !requiredNonblank
		}
		if requiredNonblank && (!present || strings.TrimSpace(value) == "") {
			return fmt.Errorf("required label %q is missing or blank", name)
		}
		if requiredPresent && !present {
			return fmt.Errorf("present label %q is missing", name)
		}
		if forbidden && present {
			return fmt.Errorf("conditionally absent label %q is present", name)
		}
		if present && schema.Domain.Kind == "closed" && !slices.Contains(schema.Domain.Values, value) {
			return fmt.Errorf("label %q value %q is outside closed domain %v", name, value, schema.Domain.Values)
		}
		delete(observed, name)
	}
	if len(observed) != 0 {
		return fmt.Errorf("source labels %v are not declared by signal %q", slices.Sorted(maps.Keys(observed)), occurrence.signal)
	}
	signal := candidate.program.signals[occurrence.signal]
	for _, id := range sortedMapKeys(signal.labelPresenceConstraints) {
		constraint := signal.labelPresenceConstraints[id]
		if _, err := selectedLabelAlternative(constraint.Alternatives, labels); err != nil {
			return fmt.Errorf("label presence constraint %q: %w", id, err)
		}
	}
	return nil
}

func replayStructuralLabel(component string) string {
	switch component {
	case "histogram_bucket":
		return "le"
	case "summary_quantile":
		return "quantile"
	default:
		return ""
	}
}
