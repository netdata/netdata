// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

// PCF internal error codes for response parsing
const (
	ErrInternalParsing = -10001
	ErrInternalShort   = -10002
	ErrInternalCorrupt = -10003
)

// MQ Queue Type constants
const (
	QueueTypeLocal   = int32(C.MQQT_LOCAL)
	QueueTypeModel   = int32(C.MQQT_MODEL)
	QueueTypeAlias   = int32(C.MQQT_ALIAS)
	QueueTypeRemote  = int32(C.MQQT_REMOTE)
	QueueTypeCluster = int32(C.MQQT_CLUSTER)
)



// MQ object name parameter definitions
// These define the correct padding lengths and fill characters for different MQ object types
var mqObjectNameParams = map[C.MQLONG]objectNameInfo{
	// Queue names - 48 characters, space-padded
	C.MQCA_Q_NAME: {padLength: 48, fillChar: ' '},
	// Channel names - 20 characters, space-padded
	C.MQCACH_CHANNEL_NAME: {padLength: 20, fillChar: ' '},
	// Topic names - 256 characters, space-padded
	C.MQCA_TOPIC_NAME: {padLength: 256, fillChar: ' '},
	// Process names - 48 characters, space-padded
	C.MQCA_PROCESS_NAME: {padLength: 48, fillChar: ' '},
	// Namelist names - 48 characters, space-padded
	C.MQCA_NAMELIST_NAME: {padLength: 48, fillChar: ' '},
	// CF Structure names - 12 characters, space-padded
	C.MQCA_CF_STRUC_NAME: {padLength: 12, fillChar: ' '},
	// Authentication Info names - 48 characters, space-padded
	C.MQCA_AUTH_INFO_NAME: {padLength: 48, fillChar: ' '},
	// Storage Class names - 8 characters, space-padded
	C.MQCA_STORAGE_CLASS: {padLength: 8, fillChar: ' '},
	// Subscription names - 10240 characters, null-padded
	C.MQCACF_SUB_NAME: {padLength: 10240, fillChar: '\x00'},
}

// Object name parameter information
type objectNameInfo struct {
	padLength int  // Padding length in bytes
	fillChar  byte // Fill character for padding
}

// getObjectNameInfo returns the object name information for a given parameter
// Returns nil if the parameter is not an MQ object name parameter
func getObjectNameInfo(param C.MQLONG) *objectNameInfo {
	if info, exists := mqObjectNameParams[param]; exists {
		return &info
	}
	return nil
}