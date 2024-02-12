// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("nginx", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

const (
	defaultURL         = "http://127.0.0.1/stub_status"
	defaultHTTPTimeout = time.Second
)

// New creates Nginx with default values.
func New() *Nginx {
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

	return &Nginx{Config: config}
}

// Config is the Nginx module configuration.
type Config struct {
	web.HTTP `yaml:",inline"`
}

// Nginx nginx module.
type Nginx struct {
	module.Base
	Config `yaml:",inline"`

	apiClient *apiClient
}

// Cleanup makes cleanup.
func (Nginx) Cleanup() {}

// Init makes initialization.
func (n *Nginx) Init() bool {
	if n.URL == "" {
		n.Error("URL not set")
		return false
	}

	client, err := web.NewHTTPClient(n.Client)
	if err != nil {
		n.Error(err)
		return false
	}

	n.apiClient = newAPIClient(client, n.Request)

	n.Debugf("using URL %s", n.URL)
	n.Debugf("using timeout: %s", n.Timeout.Duration)

	return true
}

// Check makes check.
func (n *Nginx) Check() bool { return len(n.Collect()) > 0 }

// Charts creates Charts.
func (Nginx) Charts() *Charts { return charts.Copy() }

// Collect collects metrics.
func (n *Nginx) Collect() map[string]int64 {
	mx, err := n.collect()

	if err != nil {
		n.Error(err)
		return nil
	}

	return mx
}
