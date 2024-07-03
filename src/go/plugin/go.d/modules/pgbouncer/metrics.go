// SPDX-License-Identifier: GPL-3.0-or-later

package pgbouncer

type metrics struct {
	dbs map[string]*dbMetrics
}

// dbMetrics represents PgBouncer database (not the PostgreSQL database of the outgoing connection).
type dbMetrics struct {
	name     string
	pgDBName string

	updated   bool
	hasCharts bool

	// command 'SHOW DATABASES;'
	maxConnections     int64
	currentConnections int64
	paused             int64
	disabled           int64

	// command 'SHOW STATS;'
	// https://github.com/pgbouncer/pgbouncer/blob/9a346b0e451d842d7202abc3eccf0ff5a66b2dd6/src/stats.c#L76
	totalXactCount  int64 // v1.8+
	totalQueryCount int64 // v1.8+
	totalReceived   int64
	totalSent       int64
	totalXactTime   int64 // v1.8+
	totalQueryTime  int64
	totalWaitTime   int64 // v1.8+
	avgXactTime     int64 // v1.8+
	avgQueryTime    int64

	// command 'SHOW POOLS;'
	// https://github.com/pgbouncer/pgbouncer/blob/9a346b0e451d842d7202abc3eccf0ff5a66b2dd6/src/admin.c#L804
	clActive    int64
	clWaiting   int64
	clCancelReq int64
	svActive    int64
	svIdle      int64
	svUsed      int64
	svTested    int64
	svLogin     int64
	maxWait     int64
	maxWaitUS   int64 // v1.8+
}
