// SPDX-License-Identifier: GPL-3.0-or-later

package icecast

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
	module.Register("icecast", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Icecast {
	return &Icecast{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8000",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second * 1),
				},
			},
		},
		charts: &module.Charts{},

		seenSources: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
}

type Icecast struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	seenSources map[string]bool

	httpClient *http.Client
}

func (ic *Icecast) Configuration() any {
	return ic.Config
}

func (ic *Icecast) Init() error {
	if ic.URL == "" {
		ic.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(ic.Client)
	if err != nil {
		ic.Error(err)
		return err
	}
	ic.httpClient = client

	ic.Debugf("using URL %s", ic.URL)
	ic.Debugf("using timeout: %s", ic.Timeout)

	return nil
}

func (ic *Icecast) Check() error {
	mx, err := ic.collect()
	if err != nil {
		ic.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (ic *Icecast) Charts() *module.Charts {
	return ic.charts
}

func (ic *Icecast) Collect() map[string]int64 {
	mx, err := ic.collect()
	if err != nil {
		ic.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (ic *Icecast) Cleanup() {
	if ic.httpClient != nil {
		ic.httpClient.CloseIdleConnections()
	}
}
