// SPDX-License-Identifier: GPL-3.0-or-later

package pcf

import (
	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)


// ResetQueueStats resets queue statistics and returns the peak values.
// WARNING: This is destructive - it resets counters to zero!
func (c *Client) ResetQueueStats(queueName string) (map[string]int64, error) {
	params := []pcfParameter{
		newStringParameter(ibmmq.MQCA_Q_NAME, queueName),
	}
	
	response, err := c.sendPCFCommand(ibmmq.MQCMD_RESET_Q_STATS, params)
	if err != nil {
		return nil, err
	}

	attrs, err := c.parsePCFResponseFromParams(response, "")
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)
	
	// Extract peak/high water mark values
	if highDepth, ok := attrs[ibmmq.MQIA_HIGH_Q_DEPTH]; ok {
		mx["high_depth"] = int64(highDepth.(int32))
	}
	
	// Extract reset message counts
	if msgEnqCount, ok := attrs[ibmmq.MQIA_MSG_ENQ_COUNT]; ok {
		mx["msg_enq_count"] = int64(msgEnqCount.(int32))
	}
	if msgDeqCount, ok := attrs[ibmmq.MQIA_MSG_DEQ_COUNT]; ok {
		mx["msg_deq_count"] = int64(msgDeqCount.(int32))
	}
	
	// Time since reset
	if timeSinceReset, ok := attrs[ibmmq.MQIA_TIME_SINCE_RESET]; ok {
		mx["time_since_reset"] = int64(timeSinceReset.(int32))
	}

	return mx, nil
}