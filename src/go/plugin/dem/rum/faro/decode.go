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
	maxUserIDLen  = 128
	maxSelector   = 200
	maxNameLen    = 64
	maxMessageLen = 1024
	maxStackLen   = 4096
	maxEventAttrs = 32
	maxAttrLen    = 256
	maxItems      = 500 // per array; the body cap bounds the rest
)

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
	Country            string
	City               string
	Lat                float64
	Lon                float64
	HasGeo             bool
	EventLogs          bool
	FrustrationSignals bool
	Redactor           *redact.Redactor
	ConsoleLogs        bool
	Tracing            bool
	PathRules          []beacon.PathRule
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
	now := opt.Now
	var p faroPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	b := &beacon.Beacon{
		Site:           opt.Site,
		Received:       now,
		SessionID:      beacon.Clean(p.Meta.Session.ID, maxNameLen),
		PageID:         beacon.Clean(p.Meta.Page.ID, maxNameLen),
		Browser:        opt.text(p.Meta.Browser.Name, maxNameLen),
		BrowserVersion: opt.text(p.Meta.Browser.Version, maxNameLen),
		OS:             opt.text(p.Meta.Browser.OS, maxNameLen),
		Device:         beacon.DeviceDesktop,
		Country:        opt.Country,
		City:           opt.City,
		Lat:            opt.Lat,
		Lon:            opt.Lon,
		HasGeo:         opt.HasGeo,
		View:           opt.view(p.Meta.View.Name),
		AppVersion:     opt.text(p.Meta.App.Version, maxNameLen),
		UserID:         opt.text(p.Meta.User.ID, maxUserIDLen),
		Environment:    opt.text(p.Meta.App.Environment, maxNameLen),
	}
	if p.Meta.Browser.Mobile {
		b.Device = beacon.DeviceMobile
		// Tablet needs a viewport width the SDK actually reported; an unparsable/absent width keeps the mobile|desktop
		// split unchanged.
		if w, err := strconv.Atoi(strings.TrimSpace(p.Meta.Browser.ViewportWidth)); err == nil &&
			w >= beacon.TabletViewportMinWidth {
			b.Device = beacon.DeviceTablet
		}
	}
	pageURL := opt.cleanURL(p.Meta.Page.URL)
	b.Path = beacon.Path(pageURL)
	b.PageHost = pageHost(pageURL)
	if b.View != "" {
		b.PageGroup = b.View
	} else {
		b.PageGroup = beacon.PageGroup(b.Path)
	}

	for i, m := range p.Measurements {
		if i >= maxItems {
			break
		}
		ts := flexTime(m.Timestamp, now)
		if len(m.Values) > 0 {
			for k, v := range m.Values {
				name, ok := vitalKeys[strings.ToLower(k)]
				if !ok || v < 0 || v != v { // unknown key or NaN
					continue
				}
				b.Vitals = append(
					b.Vitals,
					beacon.Vital{
						Name:    name,
						Value:   v,
						Time:    ts,
						Element: opt.attributedElement(name, m.Context),
					},
				)
			}
			continue
		}
	}

	for i, e := range p.Exceptions {
		if i >= maxItems {
			break
		}
		var stack strings.Builder
		firstFrame := ""
		for i, frame := range e.Stacktrace.Frames {
			identity := opt.appendFrame(&stack, frame)
			if i == 0 {
				firstFrame = identity
			}
			if stack.Len() >= maxStackLen {
				break
			}
		}
		typ := opt.text(e.Type, maxNameLen)
		msg := opt.text(e.Value, maxMessageLen)
		b.Errors = append(b.Errors, beacon.Error{
			Type:        typ,
			Message:     msg,
			Stack:       beacon.Truncate(stack.String(), maxStackLen),
			Time:        flexTime(e.Timestamp, now),
			Fingerprint: beacon.Fingerprint(typ, msg, firstFrame),
		})
	}

	for i, ev := range p.Events {
		if i >= maxItems {
			break
		}
		// Classification uses protocol names, before free-text transformation.
		name := beacon.Clean(ev.Name, len(ev.Name))
		if name == "" || name == "securitypolicyviolation" || name == "faro.user.action" || (beacon.IsFrustration(name) && !opt.FrustrationSignals) {
			continue
		}
		kind := eventKind(name)
		if kind == beacon.EventCustom && !beacon.IsFrustration(name) {
			name = opt.text(ev.Name, maxNameLen)
		}
		var attrs map[string]string
		if len(ev.Attributes) > 0 {
			for k, v := range ev.Attributes {
				if len(attrs) >= maxEventAttrs {
					break
				}
				k = beacon.Clean(k, len(k))
				nativeAttr := keepEventAttr(kind, name, k)
				if !opt.EventLogs && !nativeAttr {
					continue
				}
				value, valid := opt.eventAttr(kind, k, v)
				if !valid {
					continue
				}
				// Known keys carry protocol semantics; custom keys are diagnostic text.
				if !nativeAttr {
					k = opt.text(k, maxNameLen)
				}
				if k != "" {
					if attrs == nil {
						attrs = make(map[string]string, min(len(ev.Attributes), maxEventAttrs))
					}
					attrs[k] = value
				}
			}
		}
		b.Events = append(b.Events, beacon.Event{
			Name:    name,
			Kind:    kind,
			Domain:  opt.text(ev.Domain, maxNameLen),
			Attrs:   attrs,
			Time:    flexTime(ev.Timestamp, now),
			TraceID: hexID(ev.Trace.TraceID, 32),
		})
		// Navigation/resource timing ride the generic events array — also lift them into typed fields the aggregator uses
		// for the rum.load/rum.dcl/rum.resource_* charts. Attribute names
		// verified against a captured Faro 2.11 beacon, see
		// TestParseFaro timing-event cases in decode_test.go.
		switch name {
		case navigationEvent:
			b.Navigation = parseNavigationAttrs(attrs)
		case resourceEvent:
			if r, ok := parseResourceAttrs(attrs); ok {
				b.Resources = append(b.Resources, r)
			}
		}
	}

	if opt.Tracing && p.Traces != nil {
		b.Spans, b.ServiceName = parseSpans(p.Traces.ResourceSpans, opt)
	}

	if opt.ConsoleLogs {
		for i, l := range p.Logs {
			if i >= maxItems {
				break
			}
			msg := opt.text(l.Message, maxMessageLen)
			if msg == "" {
				continue
			}
			errorType, stack := consoleErrorContext(l.Context, opt)
			b.Logs = append(b.Logs, beacon.Log{
				ErrorType: errorType,
				Stack:     stack,
				Level:     opt.text(strings.ToLower(l.Level), 16),
				Message:   msg,
				Time:      flexTime(l.Timestamp, now),
			})
		}
	}
	return b, nil
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
// classification), independent of beacon.Path's query/fragment stripping.
// "" when the URL has no host (relative path, unparsable).
func pageHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// parseNavigationAttrs reads the two navigation-timing attributes the
// rum.load/rum.dcl charts need from a faro.performance.navigation
// event's attributes. Field names verified against a captured Faro 2.11
// beacon and pinned by TestParseFaro in decode_test.go:
// "pageLoadTime" (fetchStart..domComplete, the SDK's own "page load"
// metric — there is no separate "time to DOMContentLoaded from
// navigation start" attribute) and "domContentLoadHandlerTime" (the
// DOMContentLoaded handler's own duration) — the same two fields
// Grafana's own Faro-based Frontend Observability product charts as
// "page load" and "DOM content loaded". A missing/unparsable attribute
// (the SDK omits it rather than sending a placeholder) leaves Has* false.
func parseNavigationAttrs(attrs map[string]string) *beacon.Navigation {
	if len(attrs) == 0 {
		return nil
	}
	n := &beacon.Navigation{}
	if v, err := strconv.ParseFloat(attrs["pageLoadTime"], 64); err == nil && v >= 0 {
		n.LoadMS, n.HasLoad = v, true
	}
	if v, err := strconv.ParseFloat(attrs["domContentLoadHandlerTime"], 64); err == nil && v >= 0 {
		n.DCLMS, n.HasDCL = v, true
	}
	return n
}

// parseResourceAttrs reads host, duration, transfer size and initiator
// type from a faro.performance.resource event. TestParseFaro pins the
// captured field names: "httpHost", "duration",
// "transferSize", and "initiatorType". ok is false when no numeric
// attribute is present.
func parseResourceAttrs(attrs map[string]string) (beacon.Resource, bool) {
	dur, durErr := strconv.ParseFloat(attrs["duration"], 64)
	if durErr != nil {
		return beacon.Resource{}, false
	}
	r := beacon.Resource{
		Host:       strings.ToLower(attrs["httpHost"]),
		DurationMS: dur,
		Initiator:  beacon.Clean(attrs["initiatorType"], maxNameLen),
	}
	if xfer, err := strconv.ParseFloat(attrs["transferSize"], 64); err == nil {
		r.TransferB = xfer
	}
	if r.Host == "" {
		r.Host = "unknown"
	}
	r.Self = isCollectorRequest(attrs["name"])
	return r, true
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
	return typ, beacon.Truncate(stack.String(), maxStackLen)
}
