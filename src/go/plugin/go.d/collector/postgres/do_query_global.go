// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"fmt"
	"strconv"
)

func (c *Collector) doQueryGlobalMetrics() error {
	if err := c.doQueryConnectionsUsed(); err != nil {
		return fmt.Errorf("querying server connections used error: %v", err)
	}
	if err := c.doQueryConnectionsState(); err != nil {
		return fmt.Errorf("querying server connections state error: %v", err)
	}
	if err := c.doQueryCheckpoints(); err != nil {
		return fmt.Errorf("querying database conflicts error: %v", err)
	}
	if err := c.doQueryUptime(); err != nil {
		return fmt.Errorf("querying server uptime error: %v", err)
	}
	if err := c.doQueryTXIDWraparound(); err != nil {
		return fmt.Errorf("querying txid wraparound error: %v", err)
	}
	if err := c.doQueryWALWrites(); err != nil {
		return fmt.Errorf("querying wal writes error: %v", err)
	}
	if err := c.doQueryCatalogRelations(); err != nil {
		return fmt.Errorf("querying catalog relations error: %v", err)
	}
	if c.pgVersion >= pgVersion94 {
		if err := c.doQueryAutovacuumWorkers(); err != nil {
			return fmt.Errorf("querying autovacuum workers error: %v", err)
		}
	}
	if c.pgVersion >= pgVersion10 {
		if err := c.doQueryXactQueryRunningTime(); err != nil {
			return fmt.Errorf("querying xact/query running time: %v", err)
		}
	}

	if !c.isSuperUser() {
		return nil
	}

	if c.pgVersion >= pgVersion94 {
		if err := c.doQueryWALFiles(); err != nil {
			return fmt.Errorf("querying wal files error: %v", err)
		}
	}
	if err := c.doQueryWALArchiveFiles(); err != nil {
		return fmt.Errorf("querying wal archive files error: %v", err)
	}

	return nil
}

func (c *Collector) doQueryConnectionsUsed() error {
	q := queryServerCurrentConnectionsUsed()

	var v string
	if err := c.doQueryRow(q, &v); err != nil {
		return err
	}

	c.mx.connUsed = parseInt(v)

	return nil
}

func (c *Collector) doQueryConnectionsState() error {
	q := queryServerConnectionsState()

	var state string
	return c.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "state":
			state = value
		case "count":
			switch state {
			case "active":
				c.mx.connStateActive = parseInt(value)
			case "idle":
				c.mx.connStateIdle = parseInt(value)
			case "idle in transaction":
				c.mx.connStateIdleInTrans = parseInt(value)
			case "idle in transaction (aborted)":
				c.mx.connStateIdleInTransAborted = parseInt(value)
			case "fastpath function call":
				c.mx.connStateFastpathFunctionCall = parseInt(value)
			case "disabled":
				c.mx.connStateDisabled = parseInt(value)
			}
		}
	})
}

func (c *Collector) doQueryCheckpoints() error {
	q := queryCheckpoints(c.pgVersion)

	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "checkpoints_timed":
			c.mx.checkpointsTimed = parseInt(value)
		case "checkpoints_req":
			c.mx.checkpointsReq = parseInt(value)
		case "checkpoint_write_time":
			c.mx.checkpointWriteTime = parseInt(value)
		case "checkpoint_sync_time":
			c.mx.checkpointSyncTime = parseInt(value)
		case "buffers_checkpoint_bytes":
			c.mx.buffersCheckpoint = parseInt(value)
		case "buffers_clean_bytes":
			c.mx.buffersClean = parseInt(value)
		case "maxwritten_clean":
			c.mx.maxwrittenClean = parseInt(value)
		case "buffers_backend_bytes":
			c.mx.buffersBackend = parseInt(value)
		case "buffers_backend_fsync":
			c.mx.buffersBackendFsync = parseInt(value)
		case "buffers_alloc_bytes":
			c.mx.buffersAlloc = parseInt(value)
		}
	})
}

func (c *Collector) doQueryUptime() error {
	q := queryServerUptime()

	var s string
	if err := c.doQueryRow(q, &s); err != nil {
		return err
	}

	c.mx.uptime = parseFloat(s)

	return nil
}

func (c *Collector) doQueryTXIDWraparound() error {
	q := queryTXIDWraparound()

	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "oldest_current_xid":
			c.mx.oldestXID = parseInt(value)
		case "percent_towards_wraparound":
			c.mx.percentTowardsWraparound = parseInt(value)
		case "percent_towards_emergency_autovacuum":
			c.mx.percentTowardsEmergencyAutovacuum = parseInt(value)
		}
	})
}

func (c *Collector) doQueryWALWrites() error {
	q := queryWALWrites(c.pgVersion)

	var v int64
	if err := c.doQueryRow(q, &v); err != nil {
		return err
	}

	c.mx.walWrites = v

	return nil
}

func (c *Collector) doQueryWALFiles() error {
	q := queryWALFiles(c.pgVersion)

	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "wal_recycled_files":
			c.mx.walRecycledFiles = parseInt(value)
		case "wal_written_files":
			c.mx.walWrittenFiles = parseInt(value)
		}
	})
}

func (c *Collector) doQueryWALArchiveFiles() error {
	q := queryWALArchiveFiles(c.pgVersion)

	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "wal_archive_files_ready_count":
			c.mx.walArchiveFilesReady = parseInt(value)
		case "wal_archive_files_done_count":
			c.mx.walArchiveFilesDone = parseInt(value)
		}
	})
}

func (c *Collector) doQueryCatalogRelations() error {
	q := queryCatalogRelations()

	var kind string
	var count, size int64
	return c.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "relkind":
			kind = value
		case "count":
			count = parseInt(value)
		case "size":
			size = parseInt(value)
		}
		if !rowEnd {
			return
		}
		// https://www.postgresql.org/docs/current/catalog-pg-class.html
		switch kind {
		case "r":
			c.mx.relkindOrdinaryTable = count
			c.mx.relkindOrdinaryTableSize = size
		case "i":
			c.mx.relkindIndex = count
			c.mx.relkindIndexSize = size
		case "S":
			c.mx.relkindSequence = count
			c.mx.relkindSequenceSize = size
		case "t":
			c.mx.relkindTOASTTable = count
			c.mx.relkindTOASTTableSize = size
		case "v":
			c.mx.relkindView = count
			c.mx.relkindViewSize = size
		case "m":
			c.mx.relkindMatView = count
			c.mx.relkindMatViewSize = size
		case "c":
			c.mx.relkindCompositeType = count
			c.mx.relkindCompositeTypeSize = size
		case "f":
			c.mx.relkindForeignTable = count
			c.mx.relkindForeignTableSize = size
		case "p":
			c.mx.relkindPartitionedTable = count
			c.mx.relkindPartitionedTableSize = size
		case "I":
			c.mx.relkindPartitionedIndex = count
			c.mx.relkindPartitionedIndexSize = size
		}
	})
}

func (c *Collector) doQueryAutovacuumWorkers() error {
	q := queryAutovacuumWorkers()

	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "autovacuum_analyze":
			c.mx.autovacuumWorkersAnalyze = parseInt(value)
		case "autovacuum_vacuum_analyze":
			c.mx.autovacuumWorkersVacuumAnalyze = parseInt(value)
		case "autovacuum_vacuum":
			c.mx.autovacuumWorkersVacuum = parseInt(value)
		case "autovacuum_vacuum_freeze":
			c.mx.autovacuumWorkersVacuumFreeze = parseInt(value)
		case "autovacuum_brin_summarize":
			c.mx.autovacuumWorkersBrinSummarize = parseInt(value)
		}
	})
}

func (c *Collector) doQueryXactQueryRunningTime() error {
	q := queryXactQueryRunningTime()

	var state string
	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "state":
			state = value
		case "xact_running_time":
			v, _ := strconv.ParseFloat(value, 64)
			c.mx.xactTimeHist.Observe(v)
		case "query_running_time":
			if state == "active" {
				v, _ := strconv.ParseFloat(value, 64)
				c.mx.queryTimeHist.Observe(v)
			}
		}
	})
}
