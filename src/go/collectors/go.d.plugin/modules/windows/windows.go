// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	_ "embed"
	"net/http"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/prometheus"
	"github.com/netdata/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("windows", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *Windows {
	return &Windows{
		Config: Config{
			HTTP: web.HTTP{
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second * 5},
				},
			},
		},
		cache: cache{
			collection:                  make(map[string]bool),
			collectors:                  make(map[string]bool),
			cores:                       make(map[string]bool),
			nics:                        make(map[string]bool),
			volumes:                     make(map[string]bool),
			thermalZones:                make(map[string]bool),
			processes:                   make(map[string]bool),
			iis:                         make(map[string]bool),
			adcs:                        make(map[string]bool),
			services:                    make(map[string]bool),
			netFrameworkCLRExceptions:   make(map[string]bool),
			netFrameworkCLRInterops:     make(map[string]bool),
			netFrameworkCLRJIT:          make(map[string]bool),
			netFrameworkCLRLoading:      make(map[string]bool),
			netFrameworkCLRLocksThreads: make(map[string]bool),
			netFrameworkCLRMemory:       make(map[string]bool),
			netFrameworkCLRRemoting:     make(map[string]bool),
			netFrameworkCLRSecurity:     make(map[string]bool),
			mssqlInstances:              make(map[string]bool),
			mssqlDBs:                    make(map[string]bool),
			exchangeWorkload:            make(map[string]bool),
			exchangeLDAP:                make(map[string]bool),
			exchangeHTTPProxy:           make(map[string]bool),
			hypervVMMem:                 make(map[string]bool),
			hypervVMDevices:             make(map[string]bool),
			hypervVMInterfaces:          make(map[string]bool),
			hypervVswitch:               make(map[string]bool),
		},
		charts: &module.Charts{},
	}
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type (
	Windows struct {
		module.Base
		Config `yaml:",inline"`

		charts *module.Charts

		doCheck bool

		httpClient *http.Client
		prom       prometheus.Prometheus

		cache cache
	}
	cache struct {
		cores                       map[string]bool
		volumes                     map[string]bool
		nics                        map[string]bool
		thermalZones                map[string]bool
		processes                   map[string]bool
		iis                         map[string]bool
		adcs                        map[string]bool
		mssqlInstances              map[string]bool
		mssqlDBs                    map[string]bool
		services                    map[string]bool
		netFrameworkCLRExceptions   map[string]bool
		netFrameworkCLRInterops     map[string]bool
		netFrameworkCLRJIT          map[string]bool
		netFrameworkCLRLoading      map[string]bool
		netFrameworkCLRLocksThreads map[string]bool
		netFrameworkCLRMemory       map[string]bool
		netFrameworkCLRRemoting     map[string]bool
		netFrameworkCLRSecurity     map[string]bool
		collectors                  map[string]bool
		collection                  map[string]bool
		exchangeWorkload            map[string]bool
		exchangeLDAP                map[string]bool
		exchangeHTTPProxy           map[string]bool
		hypervVMMem                 map[string]bool
		hypervVMDevices             map[string]bool
		hypervVMInterfaces          map[string]bool
		hypervVswitch               map[string]bool
	}
)

func (w *Windows) Init() bool {
	if err := w.validateConfig(); err != nil {
		w.Errorf("config validation: %v", err)
		return false
	}

	httpClient, err := w.initHTTPClient()
	if err != nil {
		w.Errorf("init HTTP client: %v", err)
		return false
	}
	w.httpClient = httpClient

	prom, err := w.initPrometheusClient(w.httpClient)
	if err != nil {
		w.Errorf("init prometheus clients: %v", err)
		return false
	}
	w.prom = prom

	return true
}

func (w *Windows) Check() bool {
	return len(w.Collect()) > 0
}

func (w *Windows) Charts() *module.Charts {
	return w.charts
}

func (w *Windows) Collect() map[string]int64 {
	ms, err := w.collect()
	if err != nil {
		w.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (w *Windows) Cleanup() {
	if w.httpClient != nil {
		w.httpClient.CloseIdleConnections()
	}
}
