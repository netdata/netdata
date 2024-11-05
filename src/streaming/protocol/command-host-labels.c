// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "plugins.d/pluginsd_internals.h"

static int send_labels_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    BUFFER *wb = (BUFFER *)data;
    buffer_sprintf(wb, "LABEL \"%s\" = %d \"%s\"\n", name, ls, value);
    return 1;
}

void rrdpush_send_host_labels(RRDHOST *host) {
    if(unlikely(!rrdhost_can_send_definitions_to_parent(host)
                 || !stream_has_capability(host->sender, STREAM_CAP_HLABELS)))
        return;

    BUFFER *wb = sender_start(host->sender);

    rrdlabels_walkthrough_read(host->rrdlabels, send_labels_callback, wb);
    buffer_sprintf(wb, "OVERWRITE %s\n", "labels");

    sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);

    sender_thread_buffer_free();
}
