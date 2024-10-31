// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DYNCFG_H
#define NETDATA_DYNCFG_H

#include "../common.h"
#include "database/rrd.h"
#include "database/rrdfunctions.h"

#define DYNCFG_FUNCTIONS_VERSION 0

void dyncfg_add_streaming(BUFFER *wb);
bool dyncfg_available_for_rrdhost(RRDHOST *host);
void dyncfg_host_init(RRDHOST *host);

// low-level API used by plugins.d and high-level API
bool dyncfg_add_low_level(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type,
                          DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds,
                          usec_t created_ut, usec_t modified_ut, bool sync,
                          HTTP_ACCESS view_access, HTTP_ACCESS edit_access,
                          rrd_function_execute_cb_t execute_cb, void *execute_cb_data);
void dyncfg_del_low_level(RRDHOST *host, const char *id);
void dyncfg_status_low_level(RRDHOST *host, const char *id, DYNCFG_STATUS status);
void dyncfg_init_low_level(bool load_saved);

// high-level API for internal modules
bool dyncfg_add(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type,
                DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds,
                HTTP_ACCESS view_access, HTTP_ACCESS edit_access,
                dyncfg_cb_t cb, void *data);
void dyncfg_del(RRDHOST *host, const char *id);
void dyncfg_status(RRDHOST *host, const char *id, DYNCFG_STATUS status);

void dyncfg_init(bool load_saved);

#endif //NETDATA_DYNCFG_H
