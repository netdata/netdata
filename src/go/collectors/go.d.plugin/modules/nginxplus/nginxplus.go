// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import (
	_ "embed"
	"net/http"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("nginxplus", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *NginxPlus {
	return &NginxPlus{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second * 1},
				},
			},
		},
		charts:              baseCharts.Copy(),
		queryEndpointsEvery: time.Minute,
		cache:               newCache(),
	}
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type NginxPlus struct {
	module.Base
	Config `yaml:",inline"`

	charts *module.Charts

	httpClient *http.Client

	apiVersion int64

	endpoints struct {
		nginx             bool
		connections       bool
		ssl               bool
		httpCaches        bool
		httpRequest       bool
		httpServerZones   bool
		httpLocationZones bool
		httpUpstreams     bool
		streamServerZones bool
		streamUpstreams   bool
		resolvers         bool
	}
	queryEndpointsTime  time.Time
	queryEndpointsEvery time.Duration

	cache *cache
}

func (n *NginxPlus) Init() bool {
	if n.URL == "" {
		n.Error("config validation: 'url' can not be empty'")
		return false
	}

	client, err := web.NewHTTPClient(n.Client)
	if err != nil {
		n.Errorf("init HTTP client: %v", err)
		return false
	}
	n.httpClient = client

	return true
}

func (n *NginxPlus) Check() bool {
	return len(n.Collect()) > 0
}

func (n *NginxPlus) Charts() *module.Charts {
	return n.charts
}

func (n *NginxPlus) Collect() map[string]int64 {
	mx, err := n.collect()

	if err != nil {
		n.Error(err)
		return nil
	}

	return mx
}

func (n *NginxPlus) Cleanup() {
	if n.httpClient != nil {
		n.httpClient.CloseIdleConnections()
	}
}
