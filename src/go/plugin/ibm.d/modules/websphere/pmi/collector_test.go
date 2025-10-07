//go:build cgo
// +build cgo

package pmi

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	pmiproto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/websphere/pmi"
)

func TestAggregatorWithActivatedSamples(t *testing.T) {
	sampleFiles := []string{
		"traditional-8.5.5.27-pmi-activated-port-10080.xml",
		"traditional-9.0.5.x-pmi-activated-port-11080.xml",
	}

	for _, rel := range sampleFiles {
		path := filepath.Join("..", "..", "..", "samples.d", rel)
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("open sample %s: %v", path, err)
		}

		var snapshot pmiproto.Snapshot
		if err := xml.NewDecoder(file).Decode(&snapshot); err != nil {
			file.Close()
			t.Fatalf("decode sample %s: %v", path, err)
		}
		file.Close()

		snapshot.Normalize()

		var urlStats int
		var servletStats int
		var totalStats int
		var statsWithSubstats int
		var statsWithoutMetrics int

		var walk func(stats []pmiproto.Stat)
		walk = func(stats []pmiproto.Stat) {
			for i := range stats {
				st := &stats[i]
				totalStats++
				if len(st.SubStats) > 0 {
					statsWithSubstats++
				}
				if len(st.CountStatistics) == 0 && len(st.TimeStatistics) == 0 && len(st.RangeStatistics) == 0 && len(st.BoundedRangeStatistics) == 0 {
					statsWithoutMetrics++
				}
				if st.Name == "URLs" {
					urlStats += len(st.SubStats)
					if len(st.SubStats) > 0 {
						names := make([]string, 0, len(st.SubStats))
						for _, sub := range st.SubStats {
							names = append(names, sub.Name)
						}
						t.Logf("URLs under %s => %v", st.Path, names)
					}
				}
				if st.Name == "Servlets" {
					servletStats += len(st.SubStats)
				}
				if len(st.SubStats) > 0 {
					walk(st.SubStats)
				}
			}
		}

		for _, node := range snapshot.Nodes {
			for _, server := range node.Servers {
				walk(server.Stats)
			}
		}
		walk(snapshot.Stats)

		t.Logf("sample %s stats totals: total=%d withSubstats=%d withoutMetrics=%d urlSubentries=%d servletChildren=%d", path, totalStats, statsWithSubstats, statsWithoutMetrics, urlStats, servletStats)

		agg := newAggregator(Config{})
		agg.processSnapshot(&snapshot, selectorBundle{})
		if len(agg.urls) == 0 {
			t.Logf("no URLs captured; sample contains URLs: %d", urlStats)
		}
		for key, data := range agg.urls {
			t.Logf("url key=%s requests=%d service=%d", key, data.requestCount, data.serviceTimeMs)
		}

		state := framework.NewCollectorState()
		agg.exportMetrics(state)

		metrics := state.GetMetrics()
		if len(metrics) == 0 {
			t.Fatalf("no metrics exported for sample %s", path)
		}
		if len(agg.jdbcPools) == 0 {
			t.Fatalf("expected jdbc pools in sample %s", path)
		}
		if len(agg.webApps) == 0 {
			t.Fatalf("expected web apps in sample %s", path)
		}
		if len(agg.sessions) == 0 {
			t.Fatalf("expected sessions in sample %s", path)
		}
		if len(agg.jmsQueues) == 0 {
			t.Fatalf("expected jms queues in sample %s", path)
		}

		t.Logf("sample %s => metrics=%d threadPools=%d jdbcPools=%d jcaPools=%d jmsQueues=%d jmsTopics=%d webApps=%d sessions=%d dynamicCaches=%d urls=%d securityAuth=%d portletApps=%d portlets=%d", path, len(metrics), len(agg.threadPools), len(agg.jdbcPools), len(agg.jcaPools), len(agg.jmsQueues), len(agg.jmsTopics), len(agg.webApps), len(agg.sessions), len(agg.dynamicCaches), len(agg.urls), len(agg.securityAuth), len(agg.portletApps), len(agg.portlets))
	}
}
