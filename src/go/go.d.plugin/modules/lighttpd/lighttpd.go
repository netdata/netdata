// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import (
	_ "embed"
	"strings"
	"time"

	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("lighttpd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

const (
	defaultURL         = "http://127.0.0.1/server-status?auto"
	defaultHTTPTimeout = time.Second * 2
)

// New creates Lighttpd with default values.
func New() *Lighttpd {
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
	return &Lighttpd{Config: config}
}

// Config is the Lighttpd module configuration.
type Config struct {
	web.HTTP `yaml:",inline"`
}

type Lighttpd struct {
	module.Base
	Config    `yaml:",inline"`
	apiClient *apiClient
}

// Cleanup makes cleanup.
func (Lighttpd) Cleanup() {}

// Init makes initialization.
func (l *Lighttpd) Init() bool {
	if l.URL == "" {
		l.Error("URL not set")
		return false
	}

	if !strings.HasSuffix(l.URL, "?auto") {
		l.Errorf("bad URL '%s', should ends in '?auto'", l.URL)
		return false
	}

	client, err := web.NewHTTPClient(l.Client)
	if err != nil {
		l.Errorf("error on creating http client : %v", err)
		return false
	}
	l.apiClient = newAPIClient(client, l.Request)

	l.Debugf("using URL %s", l.URL)
	l.Debugf("using timeout: %s", l.Timeout.Duration)

	return true
}

// Check makes check
func (l *Lighttpd) Check() bool { return len(l.Collect()) > 0 }

// Charts returns Charts.
func (l Lighttpd) Charts() *Charts { return charts.Copy() }

// Collect collects metrics.
func (l *Lighttpd) Collect() map[string]int64 {
	mx, err := l.collect()

	if err != nil {
		l.Error(err)
		return nil
	}

	return mx
}
