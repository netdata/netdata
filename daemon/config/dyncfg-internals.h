// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DYNCFG_INTERNALS_H
#define NETDATA_DYNCFG_INTERNALS_H

#include "../common.h"
#include "../../database/rrd.h"
#include "../../database/rrdfunctions.h"
#include "../../database/rrdfunctions-internals.h"
#include "../../database/rrdcollector-internals.h"

typedef struct dyncfg {
    RRDHOST *host;
    uuid_t host_uuid;
    STRING *function;
    STRING *template;
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
    bool plugin_rejected;
    bool restart_required;

    BUFFER *payload;

    rrd_function_execute_cb_t execute_cb;
    void *execute_cb_data;

    // constructor data
    bool overwrite_cb;
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

void dyncfg_echo_cb(BUFFER *wb, int code, void *result_cb_data);
void dyncfg_echo(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id, DYNCFG_CMDS cmd);
void dyncfg_echo_update(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id);
void dyncfg_echo_add(const DICTIONARY_ITEM *template_item, DYNCFG *template_df, const char *template_id, const char *job_name);

const DICTIONARY_ITEM *dyncfg_add_internal(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data, bool overwrite_cb);
int dyncfg_function_intercept_cb(struct rrd_function_execute *rfe, void *data);
void dyncfg_cleanup(DYNCFG *v);

bool dyncfg_is_user_disabled(const char *id);

#endif //NETDATA_DYNCFG_INTERNALS_H
