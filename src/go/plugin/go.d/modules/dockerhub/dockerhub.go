// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("dockerhub", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *DockerHub {
	return &DockerHub{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://hub.docker.com/v2/repositories",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 2),
				},
			},
		},
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	Repositories   []string `yaml:"repositories" json:"repositories"`
}

type DockerHub struct {
	module.Base
	Config `yaml:",inline" json:""`

	client *apiClient
}

func (dh *DockerHub) Configuration() any {
	return dh.Config
}

func (dh *DockerHub) Init() error {
	if err := dh.validateConfig(); err != nil {
		dh.Errorf("config validation: %v", err)
		return err
	}

	client, err := dh.initApiClient()
	if err != nil {
		dh.Error(err)
		return err
	}
	dh.client = client

	return nil
}

func (dh *DockerHub) Check() error {
	mx, err := dh.collect()
	if err != nil {
		dh.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (dh *DockerHub) Charts() *Charts {
	cs := charts.Copy()
	addReposToCharts(dh.Repositories, cs)
	return cs
}

func (dh *DockerHub) Collect() map[string]int64 {
	mx, err := dh.collect()

	if err != nil {
		dh.Error(err)
		return nil
	}

	return mx
}

func (dh *DockerHub) Cleanup() {
	if dh.client != nil && dh.client.httpClient != nil {
		dh.client.httpClient.CloseIdleConnections()
	}
}
