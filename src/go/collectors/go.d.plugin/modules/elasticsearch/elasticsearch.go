// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	_ "embed"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("elasticsearch", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *Elasticsearch {
	return &Elasticsearch{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:9200",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second * 5},
				},
			},
			ClusterMode: false,

			DoNodeStats:     true,
			DoClusterStats:  true,
			DoClusterHealth: true,
			DoIndicesStats:  false,
		},

		charts:                     &module.Charts{},
		addClusterHealthChartsOnce: &sync.Once{},
		addClusterStatsChartsOnce:  &sync.Once{},
		nodes:                      make(map[string]bool),
		indices:                    make(map[string]bool),
	}
}

type Config struct {
	web.HTTP        `yaml:",inline"`
	ClusterMode     bool `yaml:"cluster_mode"`
	DoNodeStats     bool `yaml:"collect_node_stats"`
	DoClusterHealth bool `yaml:"collect_cluster_health"`
	DoClusterStats  bool `yaml:"collect_cluster_stats"`
	DoIndicesStats  bool `yaml:"collect_indices_stats"`
}

type Elasticsearch struct {
	module.Base
	Config `yaml:",inline"`

	httpClient *http.Client
	charts     *module.Charts

	clusterName string

	addClusterHealthChartsOnce *sync.Once
	addClusterStatsChartsOnce  *sync.Once

	nodes   map[string]bool
	indices map[string]bool
}

func (es *Elasticsearch) Init() bool {
	err := es.validateConfig()
	if err != nil {
		es.Errorf("check configuration: %v", err)
		return false
	}

	httpClient, err := es.initHTTPClient()
	if err != nil {
		es.Errorf("init HTTP client: %v", err)
		return false
	}
	es.httpClient = httpClient

	return true
}

func (es *Elasticsearch) Check() bool {
	return len(es.Collect()) > 0
}

func (es *Elasticsearch) Charts() *module.Charts {
	return es.charts
}

func (es *Elasticsearch) Collect() map[string]int64 {
	mx, err := es.collect()
	if err != nil {
		es.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (es *Elasticsearch) Cleanup() {
	if es.httpClient != nil {
		es.httpClient.CloseIdleConnections()
	}
}
