// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

func countMetricSeries(reader metrix.Reader, name string) (count int) {
	reader.ForEachByName(name, func(metrix.LabelView, metrix.SampleValue) {
		count++
	})
	return count
}
