// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTION_METRICS_CARDINALITY_H
#define NETDATA_FUNCTION_METRICS_CARDINALITY_H

#include "libnetdata/libnetdata.h"

#define RRDFUNCTIONS_METRICS_CARDINALITY_HELP "Displays metrics cardinality statistics showing distribution of instances and time-series across contexts and nodes. To change grouping, append parameter to function name: 'netdata-metrics-cardinality' (default, group by context) or 'netdata-metrics-cardinality group:by-node' (group by node)."

int function_metrics_cardinality(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

#endif //NETDATA_FUNCTION_METRICS_CARDINALITY_H
