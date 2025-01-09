// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTION_METRICS_CARDINALITY_H
#define NETDATA_FUNCTION_METRICS_CARDINALITY_H

#include "libnetdata/libnetdata.h"

#define RRDFUNCTIONS_METRICS_CARDINALITY_HELP "Shows metrics cardinality per context across all nodes."

int function_metrics_cardinality(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

#endif //NETDATA_FUNCTION_METRICS_CARDINALITY_H
