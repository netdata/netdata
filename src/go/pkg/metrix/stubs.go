// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

type snapshotSummaryNotImplemented struct{}

type statefulSummaryNotImplemented struct{}

func (snapshotSummaryNotImplemented) ObservePoint(SummaryPoint, ...LabelSet) {
	panic("metrix: summary is not implemented yet")
}

func (statefulSummaryNotImplemented) Observe(SampleValue, ...LabelSet) {
	panic("metrix: summary is not implemented yet")
}
