// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"time"
	"strconv"
)

// GetQueueManagerStatus returns the status of the queue manager.
func (c *Client) GetQueueManagerStatus() (*QueueManagerMetrics, error) {
	c.protocol.Debugf("PCF: Getting status for queue manager '%s'", c.config.QueueManager)
	
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q_MGR_STATUS, nil)
	if err != nil {
		// Fallback to basic inquiry
		c.protocol.Debugf("PCF: Status inquiry failed for queue manager '%s', trying basic inquiry: %v", c.config.QueueManager, err)
		response, err = c.SendPCFCommand(C.MQCMD_INQUIRE_Q_MGR, nil)
		if err != nil {
			c.protocol.Errorf("PCF: Failed to get status for queue manager '%s': %v", c.config.QueueManager, err)
			return nil, err
		}
	}

	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		c.protocol.Errorf("PCF: Failed to parse status response for queue manager '%s': %v", c.config.QueueManager, err)
		return nil, err
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
	c.protocol.Debugf("PCF: Available attributes for queue manager '%s': %v", c.config.QueueManager, attrs)
	if connectionCount, ok := attrs[C.MQIACF_CONNECTION_COUNT]; ok {
		if count, ok := connectionCount.(int32); ok {
			metrics.ConnectionCount = AttributeValue(count)
			c.protocol.Debugf("PCF: Queue manager '%s' connection count: %d", c.config.QueueManager, count)
		}
	} else {
		c.protocol.Debugf("PCF: Queue manager '%s' - MQIACF_CONNECTION_COUNT (value %d) not found in response", c.config.QueueManager, C.MQIACF_CONNECTION_COUNT)
	}

	// Extract queue manager start date and time for uptime calculation
	var startDate, startTime string
	
	// Try to get start date (constant 3175 = MQCACF_Q_MGR_START_DATE)
	c.protocol.Infof("PCF: Looking for start date in attributes map with %d entries", len(attrs))
	if date, ok := attrs[C.MQLONG(3175)]; ok {
		c.protocol.Infof("PCF: Found attribute 3175, type: %T, value: %v", date, date)
		if dateStr, ok := date.(string); ok {
			startDate = dateStr
			metrics.StartDate = AttributeValue(len(dateStr)) // Store length as indicator
			c.protocol.Infof("PCF: Queue manager '%s' start date: %s", c.config.QueueManager, dateStr)
		} else {
			c.protocol.Warningf("PCF: Attribute 3175 is not a string, type: %T", date)
		}
	} else {
		c.protocol.Warningf("PCF: UPTIME DEBUG: Queue manager '%s' - start date (attribute 3175) not found in response with %d attributes", c.config.QueueManager, len(attrs))
	}
	
	// Try to get start time (constant should be around 3176, but let's check for common values)
	// Check for possible start time constants
	var timeConstants = []C.MQLONG{3176, 3177, 3178} // Possible values near start date
	for _, timeConstant := range timeConstants {
		if time, ok := attrs[timeConstant]; ok {
			if timeStr, ok := time.(string); ok {
				startTime = timeStr
				metrics.StartTime = AttributeValue(len(timeStr)) // Store length as indicator
				c.protocol.Debugf("PCF: Queue manager '%s' start time: %s (attribute %d)", c.config.QueueManager, timeStr, int32(timeConstant))
				break
			}
		}
	}
	if startTime == "" {
		c.protocol.Debugf("PCF: Queue manager '%s' - start time not found in response", c.config.QueueManager)
	}
	
	// Calculate uptime if date is available (assume start time 00:00:00 if not provided)
	if startDate != "" {
		if startTime == "" {
			// If no start time provided, assume start of day (00:00:00)
			startTime = "00:00:00"
			c.protocol.Infof("PCF: Queue manager '%s' - no start time found, assuming 00:00:00", c.config.QueueManager)
		}
		if uptime, err := calculateQueueManagerUptime(startDate, startTime); err == nil {
			metrics.Uptime = AttributeValue(uptime)
			c.protocol.Infof("PCF: Queue manager '%s' uptime: %d seconds", c.config.QueueManager, uptime)
		} else {
			c.protocol.Warningf("PCF: Failed to calculate uptime for queue manager '%s': %v", c.config.QueueManager, err)
		}
	} else {
		c.protocol.Warningf("PCF: Queue manager '%s' - no start date available for uptime calculation", c.config.QueueManager)
	}

	return metrics, nil
}

// refreshStaticData refreshes cached static data on reconnection
func (c *Client) refreshStaticData() {
	c.protocol.Debugf("PCF: Refreshing static data for queue manager '%s' after connection", c.config.QueueManager)
	
	// Get queue manager version and platform info
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q_MGR, nil)
	if err != nil {
		c.protocol.Warningf("PCF: Failed to get queue manager info for '%s': %v", c.config.QueueManager, err)
		return
	}
	
	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		c.protocol.Warningf("PCF: Failed to parse queue manager info response for '%s': %v", c.config.QueueManager, err)
		return
	}
	
	// Extract and cache version info
	if cmdLevel, ok := attrs[C.MQIA_COMMAND_LEVEL]; ok {
		if level, ok := cmdLevel.(int32); ok {
			c.cachedCommandLevel = level
			// Convert command level to version string
			major := level / 100
			minor := (level % 100) / 10
			c.cachedVersion = fmt.Sprintf("%d.%d", major, minor)
			c.protocol.Debugf("PCF: Queue manager '%s' version detected: %s (command level: %d)", 
				c.config.QueueManager, c.cachedVersion, level)
		}
	}
	
	// Extract platform info
	if platform, ok := attrs[C.MQIA_PLATFORM]; ok {
		if plat, ok := platform.(int32); ok {
			c.cachedPlatform = plat
			c.cachedEdition = getPlatformString(plat)
			c.protocol.Debugf("PCF: Queue manager '%s' platform detected: %s (platform code: %d)", 
				c.config.QueueManager, c.cachedEdition, plat)
		}
	}
	
	c.protocol.Infof("PCF: Static data refreshed for queue manager '%s' - version: %s, platform: %s", 
		c.config.QueueManager, c.cachedVersion, c.cachedEdition)
}

// getPlatformString converts platform ID to human-readable string
func getPlatformString(platform int32) string {
	switch platform {
	case 1:
		return "OS/390"
	case 2:
		return "VM/ESA"
	case 3:
		return "HPUX"
	case 4:
		return "AIX"
	case 5:
		return "Sinix"
	case 6:
		return "Windows"
	case 7:
		return "OS/2"
	case 8:
		return "Ingres"
	case 10:
		return "Compaq Tru64 UNIX"
	case 11:
		return "Compaq OpenVMS"
	case 12:
		return "Tandem NSK"
	case 13:
		return "UNIX"
	case 14:
		return "OS/400"
	case 15:
		return "Max/OS"
	case 18:
		return "TPF"
	case 23:
		return "VSE"
	case 27:
		return "z/TPF"
	case 28:
		return "z/OS"
	case 29:
		return "OS X"
	case 65536:
		return "Distributed"
	default:
		return fmt.Sprintf("Unknown (%d)", platform)
	}
}

// calculateQueueManagerUptime calculates uptime in seconds from IBM MQ start date and time
// Supports both old format (YYYYMMDD, HHMMSSSS) and new format (YYYY-MM-DD, HH:MM:SS)
func calculateQueueManagerUptime(startDate, startTime string) (int64, error) {
	if startDate == "" || startTime == "" {
		return 0, fmt.Errorf("empty start date or time")
	}
	
	var parsedTime time.Time
	var err error
	
	// Try new format first: "YYYY-MM-DD" + "HH:MM:SS"
	if len(startDate) == 10 && startDate[4] == '-' {
		// New format: "2024-01-15" + "14:30:25"
		datetime := startDate + "T" + startTime + "Z"
		parsedTime, err = time.Parse("2006-01-02T15:04:05Z", datetime)
		if err != nil {
			return 0, fmt.Errorf("failed to parse new format date/time %s + %s: %v", startDate, startTime, err)
		}
	} else {
		// Try old format: "YYYYMMDD" + "HHMMSSSS" 
		if len(startDate) != 8 {
			return 0, fmt.Errorf("invalid date format: %s (expected YYYYMMDD or YYYY-MM-DD)", startDate)
		}
		if len(startTime) != 8 {
			return 0, fmt.Errorf("invalid time format: %s (expected HHMMSSSS or HH:MM:SS)", startTime)
		}
		
		// Parse YYYYMMDD
		year, _ := strconv.Atoi(startDate[0:4])
		month, _ := strconv.Atoi(startDate[4:6])
		day, _ := strconv.Atoi(startDate[6:8])
		
		// Parse HHMMSSSS (HH MM SS SS where last SS is centiseconds)
		hour, _ := strconv.Atoi(startTime[0:2])
		minute, _ := strconv.Atoi(startTime[2:4])
		second, _ := strconv.Atoi(startTime[4:6])
		// Ignore centiseconds for uptime calculation
		
		parsedTime = time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	}
	
	// Calculate uptime as seconds since start time
	uptime := time.Since(parsedTime)
	if uptime < 0 {
		return 0, fmt.Errorf("start time is in the future: %v", parsedTime)
	}
	
	return int64(uptime.Seconds()), nil
}