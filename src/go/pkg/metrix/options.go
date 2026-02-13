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

	histogramBounds []float64
	summaryQuantile []float64
	states          []string
	stateSetMode    *StateSetMode
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
