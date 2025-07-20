// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"


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