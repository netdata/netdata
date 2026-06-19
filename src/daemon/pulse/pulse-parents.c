// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS
#include "pulse.h"

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
    struct {
        // event counters (event-driven)
        struct by_reason events_by_reason;
        struct by_reason disconnects_by_reason;

        // gauge chart pointers (the per-state counts are computed read-side by the traversal)
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
        } type[2];
    } parent;

    struct {
        // event counters (event-driven)
        struct by_reason stream_info_failed_by_reason;
        struct by_reason events_by_reason;
        struct by_reason disconnects_by_reason;
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
    status &= ~(PULSE_HOST_STATUS_EPHEMERAL | PULSE_HOST_STATUS_PERMANENT);

    while(status) {
        PULSE_HOST_STATUS s = 1 << (__builtin_ffs(status) - 1);
        status &= ~s;

        bool do_parent_reason = false, do_sender_reason = false;

        switch(s) {
            default:
                break;

            // inbound and outbound node gauges are now computed read-side by the pulse traversal;
            // only the event-driven reason/event counters remain here.
            case PULSE_HOST_STATUS_RCV_OFFLINE:
                do_parent_reason = true;
                break;

            case PULSE_HOST_STATUS_RCV_WAITING:
                do_parent_reason = true;
                reason = 0;
                break;

            case PULSE_HOST_STATUS_SND_OFFLINE:
                do_sender_reason = true;
                break;

            case PULSE_HOST_STATUS_SND_CONNECTING:
                __atomic_add_fetch(&p.sender.events_by_reason.counters[STREAM_HANDSHAKE_CONNECT], 1, __ATOMIC_RELAXED);
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

    // maintain the combined resolved state on the host (CAS, lock-free; replaces the global PHOST
    // Judy + spinlock). The pulse traversal reads it to compute the streaming_inbound and
    // streaming_outbound gauges read-side.
    uint32_t cur = __atomic_load_n(&host->stream.pulse_state, __ATOMIC_RELAXED);
    uint32_t next;
    do {
        next = (status == PULSE_HOST_STATUS_DELETED) ? 0u
                                                     : (uint32_t)((cur & ~(uint32_t)remove) | (uint32_t)status);
    } while(!__atomic_compare_exchange_n(&host->stream.pulse_state, &cur, next,
                                         false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    PULSE_HOST_STATUS old = (PULSE_HOST_STATUS)cur; // the state we transitioned from
    if(status == PULSE_HOST_STATUS_DELETED)
        status = 0; // do not add anything, just remove the old flags

    // event-driven reason/event counters only (gauges are computed by the traversal)
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

        for(int i = 0; i < STREAM_HANDSHAKE_NEGATIVE_MAX ;i++) {
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

// --------------------------------------------------------------------------------------------------------------------
// inbound aggregate + per-child charts (parent only)
//
// One read-only pass over the rrdhost dictionary computes BOTH the streaming_inbound aggregate
// (nodes per ephemerality x state) AND each child's per-instance charts on the parent's localhost.
// No global shared counters and no rrd_rdlock: dfe_start_reentrant() refcounts each host during
// iteration, and every per-host value read here is a single-writer relaxed atomic.

typedef enum {
    PULSE_INBOUND_LOCAL = 0,
    PULSE_INBOUND_VIRTUAL,
    PULSE_INBOUND_LOADING,
    PULSE_INBOUND_ARCHIVED,
    PULSE_INBOUND_OFFLINE,
    PULSE_INBOUND_WAITING,
    PULSE_INBOUND_REPLICATION_WAITING,
    PULSE_INBOUND_REPLICATING,
    PULSE_INBOUND_RUNNING,

    PULSE_INBOUND_MAX,
} PULSE_INBOUND_STATE;

static PULSE_INBOUND_STATE pulse_inbound_state(PULSE_HOST_STATUS s) {
    if(s & PULSE_HOST_STATUS_LOCAL)                 return PULSE_INBOUND_LOCAL;
    if(s & PULSE_HOST_STATUS_VIRTUAL)               return PULSE_INBOUND_VIRTUAL;
    if(s & PULSE_HOST_STATUS_LOADING)               return PULSE_INBOUND_LOADING;
    if(s & PULSE_HOST_STATUS_ARCHIVED)              return PULSE_INBOUND_ARCHIVED;
    if(s & PULSE_HOST_STATUS_RCV_OFFLINE)           return PULSE_INBOUND_OFFLINE;
    if(s & PULSE_HOST_STATUS_RCV_WAITING)           return PULSE_INBOUND_WAITING;
    if(s & PULSE_HOST_STATUS_RCV_REPLICATION_WAIT)  return PULSE_INBOUND_REPLICATION_WAITING;
    if(s & PULSE_HOST_STATUS_RCV_REPLICATING)       return PULSE_INBOUND_REPLICATING;
    if(s & PULSE_HOST_STATUS_RCV_RUNNING)           return PULSE_INBOUND_RUNNING;
    return PULSE_INBOUND_MAX;
}

typedef enum {
    PULSE_OUTBOUND_OFFLINE = 0,
    PULSE_OUTBOUND_CONNECTING,
    PULSE_OUTBOUND_PENDING,
    PULSE_OUTBOUND_WAITING,
    PULSE_OUTBOUND_REPLICATING,
    PULSE_OUTBOUND_RUNNING,
    PULSE_OUTBOUND_NO_DST,
    PULSE_OUTBOUND_NO_DST_FAILED,

    PULSE_OUTBOUND_MAX,
} PULSE_OUTBOUND_STATE;

static PULSE_OUTBOUND_STATE pulse_outbound_state(PULSE_HOST_STATUS s) {
    if(s & PULSE_HOST_STATUS_SND_OFFLINE)       return PULSE_OUTBOUND_OFFLINE;
    if(s & PULSE_HOST_STATUS_SND_CONNECTING)    return PULSE_OUTBOUND_CONNECTING;
    if(s & PULSE_HOST_STATUS_SND_PENDING)       return PULSE_OUTBOUND_PENDING;
    if(s & PULSE_HOST_STATUS_SND_WAITING)       return PULSE_OUTBOUND_WAITING;
    if(s & PULSE_HOST_STATUS_SND_REPLICATING)   return PULSE_OUTBOUND_REPLICATING;
    if(s & PULSE_HOST_STATUS_SND_RUNNING)       return PULSE_OUTBOUND_RUNNING;
    if(s & PULSE_HOST_STATUS_SND_NO_DST)        return PULSE_OUTBOUND_NO_DST;
    if(s & PULSE_HOST_STATUS_SND_NO_DST_FAILED) return PULSE_OUTBOUND_NO_DST_FAILED;
    return PULSE_OUTBOUND_MAX;
}

// per-child charts on the parent's localhost: one set per child, found-or-created by id on every
// pass. No registry/cache is kept: a host that leaves the dictionary stops being updated and the
// chart engine obsoletes its charts via the standard not-collected timer (rrdset_free_obsolete_time_s),
// exactly like every other collector. Identity + copied child labels are set once, at chart creation.

static void pulse_child_chart_labels(RRDSET *st, RRDHOST *host) {
    char node_id[UUID_STR_LEN] = "";
    if(!UUIDiszero(host->node_id))
        uuid_unparse_lower(host->node_id.uuid, node_id);

    // copy the child's labels FIRST, then set the authoritative identity labels so a colliding
    // child label cannot overwrite machine_guid/hostname/node_id (rrdlabels_copy/add replace the
    // value for a shared key)
    rrdlabels_copy(st->rrdlabels, host->rrdlabels);
    rrdlabels_add(st->rrdlabels, "machine_guid", host->machine_guid, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "hostname", rrdhost_hostname(host), RRDLABEL_SRC_AUTO);
    if(node_id[0])
        rrdlabels_add(st->rrdlabels, "node_id", node_id, RRDLABEL_SRC_AUTO);
}

static void pulse_child_charts_update(RRDHOST *host, PULSE_INBOUND_STATE state) {
    char id[RRD_ID_LENGTH_MAX + 1];
    const char *guid = host->machine_guid;

    // --- traffic ---
    snprintfz(id, sizeof(id), "streaming.in.traffic.%s", guid);
    RRDSET *st_traffic = rrdset_create_localhost(
        "netdata", id, NULL, "Streaming", "netdata.streaming.in.traffic",
        "Inbound Streaming Traffic", "bytes/s", "netdata", "pulse",
        130160, localhost->rrd_update_every, RRDSET_TYPE_AREA);
    if(unlikely(!rrdlabels_exist(st_traffic->rrdlabels, "machine_guid")))
        pulse_child_chart_labels(st_traffic, host);
    rrddim_set_by_pointer(st_traffic, rrddim_add(st_traffic, "in", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL),
        (collected_number)single_writer_atomic_read(&host->stream.rcv.status.bytes_in));
    rrddim_set_by_pointer(st_traffic, rrddim_add(st_traffic, "out", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL),
        (collected_number)single_writer_atomic_read(&host->stream.rcv.status.bytes_out));
    rrdset_done(st_traffic);

    // --- state (one-hot) ---
    static const char *state_dim[PULSE_INBOUND_MAX] = {
        [PULSE_INBOUND_ARCHIVED]            = "archived",
        [PULSE_INBOUND_OFFLINE]             = "offline",
        [PULSE_INBOUND_WAITING]             = "waiting",
        [PULSE_INBOUND_REPLICATION_WAITING] = "waiting replication",
        [PULSE_INBOUND_REPLICATING]         = "replicating",
        [PULSE_INBOUND_RUNNING]             = "running",
    };
    snprintfz(id, sizeof(id), "streaming.in.state.%s", guid);
    RRDSET *st_state = rrdset_create_localhost(
        "netdata", id, NULL, "Streaming", "netdata.streaming.in.state",
        "Inbound Streaming State", "state", "netdata", "pulse",
        130161, localhost->rrd_update_every, RRDSET_TYPE_LINE);
    if(unlikely(!rrdlabels_exist(st_state->rrdlabels, "machine_guid")))
        pulse_child_chart_labels(st_state, host);
    for(size_t i = 0; i < PULSE_INBOUND_MAX ; i++) {
        if(!state_dim[i]) continue;
        rrddim_set_by_pointer(st_state, rrddim_add(st_state, state_dim[i], NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE),
            (collected_number)(state == i ? 1 : 0));
    }
    rrdset_done(st_state);

    // --- reconnects ---
    snprintfz(id, sizeof(id), "streaming.in.reconnects.%s", guid);
    RRDSET *st_reconnects = rrdset_create_localhost(
        "netdata", id, NULL, "Streaming", "netdata.streaming.in.reconnects",
        "Inbound Streaming Reconnects", "connects/s", "netdata", "pulse",
        130162, localhost->rrd_update_every, RRDSET_TYPE_LINE);
    if(unlikely(!rrdlabels_exist(st_reconnects->rrdlabels, "machine_guid")))
        pulse_child_chart_labels(st_reconnects, host);
    rrddim_set_by_pointer(st_reconnects, rrddim_add(st_reconnects, "connections", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL),
        (collected_number)__atomic_load_n(&host->stream.rcv.status.connections, __ATOMIC_RELAXED));
    rrdset_done(st_reconnects);
}

// traverse all hosts once: tally BOTH the inbound and outbound aggregates from each host's combined
// pulse_state, and refresh the per-child charts. A host may carry both an inbound (receiver) and an
// outbound (sender) state simultaneously, so both are tallied independently.
static void pulse_parents_traverse(ssize_t inbound[2][PULSE_INBOUND_MAX], ssize_t outbound[PULSE_OUTBOUND_MAX]) {
    // per-child charts only when streaming ingest is actually configured
    bool do_children = stream_conf_is_parent(false);

    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host) {
        PULSE_HOST_STATUS s = __atomic_load_n(&host->stream.pulse_state, __ATOMIC_RELAXED);
        if(!s)
            continue; // not classified yet

        PULSE_INBOUND_STATE in = pulse_inbound_state(s);
        if(in < PULSE_INBOUND_MAX) {
            size_t type = (s & PULSE_HOST_STATUS_EPHEMERAL) ? 1 : 0;
            inbound[type][in]++;

            if(do_children && !rrdhost_is_local(host))
                pulse_child_charts_update(host, in);
        }

        PULSE_OUTBOUND_STATE out = pulse_outbound_state(s);
        if(out < PULSE_OUTBOUND_MAX)
            outbound[out]++;
    }
    dfe_done(host);
}

void pulse_parents_do(bool extended) {
    bool is_parent = netdata_conf_is_parent();
    bool is_child = stream_conf_is_child();

    // one read-only pass over the host dictionary: tally both streaming aggregates and refresh the
    // per-child charts (no global counters, no rrd_rdlock)
    ssize_t inbound[2][PULSE_INBOUND_MAX] = { 0 };
    ssize_t outbound[PULSE_OUTBOUND_MAX] = { 0 };
    if(is_parent || is_child)
        pulse_parents_traverse(inbound, outbound);

    if(is_parent) {
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

            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_local, (collected_number)inbound[idx][PULSE_INBOUND_LOCAL]);
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_virtual, (collected_number)inbound[idx][PULSE_INBOUND_VIRTUAL]);
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_loading, (collected_number)inbound[idx][PULSE_INBOUND_LOADING]);
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_archived, (collected_number)inbound[idx][PULSE_INBOUND_ARCHIVED]);
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_offline, (collected_number)inbound[idx][PULSE_INBOUND_OFFLINE]);
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_waiting, (collected_number)inbound[idx][PULSE_INBOUND_WAITING]);
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_replication_waiting, (collected_number)inbound[idx][PULSE_INBOUND_REPLICATION_WAITING]);
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_replicating, (collected_number)inbound[idx][PULSE_INBOUND_REPLICATING]);
            rrddim_set_by_pointer(p.parent.type[idx].st_nodes, p.parent.type[idx].rd_running, (collected_number)inbound[idx][PULSE_INBOUND_RUNNING]);

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

    if(is_child) {
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

            rrddim_set_by_pointer(st_nodes, rd_connecting, (collected_number)outbound[PULSE_OUTBOUND_CONNECTING]);
            rrddim_set_by_pointer(st_nodes, rd_pending, (collected_number)outbound[PULSE_OUTBOUND_PENDING]);
            rrddim_set_by_pointer(st_nodes, rd_offline, (collected_number)outbound[PULSE_OUTBOUND_OFFLINE]);
            rrddim_set_by_pointer(st_nodes, rd_waiting, (collected_number)outbound[PULSE_OUTBOUND_WAITING]);
            rrddim_set_by_pointer(st_nodes, rd_replicating, (collected_number)outbound[PULSE_OUTBOUND_REPLICATING]);
            rrddim_set_by_pointer(st_nodes, rd_running, (collected_number)outbound[PULSE_OUTBOUND_RUNNING]);
            rrddim_set_by_pointer(st_nodes, rd_no_dst, (collected_number)outbound[PULSE_OUTBOUND_NO_DST]);
            rrddim_set_by_pointer(st_nodes, rd_no_dst_failed, (collected_number)outbound[PULSE_OUTBOUND_NO_DST_FAILED]);

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
