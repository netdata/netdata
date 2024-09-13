// SPDX-License-Identifier: GPL-3.0-or-later

package ipfs

import (
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ipfs", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *IPFS {
	return &IPFS{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL:    "http://127.0.0.1:5001",
					Method: http.MethodPost,
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 1),
				},
			},
			QueryRepoApi: false,
			QueryPinApi:  false,
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	QueryPinApi    bool `yaml:"pinapi" json:"pinapi"`
	QueryRepoApi   bool `yaml:"repoapi" json:"repoapi"`
}

type IPFS struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
}

func (ip *IPFS) Configuration() any {
	return ip.Config
}

func (ip *IPFS) Init() error {
	if ip.URL == "" {
		ip.Error("URL not set")
		return errors.New("url not set")
	}

	client, err := web.NewHTTPClient(ip.ClientConfig)
	if err != nil {
		ip.Error(err)
		return err
	}
	ip.httpClient = client

	if !ip.QueryPinApi {
		_ = ip.Charts().Remove(repoPinnedObjChart.ID)
	}
	if !ip.QueryRepoApi {
		_ = ip.Charts().Remove(datastoreUtilizationChart.ID)
		_ = ip.Charts().Remove(repoSizeChart.ID)
		_ = ip.Charts().Remove(repoObjChart.ID)
	}

	ip.Debugf("using URL %s", ip.URL)
	ip.Debugf("using timeout: %s", ip.Timeout)

	return nil
}

func (ip *IPFS) Check() error {
	mx, err := ip.collect()
	if err != nil {
		ip.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (ip *IPFS) Charts() *module.Charts {
	return ip.charts
}

func (ip *IPFS) Collect() map[string]int64 {
	mx, err := ip.collect()
	if err != nil {
		ip.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (ip *IPFS) Cleanup() {
	if ip.httpClient != nil {
		ip.httpClient.CloseIdleConnections()
	}
}
