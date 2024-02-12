// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

const (
	defaultURL         = "https://hub.docker.com/v2/repositories"
	defaultHTTPTimeout = time.Second * 2

	defaultUpdateEvery = 5
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("dockerhub", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		Create: func() module.Module { return New() },
	})
}

// New creates DockerHub with default values.
func New() *DockerHub {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: defaultURL,
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: defaultHTTPTimeout},
			},
		},
	}
	return &DockerHub{
		Config: config,
	}
}

// Config is the DockerHub module configuration.
type Config struct {
	web.HTTP     `yaml:",inline"`
	Repositories []string
}

// DockerHub DockerHub module.
type DockerHub struct {
	module.Base
	Config `yaml:",inline"`
	client *apiClient
}

// Cleanup makes cleanup.
func (DockerHub) Cleanup() {}

// Init makes initialization.
func (dh *DockerHub) Init() bool {
	if dh.URL == "" {
		dh.Error("URL not set")
		return false
	}

	if len(dh.Repositories) == 0 {
		dh.Error("repositories parameter is not set")
		return false
	}

	client, err := web.NewHTTPClient(dh.Client)
	if err != nil {
		dh.Errorf("error on creating http client : %v", err)
		return false
	}
	dh.client = newAPIClient(client, dh.Request)

	return true
}

// Check makes check.
func (dh DockerHub) Check() bool {
	return len(dh.Collect()) > 0
}

// Charts creates Charts.
func (dh DockerHub) Charts() *Charts {
	cs := charts.Copy()
	addReposToCharts(dh.Repositories, cs)
	return cs
}

// Collect collects metrics.
func (dh *DockerHub) Collect() map[string]int64 {
	mx, err := dh.collect()

	if err != nil {
		dh.Error(err)
		return nil
	}

	return mx
}
