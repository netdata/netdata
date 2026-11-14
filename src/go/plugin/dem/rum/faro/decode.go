// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// Faro web SDK transport body. Unknown fields are ignored; browser
// traces are decoded when tracing is enabled for the site.
type faroPayload struct {
	Meta struct {
		Page struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"page"`
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		Browser struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			OS      string `json:"os"`
			Mobile  bool   `json:"mobile"`
			// ViewportWidth is a decimal string (window.innerWidth.toString()
			// in the SDK); used only to distinguish tablet from mobile
			// when the device already reports mobile:true.
			ViewportWidth string `json:"viewportWidth"`
		} `json:"browser"`
		View struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"view"`
		App struct {
			Version     string `json:"version"`
			Environment string `json:"environment"`
		} `json:"app"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	} `json:"meta"`
	Measurements []struct {
		Type      string             `json:"type"`
		Values    map[string]float64 `json:"values"`
		Context   map[string]any     `json:"context"`
		Timestamp json.RawMessage    `json:"timestamp"`
	} `json:"measurements"`
	Exceptions []struct {
		Type       string `json:"type"`
		Value      string `json:"value"`
		Stacktrace struct {
			Frames []stackFrame `json:"frames"`
		} `json:"stacktrace"`
		Timestamp json.RawMessage `json:"timestamp"`
	} `json:"exceptions"`
	Events []struct {
		Name       string            `json:"name"`
		Domain     string            `json:"domain"`
		Attributes map[string]string `json:"attributes"`
		Timestamp  json.RawMessage   `json:"timestamp"`
		Trace      struct {
			TraceID string `json:"trace_id"`
		} `json:"trace"`
	} `json:"events"`
	// Traces is OTLP/JSON from Faro's tracing add-on (captured sample
	// testdata/tracing.json).
	Traces *struct {
		ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
	} `json:"traces"`
	Logs []struct {
		Context   json.RawMessage `json:"context"`
		Message   string          `json:"message"`
		Level     string          `json:"level"`
		Timestamp json.RawMessage `json:"timestamp"`
	} `json:"logs"`
}

// Bounds on client-supplied strings (cardinality and record size).
const (
	maxPathLen    = 512
	maxGroupLen   = 200
	maxUserIDLen  = 128
	maxSelector   = 200
	maxNameLen    = 64
	maxMessageLen = 1024
	maxStackLen   = 4096
	maxEventAttrs = 32
	maxAttrLen    = 256
	maxItems      = 500 // per array; the body cap bounds the rest
)

// tabletViewportMinWidth is the viewport width (CSS px) at or above which a
// mobile:true beacon is classified as tablet rather than mobile.
const tabletViewportMinWidth = 600

// vitalKeys maps Faro web-vitals value keys to beacon vital names.
var vitalKeys = map[string]string{
	"lcp": beacon.LCP, "fcp": beacon.FCP, "ttfb": beacon.TTFB, "inp": beacon.INP, "cls": beacon.CLS,
}

// Options supplies site policy and receiver-resolved context for decoding.
// The client IP itself never reaches Decode. Now anchors the 24-hour past to
// 10-minute future window used for measurement, event and log timestamps.
type Options struct {
	Site               string
	Now                time.Time
	Location           beacon.Location
	EventLogs          bool
	FrustrationSignals bool
	Redactor           *redact.Redactor
	ConsoleLogs        bool
	Tracing            bool
	PathRules          []PathRule
}

const (
	navigationEvent = "faro.performance.navigation"
	resourceEvent   = "faro.performance.resource"
	fetchTraceEvent = "faro.tracing.fetch"
	xhrTraceEvent   = "faro.tracing.xml-http-request"
)

// attributionKeys name the element Faro's web-vitals attribution reports
// per vital (key names from the Faro 2.11 bundle).
var attributionKeys = map[string]string{
	beacon.LCP: "element", beacon.INP: "interaction_target", beacon.CLS: "largest_shift_target",
}

// Decode normalizes a Faro transport body using the supplied site policy.
func Decode(raw []byte, opt Options) (*beacon.Beacon, error) {
	var p faroPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	b := opt.meta(&p)
	b.Vitals = opt.vitals(&p)
	b.Errors = opt.exceptions(&p)
	opt.events(&p, b)
	if opt.Tracing && p.Traces != nil {
		b.Spans, b.ServiceName = parseSpans(p.Traces.ResourceSpans, opt)
	}
	if opt.ConsoleLogs {
		b.Logs = opt.logs(&p)
	}
	return b, nil
}

// meta builds the beacon's identity, client and page context.
func (o Options) meta(p *faroPayload) *beacon.Beacon {
	m := &p.Meta
	b := &beacon.Beacon{
		Site:           o.Site,
		Received:       o.Now,
		SessionID:      observationID(m.Session.ID),
		ExperienceID:   observationID(m.Page.ID),
		ViewID:         observationID(m.View.ID),
		Browser:        o.text(m.Browser.Name, maxNameLen),
		BrowserVersion: o.text(m.Browser.Version, maxNameLen),
		OS:             o.text(m.Browser.OS, maxNameLen),
		Device:         beacon.DeviceDesktop,
		Country:        o.Location.Country,
		City:           o.Location.City,
		Lat:            o.Location.Lat,
		Lon:            o.Location.Lon,
		HasGeo:         o.Location.HasGeo,
		View:           o.view(m.View.Name),
		AppVersion:     o.text(m.App.Version, maxNameLen),
		UserID:         o.text(m.User.ID, maxUserIDLen),
		Environment:    o.text(m.App.Environment, maxNameLen),
	}
	if m.Browser.Mobile {
		b.Device = beacon.DeviceMobile
		// Tablet needs a viewport width the SDK actually reported; an unparsable/absent width keeps the mobile|desktop
		// split unchanged.
		if w, err := strconv.Atoi(strings.TrimSpace(m.Browser.ViewportWidth)); err == nil &&
			w >= tabletViewportMinWidth {
			b.Device = beacon.DeviceTablet
		}
	}
	pageURL := o.cleanURL(m.Page.URL)
	b.Path = urlPath(pageURL)
	b.PageHost = pageHost(pageURL)
	b.PageGroup = pageGroup(b.Path)
	return b
}

// vitals reads web-vitals measurements; other measurement values are ignored.
func (o Options) vitals(p *faroPayload) []beacon.Vital {
	var vitals []beacon.Vital
	for i, m := range p.Measurements {
		if i >= maxItems {
			break
		}
		ts := flexTime(m.Timestamp, o.Now)
		for k, v := range m.Values {
			name, ok := vitalKeys[strings.ToLower(k)]
			if !ok || !nonNegativeFinite(v) {
				continue
			}
			vitals = append(vitals, beacon.Vital{
				ID:       observationContextID(m.Context),
				Revision: observationContextRevision(m.Context),
				Name:     name,
				Value:    v,
				Time:     ts,
				Element:  o.attributedElement(name, m.Context),
			})
		}
	}
	return vitals
}

// exceptions reads JS errors with bounded stacks and their fingerprints.
func (o Options) exceptions(p *faroPayload) []beacon.Error {
	var errors []beacon.Error
	for i, e := range p.Exceptions {
		if i >= maxItems {
			break
		}
		var stack strings.Builder
		firstFrame := ""
		for i, frame := range e.Stacktrace.Frames {
			identity := o.appendFrame(&stack, frame)
			if i == 0 {
				firstFrame = identity
			}
			if stack.Len() >= maxStackLen {
				break
			}
		}
		typ := o.text(e.Type, maxNameLen)
		msg := o.text(e.Value, maxMessageLen)
		errors = append(errors, beacon.Error{
			Type:        typ,
			Message:     msg,
			Stack:       truncate(stack.String(), maxStackLen),
			Time:        flexTime(e.Timestamp, o.Now),
			Fingerprint: fingerprint(typ, msg, firstFrame),
		})
	}
	return errors
}

// events reads SDK and custom events. Navigation and resource timing events
// are also lifted into the typed fields aggregation measures.
func (o Options) events(p *faroPayload, b *beacon.Beacon) {
	for i, ev := range p.Events {
		if i >= maxItems {
			break
		}
		// Classification uses protocol names, before free-text transformation.
		name := clean(ev.Name, len(ev.Name))
		if name == "" || name == "securitypolicyviolation" || name == "faro.user.action" || (beacon.IsFrustration(name) && !o.FrustrationSignals) {
			continue
		}
		kind := eventKind(name)
		if kind == beacon.EventCustom && !beacon.IsFrustration(name) {
			name = o.text(ev.Name, maxNameLen)
		}
		attrs := o.eventAttrs(kind, name, ev.Attributes)
		id := observationID(ev.Attributes["observation_id"])
		revision := observationRevision(ev.Attributes["observation_sequence"])
		b.Events = append(b.Events, beacon.Event{
			ID:       id,
			Revision: revision,
			Name:     name,
			Kind:     kind,
			Domain:   o.text(ev.Domain, maxNameLen),
			Attrs:    attrs,
			Time:     flexTime(ev.Timestamp, o.Now),
			TraceID:  hexID(ev.Trace.TraceID, 32),
		})
		// Attribute names verified against a captured Faro 2.11 beacon, see
		// TestParseFaro timing-event cases in decode_test.go.
		switch name {
		case navigationEvent:
			n := parseNavigationAttrs(attrs)
			if n != nil {
				n.Revision = revision
				if b.Navigation == nil || n.Revision > b.Navigation.Revision {
					b.Navigation = n
				}
			}
		case resourceEvent:
			r := parseResourceAttrs(attrs)
			r.ID = id
			b.Resources = append(b.Resources, r)
		}
	}
}

// eventAttrs keeps the attributes native measurements read and, with optional
// event logs, the rest as bounded diagnostic text.
func (o Options) eventAttrs(kind beacon.EventKind, name string, raw map[string]string) map[string]string {
	var attrs map[string]string
	for k, v := range raw {
		if len(attrs) >= maxEventAttrs {
			break
		}
		k = clean(k, len(k))
		rule := eventAttrRule(kind, name, k)
		if !o.EventLogs && !rule.native {
			continue
		}
		value, valid := o.eventAttr(rule, k, v)
		if !valid {
			continue
		}
		// Known keys carry protocol semantics; custom keys are diagnostic text.
		if !rule.native {
			k = o.text(k, maxNameLen)
		}
		if k != "" {
			if attrs == nil {
				attrs = make(map[string]string, min(len(raw), maxEventAttrs))
			}
			attrs[k] = value
		}
	}
	return attrs
}

// logs reads console logs, keeping only the SDK's console Error context.
func (o Options) logs(p *faroPayload) []beacon.Log {
	var logs []beacon.Log
	for i, l := range p.Logs {
		if i >= maxItems {
			break
		}
		msg := o.text(l.Message, maxMessageLen)
		if msg == "" {
			continue
		}
		errorType, stack := consoleErrorContext(l.Context, o)
		logs = append(logs, beacon.Log{
			ErrorType: errorType,
			Stack:     stack,
			Level:     o.text(strings.ToLower(l.Level), 16),
			Message:   msg,
			Time:      flexTime(l.Timestamp, o.Now),
		})
	}
	return logs
}

// attributedElement is the redacted selector Faro reports for vital.
func (o Options) attributedElement(vital string, ctx map[string]any) string {
	v, ok := ctx[attributionKeys[vital]].(string)
	if !ok {
		return ""
	}
	return o.selector(v)
}

// pageHost is the page URL's host (first-party resource
// classification), independent of urlPath's query/fragment stripping.
// "" when the URL has no host (relative path, unparsable).
func pageHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// parseNavigationAttrs reads the two navigation-timing attributes Faro 2.11
// sends on faro.performance.navigation events (pinned by TestParseFaro):
// pageLoadTime (fetchStart to domComplete, the SDK's page-load metric) and
// domContentLoadHandlerTime (the DOMContentLoaded handler's own duration).
// The SDK omits an unavailable attribute, which leaves its Has* flag false.
func parseNavigationAttrs(attrs map[string]string) *beacon.Navigation {
	if len(attrs) == 0 {
		return nil
	}
	n := &beacon.Navigation{}
	n.LoadMS, n.HasLoad = parseNonNegative(attrs["pageLoadTime"])
	n.DCLMS, n.HasDCL = parseNonNegative(attrs["domContentLoadHandlerTime"])
	if !n.HasLoad && !n.HasDCL {
		return nil
	}
	return n
}

// parseResourceAttrs preserves entries whose duration is unavailable. Count and
// duration populations differ; a present finite zero remains a duration sample.
func parseResourceAttrs(attrs map[string]string) beacon.Resource {
	r := beacon.Resource{
		Host:      strings.ToLower(attrs["httpHost"]),
		Initiator: clean(attrs["initiatorType"], maxNameLen),
		Self:      isCollectorRequest(attrs["name"]),
	}
	r.DurationMS, r.HasDuration = parseNonNegative(attrs["duration"])
	if xfer, ok := parseNonNegative(attrs["transferSize"]); ok {
		r.TransferB = xfer
	}
	return r
}

// isCollectorRequest reports whether a resource URL is the snippet's own
// beacon endpoint: a path ending in rum/<site>/collect, whatever prefix
// the collector's public base carries.
func isCollectorRequest(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	seg := strings.Split(strings.TrimSuffix(u.Path, "/"), "/")
	n := len(seg)
	return n >= 3 && seg[n-3] == "rum" && seg[n-2] != "" && seg[n-1] == "collect"
}

// flexTime accepts RFC3339 strings (what Faro sends), unix seconds or
// milliseconds; anything else, or a time outside [now-24h, now+10m],
// collapses to now so the agent never rejects the record for its clock.
func flexTime(raw json.RawMessage, now time.Time) time.Time {
	t := now
	if len(raw) > 0 {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if pt, err := time.Parse(time.RFC3339Nano, s); err == nil {
				t = pt
			}
		} else if f, err := strconv.ParseFloat(strings.Trim(string(raw), `"`), 64); err == nil && f > 0 {
			if f > 1e12 {
				f /= 1000
			}
			t = time.Unix(int64(f), int64((f-float64(int64(f)))*1e9))
		}
	}
	if t.Before(now.Add(-24*time.Hour)) || t.After(now.Add(10*time.Minute)) {
		return now
	}
	return t
}

func eventKind(name string) beacon.EventKind {
	switch name {
	case navigationEvent:
		return beacon.EventNavigation
	case resourceEvent:
		return beacon.EventResource
	case fetchTraceEvent, xhrTraceEvent:
		return beacon.EventRequest
	case "document_activated":
		return beacon.EventDocument
	case "view_changed":
		return beacon.EventView
	case "session_start", "session_resume", "session_extend":
		return beacon.EventSession
	default:
		return beacon.EventCustom
	}
}

// consoleErrorContext reads the space-separated JSON frames Faro 2.11 emits.
// Only the known Error subset is retained; malformed context cannot reject vitals.
func consoleErrorContext(raw json.RawMessage, opt Options) (string, string) {
	var ctx struct {
		Type        json.RawMessage `json:"type"`
		StackFrames json.RawMessage `json:"stackFrames"`
	}
	if json.Unmarshal(raw, &ctx) != nil {
		return "", ""
	}
	var typ, encoded string
	_ = json.Unmarshal(ctx.Type, &typ)
	_ = json.Unmarshal(ctx.StackFrames, &encoded)
	typ = opt.text(typ, maxNameLen)
	var stack strings.Builder
	decoder := json.NewDecoder(strings.NewReader(encoded))
	for {
		var f *stackFrame
		if err := decoder.Decode(&f); err == io.EOF {
			break
		} else if err != nil {
			return typ, ""
		}
		if f == nil {
			return typ, ""
		}
		opt.appendFrame(&stack, *f)
		if stack.Len() >= maxStackLen {
			break
		}
	}
	return typ, truncate(stack.String(), maxStackLen)
}
