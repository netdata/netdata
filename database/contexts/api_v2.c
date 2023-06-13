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
    BUFFER *wb;
    struct api_v2_contexts_request *request;
    DICTIONARY *ctx;

    CONTEXTS_V2_OPTIONS options;
    struct query_versions versions;

    struct {
        SIMPLE_PATTERN *scope_pattern;
        SIMPLE_PATTERN *pattern;
        size_t ni;
    } nodes;

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

    struct query_timings timings;
};

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

        if(unlikely(full_text_search_string(&ctl->q.fts, q, ri->id)) ||
           (ri->name != ri->id && full_text_search_string(&ctl->q.fts, q, ri->name))) {
            matched = FTS_MATCHED_INSTANCE;
            break;
        }

        RRDMETRIC *rm;
        dfe_start_read(ri->rrdmetrics, rm) {
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

    FTS_MATCH match = ctl->q.host_match;
    if((ctl->options & CONTEXTS_V2_SEARCH) && ctl->q.pattern) {
        match = rrdcontext_to_json_v2_full_text_search(ctl, rc, ctl->q.pattern);

        if(match == FTS_MATCHED_NONE)
            return 0;
    }

    struct rrdcontext_to_json_v2_entry t = {
            .count = 0,
            .id = rc->id,
            .family = string_dup(rc->family),
            .priority = rc->priority,
            .first_time_s = rc->first_time_s,
            .last_time_s = rc->last_time_s,
            .flags = rc->flags,
            .match = match,
    }, *z = dictionary_set(ctl->ctx, string2str(rc->id), &t, sizeof(t));

    if(!z->count) {
        // we just added this
        z->count = 1;
    }
    else {
        // it is already in there
        z->count++;
        z->flags |= rc->flags;

        if(z->priority > rc->priority)
            z->priority = rc->priority;

        if(z->first_time_s > rc->first_time_s)
            z->first_time_s = rc->first_time_s;

        if(z->last_time_s < rc->last_time_s)
            z->last_time_s = rc->last_time_s;

        if(z->family != rc->family) {
            z->family = string_2way_merge(z->family, rc->family);
        }
    }

    return 1;
}

void buffer_json_node_add_v2(BUFFER *wb, RRDHOST *host, size_t ni, usec_t duration_ut) {
    buffer_json_member_add_string(wb, "mg", host->machine_guid);
    if(host->node_id)
        buffer_json_member_add_uuid(wb, "nd", host->node_id);
    buffer_json_member_add_string(wb, "nm", rrdhost_hostname(host));
    buffer_json_member_add_uint64(wb, "ni", ni);
    buffer_json_member_add_object(wb, "st");
    buffer_json_member_add_uint64(wb, "ai", 0);
    buffer_json_member_add_uint64(wb, "code", 200);
    buffer_json_member_add_string(wb, "msg", "");
    if(duration_ut)
        buffer_json_member_add_double(wb, "ms", (NETDATA_DOUBLE)duration_ut / 1000.0);
    buffer_json_object_close(wb);
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

    buffer_json_member_add_uint64(wb, "id", s->stream.id);
    buffer_json_member_add_uint64(wb, "hops", s->stream.hops);
    buffer_json_member_add_string(wb, "status", rrdhost_streaming_status_to_string(s->stream.status));
    buffer_json_member_add_time_t(wb, "since", s->stream.since);
    buffer_json_member_add_time_t(wb, "age", s->now - s->stream.since);

    if(s->stream.status == RRDHOST_STREAM_STATUS_OFFLINE)
        buffer_json_member_add_string(wb, "reason", s->stream.reason);

    if(s->stream.status == RRDHOST_STREAM_STATUS_REPLICATING || s->stream.status == RRDHOST_STREAM_STATUS_ONLINE) {

        if(s->stream.status == RRDHOST_STREAM_STATUS_REPLICATING) {
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
                {

                    if (d->ssl) {
                        snprintfz(buf, 1024, "%s:SSL", string2str(d->destination));
                        buffer_json_member_add_string(wb, "destination", buf);
                    }
                    else
                        buffer_json_member_add_string(wb, "destination", string2str(d->destination));

                    buffer_json_member_add_time_t(wb, "last_check", d->last_attempt);
                    buffer_json_member_add_time_t(wb, "age", s->now - d->last_attempt);
                    buffer_json_member_add_string(wb, "last_error", d->last_error);
                    buffer_json_member_add_string(wb, "last_handshake",
                                                  stream_handshake_error_to_string(d->last_handshake));
                    buffer_json_member_add_time_t(wb, "next_check", d->postpone_reconnection_until);
                    buffer_json_member_add_time_t(wb, "next_in",
                                                  (d->postpone_reconnection_until > s->now) ?
                                                  d->postpone_reconnection_until - s->now : 0);
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
    struct capability *capa = capas;
    while(capa->name != NULL) {
        if(!capa->enabled)
            continue;

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, capa->name);
        buffer_json_add_array_item_uint64(wb, capa->version);
        buffer_json_array_close(wb);
        capa++;
    }
    buffer_json_array_close(wb);
    freez(capas);
}

static ssize_t rrdcontext_to_json_v2_add_host(void *data, RRDHOST *host, bool queryable_host) {
    if(!queryable_host || !host->rrdctx.contexts)
        // the host matches the 'scope_host' but does not match the 'host' patterns
        // or the host does not have any contexts
        return 0;

    struct rrdcontext_to_json_v2_data *ctl = data;
    BUFFER *wb = ctl->wb;

    if(ctl->request->timeout_ms && now_monotonic_usec() > ctl->timings.received_ut + ctl->request->timeout_ms * USEC_PER_MS)
        // timed out
        return -2;

    if(ctl->request->interrupt_callback && ctl->request->interrupt_callback(ctl->request->interrupt_callback_data))
        // interrupted
        return -1;

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

        if(added == -1)
            return -1;

        if(added)
            host_matched = true;
    }

    if(host_matched && (ctl->options & (CONTEXTS_V2_NODES | CONTEXTS_V2_NODES_DETAILED | CONTEXTS_V2_DEBUG))) {
        buffer_json_add_array_item_object(wb);
        buffer_json_node_add_v2(wb, host, ctl->nodes.ni++, 0);

        if(ctl->options & CONTEXTS_V2_NODES_DETAILED) {
            buffer_json_member_add_string(wb, "v", rrdhost_program_version(host));

            host_labels2json(host, wb, "labels");

            if(host->system_info) {
                buffer_json_member_add_object(wb, "hw");
                {
                    buffer_json_member_add_string_or_empty(wb, "architecture", host->system_info->architecture);
                    buffer_json_member_add_string_or_empty(wb, "cpuFrequency", host->system_info->host_cpu_freq);
                    buffer_json_member_add_string_or_empty(wb, "cpus", host->system_info->host_cores);
                    buffer_json_member_add_string_or_empty(wb, "memory", host->system_info->host_ram_total);
                    buffer_json_member_add_string_or_empty(wb, "diskSpace", host->system_info->host_disk_space);
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

            time_t now = now_realtime_sec();
            RRDHOST_STATUS s;
            rrdhost_status(host, now, &s);
            buffer_json_member_add_object(wb, "db");
            {
                buffer_json_member_add_string(wb, "status", rrdhost_db_status_to_string(s.db.status));
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
            if(s.ml.status == RRDHOST_ML_STATUS_RUNNING) {
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

            buffer_json_member_add_object(wb, "health");
            {
                buffer_json_member_add_string(wb, "status", rrdhost_health_status_to_string(s.health.status));
                if(s.health.status == RRDHOST_HEALTH_STATUS_RUNNING) {
                    buffer_json_member_add_object(wb, "alerts");
                    {
                        buffer_json_member_add_uint64(wb, "critical", s.health.alerts.critical);
                        buffer_json_member_add_uint64(wb, "warning", s.health.alerts.warning);
                        buffer_json_member_add_uint64(wb, "clear", s.health.alerts.clear);
                        buffer_json_member_add_uint64(wb, "undefined", s.health.alerts.undefined);
                        buffer_json_member_add_uint64(wb, "uninitialized", s.health.alerts.uninitialized);
                    }
                    buffer_json_object_close(wb); // alerts
                }
            }
            buffer_json_object_close(wb); // health

        }

        buffer_json_object_close(wb);
    }

    return host_matched ? 1 : 0;
}

static void buffer_json_contexts_v2_options_to_array(BUFFER *wb, CONTEXTS_V2_OPTIONS options) {
    if(options & CONTEXTS_V2_DEBUG)
        buffer_json_add_array_item_string(wb, "debug");

    if(options & (CONTEXTS_V2_NODES | CONTEXTS_V2_NODES_DETAILED))
        buffer_json_add_array_item_string(wb, "nodes");

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

void buffer_json_agents_array_v2(BUFFER *wb, struct query_timings *timings, time_t now_s) {
    if(!now_s)
        now_s = now_realtime_sec();

    buffer_json_member_add_array(wb, "agents");
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "mg", localhost->machine_guid);
    buffer_json_member_add_uuid(wb, "nd", localhost->node_id);
    buffer_json_member_add_string(wb, "nm", rrdhost_hostname(localhost));
    buffer_json_member_add_time_t(wb, "now", now_s);
    buffer_json_member_add_uint64(wb, "ai", 0);
    buffer_json_member_add_string(wb, "v", string2str(localhost->program_version));
    agent_capabilities_to_json(wb, localhost, "cp");

    if(timings)
        buffer_json_query_timings(wb, "timings", timings);

    buffer_json_object_close(wb);
    buffer_json_array_close(wb);
}

void buffer_json_cloud_timings(BUFFER *wb, const char *key, struct query_timings *timings) {
    buffer_json_member_add_object(wb, key);
    buffer_json_member_add_double(wb, "routing_ms", 0.0);
    buffer_json_member_add_double(wb, "node_max_ms", 0.0);
    buffer_json_member_add_double(wb, "total_ms", (NETDATA_DOUBLE)(timings->finished_ut - timings->received_ut) / USEC_PER_MS);
    buffer_json_object_close(wb);
}

void contexts_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rrdcontext_to_json_v2_entry *z = value;
    string_freez(z->family);
}

int rrdcontext_to_json_v2(BUFFER *wb, struct api_v2_contexts_request *req, CONTEXTS_V2_OPTIONS options) {
    int resp = HTTP_RESP_OK;

    if(options & CONTEXTS_V2_SEARCH)
        options |= CONTEXTS_V2_CONTEXTS;

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
            .timings = {
                    .received_ut = now_monotonic_usec(),
            }
    };

    if(options & CONTEXTS_V2_CONTEXTS) {
        ctl.ctx = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL,
                sizeof(struct rrdcontext_to_json_v2_entry));

        dictionary_register_delete_callback(ctl.ctx, contexts_delete_callback, NULL);
    }

    time_t now_s = now_realtime_sec();
    buffer_json_initialize(wb, "\"", "\"", 0, true, false);
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

        buffer_json_member_add_string(wb, "q", req->q);
        buffer_json_member_add_array(wb, "options");
        buffer_json_contexts_v2_options_to_array(wb, options);
        buffer_json_array_close(wb);

        buffer_json_object_close(wb);
    }

    if(options & (CONTEXTS_V2_NODES | CONTEXTS_V2_NODES_DETAILED | CONTEXTS_V2_DEBUG))
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

    if(options & (CONTEXTS_V2_NODES | CONTEXTS_V2_NODES_DETAILED | CONTEXTS_V2_DEBUG))
        buffer_json_array_close(wb);

    ctl.timings.executed_ut = now_monotonic_usec();
    version_hashes_api_v2(wb, &ctl.versions);

    if(options & CONTEXTS_V2_CONTEXTS) {
        buffer_json_member_add_object(wb, "contexts");
        struct rrdcontext_to_json_v2_entry *z;
        dfe_start_read(ctl.ctx, z){
            bool collected = z->flags & RRD_FLAG_COLLECTED;

            buffer_json_member_add_object(wb, string2str(z->id));
            {
                buffer_json_member_add_string(wb, "family", string2str(z->family));
                buffer_json_member_add_uint64(wb, "priority", z->priority);
                buffer_json_member_add_time_t(wb, "first_entry", z->first_time_s);
                buffer_json_member_add_time_t(wb, "last_entry", collected ? now_s : z->last_time_s);
                buffer_json_member_add_boolean(wb, "live", collected);
                if (options & CONTEXTS_V2_SEARCH)
                    buffer_json_member_add_string(wb, "match", fts_match_to_string(z->match));
            }
            buffer_json_object_close(wb);
        }
        dfe_done(z);
        buffer_json_object_close(wb); // contexts
    }

    if(options & CONTEXTS_V2_SEARCH) {
        buffer_json_member_add_object(wb, "searches");
        buffer_json_member_add_uint64(wb, "strings", ctl.q.fts.string_searches);
        buffer_json_member_add_uint64(wb, "char", ctl.q.fts.char_searches);
        buffer_json_member_add_uint64(wb, "total", ctl.q.fts.searches);
        buffer_json_object_close(wb);
    }

    buffer_json_agents_array_v2(wb, &ctl.timings, now_s);
    buffer_json_cloud_timings(wb, "timings", &ctl.timings);
    buffer_json_finalize(wb);

cleanup:
    dictionary_destroy(ctl.ctx);
    simple_pattern_free(ctl.nodes.scope_pattern);
    simple_pattern_free(ctl.nodes.pattern);
    simple_pattern_free(ctl.contexts.pattern);
    simple_pattern_free(ctl.contexts.scope_pattern);
    simple_pattern_free(ctl.q.pattern);

    return resp;
}

