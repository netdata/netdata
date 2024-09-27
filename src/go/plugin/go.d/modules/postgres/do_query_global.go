// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"fmt"
	"strconv"
)

func (p *Postgres) doQueryGlobalMetrics() error {
	if err := p.doQueryConnectionsUsed(); err != nil {
		return fmt.Errorf("querying server connections used error: %v", err)
	}
	if err := p.doQueryConnectionsState(); err != nil {
		return fmt.Errorf("querying server connections state error: %v", err)
	}
	if err := p.doQueryCheckpoints(); err != nil {
		return fmt.Errorf("querying database conflicts error: %v", err)
	}
	if err := p.doQueryUptime(); err != nil {
		return fmt.Errorf("querying server uptime error: %v", err)
	}
	if err := p.doQueryTXIDWraparound(); err != nil {
		return fmt.Errorf("querying txid wraparound error: %v", err)
	}
	if err := p.doQueryWALWrites(); err != nil {
		return fmt.Errorf("querying wal writes error: %v", err)
	}
	if err := p.doQueryCatalogRelations(); err != nil {
		return fmt.Errorf("querying catalog relations error: %v", err)
	}
	if p.pgVersion >= pgVersion94 {
		if err := p.doQueryAutovacuumWorkers(); err != nil {
			return fmt.Errorf("querying autovacuum workers error: %v", err)
		}
	}
	if p.pgVersion >= pgVersion10 {
		if err := p.doQueryXactQueryRunningTime(); err != nil {
			return fmt.Errorf("querying xact/query running time: %v", err)
		}
	}

	if !p.isSuperUser() {
		return nil
	}

	if p.pgVersion >= pgVersion94 {
		if err := p.doQueryWALFiles(); err != nil {
			return fmt.Errorf("querying wal files error: %v", err)
		}
	}
	if err := p.doQueryWALArchiveFiles(); err != nil {
		return fmt.Errorf("querying wal archive files error: %v", err)
	}

	return nil
}

func (p *Postgres) doQueryConnectionsUsed() error {
	q := queryServerCurrentConnectionsUsed()

	var v string
	if err := p.doQueryRow(q, &v); err != nil {
		return err
	}

	p.mx.connUsed = parseInt(v)

	return nil
}

func (p *Postgres) doQueryConnectionsState() error {
	q := queryServerConnectionsState()

	var state string
	return p.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "state":
			state = value
		case "count":
			switch state {
			case "active":
				p.mx.connStateActive = parseInt(value)
			case "idle":
				p.mx.connStateIdle = parseInt(value)
			case "idle in transaction":
				p.mx.connStateIdleInTrans = parseInt(value)
			case "idle in transaction (aborted)":
				p.mx.connStateIdleInTransAborted = parseInt(value)
			case "fastpath function call":
				p.mx.connStateFastpathFunctionCall = parseInt(value)
			case "disabled":
				p.mx.connStateDisabled = parseInt(value)
			}
		}
	})
}

func (p *Postgres) doQueryCheckpoints() error {
	q := queryCheckpoints(p.pgVersion)

	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "checkpoints_timed":
			p.mx.checkpointsTimed = parseInt(value)
		case "checkpoints_req":
			p.mx.checkpointsReq = parseInt(value)
		case "checkpoint_write_time":
			p.mx.checkpointWriteTime = parseInt(value)
		case "checkpoint_sync_time":
			p.mx.checkpointSyncTime = parseInt(value)
		case "buffers_checkpoint_bytes":
			p.mx.buffersCheckpoint = parseInt(value)
		case "buffers_clean_bytes":
			p.mx.buffersClean = parseInt(value)
		case "maxwritten_clean":
			p.mx.maxwrittenClean = parseInt(value)
		case "buffers_backend_bytes":
			p.mx.buffersBackend = parseInt(value)
		case "buffers_backend_fsync":
			p.mx.buffersBackendFsync = parseInt(value)
		case "buffers_alloc_bytes":
			p.mx.buffersAlloc = parseInt(value)
		}
	})
}

func (p *Postgres) doQueryUptime() error {
	q := queryServerUptime()

	var s string
	if err := p.doQueryRow(q, &s); err != nil {
		return err
	}

	p.mx.uptime = parseFloat(s)

	return nil
}

func (p *Postgres) doQueryTXIDWraparound() error {
	q := queryTXIDWraparound()

	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "oldest_current_xid":
			p.mx.oldestXID = parseInt(value)
		case "percent_towards_wraparound":
			p.mx.percentTowardsWraparound = parseInt(value)
		case "percent_towards_emergency_autovacuum":
			p.mx.percentTowardsEmergencyAutovacuum = parseInt(value)
		}
	})
}

func (p *Postgres) doQueryWALWrites() error {
	q := queryWALWrites(p.pgVersion)

	var v int64
	if err := p.doQueryRow(q, &v); err != nil {
		return err
	}

	p.mx.walWrites = v

	return nil
}

func (p *Postgres) doQueryWALFiles() error {
	q := queryWALFiles(p.pgVersion)

	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "wal_recycled_files":
			p.mx.walRecycledFiles = parseInt(value)
		case "wal_written_files":
			p.mx.walWrittenFiles = parseInt(value)
		}
	})
}

func (p *Postgres) doQueryWALArchiveFiles() error {
	q := queryWALArchiveFiles(p.pgVersion)

	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "wal_archive_files_ready_count":
			p.mx.walArchiveFilesReady = parseInt(value)
		case "wal_archive_files_done_count":
			p.mx.walArchiveFilesDone = parseInt(value)
		}
	})
}

func (p *Postgres) doQueryCatalogRelations() error {
	q := queryCatalogRelations()

	var kind string
	var count, size int64
	return p.doQuery(q, func(column, value string, rowEnd bool) {
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
			p.mx.relkindOrdinaryTable = count
			p.mx.relkindOrdinaryTableSize = size
		case "i":
			p.mx.relkindIndex = count
			p.mx.relkindIndexSize = size
		case "S":
			p.mx.relkindSequence = count
			p.mx.relkindSequenceSize = size
		case "t":
			p.mx.relkindTOASTTable = count
			p.mx.relkindTOASTTableSize = size
		case "v":
			p.mx.relkindView = count
			p.mx.relkindViewSize = size
		case "m":
			p.mx.relkindMatView = count
			p.mx.relkindMatViewSize = size
		case "c":
			p.mx.relkindCompositeType = count
			p.mx.relkindCompositeTypeSize = size
		case "f":
			p.mx.relkindForeignTable = count
			p.mx.relkindForeignTableSize = size
		case "p":
			p.mx.relkindPartitionedTable = count
			p.mx.relkindPartitionedTableSize = size
		case "I":
			p.mx.relkindPartitionedIndex = count
			p.mx.relkindPartitionedIndexSize = size
		}
	})
}

func (p *Postgres) doQueryAutovacuumWorkers() error {
	q := queryAutovacuumWorkers()

	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "autovacuum_analyze":
			p.mx.autovacuumWorkersAnalyze = parseInt(value)
		case "autovacuum_vacuum_analyze":
			p.mx.autovacuumWorkersVacuumAnalyze = parseInt(value)
		case "autovacuum_vacuum":
			p.mx.autovacuumWorkersVacuum = parseInt(value)
		case "autovacuum_vacuum_freeze":
			p.mx.autovacuumWorkersVacuumFreeze = parseInt(value)
		case "autovacuum_brin_summarize":
			p.mx.autovacuumWorkersBrinSummarize = parseInt(value)
		}
	})
}

func (p *Postgres) doQueryXactQueryRunningTime() error {
	q := queryXactQueryRunningTime()

	var state string
	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "state":
			state = value
		case "xact_running_time":
			v, _ := strconv.ParseFloat(value, 64)
			p.mx.xactTimeHist.Observe(v)
		case "query_running_time":
			if state == "active" {
				v, _ := strconv.ParseFloat(value, 64)
				p.mx.queryTimeHist.Observe(v)
			}
		}
	})
}
