// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

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
	module.Register("tomcat", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Tomcat {
	return &Tomcat{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8080/manager/status?XML=true",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second * 1),
				},
			},
		},
		charts: &module.Charts{},
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
}

type Tomcat struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (tc *Tomcat) Configuration() any {
	return tc.Config
}

func (tc *Tomcat) Init() error {
	if tc.URL == "" {
		tc.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(tc.Client)
	if err != nil {
		tc.Error(err)
		return err
	}
	tc.httpClient = client

	tc.Debugf("using URL %s", tc.URL)
	tc.Debugf("using timeout: %s", tc.Timeout)

	return nil
}

func (tc *Tomcat) Check() error {
	mx, err := tc.collect()
	if err != nil {
		tc.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (tc *Tomcat) Charts() *module.Charts {
	return tc.charts
}

func (tc *Tomcat) Collect() map[string]int64 {
	mx, err := tc.collect()
	if err != nil {
		tc.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (tc *Tomcat) Cleanup() {
	if tc.httpClient != nil {
		tc.httpClient.CloseIdleConnections()
	}
}
