// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-status.h"
#include "streaming/stream-receiver-internals.h"
#include "streaming/stream-sender-internals.h"

ENUM_STR_MAP_DEFINE(RRDHOST_DB_STATUS) = {
    { RRDHOST_DB_STATUS_INITIALIZING, "initializing" },
    { RRDHOST_DB_STATUS_QUERYABLE, "online" },
    { 0, NULL } // Sentinel
};

ENUM_STR_MAP_DEFINE(RRDHOST_DB_LIVENESS) = {
    { RRDHOST_DB_LIVENESS_STALE, "stale" },
    { RRDHOST_DB_LIVENESS_LIVE, "live" },
    { 0, NULL } // Sentinel
};

ENUM_STR_MAP_DEFINE(RRDHOST_INGEST_STATUS) = {
    { RRDHOST_INGEST_STATUS_ARCHIVED, "archived" },
    { RRDHOST_INGEST_STATUS_INITIALIZING, "initializing" },
    { RRDHOST_INGEST_STATUS_REPLICATING, "replicating" },
    { RRDHOST_INGEST_STATUS_ONLINE, "online" },
    { RRDHOST_INGEST_STATUS_OFFLINE, "offline" },
    { 0, NULL } // Sentinel
};

ENUM_STR_MAP_DEFINE(RRDHOST_INGEST_TYPE) = {
    { RRDHOST_INGEST_TYPE_LOCALHOST, "localhost" },
    { RRDHOST_INGEST_TYPE_VIRTUAL, "virtual" },
    { RRDHOST_INGEST_TYPE_CHILD, "child" },
    { RRDHOST_INGEST_TYPE_ARCHIVED, "archived" },
    { 0, NULL } // Sentinel
};

ENUM_STR_MAP_DEFINE(RRDHOST_STREAMING_STATUS) = {
    { RRDHOST_STREAM_STATUS_DISABLED, "disabled" },
    { RRDHOST_STREAM_STATUS_REPLICATING, "replicating" },
    { RRDHOST_STREAM_STATUS_ONLINE, "online" },
    { RRDHOST_STREAM_STATUS_OFFLINE, "offline" },
    { 0, NULL } // Sentinel
};

ENUM_STR_MAP_DEFINE(RRDHOST_ML_STATUS) = {
    { RRDHOST_ML_STATUS_DISABLED, "disabled" },
    { RRDHOST_ML_STATUS_OFFLINE, "offline" },
    { RRDHOST_ML_STATUS_RUNNING, "online" },
    { 0, NULL } // Sentinel
};

ENUM_STR_MAP_DEFINE(RRDHOST_ML_TYPE) = {
    { RRDHOST_ML_TYPE_DISABLED, "disabled" },
    { RRDHOST_ML_TYPE_SELF, "self" },
    { RRDHOST_ML_TYPE_RECEIVED, "received" },
    { 0, NULL } // Sentinel
};

ENUM_STR_MAP_DEFINE(RRDHOST_HEALTH_STATUS) = {
    { RRDHOST_HEALTH_STATUS_DISABLED, "disabled" },
    { RRDHOST_HEALTH_STATUS_INITIALIZING, "initializing" },
    { RRDHOST_HEALTH_STATUS_RUNNING, "online" },
    { 0, NULL } // Sentinel
};

ENUM_STR_MAP_DEFINE(RRDHOST_DYNCFG_STATUS) = {
    { RRDHOST_DYNCFG_STATUS_UNAVAILABLE, "unavailable" },
    { RRDHOST_DYNCFG_STATUS_AVAILABLE, "online" },
    { 0, NULL } // Sentinel
};

ENUM_STR_DEFINE_FUNCTIONS(RRDHOST_DB_STATUS, RRDHOST_DB_STATUS_INITIALIZING, "initializing");
ENUM_STR_DEFINE_FUNCTIONS(RRDHOST_DB_LIVENESS, RRDHOST_DB_LIVENESS_STALE, "stale");
ENUM_STR_DEFINE_FUNCTIONS(RRDHOST_INGEST_STATUS, RRDHOST_INGEST_STATUS_OFFLINE, "offline");
ENUM_STR_DEFINE_FUNCTIONS(RRDHOST_INGEST_TYPE, RRDHOST_INGEST_TYPE_ARCHIVED, "archived");
ENUM_STR_DEFINE_FUNCTIONS(RRDHOST_STREAMING_STATUS, RRDHOST_STREAM_STATUS_OFFLINE, "offline");
ENUM_STR_DEFINE_FUNCTIONS(RRDHOST_ML_STATUS, RRDHOST_ML_STATUS_DISABLED, "disabled");
ENUM_STR_DEFINE_FUNCTIONS(RRDHOST_ML_TYPE, RRDHOST_ML_TYPE_DISABLED, "disabled");
ENUM_STR_DEFINE_FUNCTIONS(RRDHOST_HEALTH_STATUS, RRDHOST_HEALTH_STATUS_DISABLED, "disabled");
ENUM_STR_DEFINE_FUNCTIONS(RRDHOST_DYNCFG_STATUS, RRDHOST_DYNCFG_STATUS_UNAVAILABLE, "unavailable");

static NETDATA_DOUBLE rrdhost_sender_replication_completion_unsafe(RRDHOST *host, time_t now, size_t *instances) {
    size_t charts = rrdhost_sender_replicating_charts(host);
    NETDATA_DOUBLE completion;
    if(!charts || !host->sender || !host->sender->replication.oldest_request_after_t)
        completion = 100.0;
    else if(!host->sender->replication.latest_completed_before_t || host->sender->replication.latest_completed_before_t < host->sender->replication.oldest_request_after_t)
        completion = 0.0;
    else {
        time_t total = now - host->sender->replication.oldest_request_after_t;
        time_t current = host->sender->replication.latest_completed_before_t - host->sender->replication.oldest_request_after_t;
        completion = (NETDATA_DOUBLE) current * 100.0 / (NETDATA_DOUBLE) total;
    }

    *instances = charts;

    return completion;
}

RRDHOST_INGEST_STATUS rrdhost_ingestion_status(RRDHOST *host) {
    return rrdhost_get_ingest_status(host, now_realtime_sec());
}

int16_t rrdhost_ingestion_hops(RRDHOST *host) {
    if(host == localhost) return 0;
    if(rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST) || !host->system_info) return 1;
    return rrdhost_system_info_hops(host->system_info);
}

static inline RRDHOST_DB_STATUS rrdhost_status_db(RRDHOST *host, time_t now, RRDHOST_STATUS *s, RRDHOST_FLAGS flags, bool online) {
    RRDHOST_DB_STATUS status;

    uint32_t metrics = UINT32_MAX;
    uint32_t instances = UINT32_MAX;
    uint32_t contexts = UINT32_MAX;

    time_t first_time_s = 0, last_time_s = 0;
    rrdhost_retention(host, now, online, &first_time_s, &last_time_s);

    if (!first_time_s ||
        !last_time_s ||
        (flags & RRDHOST_FLAG_PENDING_CONTEXT_LOAD) ||
        !(metrics = __atomic_load_n(&host->rrdctx.metrics_count, __ATOMIC_RELAXED)) ||
        !(instances = __atomic_load_n(&host->rrdctx.instances_count, __ATOMIC_RELAXED)) ||
        !(contexts = __atomic_load_n(&host->rrdctx.contexts_count, __ATOMIC_RELAXED)))
        status = RRDHOST_DB_STATUS_INITIALIZING;
    else
        status = RRDHOST_DB_STATUS_QUERYABLE;


    if(s) {
        s->db.status = status;

        s->db.first_time_s = first_time_s;
        s->db.last_time_s = last_time_s;
        s->db.status = status;
        s->db.mode = host->rrd_memory_mode;

        s->db.metrics = (metrics == UINT32_MAX) ? __atomic_load_n(&host->rrdctx.metrics_count, __ATOMIC_RELAXED) : metrics;
        s->db.instances = (instances == UINT32_MAX) ? __atomic_load_n(&host->rrdctx.instances_count, __ATOMIC_RELAXED) : instances;
        s->db.contexts = (contexts == UINT32_MAX) ? __atomic_load_n(&host->rrdctx.contexts_count, __ATOMIC_RELAXED) : contexts;
    }

    return status;
}

static inline RRDHOST_INGEST_STATUS rrdhost_status_ingest(RRDHOST *host, RRDHOST_STATUS *s, RRDHOST_FLAGS flags, RRDHOST_DB_STATUS db_status, bool online) {
    RRDHOST_INGEST_STATUS status;

    uint32_t collected_metrics = UINT32_MAX;
    uint32_t replicating_instances = UINT32_MAX;

    time_t since = MAX(host->stream.rcv.status.last_connected, host->stream.rcv.status.last_disconnected);
    STREAM_HANDSHAKE reason = host->stream.rcv.status.reason;

    if (online) {
        if (db_status == RRDHOST_DB_STATUS_INITIALIZING)
            status = RRDHOST_INGEST_STATUS_INITIALIZING;

        else if (rrdhost_is_local(host)) {
            status = RRDHOST_INGEST_STATUS_ONLINE;
            since = netdata_start_time;
        }
        else if (
            (replicating_instances = rrdhost_receiver_replicating_charts(host)) > 0 ||
            !(collected_metrics = __atomic_load_n(&host->collected.metrics_count, __ATOMIC_RELAXED)))
            status = RRDHOST_INGEST_STATUS_REPLICATING;

        else
            status = RRDHOST_INGEST_STATUS_ONLINE;
    }
    else {
        if(!host->stream.rcv.status.connections)
            status = RRDHOST_INGEST_STATUS_ARCHIVED;
        else
            status = RRDHOST_INGEST_STATUS_OFFLINE;
    }

    bool has_receiver = false;

    if(s) {
        if(status == RRDHOST_INGEST_STATUS_ARCHIVED)
            since = s->db.last_time_s;

        s->ingest.status = status;

        s->ingest.since = since ? since : netdata_start_time;
        s->ingest.reason = reason;
        s->ingest.hops = rrdhost_ingestion_hops(host);

        s->ingest.collected.metrics = collected_metrics == UINT32_MAX ? __atomic_load_n(&host->collected.metrics_count, __ATOMIC_RELAXED) : collected_metrics;
        s->ingest.collected.instances = __atomic_load_n(&host->collected.instances_count, __ATOMIC_RELAXED);
        s->ingest.collected.contexts = __atomic_load_n(&host->collected.contexts_count, __ATOMIC_RELAXED);

        if(!rrdhost_is_local(host)) {
            rrdhost_receiver_lock(host);
            if (host->receiver && (flags & RRDHOST_FLAG_COLLECTOR_ONLINE)) {
                has_receiver = true;
                s->ingest.replication.instances = replicating_instances == UINT32_MAX ? rrdhost_receiver_replicating_charts(host) : replicating_instances;
                s->ingest.replication.completion = host->stream.rcv.status.replication.percent;
                s->ingest.replication.in_progress = s->ingest.replication.instances > 0;

                s->ingest.capabilities = host->receiver->capabilities;
                s->ingest.peers = nd_sock_socket_peers(&host->receiver->sock);
                s->ingest.ssl = nd_sock_is_ssl(&host->receiver->sock);
            }
            rrdhost_receiver_unlock(host);
        }

        if(host == localhost)
            s->ingest.type = RRDHOST_INGEST_TYPE_LOCALHOST;
        else if(has_receiver)
            s->ingest.type = RRDHOST_INGEST_TYPE_CHILD;
        else if(rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST))
            s->ingest.type = RRDHOST_INGEST_TYPE_VIRTUAL;
        else
            s->ingest.type = RRDHOST_INGEST_TYPE_ARCHIVED;

        s->ingest.id = host->stream.rcv.status.connections;
    }

    return status;
}

static void rrdhost_status_stream_internal(RRDHOST_STATUS *s) {
    RRDHOST *host = s->host;
    time_t now = s->now;

    if (!host->sender) {
        s->stream.status = RRDHOST_STREAM_STATUS_DISABLED;
        s->stream.hops = (int16_t)(s->ingest.hops + 1);
    }
    else {
        stream_sender_lock(host->sender);

        s->stream.since = host->sender->last_state_since_t;
        s->stream.peers = nd_sock_socket_peers(&host->sender->sock);
        s->stream.ssl = nd_sock_is_ssl(&host->sender->sock);

        {
            STREAM_CIRCULAR_BUFFER_STATS *stats = stream_circular_buffer_stats_unsafe(host->sender->scb);

            memcpy(
                s->stream.sent_bytes_on_this_connection_per_type,
                stats->bytes_sent_by_type,
                MIN(sizeof(s->stream.sent_bytes_on_this_connection_per_type), sizeof(stats->bytes_sent_by_type)));
        }

        if (rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED)) {
            s->stream.hops = host->sender->hops;
            s->stream.capabilities = host->sender->capabilities;

            s->stream.replication.completion = rrdhost_sender_replication_completion_unsafe(host, now, &s->stream.replication.instances);
            s->stream.replication.in_progress = s->stream.replication.instances > 0;

            if(s->stream.replication.in_progress)
                s->stream.status = RRDHOST_STREAM_STATUS_REPLICATING;
            else
                s->stream.status = RRDHOST_STREAM_STATUS_ONLINE;

            s->stream.compression = host->sender->thread.compressor.initialized;
        }
        else {
            s->stream.status = RRDHOST_STREAM_STATUS_OFFLINE;
            s->stream.hops = (int16_t)(s->ingest.hops + 1);
        }
        s->stream.reason = host->stream.snd.status.reason;

        stream_sender_unlock(host->sender);
    }

    s->stream.id = host->stream.snd.status.connections;

    if(!s->stream.since)
        s->stream.since = netdata_start_time;
}

static void rrdhost_status_ml_internal(RRDHOST_STATUS *s) {
    RRDHOST *host = s->host;

    if(ml_host_get_host_status(host, &s->ml.metrics)) {
        if(stream_has_capability(&s->ingest, STREAM_CAP_ML_MODELS))
            s->ml.type = RRDHOST_ML_TYPE_RECEIVED;
        else
            s->ml.type = RRDHOST_ML_TYPE_SELF;

        if(s->ingest.status == RRDHOST_INGEST_STATUS_OFFLINE || s->ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
            s->ml.status = RRDHOST_ML_STATUS_OFFLINE;
        else
            s->ml.status = RRDHOST_ML_STATUS_RUNNING;
    }
    else {
        // does not receive ML, does not run ML
        s->ml.type = RRDHOST_ML_TYPE_DISABLED;
        s->ml.status = RRDHOST_ML_STATUS_DISABLED;
    }
}

static void rrdhost_status_health_internal(RRDHOST_STATUS *s, RRDHOST_FLAGS flags) {
    RRDHOST *host = s->host;

    if(host->health.enabled) {
        if(flags & RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION)
            s->health.status = RRDHOST_HEALTH_STATUS_INITIALIZING;
        else
            s->health.status = RRDHOST_HEALTH_STATUS_RUNNING;

        RRDCALC *rc;
        foreach_rrdcalc_in_rrdhost_read(host, rc) {
            if (unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
                continue;

            switch (rc->status) {
                default:
                case RRDCALC_STATUS_REMOVED:
                    break;

                case RRDCALC_STATUS_CLEAR:
                    s->health.alerts.clear++;
                    break;

                case RRDCALC_STATUS_WARNING:
                    s->health.alerts.warning++;
                    break;

                case RRDCALC_STATUS_CRITICAL:
                    s->health.alerts.critical++;
                    break;

                case RRDCALC_STATUS_UNDEFINED:
                    s->health.alerts.undefined++;
                    break;

                case RRDCALC_STATUS_UNINITIALIZED:
                    s->health.alerts.uninitialized++;
                    break;
            }
        }
        foreach_rrdcalc_in_rrdhost_done(rc);
    }
    else
        s->health.status = RRDHOST_HEALTH_STATUS_DISABLED;
}

void rrdhost_status(RRDHOST *host, time_t now, RRDHOST_STATUS *s, RRDHOST_STATUS_INFO info) {
    memset(s, 0, sizeof(*s));

    s->host = host;
    s->now = now;

    RRDHOST_FLAGS flags = __atomic_load_n(&host->flags, __ATOMIC_RELAXED);
    bool online = rrdhost_is_local(host) || rrdhost_is_online_flags(flags);

    // --- db ---

    rrdhost_status_db(host, now, s, flags, online);

    // --- ingest ---

    rrdhost_status_ingest(host, s, flags, s->db.status, online);

    // --- db (part 2) ---

    if(s->ingest.status == RRDHOST_INGEST_STATUS_ONLINE)
        s->db.liveness = RRDHOST_DB_LIVENESS_LIVE;
    else
        s->db.liveness = RRDHOST_DB_LIVENESS_STALE;

    // --- stream ---

    if(info & (RRDHOST_STATUS_STREAM | RRDHOST_STATUS_ML))
        rrdhost_status_stream_internal(s);

    // --- ml ---

    if(info & RRDHOST_STATUS_ML)
        rrdhost_status_ml_internal(s);

    // --- dyncfg ---

    if(info & RRDHOST_STATUS_DYNCFG)
        s->dyncfg.status = dyncfg_available_for_rrdhost(host) ? RRDHOST_DYNCFG_STATUS_AVAILABLE : RRDHOST_DYNCFG_STATUS_UNAVAILABLE;

    // --- health ---

    if(info & RRDHOST_STATUS_HEALTH)
        rrdhost_status_health_internal(s, flags);

}

// Minimal function to get the ingest status only
RRDHOST_INGEST_STATUS rrdhost_get_ingest_status(RRDHOST *host, time_t now) {
    RRDHOST_FLAGS flags = __atomic_load_n(&host->flags, __ATOMIC_RELAXED);
    bool online = rrdhost_is_local(host) || rrdhost_is_online_flags(flags);

    RRDHOST_DB_STATUS db_status = rrdhost_status_db(host, now, NULL, flags, online);
    return rrdhost_status_ingest(host, NULL, flags, db_status, online);
}

