// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAggregatorNormalizationAndLimits(t *testing.T) {
	agg := newFlowAggregator(flowAggregatorConfig{
		bucketDuration: 10 * time.Second,
		maxBuckets:     2,
		maxKeys:        1,
		defaultRate:    1,
	})
	fixedNow := time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)
	agg.now = func() time.Time { return fixedNow }

	record := flowRecord{
		Timestamp:    fixedNow,
		Key:          flowKey{SrcPrefix: "10.0.0.1/32", DstPrefix: "10.0.0.2/32", SrcPort: 1000, DstPort: 80, Protocol: 6},
		Bytes:        100,
		Packets:      10,
		Flows:        1,
		SamplingRate: 10,
		ExporterIP:   "192.0.2.1",
		FlowVersion:  "v5",
	}
	agg.AddRecords([]flowRecord{record})

	second := flowRecord{
		Timestamp:   fixedNow,
		Key:         flowKey{SrcPrefix: "10.0.0.3/32", DstPrefix: "10.0.0.4/32", SrcPort: 2000, DstPort: 443, Protocol: 6},
		Bytes:       200,
		Packets:     20,
		Flows:       1,
		ExporterIP:  "192.0.2.1",
		FlowVersion: "v5",
	}
	agg.AddRecords([]flowRecord{second})

	snapshot := agg.Snapshot("agent-1")
	require.NotEmpty(t, snapshot.Buckets)
	bucket := snapshot.Buckets[0]
	require.Equal(t, uint64(1000), bucket.Bytes)
	require.Equal(t, uint64(100), bucket.Packets)
	require.Equal(t, uint64(10), bucket.Flows)
	require.Equal(t, uint64(100), bucket.RawBytes)
	require.Equal(t, uint64(10), bucket.RawPackets)
	require.Equal(t, 10, bucket.SamplingRate)
	require.Equal(t, "192.0.2.1", bucket.ExporterIP)

	require.Equal(t, uint64(1), snapshot.Summaries["dropped_records"])
}
