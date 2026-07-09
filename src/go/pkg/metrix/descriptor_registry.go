// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"strings"
)

func defaultFreshness(mode metricMode) FreshnessPolicy {
	if mode == modeSnapshot {
		return FreshnessCycle
	}
	return FreshnessCommitted
}

func (c *storeCore) registerInstrument(name string, kind metricKind, mode metricMode, opts ...InstrumentOption) (*instrumentDescriptor, error) {
	cfg := instrumentConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(&cfg)
		}
	}

	if cfg.windowSet && !isWindowAllowed(kind, mode) {
		return nil, fmt.Errorf("metrix: WithWindow is valid only for stateful histogram/summary")
	}
	if len(cfg.histogramBounds) > 0 && kind != kindHistogram {
		return nil, fmt.Errorf("metrix: histogram bounds are invalid for this instrument kind")
	}
	if len(cfg.summaryQuantile) > 0 && kind != kindSummary {
		return nil, fmt.Errorf("metrix: summary quantiles are invalid for this instrument kind")
	}
	if cfg.summaryReservoirSet && !(kind == kindSummary && mode == modeStateful) {
		return nil, fmt.Errorf("metrix: summary reservoir size is valid only for stateful summaries")
	}
	if (len(cfg.states) > 0 || cfg.stateSetMode != nil) && kind != kindStateSet {
		return nil, fmt.Errorf("metrix: stateset options are invalid for this instrument kind")
	}
	if (len(cfg.measureSetFields) > 0 || cfg.measureSetSemantics != nil) && kind != kindMeasureSet {
		return nil, fmt.Errorf("metrix: measureset options are invalid for this instrument kind")
	}

	window := WindowCumulative
	if cfg.windowSet {
		window = cfg.window
	}

	fresh := defaultFreshness(mode)
	if cfg.freshnessSet {
		fresh = cfg.freshness
	}
	if mode == modeStateful && window == WindowCycle && (kind == kindHistogram || kind == kindSummary) {
		if cfg.freshnessSet && fresh != FreshnessCycle {
			return nil, fmt.Errorf("metrix: window=cycle requires FreshnessCycle")
		}
		fresh = FreshnessCycle
	}
	if mode == modeSnapshot && fresh == FreshnessCommitted {
		return nil, fmt.Errorf("metrix: snapshot instruments cannot use FreshnessCommitted")
	}

	metricMeta := MetricMeta{
		Description:   strings.TrimSpace(cfg.description),
		ChartFamily:   strings.TrimSpace(cfg.chartFamily),
		ChartPriority: cfg.chartPriority,
		Unit:          strings.TrimSpace(cfg.unit),
		Float:         cfg.float,
	}

	var histogram *histogramSchema
	if kind == kindHistogram {
		s, err := buildHistogramSchema(cfg, mode)
		if err != nil {
			return nil, err
		}
		histogram = s
	}

	var summary *summarySchema
	if kind == kindSummary {
		s, err := buildSummarySchema(cfg)
		if err != nil {
			return nil, err
		}
		summary = s
	}

	var schema *stateSetSchema
	if kind == kindStateSet {
		s, err := buildStateSetSchema(cfg)
		if err != nil {
			return nil, err
		}
		schema = s
	}

	var measureSet *measureSetSchema
	if kind == kindMeasureSet {
		s, err := buildMeasureSetSchema(cfg)
		if err != nil {
			return nil, err
		}
		measureSet = s
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	candidate := &instrumentDescriptor{
		name:       name,
		kind:       kind,
		mode:       mode,
		freshness:  fresh,
		window:     window,
		histogram:  histogram,
		summary:    summary,
		stateSet:   schema,
		measureSet: measureSet,
		meta:       metricMeta,
		metaSet: metricMetaSet{
			description:   cfg.descriptionSet,
			chartFamily:   cfg.chartFamilySet,
			chartPriority: cfg.chartPrioritySet,
			unit:          cfg.unitSet,
			float:         cfg.floatSet,
		},
	}

	// Resolve the candidate against any existing authority for this name. Registration
	// is transactional: during an active cycle a new authority is staged into
	// pendingInstruments and installed into the committed registry only on
	// CommitCycleSuccess (from observed series); an aborted cycle discards it. A name
	// may carry several mutually-incompatible pending authorities in one cycle (a
	// superseding kind), so pending is a per-name list. A metadata mismatch against a
	// series-authority-compatible descriptor is always a declaration bug and stays
	// fail-loud regardless of cycle state.
	if committed, ok := c.instruments[name]; ok {
		// A nil-bounds histogram (a fresh registration or a never-observed committed
		// descriptor) is an as-yet-unbounded wildcard, so compare via the symmetric
		// helper. Once observed, the committed descriptor carries concrete bounds (the
		// registry is converged at commit), so no schema lookup is needed here.
		if seriesAuthoritiesCompatible(committed, candidate) {
			if err := descriptorDeclarationCompat(committed, candidate); err != nil {
				return nil, err
			}
			return dedupDescriptor(committed, candidate), nil
		}
		// The committed authority is incompatible. Outside a cycle there is nothing to
		// reconcile, so fail loud; during a cycle, defer to commit (the committed kind
		// is superseded if unobserved, or the cycle fails if both are observed) after
		// checking the candidate against any compatible pending authority below.
		if c.active == nil {
			return nil, descriptorSeriesAuthorityCompat(committed, candidate)
		}
	} else if c.active == nil {
		c.instruments[name] = candidate
		return candidate, nil
	}

	// Active cycle: dedup against a compatible pending authority (declaration must
	// still match), otherwise stage the candidate as a new pending authority.
	for _, pending := range c.active.pendingInstruments[name] {
		if seriesAuthoritiesCompatible(pending, candidate) {
			if err := descriptorDeclarationCompat(pending, candidate); err != nil {
				return nil, err
			}
			return dedupDescriptor(pending, candidate), nil
		}
	}
	c.active.pendingInstruments[name] = append(c.active.pendingInstruments[name], candidate)
	return candidate, nil
}

// descriptorSeriesAuthoritiesEqual reports whether the incoming descriptor may back the same
// series as the existing one for a shared name: kind, mode, freshness, window, and the
// type-specific schema all agree. A snapshot histogram registered with nil bounds is a wildcard
// that adopts any existing bounds. It ALLOCATES NOTHING, so hot paths that only need the yes/no
// answer (the resolver's authority grouping over many distinct schemas) use it directly rather
// than allocating and discarding a descriptive error via descriptorSeriesAuthorityCompat.
func descriptorSeriesAuthoritiesEqual(existing, incoming *instrumentDescriptor) bool {
	if existing.kind != incoming.kind || existing.mode != incoming.mode ||
		existing.freshness != incoming.freshness || existing.window != incoming.window {
		return false
	}
	switch incoming.kind {
	case kindHistogram:
		return (incoming.mode == modeSnapshot && incoming.histogram == nil) || equalHistogramSchema(existing.histogram, incoming.histogram)
	case kindSummary:
		return equalSummarySchema(existing.summary, incoming.summary)
	case kindStateSet:
		return equalStateSetSchema(existing.stateSet, incoming.stateSet)
	case kindMeasureSet:
		return equalMeasureSetSchema(existing.measureSet, incoming.measureSet)
	}
	return true
}

// descriptorDeclarationsEqual reports whether two descriptors declare identical family metadata:
// the same fields were set (metaSet) and to the same values. Allocation-free. The conflict-evidence
// dedup uses it (with descriptorsFullyEqual) so a same-authority write with a DIFFERENT declaration
// is recorded rather than collapsed, keeping its declaration conflict visible to the resolver.
func descriptorDeclarationsEqual(a, b *instrumentDescriptor) bool {
	return a.metaSet == b.metaSet &&
		a.meta.Description == b.meta.Description &&
		a.meta.ChartFamily == b.meta.ChartFamily &&
		a.meta.ChartPriority == b.meta.ChartPriority &&
		a.meta.Unit == b.meta.Unit &&
		a.meta.Float == b.meta.Float
}

// declarationFingerprint hashes exactly the fields descriptorDeclarationsEqual compares (which
// metadata was set, and its values), so equal declarations hash equally. It keys the same-key
// conflict dedup by full descriptor identity, keeping that dedup O(1) even when one authority sees
// many distinct declarations. Allocation-free.
func declarationFingerprint(d *instrumentDescriptor) uint64 {
	var bits uint64
	if d.metaSet.description {
		bits |= 1 << 0
	}
	if d.metaSet.chartFamily {
		bits |= 1 << 1
	}
	if d.metaSet.chartPriority {
		bits |= 1 << 2
	}
	if d.metaSet.unit {
		bits |= 1 << 3
	}
	if d.metaSet.float {
		bits |= 1 << 4
	}
	if d.meta.Float {
		bits |= 1 << 5
	}
	h := hashUint64(fnvOffset64, bits)
	h = hashUint64(h, uint64(d.meta.ChartPriority))
	h = hashString(h, d.meta.Description)
	h = hashString(h, d.meta.ChartFamily)
	h = hashString(h, d.meta.Unit)
	return h
}

// descriptorsFullyEqual reports whether two descriptors are identical in BOTH series authority and
// declared metadata - i.e. the same instrument in every respect the resolver reconciles.
func descriptorsFullyEqual(a, b *instrumentDescriptor) bool {
	return descriptorSeriesAuthoritiesEqual(a, b) && descriptorDeclarationsEqual(a, b)
}

// descriptorSeriesAuthorityCompat reports the same relation as descriptorSeriesAuthoritiesEqual,
// but returns the first field mismatch as a descriptive error (nil when compatible) for the
// fail-loud paths (registration, commit-time declaration failure). It re-derives the specific
// mismatch only when the fast equality check already said "not equal", so the happy path stays
// allocation-free and the error paths keep their exact messages.
func descriptorSeriesAuthorityCompat(existing, incoming *instrumentDescriptor) error {
	if descriptorSeriesAuthoritiesEqual(existing, incoming) {
		return nil
	}
	name := incoming.name
	switch {
	case existing.kind != incoming.kind:
		return fmt.Errorf("metrix: instrument kind mismatch for %s", name)
	case existing.mode != incoming.mode:
		return fmt.Errorf("metrix: instrument mode mismatch for %s", name)
	case existing.freshness != incoming.freshness:
		return fmt.Errorf("metrix: instrument freshness mismatch for %s", name)
	case existing.window != incoming.window:
		return fmt.Errorf("metrix: instrument window mismatch for %s", name)
	}
	switch incoming.kind {
	case kindHistogram:
		return fmt.Errorf("metrix: histogram schema mismatch for %s", name)
	case kindSummary:
		return fmt.Errorf("metrix: summary schema mismatch for %s", name)
	case kindStateSet:
		return fmt.Errorf("metrix: stateset schema mismatch for %s", name)
	case kindMeasureSet:
		return fmt.Errorf("metrix: measureset schema mismatch for %s", name)
	}
	return fmt.Errorf("metrix: instrument authority mismatch for %s", name) // unreachable: not equal, no field differs
}

// descriptorDeclarationCompat reports whether the incoming descriptor's explicitly
// declared family metadata conflicts with the existing (first) descriptor. Fields
// the incoming did not set are ignored (preserve-first). It returns the first
// conflict as an error, or nil when compatible.
func descriptorDeclarationCompat(existing, incoming *instrumentDescriptor) error {
	name := incoming.name
	if incoming.metaSet.description && existing.meta.Description != incoming.meta.Description {
		return fmt.Errorf("metrix: metric description mismatch for %s", name)
	}
	if incoming.metaSet.chartFamily && existing.meta.ChartFamily != incoming.meta.ChartFamily {
		return fmt.Errorf("metrix: metric chart family mismatch for %s", name)
	}
	if incoming.metaSet.chartPriority && existing.meta.ChartPriority != incoming.meta.ChartPriority {
		return fmt.Errorf("metrix: metric chart priority mismatch for %s", name)
	}
	if incoming.metaSet.unit && existing.meta.Unit != incoming.meta.Unit {
		return fmt.Errorf("metrix: metric unit mismatch for %s", name)
	}
	if incoming.metaSet.float && existing.meta.Float != incoming.meta.Float {
		return fmt.Errorf("metrix: metric float mismatch for %s", name)
	}
	return nil
}

// baselineSeriesForWrite returns the previously committed series for key only when its
// descriptor is series-authority-compatible with desc. Accumulating writes (stateful
// Add and cumulative windows) seed their baseline from the prior committed value;
// without this guard a write whose name was just superseded by a different kind would
// seed its baseline from the incompatible old series. Callers hold c.mu.
func (c *storeCore) baselineSeriesForWrite(key string, desc *instrumentDescriptor) *committedSeries {
	existing := c.snapshot.Load().series[key]
	if existing == nil || existing.desc == nil {
		return nil
	}
	if descriptorSeriesAuthorityCompat(existing.desc, desc) != nil {
		return nil
	}
	return existing
}

// effectiveHistogramDescriptor returns a descriptor whose histogram bounds reflect the
// bounds observed this cycle. A snapshot histogram registered with nil bounds is a
// wildcard descriptor; comparing two such writes by descriptor alone hides genuinely
// different observed bucket schemas, so authority resolution compares the observed
// bounds instead. Non-histogram or explicit-bounds descriptors are returned unchanged.
func effectiveHistogramDescriptor(desc *instrumentDescriptor, observedBounds []float64) *instrumentDescriptor {
	if desc.kind != kindHistogram || desc.histogram != nil {
		return desc
	}
	return cloneInstrumentDescriptorWithHistogram(desc, observedBounds)
}

// realCommittedAuthority returns the committed descriptor only when it is a live
// authority. A never-observed nil-bounds snapshot histogram is a registration wildcard
// (its bounds are established only by observation, at which point convergence installs a
// concrete descriptor), so it does not count as a committed authority in resolution.
func realCommittedAuthority(committed *instrumentDescriptor) *instrumentDescriptor {
	if committed == nil {
		return nil
	}
	if committed.kind == kindHistogram && committed.histogram == nil {
		return nil
	}
	return committed
}

// mergeInstrumentMetadata unions ONLY the declared meta fields (description, chartFamily,
// chartPriority, unit, float) of two series-authority-compatible descriptors into a canonical
// one: a field declared on either side wins, and a field declared on BOTH sides with different
// values is a conflict (returned as an error). It never touches instrument SCHEMA (histogram
// bounds, summary quantiles, ...) - that authority is fixed by the series-authority check and,
// for histograms, by the staged entry's observed bounds; so a running-canonical staged entry
// keeps its bounds-nature (a nil-bounds snapshot stays nil, crash-safe for client-driven bounds).
// Order-independent. Lazy: a semantic no-op returns a's pointer so the canonical pass skips
// re-cloning unchanged series, keeping the commit's clone/allocation work O(touched), not
// O(retained) (the pass itself still scans every live series).
func mergeInstrumentMetadata(a, b *instrumentDescriptor) (*instrumentDescriptor, error) {
	merged := a
	ensureCopy := func() {
		if merged == a {
			cp := *a
			merged = &cp
		}
	}
	if b.metaSet.description {
		switch {
		case !merged.metaSet.description:
			ensureCopy()
			merged.meta.Description = b.meta.Description
			merged.metaSet.description = true
		case merged.meta.Description != b.meta.Description:
			return nil, fmt.Errorf("metrix: metric description mismatch for %s", a.name)
		}
	}
	if b.metaSet.chartFamily {
		switch {
		case !merged.metaSet.chartFamily:
			ensureCopy()
			merged.meta.ChartFamily = b.meta.ChartFamily
			merged.metaSet.chartFamily = true
		case merged.meta.ChartFamily != b.meta.ChartFamily:
			return nil, fmt.Errorf("metrix: metric chart family mismatch for %s", a.name)
		}
	}
	if b.metaSet.chartPriority {
		switch {
		case !merged.metaSet.chartPriority:
			ensureCopy()
			merged.meta.ChartPriority = b.meta.ChartPriority
			merged.metaSet.chartPriority = true
		case merged.meta.ChartPriority != b.meta.ChartPriority:
			return nil, fmt.Errorf("metrix: metric chart priority mismatch for %s", a.name)
		}
	}
	if b.metaSet.unit {
		switch {
		case !merged.metaSet.unit:
			ensureCopy()
			merged.meta.Unit = b.meta.Unit
			merged.metaSet.unit = true
		case merged.meta.Unit != b.meta.Unit:
			return nil, fmt.Errorf("metrix: metric unit mismatch for %s", a.name)
		}
	}
	if b.metaSet.float {
		switch {
		case !merged.metaSet.float:
			ensureCopy()
			merged.meta.Float = b.meta.Float
			merged.metaSet.float = true
		case merged.meta.Float != b.meta.Float:
			return nil, fmt.Errorf("metrix: metric float mismatch for %s", a.name)
		}
	}
	return merged, nil
}

// mergeDeclarations unions the declared metadata (mergeInstrumentMetadata) AND carries concrete
// histogram bounds when one side is a nil-bounds wildcard, producing the canonical descriptor the
// resolver publishes. Used by the resolver, where a nil-bounds committed/observed descriptor must
// adopt the concrete observed bounds. `a` provides the base (authority fields); pass the
// committed/first descriptor as `a` where one exists. Order-independent; lazy (see
// mergeInstrumentMetadata).
func mergeDeclarations(a, b *instrumentDescriptor) (*instrumentDescriptor, error) {
	merged, err := mergeInstrumentMetadata(a, b)
	if err != nil {
		return nil, err
	}
	if merged.kind == kindHistogram && merged.histogram == nil && b.histogram != nil {
		if merged == a { // still the shared base; copy before adding bounds
			cp := *a
			merged = &cp
		}
		merged.histogram = b.histogram
	}
	return merged, nil
}

// seriesAuthoritiesCompatible reports series-authority compatibility treating a
// nil-bounds snapshot histogram as a wildcard on EITHER side. It is used only for
// single-pair comparisons where one side may be an as-yet-unbounded histogram (a
// fresh registration, or a never-observed committed descriptor). The resolver's
// observed grouping does NOT use it - it reduces observed histograms to concrete
// effective bounds first, keeping grouping transitive with a single directional check.
func seriesAuthoritiesCompatible(a, b *instrumentDescriptor) bool {
	return descriptorSeriesAuthoritiesEqual(a, b) || descriptorSeriesAuthoritiesEqual(b, a)
}

// dedupDescriptor is the descriptor a handle receives when its registration dedups to
// an existing compatible authority. For histograms it keeps the CANDIDATE's bounds
// shape while preserving the existing (first) declared metadata: a nil-bounds
// registration must stay nil so its record path normalizes from the observed point
// (crash-safe for client-driven bounds), and an explicit registration keeps its
// declared bounds (validated at record). Non-histogram handles reuse the existing
// descriptor unchanged.
func dedupDescriptor(existing, candidate *instrumentDescriptor) *instrumentDescriptor {
	if candidate.kind != kindHistogram {
		return existing
	}
	merged := *existing
	merged.histogram = candidate.histogram
	return &merged
}

func cloneInstrumentDescriptorWithHistogram(desc *instrumentDescriptor, bounds []float64) *instrumentDescriptor {
	cp := *desc
	cp.histogram = &histogramSchema{bounds: append([]float64(nil), bounds...)}
	return &cp
}
