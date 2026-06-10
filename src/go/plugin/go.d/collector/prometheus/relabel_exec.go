// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

// relabelBlock is a compiled relabeling block: a metric-name matcher (required,
// never nil) and the relabel processor for its rules.
type relabelBlock struct {
	match matcher.Matcher
	proc  *relabel.Processor
}

// applyBlocks runs each block whose match matches the sample's current name, in
// order, threading the sample through. It returns the relabeled sample, or the
// sample as it stood when a rule dropped it plus that drop.
func (c *Collector) applyBlocks(sample prompkg.Sample) (prompkg.Sample, relabel.DropInfo) {
	for i := range c.relabelBlocks {
		b := &c.relabelBlocks[i]
		if !b.match.MatchString(sample.Name) {
			continue
		}
		out, drop := b.proc.Apply(sample)
		if drop.Dropped() {
			return sample, drop
		}
		sample = out
	}
	return sample, relabel.DropInfo{}
}

// Structural label names and family-name suffixes that bind the components of a
// typed family (histogram/summary) together. Defined locally because the
// equivalents in pkg/prometheus are unexported; they mirror the Prometheus
// exposition format and MUST stay in sync with pkg/prometheus.
const (
	labelLE       = "le"
	labelQuantile = "quantile"
	suffixSum     = "_sum"
	suffixCount   = "_count"
	suffixBucket  = "_bucket"
)

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
func (c *Collector) relabelAndAssemble(batch prompkg.SampleBatch, checking bool) (prompkg.MetricFamilies, error) {
	processed, tracking := c.applyJobRelabel(batch)

	mfs, err := prompkg.Assemble(processed)
	if err != nil {
		return nil, err
	}

	// No typed family was altered (or dropped) by relabeling: nothing to curate.
	if !tracking.anyTypedTouched {
		return mfs, nil
	}

	invalid, violations := validateTypedFamilies(tracking, mfs)
	if len(invalid) == 0 {
		return mfs, nil
	}

	if checking {
		return nil, fmt.Errorf("relabeling corrupts typed metric families: %s", violations[0])
	}

	// Runtime: drop the corrupted families and reassemble. This can recur every scrape
	// if a rule stays bad or the exporter changed, so rate-limit the warning to avoid
	// flooding the log; the per-family detail stays at debug.
	c.Limit("relabel-typed-family-corruption", 1, 10*time.Minute).
		Warningf("relabeling produced %d typed-family corruption(s); dropped the affected families (enable debug for names)", len(violations))
	for _, v := range violations {
		c.Debugf("relabeling dropped corrupted typed family: %s", v)
	}

	// Closed invalid set computed in one pass, so a single re-filter + reassembly
	// terminates: dropping samples can only remove corruption, never add it.
	filtered := filterInvalidTypedFamilies(processed, invalid)
	return prompkg.Assemble(filtered)
}

// applyJobRelabel runs the relabel processor over every sample, building the
// processed batch (kept samples + HELP remapped from raw family name to final
// family name) and the typed-family tracking the validator consumes.
func (c *Collector) applyJobRelabel(batch prompkg.SampleBatch) (prompkg.SampleBatch, *relabelTracking) {
	t := newRelabelTracking()
	help := newHelpRemap()
	out := prompkg.SampleBatch{Samples: make([]prompkg.Sample, 0, len(batch.Samples))}

	for _, raw := range batch.Samples {
		rawKey, isTyped := typedFamilyKeyOf(raw)

		sample, drop := c.applyBlocks(raw)
		if drop.Dropped() {
			c.onRelabelDrop(raw, drop)
			if isTyped {
				t.recordDropped(rawKey)
			}
			continue
		}

		touched := sample.Name != raw.Name || !labels.Equal(sample.Labels, raw.Labels)
		if isTyped {
			finalKey, _ := typedFamilyKeyOf(sample) // Kind is preserved by relabeling, so still typed
			t.recordKept(rawKey, finalKey, raw, sample, touched)
		}

		out.Samples = append(out.Samples, sample)
		help.add(helpFamilyName(raw), helpFamilyName(sample))
	}

	out.Help = help.remap(batch.Help)
	return out, t
}

// onRelabelDrop logs why a relabel rule dropped a sample, at debug level. It logs
// the metric name and the rule outcome, never label values (cardinality/PII).
func (c *Collector) onRelabelDrop(s prompkg.Sample, d relabel.DropInfo) {
	c.When(d.RuleIndex >= 0).
		Debugf("relabel dropped metric %q: %s (rule %d, action %q)", s.Name, d.Reason, d.RuleIndex, d.Action).
		Else().
		Debugf("relabel dropped metric %q: %s", s.Name, d.Reason)
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
		fs.buckets[sample.Labels.Get(labelLE)]++
	case prompkg.SampleKindSummaryQuantile:
		if fs.quantiles == nil {
			fs.quantiles = make(map[string]int)
		}
		fs.quantiles[sample.Labels.Get(labelQuantile)]++
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
	case prompkg.SampleKindHistogramBucket:
		return typedFamilyKey{name: trimFamilySuffix(s.Name, s.Kind), hash: hashWithout(s.Labels, labelLE)}, true
	case prompkg.SampleKindSummaryQuantile:
		return typedFamilyKey{name: trimFamilySuffix(s.Name, s.Kind), hash: hashWithout(s.Labels, labelQuantile)}, true
	case prompkg.SampleKindHistogramSum, prompkg.SampleKindHistogramCount,
		prompkg.SampleKindSummarySum, prompkg.SampleKindSummaryCount:
		return typedFamilyKey{name: trimFamilySuffix(s.Name, s.Kind), hash: s.Labels.Hash()}, true
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
	return trimFamilySuffix(s.Name, s.Kind)
}

func trimFamilySuffix(name string, kind prompkg.SampleKind) string {
	switch kind {
	case prompkg.SampleKindHistogramBucket:
		return strings.TrimSuffix(name, suffixBucket)
	case prompkg.SampleKindHistogramSum, prompkg.SampleKindSummarySum:
		return strings.TrimSuffix(name, suffixSum)
	case prompkg.SampleKindHistogramCount, prompkg.SampleKindSummaryCount:
		return strings.TrimSuffix(name, suffixCount)
	default:
		return name
	}
}

// structuralMutated reports whether relabeling changed (or removed) the le or
// quantile label that defines a bucket boundary or quantile point. Kind is
// preserved by relabeling, so the raw kind selects the structural label.
func structuralMutated(raw, sample prompkg.Sample) bool {
	switch raw.Kind {
	case prompkg.SampleKindHistogramBucket:
		return raw.Labels.Get(labelLE) != sample.Labels.Get(labelLE)
	case prompkg.SampleKindSummaryQuantile:
		return raw.Labels.Get(labelQuantile) != sample.Labels.Get(labelQuantile)
	default:
		return false
	}
}

// hashWithout hashes the label set with one label removed, preserving order so
// the result matches the assembler's base-label grouping (which strips the
// structural label in place before hashing).
func hashWithout(lbs labels.Labels, name string) uint64 {
	filtered := make(labels.Labels, 0, len(lbs))
	for _, l := range lbs {
		if l.Name == name {
			continue
		}
		filtered = append(filtered, l)
	}
	return filtered.Hash()
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
