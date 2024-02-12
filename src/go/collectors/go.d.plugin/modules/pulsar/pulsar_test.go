// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/netdata/go.d.plugin/pkg/matcher"
	"github.com/netdata/go.d.plugin/pkg/tlscfg"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	metricsNonPulsar, _         = os.ReadFile("testdata/non-pulsar.txt")
	metricsStdV250Namespaces, _ = os.ReadFile("testdata/standalone-v2.5.0-namespaces.txt")
	metricsStdV250Topics, _     = os.ReadFile("testdata/standalone-v2.5.0-topics.txt")
	metricsStdV250Topics2, _    = os.ReadFile("testdata/standalone-v2.5.0-topics-2.txt")
)

func Test_readTestData(t *testing.T) {
	assert.NotNil(t, metricsNonPulsar)
	assert.NotNil(t, metricsStdV250Namespaces)
	assert.NotNil(t, metricsStdV250Topics)
	assert.NotNil(t, metricsStdV250Topics2)
}

func TestNew(t *testing.T) {
	assert.Implements(t, (*module.Module)(nil), New())
}

func TestPulsar_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"default": {
			config: New().Config,
		},
		"empty topic filter": {
			config: Config{HTTP: web.HTTP{Request: web.Request{URL: "http://127.0.0.1:8080/metric"}}},
		},
		"bad syntax topic filer": {
			config: Config{
				HTTP:       web.HTTP{Request: web.Request{URL: "http://127.0.0.1:8080/metrics"}},
				TopicFiler: matcher.SimpleExpr{Includes: []string{"+"}}},
			wantFail: true,
		},
		"empty URL": {
			config:   Config{HTTP: web.HTTP{Request: web.Request{URL: ""}}},
			wantFail: true,
		},
		"nonexistent TLS CA": {
			config: Config{HTTP: web.HTTP{
				Request: web.Request{URL: "http://127.0.0.1:8080/metric"},
				Client:  web.Client{TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"}}}},
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pulsar := New()
			pulsar.Config = test.config

			if test.wantFail {
				assert.False(t, pulsar.Init())
			} else {
				assert.True(t, pulsar.Init())
			}
		})
	}
}

func TestPulsar_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestPulsar_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*testing.T) (*Pulsar, *httptest.Server)
		wantFail bool
	}{
		"standalone v2.5.0 namespaces": {prepare: prepareClientServerStdV250Namespaces},
		"standalone v2.5.0 topics":     {prepare: prepareClientServerStdV250Topics},
		"non pulsar":                   {prepare: prepareClientServerNonPulsar, wantFail: true},
		"invalid data":                 {prepare: prepareClientServerInvalidData, wantFail: true},
		"404":                          {prepare: prepareClientServer404, wantFail: true},
		"connection refused":           {prepare: prepareClientServerConnectionRefused, wantFail: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pulsar, srv := test.prepare(t)
			defer srv.Close()

			if test.wantFail {
				assert.False(t, pulsar.Check())
			} else {
				assert.True(t, pulsar.Check())
			}
		})
	}
}

func TestPulsar_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())

}

func TestPulsar_Collect_ReturnsNilOnErrors(t *testing.T) {
	tests := map[string]struct {
		prepare func(*testing.T) (*Pulsar, *httptest.Server)
	}{
		"non pulsar":         {prepare: prepareClientServerNonPulsar},
		"invalid data":       {prepare: prepareClientServerInvalidData},
		"404":                {prepare: prepareClientServer404},
		"connection refused": {prepare: prepareClientServerConnectionRefused},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pulsar, srv := test.prepare(t)
			defer srv.Close()

			assert.Nil(t, pulsar.Collect())
		})
	}
}

func TestPulsar_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*testing.T) (*Pulsar, *httptest.Server)
		expected map[string]int64
	}{
		"standalone v2.5.0 namespaces": {
			prepare:  prepareClientServerStdV250Namespaces,
			expected: expectedStandaloneV250Namespaces,
		},
		"standalone v2.5.0 topics": {
			prepare:  prepareClientServerStdV250Topics,
			expected: expectedStandaloneV250Topics,
		},
		"standalone v2.5.0 topics filtered": {
			prepare:  prepareClientServerStdV250TopicsFiltered,
			expected: expectedStandaloneV250TopicsFiltered,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pulsar, srv := test.prepare(t)
			defer srv.Close()

			for i := 0; i < 10; i++ {
				_ = pulsar.Collect()
			}
			collected := pulsar.Collect()

			require.NotNil(t, collected)
			require.Equal(t, test.expected, collected)
			ensureCollectedHasAllChartsDimsVarsIDs(t, pulsar, collected)
		})
	}
}

func TestPulsar_Collect_RemoveAddNamespacesTopicsInRuntime(t *testing.T) {
	pulsar, srv := prepareClientServersDynamicStdV250Topics(t)
	defer srv.Close()

	oldNsCharts := Charts{}

	require.NotNil(t, pulsar.Collect())
	oldLength := len(*pulsar.Charts())

	for _, chart := range *pulsar.Charts() {
		for ns := range pulsar.cache.namespaces {
			if ns.name != "public/functions" && chart.Fam == "ns "+ns.name {
				_ = oldNsCharts.Add(chart)
			}
		}
	}

	require.NotNil(t, pulsar.Collect())

	l := oldLength + len(*pulsar.nsCharts)*2 // 2 new namespaces
	assert.Truef(t, len(*pulsar.Charts()) == l, "expected %d charts, but got %d", l, len(*pulsar.Charts()))

	for _, chart := range oldNsCharts {
		assert.Truef(t, chart.Obsolete, "expected chart '%s' Obsolete flag is set", chart.ID)
		for _, dim := range chart.Dims {
			if strings.HasPrefix(chart.ID, "topic_") {
				assert.Truef(t, dim.Obsolete, "expected chart '%s' dim '%s' Obsolete flag is set", chart.ID, dim.ID)
			}
		}
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, pulsar *Pulsar, collected map[string]int64) {
	for _, chart := range *pulsar.Charts() {
		for _, dim := range chart.Dims {
			_, ok := collected[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := collected[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func prepareClientServerStdV250Namespaces(t *testing.T) (*Pulsar, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(metricsStdV250Namespaces)
		}))

	pulsar := New()
	pulsar.URL = srv.URL
	require.True(t, pulsar.Init())

	return pulsar, srv
}

func prepareClientServerStdV250Topics(t *testing.T) (*Pulsar, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(metricsStdV250Topics)
		}))

	pulsar := New()
	pulsar.URL = srv.URL
	require.True(t, pulsar.Init())

	return pulsar, srv
}

func prepareClientServerStdV250TopicsFiltered(t *testing.T) (*Pulsar, *httptest.Server) {
	t.Helper()
	pulsar, srv := prepareClientServerStdV250Topics(t)
	pulsar.topicFilter = matcher.FALSE()

	return pulsar, srv
}

func prepareClientServersDynamicStdV250Topics(t *testing.T) (*Pulsar, *httptest.Server) {
	t.Helper()
	var i int
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if i%2 == 0 {
				_, _ = w.Write(metricsStdV250Topics)
			} else {
				_, _ = w.Write(metricsStdV250Topics2)
			}
			i++
		}))

	pulsar := New()
	pulsar.URL = srv.URL
	require.True(t, pulsar.Init())

	return pulsar, srv
}

func prepareClientServerNonPulsar(t *testing.T) (*Pulsar, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(metricsNonPulsar)
		}))

	pulsar := New()
	pulsar.URL = srv.URL
	require.True(t, pulsar.Init())

	return pulsar, srv
}

func prepareClientServerInvalidData(t *testing.T) (*Pulsar, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	pulsar := New()
	pulsar.URL = srv.URL
	require.True(t, pulsar.Init())

	return pulsar, srv
}

func prepareClientServer404(t *testing.T) (*Pulsar, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

	pulsar := New()
	pulsar.URL = srv.URL
	require.True(t, pulsar.Init())

	return pulsar, srv
}

func prepareClientServerConnectionRefused(t *testing.T) (*Pulsar, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(nil)

	pulsar := New()
	pulsar.URL = "http://127.0.0.1:38001/metrics"
	require.True(t, pulsar.Init())

	return pulsar, srv
}

var expectedStandaloneV250Namespaces = map[string]int64{
	"pulsar_consumers_count":                                 21,
	"pulsar_consumers_count_public/functions":                3,
	"pulsar_consumers_count_sample/dev":                      10,
	"pulsar_consumers_count_sample/prod":                     8,
	"pulsar_entry_size_count":                                6013,
	"pulsar_entry_size_count_public/functions":               0,
	"pulsar_entry_size_count_sample/dev":                     3012,
	"pulsar_entry_size_count_sample/prod":                    3001,
	"pulsar_entry_size_le_100_kb":                            0,
	"pulsar_entry_size_le_100_kb_public/functions":           0,
	"pulsar_entry_size_le_100_kb_sample/dev":                 0,
	"pulsar_entry_size_le_100_kb_sample/prod":                0,
	"pulsar_entry_size_le_128":                               6013,
	"pulsar_entry_size_le_128_public/functions":              0,
	"pulsar_entry_size_le_128_sample/dev":                    3012,
	"pulsar_entry_size_le_128_sample/prod":                   3001,
	"pulsar_entry_size_le_16_kb":                             0,
	"pulsar_entry_size_le_16_kb_public/functions":            0,
	"pulsar_entry_size_le_16_kb_sample/dev":                  0,
	"pulsar_entry_size_le_16_kb_sample/prod":                 0,
	"pulsar_entry_size_le_1_kb":                              0,
	"pulsar_entry_size_le_1_kb_public/functions":             0,
	"pulsar_entry_size_le_1_kb_sample/dev":                   0,
	"pulsar_entry_size_le_1_kb_sample/prod":                  0,
	"pulsar_entry_size_le_1_mb":                              0,
	"pulsar_entry_size_le_1_mb_public/functions":             0,
	"pulsar_entry_size_le_1_mb_sample/dev":                   0,
	"pulsar_entry_size_le_1_mb_sample/prod":                  0,
	"pulsar_entry_size_le_2_kb":                              0,
	"pulsar_entry_size_le_2_kb_public/functions":             0,
	"pulsar_entry_size_le_2_kb_sample/dev":                   0,
	"pulsar_entry_size_le_2_kb_sample/prod":                  0,
	"pulsar_entry_size_le_4_kb":                              0,
	"pulsar_entry_size_le_4_kb_public/functions":             0,
	"pulsar_entry_size_le_4_kb_sample/dev":                   0,
	"pulsar_entry_size_le_4_kb_sample/prod":                  0,
	"pulsar_entry_size_le_512":                               0,
	"pulsar_entry_size_le_512_public/functions":              0,
	"pulsar_entry_size_le_512_sample/dev":                    0,
	"pulsar_entry_size_le_512_sample/prod":                   0,
	"pulsar_entry_size_le_overflow":                          0,
	"pulsar_entry_size_le_overflow_public/functions":         0,
	"pulsar_entry_size_le_overflow_sample/dev":               0,
	"pulsar_entry_size_le_overflow_sample/prod":              0,
	"pulsar_entry_size_sum":                                  6013,
	"pulsar_entry_size_sum_public/functions":                 0,
	"pulsar_entry_size_sum_sample/dev":                       3012,
	"pulsar_entry_size_sum_sample/prod":                      3001,
	"pulsar_msg_backlog":                                     8,
	"pulsar_msg_backlog_public/functions":                    0,
	"pulsar_msg_backlog_sample/dev":                          8,
	"pulsar_msg_backlog_sample/prod":                         0,
	"pulsar_namespaces_count":                                3,
	"pulsar_producers_count":                                 10,
	"pulsar_producers_count_public/functions":                2,
	"pulsar_producers_count_sample/dev":                      4,
	"pulsar_producers_count_sample/prod":                     4,
	"pulsar_rate_in":                                         96023,
	"pulsar_rate_in_public/functions":                        0,
	"pulsar_rate_in_sample/dev":                              48004,
	"pulsar_rate_in_sample/prod":                             48019,
	"pulsar_rate_out":                                        242057,
	"pulsar_rate_out_public/functions":                       0,
	"pulsar_rate_out_sample/dev":                             146018,
	"pulsar_rate_out_sample/prod":                            96039,
	"pulsar_storage_read_rate":                               0,
	"pulsar_storage_read_rate_public/functions":              0,
	"pulsar_storage_read_rate_sample/dev":                    0,
	"pulsar_storage_read_rate_sample/prod":                   0,
	"pulsar_storage_size":                                    5468424,
	"pulsar_storage_size_public/functions":                   0,
	"pulsar_storage_size_sample/dev":                         2684208,
	"pulsar_storage_size_sample/prod":                        2784216,
	"pulsar_storage_write_latency_count":                     6012,
	"pulsar_storage_write_latency_count_public/functions":    0,
	"pulsar_storage_write_latency_count_sample/dev":          3012,
	"pulsar_storage_write_latency_count_sample/prod":         3000,
	"pulsar_storage_write_latency_le_0_5":                    0,
	"pulsar_storage_write_latency_le_0_5_public/functions":   0,
	"pulsar_storage_write_latency_le_0_5_sample/dev":         0,
	"pulsar_storage_write_latency_le_0_5_sample/prod":        0,
	"pulsar_storage_write_latency_le_1":                      43,
	"pulsar_storage_write_latency_le_10":                     163,
	"pulsar_storage_write_latency_le_100":                    0,
	"pulsar_storage_write_latency_le_1000":                   0,
	"pulsar_storage_write_latency_le_1000_public/functions":  0,
	"pulsar_storage_write_latency_le_1000_sample/dev":        0,
	"pulsar_storage_write_latency_le_1000_sample/prod":       0,
	"pulsar_storage_write_latency_le_100_public/functions":   0,
	"pulsar_storage_write_latency_le_100_sample/dev":         0,
	"pulsar_storage_write_latency_le_100_sample/prod":        0,
	"pulsar_storage_write_latency_le_10_public/functions":    0,
	"pulsar_storage_write_latency_le_10_sample/dev":          82,
	"pulsar_storage_write_latency_le_10_sample/prod":         81,
	"pulsar_storage_write_latency_le_1_public/functions":     0,
	"pulsar_storage_write_latency_le_1_sample/dev":           23,
	"pulsar_storage_write_latency_le_1_sample/prod":          20,
	"pulsar_storage_write_latency_le_20":                     7,
	"pulsar_storage_write_latency_le_200":                    2,
	"pulsar_storage_write_latency_le_200_public/functions":   0,
	"pulsar_storage_write_latency_le_200_sample/dev":         1,
	"pulsar_storage_write_latency_le_200_sample/prod":        1,
	"pulsar_storage_write_latency_le_20_public/functions":    0,
	"pulsar_storage_write_latency_le_20_sample/dev":          6,
	"pulsar_storage_write_latency_le_20_sample/prod":         1,
	"pulsar_storage_write_latency_le_5":                      5797,
	"pulsar_storage_write_latency_le_50":                     0,
	"pulsar_storage_write_latency_le_50_public/functions":    0,
	"pulsar_storage_write_latency_le_50_sample/dev":          0,
	"pulsar_storage_write_latency_le_50_sample/prod":         0,
	"pulsar_storage_write_latency_le_5_public/functions":     0,
	"pulsar_storage_write_latency_le_5_sample/dev":           2900,
	"pulsar_storage_write_latency_le_5_sample/prod":          2897,
	"pulsar_storage_write_latency_overflow":                  0,
	"pulsar_storage_write_latency_overflow_public/functions": 0,
	"pulsar_storage_write_latency_overflow_sample/dev":       0,
	"pulsar_storage_write_latency_overflow_sample/prod":      0,
	"pulsar_storage_write_latency_sum":                       6012,
	"pulsar_storage_write_latency_sum_public/functions":      0,
	"pulsar_storage_write_latency_sum_sample/dev":            3012,
	"pulsar_storage_write_latency_sum_sample/prod":           3000,
	"pulsar_storage_write_rate":                              100216,
	"pulsar_storage_write_rate_public/functions":             0,
	"pulsar_storage_write_rate_sample/dev":                   50200,
	"pulsar_storage_write_rate_sample/prod":                  50016,
	"pulsar_subscription_delayed":                            0,
	"pulsar_subscription_delayed_public/functions":           0,
	"pulsar_subscription_delayed_sample/dev":                 0,
	"pulsar_subscription_delayed_sample/prod":                0,
	"pulsar_subscriptions_count":                             13,
	"pulsar_subscriptions_count_public/functions":            3,
	"pulsar_subscriptions_count_sample/dev":                  6,
	"pulsar_subscriptions_count_sample/prod":                 4,
	"pulsar_throughput_in":                                   5569401,
	"pulsar_throughput_in_public/functions":                  0,
	"pulsar_throughput_in_sample/dev":                        2736243,
	"pulsar_throughput_in_sample/prod":                       2833158,
	"pulsar_throughput_out":                                  13989373,
	"pulsar_throughput_out_public/functions":                 0,
	"pulsar_throughput_out_sample/dev":                       8323043,
	"pulsar_throughput_out_sample/prod":                      5666330,
	"pulsar_topics_count":                                    7,
	"pulsar_topics_count_public/functions":                   3,
	"pulsar_topics_count_sample/dev":                         2,
	"pulsar_topics_count_sample/prod":                        2,
}

var expectedStandaloneV250Topics = map[string]int64{
	"pulsar_consumers_count": 21,
	"pulsar_consumers_count_persistent://public/functions/assignments":                          1,
	"pulsar_consumers_count_persistent://public/functions/coordinate":                           1,
	"pulsar_consumers_count_persistent://public/functions/metadata":                             1,
	"pulsar_consumers_count_persistent://sample/dev/dev-1":                                      4,
	"pulsar_consumers_count_persistent://sample/dev/dev-2":                                      6,
	"pulsar_consumers_count_persistent://sample/prod/prod-1":                                    4,
	"pulsar_consumers_count_persistent://sample/prod/prod-2":                                    4,
	"pulsar_consumers_count_public/functions":                                                   3,
	"pulsar_consumers_count_sample/dev":                                                         10,
	"pulsar_consumers_count_sample/prod":                                                        8,
	"pulsar_entry_size_count":                                                                   5867,
	"pulsar_entry_size_count_persistent://public/functions/assignments":                         0,
	"pulsar_entry_size_count_persistent://public/functions/coordinate":                          0,
	"pulsar_entry_size_count_persistent://public/functions/metadata":                            0,
	"pulsar_entry_size_count_persistent://sample/dev/dev-1":                                     1448,
	"pulsar_entry_size_count_persistent://sample/dev/dev-2":                                     1477,
	"pulsar_entry_size_count_persistent://sample/prod/prod-1":                                   1469,
	"pulsar_entry_size_count_persistent://sample/prod/prod-2":                                   1473,
	"pulsar_entry_size_count_public/functions":                                                  0,
	"pulsar_entry_size_count_sample/dev":                                                        2925,
	"pulsar_entry_size_count_sample/prod":                                                       2942,
	"pulsar_entry_size_le_100_kb":                                                               0,
	"pulsar_entry_size_le_100_kb_persistent://public/functions/assignments":                     0,
	"pulsar_entry_size_le_100_kb_persistent://public/functions/coordinate":                      0,
	"pulsar_entry_size_le_100_kb_persistent://public/functions/metadata":                        0,
	"pulsar_entry_size_le_100_kb_persistent://sample/dev/dev-1":                                 0,
	"pulsar_entry_size_le_100_kb_persistent://sample/dev/dev-2":                                 0,
	"pulsar_entry_size_le_100_kb_persistent://sample/prod/prod-1":                               0,
	"pulsar_entry_size_le_100_kb_persistent://sample/prod/prod-2":                               0,
	"pulsar_entry_size_le_100_kb_public/functions":                                              0,
	"pulsar_entry_size_le_100_kb_sample/dev":                                                    0,
	"pulsar_entry_size_le_100_kb_sample/prod":                                                   0,
	"pulsar_entry_size_le_128":                                                                  5867,
	"pulsar_entry_size_le_128_persistent://public/functions/assignments":                        0,
	"pulsar_entry_size_le_128_persistent://public/functions/coordinate":                         0,
	"pulsar_entry_size_le_128_persistent://public/functions/metadata":                           0,
	"pulsar_entry_size_le_128_persistent://sample/dev/dev-1":                                    1448,
	"pulsar_entry_size_le_128_persistent://sample/dev/dev-2":                                    1477,
	"pulsar_entry_size_le_128_persistent://sample/prod/prod-1":                                  1469,
	"pulsar_entry_size_le_128_persistent://sample/prod/prod-2":                                  1473,
	"pulsar_entry_size_le_128_public/functions":                                                 0,
	"pulsar_entry_size_le_128_sample/dev":                                                       2925,
	"pulsar_entry_size_le_128_sample/prod":                                                      2942,
	"pulsar_entry_size_le_16_kb":                                                                0,
	"pulsar_entry_size_le_16_kb_persistent://public/functions/assignments":                      0,
	"pulsar_entry_size_le_16_kb_persistent://public/functions/coordinate":                       0,
	"pulsar_entry_size_le_16_kb_persistent://public/functions/metadata":                         0,
	"pulsar_entry_size_le_16_kb_persistent://sample/dev/dev-1":                                  0,
	"pulsar_entry_size_le_16_kb_persistent://sample/dev/dev-2":                                  0,
	"pulsar_entry_size_le_16_kb_persistent://sample/prod/prod-1":                                0,
	"pulsar_entry_size_le_16_kb_persistent://sample/prod/prod-2":                                0,
	"pulsar_entry_size_le_16_kb_public/functions":                                               0,
	"pulsar_entry_size_le_16_kb_sample/dev":                                                     0,
	"pulsar_entry_size_le_16_kb_sample/prod":                                                    0,
	"pulsar_entry_size_le_1_kb":                                                                 0,
	"pulsar_entry_size_le_1_kb_persistent://public/functions/assignments":                       0,
	"pulsar_entry_size_le_1_kb_persistent://public/functions/coordinate":                        0,
	"pulsar_entry_size_le_1_kb_persistent://public/functions/metadata":                          0,
	"pulsar_entry_size_le_1_kb_persistent://sample/dev/dev-1":                                   0,
	"pulsar_entry_size_le_1_kb_persistent://sample/dev/dev-2":                                   0,
	"pulsar_entry_size_le_1_kb_persistent://sample/prod/prod-1":                                 0,
	"pulsar_entry_size_le_1_kb_persistent://sample/prod/prod-2":                                 0,
	"pulsar_entry_size_le_1_kb_public/functions":                                                0,
	"pulsar_entry_size_le_1_kb_sample/dev":                                                      0,
	"pulsar_entry_size_le_1_kb_sample/prod":                                                     0,
	"pulsar_entry_size_le_1_mb":                                                                 0,
	"pulsar_entry_size_le_1_mb_persistent://public/functions/assignments":                       0,
	"pulsar_entry_size_le_1_mb_persistent://public/functions/coordinate":                        0,
	"pulsar_entry_size_le_1_mb_persistent://public/functions/metadata":                          0,
	"pulsar_entry_size_le_1_mb_persistent://sample/dev/dev-1":                                   0,
	"pulsar_entry_size_le_1_mb_persistent://sample/dev/dev-2":                                   0,
	"pulsar_entry_size_le_1_mb_persistent://sample/prod/prod-1":                                 0,
	"pulsar_entry_size_le_1_mb_persistent://sample/prod/prod-2":                                 0,
	"pulsar_entry_size_le_1_mb_public/functions":                                                0,
	"pulsar_entry_size_le_1_mb_sample/dev":                                                      0,
	"pulsar_entry_size_le_1_mb_sample/prod":                                                     0,
	"pulsar_entry_size_le_2_kb":                                                                 0,
	"pulsar_entry_size_le_2_kb_persistent://public/functions/assignments":                       0,
	"pulsar_entry_size_le_2_kb_persistent://public/functions/coordinate":                        0,
	"pulsar_entry_size_le_2_kb_persistent://public/functions/metadata":                          0,
	"pulsar_entry_size_le_2_kb_persistent://sample/dev/dev-1":                                   0,
	"pulsar_entry_size_le_2_kb_persistent://sample/dev/dev-2":                                   0,
	"pulsar_entry_size_le_2_kb_persistent://sample/prod/prod-1":                                 0,
	"pulsar_entry_size_le_2_kb_persistent://sample/prod/prod-2":                                 0,
	"pulsar_entry_size_le_2_kb_public/functions":                                                0,
	"pulsar_entry_size_le_2_kb_sample/dev":                                                      0,
	"pulsar_entry_size_le_2_kb_sample/prod":                                                     0,
	"pulsar_entry_size_le_4_kb":                                                                 0,
	"pulsar_entry_size_le_4_kb_persistent://public/functions/assignments":                       0,
	"pulsar_entry_size_le_4_kb_persistent://public/functions/coordinate":                        0,
	"pulsar_entry_size_le_4_kb_persistent://public/functions/metadata":                          0,
	"pulsar_entry_size_le_4_kb_persistent://sample/dev/dev-1":                                   0,
	"pulsar_entry_size_le_4_kb_persistent://sample/dev/dev-2":                                   0,
	"pulsar_entry_size_le_4_kb_persistent://sample/prod/prod-1":                                 0,
	"pulsar_entry_size_le_4_kb_persistent://sample/prod/prod-2":                                 0,
	"pulsar_entry_size_le_4_kb_public/functions":                                                0,
	"pulsar_entry_size_le_4_kb_sample/dev":                                                      0,
	"pulsar_entry_size_le_4_kb_sample/prod":                                                     0,
	"pulsar_entry_size_le_512":                                                                  0,
	"pulsar_entry_size_le_512_persistent://public/functions/assignments":                        0,
	"pulsar_entry_size_le_512_persistent://public/functions/coordinate":                         0,
	"pulsar_entry_size_le_512_persistent://public/functions/metadata":                           0,
	"pulsar_entry_size_le_512_persistent://sample/dev/dev-1":                                    0,
	"pulsar_entry_size_le_512_persistent://sample/dev/dev-2":                                    0,
	"pulsar_entry_size_le_512_persistent://sample/prod/prod-1":                                  0,
	"pulsar_entry_size_le_512_persistent://sample/prod/prod-2":                                  0,
	"pulsar_entry_size_le_512_public/functions":                                                 0,
	"pulsar_entry_size_le_512_sample/dev":                                                       0,
	"pulsar_entry_size_le_512_sample/prod":                                                      0,
	"pulsar_entry_size_le_overflow":                                                             0,
	"pulsar_entry_size_le_overflow_persistent://public/functions/assignments":                   0,
	"pulsar_entry_size_le_overflow_persistent://public/functions/coordinate":                    0,
	"pulsar_entry_size_le_overflow_persistent://public/functions/metadata":                      0,
	"pulsar_entry_size_le_overflow_persistent://sample/dev/dev-1":                               0,
	"pulsar_entry_size_le_overflow_persistent://sample/dev/dev-2":                               0,
	"pulsar_entry_size_le_overflow_persistent://sample/prod/prod-1":                             0,
	"pulsar_entry_size_le_overflow_persistent://sample/prod/prod-2":                             0,
	"pulsar_entry_size_le_overflow_public/functions":                                            0,
	"pulsar_entry_size_le_overflow_sample/dev":                                                  0,
	"pulsar_entry_size_le_overflow_sample/prod":                                                 0,
	"pulsar_entry_size_sum":                                                                     5867,
	"pulsar_entry_size_sum_persistent://public/functions/assignments":                           0,
	"pulsar_entry_size_sum_persistent://public/functions/coordinate":                            0,
	"pulsar_entry_size_sum_persistent://public/functions/metadata":                              0,
	"pulsar_entry_size_sum_persistent://sample/dev/dev-1":                                       1448,
	"pulsar_entry_size_sum_persistent://sample/dev/dev-2":                                       1477,
	"pulsar_entry_size_sum_persistent://sample/prod/prod-1":                                     1469,
	"pulsar_entry_size_sum_persistent://sample/prod/prod-2":                                     1473,
	"pulsar_entry_size_sum_public/functions":                                                    0,
	"pulsar_entry_size_sum_sample/dev":                                                          2925,
	"pulsar_entry_size_sum_sample/prod":                                                         2942,
	"pulsar_msg_backlog":                                                                        0,
	"pulsar_msg_backlog_persistent://public/functions/assignments":                              0,
	"pulsar_msg_backlog_persistent://public/functions/coordinate":                               0,
	"pulsar_msg_backlog_persistent://public/functions/metadata":                                 0,
	"pulsar_msg_backlog_persistent://sample/dev/dev-1":                                          0,
	"pulsar_msg_backlog_persistent://sample/dev/dev-2":                                          0,
	"pulsar_msg_backlog_persistent://sample/prod/prod-1":                                        0,
	"pulsar_msg_backlog_persistent://sample/prod/prod-2":                                        0,
	"pulsar_msg_backlog_public/functions":                                                       0,
	"pulsar_msg_backlog_sample/dev":                                                             0,
	"pulsar_msg_backlog_sample/prod":                                                            0,
	"pulsar_namespaces_count":                                                                   3,
	"pulsar_producers_count":                                                                    10,
	"pulsar_producers_count_persistent://public/functions/assignments":                          1,
	"pulsar_producers_count_persistent://public/functions/coordinate":                           0,
	"pulsar_producers_count_persistent://public/functions/metadata":                             1,
	"pulsar_producers_count_persistent://sample/dev/dev-1":                                      2,
	"pulsar_producers_count_persistent://sample/dev/dev-2":                                      2,
	"pulsar_producers_count_persistent://sample/prod/prod-1":                                    2,
	"pulsar_producers_count_persistent://sample/prod/prod-2":                                    2,
	"pulsar_producers_count_public/functions":                                                   2,
	"pulsar_producers_count_sample/dev":                                                         4,
	"pulsar_producers_count_sample/prod":                                                        4,
	"pulsar_rate_in":                                                                            102064,
	"pulsar_rate_in_persistent://public/functions/assignments":                                  0,
	"pulsar_rate_in_persistent://public/functions/coordinate":                                   0,
	"pulsar_rate_in_persistent://public/functions/metadata":                                     0,
	"pulsar_rate_in_persistent://sample/dev/dev-1":                                              25013,
	"pulsar_rate_in_persistent://sample/dev/dev-2":                                              25014,
	"pulsar_rate_in_persistent://sample/prod/prod-1":                                            26019,
	"pulsar_rate_in_persistent://sample/prod/prod-2":                                            26018,
	"pulsar_rate_in_public/functions":                                                           0,
	"pulsar_rate_in_sample/dev":                                                                 50027,
	"pulsar_rate_in_sample/prod":                                                                52037,
	"pulsar_rate_out":                                                                           254162,
	"pulsar_rate_out_persistent://public/functions/assignments":                                 0,
	"pulsar_rate_out_persistent://public/functions/coordinate":                                  0,
	"pulsar_rate_out_persistent://public/functions/metadata":                                    0,
	"pulsar_rate_out_persistent://sample/dev/dev-1":                                             50027,
	"pulsar_rate_out_persistent://sample/dev/dev-2":                                             100060,
	"pulsar_rate_out_persistent://sample/prod/prod-1":                                           52038,
	"pulsar_rate_out_persistent://sample/prod/prod-2":                                           52037,
	"pulsar_rate_out_public/functions":                                                          0,
	"pulsar_rate_out_sample/dev":                                                                150087,
	"pulsar_rate_out_sample/prod":                                                               104075,
	"pulsar_storage_size":                                                                       8112300,
	"pulsar_storage_size_persistent://public/functions/assignments":                             0,
	"pulsar_storage_size_persistent://public/functions/coordinate":                              0,
	"pulsar_storage_size_persistent://public/functions/metadata":                                0,
	"pulsar_storage_size_persistent://sample/dev/dev-1":                                         1951642,
	"pulsar_storage_size_persistent://sample/dev/dev-2":                                         2029478,
	"pulsar_storage_size_persistent://sample/prod/prod-1":                                       2022420,
	"pulsar_storage_size_persistent://sample/prod/prod-2":                                       2108760,
	"pulsar_storage_size_public/functions":                                                      0,
	"pulsar_storage_size_sample/dev":                                                            3981120,
	"pulsar_storage_size_sample/prod":                                                           4131180,
	"pulsar_storage_write_latency_count":                                                        5867,
	"pulsar_storage_write_latency_count_persistent://public/functions/assignments":              0,
	"pulsar_storage_write_latency_count_persistent://public/functions/coordinate":               0,
	"pulsar_storage_write_latency_count_persistent://public/functions/metadata":                 0,
	"pulsar_storage_write_latency_count_persistent://sample/dev/dev-1":                          1448,
	"pulsar_storage_write_latency_count_persistent://sample/dev/dev-2":                          1477,
	"pulsar_storage_write_latency_count_persistent://sample/prod/prod-1":                        1469,
	"pulsar_storage_write_latency_count_persistent://sample/prod/prod-2":                        1473,
	"pulsar_storage_write_latency_count_public/functions":                                       0,
	"pulsar_storage_write_latency_count_sample/dev":                                             2925,
	"pulsar_storage_write_latency_count_sample/prod":                                            2942,
	"pulsar_storage_write_latency_le_0_5":                                                       0,
	"pulsar_storage_write_latency_le_0_5_persistent://public/functions/assignments":             0,
	"pulsar_storage_write_latency_le_0_5_persistent://public/functions/coordinate":              0,
	"pulsar_storage_write_latency_le_0_5_persistent://public/functions/metadata":                0,
	"pulsar_storage_write_latency_le_0_5_persistent://sample/dev/dev-1":                         0,
	"pulsar_storage_write_latency_le_0_5_persistent://sample/dev/dev-2":                         0,
	"pulsar_storage_write_latency_le_0_5_persistent://sample/prod/prod-1":                       0,
	"pulsar_storage_write_latency_le_0_5_persistent://sample/prod/prod-2":                       0,
	"pulsar_storage_write_latency_le_0_5_public/functions":                                      0,
	"pulsar_storage_write_latency_le_0_5_sample/dev":                                            0,
	"pulsar_storage_write_latency_le_0_5_sample/prod":                                           0,
	"pulsar_storage_write_latency_le_1":                                                         41,
	"pulsar_storage_write_latency_le_10":                                                        341,
	"pulsar_storage_write_latency_le_100":                                                       3,
	"pulsar_storage_write_latency_le_1000":                                                      0,
	"pulsar_storage_write_latency_le_1000_persistent://public/functions/assignments":            0,
	"pulsar_storage_write_latency_le_1000_persistent://public/functions/coordinate":             0,
	"pulsar_storage_write_latency_le_1000_persistent://public/functions/metadata":               0,
	"pulsar_storage_write_latency_le_1000_persistent://sample/dev/dev-1":                        0,
	"pulsar_storage_write_latency_le_1000_persistent://sample/dev/dev-2":                        0,
	"pulsar_storage_write_latency_le_1000_persistent://sample/prod/prod-1":                      0,
	"pulsar_storage_write_latency_le_1000_persistent://sample/prod/prod-2":                      0,
	"pulsar_storage_write_latency_le_1000_public/functions":                                     0,
	"pulsar_storage_write_latency_le_1000_sample/dev":                                           0,
	"pulsar_storage_write_latency_le_1000_sample/prod":                                          0,
	"pulsar_storage_write_latency_le_100_persistent://public/functions/assignments":             0,
	"pulsar_storage_write_latency_le_100_persistent://public/functions/coordinate":              0,
	"pulsar_storage_write_latency_le_100_persistent://public/functions/metadata":                0,
	"pulsar_storage_write_latency_le_100_persistent://sample/dev/dev-1":                         0,
	"pulsar_storage_write_latency_le_100_persistent://sample/dev/dev-2":                         1,
	"pulsar_storage_write_latency_le_100_persistent://sample/prod/prod-1":                       1,
	"pulsar_storage_write_latency_le_100_persistent://sample/prod/prod-2":                       1,
	"pulsar_storage_write_latency_le_100_public/functions":                                      0,
	"pulsar_storage_write_latency_le_100_sample/dev":                                            1,
	"pulsar_storage_write_latency_le_100_sample/prod":                                           2,
	"pulsar_storage_write_latency_le_10_persistent://public/functions/assignments":              0,
	"pulsar_storage_write_latency_le_10_persistent://public/functions/coordinate":               0,
	"pulsar_storage_write_latency_le_10_persistent://public/functions/metadata":                 0,
	"pulsar_storage_write_latency_le_10_persistent://sample/dev/dev-1":                          95,
	"pulsar_storage_write_latency_le_10_persistent://sample/dev/dev-2":                          82,
	"pulsar_storage_write_latency_le_10_persistent://sample/prod/prod-1":                        84,
	"pulsar_storage_write_latency_le_10_persistent://sample/prod/prod-2":                        80,
	"pulsar_storage_write_latency_le_10_public/functions":                                       0,
	"pulsar_storage_write_latency_le_10_sample/dev":                                             177,
	"pulsar_storage_write_latency_le_10_sample/prod":                                            164,
	"pulsar_storage_write_latency_le_1_persistent://public/functions/assignments":               0,
	"pulsar_storage_write_latency_le_1_persistent://public/functions/coordinate":                0,
	"pulsar_storage_write_latency_le_1_persistent://public/functions/metadata":                  0,
	"pulsar_storage_write_latency_le_1_persistent://sample/dev/dev-1":                           10,
	"pulsar_storage_write_latency_le_1_persistent://sample/dev/dev-2":                           15,
	"pulsar_storage_write_latency_le_1_persistent://sample/prod/prod-1":                         7,
	"pulsar_storage_write_latency_le_1_persistent://sample/prod/prod-2":                         9,
	"pulsar_storage_write_latency_le_1_public/functions":                                        0,
	"pulsar_storage_write_latency_le_1_sample/dev":                                              25,
	"pulsar_storage_write_latency_le_1_sample/prod":                                             16,
	"pulsar_storage_write_latency_le_20":                                                        114,
	"pulsar_storage_write_latency_le_200":                                                       0,
	"pulsar_storage_write_latency_le_200_persistent://public/functions/assignments":             0,
	"pulsar_storage_write_latency_le_200_persistent://public/functions/coordinate":              0,
	"pulsar_storage_write_latency_le_200_persistent://public/functions/metadata":                0,
	"pulsar_storage_write_latency_le_200_persistent://sample/dev/dev-1":                         0,
	"pulsar_storage_write_latency_le_200_persistent://sample/dev/dev-2":                         0,
	"pulsar_storage_write_latency_le_200_persistent://sample/prod/prod-1":                       0,
	"pulsar_storage_write_latency_le_200_persistent://sample/prod/prod-2":                       0,
	"pulsar_storage_write_latency_le_200_public/functions":                                      0,
	"pulsar_storage_write_latency_le_200_sample/dev":                                            0,
	"pulsar_storage_write_latency_le_200_sample/prod":                                           0,
	"pulsar_storage_write_latency_le_20_persistent://public/functions/assignments":              0,
	"pulsar_storage_write_latency_le_20_persistent://public/functions/coordinate":               0,
	"pulsar_storage_write_latency_le_20_persistent://public/functions/metadata":                 0,
	"pulsar_storage_write_latency_le_20_persistent://sample/dev/dev-1":                          26,
	"pulsar_storage_write_latency_le_20_persistent://sample/dev/dev-2":                          28,
	"pulsar_storage_write_latency_le_20_persistent://sample/prod/prod-1":                        26,
	"pulsar_storage_write_latency_le_20_persistent://sample/prod/prod-2":                        34,
	"pulsar_storage_write_latency_le_20_public/functions":                                       0,
	"pulsar_storage_write_latency_le_20_sample/dev":                                             54,
	"pulsar_storage_write_latency_le_20_sample/prod":                                            60,
	"pulsar_storage_write_latency_le_5":                                                         5328,
	"pulsar_storage_write_latency_le_50":                                                        40,
	"pulsar_storage_write_latency_le_50_persistent://public/functions/assignments":              0,
	"pulsar_storage_write_latency_le_50_persistent://public/functions/coordinate":               0,
	"pulsar_storage_write_latency_le_50_persistent://public/functions/metadata":                 0,
	"pulsar_storage_write_latency_le_50_persistent://sample/dev/dev-1":                          9,
	"pulsar_storage_write_latency_le_50_persistent://sample/dev/dev-2":                          9,
	"pulsar_storage_write_latency_le_50_persistent://sample/prod/prod-1":                        12,
	"pulsar_storage_write_latency_le_50_persistent://sample/prod/prod-2":                        10,
	"pulsar_storage_write_latency_le_50_public/functions":                                       0,
	"pulsar_storage_write_latency_le_50_sample/dev":                                             18,
	"pulsar_storage_write_latency_le_50_sample/prod":                                            22,
	"pulsar_storage_write_latency_le_5_persistent://public/functions/assignments":               0,
	"pulsar_storage_write_latency_le_5_persistent://public/functions/coordinate":                0,
	"pulsar_storage_write_latency_le_5_persistent://public/functions/metadata":                  0,
	"pulsar_storage_write_latency_le_5_persistent://sample/dev/dev-1":                           1308,
	"pulsar_storage_write_latency_le_5_persistent://sample/dev/dev-2":                           1342,
	"pulsar_storage_write_latency_le_5_persistent://sample/prod/prod-1":                         1339,
	"pulsar_storage_write_latency_le_5_persistent://sample/prod/prod-2":                         1339,
	"pulsar_storage_write_latency_le_5_public/functions":                                        0,
	"pulsar_storage_write_latency_le_5_sample/dev":                                              2650,
	"pulsar_storage_write_latency_le_5_sample/prod":                                             2678,
	"pulsar_storage_write_latency_overflow":                                                     0,
	"pulsar_storage_write_latency_overflow_persistent://public/functions/assignments":           0,
	"pulsar_storage_write_latency_overflow_persistent://public/functions/coordinate":            0,
	"pulsar_storage_write_latency_overflow_persistent://public/functions/metadata":              0,
	"pulsar_storage_write_latency_overflow_persistent://sample/dev/dev-1":                       0,
	"pulsar_storage_write_latency_overflow_persistent://sample/dev/dev-2":                       0,
	"pulsar_storage_write_latency_overflow_persistent://sample/prod/prod-1":                     0,
	"pulsar_storage_write_latency_overflow_persistent://sample/prod/prod-2":                     0,
	"pulsar_storage_write_latency_overflow_public/functions":                                    0,
	"pulsar_storage_write_latency_overflow_sample/dev":                                          0,
	"pulsar_storage_write_latency_overflow_sample/prod":                                         0,
	"pulsar_storage_write_latency_sum":                                                          5867,
	"pulsar_storage_write_latency_sum_persistent://public/functions/assignments":                0,
	"pulsar_storage_write_latency_sum_persistent://public/functions/coordinate":                 0,
	"pulsar_storage_write_latency_sum_persistent://public/functions/metadata":                   0,
	"pulsar_storage_write_latency_sum_persistent://sample/dev/dev-1":                            1448,
	"pulsar_storage_write_latency_sum_persistent://sample/dev/dev-2":                            1477,
	"pulsar_storage_write_latency_sum_persistent://sample/prod/prod-1":                          1469,
	"pulsar_storage_write_latency_sum_persistent://sample/prod/prod-2":                          1473,
	"pulsar_storage_write_latency_sum_public/functions":                                         0,
	"pulsar_storage_write_latency_sum_sample/dev":                                               2925,
	"pulsar_storage_write_latency_sum_sample/prod":                                              2942,
	"pulsar_subscription_blocked_on_unacked_messages":                                           0,
	"pulsar_subscription_blocked_on_unacked_messages_persistent://public/functions/assignments": 0,
	"pulsar_subscription_blocked_on_unacked_messages_persistent://public/functions/coordinate":  0,
	"pulsar_subscription_blocked_on_unacked_messages_persistent://public/functions/metadata":    0,
	"pulsar_subscription_blocked_on_unacked_messages_persistent://sample/dev/dev-1":             0,
	"pulsar_subscription_blocked_on_unacked_messages_persistent://sample/dev/dev-2":             0,
	"pulsar_subscription_blocked_on_unacked_messages_persistent://sample/prod/prod-1":           0,
	"pulsar_subscription_blocked_on_unacked_messages_persistent://sample/prod/prod-2":           0,
	"pulsar_subscription_blocked_on_unacked_messages_public/functions":                          0,
	"pulsar_subscription_blocked_on_unacked_messages_sample/dev":                                0,
	"pulsar_subscription_blocked_on_unacked_messages_sample/prod":                               0,
	"pulsar_subscription_delayed":                                                               0,
	"pulsar_subscription_delayed_persistent://public/functions/assignments":                     0,
	"pulsar_subscription_delayed_persistent://public/functions/coordinate":                      0,
	"pulsar_subscription_delayed_persistent://public/functions/metadata":                        0,
	"pulsar_subscription_delayed_persistent://sample/dev/dev-1":                                 0,
	"pulsar_subscription_delayed_persistent://sample/dev/dev-2":                                 0,
	"pulsar_subscription_delayed_persistent://sample/prod/prod-1":                               0,
	"pulsar_subscription_delayed_persistent://sample/prod/prod-2":                               0,
	"pulsar_subscription_delayed_public/functions":                                              0,
	"pulsar_subscription_delayed_sample/dev":                                                    0,
	"pulsar_subscription_delayed_sample/prod":                                                   0,
	"pulsar_subscription_msg_rate_redeliver":                                                    0,
	"pulsar_subscription_msg_rate_redeliver_persistent://public/functions/assignments":          0,
	"pulsar_subscription_msg_rate_redeliver_persistent://public/functions/coordinate":           0,
	"pulsar_subscription_msg_rate_redeliver_persistent://public/functions/metadata":             0,
	"pulsar_subscription_msg_rate_redeliver_persistent://sample/dev/dev-1":                      0,
	"pulsar_subscription_msg_rate_redeliver_persistent://sample/dev/dev-2":                      0,
	"pulsar_subscription_msg_rate_redeliver_persistent://sample/prod/prod-1":                    0,
	"pulsar_subscription_msg_rate_redeliver_persistent://sample/prod/prod-2":                    0,
	"pulsar_subscription_msg_rate_redeliver_public/functions":                                   0,
	"pulsar_subscription_msg_rate_redeliver_sample/dev":                                         0,
	"pulsar_subscription_msg_rate_redeliver_sample/prod":                                        0,
	"pulsar_subscriptions_count":                                                                13,
	"pulsar_subscriptions_count_persistent://public/functions/assignments":                      1,
	"pulsar_subscriptions_count_persistent://public/functions/coordinate":                       1,
	"pulsar_subscriptions_count_persistent://public/functions/metadata":                         1,
	"pulsar_subscriptions_count_persistent://sample/dev/dev-1":                                  2,
	"pulsar_subscriptions_count_persistent://sample/dev/dev-2":                                  4,
	"pulsar_subscriptions_count_persistent://sample/prod/prod-1":                                2,
	"pulsar_subscriptions_count_persistent://sample/prod/prod-2":                                2,
	"pulsar_subscriptions_count_public/functions":                                               3,
	"pulsar_subscriptions_count_sample/dev":                                                     6,
	"pulsar_subscriptions_count_sample/prod":                                                    4,
	"pulsar_throughput_in":                                                                      6023912,
	"pulsar_throughput_in_persistent://public/functions/assignments":                            0,
	"pulsar_throughput_in_persistent://public/functions/coordinate":                             0,
	"pulsar_throughput_in_persistent://public/functions/metadata":                               0,
	"pulsar_throughput_in_persistent://sample/dev/dev-1":                                        1450789,
	"pulsar_throughput_in_persistent://sample/dev/dev-2":                                        1450862,
	"pulsar_throughput_in_persistent://sample/prod/prod-1":                                      1561151,
	"pulsar_throughput_in_persistent://sample/prod/prod-2":                                      1561110,
	"pulsar_throughput_in_public/functions":                                                     0,
	"pulsar_throughput_in_sample/dev":                                                           2901651,
	"pulsar_throughput_in_sample/prod":                                                          3122261,
	"pulsar_throughput_out":                                                                     14949677,
	"pulsar_throughput_out_persistent://public/functions/assignments":                           0,
	"pulsar_throughput_out_persistent://public/functions/coordinate":                            0,
	"pulsar_throughput_out_persistent://public/functions/metadata":                              0,
	"pulsar_throughput_out_persistent://sample/dev/dev-1":                                       2901607,
	"pulsar_throughput_out_persistent://sample/dev/dev-2":                                       5803500,
	"pulsar_throughput_out_persistent://sample/prod/prod-1":                                     3122322,
	"pulsar_throughput_out_persistent://sample/prod/prod-2":                                     3122248,
	"pulsar_throughput_out_public/functions":                                                    0,
	"pulsar_throughput_out_sample/dev":                                                          8705107,
	"pulsar_throughput_out_sample/prod":                                                         6244570,
	"pulsar_topics_count":                                                                       14,
	"pulsar_topics_count_public/functions":                                                      5,
	"pulsar_topics_count_sample/dev":                                                            2,
	"pulsar_topics_count_sample/prod":                                                           7,
}

var expectedStandaloneV250TopicsFiltered = map[string]int64{
	"pulsar_consumers_count":                                           21,
	"pulsar_consumers_count_public/functions":                          3,
	"pulsar_consumers_count_sample/dev":                                10,
	"pulsar_consumers_count_sample/prod":                               8,
	"pulsar_entry_size_count":                                          5867,
	"pulsar_entry_size_count_public/functions":                         0,
	"pulsar_entry_size_count_sample/dev":                               2925,
	"pulsar_entry_size_count_sample/prod":                              2942,
	"pulsar_entry_size_le_100_kb":                                      0,
	"pulsar_entry_size_le_100_kb_public/functions":                     0,
	"pulsar_entry_size_le_100_kb_sample/dev":                           0,
	"pulsar_entry_size_le_100_kb_sample/prod":                          0,
	"pulsar_entry_size_le_128":                                         5867,
	"pulsar_entry_size_le_128_public/functions":                        0,
	"pulsar_entry_size_le_128_sample/dev":                              2925,
	"pulsar_entry_size_le_128_sample/prod":                             2942,
	"pulsar_entry_size_le_16_kb":                                       0,
	"pulsar_entry_size_le_16_kb_public/functions":                      0,
	"pulsar_entry_size_le_16_kb_sample/dev":                            0,
	"pulsar_entry_size_le_16_kb_sample/prod":                           0,
	"pulsar_entry_size_le_1_kb":                                        0,
	"pulsar_entry_size_le_1_kb_public/functions":                       0,
	"pulsar_entry_size_le_1_kb_sample/dev":                             0,
	"pulsar_entry_size_le_1_kb_sample/prod":                            0,
	"pulsar_entry_size_le_1_mb":                                        0,
	"pulsar_entry_size_le_1_mb_public/functions":                       0,
	"pulsar_entry_size_le_1_mb_sample/dev":                             0,
	"pulsar_entry_size_le_1_mb_sample/prod":                            0,
	"pulsar_entry_size_le_2_kb":                                        0,
	"pulsar_entry_size_le_2_kb_public/functions":                       0,
	"pulsar_entry_size_le_2_kb_sample/dev":                             0,
	"pulsar_entry_size_le_2_kb_sample/prod":                            0,
	"pulsar_entry_size_le_4_kb":                                        0,
	"pulsar_entry_size_le_4_kb_public/functions":                       0,
	"pulsar_entry_size_le_4_kb_sample/dev":                             0,
	"pulsar_entry_size_le_4_kb_sample/prod":                            0,
	"pulsar_entry_size_le_512":                                         0,
	"pulsar_entry_size_le_512_public/functions":                        0,
	"pulsar_entry_size_le_512_sample/dev":                              0,
	"pulsar_entry_size_le_512_sample/prod":                             0,
	"pulsar_entry_size_le_overflow":                                    0,
	"pulsar_entry_size_le_overflow_public/functions":                   0,
	"pulsar_entry_size_le_overflow_sample/dev":                         0,
	"pulsar_entry_size_le_overflow_sample/prod":                        0,
	"pulsar_entry_size_sum":                                            5867,
	"pulsar_entry_size_sum_public/functions":                           0,
	"pulsar_entry_size_sum_sample/dev":                                 2925,
	"pulsar_entry_size_sum_sample/prod":                                2942,
	"pulsar_msg_backlog":                                               0,
	"pulsar_msg_backlog_public/functions":                              0,
	"pulsar_msg_backlog_sample/dev":                                    0,
	"pulsar_msg_backlog_sample/prod":                                   0,
	"pulsar_namespaces_count":                                          3,
	"pulsar_producers_count":                                           10,
	"pulsar_producers_count_public/functions":                          2,
	"pulsar_producers_count_sample/dev":                                4,
	"pulsar_producers_count_sample/prod":                               4,
	"pulsar_rate_in":                                                   102064,
	"pulsar_rate_in_public/functions":                                  0,
	"pulsar_rate_in_sample/dev":                                        50027,
	"pulsar_rate_in_sample/prod":                                       52037,
	"pulsar_rate_out":                                                  254162,
	"pulsar_rate_out_public/functions":                                 0,
	"pulsar_rate_out_sample/dev":                                       150087,
	"pulsar_rate_out_sample/prod":                                      104075,
	"pulsar_storage_size":                                              8112300,
	"pulsar_storage_size_public/functions":                             0,
	"pulsar_storage_size_sample/dev":                                   3981120,
	"pulsar_storage_size_sample/prod":                                  4131180,
	"pulsar_storage_write_latency_count":                               5867,
	"pulsar_storage_write_latency_count_public/functions":              0,
	"pulsar_storage_write_latency_count_sample/dev":                    2925,
	"pulsar_storage_write_latency_count_sample/prod":                   2942,
	"pulsar_storage_write_latency_le_0_5":                              0,
	"pulsar_storage_write_latency_le_0_5_public/functions":             0,
	"pulsar_storage_write_latency_le_0_5_sample/dev":                   0,
	"pulsar_storage_write_latency_le_0_5_sample/prod":                  0,
	"pulsar_storage_write_latency_le_1":                                41,
	"pulsar_storage_write_latency_le_10":                               341,
	"pulsar_storage_write_latency_le_100":                              3,
	"pulsar_storage_write_latency_le_1000":                             0,
	"pulsar_storage_write_latency_le_1000_public/functions":            0,
	"pulsar_storage_write_latency_le_1000_sample/dev":                  0,
	"pulsar_storage_write_latency_le_1000_sample/prod":                 0,
	"pulsar_storage_write_latency_le_100_public/functions":             0,
	"pulsar_storage_write_latency_le_100_sample/dev":                   1,
	"pulsar_storage_write_latency_le_100_sample/prod":                  2,
	"pulsar_storage_write_latency_le_10_public/functions":              0,
	"pulsar_storage_write_latency_le_10_sample/dev":                    177,
	"pulsar_storage_write_latency_le_10_sample/prod":                   164,
	"pulsar_storage_write_latency_le_1_public/functions":               0,
	"pulsar_storage_write_latency_le_1_sample/dev":                     25,
	"pulsar_storage_write_latency_le_1_sample/prod":                    16,
	"pulsar_storage_write_latency_le_20":                               114,
	"pulsar_storage_write_latency_le_200":                              0,
	"pulsar_storage_write_latency_le_200_public/functions":             0,
	"pulsar_storage_write_latency_le_200_sample/dev":                   0,
	"pulsar_storage_write_latency_le_200_sample/prod":                  0,
	"pulsar_storage_write_latency_le_20_public/functions":              0,
	"pulsar_storage_write_latency_le_20_sample/dev":                    54,
	"pulsar_storage_write_latency_le_20_sample/prod":                   60,
	"pulsar_storage_write_latency_le_5":                                5328,
	"pulsar_storage_write_latency_le_50":                               40,
	"pulsar_storage_write_latency_le_50_public/functions":              0,
	"pulsar_storage_write_latency_le_50_sample/dev":                    18,
	"pulsar_storage_write_latency_le_50_sample/prod":                   22,
	"pulsar_storage_write_latency_le_5_public/functions":               0,
	"pulsar_storage_write_latency_le_5_sample/dev":                     2650,
	"pulsar_storage_write_latency_le_5_sample/prod":                    2678,
	"pulsar_storage_write_latency_overflow":                            0,
	"pulsar_storage_write_latency_overflow_public/functions":           0,
	"pulsar_storage_write_latency_overflow_sample/dev":                 0,
	"pulsar_storage_write_latency_overflow_sample/prod":                0,
	"pulsar_storage_write_latency_sum":                                 5867,
	"pulsar_storage_write_latency_sum_public/functions":                0,
	"pulsar_storage_write_latency_sum_sample/dev":                      2925,
	"pulsar_storage_write_latency_sum_sample/prod":                     2942,
	"pulsar_subscription_blocked_on_unacked_messages":                  0,
	"pulsar_subscription_blocked_on_unacked_messages_public/functions": 0,
	"pulsar_subscription_blocked_on_unacked_messages_sample/dev":       0,
	"pulsar_subscription_blocked_on_unacked_messages_sample/prod":      0,
	"pulsar_subscription_delayed":                                      0,
	"pulsar_subscription_delayed_public/functions":                     0,
	"pulsar_subscription_delayed_sample/dev":                           0,
	"pulsar_subscription_delayed_sample/prod":                          0,
	"pulsar_subscription_msg_rate_redeliver":                           0,
	"pulsar_subscription_msg_rate_redeliver_public/functions":          0,
	"pulsar_subscription_msg_rate_redeliver_sample/dev":                0,
	"pulsar_subscription_msg_rate_redeliver_sample/prod":               0,
	"pulsar_subscriptions_count":                                       13,
	"pulsar_subscriptions_count_public/functions":                      3,
	"pulsar_subscriptions_count_sample/dev":                            6,
	"pulsar_subscriptions_count_sample/prod":                           4,
	"pulsar_throughput_in":                                             6023912,
	"pulsar_throughput_in_public/functions":                            0,
	"pulsar_throughput_in_sample/dev":                                  2901651,
	"pulsar_throughput_in_sample/prod":                                 3122261,
	"pulsar_throughput_out":                                            14949677,
	"pulsar_throughput_out_public/functions":                           0,
	"pulsar_throughput_out_sample/dev":                                 8705107,
	"pulsar_throughput_out_sample/prod":                                6244570,
	"pulsar_topics_count":                                              14,
	"pulsar_topics_count_public/functions":                             5,
	"pulsar_topics_count_sample/dev":                                   2,
	"pulsar_topics_count_sample/prod":                                  7,
}
