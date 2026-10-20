// SPDX-License-Identifier: GPL-3.0-or-later

// Package beacon holds the normalized RUM beacon model shared by the
// collector (producer), the aggregator and the OTLP exporter
// (consumers). Keeping it separate avoids import cycles between them.
package beacon

import "time"

// Web-vital names. Values are milliseconds except CLS, a unitless score.
const (
	LCP  = "LCP"
	FCP  = "FCP"
	TTFB = "TTFB"
	INP  = "INP"
	CLS  = "CLS"
)

// Vitals lists the vitals in emission order.
var Vitals = []string{LCP, FCP, TTFB, INP, CLS}

// Devices are the three device classes derived from meta.browser.mobile
// and viewport width. A tablet needs mobile=true and a reported viewport
// width; unknown width retains the mobile/desktop classification.
const (
	DeviceMobile  = "mobile"
	DeviceTablet  = "tablet"
	DeviceDesktop = "desktop"
	// DeviceBot marks bot traffic a site keeps with bots: include.
	DeviceBot = "bot"
)

// TabletViewportMinWidth is the viewport width (CSS px) at/above which a
// mobile:true beacon is classified as tablet rather than mobile ("viewport width >= 600").
const TabletViewportMinWidth = 600

// SessionTTL is how long a session lasts without a beacon before the next
// one starts a new session.
const SessionTTL = 30 * time.Minute

// EventKind describes normalized event semantics independently of wire names.
type EventKind uint8

const (
	EventCustom EventKind = iota
	EventNavigation
	EventResource
	EventRequest
	EventView
	EventSession
	EventDocument
)

// Frustration signals the snippet detects; each event carries a
// "target" CSS selector.
const (
	RageClickEvent  = "rage_click"
	DeadClickEvent  = "dead_click"
	ErrorClickEvent = "error_click"
)

// IsFrustration reports whether an event name is a frustration signal.
func IsFrustration(name string) bool {
	return name == RageClickEvent || name == DeadClickEvent || name == ErrorClickEvent
}

// Beacon is one accepted Faro payload after origin/limit checks, URL
// stripping and GeoIP. Client IPs never appear here.
type Beacon struct {
	Site         string
	Received     time.Time
	SessionID    string
	ExperienceID string // Explicit document activation, independent of session and application view
	Path         string // Document entry URL path (query and fragment stripped)
	PageHost     string // page URL host (first-party resource classification); "" when unknown
	PageGroup    string // Document entry path with volatile segments replaced by :id
	View         string // Current application view name, empty until explicitly assigned
	ViewID       string // Current application view occurrence

	Browser        string
	BrowserVersion string
	OS             string
	Device         string  // mobile|tablet|desktop
	Country        string  // ISO 3166-1 alpha-2, "" when unknown
	City           string  // city name from the mmdb, "" when unknown
	Lat, Lon       float64 // rounded to 1 decimal degree; approximate, valid only when HasGeo
	HasGeo         bool    // false when the mmdb had no location for this IP
	AppVersion     string  // bootstrap data-version → Faro app.version
	UserID         string  // application-supplied identifier; callers choose its contents
	Environment    string  // bootstrap data-env → Faro app.environment

	Vitals     []Vital
	Errors     []Error
	Events     []Event
	Logs       []Log
	Navigation *Navigation // from a faro.performance.navigation event, nil when absent
	Resources  []Resource  // from faro.performance.resource events

	// Spans are browser trace spans when the site follows requests into
	// the backend; ServiceName is their resource service.name.
	Spans       []Span
	ServiceName string
}

// Navigation is page-load timing extracted from a faro.performance.navigation
// event. Has* distinguishes an absent attribute (SDK sent
// no value) from a genuine zero.
type Navigation struct {
	Revision uint64
	LoadMS   float64
	HasLoad  bool
	DCLMS    float64
	HasDCL   bool
}

// Resource is one entry from a faro.performance.resource event: a single sub-resource fetch (script, image, XHR,...).
type Resource struct {
	ID          string
	HasDuration bool
	Host        string
	DurationMS  float64
	TransferB   float64
	Initiator   string
	// Self marks the snippet's own request to the collector (a Faro
	// beacon), excluded from resource measurements.
	Self bool
}

// VitalOrigin contains document measurement dimensions frozen by aggregation.
// Normalization leaves it unset; accepted output updates carry the retained origin.
type VitalOrigin struct {
	PageGroup  string
	Browser    string
	Device     string
	Country    string
	AppVersion string
}

type Vital struct {
	ID       string       // SDK metric identity, preserved across updates
	Revision uint64       // Creation sequence, assigned before batching; zero means unavailable
	Origin   *VitalOrigin // Output-only frozen dimensions for an accepted update
	Name     string
	Value    float64
	Time     time.Time
	// Element is the redacted CSS selector Faro's attribution names: the
	// LCP element, the INP interaction target, the CLS largest shift.
	Element string
}

type Error struct {
	Type        string
	Message     string
	Stack       string
	Time        time.Time
	Fingerprint string // sha1(type+normalized message+first frame)[:12]
}

type Event struct {
	ID       string
	Revision uint64
	Kind     EventKind
	Name     string
	Domain   string
	Attrs    map[string]string
	Time     time.Time
	TraceID  string // hex, set on traced fetch/XHR events
}

// Span is one browser span, with URL attributes already stripped of
// query and fragment. IDs are hex as Faro sends them.
type Span struct {
	TraceID, SpanID, ParentSpanID string
	Name                          string
	Kind                          int32
	StartNS, EndNS                uint64
	Attrs                         []SpanAttr
	StatusCode                    int32
	StatusMessage                 string
	Scope, ScopeVersion           string
}

// SpanAttr is one span attribute; IsInt picks Int over Str.
type SpanAttr struct {
	Key   string
	Str   string
	Int   int64
	IsInt bool
}

type Log struct {
	// ErrorType and Stack contain only the SDK console Error context.
	ErrorType string
	Stack     string
	Level     string
	Message   string
	Time      time.Time
}

// Reject reasons.
const (
	RejectOrigin  = "rejected_origin"
	RejectRate    = "rejected_rate"
	RejectSize    = "rejected_size"
	RejectInvalid = "invalid"
	// RejectBot counts bot beacons a site filters out.
	RejectBot = "bots"
)
