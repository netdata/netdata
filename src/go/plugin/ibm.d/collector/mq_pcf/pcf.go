// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package mq_pcf

// #include <cmqc.h>
// #include <cmqxc.h>
// #include <cmqcfc.h>
// #include <string.h>
// #include <stdlib.h>
import "C"


import (
	"fmt"
	"strings"
	"unsafe"
)

// PCF constants that may not be available in all MQ versions
// These constants are defined here to avoid CGO compilation errors
const (
	// PCF command constants for name discovery
	MQCMD_INQUIRE_Q_NAMES       = C.MQLONG(18) // Queue name discovery command
	MQCMD_INQUIRE_CHANNEL_NAMES = C.MQLONG(20) // Channel name discovery command
	
	// PCF parameter constants for name queries
	MQCACF_Q_NAMES              = C.MQLONG(2065) // Queue names parameter
	MQCACF_CHANNEL_NAMES        = C.MQLONG(2066) // Channel names parameter
	MQCACF_SENDER_CHANNEL_NAMES = C.MQLONG(3019) // Sender channel names parameter
	MQCACF_SERVER_CHANNEL_NAMES = C.MQLONG(3020) // Server channel names parameter
	
	// PCF attribute selector constants
	MQIACF_Q_ATTRS              = C.MQLONG(1002) // Queue attributes selector
	MQIACF_CHANNEL_ATTRS        = C.MQLONG(1015) // Channel attributes selector
	
	// Queue Manager CPU and Memory constants (MQ 8.0+)
	MQIACF_Q_MGR_CPU_LOAD    = C.MQLONG(3024) // Queue Manager CPU load percentage
	MQIACF_Q_MGR_MEMORY_USAGE = C.MQLONG(3025) // Queue Manager memory usage in bytes
	MQIACF_Q_MGR_LOG_USAGE   = C.MQLONG(3026) // Queue Manager log usage percentage
	
	// Queue constants that may not be available in all MQ versions
	MQIA_OLDEST_MSG_AGE      = C.MQLONG(2163) // Oldest message age in seconds (MQ 8.0+)
	
	// Topic constants that may not be available in all MQ versions
	MQIA_TOPIC_MSG_COUNT     = C.MQLONG(2164) // Topic message count (MQ 8.0+)
	
	// Channel configuration constants - using C header values
	// Channel batch and timing settings
	
	// Channel runtime constants - using C header values
	// Runtime metrics we can get from MQCMD_INQUIRE_CHANNEL_STATUS
	
	// Queue configuration constants - using C header values  
	// Basic queue configuration available via MQCMD_INQUIRE_Q
)

// PCF parameter interface
type pcfParameter interface {
	size() C.size_t
	marshal(buffer unsafe.Pointer)
}

// String parameter
type stringParameter struct {
	parameter C.MQLONG
	value     string
}

func newStringParameter(param C.MQLONG, value string) pcfParameter {
	return &stringParameter{
		parameter: param,
		value:     value,
	}
}

func (p *stringParameter) size() C.size_t {
	// MQCFST structure size calculation matches C: sizeof(MQCFST) - sizeof(MQCHAR) + string length
	// The MQCFST struct already includes space for one MQCHAR, so we subtract it
	strLen := len(p.value)
	
	// Check if this is an MQ object name parameter
	if p.parameter == C.MQCA_Q_NAME || p.parameter == C.MQCACH_CHANNEL_NAME {
		strLen = 48
	}
	
	paddedLen := (strLen + 3) & ^3 // Round up to multiple of 4
	return C.sizeof_MQCFST - C.sizeof_MQCHAR + C.size_t(paddedLen)
}

func (p *stringParameter) marshal(buffer unsafe.Pointer) {
	cfst := (*C.MQCFST)(buffer)
	cfst.Type = C.MQCFT_STRING
	cfst.StrucLength = C.MQLONG(p.size())
	cfst.Parameter = p.parameter
	cfst.CodedCharSetId = C.MQCCSI_DEFAULT
	
	// For MQ object names, we need special handling
	strLen := len(p.value)
	isObjectName := p.parameter == C.MQCA_Q_NAME || p.parameter == C.MQCACH_CHANNEL_NAME
	
	// StringLength is always the actual string length, not the padded length
	cfst.StringLength = C.MQLONG(len(p.value))
	
	if isObjectName {
		// MQ object names are padded to 48 characters
		strLen = 48
	}

	// Calculate the actual buffer size for the string data (must match size())
	paddedLen := (strLen + 3) & ^3

	// The String field is part of MQCFST structure, positioned after the fixed fields
	// Calculate offset: Type(4) + StrucLength(4) + Parameter(4) + CodedCharSetId(4) + StringLength(4) = 20 bytes
	stringDataPtr := unsafe.Pointer(uintptr(buffer) + 20)

	// For MQ object names, fill with spaces first
	if isObjectName {
		C.memset(stringDataPtr, ' ', C.size_t(paddedLen))
	} else {
		// Zero out the entire padded area to ensure proper termination and padding
		C.memset(stringDataPtr, 0, C.size_t(paddedLen))
	}

	// Convert Go string to byte slice
	goBytes := []byte(p.value)

	// Copy the actual string value bytes
	if len(goBytes) > 0 {
		bytesToCopy := len(goBytes)
		if bytesToCopy > strLen {
			bytesToCopy = strLen // Truncate if Go string is too long
		}
		C.memcpy(stringDataPtr, unsafe.Pointer(&goBytes[0]), C.size_t(bytesToCopy))
	}
}

// String filter parameter
type stringFilterParameter struct {
	parameter C.MQLONG
	value     string
	operator  C.MQLONG
}

func newStringFilterParameter(param C.MQLONG, value string, operator C.MQLONG) pcfParameter {
	return &stringFilterParameter{
		parameter: param,
		value:     value,
		operator:  operator,
	}
}

func (p *stringFilterParameter) size() C.size_t {
	// MQCFSF structure size calculation matches C: sizeof(MQCFSF) - sizeof(MQCHAR) + string length
	// The MQCFSF struct already includes space for one MQCHAR, so we subtract it
	strLen := len(p.value)
	
	// Check if this is an MQ object name parameter
	if p.parameter == C.MQCA_Q_NAME || p.parameter == C.MQCACH_CHANNEL_NAME {
		strLen = 48
	}
	
	paddedLen := (strLen + 3) & ^3 // Round up to multiple of 4
	return C.sizeof_MQCFSF - C.sizeof_MQCHAR + C.size_t(paddedLen)
}

func (p *stringFilterParameter) marshal(buffer unsafe.Pointer) {
	cfsf := (*C.MQCFSF)(buffer)
	cfsf.Type = C.MQCFT_STRING_FILTER
	cfsf.StrucLength = C.MQLONG(p.size())
	cfsf.Parameter = p.parameter
	cfsf.CodedCharSetId = C.MQCCSI_DEFAULT
	cfsf.Operator = p.operator
	
	// For MQ object names, we need special handling
	strLen := len(p.value)
	isObjectName := p.parameter == C.MQCA_Q_NAME || p.parameter == C.MQCACH_CHANNEL_NAME
	
	// Calculate the actual buffer size for the string data (must match size())
	paddedLen := (strLen + 3) & ^3

	// The String field is part of MQCFSF structure, positioned after the fixed fields
	// MQCFSF has one extra field (Operator) compared to MQCFST, so offset is 24 bytes
	stringDataPtr := unsafe.Pointer(uintptr(buffer) + 24)

	// For MQ object names, fill with spaces first
	if isObjectName {
		C.memset(stringDataPtr, ' ', C.size_t(paddedLen))
	} else {
		// Zero out the entire padded area to ensure proper termination and padding
		C.memset(stringDataPtr, 0, C.size_t(paddedLen))
	}

	// Convert Go string to byte slice
	goBytes := []byte(p.value)

	// Copy the actual string value bytes
	if len(goBytes) > 0 {
		bytesToCopy := len(goBytes)
		if bytesToCopy > strLen {
			bytesToCopy = strLen // Truncate if Go string is too long
		}
		C.memcpy(stringDataPtr, unsafe.Pointer(&goBytes[0]), C.size_t(bytesToCopy))
	}
}

// Integer parameter
type intParameter struct {
	parameter C.MQLONG
	value     int32
}

func newIntParameter(param C.MQLONG, value int32) pcfParameter {
	return &intParameter{
		parameter: param,
		value:     value,
	}
}

func (p *intParameter) size() C.size_t {
	return C.sizeof_MQCFIN
}

func (p *intParameter) marshal(buffer unsafe.Pointer) {
	cfin := (*C.MQCFIN)(buffer)
	cfin.Type = C.MQCFT_INTEGER
	cfin.StrucLength = C.MQCFIN_STRUC_LENGTH
	cfin.Parameter = p.parameter
	cfin.Value = C.MQLONG(p.value)
}

// Integer list parameter
type intListParameter struct {
	parameter C.MQLONG
	values    []int32
}

func newIntListParameter(param C.MQLONG, values []int32) pcfParameter {
	return &intListParameter{
		parameter: param,
		values:    values,
	}
}

func (p *intListParameter) size() C.size_t {
	// MQCFIL fixed part (16 bytes) + integer values
	return 16 + C.size_t(len(p.values)*4)
}

func (p *intListParameter) marshal(buffer unsafe.Pointer) {
	cfil := (*C.MQCFIL)(buffer)
	cfil.Type = C.MQCFT_INTEGER_LIST
	cfil.StrucLength = C.MQLONG(p.size())
	cfil.Parameter = p.parameter
	cfil.Count = C.MQLONG(len(p.values))
	
	// Copy integer values - start after the fixed 16-byte header
	if len(p.values) > 0 {
		valuesPtr := (*C.MQLONG)(unsafe.Pointer(uintptr(buffer) + 16))
		for i, val := range p.values {
			*(*C.MQLONG)(unsafe.Pointer(uintptr(unsafe.Pointer(valuesPtr)) + uintptr(i*4))) = C.MQLONG(val)
		}
	}
}

// Parse PCF response into a map of attributes
func (c *Collector) parsePCFResponse(response []byte, command string) (map[C.MQLONG]interface{}, error) {
	if len(response) < int(C.sizeof_MQCFH) {
		return nil, fmt.Errorf("response too short for PCF header")
	}
	
	attrs := make(map[C.MQLONG]interface{})
	
	// Parse PCF header
	cfh := (*C.MQCFH)(unsafe.Pointer(&response[0]))
	
	// Only log successful responses or responses with parameters to debug
	// Commented out - too verbose even for successful responses
	// if cfh.CompCode == C.MQCC_OK || cfh.ParameterCount > 0 {
	// 	c.Debugf("parsePCFResponse: Type=%d, CompCode=%d, Reason=%d, ParameterCount=%d", 
	// 		cfh.Type, cfh.CompCode, cfh.Reason, cfh.ParameterCount)
	// }
	
	if cfh.Type != C.MQCFT_RESPONSE {
		return nil, fmt.Errorf("unexpected PCF message type: %d", cfh.Type)
	}
	
	// Store the completion code and reason in the attributes for the caller to check
	attrs[C.MQIACF_COMP_CODE] = int32(cfh.CompCode)
	attrs[C.MQIACF_REASON_CODE] = int32(cfh.Reason)
	
	// Track PCF command execution
	if command != "" {
		c.pcfTracker.trackRequest(command, int32(cfh.CompCode), int32(cfh.Reason))
	}
	
	// If no parameters, just return the attrs with error codes
	if cfh.ParameterCount == 0 {
		return attrs, nil
	}
	
	// Parse parameters
	offset := C.sizeof_MQCFH
	for i := 0; i < int(cfh.ParameterCount) && offset < len(response); i++ {
		paramType := *(*C.MQLONG)(unsafe.Pointer(&response[offset]))
		
		switch paramType {
		case C.MQCFT_INTEGER:
			if offset+int(C.sizeof_MQCFIN) > len(response) {
				c.Debugf("Not enough space for MQCFIN at offset %d", offset)
				return attrs, nil // Return what we have so far
			}
			cfin := (*C.MQCFIN)(unsafe.Pointer(&response[offset]))
			// Additional check for full structure size
			if offset+int(cfin.StrucLength) > len(response) {
				c.Debugf("MQCFIN structure extends beyond response at offset %d", offset)
				return attrs, nil // Return what we have so far
			}
			attrs[cfin.Parameter] = int32(cfin.Value)
			offset += int(cfin.StrucLength)
			
		case C.MQCFT_STRING:
			if offset+int(C.sizeof_MQCFST) > len(response) {
				c.Debugf("Not enough space for MQCFST at offset %d", offset)
				return attrs, nil
			}
			cfst := (*C.MQCFST)(unsafe.Pointer(&response[offset]))
			// Check full structure size first
			if offset+int(cfst.StrucLength) > len(response) {
				c.Debugf("MQCFST structure extends beyond response at offset %d", offset)
				return attrs, nil
			}
			// Log parameter details for debugging
			// c.Debugf("MQCFST: StrucLength=%d, StringLength=%d, Parameter=%d", 
			// 	cfst.StrucLength, cfst.StringLength, cfst.Parameter)
			
			// Extract string value using a slice for robustness
			// String data starts at offset 20 within MQCFST (after the fixed header fields)
			stringDataStart := offset + 20
			stringDataEnd := stringDataStart + int(cfst.StringLength)
			if stringDataEnd > len(response) {
				// This should ideally be caught by the StrucLength check, but as a safeguard
				c.Debugf("String data extends beyond response at offset %d", offset)
				return attrs, nil
			}
			value := string(response[stringDataStart:stringDataEnd])
			trimmedValue := strings.TrimSpace(value)
			attrs[cfst.Parameter] = trimmedValue
			offset += int(cfst.StrucLength)
			
		case C.MQCFT_INTEGER_LIST:
			if offset+int(C.sizeof_MQCFIL) > len(response) {
				c.Debugf("Not enough space for MQCFIL at offset %d", offset)
				return attrs, nil
			}
			cfil := (*C.MQCFIL)(unsafe.Pointer(&response[offset]))
			// Check full structure size
			if offset+int(cfil.StrucLength) > len(response) {
				c.Debugf("MQCFIL structure extends beyond response at offset %d", offset)
				return attrs, nil
			}
			// Handle integer lists if needed
			offset += int(cfil.StrucLength)
			
		default:
			// Skip unknown parameter types
			if offset+8 > len(response) { // Need at least 8 bytes for type + length
				c.Debugf("Not enough space for parameter header at offset %d", offset)
				return attrs, nil
			}
			strucLength := *(*C.MQLONG)(unsafe.Pointer(&response[offset+4]))
			// Validate structure length
			if strucLength <= 0 || offset+int(strucLength) > len(response) {
				c.Debugf("Invalid structure length %d at offset %d", strucLength, offset)
				return attrs, nil
			}
			offset += int(strucLength)
		}
	}
	
	return attrs, nil
}


// ChannelParseResult contains the results of parsing channel list response
type ChannelParseResult struct {
	Channels       []string           // Successfully retrieved channel names
	ErrorCounts    map[int32]int      // MQ error code -> count
	ErrorChannels  map[int32][]string // MQ error code -> channel names that failed
	InternalErrors int                // Count of parsing/internal errors
}

// Internal error codes (negative to distinguish from MQ codes)
const (
	ErrInternalParsing = -1  // Failed to parse PCF message
	ErrInternalShort   = -2  // Response too short
	ErrInternalCorrupt = -3  // Corrupted message structure
)

// mqErrorString returns a human-readable description for MQ error codes
func mqErrorString(code int32) string {
	switch code {
	case 2035:
		return "MQRC_NOT_AUTHORIZED"
	case 2067:
		return "MQRC_OBJECT_OPEN_ERROR"
	case 2085:
		return "MQRC_UNKNOWN_OBJECT_NAME"
	case 3008:
		return "MQRCCF_COMMAND_FAILED"
	case 3010:
		return "MQRCCF_CFH_PARM_ID_ERROR"
	case 3065:
		return "MQRCCF_CHANNEL_NOT_ACTIVE"
	case ErrInternalParsing:
		return "INTERNAL_PARSE_ERROR"
	case ErrInternalShort:
		return "INTERNAL_RESPONSE_TOO_SHORT"
	case ErrInternalCorrupt:
		return "INTERNAL_CORRUPT_MESSAGE"
	default:
		return fmt.Sprintf("MQ_ERROR_%d", code)
	}
}

// Parse channel list response
func (c *Collector) parseChannelListResponse(response []byte, command string) *ChannelParseResult {
	result := &ChannelParseResult{
		Channels:      make([]string, 0),
		ErrorCounts:   make(map[int32]int),
		ErrorChannels: make(map[int32][]string),
	}
	
	// Parse response in chunks (each channel gets its own response message)
	offset := 0
	itemNumber := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalShort]++
			break
		}
		
		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		// Calculate the full message size by walking through all parameters
		messageSize := int(C.sizeof_MQCFH)
		paramOffset := offset + int(C.sizeof_MQCFH)
		
		for i := 0; i < int(cfh.ParameterCount) && paramOffset < len(response); i++ {
			if paramOffset+8 > len(response) { // Need at least type + length
				break
			}
			paramLength := *(*C.MQLONG)(unsafe.Pointer(&response[paramOffset+4]))
			if paramLength <= 0 || paramOffset+int(paramLength) > len(response) {
				break
			}
			messageSize += int(paramLength)
			paramOffset += int(paramLength)
		}
		
		messageEnd := offset + messageSize
		
		if messageEnd > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalCorrupt]++
			break
		}
		
		// Increment item number for this array element
		itemNumber++
		
		// Parse this message
		attrs, err := c.parsePCFResponse(response[offset:messageEnd], "")
		if err != nil {
			// This is a real parsing error
			result.InternalErrors++
			result.ErrorCounts[ErrInternalParsing]++
			offset = messageEnd
			continue
		}
		
		// Check for MQ errors and track array item
		var mqError int32
		var compCode int32
		if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
			if reason, ok := reasonCode.(int32); ok {
				mqError = reason
				if reason != 0 {
					result.ErrorCounts[mqError]++
				}
			}
		}
		if compCodeVal, ok := attrs[C.MQIACF_COMP_CODE]; ok {
			if code, ok := compCodeVal.(int32); ok {
				compCode = code
			}
		}
		
		// Track array item
		if command != "" {
			c.pcfTracker.trackArrayItem(command, compCode, mqError)
		}
		
		// Extract channel name
		channelName := ""
		if name, ok := attrs[C.MQCACH_CHANNEL_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				channelName = strings.TrimSpace(nameStr)
			}
		}
		
		// Log individual array item failures
		if mqError != 0 && command != "" {
			objectName := "NO_NAME"
			if channelName != "" {
				objectName = channelName
			}
			c.Debugf("Failed PCF response to %s array item %d, reason %d (%s), on channel %s",
				command, itemNumber, mqError, mqErrorString(mqError), objectName)
		}
		
		// Store result based on error status
		if mqError != 0 {
			// Channel had an error
			if channelName != "" {
				result.ErrorChannels[mqError] = append(result.ErrorChannels[mqError], channelName)
			}
		} else if channelName != "" {
			// Successful channel
			result.Channels = append(result.Channels, channelName)
		}
		
		offset = messageEnd
	}
	
	return result
}

// QueueParseResult contains the results of parsing queue list response
type QueueParseResult struct {
	Queues         []string           // Successfully retrieved queue names
	ErrorCounts    map[int32]int      // MQ error code -> count
	ErrorQueues    map[int32][]string // MQ error code -> queue names that failed
	InternalErrors int                // Count of parsing/internal errors
}

// Parse queue list response
func (c *Collector) parseQueueListResponse(response []byte, command string) *QueueParseResult {
	result := &QueueParseResult{
		Queues:       make([]string, 0),
		ErrorCounts:  make(map[int32]int),
		ErrorQueues:  make(map[int32][]string),
	}
	
	// Parse response in chunks (each queue gets its own response message)
	offset := 0
	itemNumber := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalShort]++
			break
		}
		
		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		// Calculate the full message size by walking through all parameters
		messageSize := int(C.sizeof_MQCFH)
		paramOffset := offset + int(C.sizeof_MQCFH)
		
		for i := 0; i < int(cfh.ParameterCount) && paramOffset < len(response); i++ {
			if paramOffset+8 > len(response) { // Need at least type + length
				break
			}
			paramLength := *(*C.MQLONG)(unsafe.Pointer(&response[paramOffset+4]))
			if paramLength <= 0 || paramOffset+int(paramLength) > len(response) {
				break
			}
			messageSize += int(paramLength)
			paramOffset += int(paramLength)
		}
		
		messageEnd := offset + messageSize
		
		if messageEnd > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalCorrupt]++
			break
		}
		
		// Increment item number for this array element
		itemNumber++
		
		// Parse this message
		attrs, err := c.parsePCFResponse(response[offset:messageEnd], "")
		if err != nil {
			// This is a real parsing error
			result.InternalErrors++
			result.ErrorCounts[ErrInternalParsing]++
			offset = messageEnd
			continue
		}
		
		// Check for MQ errors and track array item
		var mqError int32
		var compCode int32
		if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
			if reason, ok := reasonCode.(int32); ok {
				mqError = reason
				if reason != 0 {
					result.ErrorCounts[mqError]++
				}
			}
		}
		if compCodeVal, ok := attrs[C.MQIACF_COMP_CODE]; ok {
			if code, ok := compCodeVal.(int32); ok {
				compCode = code
			}
		}
		
		// Track array item
		if command != "" {
			c.pcfTracker.trackArrayItem(command, compCode, mqError)
		}
		
		// Extract queue name
		queueName := ""
		if name, ok := attrs[C.MQCA_Q_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				queueName = strings.TrimSpace(nameStr)
			}
		}
		
		// Log individual array item failures
		if mqError != 0 && command != "" {
			objectName := "NO_NAME"
			if queueName != "" {
				objectName = queueName
			}
			c.Debugf("Failed PCF response to %s array item %d, reason %d (%s), on queue %s",
				command, itemNumber, mqError, mqErrorString(mqError), objectName)
		}
		
		// Store result based on error status
		if mqError != 0 {
			// Queue had an error
			if queueName != "" {
				result.ErrorQueues[mqError] = append(result.ErrorQueues[mqError], queueName)
			}
		} else if queueName != "" {
			// Successful queue
			result.Queues = append(result.Queues, queueName)
		}
		
		offset = messageEnd
	}
	
	return result
}

// Parse topic list response
func (c *Collector) parseTopicListResponse(response []byte, command string) ([]string, error) {
	var topics []string
	
	// Parse response in chunks (each topic gets its own response message)
	offset := 0
	itemNumber := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			break
		}
		
		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		// Calculate the full message size by walking through all parameters
		messageSize := int(C.sizeof_MQCFH)
		paramOffset := offset + int(C.sizeof_MQCFH)
		
		for i := 0; i < int(cfh.ParameterCount) && paramOffset < len(response); i++ {
			if paramOffset+8 > len(response) { // Need at least type + length
				break
			}
			paramLength := *(*C.MQLONG)(unsafe.Pointer(&response[paramOffset+4]))
			if paramLength <= 0 || paramOffset+int(paramLength) > len(response) {
				break
			}
			messageSize += int(paramLength)
			paramOffset += int(paramLength)
		}
		
		messageEnd := offset + messageSize
		
		if messageEnd > len(response) {
			break
		}
		
		// Increment item number for this array element
		itemNumber++
		
		// Parse this message
		attrs, err := c.parsePCFResponse(response[offset:messageEnd], command)
		if err != nil {
			c.Warningf("failed to parse topic response: %v", err)
			offset = messageEnd
			continue
		}
		
		// Track array item
		var mqError int32
		var compCode int32
		var topicNameStr string
		
		if command != "" {
			if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
				if reason, ok := reasonCode.(int32); ok {
					mqError = reason
				}
			}
			if compCodeVal, ok := attrs[C.MQIACF_COMP_CODE]; ok {
				if code, ok := compCodeVal.(int32); ok {
					compCode = code
				}
			}
			c.pcfTracker.trackArrayItem(command, compCode, mqError)
		}
		
		// Extract topic name
		if topicName, ok := attrs[C.MQCA_TOPIC_NAME]; ok {
			if name, ok := topicName.(string); ok && name != "" {
				topicNameStr = strings.TrimSpace(name)
				topics = append(topics, topicNameStr)
			}
		}
		
		// Log individual array item failures
		if mqError != 0 && command != "" {
			objectName := "NO_NAME"
			if topicNameStr != "" {
				objectName = topicNameStr
			}
			c.Debugf("Failed PCF response to %s array item %d, reason %d (%s), on topic %s",
				command, itemNumber, mqError, mqErrorString(mqError), objectName)
		}
		
		offset = messageEnd
	}
	
	return topics, nil
}