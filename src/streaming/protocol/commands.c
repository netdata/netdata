// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-sender-internals.h"

RRDSET_STREAM_BUFFER rrdset_push_metric_initialize(RRDSET *st, time_t wall_clock_time) {
    RRDHOST *host = st->rrdhost;

    // fetch the flags we need to check with one atomic operation
    RRDHOST_FLAGS host_flags = __atomic_load_n(&host->flags, __ATOMIC_SEQ_CST);

    // check if we are not connected
    if(unlikely(!(host_flags & RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS))) {

        if(unlikely(!(host_flags & (RRDHOST_FLAG_RRDPUSH_SENDER_ADDED | RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED))))
            stream_sender_start_host(host);

        if(unlikely(!(host_flags & RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS))) {
            rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS);
            nd_log_daemon(NDLP_NOTICE, "STREAM %s [send]: not ready - collected metrics are not sent to parent.", rrdhost_hostname(host));
        }

        return (RRDSET_STREAM_BUFFER) { .wb = NULL, };
    }
    else if(unlikely(host_flags & RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS)) {
        nd_log_daemon(NDLP_INFO, "STREAM %s [send]: sending metrics to parent...", rrdhost_hostname(host));
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS);
    }

    if(unlikely(host_flags & RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED)) {
        BUFFER *wb = sender_start(host->sender);
        rrd_global_functions_expose_rrdpush(host, wb, stream_has_capability(host->sender, STREAM_CAP_DYNCFG));
        sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
    }

    bool exposed_upstream = rrdset_check_upstream_exposed(st);
    RRDSET_FLAGS rrdset_flags = rrdset_flag_get(st);
    bool replication_in_progress = !(rrdset_flags & RRDSET_FLAG_SENDER_REPLICATION_FINISHED);

    if(unlikely((exposed_upstream && replication_in_progress) ||
                 !should_send_chart_matching(st, rrdset_flags)))
        return (RRDSET_STREAM_BUFFER) { .wb = NULL, };

    if(unlikely(!exposed_upstream)) {
        BUFFER *wb = sender_start(host->sender);
        replication_in_progress = rrdpush_chart_definition_to_pluginsd(wb, st);
        sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
    }

    if(replication_in_progress)
        return (RRDSET_STREAM_BUFFER) { .wb = NULL, };

    return (RRDSET_STREAM_BUFFER) {
        .capabilities = host->sender->capabilities,
        .v2 = stream_has_capability(host->sender, STREAM_CAP_INTERPOLATED),
        .rrdset_flags = rrdset_flags,
        .wb = sender_start(host->sender),
        .wall_clock_time = wall_clock_time,
    };
}
