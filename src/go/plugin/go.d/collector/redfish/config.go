// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

const (
	defaultUpdateEvery                = 60
	defaultAuthMethod                 = "auto"
	defaultRetries                    = 2
	defaultMaxConcurrentRequests      = 3
	defaultCollect                    = "*"
	defaultDetailedComponentsPerGroup = 100
	defaultLogBackend                 = "default"
	defaultLogServiceSelector         = "*"
	defaultMaxLogServices             = 256
)

var (
	defaultTimeout                = confopt.Duration(15 * time.Second)
	defaultLogReconciliationEvery = confopt.Duration(30 * time.Minute)
	defaultCursorOrphanRetention  = confopt.Duration(30 * 24 * time.Hour)
)

var collectionFamilies = []string{
	"base",
	"compute",
	"memory",
	"thermal",
	"power",
	"storage",
	"network",
	"pcie",
	"sensors",
	"firmware",
}

type ChartsConfig struct {
	Aggregates                     *bool `yaml:"aggregates,omitempty" json:"aggregates"`
	Details                        *bool `yaml:"details,omitempty" json:"details"`
	MaxDetailedComponentsPerFamily *int  `yaml:"max_detailed_components_per_family,omitempty" json:"max_detailed_components_per_family"`
}

type AlarmsConfig struct {
	EvaluateThresholds *bool `yaml:"evaluate_thresholds,omitempty" json:"evaluate_thresholds"`
}

type HostScopeOverride struct {
	ResourceURI string `yaml:"resource_uri" json:"resource_uri"`
	GUID        string `yaml:"guid,omitempty" json:"guid"`
	Hostname    string `yaml:"hostname,omitempty" json:"hostname"`
}

type CursorConfig struct {
	OrphanRetention *confopt.Duration `yaml:"orphan_retention,omitempty" json:"orphan_retention"`
}

type LogsConfig struct {
	Enabled                 *bool             `yaml:"enabled,omitempty" json:"enabled"`
	Backend                 *string           `yaml:"backend,omitempty" json:"backend"`
	ServiceSelector         string            `yaml:"service_selector,omitempty" json:"service_selector"`
	MaxServices             *int              `yaml:"max_services,omitempty" json:"max_services"`
	FullReconciliationEvery *confopt.Duration `yaml:"full_reconciliation_every,omitempty" json:"full_reconciliation_every"`
	Cursor                  CursorConfig      `yaml:"cursor,omitempty" json:"cursor"`
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`

	URL       string `yaml:"url" json:"url"`
	NodeMode  string `yaml:"node_mode" json:"node_mode"`
	SystemURI string `yaml:"system_uri,omitempty" json:"system_uri"`

	AuthMethod string `yaml:"auth_method,omitempty" json:"auth_method"`
	Username   string `yaml:"username,omitempty" json:"username"`
	Password   string `yaml:"password,omitempty" json:"password"`

	Timeout               confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Retries               *int             `yaml:"retries,omitempty" json:"retries"`
	MaxConcurrentRequests int              `yaml:"max_concurrent_requests,omitempty" json:"max_concurrent_requests"`
	ProxyURL              string           `yaml:"proxy_url,omitempty" json:"proxy_url"`
	TLSCA                 string           `yaml:"tls_ca,omitempty" json:"tls_ca"`
	TLSCert               string           `yaml:"tls_cert,omitempty" json:"tls_cert"`
	TLSKey                string           `yaml:"tls_key,omitempty" json:"tls_key"`
	TLSSkipVerify         bool             `yaml:"tls_skip_verify,omitempty" json:"tls_skip_verify"`

	Collect            string              `yaml:"collect,omitempty" json:"collect"`
	Charts             ChartsConfig        `yaml:"charts,omitempty" json:"charts"`
	Alarms             AlarmsConfig        `yaml:"alarms,omitempty" json:"alarms"`
	HostScopeOverrides []HostScopeOverride `yaml:"host_scope_overrides,omitempty" json:"host_scope_overrides"`
	Logs               LogsConfig          `yaml:"logs,omitempty" json:"logs"`
}

func (c *Config) applyDefaults() {
	c.URL = strings.TrimSpace(c.URL)
	c.NodeMode = strings.ToLower(strings.TrimSpace(c.NodeMode))
	c.SystemURI = strings.TrimSpace(c.SystemURI)
	c.AuthMethod = strings.ToLower(strings.TrimSpace(c.AuthMethod))
	c.Username = strings.TrimSpace(c.Username)
	c.ProxyURL = strings.TrimSpace(c.ProxyURL)
	c.TLSCA = strings.TrimSpace(c.TLSCA)
	c.TLSCert = strings.TrimSpace(c.TLSCert)
	c.TLSKey = strings.TrimSpace(c.TLSKey)
	c.Collect = strings.TrimSpace(c.Collect)
	c.Logs.ServiceSelector = strings.TrimSpace(c.Logs.ServiceSelector)

	if c.UpdateEvery == 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.AuthMethod == "" {
		c.AuthMethod = defaultAuthMethod
	}
	if c.Timeout.Duration() == 0 {
		c.Timeout = defaultTimeout
	}
	if c.Retries == nil {
		c.Retries = new(defaultRetries)
	}
	if c.MaxConcurrentRequests == 0 {
		c.MaxConcurrentRequests = defaultMaxConcurrentRequests
	}
	if c.Collect == "" {
		c.Collect = defaultCollect
	}
	if c.Charts.Aggregates == nil {
		c.Charts.Aggregates = new(true)
	}
	if c.Charts.Details == nil {
		c.Charts.Details = new(true)
	}
	if c.Charts.MaxDetailedComponentsPerFamily == nil {
		c.Charts.MaxDetailedComponentsPerFamily = new(defaultDetailedComponentsPerGroup)
	}
	if c.Alarms.EvaluateThresholds == nil {
		c.Alarms.EvaluateThresholds = new(true)
	}
	if c.Logs.Enabled == nil {
		c.Logs.Enabled = new(true)
	}
	if c.Logs.ServiceSelector == "" {
		c.Logs.ServiceSelector = defaultLogServiceSelector
	}
	if c.Logs.MaxServices == nil {
		c.Logs.MaxServices = new(defaultMaxLogServices)
	}
	if c.Logs.FullReconciliationEvery == nil {
		c.Logs.FullReconciliationEvery = new(defaultLogReconciliationEvery)
	}
	if c.Logs.Cursor.OrphanRetention == nil {
		c.Logs.Cursor.OrphanRetention = new(defaultCursorOrphanRetention)
	}
	for i := range c.HostScopeOverrides {
		override := &c.HostScopeOverrides[i]
		override.ResourceURI = strings.TrimSpace(override.ResourceURI)
		override.GUID = strings.TrimSpace(override.GUID)
		if parsed, err := uuid.Parse(override.GUID); err == nil {
			override.GUID = parsed.String()
		}
		override.Hostname = strings.TrimSpace(override.Hostname)
	}
	if c.Logs.Backend != nil {
		value := strings.TrimSpace(*c.Logs.Backend)
		c.Logs.Backend = &value
	}
}

func (c Config) validate() error {
	var errs []error

	root, _, err := normalizeServiceRoot(c.URL)
	if err != nil {
		errs = append(errs, fmt.Errorf("'url': %w", err))
	}
	switch c.NodeMode {
	case "local", "system_vnodes":
	default:
		errs = append(errs, errors.New("'node_mode' must be one of: local, system_vnodes"))
	}
	if c.NodeMode == "local" && len(c.HostScopeOverrides) > 0 {
		errs = append(errs, errors.New("'host_scope_overrides' requires node_mode \"system_vnodes\""))
	}
	switch c.AuthMethod {
	case "auto", "session", "basic":
		if c.Username == "" || c.Password == "" {
			errs = append(errs, fmt.Errorf("'username' and 'password' are required for auth_method %q", c.AuthMethod))
		}
	case "none":
		if c.Username != "" || c.Password != "" {
			errs = append(errs, errors.New("'username' and 'password' must be empty for auth_method \"none\""))
		}
	default:
		errs = append(errs, errors.New("'auth_method' must be one of: auto, session, basic, none"))
	}
	if c.UpdateEvery < 1 {
		errs = append(errs, errors.New("'update_every' must be >= 1"))
	}
	if c.Timeout.Duration() <= 0 {
		errs = append(errs, errors.New("'timeout' must be positive"))
	}
	if c.Retries == nil || *c.Retries < 0 {
		errs = append(errs, errors.New("'retries' must be non-negative"))
	}
	if c.MaxConcurrentRequests <= 0 {
		errs = append(errs, errors.New("'max_concurrent_requests' must be positive"))
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		errs = append(errs, errors.New("'tls_cert' and 'tls_key' must be configured together"))
	}
	if err := validateProxyURL(c.ProxyURL); err != nil {
		errs = append(errs, fmt.Errorf("'proxy_url': %w", err))
	}
	if err := validateCollectionPattern(c.Collect); err != nil {
		errs = append(errs, fmt.Errorf("'collect': %w", err))
	}
	if _, err := matcher.NewSimplePatternsMatcher(c.Logs.ServiceSelector); err != nil {
		errs = append(errs, fmt.Errorf("'logs.service_selector': %w", err))
	}
	if c.Charts.MaxDetailedComponentsPerFamily == nil || *c.Charts.MaxDetailedComponentsPerFamily < 0 {
		errs = append(errs, errors.New("'charts.max_detailed_components_per_family' must be non-negative"))
	}
	if c.Logs.MaxServices == nil || *c.Logs.MaxServices <= 0 {
		errs = append(errs, errors.New("'logs.max_services' must be positive"))
	}
	if c.Logs.FullReconciliationEvery == nil || c.Logs.FullReconciliationEvery.Duration() < 0 {
		errs = append(errs, errors.New("'logs.full_reconciliation_every' must be non-negative"))
	}
	if c.Logs.Cursor.OrphanRetention == nil || c.Logs.Cursor.OrphanRetention.Duration() < 0 {
		errs = append(errs, errors.New("'logs.cursor.orphan_retention' must be non-negative"))
	}
	if c.Logs.Backend != nil && *c.Logs.Backend == "" {
		errs = append(errs, errors.New("'logs.backend' must not be explicitly empty"))
	}
	if len(c.LogsBackend()) > 256 {
		errs = append(errs, errors.New("'logs.backend' must not exceed 256 bytes"))
	}

	if root != nil {
		if c.SystemURI != "" {
			if _, err := normalizeConfiguredResourceURI(root, c.SystemURI); err != nil {
				errs = append(errs, fmt.Errorf("'system_uri': %w", err))
			}
		}
		if err := validateHostScopeOverrides(root, c.HostScopeOverrides); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// PrepareDiscoveryProfile validates the endpoint-job portion of one Redfish
// discovery profile without consuming or rewriting secret references.
func PrepareDiscoveryProfile(raw map[string]any, scheme string) (map[string]any, error) {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme == "" {
		scheme = "https"
	}
	if scheme != "https" && scheme != "http" {
		return nil, errors.New("'scheme' must be one of: https, http")
	}
	for _, forbidden := range []string{"module", "name", "url", "system_uri", "host_scope_overrides"} {
		if _, ok := raw[forbidden]; ok {
			return nil, fmt.Errorf("%q is not allowed in a Redfish discovery profile", forbidden)
		}
	}

	normalized := make(map[string]any, len(raw)+1)
	maps.Copy(normalized, raw)
	if _, ok := normalized["node_mode"]; !ok {
		normalized["node_mode"] = "system_vnodes"
	}
	validated := make(map[string]any, len(normalized)+1)
	maps.Copy(validated, normalized)
	validated["url"] = scheme + "://192.0.2.1/redfish/v1/"
	if proxy, ok := validated["proxy_url"].(string); ok && strings.HasPrefix(strings.TrimSpace(proxy), "${") {
		// The discoverer resolves supported atomic references for its probe.
		// Keep the original reference in normalized so the generated job owns
		// normal runtime secret resolution.
		validated["proxy_url"] = "http://127.0.0.1"
	}

	payload, err := json.Marshal(validated)
	if err != nil {
		return nil, fmt.Errorf("encode Redfish discovery profile: %w", err)
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode Redfish discovery profile: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return normalized, nil
}

// DiscoveryEndpointIdentity returns the exact canonical endpoint URL and key
// used by ordinary Redfish jobs.
func DiscoveryEndpointIdentity(rawURL string) (string, string, error) {
	root, origin, err := normalizeServiceRoot(rawURL)
	if err != nil {
		return "", "", err
	}
	return root.String(), stableKey("netdata:redfish:endpoint:v1", origin, endpointKeyHexChars), nil
}

// NewDiscoveryHTTPClient constructs the ordinary Redfish transport without a
// BMC client certificate. Discovery is deliberately unauthenticated.
func NewDiscoveryHTTPClient(raw map[string]any, endpointURL string) (*http.Client, error) {
	values := make(map[string]any, len(raw)+1)
	maps.Copy(values, raw)
	values["url"] = endpointURL
	delete(values, "tls_cert")
	delete(values, "tls_key")
	payload, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode Redfish discovery transport: %w", err)
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode Redfish discovery transport: %w", err)
	}
	cfg.applyDefaults()
	if err := validateProxyURL(cfg.ProxyURL); err != nil {
		return nil, errors.New("invalid resolved discovery proxy_url")
	}
	return newHTTPClient(cfg)
}

func (c Config) LogsBackend() string {
	if c.Logs.Backend == nil {
		return defaultLogBackend
	}
	return *c.Logs.Backend
}

func (c ChartsConfig) aggregatesEnabled() bool {
	return c.Aggregates == nil || *c.Aggregates
}

func (c ChartsConfig) detailsEnabled() bool {
	return c.Details == nil || *c.Details
}

func (c AlarmsConfig) thresholdEvaluationEnabled() bool {
	return c.EvaluateThresholds == nil || *c.EvaluateThresholds
}

func (c LogsConfig) enabled() bool {
	return c.Enabled == nil || *c.Enabled
}

func normalizeServiceRoot(raw string) (*url.URL, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", errors.New("is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", errors.New("invalid URL syntax")
	}
	if u.Opaque != "" || u.Scheme == "" || u.Host == "" {
		return nil, "", errors.New("must be an absolute HTTP or HTTPS URL")
	}
	if u.User != nil {
		return nil, "", errors.New("must not contain user-info")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, "", errors.New("must not contain a query or fragment")
	}
	if u.RawPath != "" {
		return nil, "", errors.New("must not contain an encoded path")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, "", errors.New("scheme must be http or https")
	}
	switch u.EscapedPath() {
	case "", "/", "/redfish/v1", "/redfish/v1/":
	default:
		return nil, "", errors.New("path must be empty, /, /redfish/v1, or /redfish/v1/")
	}

	host, err := canonicalHost(u, scheme)
	if err != nil {
		return nil, "", err
	}
	origin := (&url.URL{Scheme: scheme, Host: host}).String()
	return &url.URL{Scheme: scheme, Host: host, Path: "/redfish/v1/"}, origin, nil
}

func canonicalHost(u *url.URL, scheme string) (string, error) {
	hostname := u.Hostname()
	if hostname == "" {
		return "", errors.New("host is required")
	}
	port := u.Port()
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return "", fmt.Errorf("invalid port %q", port)
		}
		port = strconv.Itoa(value)
	}

	addressHost := hostname
	if addr, err := netip.ParseAddr(hostname); err == nil {
		zone := addr.Zone()
		addr = addr.Unmap()
		if addr.Is4() && zone != "" {
			return "", errors.New("IPv4 host must not contain an interface zone")
		}
		if addr.Is6() && addr.IsLinkLocalUnicast() && addr.Zone() == "" {
			return "", errors.New("link-local IPv6 host requires an interface zone")
		}
		addressHost = addr.String()
	} else {
		if strings.Contains(hostname, "%") {
			return "", errors.New("DNS host must not contain a percent escape")
		}
		addressHost = strings.TrimSuffix(strings.ToLower(addressHost), ".")
		if addressHost == "" {
			return "", errors.New("host is required")
		}
	}

	if port == "80" && scheme == "http" || port == "443" && scheme == "https" {
		port = ""
	}
	if strings.Contains(addressHost, ":") {
		if port == "" {
			return "[" + addressHost + "]", nil
		}
		return net.JoinHostPort(addressHost, port), nil
	}
	if port != "" {
		return net.JoinHostPort(addressHost, port), nil
	}
	return addressHost, nil
}

func validateProxyURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("scheme must be http or https")
	}
	if u.Host == "" {
		return errors.New("host is required")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must not contain a query or fragment")
	}
	return nil
}

func validateCollectionPattern(expr string) error {
	if _, err := matcher.NewSimplePatternsMatcher(expr); err != nil {
		return err
	}
	for term := range strings.FieldsSeq(expr) {
		term = strings.TrimPrefix(term, "!")
		if term == "" {
			return errors.New("contains an empty pattern")
		}
		m, err := matcher.NewSimplePatternsMatcher(term)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(collectionFamilies, m.MatchString) {
			return fmt.Errorf("pattern %q matches no supported collection family", term)
		}
	}
	return nil
}

func normalizeConfiguredResourceURI(root *url.URL, raw string) (string, error) {
	if root == nil {
		return "", errors.New("missing ServiceRoot")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("invalid URI")
	}
	if u.IsAbs() || u.Host != "" || u.User != nil {
		return "", errors.New("must be an absolute-path URI on the configured origin")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", errors.New("must not contain encoding, query, or fragment")
	}
	if !strings.HasPrefix(u.Path, "/redfish/") {
		return "", errors.New("must be an absolute Redfish path")
	}
	cleaned := path.Clean(u.Path)
	if cleaned != u.Path && cleaned+"/" != u.Path {
		return "", errors.New("must not contain dot segments")
	}
	return cleaned, nil
}

func validateHostScopeOverrides(root *url.URL, overrides []HostScopeOverride) error {
	var errs []error
	uris := make(map[string]struct{}, len(overrides))
	guids := make(map[string]struct{}, len(overrides))

	for i, override := range overrides {
		uri, err := normalizeConfiguredResourceURI(root, override.ResourceURI)
		if err != nil {
			errs = append(errs, fmt.Errorf("'host_scope_overrides[%d].resource_uri': %w", i, err))
			continue
		}
		if _, ok := uris[uri]; ok {
			errs = append(errs, fmt.Errorf("'host_scope_overrides[%d].resource_uri' duplicates %q", i, uri))
		}
		uris[uri] = struct{}{}
		if override.GUID == "" && override.Hostname == "" {
			errs = append(errs, fmt.Errorf("'host_scope_overrides[%d]' must set guid or hostname", i))
			continue
		}

		guid := override.GUID
		if guid != "" {
			parsed, err := uuid.Parse(guid)
			if err != nil {
				errs = append(errs, fmt.Errorf("'host_scope_overrides[%d].guid' is invalid", i))
				continue
			}
			guid = parsed.String()
			if _, ok := guids[guid]; ok {
				errs = append(errs, fmt.Errorf("'host_scope_overrides[%d].guid' duplicates %q", i, guid))
			}
			guids[guid] = struct{}{}
		}
		if guid == "" {
			guid = uuid.NewSHA1(uuid.NameSpaceURL, []byte("netdata:redfish:host-scope-validation")).String()
		}
		hostname := override.Hostname
		if hostname == "" {
			hostname = "redfish-override"
		}
		if _, err := chartemit.PrepareHostInfo(netdataapi.HostInfo{GUID: guid, Hostname: hostname}); err != nil {
			errs = append(errs, fmt.Errorf("'host_scope_overrides[%d]': invalid HostScope identity: %w", i, err))
		}
	}
	return errors.Join(errs...)
}
