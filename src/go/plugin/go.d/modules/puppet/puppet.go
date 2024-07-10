// SPDX-License-Identifier: GPL-3.0-or-later

package puppet

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
	module.Register("puppet", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Puppet {
	return &Puppet{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "https://127.0.0.1:8140",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second * 1),
				},
			},
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
}

type Puppet struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (ppt *Puppet) Configuration() any {
	return ppt.Config
}

func (ppt *Puppet) Init() error {
	if ppt.URL == "" {
		ppt.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(ppt.Client)
	if err != nil {
		ppt.Error(err)
		return err
	}
	ppt.httpClient = client

	ppt.Debugf("using URL %s", ppt.URL)
	ppt.Debugf("using timeout: %s", ppt.Timeout)

	return nil
}

func (ppt *Puppet) Check() error {
	mx, err := ppt.collect()
	if err != nil {
		ppt.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (ppt *Puppet) Charts() *module.Charts {
	return ppt.charts
}

func (ppt *Puppet) Collect() map[string]int64 {
	mx, err := ppt.collect()
	if err != nil {
		ppt.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (ppt *Puppet) Cleanup() {
	if ppt.httpClient != nil {
		ppt.httpClient.CloseIdleConnections()
	}
}
