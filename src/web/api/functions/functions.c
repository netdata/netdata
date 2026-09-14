// SPDX-License-Identifier: GPL-3.0-or-later

#include "functions.h"

void global_functions_add(void) {
    // we register this only on localhost
    // for the other nodes, the origin server should register it
    nrpc_method_register_builtin(&(struct nrpc_builtin_desc) {
        .owner = rrdhost_nrpc_owner(localhost),
        .name = "netdata-streaming",
        .help = FUNCTION_STREAMING_HELP,
        .tags = "top",
        .timeout_s = 10,
        .priority = NRPC_PRIORITY_DEFAULT + 1,
        .version = NRPC_VERSION_DEFAULT,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        .handler = function_netdata_streaming,
    });

    nrpc_method_register_builtin(&(struct nrpc_builtin_desc) {
        .owner = rrdhost_nrpc_owner(localhost),
        .name = "topology:streaming",
        .help = FUNCTION_STREAMING_TOPOLOGY_HELP,
        .tags = "top",
        .timeout_s = 10,
        .priority = NRPC_PRIORITY_DEFAULT + 1,
        .version = NRPC_VERSION_DEFAULT,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        .handler = function_streaming_topology,
    });

    nrpc_method_register_builtin(&(struct nrpc_builtin_desc) {
        .owner = rrdhost_nrpc_owner(localhost),
        .name = "netdata-api-calls",
        .help = FUNCTION_PROGRESS_HELP,
        .tags = "top",
        .timeout_s = 10,
        .priority = NRPC_PRIORITY_DEFAULT + 1,
        .version = NRPC_VERSION_DEFAULT,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        .handler = function_progress,
    });

    nrpc_method_register_builtin(&(struct nrpc_builtin_desc) {
        .owner = rrdhost_nrpc_owner(localhost),
        .name = FUNCTION_BEARER_GET_TOKEN,
        .help = FUNCTION_BEARER_GET_TOKEN_HELP,
        .tags = NRPC_TAG_HIDDEN,
        .timeout_s = 10,
        .priority = NRPC_PRIORITY_DEFAULT + 3,
        .version = NRPC_VERSION_DEFAULT,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        .handler = function_bearer_get_token,
    });

    nrpc_method_register_builtin(&(struct nrpc_builtin_desc) {
        .owner = rrdhost_nrpc_owner(localhost),
        .name = "netdata-metrics-cardinality",
        .help = FUNCTION_METRICS_CARDINALITY_HELP,
        .tags = "top",
        .timeout_s = 10,
        .priority = NRPC_PRIORITY_DEFAULT + 1,
        .version = NRPC_VERSION_DEFAULT,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .handler = function_metrics_cardinality,
    });
}
