// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
)

// mqcmdToString returns a human-readable name for MQCMD command constants
func mqcmdToString(command C.MQLONG) string {
	switch command {
	case C.MQCMD_INQUIRE_Q_MGR:
		return "MQCMD_INQUIRE_Q_MGR"
	case C.MQCMD_INQUIRE_Q_MGR_STATUS:
		return "MQCMD_INQUIRE_Q_MGR_STATUS"
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
	switch C.MQLONG(reason) {
	case C.MQRC_NONE:
		return "MQRC_NONE"
	case C.MQRC_CONNECTION_BROKEN:
		return "MQRC_CONNECTION_BROKEN"
	case C.MQRC_HCONN_ERROR:
		return "MQRC_HCONN_ERROR"
	case C.MQRC_NO_MSG_AVAILABLE:
		return "MQRC_NO_MSG_AVAILABLE"
	case C.MQRC_NOT_AUTHORIZED:
		return "MQRC_NOT_AUTHORIZED"
	case C.MQRC_Q_MGR_NAME_ERROR:
		return "MQRC_Q_MGR_NAME_ERROR"
	case C.MQRC_Q_MGR_NOT_AVAILABLE:
		return "MQRC_Q_MGR_NOT_AVAILABLE"
	case C.MQRC_OBJECT_NAME_ERROR:
		return "MQRC_OBJECT_NAME_ERROR"
	case C.MQRC_OBJECT_IN_USE:
		return "MQRC_OBJECT_IN_USE"
	case C.MQRC_OBJECT_TYPE_ERROR:
		return "MQRC_OBJECT_TYPE_ERROR"
	case C.MQRC_TRUNCATED_MSG_FAILED:
		return "MQRC_TRUNCATED_MSG_FAILED"
	case C.MQRC_UNKNOWN_OBJECT_NAME:
		return "MQRC_UNKNOWN_OBJECT_NAME"
	case C.MQRC_NO_MSG_UNDER_CURSOR:
		return "MQRC_NO_MSG_UNDER_CURSOR"
	case C.MQRC_UNEXPECTED_ERROR:
		return "MQRC_UNEXPECTED_ERROR"
	case C.MQRC_Q_MGR_STOPPING:
		return "MQRC_Q_MGR_STOPPING"
	case C.MQRC_Q_MGR_QUIESCING:
		return "MQRC_Q_MGR_QUIESCING"
	case C.MQRC_HOST_NOT_AVAILABLE:
		return "MQRC_HOST_NOT_AVAILABLE"
	case C.MQRC_CHANNEL_CONFIG_ERROR:
		return "MQRC_CHANNEL_CONFIG_ERROR"
	case C.MQRC_CHANNEL_NOT_AVAILABLE:
		return "MQRC_CHANNEL_NOT_AVAILABLE"
	case C.MQRCCF_COMMAND_FAILED:
		return "MQRCCF_COMMAND_FAILED"
	case C.MQRCCF_OBJECT_OPEN:
		return "MQRCCF_OBJECT_OPEN"
	case C.MQRCCF_Q_TYPE_ERROR:
		return "MQRCCF_Q_TYPE_ERROR"
	case C.MQRCCF_OBJECT_NAME_ERROR:
		return "MQRCCF_OBJECT_NAME_ERROR"
	case C.MQRCCF_CHANNEL_NOT_FOUND:
		return "MQRCCF_CHANNEL_NOT_FOUND"
	case C.MQRCCF_CHANNEL_NOT_ACTIVE:
		return "MQRCCF_CHANNEL_NOT_ACTIVE"
	case C.MQRCCF_Q_NAME_ERROR:
		return "MQRCCF_Q_NAME_ERROR"
	case C.MQRCCF_OBJECT_BEING_DELETED:
		return "MQRCCF_OBJECT_BEING_DELETED"
	default:
		return fmt.Sprintf("MQRC_%d", reason)
	}
}