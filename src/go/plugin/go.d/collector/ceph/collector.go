// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/ceph/cephfunc"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("ceph", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: cephfunc.Methods,
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			c, ok := job.Collector().(*Collector)
			if !ok {
				return nil
			}
			return c.funcRouter
		},
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			OSDSelector:  "*",
			PoolSelector: "*",
			MaxOSDs:      100,
			MaxPools:     100,
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://127.0.0.1:8443",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 2),
					TLSConfig: tlscfg.TLSConfig{
						InsecureSkipVerify: true,
					},
				},
			},
		},
		Metrics: CollectConfig{
			DashboardAPIStatus: true, HealthStatus: true, Hosts: true, Monitors: true,
			OSDsSummary: true, Managers: true, ObjectGateways: true, ISCSIGateways: true,
			Capacity: true, Objects: true, PoolsSummary: true, PGs: true,
			ClientIO: true, Recovery: true, ScrubStatus: true, OSDs: true, Pools: true,
		},
		Functions: FunctionsConfig{
			Health:  FunctionConfig{Limit: 500},
			OSDs:    FunctionConfig{Limit: 500},
			Pools:   FunctionConfig{Limit: 500},
			Daemons: FunctionConfig{Limit: 500},
		},
		charts:      &collectorapi.Charts{},
		osdMatcher:  matcher.TRUE(),
		poolMatcher: matcher.TRUE(),
		seenPools:   make(map[string]*entityState),
		seenOsds:    make(map[string]*entityState),
		now:         time.Now,
	}
}

type Config struct {
	Vnode                  string   `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery            int      `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry     int      `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	FunctionOnly           bool     `yaml:"function_only,omitempty" json:"function_only"`
	AllowedRedirectOrigins []string `yaml:"allowed_redirect_origins,omitempty" json:"allowed_redirect_origins"`
	OSDSelector            string   `yaml:"osd_selector,omitempty" json:"osd_selector"`
	MaxOSDs                int      `yaml:"max_osds,omitempty" json:"max_osds"`
	PoolSelector           string   `yaml:"pool_selector,omitempty" json:"pool_selector"`
	MaxPools               int      `yaml:"max_pools,omitempty" json:"max_pools"`
	web.HTTPConfig         `yaml:",inline" json:""`
}

type CollectConfig struct {
	DashboardAPIStatus bool `yaml:"dashboard_api_status" json:"dashboard_api_status"`
	HealthStatus       bool `yaml:"health_status" json:"health_status"`
	Hosts              bool `yaml:"hosts" json:"hosts"`
	Monitors           bool `yaml:"monitors" json:"monitors"`
	OSDsSummary        bool `yaml:"osds_summary" json:"osds_summary"`
	Managers           bool `yaml:"managers" json:"managers"`
	ObjectGateways     bool `yaml:"object_gateways" json:"object_gateways"`
	ISCSIGateways      bool `yaml:"iscsi_gateways" json:"iscsi_gateways"`
	Capacity           bool `yaml:"capacity" json:"capacity"`
	Objects            bool `yaml:"objects" json:"objects"`
	PoolsSummary       bool `yaml:"pools_summary" json:"pools_summary"`
	PGs                bool `yaml:"pgs" json:"pgs"`
	ClientIO           bool `yaml:"client_io" json:"client_io"`
	Recovery           bool `yaml:"recovery" json:"recovery"`
	ScrubStatus        bool `yaml:"scrub_status" json:"scrub_status"`
	OSDs               bool `yaml:"osds" json:"osds"`
	Pools              bool `yaml:"pools" json:"pools"`
}

func (c CollectConfig) anyEnabled() bool {
	return c.DashboardAPIStatus || c.anyHealthEnabled() || c.OSDs || c.Pools
}

func (c CollectConfig) anyHealthEnabled() bool {
	return c.HealthStatus || c.Hosts || c.Monitors || c.OSDsSummary || c.Managers ||
		c.ObjectGateways || c.ISCSIGateways || c.Capacity || c.Objects || c.PoolsSummary ||
		c.PGs || c.ClientIO || c.Recovery || c.ScrubStatus
}

type FunctionsConfig struct {
	Health       FunctionConfig          `yaml:"health" json:"health"`
	OSDs         FunctionConfig          `yaml:"osds" json:"osds"`
	Pools        FunctionConfig          `yaml:"pools" json:"pools"`
	Daemons      FunctionConfig          `yaml:"daemons" json:"daemons"`
	RGWMultisite FunctionConfig          `yaml:"rgw_multisite" json:"rgw_multisite"`
	RGWQuotas    RGWQuotasFunctionConfig `yaml:"rgw_quotas" json:"rgw_quotas"`
}

type FunctionConfig struct {
	Disabled bool             `yaml:"disabled" json:"disabled"`
	Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Limit    int              `yaml:"limit,omitempty" json:"limit"`
}

type RGWQuotasFunctionConfig struct {
	FunctionConfig `yaml:",inline" json:""`
	Users          []string `yaml:"users,omitempty" json:"users"`
	Buckets        []string `yaml:"buckets,omitempty" json:"buckets"`
	Accounts       []string `yaml:"accounts,omitempty" json:"accounts"`
}

type entityState struct {
	lastSeen time.Time
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`
	// Staged internal gates are removed with the periodic-runtime rewrite.
	Metrics   CollectConfig   `yaml:"-" json:"-"`
	Functions FunctionsConfig `yaml:"-" json:"-"`

	charts               *collectorapi.Charts
	addClusterChartsOnce sync.Once

	httpClient *http.Client
	apiClient  *cephClient
	funcRouter funcapi.MethodHandler

	identityMu sync.RWMutex
	fsid       string // a unique identifier for the cluster

	osdMatcher  matcher.Matcher
	poolMatcher matcher.Matcher

	seenPools map[string]*entityState
	seenOsds  map[string]*entityState
	now       func() time.Time
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	osdMatcher, err := matcher.NewSimplePatternsMatcher(c.OSDSelector)
	if err != nil {
		return fmt.Errorf("invalid osd_selector: %v", err)
	}
	poolMatcher, err := matcher.NewSimplePatternsMatcher(c.PoolSelector)
	if err != nil {
		return fmt.Errorf("invalid pool_selector: %v", err)
	}
	c.osdMatcher = osdMatcher
	c.poolMatcher = poolMatcher

	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return fmt.Errorf("create http client: %v", err)
	}
	c.httpClient = httpClient

	apiClient, err := newCephClient(httpClient, c.RequestConfig, c.NotFollowRedirect, c.AllowedRedirectOrigins)
	if err != nil {
		return fmt.Errorf("create Ceph client: %v", err)
	}
	c.apiClient = apiClient
	c.funcRouter = cephfunc.NewRouter(funcDepsAdapter{collector: c}, c.functionRouterConfig())

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	_, err := c.probeClusterIdentity(ctx)
	return err
}

func (c *Collector) Charts() *collectorapi.Charts {
	return c.charts
}

func (c *Collector) Collect(ctx context.Context) map[string]int64 {
	if c.FunctionOnly {
		return nil
	}

	mx, err := c.collect(ctx)
	if err != nil {
		c.Limit("collection", 1, time.Minute).Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

func (c *Collector) clusterFSID() string {
	c.identityMu.RLock()
	defer c.identityMu.RUnlock()
	return c.fsid
}

func (c *Collector) FunctionAvailable(functionID string) bool {
	switch functionID {
	case cephfunc.MethodHealth, cephfunc.MethodOSDs, cephfunc.MethodPools, cephfunc.MethodDaemons:
		return true
	default:
		return false
	}
}

func (c *Collector) functionRouterConfig() cephfunc.Config {
	inheritedTimeout := c.Timeout.Duration()
	method := func(cfg FunctionConfig) cephfunc.MethodConfig {
		timeout := cfg.Timeout.Duration()
		if timeout <= 0 {
			timeout = inheritedTimeout
		}
		return cephfunc.MethodConfig{Disabled: cfg.Disabled, Timeout: timeout, Limit: cfg.Limit}
	}
	return cephfunc.Config{
		Health:  method(c.Functions.Health),
		OSDs:    method(c.Functions.OSDs),
		Pools:   method(c.Functions.Pools),
		Daemons: method(c.Functions.Daemons),
	}
}
