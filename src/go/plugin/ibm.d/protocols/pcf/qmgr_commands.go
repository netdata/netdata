// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
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