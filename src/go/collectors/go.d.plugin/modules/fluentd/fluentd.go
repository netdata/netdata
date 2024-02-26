// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("fluentd", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

const (
	defaultURL         = "http://127.0.0.1:24220"
	defaultHTTPTimeout = time.Second * 2
)

// New creates Fluentd with default values.
func New() *Fluentd {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: defaultURL,
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: defaultHTTPTimeout},
			},
		}}

	return &Fluentd{
		Config:        config,
		activePlugins: make(map[string]bool),
		charts:        charts.Copy(),
	}
}

type Config struct {
	web.HTTP     `yaml:",inline"`
	PermitPlugin string `yaml:"permit_plugin_id"`
}

// Fluentd Fluentd module.
type Fluentd struct {
	module.Base
	Config `yaml:",inline"`

	permitPlugin  matcher.Matcher
	apiClient     *apiClient
	activePlugins map[string]bool
	charts        *Charts
}

// Cleanup makes cleanup.
func (Fluentd) Cleanup() {}

// Init makes initialization.
func (f *Fluentd) Init() bool {
	if f.URL == "" {
		f.Error("URL not set")
		return false
	}

	if f.PermitPlugin != "" {
		m, err := matcher.NewSimplePatternsMatcher(f.PermitPlugin)
		if err != nil {
			f.Errorf("error on creating permit_plugin matcher : %v", err)
			return false
		}
		f.permitPlugin = matcher.WithCache(m)
	}

	client, err := web.NewHTTPClient(f.Client)
	if err != nil {
		f.Errorf("error on creating client : %v", err)
		return false
	}

	f.apiClient = newAPIClient(client, f.Request)

	f.Debugf("using URL %s", f.URL)
	f.Debugf("using timeout: %s", f.Timeout.Duration)

	return true
}

// Check makes check.
func (f Fluentd) Check() bool { return len(f.Collect()) > 0 }

// Charts creates Charts.
func (f Fluentd) Charts() *Charts { return f.charts }

// Collect collects metrics.
func (f *Fluentd) Collect() map[string]int64 {
	info, err := f.apiClient.getPluginsInfo()

	if err != nil {
		f.Error(err)
		return nil
	}

	metrics := make(map[string]int64)

	for _, p := range info.Payload {
		// TODO: if p.Category == "input" ?
		if !p.hasCategory() && !p.hasBufferQueueLength() && !p.hasBufferTotalQueuedSize() {
			continue
		}

		if f.permitPlugin != nil && !f.permitPlugin.MatchString(p.ID) {
			f.Debugf("plugin id: '%s', type: '%s', category: '%s' denied", p.ID, p.Type, p.Category)
			continue
		}

		id := fmt.Sprintf("%s_%s_%s", p.ID, p.Type, p.Category)

		if p.hasCategory() {
			metrics[id+"_retry_count"] = *p.RetryCount
		}
		if p.hasBufferQueueLength() {
			metrics[id+"_buffer_queue_length"] = *p.BufferQueueLength
		}
		if p.hasBufferTotalQueuedSize() {
			metrics[id+"_buffer_total_queued_size"] = *p.BufferTotalQueuedSize
		}

		if !f.activePlugins[id] {
			f.activePlugins[id] = true
			f.addPluginToCharts(p)
		}

	}

	return metrics
}

func (f *Fluentd) addPluginToCharts(p pluginData) {
	id := fmt.Sprintf("%s_%s_%s", p.ID, p.Type, p.Category)

	if p.hasCategory() {
		chart := f.charts.Get("retry_count")
		_ = chart.AddDim(&Dim{ID: id + "_retry_count", Name: p.ID})
		chart.MarkNotCreated()
	}
	if p.hasBufferQueueLength() {
		chart := f.charts.Get("buffer_queue_length")
		_ = chart.AddDim(&Dim{ID: id + "_buffer_queue_length", Name: p.ID})
		chart.MarkNotCreated()
	}
	if p.hasBufferTotalQueuedSize() {
		chart := f.charts.Get("buffer_total_queued_size")
		_ = chart.AddDim(&Dim{ID: id + "_buffer_total_queued_size", Name: p.ID})
		chart.MarkNotCreated()
	}
}
