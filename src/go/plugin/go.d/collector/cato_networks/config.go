// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	defaultUpdateEvery      = 60
	defaultEndpoint         = "https://api.catonetworks.com/api/v1/graphql2"
	defaultDiscoveryEvery   = 300
	defaultDiscoveryLimit   = 100
	defaultMetricsTimeFrame = "last.PT5M"
	defaultMetricsBuckets   = 5
	defaultMaxSitesPerQuery = 50
	defaultBGPRefreshEvery  = 300
	defaultBGPMaxSites      = 25
	defaultRetryAttempts    = 5
	defaultEventsMaxPages   = 10
	defaultEventsMaxSeries  = 50
	defaultEntitySelector   = "*"
	defaultMaxSites         = 500
	defaultMaxIfacesPerSite = 32
	defaultBGPMaxPeers      = 32
	maxDiscoveryPages       = 1000
	eventsFeedMaxFetchSize  = 3000
)

var (
	defaultTimeout      = confopt.Duration(30 * time.Second)
	defaultRetryWaitMin = confopt.Duration(5 * time.Second)
	defaultRetryWaitMax = confopt.Duration(30 * time.Second)
	catoLastTimeFrameRe = regexp.MustCompile(`^last\.P[A-Z0-9]+$`)
	catoUTCTimeFrameRe  = regexp.MustCompile(`^utc\.\S+$`)
)

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`

	AccountID string `yaml:"account_id" json:"account_id"`
	APIKey    string `yaml:"api_key" json:"api_key"`

	web.HTTPConfig `yaml:",inline" json:""`

	SiteSelector      string          `yaml:"site_selector,omitempty" json:"site_selector,omitempty"`
	InterfaceSelector string          `yaml:"interface_selector,omitempty" json:"interface_selector,omitempty"`
	Limits            LimitsConfig    `yaml:"limits,omitempty" json:"limits,omitempty"`
	Discovery         DiscoveryConfig `yaml:"discovery,omitempty" json:"discovery,omitempty"`
	Metrics           MetricsConfig   `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	Events            EventsConfig    `yaml:"events,omitempty" json:"events,omitempty"`
	BGP               BGPConfig       `yaml:"bgp,omitempty" json:"bgp,omitempty"`
	Topology          TopologyConfig  `yaml:"topology,omitempty" json:"topology,omitempty"`
	Retry             RetryConfig     `yaml:"retry,omitempty" json:"retry,omitempty"`
}

type DiscoveryConfig struct {
	RefreshEvery int `yaml:"refresh_every,omitempty" json:"refresh_every,omitempty"`
	PageLimit    int `yaml:"page_limit,omitempty" json:"page_limit,omitempty"`
}

type MetricsConfig struct {
	Enabled          confopt.AutoBool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	TimeFrame        string           `yaml:"time_frame,omitempty" json:"time_frame,omitempty"`
	Buckets          int64            `yaml:"buckets,omitempty" json:"buckets,omitempty"`
	GroupInterfaces  confopt.AutoBool `yaml:"group_interfaces,omitempty" json:"group_interfaces,omitempty"`
	MaxSitesPerQuery int              `yaml:"max_sites_per_query,omitempty" json:"max_sites_per_query,omitempty"`
}

type LimitsConfig struct {
	MaxSites             *int `yaml:"max_sites,omitempty" json:"max_sites,omitempty"`
	MaxInterfacesPerSite *int `yaml:"max_interfaces_per_site,omitempty" json:"max_interfaces_per_site,omitempty"`
}

type EventsConfig struct {
	Enabled          confopt.AutoBool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	MarkerFile       string           `yaml:"marker_file,omitempty" json:"marker_file,omitempty"`
	MaxPagesPerCycle int              `yaml:"max_pages_per_cycle,omitempty" json:"max_pages_per_cycle,omitempty"`
	MaxCardinality   int              `yaml:"max_cardinality,omitempty" json:"max_cardinality,omitempty"`
}

type BGPConfig struct {
	Enabled               confopt.AutoBool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	RefreshEvery          int              `yaml:"refresh_every,omitempty" json:"refresh_every,omitempty"`
	MaxSitesPerCollection int              `yaml:"max_sites_per_collection,omitempty" json:"max_sites_per_collection,omitempty"`
	PeerSelector          string           `yaml:"peer_selector,omitempty" json:"peer_selector,omitempty"`
	MaxPeersPerSite       *int             `yaml:"max_peers_per_site,omitempty" json:"max_peers_per_site,omitempty"`
}

type TopologyConfig struct {
	Enabled confopt.AutoBool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

type RetryConfig struct {
	Attempts int              `yaml:"attempts,omitempty" json:"attempts,omitempty"`
	WaitMin  confopt.Duration `yaml:"wait_min,omitempty" json:"wait_min,omitempty"`
	WaitMax  confopt.Duration `yaml:"wait_max,omitempty" json:"wait_max,omitempty"`
}

func (c *Config) applyDefaults() {
	c.AccountID = strings.TrimSpace(c.AccountID)
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.URL = strings.TrimSpace(c.URL)
	c.SiteSelector = strings.TrimSpace(c.SiteSelector)
	c.InterfaceSelector = strings.TrimSpace(c.InterfaceSelector)
	c.Metrics.TimeFrame = strings.TrimSpace(c.Metrics.TimeFrame)
	c.BGP.PeerSelector = strings.TrimSpace(c.BGP.PeerSelector)

	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.URL == "" {
		c.URL = defaultEndpoint
	}
	if c.SiteSelector == "" {
		c.SiteSelector = defaultEntitySelector
	}
	if c.InterfaceSelector == "" {
		c.InterfaceSelector = defaultEntitySelector
	}
	if c.Limits.MaxSites == nil {
		c.Limits.MaxSites = intPtr(defaultMaxSites)
	}
	if c.Limits.MaxInterfacesPerSite == nil {
		c.Limits.MaxInterfacesPerSite = intPtr(defaultMaxIfacesPerSite)
	}
	if c.Timeout.Duration() == 0 {
		c.Timeout = defaultTimeout
	}
	if c.Discovery.RefreshEvery <= 0 {
		c.Discovery.RefreshEvery = defaultDiscoveryEvery
	}
	if c.Discovery.PageLimit <= 0 {
		c.Discovery.PageLimit = defaultDiscoveryLimit
	}
	if c.Metrics.TimeFrame == "" {
		c.Metrics.TimeFrame = defaultMetricsTimeFrame
	}
	if c.Metrics.Buckets <= 0 {
		c.Metrics.Buckets = defaultMetricsBuckets
	}
	if c.Metrics.MaxSitesPerQuery <= 0 {
		c.Metrics.MaxSitesPerQuery = defaultMaxSitesPerQuery
	}
	if c.BGP.RefreshEvery <= 0 {
		c.BGP.RefreshEvery = defaultBGPRefreshEvery
	}
	if c.BGP.MaxSitesPerCollection <= 0 {
		c.BGP.MaxSitesPerCollection = defaultBGPMaxSites
	}
	if c.BGP.PeerSelector == "" {
		c.BGP.PeerSelector = defaultEntitySelector
	}
	if c.BGP.MaxPeersPerSite == nil {
		c.BGP.MaxPeersPerSite = intPtr(defaultBGPMaxPeers)
	}
	if c.Events.MaxPagesPerCycle <= 0 {
		c.Events.MaxPagesPerCycle = defaultEventsMaxPages
	}
	if c.Events.MaxCardinality <= 0 {
		c.Events.MaxCardinality = defaultEventsMaxSeries
	}
	if c.Retry.Attempts <= 0 {
		c.Retry.Attempts = defaultRetryAttempts
	}
	if c.Retry.WaitMin.Duration() == 0 {
		c.Retry.WaitMin = defaultRetryWaitMin
	}
	if c.Retry.WaitMax.Duration() == 0 {
		c.Retry.WaitMax = defaultRetryWaitMax
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
	if !validCatoTimeFrame(c.Metrics.TimeFrame) {
		errs = append(errs, errors.New("'metrics.time_frame' must use a Cato TimeFrame value such as 'last.PT5M' or 'utc.2020-02-11/{04:50:15--16:50:15}'"))
	}
	if c.Discovery.RefreshEvery < 60 {
		errs = append(errs, errors.New("'discovery.refresh_every' must be >= 60 seconds"))
	}
	if c.Discovery.PageLimit < 1 || c.Discovery.PageLimit > 1000 {
		errs = append(errs, errors.New("'discovery.page_limit' must be between 1 and 1000"))
	}
	if c.Metrics.Buckets < 1 || c.Metrics.Buckets > 1000 {
		errs = append(errs, errors.New("'metrics.buckets' must be between 1 and 1000"))
	}
	if c.Metrics.MaxSitesPerQuery < 1 || c.Metrics.MaxSitesPerQuery > 1000 {
		errs = append(errs, errors.New("'metrics.max_sites_per_query' must be between 1 and 1000"))
	}
	if c.maxSitesLimit() < 0 || c.maxSitesLimit() > 100000 {
		errs = append(errs, errors.New("'limits.max_sites' must be between 0 and 100000"))
	}
	if c.maxInterfacesPerSiteLimit() < 0 || c.maxInterfacesPerSiteLimit() > 10000 {
		errs = append(errs, errors.New("'limits.max_interfaces_per_site' must be between 0 and 10000"))
	}
	if c.BGP.RefreshEvery < 60 {
		errs = append(errs, errors.New("'bgp.refresh_every' must be >= 60 seconds"))
	}
	if c.BGP.MaxSitesPerCollection < 1 || c.BGP.MaxSitesPerCollection > 1000 {
		errs = append(errs, errors.New("'bgp.max_sites_per_collection' must be between 1 and 1000"))
	}
	if c.bgpMaxPeersPerSiteLimit() < 0 || c.bgpMaxPeersPerSiteLimit() > 10000 {
		errs = append(errs, errors.New("'bgp.max_peers_per_site' must be between 0 and 10000"))
	}
	if c.Events.MaxPagesPerCycle < 1 || c.Events.MaxPagesPerCycle > 100 {
		errs = append(errs, errors.New("'events.max_pages_per_cycle' must be between 1 and 100"))
	}
	if c.Events.MaxCardinality < 1 || c.Events.MaxCardinality > 10000 {
		errs = append(errs, errors.New("'events.max_cardinality' must be between 1 and 10000"))
	}
	if c.Retry.Attempts < 1 || c.Retry.Attempts > 10 {
		errs = append(errs, errors.New("'retry.attempts' must be between 1 and 10"))
	}
	if c.Retry.WaitMin.Duration() < 0 || c.Retry.WaitMax.Duration() < 0 {
		errs = append(errs, errors.New("'retry.wait_min' and 'retry.wait_max' cannot be negative"))
	}
	if c.Retry.WaitMax.Duration() < c.Retry.WaitMin.Duration() {
		errs = append(errs, fmt.Errorf("'retry.wait_max' must be greater than or equal to 'retry.wait_min'"))
	}

	return errors.Join(errs...)
}

func validCatoTimeFrame(v string) bool {
	v = strings.TrimSpace(v)
	return catoLastTimeFrameRe.MatchString(v) || catoUTCTimeFrameRe.MatchString(v)
}

func (c Config) metricsEnabled() bool {
	return c.Metrics.Enabled.Bool(true)
}

func (c Config) eventsEnabled() bool {
	return c.Events.Enabled.Bool(true)
}

func (c Config) bgpEnabled() bool {
	return c.BGP.Enabled.Bool(true)
}

func (c Config) topologyEnabled() bool {
	return c.Topology.Enabled.Bool(true)
}

func (c Config) groupInterfaces() *bool {
	return c.Metrics.GroupInterfaces.ToBool()
}

func (c Config) maxSitesLimit() int {
	if c.Limits.MaxSites == nil {
		return defaultMaxSites
	}
	return *c.Limits.MaxSites
}

func (c Config) maxInterfacesPerSiteLimit() int {
	if c.Limits.MaxInterfacesPerSite == nil {
		return defaultMaxIfacesPerSite
	}
	return *c.Limits.MaxInterfacesPerSite
}

func (c Config) bgpMaxPeersPerSiteLimit() int {
	if c.BGP.MaxPeersPerSite == nil {
		return defaultBGPMaxPeers
	}
	return *c.BGP.MaxPeersPerSite
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func intPtr(v int) *int { return &v }
