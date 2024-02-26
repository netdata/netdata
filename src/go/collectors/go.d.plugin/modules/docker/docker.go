// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("docker", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Docker {
	return &Docker{
		Config: Config{
			Address:              docker.DefaultDockerHost,
			Timeout:              web.Duration{Duration: time.Second * 5},
			CollectContainerSize: false,
		},

		charts: summaryCharts.Copy(),
		newClient: func(cfg Config) (dockerClient, error) {
			return docker.NewClientWithOpts(docker.WithHost(cfg.Address))
		},
		containers: make(map[string]bool),
	}
}

type Config struct {
	Timeout              web.Duration `yaml:"timeout"`
	Address              string       `yaml:"address"`
	CollectContainerSize bool         `yaml:"collect_container_size"`
}

type (
	Docker struct {
		module.Base
		Config `yaml:",inline"`

		charts *module.Charts

		newClient     func(Config) (dockerClient, error)
		client        dockerClient
		verNegotiated bool

		containers map[string]bool
	}
	dockerClient interface {
		NegotiateAPIVersion(context.Context)
		Info(context.Context) (types.Info, error)
		ImageList(context.Context, types.ImageListOptions) ([]types.ImageSummary, error)
		ContainerList(context.Context, types.ContainerListOptions) ([]types.Container, error)
		Close() error
	}
)

func (d *Docker) Init() bool {
	return true
}

func (d *Docker) Check() bool {
	return len(d.Collect()) > 0
}

func (d *Docker) Charts() *module.Charts {
	return d.charts
}

func (d *Docker) Collect() map[string]int64 {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (d *Docker) Cleanup() {
	if d.client == nil {
		return
	}
	if err := d.client.Close(); err != nil {
		d.Warningf("error on closing docker client: %v", err)
	}
	d.client = nil
}
