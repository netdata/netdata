// SPDX-License-Identifier: GPL-3.0-or-later

package pcf

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)

// GetListenerList returns a list of listeners.
func (c *Client) GetListenerList() ([]string, error) {
	c.protocol.Debugf("Getting listener list from queue manager '%s'", c.config.QueueManager)

	const pattern = "*"
	params := []pcfParameter{
		newStringParameter(ibmmq.MQCACH_LISTENER_NAME, pattern),
	}
	response, err := c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_LISTENER, params)
	if err != nil {
		c.protocol.Errorf("Failed to get listener list from queue manager '%s': %v", c.config.QueueManager, err)
		return nil, err
	}

	// Parse the multi-message response
	result := c.parseListenerListResponseFromParams(response)

	// Log any errors encountered during parsing
	if result.InternalErrors > 0 {
		c.protocol.Warningf("Encountered %d internal errors while parsing listener list from queue manager '%s'",
			result.InternalErrors, c.config.QueueManager)
	}

	for errCode, count := range result.ErrorCounts {
		if errCode < 0 {
			// Internal error
			c.protocol.Warningf("Internal error %d occurred %d times while parsing listener list from queue manager '%s'",
				errCode, count, c.config.QueueManager)
		} else {
			// MQ error
			c.protocol.Warningf("MQ error %d (%s) occurred %d times while parsing listener list from queue manager '%s'",
				errCode, mqReasonString(errCode), count, c.config.QueueManager)
		}
	}

	c.protocol.Debugf("Retrieved %d listeners from queue manager '%s'", len(result.Listeners), c.config.QueueManager)

	return result.Listeners, nil
}


// GetListenerStatus returns runtime status for a specific listener.
func (c *Client) GetListenerStatus(listenerName string) (*ListenerMetrics, error) {
	c.protocol.Debugf("Getting status for listener '%s' from queue manager '%s'", listenerName, c.config.QueueManager)

	params := []pcfParameter{
		newStringParameter(ibmmq.MQCACH_LISTENER_NAME, listenerName),
	}
	response, err := c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_LISTENER_STATUS, params)
	if err != nil {
		c.protocol.Errorf("Failed to get status for listener '%s' from queue manager '%s': %v",
			listenerName, c.config.QueueManager, err)
		return nil, err
	}

	attrs, err := c.parsePCFResponseFromParams(response, "")
	if err != nil {
		c.protocol.Errorf("Failed to parse status response for listener '%s' from queue manager '%s': %v",
			listenerName, c.config.QueueManager, err)
		return nil, err
	}

	metrics := &ListenerMetrics{
		Name: listenerName,
	}

	// Get listener status
	if status, ok := attrs[ibmmq.MQIACH_LISTENER_STATUS]; ok {
		metrics.Status = ListenerStatus(status.(int32))
	}

	// Get listener port
	if port, ok := attrs[ibmmq.MQIA_LISTENER_PORT_NUMBER]; ok {
		metrics.Port = int64(port.(int32))
	}

	// Get backlog
	if backlog, ok := attrs[ibmmq.MQIACH_BACKLOG]; ok {
		if val, ok := backlog.(int32); ok {
			metrics.Backlog = AttributeValue(val)
		}
	} else {
		metrics.Backlog = NotCollected
	}

	// Get IP address
	if ipAddr, ok := attrs[ibmmq.MQCACH_IP_ADDRESS]; ok {
		if val, ok := ipAddr.(string); ok {
			metrics.IPAddress = strings.TrimSpace(val)
		}
	}

	// Get description
	if desc, ok := attrs[ibmmq.MQCACH_LISTENER_DESC]; ok {
		if val, ok := desc.(string); ok {
			metrics.Description = strings.TrimSpace(val)
		}
	}

	// Get start date and time to calculate uptime
	if startDate, ok := attrs[ibmmq.MQCACH_LISTENER_START_DATE]; ok {
		if dateStr, ok := startDate.(string); ok {
			metrics.StartDate = strings.TrimSpace(dateStr)
		}
	}

	if startTime, ok := attrs[ibmmq.MQCACH_LISTENER_START_TIME]; ok {
		if timeStr, ok := startTime.(string); ok {
			metrics.StartTime = strings.TrimSpace(timeStr)
		}
	}

	// Calculate uptime if we have both start date and time (only for running listeners)
	if metrics.StartDate != "" && metrics.StartTime != "" && metrics.Status == ListenerStatusRunning {
		uptime, err := CalculateUptimeSeconds(metrics.StartDate, metrics.StartTime)
		if err != nil {
			c.protocol.Debugf("Failed to calculate uptime for listener '%s': %v", listenerName, err)
			metrics.Uptime = -1 // Indicate error in calculation
		} else {
			metrics.Uptime = uptime
			c.protocol.Debugf("Listener '%s' uptime: %d seconds (start: %s %s)",
				listenerName, uptime, metrics.StartDate, metrics.StartTime)
		}
	} else {
		metrics.Uptime = 0 // Not running or no start time available
		c.protocol.Debugf("Listener '%s' - no uptime: status=%v, date='%s', time='%s'",
			listenerName, metrics.Status, metrics.StartDate, metrics.StartTime)
	}

	return metrics, nil
}

// GetListeners collects comprehensive listener metrics with full transparency statistics
func (c *Client) GetListeners(collectMetrics bool, maxListeners int, selector string, collectSystem bool) (*ListenerCollectionResult, error) {
	c.protocol.Debugf("Collecting listener metrics with selector '%s', max=%d, metrics=%v, system=%v",
		selector, maxListeners, collectMetrics, collectSystem)

	result := &ListenerCollectionResult{
		Stats: CollectionStats{},
	}

	// Step 1: Discovery - get all listener names
	response, err := c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_LISTENER, []pcfParameter{
		newStringParameter(ibmmq.MQCACH_LISTENER_NAME, "*"), // Always discover ALL listeners
	})
	if err != nil {
		result.Stats.Discovery.Success = false
		c.protocol.Errorf("Listener discovery failed: %v", err)
		return result, fmt.Errorf("listener discovery failed: %w", err)
	}

	result.Stats.Discovery.Success = true

	// Parse discovery response
	parsed := c.parseListenerListResponseFromParams(response)

	// Track discovery statistics
	successfulItems := int64(len(parsed.Listeners))

	// Count all errors as invisible items
	var invisibleItems int64
	for _, count := range parsed.ErrorCounts {
		invisibleItems += int64(count)
	}

	result.Stats.Discovery.AvailableItems = successfulItems + invisibleItems
	result.Stats.Discovery.InvisibleItems = invisibleItems
	result.Stats.Discovery.UnparsedItems = 0 // No unparsed items at discovery level
	result.Stats.Discovery.ErrorCounts = parsed.ErrorCounts

	// Log discovery errors
	for errCode, count := range parsed.ErrorCounts {
		if errCode < 0 {
			c.protocol.Warningf("Internal error %d occurred %d times during listener discovery", errCode, count)
		} else {
			c.protocol.Warningf("MQ error %d (%s) occurred %d times during listener discovery",
				errCode, mqReasonString(errCode), count)
		}
	}

	if len(parsed.Listeners) == 0 {
		c.protocol.Debugf("No listeners discovered")
		return result, nil
	}

	// Step 2: Smart filtering decision
	visibleItems := result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems - result.Stats.Discovery.UnparsedItems
	enrichAll := maxListeners <= 0 || visibleItems <= int64(maxListeners)

	c.protocol.Debugf("Discovery found %d visible listeners (total: %d, invisible: %d, unparsed: %d). EnrichAll=%v",
		visibleItems, result.Stats.Discovery.AvailableItems, result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.UnparsedItems, enrichAll)

	// Step 3: Apply filtering
	var listenersToEnrich []string
	if enrichAll || selector == "*" {
		// Enrich everything we can see (with system filtering)
		for _, listenerName := range parsed.Listeners {
			// Filter out system listeners if not wanted
			if !collectSystem && strings.HasPrefix(listenerName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}
			listenersToEnrich = append(listenersToEnrich, listenerName)
			result.Stats.Discovery.IncludedItems++
		}
		c.protocol.Debugf("Enriching %d listeners (excluded %d system listeners)",
			len(listenersToEnrich), result.Stats.Discovery.ExcludedItems)
	} else {
		// Apply selector pattern and system filtering
		for _, listenerName := range parsed.Listeners {
			// Filter out system listeners first if not wanted
			if !collectSystem && strings.HasPrefix(listenerName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}

			matched, err := filepath.Match(selector, listenerName)
			if err != nil {
				c.protocol.Warningf("Invalid selector pattern '%s': %v", selector, err)
				matched = false
			}

			if matched {
				listenersToEnrich = append(listenersToEnrich, listenerName)
				result.Stats.Discovery.IncludedItems++
			} else {
				result.Stats.Discovery.ExcludedItems++
			}
		}
		c.protocol.Debugf("Selector '%s' matched %d listeners, excluded %d (including system filtering)",
			selector, result.Stats.Discovery.IncludedItems, result.Stats.Discovery.ExcludedItems)
	}

	// Step 4: Enrich selected listeners
	for _, listenerName := range listenersToEnrich {
		lm := ListenerMetrics{
			Name: listenerName,
		}

		// Collect metrics if requested
		if collectMetrics {
			if result.Stats.Metrics == nil {
				result.Stats.Metrics = &EnrichmentStats{
					TotalItems:  int64(len(listenersToEnrich)),
					ErrorCounts: make(map[int32]int),
				}
			}

			metricsData, err := c.GetListenerStatus(listenerName)
			if err != nil {
				result.Stats.Metrics.FailedItems++
				if pcfErr, ok := err.(*PCFError); ok {
					result.Stats.Metrics.ErrorCounts[pcfErr.Code]++
				} else {
					result.Stats.Metrics.ErrorCounts[-1]++
				}
				c.protocol.Debugf("Failed to get metrics for listener '%s': %v", listenerName, err)
			} else {
				result.Stats.Metrics.OkItems++

				// Copy all metrics data
				lm.Status = metricsData.Status
				lm.Port = metricsData.Port
				lm.Backlog = metricsData.Backlog
				lm.IPAddress = metricsData.IPAddress
				lm.Description = metricsData.Description
				lm.StartDate = metricsData.StartDate
				lm.StartTime = metricsData.StartTime
				lm.Uptime = metricsData.Uptime
			}
		}

		result.Listeners = append(result.Listeners, lm)
	}

	// Count collected fields across all listeners for detailed summary
	fieldCounts := make(map[string]int)
	for _, lst := range result.Listeners {
		// Always have status
		fieldCounts["status"]++

		// Port is usually available
		if lst.Port > 0 {
			fieldCounts["port"]++
		}

		// Backlog
		if lst.Backlog.IsCollected() {
			fieldCounts["backlog"]++
		}

		// IP address
		if lst.IPAddress != "" {
			fieldCounts["ip_address"]++
		}

		// Uptime
		if lst.Uptime > 0 {
			fieldCounts["uptime"]++
		}
	}

	// Log summary
	c.protocol.Debugf("Listener collection complete - discovered:%d visible:%d included:%d enriched:%d",
		result.Stats.Discovery.AvailableItems,
		result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Listeners))

	// Log field collection summary
	c.protocol.Debugf("Listener field collection summary: status=%d port=%d backlog=%d ip_address=%d uptime=%d",
		fieldCounts["status"], fieldCounts["port"], fieldCounts["backlog"], 
		fieldCounts["ip_address"], fieldCounts["uptime"])

	return result, nil
}

// ListenerListResult contains the result of parsing listener list response
type ListenerListResult struct {
	Listeners      []string
	InternalErrors int
	ErrorCounts    map[int32]int
}