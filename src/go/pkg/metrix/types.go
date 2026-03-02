// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// Labels is a lookup input map used for reader queries.
// The store canonicalizes it internally for identity matching.
type Labels map[string]string

// SampleValue is the phase-2 scalar value type.
type SampleValue = float64

// Label is a single key/value pair.
type Label struct {
	Key   string
	Value string
}

// LabelSet is an immutable compiled label handle owned by one store.
type LabelSet struct {
	set *compiledLabelSet
}

// SeriesID identifies one metric series in canonical storage.
type SeriesID string

// SeriesIdentity is the stable per-series identity handle used by readers/engines.
type SeriesIdentity struct {
	ID     SeriesID
	Hash64 uint64
}

type CollectStatus uint8

const (
	CollectStatusUnknown CollectStatus = iota
	CollectStatusSuccess
	CollectStatusFailed
)

type SeriesMeta struct {
	// LastSeenSuccessSeq is the collect/runtime sequence of last successful write.
	LastSeenSuccessSeq uint64
	// Kind is the exposed kind in current reader view (e.g. flattened scalar kind).
	Kind MetricKind
	// SourceKind is the original family kind before flattening.
	SourceKind MetricKind
	// FlattenRole identifies synthetic component semantics in flattened views.
	FlattenRole FlattenRole
}

// MetricMeta stores optional family-level metadata for one metric name.
// It is used by downstream consumers (e.g. chart autogen) as hints.
type MetricMeta struct {
	Description string
	ChartFamily string
	Unit        string
	Float       bool
}

// MetricKind identifies the logical metric family type.
// For flattened views, Kind can differ from SourceKind.
type MetricKind uint8

const (
	MetricKindUnknown MetricKind = iota
	MetricKindGauge
	MetricKindCounter
	MetricKindHistogram
	MetricKindSummary
	MetricKindStateSet
)

// FlattenRole describes synthetic scalar roles produced by Read(ReadFlatten()).
type FlattenRole uint8

const (
	FlattenRoleNone FlattenRole = iota
	FlattenRoleHistogramBucket
	FlattenRoleHistogramCount
	FlattenRoleHistogramSum
	FlattenRoleSummaryCount
	FlattenRoleSummarySum
	FlattenRoleSummaryQuantile
	FlattenRoleStateSetState
)

type CollectMeta struct {
	LastAttemptSeq    uint64
	LastAttemptStatus CollectStatus
	LastSuccessSeq    uint64
}

type QuantilePoint struct {
	Quantile float64
	Value    SampleValue
}

type SummaryPoint struct {
	Count     SampleValue
	Sum       SampleValue
	Quantiles []QuantilePoint
}

type BucketPoint struct {
	UpperBound      float64
	CumulativeCount SampleValue
}

type HistogramPoint struct {
	Count   SampleValue
	Sum     SampleValue
	Buckets []BucketPoint
}

type StateSetPoint struct {
	States map[string]bool
}

type StateSetMode int

const (
	ModeBitSet StateSetMode = iota
	ModeEnum
)

type FreshnessPolicy uint8

const (
	FreshnessCycle FreshnessPolicy = iota
	FreshnessCommitted
)

type MetricWindow uint8

const (
	WindowCumulative MetricWindow = iota
	WindowCycle
)

type LabelView interface {
	Len() int
	Get(key string) (string, bool)
	Range(fn func(key, value string) bool)
	CloneMap() map[string]string
}

// FamilyView is a reusable snapshot-bound view over one metric name.
type FamilyView interface {
	Name() string
	ForEach(fn func(labels LabelView, v SampleValue))
}
