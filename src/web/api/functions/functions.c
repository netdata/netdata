// SPDX-License-Identifier: GPL-3.0-or-later

#include "functions.h"

void global_functions_add(void) {
    // we register this only on localhost
    // for the other nodes, the origin server should register it
    rrd_function_add_inline(
        localhost,
        NULL,
        "netdata-streaming",
        10,
        RRDFUNCTIONS_PRIORITY_DEFAULT + 1,
        RRDFUNCTIONS_VERSION_DEFAULT,
        RRDFUNCTIONS_STREAMING_HELP,
        "top",
        HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        function_streaming);

    rrd_function_add_inline(
        localhost,
        NULL,
        "netdata-api-calls",
        10,
        RRDFUNCTIONS_PRIORITY_DEFAULT + 1,
        RRDFUNCTIONS_VERSION_DEFAULT,
        RRDFUNCTIONS_PROGRESS_HELP,
        "top",
        HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        function_progress);

    rrd_function_add_inline(
        localhost,
        NULL,
        RRDFUNCTIONS_BEARER_GET_TOKEN,
        10,
        RRDFUNCTIONS_PRIORITY_DEFAULT + 3,
        RRDFUNCTIONS_VERSION_DEFAULT,
        RRDFUNCTIONS_BEARER_GET_TOKEN_HELP,
        RRDFUNCTIONS_TAG_HIDDEN,
        HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        function_bearer_get_token);

    rrd_function_add_inline(
        localhost,
        NULL,
        "netdata-metrics-cardinality",
        10,
        RRDFUNCTIONS_PRIORITY_DEFAULT + 1,
        RRDFUNCTIONS_VERSION_DEFAULT,
        RRDFUNCTIONS_METRICS_CARDINALITY_HELP,
        "top",
        HTTP_ACCESS_ANONYMOUS_DATA,
        function_metrics_cardinality);
}
