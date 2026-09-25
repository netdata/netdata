// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"math"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// valueFields are the ms/h value statistics, in MeasureSet field order.
var valueFields = [...]metrix.MeasureFieldSpec{
	{Name: "min", Float: true}, {Name: "max", Float: true}, {Name: "mean", Float: true},
	{Name: "p50", Float: true}, {Name: "p95", Float: true},
}

// instruments are the Collect-owned handles of one series, named
// <type>.<role>.<encoded-name> and created in the cycle of its first write. A
// retained series is written in every successful cycle, so its descriptors never
// go idle while the handles exist; the handles end with the series.
type instruments struct {
	counter           metrix.SnapshotCounter
	gauge, count, sum metrix.SnapshotGauge // gauge: the g value or s cardinality
	values            metrix.SnapshotMeasureSetGauge
}

func (c *Collector) instruments(e *series) *instruments {
	if e.out != nil {
		return e.out
	}
	d := e.meta
	meter := c.store.Write().SnapshotMeter(string(d.key.kind)).WithLabels(e.labels...)
	opts := d.options(d.unit, d.title)
	h := &instruments{}
	switch d.key.kind {
	case counter:
		h.counter = meter.Counter("total."+d.encodedName, opts...)
	case gauge:
		h.gauge = meter.Gauge("value."+d.encodedName, opts...)
	case set:
		h.gauge = meter.Gauge("cardinality."+d.encodedName, opts...)
	case timer, histogram:
		opts = append(opts, metrix.WithMeasureSetFields(valueFields[:]...))
		h.values = meter.MeasureSetGauge("values."+d.encodedName, opts...)
		h.count = meter.Gauge("count."+d.encodedName, d.options("observations", d.title+" count")...)
		h.sum = meter.Gauge("sum."+d.encodedName, d.options(d.unit, d.title+" sum")...)
	}
	e.out = h
	return h
}

// writeMeasurement publishes one detached series.
func (c *Collector) writeMeasurement(m measurement) {
	h := c.instruments(m.owner)
	switch m.owner.meta.key.kind {
	case counter:
		h.counter.ObserveTotal(m.value)
	case gauge:
		h.gauge.Observe(m.value)
	case set:
		h.gauge.Observe(m.window.cardinality())
	case timer, histogram:
		c.writeObservations(h, m.window)
	}
}

// writeObservations publishes ms/h interval statistics. An empty interval has
// zero count and sum, and unavailable (NaN) value statistics.
func (c *Collector) writeObservations(h *instruments, w *interval) {
	values := c.values[:]
	var count, sum float64
	if w != nil && w.count != 0 {
		count, sum = w.count, w.sum
		var q [2]float64
		q, c.scratch = w.quantiles.Query(c.scratch)
		if math.IsNaN(q[0]) {
			// Rank ambiguity is known only here, before the window resets at release.
			c.diagnostics.percentilesWithheld(w.quantiles.Withheld())
		}
		values[0], values[1], values[2], values[3], values[4] = w.min, w.max, sum/count, q[0], q[1]
	} else {
		for i := range values {
			values[i] = math.NaN()
		}
	}
	h.values.ObservePoint(metrix.MeasureSetPoint{
		Values: values,
	})
	h.count.Observe(count)
	h.sum.Observe(sum)
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
