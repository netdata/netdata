// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo && ibm_mq
// +build cgo,ibm_mq

package pcf

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)

// GetChannelList returns a list of channels.
func (c *Client) GetChannelList() ([]string, error) {
	c.protocol.Debugf("Getting channel list from queue manager '%s'", c.config.QueueManager)

	const pattern = "*"
	params := []pcfParameter{
		newStringParameter(ibmmq.MQCACH_CHANNEL_NAME, pattern),
	}
	response, err := c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_CHANNEL, params)
	if err != nil {
		c.protocol.Errorf("Failed to get channel list from queue manager '%s': %v", c.config.QueueManager, err)
		return nil, err
	}

	result := c.parseChannelListResponseFromParams(response)

	if result.InternalErrors > 0 {
		c.protocol.Warningf("Encountered %d internal errors while parsing channel list from queue manager '%s'",
			result.InternalErrors, c.config.QueueManager)
	}

	for errCode, count := range result.ErrorCounts {
		if errCode < 0 {
			c.protocol.Warningf("Internal error %d occurred %d times while parsing channel list from queue manager '%s'",
				errCode, count, c.config.QueueManager)
		} else {
			c.protocol.Warningf("MQ error %d (%s) occurred %d times while parsing channel list from queue manager '%s'",
				errCode, mqReasonString(errCode), count, c.config.QueueManager)
		}
	}

	c.protocol.Debugf("Retrieved %d channels from queue manager '%s'", len(result.Channels), c.config.QueueManager)

	return result.Channels, nil
}

// GetChannelMetrics returns metrics for a specific channel.
func (c *Client) GetChannelMetrics(channelName string) (*ChannelMetrics, error) {
	c.protocol.Debugf("Getting metrics for channel '%s' from queue manager '%s'", channelName, c.config.QueueManager)

	params := []pcfParameter{
		newStringParameter(ibmmq.MQCACH_CHANNEL_NAME, channelName),
	}
	response, err := c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_CHANNEL_STATUS, params)
	if err != nil {
		c.protocol.Errorf("Failed to get metrics for channel '%s' from queue manager '%s': %v",
			channelName, c.config.QueueManager, err)
		return nil, err
	}

	attrs, err := c.parsePCFResponseFromParams(response, "")
	if err != nil {
		c.protocol.Errorf("Failed to parse metrics response for channel '%s' from queue manager '%s': %v",
			channelName, c.config.QueueManager, err)
		return nil, err
	}

	metrics := &ChannelMetrics{
		Name: channelName,
	}

	if status, ok := attrs[ibmmq.MQIACH_CHANNEL_STATUS]; ok {
		metrics.Status = ChannelStatus(status.(int32))
	}

	if messages, ok := attrs[ibmmq.MQIACH_MSGS]; ok {
		val := int64(messages.(int32))
		metrics.Messages = &val
	}
	if bytes, ok := attrs[ibmmq.MQIACH_BYTES_SENT]; ok {
		val := int64(bytes.(int32))
		metrics.Bytes = &val
	}
	if batches, ok := attrs[ibmmq.MQIACH_BATCHES]; ok {
		val := int64(batches.(int32))
		metrics.Batches = &val
	}

	// Initialize extended status metrics as NotCollected
	metrics.BuffersSent = NotCollected
	metrics.BuffersReceived = NotCollected
	metrics.CurrentMessages = NotCollected
	metrics.XmitQueueTime = NotCollected
	metrics.MCAStatus = NotCollected
	metrics.InDoubtStatus = NotCollected
	metrics.SSLKeyResets = NotCollected
	metrics.NPMSpeed = NotCollected
	metrics.CurrentSharingConvs = NotCollected

	// Collect extended status metrics if available
	if val, ok := attrs[ibmmq.MQIACH_BUFFERS_SENT]; ok {
		metrics.BuffersSent = AttributeValue(val.(int32))
		c.protocol.Debugf("Channel '%s' buffers sent: %d", channelName, val.(int32))
	}
	if val, ok := attrs[ibmmq.MQIACH_BUFFERS_RCVD]; ok {
		metrics.BuffersReceived = AttributeValue(val.(int32))
		c.protocol.Debugf("Channel '%s' buffers received: %d", channelName, val.(int32))
	}
	if val, ok := attrs[ibmmq.MQIACH_CURRENT_MSGS]; ok {
		metrics.CurrentMessages = AttributeValue(val.(int32))
		c.protocol.Debugf("Channel '%s' current messages: %d", channelName, val.(int32))
	}
	if val, ok := attrs[ibmmq.MQIACH_XMITQ_TIME_INDICATOR]; ok {
		metrics.XmitQueueTime = AttributeValue(val.(int32))
		c.protocol.Debugf("Channel '%s' XMITQ time: %d", channelName, val.(int32))
	}
	if val, ok := attrs[ibmmq.MQIACH_MCA_STATUS]; ok {
		metrics.MCAStatus = AttributeValue(val.(int32))
		c.protocol.Debugf("Channel '%s' MCA status: %d", channelName, val.(int32))
	}
	if val, ok := attrs[ibmmq.MQIACH_INDOUBT_STATUS]; ok {
		metrics.InDoubtStatus = AttributeValue(val.(int32))
		c.protocol.Debugf("Channel '%s' in-doubt status: %d", channelName, val.(int32))
	}
	if val, ok := attrs[ibmmq.MQIACH_SSL_KEY_RESETS]; ok {
		metrics.SSLKeyResets = AttributeValue(val.(int32))
		c.protocol.Debugf("Channel '%s' SSL key resets: %d", channelName, val.(int32))
	}
	if val, ok := attrs[ibmmq.MQIACH_NPM_SPEED]; ok {
		metrics.NPMSpeed = AttributeValue(val.(int32))
		c.protocol.Debugf("Channel '%s' NPM speed: %d", channelName, val.(int32))
	}
	if val, ok := attrs[ibmmq.MQIACH_CURRENT_SHARING_CONVS]; ok {
		metrics.CurrentSharingConvs = AttributeValue(val.(int32))
		c.protocol.Debugf("Channel '%s' current sharing conversations: %d", channelName, val.(int32))
	}

	// Connection name (string attribute)
	if val, ok := attrs[ibmmq.MQCACH_CONNECTION_NAME]; ok {
		if connStr, ok := val.(string); ok {
			metrics.ConnectionName = strings.TrimSpace(connStr)
			c.protocol.Debugf("Channel '%s' connection name: '%s'", channelName, metrics.ConnectionName)
		}
	}

	return metrics, nil
}

// GetChannelConfig returns configuration for a specific channel.
func (c *Client) GetChannelConfig(channelName string) (*ChannelConfig, error) {
	params := []pcfParameter{
		newStringParameter(ibmmq.MQCACH_CHANNEL_NAME, channelName),
	}
	response, err := c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_CHANNEL, params)
	if err != nil {
		return nil, err
	}

	attrs, err := c.parsePCFResponseFromParams(response, "")
	if err != nil {
		return nil, err
	}

	config := &ChannelConfig{
		Name: channelName,
	}

	if channelType, ok := attrs[ibmmq.MQIACH_CHANNEL_TYPE]; ok {
		config.Type = ChannelType(channelType.(int32))
	}

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

	if batchSize, ok := attrs[ibmmq.MQIACH_BATCH_SIZE]; ok {
		config.BatchSize = AttributeValue(batchSize.(int32))
	}
	if batchInterval, ok := attrs[ibmmq.MQIACH_BATCH_INTERVAL]; ok {
		config.BatchInterval = AttributeValue(batchInterval.(int32))
	}

	if discInterval, ok := attrs[ibmmq.MQIACH_DISC_INTERVAL]; ok {
		config.DiscInterval = AttributeValue(discInterval.(int32))
	}
	if hbInterval, ok := attrs[ibmmq.MQIACH_HB_INTERVAL]; ok {
		config.HbInterval = AttributeValue(hbInterval.(int32))
	}
	if keepAliveInterval, ok := attrs[ibmmq.MQIACH_KEEP_ALIVE_INTERVAL]; ok {
		config.KeepAliveInterval = AttributeValue(keepAliveInterval.(int32))
	}

	if shortRetry, ok := attrs[ibmmq.MQIACH_SHORT_RETRY]; ok {
		config.ShortRetry = AttributeValue(shortRetry.(int32))
	}
	if longRetry, ok := attrs[ibmmq.MQIACH_LONG_RETRY]; ok {
		config.LongRetry = AttributeValue(longRetry.(int32))
	}

	if maxMsgLength, ok := attrs[ibmmq.MQIACH_MAX_MSG_LENGTH]; ok {
		config.MaxMsgLength = AttributeValue(maxMsgLength.(int32))
	}
	if sharingConvs, ok := attrs[ibmmq.MQIACH_SHARING_CONVERSATIONS]; ok {
		config.SharingConversations = AttributeValue(sharingConvs.(int32))
	}
	if netPriority, ok := attrs[ibmmq.MQIACH_NETWORK_PRIORITY]; ok {
		config.NetworkPriority = AttributeValue(netPriority.(int32))
	}

	return config, nil
}

// GetChannels collects comprehensive channel metrics with full transparency statistics
func (c *Client) GetChannels(collectConfig, collectMetrics bool, maxChannels int, selector string, collectSystem bool) (*ChannelCollectionResult, error) {
	c.protocol.Debugf("Collecting channel metrics with selector '%s', max=%d, config=%v, metrics=%v, system=%v",
		selector, maxChannels, collectConfig, collectMetrics, collectSystem)

	result := &ChannelCollectionResult{
		Stats: CollectionStats{},
	}

	// Step 1: Enhanced Discovery (with channel types)
	channelInfos, err := c.discoverChannelsWithInfo(result)
	if err != nil {
		return result, err
	}

	// Step 2: Filtering (including template filtering)
	channelInfosToEnrich := c.filterChannelsEnhanced(channelInfos, selector, collectSystem, maxChannels, result)

	// Step 3: Enrichment (now pass channel info with types)
	c.enrichChannelsWithTypes(channelInfosToEnrich, collectConfig, collectMetrics, result)

	c.logChannelCollectionSummary(result)

	return result, nil
}

func (c *Client) discoverChannelsWithInfo(result *ChannelCollectionResult) ([]ChannelInfo, error) {
	response, err := c.sendPCFCommand(ibmmq.MQCMD_INQUIRE_CHANNEL, []pcfParameter{
		newStringParameter(ibmmq.MQCACH_CHANNEL_NAME, "*"),
	})
	if err != nil {
		result.Stats.Discovery.Success = false
		c.protocol.Errorf("Channel discovery failed: %v", err)
		return nil, fmt.Errorf("channel discovery failed: %w", err)
	}

	result.Stats.Discovery.Success = true

	// Parse channel info including type
	channelInfos := c.parseChannelInfoFromParams(response)

	successfulItems := int64(len(channelInfos))
	var invisibleItems int64
	// TODO: Add error counting when we update parseChannelInfoFromParams

	result.Stats.Discovery.AvailableItems = successfulItems + invisibleItems
	result.Stats.Discovery.InvisibleItems = invisibleItems

	if len(channelInfos) == 0 {
		c.protocol.Debugf("No channels discovered")
	} else {
		// Log channel types for debugging
		c.protocol.Debugf("Discovered %d channels with types", len(channelInfos))
		for _, info := range channelInfos {
			c.protocol.Debugf("  Channel '%s' type=%d", info.Name, info.Type)
		}
	}

	return channelInfos, nil
}

// Keep old function for compatibility
func (c *Client) discoverChannels(result *ChannelCollectionResult) ([]string, error) {
	infos, err := c.discoverChannelsWithInfo(result)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, info := range infos {
		names = append(names, info.Name)
	}
	return names, nil
}

func (c *Client) filterChannelsEnhanced(channelInfos []ChannelInfo, selector string, collectSystem bool, maxChannels int, result *ChannelCollectionResult) []ChannelInfo {
	visibleItems := result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems
	enrichAll := maxChannels <= 0 || visibleItems <= int64(maxChannels)

	c.protocol.Debugf("Discovery found %d visible channels (total: %d, invisible: %d). EnrichAll=%v",
		visibleItems, result.Stats.Discovery.AvailableItems, result.Stats.Discovery.InvisibleItems, enrichAll)

	var channelsToEnrich []ChannelInfo
	var templateChannelsSkipped int64

	if enrichAll || selector == "*" {
		for _, info := range channelInfos {
			// Skip template channels (SYSTEM.DEF.*)
			if strings.HasPrefix(info.Name, "SYSTEM.DEF.") {
				templateChannelsSkipped++
				c.protocol.Debugf("Skipping template channel '%s' (type=%d) - no runtime status available",
					info.Name, info.Type)
				continue
			}

			if !collectSystem && strings.HasPrefix(info.Name, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}
			channelsToEnrich = append(channelsToEnrich, info)
			result.Stats.Discovery.IncludedItems++
		}
		c.protocol.Debugf("Enriching %d channels (excluded %d system channels, skipped %d template channels)",
			len(channelsToEnrich), result.Stats.Discovery.ExcludedItems, templateChannelsSkipped)
	} else {
		for _, info := range channelInfos {
			// Skip template channels (SYSTEM.DEF.*)
			if strings.HasPrefix(info.Name, "SYSTEM.DEF.") {
				templateChannelsSkipped++
				c.protocol.Debugf("Skipping template channel '%s' (type=%d) - no runtime status available",
					info.Name, info.Type)
				continue
			}

			if !collectSystem && strings.HasPrefix(info.Name, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}

			matched, err := filepath.Match(selector, info.Name)
			if err != nil {
				c.protocol.Warningf("Invalid selector pattern '%s': %v", selector, err)
				matched = false
			}

			if matched {
				channelsToEnrich = append(channelsToEnrich, info)
				result.Stats.Discovery.IncludedItems++
			} else {
				result.Stats.Discovery.ExcludedItems++
			}
		}
		c.protocol.Debugf("Selector '%s' matched %d channels, excluded %d (including system filtering), skipped %d templates",
			selector, result.Stats.Discovery.IncludedItems, result.Stats.Discovery.ExcludedItems, templateChannelsSkipped)
	}

	// Update stats to account for template channels
	result.Stats.Discovery.AvailableItems -= templateChannelsSkipped

	return channelsToEnrich
}

// Keep old function for compatibility
func (c *Client) filterChannels(channels []string, selector string, collectSystem bool, maxChannels int, result *ChannelCollectionResult) []string {
	// Convert to ChannelInfo with unknown type for backward compatibility
	var infos []ChannelInfo
	for _, name := range channels {
		infos = append(infos, ChannelInfo{Name: name, Type: 0})
	}
	filteredInfos := c.filterChannelsEnhanced(infos, selector, collectSystem, maxChannels, result)

	// Convert back to strings
	var names []string
	for _, info := range filteredInfos {
		names = append(names, info.Name)
	}
	return names
}

func (c *Client) enrichChannels(channelsToEnrich []string, collectConfig, collectMetrics bool, result *ChannelCollectionResult) {
	for _, channelName := range channelsToEnrich {
		cm := ChannelMetrics{
			Name:                 channelName,
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

		if collectConfig {
			c.enrichChannelWithConfig(&cm, result)
		}

		if collectMetrics {
			c.enrichChannelWithMetrics(&cm, result)
		}

		result.Channels = append(result.Channels, cm)
	}
}

func (c *Client) enrichChannelsWithTypes(channelInfosToEnrich []ChannelInfo, collectConfig, collectMetrics bool, result *ChannelCollectionResult) {
	for _, info := range channelInfosToEnrich {
		cm := ChannelMetrics{
			Name:                 info.Name,
			Type:                 info.Type, // Set type from discovery
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

		if collectConfig {
			c.enrichChannelWithConfig(&cm, result)
		}

		if collectMetrics {
			c.enrichChannelWithMetrics(&cm, result)
		}

		result.Channels = append(result.Channels, cm)
	}
}

func (c *Client) enrichChannelWithConfig(cm *ChannelMetrics, result *ChannelCollectionResult) {
	if result.Stats.Config == nil {
		result.Stats.Config = &EnrichmentStats{
			TotalItems:  int64(len(result.Channels)),
			ErrorCounts: make(map[int32]int),
		}
	}

	configData, err := c.GetChannelConfig(cm.Name)
	if err != nil {
		result.Stats.Config.FailedItems++
		if pcfErr, ok := err.(*PCFError); ok {
			result.Stats.Config.ErrorCounts[pcfErr.Code]++
		} else {
			result.Stats.Config.ErrorCounts[-1]++
		}
		c.protocol.Debugf("Failed to get config for channel '%s': %v", cm.Name, err)
	} else {
		result.Stats.Config.OkItems++
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

func (c *Client) enrichChannelWithMetrics(cm *ChannelMetrics, result *ChannelCollectionResult) {
	if result.Stats.Metrics == nil {
		result.Stats.Metrics = &EnrichmentStats{
			TotalItems:  int64(len(result.Channels)),
			ErrorCounts: make(map[int32]int),
		}
	}

	metricsData, err := c.GetChannelMetrics(cm.Name)
	if err != nil {
		result.Stats.Metrics.FailedItems++
		if pcfErr, ok := err.(*PCFError); ok {
			result.Stats.Metrics.ErrorCounts[pcfErr.Code]++
		} else {
			result.Stats.Metrics.ErrorCounts[-1]++
		}
		c.protocol.Debugf("Failed to get metrics for channel '%s': %v", cm.Name, err)
	} else {
		result.Stats.Metrics.OkItems++
		cm.Status = metricsData.Status
		cm.Messages = metricsData.Messages
		cm.Bytes = metricsData.Bytes
		cm.Batches = metricsData.Batches
		cm.Connections = metricsData.Connections
		cm.BuffersUsed = metricsData.BuffersUsed
		cm.BuffersMax = metricsData.BuffersMax

		// Copy extended status metrics
		cm.BuffersSent = metricsData.BuffersSent
		cm.BuffersReceived = metricsData.BuffersReceived
		cm.CurrentMessages = metricsData.CurrentMessages
		cm.XmitQueueTime = metricsData.XmitQueueTime
		cm.MCAStatus = metricsData.MCAStatus
		cm.InDoubtStatus = metricsData.InDoubtStatus
		cm.SSLKeyResets = metricsData.SSLKeyResets
		cm.NPMSpeed = metricsData.NPMSpeed
		cm.CurrentSharingConvs = metricsData.CurrentSharingConvs
		cm.ConnectionName = metricsData.ConnectionName
	}
}

func (c *Client) logChannelCollectionSummary(result *ChannelCollectionResult) {
	fieldCounts := make(map[string]int)
	for _, ch := range result.Channels {
		fieldCounts["status"]++
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

	c.protocol.Debugf("Channel collection complete - discovered:%d visible:%d included:%d enriched:%d",
		result.Stats.Discovery.AvailableItems,
		result.Stats.Discovery.AvailableItems-result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Channels))

	c.protocol.Debugf("Channel field collection summary: status=%d messages=%d bytes=%d batches=%d connections=%d "+
		"buffers_used=%d buffers_max=%d batch_size=%d batch_interval=%d disc_interval=%d hb_interval=%d "+
		"keep_alive_interval=%d short_retry=%d long_retry=%d max_msg_length=%d sharing_conversations=%d network_priority=%d",
		fieldCounts["status"], fieldCounts["messages"], fieldCounts["bytes"], fieldCounts["batches"],
		fieldCounts["connections"], fieldCounts["buffers_used"], fieldCounts["buffers_max"],
		fieldCounts["batch_size"], fieldCounts["batch_interval"], fieldCounts["disc_interval"],
		fieldCounts["hb_interval"], fieldCounts["keep_alive_interval"], fieldCounts["short_retry"],
		fieldCounts["long_retry"], fieldCounts["max_msg_length"], fieldCounts["sharing_conversations"],
		fieldCounts["network_priority"])
}
