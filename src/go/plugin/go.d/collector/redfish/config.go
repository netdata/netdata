// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

const (
	defaultUpdateEvery           = 60
	defaultAuthMethod            = "auto"
	defaultRetries               = 2
	defaultMaxConcurrentRequests = 3
	defaultCollect               = "*"
)

var (
	defaultTimeout = confopt.Duration(15 * time.Second)
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

type Config struct {
	Name               string `yaml:"name,omitempty" json:"name,omitempty"`
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`

	URL string `yaml:"url" json:"url"`

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

	Collect string `yaml:"collect,omitempty" json:"collect"`
}

func (c *Config) applyDefaults() {
	c.URL = strings.TrimSpace(c.URL)
	c.AuthMethod = strings.ToLower(strings.TrimSpace(c.AuthMethod))
	c.Username = strings.TrimSpace(c.Username)
	c.ProxyURL = strings.TrimSpace(c.ProxyURL)
	c.TLSCA = strings.TrimSpace(c.TLSCA)
	c.TLSCert = strings.TrimSpace(c.TLSCert)
	c.TLSKey = strings.TrimSpace(c.TLSKey)
	c.Collect = strings.TrimSpace(c.Collect)

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

}

func (c Config) validate() error {
	var errs []error

	_, _, err := normalizeServiceRoot(c.URL)
	if err != nil {
		errs = append(errs, fmt.Errorf("'url': %w", err))
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

	return errors.Join(errs...)
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
