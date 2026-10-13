// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

static int send_host_labels_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    BUFFER *wb = (BUFFER *)data;
    buffer_sprintf(wb, PLUGINSD_KEYWORD_LABEL " \"%s\" = %u \"%s\"\n", name, (unsigned)ls, value);
    return 1;
}

void stream_send_host_labels(RRDHOST *host) {
    if(unlikely(!rrdhost_can_stream_metadata_to_parent(host)
                 || !stream_has_capability(host->sender, STREAM_CAP_HLABELS)))
        return;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);

    // Several threads send the labels of the same host (the ready hook, the receiver forwarding a child's
    // change, reloads). Snapshot and commit under one lock, so that an older snapshot cannot be committed
    // after a newer one and leave the parent with stale labels.
    spinlock_lock(&host->stream.snd.labels_spinlock);

    rrdlabels_walkthrough_read(host->rrdlabels, send_host_labels_callback, wb);
    buffer_sprintf(wb, PLUGINSD_KEYWORD_OVERWRITE " %s\n", "labels");

    sender_commit_clean_buffer(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);

    spinlock_unlock(&host->stream.snd.labels_spinlock);
}
