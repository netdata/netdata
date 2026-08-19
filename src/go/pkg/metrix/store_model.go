// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"sync"
	"sync/atomic"
)

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
	// counterNoResetFallback makes Delta() unavailable on decreases for
	// counter-shaped series where a decrease can be a valid signed observation,
	// not necessarily a reset.
	counterNoResetFallback bool

	// Histogram current sample (used by Histogram()).
	histogramCount      SampleValue
	histogramSum        SampleValue
	histogramCumulative []SampleValue
	// Histogram two-sample state (used by flattened histogram Delta()).
	histogramPreviousCount      SampleValue
	histogramPreviousSum        SampleValue
	histogramPreviousCumulative []SampleValue
	histogramHasPrev            bool
	histogramCurrentSeq         uint64
	histogramPreviousSeq        uint64

	// Summary current sample (used by Summary()).
	summaryCount     SampleValue
	summarySum       SampleValue
	summaryQuantiles []SampleValue
	summarySketch    *summaryQuantileSketch // cumulative stateful quantile estimator
	// Summary two-sample state (used by flattened summary Delta()).
	summaryPreviousCount SampleValue
	summaryPreviousSum   SampleValue
	summaryHasPrev       bool
	summaryCurrentSeq    uint64
	summaryPreviousSeq   uint64

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
	series      map[string]*committedSeries // key => series
	// index is non-nil when a complete immutable index was built with this
	// snapshot. Collector canonical snapshots build it lazily through collector.
	index     *snapshotSeriesIndex
	collector *collectorSnapshotState
	// runtimeBase links runtime snapshots in overlay mode (nil for materialized snapshots).
	runtimeBase *readSnapshot
	// runtimeDepth tracks overlay chain depth for runtime compaction heuristics.
	runtimeDepth int
}

type collectorSnapshotState struct {
	index     retryableLazyPointer[snapshotSeriesIndex]
	flattened retryableLazyPointer[readSnapshot]
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
