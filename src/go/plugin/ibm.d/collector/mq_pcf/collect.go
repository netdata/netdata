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
	precision          = 1000             // Precision multiplier for floating-point values
	maxMQGetBufferSize = 10 * 1024 * 1024 // 10MB maximum buffer size for MQGET responses
)

// MQ connection state
type mqConnection struct {
	hConn          C.MQHCONN  // Connection handle
	hObj           C.MQHOBJ   // Object handle for admin queue
	hReplyObj      C.MQHOBJ   // Object handle for persistent reply queue
	replyQueueName [48]C.char // Store the actual reply queue name
	connected      bool
}

// PCF command tracking for detailed success/failure analysis
type pcfTracker struct {
	// Global header tracking (all commands - single and array responses)
	requests map[string]map[string]int // command -> CompCode -> count
	reasons  map[string]map[int32]int  // command -> Reason -> count

	// Array item tracking (only for array-returning commands)
	itemReasons map[string]map[int32]int  // command -> MQIACF_REASON_CODE -> count
	itemCodes   map[string]map[string]int // command -> MQIACF_COMP_CODE -> count
}

func newPCFTracker() *pcfTracker {
	return &pcfTracker{
		requests:    make(map[string]map[string]int),
		reasons:     make(map[string]map[int32]int),
		itemReasons: make(map[string]map[int32]int),
		itemCodes:   make(map[string]map[string]int),
	}
}

func (p *pcfTracker) trackGlobalHeader(command string, compCode int32, reason int32) {
	// Track global CompCode
	if p.requests[command] == nil {
		p.requests[command] = make(map[string]int)
	}
	compCodeStr := p.compCodeToString(compCode)
	p.requests[command][compCodeStr]++

	// Track global Reason
	if p.reasons[command] == nil {
		p.reasons[command] = make(map[int32]int)
	}
	p.reasons[command][reason]++
}

func (p *pcfTracker) trackArrayItem(command string, itemCompCode int32, itemReason int32) {
	// Track item MQIACF_COMP_CODE
	if p.itemCodes[command] == nil {
		p.itemCodes[command] = make(map[string]int)
	}
	itemCompCodeStr := p.compCodeToString(itemCompCode)
	p.itemCodes[command][itemCompCodeStr]++

	// Track item MQIACF_REASON_CODE
	if p.itemReasons[command] == nil {
		p.itemReasons[command] = make(map[int32]int)
	}
	p.itemReasons[command][itemReason]++
}

// trackRequest is a convenience method that calls trackGlobalHeader
func (p *pcfTracker) trackRequest(command string, compCode int32, reason int32) {
	p.trackGlobalHeader(command, compCode, reason)
}

func (p *pcfTracker) compCodeToString(compCode int32) string {
	switch compCode {
	case 0:
		return "MQCC_OK"
	case 1:
		return "MQCC_WARNING"
	case 2:
		return "MQCC_FAILED"
	default:
		return fmt.Sprintf("UNKNOWN_%d", compCode)
	}
}

func (p *pcfTracker) reasonCodeToString(reason int32) string {
	switch reason {
	case 0:
		return "MQRC_NONE"
	case 2035:
		return "MQRC_NOT_AUTHORIZED"
	case 2085:
		return "MQRC_UNKNOWN_OBJECT_NAME"
	case 2033:
		return "MQRC_NO_MSG_AVAILABLE"
	case 2067:
		return "MQRC_OBJECT_OPEN_ERROR"
	default:
		return fmt.Sprintf("MQRC_%d", reason)
	}
}

// logCollectionSummary logs what was actually collected and detailed PCF command analysis
func (c *Collector) logCollectionSummary(mx map[string]int64, totalDuration time.Duration) {
	// Count what we collected
	queueManagerMetrics := 0
	if _, exists := mx["qmgr_status"]; exists {
		queueManagerMetrics = 1
	}

	// Count queues monitored
	queuesMonitored := int(mx["queues_monitored"])
	queuesExcluded := int(mx["queues_excluded"])
	queuesModel := int(mx["queues_model"])
	queuesUnauthorized := int(mx["queues_unauthorized"])
	queuesFailed := int(mx["queues_failed"])
	totalQueuesAttempted := queuesMonitored + queuesModel + queuesUnauthorized + queuesFailed

	// Count channels monitored
	channelsMonitored := int(mx["channels_monitored"])
	channelsExcluded := int(mx["channels_excluded"])
	channelsUnauthorized := int(mx["channels_unauthorized"])
	channelsFailed := int(mx["channels_failed"])
	totalChannelsAttempted := channelsMonitored + channelsUnauthorized + channelsFailed

	// Estimate PCF commands executed
	// Rule of thumb: 1 discovery + 2-3 per successful queue/channel (status + config + optional statistics)
	estimatedPCFCommands := queueManagerMetrics // MQCMD_INQUIRE_Q_MGR

	if totalQueuesAttempted > 0 {
		estimatedPCFCommands += 1                   // MQCMD_INQUIRE_Q (discovery)
		estimatedPCFCommands += queuesMonitored * 3 // MQCMD_INQUIRE_Q (config + status + statistics per queue)
	}

	if totalChannelsAttempted > 0 {
		estimatedPCFCommands += 1                     // MQCMD_INQUIRE_CHANNEL (discovery)
		estimatedPCFCommands += channelsMonitored * 3 // MQCMD_INQUIRE_CHANNEL (config + status + statistics per channel)
	}

	// Log collection summary
	c.Debugf("Collection summary (%v): QMgr=%d, Queues=%d/%d successful (%d excluded, %d model, %d auth, %d failed), Channels=%d/%d successful (%d excluded, %d auth, %d failed), Est. PCF commands=~%d",
		totalDuration,
		queueManagerMetrics,
		queuesMonitored, totalQueuesAttempted, queuesExcluded, queuesModel, queuesUnauthorized, queuesFailed,
		channelsMonitored, totalChannelsAttempted, channelsExcluded, channelsUnauthorized, channelsFailed,
		estimatedPCFCommands)

	// Log detailed PCF command tracking
	c.logPCFTracking()
}

// logPCFTracking logs detailed PCF command success/failure breakdown
func (c *Collector) logPCFTracking() {
	// Log global request tracking (CompCode counts)
	for command, compCodes := range c.pcfTracker.requests {
		c.Debugf("%s REQUESTS: %v", command, compCodes)
	}

	// Log global reason tracking
	for command, reasons := range c.pcfTracker.reasons {
		reasonStrs := make(map[string]int)
		for reason, count := range reasons {
			reasonStrs[c.pcfTracker.reasonCodeToString(reason)] = count
		}
		c.Debugf("%s REASONS: %v", command, reasonStrs)
	}

	// Log array item reason tracking (only for array responses)
	for command, itemReasons := range c.pcfTracker.itemReasons {
		reasonStrs := make(map[string]int)
		for reason, count := range itemReasons {
			reasonStrs[c.pcfTracker.reasonCodeToString(reason)] = count
		}
		c.Debugf("%s ITEMS REASONS: %v", command, reasonStrs)
	}

	// Log array item CompCode tracking (only for array responses)
	for command, itemCodes := range c.pcfTracker.itemCodes {
		c.Debugf("%s ITEMS CODES: %v", command, itemCodes)
	}
}

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	collectStart := time.Now()
	c.Debugf("collect() called - connection status: %v", c.mqConn != nil && c.mqConn.connected)
	mx := make(map[string]int64)

	// Reset PCF tracking for this collection cycle
	c.pcfTracker = newPCFTracker()

	// Reset seen tracking for this collection cycle
	c.resetSeenTracking()

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
		if err := c.collectAllQueues(ctx, mx); err != nil {
			c.Warningf("failed to collect queue metrics: %v", err)
		}
		c.Debugf("Queue metrics collection took %v", time.Since(queueStart))
	}

	// Collect channel metrics (respect admin configuration)
	if c.CollectChannels != nil && *c.CollectChannels {
		channelStart := time.Now()
		if err := c.collectAllChannels(ctx, mx); err != nil {
			c.Warningf("failed to collect channel metrics: %v", err)
		}
		c.Debugf("Channel metrics collection took %v", time.Since(channelStart))
	}

	// Collect topic metrics (respect admin configuration)
	if c.CollectTopics != nil && *c.CollectTopics {
		topicStart := time.Now()
		if err := c.collectAllTopics(ctx, mx); err != nil {
			c.Warningf("failed to collect topic metrics: %v", err)
		}
		c.Debugf("Topic metrics collection took %v", time.Since(topicStart))
	}

	// Clean up obsolete charts for instances that no longer exist
	c.cleanupAbsentInstances()

	// Log comprehensive collection summary including PCF command tracking
	c.logCollectionSummary(mx, time.Since(collectStart))

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

	c.Infof("Creating MQ connection - this will create 2 new queues")

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
	cno.Options = C.MQCNO_CLIENT_BINDING | C.MQCNO_RECONNECT | C.MQCNO_HANDLE_SHARE_BLOCK

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
		return fmt.Errorf("MQCONNX failed: completion code %d, reason %d (%s) (check queue manager '%s' is running and accessible on %s:%d)",
			compCode, reason, mqReasonString(int32(reason)), c.QueueManager, c.Host, c.Port)
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
		return fmt.Errorf("MQOPEN failed: completion code %d, reason %d (%s) (check PCF permissions for SYSTEM.ADMIN.COMMAND.QUEUE)", openCompCode, openReason, mqReasonString(int32(openReason)))
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
		return fmt.Errorf("MQOPEN reply queue failed: completion code %d, reason %d (%s)", replyOpenCompCode, replyOpenReason, mqReasonString(int32(replyOpenReason)))
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
			c.Warningf("Failed to close reply queue: completion code %d, reason %d (%s)", compCode, reason, mqReasonString(int32(reason)))
		}
	}

	if c.mqConn.hObj != C.MQHO_UNUSABLE_HOBJ {
		C.MQCLOSE(c.mqConn.hConn, &c.mqConn.hObj, C.MQCO_NONE, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.Warningf("Failed to close admin queue: completion code %d, reason %d (%s)", compCode, reason, mqReasonString(int32(reason)))
		}
	}

	if c.mqConn.hConn != C.MQHC_UNUSABLE_HCONN {
		C.MQDISC(&c.mqConn.hConn, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.Warningf("Failed to disconnect: completion code %d, reason %d (%s)", compCode, reason, mqReasonString(int32(reason)))
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

// mqcmdToString returns a human-readable name for MQCMD command constants
func mqcmdToString(command C.MQLONG) string {
	switch command {
	case C.MQCMD_INQUIRE_Q_MGR:
		return "MQCMD_INQUIRE_Q_MGR"
	case C.MQCMD_INQUIRE_Q:
		return "MQCMD_INQUIRE_Q"
	case C.MQCMD_INQUIRE_Q_STATUS:
		return "MQCMD_INQUIRE_Q_STATUS"
	case C.MQCMD_INQUIRE_CHANNEL:
		return "MQCMD_INQUIRE_CHANNEL"
	case C.MQCMD_INQUIRE_CHANNEL_STATUS:
		return "MQCMD_INQUIRE_CHANNEL_STATUS"
	case C.MQCMD_INQUIRE_TOPIC:
		return "MQCMD_INQUIRE_TOPIC"
	case C.MQCMD_INQUIRE_TOPIC_STATUS:
		return "MQCMD_INQUIRE_TOPIC_STATUS"
	default:
		return fmt.Sprintf("MQCMD_%d", command)
	}
}

// mqReasonString returns a human-readable name for MQ reason codes
func mqReasonString(reason int32) string {
	switch reason {
	case 0:
		return "MQRC_NONE"
	case 2009:
		return "MQRC_CONNECTION_BROKEN"
	case 2018:
		return "MQRC_HCONN_ERROR"
	case 2033:
		return "MQRC_NO_MSG_AVAILABLE"
	case 2035:
		return "MQRC_NOT_AUTHORIZED"
	case 2058:
		return "MQRC_Q_MGR_NAME_ERROR"
	case 2059:
		return "MQRC_Q_MGR_NOT_AVAILABLE"
	case 2067:
		return "MQRC_OBJECT_OPEN_ERROR"
	case 2080:
		return "MQRC_TRUNCATED_MSG_FAILED"
	case 2085:
		return "MQRC_UNKNOWN_OBJECT_NAME"
	case 2538:
		return "MQRC_HOST_NOT_AVAILABLE"
	case 2540:
		return "MQRC_CHANNEL_CONFIG_ERROR"
	case 3008:
		return "MQRCCF_COMMAND_FAILED"
	case 3065:
		return "MQRCCF_CHANNEL_NOT_ACTIVE"
	default:
		return fmt.Sprintf("MQRC_%d", reason)
	}
}

func (c *Collector) sendPCFCommand(command C.MQLONG, parameters []pcfParameter) ([]byte, error) {
	// Validate connection before sending command
	if err := c.validateConnection(); err != nil {
		return nil, err
	}

	// Increment command count for debugging
	c.pcfCommandCount++

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

	// c.Debugf("Sending PCF command %d with %d parameters, message size %d", command, len(parameters), msgSize)

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
			c.Errorf("MQPUT failed with HCONN_ERROR for %s - connection handle is now invalid", mqcmdToString(command))
		}
		return nil, fmt.Errorf("MQPUT failed for %s: completion code %d, reason %d (%s)", mqcmdToString(command), compCode, reason, mqReasonString(int32(reason)))
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
		gmo.Options = C.MQGMO_WAIT | C.MQGMO_FAIL_IF_QUIESCING
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
				return nil, fmt.Errorf("MQGET length check failed: completion code %d, reason %d (%s)", compCode, reason, mqReasonString(int32(reason)))
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
				return nil, fmt.Errorf("MQGET failed: completion code %d, reason %d (%s)", compCode, reason, mqReasonString(int32(reason)))
			}
			// For subsequent messages, just return what we have
			break
		}

		// Check the PCF header
		cfh := (*C.MQCFH)(unsafe.Pointer(&buffer[0]))

		// Append this response to our collection
		allResponses = append(allResponses, buffer[:bufferLength]...)

		// If this is an error response (CompCode != 0) AND it's the last/only message,
		// we've collected the error - no need to wait for more
		if cfh.CompCode != C.MQCC_OK && cfh.Control == C.MQCFC_LAST {
			break
		}

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
	// Use MQCMD_INQUIRE_Q_MGR_STATUS to get actual Queue Manager status metrics
	// This is the proper PCF command for queue manager status information
	var attrs map[C.MQLONG]interface{}

	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q_MGR_STATUS, nil)
	if err == nil {
		attrs, err = c.parsePCFResponse(response, "MQCMD_INQUIRE_Q_MGR_STATUS")
	}

	// Command tracking is now handled automatically in parsePCFResponse
	if err != nil {
		c.Debugf("Queue manager status inquiry failed: %v", err)
		// Fallback to basic MQCMD_INQUIRE_Q_MGR for minimal status
		response, err = c.sendPCFCommand(C.MQCMD_INQUIRE_Q_MGR, nil)
		if err == nil {
			attrs, err = c.parsePCFResponse(response, "MQCMD_INQUIRE_Q_MGR")
		}
		if err != nil {
			c.Debugf("Queue manager basic inquiry also failed: %v", err)
			return nil
		}
	}

	// Set basic status (1 = running if we got any response)
	mx["qmgr_status"] = 1

	// Extract actual Queue Manager status metrics using real MQ constants
	// Note: These metrics depend on what the actual MQ version supports
	// We should examine the attrs map to see what's actually available

	// For now, just log what attributes we received to understand what's available
	if len(attrs) > 0 {
		c.Debugf("Queue manager status response contains %d attributes", len(attrs))
		// Log attributes to see what's actually available for proper metric collection
		count := 0
		for attr, value := range attrs {
			if count < 10 {
				c.Debugf("  Attribute %d = %v", attr, value)
				count++
			}
		}
	}

	return nil
}

func (c *Collector) collectAllQueues(ctx context.Context, mx map[string]int64) error {
	// Initialize overview metrics
	mx["queues_monitored"] = 0
	mx["queues_excluded"] = 0
	mx["queues_model"] = 0
	mx["queues_unauthorized"] = 0
	mx["queues_unknown"] = 0
	mx["queues_failed"] = 0

	// Skip if queue collection has been disabled (by user config or previous auth failure)
	if c.CollectQueues != nil && !*c.CollectQueues {
		return nil
	}

	// Get queue list with error details
	result, err := c.getQueueList(ctx)
	if err != nil {
		// Command tracking is now handled automatically in parseQueueListResponse
		return fmt.Errorf("failed to get queue list: %w", err)
	}

	// Command tracking is now handled automatically in parseQueueListResponse

	// Calculate total queues attempted
	totalQueues := len(result.Queues)
	// Use error counts instead of queue lists since error responses may not include queue names
	for _, count := range result.ErrorCounts {
		totalQueues += count
	}

	// Populate error overview metrics
	if authCount := result.ErrorCounts[2035]; authCount > 0 {
		mx["queues_unauthorized"] = int64(authCount)
	}
	if modelCount := result.ErrorCounts[2085]; modelCount > 0 {
		mx["queues_model"] = int64(modelCount)
	}
	// Sum other errors
	for code, count := range result.ErrorCounts {
		if code != 2035 && code != 2085 {
			mx["queues_failed"] += int64(count)
		}
	}

	// Log error summary if there were errors
	if len(result.ErrorCounts) > 0 {
		// Build error summary
		errorSummary := fmt.Sprintf("Queue discovery (MQCMD_INQUIRE_Q) errors: ")
		first := true
		for code, count := range result.ErrorCounts {
			if !first {
				errorSummary += ", "
			}
			errorSummary += fmt.Sprintf("%s (%d queues)", mqErrorString(code), count)
			first = false
		}
		c.Warningf(errorSummary)

		// Special handling for model queue errors (2085 = MQRC_UNKNOWN_OBJECT_NAME)
		if modelQueueErrors := result.ErrorCounts[2085]; modelQueueErrors > 0 {
			c.Debugf("Note: %d model queues don't have status (this is expected)", modelQueueErrors)
		}
	}

	// Check if ALL queues failed with authorization errors
	authErrors := result.ErrorCounts[2035] // MQRC_NOT_AUTHORIZED
	if authErrors > 0 && authErrors == totalQueues {
		c.Warningf("All %d queues returned NOT_AUTHORIZED - disabling queue collection", totalQueues)
		// Disable queue collection for future runs
		disableQueues := false
		c.CollectQueues = &disableQueues
		return nil // Don't fail entire collection
	}

	c.Debugf("Found %d successful queues out of %d total", len(result.Queues), totalQueues)

	// Process successful queues
	collected := 0
	excluded := 0
	for _, queueName := range result.Queues {
		if !c.shouldCollectQueue(queueName) {
			excluded++
			continue
		}
		collected++

		// Mark as seen for cleanup tracking

		cleanName := c.cleanName(queueName)

		// Collect data first, then create charts based on what we successfully collected
		c.collectQueueMetrics(ctx, queueName, cleanName, mx)
	}

	// Update overview metrics
	mx["queues_monitored"] = int64(collected)
	mx["queues_excluded"] = int64(excluded)

	c.Debugf("Monitoring %d queues (filtered from %d successful queues)", collected, len(result.Queues))
	return nil
}

// collectQueueConfigMetrics collects queue configuration attributes using MQCMD_INQUIRE_Q
func (c *Collector) collectQueueConfigMetrics(ctx context.Context, queueName, cleanName string, mx map[string]int64) error {
	// Send INQUIRE_Q for specific queue to get configuration attributes
	// Note: We request ALL attributes by not specifying an attribute list
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, queueName),
	}

	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		return fmt.Errorf("failed to send MQCMD_INQUIRE_Q for %s: %w", queueName, err)
	}

	// Parse response
	attrs, err := c.parsePCFResponse(response, "MQCMD_INQUIRE_Q")
	if err != nil {
		return fmt.Errorf("failed to parse queue config response for %s: %w", queueName, err)
	}

	// Check for MQ errors in the response
	if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
		if reason, ok := reasonCode.(int32); ok && reason != 0 {
			return fmt.Errorf("MQCMD_INQUIRE_Q failed: reason %d (%s)", reason, mqReasonString(reason))
		}
	}

	// Log what attributes we got (temporary debug)
	attrCount := 0
	hasEnqCount := false
	hasDeqCount := false
	for attrKey := range attrs {
		attrCount++
		if attrKey == C.MQIA_MSG_ENQ_COUNT {
			hasEnqCount = true
		}
		if attrKey == C.MQIA_MSG_DEQ_COUNT {
			hasDeqCount = true
		}
	}
	c.Debugf("MQCMD_INQUIRE_Q for %s returned %d attributes (has ENQ_COUNT=%v, has DEQ_COUNT=%v)",
		queueName, attrCount, hasEnqCount, hasDeqCount)

	// Extract configuration metrics
	if maxDepth, ok := attrs[C.MQIA_MAX_Q_DEPTH]; ok {
		mx[fmt.Sprintf("queue_%s_max_depth", cleanName)] = int64(maxDepth.(int32))
	}

	if backoutThreshold, ok := attrs[C.MQIA_BACKOUT_THRESHOLD]; ok {
		mx[fmt.Sprintf("queue_%s_backout_threshold", cleanName)] = int64(backoutThreshold.(int32))
	}

	if triggerDepth, ok := attrs[C.MQIA_TRIGGER_DEPTH]; ok {
		mx[fmt.Sprintf("queue_%s_trigger_depth", cleanName)] = int64(triggerDepth.(int32))
	}

	if inhibitGet, ok := attrs[C.MQIA_INHIBIT_GET]; ok {
		mx[fmt.Sprintf("queue_%s_inhibit_get", cleanName)] = int64(inhibitGet.(int32))
	}

	if inhibitPut, ok := attrs[C.MQIA_INHIBIT_PUT]; ok {
		mx[fmt.Sprintf("queue_%s_inhibit_put", cleanName)] = int64(inhibitPut.(int32))
	}

	if defPriority, ok := attrs[C.MQIA_DEF_PRIORITY]; ok {
		mx[fmt.Sprintf("queue_%s_def_priority", cleanName)] = int64(defPriority.(int32))
	}

	if defPersistence, ok := attrs[C.MQIA_DEF_PERSISTENCE]; ok {
		mx[fmt.Sprintf("queue_%s_def_persistence", cleanName)] = int64(defPersistence.(int32))
	}

	// Extract depth event thresholds and peak metrics
	if depthHighLimit, ok := attrs[C.MQIA_Q_DEPTH_HIGH_LIMIT]; ok {
		mx[fmt.Sprintf("queue_%s_depth_high_limit", cleanName)] = int64(depthHighLimit.(int32))
	}

	if depthLowLimit, ok := attrs[C.MQIA_Q_DEPTH_LOW_LIMIT]; ok {
		mx[fmt.Sprintf("queue_%s_depth_low_limit", cleanName)] = int64(depthLowLimit.(int32))
	}

	if highQDepth, ok := attrs[C.MQIA_HIGH_Q_DEPTH]; ok {
		mx[fmt.Sprintf("queue_%s_high_q_depth", cleanName)] = int64(highQDepth.(int32))
	}

	// Extract message counters if available from MQCMD_INQUIRE_Q
	// These may only be available when statistics are enabled
	if enqueued, ok := attrs[C.MQIA_MSG_ENQ_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_enqueued", cleanName)] = int64(enqueued.(int32))
	}

	if dequeued, ok := attrs[C.MQIA_MSG_DEQ_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_dequeued", cleanName)] = int64(dequeued.(int32))
	}

	// Extract open handles if available from MQCMD_INQUIRE_Q (unlikely to be present)
	if openInputCount, ok := attrs[C.MQIA_OPEN_INPUT_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_open_input_count", cleanName)] = int64(openInputCount.(int32))
	}

	if openOutputCount, ok := attrs[C.MQIA_OPEN_OUTPUT_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_open_output_count", cleanName)] = int64(openOutputCount.(int32))
	}

	// Collect runtime status using MQCMD_INQUIRE_Q_STATUS
	if err := c.collectQueueRuntimeMetrics(ctx, mx, queueName, cleanName); err != nil {
		// Log but don't fail the entire collection
		c.Debugf("Failed to collect runtime metrics for queue %s: %v", queueName, err)
	}

	// Collect reset statistics if enabled (DESTRUCTIVE operation)
	if err := c.collectQueueResetStats(ctx, mx, queueName, cleanName); err != nil {
		// Log but don't fail the entire collection
		c.Warningf("Failed to collect reset stats for queue %s: %v", queueName, err)
	}

	return nil
}

// collectQueueRuntimeMetrics uses MQCMD_INQUIRE_Q_STATUS to get runtime metrics
// that are not available from MQCMD_INQUIRE_Q (open counts, last put/get times)
func (c *Collector) collectQueueRuntimeMetrics(ctx context.Context, mx map[string]int64, queueName, cleanName string) error {
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, queueName),
		newIntParameter(C.MQIACF_Q_STATUS_TYPE, C.MQIACF_Q_STATUS), // Request general queue status
	}

	// Send MQCMD_INQUIRE_Q_STATUS command
	resp, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q_STATUS, params)
	if err != nil {
		// Queue status is often not available if no processes have the queue open
		// This is not an error condition, just means no runtime data available
		return nil
	}

	// Parse response attributes
	attrs, err := c.parsePCFResponse(resp, "MQCMD_INQUIRE_Q_STATUS")
	if err != nil {
		return fmt.Errorf("failed to parse queue status response: %w", err)
	}

	// Extract runtime metrics that are only available from INQUIRE_Q_STATUS
	if openInputCount, ok := attrs[C.MQIA_OPEN_INPUT_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_open_input_count", cleanName)] = int64(openInputCount.(int32))
	}

	if openOutputCount, ok := attrs[C.MQIA_OPEN_OUTPUT_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_open_output_count", cleanName)] = int64(openOutputCount.(int32))
	}

	// Note: MSG_ENQ_COUNT and MSG_DEQ_COUNT are not available from INQUIRE_Q_STATUS
	// They require MQCMD_RESET_Q_STATS which is destructive (resets the counters)
	// For monitoring purposes, we avoid using RESET_Q_STATS to prevent interfering
	// with other monitoring tools or applications that rely on these statistics

	return nil
}

// collectQueueResetStats uses MQCMD_RESET_Q_STATS to get message counters
// WARNING: This is a destructive operation that resets counters to zero!
func (c *Collector) collectQueueResetStats(ctx context.Context, mx map[string]int64, queueName, cleanName string) error {
	// Only collect if explicitly enabled by user
	if c.CollectResetQueueStats == nil || !*c.CollectResetQueueStats {
		return nil
	}

	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, queueName),
	}

	// Send MQCMD_RESET_Q_STATS command
	resp, err := c.sendPCFCommand(C.MQCMD_RESET_Q_STATS, params)
	if err != nil {
		// Statistics might not be available (STATQ not enabled)
		c.Debugf("Failed to reset queue stats for %s: %v", queueName, err)
		return nil
	}

	// Parse response attributes
	attrs, err := c.parsePCFResponse(resp, "MQCMD_RESET_Q_STATS")
	if err != nil {
		return fmt.Errorf("failed to parse reset stats response: %w", err)
	}

	// Extract message counters
	if enqueued, ok := attrs[C.MQIA_MSG_ENQ_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_enqueued", cleanName)] = int64(enqueued.(int32))
	}

	if dequeued, ok := attrs[C.MQIA_MSG_DEQ_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_dequeued", cleanName)] = int64(dequeued.(int32))
	}

	// Extract peak depth since last reset
	if highQDepth, ok := attrs[C.MQIA_HIGH_Q_DEPTH]; ok {
		mx[fmt.Sprintf("queue_%s_high_q_depth", cleanName)] = int64(highQDepth.(int32))
	}

	// Log time since reset for debugging
	if timeSinceReset, ok := attrs[C.MQIA_TIME_SINCE_RESET]; ok {
		seconds := int64(timeSinceReset.(int32))
		if seconds > 0 {
			c.Debugf("Queue %s stats reset after %d seconds", queueName, seconds)
		}
	}

	return nil
}

func (c *Collector) collectAllChannels(ctx context.Context, mx map[string]int64) error {
	// Initialize overview metrics
	mx["channels_monitored"] = 0
	mx["channels_excluded"] = 0
	mx["channels_unauthorized"] = 0
	mx["channels_failed"] = 0

	// Skip if channel collection has been disabled (by user config or previous auth failure)
	if c.CollectChannels != nil && !*c.CollectChannels {
		return nil
	}

	// Get channel list with error details
	result, err := c.getChannelList(ctx)
	if err != nil {
		return fmt.Errorf("failed to get channel list: %w", err)
	}

	// Calculate total channels attempted
	totalChannels := len(result.Channels)
	// Use error counts instead of channel lists since error responses may not include channel names
	for _, count := range result.ErrorCounts {
		totalChannels += count
	}

	// Populate error overview metrics
	if authCount := result.ErrorCounts[2035]; authCount > 0 {
		mx["channels_unauthorized"] = int64(authCount)
	}
	// Sum other errors
	for code, count := range result.ErrorCounts {
		if code != 2035 {
			mx["channels_failed"] += int64(count)
		}
	}

	// Log error summary if there were errors
	if len(result.ErrorCounts) > 0 {
		// Build error summary
		errorSummary := fmt.Sprintf("Channel discovery (MQCMD_INQUIRE_CHANNEL) errors: ")
		first := true
		for code, count := range result.ErrorCounts {
			if !first {
				errorSummary += ", "
			}
			errorSummary += fmt.Sprintf("%s (%d channels)", mqErrorString(code), count)
			first = false
		}
		c.Warningf(errorSummary)
	}

	// Check if ALL channels failed with authorization errors
	authErrors := result.ErrorCounts[2035] // MQRC_NOT_AUTHORIZED
	if authErrors > 0 && authErrors == totalChannels {
		c.Warningf("All %d channels returned NOT_AUTHORIZED - disabling channel collection", totalChannels)
		// Disable channel collection for future runs
		disableChannels := false
		c.CollectChannels = &disableChannels
		return nil // Don't fail entire collection
	}

	c.Debugf("Found %d successful channels out of %d total", len(result.Channels), totalChannels)

	// Process successful channels
	collected := 0
	excluded := 0
	for _, channelName := range result.Channels {
		if !c.shouldCollectChannel(channelName) {
			excluded++
			continue
		}
		collected++

		// Mark as seen for cleanup tracking

		cleanName := c.cleanName(channelName)

		// Collect data first, then create charts based on what we successfully collected
		c.collectChannelMetrics(ctx, channelName, cleanName, mx)
	}

	// Update overview metrics
	mx["channels_monitored"] = int64(collected)
	mx["channels_excluded"] = int64(excluded)

	c.Debugf("Monitoring %d channels (filtered from %d successful channels)", collected, len(result.Channels))
	return nil
}

func (c *Collector) collectAllTopics(ctx context.Context, mx map[string]int64) error {
	// Initialize overview metrics
	mx["topics_monitored"] = 0
	mx["topics_excluded"] = 0
	mx["topics_unauthorized"] = 0
	mx["topics_failed"] = 0

	// Get list of topics
	topics, err := c.getTopicList(ctx)
	if err != nil {
		// Check if it's an authorization error
		if strings.Contains(err.Error(), "2035") {
			mx["topics_unauthorized"] = 1
		} else {
			mx["topics_failed"] = 1
		}
		return fmt.Errorf("failed to get topic list: %w", err)
	}

	c.Debugf("Found %d topics", len(topics))

	collected := 0
	excluded := 0
	for _, topic := range topics {
		if !c.shouldCollectTopic(topic.Name) {
			excluded++
			continue
		}
		collected++

		cleanName := c.cleanName(topic.Name)

		// Collect data first, then create charts based on what we successfully collected
		c.collectTopicMetrics(ctx, topic.Name, topic.TopicString, cleanName, mx)
	}

	// Update overview metrics
	mx["topics_monitored"] = int64(collected)
	mx["topics_excluded"] = int64(excluded)

	c.Debugf("Monitoring %d out of %d topics", collected, len(topics))
	return nil
}

func (c *Collector) getQueueList(ctx context.Context) (*QueueParseResult, error) {
	// Use MQCMD_INQUIRE_Q with wildcard queue name
	// This is the standard way to get a list of all queues
	c.Debugf("Sending MQCMD_INQUIRE_Q with Q_NAME='*'")
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, "*"),
	}
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		return nil, fmt.Errorf("queue discovery failed: %w", err)
	}

	// Parse response and return structured result
	result := c.parseQueueListResponse(response, "MQCMD_INQUIRE_Q")
	return result, nil
}

func (c *Collector) getChannelList(ctx context.Context) (*ChannelParseResult, error) {
	// Use MQCMD_INQUIRE_CHANNEL with wildcard channel name
	// This is the standard way to get a list of all channels
	c.Debugf("Sending MQCMD_INQUIRE_CHANNEL with CHANNEL_NAME='*'")
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, "*"),
	}
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_CHANNEL, params)
	if err != nil {
		return nil, fmt.Errorf("channel discovery failed: %w", err)
	}

	// Parse response and return structured result
	result := c.parseChannelListResponse(response, "MQCMD_INQUIRE_CHANNEL")
	return result, nil
}

func (c *Collector) shouldCollectQueue(queueName string) bool {
	// Check system queue collection setting
	if strings.HasPrefix(queueName, "SYSTEM.") {
		if c.CollectSystemQueues == nil || !*c.CollectSystemQueues {
			c.Debugf("Skipping system queue (disabled): %s", queueName)
			return false
		}
		// c.Debugf("Including system queue (enabled): %s", queueName)
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
		// c.Debugf("Including system channel (enabled): %s", channelName)
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

// collectChannelConfigMetrics collects channel configuration attributes using MQCMD_INQUIRE_CHANNEL
func (c *Collector) collectChannelConfigMetrics(ctx context.Context, channelName, cleanName string, mx map[string]int64) error {
	// Send INQUIRE_CHANNEL for specific channel to get configuration attributes
	// Note: Unlike queue inquiries, channel inquiries don't use an attribute selector.
	// When you inquire on a specific channel by name, MQ returns all available attributes.
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, channelName),
	}

	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_CHANNEL, params)
	if err != nil {
		return fmt.Errorf("failed to send MQCMD_INQUIRE_CHANNEL for %s: %w", channelName, err)
	}

	// Parse response
	attrs, err := c.parsePCFResponse(response, "MQCMD_INQUIRE_CHANNEL")
	if err != nil {
		return fmt.Errorf("failed to parse channel config response for %s: %w", channelName, err)
	}

	// Check for MQ errors in the response
	if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
		if reason, ok := reasonCode.(int32); ok && reason != 0 {
			return fmt.Errorf("MQCMD_INQUIRE_CHANNEL failed: reason %d (%s)", reason, mqReasonString(reason))
		}
	}

	// Extract configuration metrics
	if batchSize, ok := attrs[C.MQIACH_BATCH_SIZE]; ok {
		mx[fmt.Sprintf("channel_%s_batch_size", cleanName)] = int64(batchSize.(int32))
	}

	if batchInterval, ok := attrs[C.MQIACH_BATCH_INTERVAL]; ok {
		mx[fmt.Sprintf("channel_%s_batch_interval", cleanName)] = int64(batchInterval.(int32)) * precision
	}

	if discInterval, ok := attrs[C.MQIACH_DISC_INTERVAL]; ok {
		mx[fmt.Sprintf("channel_%s_disc_interval", cleanName)] = int64(discInterval.(int32)) * precision
	}

	if hbInterval, ok := attrs[C.MQIACH_HB_INTERVAL]; ok {
		mx[fmt.Sprintf("channel_%s_hb_interval", cleanName)] = int64(hbInterval.(int32)) * precision
	}

	if keepAliveInterval, ok := attrs[C.MQIACH_KEEP_ALIVE_INTERVAL]; ok {
		mx[fmt.Sprintf("channel_%s_keep_alive_interval", cleanName)] = int64(keepAliveInterval.(int32)) * precision
	}

	if shortRetry, ok := attrs[C.MQIACH_SHORT_RETRY]; ok {
		mx[fmt.Sprintf("channel_%s_short_retry", cleanName)] = int64(shortRetry.(int32))
	}

	if shortTimer, ok := attrs[C.MQIACH_SHORT_TIMER]; ok {
		mx[fmt.Sprintf("channel_%s_short_timer", cleanName)] = int64(shortTimer.(int32)) * precision
	}

	if longRetry, ok := attrs[C.MQIACH_LONG_RETRY]; ok {
		mx[fmt.Sprintf("channel_%s_long_retry", cleanName)] = int64(longRetry.(int32))
	}

	if longTimer, ok := attrs[C.MQIACH_LONG_TIMER]; ok {
		mx[fmt.Sprintf("channel_%s_long_timer", cleanName)] = int64(longTimer.(int32)) * precision
	}

	if maxMsgLength, ok := attrs[C.MQIACH_MAX_MSG_LENGTH]; ok {
		mx[fmt.Sprintf("channel_%s_max_msg_length", cleanName)] = int64(maxMsgLength.(int32))
	}

	if sharingConv, ok := attrs[C.MQIACH_SHARING_CONVERSATIONS]; ok {
		mx[fmt.Sprintf("channel_%s_sharing_conversations", cleanName)] = int64(sharingConv.(int32))
	}

	if netPriority, ok := attrs[C.MQIACH_NETWORK_PRIORITY]; ok {
		mx[fmt.Sprintf("channel_%s_network_priority", cleanName)] = int64(netPriority.(int32))
	}

	return nil
}

func (c *Collector) getTopicList(ctx context.Context) ([]TopicInfo, error) {
	// Send INQUIRE_TOPIC command with generic topic name
	params := []pcfParameter{
		newStringParameter(C.MQCA_TOPIC_NAME, "*"),
	}

	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_TOPIC, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send MQCMD_INQUIRE_TOPIC: %w", err)
	}

	// Parse topic names from response
	topics, err := c.parseTopicListResponse(response, "MQCMD_INQUIRE_TOPIC")
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

func (c *Collector) detectVersion(ctx context.Context) error {
	// Send PCF command to inquire queue manager
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q_MGR, nil)
	if err != nil {
		return fmt.Errorf("failed to send MQCMD_INQUIRE_Q_MGR: %w", err)
	}

	// Parse response
	attrs, err := c.parsePCFResponse(response, "MQCMD_INQUIRE_Q_MGR")
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
