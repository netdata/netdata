// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"strings"
)

// EnableResourceMonitoring initializes the resource monitoring system
// This should be called after successful connection to the queue manager
func (c *Client) EnableResourceMonitoring() error {
	if !c.connected {
		return fmt.Errorf("not connected to queue manager")
	}
	
	// Check global ResourceStatus - never retry if discovery failed permanently
	switch c.resourceStatus {
	case ResourceStatusFailed:
		c.protocol.Debugf("Resource monitoring permanently disabled due to previous discovery failure")
		return fmt.Errorf("resource monitoring discovery failed permanently")
		
	case ResourceStatusEnabled:
		c.protocol.Debugf("Resource monitoring already enabled for queue manager '%s'", c.config.QueueManager)
		return nil
		
	case ResourceStatusDisabled:
		// First attempt - proceed with initialization
		c.protocol.Debugf("Attempting resource monitoring discovery for queue manager '%s'", c.config.QueueManager)
	}
	
	// Mark that resource monitoring should be enabled for this connection
	c.resourceMonitoringEnabled = true
	
	// Create and initialize the resource monitor
	c.resourceMonitor = NewResourceMonitor(c)
	if err := c.resourceMonitor.Initialize(); err != nil {
		// Discovery failed permanently - mark as failed and never try again
		c.resourceMonitor = nil
		c.resourceStatus = ResourceStatusFailed
		c.protocol.Warningf("Resource monitoring discovery failed permanently: %v", err)
		return fmt.Errorf("failed to initialize resource monitor: %w", err)
	}
	
	// Success - mark as enabled
	c.resourceStatus = ResourceStatusEnabled
	c.protocol.Infof("Resource monitoring enabled successfully for queue manager '%s'", c.config.QueueManager)
	return nil
}

// GetResourcePublications retrieves and processes resource monitoring publications
func (c *Client) GetResourcePublications() (*ResourcePublicationResult, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected to queue manager")
	}
	
	if c.resourceMonitor == nil {
		return nil, fmt.Errorf("resource monitoring not enabled")
	}
	
	c.protocol.Debugf("Getting resource publications from queue manager '%s'", c.config.QueueManager)
	
	publications, err := c.resourceMonitor.ProcessPublications()
	if err != nil {
		return nil, fmt.Errorf("failed to process publications: %w", err)
	}
	
	// Process publications into metrics
	result := &ResourcePublicationResult{
		Stats: CollectionStats{},
	}
	result.Stats.Discovery.Success = true
	result.Stats.Discovery.AvailableItems = int64(len(publications))
	
	c.protocol.Debugf("Processing %d publications into metrics", len(publications))
	
	// Group publications by type and extract metrics
	for i, pub := range publications {
		c.protocol.Debugf("Publication %d: class=%s, type=%s, values=%d", 
			i, pub.MetricClass, pub.MetricType, len(pub.Values))
		
		// Log all values for debugging
		for k, v := range pub.Values {
			c.protocol.Debugf("  %s = %d", k, v)
		}
		
		// Process values using discovered parameter IDs
		for paramKey, value := range pub.Values {
			// Extract parameter ID from key (e.g., "metric_1234" -> 1234)
			var paramID int32
			if _, err := fmt.Sscanf(paramKey, "metric_%d", &paramID); err != nil {
				continue
			}
			
			// Look up metric definition by parameter ID
			if metric, ok := c.resourceMonitor.GetMetricByParameterID(C.MQLONG(paramID)); ok {
				c.protocol.Debugf("Found metric for param %d: %s/%s/%s", paramID, metric.Class, metric.Type, metric.Element)
				
				// Map to result fields based on element name
				switch {
				case metric.Class == "CPU" && strings.Contains(metric.Element, "User CPU"):
					result.UserCPUPercent = AttributeValue(value)
					c.protocol.Debugf("User CPU: %d%%", value)
					
				case metric.Class == "CPU" && strings.Contains(metric.Element, "System CPU"):
					result.SystemCPUPercent = AttributeValue(value)
					c.protocol.Debugf("System CPU: %d%%", value)
					
				case metric.Class == "MEMORY" && strings.Contains(metric.Element, "bytes in use"):
					result.MemoryUsedMB = AttributeValue(value / (1024 * 1024)) // Convert bytes to MB
					c.protocol.Debugf("Memory used: %d MB", value/(1024*1024))
					
				case metric.Class == "DISK" && metric.Type == "Log" && strings.Contains(metric.Element, "bytes in use"):
					result.LogUsedBytes = AttributeValue(value)
					c.protocol.Debugf("Log bytes used: %d", value)
					
				case metric.Class == "DISK" && metric.Type == "Log" && strings.Contains(metric.Element, "bytes max"):
					result.LogMaxBytes = AttributeValue(value)
					c.protocol.Debugf("Log max bytes: %d", value)
				}
			}
		}
		
	}
	
	c.protocol.Infof("Processed %d resource publications from queue manager '%s' - CPU: %v/%v, Memory: %v, Log: %v/%v", 
		len(publications), c.config.QueueManager, 
		result.UserCPUPercent.IsCollected(), result.SystemCPUPercent.IsCollected(),
		result.MemoryUsedMB.IsCollected(),
		result.LogUsedBytes.IsCollected(), result.LogMaxBytes.IsCollected())
	
	return result, nil
}

// ResourcePublicationResult contains the processed resource monitoring metrics
type ResourcePublicationResult struct {
	// CPU metrics
	UserCPUPercent   AttributeValue
	SystemCPUPercent AttributeValue
	
	// Memory metrics
	MemoryUsedMB     AttributeValue
	MemoryMaxMB      AttributeValue
	
	// Log utilization metrics
	LogUsedBytes     AttributeValue
	LogMaxBytes      AttributeValue
	LogUtilPercent   AttributeValue
	
	// Collection statistics
	Stats CollectionStats
}

// IsResourceMonitoringSupported checks if the queue manager supports resource monitoring
func (c *Client) IsResourceMonitoringSupported() bool {
	if !c.connected {
		return false
	}
	
	// Basic version and platform requirements
	if c.cachedCommandLevel < 900 {
		c.protocol.Debugf("Resource monitoring not supported: command level %d < 900 (MQ V9.0+ required)", c.cachedCommandLevel)
		return false
	}
	
	// z/OS (platform 0) doesn't support resource publications
	if c.cachedPlatform == 0 {
		c.protocol.Debugf("Resource monitoring not supported: z/OS platform doesn't support resource publications")
		return false
	}
	
	// Enhanced validation - check MONINT configuration
	// This is the critical check that IBM does to ensure metadata publication works
	if !c.validateResourceMonitoringConfiguration() {
		return false
	}
	
	return true
}

// validateResourceMonitoringConfiguration validates queue manager configuration for resource monitoring
// Based on IBM's approach - checks MONINT and other critical settings
func (c *Client) validateResourceMonitoringConfiguration() bool {
	c.protocol.Debugf("Validating resource monitoring configuration for queue manager '%s'", c.config.QueueManager)
	
	// Query queue manager attributes to check MONINT and other resource monitoring settings
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q_MGR, nil)
	if err != nil {
		c.protocol.Warningf("Failed to query queue manager attributes for resource monitoring validation: %v", err)
		return false
	}
	
	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		c.protocol.Warningf("Failed to parse queue manager attributes for resource monitoring validation: %v", err)
		return false
	}
	
	// Log basic queue manager info
	c.protocol.Debugf("Validating resource monitoring configuration for queue manager '%s' (%d attributes returned)", c.config.QueueManager, len(attrs))
	
	// Check MONINT using the correct constant: MQIA_STATISTICS_INTERVAL
	// This is the actual constant that represents the monitoring interval (MONINT)
	if monint, ok := attrs[C.MQIA_STATISTICS_INTERVAL]; ok {
		if interval, ok := monint.(int32); ok {
			if interval == 0 {
				c.protocol.Infof("Resource monitoring disabled: MONINT is OFF (0). Enable with 'ALTER QMGR MONINT(10)'")
				return false
			}
			c.protocol.Infof("Queue manager '%s' has MONINT enabled: %d seconds - resource monitoring available", c.config.QueueManager, interval)
		}
	} else {
		c.protocol.Warningf("MQIA_STATISTICS_INTERVAL attribute not found - resource monitoring may not be available")
		return false
	}
	
	// Log available monitoring-related attributes for debugging
	// These constants exist in the MQ headers and help understand monitoring configuration
	
	// Check MONCHL (Channel Monitoring) - affects resource data quality
	if monchl, ok := attrs[C.MQIA_MONITORING_CHANNEL]; ok {
		if chlMon, ok := monchl.(int32); ok {
			c.protocol.Debugf("Queue manager '%s' MONCHL (MQIA_MONITORING_CHANNEL): %d", c.config.QueueManager, chlMon)
		}
	}
	
	// Check MONQ (Queue Monitoring) - affects resource data quality
	if monq, ok := attrs[C.MQIA_MONITORING_Q]; ok {
		if qMon, ok := monq.(int32); ok {
			c.protocol.Debugf("Queue manager '%s' MONQ (MQIA_MONITORING_Q): %d", c.config.QueueManager, qMon)
		}
	}
	
	// Check MONACLS (Activity Recording) - affects resource monitoring availability
	if monacls, ok := attrs[C.MQIA_ACTIVITY_RECORDING]; ok {
		if acls, ok := monacls.(int32); ok {
			c.protocol.Debugf("Queue manager '%s' MONACLS (MQIA_ACTIVITY_RECORDING): %d", c.config.QueueManager, acls)
		}
	}
	
	c.protocol.Infof("Resource monitoring configuration validated successfully for queue manager '%s'", c.config.QueueManager)
	return true
}