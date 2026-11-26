// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/docker/dockerfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/dockerhost"

	docker "github.com/moby/moby/client"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("docker", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			// Docker Engine 29+ with the containerd image store makes the container list expensive for the daemon.
			UpdateEvery: 10,
		},
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: dockerfunc.Methods,
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

		newClient: func(cfg Config) (dockerClient, error) {
			return docker.New(docker.WithHost(cfg.Address))
		},
		cntrSr: matcher.TRUE(),
		now:    time.Now,
		store:  metrix.NewCollectorStore(),
	}
	c.funcRouter = dockerfunc.NewRouter(funcDepsAdapter{
		collector: c,
	})
	return c
}

type Config struct {
	Vnode                string           `yaml:"vnode,omitempty"               json:"vnode"`
	UpdateEvery          int              `yaml:"update_every,omitempty"        json:"update_every"`
	AutoDetectionRetry   int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address              string           `yaml:"address"                       json:"address"`
	Timeout              confopt.Duration `yaml:"timeout,omitempty"             json:"timeout"`
	ContainerSelector    string           `yaml:"container_selector,omitempty"  json:"container_selector"`
	CollectContainerSize bool             `yaml:"collect_container_size"        json:"collect_container_size"`
}

type (
	Collector struct {
		collectorapi.Base
		Config `yaml:",inline" json:""`

		client    dockerClient
		newClient func(Config) (dockerClient, error)

		funcRouter funcapi.MethodHandler

		cntrSr matcher.Matcher

		now               func() time.Time
		images            imagesSnapshot
		imagesNextRefresh time.Time

		store metrix.CollectorStore
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
		c.funcRouter = dockerfunc.NewRouter(funcDepsAdapter{
			collector: c,
		})
	}

	return nil
}

// Check makes the Docker API calls of a collection, so a daemon or socket proxy that refuses any of them fails
// detection. The image list it fetches is cached for the following collections.
func (c *Collector) Check(ctx context.Context) error {
	_, err := c.queryDaemon(ctx)
	return err
}

func (c *Collector) Collect(ctx context.Context) error {
	return c.collect(ctx)
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

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
