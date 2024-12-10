// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer16ApiHealthMinimal, _ = os.ReadFile("testdata/v16.2.15/api_health_minimal.json")
	dataVer16ApiOsd, _           = os.ReadFile("testdata/v16.2.15/api_osd.json")
	dataVer16ApiPoolStats, _     = os.ReadFile("testdata/v16.2.15/api_pool_stats.json")
	dataVer16ApiMonitor, _       = os.ReadFile("testdata/v16.2.15/api_monitor.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":            dataConfigJSON,
		"dataConfigYAML":            dataConfigYAML,
		"dataVer16ApiHealthMinimal": dataVer16ApiHealthMinimal,
		"dataVer16ApiOsd":           dataVer16ApiOsd,
		"dataVer16ApiPoolStats":     dataVer16ApiPoolStats,
		"dataVer16ApiMonitor":       dataVer16ApiMonitor,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"fails with default": {
			wantFail: true,
			config:   New().Config,
		},
		"fail when URL not set": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (collr *Collector, cleanup func())
	}{
		"success with valid API key": {
			wantFail: false,
			prepare:  caseOk,
		},
		"fail on connection refused": {
			wantFail: true,
			prepare:  caseConnectionRefused,
		},
		"fail on 404 response": {
			wantFail: true,
			prepare:  case404,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare         func(t *testing.T) (collr *Collector, cleanup func())
		wantNumOfCharts int
		wantMetrics     map[string]int64
	}{
		"success with valid API key": {
			prepare:         caseOk,
			wantNumOfCharts: len(clusterCharts) + len(osdChartsTmpl)*2 + len(poolChartsTmpl)*2,
			wantMetrics: map[string]int64{
				"client_perf_read_bytes_sec":           1,
				"client_perf_read_op_per_sec":          1,
				"client_perf_recovering_bytes_per_sec": 1,
				"client_perf_write_bytes_sec":          1,
				"client_perf_write_op_per_sec":         1,
				"health_err":                           0,
				"health_ok":                            0,
				"health_warn":                          1,
				"hosts_num":                            1,
				"iscsi_daemons_down_num":               1,
				"iscsi_daemons_num":                    2,
				"iscsi_daemons_up_num":                 1,
				"mgr_active_num":                       1,
				"mgr_standby_num":                      1,
				"monitors_num":                         1,
				"objects_degraded_num":                 1,
				"objects_healthy_num":                  3,
				"objects_misplaced_num":                1,
				"objects_num":                          6,
				"objects_unfound_num":                  1,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_apply_latency_ms":  1,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_commit_latency_ms": 1,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_read_bytes":        1,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_read_ops":          1,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_size_bytes":        68715282432,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_space_avail_bytes": 68410753024,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_space_used_bytes":  304529408,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_status_down":       0,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_status_in":         1,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_status_out":        0,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_status_up":         1,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_write_ops":         1,
				"osd_f5bbbe9d-e85b-419c-af5a-a57e2527cad3_written_bytes":     1,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_apply_latency_ms":  1,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_commit_latency_ms": 1,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_read_bytes":        1,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_read_ops":          1,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_size_bytes":        107369988096,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_space_avail_bytes": 107065458688,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_space_used_bytes":  304529408,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_status_down":       0,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_status_in":         1,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_status_out":        0,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_status_up":         1,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_write_ops":         1,
				"osd_f78537db-9b18-4c62-a24f-a4344fc28de7_written_bytes":     1,
				"osds_down_num":                                0,
				"osds_in_num":                                  2,
				"osds_num":                                     2,
				"osds_out_num":                                 0,
				"osds_up_num":                                  2,
				"pg_status_category_clean":                     1,
				"pg_status_category_unknown":                   0,
				"pg_status_category_warning":                   1,
				"pg_status_category_working":                   0,
				"pgs_num":                                      2,
				"pgs_per_osd":                                  2,
				"pool_device_health_metrics_objects":           3,
				"pool_device_health_metrics_read_bytes":        1,
				"pool_device_health_metrics_read_ops":          1,
				"pool_device_health_metrics_size":              166530172973,
				"pool_device_health_metrics_space_avail_bytes": 166530172972,
				"pool_device_health_metrics_space_used_bytes":  1,
				"pool_device_health_metrics_space_utilization": 1000,
				"pool_device_health_metrics_write_ops":         3,
				"pool_device_health_metrics_written_bytes":     6144,
				"pool_mySuperPool_objects":                     1,
				"pool_mySuperPool_read_bytes":                  1,
				"pool_mySuperPool_read_ops":                    1,
				"pool_mySuperPool_size":                        166530172973,
				"pool_mySuperPool_space_avail_bytes":           166530172972,
				"pool_mySuperPool_space_used_bytes":            1,
				"pool_mySuperPool_space_utilization":           1000,
				"pool_mySuperPool_write_ops":                   1,
				"pool_mySuperPool_written_bytes":               1,
				"pools_num":                                    2,
				"raw_capacity_avail_bytes":                     175476178944,
				"raw_capacity_used_bytes":                      609091584,
				"raw_capacity_utilization":                     345,
				"rgw_num":                                      1,
				"scrub_status_active":                          0,
				"scrub_status_disabled":                        0,
				"scrub_status_inactive":                        1,
			},
		},
		"fail on connection refused": {
			prepare:     caseConnectionRefused,
			wantMetrics: nil,
		},
		"fail on 404 response": {
			prepare:     case404,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			_ = collr.Check(context.Background())

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantNumOfCharts, len(*collr.Charts()), "want charts")

				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func caseOk(t *testing.T) (*Collector, func()) {
	t.Helper()

	loginResp, _ := json.Marshal(authLoginResp{Token: "secret_token"})
	checkResp, _ := json.Marshal(authCheckResp{Username: "username"})
	var loggedIn atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				switch r.URL.Path {
				case urlPathApiAuth:
					_, _ = w.Write(loginResp)
					w.WriteHeader(http.StatusCreated)
					loggedIn.Store(true)
				case urlPathApiAuthCheck:
					bs, _ := io.ReadAll(r.Body)
					if bytes.Equal(bs, loginResp) {
						_, _ = w.Write(checkResp)
					} else {
						w.WriteHeader(http.StatusNotFound)
					}
				case urlPathApiAuthLogout:
					w.WriteHeader(http.StatusOK)
					loggedIn.Store(false)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			case http.MethodGet:
				if !loggedIn.Load() {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				switch r.URL.Path {
				case urlPathApiHealthMinimal:
					_, _ = w.Write(dataVer16ApiHealthMinimal)
				case urlPathApiOsd:
					_, _ = w.Write(dataVer16ApiOsd)
				case urlPathApiPool:
					if r.URL.RawQuery != urlQueryApiPool {
						w.WriteHeader(http.StatusNotFound)
					} else {
						_, _ = w.Write(dataVer16ApiPoolStats)
					}
				case urlPathApiMonitor:
					_, _ = w.Write(dataVer16ApiMonitor)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}
		}))

	collr := New()
	collr.URL = srv.URL
	collr.Username = "user"
	collr.Password = "password"
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:65001"
	collr.Username = "user"
	collr.Password = "password"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
}

func case404(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL
	collr.Username = "user"
	collr.Password = "password"
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}
