// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import "strconv"

func (c *Collector) writeMetrics(s sample) {
	if s.GPUUtilization != nil {
		c.metrics.gpuUtilization.Observe(*s.GPUUtilization)
	}
	if s.GPUFrequency != nil {
		c.metrics.gpuFrequency.Observe(*s.GPUFrequency)
	}
	for i, frequency := range s.GPCFrequencies {
		if frequency != nil {
			c.metrics.gpcFrequency.WithLabelValues(strconv.Itoa(i)).Observe(*frequency)
		}
	}
	if s.EMCUtilization != nil {
		c.metrics.emcUtilization.Observe(*s.EMCUtilization)
	}
	if s.EMCFrequency != nil {
		c.metrics.emcFrequency.Observe(*s.EMCFrequency)
	}
	for rail, power := range s.PowerRails {
		c.metrics.powerRailPower.WithLabelValues(rail).Observe(power)
	}
}
