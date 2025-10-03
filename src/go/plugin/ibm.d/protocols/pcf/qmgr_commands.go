// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo && ibm_mq
// +build cgo,ibm_mq

package pcf

import (
	"fmt"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)

// GetQueueManagerStatus returns the status of the queue manager.
func (c *Client) GetQueueManagerStatus() (*QueueManagerMetrics, error) {
	c.protocol.Debugf("QMGR getting status for queue manager '%s'", c.config.QueueManager)

	response, err := c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_Q_MGR_STATUS, nil)
	if err != nil {
		// Fallback to basic inquiry
		c.protocol.Debugf("QMGR status inquiry failed for queue manager '%s', trying basic inquiry: %v", c.config.QueueManager, err)
		response, err = c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_Q_MGR, nil)
		if err != nil {
			c.protocol.Errorf("QMGR failed to get status for queue manager '%s': %v", c.config.QueueManager, err)
			return nil, err
		}
	}

	attrs := make(map[int32]interface{})
	for _, param := range response {
		switch param.Type {
		case ibmmq.MQCFT_STRING:
			if len(param.String) > 0 {
				attrs[param.Parameter] = param.String[0]
			}
		case ibmmq.MQCFT_INTEGER:
			if len(param.Int64Value) > 0 {
				attrs[param.Parameter] = int32(param.Int64Value[0])
			}
		}
	}

	// If we get a successful response, the queue manager is running
	metrics := &QueueManagerMetrics{
		Status:          1,
		ConnectionCount: NotCollected, // Default to not collected
		StartDate:       NotCollected, // Default to not collected
		StartTime:       NotCollected, // Default to not collected
		Uptime:          NotCollected, // Default to not collected
	}

	// Extract connection count if available (only from MQCMD_INQUIRE_Q_MGR_STATUS)
	if connectionCount, ok := attrs[ibmmq.MQIACF_CONNECTION_COUNT]; ok {
		if count, ok := connectionCount.(int32); ok {
			metrics.ConnectionCount = AttributeValue(count)
			c.protocol.Debugf("QMGR '%s' connection count: %d", c.config.QueueManager, count)
		}
	} else {
		c.protocol.Debugf("QMGR '%s' - MQIACF_CONNECTION_COUNT (value %d) not found in response", c.config.QueueManager, ibmmq.MQIACF_CONNECTION_COUNT)
	}

	// Extract queue manager start date and time for uptime calculation
	var startDate, startTime string

	// Try to get start date (constant 3175 = MQCACF_Q_MGR_START_DATE)
	if date, ok := attrs[int32(3175)]; ok {
		c.protocol.Debugf("Found attribute 3175, type: %T, value: %v", date, date)
		if dateStr, ok := date.(string); ok {
			startDate = dateStr
			metrics.StartDate = AttributeValue(len(dateStr)) // Store length as indicator
			c.protocol.Debugf("QMGR '%s' start date: %s", c.config.QueueManager, dateStr)
		} else {
			c.protocol.Warningf("QMGR: Attribute 3175 is not a string, type: %T", date)
		}
	} else {
		c.protocol.Warningf("UPTIME DEBUG: QMGR '%s' - start date (attribute 3175) not found in response with %d attributes", c.config.QueueManager, len(attrs))
	}

	// Try to get start time (constant should be around 3176, but let's check for common values)
	// Check for possible start time constants
	timeConstants := []int32{3176, 3177, 3178} // Possible values near start date
	for _, timeConstant := range timeConstants {
		if time, ok := attrs[timeConstant]; ok {
			if timeStr, ok := time.(string); ok {
				startTime = timeStr
				metrics.StartTime = AttributeValue(len(timeStr)) // Store length as indicator
				c.protocol.Debugf("QMGR '%s' start time: %s (attribute %d)", c.config.QueueManager, timeStr, int32(timeConstant))
				break
			}
		}
	}
	if startTime == "" {
		c.protocol.Debugf("QMGR '%s' - start time not found in response", c.config.QueueManager)
	}

	// Calculate uptime if date is available (assume start time 00:00:00 if not provided)
	if startDate != "" {
		if startTime == "" {
			// If no start time provided, assume start of day (00:00:00)
			startTime = "00:00:00"
			c.protocol.Debugf("QMGR '%s' - no start time found, assuming 00:00:00", c.config.QueueManager)
		}
		if uptime, err := CalculateUptimeSeconds(startDate, startTime); err == nil {
			metrics.Uptime = AttributeValue(uptime)
			c.protocol.Debugf("QMGR '%s' uptime: %d seconds", c.config.QueueManager, uptime)
		} else {
			c.protocol.Warningf("QMGR failed to calculate uptime for queue manager '%s': %v", c.config.QueueManager, err)
		}
	} else {
		c.protocol.Warningf("QMGR '%s' - no start date available for uptime calculation", c.config.QueueManager)
	}

	return metrics, nil
}

// refreshStaticDataFromPCF refreshes cached static data using PCF commands (called from connection.go)
func (c *Client) refreshStaticDataFromPCF() error {
	c.protocol.Debugf("Refreshing static data for queue manager '%s' after connection", c.config.QueueManager)

	// Get queue manager version and platform info
	response, err := c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_Q_MGR, nil)
	if err != nil {
		return fmt.Errorf("failed to get queue manager info for '%s': %w", c.config.QueueManager, err)
	}

	attrs := make(map[int32]interface{})
	for _, param := range response {
		switch param.Type {
		case ibmmq.MQCFT_STRING:
			if len(param.String) > 0 {
				attrs[param.Parameter] = param.String[0]
			}
		case ibmmq.MQCFT_INTEGER:
			if len(param.Int64Value) > 0 {
				attrs[param.Parameter] = int32(param.Int64Value[0])
			}
		}
	}

	// Extract and cache version info
	if cmdLevel, ok := attrs[ibmmq.MQIA_COMMAND_LEVEL]; ok {
		if level, ok := cmdLevel.(int32); ok {
			c.cachedCommandLevel = level
			// Convert command level to version string
			major := level / 100
			minor := (level % 100) / 10
			c.cachedVersion = fmt.Sprintf("%d.%d", major, minor)
			c.protocol.Debugf("QMGR '%s' version detected: %s (command level: %d)",
				c.config.QueueManager, c.cachedVersion, level)
		}
	}

	// Extract platform info
	if platform, ok := attrs[ibmmq.MQIA_PLATFORM]; ok {
		if plat, ok := platform.(int32); ok {
			c.cachedPlatform = plat
			// Use IBM library to get the correct platform name
			platformName := ibmmq.MQItoString("PL", int(plat))
			c.cachedEdition = platformName
			c.protocol.Debugf("QMGR '%s' platform detected: %s (platform code: %d)",
				c.config.QueueManager, platformName, plat)
		}
	}

	// Extract STATINT (Statistics Interval) using official IBM MQ constant
	// MQIA_STATISTICS_INTERVAL = 131 (X'00000083') - Controls statistics publishing interval
	if statInt, ok := attrs[ibmmq.MQIA_STATISTICS_INTERVAL]; ok {
		if interval, ok := statInt.(int32); ok && interval > 0 {
			c.cachedStatisticsInterval = interval
			c.protocol.Debugf("QMGR '%s' STATINT detected: %d seconds (MQIA_STATISTICS_INTERVAL=%d)",
				c.config.QueueManager, interval, ibmmq.MQIA_STATISTICS_INTERVAL)
		}
	} else {
		c.protocol.Debugf("QMGR '%s' STATINT not available (MQIA_STATISTICS_INTERVAL not in response)", c.config.QueueManager)
	}

	// Extract queue monitoring interval using MQIA_MONITORING_Q = 123
	if queueMonInt, ok := attrs[int32(123)]; ok {
		if interval, ok := queueMonInt.(int32); ok && interval >= 0 {
			c.cachedMonitoringQueue = interval
			c.protocol.Debugf("QMGR '%s' queue monitoring interval detected: %d seconds (MQIA_MONITORING_Q=123)",
				c.config.QueueManager, interval)
		}
	} else {
		c.protocol.Debugf("QMGR '%s' queue monitoring interval not available (MQIA_MONITORING_Q not in response)", c.config.QueueManager)
	}

	// Extract channel monitoring interval using MQIA_MONITORING_CHANNEL = 122
	if channelMonInt, ok := attrs[int32(122)]; ok {
		if interval, ok := channelMonInt.(int32); ok && interval >= 0 {
			c.cachedMonitoringChannel = interval
			c.protocol.Debugf("QMGR '%s' channel monitoring interval detected: %d seconds (MQIA_MONITORING_CHANNEL=122)",
				c.config.QueueManager, interval)
		}
	} else {
		c.protocol.Debugf("QMGR '%s' channel monitoring interval not available (MQIA_MONITORING_CHANNEL not in response)", c.config.QueueManager)
	}

	// Extract auto-defined cluster sender channel monitoring interval using MQIA_MONITORING_AUTO_CLUSSDR = 124
	if clussdrMonInt, ok := attrs[int32(124)]; ok {
		if interval, ok := clussdrMonInt.(int32); ok && interval >= 0 {
			c.cachedMonitoringAutoClussdr = interval
			c.protocol.Debugf("QMGR '%s' auto cluster sender channel monitoring interval detected: %d seconds (MQIA_MONITORING_AUTO_CLUSSDR=124)",
				c.config.QueueManager, interval)
		}
	} else {
		c.protocol.Debugf("QMGR '%s' auto cluster sender channel monitoring interval not available (MQIA_MONITORING_AUTO_CLUSSDR not in response)", c.config.QueueManager)
	}

	// For $SYS topic interval, we'll use a default of 10 seconds as per IBM documentation
	// This interval is typically not exposed as a queue manager attribute but is configurable
	// The default publication frequency to $SYS/ topics is approximately every 10 seconds
	c.cachedSysTopicInterval = 10
	c.protocol.Debugf("QMGR '%s' $SYS topic interval set to default: %d seconds (configurable, see IBM MQ documentation)",
		c.config.QueueManager, c.cachedSysTopicInterval)

	c.protocol.Debugf("QMGR static data refreshed for queue manager '%s' - version: %s, platform: %s, STATINT: %d, MONQ: %d, MONCH: %d, MONCLS: %d, SYSTOPIC: %d",
		c.config.QueueManager, c.cachedVersion, c.cachedEdition, c.cachedStatisticsInterval,
		c.cachedMonitoringQueue, c.cachedMonitoringChannel, c.cachedMonitoringAutoClussdr, c.cachedSysTopicInterval)

	return nil
}
