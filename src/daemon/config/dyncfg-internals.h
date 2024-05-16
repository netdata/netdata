// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DYNCFG_INTERNALS_H
#define NETDATA_DYNCFG_INTERNALS_H

#include "../common.h"
#include "database/rrd.h"
#include "database/rrdfunctions.h"
#include "database/rrdfunctions-internals.h"
#include "database/rrdcollector-internals.h"

typedef struct dyncfg {
    ND_UUID host_uuid;
    STRING *function;
    STRING *template;
    STRING *path;
    DYNCFG_CMDS cmds;
    DYNCFG_TYPE type;

    HTTP_ACCESS view_access;
    HTTP_ACCESS edit_access;

    struct {
        DYNCFG_STATUS status;
        DYNCFG_SOURCE_TYPE source_type;
        STRING *source;
        usec_t created_ut;
        usec_t modified_ut;
    } current;

    struct {
        uint32_t saves;
        bool restart_required;
        bool plugin_rejected;
        bool user_disabled;
        DYNCFG_STATUS status;
        DYNCFG_SOURCE_TYPE source_type;
        STRING *source;
        BUFFER *payload;
        usec_t created_ut;
        usec_t modified_ut;
    } dyncfg;

    bool sync;
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

void dyncfg_echo_cb(BUFFER *wb, int code, void *result_cb_data);
void dyncfg_echo(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id, DYNCFG_CMDS cmd);
void dyncfg_echo_update(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id);
void dyncfg_echo_add(const DICTIONARY_ITEM *item_template, const DICTIONARY_ITEM *item_job, DYNCFG *df_template, DYNCFG *df_job, const char *template_id, const char *job_name);

const DICTIONARY_ITEM *dyncfg_add_internal(RRDHOST *host, const char *id, const char *path,
                                           DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type,
                                           const char *source, DYNCFG_CMDS cmds,
                                           usec_t created_ut, usec_t modified_ut,
                                           bool sync, HTTP_ACCESS view_access, HTTP_ACCESS edit_access,
                                           rrd_function_execute_cb_t execute_cb, void *execute_cb_data,
                                           bool overwrite_cb);

int dyncfg_function_intercept_cb(struct rrd_function_execute *rfe, void *data);
void dyncfg_cleanup(DYNCFG *v);

const DICTIONARY_ITEM *dyncfg_get_template_of_new_job(const char *job_id);

bool dyncfg_is_user_disabled(const char *id);

RRDHOST *dyncfg_rrdhost_by_uuid(ND_UUID *uuid);
RRDHOST *dyncfg_rrdhost(DYNCFG *df);

static inline void dyncfg_copy_dyncfg_source_to_current(DYNCFG *df) {
    STRING *old = df->current.source;
    df->current.source = string_dup(df->dyncfg.source);
    string_freez(old);
}

static inline void dyncfg_set_dyncfg_source_from_txt(DYNCFG *df, const char *source) {
    STRING *old = df->dyncfg.source;
    df->dyncfg.source = string_strdupz(source);
    string_freez(old);
}

static inline void dyncfg_set_current_from_dyncfg(DYNCFG *df) {
    df->current.status = df->dyncfg.status;
    df->current.source_type = df->dyncfg.source_type;

    dyncfg_copy_dyncfg_source_to_current(df);

    if(df->dyncfg.created_ut < df->current.created_ut)
        df->current.created_ut = df->dyncfg.created_ut;

    if(df->dyncfg.modified_ut > df->current.modified_ut)
        df->current.modified_ut = df->dyncfg.modified_ut;
}

static inline void dyncfg_update_status_on_successful_add_or_update(DYNCFG *df, int code) {
    df->dyncfg.plugin_rejected = false;

    if (code == DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED)
        df->dyncfg.restart_required = true;
    else
        df->dyncfg.restart_required = false;

    dyncfg_set_current_from_dyncfg(df);
}

static inline DYNCFG_STATUS dyncfg_status_from_successful_response(int code) {
    DYNCFG_STATUS status = DYNCFG_STATUS_ACCEPTED;

    switch(code) {
        default:
        case DYNCFG_RESP_ACCEPTED:
        case DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED:
            status = DYNCFG_STATUS_ACCEPTED;
            break;

        case DYNCFG_RESP_ACCEPTED_DISABLED:
            status = DYNCFG_STATUS_DISABLED;
            break;

        case DYNCFG_RESP_RUNNING:
            status = DYNCFG_STATUS_RUNNING;
            break;

    }

    return status;
}

#endif //NETDATA_DYNCFG_INTERNALS_H
