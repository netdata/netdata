// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("tengine", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Tengine {
	return &Tengine{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1/us",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second * 2),
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

type Tengine struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	apiClient *apiClient
}

func (t *Tengine) Configuration() any {
	return t.Config
}

func (t *Tengine) Init() error {
	if t.URL == "" {
		t.Error("url not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(t.Client)
	if err != nil {
		t.Errorf("error on creating http client : %v", err)
		return err
	}

	t.apiClient = newAPIClient(client, t.Request)

	t.Debugf("using URL: %s", t.URL)
	t.Debugf("using timeout: %s", t.Timeout)

	return nil
}

func (t *Tengine) Check() error {
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

func (t *Tengine) Charts() *module.Charts {
	return t.charts
}

func (t *Tengine) Collect() map[string]int64 {
	mx, err := t.collect()

	if err != nil {
		t.Error(err)
		return nil
	}

	return mx
}

func (t *Tengine) Cleanup() {
	if t.apiClient != nil && t.apiClient.httpClient != nil {
		t.apiClient.httpClient.CloseIdleConnections()
	}
}
