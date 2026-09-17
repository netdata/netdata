// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

const (
	defaultUpdateEvery           = 60
	defaultAuthMethod            = "auto"
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
	Name               string `yaml:"name,omitempty"                json:"name,omitempty"`
	Vnode              string `yaml:"vnode,omitempty"               json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty"        json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`

	URL string `yaml:"url" json:"url"`

	AuthMethod string `yaml:"auth_method,omitempty" json:"auth_method"`
	Username   string `yaml:"username,omitempty"    json:"username"`
	Password   string `yaml:"password,omitempty"    json:"password"`

	Timeout               confopt.Duration `yaml:"timeout,omitempty"                 json:"timeout"`
	MaxConcurrentRequests int              `yaml:"max_concurrent_requests,omitempty" json:"max_concurrent_requests"`
	ProxyURL              string           `yaml:"proxy_url,omitempty"               json:"proxy_url"`
	TLSCA                 string           `yaml:"tls_ca,omitempty"                  json:"tls_ca"`
	TLSCert               string           `yaml:"tls_cert,omitempty"                json:"tls_cert"`
	TLSKey                string           `yaml:"tls_key,omitempty"                 json:"tls_key"`
	TLSSkipVerify         bool             `yaml:"tls_skip_verify,omitempty"         json:"tls_skip_verify"`

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
