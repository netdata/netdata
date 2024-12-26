// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-replication-tracking.h"
#include "database/rrd.h"
#include "stream-receiver-internals.h"

#ifdef REPLICATION_TRACKING

void replication_tracking_counters(struct rrdhost *host, struct replay_who_counters *c) {
    if(!rrdhost_flag_check(host, RRDHOST_FLAG_COLLECTOR_ONLINE))
        return;

    bool is_host_local = host == localhost || rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST);
    bool is_host_sending = rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS);

    RRDSET *st;
    rrdset_foreach_read(st, host) {
        RRDSET_FLAGS st_flags = __atomic_load_n(&st->flags, __ATOMIC_RELAXED);

        if(st_flags & RRDSET_FLAG_OBSOLETE)
            continue;

        if(!is_host_local && !(st_flags & RRDSET_FLAG_ANOMALY_DETECTION)) {
            REPLAY_WHO rcv = st->stream.rcv.who;
            if (rcv <= 0 || rcv >= REPLAY_WHO_MAX) {
                rcv = REPLAY_WHO_UNKNOWN;
#ifdef NETDATA_LOG_STREAM_RECEIVER
                char buf[1024];
                snprintfz(buf, sizeof(buf), "### REPLICATION WHO UNKNOWN ON CHART '%s'\n", rrdset_id(st));
                stream_receiver_log_payload(host->receiver, buf, STREAM_TRAFFIC_TYPE_METADATA, true);
#endif
            }
            c->rcv[rcv]++;
        }

        if(is_host_sending && (st_flags & RRDSET_FLAG_UPSTREAM_SEND) && !(st_flags & RRDSET_FLAG_UPSTREAM_IGNORE)) {
            REPLAY_WHO snd = st->stream.snd.who;
            if (snd <= 0 || snd >= REPLAY_WHO_MAX)
                snd = REPLAY_WHO_UNKNOWN;
            c->snd[snd]++;
        }
    }
    rrdset_foreach_done(st);
}

#endif
