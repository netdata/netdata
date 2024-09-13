// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	_ "embed"
	"errors"
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
	module.Register("couchdb", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *CouchDB {
	return &CouchDB{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:5984",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 2),
				},
			},
			Node: "_local",
		},
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	Node           string `yaml:"node,omitempty" json:"node"`
	Databases      string `yaml:"databases,omitempty" json:"databases"`
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
