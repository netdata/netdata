// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package mq_pcf

// #include <cmqc.h>
// #include <cmqcfc.h>
// #include <string.h>
// #include <stdlib.h>
//
// // Helper functions to work around CGO type issues
// void set_object_name(MQOD* od, const char* name) {
//     strncpy(od->ObjectName, name, sizeof(od->ObjectName));
// }
//
// void set_format(MQMD* md, const char* format) {
//     strncpy(md->Format, format, sizeof(md->Format));
// }
//
// void copy_msg_id(MQMD* dest, MQMD* src) {
//     memcpy(dest->CorrelId, src->MsgId, sizeof(src->MsgId));
// }
//
import "C"

import (
	"context"
	"fmt"
	"strings"
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
	connected  bool
}

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	mx := make(map[string]int64)
	
	// Connect to queue manager if not connected
	if err := c.ensureConnection(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to queue manager: %w", err)
	}
	
	// Detect version on first connection
	if c.version == "" {
		if err := c.detectVersion(ctx); err != nil {
			c.Warningf("failed to detect MQ version: %v", err)
		} else {
			c.addVersionLabelsToCharts()
		}
	}
	
	// Collect queue manager metrics
	if err := c.collectQueueManagerMetrics(ctx, mx); err != nil {
		c.Warningf("failed to collect queue manager metrics: %v", err)
	}
	
	// Collect queue metrics (respect admin configuration)
	if c.conf.CollectQueues != nil && *c.conf.CollectQueues {
		if err := c.collectQueueMetrics(ctx, mx); err != nil {
			c.Warningf("failed to collect queue metrics: %v", err)
		}
	}
	
	// Collect channel metrics (respect admin configuration)
	if c.conf.CollectChannels != nil && *c.conf.CollectChannels {
		if err := c.collectChannelMetrics(ctx, mx); err != nil {
			c.Warningf("failed to collect channel metrics: %v", err)
		}
	}
	
	// Collect topic metrics (respect admin configuration)
	if c.conf.CollectTopics != nil && *c.conf.CollectTopics {
		if err := c.collectTopicMetrics(ctx, mx); err != nil {
			c.Warningf("failed to collect topic metrics: %v", err)
		}
	}
	
	return mx, nil
}

func (c *Collector) ensureConnection(ctx context.Context) error {
	if c.mqConn != nil && c.mqConn.connected {
		return nil // Already connected
	}
	
	// Clean up any existing connection
	if c.mqConn != nil {
		c.disconnect()
	}
	
	c.mqConn = &mqConnection{}
	
	// Build connection string: HOST(port)/CHANNEL/QueueManager
	connStr := fmt.Sprintf("%s(%d)/%s/%s", c.conf.Host, c.conf.Port, c.conf.Channel, c.conf.QueueManager)
	cConnStr := C.CString(connStr)
	defer C.free(unsafe.Pointer(cConnStr))
	
	var compCode C.MQLONG
	var reason C.MQLONG
	
	// Connect to queue manager
	C.MQCONN(cConnStr, &c.mqConn.hConn, &compCode, &reason)
	
	if compCode != C.MQCC_OK {
		return fmt.Errorf("MQCONN failed: completion code %d, reason code %d (check queue manager '%s' is running and accessible on %s:%d)", 
			compCode, reason, c.conf.QueueManager, c.conf.Host, c.conf.Port)
	}
	
	// Open system command input queue for PCF commands
	var od C.MQOD
	C.memset(unsafe.Pointer(&od), 0, C.sizeof_MQOD)
	od.Version = C.MQOD_VERSION_1
	C.set_object_name(&od, C.CString("SYSTEM.ADMIN.COMMAND.QUEUE"))
	od.ObjectType = C.MQOT_Q
	
	var openOptions C.MQLONG = C.MQOO_OUTPUT | C.MQOO_FAIL_IF_QUIESCING
	
	C.MQOPEN(c.mqConn.hConn, C.PMQVOID(unsafe.Pointer(&od)), openOptions, &c.mqConn.hObj, &compCode, &reason)
	
	if compCode != C.MQCC_OK {
		C.MQDISC(&c.mqConn.hConn, &compCode, &reason)
		return fmt.Errorf("MQOPEN failed: completion code %d, reason code %d (check PCF permissions for SYSTEM.ADMIN.COMMAND.QUEUE)", compCode, reason)
	}
	
	c.mqConn.connected = true
	c.Infof("Successfully connected to queue manager %s on %s:%d via channel %s", 
		c.conf.QueueManager, c.conf.Host, c.conf.Port, c.conf.Channel)
	
	return nil
}

func (c *Collector) disconnect() {
	if c.mqConn == nil {
		return
	}
	
	var compCode C.MQLONG
	var reason C.MQLONG
	
	if c.mqConn.hObj != C.MQHO_UNUSABLE_HOBJ {
		C.MQCLOSE(c.mqConn.hConn, &c.mqConn.hObj, C.MQCO_NONE, &compCode, &reason)
	}
	
	if c.mqConn.hConn != C.MQHC_UNUSABLE_HCONN {
		C.MQDISC(&c.mqConn.hConn, &compCode, &reason)
		c.Debugf("Disconnected from queue manager %s", c.conf.QueueManager)
	}
	
	c.mqConn.connected = false
	c.mqConn = nil
}

func (c *Collector) sendPCFCommand(command C.MQLONG, parameters []pcfParameter) ([]byte, error) {
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
	cfh.Version = C.MQCFH_VERSION_3
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
	md.Version = C.MQMD_VERSION_2
	formatStr := C.CString("MQADMIN ")
	defer C.free(unsafe.Pointer(formatStr))
	C.set_format(&md, formatStr)
	md.MsgType = C.MQMT_REQUEST
	md.Expiry = 60 // 60 seconds expiry
	
	var pmo C.MQPMO
	C.memset(unsafe.Pointer(&pmo), 0, C.sizeof_MQPMO)
	pmo.Version = C.MQPMO_VERSION_2
	pmo.Options = C.MQPMO_NEW_MSG_ID | C.MQPMO_NEW_CORREL_ID | C.MQPMO_FAIL_IF_QUIESCING
	
	var compCode C.MQLONG
	var reason C.MQLONG
	
	C.MQPUT(c.mqConn.hConn, c.mqConn.hObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&pmo)), C.MQLONG(msgSize), C.PMQVOID(msgBuffer), &compCode, &reason)
	
	if compCode != C.MQCC_OK {
		return nil, fmt.Errorf("MQPUT failed: completion code %d, reason code %d", compCode, reason)
	}
	
	// Get response from reply queue
	return c.getPCFResponse(&md)
}

func (c *Collector) getPCFResponse(requestMd *C.MQMD) ([]byte, error) {
	// Open system command reply queue
	var od C.MQOD
	C.memset(unsafe.Pointer(&od), 0, C.sizeof_MQOD)
	od.Version = C.MQOD_VERSION_1
	C.set_object_name(&od, C.CString("SYSTEM.ADMIN.COMMAND.REPLY.MODEL"))
	od.ObjectType = C.MQOT_Q
	
	var hReplyObj C.MQHOBJ
	var openOptions C.MQLONG = C.MQOO_INPUT_SHARED | C.MQOO_FAIL_IF_QUIESCING
	var compCode C.MQLONG
	var reason C.MQLONG
	
	C.MQOPEN(c.mqConn.hConn, C.PMQVOID(unsafe.Pointer(&od)), openOptions, &hReplyObj, &compCode, &reason)
	if compCode != C.MQCC_OK {
		return nil, fmt.Errorf("MQOPEN reply queue failed: completion code %d, reason code %d", compCode, reason)
	}
	defer C.MQCLOSE(c.mqConn.hConn, &hReplyObj, C.MQCO_NONE, &compCode, &reason)
	
	// Get message
	var md C.MQMD
	C.memset(unsafe.Pointer(&md), 0, C.sizeof_MQMD)
	md.Version = C.MQMD_VERSION_2
	C.copy_msg_id(&md, requestMd)
	
	var gmo C.MQGMO
	C.memset(unsafe.Pointer(&gmo), 0, C.sizeof_MQGMO)
	gmo.Version = C.MQGMO_VERSION_2
	gmo.Options = C.MQGMO_WAIT | C.MQGMO_FAIL_IF_QUIESCING | C.MQGMO_CONVERT
	gmo.WaitInterval = 5000 // 5 seconds timeout
	
	// Get message length first
	var bufferLength C.MQLONG = 0
	C.MQGET(c.mqConn.hConn, hReplyObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), bufferLength, nil, &bufferLength, &compCode, &reason)
	
	if reason != C.MQRC_TRUNCATED_MSG_FAILED {
		return nil, fmt.Errorf("MQGET length check failed: completion code %d, reason code %d", compCode, reason)
	}
	
	// Defensive check: prevent excessive memory allocation
	if bufferLength > maxMQGetBufferSize {
		return nil, fmt.Errorf("PCF response too large (%d bytes), maximum allowed is %d bytes", bufferLength, maxMQGetBufferSize)
	}
	
	// Allocate buffer and get actual message
	buffer := make([]byte, bufferLength)
	C.MQGET(c.mqConn.hConn, hReplyObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), bufferLength, C.PMQVOID(unsafe.Pointer(&buffer[0])), &bufferLength, &compCode, &reason)
	
	if compCode != C.MQCC_OK {
		return nil, fmt.Errorf("MQGET failed: completion code %d, reason code %d", compCode, reason)
	}
	
	return buffer[:bufferLength], nil
}

func (c *Collector) collectQueueManagerMetrics(ctx context.Context, mx map[string]int64) error {
	// Send PCF command to inquire queue manager status
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q_MGR_STATUS, nil)
	if err != nil {
		return fmt.Errorf("failed to send INQUIRE_Q_MGR_STATUS: %w", err)
	}
	
	// Parse response
	attrs, err := c.parsePCFResponse(response)
	if err != nil {
		return fmt.Errorf("failed to parse queue manager status response: %w", err)
	}
	
	// Extract metrics - set basic status (1 = running, 0 = unknown)
	mx["qmgr_status"] = 1
	
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
	
	// Log successful collection for debugging
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
		return fmt.Errorf("failed to get channel list: %w", err)
	}
	
	c.Debugf("Found %d channels", len(channels))
	
	collected := 0
	for _, channelName := range channels {
		if !c.shouldCollectChannel(channelName) {
			continue
		}
		collected++
		
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
	// Send INQUIRE_Q command with generic queue name
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, "*"),
	}
	
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send INQUIRE_Q: %w", err)
	}
	
	// Parse queue names from response
	queues, err := c.parseQueueListResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse queue list response: %w", err)
	}
	
	return queues, nil
}

func (c *Collector) getChannelList(ctx context.Context) ([]string, error) {
	// Send INQUIRE_CHANNEL command with generic channel name
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, "*"),
	}
	
	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_CHANNEL, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send INQUIRE_CHANNEL: %w", err)
	}
	
	// Parse channel names from response
	channels, err := c.parseChannelListResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse channel list response: %w", err)
	}
	
	return channels, nil
}

func (c *Collector) shouldCollectQueue(queueName string) bool {
	// Skip system queues by default
	if strings.HasPrefix(queueName, "SYSTEM.") {
		c.Debugf("Skipping system queue: %s", queueName)
		return false
	}
	
	// Apply queue selector if configured
	if c.queueSelectorRegex != nil {
		matches := c.queueSelectorRegex.MatchString(queueName)
		if !matches {
			c.Debugf("Queue %s does not match selector pattern '%s', skipping", queueName, c.conf.QueueSelector)
		}
		return matches
	}
	
	return true
}

func (c *Collector) shouldCollectChannel(channelName string) bool {
	// Skip system channels by default
	if strings.HasPrefix(channelName, "SYSTEM.") {
		c.Debugf("Skipping system channel: %s", channelName)
		return false
	}
	
	// Apply channel selector if configured
	if c.channelSelectorRegex != nil {
		matches := c.channelSelectorRegex.MatchString(channelName)
		if !matches {
			c.Debugf("Channel %s does not match selector pattern '%s', skipping", channelName, c.conf.ChannelSelector)
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
	
	// Extract metrics
	if depth, ok := attrs[C.MQIA_CURRENT_Q_DEPTH]; ok {
		mx[fmt.Sprintf("queue_%s_depth", cleanName)] = int64(depth.(int32))
	}
	
	if enqueued, ok := attrs[C.MQIA_MSG_ENQ_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_enqueued", cleanName)] = int64(enqueued.(int32))
	}
	
	if dequeued, ok := attrs[C.MQIA_MSG_DEQ_COUNT]; ok {
		mx[fmt.Sprintf("queue_%s_dequeued", cleanName)] = int64(dequeued.(int32))
	}
	
	// Extract oldest message age if available (seconds)
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