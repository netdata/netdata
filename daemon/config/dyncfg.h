// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DYNCFG_H
#define NETDATA_DYNCFG_H

#include "../common.h"


#include "../../database/rrd.h"
#include "../../database/rrdfunctions.h"

bool dyncfg_add(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data);
void dyncfg_add_streaming(BUFFER *wb);
bool dyncfg_available_for_rrdhost(RRDHOST *host);
void dyncfg_host_init(RRDHOST *host);

void dyncfg_init(bool load_saved);

#endif //NETDATA_DYNCFG_H
