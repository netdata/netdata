// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-status.h"
#include "sender-internals.h"

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

void rrdhost_status(RRDHOST *host, time_t now, RRDHOST_STATUS *s) {
    memset(s, 0, sizeof(*s));

    s->host = host;
    s->now = now;

    RRDHOST_FLAGS flags = __atomic_load_n(&host->flags, __ATOMIC_RELAXED);

    // --- dyncfg ---

    s->dyncfg.status = dyncfg_available_for_rrdhost(host) ? RRDHOST_DYNCFG_STATUS_AVAILABLE : RRDHOST_DYNCFG_STATUS_UNAVAILABLE;

    // --- db ---

    bool online = rrdhost_is_online(host);

    rrdhost_retention(host, now, online, &s->db.first_time_s, &s->db.last_time_s);
    s->db.metrics = host->rrdctx.metrics;
    s->db.instances = host->rrdctx.instances;
    s->db.contexts = dictionary_entries(host->rrdctx.contexts);
    if(!s->db.first_time_s || !s->db.last_time_s || !s->db.metrics || !s->db.instances || !s->db.contexts ||
        (flags & (RRDHOST_FLAG_PENDING_CONTEXT_LOAD)))
        s->db.status = RRDHOST_DB_STATUS_INITIALIZING;
    else
        s->db.status = RRDHOST_DB_STATUS_QUERYABLE;

    s->db.mode = host->rrd_memory_mode;

    // --- ingest ---

    s->ingest.since = MAX(host->stream.rcv.status.last_connected, host->stream.rcv.status.last_disconnected);
    s->ingest.reason = (online) ? STREAM_HANDSHAKE_NEVER : host->stream.rcv.status.exit_reason;

    spinlock_lock(&host->receiver_lock);
    s->ingest.hops = (int16_t)(host->system_info ? host->system_info->hops : (host == localhost) ? 0 : 1);
    bool has_receiver = false;
    if (host->receiver && !rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED)) {
        has_receiver = true;
        s->ingest.replication.instances = rrdhost_receiver_replicating_charts(host);
        s->ingest.replication.completion = host->stream.rcv.status.replication.percent;
        s->ingest.replication.in_progress = s->ingest.replication.instances > 0;

        s->ingest.capabilities = host->receiver->capabilities;
        s->ingest.peers = nd_sock_socket_peers(&host->receiver->sock);
        s->ingest.ssl = nd_sock_is_ssl(&host->receiver->sock);
    }
    spinlock_unlock(&host->receiver_lock);

    if (online) {
        if(s->db.status == RRDHOST_DB_STATUS_INITIALIZING)
            s->ingest.status = RRDHOST_INGEST_STATUS_INITIALIZING;

        else if (host == localhost || rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST)) {
            s->ingest.status = RRDHOST_INGEST_STATUS_ONLINE;
            s->ingest.since = netdata_start_time;
        }

        else if (s->ingest.replication.in_progress)
            s->ingest.status = RRDHOST_INGEST_STATUS_REPLICATING;

        else
            s->ingest.status = RRDHOST_INGEST_STATUS_ONLINE;
    }
    else {
        if (!s->ingest.since) {
            s->ingest.status = RRDHOST_INGEST_STATUS_ARCHIVED;
            s->ingest.since = s->db.last_time_s;
        }

        else
            s->ingest.status = RRDHOST_INGEST_STATUS_OFFLINE;
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

    if(!s->ingest.since)
        s->ingest.since = netdata_start_time;

    if(s->ingest.status == RRDHOST_INGEST_STATUS_ONLINE)
        s->db.liveness = RRDHOST_DB_LIVENESS_LIVE;
    else
        s->db.liveness = RRDHOST_DB_LIVENESS_STALE;

    // --- stream ---

    if (!host->sender) {
        s->stream.status = RRDHOST_STREAM_STATUS_DISABLED;
        s->stream.hops = (int16_t)(s->ingest.hops + 1);
    }
    else {
        sender_lock(host->sender);

        s->stream.since = host->sender->last_state_since_t;
        s->stream.peers = nd_sock_socket_peers(&host->sender->sock);
        s->stream.ssl = nd_sock_is_ssl(&host->sender->sock);

        memcpy(s->stream.sent_bytes_on_this_connection_per_type,
               host->sender->dispatcher.bytes_sent_by_type,
               MIN(sizeof(s->stream.sent_bytes_on_this_connection_per_type),
                   sizeof(host->sender->dispatcher.bytes_sent_by_type)));

        if (rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED)) {
            s->stream.hops = host->sender->hops;
            s->stream.reason = STREAM_HANDSHAKE_NEVER;
            s->stream.capabilities = host->sender->capabilities;

            s->stream.replication.completion = rrdhost_sender_replication_completion_unsafe(host, now, &s->stream.replication.instances);
            s->stream.replication.in_progress = s->stream.replication.instances > 0;

            if(s->stream.replication.in_progress)
                s->stream.status = RRDHOST_STREAM_STATUS_REPLICATING;
            else
                s->stream.status = RRDHOST_STREAM_STATUS_ONLINE;

            s->stream.compression = host->sender->compressor.initialized;
        }
        else {
            s->stream.status = RRDHOST_STREAM_STATUS_OFFLINE;
            s->stream.hops = (int16_t)(s->ingest.hops + 1);
            s->stream.reason = host->sender->exit.reason;
        }

        sender_unlock(host->sender);
    }

    s->stream.id = host->stream.snd.status.connections;

    if(!s->stream.since)
        s->stream.since = netdata_start_time;

    // --- ml ---

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

    // --- health ---

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
