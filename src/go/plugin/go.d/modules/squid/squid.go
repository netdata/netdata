// SPDX-License-Identifier: GPL-3.0-or-later

package squid

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("squid", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Squid {
	return &Squid{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:3128",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second * 1),
				},
			},
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
}

type Squid struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (sq *Squid) Configuration() any {
	return sq.Config
}

func (sq *Squid) Init() error {
	if sq.URL == "" {
		sq.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(sq.Client)
	if err != nil {
		sq.Error(err)
		return err
	}
	sq.httpClient = client

	sq.Debugf("using URL %s", sq.URL)
	sq.Debugf("using timeout: %s", sq.Timeout)

	return nil
}

func (sq *Squid) Check() error {
	mx, err := sq.collect()
	if err != nil {
		sq.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (sq *Squid) Charts() *module.Charts {
	return sq.charts
}

func (sq *Squid) Collect() map[string]int64 {
	mx, err := sq.collect()
	if err != nil {
		sq.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (sq *Squid) Cleanup() {
	if sq.httpClient != nil {
		sq.httpClient.CloseIdleConnections()
	}
}
