// SPDX-License-Identifier: GPL-3.0-or-later

package logstash

import (
	_ "embed"
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
					Timeout: web.Duration{Duration: time.Second},
				},
			},
		},
		charts:    charts.Copy(),
		pipelines: make(map[string]bool),
	}
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type Logstash struct {
	module.Base
	Config     `yaml:",inline"`
	httpClient *http.Client
	charts     *module.Charts
	pipelines  map[string]bool
}

func (l *Logstash) Init() bool {
	if l.URL == "" {
		l.Error("config validation: 'url' cannot be empty")
		return false
	}

	httpClient, err := web.NewHTTPClient(l.Client)
	if err != nil {
		l.Errorf("init HTTP client: %v", err)
		return false
	}
	l.httpClient = httpClient

	l.Debugf("using URL %s", l.URL)
	l.Debugf("using timeout: %s", l.Timeout.Duration)
	return true
}

func (l *Logstash) Check() bool {
	return len(l.Collect()) > 0
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
