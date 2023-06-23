// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

#include "aclk/aclk_capas.h"

// ----------------------------------------------------------------------------
// /api/v2/contexts API

typedef enum __attribute__ ((__packed__)) {
    FTS_MATCHED_NONE = 0,
    FTS_MATCHED_HOST,
    FTS_MATCHED_CONTEXT,
    FTS_MATCHED_INSTANCE,
    FTS_MATCHED_DIMENSION,
    FTS_MATCHED_LABEL,
    FTS_MATCHED_ALERT,
    FTS_MATCHED_ALERT_INFO,
    FTS_MATCHED_FAMILY,
    FTS_MATCHED_TITLE,
    FTS_MATCHED_UNITS,
} FTS_MATCH;

static const char *fts_match_to_string(FTS_MATCH match) {
    switch(match) {
        case FTS_MATCHED_HOST:
            return "HOST";

        case FTS_MATCHED_CONTEXT:
            return "CONTEXT";

        case FTS_MATCHED_INSTANCE:
            return "INSTANCE";

        case FTS_MATCHED_DIMENSION:
            return "DIMENSION";

        case FTS_MATCHED_ALERT:
            return "ALERT";

        case FTS_MATCHED_ALERT_INFO:
            return "ALERT_INFO";

        case FTS_MATCHED_LABEL:
            return "LABEL";

        case FTS_MATCHED_FAMILY:
            return "FAMILY";

        case FTS_MATCHED_TITLE:
            return "TITLE";

        case FTS_MATCHED_UNITS:
            return "UNITS";

        default:
            return "NONE";
    }
}

struct rrdfunction_to_json_v2 {
    size_t size;
    size_t used;
    size_t *node_ids;
    STRING *help;
};

struct rrdcontext_to_json_v2_entry {
    size_t count;
    STRING *id;
    STRING *family;
    uint32_t priority;
    time_t first_time_s;
    time_t last_time_s;
    RRD_FLAGS flags;
    FTS_MATCH match;
};

typedef struct full_text_search_index {
    size_t searches;
    size_t string_searches;
    size_t char_searches;
} FTS_INDEX;

static inline bool full_text_search_string(FTS_INDEX *fts, SIMPLE_PATTERN *q, STRING *ptr) {
    fts->searches++;
    fts->string_searches++;
    return simple_pattern_matches_string(q, ptr);
}

static inline bool full_text_search_char(FTS_INDEX *fts, SIMPLE_PATTERN *q, char *ptr) {
    fts->searches++;
    fts->char_searches++;
    return simple_pattern_matches(q, ptr);
}

struct rrdcontext_to_json_v2_data {
    time_t now;

    BUFFER *wb;
    union {
        struct api_v2_contexts_request *request;
        struct api_v2_alerts_request *alerts_request;
    };

    DICTIONARY *ctx;

    CONTEXTS_V2_OPTIONS options;
    struct query_versions versions;

    struct {
        SIMPLE_PATTERN *scope_pattern;
        SIMPLE_PATTERN *pattern;
        size_t ni;
    } nodes;

    struct {
        Pvoid_t JudyHS;
//        ALERT_OPTIONS alert_options;
        SIMPLE_PATTERN *scope_pattern;
        SIMPLE_PATTERN *pattern;
//        time_t after;
//        time_t before;
        size_t li;
    } alerts;

    struct {
        SIMPLE_PATTERN *scope_pattern;
        SIMPLE_PATTERN *pattern;
    } contexts;

    struct {
        FTS_MATCH host_match;
        char host_node_id_str[UUID_STR_LEN];
        SIMPLE_PATTERN *pattern;
        FTS_INDEX fts;
    } q;

    struct {
        DICTIONARY *dict;
    } functions;

    struct {
        bool enabled;
        bool relative;
        time_t after;
        time_t before;
    } window;

    struct query_timings timings;
};

static void add_alert_index(Pvoid_t *JudyHS, uuid_t *uuid, ssize_t idx)
{
    Pvoid_t *PValue = JudyHSIns(JudyHS, uuid, sizeof(*uuid), PJE0);
    if (!PValue)
        return;
    *((Word_t *) PValue) = (Word_t) idx;
}

ssize_t get_alert_index(Pvoid_t JudyHS, uuid_t *uuid)
{
    Pvoid_t *PValue = JudyHSGet(JudyHS, uuid, sizeof(*uuid));
    if (!PValue)
        return -1;
    return (ssize_t) *((Word_t *) PValue);
}

static FTS_MATCH rrdcontext_to_json_v2_full_text_search(struct rrdcontext_to_json_v2_data *ctl, RRDCONTEXT *rc, SIMPLE_PATTERN *q) {
    if(unlikely(full_text_search_string(&ctl->q.fts, q, rc->id) ||
                full_text_search_string(&ctl->q.fts, q, rc->family)))
        return FTS_MATCHED_CONTEXT;

    if(unlikely(full_text_search_string(&ctl->q.fts, q, rc->title)))
        return FTS_MATCHED_TITLE;

    if(unlikely(full_text_search_string(&ctl->q.fts, q, rc->units)))
        return FTS_MATCHED_UNITS;

    FTS_MATCH matched = FTS_MATCHED_NONE;
    RRDINSTANCE *ri;
    dfe_start_read(rc->rrdinstances, ri) {
        if(matched) break;

        if(ctl->window.enabled && !query_matches_retention(ctl->window.after, ctl->window.before, ri->first_time_s, (ri->flags & RRD_FLAG_COLLECTED) ? ctl->now : ri->last_time_s, 0))
            continue;

        if(unlikely(full_text_search_string(&ctl->q.fts, q, ri->id)) ||
           (ri->name != ri->id && full_text_search_string(&ctl->q.fts, q, ri->name))) {
            matched = FTS_MATCHED_INSTANCE;
            break;
        }

        RRDMETRIC *rm;
        dfe_start_read(ri->rrdmetrics, rm) {
            if(ctl->window.enabled && !query_matches_retention(ctl->window.after, ctl->window.before, rm->first_time_s, (rm->flags & RRD_FLAG_COLLECTED) ? ctl->now : rm->last_time_s, 0))
                continue;

            if(unlikely(full_text_search_string(&ctl->q.fts, q, rm->id)) ||
               (rm->name != rm->id && full_text_search_string(&ctl->q.fts, q, rm->name))) {
                matched = FTS_MATCHED_DIMENSION;
                break;
            }
        }
        dfe_done(rm);

        size_t label_searches = 0;
        if(unlikely(ri->rrdlabels && dictionary_entries(ri->rrdlabels) &&
                    rrdlabels_match_simple_pattern_parsed(ri->rrdlabels, q, ':', &label_searches))) {
            ctl->q.fts.searches += label_searches;
            ctl->q.fts.char_searches += label_searches;
            matched = FTS_MATCHED_LABEL;
            break;
        }
        ctl->q.fts.searches += label_searches;
        ctl->q.fts.char_searches += label_searches;

        if(ri->rrdset) {
            RRDSET *st = ri->rrdset;
            netdata_rwlock_rdlock(&st->alerts.rwlock);
            for (RRDCALC *rcl = st->alerts.base; rcl; rcl = rcl->next) {
                if(unlikely(full_text_search_string(&ctl->q.fts, q, rcl->name))) {
                    matched = FTS_MATCHED_ALERT;
                    break;
                }

                if(unlikely(full_text_search_string(&ctl->q.fts, q, rcl->info))) {
                    matched = FTS_MATCHED_ALERT_INFO;
                    break;
                }
            }
            netdata_rwlock_unlock(&st->alerts.rwlock);
        }
    }
    dfe_done(ri);
    return matched;
}

static ssize_t rrdcontext_to_json_v2_add_context(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context __maybe_unused) {
    struct rrdcontext_to_json_v2_data *ctl = data;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    if(ctl->window.enabled && !query_matches_retention(ctl->window.after, ctl->window.before, rc->first_time_s, (rc->flags & RRD_FLAG_COLLECTED) ? ctl->now : rc->last_time_s, 0))
        return 0; // continue to next context

    FTS_MATCH match = ctl->q.host_match;
    if((ctl->options & CONTEXTS_V2_SEARCH) && ctl->q.pattern) {
        match = rrdcontext_to_json_v2_full_text_search(ctl, rc, ctl->q.pattern);

        if(match == FTS_MATCHED_NONE)
            return 0; // continue to next context
    }

    struct rrdcontext_to_json_v2_entry t = {
            .count = 1,
            .id = rc->id,
            .family = string_dup(rc->family),
            .priority = rc->priority,
            .first_time_s = rc->first_time_s,
            .last_time_s = rc->last_time_s,
            .flags = rc->flags,
            .match = match,
    };

    dictionary_set(ctl->ctx, string2str(rc->id), &t, sizeof(t));

    return 1;
}

void buffer_json_agent_status_id(BUFFER *wb, size_t ai, usec_t duration_ut) {
    buffer_json_member_add_object(wb, "st");
    {
        buffer_json_member_add_uint64(wb, "ai", ai);
        buffer_json_member_add_uint64(wb, "code", 200);
        buffer_json_member_add_string(wb, "msg", "");
        if (duration_ut)
            buffer_json_member_add_double(wb, "ms", (NETDATA_DOUBLE) duration_ut / 1000.0);
    }
    buffer_json_object_close(wb);
}

static ssize_t alert_to_json_v2_add_context(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context __maybe_unused)
{
    return rrdcontext_to_json_v2_add_context(data, rca, queryable_context);
}

void buffer_json_node_add_v2(BUFFER *wb, RRDHOST *host, size_t ni, usec_t duration_ut, bool status) {
    buffer_json_member_add_string(wb, "mg", host->machine_guid);

    if(host->node_id)
        buffer_json_member_add_uuid(wb, "nd", host->node_id);
    buffer_json_member_add_string(wb, "nm", rrdhost_hostname(host));
    buffer_json_member_add_uint64(wb, "ni", ni);

    if(status)
        buffer_json_agent_status_id(wb, 0, duration_ut);
}

static void rrdhost_receiver_to_json(BUFFER *wb, RRDHOST_STATUS *s, const char *key) {
    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_uint64(wb, "id", s->ingest.id);
        buffer_json_member_add_uint64(wb, "hops", s->ingest.hops);
        buffer_json_member_add_string(wb, "type", rrdhost_ingest_type_to_string(s->ingest.type));
        buffer_json_member_add_string(wb, "status", rrdhost_ingest_status_to_string(s->ingest.status));
        buffer_json_member_add_time_t(wb, "since", s->ingest.since);
        buffer_json_member_add_time_t(wb, "age", s->now - s->ingest.since);

        if(s->ingest.type == RRDHOST_INGEST_TYPE_CHILD) {
            if(s->ingest.status == RRDHOST_INGEST_STATUS_OFFLINE)
                buffer_json_member_add_string(wb, "reason", s->ingest.reason);

            if(s->ingest.status == RRDHOST_INGEST_STATUS_REPLICATING) {
                buffer_json_member_add_object(wb, "replication");
                {
                    buffer_json_member_add_boolean(wb, "in_progress", s->ingest.replication.in_progress);
                    buffer_json_member_add_double(wb, "completion", s->ingest.replication.completion);
                    buffer_json_member_add_uint64(wb, "instances", s->ingest.replication.instances);
                }
                buffer_json_object_close(wb); // replication
            }

            if(s->ingest.status == RRDHOST_INGEST_STATUS_REPLICATING || s->ingest.status == RRDHOST_INGEST_STATUS_ONLINE) {
                buffer_json_member_add_object(wb, "source");
                {
                    char buf[1024 + 1];
                    snprintfz(buf, 1024, "[%s]:%d%s", s->ingest.peers.local.ip, s->ingest.peers.local.port, s->ingest.ssl ? ":SSL" : "");
                    buffer_json_member_add_string(wb, "local", buf);

                    snprintfz(buf, 1024, "[%s]:%d%s", s->ingest.peers.peer.ip, s->ingest.peers.peer.port, s->ingest.ssl ? ":SSL" : "");
                    buffer_json_member_add_string(wb, "remote", buf);

                    stream_capabilities_to_json_array(wb, s->ingest.capabilities, "capabilities");
                }
                buffer_json_object_close(wb); // source
            }
        }
    }
    buffer_json_object_close(wb); // collection
}

static void rrdhost_sender_to_json(BUFFER *wb, RRDHOST_STATUS *s, const char *key) {
    if(s->stream.status == RRDHOST_STREAM_STATUS_DISABLED)
        return;

    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_uint64(wb, "id", s->stream.id);
        buffer_json_member_add_uint64(wb, "hops", s->stream.hops);
        buffer_json_member_add_string(wb, "status", rrdhost_streaming_status_to_string(s->stream.status));
        buffer_json_member_add_time_t(wb, "since", s->stream.since);
        buffer_json_member_add_time_t(wb, "age", s->now - s->stream.since);

        if (s->stream.status == RRDHOST_STREAM_STATUS_OFFLINE)
            buffer_json_member_add_string(wb, "reason", s->stream.reason);

        if (s->stream.status == RRDHOST_STREAM_STATUS_REPLICATING) {
            buffer_json_member_add_object(wb, "replication");
            {
                buffer_json_member_add_boolean(wb, "in_progress", s->stream.replication.in_progress);
                buffer_json_member_add_double(wb, "completion", s->stream.replication.completion);
                buffer_json_member_add_uint64(wb, "instances", s->stream.replication.instances);
            }
            buffer_json_object_close(wb);
        }

        buffer_json_member_add_object(wb, "destination");
        {
            char buf[1024 + 1];
            snprintfz(buf, 1024, "[%s]:%d%s", s->stream.peers.local.ip, s->stream.peers.local.port, s->stream.ssl ? ":SSL" : "");
            buffer_json_member_add_string(wb, "local", buf);

            snprintfz(buf, 1024, "[%s]:%d%s", s->stream.peers.peer.ip, s->stream.peers.peer.port, s->stream.ssl ? ":SSL" : "");
            buffer_json_member_add_string(wb, "remote", buf);

            stream_capabilities_to_json_array(wb, s->stream.capabilities, "capabilities");

            buffer_json_member_add_object(wb, "traffic");
            {
                buffer_json_member_add_boolean(wb, "compression", s->stream.compression);
                buffer_json_member_add_uint64(wb, "data", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DATA]);
                buffer_json_member_add_uint64(wb, "metadata", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_METADATA]);
                buffer_json_member_add_uint64(wb, "functions", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_FUNCTIONS]);
                buffer_json_member_add_uint64(wb, "replication", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_REPLICATION]);
            }
            buffer_json_object_close(wb); // traffic

            buffer_json_member_add_array(wb, "candidates");
            struct rrdpush_destinations *d;
            for (d = s->host->destinations; d; d = d->next) {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_uint64(wb, "attempts", d->attempts);
                {

                    if (d->ssl) {
                        snprintfz(buf, 1024, "%s:SSL", string2str(d->destination));
                        buffer_json_member_add_string(wb, "destination", buf);
                    }
                    else
                        buffer_json_member_add_string(wb, "destination", string2str(d->destination));

                    buffer_json_member_add_time_t(wb, "last_check", d->last_attempt);
                    buffer_json_member_add_time_t(wb, "age", s->now - d->last_attempt);
                    buffer_json_member_add_string_or_omit(wb, "last_error", d->last_error);
                    buffer_json_member_add_string(wb, "last_handshake", stream_handshake_error_to_string(d->last_handshake));
                    if(d->postpone_reconnection_until > s->now) {
                        buffer_json_member_add_time_t(wb, "next_check", d->postpone_reconnection_until);
                        buffer_json_member_add_time_t(wb, "next_in", d->postpone_reconnection_until - s->now);
                    }
                }
                buffer_json_object_close(wb); // each candidate
            }
            buffer_json_array_close(wb); // candidates
        }
        buffer_json_object_close(wb); // destination
    }
    buffer_json_object_close(wb); // streaming
}

static void agent_capabilities_to_json(BUFFER *wb, RRDHOST *host, const char *key) {
    buffer_json_member_add_array(wb, key);

    struct capability *capas = aclk_get_node_instance_capas(host);
    for(struct capability *capa = capas; capa->name ;capa++) {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "name", capa->name);
            buffer_json_member_add_uint64(wb, "version", capa->version);
            buffer_json_member_add_boolean(wb, "enabled", capa->enabled);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);
    freez(capas);
}

static inline void rrdhost_health_to_json_v2(BUFFER *wb, const char *key, RRDHOST_STATUS *s) {
    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_string(wb, "status", rrdhost_health_status_to_string(s->health.status));
        if (s->health.status == RRDHOST_HEALTH_STATUS_RUNNING) {
            buffer_json_member_add_object(wb, "alerts");
            {
                buffer_json_member_add_uint64(wb, "critical", s->health.alerts.critical);
                buffer_json_member_add_uint64(wb, "warning", s->health.alerts.warning);
                buffer_json_member_add_uint64(wb, "clear", s->health.alerts.clear);
                buffer_json_member_add_uint64(wb, "undefined", s->health.alerts.undefined);
                buffer_json_member_add_uint64(wb, "uninitialized", s->health.alerts.uninitialized);
            }
            buffer_json_object_close(wb); // alerts
        }
    }
    buffer_json_object_close(wb); // health
}

static ssize_t rrdcontext_to_json_v2_add_host(void *data, RRDHOST *host, bool queryable_host) {
    if(!queryable_host || !host->rrdctx.contexts)
        // the host matches the 'scope_host' but does not match the 'host' patterns
        // or the host does not have any contexts
        return 0; // continue to next host

    struct rrdcontext_to_json_v2_data *ctl = data;
    BUFFER *wb = ctl->wb;

    if(ctl->window.enabled && !rrdhost_matches_window(host, ctl->window.after, ctl->window.before, ctl->now))
        // the host does not have data in the requested window
        return 0; // continue to next host

    if(ctl->request->timeout_ms && now_monotonic_usec() > ctl->timings.received_ut + ctl->request->timeout_ms * USEC_PER_MS)
        // timed out
        return -2; // stop the query

    if(ctl->request->interrupt_callback && ctl->request->interrupt_callback(ctl->request->interrupt_callback_data))
        // interrupted
        return -1; // stop the query

    bool host_matched = (ctl->options & CONTEXTS_V2_NODES);
    bool do_contexts = (ctl->options & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH));

    ctl->q.host_match = FTS_MATCHED_NONE;
    if((ctl->options & CONTEXTS_V2_SEARCH)) {
        // check if we match the host itself
        if(ctl->q.pattern && (
                full_text_search_string(&ctl->q.fts, ctl->q.pattern, host->hostname) ||
                full_text_search_char(&ctl->q.fts, ctl->q.pattern, host->machine_guid) ||
                (ctl->q.pattern && full_text_search_char(&ctl->q.fts, ctl->q.pattern, ctl->q.host_node_id_str)))) {
            ctl->q.host_match = FTS_MATCHED_HOST;
            do_contexts = true;
        }
    }

    if(do_contexts) {
        // save it
        SIMPLE_PATTERN *old_q = ctl->q.pattern;

        if(ctl->q.host_match == FTS_MATCHED_HOST)
            // do not do pattern matching on contexts - we matched the host itself
            ctl->q.pattern = NULL;

        ssize_t added = query_scope_foreach_context(
                host, ctl->request->scope_contexts,
                ctl->contexts.scope_pattern, ctl->contexts.pattern,
                rrdcontext_to_json_v2_add_context, queryable_host, ctl);

        // restore it
        ctl->q.pattern = old_q;

        if(unlikely(added < 0))
            return -1; // stop the query

        if(added)
            host_matched = true;
    }

    if(!host_matched)
        return 0;

    if(ctl->options & CONTEXTS_V2_FUNCTIONS) {
        struct rrdfunction_to_json_v2 t = {
                .used = 1,
                .size = 1,
                .node_ids = &ctl->nodes.ni,
                .help = NULL,
        };
        host_functions_to_dict(host, ctl->functions.dict, &t, sizeof(t), &t.help);
    }

    if(ctl->options & CONTEXTS_V2_NODES) {
        buffer_json_add_array_item_object(wb); // this node
        buffer_json_node_add_v2(wb, host, ctl->nodes.ni++, 0,
                                (ctl->options & CONTEXTS_V2_AGENTS) && !(ctl->options & CONTEXTS_V2_NODES_INSTANCES));

        if(ctl->options & (CONTEXTS_V2_NODES_INFO | CONTEXTS_V2_NODES_INSTANCES)) {
            RRDHOST_STATUS s;
            rrdhost_status(host, ctl->now, &s);

            if (ctl->options & (CONTEXTS_V2_NODES_INFO)) {
                buffer_json_member_add_string(wb, "v", rrdhost_program_version(host));

                host_labels2json(host, wb, "labels");

                if (host->system_info) {
                    buffer_json_member_add_object(wb, "hw");
                    {
                        buffer_json_member_add_string_or_empty(wb, "architecture", host->system_info->architecture);
                        buffer_json_member_add_string_or_empty(wb, "cpu_frequency", host->system_info->host_cpu_freq);
                        buffer_json_member_add_string_or_empty(wb, "cpus", host->system_info->host_cores);
                        buffer_json_member_add_string_or_empty(wb, "memory", host->system_info->host_ram_total);
                        buffer_json_member_add_string_or_empty(wb, "disk_space", host->system_info->host_disk_space);
                        buffer_json_member_add_string_or_empty(wb, "virtualization", host->system_info->virtualization);
                        buffer_json_member_add_string_or_empty(wb, "container", host->system_info->container);
                    }
                    buffer_json_object_close(wb);

                    buffer_json_member_add_object(wb, "os");
                    {
                        buffer_json_member_add_string_or_empty(wb, "id", host->system_info->host_os_id);
                        buffer_json_member_add_string_or_empty(wb, "nm", host->system_info->host_os_name);
                        buffer_json_member_add_string_or_empty(wb, "v", host->system_info->host_os_version);
                        buffer_json_member_add_object(wb, "kernel");
                        buffer_json_member_add_string_or_empty(wb, "nm", host->system_info->kernel_name);
                        buffer_json_member_add_string_or_empty(wb, "v", host->system_info->kernel_version);
                        buffer_json_object_close(wb);
                    }
                    buffer_json_object_close(wb);
                }

                // created      - the node is created but never connected to cloud
                // unreachable  - not currently connected
                // stale        - connected but not having live data
                // reachable    - connected with live data
                // pruned       - not connected for some time and has been removed
                buffer_json_member_add_string(wb, "state", rrdhost_state_cloud_emulation(host) ? "reachable" : "stale");

                rrdhost_health_to_json_v2(wb, "health", &s);
                agent_capabilities_to_json(wb, host, "capabilities");
            }

            if (ctl->options & (CONTEXTS_V2_NODES_INSTANCES)) {
                buffer_json_member_add_array(wb, "instances");
                buffer_json_add_array_item_object(wb); // this instance
                {
                    buffer_json_agent_status_id(wb, 0, 0);

                    buffer_json_member_add_object(wb, "db");
                    {
                        buffer_json_member_add_string(wb, "status", rrdhost_db_status_to_string(s.db.status));
                        buffer_json_member_add_string(wb, "liveness", rrdhost_db_liveness_to_string(s.db.liveness));
                        buffer_json_member_add_string(wb, "mode", rrd_memory_mode_name(s.db.mode));
                        buffer_json_member_add_time_t(wb, "first_time", s.db.first_time_s);
                        buffer_json_member_add_time_t(wb, "last_time", s.db.last_time_s);
                        buffer_json_member_add_uint64(wb, "metrics", s.db.metrics);
                        buffer_json_member_add_uint64(wb, "instances", s.db.instances);
                        buffer_json_member_add_uint64(wb, "contexts", s.db.contexts);
                    }
                    buffer_json_object_close(wb);

                    rrdhost_receiver_to_json(wb, &s, "ingest");
                    rrdhost_sender_to_json(wb, &s, "stream");

                    buffer_json_member_add_object(wb, "ml");
                    buffer_json_member_add_string(wb, "status", rrdhost_ml_status_to_string(s.ml.status));
                    buffer_json_member_add_string(wb, "type", rrdhost_ml_type_to_string(s.ml.type));
                    if (s.ml.status == RRDHOST_ML_STATUS_RUNNING) {
                        buffer_json_member_add_object(wb, "metrics");
                        {
                            buffer_json_member_add_uint64(wb, "anomalous", s.ml.metrics.anomalous);
                            buffer_json_member_add_uint64(wb, "normal", s.ml.metrics.normal);
                            buffer_json_member_add_uint64(wb, "trained", s.ml.metrics.trained);
                            buffer_json_member_add_uint64(wb, "pending", s.ml.metrics.pending);
                            buffer_json_member_add_uint64(wb, "silenced", s.ml.metrics.silenced);
                        }
                        buffer_json_object_close(wb); // metrics
                    }
                    buffer_json_object_close(wb); // ml

                    rrdhost_health_to_json_v2(wb, "health", &s);

                    host_functions2json(host, wb); // functions
                    agent_capabilities_to_json(wb, host, "capabilities");
                }
                buffer_json_object_close(wb); // this instance
                buffer_json_array_close(wb); // instances
            }
        }
        buffer_json_object_close(wb); // this node
    }

    return 1;
}

static ssize_t alert_to_json_v2_add_host(void *data, RRDHOST *host, bool queryable_host) {
    if(!queryable_host || !host->rrdctx.contexts)
        // the host matches the 'scope_host' but does not match the 'host' patterns
        // or the host does not have any contexts
        return 0;

    struct rrdcontext_to_json_v2_data *ctl = data;
    BUFFER *wb = ctl->wb;

    bool host_matched = (ctl->options & CONTEXTS_V2_NODES);
    bool do_contexts = (ctl->options & (CONTEXTS_V2_CONTEXTS));

    ctl->q.host_match = FTS_MATCHED_NONE;
    if((ctl->options & CONTEXTS_V2_SEARCH)) {
        // check if we match the host itself
        if(ctl->q.pattern && (
                                  full_text_search_string(&ctl->q.fts, ctl->q.pattern, host->hostname) ||
                                  full_text_search_char(&ctl->q.fts, ctl->q.pattern, host->machine_guid) ||
                                  (ctl->q.pattern && full_text_search_char(&ctl->q.fts, ctl->q.pattern, ctl->q.host_node_id_str)))) {
            ctl->q.host_match = FTS_MATCHED_HOST;
            do_contexts = true;
        }
    }

    if(do_contexts) {
        // save it
        SIMPLE_PATTERN *old_q = ctl->q.pattern;

        if(ctl->q.host_match == FTS_MATCHED_HOST)
            // do not do pattern matching on contexts - we matched the host itself
            ctl->q.pattern = NULL;

        ssize_t added = query_scope_foreach_context(
            host, ctl->alerts_request->scope_contexts,
            ctl->contexts.scope_pattern, ctl->contexts.pattern,
            alert_to_json_v2_add_context, queryable_host, ctl);

        // restore it
        ctl->q.pattern = old_q;

        if(added == -1)
            return -1;

        if(added)
            host_matched = true;
    }

    if(host_matched && (ctl->options & (CONTEXTS_V2_NODES))) {
        buffer_json_add_array_item_object(wb);
        buffer_json_node_add_v2(wb, host, ctl->nodes.ni++, 0, false);

        if (ctl->alerts_request->options & ALERT_OPTION_TRANSITIONS) {
            if (rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH)) {
                buffer_json_member_add_array(wb, "instances");
                health_alert2json(host, wb, ctl->alerts_request->options, ctl->alerts.JudyHS, ctl->alerts_request->after, ctl->alerts_request->before, ctl->alerts_request->last);
                buffer_json_array_close(wb);
            }
        }

        buffer_json_object_close(wb);
    }

    return host_matched ? 1 : 0;
}

static inline bool alert_is_matched( struct api_v2_alerts_request *alerts_request, RRDCALC *rc)
{
    char hash_id[UUID_STR_LEN];
    uuid_unparse_lower(rc->config_hash_id, hash_id);

    if (alerts_request->alert_id)
        return (rc->id == alerts_request->alert_id);

    SIMPLE_PATTERN_RESULT match = SP_MATCHED_POSITIVE;
    SIMPLE_PATTERN *match_pattern = alerts_request->config_hash_pattern;
    if(match_pattern) {
        match = simple_pattern_matches_extract(match_pattern, hash_id, NULL, 0);
        if(match == SP_NOT_MATCHED)
            return false;;
    }

    match = SP_MATCHED_POSITIVE;
    match_pattern = alerts_request->alert_name_pattern;
    if(match_pattern) {
        match = simple_pattern_matches_string_extract(match_pattern, rc->name, NULL, 0);
        if(match == SP_NOT_MATCHED)
            return false;
    }

    return true;
}

static ssize_t alert_to_json_v2_add_alert(void *data, RRDHOST *host, bool queryable_host) {
    if(!queryable_host || !host->rrdctx.contexts)
        // the host matches the 'scope_host' but does not match the 'host' patterns
        // or the host does not have any contexts
        return 0;

    struct rrdcontext_to_json_v2_data *ctl = data;
    BUFFER *wb = ctl->wb;

    bool host_matched = (ctl->options & CONTEXTS_V2_NODES);
    bool do_contexts = (ctl->options & (CONTEXTS_V2_CONTEXTS));

    if(do_contexts) {
        ssize_t added = query_scope_foreach_context(
            host, ctl->request->scope_contexts,
            ctl->contexts.scope_pattern, ctl->contexts.pattern,
            alert_to_json_v2_add_context, queryable_host, ctl);

        if(added == -1)
            return -1;

        if(added)
            host_matched = true;
    }

    if(host_matched && (ctl->options & (CONTEXTS_V2_NODES))) {
        if (rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH)) {

            RRDCALC *rc;
            foreach_rrdcalc_in_rrdhost_read(host, rc) {
                if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
                    continue;

                if (unlikely(!rrdset_is_available_for_exporting_and_alarms(rc->rrdset)))
                    continue;

                if ((ctl->alerts_request->options & ALERT_OPTION_ACTIVE) &&
                    !(rc->status == RRDCALC_STATUS_WARNING || rc->status == RRDCALC_STATUS_CRITICAL))
                    continue;

                char hash_id[GUID_LEN + 1];
                uuid_unparse_lower(rc->config_hash_id, hash_id);

                if (!alert_is_matched(ctl->alerts_request, rc))
                    continue;

                ssize_t idx = get_alert_index(ctl->alerts.JudyHS, &rc->config_hash_id);
                if (idx >= 0)
                    continue;

                buffer_json_add_array_item_object(wb);
                add_alert_index(&ctl->alerts.JudyHS, &rc->config_hash_id, (ssize_t)ctl->alerts.li++);

                buffer_json_member_add_string(wb, "config_hash_id", hash_id);
                buffer_json_member_add_string(wb, "name", rrdcalc_name(rc));
                buffer_json_member_add_string(wb, "chart", rrdcalc_chart_name(rc));
                buffer_json_member_add_string(wb, "family", (rc->rrdset) ? rrdset_family(rc->rrdset) : "");
                buffer_json_member_add_string(wb, "class", rc->classification ? rrdcalc_classification(rc) : "Unknown");
                buffer_json_member_add_string(wb, "component", rc->component ? rrdcalc_component(rc) : "Unknown");
                buffer_json_member_add_string(wb, "type", rc->type ? rrdcalc_type(rc) : "Unknown");
                buffer_json_member_add_string(wb, "units", rrdcalc_units(rc));
                buffer_json_member_add_boolean(wb, "enabled", host->health.health_enabled);

                if (ctl->alerts_request->options & ALERT_OPTION_CONFIG) {
                    buffer_json_member_add_object(wb, "config");
                    {
                        buffer_json_member_add_boolean(wb, "active", (rc->rrdset));
                        buffer_json_member_add_boolean(wb, "disabled", (rc->run_flags & RRDCALC_FLAG_DISABLED));
                        buffer_json_member_add_boolean(wb, "silenced", (rc->run_flags & RRDCALC_FLAG_SILENCED));
                        buffer_json_member_add_string(
                            wb, "exec", rc->exec ? rrdcalc_exec(rc) : string2str(host->health.health_default_exec));
                        buffer_json_member_add_string(
                            wb,
                            "recipient",
                            rc->recipient ? rrdcalc_recipient(rc) : string2str(host->health.health_default_recipient));
                        buffer_json_member_add_string(wb, "info", rrdcalc_info(rc));
                        buffer_json_member_add_string(wb, "source", rrdcalc_source(rc));
                        buffer_json_member_add_time_t(wb, "update_every", rc->update_every);
                        buffer_json_member_add_time_t(wb, "delay_up_duration", rc->delay_up_duration);
                        buffer_json_member_add_time_t(wb, "delay_down_duration", rc->delay_down_duration);
                        buffer_json_member_add_time_t(wb, "delay_max_duration", rc->delay_max_duration);
                        buffer_json_member_add_double(wb, "delay_multiplier", rc->delay_multiplier);
                        buffer_json_member_add_time_t(wb, "delay", rc->delay_last);
                        buffer_json_member_add_time_t(wb, "warn_repeat_every", rc->warn_repeat_every);
                        buffer_json_member_add_time_t(wb, "crit_repeat_every", rc->crit_repeat_every);
                        if (unlikely(rc->options & RRDCALC_OPTION_NO_CLEAR_NOTIFICATION)) {
                            buffer_json_member_add_boolean(wb, "no_clear_notification", true);
                        }

                        if (rc->calculation) {
                            buffer_json_member_add_string(wb, "calc", rc->calculation->source);
                            buffer_json_member_add_string(wb, "calc_parsed", rc->calculation->parsed_as);
                        }

                        if (rc->warning) {
                            buffer_json_member_add_string(wb, "warn", rc->warning->source);
                            buffer_json_member_add_string(wb, "warn_parsed", rc->warning->parsed_as);
                        }

                        if (rc->critical) {
                            buffer_json_member_add_string(wb, "crit", rc->critical->source);
                            buffer_json_member_add_string(wb, "crit_parsed", rc->critical->parsed_as);
                        }

                        if (RRDCALC_HAS_DB_LOOKUP(rc)) {
                            if (rc->dimensions)
                                buffer_json_member_add_string(wb, "lookup_dimensions", rrdcalc_dimensions(rc));

                            buffer_json_member_add_string(wb, "lookup_method", time_grouping_method2string(rc->group));
                            buffer_json_member_add_time_t(wb, "lookup_after", rc->after);
                            buffer_json_member_add_time_t(wb, "lookup_before", rc->before);

                            BUFFER *temp_id = buffer_create(1, NULL);
                            buffer_data_options2string(temp_id, rc->options);

                            buffer_json_member_add_string(wb, "lookup_options", buffer_tostring(temp_id));

                            buffer_free(temp_id);
                        }
                    }
                    buffer_json_object_close(wb); // config
                }
                buffer_json_object_close(wb);   // Alert
            }
            foreach_rrdcalc_in_rrdhost_done(rc);
        }
    }

    return host_matched ? 1 : 0;
}

static void buffer_json_contexts_v2_options_to_array(BUFFER *wb, CONTEXTS_V2_OPTIONS options) {
    if(options & CONTEXTS_V2_DEBUG)
        buffer_json_add_array_item_string(wb, "debug");

    if(options & CONTEXTS_V2_MINIFY)
        buffer_json_add_array_item_string(wb, "minify");

    if(options & CONTEXTS_V2_VERSIONS)
        buffer_json_add_array_item_string(wb, "versions");

    if(options & CONTEXTS_V2_AGENTS)
        buffer_json_add_array_item_string(wb, "agents");

    if(options & CONTEXTS_V2_AGENTS_INFO)
        buffer_json_add_array_item_string(wb, "agents-info");

    if(options & CONTEXTS_V2_NODES)
        buffer_json_add_array_item_string(wb, "nodes");

    if(options & CONTEXTS_V2_NODES_INFO)
        buffer_json_add_array_item_string(wb, "nodes-info");

    if(options & CONTEXTS_V2_NODES_INSTANCES)
        buffer_json_add_array_item_string(wb, "nodes-instances");

    if(options & CONTEXTS_V2_CONTEXTS)
        buffer_json_add_array_item_string(wb, "contexts");

    if(options & CONTEXTS_V2_SEARCH)
        buffer_json_add_array_item_string(wb, "search");
}

void buffer_json_query_timings(BUFFER *wb, const char *key, struct query_timings *timings) {
    timings->finished_ut = now_monotonic_usec();
    if(!timings->executed_ut)
        timings->executed_ut = timings->finished_ut;
    if(!timings->preprocessed_ut)
        timings->preprocessed_ut = timings->received_ut;
    buffer_json_member_add_object(wb, key);
    buffer_json_member_add_double(wb, "prep_ms", (NETDATA_DOUBLE)(timings->preprocessed_ut - timings->received_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "query_ms", (NETDATA_DOUBLE)(timings->executed_ut - timings->preprocessed_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "output_ms", (NETDATA_DOUBLE)(timings->finished_ut - timings->executed_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "total_ms", (NETDATA_DOUBLE)(timings->finished_ut - timings->received_ut) / USEC_PER_MS);
    buffer_json_member_add_double(wb, "cloud_ms", (NETDATA_DOUBLE)(timings->finished_ut - timings->received_ut) / USEC_PER_MS);
    buffer_json_object_close(wb);
}

void buffer_json_agents_array_v2(BUFFER *wb, struct query_timings *timings, time_t now_s, bool info) {
    if(!now_s)
        now_s = now_realtime_sec();

    buffer_json_member_add_array(wb, "agents");
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "mg", localhost->machine_guid);
    buffer_json_member_add_uuid(wb, "nd", localhost->node_id);
    buffer_json_member_add_string(wb, "nm", rrdhost_hostname(localhost));
    buffer_json_member_add_time_t(wb, "now", now_s);
    buffer_json_member_add_uint64(wb, "ai", 0);

    if(info) {
        buffer_json_member_add_string(wb, "v", string2str(localhost->program_version));

        buffer_json_member_add_object(wb, "cloud");
        {
            size_t id = cloud_connection_id();
            CLOUD_STATUS status = cloud_status();
            time_t last_change = cloud_last_change();
            time_t next_connect = cloud_next_connection_attempt();
            buffer_json_member_add_uint64(wb, "id", id);
            buffer_json_member_add_string(wb, "status", cloud_status_to_string(status));
            buffer_json_member_add_time_t(wb, "since", last_change);
            buffer_json_member_add_time_t(wb, "age", now_s - last_change);

            if (status != CLOUD_STATUS_ONLINE)
                buffer_json_member_add_string(wb, "reason", cloud_offline_reason());

            if (status == CLOUD_STATUS_OFFLINE && next_connect > now_s) {
                buffer_json_member_add_time_t(wb, "next_check", next_connect);
                buffer_json_member_add_time_t(wb, "next_in", next_connect - now_s);
            }

            if (status != CLOUD_STATUS_DISABLED && cloud_base_url())
                buffer_json_member_add_string(wb, "url", cloud_base_url());
        }
        buffer_json_object_close(wb); // cloud

        buffer_json_member_add_array(wb, "db_size");
        for (size_t tier = 0; tier < storage_tiers; tier++) {
            STORAGE_ENGINE *eng = localhost->db[tier].eng;
            if (!eng) continue;

            size_t max = storage_engine_disk_space_max(eng->backend, localhost->db[tier].instance);
            size_t used = storage_engine_disk_space_used(eng->backend, localhost->db[tier].instance);
            NETDATA_DOUBLE percent;
            if (used && max)
                percent = (NETDATA_DOUBLE) used * 100.0 / (NETDATA_DOUBLE) max;
            else
                percent = 0.0;

            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_uint64(wb, "tier", tier);
            buffer_json_member_add_uint64(wb, "disk_used", used);
            buffer_json_member_add_uint64(wb, "disk_max", max);
            buffer_json_member_add_double(wb, "disk_percent", percent);
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb); // db_size
    }

    if(timings)
        buffer_json_query_timings(wb, "timings", timings);

    buffer_json_object_close(wb);
    buffer_json_array_close(wb);
}

void buffer_json_cloud_timings(BUFFER *wb, const char *key, struct query_timings *timings) {
    if(!timings->finished_ut)
        timings->finished_ut = now_monotonic_usec();

    buffer_json_member_add_object(wb, key);
    buffer_json_member_add_double(wb, "routing_ms", 0.0);
    buffer_json_member_add_double(wb, "node_max_ms", 0.0);
    buffer_json_member_add_double(wb, "total_ms", (NETDATA_DOUBLE)(timings->finished_ut - timings->received_ut) / USEC_PER_MS);
    buffer_json_object_close(wb);
}

static void functions_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rrdfunction_to_json_v2 *t = value;

    // it is initialized with a static reference - we need to mallocz() the array
    size_t *v = t->node_ids;
    t->node_ids = mallocz(sizeof(size_t));
    *t->node_ids = *v;
    t->size = 1;
    t->used = 1;
}

static bool functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct rrdfunction_to_json_v2 *t = old_value, *n = new_value;
    size_t *v = n->node_ids;

    if(t->used >= t->size) {
        t->node_ids = reallocz(t->node_ids, t->size * 2 * sizeof(size_t));
        t->size *= 2;
    }

    t->node_ids[t->used++] = *v;

    return true;
}

static void functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rrdfunction_to_json_v2 *t = value;
    freez(t->node_ids);
}

static bool contexts_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct rrdcontext_to_json_v2_entry *o = old_value;
    struct rrdcontext_to_json_v2_entry *n = new_value;

    o->count++;

    if(o->family != n->family) {
        STRING *m = string_2way_merge(o->family, n->family);
        string_freez(o->family);
        o->family = m;
    }

    if(o->priority != n->priority) {
        if((o->flags & RRD_FLAG_COLLECTED) && !(n->flags & RRD_FLAG_COLLECTED))
            // keep o
            ;
        else if(!(o->flags & RRD_FLAG_COLLECTED) && (n->flags & RRD_FLAG_COLLECTED))
            // keep n
            o->priority = n->priority;
        else
            // keep the min
            o->priority = MIN(o->priority, n->priority);
    }

    if(o->first_time_s && n->first_time_s)
        o->first_time_s = MIN(o->first_time_s, n->first_time_s);
    else if(!o->first_time_s)
        o->first_time_s = n->first_time_s;

    if(o->last_time_s && n->last_time_s)
        o->last_time_s = MAX(o->last_time_s, n->last_time_s);
    else if(!o->last_time_s)
        o->last_time_s = n->last_time_s;

    o->flags |= n->flags;
    o->match = MIN(o->match, n->match);

    string_freez(n->family);

    return true;
}

static void contexts_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rrdcontext_to_json_v2_entry *z = value;
    string_freez(z->family);
}

int rrdcontext_to_json_v2(BUFFER *wb, struct api_v2_contexts_request *req, CONTEXTS_V2_OPTIONS options) {
    int resp = HTTP_RESP_OK;

    if(options & CONTEXTS_V2_SEARCH)
        options |= CONTEXTS_V2_CONTEXTS;

    if(options & (CONTEXTS_V2_NODES_INFO | CONTEXTS_V2_NODES_INSTANCES))
        options |= CONTEXTS_V2_NODES;

    if(options & (CONTEXTS_V2_AGENTS_INFO))
        options |= CONTEXTS_V2_AGENTS;

    struct rrdcontext_to_json_v2_data ctl = {
            .wb = wb,
            .request = req,
            .ctx = NULL,
            .options = options,
            .versions = { 0 },
            .nodes.scope_pattern = string_to_simple_pattern(req->scope_nodes),
            .nodes.pattern = string_to_simple_pattern(req->nodes),
            .contexts.pattern = string_to_simple_pattern(req->contexts),
            .contexts.scope_pattern = string_to_simple_pattern(req->scope_contexts),
            .q.pattern = string_to_simple_pattern_nocase(req->q),
            .window = {
                    .enabled = false,
                    .relative = false,
                    .after = req->after,
                    .before = req->before,
            },
            .timings = {
                    .received_ut = now_monotonic_usec(),
            }
    };

    if(options & CONTEXTS_V2_CONTEXTS) {
        ctl.ctx = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL,
                sizeof(struct rrdcontext_to_json_v2_entry));

        dictionary_register_conflict_callback(ctl.ctx, contexts_conflict_callback, NULL);
        dictionary_register_delete_callback(ctl.ctx, contexts_delete_callback, NULL);
    }

    if(options & CONTEXTS_V2_FUNCTIONS) {
        ctl.functions.dict = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL,
                sizeof(struct rrdfunction_to_json_v2));

        dictionary_register_insert_callback(ctl.functions.dict, functions_insert_callback, NULL);
        dictionary_register_conflict_callback(ctl.functions.dict, functions_conflict_callback, NULL);
        dictionary_register_delete_callback(ctl.functions.dict, functions_delete_callback, NULL);
    }

    if(req->after || req->before) {
        ctl.window.relative = rrdr_relative_window_to_absolute(&ctl.window.after, &ctl.window.before, &ctl.now);
        ctl.window.enabled = true;
    }
    else
        ctl.now = now_realtime_sec();

    buffer_json_initialize(wb, "\"", "\"", 0,
                           true, (options & CONTEXTS_V2_MINIFY) && !(options & CONTEXTS_V2_DEBUG));

    buffer_json_member_add_uint64(wb, "api", 2);

    if(options & CONTEXTS_V2_DEBUG) {
        buffer_json_member_add_object(wb, "request");

        buffer_json_member_add_object(wb, "scope");
        buffer_json_member_add_string(wb, "scope_nodes", req->scope_nodes);
        buffer_json_member_add_string(wb, "scope_contexts", req->scope_contexts);
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "selectors");
        buffer_json_member_add_string(wb, "nodes", req->nodes);
        buffer_json_member_add_string(wb, "contexts", req->contexts);
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "filters");
        buffer_json_member_add_string(wb, "q", req->q);
        buffer_json_member_add_time_t(wb, "after", req->after);
        buffer_json_member_add_time_t(wb, "before", req->before);
        buffer_json_object_close(wb);

        buffer_json_member_add_array(wb, "options");
        buffer_json_contexts_v2_options_to_array(wb, options);
        buffer_json_array_close(wb);

        buffer_json_object_close(wb);
    }

    if(options & CONTEXTS_V2_NODES)
        buffer_json_member_add_array(wb, "nodes");

    ssize_t ret = query_scope_foreach_host(ctl.nodes.scope_pattern, ctl.nodes.pattern,
                             rrdcontext_to_json_v2_add_host, &ctl,
                             &ctl.versions, ctl.q.host_node_id_str);

    if(unlikely(ret < 0)) {
        buffer_flush(wb);

        if(ret == -2) {
            buffer_strcat(wb, "query timeout");
            resp = HTTP_RESP_GATEWAY_TIMEOUT;
        }
        else {
            buffer_strcat(wb, "query interrupted");
            resp = HTTP_RESP_BACKEND_FETCH_FAILED;
        }
        goto cleanup;
    }

    if(options & CONTEXTS_V2_NODES)
        buffer_json_array_close(wb);

    ctl.timings.executed_ut = now_monotonic_usec();

    if(options & CONTEXTS_V2_FUNCTIONS) {
        buffer_json_member_add_array(wb, "functions");
        {
            struct rrdfunction_to_json_v2 *t;
            dfe_start_read(ctl.functions.dict, t) {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "name", t_dfe.name);
                buffer_json_member_add_string(wb, "help", string2str(t->help));
                buffer_json_member_add_array(wb, "ni");
                for(size_t i = 0; i < t->used ;i++)
                    buffer_json_add_array_item_uint64(wb, t->node_ids[i]);
                buffer_json_array_close(wb);
                buffer_json_object_close(wb);
            }
            dfe_done(t);
        }
        buffer_json_array_close(wb);
    }

    if(options & CONTEXTS_V2_CONTEXTS) {
        buffer_json_member_add_object(wb, "contexts");
        {
            struct rrdcontext_to_json_v2_entry *z;
            dfe_start_read(ctl.ctx, z) {
                bool collected = z->flags & RRD_FLAG_COLLECTED;

                buffer_json_member_add_object(wb, string2str(z->id));
                {
                    buffer_json_member_add_string(wb, "family", string2str(z->family));
                    buffer_json_member_add_uint64(wb, "priority", z->priority);
                    buffer_json_member_add_time_t(wb, "first_entry", z->first_time_s);
                    buffer_json_member_add_time_t(wb, "last_entry", collected ? ctl.now : z->last_time_s);
                    buffer_json_member_add_boolean(wb, "live", collected);
                    if (options & CONTEXTS_V2_SEARCH)
                        buffer_json_member_add_string(wb, "match", fts_match_to_string(z->match));
                }
                buffer_json_object_close(wb);
            }
            dfe_done(z);
        }
        buffer_json_object_close(wb); // contexts
    }

    if(options & CONTEXTS_V2_SEARCH) {
        buffer_json_member_add_object(wb, "searches");
        {
            buffer_json_member_add_uint64(wb, "strings", ctl.q.fts.string_searches);
            buffer_json_member_add_uint64(wb, "char", ctl.q.fts.char_searches);
            buffer_json_member_add_uint64(wb, "total", ctl.q.fts.searches);
        }
        buffer_json_object_close(wb);
    }

    if(options & (CONTEXTS_V2_VERSIONS))
        version_hashes_api_v2(wb, &ctl.versions);

    if(options & CONTEXTS_V2_AGENTS)
        buffer_json_agents_array_v2(wb, &ctl.timings, ctl.now, options & (CONTEXTS_V2_AGENTS_INFO));

    buffer_json_cloud_timings(wb, "timings", &ctl.timings);

    buffer_json_finalize(wb);

cleanup:
    dictionary_destroy(ctl.functions.dict);
    dictionary_destroy(ctl.ctx);
    simple_pattern_free(ctl.nodes.scope_pattern);
    simple_pattern_free(ctl.nodes.pattern);
    simple_pattern_free(ctl.contexts.pattern);
    simple_pattern_free(ctl.contexts.scope_pattern);
    simple_pattern_free(ctl.q.pattern);

    return resp;
}

int alerts_to_json_v2(BUFFER *wb, struct api_v2_alerts_request *req, CONTEXTS_V2_OPTIONS options)
{
    int resp = HTTP_RESP_OK;

//    ALERT_OPTIONS alert_options = req->options;
    struct rrdcontext_to_json_v2_data ctl = {
        .wb = wb,
        .alerts_request = req,
        .ctx = NULL,
        .options = options,
        .versions = { 0 },
        .nodes.scope_pattern = string_to_simple_pattern(req->scope_nodes),
        .nodes.pattern = string_to_simple_pattern(req->nodes),
        .contexts.pattern = string_to_simple_pattern(req->contexts),
        .contexts.scope_pattern = string_to_simple_pattern(req->scope_contexts),
        .timings = {
            .received_ut = now_monotonic_usec(),
        }
    };

    if(options & CONTEXTS_V2_CONTEXTS)
    {
        ctl.ctx = dictionary_create_advanced(
            DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL,
            sizeof(struct rrdcontext_to_json_v2_entry));

        dictionary_register_delete_callback(ctl.ctx, contexts_delete_callback, NULL);
    }

    time_t now_s = now_realtime_sec();
    buffer_json_member_add_uint64(wb, "api", 2);

    {
        buffer_json_member_add_object(wb, "request");

        buffer_json_member_add_object(wb, "scope");
        buffer_json_member_add_string(wb, "scope_nodes", req->scope_nodes);
        buffer_json_member_add_string(wb, "scope_contexts", req->scope_contexts);
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "selectors");
        buffer_json_member_add_string(wb, "nodes", req->nodes);
        buffer_json_member_add_string(wb, "contexts", req->contexts);
        buffer_json_object_close(wb);

        buffer_json_member_add_array(wb, "options");
        buffer_json_contexts_v2_options_to_array(wb, options);
        buffer_json_array_close(wb);

        buffer_json_object_close(wb);
    }

    // Alert configuration
    buffer_json_member_add_array(wb, "alerts");

    ssize_t ret = query_scope_foreach_host(ctl.nodes.scope_pattern, ctl.nodes.pattern,
            alert_to_json_v2_add_alert, &ctl, &ctl.versions, NULL);

    if(unlikely(ret < 0)) {
        buffer_flush(wb);

        if(ret == -2) {
            buffer_strcat(wb, "query timeout");
            resp = HTTP_RESP_GATEWAY_TIMEOUT;
        }
        else {
            buffer_strcat(wb, "query interrupted");
            resp = HTTP_RESP_BACKEND_FETCH_FAILED;
        }
        goto cleanup;
    }

    buffer_json_array_close(wb); // alerts

    buffer_json_member_add_array(wb, "nodes");

    ret = query_scope_foreach_host(ctl.nodes.scope_pattern, ctl.nodes.pattern,
                                           alert_to_json_v2_add_host, &ctl,
                                           &ctl.versions, NULL);

    if(unlikely(ret < 0)) {
        buffer_flush(wb);

        if(ret == -2) {
            buffer_strcat(wb, "query timeout");
            resp = HTTP_RESP_GATEWAY_TIMEOUT;
        }
        else {
            buffer_strcat(wb, "query interrupted");
            resp = HTTP_RESP_BACKEND_FETCH_FAILED;
        }
        goto cleanup;
    }

    buffer_json_array_close(wb);        // Nodes

    ctl.timings.executed_ut = now_monotonic_usec();
    version_hashes_api_v2(wb, &ctl.versions);

    buffer_json_agents_array_v2(wb, &ctl.timings, now_s, false);
    buffer_json_cloud_timings(wb, "timings", &ctl.timings);
    buffer_json_finalize(wb);

    JudyHSFreeArray(&ctl.alerts.JudyHS, PJE0);

cleanup:
//    dictionary_destroy(ctl.ctx);
    simple_pattern_free(ctl.nodes.scope_pattern);
    simple_pattern_free(ctl.nodes.pattern);
    simple_pattern_free(ctl.contexts.pattern);
    simple_pattern_free(ctl.contexts.scope_pattern);
    simple_pattern_free(req->config_hash_pattern);
    simple_pattern_free(req->alert_name_pattern);

    return resp;
}
