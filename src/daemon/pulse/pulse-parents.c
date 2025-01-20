// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS
#include "pulse.h"

DEFINE_JUDYL_TYPED(PHOST, PULSE_HOST_STATUS);

// --------------------------------------------------------------------------------------------------------------------
// parents

struct {
    SPINLOCK spinlock;
    PHOST_JudyLSet index;

    struct {
        // counters
        size_t stream_info_requests_received;
        size_t stream_requests_received;
        size_t stream_rejections_sent;
        size_t rejections_sent_by_reason[STREAM_HANDSHAKE_NEGATIVE_MAX];
        size_t disconnects_by_reason[STREAM_HANDSHAKE_NEGATIVE_MAX];

        // gauges
        ssize_t nodes_local;
        ssize_t nodes_virtual;
        ssize_t nodes_initializing;
        ssize_t nodes_archived;
        ssize_t nodes_offline;
        ssize_t nodes_waiting;
        ssize_t nodes_replicating;
        ssize_t nodes_running;
    } parent;

    struct {
        // counters
        size_t stream_info_requests_sent;
        size_t connection_attempts;
        size_t stream_info_failed_by_reason[STREAM_HANDSHAKE_NEGATIVE_MAX];
        size_t connections_rejected_by_reason[STREAM_HANDSHAKE_NEGATIVE_MAX];
        size_t disconnects_by_reason[STREAM_HANDSHAKE_NEGATIVE_MAX];

        // gauges
        ssize_t nodes_offline;
        ssize_t nodes_connecting;
        ssize_t nodes_waiting;
        ssize_t nodes_replicating;
        ssize_t nodes_running;
        ssize_t nodes_no_dst;
    } sender;

} p = { 0 };

static PULSE_HOST_STATUS pulse_host_detect_receiver_status(RRDHOST *host) {
    RRDHOST_STATUS status = { 0 };
    rrdhost_status(host, now_realtime_sec(), &status);

    PULSE_HOST_STATUS rc = 0;

    if(status.db.status == RRDHOST_DB_STATUS_INITIALIZING || status.ingest.status == RRDHOST_INGEST_STATUS_INITIALIZING)
        rc = PULSE_HOST_STATUS_INITIALIZING;

    else if(status.ingest.type == RRDHOST_INGEST_TYPE_LOCALHOST)
        rc = PULSE_HOST_STATUS_LOCAL;

    else if(status.ingest.type == RRDHOST_INGEST_TYPE_VIRTUAL)
        rc = PULSE_HOST_STATUS_VIRTUAL;

    else if(status.ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
        rc = PULSE_HOST_STATUS_ARCHIVED;

    else if(status.ingest.status == RRDHOST_INGEST_STATUS_REPLICATING)
        rc = PULSE_HOST_STATUS_RCV_REPLICATING;

    else if(status.ingest.status == RRDHOST_INGEST_STATUS_OFFLINE)
        rc = PULSE_HOST_STATUS_RCV_OFFLINE;

    else if(status.ingest.status == RRDHOST_INGEST_STATUS_ONLINE)
        rc = PULSE_HOST_STATUS_RCV_RUNNING;

    return rc;
}

static void pulse_host_add_sub_status(PULSE_HOST_STATUS status, ssize_t val, STREAM_HANDSHAKE reason) {
    while(status) {
        PULSE_HOST_STATUS s = 1 << (__builtin_ffs(status) - 1);
        status &= ~s;

        bool do_parent_reason = false, do_sender_reason = false;

        switch(s) {
            default:
                break;

            case PULSE_HOST_STATUS_LOCAL:
                __atomic_add_fetch(&p.parent.nodes_local, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_VIRTUAL:
                __atomic_add_fetch(&p.parent.nodes_virtual, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_INITIALIZING:
                __atomic_add_fetch(&p.parent.nodes_initializing, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_ARCHIVED:
                __atomic_add_fetch(&p.parent.nodes_archived, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_RCV_OFFLINE:
                __atomic_add_fetch(&p.parent.nodes_offline, val, __ATOMIC_RELAXED);
                do_parent_reason = true;
                break;

            case PULSE_HOST_STATUS_RCV_WAITING:
                __atomic_add_fetch(&p.parent.nodes_waiting, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_RCV_REPLICATING:
                __atomic_add_fetch(&p.parent.nodes_replicating, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_RCV_RUNNING:
                __atomic_add_fetch(&p.parent.nodes_running, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_SND_OFFLINE:
                __atomic_add_fetch(&p.sender.nodes_offline, val, __ATOMIC_RELAXED);
                do_sender_reason = true;
                break;

            case PULSE_HOST_STATUS_SND_CONNECTING:
                __atomic_add_fetch(&p.sender.nodes_connecting, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_SND_WAITING:
                __atomic_add_fetch(&p.sender.nodes_waiting, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_SND_REPLICATING:
                __atomic_add_fetch(&p.sender.nodes_replicating, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_SND_RUNNING:
                __atomic_add_fetch(&p.sender.nodes_running, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_SND_NO_DST:
                __atomic_add_fetch(&p.sender.nodes_no_dst, val, __ATOMIC_RELAXED);
                break;
        }

        if(do_parent_reason) {
            int r = reason < 0 && reason > -STREAM_HANDSHAKE_NEGATIVE_MAX ? -reason : 0;
            __atomic_add_fetch(&p.parent.disconnects_by_reason[r], 1, __ATOMIC_RELAXED);
        }

        if(do_sender_reason) {
            int r = reason < 0 && reason > -STREAM_HANDSHAKE_NEGATIVE_MAX ? -reason : 0;
            __atomic_add_fetch(&p.sender.disconnects_by_reason[r], 1, __ATOMIC_RELAXED);
        }
    }
}

void pulse_host_status(RRDHOST *host, PULSE_HOST_STATUS status, STREAM_HANDSHAKE reason) {
    PULSE_HOST_STATUS remove = 0;

    if(!status)
        status = pulse_host_detect_receiver_status(host);

    PULSE_HOST_STATUS basic = PULSE_HOST_STATUS_LOCAL|PULSE_HOST_STATUS_VIRTUAL|PULSE_HOST_STATUS_INITIALIZING|PULSE_HOST_STATUS_ARCHIVED|PULSE_HOST_STATUS_DELETED;
    PULSE_HOST_STATUS rcv = PULSE_HOST_STATUS_RCV_OFFLINE|PULSE_HOST_STATUS_RCV_WAITING|PULSE_HOST_STATUS_RCV_REPLICATING|PULSE_HOST_STATUS_RCV_RUNNING;
    PULSE_HOST_STATUS snd = PULSE_HOST_STATUS_SND_OFFLINE|PULSE_HOST_STATUS_SND_CONNECTING|PULSE_HOST_STATUS_SND_WAITING|PULSE_HOST_STATUS_SND_REPLICATING|PULSE_HOST_STATUS_SND_RUNNING|PULSE_HOST_STATUS_SND_NO_DST;

    if(status & basic)
        remove = ~0;
    else if(status & rcv)
        remove = basic | rcv;
    else if(status & snd)
        remove = snd;

    spinlock_lock(&p.spinlock);
    PULSE_HOST_STATUS old = PHOST_GET(&p.index, (uintptr_t)host);
    if(status == PULSE_HOST_STATUS_DELETED)
        PHOST_DEL(&p.index, (uintptr_t)host);
    else
        PHOST_SET(&p.index, (uintptr_t)host, (old & ~remove) | status);
    spinlock_unlock(&p.spinlock);

    remove &= old;
    pulse_host_add_sub_status(remove, -1, 0);

    if(status != PULSE_HOST_STATUS_DELETED)
        pulse_host_add_sub_status(status, 1, reason);
}

void pulse_parent_stream_info_received_request(void) {
    __atomic_add_fetch(&p.parent.stream_info_requests_received, 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_request(void) {
    __atomic_add_fetch(&p.parent.stream_requests_received, 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_rejected(STREAM_HANDSHAKE reason) {
    __atomic_add_fetch(&p.parent.stream_rejections_sent, 1, __ATOMIC_RELAXED);

    int r = reason < 0 && reason > -STREAM_HANDSHAKE_NEGATIVE_MAX ? -reason : 0;
    __atomic_add_fetch(&p.parent.rejections_sent_by_reason[r], 1, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------
// children / senders

void pulse_stream_info_sent_request(void) {
    __atomic_add_fetch(&p.sender.stream_info_requests_sent, 1, __ATOMIC_RELAXED);
}

void pulse_sender_stream_info_failed(const char *destination __maybe_unused, STREAM_HANDSHAKE reason) {
    int r = reason < 0 && reason > -STREAM_HANDSHAKE_NEGATIVE_MAX ? -reason : 0;
    __atomic_add_fetch(&p.sender.stream_info_failed_by_reason[r], 1, __ATOMIC_RELAXED);
}

void pulse_sender_connection_failed(const char *destination __maybe_unused, STREAM_HANDSHAKE reason) {
    int r = reason < 0 && reason > -STREAM_HANDSHAKE_NEGATIVE_MAX ? -reason : 0;
    __atomic_add_fetch(&p.sender.connections_rejected_by_reason[r], 1, __ATOMIC_RELAXED);
}

// --------------------------------------------------------------------------------------------------------------------

void pulse_parents_do(bool extended __maybe_unused) {
    if(netdata_conf_is_parent()) {
        {
            static RRDSET *st_nodes = NULL;
            static RRDDIM *rd_local = NULL;
            static RRDDIM *rd_virtual = NULL;
            static RRDDIM *rd_archived = NULL;
            static RRDDIM *rd_offline = NULL;
            static RRDDIM *rd_waiting = NULL;
            static RRDDIM *rd_replicating = NULL;
            static RRDDIM *rd_running = NULL;

            if (unlikely(!st_nodes)) {
                st_nodes = rrdset_create_localhost(
                    "netdata"
                    , "nodes_inbound"
                    , NULL
                    , "Streaming"
                    , "netdata.nodes_inbound"
                    , "Inbound Nodes"
                    , "nodes"
                    , "netdata"
                    , "pulse"
                    , 130150
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                );

                rd_local = rrddim_add(st_nodes, "local", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_virtual = rrddim_add(st_nodes, "virtual", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_archived = rrddim_add(st_nodes, "archived", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_offline = rrddim_add(st_nodes, "offline", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_waiting = rrddim_add(st_nodes, "waiting", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_replicating = rrddim_add(st_nodes, "replicating", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_running = rrddim_add(st_nodes, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(st_nodes, rd_local, (collected_number)__atomic_load_n(&p.parent.nodes_local, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_virtual, (collected_number)__atomic_load_n(&p.parent.nodes_virtual, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_archived, (collected_number)__atomic_load_n(&p.parent.nodes_archived, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_offline, (collected_number)__atomic_load_n(&p.parent.nodes_offline, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_waiting, (collected_number)__atomic_load_n(&p.parent.nodes_waiting, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_replicating, (collected_number)__atomic_load_n(&p.parent.nodes_replicating, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_running, (collected_number)__atomic_load_n(&p.parent.nodes_running, __ATOMIC_RELAXED));

            rrdset_done(st_nodes);
        }
    }

    if(stream_conf_is_child()) {
        {
            static RRDSET *st_nodes = NULL;
            static RRDDIM *rc_connecting = NULL;
            static RRDDIM *rd_offline = NULL;
            static RRDDIM *rd_waiting = NULL;
            static RRDDIM *rd_replicating = NULL;
            static RRDDIM *rd_running = NULL;
            static RRDDIM *rd_no_dst = NULL;

            if (unlikely(!st_nodes)) {
                st_nodes = rrdset_create_localhost(
                    "netdata"
                    , "nodes_outbound"
                    , NULL
                    , "Streaming"
                    , "netdata.nodes_outbound"
                    , "Outbound Nodes"
                    , "nodes"
                    , "netdata"
                    , "pulse"
                    , 130151
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                );

                rc_connecting = rrddim_add(st_nodes, "connecting", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_offline = rrddim_add(st_nodes, "offline", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_waiting = rrddim_add(st_nodes, "waiting", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_replicating = rrddim_add(st_nodes, "replicating", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_running = rrddim_add(st_nodes, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_no_dst = rrddim_add(st_nodes, "no dst", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(st_nodes, rc_connecting, (collected_number)__atomic_load_n(&p.sender.nodes_connecting, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_offline, (collected_number)__atomic_load_n(&p.sender.nodes_offline, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_waiting, (collected_number)__atomic_load_n(&p.sender.nodes_waiting, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_replicating, (collected_number)__atomic_load_n(&p.sender.nodes_replicating, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_running, (collected_number)__atomic_load_n(&p.sender.nodes_running, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_no_dst, (collected_number)__atomic_load_n(&p.sender.nodes_no_dst, __ATOMIC_RELAXED));

            rrdset_done(st_nodes);
        }
    }
}
