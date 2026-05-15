// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// SeededGauge declares a stateful gauge and seeds it with zero immediately.
func SeededGauge(m StatefulMeter, name string, opts ...InstrumentOption) StatefulGauge {
	g := m.Gauge(name, opts...)
	g.Set(0)
	return g
}

// SeededCounter declares a stateful counter and seeds it with zero immediately.
//
// Note: Add(0) advances per-series counter sequence bookkeeping, which is
// harmless for value semantics and ensures a visible committed zero series.
func SeededCounter(m StatefulMeter, name string, opts ...InstrumentOption) StatefulCounter {
	c := m.Counter(name, opts...)
	c.Add(0)
	return c
}
