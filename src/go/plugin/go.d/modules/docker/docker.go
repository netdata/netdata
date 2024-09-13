// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"time"

	"github.com/docker/docker/api/types"
	typesContainer "github.com/docker/docker/api/types/container"
	typesImage "github.com/docker/docker/api/types/image"
	typesSystem "github.com/docker/docker/api/types/system"
	docker "github.com/docker/docker/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/dockerhost"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("docker", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Docker {
	return &Docker{
		Config: Config{
			Address:              docker.DefaultDockerHost,
			Timeout:              confopt.Duration(time.Second * 2),
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
	UpdateEvery          int              `yaml:"update_every,omitempty" json:"update_every"`
	Address              string           `yaml:"address" json:"address"`
	Timeout              confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	CollectContainerSize bool             `yaml:"collect_container_size" json:"collect_container_size"`
}

type (
	Docker struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client    dockerClient
		newClient func(Config) (dockerClient, error)

		verNegotiated bool
		containers    map[string]bool
	}
	dockerClient interface {
		NegotiateAPIVersion(context.Context)
		Info(context.Context) (typesSystem.Info, error)
		ImageList(context.Context, typesImage.ListOptions) ([]typesImage.Summary, error)
		ContainerList(context.Context, typesContainer.ListOptions) ([]types.Container, error)
		Close() error
	}
)

func (d *Docker) Configuration() any {
	return d.Config
}

func (d *Docker) Init() error {
	if addr := dockerhost.FromEnv(); addr != "" && d.Address == docker.DefaultDockerHost {
		d.Infof("using docker host from environment: %s ", addr)
		d.Address = addr
	}
	return nil
}

func (d *Docker) Check() error {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
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
