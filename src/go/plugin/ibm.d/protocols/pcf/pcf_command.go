// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"unsafe"
)

// SendPCFCommand sends a PCF command and returns the response
func (c *Client) SendPCFCommand(command C.MQLONG, parameters []pcfParameter) ([]byte, error) {
	if !c.connected {
		c.protocol.Warningf("PCF: Cannot send command %s to queue manager '%s' - not connected", 
			mqcmdToString(command), c.config.QueueManager)
		return nil, fmt.Errorf("not connected")
	}
	
	c.protocol.Debugf("PCF: Sending command %s to queue manager '%s' with %d parameters", 
		mqcmdToString(command), c.config.QueueManager, len(parameters))

	var compCode C.MQLONG
	var reason C.MQLONG

	// Calculate message size
	msgSize := int(C.sizeof_MQCFH)
	for _, param := range parameters {
		msgSize += int(param.size())
	}

	// Allocate message buffer
	msgBuffer := C.malloc(C.size_t(msgSize))
	defer C.free(msgBuffer)

	// Build PCF header
	cfh := (*C.MQCFH)(msgBuffer)
	C.memset(unsafe.Pointer(cfh), 0, C.sizeof_MQCFH)
	cfh.Type = C.MQCFT_COMMAND
	cfh.StrucLength = C.MQCFH_STRUC_LENGTH
	cfh.Version = C.MQCFH_VERSION_1
	cfh.Command = command
	cfh.MsgSeqNumber = 1
	cfh.Control = C.MQCFC_LAST
	cfh.ParameterCount = C.MQLONG(len(parameters))

	// Add parameters
	offset := int(C.sizeof_MQCFH)
	for _, param := range parameters {
		param.marshal(unsafe.Pointer(uintptr(msgBuffer) + uintptr(offset)))
		offset += int(param.size())
	}

	// Send message
	var md C.MQMD
	C.memset(unsafe.Pointer(&md), 0, C.sizeof_MQMD)
	C.set_md_struc_id(&md)
	md.Version = C.MQMD_VERSION_1
	formatStr := C.CString("MQADMIN")
	defer C.free(unsafe.Pointer(formatStr))
	C.set_format(&md, formatStr)
	md.MsgType = C.MQMT_REQUEST
	md.Expiry = C.MQEI_UNLIMITED // No expiry
	md.Encoding = C.MQENC_NATIVE
	md.CodedCharSetId = 1208 // UTF-8 - Netdata is UTF-8 everywhere
	md.Priority = C.MQPRI_PRIORITY_AS_Q_DEF
	md.Persistence = C.MQPER_NOT_PERSISTENT

	// Use the persistent reply queue name
	C.memcpy(unsafe.Pointer(&md.ReplyToQ), unsafe.Pointer(&c.replyQueueName), 48)

	var pmo C.MQPMO
	C.memset(unsafe.Pointer(&pmo), 0, C.sizeof_MQPMO)
	C.set_pmo_struc_id(&pmo)
	pmo.Version = C.MQPMO_VERSION_1
	pmo.Options = C.MQPMO_NO_SYNCPOINT | C.MQPMO_FAIL_IF_QUIESCING

	c.protocol.Debugf("PCF: Executing MQPUT for command %s to queue manager '%s' (message size: %d bytes)", 
		mqcmdToString(command), c.config.QueueManager, msgSize)
	C.MQPUT(c.hConn, c.hObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&pmo)), C.MQLONG(msgSize), C.PMQVOID(msgBuffer), &compCode, &reason)

	if compCode != C.MQCC_OK {
		c.protocol.Errorf("PCF: MQPUT failed for command %s to queue manager '%s' - completion code %d, reason %d (%s)", 
			mqcmdToString(command), c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		if reason == C.MQRC_HCONN_ERROR {
			c.connected = false
			c.protocol.Warningf("PCF: Connection lost to queue manager '%s' - marking as disconnected", c.config.QueueManager)
		}
		return nil, fmt.Errorf("MQPUT failed for %s: completion code %d, reason %d (%s)", mqcmdToString(command), compCode, reason, mqReasonString(int32(reason)))
	}
	c.protocol.Debugf("PCF: MQPUT successful for command %s to queue manager '%s'", mqcmdToString(command), c.config.QueueManager)

	// Get response from persistent reply queue
	response, err := c.getPCFResponse(&md, c.hReplyObj)

	return response, err
}

func (c *Client) getPCFResponse(requestMd *C.MQMD, hReplyObj C.MQHOBJ) ([]byte, error) {
	var compCode C.MQLONG
	var reason C.MQLONG
	var allResponses []byte
	messageCount := 0

	c.protocol.Debugf("PCF: Waiting for response from queue manager '%s'", c.config.QueueManager)

	// For wildcard queries, MQ may return multiple response messages
	for {
		messageCount++
		// Get message
		var md C.MQMD
		C.memset(unsafe.Pointer(&md), 0, C.sizeof_MQMD)
		C.set_md_struc_id(&md)
		md.Version = C.MQMD_VERSION_1
		C.copy_msg_id(&md, requestMd)

		var gmo C.MQGMO
		C.memset(unsafe.Pointer(&gmo), 0, C.sizeof_MQGMO)
		C.set_gmo_struc_id(&gmo)
		gmo.Version = C.MQGMO_VERSION_1
		gmo.Options = C.MQGMO_WAIT | C.MQGMO_FAIL_IF_QUIESCING
		gmo.WaitInterval = 1000 // 1 second timeout for subsequent messages

		// Get message length first
		var bufferLength C.MQLONG = 0
		c.protocol.Debugf("PCF: Attempting MQGET (message #%d) from queue manager '%s' (wait interval: %dms)", 
			messageCount, c.config.QueueManager, gmo.WaitInterval)
		C.MQGET(c.hConn, hReplyObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), bufferLength, nil, &bufferLength, &compCode, &reason)

		if reason == C.MQRC_NO_MSG_AVAILABLE {
			// No more messages, return what we have
			c.protocol.Debugf("PCF: No more messages available from queue manager '%s' (collected %d messages)", 
				c.config.QueueManager, messageCount-1)
			break
		}

		if reason != C.MQRC_TRUNCATED_MSG_FAILED {
			if reason == C.MQRC_HCONN_ERROR {
				c.connected = false
				c.protocol.Warningf("PCF: Connection lost to queue manager '%s' during MQGET - marking as disconnected", c.config.QueueManager)
			}
			if len(allResponses) == 0 {
				c.protocol.Errorf("PCF: MQGET length check failed for queue manager '%s' - completion code %d, reason %d (%s)", 
					c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
				return nil, fmt.Errorf("MQGET length check failed: completion code %d, reason %d (%s)", compCode, reason, mqReasonString(int32(reason)))
			}
			c.protocol.Warningf("PCF: MQGET failed for subsequent message from queue manager '%s' - completion code %d, reason %d (%s)", 
				c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
			break
		}

		if bufferLength > maxMQGetBufferSize {
			c.protocol.Errorf("PCF: Response from queue manager '%s' too large (%d bytes), maximum allowed is %d bytes", 
				c.config.QueueManager, bufferLength, maxMQGetBufferSize)
			return nil, fmt.Errorf("PCF response too large (%d bytes), maximum allowed is %d bytes", bufferLength, maxMQGetBufferSize)
		}

		c.protocol.Debugf("PCF: Response message size is %d bytes, allocating buffer", bufferLength)

		// Allocate buffer and get actual message
		buffer := make([]byte, bufferLength)
		C.MQGET(c.hConn, hReplyObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), bufferLength, C.PMQVOID(unsafe.Pointer(&buffer[0])), &bufferLength, &compCode, &reason)

		if compCode != C.MQCC_OK {
			if reason == C.MQRC_HCONN_ERROR {
				c.connected = false
				c.protocol.Warningf("PCF: Connection lost to queue manager '%s' during MQGET data - marking as disconnected", c.config.QueueManager)
			}
			if len(allResponses) == 0 {
				c.protocol.Errorf("PCF: MQGET data failed for queue manager '%s' - completion code %d, reason %d (%s)", 
					c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
				return nil, fmt.Errorf("MQGET failed: completion code %d, reason %d (%s)", compCode, reason, mqReasonString(int32(reason)))
			}
			c.protocol.Warningf("PCF: MQGET data failed for subsequent message from queue manager '%s' - completion code %d, reason %d (%s)", 
				c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
			break
		}
		
		c.protocol.Debugf("PCF: Successfully received message #%d (%d bytes) from queue manager '%s'", 
			messageCount, bufferLength, c.config.QueueManager)

		cfh := (*C.MQCFH)(unsafe.Pointer(&buffer[0]))

		allResponses = append(allResponses, buffer[:bufferLength]...)
		
		// Check if this is the last message
		if cfh.Control == C.MQCFC_LAST {
			break
		}
		
		// For subsequent messages, use a shorter timeout
		gmo.WaitInterval = 100 // 100ms for continuation messages
	}
	
	if len(allResponses) == 0 {
		c.protocol.Errorf("PCF: No response received from queue manager '%s' after waiting", c.config.QueueManager)
		return nil, fmt.Errorf("no PCF response received")
	}
	
	c.protocol.Debugf("PCF: Successfully collected %d total bytes of response data from queue manager '%s'", 
		len(allResponses), c.config.QueueManager)
	
	return allResponses, nil
}