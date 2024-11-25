// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

// https://dnsdist.org/guides/webserver.html#get--jsonstat
// https://dnsdist.org/statistics.html

type statisticMetrics struct {
	AclDrops              float64 `stm:"acl-drops" json:"acl-drops"`
	CacheHits             float64 `stm:"cache-hits" json:"cache-hits"`
	CacheMisses           float64 `stm:"cache-misses" json:"cache-misses"`
	CPUSysMsec            float64 `stm:"cpu-sys-msec" json:"cpu-sys-msec"`
	CPUUserMsec           float64 `stm:"cpu-user-msec" json:"cpu-user-msec"`
	DownStreamSendErrors  float64 `stm:"downstream-send-errors" json:"downstream-send-errors"`
	DownStreamTimeout     float64 `stm:"downstream-timeouts" json:"downstream-timeouts"`
	DynBlocked            float64 `stm:"dyn-blocked" json:"dyn-blocked"`
	EmptyQueries          float64 `stm:"empty-queries" json:"empty-queries"`
	LatencyAvg100         float64 `stm:"latency-avg100" json:"latency-avg100"`
	LatencyAvg1000        float64 `stm:"latency-avg1000" json:"latency-avg1000"`
	LatencyAvg10000       float64 `stm:"latency-avg10000" json:"latency-avg10000"`
	LatencyAvg1000000     float64 `stm:"latency-avg1000000" json:"latency-avg1000000"`
	LatencySlow           float64 `stm:"latency-slow" json:"latency-slow"`
	Latency0              float64 `stm:"latency0-1" json:"latency0-1"`
	Latency1              float64 `stm:"latency1-10" json:"latency1-10"`
	Latency10             float64 `stm:"latency10-50" json:"latency10-50"`
	Latency100            float64 `stm:"latency100-1000" json:"latency100-1000"`
	Latency50             float64 `stm:"latency50-100" json:"latency50-100"`
	NoPolicy              float64 `stm:"no-policy" json:"no-policy"`
	NonCompliantQueries   float64 `stm:"noncompliant-queries" json:"noncompliant-queries"`
	NonCompliantResponses float64 `stm:"noncompliant-responses" json:"noncompliant-responses"`
	Queries               float64 `stm:"queries" json:"queries"`
	RdQueries             float64 `stm:"rdqueries" json:"rdqueries"`
	RealMemoryUsage       float64 `stm:"real-memory-usage" json:"real-memory-usage"`
	Responses             float64 `stm:"responses" json:"responses"`
	RuleDrop              float64 `stm:"rule-drop" json:"rule-drop"`
	RuleNxDomain          float64 `stm:"rule-nxdomain" json:"rule-nxdomain"`
	RuleRefused           float64 `stm:"rule-refused" json:"rule-refused"`
	SelfAnswered          float64 `stm:"self-answered" json:"self-answered"`
	ServFailResponses     float64 `stm:"servfail-responses" json:"servfail-responses"`
	TruncFailures         float64 `stm:"trunc-failures" json:"trunc-failures"`
}
