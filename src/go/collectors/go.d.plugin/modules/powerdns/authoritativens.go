// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

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
	module.Register("powerdns", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *AuthoritativeNS {
	return &AuthoritativeNS{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8081",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type AuthoritativeNS struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (ns *AuthoritativeNS) Configuration() any {
	return ns.Config
}

func (ns *AuthoritativeNS) Init() error {
	err := ns.validateConfig()
	if err != nil {
		ns.Errorf("config validation: %v", err)
		return err
	}

	client, err := ns.initHTTPClient()
	if err != nil {
		ns.Errorf("init HTTP client: %v", err)
		return err
	}
	ns.httpClient = client

	cs, err := ns.initCharts()
	if err != nil {
		ns.Errorf("init charts: %v", err)
		return err
	}
	ns.charts = cs

	return nil
}

func (ns *AuthoritativeNS) Check() error {
	mx, err := ns.collect()
	if err != nil {
		ns.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (ns *AuthoritativeNS) Charts() *module.Charts {
	return ns.charts
}

func (ns *AuthoritativeNS) Collect() map[string]int64 {
	ms, err := ns.collect()
	if err != nil {
		ns.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (ns *AuthoritativeNS) Cleanup() {
	if ns.httpClient == nil {
		return
	}
	ns.httpClient.CloseIdleConnections()
}
