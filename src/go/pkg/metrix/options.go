// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

type InstrumentOption interface {
	apply(*instrumentConfig)
}

type optionFunc func(*instrumentConfig)

func (f optionFunc) apply(cfg *instrumentConfig) { f(cfg) }

// CollectorStoreOption configures a collector store at construction time.
type CollectorStoreOption interface {
	apply(*collectorStoreConfig)
}

type collectorStoreConfig struct {
	expireAfterSuccessCycles uint64
	maxSeries                int
	graceCycles              uint64
	graceSet                 bool
}

type collectorStoreOptionFunc func(*collectorStoreConfig)

func (f collectorStoreOptionFunc) apply(cfg *collectorStoreConfig) { f(cfg) }

// WithExpireAfterSuccessCycles sets how many successful commits an unobserved series
// is retained before eviction (0 disables series expiry).
func WithExpireAfterSuccessCycles(cycles uint64) CollectorStoreOption {
	return collectorStoreOptionFunc(func(cfg *collectorStoreConfig) {
		cfg.expireAfterSuccessCycles = cycles
	})
}

// WithMaxSeries caps the number of committed series; the oldest are evicted past the
// cap (0 disables the cap).
func WithMaxSeries(max int) CollectorStoreOption {
	return collectorStoreOptionFunc(func(cfg *collectorStoreConfig) {
		cfg.maxSeries = max
	})
}

// WithDescriptorGraceCycles sets how many successful commits a descriptor is kept after
// its last series is evicted, before the descriptor itself is swept. It defaults to
// expireAfterSuccessCycles, so the effective descriptor lifetime after a name goes idle
// is expire+grace; set it explicitly (including 0) to decouple the two.
func WithDescriptorGraceCycles(cycles uint64) CollectorStoreOption {
	return collectorStoreOptionFunc(func(cfg *collectorStoreConfig) {
		cfg.graceCycles = cycles
		cfg.graceSet = true
	})
}

type instrumentConfig struct {
	freshnessSet bool
	freshness    FreshnessPolicy

	windowSet bool
	window    MetricWindow

	histogramBounds     []float64
	summaryQuantile     []float64
	summaryReservoirSet bool
	summaryReservoir    int
	states              []string
	stateSetMode        *StateSetMode
	measureSetFields    []MeasureFieldSpec
	measureSetSemantics *MeasureSetSemantics

	descriptionSet   bool
	description      string
	chartFamilySet   bool
	chartFamily      string
	chartPrioritySet bool
	chartPriority    int
	unitSet          bool
	unit             string
	floatSet         bool
	float            bool
}

func WithFreshness(policy FreshnessPolicy) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.freshnessSet = true
		cfg.freshness = policy
	})
}

func WithWindow(w MetricWindow) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.windowSet = true
		cfg.window = w
	})
}

func WithHistogramBounds(bounds ...float64) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.histogramBounds = append([]float64(nil), bounds...)
	})
}

func WithSummaryQuantiles(qs ...float64) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.summaryQuantile = append([]float64(nil), qs...)
	})
}

// WithSummaryReservoirSize sets the bounded sample size used for stateful summary quantile estimation.
// Valid only for stateful summaries.
func WithSummaryReservoirSize(size int) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.summaryReservoirSet = true
		cfg.summaryReservoir = size
	})
}

func WithStateSetStates(states ...string) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.states = append([]string(nil), states...)
	})
}

func WithStateSetMode(mode StateSetMode) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		m := mode
		cfg.stateSetMode = &m
	})
}

func WithMeasureSetFields(fields ...MeasureFieldSpec) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.measureSetFields = append([]MeasureFieldSpec(nil), fields...)
	})
}

func withMeasureSetSemantics(semantics MeasureSetSemantics) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		s := semantics
		cfg.measureSetSemantics = &s
	})
}

// WithDescription sets optional metric-family description metadata.
func WithDescription(description string) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.descriptionSet = true
		cfg.description = description
	})
}

// WithChartFamily sets optional metric-family chart grouping metadata.
func WithChartFamily(chartFamily string) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.chartFamilySet = true
		cfg.chartFamily = chartFamily
	})
}

// WithChartPriority sets optional metric-family chart priority metadata.
// It is currently consumed only by chartengine autogen.
// TODO: Revisit whether chart-template charts should also honor metrix priority hints.
func WithChartPriority(chartPriority int) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.chartPrioritySet = true
		cfg.chartPriority = chartPriority
	})
}

// WithUnit sets optional metric-family unit metadata.
func WithUnit(unit string) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.unitSet = true
		cfg.unit = unit
	})
}

// WithFloat sets optional metric-family float-dimension metadata hint.
func WithFloat(isFloat bool) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.floatSet = true
		cfg.float = isFloat
	})
}
