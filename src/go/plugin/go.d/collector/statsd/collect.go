// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"context"
	"math"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

var valueFields = []metrix.MeasureFieldSpec{
	{Name: "min", Float: true}, {Name: "max", Float: true}, {Name: "mean", Float: true},
	{Name: "p50", Float: true}, {Name: "p95", Float: true},
}

func (c *Collector) collect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.receiver == nil {
		return rejectUnavailable
	}
	batch, err := c.receiver.cut(c.now())
	if err != nil {
		return err
	}
	defer c.receiver.release(batch)
	for _, m := range batch {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.writeMeasurement(m)
	}
	return ctx.Err()
}

func (c *Collector) writeMeasurement(m measurement) {
	meta := m.owner.meta
	meter := c.store.Write().SnapshotMeter(string(meta.key.kind)).WithLabels(m.owner.labels...)
	opts := []metrix.InstrumentOption{
		metrix.WithFloat(true),
		metrix.WithUnit(meta.unit),
		metrix.WithDescription(meta.title),
		metrix.WithChartFamily(meta.family),
	}
	switch meta.key.kind {
	case counter:
		meter.Counter("total."+meta.encodedName, opts...).ObserveTotal(m.value)
	case gauge:
		meter.Gauge("value."+meta.encodedName, opts...).Observe(m.value)
	case set:
		var cardinality float64
		if m.window != nil && m.window.count != 0 {
			cardinality = float64(m.window.members.Estimate())
		}
		meter.Gauge("cardinality."+meta.encodedName, opts...).Observe(cardinality)
	case timer, histogram:
		values := []metrix.SampleValue{math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN()}
		var count, sum float64
		if w := m.window; w != nil && w.count != 0 {
			count, sum = w.count, w.sum
			var q [2]float64
			q, c.scratch = w.quantiles.query(c.scratch)
			values = []metrix.SampleValue{w.min, w.max, sum / count, q[0], q[1]}
		}
		meter.MeasureSetGauge("values."+meta.encodedName, append(opts, metrix.WithMeasureSetFields(valueFields...))...).
			ObservePoint(metrix.MeasureSetPoint{
				Values: values,
			})
		meter.Gauge("count."+meta.encodedName, metrix.WithFloat(true), metrix.WithUnit("observations"), metrix.WithDescription(meta.title+" count"), metrix.WithChartFamily(meta.family)).
			Observe(count)
		meter.Gauge("sum."+meta.encodedName, metrix.WithFloat(true), metrix.WithUnit(meta.unit), metrix.WithDescription(meta.title+" sum"), metrix.WithChartFamily(meta.family)).
			Observe(sum)
	}
}
