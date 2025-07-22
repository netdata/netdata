// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

// ResourceMonitor manages the IBM MQ resource publication monitoring system
type ResourceMonitor struct {
	client        *Client
	subscriptions []C.MQHOBJ // Slice of subscription handles for cleanup
	metrics       *DiscoveredMetrics
}

// MonitoringClass represents a monitoring class discovered from metadata
type MonitoringClass struct {
	ID          C.MQLONG // MQIAMO_MONITOR_CLASS
	Name        string   // MQCAMO_MONITOR_CLASS
	Description string   // MQCAMO_MONITOR_DESC
	TypesTopic  string   // MQCA_TOPIC_STRING - topic to discover types
}

// MonitoringType represents a monitoring type within a class
type MonitoringType struct {
	ID          C.MQLONG // MQIAMO_MONITOR_TYPE
	Name        string   // MQCAMO_MONITOR_TYPE
	Description string   // MQCAMO_MONITOR_DESC
	ElementTopic string  // Topic to discover elements
	DataTopic   string   // Topic for actual data publications
}

// MonitoringElement represents a monitoring element within a type
type MonitoringElement struct {
	ID          C.MQLONG // MQIAMO_MONITOR_ELEMENT
	DataType    C.MQLONG // MQIAMO_MONITOR_DATATYPE
	Description string   // MQCAMO_MONITOR_DESC
}

// DiscoveredMetrics contains the complete hierarchy of discovered metrics
type DiscoveredMetrics struct {
	Classes map[string]*MonitoringClass
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor(client *Client) *ResourceMonitor {
	return &ResourceMonitor{
		client:        client,
		subscriptions: make([]C.MQHOBJ, 0),
	}
}

// Initialize sets up the resource monitoring system
func (rm *ResourceMonitor) Initialize() error {
	rm.client.protocol.Debugf("Initializing resource monitor for queue manager '%s'", rm.client.config.QueueManager)
	
	// Initialize subscriptions slice for cleanup
	rm.subscriptions = make([]C.MQHOBJ, 0)
	
	// Discover available metrics
	rm.client.protocol.Debugf("Discovering available resource metrics")
	if err := rm.discoverMetrics(); err != nil {
		return fmt.Errorf("failed to discover metrics: %w", err)
	}
	
	return nil
}

// discoverMetrics discovers available resource monitoring metrics
func (rm *ResourceMonitor) discoverMetrics() error {
	rm.client.protocol.Debugf("Discovering available resource metrics for queue manager '%s'", rm.client.config.QueueManager)
	
	// Step 1: Discover Classes
	classesTopic := fmt.Sprintf("$SYS/MQ/INFO/QMGR/%s/Monitor/METADATA/CLASSES", rm.client.config.QueueManager)
	classes, err := rm.discoverClasses(classesTopic)
	if err != nil {
		return fmt.Errorf("failed to discover monitoring classes: %w", err)
	}
	
	rm.client.protocol.Debugf("Discovered %d monitoring classes", len(classes))
	
	// Step 2: For each class, discover types
	for _, class := range classes {
		types, err := rm.discoverTypes(class)
		if err != nil {
			rm.client.protocol.Warningf("Failed to discover types for class %s: %v", class.Name, err)
			continue
		}
		
		rm.client.protocol.Debugf("Discovered %d types for class %s", len(types), class.Name)
		
		// Step 3: For each type, discover elements
		for _, typ := range types {
			elements, err := rm.discoverElements(class, typ)
			if err != nil {
				rm.client.protocol.Warningf("Failed to discover elements for type %s: %v", typ.Name, err)
				continue
			}
			
			rm.client.protocol.Debugf("Discovered %d elements for type %s", len(elements), typ.Name)
		}
	}
	
	rm.client.protocol.Infof("Successfully discovered resource metrics")
	return nil
}

// discoverClasses discovers monitoring classes from the metadata topic
func (rm *ResourceMonitor) discoverClasses(topic string) ([]*MonitoringClass, error) {
	rm.client.protocol.Debugf("Subscribing to classes metadata topic: '%s'", topic)
	
	// Create managed subscription to classes metadata topic
	hDest, err := rm.subscribeManaged(topic)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to classes metadata topic: %w", err)
	}
	defer rm.closeMQObject(hDest)
	
	rm.client.protocol.Debugf("Waiting for classes metadata publication on topic '%s'", topic)
	
	// First try a quick non-wait check to see if messages are already available
	rm.client.protocol.Debugf("Doing quick non-wait check for existing messages on queue")
	quickData, quickErr := rm.getMetadataFromQueueNoWait(hDest)
	if quickErr == nil {
		rm.client.protocol.Debugf("Found existing message immediately - using it")
		return rm.parseClassesMetadata(quickData)
	}
	rm.client.protocol.Debugf("No immediate messages available, trying with wait: %v", quickErr)

	// Get the metadata publication from the managed queue (with wait)
	data, err := rm.getMetadataFromQueue(hDest)
	if err != nil {
		rm.client.protocol.Debugf("No classes metadata received within timeout period for topic '%s'", topic)
		rm.client.protocol.Debugf("This could indicate: 1) Queue manager not publishing $SYS metadata, 2) MONINT not configured, 3) MQ version < 9.0")
		return nil, fmt.Errorf("failed to get classes metadata: %w", err)
	}
	
	// Parse PCF message for classes
	classes, err := rm.parseClassesMetadata(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse classes metadata: %w", err)
	}
	
	return classes, nil
}

// discoverTypes discovers monitoring types for a class
func (rm *ResourceMonitor) discoverTypes(class *MonitoringClass) ([]*MonitoringType, error) {
	rm.client.protocol.Debugf("Subscribing to types metadata topic: '%s'", class.TypesTopic)
	
	// Create managed subscription to types metadata topic
	hDest, err := rm.subscribeManaged(class.TypesTopic)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to types metadata topic: %w", err)
	}
	defer rm.closeMQObject(hDest)
	
	// Get the metadata publication from the managed queue
	data, err := rm.getMetadataFromQueue(hDest)
	if err != nil {
		return nil, fmt.Errorf("failed to get types metadata: %w", err)
	}
	
	// Parse PCF message for types
	types, err := rm.parseTypesMetadata(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse types metadata: %w", err)
	}
	
	return types, nil
}

// discoverElements discovers monitoring elements for a type
func (rm *ResourceMonitor) discoverElements(class *MonitoringClass, typ *MonitoringType) ([]*MonitoringElement, error) {
	rm.client.protocol.Debugf("Subscribing to elements metadata topic: '%s'", typ.ElementTopic)
	
	// Create managed subscription to elements metadata topic
	hDest, err := rm.subscribeManaged(typ.ElementTopic)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to elements metadata topic: %w", err)
	}
	defer rm.closeMQObject(hDest)
	
	// Get the metadata publication from the managed queue
	data, err := rm.getMetadataFromQueue(hDest)
	if err != nil {
		return nil, fmt.Errorf("failed to get elements metadata: %w", err)
	}
	
	// Parse PCF message for elements
	elements, err := rm.parseElementsMetadata(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse elements metadata: %w", err)
	}
	
	return elements, nil
}

// subscribeManaged creates a managed subscription that automatically creates a dynamic queue
func (rm *ResourceMonitor) subscribeManaged(topic string) (C.MQHOBJ, error) {
	rm.client.protocol.Debugf("Creating managed subscription to topic: %s", topic)
	
	// Initialize Subscription Descriptor
	var sd C.MQSD
	C.init_mqsd(&sd)
	
	// Set subscription options for managed subscription
	sd.Options = C.MQSO_CREATE | C.MQSO_NON_DURABLE | C.MQSO_MANAGED | C.MQSO_FAIL_IF_QUIESCING
	
	// Set topic string
	topicCStr := C.CString(topic)
	defer C.free(unsafe.Pointer(topicCStr))
	C.set_topic_string(&sd, topicCStr)
	
	var compCode C.MQLONG
	var reason C.MQLONG
	var hSub C.MQHOBJ
	var hDest C.MQHOBJ
	
	// Create managed subscription - MQ will create dynamic queue automatically
	rm.client.protocol.Debugf("MQSUB call: hConn=%d, topic='%s', options=0x%X", rm.client.hConn, topic, sd.Options)
	C.MQSUB(rm.client.hConn, C.PMQVOID(unsafe.Pointer(&sd)), &hDest, &hSub, &compCode, &reason)
	rm.client.protocol.Debugf("MQSUB result: compCode=%d, reason=%d, hSub=%d, hDest=%d", compCode, reason, hSub, hDest)
	
	if compCode != C.MQCC_OK {
		return C.MQHO_UNUSABLE_HOBJ, fmt.Errorf("MQSUB for managed subscription failed: completion code %d, reason %d (%s)", 
			compCode, reason, mqReasonString(int32(reason)))
	}
	
	rm.client.protocol.Debugf("Created managed subscription (sub handle=%d, dest handle=%d)", hSub, hDest)
	
	// Store subscription handle for cleanup
	rm.subscriptions = append(rm.subscriptions, hSub)
	
	return hDest, nil
}

// getMetadataFromQueue gets a metadata publication from a managed subscription queue
func (rm *ResourceMonitor) getMetadataFromQueue(hDest C.MQHOBJ) ([]byte, error) {
	bufferSize := 32768 // Start with 32KB like IBM reference code
	maxBufSize := 100 * 1024 * 1024 // 100MB limit
	buffer := make([]byte, bufferSize)
	
	for {
		// Initialize Message Descriptor and Get Message Options on each iteration
		var md C.MQMD
		C.init_mqmd(&md)
		
		var gmo C.MQGMO
		C.init_mqgmo(&gmo)
		gmo.Options = C.MQGMO_WAIT | C.MQGMO_NO_SYNCPOINT | C.MQGMO_FAIL_IF_QUIESCING | C.MQGMO_CONVERT
		gmo.WaitInterval = 30000 // 30 seconds like IBM reference implementation
		
		rm.client.protocol.Debugf("MQGET metadata: attempting with buffer size %d, hDest=%d, options=0x%X, waitInterval=%d", len(buffer), hDest, gmo.Options, gmo.WaitInterval)
		
		// Attempt to get the message with current buffer size
		var compCode C.MQLONG
		var reason C.MQLONG
		var dataLength C.MQLONG
		
		C.MQGET(rm.client.hConn, hDest, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), 
			C.MQLONG(len(buffer)), C.PMQVOID(unsafe.Pointer(&buffer[0])), &dataLength, &compCode, &reason)
		
		rm.client.protocol.Debugf("MQGET result: compCode=%d, reason=%d, dataLength=%d", compCode, reason, dataLength)
		
		if compCode == C.MQCC_OK {
			// Success - return the data
			return buffer[:dataLength], nil
		}
		
		// Handle truncation by doubling buffer size and retrying (IBM pattern)
		if reason == C.MQRC_TRUNCATED_MSG_FAILED && len(buffer) < maxBufSize {
			newSize := len(buffer) * 2
			if newSize > maxBufSize {
				newSize = maxBufSize
			}
			buffer = make([]byte, newSize)
			rm.client.protocol.Debugf("Message truncated, retrying with buffer size %d", newSize)
			continue // Retry with larger buffer
		}
		
		// Handle timeout or other errors
		if reason == C.MQRC_NO_MSG_AVAILABLE {
			rm.client.protocol.Debugf("No metadata publication available")
			return nil, fmt.Errorf("no metadata publication available")
		}
		
		return nil, fmt.Errorf("MQGET failed: completion code %d, reason %d (%s)", 
			compCode, reason, mqReasonString(int32(reason)))
	}
}

// parseClassesMetadata parses PCF message containing monitoring classes
func (rm *ResourceMonitor) parseClassesMetadata(buffer []byte) ([]*MonitoringClass, error) {
	var classes []*MonitoringClass
	
	if len(buffer) < int(C.sizeof_MQCFH) {
		return nil, fmt.Errorf("buffer too small for PCF header")
	}
	
	// Parse PCF header
	header := (*C.MQCFH)(unsafe.Pointer(&buffer[0]))
	// MQCMD_MONITOR_METADATA is likely 228 based on PCF command patterns
	const MQCMD_MONITOR_METADATA = 228
	if header.Command != MQCMD_MONITOR_METADATA {
		return nil, fmt.Errorf("unexpected command %d, expected MQCMD_MONITOR_METADATA (%d)", header.Command, MQCMD_MONITOR_METADATA)
	}
	
	offset := int(C.sizeof_MQCFH)
	
	// Parse groups (MQGACF_MONITOR_CLASS)
	for i := C.MQLONG(0); i < header.ParameterCount && offset < len(buffer); i++ {
		param := (*C.MQCFGR)(unsafe.Pointer(&buffer[offset]))
		
		if param.Type == C.MQCFT_GROUP && param.Parameter == C.MQGACF_MONITOR_CLASS {
			class := &MonitoringClass{}
			groupOffset := offset + int(C.sizeof_MQCFGR)
			
			// Parse parameters within the group
			for j := C.MQLONG(0); j < param.ParameterCount && groupOffset < len(buffer); j++ {
				// Check parameter type
				baseParam := (*C.MQCFH)(unsafe.Pointer(&buffer[groupOffset]))
				
				switch baseParam.Type {
				case C.MQCFT_INTEGER:
					intParam := (*C.MQCFIN)(unsafe.Pointer(&buffer[groupOffset]))
					if intParam.Parameter == C.MQIAMO_MONITOR_CLASS {
						class.ID = intParam.Value
					}
					groupOffset += int(intParam.StrucLength)
					
				case C.MQCFT_STRING:
					strParam := (*C.MQCFST)(unsafe.Pointer(&buffer[groupOffset]))
					strLen := int(strParam.StringLength)
					strData := (*[1 << 30]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(strParam)) + C.sizeof_MQCFST))[:strLen:strLen]
					
					switch strParam.Parameter {
					case C.MQCAMO_MONITOR_CLASS:
						class.Name = strings.TrimSpace(string(strData))
					case C.MQCAMO_MONITOR_DESC:
						class.Description = strings.TrimSpace(string(strData))
					case C.MQCA_TOPIC_STRING:
						class.TypesTopic = strings.TrimSpace(string(strData))
					}
					groupOffset += int(strParam.StrucLength)
					
				default:
					// Skip unknown parameter type
					groupOffset += 16 // Minimum structure size
				}
			}
			
			if class.Name != "" {
				classes = append(classes, class)
			}
			
			offset += int(param.StrucLength)
		} else {
			// Skip this parameter
			offset += 16 // Minimum structure size
		}
	}
	
	return classes, nil
}

// parseTypesMetadata parses PCF message containing monitoring types
func (rm *ResourceMonitor) parseTypesMetadata(buffer []byte) ([]*MonitoringType, error) {
	var types []*MonitoringType
	
	if len(buffer) < int(C.sizeof_MQCFH) {
		return nil, fmt.Errorf("buffer too small for PCF header")
	}
	
	header := (*C.MQCFH)(unsafe.Pointer(&buffer[0]))
	// MQCMD_MONITOR_METADATA is likely 228
	const MQCMD_MONITOR_METADATA = 228
	if header.Command != MQCMD_MONITOR_METADATA {
		return nil, fmt.Errorf("unexpected command %d, expected %d", header.Command, MQCMD_MONITOR_METADATA)
	}
	
	offset := int(C.sizeof_MQCFH)
	
	// Parse groups (MQGACF_MONITOR_TYPE)
	for i := C.MQLONG(0); i < header.ParameterCount && offset < len(buffer); i++ {
		param := (*C.MQCFGR)(unsafe.Pointer(&buffer[offset]))
		
		if param.Type == C.MQCFT_GROUP && param.Parameter == C.MQGACF_MONITOR_TYPE {
			typ := &MonitoringType{}
			groupOffset := offset + int(C.sizeof_MQCFGR)
			
			// Parse parameters within the group
			for j := C.MQLONG(0); j < param.ParameterCount && groupOffset < len(buffer); j++ {
				baseParam := (*C.MQCFH)(unsafe.Pointer(&buffer[groupOffset]))
				
				switch baseParam.Type {
				case C.MQCFT_INTEGER:
					intParam := (*C.MQCFIN)(unsafe.Pointer(&buffer[groupOffset]))
					if intParam.Parameter == C.MQIAMO_MONITOR_TYPE {
						typ.ID = intParam.Value
					}
					groupOffset += int(intParam.StrucLength)
					
				case C.MQCFT_STRING:
					strParam := (*C.MQCFST)(unsafe.Pointer(&buffer[groupOffset]))
					strLen := int(strParam.StringLength)
					strData := (*[1 << 30]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(strParam)) + C.sizeof_MQCFST))[:strLen:strLen]
					
					switch strParam.Parameter {
					case C.MQCAMO_MONITOR_TYPE:
						typ.Name = strings.TrimSpace(string(strData))
					case C.MQCAMO_MONITOR_DESC:
						typ.Description = strings.TrimSpace(string(strData))
					case C.MQCA_TOPIC_STRING:
						topicStr := strings.TrimSpace(string(strData))
						// Topic string contains both elements topic and data topic
						// Elements topic ends with /ELEMENTS
						if strings.HasSuffix(topicStr, "/ELEMENTS") {
							typ.ElementTopic = topicStr
							typ.DataTopic = strings.TrimSuffix(topicStr, "/ELEMENTS")
						} else {
							typ.DataTopic = topicStr
							typ.ElementTopic = topicStr + "/ELEMENTS"
						}
					}
					groupOffset += int(strParam.StrucLength)
					
				default:
					groupOffset += 16
				}
			}
			
			if typ.Name != "" {
				types = append(types, typ)
			}
			
			offset += int(param.StrucLength)
		} else {
			offset += 16
		}
	}
	
	return types, nil
}

// parseElementsMetadata parses PCF message containing monitoring elements
func (rm *ResourceMonitor) parseElementsMetadata(buffer []byte) ([]*MonitoringElement, error) {
	var elements []*MonitoringElement
	
	if len(buffer) < int(C.sizeof_MQCFH) {
		return nil, fmt.Errorf("buffer too small for PCF header")
	}
	
	header := (*C.MQCFH)(unsafe.Pointer(&buffer[0]))
	// MQCMD_MONITOR_METADATA is likely 228
	const MQCMD_MONITOR_METADATA = 228
	if header.Command != MQCMD_MONITOR_METADATA {
		return nil, fmt.Errorf("unexpected command %d, expected %d", header.Command, MQCMD_MONITOR_METADATA)
	}
	
	offset := int(C.sizeof_MQCFH)
	
	// Parse groups (MQGACF_MONITOR_ELEMENT)
	for i := C.MQLONG(0); i < header.ParameterCount && offset < len(buffer); i++ {
		param := (*C.MQCFGR)(unsafe.Pointer(&buffer[offset]))
		
		if param.Type == C.MQCFT_GROUP && param.Parameter == C.MQGACF_MONITOR_ELEMENT {
			element := &MonitoringElement{}
			elementName := ""
			groupOffset := offset + int(C.sizeof_MQCFGR)
			
			// Parse parameters within the group
			for j := C.MQLONG(0); j < param.ParameterCount && groupOffset < len(buffer); j++ {
				baseParam := (*C.MQCFH)(unsafe.Pointer(&buffer[groupOffset]))
				
				switch baseParam.Type {
				case C.MQCFT_INTEGER:
					intParam := (*C.MQCFIN)(unsafe.Pointer(&buffer[groupOffset]))
					switch intParam.Parameter {
					case C.MQIAMO_MONITOR_ELEMENT:
						element.ID = intParam.Value
					case C.MQIAMO_MONITOR_DATATYPE:
						element.DataType = intParam.Value
					}
					groupOffset += int(intParam.StrucLength)
					
				case C.MQCFT_STRING:
					strParam := (*C.MQCFST)(unsafe.Pointer(&buffer[groupOffset]))
					strLen := int(strParam.StringLength)
					strData := (*[1 << 30]byte)(unsafe.Pointer(uintptr(unsafe.Pointer(strParam)) + C.sizeof_MQCFST))[:strLen:strLen]
					
					if strParam.Parameter == C.MQCAMO_MONITOR_DESC {
						elementName = strings.TrimSpace(string(strData))
						element.Description = elementName
					}
					groupOffset += int(strParam.StrucLength)
					
				default:
					groupOffset += 16
				}
			}
			
			if elementName != "" && element.ID != 0 {
				elements = append(elements, element)
			}
			
			offset += int(param.StrucLength)
		} else {
			offset += 16
		}
	}
	
	return elements, nil
}

func (rm *ResourceMonitor) closeMQObject(hObj C.MQHOBJ) {
	if hObj == C.MQHO_UNUSABLE_HOBJ {
		return
	}
	
	var compCode C.MQLONG
	var reason C.MQLONG
	var options C.MQLONG = 0
	
	C.MQCLOSE(rm.client.hConn, &hObj, options, &compCode, &reason)
	if compCode != C.MQCC_OK {
		rm.client.protocol.Debugf("Failed to close MQ object: reason %d", reason)
	}
}

func (rm *ResourceMonitor) closeMQSubscription(hSub C.MQHOBJ) {
	if hSub == C.MQHO_UNUSABLE_HOBJ {
		return
	}
	
	var compCode C.MQLONG
	var reason C.MQLONG
	var options C.MQLONG = C.MQCO_REMOVE_SUB
	
	C.MQCLOSE(rm.client.hConn, &hSub, options, &compCode, &reason)
	if compCode != C.MQCC_OK {
		rm.client.protocol.Debugf("Failed to close subscription: reason %d", reason)
	}
}

func (rm *ResourceMonitor) close() {
	// Close all subscriptions
	for i, hSub := range rm.subscriptions {
		rm.client.protocol.Debugf("Closing subscription %d", i)
		rm.closeMQSubscription(hSub)
	}
	rm.subscriptions = nil
}

// Cleanup closes all MQ resources used by the resource monitor
func (rm *ResourceMonitor) Cleanup() {
	rm.close()
}

// ProcessPublications retrieves resource monitoring publications from the queue manager
func (rm *ResourceMonitor) ProcessPublications() ([]ResourcePublication, error) {
	// This is a placeholder implementation - in a full implementation this would:
	// 1. Subscribe to $SYS/MQ/INFO/QMGR/{qmgr}/Monitor/{class}/{type} topics
	// 2. Wait for publication messages
	// 3. Parse the PCF messages to extract metric values
	
	rm.client.protocol.Debugf("ProcessPublications called - returning empty results (not implemented)")
	return []ResourcePublication{}, nil
}

// GetMetricByParameterID looks up a metric definition by its parameter ID
func (rm *ResourceMonitor) GetMetricByParameterID(paramID C.MQLONG) (MetricDefinition, bool) {
	// This is a placeholder implementation - in a full implementation this would:
	// 1. Use the discovered metadata to map parameter IDs to metric definitions
	// 2. Return the metric class, type, and element information
	
	rm.client.protocol.Debugf("GetMetricByParameterID called for parameter %d - returning empty result (not implemented)", paramID)
	return MetricDefinition{}, false
}

// getMetadataFromQueueNoWait tries to get a message immediately without waiting
func (rm *ResourceMonitor) getMetadataFromQueueNoWait(hDest C.MQHOBJ) ([]byte, error) {
	bufferSize := 32768
	maxBufSize := 100 * 1024 * 1024
	buffer := make([]byte, bufferSize)
	
	for {
		// Initialize Message Descriptor and Get Message Options
		var md C.MQMD
		C.init_mqmd(&md)
		
		var gmo C.MQGMO
		C.init_mqgmo(&gmo)
		gmo.Options = C.MQGMO_NO_SYNCPOINT | C.MQGMO_FAIL_IF_QUIESCING | C.MQGMO_CONVERT
		// NO MQGMO_WAIT - immediate check only
		gmo.WaitInterval = 0
		
		rm.client.protocol.Debugf("MQGET no-wait: attempting with buffer size %d, hDest=%d, options=0x%X", len(buffer), hDest, gmo.Options)
		
		var compCode C.MQLONG
		var reason C.MQLONG
		var dataLength C.MQLONG
		
		C.MQGET(rm.client.hConn, hDest, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), 
			C.MQLONG(len(buffer)), C.PMQVOID(unsafe.Pointer(&buffer[0])), &dataLength, &compCode, &reason)
		
		rm.client.protocol.Debugf("MQGET no-wait result: compCode=%d, reason=%d, dataLength=%d", compCode, reason, dataLength)
		
		if compCode == C.MQCC_OK {
			return buffer[:dataLength], nil
		}
		
		// Handle truncation by doubling buffer size and retrying
		if reason == C.MQRC_TRUNCATED_MSG_FAILED && len(buffer) < maxBufSize {
			newSize := len(buffer) * 2
			if newSize > maxBufSize {
				newSize = maxBufSize
			}
			buffer = make([]byte, newSize)
			rm.client.protocol.Debugf("Message truncated in no-wait, retrying with buffer size %d", newSize)
			continue
		}
		
		// Return the error for no message available or other issues
		return nil, fmt.Errorf("MQGET no-wait failed: completion code %d, reason %d (%s)", 
			compCode, reason, mqReasonString(int32(reason)))
	}
}

// Supporting types for the above methods

// ResourcePublication represents a single resource monitoring publication
type ResourcePublication struct {
	MetricClass string
	MetricType  string
	Values      map[string]int64
}

// MetricDefinition contains metadata about a discovered metric
type MetricDefinition struct {
	Class   string
	Type    string
	Element string
}