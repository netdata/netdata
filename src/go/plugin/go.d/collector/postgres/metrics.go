// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

type pgMetrics struct {
	srvMetrics
	dbs       map[string]*dbMetrics
	tables    map[string]*tableMetrics
	indexes   map[string]*indexMetrics
	replApps  map[string]*replStandbyAppMetrics
	replSlots map[string]*replSlotMetrics
}

type srvMetrics struct {
	xactTimeHist  metrix.Histogram
	queryTimeHist metrix.Histogram

	maxConnections int64
	maxLocksHeld   int64

	uptime int64

	relkindOrdinaryTable        int64
	relkindIndex                int64
	relkindSequence             int64
	relkindTOASTTable           int64
	relkindView                 int64
	relkindMatView              int64
	relkindCompositeType        int64
	relkindForeignTable         int64
	relkindPartitionedTable     int64
	relkindPartitionedIndex     int64
	relkindOrdinaryTableSize    int64
	relkindIndexSize            int64
	relkindSequenceSize         int64
	relkindTOASTTableSize       int64
	relkindViewSize             int64
	relkindMatViewSize          int64
	relkindCompositeTypeSize    int64
	relkindForeignTableSize     int64
	relkindPartitionedTableSize int64
	relkindPartitionedIndexSize int64

	connUsed                      int64
	connStateActive               int64
	connStateIdle                 int64
	connStateIdleInTrans          int64
	connStateIdleInTransAborted   int64
	connStateFastpathFunctionCall int64
	connStateDisabled             int64

	checkpointsTimed    int64
	checkpointsReq      int64
	checkpointWriteTime int64
	checkpointSyncTime  int64
	buffersCheckpoint   int64
	buffersClean        int64
	maxwrittenClean     int64
	buffersBackend      int64
	buffersBackendFsync int64
	buffersAlloc        int64

	oldestXID                         int64
	percentTowardsWraparound          int64
	percentTowardsEmergencyAutovacuum int64

	walWrites            int64
	walRecycledFiles     int64
	walWrittenFiles      int64
	walArchiveFilesReady int64
	walArchiveFilesDone  int64

	autovacuumWorkersAnalyze       int64
	autovacuumWorkersVacuumAnalyze int64
	autovacuumWorkersVacuum        int64
	autovacuumWorkersVacuumFreeze  int64
	autovacuumWorkersBrinSummarize int64
}

type dbMetrics struct {
	name string

	updated   bool
	hasCharts bool

	numBackends  int64
	datConnLimit int64
	xactCommit   int64
	xactRollback int64
	blksRead     incDelta
	blksHit      incDelta
	tupReturned  incDelta
	tupFetched   incDelta
	tupInserted  int64
	tupUpdated   int64
	tupDeleted   int64
	conflicts    int64
	tempFiles    int64
	tempBytes    int64
	deadlocks    int64

	size *int64 // need 'connect' privilege for pg_database_size()

	conflTablespace int64
	conflLock       int64
	conflSnapshot   int64
	conflBufferpin  int64
	conflDeadlock   int64

	accessShareLockHeld             int64
	rowShareLockHeld                int64
	rowExclusiveLockHeld            int64
	shareUpdateExclusiveLockHeld    int64
	shareLockHeld                   int64
	shareRowExclusiveLockHeld       int64
	exclusiveLockHeld               int64
	accessExclusiveLockHeld         int64
	accessShareLockAwaited          int64
	rowShareLockAwaited             int64
	rowExclusiveLockAwaited         int64
	shareUpdateExclusiveLockAwaited int64
	shareLockAwaited                int64
	shareRowExclusiveLockAwaited    int64
	exclusiveLockAwaited            int64
	accessExclusiveLockAwaited      int64
}

type replStandbyAppMetrics struct {
	name string

	updated   bool
	hasCharts bool

	walSentDelta   int64
	walWriteDelta  int64
	walFlushDelta  int64
	walReplayDelta int64

	walWriteLag  int64
	walFlushLag  int64
	walReplayLag int64
}

type replSlotMetrics struct {
	name string

	updated   bool
	hasCharts bool

	walKeep int64
	files   int64
}

type tableMetrics struct {
	name       string
	parentName string
	db         string
	schema     string

	updated                  bool
	hasCharts                bool
	hasLastAutoVacuumChart   bool
	hasLastVacuumChart       bool
	hasLastAutoAnalyzeChart  bool
	hasLastAnalyzeChart      bool
	hasTableIOCharts         bool
	hasTableIdxIOCharts      bool
	hasTableTOASTIOCharts    bool
	hasTableTOASTIdxIOCharts bool

	// pg_stat_user_tables
	seqScan            int64
	seqTupRead         int64
	idxScan            int64
	idxTupFetch        int64
	nTupIns            int64
	nTupUpd            incDelta
	nTupDel            int64
	nTupHotUpd         incDelta
	nLiveTup           int64
	nDeadTup           int64
	lastVacuumAgo      int64
	lastAutoVacuumAgo  int64
	lastAnalyzeAgo     int64
	lastAutoAnalyzeAgo int64
	vacuumCount        int64
	autovacuumCount    int64
	analyzeCount       int64
	autoAnalyzeCount   int64

	// pg_statio_user_tables
	heapBlksRead  incDelta
	heapBlksHit   incDelta
	idxBlksRead   incDelta
	idxBlksHit    incDelta
	toastBlksRead incDelta
	toastBlksHit  incDelta
	tidxBlksRead  incDelta
	tidxBlksHit   incDelta

	totalSize int64

	bloatSize     *int64 // need 'SELECT' access to the table
	bloatSizePerc *int64 // need 'SELECT' access to the table
	nullColumns   *int64 // need 'SELECT' access to the table
}

type indexMetrics struct {
	name        string
	db          string
	schema      string
	table       string
	parentTable string

	updated   bool
	hasCharts bool

	idxScan     int64
	idxTupRead  int64
	idxTupFetch int64

	size int64

	bloatSize     *int64 // need 'SELECT' access to the table
	bloatSizePerc *int64 // need 'SELECT' access to the table
}
type incDelta struct{ prev, last int64 }

func (pc *incDelta) delta() int64 { return pc.last - pc.prev }
