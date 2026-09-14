// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	defaultUpdateEvery      = 60
	defaultEndpoint         = "https://api.catonetworks.com/api/v1/graphql2"
	defaultDiscoveryEvery   = 3600
	defaultDiscoveryLimit   = 100
	defaultMetricsTimeFrame = "last.PT5M"
	defaultMetricsBuckets   = 5
	defaultMetricsParallel  = 4
	defaultNoBGPCacheTTL    = 3600
	defaultEntitySelector   = "*"
	maxDiscoveryPages       = 1000
)

var defaultTimeout = confopt.Duration(30 * time.Second)

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`

	AccountID string `yaml:"account_id" json:"account_id"`
	APIKey    string `yaml:"api_key" json:"api_key"`

	web.HTTPConfig `yaml:",inline" json:""`

	SiteSelector string `yaml:"site_selector,omitempty" json:"site_selector,omitempty"`
}

func (c *Config) applyDefaults() {
	c.AccountID = strings.TrimSpace(c.AccountID)
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.URL = strings.TrimSpace(c.URL)
	c.SiteSelector = strings.TrimSpace(c.SiteSelector)

	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.URL == "" {
		c.URL = defaultEndpoint
	}
	if c.SiteSelector == "" {
		c.SiteSelector = defaultEntitySelector
	}
	if c.Timeout.Duration() == 0 {
		c.Timeout = defaultTimeout
	}
}

func (c Config) validate() error {
	var errs []error

	if strings.TrimSpace(c.AccountID) == "" {
		errs = append(errs, errors.New("'account_id' is required"))
	}
	if strings.TrimSpace(c.APIKey) == "" {
		errs = append(errs, errors.New("'api_key' is required"))
	}
	if strings.TrimSpace(c.URL) == "" {
		errs = append(errs, errors.New("'url' is required"))
	} else if u, err := url.Parse(strings.TrimSpace(c.URL)); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, errors.New("'url' must be a valid absolute URL"))
	} else if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		errs = append(errs, errors.New("'url' scheme must be https unless using a loopback HTTP endpoint"))
	}
	if c.UpdateEvery < 60 {
		errs = append(errs, errors.New("'update_every' must be >= 60 seconds"))
	}

	return errors.Join(errs...)
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
