// SPDX-License-Identifier: GPL-3.0-or-later

package config

// Site is one monitored web property. AllowedOrigins is required
// Native job admission determines whether its endpoints exist.
type Site struct {
	Name               string   `yaml:"name"                           json:"name"`
	DisplayName        string   `yaml:"display_name"                   json:"display_name"`
	CollectConsoleLogs bool     `yaml:"collect_console_logs,omitempty" json:"collect_console_logs"`
	AllowedOrigins     []string `yaml:"allowed_origins"                json:"allowed_origins"`
	PageGroups         int      `yaml:"page_groups"                    json:"page_groups"` // top-N page groups kept as instances (1..100)
	Countries          int      `yaml:"countries"                      json:"countries"`   // top-N countries (1..100)
	// PublicURL is the externally reachable collector base (reverse
	// proxy/TLS) used in the setup snippet; "" derives one
	// from the receiver listener and diagnostic observations.
	PublicURL string `yaml:"public_url,omitempty"           json:"public_url"`
	// MeasureSampleRate is the share of sessions the snippet measures, set
	// in the browser; unsampled browsers send nothing. 0 means 1.
	MeasureSampleRate float64 `yaml:"measure_sample_rate,omitempty"  json:"measure_sample_rate"`
	// Investigate samples which measured sessions are kept in full
	// (history, events); nil keeps every session.
	Investigate *Investigate `yaml:"investigate,omitempty"          json:"investigate"`
	// Bots is BotsExclude (default: crawlers, headless browsers and other
	// automation send nothing and are counted as filtered) or BotsInclude
	// (kept, with device "bot").
	Bots string `yaml:"bots,omitempty"                 json:"bots"`
	// Tracing follows browser requests into the backend; nil is off.
	Tracing *Tracing `yaml:"tracing,omitempty"              json:"tracing"`
	// RedactPaths are site-specific path redactions applied before the
	// built-in ones.
	RedactPaths []PathRule `yaml:"redact_paths,omitempty"         json:"redact_paths"`
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
	PropagateTo []string `yaml:"propagate_to,omitempty" json:"propagate_to"`
	// ExportTo is where browser spans go: "" is the agent's own otel.plugin
	// (otel.endpoint), else a gRPC host:port (plaintext) or
	// https://host:port (TLS), such as the backend's agent. IPv6 literals
	// use brackets, for example [::1]:4317; ports must be 1-65535.
	ExportTo string `yaml:"export_to,omitempty"    json:"export_to"`
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
