// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("lighttpd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Lighttpd {
	return &Lighttpd{Config: Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL: "http://127.0.0.1/server-status?auto",
			},
			ClientConfig: web.ClientConfig{
				Timeout: confopt.Duration(time.Second * 2),
			},
		},
	},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type Lighttpd struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (l *Lighttpd) Configuration() any {
	return l.Config
}

func (l *Lighttpd) Init() error {
	if l.URL == "" {
		return errors.New("URL is required but not set")
	}
	if !strings.HasSuffix(l.URL, "?auto") {
		return fmt.Errorf("bad URL '%s', should ends in '?auto'", l.URL)
	}

	httpClient, err := web.NewHTTPClient(l.ClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create http client: %v", err)
	}
	l.httpClient = httpClient

	l.Debugf("using URL %s", l.URL)
	l.Debugf("using timeout: %s", l.Timeout.Duration())

	return nil
}

func (l *Lighttpd) Check() error {
	mx, err := l.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (l *Lighttpd) Charts() *Charts {
	return l.charts
}

func (l *Lighttpd) Collect() map[string]int64 {
	mx, err := l.collect()
	if err != nil {
		l.Error(err)
		return nil
	}

	return mx
}

func (l *Lighttpd) Cleanup() {
	if l.httpClient != nil {
		l.httpClient.CloseIdleConnections()
	}
}
