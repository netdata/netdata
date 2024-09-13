// SPDX-License-Identifier: GPL-3.0-or-later

package traefik

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("traefik", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Traefik {
	return &Traefik{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8082/metrics",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},

		charts:       &module.Charts{},
		checkMetrics: true,
		cache: &cache{
			entrypoints: make(map[string]*cacheEntrypoint),
		},
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type (
	Traefik struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		prom prometheus.Prometheus

		checkMetrics bool
		cache        *cache
	}
	cache struct {
		entrypoints map[string]*cacheEntrypoint
	}
	cacheEntrypoint struct {
		name, proto     string
		requests        *module.Chart
		reqDur          *module.Chart
		reqDurData      map[string]cacheEntrypointReqDur
		openConn        *module.Chart
		openConnMethods map[string]bool
	}
	cacheEntrypointReqDur struct {
		prev, cur struct{ reqs, secs float64 }
		seen      bool
	}
)

func (t *Traefik) Configuration() any {
	return t.Config
}

func (t *Traefik) Init() error {
	if err := t.validateConfig(); err != nil {
		t.Errorf("config validation: %v", err)
		return err
	}

	prom, err := t.initPrometheusClient()
	if err != nil {
		t.Errorf("prometheus client initialization: %v", err)
		return err
	}
	t.prom = prom

	return nil
}

func (t *Traefik) Check() error {
	mx, err := t.collect()
	if err != nil {
		t.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (t *Traefik) Charts() *module.Charts {
	return t.charts
}

func (t *Traefik) Collect() map[string]int64 {
	mx, err := t.collect()
	if err != nil {
		t.Error(err)
		return nil
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (t *Traefik) Cleanup() {}
