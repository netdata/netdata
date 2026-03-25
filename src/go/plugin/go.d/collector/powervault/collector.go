// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/powervault/client"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("powervault", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 30,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

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
			AuthDigest: "sha256",
		},
		store: store,
		mx:    mx,
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	AuthDigest         string `yaml:"auth_digest,omitempty" json:"auth_digest"`
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
	}
	discovered struct {
		system      []client.SystemInfo
		controllers []client.Controller
		drives      []client.Drive
		fans        []client.Fan
		psus        []client.PowerSupply
		sensors     []client.Sensor
		frus        []client.FRU
		volumes     map[string]client.Volume // keyed by volume-name
		pools       []client.Pool
		ports       []client.Port
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.Username == "" || c.Password == "" {
		return errors.New("config: username and password aren't set")
	}

	digest := c.AuthDigest
	if digest == "" {
		digest = "sha256"
	}
	if digest != "sha256" && digest != "md5" {
		return fmt.Errorf("config: auth_digest must be 'sha256' or 'md5', got %q", digest)
	}

	cli, err := client.New(c.ClientConfig, c.RequestConfig, digest)
	if err != nil {
		return fmt.Errorf("error creating PowerVault client: %v", err)
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
	c.Debugf("using auth digest: %s", digest)

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
	if err := c.client.SetLocale(); err != nil {
		c.Warningf("error setting locale: %v", err)
	}
	return c.discovery()
}

func (c *Collector) Collect(context.Context) error {
	return c.collect()
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }

func (c *Collector) Cleanup(context.Context) {}
