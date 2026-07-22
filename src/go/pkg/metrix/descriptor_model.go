// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "math"

type metricKind uint8

type metricMode uint8

const (
	kindGauge metricKind = iota
	kindCounter
	kindHistogram
	kindSummary
	kindStateSet
	kindMeasureSet
)

const (
	modeSnapshot metricMode = iota
	modeStateful
)

type instrumentDescriptor struct {
	name       string
	kind       metricKind
	mode       metricMode
	freshness  FreshnessPolicy // visibility policy used by Read()
	window     MetricWindow
	histogram  *histogramSchema  // set for kindHistogram only
	summary    *summarySchema    // set for kindSummary only
	stateSet   *stateSetSchema   // set for kindStateSet only
	measureSet *measureSetSchema // set for kindMeasureSet only
	meta       MetricMeta
	// metaSet records which optional metadata fields were explicitly declared, so
	// declaration compatibility preserves registration's "compare only if set" rule.
	metaSet metricMetaSet
}

// metricMetaSet records which optional family-metadata fields a registration
// explicitly set. It gates declaration-compatibility checks (preserve-first).
type metricMetaSet struct {
	description   bool
	chartFamily   bool
	chartPriority bool
	unit          bool
	float         bool
}

type histogramSchema struct {
	bounds []float64
}

type summarySchema struct {
	quantiles     []float64
	reservoirSize int
}

type stateSetSchema struct {
	mode   StateSetMode
	states []string
	index  map[string]struct{}
}

type measureSetSchema struct {
	semantics MeasureSetSemantics
	fields    []MeasureFieldSpec
	index     map[string]int
}

// stagedDescConflict is a same-key write whose authority is incompatible with the staged entry
// for that key. desc is the descriptor the resolver reconciles: the effective, bounds-filled
// descriptor for a histogram; the handle itself otherwise.
type stagedDescConflict struct {
	name string
	desc *instrumentDescriptor
}

// sameKeyConflictID keys recorded same-key conflict evidence by series key, authority IDENTITY
// (fingerprint), and a DECLARATION fingerprint, so distinct FULL descriptors (authority AND declared
// metadata) each get their own O(1) bucket: many handles/samples sharing a full descriptor dedup to
// one record, while a same-authority write with a different declaration (a distinct conflict) still
// records without scanning a growing per-authority bucket. declFP collisions (or authority-fp
// collisions) put >1 entry in a bucket, which the *Recorded checks separate with descriptorsFullyEqual.
type sameKeyConflictID struct {
	key    string
	fp     authorityFingerprint
	declFP uint64
}

// authorityFingerprint is a comparable identity for a series authority: two descriptors that are
// series-authority-compatible share a fingerprint (kind/mode/freshness/window/schema). It keys the
// conflict-evidence map so dedup is O(1) and bounded by DISTINCT authorities rather than by handle
// pointer or sample count. It EXCLUDES declaration metadata (unit, description, ...), which is
// reconciled by mergeInstrumentMetadata, not authority identity. schemaHash is a stable hash of the
// type-specific schema; a collision only puts two distinct authorities in one bucket, and the
// bucket is verified with descriptorSeriesAuthoritiesEqual (or a direct bounds compare) so distinct
// authorities are never silently merged.
//
// NOTE on the histogram nil-bounds wildcard: a nil-bounds snapshot histogram is compatible with ANY
// concrete-bounds histogram, so it has no single equal fingerprint. Fingerprints are only taken of
// CONCRETE descriptors (the conflict evidence is the effective, bounds-filled descriptor); the
// committed-vs-wildcard comparison keeps using seriesAuthoritiesCompatible.
type authorityFingerprint struct {
	kind       metricKind
	mode       metricMode
	freshness  FreshnessPolicy
	window     MetricWindow
	schemaHash uint64
}

func authorityFingerprintOf(d *instrumentDescriptor) authorityFingerprint {
	return authorityFingerprint{
		kind:       d.kind,
		mode:       d.mode,
		freshness:  d.freshness,
		window:     d.window,
		schemaHash: d.authoritySchemaHash(),
	}
}

// histogramAuthorityFingerprint fingerprints a histogram write by its EFFECTIVE (observed) bounds
// without cloning: a nil-bounds snapshot descriptor carries no bounds, so the observed bounds are
// hashed directly. It matches authorityFingerprintOf(effectiveHistogramDescriptor(d, bounds)).
func histogramAuthorityFingerprint(d *instrumentDescriptor, bounds []float64) authorityFingerprint {
	return authorityFingerprint{
		kind:       d.kind,
		mode:       d.mode,
		freshness:  d.freshness,
		window:     d.window,
		schemaHash: hashFloat64s(fnvOffset64, bounds),
	}
}

// authoritySchemaHash hashes exactly the type-specific schema fields that
// descriptorSeriesAuthoritiesEqual compares, so equal authorities hash equally. Scalar kinds
// (gauge/counter) have no schema and hash to 0.
func (d *instrumentDescriptor) authoritySchemaHash() uint64 {
	switch d.kind {
	case kindHistogram:
		if d.histogram == nil {
			return 0
		}
		return hashFloat64s(fnvOffset64, d.histogram.bounds)
	case kindSummary:
		if d.summary == nil {
			return 0
		}
		h := hashUint64(fnvOffset64, uint64(d.summary.reservoirSize))
		return hashFloat64s(h, d.summary.quantiles)
	case kindStateSet:
		if d.stateSet == nil {
			return 0
		}
		h := hashUint64(fnvOffset64, uint64(d.stateSet.mode))
		return hashStrings(h, d.stateSet.states)
	case kindMeasureSet:
		if d.measureSet == nil {
			return 0
		}
		h := hashUint64(fnvOffset64, uint64(d.measureSet.semantics))
		for _, f := range d.measureSet.fields {
			h = hashString(h, f.Name)
			if f.Float {
				h = hashUint64(h, 1)
			} else {
				h = hashUint64(h, 0)
			}
		}
		return h
	}
	return 0
}

// FNV-1a helpers (allocation-free) for authority schema hashing. Lengths are folded in so distinct
// groupings (e.g. ["ab","c"] vs ["a","bc"]) do not collide.
const (
	fnvOffset64 uint64 = 14695981039346656037
	fnvPrime64  uint64 = 1099511628211
)

func hashUint64(h, v uint64) uint64 {
	for i := range 8 {
		h ^= (v >> (uint(i) * 8)) & 0xff
		h *= fnvPrime64
	}
	return h
}

func hashFloat64s(h uint64, xs []float64) uint64 {
	h = hashUint64(h, uint64(len(xs)))
	for _, x := range xs {
		if x == 0 {
			x = 0 // canonicalize -0.0 to +0.0 so signed zero hashes as it compares (-0.0 == +0.0)
		}
		h = hashUint64(h, math.Float64bits(x))
	}
	return h
}

func hashString(h uint64, s string) uint64 {
	h = hashUint64(h, uint64(len(s)))
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}
	return h
}

func hashStrings(h uint64, xs []string) uint64 {
	h = hashUint64(h, uint64(len(xs)))
	for _, s := range xs {
		h = hashString(h, s)
	}
	return h
}
