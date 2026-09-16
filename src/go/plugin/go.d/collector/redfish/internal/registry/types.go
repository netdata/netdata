// SPDX-License-Identifier: GPL-3.0-or-later

// Package registry is the closed executable Redfish monitoring contract.
//
// It intentionally depends only on the standard library. Runtime collection,
// Function presentation, chart generation, and alert generation consume the
// same compiled declarations instead of maintaining parallel semantic tables.
package registry

type (
	Kind       string
	Document   string
	Algorithm  string
	MetricKind string
	ChartType  string
	ChartClass string
	Exposure   string
)

const (
	AlgorithmAbsolute        Algorithm = "absolute"
	AlgorithmRate            Algorithm = "collector_rate"
	AlgorithmDurationPercent Algorithm = "collector_duration_percent"
	AlgorithmStateSet        Algorithm = "stateset"
	AlgorithmFlags           Algorithm = "flags"
	AlgorithmInventory       Algorithm = "inventory"
)

const (
	ExposureInventoryOnly      Exposure = "inventory_only"
	ExposureOperationalScalar  Exposure = "operational_scalar"
	ExposureOperationalReading Exposure = "operational_reading"
)

const (
	MetricGauge    MetricKind = "gauge"
	MetricStateSet MetricKind = "stateset"
	MetricFixedSet MetricKind = "fixed_set"
)

const (
	ChartLine    ChartType = "line"
	ChartStacked ChartType = "stacked"
	ChartHeatmap ChartType = "heatmap"
)

const (
	ClassOperational         ChartClass = "operational"
	ClassResourceScalar      ChartClass = "resource_scalar"
	ClassResourceCategorical ChartClass = "resource_categorical"
	ClassReadingScalar       ChartClass = "reading_scalar"
	ClassReadingAuxiliary    ChartClass = "reading_auxiliary"
	ClassReadingAlarm        ChartClass = "reading_alarm"
	ClassNumericParent       ChartClass = "numeric_parent"
	ClassCategoricalParent   ChartClass = "categorical_parent"
	ClassHistogramParent     ChartClass = "histogram_parent"
)

type Rational struct {
	Num int64
	Den int64
}

var Identity = Rational{Num: 1, Den: 1}

type KindSpec struct {
	ID                     Kind
	Display                string
	TopFamily              string
	LeafFamily             string
	ComponentFamily        string
	ComponentClass         string
	ParentPresentationRank int
}

type RelationshipMode string

const (
	RelationshipComponent   RelationshipMode = "component"
	RelationshipEnrichment  RelationshipMode = "enrichment"
	RelationshipTraversal   RelationshipMode = "traversal"
	RelationshipAssociation RelationshipMode = "association"
	RelationshipLegacy      RelationshipMode = "legacy"
)

type RelationshipSpec struct {
	Order       int
	Parent      Kind
	Path        string
	Child       Kind
	Family      string
	Mode        RelationshipMode
	Embedded    bool
	SourceModel string
	RollupRank  int
}

type SourceCandidate struct {
	Document Document
	Path     string
	Unit     string
	Scale    Rational
	Requires []SourceRequirement

	// MultiplierPath names a sibling value that must be present and positive.
	// It is used only where the Redfish schema defines a total in blocks and a
	// current block size in the same document.
	MultiplierDocument Document
	MultiplierPath     string
	MultiplierScale    Rational
	MultiplierColumn   string
}

type SourceRequirement struct {
	Path  string
	Value string
}

type FieldSpec struct {
	Order            int
	ID               string
	Kind             Kind
	Candidates       []SourceCandidate
	EquivalenceProof string

	Metric           string
	Context          string
	Role             string
	Column           string
	Title            string
	Units            string
	Scale            Rational
	Algorithm        Algorithm
	Float            bool
	MixedColumnUnits bool
	Exposure         Exposure

	Additive       bool
	Histogram      string
	AggregateClass string
	AggregateKinds []Kind
	ComponentClass string
}

type StateSpec struct {
	Order          int
	Kind           Kind
	Document       Document
	Path           string
	Metric         string
	Context        string
	Title          string
	Column         string
	States         []string
	BooleanFalse   string
	BooleanTrue    string
	AggregateKinds []Kind
	ComponentClass string
}

type FlagMemberSpec struct {
	Path   string
	Role   string
	Column string
	Invert bool
}

type FlagSetSpec struct {
	Order          int
	Kind           Kind
	Document       Document
	Metric         string
	Context        string
	Title          string
	Members        []FlagMemberSpec
	AggregateKinds []Kind
	ComponentClass string
}

type StatusSpec struct {
	Kind             Kind
	Status           bool
	PowerState       bool
	FailurePredicted bool
	AggregateKinds   []Kind
}

type DimensionSpec struct {
	ID        string
	Name      string
	Metric    string
	Selector  string
	Algorithm string
	Float     bool
}

type ChartSpec struct {
	Order          int
	Module         string
	ID             string
	Context        string
	BaseRowContext string
	ScalarRoleRank int
	Title          string
	Units          string
	Type           ChartType
	Class          ChartClass
	TopFamily      string
	LeafFamily     string
	InstanceLabels []string
	PromotedLabels []string
	Dimensions     []DimensionSpec
	Priority       int
	ExpireAfter    int
}

type ColumnType string

const (
	ColumnString    ColumnType = "string"
	ColumnEnum      ColumnType = "enum"
	ColumnInteger   ColumnType = "integer"
	ColumnFloat     ColumnType = "float"
	ColumnBoolean   ColumnType = "boolean"
	ColumnTimestamp ColumnType = "timestamp"
)

type ColumnSpec struct {
	Order      int
	ID         string
	Name       string
	Tooltip    string
	Type       ColumnType
	Units      string
	Scale      Rational
	Visible    bool
	Facet      bool
	Sortable   bool
	Structured bool
	Sticky     bool
	Unique     bool
	Additive   bool
	Members    map[string]struct{}
}

type InventoryFieldSpec struct {
	Order      int
	Kind       Kind
	Expression string
	Path       string
	Column     string
	SourceType ColumnType
	Type       ColumnType
	Units      string
	Scale      Rational
	Visible    bool
	Facet      bool
	Structured bool
}

type OperationalSpec struct {
	Order          int
	TopFamily      string
	LeafFamily     string
	InstanceLabels []string
	Metric         string
	Context        string
	Title          string
	Units          string
	Type           ChartType
	Incremental    bool
	Dimensions     []string
	Float          bool
}

type ReadingTypeSpec struct {
	Order       int
	SourceType  string
	SourceUnits []string
	Family      string
	Units       string
	Scale       Rational
}

type ReadingRoleSpec struct {
	Order    int
	ID       string
	Exposure Exposure
	Primary  bool
}

type ReadingSurfaceSpec struct {
	Order             int
	Family            string
	Basis             string
	Role              string
	SemanticClass     string
	Metric            string
	Context           string
	Title             string
	Units             string
	TopFamily         string
	LeafFamily        string
	Histogram         string
	Exposure          Exposure
	Primary           bool
	AlarmMetric       string
	AlarmContext      string
	AggregateMetric   string
	AggregateClass    string
	AggregateKinds    []Kind
	ComponentClass    string
	CommonContext     bool
	DerivedFromEnergy bool
}

type HistogramSpec struct {
	ID      string
	Buckets []HistogramBucket
}

type HistogramBucket struct {
	ID             string
	UpperExclusive *float64
	UpperInclusive bool
}

type Contract struct {
	Kinds          []KindSpec
	Relationships  []RelationshipSpec
	Fields         []FieldSpec
	Status         []StatusSpec
	States         []StateSpec
	Flags          []FlagSetSpec
	Columns        []ColumnSpec
	Inventory      []InventoryFieldSpec
	Operational    []OperationalSpec
	ReadingTypes   []ReadingTypeSpec
	ReadingRoles   []ReadingRoleSpec
	Readings       []ReadingSurfaceSpec
	Histograms     []HistogramSpec
	SummaryClasses []SummaryClassSpec
	Charts         []ChartSpec
}

type SummaryClassSpec struct {
	ID        string
	Title     string
	Units     string
	Additive  bool
	Histogram string
}
