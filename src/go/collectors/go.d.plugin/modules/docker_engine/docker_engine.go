// SPDX-License-Identifier: GPL-3.0-or-later

package docker_engine

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
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
	return &DockerEngine{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:9323/metrics",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type DockerEngine struct {
	module.Base
	Config `yaml:",inline" json:""`

	prom prometheus.Prometheus

	isSwarmManager     bool
	hasContainerStates bool
}

func (de *DockerEngine) Configuration() any {
	return de.Config
}

func (de *DockerEngine) Init() error {
	if err := de.validateConfig(); err != nil {
		de.Errorf("config validation: %v", err)
		return err
	}

	prom, err := de.initPrometheusClient()
	if err != nil {
		de.Error(err)
		return err
	}
	de.prom = prom

	return nil
}

func (de *DockerEngine) Check() error {
	mx, err := de.collect()
	if err != nil {
		de.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (de *DockerEngine) Charts() *Charts {
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

func (de *DockerEngine) Cleanup() {
	if de.prom != nil && de.prom.HTTPClient() != nil {
		de.prom.HTTPClient().CloseIdleConnections()
	}
}
