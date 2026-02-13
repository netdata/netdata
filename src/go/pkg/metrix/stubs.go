// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

type snapshotHistogramNotImplemented struct{}

type statefulHistogramNotImplemented struct{}

type snapshotSummaryNotImplemented struct{}

type statefulSummaryNotImplemented struct{}

func (snapshotHistogramNotImplemented) ObservePoint(HistogramPoint, ...LabelSet) {
	panic("metrix: histogram is not implemented yet")
}

func (statefulHistogramNotImplemented) Observe(SampleValue, ...LabelSet) {
	panic("metrix: histogram is not implemented yet")
}

func (snapshotSummaryNotImplemented) ObservePoint(SummaryPoint, ...LabelSet) {
	panic("metrix: summary is not implemented yet")
}

func (statefulSummaryNotImplemented) Observe(SampleValue, ...LabelSet) {
	panic("metrix: summary is not implemented yet")
}
