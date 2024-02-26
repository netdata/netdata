// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	_ "embed"
	"fmt"
	"strings"
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

const (
	defaultURL         = "http://127.0.0.1:8653/json/v1"
	defaultHTTPTimeout = time.Second * 2
)

// New creates Bind with default values.
func New() *Bind {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: defaultURL,
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: defaultHTTPTimeout},
			},
		},
	}

	return &Bind{
		Config: config,
		charts: &Charts{},
	}
}

type bindAPIClient interface {
	serverStats() (*serverStats, error)
}

// Config is the Bind module configuration.
type Config struct {
	web.HTTP   `yaml:",inline"`
	PermitView string `yaml:"permit_view"`
}

// Bind Bind module.
type Bind struct {
	module.Base
	Config `yaml:",inline"`

	bindAPIClient
	permitView matcher.Matcher
	charts     *Charts
}

// Cleanup makes cleanup.
func (Bind) Cleanup() {}

// Init makes initialization.
func (b *Bind) Init() bool {
	if b.URL == "" {
		b.Error("URL not set")
		return false
	}

	client, err := web.NewHTTPClient(b.Client)
	if err != nil {
		b.Errorf("error on creating http client : %v", err)
		return false
	}

	switch {
	case strings.HasSuffix(b.URL, "/xml/v3"): // BIND 9.9+
		b.bindAPIClient = newXML3Client(client, b.Request)
	case strings.HasSuffix(b.URL, "/json/v1"): // BIND 9.10+
		b.bindAPIClient = newJSONClient(client, b.Request)
	default:
		b.Errorf("URL %s is wrong, supported endpoints: `/xml/v3`, `/json/v1`", b.URL)
		return false
	}

	if b.PermitView != "" {
		m, err := matcher.NewSimplePatternsMatcher(b.PermitView)
		if err != nil {
			b.Errorf("error on creating permitView matcher : %v", err)
			return false
		}
		b.permitView = matcher.WithCache(m)
	}

	return true
}

// Check makes check.
func (b *Bind) Check() bool {
	return len(b.Collect()) > 0
}

// Charts creates Charts.
func (b Bind) Charts() *Charts {
	return b.charts
}

// Collect collects metrics.
func (b *Bind) Collect() map[string]int64 {
	metrics := make(map[string]int64)

	s, err := b.serverStats()
	if err != nil {
		b.Error(err)
		return nil
	}
	b.collectServerStats(metrics, s)

	return metrics
}

func (b *Bind) collectServerStats(metrics map[string]int64, stats *serverStats) {
	var chart *Chart

	for k, v := range stats.NSStats {
		var (
			algo    = module.Incremental
			dimName = k
			chartID string
		)
		switch {
		default:
			continue
		case k == "RecursClients":
			dimName = "clients"
			chartID = keyRecursiveClients
			algo = module.Absolute
		case k == "Requestv4":
			dimName = "IPv4"
			chartID = keyReceivedRequests
		case k == "Requestv6":
			dimName = "IPv6"
			chartID = keyReceivedRequests
		case k == "QryFailure":
			dimName = "failures"
			chartID = keyQueryFailures
		case k == "QryUDP":
			dimName = "UDP"
			chartID = keyProtocolsQueries
		case k == "QryTCP":
			dimName = "TCP"
			chartID = keyProtocolsQueries
		case k == "QrySuccess":
			dimName = "queries"
			chartID = keyQueriesSuccess
		case strings.HasSuffix(k, "QryRej"):
			chartID = keyQueryFailuresDetail
		case strings.HasPrefix(k, "Qry"):
			chartID = keyQueriesAnalysis
		case strings.HasPrefix(k, "Update"):
			chartID = keyReceivedUpdates
		}

		if !b.charts.Has(chartID) {
			_ = b.charts.Add(charts[chartID].Copy())
		}

		chart = b.charts.Get(chartID)

		if !chart.HasDim(k) {
			_ = chart.AddDim(&Dim{ID: k, Name: dimName, Algo: algo})
			chart.MarkNotCreated()
		}

		delete(stats.NSStats, k)
		metrics[k] = v
	}

	for _, v := range []struct {
		item    map[string]int64
		chartID string
	}{
		{item: stats.NSStats, chartID: keyNSStats},
		{item: stats.OpCodes, chartID: keyInOpCodes},
		{item: stats.QTypes, chartID: keyInQTypes},
		{item: stats.SockStats, chartID: keyInSockStats},
	} {
		if len(v.item) == 0 {
			continue
		}

		if !b.charts.Has(v.chartID) {
			_ = b.charts.Add(charts[v.chartID].Copy())
		}

		chart = b.charts.Get(v.chartID)

		for key, val := range v.item {
			if !chart.HasDim(key) {
				_ = chart.AddDim(&Dim{ID: key, Algo: module.Incremental})
				chart.MarkNotCreated()
			}

			metrics[key] = val
		}
	}

	if !(b.permitView != nil && len(stats.Views) > 0) {
		return
	}

	for name, view := range stats.Views {
		if !b.permitView.MatchString(name) {
			continue
		}
		r := view.Resolver

		delete(r.Stats, "BucketSize")

		for key, val := range r.Stats {
			var (
				algo     = module.Incremental
				dimName  = key
				chartKey string
			)

			switch {
			default:
				chartKey = keyResolverStats
			case key == "NumFetch":
				chartKey = keyResolverNumFetch
				dimName = "queries"
				algo = module.Absolute
			case strings.HasPrefix(key, "QryRTT"):
				// TODO: not ordered
				chartKey = keyResolverRTT
			}

			chartID := fmt.Sprintf(chartKey, name)

			if !b.charts.Has(chartID) {
				chart = charts[chartKey].Copy()
				chart.ID = chartID
				chart.Fam = fmt.Sprintf(chart.Fam, name)
				_ = b.charts.Add(chart)
			}

			chart = b.charts.Get(chartID)
			dimID := fmt.Sprintf("%s_%s", name, key)

			if !chart.HasDim(dimID) {
				_ = chart.AddDim(&Dim{ID: dimID, Name: dimName, Algo: algo})
				chart.MarkNotCreated()
			}

			metrics[dimID] = val
		}

		if len(r.QTypes) > 0 {
			chartID := fmt.Sprintf(keyResolverInQTypes, name)

			if !b.charts.Has(chartID) {
				chart = charts[keyResolverInQTypes].Copy()
				chart.ID = chartID
				chart.Fam = fmt.Sprintf(chart.Fam, name)
				_ = b.charts.Add(chart)
			}

			chart = b.charts.Get(chartID)

			for key, val := range r.QTypes {
				dimID := fmt.Sprintf("%s_%s", name, key)
				if !chart.HasDim(dimID) {
					_ = chart.AddDim(&Dim{ID: dimID, Name: key, Algo: module.Incremental})
					chart.MarkNotCreated()
				}
				metrics[dimID] = val
			}
		}

		if len(r.CacheStats) > 0 {
			chartID := fmt.Sprintf(keyResolverCacheHits, name)

			if !b.charts.Has(chartID) {
				chart = charts[keyResolverCacheHits].Copy()
				chart.ID = chartID
				chart.Fam = fmt.Sprintf(chart.Fam, name)
				_ = b.charts.Add(chart)
				for _, dim := range chart.Dims {
					dim.ID = fmt.Sprintf(dim.ID, name)
				}
			}

			metrics[name+"_CacheHits"] = r.CacheStats["CacheHits"]
			metrics[name+"_CacheMisses"] = r.CacheStats["CacheMisses"]
		}
	}
}
