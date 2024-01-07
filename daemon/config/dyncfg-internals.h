// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DYNCFG_INTERNALS_H
#define NETDATA_DYNCFG_INTERNALS_H

#define RRD_COLLECTOR_INTERNALS
#define RRD_FUNCTIONS_INTERNALS

#include "../common.h"
#include "../../database/rrd.h"
#include "../../database/rrdfunctions.h"

typedef struct dyncfg {
    RRDHOST *host;
    uuid_t host_uuid;
    STRING *function;
    STRING *path;
    DYNCFG_STATUS status;
    DYNCFG_TYPE type;
    DYNCFG_CMDS cmds;
    DYNCFG_SOURCE_TYPE source_type;
    STRING *source;
    usec_t created_ut;
    usec_t modified_ut;
    uint32_t saves;
    bool sync;
    bool user_disabled;
    bool restart_required;

    BUFFER *payload;

    rrd_function_execute_cb_t execute_cb;
    void *execute_cb_data;
} DYNCFG;

struct dyncfg_globals {
    const char *dir;
    DICTIONARY *nodes;
};

extern struct dyncfg_globals dyncfg_globals;

void dyncfg_load_all(void);
void dyncfg_file_load(const char *filename);
void dyncfg_file_save(const char *id, DYNCFG *df);
void dyncfg_file_delete(const char *id);

bool dyncfg_get_schema(const char *id, BUFFER *dst);

void dyncfg_echo_status(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id);
void dyncfg_echo_update(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id);
void dyncfg_echo_add(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id, const char *job_name);

const DICTIONARY_ITEM *dyncfg_add_internal(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data);
int dyncfg_function_execute_cb(uuid_t *transaction, BUFFER *result_body_wb, BUFFER *payload,
                               usec_t *stop_monotonic_ut, const char *function,
                               void *execute_cb_data __maybe_unused,
                               rrd_function_result_callback_t result_cb, void *result_cb_data,
                               rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                               rrd_function_is_cancelled_cb_t is_cancelled_cb,
                               void *is_cancelled_cb_data,
                               rrd_function_register_canceller_cb_t register_canceller_cb,
                               void *register_canceller_cb_data,
                               rrd_function_register_progresser_cb_t register_progresser_cb,
                               void *register_progresser_cb_data);

void dyncfg_cleanup(DYNCFG *v);

#endif //NETDATA_DYNCFG_INTERNALS_H
