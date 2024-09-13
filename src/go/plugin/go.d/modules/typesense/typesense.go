// SPDX-License-Identifier: GPL-3.0-or-later

package typesense

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("typesense", module.Creator{
		Create:          func() module.Module { return New() },
		JobConfigSchema: configSchema,
		Config:          func() any { return &Config{} },
	})
}

func New() *Typesense {
	return &Typesense{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8108",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
		charts:  baseCharts.Copy(),
		doStats: true,
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
	APIKey      string `yaml:"api_key,omitempty" json:"api_key"`
}

type Typesense struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts
	once   sync.Once

	httpClient *http.Client

	doStats bool
}

func (ts *Typesense) Configuration() any {
	return ts.Config
}

func (ts *Typesense) Init() error {
	if ts.URL == "" {
		ts.Error("typesense URL not configured")
		return errors.New("typesense URL not configured")
	}

	httpClient, err := web.NewHTTPClient(ts.Client)
	if err != nil {
		return fmt.Errorf("initialize http client: %w", err)
	}

	ts.httpClient = httpClient

	if ts.APIKey == "" {
		ts.Warning("API key not set in configuration. Only health status will be collected.")
	}
	ts.Debugf("using URL %s", ts.URL)
	ts.Debugf("using timeout: %s", ts.Timeout)

	return nil
}

func (ts *Typesense) Check() error {
	mx, err := ts.collect()
	if err != nil {
		ts.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (ts *Typesense) Charts() *module.Charts {
	return ts.charts
}

func (ts *Typesense) Collect() map[string]int64 {
	mx, err := ts.collect()
	if err != nil {
		ts.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (ts *Typesense) Cleanup() {
	if ts.httpClient != nil {
		ts.httpClient.CloseIdleConnections()
	}
}
