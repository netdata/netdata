// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"math"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// valueFields are the ms/h value statistics, in MeasureSet field order.
var valueFields = []metrix.MeasureFieldSpec{
	{Name: "min", Float: true}, {Name: "max", Float: true}, {Name: "mean", Float: true},
	{Name: "p50", Float: true}, {Name: "p95", Float: true},
}

// writeMeasurement publishes one detached series as <type>.<role>.<encoded-name>
// with direct in-cycle handles.
func (c *Collector) writeMeasurement(m measurement) {
	meta := m.owner.meta
	meter := c.store.Write().SnapshotMeter(string(meta.key.kind)).WithLabels(m.owner.labels...)
	switch meta.key.kind {
	case counter:
		meter.Counter("total."+meta.encodedName, meta.options(meta.unit, meta.title)...).ObserveTotal(m.value)
	case gauge:
		meter.Gauge("value."+meta.encodedName, meta.options(meta.unit, meta.title)...).Observe(m.value)
	case set:
		meter.Gauge("cardinality."+meta.encodedName, meta.options(meta.unit, meta.title)...).
			Observe(m.window.cardinality())
	case timer, histogram:
		c.writeObservations(meter, meta, m.window)
	}
}

// writeObservations publishes ms/h interval statistics. An empty interval has
// zero count and sum, and unavailable (NaN) value statistics.
func (c *Collector) writeObservations(meter metrix.SnapshotMeter, meta *declaration, w *interval) {
	values := []metrix.SampleValue{math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN()}
	var count, sum float64
	if w != nil && w.count != 0 {
		count, sum = w.count, w.sum
		var q [2]float64
		q, c.scratch = w.quantiles.query(c.scratch)
		if math.IsNaN(q[0]) {
			// Rank ambiguity is known only here, before the window resets at release.
			c.diagnostics.percentilesWithheld(w.quantiles.reason)
		}
		values = []metrix.SampleValue{w.min, w.max, sum / count, q[0], q[1]}
	}
	valueOpts := append(meta.options(meta.unit, meta.title), metrix.WithMeasureSetFields(valueFields...))
	meter.MeasureSetGauge("values."+meta.encodedName, valueOpts...).ObservePoint(metrix.MeasureSetPoint{
		Values: values,
	})
	meter.Gauge("count."+meta.encodedName, meta.options("observations", meta.title+" count")...).Observe(count)
	meter.Gauge("sum."+meta.encodedName, meta.options(meta.unit, meta.title+" sum")...).Observe(sum)
}

// options presents one role of the declaration. Roles share the chart family and
// differ only in unit and title; values are floating point.
func (d *declaration) options(unit, title string) []metrix.InstrumentOption {
	return []metrix.InstrumentOption{
		metrix.WithFloat(true),
		metrix.WithUnit(unit),
		metrix.WithDescription(title),
		metrix.WithChartFamily(d.family),
	}
}
