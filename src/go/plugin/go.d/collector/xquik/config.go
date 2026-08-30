// SPDX-License-Identifier: GPL-3.0-or-later

package xquik

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

const (
	defaultUpdateEvery = 300
	defaultEndpoint    = "https://xquik.com/api/v1"
	minimumUpdateEvery = 60
)

var defaultTimeout = confopt.Duration(30 * time.Second)

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`

	User   string `yaml:"user" json:"user"`
	APIKey string `yaml:"api_key" json:"api_key"`

	HTTPConfig `yaml:",inline" json:""`
}

type HTTPConfig struct {
	URL      string           `yaml:"url" json:"url"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	ProxyURL string           `yaml:"proxy_url,omitempty" json:"proxy_url"`

	tlscfg.TLSConfig `yaml:",inline" json:""`
}

func (c *Config) applyDefaults() {
	c.User = strings.TrimSpace(c.User)
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.URL = strings.TrimSpace(c.URL)

	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.URL == "" {
		c.URL = defaultEndpoint
	}
	if c.Timeout.Duration() == 0 {
		c.Timeout = defaultTimeout
	}
}

func (c Config) validate() error {
	var errs []error

	if !validUserIdentifier(c.User) {
		errs = append(errs, errors.New("'user' must be an X username without @ or a numeric user ID"))
	}
	if c.APIKey == "" {
		errs = append(errs, errors.New("'api_key' is required"))
	}
	if c.UpdateEvery < minimumUpdateEvery {
		errs = append(errs, errors.New("'update_every' must be >= 60 seconds"))
	}
	if c.AutoDetectionRetry < 0 {
		errs = append(errs, errors.New("'autodetection_retry' must not be negative"))
	}
	if c.Timeout.Duration() < 0 {
		errs = append(errs, errors.New("'timeout' must not be negative"))
	}

	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, errors.New("'url' must be a valid absolute URL"))
	} else {
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			errs = append(errs, errors.New("'url' must not include credentials, a query, or a fragment"))
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
			errs = append(errs, errors.New("'url' scheme must be https unless using a loopback HTTP endpoint"))
		}
	}

	return errors.Join(errs...)
}

func validUserIdentifier(value string) bool {
	if value == "" || len(value) > 20 {
		return false
	}

	allDigits := true
	for _, r := range value {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return true
	}
	if len(value) > 15 {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
