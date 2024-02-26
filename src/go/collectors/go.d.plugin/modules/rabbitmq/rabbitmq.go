// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

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
	module.Register("rabbitmq", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *RabbitMQ {
	return &RabbitMQ{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL:      "http://localhost:15672",
					Username: "guest",
					Password: "guest",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second},
				},
			},
			CollectQueues: false,
		},
		charts: baseCharts.Copy(),
		vhosts: make(map[string]bool),
		queues: make(map[string]queueCache),
	}
}

type Config struct {
	web.HTTP      `yaml:",inline"`
	CollectQueues bool `yaml:"collect_queues_metrics"`
}

type (
	RabbitMQ struct {
		module.Base
		Config `yaml:",inline"`

		charts *module.Charts

		httpClient *http.Client

		nodeName string

		vhosts map[string]bool
		queues map[string]queueCache
	}
	queueCache struct {
		name, vhost string
	}
)

func (r *RabbitMQ) Init() bool {
	if r.URL == "" {
		r.Error("'url' can not be empty")
		return false
	}

	client, err := web.NewHTTPClient(r.Client)
	if err != nil {
		r.Errorf("init HTTP client: %v", err)
		return false
	}
	r.httpClient = client

	r.Debugf("using URL %s", r.URL)
	r.Debugf("using timeout: %s", r.Timeout.Duration)

	return true
}

func (r *RabbitMQ) Check() bool {
	return len(r.Collect()) > 0
}

func (r *RabbitMQ) Charts() *module.Charts {
	return r.charts
}

func (r *RabbitMQ) Collect() map[string]int64 {
	mx, err := r.collect()
	if err != nil {
		r.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (r *RabbitMQ) Cleanup() {
	if r.httpClient != nil {
		r.httpClient.CloseIdleConnections()
	}
}
