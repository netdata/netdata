// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func (c *Collector) Collect(context.Context) error {
	m := c.metrics.meter
	available := c.deps.Registry.Availability().Serving
	state := "unavailable"
	if available {
		state = "available"
	}
	c.metrics.ingress.Enable(state)
	s := c.aggregator.Snapshot()
	for _, counter := range c.metrics.diagnostics {
		counter.instrument.ObserveTotal(float64(s.Counters[counter.name]))
	}
	if !available {
		return nil
	}
	for _, counter := range c.metrics.traffic {
		counter.instrument.ObserveTotal(float64(s.Counters[counter.name]))
	}
	c.metrics.activeSessions.Observe(float64(s.ActiveSessions))
	c.metrics.windowPageviews.Observe(float64(s.PageviewsWindow))
	c.metrics.windowJSErrors.Observe(float64(s.JSErrorsWindow))
	for vital, st := range s.Vitals {
		instruments := c.metrics.vitals[vital]
		instruments.observe(st)
		instruments.good.Observe(float64(st.Good))
		instruments.needsImprovement.Observe(float64(st.NeedsImpr))
		instruments.poor.Observe(float64(st.Poor))
	}
	c.metrics.load.observe(s.Load)
	c.metrics.dcl.observe(s.DCL)
	c.metrics.api.observe(s.API)
	// These label sets can churn. Do not retain a Vec handle for every value.
	for kind, groups := range s.Breakdowns {
		for _, g := range groups {
			gm := m.WithLabels(metrix.Label{
				Key:   kind,
				Value: c.redactor.Apply(g.Value),
			})
			gm.Counter("breakdown_" + kind + "_pageviews").ObserveTotal(float64(g.Pageviews))
			gm.Counter("breakdown_" + kind + "_js_errors").ObserveTotal(float64(g.JSErrors))
			for vital, value := range g.P75 {
				gm.Gauge("breakdown_"+kind+"_"+strings.ToLower(vital), metrix.WithFloat(true)).Observe(value)
			}
		}
	}
	for _, g := range s.ErrorGroups {
		msg := []rune(c.redactor.Apply(g.Message))
		if len(msg) > 80 {
			msg = msg[:80]
		}
		m.WithLabels(metrix.Label{
			Key:   "fingerprint",
			Value: g.Fingerprint,
		}, metrix.Label{
			Key:   "message",
			Value: string(msg),
		}).
			Counter("error_group_total").
			ObserveTotal(float64(g.Total))
	}
	if s.ResourcesSeen {
		c.metrics.firstPartyResources.ObserveTotal(float64(s.FirstPartyResources))
		c.metrics.thirdPartyResources.ObserveTotal(float64(s.ThirdPartyResources))
		for _, host := range s.ResourceHosts {
			hm := m.WithLabels(metrix.Label{
				Key:   "host",
				Value: c.redactor.Apply(host.Host),
			})
			hm.Counter("resource_host_count").ObserveTotal(float64(host.Count))
			if host.HasDuration {
				hm.Gauge("resource_host_p75", metrix.WithFloat(true)).Observe(host.P75Duration)
			}
		}
	}
	return nil
}
