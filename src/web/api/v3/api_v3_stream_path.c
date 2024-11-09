// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v3_calls.h"

int api_v3_stream_path(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return api_v3_contexts_internal(host, w, url, CONTEXTS_V2_NODES | CONTEXTS_V2_NODES_STREAM_PATH);
}
