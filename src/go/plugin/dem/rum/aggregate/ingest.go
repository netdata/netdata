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
	st.counters[CounterAccepted]++
	st.accepted = append(st.accepted, now)
	if now.After(st.lastAccepted) {
		st.lastAccepted = now
	}
	obs := *b
	obs.VnodeGUID = st.cfg.VnodeGUID
	obs.Vitals, obs.Events, obs.Resources, obs.Navigation = nil, nil, nil, nil
	attrs := reportAttributes{
		source: b,
	}
	var documents, views, frustrations uint64
	var resourceEvents []beacon.Event
	var navigationEvent *beacon.Event
	for _, ev := range b.Events {
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
			if !st.admitIdentity(key, 1, now) {
				continue
			}
			if ev.Kind == beacon.EventDocument {
				documents++
			} else {
				views++
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
			}, 1, now) {
				continue
			}
		case beacon.EventNavigation:
			if b.Navigation != nil && ev.Revision == b.Navigation.Revision && navigationEvent == nil {
				event := ev
				navigationEvent = &event
			}
			continue
		case beacon.EventResource:
			resourceEvents = append(resourceEvents, ev)
			continue // Typed resources preserve decoder event order.

		}
		if c, ok := frustrationCounters[ev.Name]; ok {
			st.counters[c]++
			frustrations++
		}
		obs.Events = append(obs.Events, ev)
	}
	st.counters[CounterPageviews] += documents
	st.counters[CounterApplicationViews] += views
	if documents+views+frustrations > 0 {
		st.appendActivity(now, activityObservation{
			attrs:        attrs.shared(),
			documents:    documents,
			views:        views,
			frustrations: frustrations,
		})
	}
	for _, v := range b.Vitals {
		if _, supported := thresholds[v.Name]; !supported || b.ExperienceID == "" || v.ID == "" || v.Revision == 0 ||
			!validMeasurement(v.Value) {
			st.counters[CounterInvalidMeasurements]++
			continue
		}
		if origin, admitted := st.observeVital(b.ExperienceID, v, &attrs, now); admitted {
			v.Origin = &beacon.VitalOrigin{
				PageGroup:  origin.page,
				Browser:    origin.browser,
				Device:     origin.device,
				Country:    origin.country,
				AppVersion: origin.version,
			}
			obs.Vitals = append(obs.Vitals, v)
		}
	}
	if nav := b.Navigation; nav != nil {
		if b.ExperienceID == "" || nav.Revision == 0 {
			st.counters[CounterInvalidMeasurements]++
		} else if st.admitIdentity(identityKey{
			kind:       "navigation",
			experience: b.ExperienceID,
		}, nav.Revision, now) {
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
					st.putObservation(now, vitalObservation{
						key:      key,
						metricID: b.ExperienceID,
						revision: n.Revision,
						value:    metric.value,
						attrs:    attrs.shared(),
					})
				} else if el := st.observations[key]; el != nil {
					st.observationOrder.remove(el)
					delete(st.observations, key)
				}
			}
			obs.Navigation = &n
			if navigationEvent != nil {
				obs.Events = append(obs.Events, *navigationEvent)
			}
		}
	}
	for _, e := range b.Errors {
		st.counters[CounterJSErrors]++
		st.appendActivity(now, activityObservation{
			attrs:       attrs.shared(),
			errors:      1,
			fingerprint: e.Fingerprint,
			message:     e.Message,
		})
	}
	for i, r := range b.Resources {
		if r.Self {
			continue
		}
		if b.ExperienceID == "" || r.ID == "" {
			st.counters[CounterInvalidMeasurements]++
			continue
		}
		if !st.admitIdentity(identityKey{
			kind:       "resource",
			experience: b.ExperienceID,
			item:       r.ID,
		}, 1, now) {
			continue
		}
		if r.HasDuration && !validMeasurement(r.DurationMS) {
			r.HasDuration = false
			st.counters[CounterInvalidMeasurements]++
		}
		st.resourcesSeen = true
		firstParty := isFirstParty(b.PageHost, r.Host)
		switch {
		case b.PageHost == "" || r.Host == "":
			st.unknownResources++
		case firstParty:
			st.firstPartyResources++
		default:
			st.thirdPartyResources++
		}
		st.appendActivity(now, activityObservation{
			attrs:    attrs.shared(),
			resource: &r,
			api:      firstParty && r.HasDuration && (r.Initiator == "fetch" || r.Initiator == "xmlhttprequest"),
		})
		obs.Resources = append(obs.Resources, r)
		if i < len(resourceEvents) && resourceEvents[i].ID == r.ID {
			obs.Events = append(obs.Events, resourceEvents[i])
		}
	}
	// Identified observations renew session membership only when newly admitted.
	// Errors, logs, spans and custom events do not carry this replay guarantee.
	if len(obs.Vitals) > 0 || len(obs.Events) > 0 || len(obs.Errors) > 0 || len(obs.Logs) > 0 || len(obs.Spans) > 0 ||
		obs.Navigation != nil ||
		len(obs.Resources) > 0 {
		st.observeSession(b.SessionID, b.PageGroup, now)
	}
	pageView := documents > 0
	var investigated bool
	if b.SessionID != "" {
		investigated = st.touchSession(&obs, pageView, now, a.history)
	} else {
		investigated = st.cfg.Investigate.Rate >= 1 || st.cfg.Investigate.alwaysKeep(&obs)
	}
	if investigated && b.SessionID == "" && a.history != nil {
		// A sessionless error has no timeline; its page group stays as reported.
		context := newEventContext(&obs)
		for _, e := range obs.Errors {
			event, detail := newErrorEvent(e, now, b.PageGroup)
			event.context = context
			rec := event.history(st.cfg.Name, "")
			detail.apply(&rec)
			a.history.Event(rec)
		}
	}
	a.appendLive(&obs, pageView, now)
	return Result{
		PageView:     pageView,
		Investigated: investigated,
		Observation:  &obs,
	}
}
func validMeasurement(v float64) bool { return v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }

// reportAttributes yields one attribute set for everything a report retains,
// allocated only when the report adds a retained item.
type reportAttributes struct {
	source *beacon.Beacon
	set    *attributes
}

func (r *reportAttributes) shared() *attributes {
	if r.set == nil {
		r.set = &attributes{
			browser: r.source.Browser,
			device:  r.source.Device,
			country: r.source.Country,
			page:    r.source.PageGroup,
			version: r.source.AppVersion,
			view:    r.source.View,
		}
	}
	return r.set
}
func (st *siteState) admitIdentity(key identityKey, revision uint64, now time.Time) bool {
	index, order := st.identities, &st.identityOrder
	if key.kind == "resource" {
		index, order = st.resourceIdentities, &st.resourceIdentityOrder
	}
	if el := index[key]; el != nil {
		if now.Sub(el.received) <= identityRetention {
			if revision <= el.Value.revision {
				return false
			}
			el.Value.revision = revision
			order.renew(el, now)
			return true
		}
		order.remove(el)
		delete(index, key)
	}
	index[key] = order.add(now, identity{
		key:      key,
		revision: revision,
	})
	return true
}

// observeVital admits a new vital or a newer revision of a known one. A known
// vital keeps the dimensions of its first admission; the returned set is that origin.
func (st *siteState) observeVital(
	experience string,
	v beacon.Vital,
	report *reportAttributes,
	now time.Time,
) (*attributes, bool) {
	key := observationKey{experience, v.Name}
	var attrs *attributes
	if el := st.observations[key]; el != nil {
		attrs = el.Value.attrs
	}
	evidence := st.vitalIdentities[key]
	if evidence != nil && now.Sub(evidence.received) > identityRetention {
		st.vitalIdentityOrder.remove(evidence)
		delete(st.vitalIdentities, key)
		evidence = nil
	}
	if evidence != nil {
		if evidence.Value.metricID != v.ID {
			st.counters[CounterInvalidMeasurements]++
			return nil, false
		}
		if v.Revision <= evidence.Value.revision {
			return nil, false
		}
		attrs = evidence.Value.attrs
		evidence.Value.revision = v.Revision
		st.vitalIdentityOrder.renew(evidence, now)
	} else {
		if attrs == nil {
			attrs = report.shared()
		}
		st.vitalIdentities[key] = st.vitalIdentityOrder.add(now, vitalIdentity{
			key:      key,
			metricID: v.ID,
			revision: v.Revision,
			attrs:    attrs,
		})
	}
	st.putObservation(now, vitalObservation{
		key:      key,
		metricID: v.ID,
		revision: v.Revision,
		value:    v.Value,
		element:  v.Element,
		attrs:    attrs,
	})
	return attrs, true
}

// putObservation replaces a retained value; its receipt time never moves backwards.
func (st *siteState) putObservation(received time.Time, v vitalObservation) {
	if el := st.observations[v.key]; el != nil {
		el.Value = v
		st.observationOrder.renew(el, received)
		return
	}
	st.observations[v.key] = st.observationOrder.add(received, v)
}
func (st *siteState) appendActivity(received time.Time, v activityObservation) {
	order := &st.activity
	if v.resource != nil {
		order = &st.resources
	}
	order.add(received, v)
}
func (st *siteState) observeSession(id, page string, now time.Time) {
	if id == "" {
		return
	}
	st.touchObservedSession(st.sessions, &st.sessionOrder, id, "", now)
	st.touchObservedSession(st.pageSessions, &st.pageSessionOrder, strconv.Itoa(len(id))+":"+id+page, page, now)
}
func (st *siteState) touchObservedSession(
	index map[string]*receiptEntry[sessionObservation],
	order *receiptHeap[sessionObservation],
	key, page string,
	now time.Time,
) {
	if el := index[key]; el != nil {
		order.renew(el, now)
		return
	}
	index[key] = order.add(now, sessionObservation{
		key:  key,
		page: page,
	})
}
