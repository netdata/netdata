// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"math"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// Reject counts one request the receiver refused for this site.
func (a *Aggregator) Reject(reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := &a.site
	now := a.now()
	st.expireActivityIfDue(now)
	st.counters[reason]++
	switch reason {
	case beacon.RejectOrigin, beacon.RejectRate, beacon.RejectSize:
		st.rejected = append(st.rejected, now)
	case beacon.RejectBot:
		st.bots = append(st.bots, now)
	}
}

// Result carries the newly admitted observations for investigation/export.
// Observation is never nil, including for a replay with no new measurements.
type Result struct {
	PageView, Investigated bool
	Observation            *beacon.Beacon
}

// Ingest never changes its input. Duplicate checks are O(1); accepted receipt
// inserts/updates and expired-item removal are O(log retained) per item.
// Receiver activity timestamps are compacted at most once per minute.
func (a *Aggregator) Ingest(b *beacon.Beacon) Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := &a.site
	clock := a.now()
	st.expireMeasurements(clock)
	st.expireActivityIfDue(clock)
	now := b.Received
	if now.IsZero() {
		now = clock
	}
	st.countAccepted(now)
	obs := *b
	obs.VnodeGUID = st.cfg.VnodeGUID
	obs.Vitals, obs.Events, obs.Resources, obs.Navigation = nil, nil, nil, nil
	r := report{
		in:  b,
		out: &obs,
		now: now,
	}
	// Section order fixes receipt-heap insertion order for equal receipt times.
	st.admitEvents(&r)
	st.admitVitals(&r)
	st.admitNavigation(&r)
	st.admitErrors(&r)
	st.admitResources(&r)
	// Identified observations renew session membership only when newly admitted.
	// Errors, logs, spans and custom events do not carry this replay guarantee.
	if len(obs.Vitals) > 0 || len(obs.Events) > 0 || len(obs.Errors) > 0 || len(obs.Logs) > 0 || len(obs.Spans) > 0 ||
		obs.Navigation != nil ||
		len(obs.Resources) > 0 {
		st.observeSession(b.SessionID, b.PageGroup, now)
	}
	pageView := r.documents > 0
	var investigated bool
	if b.SessionID != "" {
		investigated = st.touchSession(&obs, pageView, now, a.history)
	} else {
		investigated = st.recordSessionless(&obs, now, a.history)
	}
	a.appendLive(&obs, pageView, now)
	return Result{
		PageView:     pageView,
		Investigated: investigated,
		Observation:  &obs,
	}
}
func validMeasurement(v float64) bool { return v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }

func (st *siteState) countAccepted(now time.Time) {
	st.counters[CounterAccepted]++
	st.accepted = append(st.accepted, now)
	if now.After(st.lastAccepted) {
		st.lastAccepted = now
	}
}

// report is one beacon's admission state: the filtered observation it builds
// and the timing events that pair with its typed navigation and resources.
// navigationEvent points into the unchanged input.
type report struct {
	in, out          *beacon.Beacon
	now              time.Time
	attrs            *attributes
	documents, views uint64
	navigationEvent  *beacon.Event
	resourceEvents   []beacon.Event
}

// shared yields one attribute set for everything a report retains, allocated
// only when the report adds a retained item.
func (r *report) shared() *attributes {
	if r.attrs == nil {
		r.attrs = &attributes{
			browser: r.in.Browser,
			device:  r.in.Device,
			country: r.in.Country,
			page:    r.in.PageGroup,
			version: r.in.AppVersion,
			view:    r.in.View,
		}
	}
	return r.attrs
}

// admitEvents counts activations and admits session lifecycle occurrences once
// per identity, and keeps custom, request and frustration events. Timing events
// wait for the typed navigation and resources they describe.
func (st *siteState) admitEvents(r *report) {
	b := r.in
	var frustrations uint64
	for i, ev := range b.Events {
		switch ev.Kind {
		case beacon.EventDocument, beacon.EventView:
			if b.ExperienceID == "" || ev.ID == "" || ev.Revision == 0 ||
				(ev.Kind == beacon.EventView && b.ViewID == "") {
				st.counters[CounterInvalidMeasurements]++
				continue
			}
			// Activations are occurrences, not revision-counted events.
			key := identityKey{
				kind:       "document",
				experience: b.ExperienceID,
			}
			if ev.Kind == beacon.EventView {
				key = identityKey{
					kind:       "view",
					experience: b.ExperienceID,
					item:       b.ViewID,
				}
			}
			if !st.admitIdentity(key, 1, r.now) {
				continue
			}
			if ev.Kind == beacon.EventDocument {
				r.documents++
			} else {
				r.views++
			}
		case beacon.EventSession:
			if b.ExperienceID == "" || b.SessionID == "" || ev.ID == "" || ev.Revision == 0 {
				st.counters[CounterInvalidMeasurements]++
				continue
			}
			if !st.admitIdentity(identityKey{
				kind:       "session",
				experience: b.ExperienceID,
				item:       ev.ID,
			}, 1, r.now) {
				continue
			}
		case beacon.EventNavigation:
			if b.Navigation != nil && ev.Revision == b.Navigation.Revision && r.navigationEvent == nil {
				r.navigationEvent = &b.Events[i]
			}
			continue
		case beacon.EventResource:
			r.resourceEvents = append(r.resourceEvents, ev)
			continue // Typed resources preserve decoder event order.
		}
		if c, ok := frustrationCounters[ev.Name]; ok {
			st.counters[c]++
			frustrations++
		}
		r.out.Events = append(r.out.Events, ev)
	}
	st.counters[CounterPageviews] += r.documents
	st.counters[CounterApplicationViews] += r.views
	if r.documents+r.views+frustrations > 0 {
		st.appendActivity(r.now, activityObservation{
			attrs:        r.shared(),
			documents:    r.documents,
			views:        r.views,
			frustrations: frustrations,
		})
	}
}

// admitVitals admits new vitals and newer revisions; an accepted update carries
// the dimensions frozen at the vital's first admission.
func (st *siteState) admitVitals(r *report) {
	b := r.in
	for _, v := range b.Vitals {
		if _, supported := thresholds[v.Name]; !supported || b.ExperienceID == "" || v.ID == "" || v.Revision == 0 ||
			!validMeasurement(v.Value) {
			st.counters[CounterInvalidMeasurements]++
			continue
		}
		if origin, admitted := st.observeVital(r, v); admitted {
			v.Origin = &beacon.VitalOrigin{
				PageGroup:  origin.page,
				Browser:    origin.browser,
				Device:     origin.device,
				Country:    origin.country,
				AppVersion: origin.version,
			}
			r.out.Vitals = append(r.out.Vitals, v)
		}
	}
}

// admitNavigation replaces a document's navigation timing with a newer revision.
func (st *siteState) admitNavigation(r *report) {
	b := r.in
	nav := b.Navigation
	if nav == nil {
		return
	}
	if b.ExperienceID == "" || nav.Revision == 0 {
		st.counters[CounterInvalidMeasurements]++
		return
	}
	if !st.admitIdentity(identityKey{
		kind:       "navigation",
		experience: b.ExperienceID,
	}, nav.Revision, r.now) {
		return
	}
	n := *nav
	if n.HasLoad && !validMeasurement(n.LoadMS) {
		n.HasLoad = false
		st.counters[CounterInvalidMeasurements]++
	}
	if n.HasDCL && !validMeasurement(n.DCLMS) {
		n.HasDCL = false
		st.counters[CounterInvalidMeasurements]++
	}
	// Revision replacement also removes a field absent from the newer report.
	for _, metric := range []struct {
		name    string
		value   float64
		present bool
	}{{navLoadName, n.LoadMS, n.HasLoad}, {navDCLName, n.DCLMS, n.HasDCL}} {
		key := observationKey{b.ExperienceID, metric.name}
		if metric.present {
			st.observations.put(r.now, key, vitalObservation{
				metricID: b.ExperienceID,
				revision: n.Revision,
				value:    metric.value,
				attrs:    r.shared(),
			})
		} else if el := st.observations.get(key); el != nil {
			st.observations.remove(el)
		}
	}
	r.out.Navigation = &n
	if r.navigationEvent != nil {
		r.out.Events = append(r.out.Events, *r.navigationEvent)
	}
}

func (st *siteState) admitErrors(r *report) {
	for _, e := range r.in.Errors {
		st.counters[CounterJSErrors]++
		st.appendActivity(r.now, activityObservation{
			attrs:       r.shared(),
			errors:      1,
			fingerprint: e.Fingerprint,
			message:     e.Message,
		})
	}
}

// admitResources admits each resource identity once and classifies ownership.
// The decoder creates one resource per resource event, so they pair by position.
func (st *siteState) admitResources(r *report) {
	b := r.in
	for i, res := range b.Resources {
		if res.Self {
			continue
		}
		if b.ExperienceID == "" || res.ID == "" {
			st.counters[CounterInvalidMeasurements]++
			continue
		}
		if !st.admitIdentity(identityKey{
			kind:       "resource",
			experience: b.ExperienceID,
			item:       res.ID,
		}, 1, r.now) {
			continue
		}
		if res.HasDuration && !validMeasurement(res.DurationMS) {
			res.HasDuration = false
			st.counters[CounterInvalidMeasurements]++
		}
		st.resourcesSeen = true
		firstParty := isFirstParty(b.PageHost, res.Host)
		switch {
		case b.PageHost == "" || res.Host == "":
			st.unknownResources++
		case firstParty:
			st.firstPartyResources++
		default:
			st.thirdPartyResources++
		}
		st.appendActivity(r.now, activityObservation{
			attrs:    r.shared(),
			resource: &res,
			api:      firstParty && res.HasDuration && (res.Initiator == "fetch" || res.Initiator == "xmlhttprequest"),
		})
		r.out.Resources = append(r.out.Resources, res)
		if i < len(r.resourceEvents) && r.resourceEvents[i].ID == res.ID {
			r.out.Events = append(r.out.Events, r.resourceEvents[i])
		}
	}
}
func (st *siteState) admitIdentity(key identityKey, revision uint64, now time.Time) bool {
	index := &st.identities
	if key.kind == "resource" {
		index = &st.resourceIdentities
	}
	if el := index.current(key, now, identityRetention); el != nil {
		if revision <= el.Value {
			return false
		}
		el.Value = revision
		index.renew(el, now)
		return true
	}
	index.add(now, key, revision)
	return true
}

// observeVital admits a new vital or a newer revision of a known one. A known
// vital keeps the dimensions of its first admission; the returned set is that origin.
func (st *siteState) observeVital(r *report, v beacon.Vital) (*attributes, bool) {
	now := r.now
	key := observationKey{r.in.ExperienceID, v.Name}
	var attrs *attributes
	if el := st.observations.get(key); el != nil {
		attrs = el.Value.attrs
	}
	if evidence := st.vitalIdentities.current(key, now, identityRetention); evidence != nil {
		if evidence.Value.metricID != v.ID {
			st.counters[CounterInvalidMeasurements]++
			return nil, false
		}
		if v.Revision <= evidence.Value.revision {
			return nil, false
		}
		attrs = evidence.Value.attrs
		evidence.Value.revision = v.Revision
		st.vitalIdentities.renew(evidence, now)
	} else {
		if attrs == nil {
			attrs = r.shared()
		}
		st.vitalIdentities.add(now, key, vitalIdentity{
			metricID: v.ID,
			revision: v.Revision,
			attrs:    attrs,
		})
	}
	st.observations.put(now, key, vitalObservation{
		metricID: v.ID,
		revision: v.Revision,
		value:    v.Value,
		element:  v.Element,
		attrs:    attrs,
	})
	return attrs, true
}
func (st *siteState) appendActivity(received time.Time, v activityObservation) {
	order := &st.activity
	if v.resource != nil {
		order = &st.resources
	}
	order.add(received, struct{}{}, v)
}
func (st *siteState) observeSession(id, page string, now time.Time) {
	if id == "" {
		return
	}
	st.sessions.put(now, id, struct{}{})
	st.pageSessions.put(now, strconv.Itoa(len(id))+":"+id+page, page)
}
