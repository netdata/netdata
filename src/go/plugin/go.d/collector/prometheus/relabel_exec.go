// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

type relabelStage struct {
	name            string
	limiterKey      string
	diagnosticStage PipelineRelabelStage
}

var (
	jobRelabelStage = relabelStage{
		name: "job relabeling", limiterKey: "job-relabel-typed-family-corruption", diagnosticStage: PipelineRelabelStageJob,
	}
	profileRelabelStage = relabelStage{
		name: "profile relabeling", limiterKey: "profile-relabel-typed-family-corruption", diagnosticStage: PipelineRelabelStageProfile,
	}
)

type relabelResult struct {
	raw    prompkg.Sample
	sample prompkg.Sample
	drop   relabel.DropInfo
}

// relabelAndAssemble runs the job-level relabel pipeline: relabel every sample,
// assemble, then curate typed families so relabeling cannot silently corrupt a
// histogram/summary. A histogram/summary is a set of physical samples
// (_bucket/_sum/_count, or quantile/_sum/_count) folded by base name + base
// labels; the assembler carries no provenance, so a rename or drop that splits
// or merges those components is invisible after assembly (e.g. renaming only
// `_sum` leaves the base family assembling with a fabricated sum=0). The fix is
// to track each raw family's fate across relabeling and reject the result.
//
// checking is true under Check (autodetection): any corruption is a hard error
// so a broken rule fails fast. Under Collect it is false: corrupted families are
// dropped and the rest reassembled once, with a warning, so a transient
// exposition change cannot take the whole job down.
func (c *Collector) relabelAndAssemble(
	batch prompkg.SampleBatch,
	pipeline *relabel.Pipeline,
	stage relabelStage,
	checking bool,
) (prompkg.SampleBatch, prompkg.MetricFamilies, error) {
	processed, tracking := c.applyPipelineRelabel(batch, pipeline, stage)
	return c.finishRelabel(processed, tracking, stage, checking, true)
}

// relabelAndValidateBatch runs relabeling and typed-family safety when the caller
// needs the resulting sample batch but not an intermediate family assembly.
func (c *Collector) relabelAndValidateBatch(
	batch prompkg.SampleBatch,
	pipeline *relabel.Pipeline,
	stage relabelStage,
	checking bool,
) (prompkg.SampleBatch, error) {
	processed, tracking := c.applyPipelineRelabel(batch, pipeline, stage)
	processed, _, err := c.finishRelabel(processed, tracking, stage, checking, false)
	return processed, err
}

func (c *Collector) finishRelabel(
	processed prompkg.SampleBatch,
	tracking *relabelTracking,
	stage relabelStage,
	checking bool,
	needFamilies bool,
) (prompkg.SampleBatch, prompkg.MetricFamilies, error) {
	if !tracking.anyTypedTouched && !needFamilies {
		return processed, nil, nil
	}
	mfs, err := prompkg.Assemble(processed)
	if err != nil {
		return prompkg.SampleBatch{}, nil, err
	}

	// No typed family was altered (or dropped) by relabeling: nothing to curate.
	if !tracking.anyTypedTouched {
		return processed, mfs, nil
	}

	invalid, violations := validateTypedFamilies(tracking, mfs)
	if len(invalid) == 0 {
		if !needFamilies {
			mfs = nil
		}
		return processed, mfs, nil
	}
	if c.pipelineObserver != nil {
		rejected := make(map[prompkg.SampleSeriesIdentity]struct{}, len(invalid))
		for _, sample := range processed.Samples {
			key, typed := typedFamilyKeyOf(sample)
			if !typed {
				continue
			}
			if _, bad := invalid[key]; !bad {
				continue
			}
			identity := prompkg.IdentifySampleSeries(sample)
			if _, seen := rejected[identity]; seen {
				continue
			}
			rejected[identity] = struct{}{}
			c.observePipeline(PipelineDiagnostic{
				Decision:     PipelineTypedFamilyRejected,
				Reason:       PipelineReasonTypedFamilyCorruption,
				Destination:  identity,
				MetricName:   identity.Family,
				RelabelStage: stage.diagnosticStage,
			})
		}
	}

	if checking {
		return prompkg.SampleBatch{}, nil, fmt.Errorf("%s corrupts typed metric families: %s", stage.name, violations[0])
	}

	// Runtime: drop the corrupted families and reassemble. This can recur every scrape
	// if a rule stays bad or the exporter changed, so rate-limit the warning to avoid
	// flooding the log; the per-family detail stays at debug.
	c.Limit(stage.limiterKey, 1, 10*time.Minute).
		Warningf("%s produced %d typed-family corruption(s); dropped the affected families (enable debug for names)", stage.name, len(violations))
	for _, v := range violations {
		c.Debugf("%s dropped corrupted typed family: %s", stage.name, v)
	}

	// Closed invalid set computed in one pass, so a single re-filter + reassembly
	// terminates: dropping samples can only remove corruption, never add it.
	filtered := filterInvalidTypedFamilies(processed, invalid)
	if !needFamilies {
		return filtered, nil, nil
	}
	mfs, err = prompkg.Assemble(filtered)
	return filtered, mfs, err
}

func (c *Collector) applyPipelineRelabel(
	batch prompkg.SampleBatch,
	pipeline *relabel.Pipeline,
	stage relabelStage,
) (prompkg.SampleBatch, *relabelTracking) {
	t := newRelabelTracking()
	help := newHelpRemap()
	out := prompkg.SampleBatch{Samples: make([]prompkg.Sample, 0, len(batch.Samples))}
	for _, raw := range batch.Samples {
		sample, drop := c.applyObservedPipeline(raw, pipeline, stage, "", true)
		c.appendRelabelResult(&out, t, help, stage, relabelResult{raw: raw, sample: sample, drop: drop})
	}
	out.Help = help.remap(batch.Help)
	return out, t
}

func (c *Collector) applyObservedPipeline(
	raw prompkg.Sample,
	pipeline *relabel.Pipeline,
	stage relabelStage,
	profileName string,
	observeRaw bool,
) (prompkg.Sample, relabel.DropInfo) {
	if c.pipelineObserver == nil {
		if pipeline == nil {
			return raw, relabel.DropInfo{}
		}
		return pipeline.Apply(raw)
	}

	source := prompkg.IdentifySampleSeries(raw)
	rawIdentity := prompkg.IdentifyRawSample(raw.Name, raw.Labels)
	valueIdentity := PipelineValueIdentity{}
	scalarValue := raw.Kind == prompkg.SampleKindScalar
	if scalarValue {
		valueIdentity = pipelineScalarValueIdentity(raw.Value)
	}
	if observeRaw {
		c.observePipeline(PipelineDiagnostic{
			Decision:      PipelineRawAccepted,
			RawIdentity:   rawIdentity,
			Source:        source,
			ValueIdentity: valueIdentity,
			ScalarValue:   scalarValue,
			MetricName:    raw.Name,
		})
	}

	sample := raw
	drop := relabel.DropInfo{}
	dropMetricName := sample.Name
	dropBlockIndex := -1
	if pipeline != nil {
		sample, drop = pipeline.ApplyWithObserver(
			raw,
			func(fact relabel.BlockDiagnostic) {
				dropBlockIndex = fact.BlockIndex
				c.observePipeline(PipelineDiagnostic{
					Decision:        PipelineRelabelBlockEntered,
					RawIdentity:     rawIdentity,
					Source:          source,
					ValueIdentity:   valueIdentity,
					ScalarValue:     scalarValue,
					InputMetricName: fact.InputMetricName,
					InputLabelNames: fact.InputLabelNames,
					BlockIndex:      fact.BlockIndex,
					RelabelStage:    stage.diagnosticStage,
					ProfileName:     profileName,
				})
			},
			func(blockIndex int, fact relabel.RuleDiagnostic) {
				dropMetricName = fact.OutputMetricName
				c.observePipeline(PipelineDiagnostic{
					Decision:           PipelineRelabelRuleEvaluated,
					RawIdentity:        rawIdentity,
					Source:             source,
					ValueIdentity:      valueIdentity,
					ScalarValue:        scalarValue,
					InputMetricName:    fact.InputMetricName,
					OutputMetricName:   fact.OutputMetricName,
					InputLabels:        pipelineLabels(fact.InputLabels),
					OutputLabels:       pipelineLabels(fact.OutputLabels),
					BlockIndex:         blockIndex,
					RuleIndex:          fact.RuleIndex,
					RelabelAction:      fact.Action,
					RelabelRuleMatched: fact.Matched,
					RelabelRuleDropped: fact.Dropped,
					RelabelStage:       stage.diagnosticStage,
					ProfileName:        profileName,
				})
			},
		)
	}
	if drop.Dropped() {
		c.observePipeline(PipelineDiagnostic{
			Decision:      PipelineRelabelDropped,
			RawIdentity:   rawIdentity,
			Source:        source,
			ValueIdentity: valueIdentity,
			ScalarValue:   scalarValue,
			MetricName:    dropMetricName,
			BlockIndex:    dropBlockIndex,
			RuleIndex:     drop.RuleIndex,
			RelabelAction: drop.Action,
			RelabelDrop:   drop,
			RelabelStage:  stage.diagnosticStage,
			ProfileName:   profileName,
		})
		return sample, drop
	}

	fact := PipelineDiagnostic{
		Decision:               PipelineRelabelOutput,
		RawIdentity:            rawIdentity,
		DestinationRawIdentity: prompkg.IdentifyRawSample(sample.Name, sample.Labels),
		Source:                 source,
		Destination:            prompkg.IdentifySampleSeries(sample),
		MetricName:             sample.Name,
		OutputLabels:           pipelineLabels(sample.Labels),
		RelabelStage:           stage.diagnosticStage,
		ProfileName:            profileName,
	}
	fact.ValueIdentity = valueIdentity
	fact.ScalarValue = scalarValue
	c.observePipeline(fact)
	return sample, relabel.DropInfo{}
}

func (c *Collector) appendRelabelResult(
	out *prompkg.SampleBatch,
	t *relabelTracking,
	help *helpRemap,
	stage relabelStage,
	result relabelResult,
) {
	raw := result.raw
	sample := result.sample
	rawKey, isTyped := typedFamilyKeyOf(raw)

	if result.drop.Dropped() {
		// Log the sample as it stood when the drop happened — an earlier block may
		// have renamed it. (rawKey stays keyed on the original for family tracking.)
		c.onRelabelDrop(stage, sample, result.drop)
		if isTyped {
			t.recordDropped(rawKey)
		}
		return
	}

	touched := sample.Name != raw.Name || !labels.Equal(sample.Labels, raw.Labels)
	if isTyped {
		finalKey, _ := typedFamilyKeyOf(sample) // Kind is preserved by relabeling, so still typed
		t.recordKept(rawKey, finalKey, raw, sample, touched)
	}

	out.Samples = append(out.Samples, sample)
	help.add(helpFamilyName(raw), helpFamilyName(sample))
}

// onRelabelDrop logs why a relabel rule dropped a sample, at debug level. It logs
// the metric name and the rule outcome, never label values (cardinality/PII).
func (c *Collector) onRelabelDrop(stage relabelStage, s prompkg.Sample, d relabel.DropInfo) {
	c.When(d.RuleIndex >= 0).
		Debugf("%s dropped metric %q: %s (rule %d, action %q)", stage.name, s.Name, d.Reason, d.RuleIndex, d.Action).
		Else().
		Debugf("%s dropped metric %q: %s", stage.name, s.Name, d.Reason)
}

// typedFamilyKey identifies one logical histogram/summary instance: the base
// family name (suffix trimmed) plus the hash of its base labels (the structural
// le/quantile stripped). It matches how the assembler groups components, so a key
// computed here resolves to the same assembled family.
type typedFamilyKey struct {
	name string
	hash uint64
}

// relabelTracking accumulates, per scrape, the fate of every typed-family
// component across relabeling so the validator can detect corruption.
type relabelTracking struct {
	anyTypedTouched bool // a typed component changed or was dropped — gates validation
	raw             map[typedFamilyKey]*rawFamilyState
	final           map[typedFamilyKey]*finalFamilyState
}

// rawFamilyState is the fate of one source typed family.
type rawFamilyState struct {
	touched   bool
	rawCount  int // components seen (kept or dropped)
	keptCount int // components that survived relabeling
	mutated   bool
	finalKeys map[typedFamilyKey]struct{} // distinct final keys the kept components landed on
}

// finalFamilyState is what relabeling produced under one final key, used to
// detect families merged from multiple sources or carrying duplicate components.
type finalFamilyState struct {
	touched    bool                        // a relabel-touched component contributed
	rawKeys    map[typedFamilyKey]struct{} // source families that landed here (>1 == merge)
	sumCount   int
	countCount int
	buckets    map[string]int // le value -> count (>1 == duplicate bucket)
	quantiles  map[string]int // quantile value -> count (>1 == duplicate quantile)
}

func newRelabelTracking() *relabelTracking {
	return &relabelTracking{
		raw:   make(map[typedFamilyKey]*rawFamilyState),
		final: make(map[typedFamilyKey]*finalFamilyState),
	}
}

func (t *relabelTracking) rawState(k typedFamilyKey) *rawFamilyState {
	rs := t.raw[k]
	if rs == nil {
		rs = &rawFamilyState{finalKeys: make(map[typedFamilyKey]struct{})}
		t.raw[k] = rs
	}
	return rs
}

func (t *relabelTracking) finalState(k typedFamilyKey) *finalFamilyState {
	fs := t.final[k]
	if fs == nil {
		fs = &finalFamilyState{rawKeys: make(map[typedFamilyKey]struct{})}
		t.final[k] = fs
	}
	return fs
}

// recordDropped marks a source family as altered when one of its components is
// dropped (a partial drop is corruption; a full drop is clean).
func (t *relabelTracking) recordDropped(rawKey typedFamilyKey) {
	rs := t.rawState(rawKey)
	rs.rawCount++
	rs.touched = true
	t.anyTypedTouched = true
}

func (t *relabelTracking) recordKept(rawKey, finalKey typedFamilyKey, raw, sample prompkg.Sample, touched bool) {
	rs := t.rawState(rawKey)
	rs.rawCount++
	rs.keptCount++
	rs.finalKeys[finalKey] = struct{}{}
	if touched {
		rs.touched = true
		t.anyTypedTouched = true
	}
	if structuralMutated(raw, sample) {
		rs.mutated = true
	}

	fs := t.finalState(finalKey)
	fs.rawKeys[rawKey] = struct{}{}
	if touched {
		fs.touched = true
	}
	switch sample.Kind {
	case prompkg.SampleKindHistogramSum, prompkg.SampleKindSummarySum:
		fs.sumCount++
	case prompkg.SampleKindHistogramCount, prompkg.SampleKindSummaryCount:
		fs.countCount++
	case prompkg.SampleKindHistogramBucket:
		if fs.buckets == nil {
			fs.buckets = make(map[string]int)
		}
		fs.buckets[sample.Labels.Get(prompkg.SampleStructuralLabelName(sample.Kind))]++
	case prompkg.SampleKindSummaryQuantile:
		if fs.quantiles == nil {
			fs.quantiles = make(map[string]int)
		}
		fs.quantiles[sample.Labels.Get(prompkg.SampleStructuralLabelName(sample.Kind))]++
	}
}

// validateTypedFamilies returns the set of final keys whose samples must be
// dropped and a deterministically ordered list of human-readable reasons. A
// touched source family is corrupt when relabeling splits it across final keys,
// drops only some components, or mutates a le/quantile; a final family is corrupt
// when it is merged from multiple sources or carries duplicate components. As a
// backstop, a touched family whose single final key did not assemble into a valid
// typed family is dropped too.
func validateTypedFamilies(t *relabelTracking, mfs prompkg.MetricFamilies) (map[typedFamilyKey]struct{}, []string) {
	invalid := make(map[typedFamilyKey]struct{})
	assembled := assembledTypedFamilyKeys(mfs)

	type violation struct {
		key typedFamilyKey
		msg string
	}
	var violations []violation

	for rawKey, rs := range t.raw {
		if !rs.touched {
			continue
		}
		switch {
		case len(rs.finalKeys) > 1:
			markInvalid(invalid, rs.finalKeys)
			violations = append(violations, violation{rawKey, fmt.Sprintf("typed family %q is split across multiple series by relabeling", rawKey.name)})
		case rs.keptCount > 0 && rs.keptCount < rs.rawCount:
			markInvalid(invalid, rs.finalKeys)
			violations = append(violations, violation{rawKey, fmt.Sprintf("typed family %q has components dropped by relabeling", rawKey.name)})
		case rs.mutated:
			markInvalid(invalid, rs.finalKeys)
			violations = append(violations, violation{rawKey, fmt.Sprintf("typed family %q has its le/quantile label mutated by relabeling", rawKey.name)})
		case rs.keptCount > 0:
			for fk := range rs.finalKeys { // exactly one
				if _, ok := assembled[fk]; !ok {
					invalid[fk] = struct{}{}
					violations = append(violations, violation{rawKey, fmt.Sprintf("typed family %q does not assemble into a valid family after relabeling", rawKey.name)})
				}
			}
		}
	}

	for finalKey, fs := range t.final {
		if !fs.touched {
			continue
		}
		switch {
		case len(fs.rawKeys) > 1:
			invalid[finalKey] = struct{}{}
			violations = append(violations, violation{finalKey, fmt.Sprintf("typed family %q is merged from multiple source families by relabeling", finalKey.name)})
		case fs.sumCount > 1 || fs.countCount > 1 || hasDuplicate(fs.buckets) || hasDuplicate(fs.quantiles):
			invalid[finalKey] = struct{}{}
			violations = append(violations, violation{finalKey, fmt.Sprintf("typed family %q has duplicate components after relabeling", finalKey.name)})
		}
	}

	slices.SortFunc(violations, func(a, b violation) int {
		if d := cmp.Compare(a.key.name, b.key.name); d != 0 {
			return d
		}
		return cmp.Compare(a.key.hash, b.key.hash)
	})
	msgs := make([]string, len(violations))
	for i, v := range violations {
		msgs[i] = v.msg
	}
	return invalid, msgs
}

// filterInvalidTypedFamilies drops every processed sample whose final typed
// family is invalid, returning a batch to reassemble. HELP is carried through
// unchanged: a HELP entry left pointing at a dropped family is harmless because
// Assemble prunes families with no metrics.
func filterInvalidTypedFamilies(batch prompkg.SampleBatch, invalid map[typedFamilyKey]struct{}) prompkg.SampleBatch {
	out := prompkg.SampleBatch{
		Help:    batch.Help,
		Samples: make([]prompkg.Sample, 0, len(batch.Samples)),
	}
	for _, s := range batch.Samples {
		if key, ok := typedFamilyKeyOf(s); ok {
			if _, bad := invalid[key]; bad {
				continue
			}
		}
		out.Samples = append(out.Samples, s)
	}
	return out
}

// assembledTypedFamilyKeys is the set of typed-family keys that actually
// assembled into a histogram/summary family. The assembled metric's labels are
// the base labels (le/quantile already stripped), so its hash matches a
// typedFamilyKey computed from the source samples.
func assembledTypedFamilyKeys(mfs prompkg.MetricFamilies) map[typedFamilyKey]struct{} {
	keys := make(map[typedFamilyKey]struct{})
	for _, mf := range mfs {
		if mf == nil {
			continue
		}
		if mf.Type() != commonmodel.MetricTypeSummary && mf.Type() != commonmodel.MetricTypeHistogram {
			continue
		}
		for _, m := range mf.Metrics() {
			keys[typedFamilyKey{name: mf.Name(), hash: m.Labels().Hash()}] = struct{}{}
		}
	}
	return keys
}

func markInvalid(invalid map[typedFamilyKey]struct{}, keys map[typedFamilyKey]struct{}) {
	for k := range keys {
		invalid[k] = struct{}{}
	}
}

func hasDuplicate(counts map[string]int) bool {
	for _, n := range counts {
		if n > 1 {
			return true
		}
	}
	return false
}

// typedFamilyKeyOf returns the typed-family key of a sample and whether it is part
// of a typed family at all. It mirrors the assembler's grouping: structural
// le/quantile labels are stripped from the hash, the suffix is trimmed from the
// name; a plain gauge/counter is not typed.
func typedFamilyKeyOf(s prompkg.Sample) (typedFamilyKey, bool) {
	switch s.Kind {
	case prompkg.SampleKindHistogramBucket,
		prompkg.SampleKindSummaryQuantile,
		prompkg.SampleKindHistogramSum, prompkg.SampleKindHistogramCount,
		prompkg.SampleKindSummarySum, prompkg.SampleKindSummaryCount:
		return typedFamilyKey{name: prompkg.SampleFamilyName(s), hash: prompkg.SampleSeriesHash(s)}, true
	default:
		// A summary/histogram base series with neither structural label nor a
		// _sum/_count suffix (e.g. an empty summary) is still a typed component.
		if s.FamilyType != commonmodel.MetricTypeSummary && s.FamilyType != commonmodel.MetricTypeHistogram {
			return typedFamilyKey{}, false
		}
		return typedFamilyKey{name: s.Name, hash: s.Labels.Hash()}, true
	}
}

// helpFamilyName is the family name a sample's HELP belongs to (the base name,
// suffix trimmed) — the key Prometheus uses for # HELP.
func helpFamilyName(s prompkg.Sample) string {
	return prompkg.SampleFamilyName(s)
}

// structuralMutated reports whether relabeling changed (or removed) the le or
// quantile label that defines a bucket boundary or quantile point. Kind is
// preserved by relabeling, so the raw kind selects the structural label.
func structuralMutated(raw, sample prompkg.Sample) bool {
	labelName := prompkg.SampleStructuralLabelName(raw.Kind)
	if labelName == "" {
		return false
	}
	return raw.Labels.Get(labelName) != sample.Labels.Get(labelName)
}

// helpRemap maps each source family name to the final family name(s) its samples
// were relabeled into, so HELP text follows a rename. A family may map to more
// than one target (a split), which the validator then rejects.
type helpRemap struct {
	targets map[string][]string
	seen    map[string]map[string]struct{}
}

func newHelpRemap() *helpRemap {
	return &helpRemap{
		targets: make(map[string][]string),
		seen:    make(map[string]map[string]struct{}),
	}
}

func (h *helpRemap) add(source, target string) {
	if source == "" || target == "" {
		return
	}
	set, ok := h.seen[source]
	if !ok {
		set = make(map[string]struct{})
		h.seen[source] = set
	}
	if _, ok := set[target]; ok {
		return
	}
	set[target] = struct{}{}
	h.targets[source] = append(h.targets[source], target)
}

// remap rewrites HELP entries onto their final family names. An entry whose
// family was fully dropped maps to nothing and is omitted.
func (h *helpRemap) remap(in []prompkg.HelpEntry) []prompkg.HelpEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]prompkg.HelpEntry, 0, len(in))
	for _, e := range in {
		for _, target := range h.targets[e.Name] {
			out = append(out, prompkg.HelpEntry{Name: target, Help: e.Help})
		}
	}
	return out
}
