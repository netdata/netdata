// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// meterBackend is the internal write/declaration backend used by meters/instruments.
// Store-backed implementations provide cycle semantics; alternate backends can be added
// later without changing collector-facing meter/instrument interfaces.
type meterBackend interface {
	compileLabelSet(labels ...Label) LabelSet
	registerInstrument(name string, kind metricKind, mode metricMode, opts ...InstrumentOption) (*instrumentDescriptor, error)
	recordGaugeSet(desc *instrumentDescriptor, scope HostScope, value SampleValue, sets []LabelSet)
	recordGaugeAdd(desc *instrumentDescriptor, scope HostScope, delta SampleValue, sets []LabelSet)
	recordCounterObserveTotal(desc *instrumentDescriptor, scope HostScope, value SampleValue, sets []LabelSet)
	recordCounterAdd(desc *instrumentDescriptor, scope HostScope, delta SampleValue, sets []LabelSet)
	recordHistogramObservePoint(desc *instrumentDescriptor, scope HostScope, point HistogramPoint, sets []LabelSet)
	recordHistogramObserve(desc *instrumentDescriptor, scope HostScope, value SampleValue, sets []LabelSet)
	recordSummaryObservePoint(desc *instrumentDescriptor, scope HostScope, point SummaryPoint, sets []LabelSet)
	recordSummaryObserve(desc *instrumentDescriptor, scope HostScope, value SampleValue, sets []LabelSet)
	recordStateSetObserve(desc *instrumentDescriptor, scope HostScope, point StateSetPoint, sets []LabelSet)
	recordMeasureSetGaugeObservePoint(desc *instrumentDescriptor, scope HostScope, point MeasureSetPoint, sets []LabelSet)
	recordMeasureSetGaugeSetPoint(desc *instrumentDescriptor, scope HostScope, point MeasureSetPoint, sets []LabelSet)
	recordMeasureSetGaugeAddPoint(desc *instrumentDescriptor, scope HostScope, delta MeasureSetPoint, sets []LabelSet)
	recordMeasureSetGaugeSetField(desc *instrumentDescriptor, scope HostScope, field string, value SampleValue, sets []LabelSet)
	recordMeasureSetCounterObserveTotalPoint(desc *instrumentDescriptor, scope HostScope, point MeasureSetPoint, sets []LabelSet)
	recordMeasureSetCounterAddPoint(desc *instrumentDescriptor, scope HostScope, delta MeasureSetPoint, sets []LabelSet)
}

var _ meterBackend = (*storeCore)(nil)
