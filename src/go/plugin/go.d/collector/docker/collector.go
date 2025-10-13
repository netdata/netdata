// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/dockerhost"

	"github.com/docker/docker/api/types"
	typesContainer "github.com/docker/docker/api/types/container"
	typesImage "github.com/docker/docker/api/types/image"
	typesSystem "github.com/docker/docker/api/types/system"
	docker "github.com/docker/docker/client"
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

func New() *Collector {
	return &Collector{
		Config: Config{
			Address:              docker.DefaultDockerHost,
			Timeout:              confopt.Duration(time.Second * 2),
			ContainerSelector:    "*",
			CollectContainerSize: false,
		},

		charts: summaryCharts.Copy(),
		newClient: func(cfg Config) (dockerClient, error) {
			return docker.NewClientWithOpts(docker.WithHost(cfg.Address))
		},
		cntrSr:     matcher.TRUE(),
		containers: make(map[string]bool),
	}
}

type Config struct {
	Vnode                string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery          int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry   int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address              string           `yaml:"address" json:"address"`
	Timeout              confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	ContainerSelector    string           `yaml:"container_selector,omitempty" json:"container_selector"`
	CollectContainerSize bool             `yaml:"collect_container_size" json:"collect_container_size"`
}

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client    dockerClient
		newClient func(Config) (dockerClient, error)

		verNegotiated bool
		containers    map[string]bool
		cntrSr        matcher.Matcher
	}
	dockerClient interface {
		NegotiateAPIVersion(context.Context)
		Info(context.Context) (typesSystem.Info, error)
		ImageList(context.Context, typesImage.ListOptions) ([]typesImage.Summary, error)
		ContainerList(context.Context, typesContainer.ListOptions) ([]types.Container, error)
		Close() error
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if addr := dockerhost.FromEnv(); addr != "" && c.Address == docker.DefaultDockerHost {
		c.Infof("using docker host from environment: %s ", addr)
		c.Address = addr
	}
	if c.ContainerSelector != "" {
		sr, err := matcher.NewSimplePatternsMatcher(c.ContainerSelector)
		if err != nil {
			return err
		}
		c.cntrSr = sr
	}

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.client == nil {
		return
	}
	if err := c.client.Close(); err != nil {
		c.Warningf("error on closing docker client: %v", err)
	}
	c.client = nil
}
