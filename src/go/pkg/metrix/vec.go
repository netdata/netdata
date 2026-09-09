// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// snapshotGaugeVec caches snapshot gauge series handles by vec label values.
type snapshotGaugeVec struct {
	cache *vecCache[*snapshotGaugeInstrument]
	scope HostScope
}

// statefulGaugeVec caches stateful gauge series handles by vec label values.
type statefulGaugeVec struct {
	cache *vecCache[*statefulGaugeInstrument]
	scope HostScope
}

// snapshotCounterVec caches snapshot counter series handles by vec label values.
type snapshotCounterVec struct {
	cache *vecCache[*snapshotCounterInstrument]
	scope HostScope
}

// statefulCounterVec caches stateful counter series handles by vec label values.
type statefulCounterVec struct {
	cache *vecCache[*statefulCounterInstrument]
	scope HostScope
}

// snapshotHistogramVec caches snapshot histogram series handles by vec label values.
type snapshotHistogramVec struct {
	cache *vecCache[*snapshotHistogramInstrument]
	scope HostScope
}

// statefulHistogramVec caches stateful histogram series handles by vec label values.
type statefulHistogramVec struct {
	cache *vecCache[*statefulHistogramInstrument]
	scope HostScope
}

// snapshotSummaryVec caches snapshot summary series handles by vec label values.
type snapshotSummaryVec struct {
	cache *vecCache[*snapshotSummaryInstrument]
	scope HostScope
}

// statefulSummaryVec caches stateful summary series handles by vec label values.
type statefulSummaryVec struct {
	cache *vecCache[*statefulSummaryInstrument]
	scope HostScope
}

// snapshotStateSetVec caches snapshot stateset series handles by vec label values.
type snapshotStateSetVec struct {
	cache *vecCache[*snapshotStateSetInstrument]
	scope HostScope
}

// statefulStateSetVec caches stateful stateset series handles by vec label values.
type statefulStateSetVec struct {
	cache *vecCache[*statefulStateSetInstrument]
	scope HostScope
}

// snapshotMeasureSetGaugeVec caches snapshot MeasureSet gauge series handles by vec label values.
type snapshotMeasureSetGaugeVec struct {
	cache *vecCache[*snapshotMeasureSetGaugeInstrument]
	scope HostScope
}

// snapshotMeasureSetCounterVec caches snapshot MeasureSet counter series handles by vec label values.
type snapshotMeasureSetCounterVec struct {
	cache *vecCache[*snapshotMeasureSetCounterInstrument]
	scope HostScope
}

// statefulMeasureSetGaugeVec caches stateful MeasureSet gauge series handles by vec label values.
type statefulMeasureSetGaugeVec struct {
	cache *vecCache[*statefulMeasureSetGaugeInstrument]
	scope HostScope
}

// statefulMeasureSetCounterVec caches stateful MeasureSet counter series handles by vec label values.
type statefulMeasureSetCounterVec struct {
	cache *vecCache[*statefulMeasureSetCounterInstrument]
	scope HostScope
}
