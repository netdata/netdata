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
        RRDSET_FLAGS st_flags = rrdset_flag_get(st);

        if(st_flags & RRDSET_FLAG_OBSOLETE)
            continue;

        if(!is_host_local && !(st_flags & RRDSET_FLAG_ANOMALY_DETECTION)) {
            REPLAY_WHO rcv = st->stream.rcv.who;
            if (rcv <= 0 || rcv >= REPLAY_WHO_MAX)
                rcv = REPLAY_WHO_UNKNOWN;
            c->rcv[rcv]++;

#ifdef NETDATA_LOG_STREAM_RECEIVER
            if(rcv == REPLAY_WHO_ME || rcv == REPLAY_WHO_THEM) {
                char buf[1024];
                snprintfz(buf, sizeof(buf), "### REPLICATION RECEIVE waits on %s for chart '%s'\n",
                          rcv == REPLAY_WHO_ME ? "me" : "them", rrdset_id(st));
                stream_receiver_log_payload(host->receiver, buf, STREAM_TRAFFIC_TYPE_METADATA, rcv == REPLAY_WHO_THEM);
            }
#endif
        }

        if(is_host_sending && (st_flags & RRDSET_FLAG_UPSTREAM_SEND) && !(st_flags & RRDSET_FLAG_UPSTREAM_IGNORE)) {
            REPLAY_WHO snd = st->stream.snd.who;
            if (snd <= 0 || snd >= REPLAY_WHO_MAX)
                snd = REPLAY_WHO_UNKNOWN;
            c->snd[snd]++;

#ifdef NETDATA_LOG_STREAM_SENDER
            if(snd == REPLAY_WHO_ME || snd == REPLAY_WHO_THEM) {
                char buf[1024];
                snprintfz(buf, sizeof(buf), "### REPLICATION SEND waits on %s for chart '%s'\n",
                          snd == REPLAY_WHO_ME ? "me" : "them", rrdset_id(st));
                stream_receiver_log_payload(host->receiver, buf, STREAM_TRAFFIC_TYPE_METADATA, snd == REPLAY_WHO_THEM);
            }
#endif
        }
    }
    rrdset_foreach_done(st);
}

#endif
