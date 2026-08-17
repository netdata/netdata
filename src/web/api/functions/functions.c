// SPDX-License-Identifier: GPL-3.0-or-later

#include "functions.h"

void global_functions_add(void) {
    // we register this only on localhost
    // for the other nodes, the origin server should register it
    nrpc_method_register_builtin(
        localhost,
        NULL,
        "netdata-streaming",
        10,
        NRPC_PRIORITY_DEFAULT + 1,
        NRPC_VERSION_DEFAULT,
        FUNCTION_STREAMING_HELP,
        "top",
        HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        function_netdata_streaming);

    nrpc_method_register_builtin(
        localhost,
        NULL,
        "topology:streaming",
        10,
        NRPC_PRIORITY_DEFAULT + 1,
        NRPC_VERSION_DEFAULT,
        FUNCTION_STREAMING_TOPOLOGY_HELP,
        "top",
        HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        function_streaming_topology);

    nrpc_method_register_builtin(
        localhost,
        NULL,
        "netdata-api-calls",
        10,
        NRPC_PRIORITY_DEFAULT + 1,
        NRPC_VERSION_DEFAULT,
        FUNCTION_PROGRESS_HELP,
        "top",
        HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        function_progress);

    nrpc_method_register_builtin(
        localhost,
        NULL,
        FUNCTION_BEARER_GET_TOKEN,
        10,
        NRPC_PRIORITY_DEFAULT + 3,
        NRPC_VERSION_DEFAULT,
        FUNCTION_BEARER_GET_TOKEN_HELP,
        NRPC_TAG_HIDDEN,
        HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        function_bearer_get_token);

    nrpc_method_register_builtin(
        localhost,
        NULL,
        "netdata-metrics-cardinality",
        10,
        NRPC_PRIORITY_DEFAULT + 1,
        NRPC_VERSION_DEFAULT,
        FUNCTION_METRICS_CARDINALITY_HELP,
        "top",
        HTTP_ACCESS_ANONYMOUS_DATA,
        function_metrics_cardinality);
}
