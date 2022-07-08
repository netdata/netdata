// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_CONTEXT_H
#define NETDATA_SQLITE_CONTEXT_H

#include "daemon/common.h"
#include "sqlite3.h"

typedef struct ctx_chart {
    uuid_t chart_id;
    char *id;
    char *context;
    int update_every;
} ctx_chart_t;

typedef struct ctx_dimension {
    uuid_t dim_id;
    char *id;
} ctx_dimension_t;

typedef struct ctx_label {
    char *label_key;
    char *label_value;
    int label_source;
} ctx_label_t;

// Structure to store or delete
typedef struct versioned_context_data {
    uint64_t version;       // the version of this context as EPOCH in seconds

    const char *id;         // the id of the context
    const char *title;      // the title of the context
    const char *chart_type; // the chart_type of the context
    const char *units;      // the units of the context

    uint64_t priority;      // the chart priority of the context

    uint64_t first_time_t;  // the first entry in the database, in seconds
    uint64_t last_time_t;   // the last point in the database, in seconds

    bool deleted;           // true when this is deleted

} VERSIONED_CONTEXT_DATA;

extern void ctx_get_context_list(void (*dict_cb)(void *));

extern void ctx_get_chart_list(uuid_t *host_id, void (*dict_cb)(void *));
extern void ctx_get_label_list(uuid_t *chart_id, void (*dict_cb)(void *));
extern void ctx_get_dimension_list(uuid_t *chart_id, void (*dict_cb)(void *));

extern int ctx_store_context(VERSIONED_CONTEXT_DATA *context_data);

#define ctx_update_context(context_data)    ctx_store_context(context_data)

extern int ctx_delete_context(VERSIONED_CONTEXT_DATA *context_data);

extern int sql_init_context_database(int memory);
extern int ctx_unittest(void);
#endif //NETDATA_SQLITE_CONTEXT_H
