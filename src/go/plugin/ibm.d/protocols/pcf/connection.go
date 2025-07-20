// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"os"
	"unsafe"
)

// Connect connects to the queue manager.
func (c *Client) Connect() error {
	c.protocol.Debugf("PCF protocol starting connection attempt to queue manager '%s' on %s:%d via channel '%s' (user: '%s')", 
		c.config.QueueManager, c.config.Host, c.config.Port, c.config.Channel, c.config.User)
	
	backoff := c.protocol.GetBackoff()
	attemptCount := 0
	err := backoff.Retry(func() error {
		attemptCount++
		c.protocol.Debugf("PCF connection attempt #%d to %s:%d (queue manager: '%s', channel: '%s')", 
			attemptCount, c.config.Host, c.config.Port, c.config.QueueManager, c.config.Channel)
		return c.doConnect()
	})
	if err != nil {
		c.protocol.Errorf("PCF connection failed to queue manager '%s' at %s:%d after %d attempts: %v", 
			c.config.QueueManager, c.config.Host, c.config.Port, attemptCount, err)
		return err
	}
	c.protocol.MarkConnected()
	c.protocol.Debugf("PCF successfully connected to queue manager '%s' at %s:%d (channel: '%s', user: '%s') after %d attempts", 
		c.config.QueueManager, c.config.Host, c.config.Port, c.config.Channel, c.config.User, attemptCount)
	return nil
}

func (c *Client) doConnect() error {
	if c.connected {
		c.protocol.Debugf("PCF already connected to queue manager '%s' at %s:%d, skipping connection attempt", 
			c.config.QueueManager, c.config.Host, c.config.Port)
		return nil
	}

	// CRITICAL: IBM MQ PCF fails with LC_ALL=C locale
	// The MQ client library has issues with string handling in the C locale,
	// causing MQRCCF_CFH_PARM_ID_ERROR and MQRCCF_COMMAND_FAILED errors.
	// We must unset LC_ALL to allow MQ to use the default locale handling.
	if os.Getenv("LC_ALL") == "C" {
		c.protocol.Debugf("PCF: Unsetting LC_ALL=C for IBM MQ compatibility (queue manager: '%s')", c.config.QueueManager)
		os.Unsetenv("LC_ALL")
	}

	// Use MQCONNX for client connections
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
	cChannelName := C.CString(c.config.Channel)
	defer C.free(unsafe.Pointer(cChannelName))
	C.strncpy((*C.char)(unsafe.Pointer(&cd.ChannelName)), cChannelName, C.MQ_CHANNEL_NAME_LENGTH)

	// Set connection name (host and port)
	connName := fmt.Sprintf("%s(%d)", c.config.Host, c.config.Port)
	c.protocol.Debugf("PCF: Setting connection name to '%s' for queue manager '%s'", connName, c.config.QueueManager)
	cConnName := C.CString(connName)
	defer C.free(unsafe.Pointer(cConnName))
	C.strncpy((*C.char)(unsafe.Pointer(&cd.ConnectionName)), cConnName, C.MQ_CONN_NAME_LENGTH)

	// Set user credentials if provided
	if c.config.User != "" {
		cUser := C.CString(c.config.User)
		defer C.free(unsafe.Pointer(cUser))
		C.strncpy((*C.char)(unsafe.Pointer(&cd.UserIdentifier)), cUser, C.MQ_USER_ID_LENGTH)
	}

	// Set up connection options to use the channel definition
	cno.ClientConnPtr = C.MQPTR(unsafe.Pointer(cd))

	// Set connection options for better stability
	cno.Options = C.MQCNO_CLIENT_BINDING | C.MQCNO_RECONNECT | C.MQCNO_HANDLE_SHARE_BLOCK

	// Set up authentication if password is provided
	var cspUser, cspPassword *C.char
	if c.config.Password != "" {
		csp := (*C.MQCSP)(C.malloc(C.sizeof_MQCSP))
		defer C.free(unsafe.Pointer(csp))
		C.memset(unsafe.Pointer(csp), 0, C.sizeof_MQCSP)
		C.set_csp_struc_id(csp)
		csp.Version = C.MQCSP_VERSION_1
		csp.AuthenticationType = C.MQCSP_AUTH_USER_ID_AND_PWD
		cspUser = C.CString(c.config.User)
		defer C.free(unsafe.Pointer(cspUser))
		csp.CSPUserIdPtr = C.MQPTR(unsafe.Pointer(cspUser))
		csp.CSPUserIdLength = C.MQLONG(len(c.config.User))
		cspPassword = C.CString(c.config.Password)
		defer C.free(unsafe.Pointer(cspPassword))
		csp.CSPPasswordPtr = C.MQPTR(unsafe.Pointer(cspPassword))
		csp.CSPPasswordLength = C.MQLONG(len(c.config.Password))

		cno.Version = C.MQCNO_VERSION_5
		cno.SecurityParmsPtr = (*C.MQCSP)(unsafe.Pointer(csp))
	}

	// Queue manager name
	qmName := C.CString(c.config.QueueManager)
	defer C.free(unsafe.Pointer(qmName))

	var compCode C.MQLONG
	var reason C.MQLONG

	// Connect to queue manager
	c.protocol.Debugf("PCF: Calling MQCONNX to connect to queue manager '%s' at %s:%d via channel '%s'", 
		c.config.QueueManager, c.config.Host, c.config.Port, c.config.Channel)
	C.MQCONNX(qmName, cno, &c.hConn, &compCode, &reason)

	if compCode != C.MQCC_OK {
		c.protocol.Errorf("PCF: MQCONNX failed for queue manager '%s' at %s:%d - completion code %d, reason %d (%s)",
			c.config.QueueManager, c.config.Host, c.config.Port, compCode, reason, mqReasonString(int32(reason)))
		return fmt.Errorf("MQCONNX failed: completion code %d, reason %d (%s) (check queue manager '%s' is running and accessible on %s:%d)",
			compCode, reason, mqReasonString(int32(reason)), c.config.QueueManager, c.config.Host, c.config.Port)
	}
	c.protocol.Debugf("PCF: MQCONNX successful for queue manager '%s', connection handle: %d", c.config.QueueManager, c.hConn)

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

	c.protocol.Debugf("PCF: Opening SYSTEM.ADMIN.COMMAND.QUEUE for queue manager '%s'", c.config.QueueManager)
	C.MQOPEN(c.hConn, C.PMQVOID(unsafe.Pointer(&od)), openOptions, &c.hObj, &compCode, &reason)

	if compCode != C.MQCC_OK {
		// Save the actual error codes before disconnect
		openCompCode := compCode
		openReason := reason

		c.protocol.Errorf("PCF: MQOPEN failed for SYSTEM.ADMIN.COMMAND.QUEUE on queue manager '%s' - completion code %d, reason %d (%s)",
			c.config.QueueManager, openCompCode, openReason, mqReasonString(int32(openReason)))

		var discCompCode, discReason C.MQLONG
		C.MQDISC(&c.hConn, &discCompCode, &discReason)
		return fmt.Errorf("MQOPEN failed: completion code %d, reason %d (%s) (check PCF permissions for SYSTEM.ADMIN.COMMAND.QUEUE)", openCompCode, openReason, mqReasonString(int32(openReason)))
	}
	c.protocol.Debugf("PCF: Successfully opened SYSTEM.ADMIN.COMMAND.QUEUE for queue manager '%s', handle: %d", c.config.QueueManager, c.hObj)

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

	c.protocol.Debugf("PCF: Creating dynamic reply queue for queue manager '%s' using model queue SYSTEM.DEFAULT.MODEL.QUEUE", c.config.QueueManager)
	C.MQOPEN(c.hConn, C.PMQVOID(unsafe.Pointer(&replyOd)), replyOpenOptions, &c.hReplyObj, &compCode, &reason)
	if compCode != C.MQCC_OK {
		// Save the actual error codes before disconnect
		replyOpenCompCode := compCode
		replyOpenReason := reason

		c.protocol.Errorf("PCF: Failed to create dynamic reply queue for queue manager '%s' - completion code %d, reason %d (%s)",
			c.config.QueueManager, replyOpenCompCode, replyOpenReason, mqReasonString(int32(replyOpenReason)))

		// Close admin queue
		C.MQCLOSE(c.hConn, &c.hObj, C.MQCO_NONE, &compCode, &reason)

		var discCompCode, discReason C.MQLONG
		C.MQDISC(&c.hConn, &discCompCode, &discReason)
		return fmt.Errorf("MQOPEN reply queue failed: completion code %d, reason %d (%s)", replyOpenCompCode, replyOpenReason, mqReasonString(int32(replyOpenReason)))
	}
	
	// Store the actual dynamic queue name that was created
	C.memcpy(unsafe.Pointer(&c.replyQueueName), unsafe.Pointer(&replyOd.ObjectName), 48)
	
	// Convert the queue name to Go string for logging
	replyQNameBytes := make([]byte, 48)
	C.memcpy(unsafe.Pointer(&replyQNameBytes[0]), unsafe.Pointer(&replyOd.ObjectName), 48)
	replyQNameStr := string(replyQNameBytes[:])
	c.protocol.Debugf("PCF: Successfully created dynamic reply queue '%s' for queue manager '%s', handle: %d", 
		replyQNameStr, c.config.QueueManager, c.hReplyObj)

	c.connected = true
	
	// Refresh static data on successful connection
	c.refreshStaticData()
	
	return nil
}

// Disconnect disconnects from the queue manager.
func (c *Client) Disconnect() {
	if !c.connected {
		c.protocol.Debugf("PCF: Already disconnected from queue manager '%s', skipping disconnect", c.config.QueueManager)
		return
	}

	c.protocol.Debugf("PCF: Starting disconnect from queue manager '%s' at %s:%d", 
		c.config.QueueManager, c.config.Host, c.config.Port)

	var compCode, reason C.MQLONG

	// Close reply queue
	if c.hReplyObj != 0 {
		c.protocol.Debugf("PCF: Closing reply queue for queue manager '%s'", c.config.QueueManager)
		C.MQCLOSE(c.hConn, &c.hReplyObj, C.MQCO_DELETE_PURGE, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.protocol.Warningf("PCF: Failed to close reply queue for queue manager '%s' - completion code %d, reason %d (%s)",
				c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		}
		c.hReplyObj = 0
	}

	// Close admin queue
	if c.hObj != 0 {
		c.protocol.Debugf("PCF: Closing SYSTEM.ADMIN.COMMAND.QUEUE for queue manager '%s'", c.config.QueueManager)
		C.MQCLOSE(c.hConn, &c.hObj, C.MQCO_NONE, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.protocol.Warningf("PCF: Failed to close SYSTEM.ADMIN.COMMAND.QUEUE for queue manager '%s' - completion code %d, reason %d (%s)",
				c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		}
		c.hObj = 0
	}

	// Disconnect from queue manager
	if c.hConn != 0 {
		c.protocol.Debugf("PCF: Disconnecting from queue manager '%s'", c.config.QueueManager)
		C.MQDISC(&c.hConn, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.protocol.Warningf("PCF: MQDISC failed for queue manager '%s' - completion code %d, reason %d (%s)",
				c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		}
		c.hConn = 0
	}

	c.connected = false
	c.protocol.Debugf("PCF: Successfully disconnected from queue manager '%s'", c.config.QueueManager)
}