// SPDX-License-Identifier: GPL-3.0-or-later

package redfish_logs

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type backendMetrics struct {
	state               metrix.SnapshotStateSetVec
	storageUsed         metrix.SnapshotGaugeVec
	storageTarget       metrix.SnapshotGaugeVec
	filesActive         metrix.SnapshotGaugeVec
	filesArchived       metrix.SnapshotGaugeVec
	producersActive     metrix.SnapshotGaugeVec
	pipelineReceived    metrix.SnapshotCounterVec
	pipelineCommitted   metrix.SnapshotCounterVec
	pipelineDuplicates  metrix.SnapshotCounterVec
	pipelineWriteFailed metrix.SnapshotCounterVec
	retentionFiles      metrix.SnapshotCounterVec
	retentionBytes      metrix.SnapshotCounterVec
	syncDuration        metrix.SnapshotGaugeVec
}

func newBackendMetrics(store metrix.CollectorStore) *backendMetrics {
	vec := store.Write().SnapshotMeter("").Vec("backend_key", "backend_name")
	return &backendMetrics{
		state: vec.StateSet(
			"log_backend_state",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates("ready", "unavailable"),
		),
		storageUsed:         vec.Gauge("log_backend_storage_used_bytes"),
		storageTarget:       vec.Gauge("log_backend_storage_target_bytes"),
		filesActive:         vec.Gauge("log_backend_files_active"),
		filesArchived:       vec.Gauge("log_backend_files_archived"),
		producersActive:     vec.Gauge("log_backend_producers_active"),
		pipelineReceived:    vec.Counter("log_backend_pipeline_received_total"),
		pipelineCommitted:   vec.Counter("log_backend_pipeline_committed_total"),
		pipelineDuplicates:  vec.Counter("log_backend_pipeline_duplicate_suppressed_total"),
		pipelineWriteFailed: vec.Counter("log_backend_pipeline_write_failed_total"),
		retentionFiles:      vec.Counter("log_backend_retention_files_deleted_total"),
		retentionBytes:      vec.Counter("log_backend_retention_bytes_deleted_total"),
		syncDuration:        vec.Gauge("log_backend_sync_duration_seconds"),
	}
}
