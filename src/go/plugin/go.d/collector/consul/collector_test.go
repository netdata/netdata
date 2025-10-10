// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer1132Checks, _                        = os.ReadFile("testdata/v1.13.2/v1-agent-checks.json")
	dataVer1132ClientSelf, _                    = os.ReadFile("testdata/v1.13.2/client_v1-agent-self.json")
	dataVer1132ClientPromMetrics, _             = os.ReadFile("testdata/v1.13.2/client_v1-agent-metrics.txt")
	dataVer1132ServerSelf, _                    = os.ReadFile("testdata/v1.13.2/server_v1-agent-self.json")
	dataVer1132ServerSelfDisabledPrometheus, _  = os.ReadFile("testdata/v1.13.2/server_v1-agent-self_disabled_prom.json")
	dataVer1132ServerSelfWithHostname, _        = os.ReadFile("testdata/v1.13.2/server_v1-agent-self_with_hostname.json")
	dataVer1132ServerPromMetrics, _             = os.ReadFile("testdata/v1.13.2/server_v1-agent-metrics.txt")
	dataVer1132ServerPromMetricsWithHostname, _ = os.ReadFile("testdata/v1.13.2/server_v1-agent-metrics_with_hostname.txt")
	dataVer1132ServerOperatorAutopilotHealth, _ = os.ReadFile("testdata/v1.13.2/server_v1-operator-autopilot-health.json")
	dataVer1132ServerCoordinateNodes, _         = os.ReadFile("testdata/v1.13.2/server_v1-coordinate-nodes.json")

	dataVer1143CloudServerPromMetrics, _     = os.ReadFile("testdata/v1.14.3-cloud/server_v1-agent-metrics.txt")
	dataVer1143CloudServerSelf, _            = os.ReadFile("testdata/v1.14.3-cloud/server_v1-agent-self.json")
	dataVer1143CloudServerCoordinateNodes, _ = os.ReadFile("testdata/v1.14.3-cloud/server_v1-coordinate-nodes.json")
	dataVer1143CloudChecks, _                = os.ReadFile("testdata/v1.14.3-cloud/v1-agent-checks.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":                           dataConfigJSON,
		"dataConfigYAML":                           dataConfigYAML,
		"dataVer1132Checks":                        dataVer1132Checks,
		"dataVer1132ClientSelf":                    dataVer1132ClientSelf,
		"dataVer1132ClientPromMetrics":             dataVer1132ClientPromMetrics,
		"dataVer1132ServerSelf":                    dataVer1132ServerSelf,
		"dataVer1132ServerSelfWithHostname":        dataVer1132ServerSelfWithHostname,
		"dataVer1132ServerSelfDisabledPrometheus":  dataVer1132ServerSelfDisabledPrometheus,
		"dataVer1132ServerPromMetrics":             dataVer1132ServerPromMetrics,
		"dataVer1132ServerPromMetricsWithHostname": dataVer1132ServerPromMetricsWithHostname,
		"dataVer1132ServerOperatorAutopilotHealth": dataVer1132ServerOperatorAutopilotHealth,
		"dataVer1132ServerCoordinateNodes":         dataVer1132ServerCoordinateNodes,
		"dataVer1143CloudServerPromMetrics":        dataVer1143CloudServerPromMetrics,
		"dataVer1143CloudServerSelf":               dataVer1143CloudServerSelf,
		"dataVer1143CloudServerCoordinateNodes":    dataVer1143CloudServerCoordinateNodes,
		"dataVer1143CloudChecks":                   dataVer1143CloudChecks,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success with default": {
			wantFail: false,
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
		"success on response from Consul v1.13.2 server": {
			wantFail: false,
			prepare:  caseConsulV1132ServerResponse,
		},
		"success on response from Consul v1.14.3 server cloud managed": {
			wantFail: false,
			prepare:  caseConsulV1143CloudServerResponse,
		},
		"success on response from Consul v1.13.2 server with enabled hostname": {
			wantFail: false,
			prepare:  caseConsulV1132ServerWithHostnameResponse,
		},
		"success on response from Consul v1.13.2 server with disabled prometheus": {
			wantFail: false,
			prepare:  caseConsulV1132ServerWithDisabledPrometheus,
		},
		"success on response from Consul v1.13.2 client": {
			wantFail: false,
			prepare:  caseConsulV1132ClientResponse,
		},
		"fail on invalid data response": {
			wantFail: true,
			prepare:  caseInvalidDataResponse,
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

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare         func(t *testing.T) (collr *Collector, cleanup func())
		wantNumOfCharts int
		wantMetrics     map[string]int64
	}{
		"success on response from Consul v1.13.2 server": {
			prepare: caseConsulV1132ServerResponse,
			// 3 node, 1 service check, no license
			wantNumOfCharts: len(serverCommonCharts) + len(serverAutopilotHealthCharts) + len(serverLeaderCharts) + 3 + 1 - 1,
			wantMetrics: map[string]int64{
				"autopilot_failure_tolerance":               1,
				"autopilot_healthy_no":                      0,
				"autopilot_healthy_yes":                     1,
				"autopilot_server_healthy_no":               0,
				"autopilot_server_healthy_yes":              1,
				"autopilot_server_lastContact_leader":       13,
				"autopilot_server_sefStatus_alive":          1,
				"autopilot_server_sefStatus_failed":         0,
				"autopilot_server_sefStatus_left":           0,
				"autopilot_server_sefStatus_none":           0,
				"autopilot_server_stable_time":              265849,
				"autopilot_server_voter_no":                 0,
				"autopilot_server_voter_yes":                1,
				"client_rpc":                                6838,
				"client_rpc_exceeded":                       0,
				"client_rpc_failed":                         0,
				"health_check_chk1_critical_status":         0,
				"health_check_chk1_maintenance_status":      0,
				"health_check_chk1_passing_status":          1,
				"health_check_chk1_warning_status":          0,
				"health_check_chk2_critical_status":         1,
				"health_check_chk2_maintenance_status":      0,
				"health_check_chk2_passing_status":          0,
				"health_check_chk2_warning_status":          0,
				"health_check_chk3_critical_status":         1,
				"health_check_chk3_maintenance_status":      0,
				"health_check_chk3_passing_status":          0,
				"health_check_chk3_warning_status":          0,
				"health_check_mysql_critical_status":        1,
				"health_check_mysql_maintenance_status":     0,
				"health_check_mysql_passing_status":         0,
				"health_check_mysql_warning_status":         0,
				"kvs_apply_count":                           0,
				"kvs_apply_quantile=0.5":                    0,
				"kvs_apply_quantile=0.9":                    0,
				"kvs_apply_quantile=0.99":                   0,
				"kvs_apply_sum":                             0,
				"network_lan_rtt_avg":                       737592,
				"network_lan_rtt_count":                     2,
				"network_lan_rtt_max":                       991168,
				"network_lan_rtt_min":                       484017,
				"network_lan_rtt_sum":                       1475185,
				"raft_apply":                                10681000,
				"raft_boltdb_freelistBytes":                 11264,
				"raft_boltdb_logsPerBatch_count":            12360,
				"raft_boltdb_logsPerBatch_quantile=0.5":     1000000,
				"raft_boltdb_logsPerBatch_quantile=0.9":     1000000,
				"raft_boltdb_logsPerBatch_quantile=0.99":    1000000,
				"raft_boltdb_logsPerBatch_sum":              12362000,
				"raft_boltdb_storeLogs_count":               12360,
				"raft_boltdb_storeLogs_quantile=0.5":        13176624,
				"raft_boltdb_storeLogs_quantile=0.9":        13176624,
				"raft_boltdb_storeLogs_quantile=0.99":       13176624,
				"raft_boltdb_storeLogs_sum":                 651888027,
				"raft_commitTime_count":                     12345,
				"raft_commitTime_quantile=0.5":              41146488,
				"raft_commitTime_quantile=0.9":              41146488,
				"raft_commitTime_quantile=0.99":             41146488,
				"raft_commitTime_sum":                       955781149,
				"raft_fsm_lastRestoreDuration":              2,
				"raft_leader_lastContact_count":             80917,
				"raft_leader_lastContact_quantile=0.5":      33000000,
				"raft_leader_lastContact_quantile=0.9":      68000000,
				"raft_leader_lastContact_quantile=0.99":     68000000,
				"raft_leader_lastContact_sum":               3066900000,
				"raft_leader_oldestLogAge":                  166046464,
				"raft_rpc_installSnapshot_count":            0,
				"raft_rpc_installSnapshot_quantile=0.5":     0,
				"raft_rpc_installSnapshot_quantile=0.9":     0,
				"raft_rpc_installSnapshot_quantile=0.99":    0,
				"raft_rpc_installSnapshot_sum":              0,
				"raft_state_candidate":                      1,
				"raft_state_leader":                         1,
				"raft_thread_fsm_saturation_count":          11923,
				"raft_thread_fsm_saturation_quantile=0.5":   0,
				"raft_thread_fsm_saturation_quantile=0.9":   0,
				"raft_thread_fsm_saturation_quantile=0.99":  0,
				"raft_thread_fsm_saturation_sum":            90,
				"raft_thread_main_saturation_count":         43067,
				"raft_thread_main_saturation_quantile=0.5":  0,
				"raft_thread_main_saturation_quantile=0.9":  0,
				"raft_thread_main_saturation_quantile=0.99": 0,
				"raft_thread_main_saturation_sum":           205409,
				"runtime_alloc_bytes":                       53065368,
				"runtime_sys_bytes":                         84955160,
				"runtime_total_gc_pause_ns":                 1372001280,
				"server_isLeader_no":                        0,
				"server_isLeader_yes":                       1,
				"txn_apply_count":                           0,
				"txn_apply_quantile=0.5":                    0,
				"txn_apply_quantile=0.9":                    0,
				"txn_apply_quantile=0.99":                   0,
				"txn_apply_sum":                             0,
			},
		},
		"success on response from Consul v1.14.3 server cloud managed": {
			prepare: caseConsulV1143CloudServerResponse,
			// 3 node, 1 service check, license
			wantNumOfCharts: len(serverCommonCharts) + len(serverLeaderCharts) + 3 + 1,
			wantMetrics: map[string]int64{
				"autopilot_failure_tolerance":               0,
				"autopilot_healthy_no":                      0,
				"autopilot_healthy_yes":                     1,
				"client_rpc":                                438718,
				"client_rpc_exceeded":                       0,
				"client_rpc_failed":                         0,
				"health_check_chk1_critical_status":         0,
				"health_check_chk1_maintenance_status":      0,
				"health_check_chk1_passing_status":          1,
				"health_check_chk1_warning_status":          0,
				"health_check_chk2_critical_status":         1,
				"health_check_chk2_maintenance_status":      0,
				"health_check_chk2_passing_status":          0,
				"health_check_chk2_warning_status":          0,
				"health_check_chk3_critical_status":         1,
				"health_check_chk3_maintenance_status":      0,
				"health_check_chk3_passing_status":          0,
				"health_check_chk3_warning_status":          0,
				"health_check_mysql_critical_status":        1,
				"health_check_mysql_maintenance_status":     0,
				"health_check_mysql_passing_status":         0,
				"health_check_mysql_warning_status":         0,
				"kvs_apply_count":                           2,
				"kvs_apply_quantile=0.5":                    0,
				"kvs_apply_quantile=0.9":                    0,
				"kvs_apply_quantile=0.99":                   0,
				"kvs_apply_sum":                             18550,
				"network_lan_rtt_avg":                       1321107,
				"network_lan_rtt_count":                     1,
				"network_lan_rtt_max":                       1321107,
				"network_lan_rtt_min":                       1321107,
				"network_lan_rtt_sum":                       1321107,
				"raft_apply":                                115252000,
				"raft_boltdb_freelistBytes":                 26008,
				"raft_boltdb_logsPerBatch_count":            122794,
				"raft_boltdb_logsPerBatch_quantile=0.5":     1000000,
				"raft_boltdb_logsPerBatch_quantile=0.9":     1000000,
				"raft_boltdb_logsPerBatch_quantile=0.99":    1000000,
				"raft_boltdb_logsPerBatch_sum":              122856000,
				"raft_boltdb_storeLogs_count":               122794,
				"raft_boltdb_storeLogs_quantile=0.5":        1673303,
				"raft_boltdb_storeLogs_quantile=0.9":        2210979,
				"raft_boltdb_storeLogs_quantile=0.99":       2210979,
				"raft_boltdb_storeLogs_sum":                 278437403,
				"raft_commitTime_count":                     122785,
				"raft_commitTime_quantile=0.5":              1718204,
				"raft_commitTime_quantile=0.9":              2262192,
				"raft_commitTime_quantile=0.99":             2262192,
				"raft_commitTime_sum":                       284260428,
				"raft_fsm_lastRestoreDuration":              0,
				"raft_leader_lastContact_count":             19,
				"raft_leader_lastContact_quantile=0.5":      0,
				"raft_leader_lastContact_quantile=0.9":      0,
				"raft_leader_lastContact_quantile=0.99":     0,
				"raft_leader_lastContact_sum":               598000,
				"raft_leader_oldestLogAge":                  68835264,
				"raft_rpc_installSnapshot_count":            1,
				"raft_rpc_installSnapshot_quantile=0.5":     0,
				"raft_rpc_installSnapshot_quantile=0.9":     0,
				"raft_rpc_installSnapshot_quantile=0.99":    0,
				"raft_rpc_installSnapshot_sum":              473038,
				"raft_state_candidate":                      1,
				"raft_state_leader":                         1,
				"raft_thread_fsm_saturation_count":          44326,
				"raft_thread_fsm_saturation_quantile=0.5":   0,
				"raft_thread_fsm_saturation_quantile=0.9":   0,
				"raft_thread_fsm_saturation_quantile=0.99":  0,
				"raft_thread_fsm_saturation_sum":            729,
				"raft_thread_main_saturation_count":         451221,
				"raft_thread_main_saturation_quantile=0.5":  0,
				"raft_thread_main_saturation_quantile=0.9":  0,
				"raft_thread_main_saturation_quantile=0.99": 9999,
				"raft_thread_main_saturation_sum":           213059,
				"runtime_alloc_bytes":                       51729856,
				"runtime_sys_bytes":                         160156960,
				"runtime_total_gc_pause_ns":                 832754048,
				"server_isLeader_no":                        0,
				"server_isLeader_yes":                       1,
				"system_licenseExpiration":                  2949945,
				"txn_apply_count":                           0,
				"txn_apply_quantile=0.5":                    0,
				"txn_apply_quantile=0.9":                    0,
				"txn_apply_quantile=0.99":                   0,
				"txn_apply_sum":                             0,
			},
		},
		"success on response from Consul v1.13.2 server with enabled hostname": {
			prepare: caseConsulV1132ServerResponse,
			// 3 node, 1 service check, no license
			wantNumOfCharts: len(serverCommonCharts) + len(serverAutopilotHealthCharts) + len(serverLeaderCharts) + 3 + 1 - 1,
			wantMetrics: map[string]int64{
				"autopilot_failure_tolerance":               1,
				"autopilot_healthy_no":                      0,
				"autopilot_healthy_yes":                     1,
				"autopilot_server_healthy_no":               0,
				"autopilot_server_healthy_yes":              1,
				"autopilot_server_lastContact_leader":       13,
				"autopilot_server_sefStatus_alive":          1,
				"autopilot_server_sefStatus_failed":         0,
				"autopilot_server_sefStatus_left":           0,
				"autopilot_server_sefStatus_none":           0,
				"autopilot_server_stable_time":              265825,
				"autopilot_server_voter_no":                 0,
				"autopilot_server_voter_yes":                1,
				"client_rpc":                                6838,
				"client_rpc_exceeded":                       0,
				"client_rpc_failed":                         0,
				"health_check_chk1_critical_status":         0,
				"health_check_chk1_maintenance_status":      0,
				"health_check_chk1_passing_status":          1,
				"health_check_chk1_warning_status":          0,
				"health_check_chk2_critical_status":         1,
				"health_check_chk2_maintenance_status":      0,
				"health_check_chk2_passing_status":          0,
				"health_check_chk2_warning_status":          0,
				"health_check_chk3_critical_status":         1,
				"health_check_chk3_maintenance_status":      0,
				"health_check_chk3_passing_status":          0,
				"health_check_chk3_warning_status":          0,
				"health_check_mysql_critical_status":        1,
				"health_check_mysql_maintenance_status":     0,
				"health_check_mysql_passing_status":         0,
				"health_check_mysql_warning_status":         0,
				"kvs_apply_count":                           0,
				"kvs_apply_quantile=0.5":                    0,
				"kvs_apply_quantile=0.9":                    0,
				"kvs_apply_quantile=0.99":                   0,
				"kvs_apply_sum":                             0,
				"network_lan_rtt_avg":                       737592,
				"network_lan_rtt_count":                     2,
				"network_lan_rtt_max":                       991168,
				"network_lan_rtt_min":                       484017,
				"network_lan_rtt_sum":                       1475185,
				"raft_apply":                                10681000,
				"raft_boltdb_freelistBytes":                 11264,
				"raft_boltdb_logsPerBatch_count":            12360,
				"raft_boltdb_logsPerBatch_quantile=0.5":     1000000,
				"raft_boltdb_logsPerBatch_quantile=0.9":     1000000,
				"raft_boltdb_logsPerBatch_quantile=0.99":    1000000,
				"raft_boltdb_logsPerBatch_sum":              12362000,
				"raft_boltdb_storeLogs_count":               12360,
				"raft_boltdb_storeLogs_quantile=0.5":        13176624,
				"raft_boltdb_storeLogs_quantile=0.9":        13176624,
				"raft_boltdb_storeLogs_quantile=0.99":       13176624,
				"raft_boltdb_storeLogs_sum":                 651888027,
				"raft_commitTime_count":                     12345,
				"raft_commitTime_quantile=0.5":              41146488,
				"raft_commitTime_quantile=0.9":              41146488,
				"raft_commitTime_quantile=0.99":             41146488,
				"raft_commitTime_sum":                       955781149,
				"raft_fsm_lastRestoreDuration":              2,
				"raft_leader_lastContact_count":             80917,
				"raft_leader_lastContact_quantile=0.5":      33000000,
				"raft_leader_lastContact_quantile=0.9":      68000000,
				"raft_leader_lastContact_quantile=0.99":     68000000,
				"raft_leader_lastContact_sum":               3066900000,
				"raft_leader_oldestLogAge":                  166046464,
				"raft_rpc_installSnapshot_count":            0,
				"raft_rpc_installSnapshot_quantile=0.5":     0,
				"raft_rpc_installSnapshot_quantile=0.9":     0,
				"raft_rpc_installSnapshot_quantile=0.99":    0,
				"raft_rpc_installSnapshot_sum":              0,
				"raft_state_candidate":                      1,
				"raft_state_leader":                         1,
				"raft_thread_fsm_saturation_count":          11923,
				"raft_thread_fsm_saturation_quantile=0.5":   0,
				"raft_thread_fsm_saturation_quantile=0.9":   0,
				"raft_thread_fsm_saturation_quantile=0.99":  0,
				"raft_thread_fsm_saturation_sum":            90,
				"raft_thread_main_saturation_count":         43067,
				"raft_thread_main_saturation_quantile=0.5":  0,
				"raft_thread_main_saturation_quantile=0.9":  0,
				"raft_thread_main_saturation_quantile=0.99": 0,
				"raft_thread_main_saturation_sum":           205409,
				"runtime_alloc_bytes":                       53065368,
				"runtime_sys_bytes":                         84955160,
				"runtime_total_gc_pause_ns":                 1372001280,
				"server_isLeader_no":                        0,
				"server_isLeader_yes":                       1,
				"txn_apply_count":                           0,
				"txn_apply_quantile=0.5":                    0,
				"txn_apply_quantile=0.9":                    0,
				"txn_apply_quantile=0.99":                   0,
				"txn_apply_sum":                             0,
			},
		},
		"success on response from Consul v1.13.2 server with disabled prometheus": {
			prepare: caseConsulV1132ServerWithDisabledPrometheus,
			// 3 node, 1 service check, no license
			wantNumOfCharts: len(serverAutopilotHealthCharts) + 3 + 1,
			wantMetrics: map[string]int64{
				"autopilot_server_healthy_no":           0,
				"autopilot_server_healthy_yes":          1,
				"autopilot_server_lastContact_leader":   13,
				"autopilot_server_sefStatus_alive":      1,
				"autopilot_server_sefStatus_failed":     0,
				"autopilot_server_sefStatus_left":       0,
				"autopilot_server_sefStatus_none":       0,
				"autopilot_server_stable_time":          265805,
				"autopilot_server_voter_no":             0,
				"autopilot_server_voter_yes":            1,
				"health_check_chk1_critical_status":     0,
				"health_check_chk1_maintenance_status":  0,
				"health_check_chk1_passing_status":      1,
				"health_check_chk1_warning_status":      0,
				"health_check_chk2_critical_status":     1,
				"health_check_chk2_maintenance_status":  0,
				"health_check_chk2_passing_status":      0,
				"health_check_chk2_warning_status":      0,
				"health_check_chk3_critical_status":     1,
				"health_check_chk3_maintenance_status":  0,
				"health_check_chk3_passing_status":      0,
				"health_check_chk3_warning_status":      0,
				"health_check_mysql_critical_status":    1,
				"health_check_mysql_maintenance_status": 0,
				"health_check_mysql_passing_status":     0,
				"health_check_mysql_warning_status":     0,
				"network_lan_rtt_avg":                   737592,
				"network_lan_rtt_count":                 2,
				"network_lan_rtt_max":                   991168,
				"network_lan_rtt_min":                   484017,
				"network_lan_rtt_sum":                   1475185,
			},
		},
		"success on response from Consul v1.13.2 client": {
			prepare: caseConsulV1132ClientResponse,
			// 3 node, 1 service check, no license
			wantNumOfCharts: len(clientCharts) + 3 + 1 - 1,
			wantMetrics: map[string]int64{
				"client_rpc":                            34,
				"client_rpc_exceeded":                   0,
				"client_rpc_failed":                     0,
				"health_check_chk1_critical_status":     0,
				"health_check_chk1_maintenance_status":  0,
				"health_check_chk1_passing_status":      1,
				"health_check_chk1_warning_status":      0,
				"health_check_chk2_critical_status":     1,
				"health_check_chk2_maintenance_status":  0,
				"health_check_chk2_passing_status":      0,
				"health_check_chk2_warning_status":      0,
				"health_check_chk3_critical_status":     1,
				"health_check_chk3_maintenance_status":  0,
				"health_check_chk3_passing_status":      0,
				"health_check_chk3_warning_status":      0,
				"health_check_mysql_critical_status":    1,
				"health_check_mysql_maintenance_status": 0,
				"health_check_mysql_passing_status":     0,
				"health_check_mysql_warning_status":     0,
				"runtime_alloc_bytes":                   26333408,
				"runtime_sys_bytes":                     51201032,
				"runtime_total_gc_pause_ns":             4182423,
			},
		},
		"fail on invalid data response": {
			prepare:         caseInvalidDataResponse,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
		"fail on connection refused": {
			prepare:         caseConnectionRefused,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
		"fail on 404 response": {
			prepare:         case404,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			mx := collr.Collect(context.Background())

			delete(mx, "autopilot_server_stable_time")
			delete(test.wantMetrics, "autopilot_server_stable_time")

			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantNumOfCharts, len(*collr.Charts()))
			}
		})
	}
}

func caseConsulV1143CloudServerResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == urlPathAgentSelf:
				_, _ = w.Write(dataVer1143CloudServerSelf)
			case r.URL.Path == urlPathAgentChecks:
				_, _ = w.Write(dataVer1143CloudChecks)
			case r.URL.Path == urlPathAgentMetrics && r.URL.RawQuery == "format=prometheus":
				_, _ = w.Write(dataVer1143CloudServerPromMetrics)
			case r.URL.Path == urlPathOperationAutopilotHealth:
				w.WriteHeader(http.StatusForbidden)
			case r.URL.Path == urlPathCoordinateNodes:
				_, _ = w.Write(dataVer1143CloudServerCoordinateNodes)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	collr := New()
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConsulV1132ServerResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == urlPathAgentSelf:
				_, _ = w.Write(dataVer1132ServerSelf)
			case r.URL.Path == urlPathAgentChecks:
				_, _ = w.Write(dataVer1132Checks)
			case r.URL.Path == urlPathAgentMetrics && r.URL.RawQuery == "format=prometheus":
				_, _ = w.Write(dataVer1132ServerPromMetrics)
			case r.URL.Path == urlPathOperationAutopilotHealth:
				_, _ = w.Write(dataVer1132ServerOperatorAutopilotHealth)
			case r.URL.Path == urlPathCoordinateNodes:
				_, _ = w.Write(dataVer1132ServerCoordinateNodes)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	collr := New()
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConsulV1132ServerWithHostnameResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == urlPathAgentSelf:
				_, _ = w.Write(dataVer1132ServerSelfWithHostname)
			case r.URL.Path == urlPathAgentChecks:
				_, _ = w.Write(dataVer1132Checks)
			case r.URL.Path == urlPathAgentMetrics && r.URL.RawQuery == "format=prometheus":
				_, _ = w.Write(dataVer1132ServerPromMetricsWithHostname)
			case r.URL.Path == urlPathOperationAutopilotHealth:
				_, _ = w.Write(dataVer1132ServerOperatorAutopilotHealth)
			case r.URL.Path == urlPathCoordinateNodes:
				_, _ = w.Write(dataVer1132ServerCoordinateNodes)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	collr := New()
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConsulV1132ServerWithDisabledPrometheus(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathAgentSelf:
				_, _ = w.Write(dataVer1132ServerSelfDisabledPrometheus)
			case urlPathAgentChecks:
				_, _ = w.Write(dataVer1132Checks)
			case urlPathOperationAutopilotHealth:
				_, _ = w.Write(dataVer1132ServerOperatorAutopilotHealth)
			case urlPathCoordinateNodes:
				_, _ = w.Write(dataVer1132ServerCoordinateNodes)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	collr := New()
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConsulV1132ClientResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == urlPathAgentSelf:
				_, _ = w.Write(dataVer1132ClientSelf)
			case r.URL.Path == urlPathAgentChecks:
				_, _ = w.Write(dataVer1132Checks)
			case r.URL.Path == urlPathAgentMetrics && r.URL.RawQuery == "format=prometheus":
				_, _ = w.Write(dataVer1132ClientPromMetrics)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	collr := New()
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseInvalidDataResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	collr := New()
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:65535/"
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
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}
