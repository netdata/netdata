package framework

import (
	"time"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Context represents a metric collection context with compile-time type safety
type Context[T any] struct {
	Name       string           // Full context name (e.g., "mq_pcf.queue.depth")
	Family     string           // Chart family
	Title      string           // Human-readable title
	Units      string           // Units of measurement
	Type       module.ChartType // Chart type (Line, Area, Stacked)
	Priority   int              // Chart priority
	UpdateEvery int             // Minimum collection interval (seconds)
	Dimensions []Dimension      // Dimension definitions
	LabelKeys  []string         // Label keys in order (empty for unlabeled contexts)
}

// Dimension represents a single metric dimension
type Dimension struct {
	Name      string          // Dimension name
	Algorithm module.DimAlgo  // Algorithm (Absolute, Incremental, etc.)
	Mul       int             // Multiplier for unit conversion
	Div       int             // Divider for unit conversion
	Precision int             // Precision multiplier
}

// Instance represents a unique instance of a context with labels
type Instance struct {
	key         string
	contextName string
	labels      map[string]string
	lastSeen    time.Time
}

// MetricValue holds a single metric value
type MetricValue struct {
	Instance  Instance
	Dimension string
	Value     int64
	Timestamp time.Time
}

// CollectorState provides metric collection capabilities
type CollectorState struct {
	instances         map[string]*Instance
	obsoleteInstances []string // Track instances that became obsolete
	metrics           []MetricValue
	iteration         int64    // Global iteration counter (incremented every CollectOnce)
	errors            map[string]error
	protocols         map[string]*ProtocolMetrics
}

// ProtocolMetrics tracks protocol-level observability
type ProtocolMetrics struct {
	Name         string
	RequestCount int64
	ErrorCount   int64
	TotalLatency int64
	MaxLatency   int64
	BytesSent    int64
	BytesReceived int64
}

// ErrorType classifies errors for retry logic
type ErrorType int

const (
	ErrorTemporary ErrorType = iota // Retry with backoff
	ErrorFatal                      // Don't retry
	ErrorAuth                       // Authentication failure
)

// CollectorError provides error classification
type CollectorError struct {
	Err  error
	Type ErrorType
}

func (e CollectorError) Error() string {
	return e.Err.Error()
}

// CollectorImpl is the interface that specific collectors must implement
type CollectorImpl interface {
	CollectOnce() error
}

// Config holds framework configuration
type Config struct {
	ObsoletionIterations int               `yaml:"obsoletion_iterations,omitempty" json:"obsoletion_iterations"`  // Number of iterations before marking obsolete (default: 60)
	UpdateEvery          int               `yaml:"update_every,omitempty" json:"update_every"`                   // Base collection interval in seconds
	CollectionGroups     map[string]int    `yaml:"collection_groups,omitempty" json:"collection_groups"`         // Named groups with custom intervals
}

// ContextMetadata is a non-generic version of Context for runtime reflection
type ContextMetadata struct {
	Name       string              // Full context name (e.g., "mq_pcf.queue.depth")
	Family     string              // Chart family
	Title      string              // Human-readable title
	Units      string              // Units of measurement
	Type       module.ChartType    // Chart type (Line, Area, Stacked)
	Priority   int                 // Chart priority
	UpdateEvery int                // Minimum collection interval (seconds)
	Dimensions []Dimension         // Dimension definitions
	HasLabels  bool                // Whether this context has labels
	LabelOrder []string            // Order of labels from YAML definition
}