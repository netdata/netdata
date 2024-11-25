// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
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
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1/us",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 2),
				},
			},
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type Tengine struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (t *Tengine) Configuration() any {
	return t.Config
}

func (t *Tengine) Init() error {
	if t.URL == "" {
		return errors.New("config: url not set")
	}

	httpClient, err := web.NewHTTPClient(t.ClientConfig)
	if err != nil {
		return fmt.Errorf("error on creating http client : %v", err)
	}
	t.httpClient = httpClient

	t.Debugf("using URL: %s", t.URL)
	t.Debugf("using timeout: %s", t.Timeout)

	return nil
}

func (t *Tengine) Check() error {
	mx, err := t.collect()
	if err != nil {
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

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (t *Tengine) Cleanup() {
	if t.httpClient != nil {
		t.httpClient.CloseIdleConnections()
	}
}
