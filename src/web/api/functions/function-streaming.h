// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTION_STREAMING_H
#define NETDATA_FUNCTION_STREAMING_H

#include "database/rrd.h"

#define RRDFUNCTIONS_STREAMING_HELP "Shows real-time streaming connections and replication status between parent and child nodes, including connection health, data flow metrics, and ML status."

int function_streaming(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

#endif //NETDATA_FUNCTION_STREAMING_H
