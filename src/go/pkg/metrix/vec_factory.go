// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// GaugeVec declares or reuses a snapshot gauge and exposes a label-values lookup API.
func (m *snapshotMeter) GaugeVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotGaugeVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindGauge, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotGaugeVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *snapshotGaugeInstrument {
			return &snapshotGaugeInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// GaugeVec declares or reuses a stateful gauge and exposes a label-values lookup API.
func (m *statefulMeter) GaugeVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulGaugeVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindGauge, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulGaugeVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *statefulGaugeInstrument {
			return &statefulGaugeInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// CounterVec declares or reuses a snapshot counter and exposes a label-values lookup API.
func (m *snapshotMeter) CounterVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotCounterVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindCounter, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotCounterVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *snapshotCounterInstrument {
			return &snapshotCounterInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// CounterVec declares or reuses a stateful counter and exposes a label-values lookup API.
func (m *statefulMeter) CounterVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulCounterVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindCounter, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulCounterVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *statefulCounterInstrument {
			return &statefulCounterInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// HistogramVec declares or reuses a snapshot histogram and exposes a label-values lookup API.
func (m *snapshotMeter) HistogramVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotHistogramVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindHistogram, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotHistogramVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *snapshotHistogramInstrument {
			return &snapshotHistogramInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// HistogramVec declares or reuses a stateful histogram and exposes a label-values lookup API.
func (m *statefulMeter) HistogramVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulHistogramVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindHistogram, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulHistogramVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *statefulHistogramInstrument {
			return &statefulHistogramInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// SummaryVec declares or reuses a snapshot summary and exposes a label-values lookup API.
func (m *snapshotMeter) SummaryVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotSummaryVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindSummary, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotSummaryVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *snapshotSummaryInstrument {
			return &snapshotSummaryInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// SummaryVec declares or reuses a stateful summary and exposes a label-values lookup API.
func (m *statefulMeter) SummaryVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulSummaryVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindSummary, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulSummaryVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *statefulSummaryInstrument {
			return &statefulSummaryInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// StateSetVec declares or reuses a snapshot stateset and exposes a label-values lookup API.
func (m *snapshotMeter) StateSetVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotStateSetVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindStateSet, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotStateSetVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *snapshotStateSetInstrument {
			return &snapshotStateSetInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// StateSetVec declares or reuses a stateful stateset and exposes a label-values lookup API.
func (m *statefulMeter) StateSetVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulStateSetVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindStateSet, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulStateSetVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *statefulStateSetInstrument {
			return &statefulStateSetInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// MeasureSetGaugeVec declares or reuses a snapshot MeasureSet gauge and exposes a label-values lookup API.
func (m *snapshotMeter) MeasureSetGaugeVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotMeasureSetGaugeVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindMeasureSet, modeSnapshot, appendMeasureSetSemantics(opts, MeasureSetSemanticsGauge)...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotMeasureSetGaugeVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *snapshotMeasureSetGaugeInstrument {
			return &snapshotMeasureSetGaugeInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// MeasureSetCounterVec declares or reuses a snapshot MeasureSet counter and exposes a label-values lookup API.
func (m *snapshotMeter) MeasureSetCounterVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotMeasureSetCounterVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindMeasureSet, modeSnapshot, appendMeasureSetSemantics(opts, MeasureSetSemanticsCounter)...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotMeasureSetCounterVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *snapshotMeasureSetCounterInstrument {
			return &snapshotMeasureSetCounterInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// MeasureSetGaugeVec declares or reuses a stateful MeasureSet gauge and exposes a label-values lookup API.
func (m *statefulMeter) MeasureSetGaugeVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulMeasureSetGaugeVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindMeasureSet, modeStateful, appendMeasureSetSemantics(opts, MeasureSetSemanticsGauge)...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulMeasureSetGaugeVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *statefulMeasureSetGaugeInstrument {
			return &statefulMeasureSetGaugeInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// MeasureSetCounterVec declares or reuses a stateful MeasureSet counter and exposes a label-values lookup API.
func (m *statefulMeter) MeasureSetCounterVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulMeasureSetCounterVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindMeasureSet, modeStateful, appendMeasureSetSemantics(opts, MeasureSetSemanticsCounter)...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulMeasureSetCounterVec{
		scope: m.scope,
		cache: newVecCache(m.backend, base, keys, func(scope HostScope, base []LabelSet, vecSet LabelSet) *statefulMeasureSetCounterInstrument {
			return &statefulMeasureSetCounterInstrument{
				backend: m.backend,
				desc:    desc,
				scope:   scope,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}
