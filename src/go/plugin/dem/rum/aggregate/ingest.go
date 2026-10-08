// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"math"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

func (a *Aggregator) Reject(site, reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := &a.site
	if site != st.cfg.Name {
		return
	}
	st.counters[reason]++
	switch reason {
	case beacon.RejectOrigin, beacon.RejectRate, beacon.RejectSize:
		st.rejected = append(st.rejected, a.now())
	case beacon.RejectBot:
		st.bots = append(st.bots, a.now())
	}
}
func (a *Aggregator) Add(counter string, n uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.site.counters[counter] += n
}

// Result carries the newly admitted observations for investigation/export.
// Observation is non-nil for an accepted batch, including a replay with no new measurements.
type Result struct {
	Accepted, PageView, Investigated bool
	Observation                      *beacon.Beacon
}

// Ingest never changes its input. Duplicate checks are O(1); accepted receipt
// inserts/updates are O(log retained) per item. No ingestion path scans retained state.
func (a *Aggregator) Ingest(b *beacon.Beacon) Result {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := &a.site
	if b.Site != st.cfg.Name {
		return Result{}
	}
	now := b.Received
	if now.IsZero() {
		now = a.now()
	}
	st.counters[CounterAccepted]++
	st.accepted = append(st.accepted, now)
	if now.After(st.lastAccepted) {
		st.lastAccepted = now
	}
	obs := *b
	obs.Vitals, obs.Events, obs.Resources, obs.Navigation = nil, nil, nil, nil
	attrs := measurementAttributes(b)
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
		st.appendActivity(activityObservation{
			received:     now,
			attrs:        attrs,
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
		if origin, admitted := st.observeVital(b.ExperienceID, v, attrs, now); admitted {
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
					st.putObservation(vitalObservation{
						key:      key,
						metricID: b.ExperienceID,
						revision: n.Revision,
						received: now,
						value:    metric.value,
						attrs:    attrs,
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
		st.appendActivity(activityObservation{
			received:    now,
			attrs:       attrs,
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
		switch {
		case b.PageHost == "" || r.Host == "":
			st.unknownResources++
		case isFirstParty(b.PageHost, r.Host):
			st.firstPartyResources++
		default:
			st.thirdPartyResources++
		}
		st.appendActivity(activityObservation{
			received: now,
			attrs:    attrs,
			resource: &r,
			pageHost: b.PageHost,
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
		st.observeSession(b.SessionID, attrs.page, now)
	}
	pageView := documents > 0
	var investigated bool
	if b.SessionID != "" {
		investigated = st.touchSession(&obs, pageView, now, a.history)
	} else {
		investigated = st.cfg.Investigate.Rate >= 1 || st.cfg.Investigate.alwaysKeep(&obs)
	}
	if investigated && b.SessionID == "" && a.history != nil {
		for _, e := range obs.Errors {
			a.history.Event(HistoryEvent{
				Site:         st.cfg.Name,
				ObservedUS:   now.UnixMicro(),
				Type:         "error",
				Page:         b.PageGroup,
				ExperienceID: b.ExperienceID,
				View:         b.View,
				ViewID:       b.ViewID,
				Browser:      b.Browser,
				Device:       b.Device,
				Country:      b.Country,
				Version:      b.AppVersion,
				UserID:       b.UserID,
				Fingerprint:  e.Fingerprint,
				ErrorType:    e.Type,
				Message:      e.Message,
				Text:         truncateRunes(e.Type+": "+e.Message, sessionEventTextMax),
				SampleStack:  truncateStack(e.Stack),
			})
		}
	}
	a.appendLive(&obs, pageView, now)
	return Result{
		Accepted:     true,
		PageView:     pageView,
		Investigated: investigated,
		Observation:  &obs,
	}
}
func validMeasurement(v float64) bool { return v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
func measurementAttributes(b *beacon.Beacon) attributes {
	return attributes{
		browser: b.Browser,
		device:  b.Device,
		country: b.Country,
		page:    b.PageGroup,
		version: b.AppVersion,
		view:    b.View,
	}
}
func (st *siteState) admitIdentity(key identityKey, revision uint64, now time.Time) bool {
	index, order := st.identities, &st.identityOrder
	if key.kind == "resource" {
		index, order = st.resourceIdentities, &st.resourceIdentityOrder
	}
	if el := index[key]; el != nil {
		old := el.Value.(*identity)
		if now.Sub(old.received) <= identityRetention {
			if revision <= old.revision {
				return false
			}
			old.revision = revision
			if now.After(old.received) {
				old.received = now
				order.fix(el)
			}
			return true
		}
		order.remove(el)
		delete(index, key)
	}
	entry := &identity{
		key:      key,
		revision: revision,
		received: now,
	}
	if victim := order.capacityVictim(entry); victim != nil {
		old := victim.(*identity)
		if st.lastAccepted.Sub(old.received) <= identityRetention {
			st.loseIdentity(old.key)
		}
		if old == entry {
			return true
		}
		delete(index, old.key)
	}
	index[key] = order.add(entry)
	return true
}

func (st *siteState) loseIdentity(key identityKey) {
	kind := key.kind
	if kind == "vital" {
		kind = key.item
	}
	loss := st.identityLoss[kind]
	if loss == nil {
		loss = &lossInterval{}
		st.identityLoss[kind] = loss
	}
	loss.add(st.lastAccepted, st.window)
	// Every newly admitted identity can renew observed-session membership.
	st.identitySessionLoss.add(st.lastAccepted, st.window)
	st.counters[CounterSamplesDropped]++
}
func (st *siteState) observeVital(experience string, v beacon.Vital, attrs attributes, now time.Time) (attributes, bool) {
	key := observationKey{experience, v.Name}
	if el := st.observations[key]; el != nil {
		attrs = el.Value.(*vitalObservation).attrs
	}
	ledgerKey := identityKey{
		kind:       "vital",
		experience: experience,
		item:       v.Name,
	}
	if el := st.identities[ledgerKey]; el != nil {
		old := el.Value.(*identity)
		if now.Sub(old.received) <= identityRetention {
			if old.metricID != v.ID {
				st.counters[CounterInvalidMeasurements]++
				return attrs, false
			}
			attrs = old.attrs
		}
	}
	if !st.admitIdentity(ledgerKey, v.Revision, now) {
		return attrs, false
	}
	if el := st.identities[ledgerKey]; el != nil {
		evidence := el.Value.(*identity)
		evidence.metricID = v.ID
		evidence.attrs = attrs
	}
	st.putObservation(vitalObservation{
		key:      key,
		metricID: v.ID,
		revision: v.Revision,
		received: now,
		value:    v.Value,
		element:  v.Element,
		attrs:    attrs,
	})
	return attrs, true
}
func (st *siteState) putObservation(v vitalObservation) {
	if el := st.observations[v.key]; el != nil {
		old := el.Value.(*vitalObservation)
		changed := v.received.After(old.received)
		if old.received.After(v.received) {
			v.received = old.received
		}
		*old = v
		if changed {
			st.observationOrder.fix(el)
		}
		return
	}
	entry := new(vitalObservation)
	*entry = v
	if victim := st.observationOrder.capacityVictim(entry); victim != nil {
		old := victim.(*vitalObservation)
		if !old.received.Before(st.lastAccepted.Add(-st.window)) {
			loss := st.vitalLoss[old.key.name]
			if loss == nil {
				loss = &lossInterval{}
				st.vitalLoss[old.key.name] = loss
			}
			loss.add(old.received, st.window)
			st.counters[CounterSamplesDropped]++
		}
		if old == entry {
			return
		}
		delete(st.observations, old.key)
	}
	st.observations[v.key] = st.observationOrder.add(entry)
}
func (st *siteState) appendActivity(v activityObservation) {
	order, loss := &st.activity, &st.activityLoss
	if v.resource != nil {
		order, loss = &st.resources, &st.resourceLoss
	}
	if victim := order.capacityVictim(&v); victim != nil {
		old := victim.(*activityObservation)
		if !old.received.Before(st.lastAccepted.Add(-st.window)) {
			loss.add(old.received, st.window)
			st.counters[CounterSamplesDropped]++
		}
		if old == &v {
			return
		}
	}
	order.add(&v)
}
func (st *siteState) observeSession(id, page string, now time.Time) {
	if id == "" {
		return
	}
	st.touchObservedSession(st.sessions, &st.sessionOrder, id, "", now, &st.sessionLoss)
	st.touchObservedSession(st.pageSessions, &st.pageSessionOrder, strconv.Itoa(len(id))+":"+id+page, page, now, &st.pageSessionLoss)
}
func (st *siteState) touchObservedSession(index map[string]*receiptEntry, order *receiptHeap, key, page string, now time.Time, loss *lossInterval) {
	if el := index[key]; el != nil {
		v := el.Value.(*sessionObservation)
		if now.After(v.received) {
			v.received = now
			order.fix(el)
		}
		return
	}
	entry := &sessionObservation{
		key:      key,
		page:     page,
		received: now,
	}
	if victim := order.capacityVictim(entry); victim != nil {
		v := victim.(*sessionObservation)
		if !v.received.Before(st.lastAccepted.Add(-st.window)) {
			loss.add(v.received, st.window)
			st.counters[CounterSamplesDropped]++
		}
		if v == entry {
			return
		}
		delete(index, v.key)
	}
	index[key] = order.add(entry)
}
