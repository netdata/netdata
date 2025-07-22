// SPDX-License-Identifier: GPL-3.0-or-later

package pcf

import "github.com/ibm-messaging/mq-golang/v5/ibmmq"

// PCF internal error codes for response parsing
const (
	ErrInternalParsing = -10001
	ErrInternalShort   = -10002
	ErrInternalCorrupt = -10003
)

// MQ Queue Type constants (from IBM library)
const (
	QueueTypeLocal   = int32(ibmmq.MQQT_LOCAL)
	QueueTypeModel   = int32(ibmmq.MQQT_MODEL)
	QueueTypeAlias   = int32(ibmmq.MQQT_ALIAS)
	QueueTypeRemote  = int32(ibmmq.MQQT_REMOTE)
	QueueTypeCluster = int32(ibmmq.MQQT_CLUSTER)
)

