// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/docker/dockerfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/dockerhost"

	docker "github.com/moby/moby/client"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("docker", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
		Methods:         dockerfunc.Methods,
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			c, ok := job.Collector().(*Collector)
			if !ok {
				return nil
			}
			return c.funcRouter
		},
	})
}

func New() *Collector {
	c := &Collector{
		Config: Config{
			Address:              docker.DefaultDockerHost,
			Timeout:              confopt.Duration(time.Second * 2),
			ContainerSelector:    "*",
			CollectContainerSize: false,
		},

		charts: summaryCharts.Copy(),
		newClient: func(cfg Config) (dockerClient, error) {
			return docker.New(docker.WithHost(cfg.Address))
		},
		cntrSr:     matcher.TRUE(),
		containers: make(map[string]bool),
	}
	c.funcRouter = dockerfunc.NewRouter(funcDepsAdapter{collector: c})
	return c
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
		collectorapi.Base
		Config `yaml:",inline" json:""`

		charts *collectorapi.Charts

		client    dockerClient
		newClient func(Config) (dockerClient, error)

		funcRouter funcapi.MethodHandler

		containers map[string]bool
		cntrSr     matcher.Matcher
	}
	dockerClient interface {
		Info(context.Context, docker.InfoOptions) (docker.SystemInfoResult, error)
		ImageList(context.Context, docker.ImageListOptions) (docker.ImageListResult, error)
		ContainerList(context.Context, docker.ContainerListOptions) (docker.ContainerListResult, error)
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
	if c.funcRouter == nil {
		c.funcRouter = dockerfunc.NewRouter(funcDepsAdapter{collector: c})
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

func (c *Collector) Charts() *collectorapi.Charts {
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

func (c *Collector) Cleanup(ctx context.Context) {
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	if c.client == nil {
		return
	}
	if err := c.client.Close(); err != nil {
		c.Warningf("error on closing docker client: %v", err)
	}
	c.client = nil
}
