// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	_ "embed"
	"errors"
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
					Timeout: web.Duration(time.Second * 2),
				},
			},
			Node: "_local",
		},
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int    `yaml:"update_every" json:"update_every"`
	Node        string `yaml:"node" json:"node"`
	Databases   string `yaml:"databases" json:"databases"`
}

type CouchDB struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client

	databases []string
}

func (cdb *CouchDB) Configuration() any {
	return cdb.Config
}

func (cdb *CouchDB) Init() error {
	err := cdb.validateConfig()
	if err != nil {
		cdb.Errorf("check configuration: %v", err)
		return err
	}

	cdb.databases = strings.Fields(cdb.Config.Databases)

	httpClient, err := cdb.initHTTPClient()
	if err != nil {
		cdb.Errorf("init HTTP client: %v", err)
		return err
	}
	cdb.httpClient = httpClient

	charts, err := cdb.initCharts()
	if err != nil {
		cdb.Errorf("init charts: %v", err)
		return err
	}
	cdb.charts = charts

	return nil
}

func (cdb *CouchDB) Check() error {
	if err := cdb.pingCouchDB(); err != nil {
		cdb.Error(err)
		return err
	}

	mx, err := cdb.collect()
	if err != nil {
		cdb.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
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

func (cdb *CouchDB) Cleanup() {
	if cdb.httpClient == nil {
		return
	}
	cdb.httpClient.CloseIdleConnections()
}
