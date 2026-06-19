// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTION_TOPOLOGY_STREAMING_H
#define NETDATA_FUNCTION_TOPOLOGY_STREAMING_H

#include "database/rrd.h"

#define RRDFUNCTIONS_STREAMING_TOPOLOGY_HELP "Shows streaming topology relations across Netdata agents, including directional parent/child links and transport metadata."

int function_streaming_topology(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

#endif //NETDATA_FUNCTION_TOPOLOGY_STREAMING_H
