// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"net/http"
	"regexp"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("httpcheck", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *HTTPCheck {
	return &HTTPCheck{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
			AcceptedStatuses: []int{200},
		},

		acceptedStatuses: make(map[int]bool),
	}
}

type (
	Config struct {
		UpdateEvery      int `yaml:"update_every,omitempty" json:"update_every"`
		web.HTTPConfig   `yaml:",inline" json:""`
		AcceptedStatuses []int               `yaml:"status_accepted" json:"status_accepted"`
		ResponseMatch    string              `yaml:"response_match,omitempty" json:"response_match"`
		CookieFile       string              `yaml:"cookie_file,omitempty" json:"cookie_file"`
		HeaderMatch      []headerMatchConfig `yaml:"header_match,omitempty" json:"header_match"`
	}
	headerMatchConfig struct {
		Exclude bool   `yaml:"exclude" json:"exclude"`
		Key     string `yaml:"key" json:"key"`
		Value   string `yaml:"value" json:"value"`
	}
)

type HTTPCheck struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	acceptedStatuses  map[int]bool
	reResponse        *regexp.Regexp
	headerMatch       []headerMatch
	cookieFileModTime time.Time

	metrics metrics
}

func (hc *HTTPCheck) Configuration() any {
	return hc.Config
}

func (hc *HTTPCheck) Init() error {
	if err := hc.validateConfig(); err != nil {
		hc.Errorf("config validation: %v", err)
		return err
	}

	hc.charts = hc.initCharts()

	httpClient, err := hc.initHTTPClient()
	if err != nil {
		hc.Errorf("init HTTPConfig client: %v", err)
		return err
	}
	hc.httpClient = httpClient

	re, err := hc.initResponseMatchRegexp()
	if err != nil {
		hc.Errorf("init response match regexp: %v", err)
		return err
	}
	hc.reResponse = re

	hm, err := hc.initHeaderMatch()
	if err != nil {
		hc.Errorf("init header match: %v", err)
		return err
	}
	hc.headerMatch = hm

	for _, v := range hc.AcceptedStatuses {
		hc.acceptedStatuses[v] = true
	}

	hc.Debugf("using URL %s", hc.URL)
	hc.Debugf("using HTTPConfig timeout %s", hc.Timeout.Duration())
	hc.Debugf("using accepted HTTPConfig statuses %v", hc.AcceptedStatuses)
	if hc.reResponse != nil {
		hc.Debugf("using response match regexp %s", hc.reResponse)
	}

	return nil
}

func (hc *HTTPCheck) Check() error {
	mx, err := hc.collect()
	if err != nil {
		hc.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (hc *HTTPCheck) Charts() *module.Charts {
	return hc.charts
}

func (hc *HTTPCheck) Collect() map[string]int64 {
	mx, err := hc.collect()
	if err != nil {
		hc.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (hc *HTTPCheck) Cleanup() {
	if hc.httpClient != nil {
		hc.httpClient.CloseIdleConnections()
	}
}
