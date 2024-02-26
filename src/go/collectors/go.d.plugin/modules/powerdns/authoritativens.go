// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import (
	_ "embed"
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
					Timeout: web.Duration{Duration: time.Second},
				},
			},
		},
	}
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type AuthoritativeNS struct {
	module.Base
	Config `yaml:",inline"`

	httpClient *http.Client
	charts     *module.Charts
}

func (ns *AuthoritativeNS) Init() bool {
	err := ns.validateConfig()
	if err != nil {
		ns.Errorf("config validation: %v", err)
		return false
	}

	client, err := ns.initHTTPClient()
	if err != nil {
		ns.Errorf("init HTTP client: %v", err)
		return false
	}
	ns.httpClient = client

	cs, err := ns.initCharts()
	if err != nil {
		ns.Errorf("init charts: %v", err)
		return false
	}
	ns.charts = cs

	return true
}

func (ns *AuthoritativeNS) Check() bool {
	return len(ns.Collect()) > 0
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
