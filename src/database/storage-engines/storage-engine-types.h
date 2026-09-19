// SPDX-License-Identifier: GPL-3.0-or-later

// The plain types shared by the storage-engine vtable and the engines that implement it: the vocabulary an
// engine's public API is written in. It lives beside the dispatcher, owned by neither engine and by nothing in
// libnetdata; RRDDIM is only forward-declared here for the vtable, and the RRDHOST/RRDSET/RRDDIM definitions stay
// in the daemon's RRD headers.

#ifndef NETDATA_STORAGE_ENGINE_TYPES_H
#define NETDATA_STORAGE_ENGINE_TYPES_H

#include <time.h>

typedef struct rrddim RRDDIM;

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
    STORAGE_ENGINE_BACKEND_RAM = 1,
    STORAGE_ENGINE_BACKEND_DBENGINE = 2,
} STORAGE_ENGINE_BACKEND;

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

// --------------------------------------------------------------------------------------------------------------------
// engine-specific iterator state for dimension data collection

typedef struct storage_collect_handle {
    STORAGE_ENGINE_BACKEND seb;
} STORAGE_COLLECT_HANDLE;

#endif // NETDATA_STORAGE_ENGINE_TYPES_H
