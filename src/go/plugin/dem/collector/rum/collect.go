// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

func (c *Collector) Collect(context.Context) error {
	m := c.store.Write().SnapshotMeter("")
	if c.Name != "" && c.Name != c.Key {
		m = m.WithLabels(metrix.Label{
			Key:   "display_name",
			Value: c.redactor.Apply(c.Name),
		})
	}
	available := c.deps.Hub.Availability().Serving
	state := "unavailable"
	if available {
		state = "available"
	}
	m.StateSet("ingress_state", metrix.WithStateSetStates("available", "unavailable"), metrix.WithStateSetMode(metrix.ModeEnum)).
		Enable(state)
	snaps := c.aggregator.Snapshot()
	if len(snaps) == 0 {
		return nil
	}
	s := snaps[0]
	for _, name := range []string{agg.CounterOTLPSent, agg.CounterOTLPDropped, agg.CounterOTLPErrors, agg.CounterHistoryWritten, agg.CounterHistoryDropped, agg.CounterSpansSent, agg.CounterSpansDropped, agg.CounterSpansErrors} {
		m.Counter(name).ObserveTotal(float64(s.Counters[name]))
	}
	if !available {
		return nil
	}
	for _, name := range []string{agg.CounterPageviews, agg.CounterJSErrors, agg.CounterAccepted, beacon.RejectOrigin, beacon.RejectRate, beacon.RejectSize, beacon.RejectInvalid, beacon.RejectBot, agg.CounterRageClicks, agg.CounterDeadClicks, agg.CounterErrorClicks, agg.CounterSamplesDropped} {
		m.Counter(name).ObserveTotal(float64(s.Counters[name]))
	}
	m.Gauge("active_sessions").Observe(float64(s.ActiveSessions))
	m.Gauge("window_pageviews").Observe(float64(s.PageviewsWindow))
	m.Gauge("window_js_errors").Observe(float64(s.JSErrorsWindow))
	for vital, st := range s.Vitals {
		name := strings.ToLower(vital)
		observePercentiles(m, name, st)
		m.Gauge(name + "_good").Observe(float64(st.Good))
		m.Gauge(name + "_needs_improvement").Observe(float64(st.NeedsImpr))
		m.Gauge(name + "_poor").Observe(float64(st.Poor))
	}
	observePercentiles(m, "load", s.Load)
	observePercentiles(m, "dcl", s.DCL)
	observePercentiles(m, "api", s.API)
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
		m.Counter("resources_first_party").ObserveTotal(float64(s.FirstPartyResources))
		m.Counter("resources_third_party").ObserveTotal(float64(s.ThirdPartyResources))
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
func observePercentiles(m metrix.SnapshotMeter, name string, st agg.VitalStats) {
	if st.N == 0 {
		return
	}
	m.Gauge(name+"_p50", metrix.WithFloat(true)).Observe(st.P50)
	m.Gauge(name+"_p75", metrix.WithFloat(true)).Observe(st.P75)
	m.Gauge(name+"_p95", metrix.WithFloat(true)).Observe(st.P95)
}
