// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

// GetListeners returns the runtime status of all listeners.
func (c *Client) GetListeners() ([]ListenerMetrics, error) {
	c.protocol.Debugf("PCF: Getting listener status for queue manager '%s'", c.config.QueueManager)
	
	// Request listener status for all listeners
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_LISTENER_STATUS, nil)
	if err != nil {
		c.protocol.Errorf("PCF: Failed to get listener status for queue manager '%s': %v", c.config.QueueManager, err)
		return nil, err
	}

	// Parse response to get listener information
	attrs, err := c.ParsePCFResponse(response, "MQCMD_INQUIRE_LISTENER_STATUS")
	if err != nil {
		c.protocol.Errorf("PCF: Failed to parse listener status response for queue manager '%s': %v", c.config.QueueManager, err)
		return nil, err
	}

	var listeners []ListenerMetrics
	listener := ListenerMetrics{}
	
	// Extract listener name (required)
	if name, ok := attrs[C.MQCACH_LISTENER_NAME]; ok {
		if nameStr, ok := name.(string); ok {
			listener.Name = nameStr
		} else {
			c.protocol.Warningf("PCF: Invalid listener name type for queue manager '%s'", c.config.QueueManager)
			return listeners, nil
		}
	} else {
		c.protocol.Debugf("PCF: No listeners found for queue manager '%s'", c.config.QueueManager)
		return listeners, nil
	}
	
	// Extract listener status (required)
	if status, ok := attrs[C.MQIACH_LISTENER_STATUS]; ok {
		if statusInt, ok := status.(int32); ok {
			listener.Status = ListenerStatus(statusInt)
			c.protocol.Debugf("PCF: Listener '%s' status: %d", listener.Name, statusInt)
		} else {
			c.protocol.Warningf("PCF: Invalid listener status type for listener '%s'", listener.Name)
			return listeners, nil
		}
	} else {
		c.protocol.Warningf("PCF: Missing listener status for listener '%s'", listener.Name)
		return listeners, nil
	}
	
	// Extract listener port (optional, for enhanced monitoring)
	if port, ok := attrs[C.MQIA_LISTENER_PORT_NUMBER]; ok {
		if portInt, ok := port.(int32); ok {
			listener.Port = int64(portInt)
			c.protocol.Debugf("PCF: Listener '%s' port: %d", listener.Name, portInt)
		}
	}
	
	listeners = append(listeners, listener)
	c.protocol.Debugf("PCF: Found listener '%s' with status %d and port %d", listener.Name, listener.Status, listener.Port)
	
	c.protocol.Debugf("PCF: Found %d listeners for queue manager '%s'", len(listeners), c.config.QueueManager)
	return listeners, nil
}