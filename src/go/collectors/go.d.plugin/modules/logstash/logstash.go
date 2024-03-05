// SPDX-License-Identifier: GPL-3.0-or-later

package logstash

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("logstash", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Logstash {
	return &Logstash{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://localhost:9600",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
		charts:    charts.Copy(),
		pipelines: make(map[string]bool),
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
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

	httpClient, err := web.NewHTTPClient(l.Client)
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
