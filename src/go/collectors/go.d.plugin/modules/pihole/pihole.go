// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	_ "embed"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("pihole", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *Pihole {
	return &Pihole{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second * 5),
				},
			},
			SetupVarsPath: "/etc/pihole/setupVars.conf",
		},
		checkVersion:           true,
		charts:                 baseCharts.Copy(),
		addQueriesTypesOnce:    &sync.Once{},
		addFwsDestinationsOnce: &sync.Once{},
	}
}

type Config struct {
	web.HTTP      `yaml:",inline" json:""`
	UpdateEvery   int    `yaml:"update_every" json:"update_every"`
	SetupVarsPath string `yaml:"setup_vars_path" json:"setup_vars_path"`
}

type Pihole struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts                 *module.Charts
	addQueriesTypesOnce    *sync.Once
	addFwsDestinationsOnce *sync.Once

	httpClient *http.Client

	checkVersion bool
}

func (p *Pihole) Configuration() any {
	return p.Config
}

func (p *Pihole) Init() error {
	if err := p.validateConfig(); err != nil {
		p.Errorf("config validation: %v", err)
		return err
	}

	httpClient, err := p.initHTTPClient()
	if err != nil {
		p.Errorf("init http client: %v", err)
		return err
	}
	p.httpClient = httpClient

	p.Password = p.getWebPassword()
	if p.Password == "" {
		p.Warning("no web password, not all metrics available")
	} else {
		p.Debugf("web password: %s", p.Password)
	}

	return nil
}

func (p *Pihole) Check() error {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (p *Pihole) Charts() *module.Charts {
	return p.charts
}

func (p *Pihole) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (p *Pihole) Cleanup() {
	if p.httpClient != nil {
		p.httpClient.CloseIdleConnections()
	}
}
