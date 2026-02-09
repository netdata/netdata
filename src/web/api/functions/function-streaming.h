// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTION_STREAMING_H
#define NETDATA_FUNCTION_STREAMING_H

#include "database/rrd.h"

#define RRDFUNCTIONS_STREAMING_HELP "Shows real-time streaming connections and replication status between parent and child nodes, including connection health, data flow metrics, and ML status."
#define RRDFUNCTIONS_STREAMING_TOPOLOGY_HELP "Shows streaming topology relations across Netdata agents, including directional parent/child links and transport metadata."

int function_streaming(BUFFER *wb, const char *function, BUFFER *payload, const char *source);
int function_streaming_topology(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

#endif //NETDATA_FUNCTION_STREAMING_H
