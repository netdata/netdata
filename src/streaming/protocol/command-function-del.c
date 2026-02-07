// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

void stream_send_function_del(RRDHOST *host, const char *function_name) {
    if(!stream_sender_has_capabilities(host, STREAM_CAP_FUNCTION_DEL))
        return;

    if(unlikely(!rrdhost_can_stream_metadata_to_parent(host)))
        return;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb, PLUGINSD_KEYWORD_FUNCTION_DEL " GLOBAL \"%s\"\n", function_name);
    sender_commit_clean_buffer(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
}
