// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("nginx", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Nginx {
	return &Nginx{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1/stub_status",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 1),
				},
			},
		}}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type Nginx struct {
	module.Base
	Config `yaml:",inline" json:""`

	apiClient *apiClient
}

func (n *Nginx) Configuration() any {
	return n.Config
}

func (n *Nginx) Init() error {
	if n.URL == "" {
		n.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(n.ClientConfig)
	if err != nil {
		n.Error(err)
		return err
	}

	n.apiClient = newAPIClient(client, n.RequestConfig)

	n.Debugf("using URL %s", n.URL)
	n.Debugf("using timeout: %s", n.Timeout)

	return nil
}

func (n *Nginx) Check() error {
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

func (n *Nginx) Charts() *Charts {
	return charts.Copy()
}

func (n *Nginx) Collect() map[string]int64 {
	mx, err := n.collect()
	if err != nil {
		n.Error(err)
		return nil
	}

	return mx
}

func (n *Nginx) Cleanup() {
	if n.apiClient != nil && n.apiClient.httpClient != nil {
		n.apiClient.httpClient.CloseIdleConnections()
	}
}
