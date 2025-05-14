// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCONTEXT_CONTEXT_REGISTRY_H
#define NETDATA_RRDCONTEXT_CONTEXT_REGISTRY_H

#include "libnetdata/libnetdata.h"

void rrdcontext_context_registry_destroy(void);

bool rrdcontext_context_registry_add(STRING *context);
bool rrdcontext_context_registry_remove(STRING *context);

size_t rrdcontext_context_registry_unique_count(void);

void rrdcontext_context_registry_json_mcp_array(BUFFER *wb, SIMPLE_PATTERN *pattern);

#endif // NETDATA_RRDCONTEXT_CONTEXT_REGISTRY_H