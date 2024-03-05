// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	_ "embed"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
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
					Timeout: web.Duration(time.Second * 2),
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
	web.HTTP        `yaml:",inline" json:""`
	UpdateEvery     int  `yaml:"update_every" json:"update_every"`
	ClusterMode     bool `yaml:"cluster_mode" json:"cluster_mode"`
	DoNodeStats     bool `yaml:"collect_node_stats" json:"collect_node_stats"`
	DoClusterHealth bool `yaml:"collect_cluster_health" json:"collect_cluster_health"`
	DoClusterStats  bool `yaml:"collect_cluster_stats" json:"collect_cluster_stats"`
	DoIndicesStats  bool `yaml:"collect_indices_stats" json:"collect_indices_stats"`
}

type Elasticsearch struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts                     *module.Charts
	addClusterHealthChartsOnce *sync.Once
	addClusterStatsChartsOnce  *sync.Once

	httpClient *http.Client

	clusterName string
	nodes       map[string]bool
	indices     map[string]bool
}

func (es *Elasticsearch) Configuration() any {
	return es.Config
}

func (es *Elasticsearch) Init() error {
	err := es.validateConfig()
	if err != nil {
		es.Errorf("check configuration: %v", err)
		return err
	}

	httpClient, err := es.initHTTPClient()
	if err != nil {
		es.Errorf("init HTTP client: %v", err)
		return err
	}
	es.httpClient = httpClient

	return nil
}

func (es *Elasticsearch) Check() error {
	mx, err := es.collect()
	if err != nil {
		es.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
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
