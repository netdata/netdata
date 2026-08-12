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
		charts:      &collectorapi.Charts{},
		osdMatcher:  matcher.TRUE(),
		poolMatcher: matcher.TRUE(),
		seenPools:   make(map[string]*entityState),
		seenOsds:    make(map[string]*entityState),
		now:         time.Now,
	}
}

type Config struct {
	Vnode                  string   `yaml:"vnode,omitempty"                    json:"vnode"`
	UpdateEvery            int      `yaml:"update_every,omitempty"             json:"update_every"`
	AutoDetectionRetry     int      `yaml:"autodetection_retry,omitempty"      json:"autodetection_retry"`
	FunctionOnly           bool     `yaml:"function_only,omitempty"            json:"function_only"`
	AllowedRedirectOrigins []string `yaml:"allowed_redirect_origins,omitempty" json:"allowed_redirect_origins"`
	OSDSelector            string   `yaml:"osd_selector,omitempty"             json:"osd_selector"`
	MaxOSDs                int      `yaml:"max_osds,omitempty"                 json:"max_osds"`
	PoolSelector           string   `yaml:"pool_selector,omitempty"            json:"pool_selector"`
	MaxPools               int      `yaml:"max_pools,omitempty"                json:"max_pools"`
	web.HTTPConfig         `         yaml:",inline"                            json:""`
}

type entityState struct {
	lastSeen time.Time
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

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
	c.funcRouter = cephfunc.NewRouter(funcDepsAdapter{
		collector: c,
	})

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	_, err := c.probeClusterIdentity(ctx)
	if err == nil && !c.FunctionOnly {
		c.addClusterChartsOnce.Do(c.addClusterCharts)
	}
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
