// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS
#include "pulse.h"

DEFINE_JUDYL_TYPED(PHOST, PULSE_HOST_STATUS);

// --------------------------------------------------------------------------------------------------------------------
// parents

struct by_reason {
    size_t counters[STREAM_HANDSHAKE_NEGATIVE_MAX + 3];
    RRDSET *st;
    RRDDIM *rd[STREAM_HANDSHAKE_NEGATIVE_MAX + 3];
};

#define STREAM_HANDSHAKE_STREAM_INFO (STREAM_HANDSHAKE_NEGATIVE_MAX)
#define STREAM_HANDSHAKE_CONNECT (STREAM_HANDSHAKE_NEGATIVE_MAX + 1)
#define STREAM_HANDSHAKE_OTHER (STREAM_HANDSHAKE_NEGATIVE_MAX + 2)

struct {
    SPINLOCK spinlock;
    PHOST_JudyLSet index;

    struct {
        // counters
        struct by_reason events_by_reason;
        struct by_reason disconnects_by_reason;

        // gauges
        struct {
            RRDSET *st_nodes;
            RRDDIM *rd_loading;
            RRDDIM *rd_local;
            RRDDIM *rd_virtual;
            RRDDIM *rd_archived;
            RRDDIM *rd_offline;
            RRDDIM *rd_waiting;
            RRDDIM *rd_replication_waiting;
            RRDDIM *rd_replicating;
            RRDDIM *rd_running;

            ssize_t nodes_local;
            ssize_t nodes_virtual;
            ssize_t nodes_loading;
            ssize_t nodes_archived;
            ssize_t nodes_offline;
            ssize_t nodes_waiting;
            ssize_t nodes_replicating;
            ssize_t nodes_replication_waiting;
            ssize_t nodes_running;
        } type[2];
    } parent;

    struct {
        // counters
        struct by_reason stream_info_failed_by_reason;
        struct by_reason events_by_reason;
        struct by_reason disconnects_by_reason;

        // gauges
        ssize_t nodes_offline;
        ssize_t nodes_connecting;
        ssize_t nodes_pending;
        ssize_t nodes_waiting;
        ssize_t nodes_replicating;
        ssize_t nodes_running;
        ssize_t nodes_no_dst;
        ssize_t nodes_no_dst_failed;
    } sender;

} p = { 0 };

static PULSE_HOST_STATUS pulse_host_detect_receiver_status(RRDHOST *host) {
    RRDHOST_STATUS status = { 0 };
    rrdhost_status(host, now_realtime_sec(), &status, RRDHOST_STATUS_BASIC);

    PULSE_HOST_STATUS rc = 0;

    if(status.db.status == RRDHOST_DB_STATUS_INITIALIZING || status.ingest.status == RRDHOST_INGEST_STATUS_INITIALIZING)
        rc = PULSE_HOST_STATUS_LOADING;

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

static void update_reason(struct by_reason *b, STREAM_HANDSHAKE reason) {
    int r = reason;

    if(r >= 0)
        r = 0;
    else if(r > -STREAM_HANDSHAKE_NEGATIVE_MAX)
        r = -reason;
    else
        r = STREAM_HANDSHAKE_NEGATIVE_MAX;

    __atomic_add_fetch(&b->counters[r], 1, __ATOMIC_RELAXED);
}

static void pulse_host_add_sub_status(PULSE_HOST_STATUS status, ssize_t val, STREAM_HANDSHAKE reason) {
    size_t idx = status & PULSE_HOST_STATUS_EPHEMERAL ? 1 : 0;
    status &= ~(PULSE_HOST_STATUS_EPHEMERAL | PULSE_HOST_STATUS_PERMANENT);

    while(status) {
        PULSE_HOST_STATUS s = 1 << (__builtin_ffs(status) - 1);
        status &= ~s;

        bool do_parent_reason = false, do_sender_reason = false;

        switch(s) {
            default:
                break;

            case PULSE_HOST_STATUS_LOCAL:
                __atomic_add_fetch(&p.parent.type[idx].nodes_local, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_VIRTUAL:
                __atomic_add_fetch(&p.parent.type[idx].nodes_virtual, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_LOADING:
                __atomic_add_fetch(&p.parent.type[idx].nodes_loading, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_ARCHIVED:
                __atomic_add_fetch(&p.parent.type[idx].nodes_archived, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_RCV_OFFLINE:
                __atomic_add_fetch(&p.parent.type[idx].nodes_offline, val, __ATOMIC_RELAXED);
                do_parent_reason = true;
                break;

            case PULSE_HOST_STATUS_RCV_WAITING:
                __atomic_add_fetch(&p.parent.type[idx].nodes_waiting, val, __ATOMIC_RELAXED);
                do_parent_reason = true;
                reason = 0;
                break;

            case PULSE_HOST_STATUS_RCV_REPLICATION_WAIT:
                __atomic_add_fetch(&p.parent.type[idx].nodes_replication_waiting, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_RCV_REPLICATING:
                __atomic_add_fetch(&p.parent.type[idx].nodes_replicating, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_RCV_RUNNING:
                __atomic_add_fetch(&p.parent.type[idx].nodes_running, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_SND_OFFLINE:
                __atomic_add_fetch(&p.sender.nodes_offline, val, __ATOMIC_RELAXED);
                do_sender_reason = true;
                break;

            case PULSE_HOST_STATUS_SND_PENDING:
                __atomic_add_fetch(&p.sender.nodes_pending, val, __ATOMIC_RELAXED);
                break;

            case PULSE_HOST_STATUS_SND_CONNECTING:
                __atomic_add_fetch(&p.sender.nodes_connecting, val, __ATOMIC_RELAXED);
                __atomic_add_fetch(&p.sender.events_by_reason.counters[STREAM_HANDSHAKE_CONNECT], 1, __ATOMIC_RELAXED);
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

            case PULSE_HOST_STATUS_SND_NO_DST_FAILED:
                __atomic_add_fetch(&p.sender.nodes_no_dst_failed, val, __ATOMIC_RELAXED);
                break;
        }

        if(do_parent_reason && val > 0)
            update_reason(&p.parent.disconnects_by_reason, reason);

        if(do_sender_reason && val > 0)
            update_reason(&p.sender.disconnects_by_reason, reason);
    }
}

void pulse_host_status(RRDHOST *host, PULSE_HOST_STATUS status, STREAM_HANDSHAKE reason) {
    PULSE_HOST_STATUS remove = 0;

    if(!status)
        status = pulse_host_detect_receiver_status(host);

    static const PULSE_HOST_STATUS ephemerality =
        PULSE_HOST_STATUS_EPHEMERAL | PULSE_HOST_STATUS_PERMANENT;

    static const PULSE_HOST_STATUS basic =
        PULSE_HOST_STATUS_LOCAL | PULSE_HOST_STATUS_VIRTUAL | PULSE_HOST_STATUS_LOADING |
        PULSE_HOST_STATUS_ARCHIVED | PULSE_HOST_STATUS_DELETED;

    static const PULSE_HOST_STATUS receiver =
        PULSE_HOST_STATUS_RCV_OFFLINE | PULSE_HOST_STATUS_RCV_WAITING | PULSE_HOST_STATUS_RCV_REPLICATING |
        PULSE_HOST_STATUS_RCV_REPLICATION_WAIT | PULSE_HOST_STATUS_RCV_RUNNING;

    static const PULSE_HOST_STATUS sender =
        PULSE_HOST_STATUS_SND_OFFLINE | PULSE_HOST_STATUS_SND_PENDING | PULSE_HOST_STATUS_SND_CONNECTING |
        PULSE_HOST_STATUS_SND_WAITING | PULSE_HOST_STATUS_SND_REPLICATING | PULSE_HOST_STATUS_SND_RUNNING |
        PULSE_HOST_STATUS_SND_NO_DST | PULSE_HOST_STATUS_SND_NO_DST_FAILED;

    if((status & (basic | receiver)) && !(status & ephemerality))
        status |= rrdhost_option_check(host, RRDHOST_OPTION_EPHEMERAL_HOST) ?
                      PULSE_HOST_STATUS_EPHEMERAL : PULSE_HOST_STATUS_PERMANENT;

    if(status & basic)
        remove = basic | receiver | ephemerality | sender;
    else if(status & receiver)
        remove = basic | receiver | ephemerality;
    else if(status & sender)
        remove = sender;

    spinlock_lock(&p.spinlock);
    PULSE_HOST_STATUS old = PHOST_GET(&p.index, (uintptr_t)host);
    if(status == PULSE_HOST_STATUS_DELETED) {
        PHOST_DEL(&p.index, (uintptr_t)host);
        status = 0; // do not add anything, just remove the old flags
    }
    else
        PHOST_SET(&p.index, (uintptr_t)host, (old & ~remove) | status);
    spinlock_unlock(&p.spinlock);

    remove &= old;

    pulse_host_add_sub_status(remove, -1, 0);
    pulse_host_add_sub_status(status, 1, reason);
}

void pulse_parent_stream_info_received_request(void) {
    __atomic_add_fetch(&p.parent.events_by_reason.counters[STREAM_HANDSHAKE_STREAM_INFO], 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_request(void) {
    __atomic_add_fetch(&p.parent.events_by_reason.counters[STREAM_HANDSHAKE_CONNECT], 1, __ATOMIC_RELAXED);
}

void pulse_parent_receiver_rejected(STREAM_HANDSHAKE reason) {
    update_reason(&p.parent.events_by_reason, reason);
}

// --------------------------------------------------------------------------------------------------------------------
// children / senders

void pulse_stream_info_sent_request(void) {
    __atomic_add_fetch(&p.sender.events_by_reason.counters[STREAM_HANDSHAKE_STREAM_INFO], 1, __ATOMIC_RELAXED);
}

void pulse_sender_stream_info_failed(const char *destination __maybe_unused, STREAM_HANDSHAKE reason) {
    update_reason(&p.sender.stream_info_failed_by_reason, reason);
}

void pulse_sender_connection_failed(const char *destination __maybe_unused, STREAM_HANDSHAKE reason) {
    update_reason(&p.sender.events_by_reason, reason);
}

// --------------------------------------------------------------------------------------------------------------------

static void chart_by_reason(struct by_reason *b, const char *id, const char *context, const char *title, const char *label, int priority) {
    if(!b->st) {
        b->st = rrdset_create_localhost(
            "netdata"
            , id
            , NULL
            , "Streaming"
            , context
            , title
            , "events/s"
            , "netdata"
            , "pulse"
            , priority
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE
        );

        for(size_t i = 0; i < STREAM_HANDSHAKE_NEGATIVE_MAX ;i++) {
            char buf[1024];
            if(!i)
                strncpyz(buf, "connected", sizeof(buf) - 1);
            else
                strncpyz(buf, stream_handshake_error_to_string(-i), sizeof(buf) - 1);
            for(int c = 0; buf[c] ;c++)
                buf[c] = (char)tolower(buf[c]);

            b->rd[i] = rrddim_add(b->st, buf, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrdlabels_add(b->st->rrdlabels, "type", label, RRDLABEL_SRC_AUTO);

        b->rd[STREAM_HANDSHAKE_STREAM_INFO] = rrddim_add(b->st, "info", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        b->rd[STREAM_HANDSHAKE_CONNECT] = rrddim_add(b->st, "connect", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        b->rd[STREAM_HANDSHAKE_OTHER] = rrddim_add(b->st, "other", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    for(size_t i = 0; i <= STREAM_HANDSHAKE_OTHER ;i++)
        rrddim_set_by_pointer(b->st, b->rd[i], (collected_number)__atomic_load_n(&b->counters[i], __ATOMIC_RELAXED));

    rrdset_done(b->st);
}

void pulse_parents_do(bool extended) {
    if(netdata_conf_is_parent()) {
        for(size_t idx = 0; idx < _countof(p.parent.type) ; idx++) {
            if (unlikely(!p.parent.type[idx].st_nodes)) {
                const char *type;
                const char *id;
                if(idx == 0) {
                    type = "permanent";
                    id = "netdata.streaming_inbound_permanent";
                }
                else {
                    type = "ephemeral";
                    id = "netdata.streaming_inbound_ephemeral";
                }

                p.parent.type[idx].st_nodes = rrdset_create_localhost(
                    "netdata"
                    , id
                    , NULL
                    , "Streaming"
                    , "netdata.streaming_inbound"
                    , "Inbound Nodes"
                    , "nodes"
                    , "netdata"
                    , "pulse"
                    , 130150
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                );

                rrdlabels_add(p.parent.type[idx].st_nodes->rrdlabels, "type", type, RRDLABEL_SRC_AUTO);

                p.parent.type[idx].rd_local = rrddim_add(p.parent.type[idx].st_nodes, "local", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p.parent.type[idx].rd_virtual = rrddim_add(p.parent.type[idx].st_nodes, "virtual", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p.parent.type[idx].rd_loading = rrddim_add(p.parent.type[idx].st_nodes, "loading", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p.parent.type[idx].rd_archived = rrddim_add(p.parent.type[idx].st_nodes, "stale archived", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p.parent.type[idx].rd_offline = rrddim_add(p.parent.type[idx].st_nodes, "stale disconnected", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p.parent.type[idx].rd_waiting = rrddim_add(p.parent.type[idx].st_nodes, "waiting", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p.parent.type[idx].rd_replication_waiting = rrddim_add(p.parent.type[idx].st_nodes, "waiting replication", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p.parent.type[idx].rd_replicating = rrddim_add(p.parent.type[idx].st_nodes, "replicating", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                p.parent.type[idx].rd_running = rrddim_add(p.parent.type[idx].st_nodes, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_local, (collected_number)__atomic_load_n(&p.parent.type[idx].nodes_local, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_virtual, (collected_number)__atomic_load_n(&p.parent.type[idx].nodes_virtual, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes,p.parent.type[idx].rd_loading, (collected_number)__atomic_load_n(&p.parent.type[idx].nodes_loading, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_archived, (collected_number)__atomic_load_n(&p.parent.type[idx].nodes_archived, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_offline, (collected_number)__atomic_load_n(&p.parent.type[idx].nodes_offline, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_waiting, (collected_number)__atomic_load_n(&p.parent.type[idx].nodes_waiting, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_replication_waiting, (collected_number)__atomic_load_n(&p.parent.type[idx].nodes_replication_waiting, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_replicating, (collected_number)__atomic_load_n(&p.parent.type[idx].nodes_replicating, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_running, (collected_number)__atomic_load_n(&p.parent.type[idx].nodes_running, __ATOMIC_RELAXED));

            rrdset_done(p.parent.type[idx].st_nodes);
        }

        if(extended) {
            chart_by_reason(
                &p.parent.events_by_reason,
                "streaming_rejections_inbound",
                "netdata.streaming_events_inbound",
                "Inbound Streaming Events",
                "rejections",
                130151);
            chart_by_reason(
                &p.parent.disconnects_by_reason,
                "streaming_disconnects_inbound",
                "netdata.streaming_events_inbound",
                "Inbound Streaming Events",
                "disconnects",
                130151);
        }
    }

    if(stream_conf_is_child()) {
        {
            static RRDSET *st_nodes = NULL;
            static RRDDIM *rd_pending = NULL;
            static RRDDIM *rd_connecting = NULL;
            static RRDDIM *rd_offline = NULL;
            static RRDDIM *rd_waiting = NULL;
            static RRDDIM *rd_replicating = NULL;
            static RRDDIM *rd_running = NULL;
            static RRDDIM *rd_no_dst = NULL;
            static RRDDIM *rd_no_dst_failed = NULL;

            if (unlikely(!st_nodes)) {
                st_nodes = rrdset_create_localhost(
                    "netdata"
                    , "streaming_outbound"
                    , NULL
                    , "Streaming"
                    , "netdata.streaming_outbound"
                    , "Outbound Nodes"
                    , "nodes"
                    , "netdata"
                    , "pulse"
                    , 130153
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
                );

                rd_connecting = rrddim_add(st_nodes, "connecting", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_pending = rrddim_add(st_nodes, "pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_offline = rrddim_add(st_nodes, "offline", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_waiting = rrddim_add(st_nodes, "waiting", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_replicating = rrddim_add(st_nodes, "replicating", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_running = rrddim_add(st_nodes, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_no_dst = rrddim_add(st_nodes, "no dst", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_no_dst_failed = rrddim_add(st_nodes, "failed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(st_nodes, rd_connecting, (collected_number)__atomic_load_n(&p.sender.nodes_connecting, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_pending, (collected_number)__atomic_load_n(&p.sender.nodes_pending, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_offline, (collected_number)__atomic_load_n(&p.sender.nodes_offline, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_waiting, (collected_number)__atomic_load_n(&p.sender.nodes_waiting, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_replicating, (collected_number)__atomic_load_n(&p.sender.nodes_replicating, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_running, (collected_number)__atomic_load_n(&p.sender.nodes_running, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_no_dst, (collected_number)__atomic_load_n(&p.sender.nodes_no_dst, __ATOMIC_RELAXED));
            rrddim_set_by_pointer(st_nodes, rd_no_dst_failed, (collected_number)__atomic_load_n(&p.sender.nodes_no_dst_failed, __ATOMIC_RELAXED));

            rrdset_done(st_nodes);
        }

        if(extended) {
            chart_by_reason(
                &p.sender.stream_info_failed_by_reason,
                "streaming_info_failed_outbound",
                "netdata.streaming_events_outbound",
                "Outbound Streaming Events",
                "stream-info",
                130154);
            chart_by_reason(
                &p.sender.events_by_reason,
                "streaming_rejections_outbound",
                "netdata.streaming_events_outbound",
                "Outbound Streaming Events",
                "rejections",
                130154);
            chart_by_reason(
                &p.sender.disconnects_by_reason,
                "streaming_disconnects_outbound",
                "netdata.streaming_events_outbound",
                "Outbound Streaming Events",
                "disconnects",
                130154);
        }
    }
}
