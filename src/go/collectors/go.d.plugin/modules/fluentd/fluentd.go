// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("fluentd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Fluentd {
	return &Fluentd{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:24220",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			}},
		activePlugins: make(map[string]bool),
		charts:        charts.Copy(),
	}
}

type Config struct {
	web.HTTP     `yaml:",inline" json:""`
	UpdateEvery  int    `yaml:"update_every" json:"update_every"`
	PermitPlugin string `yaml:"permit_plugin_id" json:"permit_plugin_id"`
}

type Fluentd struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	apiClient *apiClient

	permitPlugin  matcher.Matcher
	activePlugins map[string]bool
}

func (f *Fluentd) Configuration() any {
	return f.Config
}

func (f *Fluentd) Init() error {
	if err := f.validateConfig(); err != nil {
		f.Error(err)
		return err
	}

	pm, err := f.initPermitPluginMatcher()
	if err != nil {
		f.Error(err)
		return err
	}
	f.permitPlugin = pm

	client, err := f.initApiClient()
	if err != nil {
		f.Error(err)
		return err
	}
	f.apiClient = client

	f.Debugf("using URL %s", f.URL)
	f.Debugf("using timeout: %s", f.Timeout.Duration())

	return nil
}

func (f *Fluentd) Check() error {
	mx, err := f.collect()
	if err != nil {
		f.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (f *Fluentd) Charts() *Charts {
	return f.charts
}

func (f *Fluentd) Collect() map[string]int64 {
	mx, err := f.collect()

	if err != nil {
		f.Error(err)
		return nil
	}

	return mx
}

func (f *Fluentd) Cleanup() {
	if f.apiClient != nil && f.apiClient.httpClient != nil {
		f.apiClient.httpClient.CloseIdleConnections()
	}
}
