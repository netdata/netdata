// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"fmt"

	"github.com/blang/semver/v4"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	_ = collectorapi.Priority + iota
	prioKVSApplyTime
	prioKVSApplyOperations
	prioTXNApplyTime
	prioTXNApplyOperations
	prioRaftCommitTime
	prioRaftCommitsRate

	prioServerLeadershipStatus
	prioRaftLeaderLastContactTime
	prioRaftFollowerLastContactLeaderTime
	prioRaftLeaderElections
	prioRaftLeadershipTransitions

	prioAutopilotClusterHealthStatus
	prioAutopilotFailureTolerance
	prioAutopilotServerHealthStatus
	prioAutopilotServerStableTime
	prioAutopilotServerSerfStatus
	prioAutopilotServerVoterStatus

	prioNetworkLanRTT

	prioRPCRequests
	prioRPCRequestsExceeded
	prioRPCRequestsFailed

	prioRaftThreadMainSaturation
	prioRaftThreadFSMSaturation

	prioRaftFSMLastRestoreDuration
	prioRaftLeaderOldestLogAge
	prioRaftRPCInstallSnapshotTime

	prioBoltDBFreelistBytes
	prioBoltDBLogsPerBatch
	prioBoltDBStoreLogsTime

	prioMemoryAllocated
	prioMemorySys
	prioGCPauseTime

	prioServiceHealthCheckStatus
	prioNodeHealthCheckStatus

	prioLicenseExpirationTime
)

var (
	clientCharts = collectorapi.Charts{
		clientRPCRequestsRateChart.Copy(),
		clientRPCRequestsExceededRateChart.Copy(),
		clientRPCRequestsFailedRateChart.Copy(),

		memoryAllocatedChart.Copy(),
		memorySysChart.Copy(),
		gcPauseTimeChart.Copy(),

		licenseExpirationTimeChart.Copy(),
	}

	serverLeaderCharts = collectorapi.Charts{
		raftCommitTimeChart.Copy(),
		raftLeaderLastContactTimeChart.Copy(),
		raftCommitsRateChart.Copy(),
		raftLeaderOldestLogAgeChart.Copy(),
	}
	serverFollowerCharts = collectorapi.Charts{
		raftFollowerLastContactLeaderTimeChart.Copy(),
		raftRPCInstallSnapshotTimeChart.Copy(),
	}
	serverAutopilotHealthCharts = collectorapi.Charts{
		autopilotServerHealthStatusChart.Copy(),
		autopilotServerStableTimeChart.Copy(),
		autopilotServerSerfStatusChart.Copy(),
		autopilotServerVoterStatusChart.Copy(),
	}
	serverCommonCharts = collectorapi.Charts{
		kvsApplyTimeChart.Copy(),
		kvsApplyOperationsRateChart.Copy(),
		txnApplyTimeChart.Copy(),
		txnApplyOperationsRateChart.Copy(),

		autopilotClusterHealthStatusChart.Copy(),
		autopilotFailureTolerance.Copy(),

		raftLeaderElectionsRateChart.Copy(),
		raftLeadershipTransitionsRateChart.Copy(),
		serverLeadershipStatusChart.Copy(),

		networkLanRTTChart.Copy(),

		clientRPCRequestsRateChart.Copy(),
		clientRPCRequestsExceededRateChart.Copy(),
		clientRPCRequestsFailedRateChart.Copy(),

		raftThreadMainSaturationPercChart.Copy(),
		raftThreadFSMSaturationPercChart.Copy(),

		raftFSMLastRestoreDurationChart.Copy(),

		raftBoltDBFreelistBytesChart.Copy(),
		raftBoltDBLogsPerBatchChart.Copy(),
		raftBoltDBStoreLogsTimeChart.Copy(),

		memoryAllocatedChart.Copy(),
		memorySysChart.Copy(),
		gcPauseTimeChart.Copy(),

		licenseExpirationTimeChart.Copy(),
	}

	kvsApplyTimeChart = collectorapi.Chart{
		ID:       "kvs_apply_time",
		Title:    "KVS apply time",
		Units:    "ms",
		Fam:      "transaction timing",
		Ctx:      "consul.kvs_apply_time",
		Priority: prioKVSApplyTime,
		Dims: collectorapi.Dims{
			{ID: "kvs_apply_quantile=0.5", Name: "quantile_0.5", Div: precision * precision},
			{ID: "kvs_apply_quantile=0.9", Name: "quantile_0.9", Div: precision * precision},
			{ID: "kvs_apply_quantile=0.99", Name: "quantile_0.99", Div: precision * precision},
		},
	}
	kvsApplyOperationsRateChart = collectorapi.Chart{
		ID:       "kvs_apply_operations_rate",
		Title:    "KVS apply operations",
		Units:    "ops/s",
		Fam:      "transaction timing",
		Ctx:      "consul.kvs_apply_operations_rate",
		Priority: prioKVSApplyOperations,
		Dims: collectorapi.Dims{
			{ID: "kvs_apply_count", Name: "kvs_apply"},
		},
	}
	txnApplyTimeChart = collectorapi.Chart{
		ID:       "txn_apply_time",
		Title:    "Transaction apply time",
		Units:    "ms",
		Fam:      "transaction timing",
		Ctx:      "consul.txn_apply_time",
		Priority: prioTXNApplyTime,
		Dims: collectorapi.Dims{
			{ID: "txn_apply_quantile=0.5", Name: "quantile_0.5", Div: precision * precision},
			{ID: "txn_apply_quantile=0.9", Name: "quantile_0.9", Div: precision * precision},
			{ID: "txn_apply_quantile=0.99", Name: "quantile_0.99", Div: precision * precision},
		},
	}
	txnApplyOperationsRateChart = collectorapi.Chart{
		ID:       "txn_apply_operations_rate",
		Title:    "Transaction apply operations",
		Units:    "ops/s",
		Fam:      "transaction timing",
		Ctx:      "consul.txn_apply_operations_rate",
		Priority: prioTXNApplyOperations,
		Dims: collectorapi.Dims{
			{ID: "txn_apply_count", Name: "kvs_apply"},
		},
	}

	raftCommitTimeChart = collectorapi.Chart{
		ID:       "raft_commit_time",
		Title:    "Raft commit time",
		Units:    "ms",
		Fam:      "transaction timing",
		Ctx:      "consul.raft_commit_time",
		Priority: prioRaftCommitTime,
		Dims: collectorapi.Dims{
			{ID: "raft_commitTime_quantile=0.5", Name: "quantile_0.5", Div: precision * precision},
			{ID: "raft_commitTime_quantile=0.9", Name: "quantile_0.9", Div: precision * precision},
			{ID: "raft_commitTime_quantile=0.99", Name: "quantile_0.99", Div: precision * precision},
		},
	}
	raftCommitsRateChart = collectorapi.Chart{
		ID:       "raft_commits_rate",
		Title:    "Raft commits rate",
		Units:    "commits/s",
		Fam:      "transaction timing",
		Ctx:      "consul.raft_commits_rate",
		Priority: prioRaftCommitsRate,
		Dims: collectorapi.Dims{
			{ID: "raft_apply", Name: "commits", Div: precision, Algo: collectorapi.Incremental},
		},
	}

	autopilotClusterHealthStatusChart = collectorapi.Chart{
		ID:       "autopilot_health_status",
		Title:    "Autopilot cluster health status",
		Units:    "status",
		Fam:      "autopilot",
		Ctx:      "consul.autopilot_health_status",
		Priority: prioAutopilotClusterHealthStatus,
		Dims: collectorapi.Dims{
			{ID: "autopilot_healthy_yes", Name: "healthy"},
			{ID: "autopilot_healthy_no", Name: "unhealthy"},
		},
	}
	autopilotFailureTolerance = collectorapi.Chart{
		ID:       "autopilot_failure_tolerance",
		Title:    "Autopilot cluster failure tolerance",
		Units:    "servers",
		Fam:      "autopilot",
		Ctx:      "consul.autopilot_failure_tolerance",
		Priority: prioAutopilotFailureTolerance,
		Dims: collectorapi.Dims{
			{ID: "autopilot_failure_tolerance", Name: "failure_tolerance"},
		},
	}
	autopilotServerHealthStatusChart = collectorapi.Chart{
		ID:       "autopilot_server_health_status",
		Title:    "Autopilot server health status",
		Units:    "status",
		Fam:      "autopilot",
		Ctx:      "consul.autopilot_server_health_status",
		Priority: prioAutopilotServerHealthStatus,
		Dims: collectorapi.Dims{
			{ID: "autopilot_server_healthy_yes", Name: "healthy"},
			{ID: "autopilot_server_healthy_no", Name: "unhealthy"},
		},
	}
	autopilotServerStableTimeChart = collectorapi.Chart{
		ID:       "autopilot_server_stable_time",
		Title:    "Autopilot server stable time",
		Units:    "seconds",
		Fam:      "autopilot",
		Ctx:      "consul.autopilot_server_stable_time",
		Priority: prioAutopilotServerStableTime,
		Dims: collectorapi.Dims{
			{ID: "autopilot_server_stable_time", Name: "stable"},
		},
	}
	autopilotServerSerfStatusChart = collectorapi.Chart{
		ID:       "autopilot_server_serf_status",
		Title:    "Autopilot server Serf status",
		Units:    "status",
		Fam:      "autopilot",
		Ctx:      "consul.autopilot_server_serf_status",
		Priority: prioAutopilotServerSerfStatus,
		Dims: collectorapi.Dims{
			{ID: "autopilot_server_sefStatus_alive", Name: "alive"},
			{ID: "autopilot_server_sefStatus_failed", Name: "failed"},
			{ID: "autopilot_server_sefStatus_left", Name: "left"},
			{ID: "autopilot_server_sefStatus_none", Name: "none"},
		},
	}
	autopilotServerVoterStatusChart = collectorapi.Chart{
		ID:       "autopilot_server_voter_status",
		Title:    "Autopilot server Raft voting membership",
		Units:    "status",
		Fam:      "autopilot",
		Ctx:      "consul.autopilot_server_voter_status",
		Priority: prioAutopilotServerVoterStatus,
		Dims: collectorapi.Dims{
			{ID: "autopilot_server_voter_yes", Name: "voter"},
			{ID: "autopilot_server_voter_no", Name: "not_voter"},
		},
	}

	raftLeaderLastContactTimeChart = collectorapi.Chart{
		ID:       "raft_leader_last_contact_time",
		Title:    "Raft leader last contact time",
		Units:    "ms",
		Fam:      "leadership changes",
		Ctx:      "consul.raft_leader_last_contact_time",
		Priority: prioRaftLeaderLastContactTime,
		Dims: collectorapi.Dims{
			{ID: "raft_leader_lastContact_quantile=0.5", Name: "quantile_0.5", Div: precision * precision},
			{ID: "raft_leader_lastContact_quantile=0.9", Name: "quantile_0.9", Div: precision * precision},
			{ID: "raft_leader_lastContact_quantile=0.99", Name: "quantile_0.99", Div: precision * precision},
		},
	}
	raftFollowerLastContactLeaderTimeChart = collectorapi.Chart{
		ID:       "raft_follower_last_contact_leader_time",
		Title:    "Raft follower last contact with the leader time",
		Units:    "ms",
		Fam:      "leadership changes",
		Ctx:      "consul.raft_follower_last_contact_leader_time",
		Priority: prioRaftFollowerLastContactLeaderTime,
		Dims: collectorapi.Dims{
			{ID: "autopilot_server_lastContact_leader", Name: "leader_last_contact"},
		},
	}
	raftLeaderElectionsRateChart = collectorapi.Chart{
		ID:       "raft_leader_elections_rate",
		Title:    "Raft leader elections rate",
		Units:    "elections/s",
		Fam:      "leadership changes",
		Ctx:      "consul.raft_leader_elections_rate",
		Priority: prioRaftLeaderElections,
		Dims: collectorapi.Dims{
			{ID: "raft_state_candidate", Name: "leader", Algo: collectorapi.Incremental},
		},
	}
	raftLeadershipTransitionsRateChart = collectorapi.Chart{
		ID:       "raft_leadership_transitions_rate",
		Title:    "Raft leadership transitions rate",
		Units:    "transitions/s",
		Fam:      "leadership changes",
		Ctx:      "consul.raft_leadership_transitions_rate",
		Priority: prioRaftLeadershipTransitions,
		Dims: collectorapi.Dims{
			{ID: "raft_state_leader", Name: "leadership", Algo: collectorapi.Incremental},
		},
	}
	serverLeadershipStatusChart = collectorapi.Chart{
		ID:       "server_leadership_status",
		Title:    "Server leadership status",
		Units:    "status",
		Fam:      "leadership changes",
		Ctx:      "consul.server_leadership_status",
		Priority: prioServerLeadershipStatus,
		Dims: collectorapi.Dims{
			{ID: "server_isLeader_yes", Name: "leader"},
			{ID: "server_isLeader_no", Name: "not_leader"},
		},
	}

	networkLanRTTChart = collectorapi.Chart{
		ID:       "network_lan_rtt",
		Title:    "Network lan RTT",
		Units:    "ms",
		Fam:      "network rtt",
		Ctx:      "consul.network_lan_rtt",
		Type:     collectorapi.Area,
		Priority: prioNetworkLanRTT,
		Dims: collectorapi.Dims{
			{ID: "network_lan_rtt_min", Name: "min", Div: 1e6},
			{ID: "network_lan_rtt_max", Name: "max", Div: 1e6},
			{ID: "network_lan_rtt_avg", Name: "avg", Div: 1e6},
		},
	}

	clientRPCRequestsRateChart = collectorapi.Chart{
		ID:       "client_rpc_requests_rate",
		Title:    "Client RPC requests",
		Units:    "requests/s",
		Fam:      "rpc network activity",
		Ctx:      "consul.client_rpc_requests_rate",
		Priority: prioRPCRequests,
		Dims: collectorapi.Dims{
			{ID: "client_rpc", Name: "rpc", Algo: collectorapi.Incremental},
		},
	}
	clientRPCRequestsExceededRateChart = collectorapi.Chart{
		ID:       "client_rpc_requests_exceeded_rate",
		Title:    "Client rate-limited RPC requests",
		Units:    "requests/s",
		Fam:      "rpc network activity",
		Ctx:      "consul.client_rpc_requests_exceeded_rate",
		Priority: prioRPCRequestsExceeded,
		Dims: collectorapi.Dims{
			{ID: "client_rpc_exceeded", Name: "exceeded", Algo: collectorapi.Incremental},
		},
	}
	clientRPCRequestsFailedRateChart = collectorapi.Chart{
		ID:       "client_rpc_requests_failed_rate",
		Title:    "Client failed RPC requests",
		Units:    "requests/s",
		Fam:      "rpc network activity",
		Ctx:      "consul.client_rpc_requests_failed_rate",
		Priority: prioRPCRequestsFailed,
		Dims: collectorapi.Dims{
			{ID: "client_rpc_failed", Name: "failed", Algo: collectorapi.Incremental},
		},
	}

	raftThreadMainSaturationPercChart = collectorapi.Chart{
		ID:       "raft_thread_main_saturation_perc",
		Title:    "Raft main thread saturation",
		Units:    "percentage",
		Fam:      "raft saturation",
		Ctx:      "consul.raft_thread_main_saturation_perc",
		Priority: prioRaftThreadMainSaturation,
		Dims: collectorapi.Dims{
			{ID: "raft_thread_main_saturation_quantile=0.5", Name: "quantile_0.5", Div: precision * 10},
			{ID: "raft_thread_main_saturation_quantile=0.9", Name: "quantile_0.9", Div: precision * 10},
			{ID: "raft_thread_main_saturation_quantile=0.99", Name: "quantile_0.99", Div: precision * 10},
		},
	}
	raftThreadFSMSaturationPercChart = collectorapi.Chart{
		ID:       "raft_thread_fsm_saturation_perc",
		Title:    "Raft FSM thread saturation",
		Units:    "percentage",
		Fam:      "raft saturation",
		Ctx:      "consul.raft_thread_fsm_saturation_perc",
		Priority: prioRaftThreadFSMSaturation,
		Dims: collectorapi.Dims{
			{ID: "raft_thread_fsm_saturation_quantile=0.5", Name: "quantile_0.5", Div: precision * 10},
			{ID: "raft_thread_fsm_saturation_quantile=0.9", Name: "quantile_0.9", Div: precision * 10},
			{ID: "raft_thread_fsm_saturation_quantile=0.99", Name: "quantile_0.99", Div: precision * 10},
		},
	}

	raftFSMLastRestoreDurationChart = collectorapi.Chart{
		ID:       "raft_fsm_last_restore_duration",
		Title:    "Raft last restore duration",
		Units:    "ms",
		Fam:      "raft replication capacity",
		Ctx:      "consul.raft_fsm_last_restore_duration",
		Priority: prioRaftFSMLastRestoreDuration,
		Dims: collectorapi.Dims{
			{ID: "raft_fsm_lastRestoreDuration", Name: "last_restore_duration"},
		},
	}
	raftLeaderOldestLogAgeChart = collectorapi.Chart{
		ID:       "raft_leader_oldest_log_age",
		Title:    "Raft leader oldest log age",
		Units:    "seconds",
		Fam:      "raft replication capacity",
		Ctx:      "consul.raft_leader_oldest_log_age",
		Priority: prioRaftLeaderOldestLogAge,
		Dims: collectorapi.Dims{
			{ID: "raft_leader_oldestLogAge", Name: "oldest_log_age", Div: 1000},
		},
	}
	raftRPCInstallSnapshotTimeChart = collectorapi.Chart{
		ID:       "raft_rpc_install_snapshot_time",
		Title:    "Raft RPC install snapshot time",
		Units:    "ms",
		Fam:      "raft replication capacity",
		Ctx:      "consul.raft_rpc_install_snapshot_time",
		Priority: prioRaftRPCInstallSnapshotTime,
		Dims: collectorapi.Dims{
			{ID: "raft_rpc_installSnapshot_quantile=0.5", Name: "quantile_0.5", Div: precision * precision},
			{ID: "raft_rpc_installSnapshot_quantile=0.9", Name: "quantile_0.9", Div: precision * precision},
			{ID: "raft_rpc_installSnapshot_quantile=0.99", Name: "quantile_0.99", Div: precision * precision},
		},
	}

	raftBoltDBFreelistBytesChart = collectorapi.Chart{
		ID:       "raft_boltdb_freelist_bytes",
		Title:    "Raft BoltDB freelist",
		Units:    "bytes",
		Fam:      "boltdb performance",
		Ctx:      "consul.raft_boltdb_freelist_bytes",
		Priority: prioBoltDBFreelistBytes,
		Dims: collectorapi.Dims{
			{ID: "raft_boltdb_freelistBytes", Name: "freelist"},
		},
	}
	raftBoltDBLogsPerBatchChart = collectorapi.Chart{
		ID:       "raft_boltdb_logs_per_batch_rate",
		Title:    "Raft BoltDB logs written per batch",
		Units:    "logs/s",
		Fam:      "boltdb performance",
		Ctx:      "consul.raft_boltdb_logs_per_batch_rate",
		Priority: prioBoltDBLogsPerBatch,
		Dims: collectorapi.Dims{
			{ID: "raft_boltdb_logsPerBatch_sum", Name: "written", Algo: collectorapi.Incremental},
		},
	}

	raftBoltDBStoreLogsTimeChart = collectorapi.Chart{
		ID:       "raft_boltdb_store_logs_time",
		Title:    "Raft BoltDB store logs time",
		Units:    "ms",
		Fam:      "boltdb performance",
		Ctx:      "consul.raft_boltdb_store_logs_time",
		Priority: prioBoltDBStoreLogsTime,
		Dims: collectorapi.Dims{
			{ID: "raft_boltdb_storeLogs_quantile=0.5", Name: "quantile_0.5", Div: precision * precision},
			{ID: "raft_boltdb_storeLogs_quantile=0.9", Name: "quantile_0.9", Div: precision * precision},
			{ID: "raft_boltdb_storeLogs_quantile=0.99", Name: "quantile_0.99", Div: precision * precision},
		},
	}

	memoryAllocatedChart = collectorapi.Chart{
		ID:       "memory_allocated",
		Title:    "Memory allocated by the Consul process",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "consul.memory_allocated",
		Priority: prioMemoryAllocated,
		Dims: collectorapi.Dims{
			{ID: "runtime_alloc_bytes", Name: "allocated"},
		},
	}
	memorySysChart = collectorapi.Chart{
		ID:       "memory_sys",
		Title:    "Memory obtained from the OS",
		Units:    "bytes",
		Fam:      "memory",
		Ctx:      "consul.memory_sys",
		Priority: prioMemorySys,
		Dims: collectorapi.Dims{
			{ID: "runtime_sys_bytes", Name: "sys"},
		},
	}

	gcPauseTimeChart = collectorapi.Chart{
		ID:       "gc_pause_time",
		Title:    "Garbage collection stop-the-world pause time",
		Units:    "seconds",
		Fam:      "garbage collection",
		Ctx:      "consul.gc_pause_time",
		Priority: prioGCPauseTime,
		Dims: collectorapi.Dims{
			{ID: "runtime_total_gc_pause_ns", Name: "gc_pause", Algo: collectorapi.Incremental, Div: 1e9},
		},
	}

	licenseExpirationTimeChart = collectorapi.Chart{
		ID:       "license_expiration_time",
		Title:    "License expiration time",
		Units:    "seconds",
		Fam:      "license",
		Ctx:      "consul.license_expiration_time",
		Priority: prioLicenseExpirationTime,
		Dims: collectorapi.Dims{
			{ID: "system_licenseExpiration", Name: "license_expiration"},
		},
	}
)

var (
	serviceHealthCheckStatusChartTmpl = collectorapi.Chart{
		ID:       "health_check_%s_status",
		Title:    "Service health check status",
		Units:    "status",
		Fam:      "service health checks",
		Ctx:      "consul.service_health_check_status",
		Priority: prioServiceHealthCheckStatus,
		Dims: collectorapi.Dims{
			{ID: "health_check_%s_passing_status", Name: "passing"},
			{ID: "health_check_%s_critical_status", Name: "critical"},
			{ID: "health_check_%s_maintenance_status", Name: "maintenance"},
			{ID: "health_check_%s_warning_status", Name: "warning"},
		},
	}
	nodeHealthCheckStatusChartTmpl = collectorapi.Chart{
		ID:       "health_check_%s_status",
		Title:    "Node health check status",
		Units:    "status",
		Fam:      "node health checks",
		Ctx:      "consul.node_health_check_status",
		Priority: prioNodeHealthCheckStatus,
		Dims: collectorapi.Dims{
			{ID: "health_check_%s_passing_status", Name: "passing"},
			{ID: "health_check_%s_critical_status", Name: "critical"},
			{ID: "health_check_%s_maintenance_status", Name: "maintenance"},
			{ID: "health_check_%s_warning_status", Name: "warning"},
		},
	}
)

func (c *Collector) addGlobalCharts() {
	if !c.isTelemetryPrometheusEnabled() {
		return
	}

	var charts *collectorapi.Charts

	if !c.isServer() {
		charts = clientCharts.Copy()
	} else {
		charts = serverCommonCharts.Copy()

		// can't really rely on checking if a response contains a metric due to retention of some metrics
		// https://github.com/hashicorp/go-metrics/blob/b6d5c860c07ef6eeec89f4a662c7b452dd4d0c93/prometheus/prometheus.go#L75-L76
		if c.version != nil {
			if c.version.LT(semver.Version{Major: 1, Minor: 13, Patch: 0}) {
				_ = charts.Remove(raftThreadMainSaturationPercChart.ID)
				_ = charts.Remove(raftThreadFSMSaturationPercChart.ID)
			}
			if c.version.LT(semver.Version{Major: 1, Minor: 11, Patch: 0}) {
				_ = charts.Remove(kvsApplyTimeChart.ID)
				_ = charts.Remove(kvsApplyOperationsRateChart.ID)
				_ = charts.Remove(txnApplyTimeChart.ID)
				_ = charts.Remove(txnApplyOperationsRateChart.ID)
				_ = charts.Remove(raftBoltDBFreelistBytesChart.ID)
			}
		}
	}

	if !c.hasLicense() {
		_ = charts.Remove(licenseExpirationTimeChart.ID)
	}

	for _, chart := range *charts {
		chart.Labels = []collectorapi.Label{
			{Key: "datacenter", Value: c.cfg.Config.Datacenter},
			{Key: "node_name", Value: c.cfg.Config.NodeName},
		}
	}

	if err := c.Charts().Add(*charts.Copy()...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addServerAutopilotHealthCharts() {
	charts := serverAutopilotHealthCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []collectorapi.Label{
			{Key: "datacenter", Value: c.cfg.Config.Datacenter},
			{Key: "node_name", Value: c.cfg.Config.NodeName},
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func newServiceHealthCheckChart(check *agentCheck) *collectorapi.Chart {
	chart := serviceHealthCheckStatusChartTmpl.Copy()
	chart.ID = fmt.Sprintf(chart.ID, check.CheckID)
	chart.Labels = []collectorapi.Label{
		{Key: "node_name", Value: check.Node},
		{Key: "check_name", Value: check.Name},
		{Key: "service_name", Value: check.ServiceName},
	}
	for _, d := range chart.Dims {
		d.ID = fmt.Sprintf(d.ID, check.CheckID)
	}
	return chart
}

func newNodeHealthCheckChart(check *agentCheck) *collectorapi.Chart {
	chart := nodeHealthCheckStatusChartTmpl.Copy()
	chart.ID = fmt.Sprintf(chart.ID, check.CheckID)
	chart.Labels = []collectorapi.Label{
		{Key: "node_name", Value: check.Node},
		{Key: "check_name", Value: check.Name},
	}
	for _, d := range chart.Dims {
		d.ID = fmt.Sprintf(d.ID, check.CheckID)
	}
	return chart
}

func (c *Collector) addHealthCheckCharts(check *agentCheck) {
	var chart *collectorapi.Chart

	if check.ServiceName != "" {
		chart = newServiceHealthCheckChart(check)
	} else {
		chart = newNodeHealthCheckChart(check)
	}

	chart.Labels = append(chart.Labels, collectorapi.Label{
		Key:   "datacenter",
		Value: c.cfg.Config.Datacenter,
	})

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHealthCheckCharts(checkID string) {
	id := fmt.Sprintf("health_check_%s_status", checkID)

	chart := c.Charts().Get(id)
	if chart == nil {
		c.Warningf("failed to remove '%s' chart: the chart does not exist", id)
		return
	}

	chart.MarkRemove()
	chart.MarkNotCreated()
}

func (c *Collector) addLeaderCharts() {
	charts := serverLeaderCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []collectorapi.Label{
			{Key: "datacenter", Value: c.cfg.Config.Datacenter},
			{Key: "node_name", Value: c.cfg.Config.NodeName},
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeLeaderCharts() {
	s := make(map[string]bool)
	for _, v := range serverLeaderCharts {
		s[v.ID] = true
	}

	for _, v := range *c.Charts() {
		if s[v.ID] {
			v.MarkRemove()
			v.MarkNotCreated()
		}
	}
}

func (c *Collector) addFollowerCharts() {
	charts := serverFollowerCharts.Copy()
	if c.isCloudManaged() {
		// 'autopilot_server_lastContact_leader' comes from 'operator/autopilot/health' which is disabled
		_ = charts.Remove(raftFollowerLastContactLeaderTimeChart.ID)
	}

	for _, chart := range *charts {
		chart.Labels = []collectorapi.Label{
			{Key: "datacenter", Value: c.cfg.Config.Datacenter},
			{Key: "node_name", Value: c.cfg.Config.NodeName},
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeFollowerCharts() {
	s := make(map[string]bool)
	for _, v := range serverFollowerCharts {
		s[v.ID] = true
	}

	for _, v := range *c.Charts() {
		if s[v.ID] {
			v.MarkRemove()
			v.MarkNotCreated()
		}
	}
}
