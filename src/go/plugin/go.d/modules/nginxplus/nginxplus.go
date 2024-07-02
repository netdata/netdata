// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("nginxplus", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
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
					Timeout: web.Duration(time.Second * 1),
				},
			},
		},
		charts:              baseCharts.Copy(),
		queryEndpointsEvery: time.Minute,
		cache:               newCache(),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
}

type NginxPlus struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	apiVersion int64
	endpoints  struct {
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
	cache               *cache
}

func (n *NginxPlus) Configuration() any {
	return n.Config
}

func (n *NginxPlus) Init() error {
	if n.URL == "" {
		n.Error("config validation: 'url' can not be empty'")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(n.Client)
	if err != nil {
		n.Errorf("init HTTP client: %v", err)
		return err
	}
	n.httpClient = client

	return nil
}

func (n *NginxPlus) Check() error {
	mx, err := n.collect()
	if err != nil {
		n.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
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
