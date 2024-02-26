// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	_ "embed"
	"net/http"
	"strings"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("couchdb", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *CouchDB {
	return &CouchDB{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:5984",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second * 5},
				},
			},
			Node: "_local",
		},
	}
}

type (
	Config struct {
		web.HTTP  `yaml:",inline"`
		Node      string `yaml:"node"`
		Databases string `yaml:"databases"`
	}

	CouchDB struct {
		module.Base
		Config `yaml:",inline"`

		httpClient *http.Client
		charts     *module.Charts

		databases []string
	}
)

func (cdb *CouchDB) Cleanup() {
	if cdb.httpClient == nil {
		return
	}
	cdb.httpClient.CloseIdleConnections()
}

func (cdb *CouchDB) Init() bool {
	err := cdb.validateConfig()
	if err != nil {
		cdb.Errorf("check configuration: %v", err)
		return false
	}

	cdb.databases = strings.Fields(cdb.Config.Databases)

	httpClient, err := cdb.initHTTPClient()
	if err != nil {
		cdb.Errorf("init HTTP client: %v", err)
		return false
	}
	cdb.httpClient = httpClient

	charts, err := cdb.initCharts()
	if err != nil {
		cdb.Errorf("init charts: %v", err)
		return false
	}
	cdb.charts = charts

	return true
}

func (cdb *CouchDB) Check() bool {
	if err := cdb.pingCouchDB(); err != nil {
		cdb.Error(err)
		return false
	}
	return len(cdb.Collect()) > 0
}

func (cdb *CouchDB) Charts() *Charts {
	return cdb.charts
}

func (cdb *CouchDB) Collect() map[string]int64 {
	mx, err := cdb.collect()
	if err != nil {
		cdb.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}
