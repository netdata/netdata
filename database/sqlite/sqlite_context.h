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
typedef struct ctx_context {
    char *context;
    time_t first_time_t;
    time_t last_time_t;
} ctx_context_t;

extern void ctx_get_context_list(void (*dict_cb)(void *));

extern void ctx_get_chart_list(uuid_t *host_id, void (*dict_cb)(void *));
extern void ctx_get_label_list(uuid_t *chart_id, void (*dict_cb)(void *));
extern void ctx_get_dimension_list(uuid_t *chart_id, void (*dict_cb)(void *));

extern int ctx_store_context(ctx_context_t *context_data);

#define ctx_update_context(context_data)    ctx_store_context(context_data)

extern int ctx_delete_context(ctx_context_t *context_data);

extern int sql_init_context_database(int memory);
extern int ctx_unittest(void);
#endif //NETDATA_SQLITE_CONTEXT_H
