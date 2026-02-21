// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// meterBackend is the internal write/declaration backend used by meters/instruments.
// Store-backed implementations provide cycle semantics; alternate backends can be added
// later without changing collector-facing meter/instrument interfaces.
type meterBackend interface {
	compileLabelSet(labels ...Label) LabelSet
	registerInstrument(name string, kind metricKind, mode metricMode, opts ...InstrumentOption) (*instrumentDescriptor, error)
	recordGaugeSet(desc *instrumentDescriptor, value SampleValue, sets []LabelSet)
	recordGaugeAdd(desc *instrumentDescriptor, delta SampleValue, sets []LabelSet)
	recordCounterObserveTotal(desc *instrumentDescriptor, value SampleValue, sets []LabelSet)
	recordCounterAdd(desc *instrumentDescriptor, delta SampleValue, sets []LabelSet)
	recordHistogramObservePoint(desc *instrumentDescriptor, point HistogramPoint, sets []LabelSet)
	recordHistogramObserve(desc *instrumentDescriptor, value SampleValue, sets []LabelSet)
	recordSummaryObservePoint(desc *instrumentDescriptor, point SummaryPoint, sets []LabelSet)
	recordSummaryObserve(desc *instrumentDescriptor, value SampleValue, sets []LabelSet)
	recordStateSetObserve(desc *instrumentDescriptor, point StateSetPoint, sets []LabelSet)
}

var _ meterBackend = (*storeCore)(nil)
