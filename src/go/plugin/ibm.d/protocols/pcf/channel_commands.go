// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"path/filepath"
	"strings"
)


// GetChannelList returns a list of channels.
func (c *Client) GetChannelList() ([]string, error) {
	c.protocol.Debugf("PCF: Getting channel list from queue manager '%s'", c.config.QueueManager)
	
	const pattern = "*"
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, pattern),
	}
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_CHANNEL, params)
	if err != nil {
		c.protocol.Errorf("PCF: Failed to get channel list from queue manager '%s': %v", c.config.QueueManager, err)
		return nil, err
	}

	// Parse the multi-message response
	result := c.parseChannelListResponse(response)
	
	// Log any errors encountered during parsing
	if result.InternalErrors > 0 {
		c.protocol.Warningf("PCF: Encountered %d internal errors while parsing channel list from queue manager '%s'", 
			result.InternalErrors, c.config.QueueManager)
	}
	
	for errCode, count := range result.ErrorCounts {
		if errCode < 0 {
			// Internal error
			c.protocol.Warningf("PCF: Internal error %d occurred %d times while parsing channel list from queue manager '%s'", 
				errCode, count, c.config.QueueManager)
		} else {
			// MQ error
			c.protocol.Warningf("PCF: MQ error %d (%s) occurred %d times while parsing channel list from queue manager '%s'", 
				errCode, mqReasonString(errCode), count, c.config.QueueManager)
		}
	}
	
	c.protocol.Debugf("PCF: Retrieved %d channels from queue manager '%s'", len(result.Channels), c.config.QueueManager)
	
	return result.Channels, nil
}

// GetChannelMetrics returns metrics for a specific channel.
func (c *Client) GetChannelMetrics(channelName string) (*ChannelMetrics, error) {
	c.protocol.Debugf("PCF: Getting metrics for channel '%s' from queue manager '%s'", channelName, c.config.QueueManager)
	
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, channelName),
	}
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_CHANNEL_STATUS, params)
	if err != nil {
		c.protocol.Errorf("PCF: Failed to get metrics for channel '%s' from queue manager '%s': %v", 
			channelName, c.config.QueueManager, err)
		return nil, err
	}

	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		c.protocol.Errorf("PCF: Failed to parse metrics response for channel '%s' from queue manager '%s': %v", 
			channelName, c.config.QueueManager, err)
		return nil, err
	}
	

	metrics := &ChannelMetrics{
		Name: channelName,
	}
	
	// Channel type is not available in INQUIRE_CHANNEL_STATUS
	// Would need a separate INQUIRE_CHANNEL call to get it
	
	// Get channel status
	if status, ok := attrs[C.MQIACH_CHANNEL_STATUS]; ok {
		metrics.Status = ChannelStatus(status.(int32))
	}
	
	// Get message metrics (only for message channels)
	if messages, ok := attrs[C.MQIACH_MSGS]; ok {
		val := int64(messages.(int32))
		metrics.Messages = &val
	}
	if bytes, ok := attrs[C.MQIACH_BYTES_SENT]; ok {
		val := int64(bytes.(int32))
		metrics.Bytes = &val
	}
	if batches, ok := attrs[C.MQIACH_BATCHES]; ok {
		val := int64(batches.(int32))
		metrics.Batches = &val
	}
	
	// Note: Current connection count is not available via PCF for individual channels
	// It's only available at the queue manager level

	return metrics, nil
}

// GetChannelConfig returns configuration for a specific channel.
func (c *Client) GetChannelConfig(channelName string) (*ChannelConfig, error) {
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, channelName),
	}
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_CHANNEL, params)
	if err != nil {
		return nil, err
	}

	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		return nil, err
	}

	config := &ChannelConfig{
		Name: channelName,
	}
	
	// Get channel type
	if channelType, ok := attrs[C.MQIACH_CHANNEL_TYPE]; ok {
		config.Type = ChannelType(channelType.(int32))
	}
	
	// Initialize all attributes to NotCollected
	config.BatchSize = NotCollected
	config.BatchInterval = NotCollected
	config.DiscInterval = NotCollected
	config.HbInterval = NotCollected
	config.KeepAliveInterval = NotCollected
	config.ShortRetry = NotCollected
	config.LongRetry = NotCollected
	config.MaxMsgLength = NotCollected
	config.SharingConversations = NotCollected
	config.NetworkPriority = NotCollected
	
	// Batch configuration - only set when actually present
	if batchSize, ok := attrs[C.MQIACH_BATCH_SIZE]; ok {
		config.BatchSize = AttributeValue(batchSize.(int32))
	}
	if batchInterval, ok := attrs[C.MQIACH_BATCH_INTERVAL]; ok {
		config.BatchInterval = AttributeValue(batchInterval.(int32))
	}
	
	// Intervals - only set when actually present
	if discInterval, ok := attrs[C.MQIACH_DISC_INTERVAL]; ok {
		config.DiscInterval = AttributeValue(discInterval.(int32))
	}
	if hbInterval, ok := attrs[C.MQIACH_HB_INTERVAL]; ok {
		config.HbInterval = AttributeValue(hbInterval.(int32))
	}
	if keepAliveInterval, ok := attrs[C.MQIACH_KEEP_ALIVE_INTERVAL]; ok {
		config.KeepAliveInterval = AttributeValue(keepAliveInterval.(int32))
	}
	
	// Retry configuration - only set when actually present
	if shortRetry, ok := attrs[C.MQIACH_SHORT_RETRY]; ok {
		config.ShortRetry = AttributeValue(shortRetry.(int32))
	}
	if longRetry, ok := attrs[C.MQIACH_LONG_RETRY]; ok {
		config.LongRetry = AttributeValue(longRetry.(int32))
	}
	
	// Limits - only set when actually present
	if maxMsgLength, ok := attrs[C.MQIACH_MAX_MSG_LENGTH]; ok {
		config.MaxMsgLength = AttributeValue(maxMsgLength.(int32))
	}
	if sharingConvs, ok := attrs[C.MQIACH_SHARING_CONVERSATIONS]; ok {
		config.SharingConversations = AttributeValue(sharingConvs.(int32))
	}
	if netPriority, ok := attrs[C.MQIACH_NETWORK_PRIORITY]; ok {
		config.NetworkPriority = AttributeValue(netPriority.(int32))
	}

	return config, nil
}

// GetChannels collects comprehensive channel metrics with full transparency statistics
func (c *Client) GetChannels(collectConfig, collectMetrics bool, maxChannels int, selector string, collectSystem bool) (*ChannelCollectionResult, error) {
	c.protocol.Debugf("PCF: Collecting channel metrics with selector '%s', max=%d, config=%v, metrics=%v, system=%v", 
		selector, maxChannels, collectConfig, collectMetrics, collectSystem)
	
	result := &ChannelCollectionResult{
		Stats: CollectionStats{},
	}
	
	// Step 1: Discovery - get all channel names
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_CHANNEL, []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, "*"), // Always discover ALL channels
	})
	if err != nil {
		result.Stats.Discovery.Success = false
		c.protocol.Errorf("PCF: Channel discovery failed: %v", err)
		return result, fmt.Errorf("channel discovery failed: %w", err)
	}
	
	result.Stats.Discovery.Success = true
	
	// Parse discovery response
	parsed := c.parseChannelListResponse(response)
	
	// Track discovery statistics
	successfulItems := int64(len(parsed.Channels))
	
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
			c.protocol.Warningf("PCF: Internal error %d occurred %d times during channel discovery", errCode, count)
		} else {
			c.protocol.Warningf("PCF: MQ error %d (%s) occurred %d times during channel discovery", 
				errCode, mqReasonString(errCode), count)
		}
	}
	
	if len(parsed.Channels) == 0 {
		c.protocol.Debugf("PCF: No channels discovered")
		return result, nil
	}
	
	// Step 2: Smart filtering decision
	visibleItems := result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems - result.Stats.Discovery.UnparsedItems
	enrichAll := maxChannels <= 0 || visibleItems <= int64(maxChannels)
	
	c.protocol.Debugf("PCF: Discovery found %d visible channels (total: %d, invisible: %d, unparsed: %d). EnrichAll=%v", 
		visibleItems, result.Stats.Discovery.AvailableItems, result.Stats.Discovery.InvisibleItems, 
		result.Stats.Discovery.UnparsedItems, enrichAll)
	
	// Step 3: Apply filtering
	var channelsToEnrich []string
	if enrichAll || selector == "*" {
		// Enrich everything we can see (with system filtering)
		for _, channelName := range parsed.Channels {
			// Filter out system channels if not wanted
			if !collectSystem && strings.HasPrefix(channelName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}
			channelsToEnrich = append(channelsToEnrich, channelName)
			result.Stats.Discovery.IncludedItems++
		}
		c.protocol.Debugf("PCF: Enriching %d channels (excluded %d system channels)", 
			len(channelsToEnrich), result.Stats.Discovery.ExcludedItems)
	} else {
		// Apply selector pattern and system filtering
		for _, channelName := range parsed.Channels {
			// Filter out system channels first if not wanted
			if !collectSystem && strings.HasPrefix(channelName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}
			
			matched, err := filepath.Match(selector, channelName)
			if err != nil {
				c.protocol.Warningf("PCF: Invalid selector pattern '%s': %v", selector, err)
				matched = false
			}
			
			if matched {
				channelsToEnrich = append(channelsToEnrich, channelName)
				result.Stats.Discovery.IncludedItems++
			} else {
				result.Stats.Discovery.ExcludedItems++
			}
		}
		c.protocol.Debugf("PCF: Selector '%s' matched %d channels, excluded %d (including system filtering)", 
			selector, result.Stats.Discovery.IncludedItems, result.Stats.Discovery.ExcludedItems)
	}
	
	// Step 4: Enrich selected channels
	for _, channelName := range channelsToEnrich {
		cm := ChannelMetrics{
			Name: channelName,
			// Initialize configuration fields to NotCollected
			BatchSize:            NotCollected,
			BatchInterval:        NotCollected,
			DiscInterval:         NotCollected,
			HbInterval:           NotCollected,
			KeepAliveInterval:    NotCollected,
			ShortRetry:           NotCollected,
			LongRetry:            NotCollected,
			MaxMsgLength:         NotCollected,
			SharingConversations: NotCollected,
			NetworkPriority:      NotCollected,
		}
		
		// 4a: Collect configuration if requested
		if collectConfig {
			if result.Stats.Config == nil {
				result.Stats.Config = &EnrichmentStats{
					TotalItems:  int64(len(channelsToEnrich)),
					ErrorCounts: make(map[int32]int),
				}
			}
			
			configData, err := c.GetChannelConfig(channelName)
			if err != nil {
				result.Stats.Config.FailedItems++
				// Extract error code if possible
				if pcfErr, ok := err.(*PCFError); ok {
					result.Stats.Config.ErrorCounts[pcfErr.Code]++
				} else {
					result.Stats.Config.ErrorCounts[-1]++ // Unknown error
				}
				c.protocol.Debugf("PCF: Failed to get config for channel '%s': %v", channelName, err)
				// Don't skip the channel - continue to collect runtime metrics
			} else {
				result.Stats.Config.OkItems++
				
				// Copy config data
				cm.Type = configData.Type
				cm.BatchSize = configData.BatchSize
				cm.BatchInterval = configData.BatchInterval
				cm.DiscInterval = configData.DiscInterval
				cm.HbInterval = configData.HbInterval
				cm.KeepAliveInterval = configData.KeepAliveInterval
				cm.ShortRetry = configData.ShortRetry
				cm.LongRetry = configData.LongRetry
				cm.MaxMsgLength = configData.MaxMsgLength
				cm.SharingConversations = configData.SharingConversations
				cm.NetworkPriority = configData.NetworkPriority
			}
		}
		
		// 4b: Collect metrics if requested
		if collectMetrics {
			if result.Stats.Metrics == nil {
				result.Stats.Metrics = &EnrichmentStats{
					TotalItems:  int64(len(channelsToEnrich)),
					ErrorCounts: make(map[int32]int),
				}
			}
			
			metricsData, err := c.GetChannelMetrics(channelName)
			if err != nil {
				result.Stats.Metrics.FailedItems++
				if pcfErr, ok := err.(*PCFError); ok {
					result.Stats.Metrics.ErrorCounts[pcfErr.Code]++
				} else {
					result.Stats.Metrics.ErrorCounts[-1]++
				}
				c.protocol.Debugf("PCF: Failed to get metrics for channel '%s': %v", channelName, err)
			} else {
				result.Stats.Metrics.OkItems++
				
				// Copy metrics data
				cm.Status = metricsData.Status
				cm.Messages = metricsData.Messages
				cm.Bytes = metricsData.Bytes
				cm.Batches = metricsData.Batches
				cm.Connections = metricsData.Connections
				cm.BuffersUsed = metricsData.BuffersUsed
				cm.BuffersMax = metricsData.BuffersMax
			}
		}
		
		result.Channels = append(result.Channels, cm)
	}
	
	// Count collected fields across all channels for detailed summary
	fieldCounts := make(map[string]int)
	for _, ch := range result.Channels {
		// Always have status
		fieldCounts["status"]++
		
		// Runtime metrics
		if ch.Messages != nil {
			fieldCounts["messages"]++
		}
		if ch.Bytes != nil {
			fieldCounts["bytes"]++
		}
		if ch.Batches != nil {
			fieldCounts["batches"]++
		}
		if ch.Connections != nil {
			fieldCounts["connections"]++
		}
		if ch.BuffersUsed != nil {
			fieldCounts["buffers_used"]++
		}
		if ch.BuffersMax != nil {
			fieldCounts["buffers_max"]++
		}
		
		// Configuration metrics
		if ch.BatchSize.IsCollected() {
			fieldCounts["batch_size"]++
		}
		if ch.BatchInterval.IsCollected() {
			fieldCounts["batch_interval"]++
		}
		if ch.DiscInterval.IsCollected() {
			fieldCounts["disc_interval"]++
		}
		if ch.HbInterval.IsCollected() {
			fieldCounts["hb_interval"]++
		}
		if ch.KeepAliveInterval.IsCollected() {
			fieldCounts["keep_alive_interval"]++
		}
		if ch.ShortRetry.IsCollected() {
			fieldCounts["short_retry"]++
		}
		if ch.LongRetry.IsCollected() {
			fieldCounts["long_retry"]++
		}
		if ch.MaxMsgLength.IsCollected() {
			fieldCounts["max_msg_length"]++
		}
		if ch.SharingConversations.IsCollected() {
			fieldCounts["sharing_conversations"]++
		}
		if ch.NetworkPriority.IsCollected() {
			fieldCounts["network_priority"]++
		}
	}
	
	// Log summary
	c.protocol.Infof("PCF: Channel collection complete - discovered:%d visible:%d included:%d enriched:%d", 
		result.Stats.Discovery.AvailableItems, 
		result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Channels))
	
	// Log field collection summary
	c.protocol.Infof("PCF: Channel field collection summary: status=%d messages=%d bytes=%d batches=%d connections=%d "+
		"buffers_used=%d buffers_max=%d batch_size=%d batch_interval=%d disc_interval=%d hb_interval=%d "+
		"keep_alive_interval=%d short_retry=%d long_retry=%d max_msg_length=%d sharing_conversations=%d network_priority=%d",
		fieldCounts["status"], fieldCounts["messages"], fieldCounts["bytes"], fieldCounts["batches"],
		fieldCounts["connections"], fieldCounts["buffers_used"], fieldCounts["buffers_max"],
		fieldCounts["batch_size"], fieldCounts["batch_interval"], fieldCounts["disc_interval"],
		fieldCounts["hb_interval"], fieldCounts["keep_alive_interval"], fieldCounts["short_retry"],
		fieldCounts["long_retry"], fieldCounts["max_msg_length"], fieldCounts["sharing_conversations"],
		fieldCounts["network_priority"])
	
	return result, nil
}