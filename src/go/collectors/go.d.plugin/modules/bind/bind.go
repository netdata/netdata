// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("bind", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Bind {
	return &Bind{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8653/json/v1",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
		charts: &Charts{},
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int    `yaml:"update_every" json:"update_every"`
	PermitView  string `yaml:"permit_view" json:"permit_view"`
}

type (
	Bind struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *Charts

		httpClient *http.Client
		bindAPIClient

		permitView matcher.Matcher
	}

	bindAPIClient interface {
		serverStats() (*serverStats, error)
	}
)

func (b *Bind) Configuration() any {
	return b.Config
}

func (b *Bind) Init() error {
	if err := b.validateConfig(); err != nil {
		b.Errorf("config verification: %v", err)
		return err
	}

	pvm, err := b.initPermitViewMatcher()
	if err != nil {
		b.Error(err)
		return err
	}
	if pvm != nil {
		b.permitView = pvm
	}

	httpClient, err := web.NewHTTPClient(b.Client)
	if err != nil {
		b.Errorf("creating http client : %v", err)
		return err
	}
	b.httpClient = httpClient

	bindClient, err := b.initBindApiClient(httpClient)
	if err != nil {
		b.Error(err)
		return err
	}
	b.bindAPIClient = bindClient

	return nil
}

func (b *Bind) Check() error {
	mx, err := b.collect()
	if err != nil {
		b.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (b *Bind) Charts() *Charts {
	return b.charts
}

func (b *Bind) Collect() map[string]int64 {
	mx, err := b.collect()

	if err != nil {
		b.Error(err)
		return nil
	}

	return mx
}

func (b *Bind) Cleanup() {
	if b.httpClient != nil {
		b.httpClient.CloseIdleConnections()
	}
}
