// SPDX-License-Identifier: GPL-3.0-or-later

package logstash

// https://www.elastic.co/guide/en/logstash/current/node-stats-api.html

type nodeStats struct {
	JVM       jvmStats                 `json:"jvm" stm:"jvm"`
	Process   processStats             `json:"process" stm:"process"`
	Event     eventsStats              `json:"event" stm:"event"`
	Pipelines map[string]pipelineStats `json:"pipelines" stm:"pipelines"`
}

type pipelineStats struct {
	Event eventsStats `json:"events" stm:"event"`
}

type eventsStats struct {
	In                        int `json:"in" stm:"in"`
	Filtered                  int `json:"filtered" stm:"filtered"`
	Out                       int `json:"out" stm:"out"`
	DurationInMillis          int `json:"duration_in_millis" stm:"duration_in_millis"`
	QueuePushDurationInMillis int `json:"queue_push_duration_in_millis" stm:"queue_push_duration_in_millis"`
}

type processStats struct {
	OpenFileDescriptors int `json:"open_file_descriptors" stm:"open_file_descriptors"`
}

type jvmStats struct {
	Threads struct {
		Count int `stm:"count"`
	} `stm:"threads"`
	Mem            jvmMemStats `stm:"mem"`
	GC             jvmGCStats  `stm:"gc"`
	UptimeInMillis int         `json:"uptime_in_millis" stm:"uptime_in_millis"`
}

type jvmMemStats struct {
	HeapUsedPercent      int `json:"heap_used_percent" stm:"heap_used_percent"`
	HeapCommittedInBytes int `json:"heap_committed_in_bytes" stm:"heap_committed_in_bytes"`
	HeapUsedInBytes      int `json:"heap_used_in_bytes" stm:"heap_used_in_bytes"`
	Pools                struct {
		Survivor jvmPoolStats `stm:"survivor"`
		Old      jvmPoolStats `stm:"old"`
		Young    jvmPoolStats `stm:"eden"`
	} `stm:"pools"`
}

type jvmPoolStats struct {
	UsedInBytes      int `json:"used_in_bytes" stm:"used_in_bytes"`
	CommittedInBytes int `json:"committed_in_bytes" stm:"committed_in_bytes"`
}

type jvmGCStats struct {
	Collectors struct {
		Old   gcCollectorStats `stm:"old"`
		Young gcCollectorStats `stm:"eden"`
	} `stm:"collectors"`
}

type gcCollectorStats struct {
	CollectionTimeInMillis int `json:"collection_time_in_millis" stm:"collection_time_in_millis"`
	CollectionCount        int `json:"collection_count" stm:"collection_count"`
}
