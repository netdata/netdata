// SPDX-License-Identifier: GPL-3.0-or-later

package logstash

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
	module.Register("logstash", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Logstash {
	return &Logstash{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://localhost:9600",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
		charts:    charts.Copy(),
		pipelines: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type Logstash struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	pipelines map[string]bool
}

func (l *Logstash) Configuration() any {
	return l.Config
}

func (l *Logstash) Init() error {
	if l.URL == "" {
		l.Error("config validation: 'url' cannot be empty")
		return errors.New("url not set")
	}

	httpClient, err := web.NewHTTPClient(l.ClientConfig)
	if err != nil {
		l.Errorf("init HTTP client: %v", err)
		return err
	}
	l.httpClient = httpClient

	l.Debugf("using URL %s", l.URL)
	l.Debugf("using timeout: %s", l.Timeout.Duration())

	return nil
}

func (l *Logstash) Check() error {
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

func (l *Logstash) Charts() *module.Charts {
	return l.charts
}

func (l *Logstash) Collect() map[string]int64 {
	mx, err := l.collect()
	if err != nil {
		l.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (l *Logstash) Cleanup() {
	if l.httpClient != nil {
		l.httpClient.CloseIdleConnections()
	}
}
