// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGE_ENGINE_TYPES_H
#define NETDATA_STORAGE_ENGINE_TYPES_H

#include "libnetdata/libnetdata.h"

typedef struct rrddim RRDDIM;
typedef struct storage_instance STORAGE_INSTANCE;
typedef struct storage_metric_handle STORAGE_METRIC_HANDLE;
typedef struct storage_alignment STORAGE_METRICS_GROUP;
typedef struct storage_collect_handle STORAGE_COLLECT_HANDLE;
typedef struct storage_query_handle STORAGE_QUERY_HANDLE;

typedef enum __attribute__ ((__packed__)) {
    STORAGE_ENGINE_NONE     = 0,
    STORAGE_ENGINE_RAM      = 1,
    STORAGE_ENGINE_MAP      = 2,
    STORAGE_ENGINE_SAVE     = 3,
    STORAGE_ENGINE_ALLOC    = 4,
    STORAGE_ENGINE_DBENGINE = 5,
} STORAGE_ENGINE_ID;

#define STORAGE_ENGINE_NONE_NAME     "none"
#define STORAGE_ENGINE_RAM_NAME      "ram"
#define STORAGE_ENGINE_MAP_NAME      "map"
#define STORAGE_ENGINE_SAVE_NAME     "save"
#define STORAGE_ENGINE_ALLOC_NAME    "alloc"
#define STORAGE_ENGINE_DBENGINE_NAME "dbengine"

const char *storage_engine_name(STORAGE_ENGINE_ID id);
bool storage_engine_id(const char *name, STORAGE_ENGINE_ID *id);

typedef enum __attribute__ ((__packed__)) storage_priority {
    STORAGE_PRIORITY_INTERNAL_DBENGINE = 0,
    STORAGE_PRIORITY_INTERNAL_QUERY_PREP,

    STORAGE_PRIORITY_HIGH,
    STORAGE_PRIORITY_NORMAL,
    STORAGE_PRIORITY_LOW,
    STORAGE_PRIORITY_BEST_EFFORT,

    STORAGE_PRIORITY_SYNCHRONOUS,

    STORAGE_PRIORITY_INTERNAL_MAX_DONT_USE,
} STORAGE_PRIORITY;

struct storage_engine_query_handle {
    time_t start_time_s;
    time_t end_time_s;
    STORAGE_PRIORITY priority;
    STORAGE_ENGINE_ID id;
    STORAGE_QUERY_HANDLE *handle;
};

#endif /* NETDATA_STORAGE_ENGINE_TYPES_H */