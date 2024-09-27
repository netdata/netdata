// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ceph", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Ceph {
	return &Ceph{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://127.0.0.1:8443",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 2),
					TLSConfig: tlscfg.TLSConfig{
						InsecureSkipVerify: true,
					},
				},
			},
		},
		charts:    &module.Charts{},
		seenPools: make(map[string]bool),
		seenOsds:  make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type Ceph struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts               *module.Charts
	addClusterChartsOnce sync.Once

	httpClient *http.Client

	token string

	fsid string // a unique identifier for the cluster

	seenPools map[string]bool
	seenOsds  map[string]bool
}

func (c *Ceph) Configuration() any {
	return c.Config
}

func (c *Ceph) Init() error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return err
	}
	c.httpClient = httpClient

	return nil
}

func (c *Ceph) Check() error {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (c *Ceph) Charts() *module.Charts {
	return c.charts
}

func (c *Ceph) Collect() map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *Ceph) Cleanup() {
	if c.httpClient != nil {
		if err := c.authLogout(); err != nil {
			c.Warningf("failed to logout: %v", err)
		}
		c.httpClient.CloseIdleConnections()
	}
}
