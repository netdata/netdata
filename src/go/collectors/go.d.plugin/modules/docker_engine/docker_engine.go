// SPDX-License-Identifier: GPL-3.0-or-later

package docker_engine

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("docker_engine", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *DockerEngine {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: "http://127.0.0.1:9323/metrics",
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: time.Second},
			},
		},
	}
	return &DockerEngine{
		Config: config,
	}
}

type (
	Config struct {
		web.HTTP `yaml:",inline"`
	}
	DockerEngine struct {
		module.Base
		Config `yaml:",inline"`

		prom               prometheus.Prometheus
		isSwarmManager     bool
		hasContainerStates bool
	}
)

func (de DockerEngine) validateConfig() error {
	if de.URL == "" {
		return errors.New("URL is not set")
	}
	return nil
}

func (de *DockerEngine) initClient() error {
	client, err := web.NewHTTPClient(de.Client)
	if err != nil {
		return err
	}

	de.prom = prometheus.New(client, de.Request)
	return nil
}

func (de *DockerEngine) Init() bool {
	if err := de.validateConfig(); err != nil {
		de.Errorf("config validation: %v", err)
		return false
	}
	if err := de.initClient(); err != nil {
		de.Errorf("client initialization: %v", err)
		return false
	}
	return true
}

func (de *DockerEngine) Check() bool {
	return len(de.Collect()) > 0
}

func (de DockerEngine) Charts() *Charts {
	cs := charts.Copy()
	if !de.hasContainerStates {
		if err := cs.Remove("engine_daemon_container_states_containers"); err != nil {
			de.Warning(err)
		}
	}

	if !de.isSwarmManager {
		return cs
	}

	if err := cs.Add(*swarmManagerCharts.Copy()...); err != nil {
		de.Warning(err)
	}
	return cs
}

func (de *DockerEngine) Collect() map[string]int64 {
	mx, err := de.collect()
	if err != nil {
		de.Error(err)
		return nil
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (DockerEngine) Cleanup() {}
