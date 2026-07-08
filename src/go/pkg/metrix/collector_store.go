// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

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

type committedSeries struct {
	id     SeriesID
	hash64 uint64
	key    string
	name   string
	// hostScope is immutable after publish and partitions otherwise-identical series.
	hostScopeKey string
	hostScope    HostScope
	// labels are immutable after series publish and can be safely shared across snapshots.
	labels    []Label
	labelsKey string
	desc      *instrumentDescriptor
	value     SampleValue // last committed sample value
	// Internal successful-cycle clock used only for retention aging.
	lastSeenSuccessCycle uint64
	// Internal runtime clock (unix nanos) used only by runtime retention.
	runtimeLastSeenUnixNano int64

	// Counter two-sample state (used by Delta()).
	counterCurrent     SampleValue
	counterPrevious    SampleValue
	counterHasPrev     bool
	counterCurrentSeq  uint64
	counterPreviousSeq uint64

	// Histogram current sample (used by Histogram()).
	histogramCount      SampleValue
	histogramSum        SampleValue
	histogramCumulative []SampleValue

	// Summary current sample (used by Summary()).
	summaryCount     SampleValue
	summarySum       SampleValue
	summaryQuantiles []SampleValue
	summarySketch    *summaryQuantileSketch // cumulative stateful quantile estimator

	// StateSet current sample (used by StateSet()).
	stateSetValues map[string]bool

	// MeasureSet current sample (used by MeasureSet()).
	measureSetValues         []SampleValue
	measureSetPreviousValues []SampleValue
	measureSetHasPrev        bool
	measureSetCurrentSeq     uint64
	measureSetPreviousSeq    uint64

	meta SeriesMeta
}

type readSnapshot struct {
	collectMeta CollectMeta
	series      map[string]*committedSeries   // key => series
	byName      map[string][]*committedSeries // metric name => stable ordered series list
	// runtimeBase links runtime snapshots in overlay mode (nil for materialized snapshots).
	runtimeBase *readSnapshot
	// runtimeDepth tracks overlay chain depth for runtime compaction heuristics.
	runtimeDepth int
}

type cycleFrame struct {
	seq                uint64
	err                error
	hostScopes         map[string]HostScope
	gauges             map[string]*stagedGauge
	counters           map[string]*stagedCounter
	histograms         map[string]*stagedHistogram
	summaries          map[string]*stagedSummary
	stateSet           map[string]*stagedStateSet
	measureSetGauges   map[string]*stagedMeasureSet
	measureSetCounters map[string]*stagedMeasureSet
	// pendingInstruments holds the descriptors registered during this cycle, grouped
	// per name. A name may carry several mutually-incompatible authorities in one
	// cycle (e.g. a kind that supersedes the committed one), so each name maps to a
	// list of authority representatives. They drive registration dedup within the
	// cycle and are discarded when it ends; they are NOT merged into the committed
	// registry (only observed series install a committed descriptor, see
	// CommitCycleSuccess).
	pendingInstruments map[string][]*instrumentDescriptor
	// conflicts records same-key writes whose authority is INCOMPATIBLE with the staged entry
	// (a different kind/mode/schema, or a conflicting declaration). A compatible same-key write
	// is instead merged into the staged entry's running-canonical descriptor and never recorded
	// here. Commit-time resolution folds these in so the name fails or drops rather than silently
	// keeping one arbitrary authority. Deduped by full descriptor identity (key, authority
	// fingerprint, declaration fingerprint) via recordedConflict, so many handles or samples that
	// share a full descriptor record once (evidence stays O(distinct full descriptors), not
	// O(handles)/O(samples)), while each distinct authority OR declaration is recorded.
	conflicts []stagedDescConflict
	// recordedConflict buckets recorded conflict evidence by full descriptor identity (series key,
	// authority fingerprint, declaration fingerprint) for O(1) dedup. Each bucket holds the evidence
	// descriptors for that identity - normally one; more than one only on a fingerprint collision,
	// which verify-on-hit (descriptorsFullyEqual, or effective-bounds plus declaration compare for
	// histograms) resolves so distinct descriptors are never silently merged. Lazily allocated.
	recordedConflict map[sameKeyConflictID][]*instrumentDescriptor
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
	for i := 0; i < 8; i++ {
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

type storeCore struct {
	mu sync.RWMutex

	sequence    uint64
	successSeq  uint64
	active      *cycleFrame
	instruments map[string]*instrumentDescriptor // metric name => descriptor (mode/kind locked)
	// instrumentZeroSince records, per name, the successful-commit seq at which the name
	// last went idle (no live series). The descriptor-universe sweep uses it to age idle
	// descriptors out of instruments after descriptorGraceCycles. Lazily allocated; a name
	// with a live series has no entry.
	instrumentZeroSince map[string]uint64
	retention           collectorRetentionPolicy

	snapshot atomic.Pointer[readSnapshot] // atomically swapped immutable read view
}

type collectorRetentionPolicy struct {
	expireAfterSuccessCycles uint64
	maxSeries                int
	// descriptorGraceCycles is how many successful commits a descriptor is kept in
	// instruments after its last series is evicted, before the descriptor-universe
	// sweep removes it. Defaults to expireAfterSuccessCycles (see NewCollectorStore).
	descriptorGraceCycles uint64
}

const (
	defaultCollectorExpireAfterSuccessCycles uint64 = 10
	defaultCollectorMaxSeries                       = 0 // disabled
)

type storeView struct {
	core *storeCore
}

type managedStore struct {
	core *storeCore
}

type storeCycleController struct {
	core *storeCore
}

// NewCollectorStore creates a collection store with staged writes and immutable read snapshots.
func NewCollectorStore(opts ...CollectorStoreOption) CollectorStore {
	cfg := collectorStoreConfig{
		expireAfterSuccessCycles: defaultCollectorExpireAfterSuccessCycles,
		maxSeries:                defaultCollectorMaxSeries,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(&cfg)
		}
	}
	// Grace defaults to the (possibly overridden) expire so the invariant grace >= expire
	// holds by construction; an explicit WithDescriptorGraceCycles decouples them.
	grace := cfg.expireAfterSuccessCycles
	if cfg.graceSet {
		grace = cfg.graceCycles
	}
	core := &storeCore{
		instruments: make(map[string]*instrumentDescriptor),
		retention: collectorRetentionPolicy{
			expireAfterSuccessCycles: cfg.expireAfterSuccessCycles,
			maxSeries:                cfg.maxSeries,
			descriptorGraceCycles:    grace,
		},
	}
	core.snapshot.Store(&readSnapshot{
		collectMeta: CollectMeta{LastAttemptStatus: CollectStatusUnknown},
		series:      make(map[string]*committedSeries),
		byName:      make(map[string][]*committedSeries),
	})
	return &storeView{core: core}
}

// AsCycleManagedStore exposes runtime cycle control for stores created by NewCollectorStore.
// This is intended for runtime internals, not collector code.
func AsCycleManagedStore(s CollectorStore) (CycleManagedStore, bool) {
	switch v := s.(type) {
	case *managedStore:
		return v, true
	case *storeView:
		return &managedStore{core: v.core}, true
	default:
		return nil, false
	}
}

func (s *storeView) Read(opts ...ReadOption) Reader {
	cfg := resolveReadConfig(opts...)
	snap := s.core.snapshot.Load()
	if cfg.flatten {
		snap = flattenSnapshot(snap)
	}
	return &storeReader{snap: snap, raw: cfg.raw, flattened: cfg.flatten, hostScopeKey: cfg.hostScopeKey}
}

func (s *storeView) Write() Writer {
	return &writeView{backend: s.core}
}

func (s *managedStore) Read(opts ...ReadOption) Reader {
	return (&storeView{core: s.core}).Read(opts...)
}

func (s *managedStore) Write() Writer {
	return (&storeView{core: s.core}).Write()
}

func (s *managedStore) CycleController() CycleController {
	return &storeCycleController{core: s.core}
}

// DescriptorRetention (optional interface) - both store facades delegate to storeCore.

func (s *storeView) DescriptorRetentionWindow() uint64 { return s.core.descriptorRetentionWindow() }
func (s *storeView) SuccessfulCommits() uint64         { return s.core.successfulCommits() }

func (s *managedStore) DescriptorRetentionWindow() uint64 { return s.core.descriptorRetentionWindow() }
func (s *managedStore) SuccessfulCommits() uint64         { return s.core.successfulCommits() }

// DescriptorRetentionUnbounded is the DescriptorRetentionWindow() value when series age-expiry
// is disabled (WithExpireAfterSuccessCycles(0)): a name's series - and thus its descriptor - can
// live indefinitely, so a consumer must retain its cached state for that name indefinitely too
// rather than aging it out on a finite window.
const DescriptorRetentionUnbounded uint64 = math.MaxUint64

// descriptorRetentionWindow is the number of successful commits a descriptor can outlive its
// last series: expire+grace. With age-expiry disabled (expire==0) a series - and thus its
// descriptor - can live indefinitely, so the window is unbounded; a grace so large that
// expire+grace overflows uint64 also saturates to unbounded. Both fields are fixed at
// construction, so no lock.
func (c *storeCore) descriptorRetentionWindow() uint64 {
	expire := c.retention.expireAfterSuccessCycles
	if expire == 0 {
		return DescriptorRetentionUnbounded
	}
	window := expire + c.retention.descriptorGraceCycles
	if window < expire { // uint64 overflow
		return DescriptorRetentionUnbounded
	}
	return window
}

// successfulCommits reads the success clock advanced under c.mu at commit.
func (c *storeCore) successfulCommits() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.successSeq
}

// BeginCycle opens a new staged frame for collection writes.
func (c *storeCycleController) BeginCycle() {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	if c.core.active != nil {
		panic(errCycleActive)
	}

	c.core.sequence++
	c.core.active = &cycleFrame{
		seq:                c.core.sequence,
		hostScopes:         make(map[string]HostScope),
		gauges:             make(map[string]*stagedGauge),
		counters:           make(map[string]*stagedCounter),
		histograms:         make(map[string]*stagedHistogram),
		summaries:          make(map[string]*stagedSummary),
		stateSet:           make(map[string]*stagedStateSet),
		measureSetGauges:   make(map[string]*stagedMeasureSet),
		measureSetCounters: make(map[string]*stagedMeasureSet),
		pendingInstruments: make(map[string][]*instrumentDescriptor),
	}
}

// abortWithError republishes the previously committed series as a failed attempt,
// clears the active cycle, and returns err. All staged writes and staged
// registrations from the aborted cycle are discarded (never merged).
func (c *storeCycleController) abortWithError(oldSnap *readSnapshot, err error) error {
	abortSnap := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      oldSnap.series,
		byName:      oldSnap.byName,
	}
	abortSnap.collectMeta.LastAttemptSeq = c.core.active.seq
	abortSnap.collectMeta.LastAttemptStatus = CollectStatusFailed
	c.core.snapshot.Store(abortSnap)
	c.core.active = nil
	return err
}

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

// dropStagedNames removes all staged writes and staged registrations for the given set
// of names from the frame in a single pass per staged map, so ambiguous multi-kind names
// are ignored for the cycle without failing the commit - O(touched), not O(dropped*touched).
func (f *cycleFrame) dropStagedNames(names map[string]struct{}) {
	in := func(name string) bool { _, ok := names[name]; return ok }
	maps.DeleteFunc(f.gauges, func(_ string, s *stagedGauge) bool { return in(s.name) })
	maps.DeleteFunc(f.counters, func(_ string, s *stagedCounter) bool { return in(s.name) })
	maps.DeleteFunc(f.histograms, func(_ string, s *stagedHistogram) bool { return in(s.name) })
	maps.DeleteFunc(f.summaries, func(_ string, s *stagedSummary) bool { return in(s.name) })
	maps.DeleteFunc(f.stateSet, func(_ string, s *stagedStateSet) bool { return in(s.name) })
	maps.DeleteFunc(f.measureSetGauges, func(_ string, s *stagedMeasureSet) bool { return in(s.name) })
	maps.DeleteFunc(f.measureSetCounters, func(_ string, s *stagedMeasureSet) bool { return in(s.name) })
	for name := range names {
		delete(f.pendingInstruments, name)
	}
}

// CommitCycleSuccess publishes staged writes into a new committed snapshot.
func (c *storeCycleController) CommitCycleSuccess() error {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	if c.core.active == nil {
		panic(errCycleMissing)
	}

	oldSnap := c.core.snapshot.Load()
	if c.core.active.err != nil {
		return c.abortWithError(oldSnap, c.core.active.err)
	}

	// Reconcile this cycle's observed descriptors against the committed registry
	// before publishing anything. Two incompatible kinds observed for one name in
	// the same cycle is unresolvable and fails the whole commit; because no committed
	// state has been mutated yet, that failure is transactional.
	resolution := c.core.resolveObservedDescriptors()
	if resolution.failErr != nil {
		return c.abortWithError(oldSnap, resolution.failErr)
	}
	successSeq := c.core.successSeq + 1
	next := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      make(map[string]*committedSeries, len(oldSnap.series)),
		byName:      nil,
	}
	commitHostScopes := make(map[string]HostScope)

	maps.Copy(next.series, oldSnap.series)

	// Apply supersessions: a name whose committed kind was not observed this cycle is
	// replaced by the newly observed kind. Drop its carried-forward series and its
	// committed descriptor so the staged loops create fresh series for the new kind
	// (rather than mutating a series that still carries the old descriptor). One pass over
	// next.series regardless of how many names supersede - O(live), not O(superseded*live).
	if len(resolution.supersede) > 0 {
		supersedeSet := make(map[string]struct{}, len(resolution.supersede))
		for _, name := range resolution.supersede {
			supersedeSet[name] = struct{}{}
			delete(c.core.instruments, name)
			// Keep instrumentZeroSince a subset of instruments: if the new kind's series does
			// not survive this commit (e.g. maxSeries-evicted before canonicalization), the
			// name never re-enters instruments, so the sweep would never revisit and clear a
			// leftover idle stamp.
			delete(c.core.instrumentZeroSince, name)
		}
		maps.DeleteFunc(next.series, func(_ string, series *committedSeries) bool {
			_, ok := supersedeSet[series.name]
			return ok
		})
	}

	// Drop ambiguous multi-kind names: remove their staged writes and staged registration
	// so they are ignored this cycle (prior committed series carry forward untouched). One
	// pass per staged map - O(touched), not O(dropped*touched).
	if len(resolution.drop) > 0 {
		dropSet := make(map[string]struct{}, len(resolution.drop))
		for _, name := range resolution.drop {
			dropSet[name] = struct{}{}
		}
		c.core.active.dropStagedNames(dropSet)
	}

	for key, staged := range c.core.active.gauges {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)
		series.value = staged.value
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.counters {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)

		hadCurrent := series.desc != nil && series.desc.kind == kindCounter && series.counterCurrentSeq > 0
		if hadCurrent {
			series.counterPrevious = series.counterCurrent
			series.counterPreviousSeq = series.counterCurrentSeq
			series.counterHasPrev = true
		} else {
			series.counterPrevious = 0
			series.counterPreviousSeq = 0
			series.counterHasPrev = false
		}

		series.counterCurrent = staged.current
		series.counterCurrentSeq = c.core.active.seq
		series.value = staged.current // Value() for counters returns current total.
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.histograms {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		// The canonical descriptor already carries concrete bounds (the resolver reduces a
		// nil-bounds observation to its observed bounds), and the resolver groups a name's
		// writes by effective bounds, so every staged write of an accepted name shares the
		// canonical bounds - no per-series schema capture or drift check is needed here.
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)
		series.histogramCount = staged.count
		series.histogramSum = staged.sum
		series.histogramCumulative = append(series.histogramCumulative[:0], staged.cumulative...)
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.summaries {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)

		if staged.desc.mode == modeStateful && len(staged.desc.summaryQuantiles()) > 0 {
			if staged.sketch != nil {
				staged.quantileValues = staged.sketch.quantiles(staged.desc.summaryQuantiles())
			} else {
				// Defensive fallback for malformed staged state.
				staged.quantileValues = nanSummaryQuantiles(staged.desc.summaryQuantiles())
			}
		}

		series.summaryCount = staged.count
		series.summarySum = staged.sum
		if len(staged.quantileValues) > 0 {
			series.summaryQuantiles = append(series.summaryQuantiles[:0], staged.quantileValues...)
		} else {
			series.summaryQuantiles = nil
		}
		if staged.sketch != nil && series.desc != nil && series.desc.window == WindowCumulative {
			series.summarySketch = staged.sketch.clone()
		} else {
			series.summarySketch = nil
		}
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.stateSet {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)

		series.stateSetValues = cloneStateMap(staged.states)
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.measureSetGauges {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)
		series.measureSetValues = append(series.measureSetValues[:0], staged.values...)
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.measureSetCounters {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)

		if series.desc != nil && series.desc.kind == kindMeasureSet && series.desc.measureSet != nil && series.desc.measureSet.semantics == MeasureSetSemanticsCounter && series.measureSetCurrentSeq > 0 {
			series.measureSetPreviousValues = append(series.measureSetPreviousValues[:0], series.measureSetValues...)
			series.measureSetPreviousSeq = series.measureSetCurrentSeq
			series.measureSetHasPrev = true
		} else {
			series.measureSetPreviousValues = nil
			series.measureSetPreviousSeq = 0
			series.measureSetHasPrev = false
		}

		series.measureSetValues = append(series.measureSetValues[:0], staged.values...)
		series.measureSetCurrentSeq = c.core.active.seq
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	refreshCommittedHostScopes(oldSnap, next, commitHostScopes)
	applyCollectorRetention(next.series, c.core.retention, successSeq)

	// Canonicalize the published snapshot in one pass, after retention: every surviving
	// series of a name observed this cycle gets the single canonical descriptor, and the
	// registry is set to match. This is the sole place descriptors are canonicalized, so
	// instruments[name], every series.desc (including carried, unobserved series of an
	// observed name), and the reader all agree regardless of label or iteration order.
	// Running after retention means an evicted-this-cycle name leaves no orphan entry.
	// Names with only carried series keep their committed entry; a defensive fill covers
	// any surviving series whose name somehow lacks one.
	// liveNames is collected here (folded into the existing scan) and consumed by the
	// descriptor-universe sweep below, so the sweep needs no separate pass over next.series.
	liveNames := make(map[string]struct{})
	for key := range next.series {
		series := next.series[key]
		liveNames[series.name] = struct{}{}
		canonical, ok := resolution.canonical[series.name]
		if !ok {
			if _, exists := c.core.instruments[series.name]; !exists {
				c.core.instruments[series.name] = series.desc
			}
			continue
		}
		// This pass scans every live series (O(live-series)), but only clones/rewrites a
		// series whose descriptor actually changes. With a no-op merge canonical == the
		// committed descriptor, so unchanged retained series are skipped: the clone/allocation
		// work stays O(touched), not O(retained).
		if series.desc != canonical {
			ensureCommitSeriesMutable(oldSnap, next, key).desc = canonical
		}
		if c.core.instruments[series.name] != canonical {
			c.core.instruments[series.name] = canonical
		}
	}

	// Bound descriptor growth: evict descriptors whose series are gone and whose grace has
	// elapsed. Runs after canonicalization so the live-name set is final.
	evicted := c.core.sweepDescriptorUniverse(liveNames, successSeq)

	next.collectMeta.LastAttemptSeq = c.core.active.seq
	next.collectMeta.LastAttemptStatus = CollectStatusSuccess
	next.collectMeta.LastSuccessSeq = c.core.active.seq
	next.collectMeta.EvictedDescriptors += evicted
	next.collectMeta.DroppedNames += uint64(len(resolution.drop))

	c.core.snapshot.Store(next)
	c.core.successSeq = successSeq
	c.core.active = nil
	return nil
}

func newCommittedSeries(key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	return &committedSeries{
		id:           SeriesID(key),
		hash64:       seriesIDHash(SeriesID(key)),
		key:          key,
		name:         name,
		hostScopeKey: hostScopeKey,
		hostScope:    cloneHostScope(hostScope),
		labels:       append([]Label(nil), labels...),
		labelsKey:    labelsKey,
		desc:         desc,
		meta:         baseSeriesMeta(desc),
	}
}

func ensureCommitSeriesMutable(old, next *readSnapshot, key string) *committedSeries {
	series := next.series[key]
	if series == nil {
		return nil
	}
	if oldSeries, ok := old.series[key]; ok && oldSeries == series {
		series = cloneCommittedSeries(series)
		next.series[key] = series
	}
	ensureSeriesMeta(series.desc, &series.meta)
	return series
}

func getOrCreateCommitSeries(old, next *readSnapshot, key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	series := ensureCommitSeriesMutable(old, next, key)
	if series != nil {
		return series
	}
	series = newCommittedSeries(key, name, hostScopeKey, hostScope, labels, labelsKey, desc)
	next.series[key] = series
	return series
}

func refreshCommittedHostScopes(old, next *readSnapshot, scopes map[string]HostScope) {
	if len(scopes) == 0 {
		return
	}
	for key, series := range next.series {
		scope, ok := scopes[series.hostScopeKey]
		if !ok || hostScopeEqual(series.hostScope, scope) {
			continue
		}
		series = ensureCommitSeriesMutable(old, next, key)
		if series == nil {
			continue
		}
		series.hostScope = cloneHostScope(scope)
	}
}

func markSeriesSeen(series *committedSeries, attemptSeq, successSeq uint64) {
	series.meta.LastSeenSuccessSeq = attemptSeq
	series.lastSeenSuccessCycle = successSeq
}

// AbortCycle discards staged writes and publishes metadata-only failed-attempt status.
func (c *storeCycleController) AbortCycle() {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	if c.core.active == nil {
		panic(errCycleMissing)
	}

	oldSnap := c.core.snapshot.Load()
	// Alias previous committed maps directly. Safe by invariant:
	// committed series/snapshots are immutable after publish.
	abortSnap := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      oldSnap.series,
		byName:      oldSnap.byName,
	}

	abortSnap.collectMeta.LastAttemptSeq = c.core.active.seq
	abortSnap.collectMeta.LastAttemptStatus = CollectStatusFailed
	c.core.snapshot.Store(abortSnap)

	c.core.active = nil
}

// buildByName builds deterministic per-name iteration lists for snapshot readers.
func buildByName(series map[string]*committedSeries) map[string][]*committedSeries {
	byName := make(map[string][]*committedSeries)
	for _, s := range series {
		if s.desc == nil || !isScalarKind(s.desc.kind) {
			continue
		}
		byName[s.name] = append(byName[s.name], s)
	}
	for _, lst := range byName {
		sort.Slice(lst, func(i, j int) bool {
			if lst[i].hostScopeKey != lst[j].hostScopeKey {
				return lst[i].hostScopeKey < lst[j].hostScopeKey
			}
			return lst[i].labelsKey < lst[j].labelsKey
		})
	}
	return byName
}

func applyCollectorRetention(series map[string]*committedSeries, policy collectorRetentionPolicy, successSeq uint64) {
	if policy.expireAfterSuccessCycles > 0 {
		for key, s := range series {
			seen := s.lastSeenSuccessCycle
			if seen == 0 || successSeq < seen {
				continue
			}
			if successSeq-seen >= policy.expireAfterSuccessCycles {
				delete(series, key)
			}
		}
	}

	evictOldestSeries(series, policy.maxSeries, func(s *committedSeries) uint64 {
		return s.lastSeenSuccessCycle
	}, nil)
}

// sweepDescriptorUniverse bounds instruments growth. The descriptor universe is exactly
// the names in instruments (post-canonicalization every live name has an entry, and idle
// names linger there until swept). For each name:
//   - a live series clears any idle stamp;
//   - an idle name is stamped with successSeq the first cycle it goes idle, then evicted
//     once it has been idle for descriptorGraceCycles successful commits.
//
// It runs under c.mu at commit, after retention and canonicalization. Cost is O(universe) -
// one pass over instruments, within the commit envelope. Returns the number of descriptors
// evicted this cycle. instrumentZeroSince is lazily allocated (a store that never idles a
// name never allocates it). Deleting the current key while ranging a map is defined in Go.
func (c *storeCore) sweepDescriptorUniverse(liveNames map[string]struct{}, successSeq uint64) uint64 {
	grace := c.retention.descriptorGraceCycles
	var evicted uint64
	for name := range c.instruments {
		if _, live := liveNames[name]; live {
			delete(c.instrumentZeroSince, name) // no-op on a nil map
			continue
		}
		since, stamped := c.instrumentZeroSince[name]
		if !stamped {
			if c.instrumentZeroSince == nil {
				c.instrumentZeroSince = make(map[string]uint64)
			}
			c.instrumentZeroSince[name] = successSeq
			since = successSeq
		}
		if successSeq-since >= grace {
			delete(c.instruments, name)
			delete(c.instrumentZeroSince, name)
			evicted++
		}
	}
	return evicted
}

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

func (c *storeCore) prepareHostScopeForWriteLocked(scope HostScope) (HostScope, bool) {
	scope = mustNormalizeHostScope(scope)
	if c.active == nil {
		return scope, true
	}
	if existing, ok := c.active.hostScopes[scope.ScopeKey]; ok {
		if hostScopeEqual(existing, scope) {
			return existing, true
		}
		c.recordCycleErrorLocked(fmt.Errorf("%w: scope_key=%q", ErrHostScopeConflict, scope.ScopeKey))
		return scope, false
	}
	c.active.hostScopes[scope.ScopeKey] = cloneHostScope(scope)
	return scope, true
}

func (c *storeCore) recordCycleErrorLocked(err error) {
	if err == nil || c.active == nil {
		return
	}
	c.active.err = errors.Join(c.active.err, err)
}

// makeSeriesKey joins host scope, metric name, and canonical label key into one stable identity key.
func makeSeriesKey(hostScopeKey, name, labelsKey string) string {
	base := name
	if labelsKey == "" {
		base = name
	} else {
		base = name + "\xfe" + labelsKey
	}
	if hostScopeKey == "" {
		return base
	}
	return hostScopeKey + "\xff" + base
}

func cloneCommittedSeries(s *committedSeries) *committedSeries {
	cp := *s
	ensureSeriesMeta(cp.desc, &cp.meta)
	cp.hostScope = cloneHostScope(s.hostScope)
	// cp.labels intentionally reuses the original immutable label slice.
	// Label identity is part of the series key and is never mutated after publish.
	if s.stateSetValues != nil {
		cp.stateSetValues = cloneStateMap(s.stateSetValues)
	}
	if len(s.measureSetValues) > 0 {
		cp.measureSetValues = append([]SampleValue(nil), s.measureSetValues...)
	}
	if len(s.measureSetPreviousValues) > 0 {
		cp.measureSetPreviousValues = append([]SampleValue(nil), s.measureSetPreviousValues...)
	}
	if len(s.histogramCumulative) > 0 {
		cp.histogramCumulative = append([]SampleValue(nil), s.histogramCumulative...)
	}
	if len(s.summaryQuantiles) > 0 {
		cp.summaryQuantiles = append([]SampleValue(nil), s.summaryQuantiles...)
	}
	if s.summarySketch != nil {
		cp.summarySketch = s.summarySketch.clone()
	}
	return &cp
}

func cloneInstrumentDescriptorWithHistogram(desc *instrumentDescriptor, bounds []float64) *instrumentDescriptor {
	cp := *desc
	cp.histogram = &histogramSchema{bounds: append([]float64(nil), bounds...)}
	return &cp
}

func cloneStateMap(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	maps.Copy(out, in)
	return out
}

func isScalarKind(kind metricKind) bool {
	return kind == kindGauge || kind == kindCounter
}

func buildHistogramSchema(cfg instrumentConfig, mode metricMode) (*histogramSchema, error) {
	bounds, err := normalizeHistogramBounds(cfg.histogramBounds)
	if err != nil {
		return nil, err
	}
	if mode == modeStateful && len(bounds) == 0 {
		return nil, fmt.Errorf("%w for stateful histogram", errHistogramBounds)
	}
	if len(bounds) == 0 {
		return nil, nil
	}
	return &histogramSchema{bounds: bounds}, nil
}

func buildSummarySchema(cfg instrumentConfig) (*summarySchema, error) {
	if cfg.summaryReservoirSet && cfg.summaryReservoir <= 0 {
		return nil, fmt.Errorf("metrix: summary reservoir size must be > 0")
	}

	qs, err := normalizeSummaryQuantiles(cfg.summaryQuantile)
	if err != nil {
		return nil, err
	}

	if len(qs) == 0 {
		return nil, nil
	}

	size := defaultSummaryReservoirSize
	if cfg.summaryReservoirSet {
		size = cfg.summaryReservoir
	}

	return &summarySchema{
		quantiles:     qs,
		reservoirSize: size,
	}, nil
}

func buildStateSetSchema(cfg instrumentConfig) (*stateSetSchema, error) {
	if len(cfg.states) == 0 {
		return nil, fmt.Errorf("metrix: stateset requires WithStateSetStates")
	}

	mode := ModeBitSet
	if cfg.stateSetMode != nil {
		mode = *cfg.stateSetMode
	}

	seen := make(map[string]struct{}, len(cfg.states))
	states := make([]string, 0, len(cfg.states))
	for _, st := range cfg.states {
		if st == "" {
			return nil, fmt.Errorf("metrix: stateset state cannot be empty")
		}
		if _, ok := seen[st]; ok {
			return nil, fmt.Errorf("metrix: duplicate stateset state %q", st)
		}
		seen[st] = struct{}{}
		states = append(states, st)
	}

	return &stateSetSchema{
		mode:   mode,
		states: states,
		index:  seen,
	}, nil
}

func equalStateSetSchema(a, b *stateSetSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.mode != b.mode || len(a.states) != len(b.states) {
		return false
	}
	for i := range a.states {
		if a.states[i] != b.states[i] {
			return false
		}
	}
	return true
}

func buildMeasureSetSchema(cfg instrumentConfig) (*measureSetSchema, error) {
	if len(cfg.measureSetFields) == 0 {
		return nil, fmt.Errorf("metrix: measureset requires WithMeasureSetFields")
	}
	if cfg.measureSetSemantics == nil {
		return nil, fmt.Errorf("metrix: measureset semantics are missing")
	}

	fields := make([]MeasureFieldSpec, 0, len(cfg.measureSetFields))
	index := make(map[string]int, len(cfg.measureSetFields))
	for i, field := range cfg.measureSetFields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			return nil, fmt.Errorf("metrix: measureset field name cannot be empty")
		}
		if _, ok := index[name]; ok {
			return nil, fmt.Errorf("metrix: duplicate measureset field %q", name)
		}
		field.Name = name
		fields = append(fields, field)
		index[name] = i
	}

	return &measureSetSchema{
		semantics: *cfg.measureSetSemantics,
		fields:    fields,
		index:     index,
	}, nil
}

func equalMeasureSetSchema(a, b *measureSetSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.semantics != b.semantics || len(a.fields) != len(b.fields) {
		return false
	}
	for i := range a.fields {
		if a.fields[i].Name != b.fields[i].Name || a.fields[i].Float != b.fields[i].Float {
			return false
		}
	}
	return true
}

func equalHistogramSchema(a, b *histogramSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	return equalHistogramBounds(a.bounds, b.bounds)
}

func equalSummarySchema(a, b *summarySchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.reservoirSize != b.reservoirSize || len(a.quantiles) != len(b.quantiles) {
		return false
	}
	for i := range a.quantiles {
		if a.quantiles[i] != b.quantiles[i] {
			return false
		}
	}
	return true
}

func equalHistogramBounds(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeHistogramBounds(in []float64) ([]float64, error) {
	if len(in) == 0 {
		return nil, nil
	}

	bounds := append([]float64(nil), in...)
	out := make([]float64, 0, len(bounds))
	prev := math.Inf(-1)
	for i, b := range bounds {
		if math.IsNaN(b) || math.IsInf(b, -1) {
			return nil, fmt.Errorf("%w: invalid upper bound", errHistogramPoint)
		}
		if math.IsInf(b, +1) {
			if i != len(bounds)-1 {
				return nil, fmt.Errorf("%w: +Inf bucket must be last", errHistogramPoint)
			}
			break // +Inf is implicit.
		}
		if b <= prev {
			return nil, fmt.Errorf("%w: bounds must be strictly increasing", errHistogramPoint)
		}
		out = append(out, b)
		prev = b
	}
	return out, nil
}

func normalizeSummaryQuantiles(in []float64) ([]float64, error) {
	if len(in) == 0 {
		return nil, nil
	}

	qs := append([]float64(nil), in...)
	prev := -1.0
	for _, q := range qs {
		if math.IsNaN(q) || q < 0 || q > 1 {
			return nil, fmt.Errorf("metrix: invalid summary quantile %v", q)
		}
		if q <= prev {
			return nil, fmt.Errorf("metrix: summary quantiles must be strictly increasing")
		}
		prev = q
	}
	return qs, nil
}

func isWindowAllowed(kind metricKind, mode metricMode) bool {
	return mode == modeStateful && (kind == kindHistogram || kind == kindSummary)
}
