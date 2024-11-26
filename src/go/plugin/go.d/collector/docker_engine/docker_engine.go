// SPDX-License-Identifier: GPL-3.0-or-later

package docker_engine

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("docker_engine", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *DockerEngine {
	return &DockerEngine{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:9323/metrics",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
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
		return fmt.Errorf("config validation: %v", err)
	}

	prom, err := de.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("init prometheus client: %v", err)
	}
	de.prom = prom

	return nil
}

func (de *DockerEngine) Check() error {
	mx, err := de.collect()
	if err != nil {
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
