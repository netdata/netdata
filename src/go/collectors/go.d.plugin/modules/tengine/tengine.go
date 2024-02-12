// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("tengine", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

const (
	defaultURL         = "http://127.0.0.1/us"
	defaultHTTPTimeout = time.Second * 2
)

// New creates Tengine with default values.
func New() *Tengine {
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
	return &Tengine{Config: config}
}

// Config is the Tengine module configuration.
type Config struct {
	web.HTTP `yaml:",inline"`
}

// Tengine Tengine module.
type Tengine struct {
	module.Base
	Config `yaml:",inline"`

	apiClient *apiClient
}

// Cleanup makes cleanup.
func (Tengine) Cleanup() {}

// Init makes initialization.
func (t *Tengine) Init() bool {
	if t.URL == "" {
		t.Error("URL not set")
		return false
	}

	client, err := web.NewHTTPClient(t.Client)
	if err != nil {
		t.Errorf("error on creating http client : %v", err)
		return false
	}

	t.apiClient = newAPIClient(client, t.Request)

	t.Debugf("using URL: %s", t.URL)
	t.Debugf("using timeout: %s", t.Timeout.Duration)
	return true
}

// Check makes check
func (t *Tengine) Check() bool {
	return len(t.Collect()) > 0
}

// Charts returns Charts.
func (t Tengine) Charts() *module.Charts {
	return charts.Copy()
}

// Collect collects metrics.
func (t *Tengine) Collect() map[string]int64 {
	mx, err := t.collect()

	if err != nil {
		t.Error(err)
		return nil
	}

	return mx
}
