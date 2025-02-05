// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

static BUFFER *preferred_sender_buffer(RRDHOST *host) {
    if(host->stream.snd.commit.receiver_tid == gettid_cached())
        return sender_host_buffer(host);
    else
        return sender_thread_buffer(host->sender);
}

ALWAYS_INLINE RRDSET_STREAM_BUFFER stream_send_metrics_init(RRDSET *st, time_t wall_clock_time) {
    RRDHOST *host = st->rrdhost;

    // fetch the flags we need to check with one atomic operation
    RRDHOST_FLAGS host_flags = __atomic_load_n(&host->flags, __ATOMIC_SEQ_CST);

    // check if we are not connected
    if(unlikely(!(host_flags & RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS))) {

        if(unlikely((host_flags & RRDHOST_FLAG_COLLECTOR_ONLINE) &&
                     !(host_flags & RRDHOST_FLAG_STREAM_SENDER_ADDED)))
            stream_sender_start_host(host);

        if(unlikely(!(host_flags & RRDHOST_FLAG_STREAM_SENDER_LOGGED_STATUS))) {
            rrdhost_flag_set(host, RRDHOST_FLAG_STREAM_SENDER_LOGGED_STATUS);

            // this message is logged in 2 cases:
            // - the parent is connected, but not yet available for streaming data
            // - the parent just disconnected, so local data are not streamed to parent

            nd_log(NDLS_DAEMON, NDLP_INFO,
                   "STREAM SND '%s': streaming is not ready, not sending data to a parent...",
                   rrdhost_hostname(host));
        }

        return (RRDSET_STREAM_BUFFER) { .wb = NULL, };
    }
    else if(unlikely(host_flags & RRDHOST_FLAG_STREAM_SENDER_LOGGED_STATUS)) {
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "STREAM SND '%s': streaming is ready, sending metrics to parent...",
               rrdhost_hostname(host));
        rrdhost_flag_clear(host, RRDHOST_FLAG_STREAM_SENDER_LOGGED_STATUS);
    }

    if(unlikely(host_flags & RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED)) {
        BUFFER *wb = preferred_sender_buffer(host);
        stream_sender_send_global_rrdhost_functions(host, wb, stream_has_capability(host->sender, STREAM_CAP_DYNCFG));
        sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
    }

    bool exposed_upstream = rrdset_check_upstream_exposed(st);
    RRDSET_FLAGS rrdset_flags = rrdset_flag_get(st);
    bool replication_in_progress = !(rrdset_flags & RRDSET_FLAG_SENDER_REPLICATION_FINISHED);

    if(unlikely((exposed_upstream && replication_in_progress) ||
                 !should_send_rrdset_matching(st, rrdset_flags)))
        return (RRDSET_STREAM_BUFFER) { .wb = NULL, };

    if(unlikely(!exposed_upstream)) {
        BUFFER *wb = preferred_sender_buffer(host);
        replication_in_progress = stream_sender_send_rrdset_definition(wb, st);
        sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
    }

    if(unlikely(replication_in_progress))
        return (RRDSET_STREAM_BUFFER) { .wb = NULL, };

    return (RRDSET_STREAM_BUFFER) {
        .capabilities = host->sender->capabilities,
        .v2 = stream_has_capability(host->sender, STREAM_CAP_INTERPOLATED),
        .rrdset_flags = rrdset_flags,
        .wb = preferred_sender_buffer(host),
        .wall_clock_time = wall_clock_time,
    };
}
