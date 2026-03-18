// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

import (
	"strconv"
	"strings"
)

// collectSensorMetrics reports per-sensor readings by type.
// Uses cached discovery data — no API calls.
//
// Sensor types and value formats:
//   - Temperature: "32 C" → parse integer before " C"
//   - Voltage: "3.30 V" → parse float before " V", report in millivolts
//   - Current: "1.25 A" → parse float before " A", report in milliamps
//   - Charge Capacity: "95%" → parse integer before "%"
func (c *Collector) collectSensorMetrics() {
	for _, s := range c.discovered.sensors {
		id := s.DurableID

		switch s.SensorType {
		case "Temperature":
			if v, ok := parseIntBefore(s.Value, " C"); ok {
				c.mx.sensor.temperature.WithLabelValues(id).Observe(float64(v))
			}
		case "Voltage":
			if v, ok := parseFloatBefore(s.Value, " V"); ok {
				c.mx.sensor.voltage.WithLabelValues(id).Observe(v * 1000) // millivolts
			}
		case "Current Sensor":
			if v, ok := parseFloatBefore(s.Value, " A"); ok {
				c.mx.sensor.current.WithLabelValues(id).Observe(v * 1000) // milliamps
			}
		case "Charge Capacity":
			if v, ok := parseIntBefore(s.Value, "%"); ok {
				c.mx.sensor.chargeCapacity.WithLabelValues(id).Observe(float64(v))
			}
		}
	}
}

func parseIntBefore(s, suffix string) (int64, bool) {
	idx := strings.Index(s, suffix)
	if idx < 0 {
		idx = len(s)
	}
	v, err := strconv.ParseInt(strings.TrimSpace(s[:idx]), 10, 64)
	return v, err == nil
}

func parseFloatBefore(s, suffix string) (float64, bool) {
	idx := strings.Index(s, suffix)
	if idx < 0 {
		idx = len(s)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s[:idx]), 64)
	return v, err == nil
}
