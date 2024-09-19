package ceph

type (
	OsdPerf struct {
		PgReady  bool     `json:"pg_ready"`
		OsdStats OsdStats `json:"osdstats"`
	}

	OsdStats struct {
		OsdPerfInfos []OsdPerfInfo `json:"osd_perf_infos"`
	}

	OsdPerfInfo struct {
		ID        int64     `json:"id"`
		PerfStats PerfStats `json:"perf_stats"`
	}

	PerfStats struct {
		CommitLatencyMs int64 `json:"commit_latency_ms"`
		ApplyLatencyMs  int64 `json:"apply_latency_ms"`
		CommitLatencyNs int64 `json:"commit_latency_ns"`
		ApplyLatencyNs  int64 `json:"apply_latency_ns"`
	}
)
