// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

func (c *Collector) Collect(context.Context) error {
	m := c.metrics.meter
	available := c.deps.Registry.Availability().Serving
	state := "unavailable"
	if available {
		state = "available"
	}
	c.metrics.ingress.Enable(state)
	for _, counter := range c.metrics.diagnostics {
		counter.instrument.ObserveTotal(float64(c.transport.get(counter.name)))
	}
	s := c.aggregator.Snapshot()
	// Inability or intentional disablement is not an observed quiet window.
	if !available || c.MeasureRate() == 0 {
		return nil
	}
	for _, counter := range c.metrics.traffic {
		counter.instrument.ObserveTotal(float64(s.Counters[counter.name]))
	}
	c.metrics.observedSessions.Observe(float64(s.ObservedSessions))
	c.metrics.windowDocumentViews.Observe(float64(s.PageviewsWindow))
	c.metrics.windowApplicationViews.Observe(float64(s.ApplicationViewsWindow))
	c.metrics.windowJSErrors.Observe(float64(s.JSErrorsWindow))
	// Publish an empty population as zero observations. It is ineligible for a
	// quality alert, while its percentile remains unavailable.
	for _, vital := range beacon.Vitals {
		st := s.Vitals[vital]
		instruments := c.metrics.vitals[vital]
		instruments.observe(st)
		instruments.good.Observe(float64(st.Good))
		instruments.needsImprovement.Observe(float64(st.NeedsImpr))
		instruments.poor.Observe(float64(st.Poor))
	}
	c.metrics.load.observe(s.Load)
	c.metrics.dcl.observe(s.DCL)
	c.metrics.fetch.observe(s.API)
	// Dynamic identities use ephemeral handles, never an unbounded Vec cache.
	for kind, groups := range s.Breakdowns {
		for _, g := range groups {
			gm := m.WithLabels(metrix.Label{
				Key:   kind,
				Value: g.Value,
			}, bucketLabel(g.Other))
			prefix := "breakdown_" + kind + "_"
			if kind == aggregate.KindView {
				gm.Gauge(prefix + "js_errors").Observe(float64(g.JSErrors))
				gm.Gauge(prefix + "application_views").Observe(float64(g.ApplicationViews))
				continue
			}
			gm.Gauge(prefix + "document_views").Observe(float64(g.Pageviews))
			gm.Gauge(prefix + "js_errors").Observe(float64(g.JSErrors))
			for _, vital := range beacon.Vitals {
				st := g.Vitals[vital]
				name := prefix + strings.ToLower(vital)
				gm.Gauge(name + "_observations").Observe(float64(st.N))
				if st.N > 0 {
					gm.Gauge(name, metrix.WithFloat(true)).Observe(st.P75)
				}
			}
		}
	}
	for _, g := range s.ErrorGroups {
		msg := []rune(g.Message)
		if len(msg) > 80 {
			msg = msg[:80]
		}
		em := m.WithLabels(metrix.Label{
			Key:   "fingerprint",
			Value: g.Fingerprint,
		}, metrix.Label{
			Key:   "message",
			Value: string(msg),
		}, bucketLabel(g.Other))
		em.Gauge("error_group_count").Observe(float64(g.Count))
	}
	if s.ResourcesSeen {
		c.metrics.sameSiteResources.ObserveTotal(float64(s.FirstPartyResources))
		c.metrics.crossSiteResources.ObserveTotal(float64(s.ThirdPartyResources))
		c.metrics.unknownResources.ObserveTotal(float64(s.UnknownResources))
		for _, host := range s.ResourceHosts {
			hostBucket := bucketLabel(host.Other)
			if host.Unknown {
				hostBucket.Value = "unknown"
			}
			hm := m.WithLabels(metrix.Label{
				Key:   "host",
				Value: host.Host,
			}, hostBucket)
			hm.Gauge("resource_host_count").Observe(float64(host.Count))
			hm.Gauge("resource_host_observations").Observe(float64(host.Duration.N))
			if host.Duration.N > 0 {
				hm.Gauge("resource_host_p75", metrix.WithFloat(true)).Observe(host.Duration.P75)
			}
		}
	}
	return nil
}

// A literal producer value "other" must not collide with the pooled complement.
func bucketLabel(other bool) metrix.Label {
	kind := "value"
	if other {
		kind = "other"
	}
	return metrix.Label{
		Key:   "bucket",
		Value: kind,
	}
}
