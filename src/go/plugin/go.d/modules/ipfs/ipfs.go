// SPDX-License-Identifier: GPL-3.0-or-later

package ipfs

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ipfs", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *IPFS {
	return &IPFS{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:5001",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second * 1),
				},
			},
			Repoapi: false,
			Pinapi: false,
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
	Pinapi bool `yaml:"pinapi" json:"pinapi"`
	Repoapi bool `yaml:"repoapi" json:"repoapi"`
}

type IPFS struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (i *IPFS) Configuration() any {
	return i.Config
}

func (i *IPFS) Init() error {
	if i.URL == "" {
		i.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(i.Client)
	if err != nil {
		i.Error(err)
		return err
	}
	i.httpClient = client

	i.Debugf("using URL %s", i.URL)
	i.Debugf("using timeout: %s", i.Timeout)

	return nil
}

func (i *IPFS) Check() error {
	mx, err := i.collect()
	if err != nil {
		i.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (i *IPFS) Charts() *module.Charts {
	return i.charts
}

func (i *IPFS) Collect() map[string]int64 {
	mx, err := i.collect()
	if err != nil {
		i.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (i *IPFS) Cleanup() {
	if i.httpClient != nil {
		i.httpClient.CloseIdleConnections()
	}
}
