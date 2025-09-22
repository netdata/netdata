// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo && ibm_mq
// +build cgo,ibm_mq

package pcf

import (
	"fmt"
	"strconv"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)

// mqcmdToString returns a human-readable name for MQCMD command constants
func mqcmdToString(command int32) string {
	// Use the library's built-in function to get the string name
	name := ibmmq.MQItoString("CMD", int(command))

	// If the library returns a numeric string, it means the constant is unknown
	if name == fmt.Sprintf("%d", command) {
		return fmt.Sprintf("MQCMD_%d", command)
	}

	// Return the full name with prefix for clarity
	return name
}

// mqReasonString returns a human-readable name for MQ reason codes
func mqReasonString(reason int32) string {
	// First try RC (reason codes)
	name := ibmmq.MQItoString("RC", int(reason))

	// If RC didn't work, try RCCF (PCF reason codes)
	if name == fmt.Sprintf("%d", reason) {
		name = ibmmq.MQItoString("RCCF", int(reason))
	}

	// If still not found, return numeric format
	if name == fmt.Sprintf("%d", reason) {
		return fmt.Sprintf("MQRC_%d", reason)
	}

	// Return the full name with prefix for clarity
	return name
}

func mqParameterToString(parameter int32) string {
	paramStr := strconv.Itoa(int(parameter))

	// Try integer attributes (includes MQIA_* and MQIACF_*)
	name := ibmmq.MQItoString("IA", int(parameter))
	if name != "" && name != paramStr {
		return name
	}

	// Try character attributes (includes MQCA_* and MQCACF_*)
	name = ibmmq.MQItoString("CA", int(parameter))
	if name != "" && name != paramStr {
		return name
	}

	// Try byte attributes
	name = ibmmq.MQItoString("BACF", int(parameter))
	if name != "" && name != paramStr {
		return name
	}

	// Try group attributes
	name = ibmmq.MQItoString("GACF", int(parameter))
	if name != "" && name != paramStr {
		return name
	}

	// Unknown parameter
	return "PARAM_" + paramStr
}

// mqOperatorToString returns a human-readable name for MQ filter operators
func mqOperatorToString(operator int32) string {
	switch operator {
	case ibmmq.MQCFOP_EQUAL:
		return "EQ"
	case ibmmq.MQCFOP_NOT_EQUAL:
		return "NE"
	case ibmmq.MQCFOP_LESS:
		return "LT"
	case ibmmq.MQCFOP_GREATER:
		return "GT"
	case ibmmq.MQCFOP_NOT_LESS:
		return "GE"
	case ibmmq.MQCFOP_NOT_GREATER:
		return "LE"
	case ibmmq.MQCFOP_LIKE:
		return "LIKE"
	case ibmmq.MQCFOP_NOT_LIKE:
		return "NOT_LIKE"
	case ibmmq.MQCFOP_CONTAINS:
		return "CONTAINS"
	case ibmmq.MQCFOP_EXCLUDES:
		return "EXCLUDES"
	default:
		return fmt.Sprintf("OP_%d", operator)
	}
}

// mqPCFTypeToString returns a human-readable name for PCF message types
func mqPCFTypeToString(msgType int32) string {
	// Use the library's built-in function to get the string name
	name := ibmmq.MQItoString("CFT", int(msgType))

	// If the library returns a numeric string, it means the constant is unknown
	if name == fmt.Sprintf("%d", msgType) {
		return fmt.Sprintf("MQCFT_%d", msgType)
	}

	// Return the full name with prefix for clarity
	return name
}

// mqPCFControlToString returns a human-readable name for PCF control options
func mqPCFControlToString(control int32) string {
	// Use the library's built-in function to get the string name
	name := ibmmq.MQItoString("CFC", int(control))

	// If the library returns a numeric string, it means the constant is unknown
	if name == fmt.Sprintf("%d", control) {
		return fmt.Sprintf("MQCFC_%d", control)
	}

	// Return the full name with prefix for clarity
	return name
}
