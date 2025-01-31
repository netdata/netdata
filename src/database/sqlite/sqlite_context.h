// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_CONTEXT_H
#define NETDATA_SQLITE_CONTEXT_H

#include "database/rrd.h"
#include "database/sqlite/vendored/sqlite3.h"

int sql_context_cache_stats(int op);
typedef struct ctx_chart {
    nd_uuid_t chart_id;
    const char *id;
    const char *name;
    const char *context;
    const char *title;
    const char *units;
    const char *family;
    int chart_type;
    int priority;
    int update_every;
} SQL_CHART_DATA;

typedef struct ctx_dimension {
    nd_uuid_t dim_id;
    char *id;
    char *name;
    bool hidden;

    char *context;
    char *chart_id;
} SQL_DIMENSION_DATA;

typedef struct ctx_label {
    char *label_key;
    char *label_value;
    int label_source;
} SQL_CLABEL_DATA;

// Structure to store or delete
typedef struct versioned_context_data {
    uint64_t version;       // the version of this context as EPOCH in seconds

    const char *id;         // the id of the context
    const char *title;      // the title of the context
    const char *chart_type; // the chart_type of the context
    const char *units;      // the units of the context
    const char *family;     // the family of the context

    uint64_t priority;      // the chart priority of the context

    uint64_t first_time_s;  // the first entry in the database, in seconds
    uint64_t last_time_s;   // the last point in the database, in seconds

    bool deleted;           // true when this is deleted

} VERSIONED_CONTEXT_DATA;

void ctx_get_context_list(nd_uuid_t *host_uuid, void (*dict_cb)(VERSIONED_CONTEXT_DATA *, void *), void *data);

void ctx_get_chart_list(nd_uuid_t *host_uuid, void (*dict_cb)(SQL_CHART_DATA *, void *), void *data);
void ctx_get_label_list(nd_uuid_t *chart_uuid, void (*dict_cb)(SQL_CLABEL_DATA *, void *), void *data);
void ctx_get_dimension_list(nd_uuid_t *host_uuid, void (*dict_cb)(SQL_DIMENSION_DATA *, void *), void *data);

int ctx_store_context(nd_uuid_t *host_uuid, VERSIONED_CONTEXT_DATA *context_data);
#define ctx_update_context(host_uuid, context_data)    ctx_store_context(host_uuid, context_data)

int ctx_delete_context(nd_uuid_t *host_id, VERSIONED_CONTEXT_DATA *context_data);

int sql_init_context_database(int memory);
uint64_t sqlite_get_context_space(void);
int ctx_unittest(void);
#endif //NETDATA_SQLITE_CONTEXT_H
