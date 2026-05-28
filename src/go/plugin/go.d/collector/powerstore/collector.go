// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/powerstore/client"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("powerstore", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 30,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

const maxConcurrentAPIRequests = 10

func New() *Collector {
	store := metrix.NewCollectorStore()
	mx := newCollectorMetrics(store)

	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://127.0.0.1",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(30 * time.Second),
				},
			},
		},
		store: store,
		mx:    mx,
		sem:   make(chan struct{}, maxConcurrentAPIRequests),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	VolumeSelector     string `yaml:"volume_selector,omitempty" json:"volume_selector"`
}

type (
	Collector struct {
		collectorapi.Base
		Config `yaml:",inline" json:""`

		store metrix.CollectorStore
		mx    *collectorMetrics

		client *client.Client

		discovered      discovered
		lastDiscoveryOK bool
		runs            int

		volMatcher matcher.Matcher
		sem        chan struct{} // limits concurrent API calls
	}
	discovered struct {
		clusters    []client.Cluster
		appliances  map[string]client.Appliance
		volumes     map[string]client.Volume
		nodes       map[string]client.Node
		fcPorts     map[string]client.FcPort
		ethPorts    map[string]client.EthPort
		fileSystems map[string]client.FileSystem
		nasServers  map[string]client.NAS
		drives      map[string]client.Hardware
		hardware    []client.Hardware
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.Username == "" || c.Password == "" {
		return errors.New("config: username and password aren't set")
	}

	cli, err := client.New(c.ClientConfig, c.RequestConfig)
	if err != nil {
		return fmt.Errorf("error creating PowerStore client: %v", err)
	}
	c.client = cli

	if c.VolumeSelector != "" {
		m, err := matcher.NewSimplePatternsMatcher(c.VolumeSelector)
		if err != nil {
			return fmt.Errorf("error creating volume selector: %v", err)
		}
		c.volMatcher = m
	}

	c.Debugf("using URL %s", c.URL)
	c.Debugf("using timeout: %s", c.Timeout)

	return nil
}

func (c *Collector) volumeMatches(name string) bool {
	if c.volMatcher == nil {
		return true
	}
	return c.volMatcher.MatchString(name)
}

func (c *Collector) Check(context.Context) error {
	if err := c.client.Login(); err != nil {
		return err
	}
	// Verify API access by running discovery (no metrics written).
	return c.discovery()
}

func (c *Collector) Collect(context.Context) error {
	return c.collect()
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }

func (c *Collector) Cleanup(context.Context) {
	if c.client == nil {
		return
	}
	c.client.Logout()
}
