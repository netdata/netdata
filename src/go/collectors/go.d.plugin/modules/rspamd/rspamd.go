// SPDX-License-Identifier: GPL-3.0-or-later

package rspamd

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
	module.Register("rspamd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Rspamd {
	return &Rspamd{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:11334",
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

type Rspamd struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (r *Rspamd) Configuration() any {
	return r.Config
}

func (r *Rspamd) Init() error {
	if r.URL == "" {
		r.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(r.Client)
	if err != nil {
		r.Error(err)
		return err
	}
	r.httpClient = client

	r.Debugf("using URL %s", r.URL)
	r.Debugf("using timeout: %s", r.Timeout)

	return nil
}

func (r *Rspamd) Check() error {
	mx, err := r.collect()
	if err != nil {
		r.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (r *Rspamd) Charts() *module.Charts {
	return r.charts
}

func (r *Rspamd) Collect() map[string]int64 {
	mx, err := r.collect()
	if err != nil {
		r.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (r *Rspamd) Cleanup() {
	if r.httpClient != nil {
		r.httpClient.CloseIdleConnections()
	}
}
