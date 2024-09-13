// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("tomcat", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Tomcat {
	return &Tomcat{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8080",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 1),
				},
			},
		},
		charts:         defaultCharts.Copy(),
		seenConnectors: make(map[string]bool),
		seenMemPools:   make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type Tomcat struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	seenConnectors map[string]bool
	seenMemPools   map[string]bool
}

func (t *Tomcat) Configuration() any {
	return t.Config
}

func (t *Tomcat) Init() error {
	if err := t.validateConfig(); err != nil {
		t.Errorf("config validation: %v", err)
		return err
	}

	httpClient, err := t.initHTTPClient()
	if err != nil {
		t.Errorf("init HTTP client: %v", err)
		return err
	}

	t.httpClient = httpClient

	t.Debugf("using URL %s", t.URL)
	t.Debugf("using timeout: %s", t.Timeout)

	return nil
}

func (t *Tomcat) Check() error {
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

func (t *Tomcat) Charts() *module.Charts {
	return t.charts
}

func (t *Tomcat) Collect() map[string]int64 {
	mx, err := t.collect()
	if err != nil {
		t.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (t *Tomcat) Cleanup() {
	if t.httpClient != nil {
		t.httpClient.CloseIdleConnections()
	}
}
