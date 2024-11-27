// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"errors"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const querySystemEvents = `
SELECT
    event,
    value 
FROM
    system.events FORMAT CSVWithNames
`

func (c *Collector) collectSystemEvents(mx map[string]int64) error {
	req, _ := web.NewHTTPRequest(c.RequestConfig)
	req.URL.RawQuery = makeURLQuery(querySystemEvents)

	px := "events_"
	var event string
	var n int

	err := c.doHTTP(req, func(column, value string, lineEnd bool) {
		switch column {
		case "event":
			event = value
		case "value":
			if !wantSystemEvents[event] {
				return
			}
			n++
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				mx[px+event] = v
			}
		}
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("no system events data returned")
	}

	// CH doesn't expose events with 0 values
	for k := range wantSystemEvents {
		k = px + k
		if _, ok := mx[k]; !ok {
			mx[k] = 0
		}
	}

	mx["events_SuccessfulQuery"] = mx["events_Query"] - mx["events_FailedQuery"]
	mx["events_SuccessfulSelectQuery"] = mx["events_SelectQuery"] - mx["events_FailedSelectQuery"]
	mx["events_SuccessfulInsertQuery"] = mx["events_InsertQuery"] - mx["events_FailedInsertQuery"]

	return nil
}

var wantSystemEvents = map[string]bool{
	"SlowRead":                                 true,
	"ReadBackoff":                              true,
	"Query":                                    true,
	"FailedQuery":                              true,
	"QueryTimeMicroseconds":                    true,
	"SelectQuery":                              true,
	"FailedSelectQuery":                        true,
	"SelectQueryTimeMicroseconds":              true,
	"InsertQuery":                              true,
	"FailedInsertQuery":                        true,
	"InsertQueryTimeMicroseconds":              true,
	"QueryPreempted":                           true,
	"QueryMemoryLimitExceeded":                 true,
	"InsertedRows":                             true,
	"InsertedBytes":                            true,
	"DelayedInserts":                           true,
	"DelayedInsertsMilliseconds":               true,
	"RejectedInserts":                          true,
	"SelectedRows":                             true,
	"SelectedBytes":                            true,
	"SelectedParts":                            true,
	"SelectedRanges":                           true,
	"SelectedMarks":                            true,
	"Merge":                                    true,
	"MergedRows":                               true,
	"MergedUncompressedBytes":                  true,
	"MergesTimeMilliseconds":                   true,
	"MergeTreeDataWriterRows":                  true,
	"MergeTreeDataWriterUncompressedBytes":     true,
	"MergeTreeDataWriterCompressedBytes":       true,
	"UncompressedCacheHits":                    true,
	"UncompressedCacheMisses":                  true,
	"MarkCacheHits":                            true,
	"MarkCacheMisses":                          true,
	"Seek":                                     true,
	"FileOpen":                                 true,
	"ReadBufferFromFileDescriptorReadBytes":    true,
	"WriteBufferFromFileDescriptorWriteBytes":  true,
	"ReadBufferFromFileDescriptorRead":         true,
	"WriteBufferFromFileDescriptorWrite":       true,
	"ReadBufferFromFileDescriptorReadFailed":   true,
	"WriteBufferFromFileDescriptorWriteFailed": true,
	"DistributedConnectionTries":               true,
	"DistributedConnectionFailTry":             true,
	"DistributedConnectionFailAtAll":           true,
	"DistributedRejectedInserts":               true,
	"DistributedDelayedInserts":                true,
	"DistributedDelayedInsertsMilliseconds":    true,
	"DistributedSyncInsertionTimeoutExceeded":  true,
	"DistributedAsyncInsertionFailures":        true,
	"ReplicatedDataLoss":                       true,
	"ReplicatedPartFetches":                    true,
	"ReplicatedPartFailedFetches":              true,
	"ReplicatedPartMerges":                     true,
	"ReplicatedPartFetchesOfMerged":            true,
}
