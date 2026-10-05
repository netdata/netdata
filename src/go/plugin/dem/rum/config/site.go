// SPDX-License-Identifier: GPL-3.0-or-later

package config

// Site is one monitored web property. AllowedOrigins is required
// Native job admission determines whether its endpoints exist.
type Site struct {
	Name           string   `yaml:"name"                          json:"name"`
	DisplayName    string   `yaml:"display_name"                  json:"display_name"`
	AllowedOrigins []string `yaml:"allowed_origins"               json:"allowed_origins"`
	PageGroups     int      `yaml:"page_groups"                   json:"page_groups"` // top-N page groups kept as instances (1..100)
	Countries      int      `yaml:"countries"                     json:"countries"`   // top-N countries (1..100)
	// PublicURL is the externally reachable collector base (reverse
	// proxy/TLS) used in the setup snippet; "" derives one
	// from the receiver listener and diagnostic observations.
	PublicURL string `yaml:"public_url,omitempty"          json:"public_url"`
	// MeasureSampleRate is the browser collection share. Nil defaults to one;
	// zero disables collection, including traffic from previously sampled browsers.
	MeasureSampleRate *float64 `yaml:"measure_sample_rate,omitempty" json:"measure_sample_rate"`
	// Investigate selects detail for native history and enabled event-log/trace
	// exports. Nil selects all received sessions.
	Investigate *Investigate `yaml:"investigate,omitempty"         json:"investigate"`
	// Capture selects optional location and interaction evidence. Nil uses defaults.
	Capture *Capture `yaml:"capture,omitempty" json:"capture"`
	// Bots is BotsExclude (default: crawlers, headless browsers and other
	// automation send nothing and are counted as filtered) or BotsInclude
	// (kept, with device "bot").
	Bots string `yaml:"bots,omitempty"                json:"bots"`
	// EventLogs optionally exports admitted events for search; nil is off.
	EventLogs *EventLogs `yaml:"event_logs"                    json:"event_logs"`
	// Tracing follows browser requests into the backend; nil is off.
	Tracing *Tracing `yaml:"tracing,omitempty"             json:"tracing"`
	// RedactPaths are site-specific path redactions applied before the
	// built-in ones.
	RedactPaths []PathRule `yaml:"redact_paths,omitempty"        json:"redact_paths"`
}

// PathRule replaces Pattern (Go regexp) matches in URL paths.
type PathRule struct {
	Pattern string `yaml:"pattern" json:"pattern"`
	Replace string `yaml:"replace" json:"replace"`
}

// Bot handling.
const (
	BotsExclude = "exclude"
	BotsInclude = "include"
)

// IncludesBots reports whether bot traffic is kept instead of filtered.
func (s Site) IncludesBots() bool { return s.Bots == BotsInclude }

// Tracing is browser-to-backend trace following.
type Tracing struct {
	Enabled bool `yaml:"enabled"                json:"enabled"`
	// PropagateTo lists cross-origin API origins that get the traceparent
	// header; same-origin requests always do. A listed backend must allow
	// the header in its CORS preflight.
	PropagateTo []string    `yaml:"propagate_to,omitempty" json:"propagate_to"`
	Destination Destination `yaml:"destination"            json:"destination"`
}

// TracingOn reports whether trace following is enabled for the site.
func (s Site) TracingOn() bool { return s.Tracing != nil && s.Tracing.Enabled }

// Label falls back to the stable name when no display name is configured.
func (s Site) Label() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return s.Name
}

// Destination is an independent OTLP/gRPC receiver. Nil endpoint means the local default.
type Destination struct {
	Endpoint  *string `yaml:"endpoint"   json:"endpoint"`
	AuthToken string  `yaml:"auth_token" json:"auth_token"`
	TLSCA     string  `yaml:"tls_ca"     json:"tls_ca"`
	TLSCert   string  `yaml:"tls_cert"   json:"tls_cert"`
	TLSKey    string  `yaml:"tls_key"    json:"tls_key"`
}

const DefaultEndpoint = "http://127.0.0.1:4317"

type EventLogs struct {
	Enabled            bool        `yaml:"enabled"              json:"enabled"`
	IncludeConsoleLogs bool        `yaml:"include_console_logs" json:"include_console_logs"`
	Destination        Destination `yaml:"destination"          json:"destination"`
}

func (s Site) EventLogsOn() bool   { return s.EventLogs != nil && s.EventLogs.Enabled }
func (s Site) ConsoleLogsOn() bool { return s.EventLogsOn() && s.EventLogs.IncludeConsoleLogs }

// Effective returns defaults without changing the stored configuration.
func (s Site) Effective() Site {
	capture := Capture{}
	if s.Capture != nil {
		capture = *s.Capture
	}
	geolocation := s.GeolocationMode()
	capture.Geolocation = &geolocation
	s.Capture = &capture
	measure, detail := s.MeasureRate(), s.InvestigateRate()
	s.MeasureSampleRate = &measure
	investigate := Investigate{}
	if s.Investigate != nil {
		investigate = *s.Investigate
	}
	investigate.SampleRate = &detail
	if investigate.AlwaysKeep == nil {
		investigate.AlwaysKeep = []string{KeepErrors, KeepPoorVitals}
	}
	s.Investigate = &investigate
	logs, traces := EventLogs{}, Tracing{}
	if s.EventLogs != nil {
		logs = *s.EventLogs
	}
	if s.Tracing != nil {
		traces = *s.Tracing
	}
	for _, d := range []*Destination{&logs.Destination, &traces.Destination} {
		endpoint := d.EndpointURL()
		d.Endpoint = &endpoint
	}
	s.EventLogs, s.Tracing = &logs, &traces
	return s
}

// EndpointURL preserves an explicit empty value and defaults only omission/null.
func (d Destination) EndpointURL() string {
	if d.Endpoint == nil {
		return DefaultEndpoint
	}
	return *d.Endpoint
}
