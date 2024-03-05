// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("lighttpd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Lighttpd {
	return &Lighttpd{Config: Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: "http://127.0.0.1/server-status?auto",
			},
			Client: web.Client{
				Timeout: web.Duration(time.Second * 2),
			},
		},
	}}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type Lighttpd struct {
	module.Base
	Config `yaml:",inline" json:""`

	apiClient *apiClient
}

func (l *Lighttpd) Configuration() any {
	return l.Config
}

func (l *Lighttpd) Init() error {
	if err := l.validateConfig(); err != nil {
		l.Errorf("config validation: %v", err)
		return err
	}

	client, err := l.initApiClient()
	if err != nil {
		l.Error(err)
		return err
	}
	l.apiClient = client

	l.Debugf("using URL %s", l.URL)
	l.Debugf("using timeout: %s", l.Timeout.Duration())

	return nil
}

func (l *Lighttpd) Check() error {
	mx, err := l.collect()
	if err != nil {
		l.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (l *Lighttpd) Charts() *Charts {
	return charts.Copy()
}

func (l *Lighttpd) Collect() map[string]int64 {
	mx, err := l.collect()

	if err != nil {
		l.Error(err)
		return nil
	}

	return mx
}

func (l *Lighttpd) Cleanup() {
	if l.apiClient != nil && l.apiClient.httpClient != nil {
		l.apiClient.httpClient.CloseIdleConnections()
	}
}
