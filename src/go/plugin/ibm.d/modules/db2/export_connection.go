package db2

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"

func (c *Collector) exportConnectionMetrics() {
	for id, metrics := range c.mx.connections {
		meta := c.connections[id]
		labels := contexts.ConnectionLabels{
			Application_id: id,
		}

		if meta != nil {
			if meta.applicationName != "" && meta.applicationName != "-" {
				labels.Application_name = meta.applicationName
			}
			if meta.clientHostname != "" && meta.clientHostname != "-" {
				labels.Client_hostname = meta.clientHostname
			}
			if meta.clientIP != "" && meta.clientIP != "-" {
				labels.Client_ip = meta.clientIP
			}
			if meta.clientUser != "" && meta.clientUser != "-" {
				labels.Client_user = meta.clientUser
			}
			if meta.connectionState != "" {
				labels.State = meta.connectionState
			}
		}

		contexts.Connection.State.Set(c.State, labels, contexts.ConnectionStateValues{
			State: metrics.State,
		})

		contexts.Connection.Activity.Set(c.State, labels, contexts.ConnectionActivityValues{
			Read:    metrics.RowsRead,
			Written: metrics.RowsWritten,
		})

		contexts.Connection.WaitTime.Set(c.State, labels, contexts.ConnectionWaitTimeValues{
			Lock:         metrics.LockWaitTime,
			Log_disk:     metrics.LogDiskWaitTime,
			Log_buffer:   metrics.LogBufferWaitTime,
			Pool_read:    metrics.PoolReadTime,
			Pool_write:   metrics.PoolWriteTime,
			Direct_read:  metrics.DirectReadTime,
			Direct_write: metrics.DirectWriteTime,
			Fcm_recv:     metrics.FCMRecvWaitTime,
			Fcm_send:     metrics.FCMSendWaitTime,
		})

		contexts.Connection.ProcessingTime.Set(c.State, labels, contexts.ConnectionProcessingTimeValues{
			Routine:  metrics.TotalRoutineTime,
			Compile:  metrics.TotalCompileTime,
			Section:  metrics.TotalSectionTime,
			Commit:   metrics.TotalCommitTime,
			Rollback: metrics.TotalRollbackTime,
		})
	}
}
