// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DYNCFG_H
#define NETDATA_DYNCFG_H

#include "../common.h"

#include "../../database/rrd.h"
#include "../../database/rrdfunctions.h"

#ifdef DYNCFG_INTERNALS
typedef struct dyncfg {
    RRDHOST *host;
    uuid_t host_uuid;
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
void dyncfg_load(const char *filename);
void dyncfg_save(const char *id, DYNCFG *df);

void dyncfg_cleanup(DYNCFG *v);
#endif

void dyncfg_add_streaming(BUFFER *wb);
bool dyncfg_available_for_rrdhost(RRDHOST *host);
void dyncfg_host_init(RRDHOST *host);

// low-level API used by plugins.d and high-level API
bool dyncfg_add_low_level(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type,
                          DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds,
                          usec_t created_ut, usec_t modified_ut, bool sync,
                          rrd_function_execute_cb_t execute_cb, void *execute_cb_data);
void dyncfg_del_low_level(RRDHOST *host, const char *id);
void dyncfg_init_low_level(bool load_saved);

// high-level API for internal modules
bool dyncfg_add(RRDHOST *host, const char *id, const char *path, DYNCFG_TYPE type,
                DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, dyncfg_cb_t cb, void *data);
void dyncfg_del(RRDHOST *host, const char *id);
void dyncfg_init(bool load_saved);

#endif //NETDATA_DYNCFG_H
