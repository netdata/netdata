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

struct function_v2_entry {
    size_t size;
    size_t used;
    size_t *node_ids;
    STRING *help;
};

struct context_v2_entry {
    size_t count;
    STRING *id;
    STRING *family;
    uint32_t priority;
    time_t first_time_s;
    time_t last_time_s;
    RRD_FLAGS flags;
    FTS_MATCH match;
};

struct alert_v2_entry {
    RRDCALC *tmp;

    STRING *name;

    size_t ati;

    size_t critical;
    size_t warning;
    size_t clear;
    size_t error;

    size_t instances;
    DICTIONARY *nodes;
    DICTIONARY *configs;
};

struct alert_config_v2_entry {
    RRDCALC *tmp;

    size_t ati;
    size_t aci;
    size_t alerts_using_this;
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

struct contexts_v2_node {
    size_t ni;
    RRDHOST *host;
};

struct rrdcontext_to_json_v2_data {
    time_t now;

    BUFFER *wb;
    struct api_v2_contexts_request *request;

    CONTEXTS_V2_MODE mode;
    CONTEXTS_V2_OPTIONS options;
    struct query_versions versions;

    struct {
        SIMPLE_PATTERN *scope_pattern;
        SIMPLE_PATTERN *pattern;
        size_t ni;
        DICTIONARY *dict; // the result set
    } nodes;

    struct {
        SIMPLE_PATTERN *scope_pattern;
        SIMPLE_PATTERN *pattern;
        size_t ci;
        DICTIONARY *dict; // the result set
    } contexts;

    struct {
        SIMPLE_PATTERN *alert_name_pattern;
        time_t alarm_id_filter;

        size_t ati;
        size_t aii;
        size_t aci;

        DICTIONARY *alerts;
        DICTIONARY *alert_instances;
        DICTIONARY *alert_configs;
    } alerts;

    struct {
        FTS_MATCH host_match;
        char host_node_id_str[UUID_STR_LEN];
        SIMPLE_PATTERN *pattern;
        FTS_INDEX fts;
    } q;

    struct {
        DICTIONARY *dict; // the result set
    } functions;

    struct {
        bool enabled;
        bool relative;
        time_t after;
        time_t before;
    } window;

    struct query_timings timings;
};

static void alerts_v2_add(struct alert_v2_entry *t, RRDCALC *rc) {
    t->instances++;

    switch(rc->status) {
        case RRDCALC_STATUS_CRITICAL:
            t->critical++;
            break;

        case RRDCALC_STATUS_WARNING:
            t->warning++;
            break;

        case RRDCALC_STATUS_CLEAR:
            t->clear++;
            break;

        case RRDCALC_STATUS_REMOVED:
        case RRDCALC_STATUS_UNINITIALIZED:
            break;

        case RRDCALC_STATUS_UNDEFINED:
        default:
            if(!netdata_double_isnumber(rc->value))
                t->error++;

            break;
    }

    dictionary_set(t->nodes, rc->rrdset->rrdhost->machine_guid, NULL, 0);

    char key[UUID_STR_LEN + 1];
    uuid_unparse_lower(rc->config_hash_id, key);
    dictionary_set(t->configs, key, NULL, 0);
}

static void alerts_v2_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct rrdcontext_to_json_v2_data *ctl = data;
    struct alert_v2_entry *t = value;
    RRDCALC *rc = t->tmp;
    t->name = rc->name;
    t->ati = ctl->alerts.ati++;

    t->nodes = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_VALUE_LINK_DONT_CLONE|DICT_OPTION_NAME_LINK_DONT_CLONE);
    t->configs = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_VALUE_LINK_DONT_CLONE|DICT_OPTION_NAME_LINK_DONT_CLONE);

    alerts_v2_add(t, rc);
}

static bool alerts_v2_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct alert_v2_entry *t = old_value, *n = new_value;
    RRDCALC *rc = n->tmp;
    alerts_v2_add(t, rc);
    return true;
}

static void alerts_v2_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct alert_v2_entry *t = value;
    dictionary_destroy(t->nodes);
    dictionary_destroy(t->configs);
}

static void alert_configs_v2_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct rrdcontext_to_json_v2_data *ctl = data;
    struct alert_config_v2_entry *t = value;
    t->aci = ctl->alerts.aci++;
    t->alerts_using_this = 1;
}

static bool alert_configs_v2_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value __maybe_unused, void *new_value __maybe_unused, void *data __maybe_unused) {
    struct alert_config_v2_entry *t = old_value;
    t->alerts_using_this++;
    return false;
}

static void alert_configs_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data __maybe_unused) {
    ;
}

static void alert_instances_v2_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct rrdcontext_to_json_v2_data *ctl = data;
    struct alert_instance_v2_entry *t = value;
    RRDCALC *rc = t->tmp;

    t->chart_id = rc->rrdset->id;
    t->chart_name = rc->rrdset->name;
    t->family = rc->rrdset->family;
    t->name = rc->name;
    t->status = rc->status;
    t->flags = rc->run_flags;
    t->info = rc->info;
    t->value = rc->value;
    t->last_updated = rc->last_updated;
    t->last_status_change = rc->last_status_change;
    t->last_status_change_value = rc->last_status_change_value;
    t->host = rc->rrdset->rrdhost;
    t->alarm_id = rc->id;
    t->ni = ctl->nodes.ni;
    t->global_id = rc->ae ? rc->ae->global_id : 0;
    t->name = rc->name;
    t->aii = ctl->alerts.aii++;

    uuid_copy(t->config_hash_id, rc->config_hash_id);
    if(rc->ae)
        uuid_copy(t->last_transition_id, rc->ae->transition_id);
}

static bool alert_instances_v2_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value __maybe_unused, void *new_value __maybe_unused, void *data __maybe_unused) {
    internal_fatal(true, "This should never happen!");
    return true;
}

static void alert_instances_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data __maybe_unused) {
    ;
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

static bool rrdcontext_matches_alert(struct rrdcontext_to_json_v2_data *ctl, RRDCONTEXT *rc) {
    size_t matches = 0;
    RRDINSTANCE *ri;
    dfe_start_read(rc->rrdinstances, ri) {
        if(ri->rrdset) {
            RRDSET *st = ri->rrdset;
            netdata_rwlock_rdlock(&st->alerts.rwlock);
            for (RRDCALC *rcl = st->alerts.base; rcl; rcl = rcl->next) {
                if(ctl->alerts.alert_name_pattern && !simple_pattern_matches_string(ctl->alerts.alert_name_pattern, rcl->name))
                    continue;

                if(ctl->alerts.alarm_id_filter && ctl->alerts.alarm_id_filter != rcl->id)
                    continue;

                size_t m = ctl->request->alerts.status & CONTEXTS_V2_ALERT_STATUSES ? 0 : 1;

                if (!m) {
                    if ((ctl->request->alerts.status & CONTEXT_V2_ALERT_UNINITIALIZED) &&
                        rcl->status == RRDCALC_STATUS_UNINITIALIZED)
                        m++;

                    if ((ctl->request->alerts.status & CONTEXT_V2_ALERT_UNDEFINED) &&
                        rcl->status == RRDCALC_STATUS_UNDEFINED)
                        m++;

                    if ((ctl->request->alerts.status & CONTEXT_V2_ALERT_CLEAR) &&
                        rcl->status == RRDCALC_STATUS_CLEAR)
                        m++;

                    if ((ctl->request->alerts.status & CONTEXT_V2_ALERT_RAISED) &&
                        rcl->status >= RRDCALC_STATUS_RAISED)
                        m++;

                    if ((ctl->request->alerts.status & CONTEXT_V2_ALERT_WARNING) &&
                        rcl->status == RRDCALC_STATUS_WARNING)
                        m++;

                    if ((ctl->request->alerts.status & CONTEXT_V2_ALERT_CRITICAL) &&
                        rcl->status == RRDCALC_STATUS_CRITICAL)
                        m++;

                    if(!m)
                        continue;
                }

                struct alert_v2_entry t = {
                        .tmp = rcl,
                };
                struct alert_v2_entry *a2e = dictionary_set(ctl->alerts.alerts, string2str(rcl->name), &t,
                                                            sizeof(struct alert_v2_entry));
                size_t ati = a2e->ati;
                size_t aci = 0;
                matches++;

                if (ctl->options & CONTEXT_V2_OPTION_ALERT_CONFIGURATIONS) {
                    char key[UUID_STR_LEN + 1];
                    uuid_unparse_lower(rcl->config_hash_id, key);
                    struct alert_config_v2_entry t2 = {
                            .tmp = rcl,
                            .ati = a2e->ati,
                    };
                    struct alert_config_v2_entry *a2c = dictionary_set(ctl->alerts.alert_configs, key, &t2,
                                                                       sizeof(struct alert_config_v2_entry));
                    aci = a2c->aci;
                }

                if (ctl->options & CONTEXT_V2_OPTION_ALERT_INSTANCES) {
                    char key[20 + 1];
                    snprintfz(key, 20, "%p", rcl);

                    struct alert_instance_v2_entry z = {
                            .ati = ati,
                            .aci = aci,
                            .tmp = rcl,
                    };
                    dictionary_set(ctl->alerts.alert_instances, key, &z, sizeof(z));
                }
            }
            netdata_rwlock_unlock(&st->alerts.rwlock);
        }
    }
    dfe_done(ri);

    return matches != 0;
}


static ssize_t rrdcontext_to_json_v2_add_context(void *data, RRDCONTEXT_ACQUIRED *rca, bool queryable_context __maybe_unused) {
    struct rrdcontext_to_json_v2_data *ctl = data;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    if(ctl->window.enabled && !query_matches_retention(ctl->window.after, ctl->window.before, rc->first_time_s, (rc->flags & RRD_FLAG_COLLECTED) ? ctl->now : rc->last_time_s, 0))
        return 0; // continue to next context

    FTS_MATCH match = ctl->q.host_match;
    if((ctl->mode & CONTEXTS_V2_SEARCH) && ctl->q.pattern) {
        match = rrdcontext_to_json_v2_full_text_search(ctl, rc, ctl->q.pattern);

        if(match == FTS_MATCHED_NONE)
            return 0; // continue to next context
    }

    if(ctl->mode & CONTEXTS_V2_ALERTS) {
        if(!rrdcontext_matches_alert(ctl, rc))
            return 0; // continue to next context
    }

    if(ctl->contexts.dict) {
        struct context_v2_entry t = {
                .count = 1,
                .id = rc->id,
                .family = string_dup(rc->family),
                .priority = rc->priority,
                .first_time_s = rc->first_time_s,
                .last_time_s = rc->last_time_s,
                .flags = rc->flags,
                .match = match,
        };

        dictionary_set(ctl->contexts.dict, string2str(rc->id), &t, sizeof(struct context_v2_entry));
    }

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
                buffer_json_member_add_string(wb, "reason", stream_handshake_error_to_string(s->ingest.reason));

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
            buffer_json_member_add_string(wb, "reason", stream_handshake_error_to_string(s->stream.reason));

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

                    buffer_json_member_add_time_t(wb, "since", d->since);
                    buffer_json_member_add_time_t(wb, "age", s->now - d->since);
                    buffer_json_member_add_string(wb, "last_handshake", stream_handshake_error_to_string(d->reason));
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

static void rrdcontext_to_json_v2_rrdhost(BUFFER *wb, RRDHOST *host, struct rrdcontext_to_json_v2_data *ctl, size_t node_id) {
    buffer_json_add_array_item_object(wb); // this node
    buffer_json_node_add_v2(wb, host, node_id, 0,
                            (ctl->mode & CONTEXTS_V2_AGENTS) && !(ctl->mode & CONTEXTS_V2_NODES_INSTANCES));

    if(ctl->mode & (CONTEXTS_V2_NODES_INFO | CONTEXTS_V2_NODES_INSTANCES)) {
        RRDHOST_STATUS s;
        rrdhost_status(host, ctl->now, &s);

        if (ctl->mode & (CONTEXTS_V2_NODES_INFO)) {
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

        if (ctl->mode & (CONTEXTS_V2_NODES_INSTANCES)) {
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

static ssize_t rrdcontext_to_json_v2_add_host(void *data, RRDHOST *host, bool queryable_host) {
    if(!queryable_host || !host->rrdctx.contexts)
        // the host matches the 'scope_host' but does not match the 'host' patterns
        // or the host does not have any contexts
        return 0; // continue to next host

    struct rrdcontext_to_json_v2_data *ctl = data;

    if(ctl->window.enabled && !rrdhost_matches_window(host, ctl->window.after, ctl->window.before, ctl->now))
        // the host does not have data in the requested window
        return 0; // continue to next host

    if(ctl->request->timeout_ms && now_monotonic_usec() > ctl->timings.received_ut + ctl->request->timeout_ms * USEC_PER_MS)
        // timed out
        return -2; // stop the query

    if(ctl->request->interrupt_callback && ctl->request->interrupt_callback(ctl->request->interrupt_callback_data))
        // interrupted
        return -1; // stop the query

    bool host_matched = (ctl->mode & CONTEXTS_V2_NODES);
    bool do_contexts = (ctl->mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_ALERTS));

    ctl->q.host_match = FTS_MATCHED_NONE;
    if((ctl->mode & CONTEXTS_V2_SEARCH)) {
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

    if(ctl->mode & CONTEXTS_V2_FUNCTIONS) {
        struct function_v2_entry t = {
                .used = 1,
                .size = 1,
                .node_ids = &ctl->nodes.ni,
                .help = NULL,
        };
        host_functions_to_dict(host, ctl->functions.dict, &t, sizeof(t), &t.help);
    }

    if(ctl->mode & CONTEXTS_V2_NODES) {
        struct contexts_v2_node t = {
            .ni = ctl->nodes.ni++,
            .host = host,
        };

        dictionary_set(ctl->nodes.dict, host->machine_guid, &t, sizeof(struct contexts_v2_node));
    }

    return 1;
}

static void buffer_json_contexts_v2_mode_to_array(BUFFER *wb, const char *key, CONTEXTS_V2_MODE mode) {
    buffer_json_member_add_array(wb, key);

    if(mode & CONTEXTS_V2_VERSIONS)
        buffer_json_add_array_item_string(wb, "versions");

    if(mode & CONTEXTS_V2_AGENTS)
        buffer_json_add_array_item_string(wb, "agents");

    if(mode & CONTEXTS_V2_AGENTS_INFO)
        buffer_json_add_array_item_string(wb, "agents-info");

    if(mode & CONTEXTS_V2_NODES)
        buffer_json_add_array_item_string(wb, "nodes");

    if(mode & CONTEXTS_V2_NODES_INFO)
        buffer_json_add_array_item_string(wb, "nodes-info");

    if(mode & CONTEXTS_V2_NODES_INSTANCES)
        buffer_json_add_array_item_string(wb, "nodes-instances");

    if(mode & CONTEXTS_V2_CONTEXTS)
        buffer_json_add_array_item_string(wb, "contexts");

    if(mode & CONTEXTS_V2_SEARCH)
        buffer_json_add_array_item_string(wb, "search");

    if(mode & CONTEXTS_V2_ALERTS)
        buffer_json_add_array_item_string(wb, "alerts");

    buffer_json_array_close(wb);
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
    struct function_v2_entry *t = value;

    // it is initialized with a static reference - we need to mallocz() the array
    size_t *v = t->node_ids;
    t->node_ids = mallocz(sizeof(size_t));
    *t->node_ids = *v;
    t->size = 1;
    t->used = 1;
}

static bool functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct function_v2_entry *t = old_value, *n = new_value;
    size_t *v = n->node_ids;

    if(t->used >= t->size) {
        t->node_ids = reallocz(t->node_ids, t->size * 2 * sizeof(size_t));
        t->size *= 2;
    }

    t->node_ids[t->used++] = *v;

    return true;
}

static void functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct function_v2_entry *t = value;
    freez(t->node_ids);
}

static bool contexts_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct context_v2_entry *o = old_value;
    struct context_v2_entry *n = new_value;

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
    struct context_v2_entry *z = value;
    string_freez(z->family);
}

static void rrdcontext_v2_set_transition_filter(const char *machine_guid, const char *context, time_t alarm_id, void *data) {
    struct rrdcontext_to_json_v2_data *ctl = data;

    if(machine_guid && *machine_guid) {
        if(ctl->nodes.scope_pattern)
            simple_pattern_free(ctl->nodes.scope_pattern);

        if(ctl->nodes.pattern)
            simple_pattern_free(ctl->nodes.pattern);

        ctl->nodes.scope_pattern = string_to_simple_pattern(machine_guid);
        ctl->nodes.pattern = NULL;
    }

    if(context && *context) {
        if(ctl->contexts.scope_pattern)
            simple_pattern_free(ctl->contexts.scope_pattern);

        if(ctl->contexts.pattern)
            simple_pattern_free(ctl->contexts.pattern);

        ctl->contexts.scope_pattern = string_to_simple_pattern(context);
        ctl->contexts.pattern = NULL;
    }

    ctl->alerts.alarm_id_filter = alarm_id;
}

int rrdcontext_to_json_v2(BUFFER *wb, struct api_v2_contexts_request *req, CONTEXTS_V2_MODE mode) {
    int resp = HTTP_RESP_OK;
    bool run = true;

    if(mode & CONTEXTS_V2_SEARCH)
        mode |= CONTEXTS_V2_CONTEXTS;

    if(mode & (CONTEXTS_V2_AGENTS_INFO))
        mode |= CONTEXTS_V2_AGENTS;

    if(mode & (CONTEXTS_V2_ALERTS | CONTEXTS_V2_FUNCTIONS | CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_NODES_INFO | CONTEXTS_V2_NODES_INSTANCES))
        mode |= CONTEXTS_V2_NODES;

    struct rrdcontext_to_json_v2_data ctl = {
            .wb = wb,
            .request = req,
            .mode = mode,
            .options = req->options,
            .versions = { 0 },
            .nodes.scope_pattern = string_to_simple_pattern(req->scope_nodes),
            .nodes.pattern = string_to_simple_pattern(req->nodes),
            .contexts.pattern = string_to_simple_pattern(req->contexts),
            .contexts.scope_pattern = string_to_simple_pattern(req->scope_contexts),
            .q.pattern = string_to_simple_pattern_nocase(req->q),
            .alerts.alert_name_pattern = string_to_simple_pattern(req->alerts.alert),
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

    bool debug = ctl.options & CONTEXT_V2_OPTION_DEBUG;

    if(mode & CONTEXTS_V2_NODES) {
        ctl.nodes.dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                    NULL, sizeof(struct contexts_v2_node));
    }

    if(mode & CONTEXTS_V2_CONTEXTS) {
        ctl.contexts.dict = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL,
                sizeof(struct context_v2_entry));

        dictionary_register_conflict_callback(ctl.contexts.dict, contexts_conflict_callback, &ctl);
        dictionary_register_delete_callback(ctl.contexts.dict, contexts_delete_callback, &ctl);
    }

    if(mode & CONTEXTS_V2_FUNCTIONS) {
        ctl.functions.dict = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL,
                sizeof(struct function_v2_entry));

        dictionary_register_insert_callback(ctl.functions.dict, functions_insert_callback, &ctl);
        dictionary_register_conflict_callback(ctl.functions.dict, functions_conflict_callback, &ctl);
        dictionary_register_delete_callback(ctl.functions.dict, functions_delete_callback, &ctl);
    }

    if(mode & CONTEXTS_V2_ALERTS) {
        if(req->alerts.transition) {
            ctl.options |= CONTEXT_V2_OPTION_ALERT_INSTANCES | CONTEXT_V2_OPTION_ALERT_CONFIGURATIONS | CONTEXT_V2_OPTION_ALERT_TRANSITIONS;
            run = sql_find_alert_transition(req->alerts.transition, rrdcontext_v2_set_transition_filter, &ctl);
            if(!run) {
                resp = HTTP_RESP_NOT_FOUND;
                goto cleanup;
            }
        }

        ctl.alerts.alerts = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                       NULL, sizeof(struct alert_v2_entry));

        dictionary_register_insert_callback(ctl.alerts.alerts, alerts_v2_insert_callback, &ctl);
        dictionary_register_conflict_callback(ctl.alerts.alerts, alerts_v2_conflict_callback, &ctl);
        dictionary_register_delete_callback(ctl.alerts.alerts, alerts_v2_delete_callback, &ctl);

        if(ctl.options & CONTEXT_V2_OPTION_ALERT_INSTANCES) {
            ctl.alerts.alert_instances = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                                    NULL, sizeof(struct alert_instance_v2_entry));

            dictionary_register_insert_callback(ctl.alerts.alert_instances, alert_instances_v2_insert_callback, &ctl);
            dictionary_register_conflict_callback(ctl.alerts.alert_instances, alert_instances_v2_conflict_callback, &ctl);
            dictionary_register_delete_callback(ctl.alerts.alert_instances, alert_instances_delete_callback, &ctl);
        }

        if(ctl.options & CONTEXT_V2_OPTION_ALERT_CONFIGURATIONS) {
            ctl.alerts.alert_configs = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                                  NULL, sizeof(struct alert_config_v2_entry));

            dictionary_register_insert_callback(ctl.alerts.alert_configs, alert_configs_v2_insert_callback, &ctl);
            dictionary_register_conflict_callback(ctl.alerts.alert_configs, alert_configs_v2_conflict_callback, &ctl);
            dictionary_register_delete_callback(ctl.alerts.alert_configs, alert_configs_delete_callback, &ctl);
        }
    }

    if(req->after || req->before) {
        ctl.window.relative = rrdr_relative_window_to_absolute(&ctl.window.after, &ctl.window.before, &ctl.now);
        ctl.window.enabled = true;
    }
    else
        ctl.now = now_realtime_sec();

    buffer_json_initialize(wb, "\"", "\"", 0,
                           true, (req->options & CONTEXT_V2_OPTION_MINIFY) && !(req->options & CONTEXT_V2_OPTION_DEBUG));

    buffer_json_member_add_uint64(wb, "api", 2);

    if(req->options & CONTEXT_V2_OPTION_DEBUG) {
        buffer_json_member_add_object(wb, "request");
        {
            buffer_json_contexts_v2_mode_to_array(wb, "mode", mode);
            web_client_api_request_v2_contexts_options_to_buffer_json_array(wb, "options", req->options);

            buffer_json_member_add_object(wb, "scope");
            {
                buffer_json_member_add_string(wb, "scope_nodes", req->scope_nodes);
                if (mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_ALERTS))
                    buffer_json_member_add_string(wb, "scope_contexts", req->scope_contexts);
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "selectors");
            {
                buffer_json_member_add_string(wb, "nodes", req->nodes);

                if (mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_ALERTS))
                    buffer_json_member_add_string(wb, "contexts", req->contexts);

                if(mode & CONTEXTS_V2_ALERTS) {
                    buffer_json_member_add_object(wb, "alerts");
                    web_client_api_request_v2_contexts_alerts_status_to_buffer_json_array(wb, "status", req->alerts.status);
                    buffer_json_member_add_string(wb, "alert", req->alerts.alert);
                    buffer_json_member_add_string(wb, "transition", req->alerts.transition);
                    buffer_json_member_add_uint64(wb, "last", req->alerts.last);
                    buffer_json_object_close(wb); // alerts
                }
            }
            buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, "filters");
            {
                if (mode & CONTEXTS_V2_SEARCH)
                    buffer_json_member_add_string(wb, "q", req->q);

                buffer_json_member_add_time_t(wb, "after", req->after);
                buffer_json_member_add_time_t(wb, "before", req->before);
            }
            buffer_json_object_close(wb);
        }
        buffer_json_object_close(wb);
    }

    ssize_t ret = 0;
    if(run)
        ret = query_scope_foreach_host(ctl.nodes.scope_pattern, ctl.nodes.pattern,
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

    ctl.timings.executed_ut = now_monotonic_usec();

    if(mode & CONTEXTS_V2_NODES) {
        buffer_json_member_add_array(wb, "nodes");
        struct contexts_v2_node *t;
        dfe_start_read(ctl.nodes.dict, t) {
            rrdcontext_to_json_v2_rrdhost(wb, t->host, &ctl, t->ni);
        }
        dfe_done(t);
        buffer_json_array_close(wb);
    }

    if(mode & CONTEXTS_V2_FUNCTIONS) {
        buffer_json_member_add_array(wb, "functions");
        {
            struct function_v2_entry *t;
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

    if(mode & CONTEXTS_V2_CONTEXTS) {
        buffer_json_member_add_object(wb, "contexts");
        {
            struct context_v2_entry *z;
            dfe_start_read(ctl.contexts.dict, z) {
                bool collected = z->flags & RRD_FLAG_COLLECTED;

                buffer_json_member_add_object(wb, string2str(z->id));
                {
                    buffer_json_member_add_string(wb, "family", string2str(z->family));
                    buffer_json_member_add_uint64(wb, "priority", z->priority);
                    buffer_json_member_add_time_t(wb, "first_entry", z->first_time_s);
                    buffer_json_member_add_time_t(wb, "last_entry", collected ? ctl.now : z->last_time_s);
                    buffer_json_member_add_boolean(wb, "live", collected);
                    if (mode & CONTEXTS_V2_SEARCH)
                        buffer_json_member_add_string(wb, "match", fts_match_to_string(z->match));
                }
                buffer_json_object_close(wb);
            }
            dfe_done(z);
        }
        buffer_json_object_close(wb); // contexts
    }

    if(mode & CONTEXTS_V2_ALERTS) {
        buffer_json_member_add_array(wb, "alerts");
        {
            struct alert_v2_entry *t;
            dfe_start_read(ctl.alerts.alerts, t){
                buffer_json_add_array_item_object(wb);
                {
                    buffer_json_member_add_uint64(wb, "ati", t->ati);
                    buffer_json_member_add_string(wb, "nm", string2str(t->name));

                    buffer_json_member_add_uint64(wb, "cr", t->critical);
                    buffer_json_member_add_uint64(wb, "wr", t->warning);
                    buffer_json_member_add_uint64(wb, "cl", t->clear);
                    buffer_json_member_add_uint64(wb, "er", t->error);

                    buffer_json_member_add_uint64(wb, "in", t->instances);
                    buffer_json_member_add_uint64(wb, "nd", dictionary_entries(t->nodes));
                    buffer_json_member_add_uint64(wb, "cfg", dictionary_entries(t->configs));
                }
                buffer_json_object_close(wb); // alert name
            }
            dfe_done(t);
        }
        buffer_json_array_close(wb); // alerts

        if(req->options & CONTEXT_V2_OPTION_ALERT_INSTANCES) {
            buffer_json_member_add_array(wb, "alert_instances");
            {
                struct alert_instance_v2_entry *t;
                dfe_start_read(ctl.alerts.alert_instances, t){
                    buffer_json_add_array_item_object(wb);
                    {
                        buffer_json_member_add_uint64(wb, "ni", t->ni);
                        buffer_json_member_add_uint64(wb, "ati", t->ati);
                        buffer_json_member_add_uint64(wb, "aii", t->aii);
                        if(req->options & CONTEXT_V2_OPTION_ALERT_CONFIGURATIONS) {
                            buffer_json_member_add_uint64(wb, "aci", t->aci);
                        }
                        buffer_json_member_add_uint64(wb, "gi", t->global_id);

                        if(debug)
                            buffer_json_member_add_string(wb, "nm", string2str(t->name));

                        buffer_json_member_add_string(wb, "fami", string2str(t->family));
                        buffer_json_member_add_string(wb, "info", string2str(t->info));
                        buffer_json_member_add_string(wb, "ch", string2str(t->chart_name));
                        buffer_json_member_add_string(wb, "st", rrdcalc_status2string(t->status));
                        buffer_json_member_add_double(wb, "v", t->value);
                        buffer_json_member_add_time_t(wb, "t", t->last_updated);
                        buffer_json_member_add_uuid  (wb, "tr_i", &t->last_transition_id);
                        buffer_json_member_add_double(wb, "tr_v", t->last_status_change_value);
                        buffer_json_member_add_time_t(wb, "tr_t", t->last_status_change);
                        buffer_json_member_add_uuid  (wb, "cfg", &t->config_hash_id);
                        rrdcalc_flags_to_json_array  (wb, "flags", t->flags);
                    }
                    buffer_json_object_close(wb); // alert instance
                }
                dfe_done(t);
            }
            buffer_json_array_close(wb); // alerts_instances

            if(req->options & CONTEXT_V2_OPTION_ALERT_TRANSITIONS) {
                buffer_json_member_add_array(wb, "alert_transitions");
                sql_health_alarm_log2json_v3(
                    wb,
                    ctl.alerts.alert_instances,
                    ctl.request->after,
                    ctl.request->before,
                    ctl.request->alerts.transition,
                    ctl.request->alerts.last ? ctl.request->alerts.last : 1,
                    debug);
                buffer_json_array_close(wb); // alerts_transitions
            }
        }

        if(req->options & CONTEXT_V2_OPTION_ALERT_CONFIGURATIONS) {
            buffer_json_member_add_array(wb, "alert_configurations");
            {
                struct alert_config_v2_entry *t;
                dfe_start_read(ctl.alerts.alert_configs, t){
                    RRDCALC *rc = t->tmp;

                    buffer_json_add_array_item_object(wb);
                    {
                        buffer_json_member_add_uint64(wb, "ati", t->ati);
                        buffer_json_member_add_uint64(wb, "aci", t->aci);
                        buffer_json_member_add_string(wb, "cfg", t_dfe.name);

                        if(debug)
                            buffer_json_member_add_string(wb, "nm", string2str(rc->name));

                        buffer_json_member_add_string(wb, "ctx", string2str(rc->rrdset->context));
                        buffer_json_member_add_string(wb, "class", string2str(rc->classification));
                        buffer_json_member_add_string(wb, "component", string2str(rc->component));
                        buffer_json_member_add_string(wb, "type", string2str(rc->type));
                        buffer_json_member_add_string(wb, "info", string2str(rc->original_info));

                        buffer_json_member_add_object(wb, "v"); // value
                        {
                            buffer_json_member_add_uint64(wb, "update_every", rc->update_every);
                            buffer_json_member_add_string(wb, "units", string2str(rc->units));

                            if (RRDCALC_HAS_DB_LOOKUP(rc) || debug) {
                                buffer_json_member_add_object(wb, "db");
                                {
                                    if (rc->dimensions || debug)
                                        buffer_json_member_add_string(wb, "dimensions", rrdcalc_dimensions(rc));

                                    buffer_json_member_add_string(wb, "method", time_grouping_method2string(rc->group));
                                    buffer_json_member_add_time_t(wb, "after", rc->after);
                                    buffer_json_member_add_time_t(wb, "before", rc->before);

                                    web_client_api_request_v1_data_options_to_buffer_json_array(wb, "options", (RRDR_OPTIONS) rc->options);
                                }
                                buffer_json_object_close(wb); // db
                            }

                            if (rc->calculation || debug) {
                                buffer_json_member_add_string(wb, "calc", rc->calculation ? rc->calculation->source : NULL);
                                // buffer_json_member_add_string(wb, "calc_parsed", rc->calculation->parsed_as);
                            }
                        }
                        buffer_json_object_close(wb); // value

                        if(rc->warning || rc->critical || debug) {
                            buffer_json_member_add_object(wb, "st"); // conditions
                            {
                                if(!isnan(rc->green) || debug)
                                    buffer_json_member_add_double(wb, "green", rc->green);

                                if(!isnan(rc->red) || debug)
                                    buffer_json_member_add_double(wb, "red", rc->red);

                                if (rc->warning || debug) {
                                    buffer_json_member_add_string(wb, "warn", rc->warning ? rc->warning->source : NULL);
                                    // buffer_json_member_add_string(wb, "warn_parsed", rc->warning ? rc->warning->parsed_as : NULL);
                                }

                                if (rc->critical || debug) {
                                    buffer_json_member_add_string(wb, "crit", rc->critical ? rc->critical->source : NULL);
                                    // buffer_json_member_add_string(wb, "crit_parsed", rc->critical ? rc->critical->parsed_as : NULL);
                                }
                            }
                            buffer_json_object_close(wb); // conditions
                        }

                        buffer_json_member_add_object(wb, "nf");
                        {
                            buffer_json_member_add_string(wb, "type", "agent");
                            buffer_json_member_add_string(wb, "method", rc->exec ? rrdcalc_exec(rc) : string2str(localhost->health.health_default_exec));
                            buffer_json_member_add_string(wb, "to", rc->recipient ? string2str(rc->recipient) : string2str(rc->rrdset->rrdhost->health.health_default_recipient));

                            if(rc->delay_up_duration || rc->delay_down_duration || debug) {
                                buffer_json_member_add_object(wb, "delay");
                                {
                                    buffer_json_member_add_time_t(wb, "up", rc->delay_up_duration);
                                    buffer_json_member_add_time_t(wb, "down", rc->delay_down_duration);
                                    buffer_json_member_add_time_t(wb, "max", rc->delay_max_duration);
                                    buffer_json_member_add_double(wb, "multiplier", rc->delay_multiplier);
                                }
                                buffer_json_object_close(wb); //delay
                            }

                            if(rc->warn_repeat_every || rc->crit_repeat_every || debug) {
                                buffer_json_member_add_object(wb, "repeat");
                                {
                                    if(rc->warn_repeat_every || debug)
                                        buffer_json_member_add_time_t(wb, "warn", rc->warn_repeat_every);

                                    if(rc->crit_repeat_every || debug)
                                        buffer_json_member_add_time_t(wb, "crit", rc->crit_repeat_every);
                                }
                                buffer_json_object_close(wb); // repeat
                            }

                            if (unlikely((rc->options & RRDCALC_OPTION_NO_CLEAR_NOTIFICATION) || debug))
                                buffer_json_member_add_boolean(wb, "no_clear_notification", rc->options & RRDCALC_OPTION_NO_CLEAR_NOTIFICATION);
                        }
                        buffer_json_object_close(wb); // notification
                        buffer_json_member_add_string(wb, "src", string2str(rc->source));
                    }
                    buffer_json_object_close(wb); // alert config
                }
                dfe_done(t);
            }
            buffer_json_array_close(wb); // alerts_configs
        }
    }

    if(mode & CONTEXTS_V2_SEARCH) {
        buffer_json_member_add_object(wb, "searches");
        {
            buffer_json_member_add_uint64(wb, "strings", ctl.q.fts.string_searches);
            buffer_json_member_add_uint64(wb, "char", ctl.q.fts.char_searches);
            buffer_json_member_add_uint64(wb, "total", ctl.q.fts.searches);
        }
        buffer_json_object_close(wb);
    }

    if(mode & (CONTEXTS_V2_VERSIONS))
        version_hashes_api_v2(wb, &ctl.versions);

    if(mode & CONTEXTS_V2_AGENTS)
        buffer_json_agents_array_v2(wb, &ctl.timings, ctl.now, mode & (CONTEXTS_V2_AGENTS_INFO));

    buffer_json_cloud_timings(wb, "timings", &ctl.timings);

    buffer_json_finalize(wb);

cleanup:
    dictionary_destroy(ctl.nodes.dict);
    dictionary_destroy(ctl.contexts.dict);
    dictionary_destroy(ctl.functions.dict);
    dictionary_destroy(ctl.alerts.alerts);
    dictionary_destroy(ctl.alerts.alert_instances);
    dictionary_destroy(ctl.alerts.alert_configs);
    simple_pattern_free(ctl.nodes.scope_pattern);
    simple_pattern_free(ctl.nodes.pattern);
    simple_pattern_free(ctl.contexts.pattern);
    simple_pattern_free(ctl.contexts.scope_pattern);
    simple_pattern_free(ctl.q.pattern);
    simple_pattern_free(ctl.alerts.alert_name_pattern);

    return resp;
}
