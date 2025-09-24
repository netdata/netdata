package pmi

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	pmiproto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/websphere/pmi"
)

func TestPMICollectorCoverageActivatedSamples(t *testing.T) {
	samples := []string{
		"traditional-8.5.5.27-pmi-activated-port-10080.xml",
		"traditional-9.0.5.x-pmi-activated-port-11080.xml",
	}

	for _, sample := range samples {
		t.Run(sample, func(t *testing.T) {
			snapshot := loadPMISnapshot(t, sample)

			agg := newAggregator(Config{
				CollectJVMMetrics:         boolPtr(true),
				CollectThreadPoolMetrics:  boolPtr(true),
				CollectJDBCMetrics:        boolPtr(true),
				CollectJCAMetrics:         boolPtr(true),
				CollectJMSMetrics:         boolPtr(true),
				CollectWebAppMetrics:      boolPtr(true),
				CollectSessionMetrics:     boolPtr(true),
				CollectTransactionMetrics: boolPtr(true),
				CollectServletMetrics:     boolPtr(true),
				CollectEJBMetrics:         boolPtr(true),
				CollectJDBCAdvanced:       boolPtr(true),
				MaxThreadPools:            0,
				MaxJDBCPools:              0,
				MaxJCAPools:               0,
				MaxJMSDestinations:        0,
				MaxApplications:           0,
				MaxServlets:               0,
				MaxEJBs:                   0,
			})

			selectors := selectorBundle{
				app:     matcher.Must(matcher.New(matcher.FmtGlob, "*")),
				pool:    matcher.Must(matcher.New(matcher.FmtGlob, "*")),
				jms:     matcher.Must(matcher.New(matcher.FmtGlob, "*")),
				servlet: matcher.Must(matcher.New(matcher.FmtGlob, "*")),
				ejb:     matcher.Must(matcher.New(matcher.FmtGlob, "*")),
			}

			agg.processSnapshot(snapshot, selectors)

			if missing := agg.coverageMissing(); len(missing) > 0 {
				t.Fatalf("unhandled PMI stats: %v", missing)
			}
		})
	}
}

func loadPMISnapshot(t *testing.T, filename string) *pmiproto.Snapshot {
	t.Helper()
	filePath := filepath.Join("..", "..", "..", "samples.d", filename)
	f, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("failed to open sample %s: %v", filename, err)
	}
	defer f.Close()

	decoder := xml.NewDecoder(f)
	var snapshot pmiproto.Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		t.Fatalf("failed to decode sample %s: %v", filename, err)
	}
	snapshot.Normalize()
	return &snapshot
}

func boolPtr(v bool) *bool {
	return &v
}
