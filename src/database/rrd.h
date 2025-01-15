// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD_H
#define NETDATA_RRD_H 1

#ifdef __cplusplus
extern "C" {
#endif

#include "libnetdata/libnetdata.h"

// --------------------------------------------------------------------------------------------------------------------

struct ml_metrics_statistics {
    size_t anomalous;
    size_t normal;
    size_t trained;
    size_t pending;
    size_t silenced;
};

// --------------------------------------------------------------------------------------------------------------------

typedef enum __attribute__ ((__packed__)) {
    QUERY_SOURCE_UNKNOWN = 0,
    QUERY_SOURCE_API_DATA,
    QUERY_SOURCE_API_BADGE,
    QUERY_SOURCE_API_WEIGHTS,
    QUERY_SOURCE_HEALTH,
    QUERY_SOURCE_ML,
    QUERY_SOURCE_UNITTEST,
} QUERY_SOURCE;

// --------------------------------------------------------------------------------------------------------------------

typedef struct rrdcalc RRDCALC;
typedef struct alarm_entry ALARM_ENTRY;
typedef struct rrdvar_acquired RRDVAR_ACQUIRED;
typedef struct rrdcalc_acquired RRDCALC_ACQUIRED;

#ifdef ENABLE_DBENGINE
struct rrdengine_instance;
#endif

// --------------------------------------------------------------------------------------------------------------------

#define UPDATE_EVERY_MIN 1
#define UPDATE_EVERY_MAX 3600
#define RRD_ID_LENGTH_MAX 1200

#define RRD_DEFAULT_HISTORY_ENTRIES 3600
#define RRD_HISTORY_ENTRIES_MAX (86400*365)

#if defined(ENV32BIT)
#define MIN_LIBUV_WORKER_THREADS 8
#define MAX_LIBUV_WORKER_THREADS 128
#define RESERVED_LIBUV_WORKER_THREADS 3
#else
#define MIN_LIBUV_WORKER_THREADS 16
#define MAX_LIBUV_WORKER_THREADS 1024
#define RESERVED_LIBUV_WORKER_THREADS 6
#endif

extern int libuv_worker_threads;
extern bool ieee754_doubles;

typedef long long total_number;

// --------------------------------------------------------------------------------------------------------------------
// global lock for all RRDHOSTs

extern netdata_rwlock_t rrd_rwlock;

#define rrd_rdlock() netdata_rwlock_rdlock(&rrd_rwlock)
#define rrd_wrlock() netdata_rwlock_wrlock(&rrd_rwlock)
#define rrd_rdunlock() netdata_rwlock_rdunlock(&rrd_rwlock)
#define rrd_wrunlock() netdata_rwlock_wrunlock(&rrd_rwlock)

// --------------------------------------------------------------------------------------------------------------------

STRING *rrd_string_strdupz(const char *s);

#include "rrd-database-mode.h"
long align_entries_to_pagesize(RRD_DB_MODE mode, long entries);

static inline uint32_t get_uint32_id() {
    return now_realtime_sec() & UINT32_MAX;
}

// --------------------------------------------------------------------------------------------------------------------

#include "storage-engine.h"

#include "rrdhost.h"
#include "rrdset.h"
#include "rrddim.h"
#include "rrddim-backfill.h"

#include "streaming/stream-sender-commit.h"
#include "streaming/stream-replication-tracking.h"
#include "rrdhost-system-info.h"
#include "daemon/common.h"
#include "web/api/queries/query.h"
#include "web/api/queries/rrdr.h"
#include "health/rrdvar.h"
#include "health/rrdcalc.h"
#include "rrdlabels.h"
#include "streaming/stream-capabilities.h"
#include "streaming/stream-path.h"
#include "streaming/stream.h"
//#include "aclk/aclk_rrdhost_state.h"
#include "sqlite/sqlite_health.h"
#include "contexts/rrdcontext.h"
#include "rrdcollector.h"
#include "rrdfunctions.h"
#ifdef ENABLE_DBENGINE
#include "database/engine/rrdengineapi.h"
#endif
#include "sqlite/sqlite_functions.h"
#include "sqlite/sqlite_context.h"
#include "sqlite/sqlite_metadata.h"
#include "sqlite/sqlite_aclk.h"
#include "sqlite/sqlite_aclk_alert.h"
#include "sqlite/sqlite_aclk_node.h"
#include "sqlite/sqlite_health.h"

// --------------------------------------------------------------------------------------------------------------------

int rrd_init(const char *hostname, struct rrdhost_system_info *system_info, bool unittest);

#ifdef __cplusplus
}
#endif

#endif /* NETDATA_RRD_H */
