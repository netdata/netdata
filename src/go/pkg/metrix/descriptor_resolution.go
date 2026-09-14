// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"errors"
	"fmt"
	"sort"
)

// descriptorResolution is the outcome of reconciling a cycle's observed instrument
// descriptors against the committed registry.
type descriptorResolution struct {
	failErr   error
	supersede []string                         // committed names whose (unobserved) kind is replaced by the observed one
	drop      []string                         // names with ambiguous multi-kind writes and no established authority
	canonical map[string]*instrumentDescriptor // accepted name => the single descriptor to publish for all its series
}

// observedAuthority accumulates one series authority observed for a name this cycle,
// canonicalizing the declared metadata of every compatible write into a single
// descriptor so the name publishes one descriptor rather than a raw per-write one.
type observedAuthority struct {
	canonical *instrumentDescriptor
}

// nameAuthorities holds every distinct series authority observed for one name this cycle. `all` is
// the flat list the resolution loop walks; `byFP` indexes it by authority fingerprint so grouping a
// new write is O(1) with no cap (a bucket per fingerprint, length 1 except on a hash collision).
type nameAuthorities struct {
	all  []*observedAuthority
	byFP map[authorityFingerprint][]*observedAuthority
}

// reconcileStagedDesc reconciles a same-key write against the staged entry's running-canonical
// descriptor. On a compatible write it returns the metadata-merged canonical (so the staged
// entry always carries the complete declaration for its authority - later writes reconcile
// against the growing canonical, not just the first descriptor). It returns an error when the
// authority is incompatible or a declaration conflicts. Pure (no side effects).
func reconcileStagedDesc(existing, incoming *instrumentDescriptor) (*instrumentDescriptor, error) {
	if err := descriptorSeriesAuthorityCompat(existing, incoming); err != nil {
		return nil, err
	}
	return mergeInstrumentMetadata(existing, incoming)
}

// reconcileSameKeyDesc merges a compatible same-key write into the staged entry's canonical and
// returns (canonical, true); on an incompatible one it records the conflicting DESCRIPTOR and
// returns (nil, false) so the caller drops the write. Conflicts are deduped by FULL descriptor
// identity (authority + declaration), so many handles or repeats of the same descriptor record once
// (evidence stays O(distinct descriptors), not O(handles)/O(samples)), while a same-authority write
// with a DIFFERENT declaration (e.g. a conflicting unit) is a distinct descriptor and IS recorded -
// so the resolver still sees and fails on that declaration conflict, and a committed-matching
// authority observed only as a same-key conflict still reaches the resolver. The identity check
// runs BEFORE reconcileStagedDesc, so a repeated known conflict is dropped without re-deriving (and
// allocating) its mismatch error. Non-histogram record paths call it under c.mu; histograms use
// reconcileSameKeyHistogram.
func (c *storeCore) reconcileSameKeyDesc(key string, existing, incoming *instrumentDescriptor) (*instrumentDescriptor, bool) {
	fp := authorityFingerprintOf(incoming)
	if c.sameKeyConflictRecorded(key, fp, incoming) {
		return nil, false // this exact descriptor already conflicts on this key
	}
	merged, err := reconcileStagedDesc(existing, incoming)
	if err != nil {
		c.recordSameKeyConflict(key, fp, incoming)
		return nil, false
	}
	return merged, true
}

// reconcileSameKeyHistogram reconciles a differing same-key histogram write. It compares EFFECTIVE
// (bounds-filled) descriptors so a different observed bucket schema is a conflict, and on a
// compatible write returns the metadata-merged staged descriptor - keeping its bounds-nature (a
// nil-bounds snapshot stays nil) so point normalization stays crash-safe for client-driven bounds.
// A histogram authority already recorded for this key is dropped BEFORE the effective clone (via a
// no-clone identity+bounds check), so a repeat of the same conflicting schema stays O(1). A DIFFERENT
// observed schema is a distinct authority and IS recorded (reaching the resolver), which is what lets
// a committed-matching observation still fail loud. Under c.mu.
func (c *storeCore) reconcileSameKeyHistogram(key string, entry *stagedHistogram, incomingDesc *instrumentDescriptor, incomingBounds []float64) (*instrumentDescriptor, bool) {
	fp := histogramAuthorityFingerprint(incomingDesc, incomingBounds)
	if c.histogramConflictRecorded(key, fp, incomingDesc, incomingBounds) {
		return nil, false
	}
	incomingEff := effectiveHistogramDescriptor(incomingDesc, incomingBounds)
	if !descriptorSeriesAuthoritiesEqual(effectiveHistogramDescriptor(entry.desc, entry.bounds), incomingEff) {
		c.recordSameKeyConflict(key, fp, incomingEff)
		return nil, false
	}
	merged, err := mergeInstrumentMetadata(entry.desc, incomingDesc)
	if err != nil {
		c.recordSameKeyConflict(key, fp, incomingEff)
		return nil, false
	}
	return merged, true
}

// recordSameKeyConflict records an incompatible same-key write as commit-time conflict evidence so
// the resolver fails/drops the name. Deduped by (key, authority fp, declaration fp) with
// verify-on-hit: each distinct FULL descriptor gets its own O(1) bucket, and the caller's *Recorded
// check confirms full equality so a fp collision records a distinct entry rather than dropping one.
// evidence is the descriptor the resolver reconciles (the effective histogram descriptor, or the
// handle itself). Under c.mu.
func (c *storeCore) recordSameKeyConflict(key string, fp authorityFingerprint, evidence *instrumentDescriptor) {
	id := sameKeyConflictID{key: key, fp: fp, declFP: declarationFingerprint(evidence)}
	if c.active.recordedConflict == nil {
		c.active.recordedConflict = make(map[sameKeyConflictID][]*instrumentDescriptor)
	}
	c.active.recordedConflict[id] = append(c.active.recordedConflict[id], evidence)
	c.active.conflicts = append(c.active.conflicts, stagedDescConflict{name: evidence.name, desc: evidence})
}

// sameKeyConflictRecorded reports whether a FULLY-equal conflict (same authority AND declaration)
// is already recorded for key. incoming is concrete (a non-histogram handle or an effective
// histogram descriptor). Keying by declaration fingerprint keeps this O(1) even for many distinct
// declarations of one authority; the bucket is normally length 1 (longer only on a fingerprint
// collision, separated by descriptorsFullyEqual). Under c.mu.
func (c *storeCore) sameKeyConflictRecorded(key string, fp authorityFingerprint, incoming *instrumentDescriptor) bool {
	id := sameKeyConflictID{key: key, fp: fp, declFP: declarationFingerprint(incoming)}
	for _, rec := range c.active.recordedConflict[id] {
		if descriptorsFullyEqual(rec, incoming) {
			return true
		}
	}
	return false
}

// histogramConflictRecorded reports whether a fully-equal histogram conflict (same observed bounds
// AND declaration) is already recorded for key. It compares the recorded effective bounds to the
// observed bounds directly, so the incoming effective descriptor need not be cloned to detect a
// repeat. Under c.mu.
func (c *storeCore) histogramConflictRecorded(key string, fp authorityFingerprint, d *instrumentDescriptor, bounds []float64) bool {
	id := sameKeyConflictID{key: key, fp: fp, declFP: declarationFingerprint(d)}
	for _, rec := range c.active.recordedConflict[id] {
		if rec.kind == kindHistogram && rec.histogram != nil &&
			rec.mode == d.mode && rec.freshness == d.freshness && rec.window == d.window &&
			equalHistogramBounds(rec.histogram.bounds, bounds) && descriptorDeclarationsEqual(rec, d) {
			return true
		}
	}
	return false
}

// resolveObservedDescriptors groups this cycle's staged writes by metric name and
// reconciles each name against its committed descriptor. Per name:
//   - a single observed kind that matches the committed authority (or a new name) -> ok;
//   - a single observed kind incompatible with a committed kind that was NOT observed
//     this cycle -> supersede the committed kind;
//   - multiple incompatible kinds observed, one of which is the committed (established)
//     kind that is actively observed -> unresolvable, fail the whole cycle;
//   - multiple incompatible kinds observed with no established/observed authority ->
//     ambiguous, drop this name's writes for the cycle (other names still commit).
//
// It only reads state; the caller applies the supersessions, drops, and failure.
func (c *storeCore) resolveObservedDescriptors() descriptorResolution {
	// authorities[name] holds the distinct (mutually incompatible) series authorities observed for
	// name this cycle. Each accumulates a CANONICAL descriptor: the declared metadata of every
	// compatible write is merged into it (order-independent), so the name publishes one descriptor
	// rather than whichever raw write landed first. Grouping is indexed by authority fingerprint,
	// so it is O(1) per write with NO cap: every distinct authority is kept, so a declaration
	// conflict on ANY of them is merged and detected (fixing the class of bug where a capped-out
	// authority silently hid a conflict), and the fail-vs-drop decision sees the true count. For a
	// pathological many-schema name this is O(distinct authorities) memory - transient, bounded by
	// the cycle's write count, and the name fails/drops anyway.
	authorities := make(map[string]*nameAuthorities)
	// declErrs collects declaration (metadata) conflicts between series-authority-
	// compatible descriptors observed this cycle; they fail the cycle at the end.
	var declErrs []error
	// committedObservedNames marks names whose live committed authority is actively observed this
	// cycle. It drives the multi-authority fail-vs-drop decision below and is computed over EVERY
	// observed authority. Because each distinct same-key authority is recorded as a conflict
	// (deduped by full descriptor identity), a committed-matching write observed only as a same-key
	// conflict still reaches this loop.
	committedObservedNames := make(map[string]bool)
	// Descriptors reach record already reduced to their effective authority (histograms carry their
	// observed bounds), so no wildcard remains and series-authority compatibility is a symmetric
	// equivalence. Compatible authorities are canonicalized by merging their declared metadata; a
	// metadata conflict fails the cycle.
	record := func(name string, desc *instrumentDescriptor) {
		if !committedObservedNames[name] {
			if committed := realCommittedAuthority(c.instruments[name]); committed != nil &&
				seriesAuthoritiesCompatible(committed, desc) {
				committedObservedNames[name] = true
			}
		}
		na := authorities[name]
		if na == nil {
			na = &nameAuthorities{byFP: make(map[authorityFingerprint][]*observedAuthority)}
			authorities[name] = na
		}
		fp := authorityFingerprintOf(desc)
		// The bucket is normally length 1 (distinct authorities have distinct fingerprints); a
		// length > 1 bucket only arises on a hash collision, which descriptorSeriesAuthoritiesEqual
		// separates so distinct authorities are never merged.
		for _, auth := range na.byFP[fp] {
			if descriptorSeriesAuthoritiesEqual(auth.canonical, desc) {
				merged, err := mergeDeclarations(auth.canonical, desc)
				if err != nil {
					declErrs = append(declErrs, err)
					return
				}
				auth.canonical = merged
				return // same authority already recorded
			}
		}
		auth := &observedAuthority{canonical: desc}
		na.byFP[fp] = append(na.byFP[fp], auth)
		na.all = append(na.all, auth)
	}
	for _, s := range c.active.gauges {
		record(s.name, s.desc)
	}
	for _, s := range c.active.counters {
		record(s.name, s.desc)
	}
	for _, s := range c.active.histograms {
		record(s.name, effectiveHistogramDescriptor(s.desc, s.bounds))
	}
	for _, s := range c.active.summaries {
		record(s.name, s.desc)
	}
	for _, s := range c.active.stateSet {
		record(s.name, s.desc)
	}
	for _, s := range c.active.measureSetGauges {
		record(s.name, s.desc)
	}
	for _, s := range c.active.measureSetCounters {
		record(s.name, s.desc)
	}
	// Fold in observed same-key descriptors that differed from their staged entry, so an
	// incompatible second write fails/drops the name and a compatible one is merged.
	for _, cc := range c.active.conflicts {
		record(cc.name, cc.desc)
	}

	res := descriptorResolution{canonical: make(map[string]*instrumentDescriptor)}
	var failNames []string
	for name, na := range authorities {
		auths := na.all
		// The raw committed descriptor supplies the canonical DECLARATION base (its declared
		// metadata is preserved) even when it is a never-observed nil-bounds histogram
		// wildcard. realCommittedAuthority is the live AUTHORITY (nil for such a wildcard)
		// and drives only the supersede/fail decision.
		rawCommitted := c.instruments[name]
		committed := realCommittedAuthority(rawCommitted)
		if len(auths) < 2 {
			observed := auths[0].canonical
			switch {
			case committed != nil && !seriesAuthoritiesCompatible(committed, observed):
				// A live committed authority of a different kind was not observed: supersede it.
				res.supersede = append(res.supersede, name)
				res.canonical[name] = observed
			case rawCommitted != nil && seriesAuthoritiesCompatible(rawCommitted, observed):
				// Committed authority (live, or a wildcard registration) is compatible: its
				// declared metadata is the canonical base, merged with the observed one.
				merged, err := mergeDeclarations(rawCommitted, observed)
				if err != nil {
					declErrs = append(declErrs, err)
					continue
				}
				res.canonical[name] = merged
			default:
				// New name (no committed registration), or an incompatible wildcard.
				res.canonical[name] = observed
			}
			continue
		}
		// Multiple incompatible authorities observed for one name this cycle. A committed
		// declaration must still be honored: an observed authority that is series-authority-
		// compatible with the committed descriptor but declares conflicting metadata is a fail-loud
		// bug, independent of whether the committed kind is a LIVE authority (realCommittedAuthority
		// classifies fail-vs-drop; declaration compatibility does not depend on it). Without this,
		// e.g. a committed nil-bounds histogram unit=bytes vs an observed unit=seconds would silently
		// drop instead of failing. The single-authority branch already merges against rawCommitted.
		if rawCommitted != nil {
			for _, auth := range auths {
				if seriesAuthoritiesCompatible(rawCommitted, auth.canonical) {
					if _, err := mergeDeclarations(rawCommitted, auth.canonical); err != nil {
						declErrs = append(declErrs, err)
					}
				}
			}
		}
		// committedObserved was computed over every observed authority, so the fail-vs-drop decision
		// is complete.
		if committedObservedNames[name] {
			// An established authority is actively written alongside an incompatible one:
			// unresolvable, fail loud (never silent data loss).
			failNames = append(failNames, name)
		} else {
			// Ambiguous with no established authority: drop this name, commit the rest.
			res.drop = append(res.drop, name)
		}
	}
	var joinErrs []error
	if len(failNames) > 0 {
		sort.Strings(failNames)
		joinErrs = append(joinErrs, fmt.Errorf("metrix: conflicting instrument kinds actively observed in one cycle for %v", failNames))
	}
	if len(declErrs) > 0 {
		// Deterministic order + dedup so the cycle error is stable across map iteration.
		sort.Slice(declErrs, func(i, j int) bool { return declErrs[i].Error() < declErrs[j].Error() })
		last := ""
		for _, err := range declErrs {
			if msg := err.Error(); msg != last {
				joinErrs = append(joinErrs, err)
				last = msg
			}
		}
	}
	res.failErr = errors.Join(joinErrs...)
	return res
}
