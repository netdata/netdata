// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo && ibm_mq
// +build cgo,ibm_mq

package pcf

import (
	"fmt"
	"strings"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)

// sendPCFCommand sends a PCF command using IBM library and returns the response parameters
func (c *Client) sendPCFCommand(command int32, params []*ibmmq.PCFParameter) ([]*ibmmq.PCFParameter, error) {
	// Handle nil params as empty slice
	if params == nil {
		params = []*ibmmq.PCFParameter{}
	}

	// Build PCF message using IBM library
	cfh := ibmmq.NewMQCFH()
	// cfh.Version defaults to MQCFH_VERSION_1 which is correct
	cfh.Type = ibmmq.MQCFT_COMMAND // Regular PCF command
	cfh.Command = command
	cfh.ParameterCount = int32(len(params))

	// Build parameter bytes first
	var paramBytes []byte
	for _, param := range params {
		paramBytes = append(paramBytes, param.Bytes()...)
	}

	// CRITICAL: Put CFH header at front of message (not after)
	msgData := append(cfh.Bytes(), paramBytes...)

	// Create message descriptor - CRITICAL: Format must be "MQADMIN" string
	md := ibmmq.NewMQMD()
	md.MsgType = ibmmq.MQMT_REQUEST
	md.Format = "MQADMIN" // CRITICAL: Must be string "MQADMIN", not constant
	md.ReplyToQ = c.replyQueueName
	md.ReplyToQMgr = c.config.QueueManager
	md.CodedCharSetId = 1208 // UTF-8 - CRITICAL: Required for PCF commands

	// Create put message options
	pmo := ibmmq.NewMQPMO()
	pmo.Options = ibmmq.MQPMO_NEW_MSG_ID | ibmmq.MQPMO_NEW_CORREL_ID

	// Put message to command queue
	err := c.cmdQueue.Put(md, pmo, msgData)
	if err != nil {
		return nil, fmt.Errorf("failed to put PCF command: %w", err)
	}

	// Log in structured format
	paramStr := ""
	for i, p := range params {
		if i > 0 {
			paramStr += ","
		}
		paramStr += fmt.Sprintf("%s(%d)", mqParameterToString(p.Parameter), p.Parameter)
	}
	c.protocol.Debugf("MQPUT %s(%d) params=[%s] msgId=%X bytes=%d",
		mqcmdToString(command), command, paramStr, md.MsgId, len(msgData))

	// Get reply - PCF replies use request's MsgId as their CorrelId
	return c.getPCFReply(md.MsgId)
}

// getPCFReply gets the PCF reply from the reply queue - handles multi-message responses
// The correlId parameter should be the request's MsgId (PCF standard behavior)
func (c *Client) getPCFReply(correlId []byte) ([]*ibmmq.PCFParameter, error) {
	// Create get message options - matching the Prometheus collector pattern
	gmo := ibmmq.NewMQGMO()
	gmo.Options = ibmmq.MQGMO_WAIT | ibmmq.MQGMO_CONVERT | ibmmq.MQGMO_FAIL_IF_QUIESCING
	gmo.WaitInterval = 5000                       // 5 seconds
	gmo.MatchOptions = ibmmq.MQMO_MATCH_CORREL_ID // CRITICAL: Must set match options

	var allParams []*ibmmq.PCFParameter
	bufferSize := 65536 // 64KB initial size

	// Loop to handle multi-message responses
	for {
		// Create new message descriptor for each get
		md := ibmmq.NewMQMD()
		// PCF replies use request's MsgId as their CorrelId
		copy(md.CorrelId[:], correlId)

		buffer := make([]byte, bufferSize)

		// Get the reply
		datalen, err := c.replyQueue.Get(md, gmo, buffer)
		if err != nil {
			// Check if buffer too small
			if mqErr, ok := err.(*ibmmq.MQReturn); ok && mqErr.MQRC == ibmmq.MQRC_DATA_LENGTH_ERROR {
				// Double the buffer size and retry with same message
				bufferSize *= 2
				c.protocol.Debugf("buffer too small, increasing to %d bytes and retrying", bufferSize)
				continue
			}
			return nil, fmt.Errorf("failed to get PCF reply: %w", err)
		}

		// Read PCF header first for logging
		tempCfh, _ := ibmmq.ReadPCFHeader(buffer[:datalen])
		if tempCfh != nil {
			// Convert codes to readable names for logging
			compCodeStr := ibmmq.MQItoString("CC", int(tempCfh.CompCode))
			reasonStr := mqReasonString(tempCfh.Reason)

			c.protocol.Debugf("MQGET Command=%s(%d) Type=%s CompCode=%s(%d) Reason=%s(%d) Params=%d Control=%s bytes=%d",
				mqcmdToString(tempCfh.Command), tempCfh.Command, mqPCFTypeToString(tempCfh.Type),
				compCodeStr, tempCfh.CompCode, reasonStr, tempCfh.Reason, tempCfh.ParameterCount, mqPCFControlToString(tempCfh.Control), datalen)
		}

		// Read PCF header to check control field
		cfh, _ := ibmmq.ReadPCFHeader(buffer[:datalen])
		if cfh == nil {
			return nil, fmt.Errorf("failed to read PCF header from response")
		}

		// Parse this message's parameters
		params, err := c.parsePCFResponseInternal(buffer[:datalen])
		if err != nil {
			// In multi-message responses, log the error but continue reading
			c.protocol.Warningf("Error in multi-message response: %v", err)

			// Check if this is the last message even with error
			if cfh.Control == ibmmq.MQCFC_LAST {
				c.protocol.Debugf("received last PCF message (with error), total parameters: %d", len(allParams))
				break
			}
			// Continue to next message
			c.protocol.Debugf("MQGET multi-message response detected, continuing despite error")
			continue
		}

		// Accumulate parameters from successful messages
		allParams = append(allParams, params...)

		// Check if this is the last message
		if cfh.Control == ibmmq.MQCFC_LAST {
			c.protocol.Debugf("received last PCF message, total parameters: %d", len(allParams))
			break
		}
		// Continue loop for MQCFC_NOT_LAST
		c.protocol.Debugf("MQGET multi-message response detected, continuing to read next message")
	}

	return allParams, nil
}

// readPCFParameters reads PCF parameters from data using IBM library
// This function is used when we know the expected parameter count from the CFH
func (c *Client) readPCFParameters(data []byte, count int) ([]*ibmmq.PCFParameter, error) {
	var params []*ibmmq.PCFParameter
	offset := 0

	for i := 0; i < count && offset < len(data); i++ {
		param, bytesRead := ibmmq.ReadPCFParameter(data[offset:])
		if param == nil || bytesRead == 0 {
			return nil, fmt.Errorf("invalid parameter %d at offset %d: nil parameter or zero bytes read", i, offset)
		}

		params = append(params, param)
		offset += bytesRead
	}

	c.protocol.Debugf("Read %d PCF parameters from %d bytes (expected %d parameters)", len(params), len(data), count)
	return params, nil
}

// convertPCFParametersToAttrs converts IBM PCFParameter array to the expected attrs format
// This preserves compatibility with existing smart function code
func convertPCFParametersToAttrs(params []*ibmmq.PCFParameter) map[int32]interface{} {
	attrs := make(map[int32]interface{})

	for _, param := range params {
		switch param.Type {
		case ibmmq.MQCFT_INTEGER:
			if len(param.Int64Value) > 0 {
				attrs[param.Parameter] = int32(param.Int64Value[0])
			}
		case ibmmq.MQCFT_INTEGER64:
			if len(param.Int64Value) > 0 {
				attrs[param.Parameter] = param.Int64Value[0]
			}
		case ibmmq.MQCFT_STRING:
			if len(param.String) > 0 {
				attrs[param.Parameter] = param.String[0]
			}
		case ibmmq.MQCFT_INTEGER_LIST:
			if len(param.Int64Value) > 0 {
				// Convert to int32 slice
				intList := make([]int32, len(param.Int64Value))
				for i, v := range param.Int64Value {
					intList[i] = int32(v)
				}
				attrs[param.Parameter] = intList
			}
		case ibmmq.MQCFT_STRING_LIST:
			if len(param.String) > 0 {
				attrs[param.Parameter] = param.String
			}
		}
	}

	return attrs
}

// parsePCFResponse provides compatibility bridge for smart functions
// This version takes []byte response data (used by list_parser.go)
func (c *Client) parsePCFResponse(data []byte, context string) (map[int32]interface{}, error) {
	// Parse the raw PCF response data into IBM library parameters
	params, err := c.parsePCFResponseInternal(data)
	if err != nil {
		return nil, err
	}

	// Convert to expected attrs format for smart functions
	attrs := convertPCFParametersToAttrs(params)
	return attrs, nil
}

// parsePCFResponseFromParams provides compatibility bridge for smart functions
// This version takes []*ibmmq.PCFParameter response data (from sendPCFCommand)
func (c *Client) parsePCFResponseFromParams(params []*ibmmq.PCFParameter, context string) (map[int32]interface{}, error) {
	// Convert IBM library parameters to expected attrs format for smart functions
	attrs := convertPCFParametersToAttrs(params)
	return attrs, nil
}

// parseChannelListResponseFromParams provides compatibility bridge for smart functions
// This version takes []*ibmmq.PCFParameter response data (from sendPCFCommand)
func (c *Client) parseChannelListResponseFromParams(params []*ibmmq.PCFParameter) *ChannelListResult {
	result := &ChannelListResult{
		Channels:      []string{},
		ErrorCounts:   make(map[int32]int),
		ErrorChannels: make(map[int32][]string),
	}

	// Convert each parameter set (assuming it's a multi-object response)
	for _, param := range params {
		if param.Type == ibmmq.MQCFT_STRING && param.Parameter == ibmmq.MQCACH_CHANNEL_NAME {
			if len(param.String) > 0 {
				channelName := strings.TrimSpace(param.String[0])
				if channelName != "" {
					result.Channels = append(result.Channels, channelName)
				}
			}
		}
	}

	return result
}

// parseChannelInfoFromParams extracts channel names and types from PCF response
func (c *Client) parseChannelInfoFromParams(params []*ibmmq.PCFParameter) []ChannelInfo {
	var channels []ChannelInfo
	var currentChannel *ChannelInfo

	// Process parameters - they come in groups per channel
	for _, param := range params {
		switch param.Parameter {
		case ibmmq.MQCACH_CHANNEL_NAME:
			// Start of new channel data
			if param.Type == ibmmq.MQCFT_STRING && len(param.String) > 0 {
				channelName := strings.TrimSpace(param.String[0])
				if channelName != "" {
					// Save previous channel if exists
					if currentChannel != nil {
						channels = append(channels, *currentChannel)
					}
					// Start new channel
					currentChannel = &ChannelInfo{
						Name: channelName,
						Type: 0, // Will be filled by MQIACH_CHANNEL_TYPE
					}
				}
			}
		case ibmmq.MQIACH_CHANNEL_TYPE:
			// Channel type for current channel
			if currentChannel != nil && param.Type == ibmmq.MQCFT_INTEGER && len(param.Int64Value) > 0 {
				currentChannel.Type = ChannelType(param.Int64Value[0])
			}
		}
	}

	// Don't forget the last channel
	if currentChannel != nil {
		channels = append(channels, *currentChannel)
	}

	return channels
}

// parseListenerListResponseFromParams provides compatibility bridge for listener discovery
// This version takes []*ibmmq.PCFParameter response data (from sendPCFCommand)
func (c *Client) parseListenerListResponseFromParams(params []*ibmmq.PCFParameter) *ListenerListResult {
	result := &ListenerListResult{
		Listeners:   []string{},
		ErrorCounts: make(map[int32]int),
	}

	// Convert each parameter set (assuming it's a multi-object response)
	for _, param := range params {
		if param.Type == ibmmq.MQCFT_STRING && param.Parameter == ibmmq.MQCACH_LISTENER_NAME {
			if len(param.String) > 0 {
				listenerName := strings.TrimSpace(param.String[0])
				if listenerName != "" {
					result.Listeners = append(result.Listeners, listenerName)
				}
			}
		}
	}

	return result
}

// parsePCFResponseInternal handles the actual parsing of PCF response data using IBM library
// This implements proper PCF message parsing using the official IBM MQ Go library API
func (c *Client) parsePCFResponseInternal(data []byte) ([]*ibmmq.PCFParameter, error) {
	var params []*ibmmq.PCFParameter

	if len(data) == 0 {
		return params, nil
	}

	// Read PCF header using IBM library function
	cfh, offset := ibmmq.ReadPCFHeader(data)
	if cfh == nil {
		return nil, fmt.Errorf("failed to read PCF header: invalid header")
	}

	// Check for MQ errors in the response
	if cfh.CompCode != ibmmq.MQCC_OK {
		// Convert codes to readable names
		compCodeStr := ibmmq.MQItoString("CC", int(cfh.CompCode))
		reasonStr := mqReasonString(cfh.Reason)

		return nil, &PCFError{
			Code: cfh.Reason,
			Message: fmt.Sprintf("MQ error in PCF response: CompCode=%s Reason=%s",
				compCodeStr, reasonStr),
		}
	}

	// Parse parameters using IBM library functions
	for i := int32(0); i < cfh.ParameterCount && offset < len(data); i++ {
		param, bytesRead := ibmmq.ReadPCFParameter(data[offset:])
		if param == nil || bytesRead == 0 {
			c.protocol.Warningf("MQMSG: invalid PCF parameter %d at offset %d: nil parameter or zero bytes read", i, offset)
			break
		}

		switch param.Type {
		case ibmmq.MQCFT_INTEGER:
			if len(param.Int64Value) > 0 {
				// Use PCFValueToString to get meaningful names for integer values
				valueStr := ibmmq.PCFValueToString(param.Parameter, param.Int64Value[0])
				c.protocol.Debugf("MQMSG #%d %s(%d) type=%s = %s (%d) offset=%d bytes=%d",
					i+1, mqParameterToString(param.Parameter), param.Parameter, mqPCFTypeToString(param.Type),
					valueStr, param.Int64Value[0], offset, bytesRead)
			}
		case ibmmq.MQCFT_STRING:
			if len(param.String) > 0 {
				c.protocol.Debugf("MQMSG #%d %s(%d) type=%s = '%s' offset=%d bytes=%d",
					i+1, mqParameterToString(param.Parameter), param.Parameter, mqPCFTypeToString(param.Type),
					strings.TrimSpace(param.String[0]), offset, bytesRead)
			}
		case ibmmq.MQCFT_INTEGER_LIST:
			c.protocol.Debugf("MQMSG #%d %s(%d) type=%s = [%d values] offset=%d bytes=%d",
				i+1, mqParameterToString(param.Parameter), param.Parameter, mqPCFTypeToString(param.Type),
				len(param.Int64Value), offset, bytesRead)
		case ibmmq.MQCFT_STRING_LIST:
			c.protocol.Debugf("MQMSG #%d %s(%d) type=%s = [%d strings] offset=%d bytes=%d",
				i+1, mqParameterToString(param.Parameter), param.Parameter, mqPCFTypeToString(param.Type),
				len(param.String), offset, bytesRead)
		case ibmmq.MQCFT_INTEGER64:
			if len(param.Int64Value) > 0 {
				// For 64-bit integers, still use PCFValueToString but note it might not have mappings
				valueStr := ibmmq.PCFValueToString(param.Parameter, param.Int64Value[0])
				c.protocol.Debugf("MQMSG #%d %s(%d) type=%s = %s (%d) offset=%d bytes=%d",
					i+1, mqParameterToString(param.Parameter), param.Parameter, mqPCFTypeToString(param.Type),
					valueStr, param.Int64Value[0], offset, bytesRead)
			}
		default:
			c.protocol.Debugf("MQMSG #%d %s(%d) type=%s offset=%d bytes=%d",
				i+1, mqParameterToString(param.Parameter), param.Parameter, mqPCFTypeToString(param.Type),
				offset, bytesRead)
		}

		params = append(params, param)
		offset += bytesRead
	}

	return params, nil
}

// Helper functions for building PCF parameters

// buildStringParameter creates a string PCF parameter
func buildStringParameter(attr int32, value string) *ibmmq.PCFParameter {
	param := &ibmmq.PCFParameter{
		Type:      ibmmq.MQCFT_STRING,
		Parameter: attr,
		String:    []string{value},
	}
	return param
}

// buildIntParameter creates an integer PCF parameter
func buildIntParameter(attr int32, value int32) *ibmmq.PCFParameter {
	param := &ibmmq.PCFParameter{
		Type:       ibmmq.MQCFT_INTEGER,
		Parameter:  attr,
		Int64Value: []int64{int64(value)},
	}
	return param
}

// buildStringListParameter creates a string list PCF parameter
func buildStringListParameter(attr int32, values []string) *ibmmq.PCFParameter {
	param := &ibmmq.PCFParameter{
		Type:      ibmmq.MQCFT_STRING_LIST,
		Parameter: attr,
		String:    values,
	}
	return param
}

// buildIntListParameter creates an integer list PCF parameter
func buildIntListParameter(attr int32, values []int32) *ibmmq.PCFParameter {
	int64Values := make([]int64, len(values))
	for i, v := range values {
		int64Values[i] = int64(v)
	}
	param := &ibmmq.PCFParameter{
		Type:       ibmmq.MQCFT_INTEGER_LIST,
		Parameter:  attr,
		Int64Value: int64Values,
	}
	return param
}
