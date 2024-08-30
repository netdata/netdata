// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_calls.h"

int api_v2_node_instances(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return api_v2_contexts_internal(
        host, w, url,
        CONTEXTS_V2_NODES | CONTEXTS_V2_NODE_INSTANCES | CONTEXTS_V2_AGENTS |
            CONTEXTS_V2_AGENTS_INFO | CONTEXTS_V2_VERSIONS);
}
