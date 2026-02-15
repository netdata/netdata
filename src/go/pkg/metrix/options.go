// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

type InstrumentOption interface {
	apply(*instrumentConfig)
}

type optionFunc func(*instrumentConfig)

func (f optionFunc) apply(cfg *instrumentConfig) { f(cfg) }

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

	descriptionSet bool
	description    string
	chartFamilySet bool
	chartFamily    string
	unitSet        bool
	unit           string
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

// WithUnit sets optional metric-family unit metadata.
func WithUnit(unit string) InstrumentOption {
	return optionFunc(func(cfg *instrumentConfig) {
		cfg.unitSet = true
		cfg.unit = unit
	})
}
