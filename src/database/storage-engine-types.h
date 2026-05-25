// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGE_ENGINE_TYPES_H
#define NETDATA_STORAGE_ENGINE_TYPES_H

// Leaf header carved from storage-engine.h to decouple the dbengine source
// tree from the kitchen-sink database/rrd.h. Anything in here must depend
// only on libnetdata and rrd-database-mode.h, never on daemon or
// other database/ headers.

#include "libnetdata/libnetdata.h"
#include "rrd-database-mode.h"

// Forward declarations of daemon-side types referenced by the storage
// engine API. Concrete definitions live elsewhere; this header only
// declares them as opaque.
typedef struct rrddim RRDDIM;
typedef struct rrdset RRDSET;

// Backfill policy. Defined here (rather than in rrddim-backfill.h) so
// the dbengine library and other low-level consumers can reference it
// without dragging in the RRDDIM definition.
typedef enum __attribute__ ((__packed__)) {
    RRD_BACKFILL_NONE = 0,
    RRD_BACKFILL_FULL,
    RRD_BACKFILL_NEW
} RRD_BACKFILL;

typedef struct storage_query_handle STORAGE_QUERY_HANDLE;

typedef enum __attribute__ ((__packed__)) storage_priority {
    STORAGE_PRIORITY_INTERNAL_DBENGINE = 0,
    STORAGE_PRIORITY_INTERNAL_QUERY_PREP,

    // query priorities
    STORAGE_PRIORITY_HIGH,
    STORAGE_PRIORITY_NORMAL,
    STORAGE_PRIORITY_SYNCHRONOUS_FIRST,
    STORAGE_PRIORITY_LOW,
    STORAGE_PRIORITY_BEST_EFFORT,

    // synchronous query, not to be dispatched to workers or queued
    STORAGE_PRIORITY_SYNCHRONOUS,

    STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE,
} STORAGE_PRIORITY;

typedef enum __attribute__ ((__packed__)) {
    STORAGE_ENGINE_BACKEND_RRDDIM = 1,
    STORAGE_ENGINE_BACKEND_DBENGINE = 2,
} STORAGE_ENGINE_BACKEND;

#define is_valid_backend(backend) ((backend) >= STORAGE_ENGINE_BACKEND_RRDDIM && (backend) <= STORAGE_ENGINE_BACKEND_DBENGINE)

// iterator state for RRD dimension data queries
struct storage_engine_query_handle {
    time_t start_time_s;
    time_t end_time_s;
    STORAGE_PRIORITY priority;
    STORAGE_ENGINE_BACKEND seb;
    STORAGE_QUERY_HANDLE *handle;
};

// non-existing structs instead of voids
// to enable type checking at compile time
typedef struct storage_instance STORAGE_INSTANCE;
typedef struct storage_metric_handle STORAGE_METRIC_HANDLE;
typedef struct storage_alignment STORAGE_METRICS_GROUP;

// engine-specific iterator state for dimension data collection
typedef struct storage_collect_handle {
    STORAGE_ENGINE_BACKEND seb;
} STORAGE_COLLECT_HANDLE;

#endif // NETDATA_STORAGE_ENGINE_TYPES_H
