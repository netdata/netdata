// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

func (v *snapshotGaugeVec) WithHostScope(scope HostScope) SnapshotGaugeVec {
	return &snapshotGaugeVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *statefulGaugeVec) WithHostScope(scope HostScope) StatefulGaugeVec {
	return &statefulGaugeVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *snapshotCounterVec) WithHostScope(scope HostScope) SnapshotCounterVec {
	return &snapshotCounterVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *statefulCounterVec) WithHostScope(scope HostScope) StatefulCounterVec {
	return &statefulCounterVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *snapshotHistogramVec) WithHostScope(scope HostScope) SnapshotHistogramVec {
	return &snapshotHistogramVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *statefulHistogramVec) WithHostScope(scope HostScope) StatefulHistogramVec {
	return &statefulHistogramVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *snapshotSummaryVec) WithHostScope(scope HostScope) SnapshotSummaryVec {
	return &snapshotSummaryVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *statefulSummaryVec) WithHostScope(scope HostScope) StatefulSummaryVec {
	return &statefulSummaryVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *snapshotStateSetVec) WithHostScope(scope HostScope) SnapshotStateSetVec {
	return &snapshotStateSetVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *statefulStateSetVec) WithHostScope(scope HostScope) StatefulStateSetVec {
	return &statefulStateSetVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *snapshotMeasureSetGaugeVec) WithHostScope(scope HostScope) SnapshotMeasureSetGaugeVec {
	return &snapshotMeasureSetGaugeVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *snapshotMeasureSetCounterVec) WithHostScope(scope HostScope) SnapshotMeasureSetCounterVec {
	return &snapshotMeasureSetCounterVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *statefulMeasureSetGaugeVec) WithHostScope(scope HostScope) StatefulMeasureSetGaugeVec {
	return &statefulMeasureSetGaugeVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

func (v *statefulMeasureSetCounterVec) WithHostScope(scope HostScope) StatefulMeasureSetCounterVec {
	return &statefulMeasureSetCounterVec{cache: v.cache, scope: mustNormalizeHostScope(scope)}
}

// GetWithLabelValues returns a snapshot gauge handle for the provided vec label values.
func (v *snapshotGaugeVec) GetWithLabelValues(labelValues ...string) (SnapshotGauge, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot gauge handle and panics on invalid label values.
func (v *snapshotGaugeVec) WithLabelValues(labelValues ...string) SnapshotGauge {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful gauge handle for the provided vec label values.
func (v *statefulGaugeVec) GetWithLabelValues(labelValues ...string) (StatefulGauge, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful gauge handle and panics on invalid label values.
func (v *statefulGaugeVec) WithLabelValues(labelValues ...string) StatefulGauge {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot counter handle for the provided vec label values.
func (v *snapshotCounterVec) GetWithLabelValues(labelValues ...string) (SnapshotCounter, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot counter handle and panics on invalid label values.
func (v *snapshotCounterVec) WithLabelValues(labelValues ...string) SnapshotCounter {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful counter handle for the provided vec label values.
func (v *statefulCounterVec) GetWithLabelValues(labelValues ...string) (StatefulCounter, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful counter handle and panics on invalid label values.
func (v *statefulCounterVec) WithLabelValues(labelValues ...string) StatefulCounter {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot histogram handle for the provided vec label values.
func (v *snapshotHistogramVec) GetWithLabelValues(labelValues ...string) (SnapshotHistogram, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot histogram handle and panics on invalid label values.
func (v *snapshotHistogramVec) WithLabelValues(labelValues ...string) SnapshotHistogram {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful histogram handle for the provided vec label values.
func (v *statefulHistogramVec) GetWithLabelValues(labelValues ...string) (StatefulHistogram, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful histogram handle and panics on invalid label values.
func (v *statefulHistogramVec) WithLabelValues(labelValues ...string) StatefulHistogram {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot summary handle for the provided vec label values.
func (v *snapshotSummaryVec) GetWithLabelValues(labelValues ...string) (SnapshotSummary, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot summary handle and panics on invalid label values.
func (v *snapshotSummaryVec) WithLabelValues(labelValues ...string) SnapshotSummary {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful summary handle for the provided vec label values.
func (v *statefulSummaryVec) GetWithLabelValues(labelValues ...string) (StatefulSummary, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful summary handle and panics on invalid label values.
func (v *statefulSummaryVec) WithLabelValues(labelValues ...string) StatefulSummary {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot stateset handle for the provided vec label values.
func (v *snapshotStateSetVec) GetWithLabelValues(labelValues ...string) (StateSetInstrument, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot stateset handle and panics on invalid label values.
func (v *snapshotStateSetVec) WithLabelValues(labelValues ...string) StateSetInstrument {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful stateset handle for the provided vec label values.
func (v *statefulStateSetVec) GetWithLabelValues(labelValues ...string) (StateSetInstrument, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful stateset handle and panics on invalid label values.
func (v *statefulStateSetVec) WithLabelValues(labelValues ...string) StateSetInstrument {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot MeasureSet gauge handle for the provided vec label values.
func (v *snapshotMeasureSetGaugeVec) GetWithLabelValues(labelValues ...string) (SnapshotMeasureSetGauge, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot MeasureSet gauge handle and panics on invalid label values.
func (v *snapshotMeasureSetGaugeVec) WithLabelValues(labelValues ...string) SnapshotMeasureSetGauge {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot MeasureSet counter handle for the provided vec label values.
func (v *snapshotMeasureSetCounterVec) GetWithLabelValues(labelValues ...string) (SnapshotMeasureSetCounter, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot MeasureSet counter handle and panics on invalid label values.
func (v *snapshotMeasureSetCounterVec) WithLabelValues(labelValues ...string) SnapshotMeasureSetCounter {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful MeasureSet gauge handle for the provided vec label values.
func (v *statefulMeasureSetGaugeVec) GetWithLabelValues(labelValues ...string) (StatefulMeasureSetGauge, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful MeasureSet gauge handle and panics on invalid label values.
func (v *statefulMeasureSetGaugeVec) WithLabelValues(labelValues ...string) StatefulMeasureSetGauge {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful MeasureSet counter handle for the provided vec label values.
func (v *statefulMeasureSetCounterVec) GetWithLabelValues(labelValues ...string) (StatefulMeasureSetCounter, error) {
	inst, err := v.cache.get(v.scope, labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful MeasureSet counter handle and panics on invalid label values.
func (v *statefulMeasureSetCounterVec) WithLabelValues(labelValues ...string) StatefulMeasureSetCounter {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}
