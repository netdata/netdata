// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Receiver struct {
	Listen         string    `yaml:"listen"               json:"listen"`   // default-secure 127.0.0.1:19938
	TLSCert        string    `yaml:"tls_cert"             json:"tls_cert"` // both or neither
	TLSKey         string    `yaml:"tls_key"              json:"tls_key"`
	TrustedProxies []string  `yaml:"trusted_proxies"      json:"trusted_proxies"` // CIDRs allowed to set XFF/X-Real-IP
	MaxBodyBytes   int64     `yaml:"max_body_bytes"       json:"max_body_bytes"`
	RateLimit      RateLimit `yaml:"rate_limit"           json:"rate_limit"`
	GeoIPDB        string    `yaml:"geoip_db"             json:"geoip_db"` // "" → agent topology-ip-intel mmdb
	// PublicURL is the receiver public base used by sites without their own override.
	PublicURL string `yaml:"public_url,omitempty" json:"public_url"`
}

type RateLimit struct {
	PerIPPerMin   int `yaml:"per_ip_per_min"   json:"per_ip_per_min"`
	PerSitePerSec int `yaml:"per_site_per_sec" json:"per_site_per_sec"`
}

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
	// from rum.listen via PublicBase.
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
	// https://host:port (TLS), such as the backend's agent.
	ExportTo string `yaml:"export_to,omitempty"    json:"export_to"`
}

// TracingOn reports whether trace following is enabled for the site.
func (s Site) TracingOn() bool { return s.Tracing != nil && s.Tracing.Enabled }

var (
	tracingOriginRe = regexp.MustCompile(`^https?://[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*(:[0-9]{1,5})?$`)
	exportToRe      = regexp.MustCompile(`^(https://)?[A-Za-z0-9.-]+:[0-9]{1,5}$`)
)

// ValidateSiteExtras checks the bot and tracing settings,
// shared by file config and dynamic configuration.
func ValidateSiteExtras(s Site) []string {
	var errs []string
	if s.Bots != "" && s.Bots != BotsExclude && s.Bots != BotsInclude {
		errs = append(errs, "bots must be exclude or include")
	}
	if t := s.Tracing; t != nil {
		for _, o := range t.PropagateTo {
			if !tracingOriginRe.MatchString(o) {
				errs = append(errs, "tracing.propagate_to entry "+o+" must be http(s)://host[:port]")
			}
		}
		if t.ExportTo != "" && !exportToRe.MatchString(t.ExportTo) {
			errs = append(errs, "tracing.export_to must be host:port or https://host:port")
		}
	}
	for _, r := range s.RedactPaths {
		if r.Pattern == "" {
			continue // an empty row from the form: ignored, never matched
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			errs = append(errs, "redact_paths pattern "+strconv.Quote(r.Pattern)+" is not a valid regular expression")
		}
	}
	return errs
}

// Investigate is investigate sampling.
type Investigate struct {
	// SampleRate is the share of measured sessions kept in full; 0 means 1.
	SampleRate float64 `yaml:"sample_rate,omitempty" json:"sample_rate"`
	// AlwaysKeep lists conditions that keep a session regardless of
	// SampleRate: "errors", "poor_vitals". Unset means both; an empty
	// list means none.
	AlwaysKeep []string `yaml:"always_keep,omitempty" json:"always_keep"`
}

// Investigate always-keep conditions.
const (
	KeepErrors     = "errors"
	KeepPoorVitals = "poor_vitals"
)

// MeasureRate is the effective measure sampling rate.
func (s Site) MeasureRate() float64 {
	if s.MeasureSampleRate <= 0 {
		return 1
	}
	return s.MeasureSampleRate
}

// InvestigateRate is the effective investigate sampling rate.
func (s Site) InvestigateRate() float64 {
	if s.Investigate == nil || s.Investigate.SampleRate <= 0 {
		return 1
	}
	return s.Investigate.SampleRate
}

// KeepsErrors reports whether sessions with errors are always kept.
func (s Site) KeepsErrors() bool { return s.keeps(KeepErrors) }

// KeepsPoorVitals reports whether sessions with a poor vital are always kept.
func (s Site) KeepsPoorVitals() bool { return s.keeps(KeepPoorVitals) }

func (s Site) keeps(cond string) bool {
	if s.Investigate == nil || s.Investigate.AlwaysKeep == nil {
		return true
	}
	for _, c := range s.Investigate.AlwaysKeep {
		if c == cond {
			return true
		}
	}
	return false
}

// SamplingLabels describes the site's sampling for the Sites table,
// e.g. "25% of sessions" and "10% of measured + errors, poor vitals".
func (s Site) SamplingLabels() (measured, investigated string) {
	measured = percent(s.MeasureRate()) + " of sessions"
	investigated = percent(s.InvestigateRate()) + " of measured"
	if s.InvestigateRate() < 1 {
		var keep []string
		if s.KeepsErrors() {
			keep = append(keep, "errors")
		}
		if s.KeepsPoorVitals() {
			keep = append(keep, "poor vitals")
		}
		if len(keep) > 0 {
			investigated += " + " + strings.Join(keep, ", ")
		}
	}
	return measured, investigated
}

// SamplingNote explains, for the Sessions and Errors tables, why they hold
// fewer sessions than the charts count; "" when nothing is sampled.
func (s Site) SamplingNote() string {
	if s.MeasureRate() >= 1 && s.InvestigateRate() >= 1 {
		return ""
	}
	measured, investigated := s.SamplingLabels()
	return fmt.Sprintf("%s: measuring %s, keeping %s in full", s.Label(), measured, investigated)
}

func percent(rate float64) string {
	return strconv.FormatFloat(rate*100, 'f', -1, 64) + "%"
}

// Label falls back to the stable name when no display name is configured.
func (s Site) Label() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return s.Name
}

// PublicBase resolves the externally reachable collector base for this
// site's setup snippet: PublicURL verbatim (minus a
// trailing slash) when set, otherwise "http://<listen>" with a bind-all
// host (0.0.0.0,::, or empty) replaced by the machine's own hostname —
// a bare listen address is not reachable from outside this host.
func (s Site) PublicBase(listen string, tls bool) string {
	if s.PublicURL != "" {
		return strings.TrimRight(s.PublicURL, "/")
	}
	scheme := "http://"
	if tls {
		scheme = "https://" // the listener only speaks TLS when a cert is configured
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return scheme + listen // defensive; validate.Config already rejects this
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		if h, herr := os.Hostname(); herr == nil && h != "" {
			host = h
		}
	}
	return scheme + net.JoinHostPort(host, port)
}
