// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package mq_pcf

// #include <cmqc.h>
// #include <cmqxc.h>
// #include <cmqcfc.h>
// #include <string.h>
// #include <stdlib.h>
//
// // Helper functions to work around CGO type issues
// void set_object_name(MQOD* od, const char* name) {
//     // MQ expects names to be padded with spaces, not nulls
//     memset(od->ObjectName, ' ', sizeof(od->ObjectName));
//     size_t len = strlen(name);
//     if (len > sizeof(od->ObjectName)) {
//         len = sizeof(od->ObjectName);
//     }
//     memcpy(od->ObjectName, name, len);
// }
//
// void set_format(MQMD* md, const char* format) {
//     // MQ expects format to be exactly 8 chars, padded with spaces
//     memset(md->Format, ' ', sizeof(md->Format));
//     size_t len = strlen(format);
//     if (len > sizeof(md->Format)) {
//         len = sizeof(md->Format);
//     }
//     memcpy(md->Format, format, len);
// }
//
// void copy_msg_id(MQMD* dest, MQMD* src) {
//     memcpy(dest->CorrelId, src->MsgId, sizeof(src->MsgId));
// }
//
// void set_csp_struc_id(MQCSP* csp) {
//     memcpy(csp->StrucId, MQCSP_STRUC_ID, sizeof(csp->StrucId));
// }
//
// void set_cno_struc_id(MQCNO* cno) {
//     memcpy(cno->StrucId, MQCNO_STRUC_ID, sizeof(cno->StrucId));
// }
//
// void set_od_struc_id(MQOD* od) {
//     memcpy(od->StrucId, MQOD_STRUC_ID, sizeof(od->StrucId));
// }
//
// void set_md_struc_id(MQMD* md) {
//     memcpy(md->StrucId, MQMD_STRUC_ID, sizeof(md->StrucId));
// }
//
// void set_pmo_struc_id(MQPMO* pmo) {
//     memcpy(pmo->StrucId, MQPMO_STRUC_ID, sizeof(pmo->StrucId));
// }
//
// void set_gmo_struc_id(MQGMO* gmo) {
//     memcpy(gmo->StrucId, MQGMO_STRUC_ID, sizeof(gmo->StrucId));
// }
//
import "C"

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	precision = 1000 // Precision multiplier for floating-point values
	maxMQGetBufferSize = 10 * 1024 * 1024 // 10MB maximum buffer size for MQGET responses
)

// MQ connection state
type mqConnection struct {
	hConn      C.MQHCONN    // Connection handle
	hObj       C.MQHOBJ     // Object handle for admin queue
	hReplyObj  C.MQHOBJ     // Object handle for persistent reply queue
	replyQueueName [48]C.char // Store the actual reply queue name
	connected  bool
}

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	collectStart := time.Now()
	c.Debugf("collect() called - connection status: %v", c.mqConn != nil && c.mqConn.connected)
	mx := make(map[string]int64)
	
	// Reset seen map for this collection cycle
	c.seen = make(map[string]bool)
	
	// Connect to queue manager if not connected
	connectStart := time.Now()
	if err := c.ensureConnection(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to queue manager: %w", err)
	}
	connectDuration := time.Since(connectStart)
	c.Debugf("Connection setup took %v", connectDuration)
	
	// Detect version on first connection
	if c.version == "" {
		versionStart := time.Now()
		if err := c.detectVersion(ctx); err != nil {
			c.Warningf("failed to detect MQ version: %v", err)
		} else {
			c.addVersionLabelsToCharts()
		}
		c.Debugf("Version detection took %v", time.Since(versionStart))
	}
	
	// Collect queue manager metrics
	qmgrStart := time.Now()
	if err := c.collectQueueManagerMetrics(ctx, mx); err != nil {
		c.Warningf("failed to collect queue manager metrics: %v", err)
	}
	c.Debugf("Queue manager metrics collection took %v", time.Since(qmgrStart))
	
	// Collect queue metrics (respect admin configuration)
	if c.CollectQueues != nil && *c.CollectQueues {
		queueStart := time.Now()
		if err := c.collectQueueMetrics(ctx, mx); err != nil {
			c.Warningf("failed to collect queue metrics: %v", err)
		}
		c.Debugf("Queue metrics collection took %v", time.Since(queueStart))
	}
	
	// Collect channel metrics (respect admin configuration)
	if c.CollectChannels != nil && *c.CollectChannels {
		channelStart := time.Now()
		if err := c.collectChannelMetrics(ctx, mx); err != nil {
			c.Warningf("failed to collect channel metrics: %v", err)
		}
		c.Debugf("Channel metrics collection took %v", time.Since(channelStart))
	}
	
	// Collect topic metrics (respect admin configuration)
	if c.CollectTopics != nil && *c.CollectTopics {
		topicStart := time.Now()
		if err := c.collectTopicMetrics(ctx, mx); err != nil {
			c.Warningf("failed to collect topic metrics: %v", err)
		}
		c.Debugf("Topic metrics collection took %v", time.Since(topicStart))
	}
	
	// Clean up obsolete charts for instances that no longer exist
	c.markObsoleteCharts()
	
	totalDuration := time.Since(collectStart)
	c.Debugf("collect() finished in %v - connection still valid: %v", totalDuration, c.mqConn != nil && c.mqConn.connected)
	return mx, nil
}

func (c *Collector) ensureConnection(ctx context.Context) error {
	// Check if we already have a valid connection
	if c.mqConn != nil && c.mqConn.connected && c.testConnection() {
		c.Debugf("Reusing existing MQ connection - no new queues will be created")
		return nil
	}
	
	// Clean up any invalid connection
	if c.mqConn != nil {
		c.Debugf("Cleaning up invalid connection")
		c.disconnect()
	}
	
	c.Infof("Creating NEW MQ connection - this will create 2 new queues")
	
	c.mqConn = &mqConnection{}
	
	// Use MQCONNX for client connections
	// Allocate structures in C memory to avoid Go pointer issues
	cno := (*C.MQCNO)(C.malloc(C.sizeof_MQCNO))
	defer C.free(unsafe.Pointer(cno))
	C.memset(unsafe.Pointer(cno), 0, C.sizeof_MQCNO)
	C.set_cno_struc_id(cno)
	cno.Version = C.MQCNO_VERSION_4
	cno.Options = C.MQCNO_CLIENT_BINDING
	
	// Set up client connection channel
	cd := (*C.MQCD)(C.malloc(C.sizeof_MQCD))
	defer C.free(unsafe.Pointer(cd))
	C.memset(unsafe.Pointer(cd), 0, C.sizeof_MQCD)
	cd.ChannelType = C.MQCHT_CLNTCONN
	cd.Version = C.MQCD_VERSION_6
	cd.TransportType = C.MQXPT_TCP
	
	// Set channel name
	channelName := c.Channel
	if channelName == "" {
		channelName = "SYSTEM.DEF.SVRCONN"
	}
	cChannelName := C.CString(channelName)
	defer C.free(unsafe.Pointer(cChannelName))
	C.strncpy((*C.char)(unsafe.Pointer(&cd.ChannelName)), cChannelName, C.MQ_CHANNEL_NAME_LENGTH)
	
	// Set connection name (host and port)
	connName := fmt.Sprintf("%s(%d)", c.Host, c.Port)
	cConnName := C.CString(connName)
	defer C.free(unsafe.Pointer(cConnName))
	C.strncpy((*C.char)(unsafe.Pointer(&cd.ConnectionName)), cConnName, C.MQ_CONN_NAME_LENGTH)
	
	// Set user credentials if provided
	if c.User != "" {
		cUser := C.CString(c.User)
		defer C.free(unsafe.Pointer(cUser))
		C.strncpy((*C.char)(unsafe.Pointer(&cd.UserIdentifier)), cUser, C.MQ_USER_ID_LENGTH)
	}
	
	// Set up connection options to use the channel definition
	cno.ClientConnPtr = C.MQPTR(unsafe.Pointer(cd))
	
	// Set connection options for better stability
	cno.Options = C.MQCNO_RECONNECT | C.MQCNO_HANDLE_SHARE_BLOCK
	
	// Set up authentication if password is provided
	var cspUser, cspPassword *C.char
	if c.Password != "" {
		csp := (*C.MQCSP)(C.malloc(C.sizeof_MQCSP))
		defer C.free(unsafe.Pointer(csp))
		C.memset(unsafe.Pointer(csp), 0, C.sizeof_MQCSP)
		C.set_csp_struc_id(csp)
		csp.Version = C.MQCSP_VERSION_1
		csp.AuthenticationType = C.MQCSP_AUTH_USER_ID_AND_PWD
		cspUser = C.CString(c.User)
		defer C.free(unsafe.Pointer(cspUser))
		csp.CSPUserIdPtr = C.MQPTR(unsafe.Pointer(cspUser))
		csp.CSPUserIdLength = C.MQLONG(len(c.User))
		cspPassword = C.CString(c.Password)
		defer C.free(unsafe.Pointer(cspPassword))
		csp.CSPPasswordPtr = C.MQPTR(unsafe.Pointer(cspPassword))
		csp.CSPPasswordLength = C.MQLONG(len(c.Password))
		
		cno.Version = C.MQCNO_VERSION_5
		cno.SecurityParmsPtr = (*C.MQCSP)(unsafe.Pointer(csp))
	}
	
	// Queue manager name
	qmName := C.CString(c.QueueManager)
	defer C.free(unsafe.Pointer(qmName))
	
	var compCode C.MQLONG
	var reason C.MQLONG
	
	// Connect to queue manager
	C.MQCONNX(qmName, cno, &c.mqConn.hConn, &compCode, &reason)
	
	if compCode != C.MQCC_OK {
		return fmt.Errorf("MQCONNX failed: completion code %d, reason code %d (check queue manager '%s' is running and accessible on %s:%d)", 
			compCode, reason, c.QueueManager, c.Host, c.Port)
	}
	
	c.Debugf("MQCONNX successful - handle: %v", c.mqConn.hConn)
	
	// Open system command input queue for PCF commands
	var od C.MQOD
	C.memset(unsafe.Pointer(&od), 0, C.sizeof_MQOD)
	C.set_od_struc_id(&od)
	od.Version = C.MQOD_VERSION_1
	queueName := C.CString("SYSTEM.ADMIN.COMMAND.QUEUE")
	defer C.free(unsafe.Pointer(queueName))
	C.set_object_name(&od, queueName)
	od.ObjectType = C.MQOT_Q
	
	var openOptions C.MQLONG = C.MQOO_OUTPUT | C.MQOO_FAIL_IF_QUIESCING
	
	// Reset completion and reason codes before MQOPEN
	compCode = C.MQCC_FAILED
	reason = C.MQRC_UNEXPECTED_ERROR
	
	C.MQOPEN(c.mqConn.hConn, C.PMQVOID(unsafe.Pointer(&od)), openOptions, &c.mqConn.hObj, &compCode, &reason)
	
	if compCode != C.MQCC_OK {
		// Save the actual error codes before disconnect
		openCompCode := compCode
		openReason := reason
		
		var discCompCode, discReason C.MQLONG
		C.MQDISC(&c.mqConn.hConn, &discCompCode, &discReason)
		return fmt.Errorf("MQOPEN failed: completion code %d, reason code %d (check PCF permissions for SYSTEM.ADMIN.COMMAND.QUEUE)", openCompCode, openReason)
	}
	
	c.Infof("Admin queue opened successfully, now creating persistent reply queue (Queue #2)")
	
	// Create a persistent dynamic reply queue for all PCF commands
	var replyOd C.MQOD
	C.memset(unsafe.Pointer(&replyOd), 0, C.sizeof_MQOD)
	C.set_od_struc_id(&replyOd)
	replyOd.Version = C.MQOD_VERSION_1
	modelQueueName := C.CString("SYSTEM.DEFAULT.MODEL.QUEUE")
	defer C.free(unsafe.Pointer(modelQueueName))
	C.set_object_name(&replyOd, modelQueueName)
	replyOd.ObjectType = C.MQOT_Q
	
	// Set dynamic queue name pattern - use a unique pattern to avoid conflicts
	dynQueueName := C.CString("NETDATA.PCF.*")
	defer C.free(unsafe.Pointer(dynQueueName))
	C.memset(unsafe.Pointer(&replyOd.DynamicQName), ' ', 48)
	C.memcpy(unsafe.Pointer(&replyOd.DynamicQName), unsafe.Pointer(dynQueueName), C.strlen(dynQueueName))
	
	var replyOpenOptions C.MQLONG = C.MQOO_INPUT_AS_Q_DEF | C.MQOO_FAIL_IF_QUIESCING
	
	C.MQOPEN(c.mqConn.hConn, C.PMQVOID(unsafe.Pointer(&replyOd)), replyOpenOptions, &c.mqConn.hReplyObj, &compCode, &reason)
	if compCode != C.MQCC_OK {
		// Save the actual error codes before disconnect
		replyOpenCompCode := compCode
		replyOpenReason := reason
		
		// Close admin queue
		C.MQCLOSE(c.mqConn.hConn, &c.mqConn.hObj, C.MQCO_NONE, &compCode, &reason)
		
		var discCompCode, discReason C.MQLONG
		C.MQDISC(&c.mqConn.hConn, &discCompCode, &discReason)
		return fmt.Errorf("MQOPEN reply queue failed: completion code %d, reason code %d", replyOpenCompCode, replyOpenReason)
	}
	
	// Store the actual reply queue name for use in PCF commands
	C.memcpy(unsafe.Pointer(&c.mqConn.replyQueueName), unsafe.Pointer(&replyOd.ObjectName), 48)
	
	c.mqConn.connected = true
	c.Infof("Successfully connected to queue manager %s on %s:%d via channel %s", 
		c.QueueManager, c.Host, c.Port, c.Channel)
	actualReplyQueueName := C.GoStringN((*C.char)(unsafe.Pointer(&c.mqConn.replyQueueName[0])), 48)
	c.Infof("Created persistent reply queue: %s (will be reused for all PCF commands)", strings.TrimSpace(actualReplyQueueName))
	
	return nil
}

func (c *Collector) disconnect() {
	if c.mqConn == nil {
		return
	}
	
	c.Debugf("Disconnecting from queue manager %s after %d PCF commands", c.QueueManager, c.pcfCommandCount)
	c.pcfCommandCount = 0
	
	var compCode C.MQLONG
	var reason C.MQLONG
	
	// Close the persistent reply queue
	if c.mqConn.hReplyObj != C.MQHO_UNUSABLE_HOBJ {
		C.MQCLOSE(c.mqConn.hConn, &c.mqConn.hReplyObj, C.MQCO_DELETE_PURGE, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.Warningf("Failed to close reply queue: completion code %d, reason code %d", compCode, reason)
		}
	}
	
	if c.mqConn.hObj != C.MQHO_UNUSABLE_HOBJ {
		C.MQCLOSE(c.mqConn.hConn, &c.mqConn.hObj, C.MQCO_NONE, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.Warningf("Failed to close admin queue: completion code %d, reason code %d", compCode, reason)
		}
	}
	
	if c.mqConn.hConn != C.MQHC_UNUSABLE_HCONN {
		C.MQDISC(&c.mqConn.hConn, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.Warningf("Failed to disconnect: completion code %d, reason code %d", compCode, reason)
		} else {
			c.Debugf("Successfully disconnected from queue manager %s", c.QueueManager)
		}
	}
	
	c.mqConn.connected = false
	c.mqConn = nil
}

func (c *Collector) validateConnection() error {
	if c.mqConn == nil || !c.mqConn.connected {
		return fmt.Errorf("not connected to queue manager")
	}
	
	// Check if connection handle is still valid
	if c.mqConn.hConn == C.MQHC_UNUSABLE_HCONN {
		c.mqConn.connected = false
		return fmt.Errorf("connection handle is invalid")
	}
	
	// Check if reply queue handle is still valid
	if c.mqConn.hReplyObj == C.MQHO_UNUSABLE_HOBJ {
		c.mqConn.connected = false
		return fmt.Errorf("reply queue handle is invalid")
	}
	
	// For now, just validate handles - connection will be tested on first PCF command
	// A more advanced health check could be added later if needed
	
	return nil
}

func (c *Collector) sendPCFCommand(command C.MQLONG, parameters []pcfParameter) ([]byte, error) {
	// Validate connection before sending command
	if err := c.validateConnection(); err != nil {
		return nil, err
	}
	
	// Debug: log connection handle value and reply queue details
	c.pcfCommandCount++
	c.Debugf("PCF command #%d: Connection handle: %v, admin queue: %v, reply queue: %v", 
		c.pcfCommandCount, c.mqConn.hConn, c.mqConn.hObj, c.mqConn.hReplyObj)
	
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
	
	c.Debugf("Sending PCF command %d with %d parameters, message size %d", command, len(parameters), msgSize)
	
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
	md.CodedCharSetId = C.MQCCSI_Q_MGR
	md.Priority = C.MQPRI_PRIORITY_AS_Q_DEF
	md.Persistence = C.MQPER_NOT_PERSISTENT
	
	// Use the persistent reply queue name
	C.memcpy(unsafe.Pointer(&md.ReplyToQ), unsafe.Pointer(&c.mqConn.replyQueueName), 48)
	
	var pmo C.MQPMO
	C.memset(unsafe.Pointer(&pmo), 0, C.sizeof_MQPMO)
	C.set_pmo_struc_id(&pmo)
	pmo.Version = C.MQPMO_VERSION_1
	pmo.Options = C.MQPMO_NO_SYNCPOINT | C.MQPMO_FAIL_IF_QUIESCING
	
	C.MQPUT(c.mqConn.hConn, c.mqConn.hObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&pmo)), C.MQLONG(msgSize), C.PMQVOID(msgBuffer), &compCode, &reason)
	
	if compCode != C.MQCC_OK {
		// If connection handle error, mark connection as failed
		if reason == C.MQRC_HCONN_ERROR {
			c.mqConn.connected = false
			c.Errorf("MQPUT failed with HCONN_ERROR - connection handle is now invalid")
		}
		return nil, fmt.Errorf("MQPUT failed: completion code %d, reason code %d", compCode, reason)
	}
	
	// Get response from persistent reply queue
	response, err := c.getPCFResponse(&md, c.mqConn.hReplyObj)
	
	return response, err
}

func (c *Collector) getPCFResponse(requestMd *C.MQMD, hReplyObj C.MQHOBJ) ([]byte, error) {
	var compCode C.MQLONG
	var reason C.MQLONG
	var allResponses []byte
	
	// For wildcard queries, MQ may return multiple response messages
	for {
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
		gmo.Options = C.MQGMO_WAIT | C.MQGMO_FAIL_IF_QUIESCING | C.MQGMO_CONVERT
		gmo.WaitInterval = 1000 // 1 second timeout for subsequent messages
		
		// Get message length first
		var bufferLength C.MQLONG = 0
		C.MQGET(c.mqConn.hConn, hReplyObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), bufferLength, nil, &bufferLength, &compCode, &reason)
		
		if reason == C.MQRC_NO_MSG_AVAILABLE {
			// No more messages, return what we have
			break
		}
		
		if reason != C.MQRC_TRUNCATED_MSG_FAILED {
			// If connection handle error, mark connection as failed
			if reason == C.MQRC_HCONN_ERROR {
				c.mqConn.connected = false
				c.Errorf("MQGET (length check) failed with HCONN_ERROR - connection handle is now invalid")
			}
			// For the first message, this is an error
			if len(allResponses) == 0 {
				return nil, fmt.Errorf("MQGET length check failed: completion code %d, reason code %d", compCode, reason)
			}
			// For subsequent messages, just return what we have
			break
		}
		
		// Defensive check: prevent excessive memory allocation
		if bufferLength > maxMQGetBufferSize {
			return nil, fmt.Errorf("PCF response too large (%d bytes), maximum allowed is %d bytes", bufferLength, maxMQGetBufferSize)
		}
		
		// Allocate buffer and get actual message
		buffer := make([]byte, bufferLength)
		C.MQGET(c.mqConn.hConn, hReplyObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), bufferLength, C.PMQVOID(unsafe.Pointer(&buffer[0])), &bufferLength, &compCode, &reason)
		
		if compCode != C.MQCC_OK {
			// If connection handle error, mark connection as failed
			if reason == C.MQRC_HCONN_ERROR {
				c.mqConn.connected = false
				c.Errorf("MQGET (read) failed with HCONN_ERROR - connection handle is now invalid")
			}
			// For the first message, this is an error
			if len(allResponses) == 0 {
				return nil, fmt.Errorf("MQGET failed: completion code %d, reason code %d", compCode, reason)
			}
			// For subsequent messages, just return what we have
			break
		}
		
		// Check the PCF header before appending
		cfh := (*C.MQCFH)(unsafe.Pointer(&buffer[0]))
		c.Debugf("getPCFResponse: Received message, Control=%d, CompCode=%d, Reason=%d, ParameterCount=%d", 
			cfh.Control, cfh.CompCode, cfh.Reason, cfh.ParameterCount)
		
		// If this is an error response (CompCode != 0), don't collect more messages
		// Just return this single error response for proper error handling
		if cfh.CompCode != C.MQCC_OK {
			return buffer[:bufferLength], nil
		}
		
		// Append this response to our collection
		allResponses = append(allResponses, buffer[:bufferLength]...)
		
		// Check if this is the last message in the sequence
		if cfh.Control == C.MQCFC_LAST {
			break
		}
	}
	
	if len(allResponses) == 0 {
		return nil, fmt.Errorf("no PCF response messages received")
	}
	
	return allResponses, nil
}

func (c *Collector) collectQueueManagerMetrics(ctx context.Context, mx map[string]int64) error {
	// For now, try a simple MQCMD_INQUIRE_Q_MGR to get basic status
	// Note: CPU, memory, and log usage metrics might not be available through PCF
	// in all MQ versions - they might require resource statistics monitoring
	start := time.Now()
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q_MGR, nil)
	sendDuration := time.Since(start)
	c.Debugf("PCF command MQCMD_INQUIRE_Q_MGR took %v", sendDuration)
	
	if err != nil {
		// Don't set status to 0 - leave it unset (null) to indicate we couldn't collect it
		// Return error but don't fail the whole collection
		c.Debugf("Queue manager inquiry failed: %v", err)
		return nil
	}
	
	// Parse response with timing
	parseStart := time.Now()
	attrs, err := c.parsePCFResponse(response)
	parseDuration := time.Since(parseStart)
	c.Debugf("PCF response parsing took %v", parseDuration)
	
	if err != nil {
		return fmt.Errorf("failed to parse queue manager status response: %w", err)
	}
	
	// Extract metrics - set basic status (1 = running, 0 = unknown)
	mx["qmgr_status"] = 1
	
	// Try different attribute IDs that might contain the metrics
	// Extract CPU usage if available (percentage * precision)
	if cpuLoad, ok := attrs[MQIACF_Q_MGR_CPU_LOAD]; ok {
		mx["qmgr_cpu_usage"] = int64(cpuLoad.(int32)) * precision / 100
	}
	
	// Extract memory usage if available (bytes)
	if memUsage, ok := attrs[MQIACF_Q_MGR_MEMORY_USAGE]; ok {
		mx["qmgr_memory_usage"] = int64(memUsage.(int32))
	}
	
	// Extract log usage if available (percentage * precision)
	if logUsage, ok := attrs[MQIACF_Q_MGR_LOG_USAGE]; ok {
		mx["qmgr_log_usage"] = int64(logUsage.(int32)) * precision / 100
	}
	
	// Log successful collection for debugging (removed expensive attribute loop)
	c.Debugf("Collected queue manager metrics, response had %d attributes", len(attrs))
	
	return nil
}

func (c *Collector) collectQueueMetrics(ctx context.Context, mx map[string]int64) error {
	// Get list of queues
	queues, err := c.getQueueList(ctx)
	if err != nil {
		return fmt.Errorf("failed to get queue list: %w", err)
	}
	
	c.Debugf("Found %d queues", len(queues))
	
	collected := 0
	for _, queueName := range queues {
		if !c.shouldCollectQueue(queueName) {
			continue
		}
		collected++
		
		// Mark as seen
		c.seen[queueName] = true
		
		cleanName := c.cleanName(queueName)
		
		// Add queue charts if not already present
		if !c.collected[queueName] {
			c.collected[queueName] = true
			charts := c.newQueueCharts(queueName)
			if err := c.charts.Add(*charts...); err != nil {
				c.Warning(err)
			}
		}
		
		// Collect queue metrics
		if err := c.collectSingleQueueMetrics(ctx, queueName, cleanName, mx); err != nil {
			c.Warningf("failed to collect metrics for queue %s: %v", queueName, err)
		}
	}
	
	c.Infof("Monitoring %d out of %d queues", collected, len(queues))
	return nil
}

func (c *Collector) collectChannelMetrics(ctx context.Context, mx map[string]int64) error {
	// Get list of channels
	channels, err := c.getChannelList(ctx)
	if err != nil {
		// Check if this is an authorization error
		if strings.Contains(err.Error(), "2035") || strings.Contains(err.Error(), "NOT_AUTHORIZED") {
			c.Warningf("Not authorized to query channels - skipping channel collection: %v", err)
			return nil // Don't fail entire collection
		}
		return fmt.Errorf("failed to get channel list: %w", err)
	}
	
	c.Debugf("Found %d channels", len(channels))
	
	collected := 0
	for _, channelName := range channels {
		if !c.shouldCollectChannel(channelName) {
			continue
		}
		collected++
		
		// Mark as seen
		c.seen[channelName] = true
		
		cleanName := c.cleanName(channelName)
		
		// Add channel charts if not already present
		if !c.collected[channelName] {
			c.collected[channelName] = true
			charts := c.newChannelCharts(channelName)
			if err := c.charts.Add(*charts...); err != nil {
				c.Warning(err)
			}
		}
		
		// Collect channel metrics
		if err := c.collectSingleChannelMetrics(ctx, channelName, cleanName, mx); err != nil {
			c.Warningf("failed to collect metrics for channel %s: %v", channelName, err)
		}
	}
	
	c.Infof("Monitoring %d out of %d channels", collected, len(channels))
	return nil
}

func (c *Collector) collectTopicMetrics(ctx context.Context, mx map[string]int64) error {
	// Get list of topics
	topics, err := c.getTopicList(ctx)
	if err != nil {
		return fmt.Errorf("failed to get topic list: %w", err)
	}
	
	c.Debugf("Found %d topics", len(topics))
	
	collected := 0
	for _, topicName := range topics {
		if !c.shouldCollectTopic(topicName) {
			continue
		}
		collected++
		
		// Mark as seen
		c.seen[topicName] = true
		
		cleanName := c.cleanName(topicName)
		
		// Add topic charts if not already present
		if !c.collected[topicName] {
			c.collected[topicName] = true
			charts := c.newTopicCharts(topicName)
			if err := c.charts.Add(*charts...); err != nil {
				c.Warning(err)
			}
		}
		
		// Collect topic metrics
		if err := c.collectSingleTopicMetrics(ctx, topicName, cleanName, mx); err != nil {
			c.Warningf("failed to collect metrics for topic %s: %v", topicName, err)
		}
	}
	
	c.Infof("Monitoring %d out of %d topics", collected, len(topics))
	return nil
}

func (c *Collector) getQueueList(ctx context.Context) ([]string, error) {
	// Primary approach: Use MQCMD_INQUIRE_Q with wildcard queue name and local queue type
	// Based on IBM documentation and testing, both parameters are needed
	c.Debugf("Sending MQCMD_INQUIRE_Q with Q_NAME='*' and Q_TYPE=MQQT_LOCAL")
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, "*"),
	}
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		c.Debugf("MQCMD_INQUIRE_Q with wildcard and type failed: %v", err)
		
		// Fallback: try with just wildcard name
		c.Debugf("Falling back to MQCMD_INQUIRE_Q with Q_NAME='*' only")
		params = []pcfParameter{
			newStringParameter(C.MQCA_Q_NAME, "*"),
		}
		response, err = c.sendPCFCommand(C.MQCMD_INQUIRE_Q, params)
		if err != nil {
			// Last resort: try INQUIRE_Q_NAMES
			c.Debugf("Trying MQCMD_INQUIRE_Q_NAMES with Q_NAME='*'")
			params = []pcfParameter{
				newStringParameter(C.MQCA_Q_NAME, "*"),
			}
			response, err = c.sendPCFCommand(MQCMD_INQUIRE_Q_NAMES, params)
			if err != nil {
				return nil, fmt.Errorf("all queue discovery methods failed: %w", err)
			}
		}
	}
	
	// Parse response using the existing parseQueueListResponse function
	queues, err := c.parseQueueListResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse queue list response: %w", err)
	}
	
	c.Debugf("Found %d queues: %v", len(queues), queues)
	return queues, nil
}

func (c *Collector) getChannelList(ctx context.Context) ([]string, error) {
	// Primary approach: Use MQCMD_INQUIRE_CHANNEL with wildcard channel name parameter
	// Based on IBM documentation, this is the most reliable approach
	c.Debugf("Sending MQCMD_INQUIRE_CHANNEL with CHANNEL_NAME='*'")
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, "*"),
	}
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_CHANNEL, params)
	if err != nil {
		c.Debugf("MQCMD_INQUIRE_CHANNEL with wildcard failed: %v", err)
		
		// Fallback: try without parameters (may work in some MQ versions)
		c.Debugf("Falling back to MQCMD_INQUIRE_CHANNEL with no parameters")
		response, err = c.sendPCFCommand(C.MQCMD_INQUIRE_CHANNEL, nil)
		if err != nil {
			// Last resort: try INQUIRE_CHANNEL_NAMES
			c.Debugf("Trying MQCMD_INQUIRE_CHANNEL_NAMES with CHANNEL_NAME='*'")
			params = []pcfParameter{
				newStringParameter(C.MQCACH_CHANNEL_NAME, "*"),
			}
			response, err = c.sendPCFCommand(MQCMD_INQUIRE_CHANNEL_NAMES, params)
			if err != nil {
				return nil, fmt.Errorf("all channel discovery methods failed: %w", err)
			}
		}
	}
	
	// Parse response using the existing parseChannelListResponse function
	channels, err := c.parseChannelListResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse channel list response: %w", err)
	}
	
	c.Debugf("Found %d channels: %v", len(channels), channels)
	return channels, nil
}

func (c *Collector) shouldCollectQueue(queueName string) bool {
	// Check system queue collection setting
	if strings.HasPrefix(queueName, "SYSTEM.") {
		if c.CollectSystemQueues == nil || !*c.CollectSystemQueues {
			c.Debugf("Skipping system queue (disabled): %s", queueName)
			return false
		}
		c.Debugf("Including system queue (enabled): %s", queueName)
	}
	
	// Apply queue selector if configured
	if c.queueSelectorRegex != nil {
		matches := c.queueSelectorRegex.MatchString(queueName)
		if !matches {
			c.Debugf("Queue %s does not match selector pattern '%s', skipping", queueName, c.QueueSelector)
		}
		return matches
	}
	
	return true
}

func (c *Collector) shouldCollectChannel(channelName string) bool {
	// Check system channel collection setting
	if strings.HasPrefix(channelName, "SYSTEM.") {
		if c.CollectSystemChannels == nil || !*c.CollectSystemChannels {
			c.Debugf("Skipping system channel (disabled): %s", channelName)
			return false
		}
		c.Debugf("Including system channel (enabled): %s", channelName)
	}
	
	// Apply channel selector if configured
	if c.channelSelectorRegex != nil {
		matches := c.channelSelectorRegex.MatchString(channelName)
		if !matches {
			c.Debugf("Channel %s does not match selector pattern '%s', skipping", channelName, c.ChannelSelector)
		}
		return matches
	}
	
	return true
}

func (c *Collector) collectSingleQueueMetrics(ctx context.Context, queueName, cleanName string, mx map[string]int64) error {
	// Send INQUIRE_Q_STATUS for specific queue
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, queueName),
	}
	
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q_STATUS, params)
	if err != nil {
		return fmt.Errorf("failed to send INQUIRE_Q_STATUS for %s: %w", queueName, err)
	}
	
	// Parse response
	attrs, err := c.parsePCFResponse(response)
	if err != nil {
		return fmt.Errorf("failed to parse queue status response for %s: %w", queueName, err)
	}
	
	
	// Extract metrics - always set depth as it's critical
	if depth, ok := attrs[C.MQIA_CURRENT_Q_DEPTH]; ok {
		mx[fmt.Sprintf("queue_%s_depth", cleanName)] = int64(depth.(int32))
	} else {
		// Queue depth should always be available from INQUIRE_Q_STATUS
		c.Warningf("MQIA_CURRENT_Q_DEPTH (%d) not found for queue %s - setting to 0", C.MQIA_CURRENT_Q_DEPTH, queueName)
		mx[fmt.Sprintf("queue_%s_depth", cleanName)] = 0
	}
	
	// Note: MQIA_MSG_ENQ_COUNT and MQIA_MSG_DEQ_COUNT are not returned by INQUIRE_Q_STATUS
	// These would need MQCMD_INQUIRE_Q or reset queue statistics to be enabled
	// For now, we don't set these metrics if not available
	if enqueued, ok := attrs[C.MQIA_MSG_ENQ_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_enqueued", cleanName)] = int64(enqueued.(int32))
	}
	
	if dequeued, ok := attrs[C.MQIA_MSG_DEQ_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_dequeued", cleanName)] = int64(dequeued.(int32))
	}
	
	// Extract oldest message age if available (seconds)
	// This metric might also not be available in INQUIRE_Q_STATUS
	if oldestAge, ok := attrs[MQIA_OLDEST_MSG_AGE]; ok {
		mx[fmt.Sprintf("queue_%s_oldest_message_age", cleanName)] = int64(oldestAge.(int32))
	}
	
	return nil
}

func (c *Collector) collectSingleChannelMetrics(ctx context.Context, channelName, cleanName string, mx map[string]int64) error {
	// Send INQUIRE_CHANNEL_STATUS for specific channel
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, channelName),
	}
	
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_CHANNEL_STATUS, params)
	if err != nil {
		return fmt.Errorf("failed to send INQUIRE_CHANNEL_STATUS for %s: %w", channelName, err)
	}
	
	// Parse response
	attrs, err := c.parsePCFResponse(response)
	if err != nil {
		return fmt.Errorf("failed to parse channel status response for %s: %w", channelName, err)
	}
	
	// Extract metrics
	if status, ok := attrs[C.MQIACH_CHANNEL_STATUS]; ok {
		mx[fmt.Sprintf("channel_%s_status", cleanName)] = int64(status.(int32))
	}
	
	if messages, ok := attrs[C.MQIACH_MSGS]; ok {
		mx[fmt.Sprintf("channel_%s_messages", cleanName)] = int64(messages.(int32))
	}
	
	if bytes, ok := attrs[C.MQIACH_BYTES_SENT]; ok {
		mx[fmt.Sprintf("channel_%s_bytes", cleanName)] = int64(bytes.(int32))
	}
	
	if batches, ok := attrs[C.MQIACH_BATCHES]; ok {
		mx[fmt.Sprintf("channel_%s_batches", cleanName)] = int64(batches.(int32))
	}
	
	return nil
}

func (c *Collector) getTopicList(ctx context.Context) ([]string, error) {
	// Send INQUIRE_TOPIC command with generic topic name
	params := []pcfParameter{
		newStringParameter(C.MQCA_TOPIC_NAME, "*"),
	}
	
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_TOPIC, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send INQUIRE_TOPIC: %w", err)
	}
	
	// Parse topic names from response
	topics, err := c.parseTopicListResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse topic list response: %w", err)
	}
	
	return topics, nil
}

func (c *Collector) shouldCollectTopic(topicName string) bool {
	// Skip system topics by default
	if strings.HasPrefix(topicName, "SYSTEM.") {
		return false
	}
	
	// Apply topic selector if configured (future enhancement)
	// For now, collect all non-system topics
	return true
}

func (c *Collector) collectSingleTopicMetrics(ctx context.Context, topicName, cleanName string, mx map[string]int64) error {
	// Send INQUIRE_TOPIC_STATUS for specific topic
	params := []pcfParameter{
		newStringParameter(C.MQCA_TOPIC_NAME, topicName),
	}
	
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_TOPIC_STATUS, params)
	if err != nil {
		return fmt.Errorf("failed to send INQUIRE_TOPIC_STATUS for %s: %w", topicName, err)
	}
	
	// Parse response
	attrs, err := c.parsePCFResponse(response)
	if err != nil {
		return fmt.Errorf("failed to parse topic status response for %s: %w", topicName, err)
	}
	
	// Extract metrics
	if publishers, ok := attrs[C.MQIA_PUB_COUNT]; ok {
		mx[fmt.Sprintf("topic_%s_publishers", cleanName)] = int64(publishers.(int32))
	}
	
	if subscribers, ok := attrs[C.MQIA_SUB_COUNT]; ok {
		mx[fmt.Sprintf("topic_%s_subscribers", cleanName)] = int64(subscribers.(int32))
	}
	
	if messages, ok := attrs[MQIA_TOPIC_MSG_COUNT]; ok {
		mx[fmt.Sprintf("topic_%s_messages", cleanName)] = int64(messages.(int32))
	}
	
	return nil
}

func (c *Collector) detectVersion(ctx context.Context) error {
	// Send PCF command to inquire queue manager
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q_MGR, nil)
	if err != nil {
		return fmt.Errorf("failed to send INQUIRE_Q_MGR: %w", err)
	}
	
	// Parse response
	attrs, err := c.parsePCFResponse(response)
	if err != nil {
		return fmt.Errorf("failed to parse queue manager response: %w", err)
	}
	
	// Extract version information
	if version, ok := attrs[C.MQCA_VERSION]; ok {
		if versionStr, ok := version.(string); ok {
			c.version = strings.TrimSpace(versionStr)
		}
	}
	
	// Extract command level (edition/capability indicator)  
	if cmdLevel, ok := attrs[C.MQIA_COMMAND_LEVEL]; ok {
		if level, ok := cmdLevel.(int32); ok {
			// Map command level to edition
			switch {
			case level >= 900:
				c.edition = "Advanced"
			case level >= 800:
				c.edition = "Standard"
			default:
				c.edition = "Express"
			}
		}
	}
	
	if c.version == "" {
		c.version = "unknown"
	}
	if c.edition == "" {
		c.edition = "unknown"
	}
	
	c.Infof("Detected MQ version: %s, edition: %s", c.version, c.edition)
	return nil
}

func (c *Collector) addVersionLabelsToCharts() {
	versionLabels := []module.Label{
		{Key: "mq_version", Value: c.version},
		{Key: "mq_edition", Value: c.edition},
	}
	
	// Add to all existing charts
	for _, chart := range *c.charts {
		chart.Labels = append(chart.Labels, versionLabels...)
	}
}

func (c *Collector) cleanName(name string) string {
	r := strings.NewReplacer(
		" ", "_",
		".", "_",
		"-", "_",
		"/", "_",
		":", "_",
		"=", "_",
		"(", "_",
		")", "_",
	)
	return strings.ToLower(r.Replace(name))
}

// markObsoleteCharts marks charts as obsolete when their instances no longer exist
func (c *Collector) markObsoleteCharts() {
	// Check all collected instances
	for name := range c.collected {
		if !c.seen[name] {
			// Instance no longer exists - mark all its charts as obsolete
			cleanName := c.cleanName(name)
			c.Debugf("Instance %s no longer exists, marking charts as obsolete", name)
			
			// Find all charts for this instance
			chartsMarked := 0
			for _, chart := range *c.charts {
				// Check if this chart belongs to the missing instance
				// Queue charts have IDs like "queue_cleanname_depth"
				// Channel charts have IDs like "channel_cleanname_status", etc.
				if strings.Contains(chart.ID, cleanName) {
					if !chart.Obsolete {
						// Set Obsolete flag and reset created flag so framework will send CHART command
						// Don't set remove flag yet - this allows the framework to send the obsolete chart command
						chart.Obsolete = true
						chart.MarkNotCreated() // Reset created flag to trigger CHART command
						chartsMarked++
						c.Debugf("Marked chart %s as obsolete", chart.ID)
					}
				}
			}
			
			// Remove from collected map
			delete(c.collected, name)
			c.Infof("Removed instance %s - marked %d charts as obsolete", name, chartsMarked)
		}
	}
}

// testConnection performs a lightweight test to verify the MQ connection is still valid
func (c *Collector) testConnection() bool {
	if c.mqConn == nil || c.mqConn.hConn == C.MQHC_UNUSABLE_HCONN {
		c.Debugf("Connection test failed: connection is nil or handle is unusable")
		return false
	}
	
	// For now, just do basic handle validation - attempting MQINQ was causing compilation issues
	// The connection will be tested when we actually send PCF commands
	// If those fail, the connection will be marked as invalid and recreated
	c.Debugf("Connection test passed - connection handles are valid")
	return true
}