// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package mq_pcf

// #include <cmqc.h>
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
	// Queue Manager CPU and Memory constants (MQ 8.0+)
	MQIACF_Q_MGR_CPU_LOAD    = C.MQLONG(3024) // Queue Manager CPU load percentage
	MQIACF_Q_MGR_MEMORY_USAGE = C.MQLONG(3025) // Queue Manager memory usage in bytes
	MQIACF_Q_MGR_LOG_USAGE   = C.MQLONG(3026) // Queue Manager log usage percentage
	
	// Queue constants that may not be available in all MQ versions
	MQIA_OLDEST_MSG_AGE      = C.MQLONG(2163) // Oldest message age in seconds (MQ 8.0+)
	
	// Topic constants that may not be available in all MQ versions
	MQIA_TOPIC_MSG_COUNT     = C.MQLONG(2164) // Topic message count (MQ 8.0+)
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
	// MQCFST structure size + string length (padded to 4-byte boundary)
	strLen := len(p.value)
	paddedLen := (strLen + 3) & ^3 // Round up to multiple of 4
	return C.sizeof_MQCFST + C.size_t(paddedLen)
}

func (p *stringParameter) marshal(buffer unsafe.Pointer) {
	cfst := (*C.MQCFST)(buffer)
	cfst.Type = C.MQCFT_STRING
	cfst.StrucLength = C.MQLONG(p.size())
	cfst.Parameter = p.parameter
	cfst.CodedCharSetId = C.MQCCSI_DEFAULT
	cfst.StringLength = C.MQLONG(len(p.value))

	// Calculate the actual buffer size for the string data (must match size())
	paddedLen := (len(p.value) + 3) & ^3

	stringDataPtr := unsafe.Pointer(uintptr(buffer) + C.sizeof_MQCFST)

	// Convert Go string to byte slice
	goBytes := []byte(p.value)

	// Zero out the entire padded area to ensure proper termination and padding
	C.memset(stringDataPtr, 0, C.size_t(paddedLen))

	// Copy the actual string value bytes, ensuring we don't write beyond paddedLen
	if len(goBytes) > 0 {
		bytesToCopy := len(goBytes)
		if bytesToCopy > paddedLen {
			bytesToCopy = paddedLen // Truncate if Go string is too long for the C buffer
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

// Parse PCF response into a map of attributes
func (c *Collector) parsePCFResponse(response []byte) (map[C.MQLONG]interface{}, error) {
	if len(response) < int(C.sizeof_MQCFH) {
		return nil, fmt.Errorf("response too short for PCF header")
	}
	
	attrs := make(map[C.MQLONG]interface{})
	
	// Parse PCF header
	cfh := (*C.MQCFH)(unsafe.Pointer(&response[0]))
	
	if cfh.Type != C.MQCFT_RESPONSE {
		return nil, fmt.Errorf("unexpected PCF message type: %d", cfh.Type)
	}
	
	if cfh.CompCode != C.MQCC_OK {
		return nil, fmt.Errorf("PCF command failed: completion code %d, reason code %d", cfh.CompCode, cfh.Reason)
	}
	
	// Parse parameters
	offset := C.sizeof_MQCFH
	for i := 0; i < int(cfh.ParameterCount) && offset < len(response); i++ {
		paramType := *(*C.MQLONG)(unsafe.Pointer(&response[offset]))
		
		switch paramType {
		case C.MQCFT_INTEGER:
			if offset+int(C.sizeof_MQCFIN) > len(response) {
				break
			}
			cfin := (*C.MQCFIN)(unsafe.Pointer(&response[offset]))
			// Additional check for full structure size
			if offset+int(cfin.StrucLength) > len(response) {
				break
			}
			attrs[cfin.Parameter] = int32(cfin.Value)
			offset += int(cfin.StrucLength)
			
		case C.MQCFT_STRING:
			if offset+int(C.sizeof_MQCFST) > len(response) {
				break
			}
			cfst := (*C.MQCFST)(unsafe.Pointer(&response[offset]))
			// Check full structure size first
			if offset+int(cfst.StrucLength) > len(response) {
				break
			}
			// Additional check that string data doesn't exceed structure bounds
			if int(cfst.StringLength) > int(cfst.StrucLength)-int(C.sizeof_MQCFST) {
				break
			}
			
			// Extract string value
			stringData := (*[256]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(&response[offset])) + C.sizeof_MQCFST))
			value := string(stringData[:cfst.StringLength])
			attrs[cfst.Parameter] = strings.TrimSpace(value)
			offset += int(cfst.StrucLength)
			
		case C.MQCFT_INTEGER_LIST:
			if offset+int(C.sizeof_MQCFIL) > len(response) {
				break
			}
			cfil := (*C.MQCFIL)(unsafe.Pointer(&response[offset]))
			// Check full structure size
			if offset+int(cfil.StrucLength) > len(response) {
				break
			}
			// Handle integer lists if needed
			offset += int(cfil.StrucLength)
			
		default:
			// Skip unknown parameter types
			if offset+8 > len(response) { // Need at least 8 bytes for type + length
				break
			}
			strucLength := *(*C.MQLONG)(unsafe.Pointer(&response[offset+4]))
			// Validate structure length
			if strucLength <= 0 || offset+int(strucLength) > len(response) {
				break
			}
			offset += int(strucLength)
		}
	}
	
	return attrs, nil
}

// Parse queue list response
func (c *Collector) parseQueueListResponse(response []byte) ([]string, error) {
	var queues []string
	
	// Parse response in chunks (each queue gets its own response message)
	offset := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			break
		}
		
		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		messageEnd := offset + int(cfh.StrucLength)
		
		if messageEnd > len(response) {
			break
		}
		
		// Parse this message
		attrs, err := c.parsePCFResponse(response[offset:messageEnd])
		if err != nil {
			c.Warningf("failed to parse queue response: %v", err)
			offset = messageEnd
			continue
		}
		
		// Extract queue name
		if queueName, ok := attrs[C.MQCA_Q_NAME]; ok {
			if name, ok := queueName.(string); ok && name != "" {
				queues = append(queues, strings.TrimSpace(name))
			}
		}
		
		offset = messageEnd
	}
	
	return queues, nil
}

// Parse channel list response
func (c *Collector) parseChannelListResponse(response []byte) ([]string, error) {
	var channels []string
	
	// Parse response in chunks (each channel gets its own response message)
	offset := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			break
		}
		
		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		messageEnd := offset + int(cfh.StrucLength)
		
		if messageEnd > len(response) {
			break
		}
		
		// Parse this message
		attrs, err := c.parsePCFResponse(response[offset:messageEnd])
		if err != nil {
			c.Warningf("failed to parse channel response: %v", err)
			offset = messageEnd
			continue
		}
		
		// Extract channel name
		if channelName, ok := attrs[C.MQCACH_CHANNEL_NAME]; ok {
			if name, ok := channelName.(string); ok && name != "" {
				channels = append(channels, strings.TrimSpace(name))
			}
		}
		
		offset = messageEnd
	}
	
	return channels, nil
}

// Parse topic list response
func (c *Collector) parseTopicListResponse(response []byte) ([]string, error) {
	var topics []string
	
	// Parse response in chunks (each topic gets its own response message)
	offset := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			break
		}
		
		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		messageEnd := offset + int(cfh.StrucLength)
		
		if messageEnd > len(response) {
			break
		}
		
		// Parse this message
		attrs, err := c.parsePCFResponse(response[offset:messageEnd])
		if err != nil {
			c.Warningf("failed to parse topic response: %v", err)
			offset = messageEnd
			continue
		}
		
		// Extract topic name
		if topicName, ok := attrs[C.MQCA_TOPIC_NAME]; ok {
			if name, ok := topicName.(string); ok && name != "" {
				topics = append(topics, strings.TrimSpace(name))
			}
		}
		
		offset = messageEnd
	}
	
	return topics, nil
}