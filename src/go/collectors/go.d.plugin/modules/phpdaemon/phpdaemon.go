// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

import (
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("phpdaemon", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

const (
	defaultURL         = "http://127.0.0.1:8509/FullStatus"
	defaultHTTPTimeout = time.Second * 2
)

// New creates PHPDaemon with default values.
func New() *PHPDaemon {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: defaultURL,
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: defaultHTTPTimeout},
			},
		},
	}

	return &PHPDaemon{
		Config: config,
		charts: charts.Copy(),
	}
}

// Config is the PHPDaemon module configuration.
type Config struct {
	web.HTTP `yaml:",inline"`
}

// PHPDaemon PHPDaemon module.
type PHPDaemon struct {
	module.Base
	Config `yaml:",inline"`

	client *client
	charts *Charts
}

// Cleanup makes cleanup.
func (PHPDaemon) Cleanup() {}

// Init makes initialization.
func (p *PHPDaemon) Init() bool {
	httpClient, err := web.NewHTTPClient(p.Client)
	if err != nil {
		p.Errorf("error on creating http client : %v", err)
		return false
	}

	_, err = web.NewHTTPRequest(p.Request)
	if err != nil {
		p.Errorf("error on creating http request to %s : %v", p.URL, err)
		return false
	}

	p.client = newAPIClient(httpClient, p.Request)

	p.Debugf("using URL %s", p.URL)
	p.Debugf("using timeout: %s", p.Timeout.Duration)

	return true
}

// Check makes check.
func (p *PHPDaemon) Check() bool {
	mx := p.Collect()

	if len(mx) == 0 {
		return false
	}
	if _, ok := mx["uptime"]; ok {
		// TODO: remove panic
		panicIf(p.charts.Add(uptimeChart.Copy()))
	}

	return true
}

// Charts creates Charts.
func (p PHPDaemon) Charts() *Charts { return p.charts }

// Collect collects metrics.
func (p *PHPDaemon) Collect() map[string]int64 {
	mx, err := p.collect()

	if err != nil {
		p.Error(err)
		return nil
	}

	return mx
}

func panicIf(err error) {
	if err == nil {
		return
	}
	panic(err)
}
