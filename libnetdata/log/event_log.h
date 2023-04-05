// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EVENT_LOG_H
#define NETDATA_EVENT_LOG_H 1

#include "daemon/common.h"

EVENT_LOG_ENTRY* event_log_create_entry(
    char *name,
    char *info);

void event_log_add_entry(RRDHOST *host, EVENT_LOG_ENTRY *ee);
void event_log_init(RRDHOST *host);
void event_log2json(RRDHOST *host, BUFFER *wb);

#endif //NETDATA_EVENT_LOG_H
