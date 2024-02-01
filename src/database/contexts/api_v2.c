// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

#include "aclk/aclk_capas.h"

// ----------------------------------------------------------------------------
// /api/v2/contexts API

struct alert_transitions_facets alert_transition_facets[] = {
        [ATF_STATUS] = {
                .id = "f_status",
                .name = "Alert Status",
                .query_param = "f_status",
                .order = 1,
        },
        [ATF_TYPE] = {
                .id = "f_type",
                .name = "Alert Type",
                .query_param = "f_type",
                .order = 2,
        },
        [ATF_ROLE] = {
                .id = "f_role",
                .name = "Recipient Role",
                .query_param = "f_role",
                .order = 3,
        },
        [ATF_CLASS] = {
                .id = "f_class",
                .name = "Alert Class",
                .query_param = "f_class",
                .order = 4,
        },
        [ATF_COMPONENT] = {
                .id = "f_component",
                .name = "Alert Component",
                .query_param = "f_component",
                .order = 5,
        },
        [ATF_NODE] = {
                .id = "f_node",
                .name = "Alert Node",
                .query_param = "f_node",
                .order = 6,
        },
        [ATF_ALERT_NAME] = {
                .id = "f_alert",
                .name = "Alert Name",
                .query_param = "f_alert",
                .order = 7,
        },
        [ATF_CHART_NAME] = {
                .id = "f_instance",
                .name = "Instance Name",
                .query_param = "f_instance",
                .order = 8,
        },
        [ATF_CONTEXT] = {
                .id = "f_context",
                .name = "Context",
                .query_param = "f_context",
                .order = 9,
        },

        // terminator
        [ATF_TOTAL_ENTRIES] = {
                .id = NULL,
                .name = NULL,
                .query_param = NULL,
                .order = 9999,
        }
};

struct facet_entry {
    uint32_t count;
};

struct alert_transitions_callback_data {
    struct rrdcontext_to_json_v2_data *ctl;
    BUFFER *wb;
    bool debug;
    bool only_one_config;

    struct {
        SIMPLE_PATTERN *pattern;
        DICTIONARY *dict;
    } facets[ATF_TOTAL_ENTRIES];

    uint32_t max_items_to_return;
    uint32_t items_to_return;

    uint32_t items_evaluated;
    uint32_t items_matched;


    struct sql_alert_transition_fixed_size *base; // double linked list - last item is base->prev
    struct sql_alert_transition_fixed_size *last_added; // the last item added, not the last of the list

    struct {
        size_t first;
        size_t skips_before;
        size_t skips_after;
        size_t backwards;
        size_t forwards;
        size_t prepend;
        size_t append;
        size_t shifts;
    } operations;

    uint32_t configs_added;
};

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
    STRING *tags;
    HTTP_ACCESS access;
    int priority;
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

struct alert_counts {
    size_t critical;
    size_t warning;
    size_t clear;
    size_t error;
};

struct alert_v2_entry {
    RRDCALC *tmp;

    STRING *name;
    STRING *summary;

    size_t ati;

    struct alert_counts counts;

    size_t instances;
    DICTIONARY *nodes;
    DICTIONARY *configs;
};

struct alert_by_x_entry {
    struct {
        struct alert_counts counts;
        size_t silent;
        size_t total;
    } running;

    struct {
        size_t available;
    } prototypes;
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

        DICTIONARY *summary;
        DICTIONARY *alert_instances;

        DICTIONARY *by_type;
        DICTIONARY *by_component;
        DICTIONARY *by_classification;
        DICTIONARY *by_recipient;
        DICTIONARY *by_module;
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

static void alert_counts_add(struct alert_counts *t, RRDCALC *rc) {
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
}

static void alerts_v2_add(struct alert_v2_entry *t, RRDCALC *rc) {
    t->instances++;

    alert_counts_add(&t->counts, rc);

    dictionary_set(t->nodes, rc->rrdset->rrdhost->machine_guid, NULL, 0);

    char key[UUID_STR_LEN + 1];
    uuid_unparse_lower(rc->config.hash_id, key);
    dictionary_set(t->configs, key, NULL, 0);
}

static void alerts_by_x_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    static STRING *silent = NULL;
    if(unlikely(!silent)) silent = string_strdupz("silent");

    struct alert_by_x_entry *b = value;
    RRDCALC *rc = data;
    if(!rc) {
        // prototype
        b->prototypes.available++;
    }
    else {
        alert_counts_add(&b->running.counts, rc);

        b->running.total++;

        if (rc->config.recipient == silent)
            b->running.silent++;
    }
}

static bool alerts_by_x_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value __maybe_unused, void *data __maybe_unused) {
    alerts_by_x_insert_callback(item, old_value, data);
    return false;
}

static void alerts_v2_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct rrdcontext_to_json_v2_data *ctl = data;
    struct alert_v2_entry *t = value;
    RRDCALC *rc = t->tmp;
    t->name = rc->config.name;
    t->summary = rc->config.summary; // the original summary
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

static void alert_instances_v2_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct rrdcontext_to_json_v2_data *ctl = data;
    struct sql_alert_instance_v2_entry *t = value;
    RRDCALC *rc = t->tmp;

    t->context = rc->rrdset->context;
    t->chart_id = rc->rrdset->id;
    t->chart_name = rc->rrdset->name;
    t->family = rc->rrdset->family;
    t->units = rc->config.units;
    t->classification = rc->config.classification;
    t->type = rc->config.type;
    t->recipient = rc->config.recipient;
    t->component = rc->config.component;
    t->name = rc->config.name;
    t->source = rc->config.source;
    t->status = rc->status;
    t->flags = rc->run_flags;
    t->info = rc->config.info;
    t->summary = rc->summary;
    t->value = rc->value;
    t->last_updated = rc->last_updated;
    t->last_status_change = rc->last_status_change;
    t->last_status_change_value = rc->last_status_change_value;
    t->host = rc->rrdset->rrdhost;
    t->alarm_id = rc->id;
    t->ni = ctl->nodes.ni;

    uuid_copy(t->config_hash_id, rc->config.hash_id);
    health_alarm_log_get_global_id_and_transition_id_for_rrdcalc(rc, &t->global_id, &t->last_transition_id);
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
        if(unlikely(ri->rrdlabels && rrdlabels_entries(ri->rrdlabels) &&
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
            rw_spinlock_read_lock(&st->alerts.spinlock);
            for (RRDCALC *rcl = st->alerts.base; rcl; rcl = rcl->next) {
                if(unlikely(full_text_search_string(&ctl->q.fts, q, rcl->config.name))) {
                    matched = FTS_MATCHED_ALERT;
                    break;
                }

                if(unlikely(full_text_search_string(&ctl->q.fts, q, rcl->config.info))) {
                    matched = FTS_MATCHED_ALERT_INFO;
                    break;
                }
            }
            rw_spinlock_read_unlock(&st->alerts.spinlock);
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
            rw_spinlock_read_lock(&st->alerts.spinlock);
            for (RRDCALC *rcl = st->alerts.base; rcl; rcl = rcl->next) {
                if(ctl->alerts.alert_name_pattern && !simple_pattern_matches_string(ctl->alerts.alert_name_pattern, rcl->config.name))
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
                struct alert_v2_entry *a2e =
                    dictionary_set(ctl->alerts.summary, string2str(rcl->config.name),
                                   &t, sizeof(struct alert_v2_entry));
                size_t ati = a2e->ati;
                matches++;

                dictionary_set_advanced(ctl->alerts.by_type,
                                        string2str(rcl->config.type),
                                        (ssize_t)string_strlen(rcl->config.type),
                                        NULL,
                                        sizeof(struct alert_by_x_entry),
                                        rcl);

                dictionary_set_advanced(ctl->alerts.by_component,
                                        string2str(rcl->config.component),
                                        (ssize_t)string_strlen(rcl->config.component),
                                        NULL,
                                        sizeof(struct alert_by_x_entry),
                                        rcl);

                dictionary_set_advanced(ctl->alerts.by_classification,
                                        string2str(rcl->config.classification),
                                        (ssize_t)string_strlen(rcl->config.classification),
                                        NULL,
                                        sizeof(struct alert_by_x_entry),
                                        rcl);

                dictionary_set_advanced(ctl->alerts.by_recipient,
                                        string2str(rcl->config.recipient),
                                        (ssize_t)string_strlen(rcl->config.recipient),
                                        NULL,
                                        sizeof(struct alert_by_x_entry),
                                        rcl);

                char *module = NULL;
                rrdlabels_get_value_strdup_or_null(st->rrdlabels, &module, "_collect_module");
                if(!module || !*module) module = "[unset]";

                dictionary_set_advanced(ctl->alerts.by_module,
                                        module,
                                        -1,
                                        NULL,
                                        sizeof(struct alert_by_x_entry),
                                        rcl);

                if (ctl->options & (CONTEXT_V2_OPTION_ALERTS_WITH_INSTANCES | CONTEXT_V2_OPTION_ALERTS_WITH_VALUES)) {
                    char key[20 + 1];
                    snprintfz(key, sizeof(key) - 1, "%p", rcl);

                    struct sql_alert_instance_v2_entry z = {
                            .ati = ati,
                            .tmp = rcl,
                    };
                    dictionary_set(ctl->alerts.alert_instances, key, &z, sizeof(z));
                }
            }
            rw_spinlock_read_unlock(&st->alerts.spinlock);
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
                    snprintfz(buf, sizeof(buf) - 1, "[%s]:%d%s", s->ingest.peers.local.ip, s->ingest.peers.local.port, s->ingest.ssl ? ":SSL" : "");
                    buffer_json_member_add_string(wb, "local", buf);

                    snprintfz(buf, sizeof(buf) - 1, "[%s]:%d%s", s->ingest.peers.peer.ip, s->ingest.peers.peer.port, s->ingest.ssl ? ":SSL" : "");
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
            snprintfz(buf, sizeof(buf) - 1, "[%s]:%d%s", s->stream.peers.local.ip, s->stream.peers.local.port, s->stream.ssl ? ":SSL" : "");
            buffer_json_member_add_string(wb, "local", buf);

            snprintfz(buf, sizeof(buf) - 1, "[%s]:%d%s", s->stream.peers.peer.ip, s->stream.peers.peer.port, s->stream.ssl ? ":SSL" : "");
            buffer_json_member_add_string(wb, "remote", buf);

            stream_capabilities_to_json_array(wb, s->stream.capabilities, "capabilities");

            buffer_json_member_add_object(wb, "traffic");
            {
                buffer_json_member_add_boolean(wb, "compression", s->stream.compression);
                buffer_json_member_add_uint64(wb, "data", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DATA]);
                buffer_json_member_add_uint64(wb, "metadata", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_METADATA]);
                buffer_json_member_add_uint64(wb, "functions", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_FUNCTIONS]);
                buffer_json_member_add_uint64(wb, "replication", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_REPLICATION]);
                buffer_json_member_add_uint64(wb, "dyncfg", s->stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DYNCFG]);
            }
            buffer_json_object_close(wb); // traffic

            buffer_json_member_add_array(wb, "candidates");
            struct rrdpush_destinations *d;
            for (d = s->host->destinations; d; d = d->next) {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_uint64(wb, "attempts", d->attempts);
                {

                    if (d->ssl) {
                        snprintfz(buf, sizeof(buf) - 1, "%s:SSL", string2str(d->destination));
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

static inline void host_dyncfg_to_json_v2(BUFFER *wb, const char *key, RRDHOST_STATUS *s) {
    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_string(wb, "status", rrdhost_dyncfg_status_to_string(s->dyncfg.status));
    }
    buffer_json_object_close(wb); // health

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
                            (ctl->mode & CONTEXTS_V2_AGENTS) && !(ctl->mode & CONTEXTS_V2_NODE_INSTANCES));

    if(ctl->mode & (CONTEXTS_V2_NODES_INFO | CONTEXTS_V2_NODE_INSTANCES)) {
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

        if (ctl->mode & (CONTEXTS_V2_NODE_INSTANCES)) {
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

                host_dyncfg_to_json_v2(wb, "dyncfg", &s);
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
    bool do_contexts = (ctl->mode & (CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_ALERTS));

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
                .tags = NULL,
                .access = HTTP_ACCESS_ALL,
                .priority = RRDFUNCTIONS_PRIORITY_DEFAULT,
        };
        host_functions_to_dict(host, ctl->functions.dict, &t, sizeof(t), &t.help, &t.tags, &t.access, &t.priority);
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

    if(mode & CONTEXTS_V2_NODE_INSTANCES)
        buffer_json_add_array_item_string(wb, "nodes-instances");

    if(mode & CONTEXTS_V2_CONTEXTS)
        buffer_json_add_array_item_string(wb, "contexts");

    if(mode & CONTEXTS_V2_SEARCH)
        buffer_json_add_array_item_string(wb, "search");

    if(mode & CONTEXTS_V2_ALERTS)
        buffer_json_add_array_item_string(wb, "alerts");

    if(mode & CONTEXTS_V2_ALERT_TRANSITIONS)
        buffer_json_add_array_item_string(wb, "alert_transitions");

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

void build_info_to_json_object(BUFFER *b);

void buffer_json_agents_v2(BUFFER *wb, struct query_timings *timings, time_t now_s, bool info, bool array) {
    if(!now_s)
        now_s = now_realtime_sec();

    if(array) {
        buffer_json_member_add_array(wb, "agents");
        buffer_json_add_array_item_object(wb);
    }
    else
        buffer_json_member_add_object(wb, "agent");

    buffer_json_member_add_string(wb, "mg", localhost->machine_guid);
    buffer_json_member_add_uuid(wb, "nd", localhost->node_id);
    buffer_json_member_add_string(wb, "nm", rrdhost_hostname(localhost));
    buffer_json_member_add_time_t(wb, "now", now_s);

    if(array)
        buffer_json_member_add_uint64(wb, "ai", 0);

    if(info) {
        buffer_json_member_add_object(wb, "application");
        build_info_to_json_object(wb);
        buffer_json_object_close(wb); // netdata

        buffer_json_cloud_status(wb, now_s);

        buffer_json_member_add_array(wb, "db_size");
        for (size_t tier = 0; tier < storage_tiers; tier++) {
            STORAGE_ENGINE *eng = localhost->db[tier].eng;
            if (!eng) continue;

            uint64_t max = storage_engine_disk_space_max(eng->seb, localhost->db[tier].si);
            uint64_t used = storage_engine_disk_space_used(eng->seb, localhost->db[tier].si);
            time_t first_time_s = storage_engine_global_first_time_s(eng->seb, localhost->db[tier].si);
            size_t currently_collected_metrics = storage_engine_collected_metrics(eng->seb, localhost->db[tier].si);

            NETDATA_DOUBLE percent;
            if (used && max)
                percent = (NETDATA_DOUBLE) used * 100.0 / (NETDATA_DOUBLE) max;
            else
                percent = 0.0;

            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_uint64(wb, "tier", tier);

            if(used || max) {
                buffer_json_member_add_uint64(wb, "disk_used", used);
                buffer_json_member_add_uint64(wb, "disk_max", max);
                buffer_json_member_add_double(wb, "disk_percent", percent);
            }

            if(first_time_s) {
                buffer_json_member_add_time_t(wb, "from", first_time_s);
                buffer_json_member_add_time_t(wb, "to", now_s);
                buffer_json_member_add_time_t(wb, "retention", now_s - first_time_s);

                if(used || max) // we have disk space information
                    buffer_json_member_add_time_t(wb, "expected_retention",
                                                  (time_t) ((NETDATA_DOUBLE) (now_s - first_time_s) * 100.0 / percent));
            }

            if(currently_collected_metrics)
                buffer_json_member_add_uint64(wb, "currently_collected_metrics", currently_collected_metrics);

            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb); // db_size
    }

    if(timings)
        buffer_json_query_timings(wb, "timings", timings);

    buffer_json_object_close(wb);

    if(array)
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
        if((o->flags & RRD_FLAG_COLLECTED) && !(n->flags & RRD_FLAG_COLLECTED))
            // keep old
            ;
        else if(!(o->flags & RRD_FLAG_COLLECTED) && (n->flags & RRD_FLAG_COLLECTED)) {
            // keep new
            string_freez(o->family);
            o->family = string_dup(n->family);
        }
        else {
            // merge
            STRING *old_family = o->family;
            o->family = string_2way_merge(o->family, n->family);
            string_freez(old_family);
        }
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

struct alert_instances_callback_data {
    BUFFER *wb;
    struct rrdcontext_to_json_v2_data *ctl;
    bool debug;
};

static void contexts_v2_alert_config_to_json_from_sql_alert_config_data(struct sql_alert_config_data *t, void *data) {
    struct alert_transitions_callback_data *d = data;
    BUFFER *wb = d->wb;
    bool debug = d->debug;
    d->configs_added++;

    if(d->only_one_config)
        buffer_json_add_array_item_object(wb); // alert config
        
    {
        buffer_json_member_add_string(wb, "name", t->name);
        buffer_json_member_add_uuid(wb, "config_hash_id", t->config_hash_id);

        buffer_json_member_add_object(wb, "selectors");
        {
            bool is_template = t->selectors.on_template && *t->selectors.on_template ? true : false;
            buffer_json_member_add_string(wb, "type", is_template ? "template" : "alarm");
            buffer_json_member_add_string(wb, "on", is_template ? t->selectors.on_template : t->selectors.on_key);

            buffer_json_member_add_string(wb, "os", t->selectors.os);
            buffer_json_member_add_string(wb, "hosts", t->selectors.hosts);
            buffer_json_member_add_string(wb, "families", t->selectors.families);
            buffer_json_member_add_string(wb, "plugin", t->selectors.plugin);
            buffer_json_member_add_string(wb, "module", t->selectors.module);
            buffer_json_member_add_string(wb, "host_labels", t->selectors.host_labels);
            buffer_json_member_add_string(wb, "chart_labels", t->selectors.chart_labels);
            buffer_json_member_add_string(wb, "charts", t->selectors.charts);
        }
        buffer_json_object_close(wb); // selectors

        buffer_json_member_add_object(wb, "value"); // value
        {
            // buffer_json_member_add_string(wb, "every", t->value.every); // does not exist in Netdata Cloud
            buffer_json_member_add_string(wb, "units", t->value.units);
            buffer_json_member_add_uint64(wb, "update_every", t->value.update_every);

            if (t->value.db.after || debug) {
                buffer_json_member_add_object(wb, "db");
                {
                    // buffer_json_member_add_string(wb, "lookup", t->value.db.lookup); // does not exist in Netdata Cloud

                    buffer_json_member_add_time_t(wb, "after", t->value.db.after);
                    buffer_json_member_add_time_t(wb, "before", t->value.db.before);
                    buffer_json_member_add_string(wb, "method", t->value.db.method);
                    buffer_json_member_add_string(wb, "dimensions", t->value.db.dimensions);
                    rrdr_options_to_buffer_json_array(wb, "options", (RRDR_OPTIONS)t->value.db.options);
                }
                buffer_json_object_close(wb); // db
            }

            if (t->value.calc || debug)
                buffer_json_member_add_string(wb, "calc", t->value.calc);
        }
        buffer_json_object_close(wb); // value

        if (t->status.warn || t->status.crit || debug) {
            buffer_json_member_add_object(wb, "status"); // status
            {
                NETDATA_DOUBLE green = t->status.green ? str2ndd(t->status.green, NULL) : NAN;
                NETDATA_DOUBLE red = t->status.red ? str2ndd(t->status.red, NULL) : NAN;

                if (!isnan(green) || debug)
                    buffer_json_member_add_double(wb, "green", green);

                if (!isnan(red) || debug)
                    buffer_json_member_add_double(wb, "red", red);

                if (t->status.warn || debug)
                    buffer_json_member_add_string(wb, "warn", t->status.warn);

                if (t->status.crit || debug)
                    buffer_json_member_add_string(wb, "crit", t->status.crit);
            }
            buffer_json_object_close(wb); // status
        }

        buffer_json_member_add_object(wb, "notification");
        {
            buffer_json_member_add_string(wb, "type", "agent");
            buffer_json_member_add_string(wb, "exec", t->notification.exec ? t->notification.exec : NULL);
            buffer_json_member_add_string(wb, "to", t->notification.to_key ? t->notification.to_key : string2str(localhost->health.health_default_recipient));
            buffer_json_member_add_string(wb, "delay", t->notification.delay);
            buffer_json_member_add_string(wb, "repeat", t->notification.repeat);
            buffer_json_member_add_string(wb, "options", t->notification.options);
        }
        buffer_json_object_close(wb); // notification

        buffer_json_member_add_string(wb, "class", t->classification);
        buffer_json_member_add_string(wb, "component", t->component);
        buffer_json_member_add_string(wb, "type", t->type);
        buffer_json_member_add_string(wb, "info", t->info);
        buffer_json_member_add_string(wb, "summary", t->summary);
        // buffer_json_member_add_string(wb, "source", t->source); // moved to alert instance
    }
    
    if(d->only_one_config)
        buffer_json_object_close(wb);
}

int contexts_v2_alert_config_to_json(struct web_client *w, const char *config_hash_id) {
    struct alert_transitions_callback_data data = {
            .wb = w->response.data,
            .debug = false,
            .only_one_config = false,
    };
    DICTIONARY *configs = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_set(configs, config_hash_id, NULL, 0);

    buffer_flush(w->response.data);

    buffer_json_initialize(w->response.data, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    int added = sql_get_alert_configuration(configs, contexts_v2_alert_config_to_json_from_sql_alert_config_data, &data, false);
    buffer_json_finalize(w->response.data);

    int ret = HTTP_RESP_OK;

    if(added <= 0) {
        buffer_flush(w->response.data);
        w->response.data->content_type = CT_TEXT_PLAIN;
        if(added < 0) {
            buffer_strcat(w->response.data, "Failed to execute SQL query.");
            ret = HTTP_RESP_INTERNAL_SERVER_ERROR;
        }
        else {
            buffer_strcat(w->response.data, "Config is not found.");
            ret = HTTP_RESP_NOT_FOUND;
        }
    }

    return ret;
}

static int contexts_v2_alert_instance_to_json_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    struct sql_alert_instance_v2_entry *t = value;
    struct alert_instances_callback_data *d = data;
    struct rrdcontext_to_json_v2_data *ctl = d->ctl; (void)ctl;
    bool debug = d->debug; (void)debug;
    BUFFER *wb = d->wb;

    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_uint64(wb, "ni", t->ni);

        buffer_json_member_add_string(wb, "nm", string2str(t->name));
        buffer_json_member_add_string(wb, "ch", string2str(t->chart_id));
        buffer_json_member_add_string(wb, "ch_n", string2str(t->chart_name));

        if(ctl->request->options & CONTEXT_V2_OPTION_ALERTS_WITH_SUMMARY)
            buffer_json_member_add_uint64(wb, "ati", t->ati);

        if(ctl->request->options & CONTEXT_V2_OPTION_ALERTS_WITH_INSTANCES) {
            buffer_json_member_add_string(wb, "units", string2str(t->units));
            buffer_json_member_add_string(wb, "fami", string2str(t->family));
            buffer_json_member_add_string(wb, "info", string2str(t->info));
            buffer_json_member_add_string(wb, "sum", string2str(t->summary));
            buffer_json_member_add_string(wb, "ctx", string2str(t->context));
            buffer_json_member_add_string(wb, "st", rrdcalc_status2string(t->status));
            buffer_json_member_add_uuid(wb, "tr_i", &t->last_transition_id);
            buffer_json_member_add_double(wb, "tr_v", t->last_status_change_value);
            buffer_json_member_add_time_t(wb, "tr_t", t->last_status_change);
            buffer_json_member_add_uuid(wb, "cfg", &t->config_hash_id);
            buffer_json_member_add_string(wb, "src", string2str(t->source));

            buffer_json_member_add_string(wb, "to", string2str(t->recipient));
            buffer_json_member_add_string(wb, "tp", string2str(t->type));
            buffer_json_member_add_string(wb, "cm", string2str(t->component));
            buffer_json_member_add_string(wb, "cl", string2str(t->classification));

            // Agent specific fields
            buffer_json_member_add_uint64(wb, "gi", t->global_id);
            // rrdcalc_flags_to_json_array  (wb, "flags", t->flags);
        }

        if(ctl->request->options & CONTEXT_V2_OPTION_ALERTS_WITH_VALUES) {
            // Netdata Cloud fetched these by querying the agents
            buffer_json_member_add_double(wb, "v", t->value);
            buffer_json_member_add_time_t(wb, "t", t->last_updated);
        }
    }
    buffer_json_object_close(wb); // alert instance

    return 1;
}

static void contexts_v2_alerts_by_x_update_prototypes(void *data, STRING *type, STRING *component, STRING *classification, STRING *recipient) {
    struct rrdcontext_to_json_v2_data *ctl = data;

    dictionary_set_advanced(ctl->alerts.by_type, string2str(type), (ssize_t)string_strlen(type), NULL, sizeof(struct alert_by_x_entry), NULL);
    dictionary_set_advanced(ctl->alerts.by_component, string2str(component), (ssize_t)string_strlen(component), NULL, sizeof(struct alert_by_x_entry), NULL);
    dictionary_set_advanced(ctl->alerts.by_classification, string2str(classification), (ssize_t)string_strlen(classification), NULL, sizeof(struct alert_by_x_entry), NULL);
    dictionary_set_advanced(ctl->alerts.by_recipient, string2str(recipient), (ssize_t)string_strlen(recipient), NULL, sizeof(struct alert_by_x_entry), NULL);
}

static void contexts_v2_alerts_by_x_to_json(BUFFER *wb, DICTIONARY *dict, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        struct alert_by_x_entry *b;
        dfe_start_read(dict, b) {
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "name", b_dfe.name);
                buffer_json_member_add_uint64(wb, "cr", b->running.counts.critical);
                buffer_json_member_add_uint64(wb, "wr", b->running.counts.warning);
                buffer_json_member_add_uint64(wb, "cl", b->running.counts.clear);
                buffer_json_member_add_uint64(wb, "er", b->running.counts.error);
                buffer_json_member_add_uint64(wb, "running", b->running.total);

                buffer_json_member_add_uint64(wb, "running_silent", b->running.silent);

                if(b->prototypes.available)
                buffer_json_member_add_uint64(wb, "available", b->prototypes.available);
            }
            buffer_json_object_close(wb);
        }
        dfe_done(b);
    }
    buffer_json_array_close(wb);
}

static void contexts_v2_alert_instances_to_json(BUFFER *wb, const char *key, struct rrdcontext_to_json_v2_data *ctl, bool debug) {
    buffer_json_member_add_array(wb, key);
    {
        struct alert_instances_callback_data data = {
                .wb = wb,
                .ctl = ctl,
                .debug = debug,
        };
        dictionary_walkthrough_rw(ctl->alerts.alert_instances, DICTIONARY_LOCK_READ,
                                  contexts_v2_alert_instance_to_json_callback, &data);
    }
    buffer_json_array_close(wb); // alerts_instances
}

static void contexts_v2_alerts_to_json(BUFFER *wb, struct rrdcontext_to_json_v2_data *ctl, bool debug) {
    if(ctl->request->options & CONTEXT_V2_OPTION_ALERTS_WITH_SUMMARY) {
        buffer_json_member_add_array(wb, "alerts");
        {
            struct alert_v2_entry *t;
            dfe_start_read(ctl->alerts.summary, t)
                    {
                        buffer_json_add_array_item_object(wb);
                        {
                            buffer_json_member_add_uint64(wb, "ati", t->ati);
                            buffer_json_member_add_string(wb, "nm", string2str(t->name));
                            buffer_json_member_add_string(wb, "sum", string2str(t->summary));

                            buffer_json_member_add_uint64(wb, "cr", t->counts.critical);
                            buffer_json_member_add_uint64(wb, "wr", t->counts.warning);
                            buffer_json_member_add_uint64(wb, "cl", t->counts.clear);
                            buffer_json_member_add_uint64(wb, "er", t->counts.error);

                            buffer_json_member_add_uint64(wb, "in", t->instances);
                            buffer_json_member_add_uint64(wb, "nd", dictionary_entries(t->nodes));
                            buffer_json_member_add_uint64(wb, "cfg", dictionary_entries(t->configs));
                        }
                        buffer_json_object_close(wb); // alert name
                    }
            dfe_done(t);
        }
        buffer_json_array_close(wb); // alerts

        health_prototype_metadata_foreach(ctl, contexts_v2_alerts_by_x_update_prototypes);
        contexts_v2_alerts_by_x_to_json(wb, ctl->alerts.by_type, "alerts_by_type");
        contexts_v2_alerts_by_x_to_json(wb, ctl->alerts.by_component, "alerts_by_component");
        contexts_v2_alerts_by_x_to_json(wb, ctl->alerts.by_classification, "alerts_by_classification");
        contexts_v2_alerts_by_x_to_json(wb, ctl->alerts.by_recipient, "alerts_by_recipient");
        contexts_v2_alerts_by_x_to_json(wb, ctl->alerts.by_module, "alerts_by_module");
    }

    if(ctl->request->options & (CONTEXT_V2_OPTION_ALERTS_WITH_INSTANCES|CONTEXT_V2_OPTION_ALERTS_WITH_VALUES)) {
        contexts_v2_alert_instances_to_json(wb, "alert_instances", ctl, debug);
    }
}

#define SQL_TRANSITION_DATA_SMALL_STRING (6 * 8)
#define SQL_TRANSITION_DATA_MEDIUM_STRING (12 * 8)
#define SQL_TRANSITION_DATA_BIG_STRING 512

struct sql_alert_transition_fixed_size {
    usec_t global_id;
    uuid_t transition_id;
    uuid_t host_id;
    uuid_t config_hash_id;
    uint32_t alarm_id;
    char alert_name[SQL_TRANSITION_DATA_SMALL_STRING];
    char chart[RRD_ID_LENGTH_MAX];
    char chart_name[RRD_ID_LENGTH_MAX];
    char chart_context[SQL_TRANSITION_DATA_MEDIUM_STRING];
    char family[SQL_TRANSITION_DATA_SMALL_STRING];
    char recipient[SQL_TRANSITION_DATA_MEDIUM_STRING];
    char units[SQL_TRANSITION_DATA_SMALL_STRING];
    char exec[SQL_TRANSITION_DATA_BIG_STRING];
    char info[SQL_TRANSITION_DATA_BIG_STRING];
    char summary[SQL_TRANSITION_DATA_BIG_STRING];
    char classification[SQL_TRANSITION_DATA_SMALL_STRING];
    char type[SQL_TRANSITION_DATA_SMALL_STRING];
    char component[SQL_TRANSITION_DATA_SMALL_STRING];
    time_t when_key;
    time_t duration;
    time_t non_clear_duration;
    uint64_t flags;
    time_t delay_up_to_timestamp;
    time_t exec_run_timestamp;
    int exec_code;
    int new_status;
    int old_status;
    int delay;
    time_t last_repeat;
    NETDATA_DOUBLE new_value;
    NETDATA_DOUBLE old_value;

    char machine_guid[UUID_STR_LEN];
    struct sql_alert_transition_fixed_size *next;
    struct sql_alert_transition_fixed_size *prev;
};

static struct sql_alert_transition_fixed_size *contexts_v2_alert_transition_dup(struct sql_alert_transition_data *t, const char *machine_guid, struct sql_alert_transition_fixed_size *dst) {
    struct sql_alert_transition_fixed_size *n = dst ? dst : mallocz(sizeof(*n));

    n->global_id = t->global_id;
    uuid_copy(n->transition_id, *t->transition_id);
    uuid_copy(n->host_id, *t->host_id);
    uuid_copy(n->config_hash_id, *t->config_hash_id);
    n->alarm_id = t->alarm_id;
    strncpyz(n->alert_name, t->alert_name ? t->alert_name : "", sizeof(n->alert_name) - 1);
    strncpyz(n->chart, t->chart ? t->chart : "", sizeof(n->chart) - 1);
    strncpyz(n->chart_name, t->chart_name ? t->chart_name : n->chart, sizeof(n->chart_name) - 1);
    strncpyz(n->chart_context, t->chart_context ? t->chart_context : "", sizeof(n->chart_context) - 1);
    strncpyz(n->family, t->family ? t->family : "", sizeof(n->family) - 1);
    strncpyz(n->recipient, t->recipient ? t->recipient : "", sizeof(n->recipient) - 1);
    strncpyz(n->units, t->units ? t->units : "", sizeof(n->units) - 1);
    strncpyz(n->exec, t->exec ? t->exec : "", sizeof(n->exec) - 1);
    strncpyz(n->info, t->info ? t->info : "", sizeof(n->info) - 1);
    strncpyz(n->summary, t->summary ? t->summary : "", sizeof(n->summary) - 1);
    strncpyz(n->classification, t->classification ? t->classification : "", sizeof(n->classification) - 1);
    strncpyz(n->type, t->type ? t->type : "", sizeof(n->type) - 1);
    strncpyz(n->component, t->component ? t->component : "", sizeof(n->component) - 1);
    n->when_key = t->when_key;
    n->duration = t->duration;
    n->non_clear_duration = t->non_clear_duration;
    n->flags = t->flags;
    n->delay_up_to_timestamp = t->delay_up_to_timestamp;
    n->exec_run_timestamp = t->exec_run_timestamp;
    n->exec_code = t->exec_code;
    n->new_status = t->new_status;
    n->old_status = t->old_status;
    n->delay = t->delay;
    n->last_repeat = t->last_repeat;
    n->new_value = t->new_value;
    n->old_value = t->old_value;

    memcpy(n->machine_guid, machine_guid, sizeof(n->machine_guid));
    n->next = n->prev = NULL;

    return n;
}

static void contexts_v2_alert_transition_free(struct sql_alert_transition_fixed_size *t) {
    freez(t);
}

static inline void contexts_v2_alert_transition_keep(struct alert_transitions_callback_data *d, struct sql_alert_transition_data *t, const char *machine_guid) {
    d->items_matched++;

    if(unlikely(t->global_id <= d->ctl->request->alerts.global_id_anchor)) {
        // this is in our past, we are not interested
        d->operations.skips_before++;
        return;
    }

    if(unlikely(!d->base)) {
        d->last_added = contexts_v2_alert_transition_dup(t, machine_guid, NULL);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(d->base, d->last_added, prev, next);
        d->items_to_return++;
        d->operations.first++;
        return;
    }

    struct sql_alert_transition_fixed_size *last = d->last_added;
    while(last->prev != d->base->prev && t->global_id > last->prev->global_id) {
        last = last->prev;
        d->operations.backwards++;
    }

    while(last->next && t->global_id < last->next->global_id) {
        last = last->next;
        d->operations.forwards++;
    }

    if(d->items_to_return >= d->max_items_to_return) {
        if(last == d->base->prev && t->global_id < last->global_id) {
            d->operations.skips_after++;
            return;
        }
    }

    d->items_to_return++;

    if(t->global_id > last->global_id) {
        if(d->items_to_return > d->max_items_to_return) {
            d->items_to_return--;
            d->operations.shifts++;
            d->last_added = d->base->prev;
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(d->base, d->last_added, prev, next);
            d->last_added = contexts_v2_alert_transition_dup(t, machine_guid, d->last_added);
        }
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(d->base, d->last_added, prev, next);
        d->operations.prepend++;
    }
    else {
        d->last_added = contexts_v2_alert_transition_dup(t, machine_guid, NULL);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(d->base, d->last_added, prev, next);
        d->operations.append++;
    }

    while(d->items_to_return > d->max_items_to_return) {
        // we have to remove something

        struct sql_alert_transition_fixed_size *tmp = d->base->prev;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(d->base, tmp, prev, next);
        d->items_to_return--;

        if(unlikely(d->last_added == tmp))
            d->last_added = d->base;

        contexts_v2_alert_transition_free(tmp);

        d->operations.shifts++;
    }
}

static void contexts_v2_alert_transition_callback(struct sql_alert_transition_data *t, void *data) {
    struct alert_transitions_callback_data *d = data;
    d->items_evaluated++;

    char machine_guid[UUID_STR_LEN] = "";
    uuid_unparse_lower(*t->host_id, machine_guid);

    const char *facets[ATF_TOTAL_ENTRIES] = {
            [ATF_STATUS] = rrdcalc_status2string(t->new_status),
            [ATF_CLASS] = t->classification,
            [ATF_TYPE] = t->type,
            [ATF_COMPONENT] = t->component,
            [ATF_ROLE] = t->recipient && *t->recipient ? t->recipient : string2str(localhost->health.health_default_recipient),
            [ATF_NODE] = machine_guid,
            [ATF_ALERT_NAME] = t->alert_name,
            [ATF_CHART_NAME] = t->chart_name,
            [ATF_CONTEXT] = t->chart_context,
    };

    for(size_t i = 0; i < ATF_TOTAL_ENTRIES ;i++) {
        if (!facets[i] || !*facets[i]) facets[i] = "unknown";

        struct facet_entry tmp = {
                .count = 0,
        };
        dictionary_set(d->facets[i].dict, facets[i], &tmp, sizeof(tmp));
    }

    bool selected[ATF_TOTAL_ENTRIES] = { 0 };

    uint32_t selected_by = 0;
    for(size_t i = 0; i < ATF_TOTAL_ENTRIES ;i++) {
        selected[i] = !d->facets[i].pattern || simple_pattern_matches(d->facets[i].pattern, facets[i]);
        if(selected[i])
            selected_by++;
    }

    if(selected_by == ATF_TOTAL_ENTRIES) {
        // this item is selected by all facets
        // put it in our result (if it fits)
        contexts_v2_alert_transition_keep(d, t, machine_guid);
    }

    if(selected_by >= ATF_TOTAL_ENTRIES - 1) {
        // this item is selected by all, or all except one facet
        // in both cases we need to add it to our counters

        for (size_t i = 0; i < ATF_TOTAL_ENTRIES; i++) {
            uint32_t counted_by = selected_by;

            if (counted_by != ATF_TOTAL_ENTRIES) {
                counted_by = 0;
                for (size_t j = 0; j < ATF_TOTAL_ENTRIES; j++) {
                    if (i == j || selected[j])
                        counted_by++;
                }
            }

            if (counted_by == ATF_TOTAL_ENTRIES) {
                // we need to count it on this facet
                struct facet_entry *x = dictionary_get(d->facets[i].dict, facets[i]);
                internal_fatal(!x, "facet is not found");
                if(x)
                    x->count++;
            }
        }
    }
}

static void contexts_v2_alert_transitions_to_json(BUFFER *wb, struct rrdcontext_to_json_v2_data *ctl, bool debug) {
    struct alert_transitions_callback_data data = {
            .wb = wb,
            .ctl = ctl,
            .debug = debug,
            .only_one_config = true,
            .max_items_to_return = ctl->request->alerts.last,
            .items_to_return = 0,
            .base = NULL,
    };

    for(size_t i = 0; i < ATF_TOTAL_ENTRIES ;i++) {
        data.facets[i].dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, sizeof(struct facet_entry));
        if(ctl->request->alerts.facets[i])
            data.facets[i].pattern = simple_pattern_create(ctl->request->alerts.facets[i], ",|", SIMPLE_PATTERN_EXACT, false);
    }

    sql_alert_transitions(
        ctl->nodes.dict,
        ctl->window.after,
        ctl->window.before,
        ctl->request->contexts,
        ctl->request->alerts.alert,
        ctl->request->alerts.transition,
        contexts_v2_alert_transition_callback,
        &data,
        debug);

    buffer_json_member_add_array(wb, "facets");
    for (size_t i = 0; i < ATF_TOTAL_ENTRIES; i++) {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", alert_transition_facets[i].id);
            buffer_json_member_add_string(wb, "name", alert_transition_facets[i].name);
            buffer_json_member_add_uint64(wb, "order", alert_transition_facets[i].order);
            buffer_json_member_add_array(wb, "options");
            {
                struct facet_entry *x;
                dfe_start_read(data.facets[i].dict, x) {
                    buffer_json_add_array_item_object(wb);
                    {
                        buffer_json_member_add_string(wb, "id", x_dfe.name);
                        if (i == ATF_NODE) {
                            RRDHOST *host = rrdhost_find_by_guid(x_dfe.name);
                            if (host)
                                buffer_json_member_add_string(wb, "name", rrdhost_hostname(host));
                            else
                                buffer_json_member_add_string(wb, "name", x_dfe.name);
                        } else
                            buffer_json_member_add_string(wb, "name", x_dfe.name);
                        buffer_json_member_add_uint64(wb, "count", x->count);
                    }
                    buffer_json_object_close(wb);
                }
                dfe_done(x);
            }
            buffer_json_array_close(wb); // options
        }
        buffer_json_object_close(wb); // facet
    }
    buffer_json_array_close(wb); // facets

    buffer_json_member_add_array(wb, "transitions");
    for(struct sql_alert_transition_fixed_size *t = data.base; t ; t = t->next) {
        buffer_json_add_array_item_object(wb);
        {
            RRDHOST *host = rrdhost_find_by_guid(t->machine_guid);

            buffer_json_member_add_uint64(wb, "gi", t->global_id);
            buffer_json_member_add_uuid(wb, "transition_id", &t->transition_id);
            buffer_json_member_add_uuid(wb, "config_hash_id", &t->config_hash_id);
            buffer_json_member_add_string(wb, "machine_guid", t->machine_guid);

            if(host) {
                buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));

                if(host->node_id)
                    buffer_json_member_add_uuid(wb, "node_id", host->node_id);
            }

            buffer_json_member_add_string(wb, "alert", *t->alert_name ? t->alert_name : NULL);
            buffer_json_member_add_string(wb, "instance", *t->chart ? t->chart : NULL);
            buffer_json_member_add_string(wb, "instance_n", *t->chart_name ? t->chart_name : NULL);
            buffer_json_member_add_string(wb, "context", *t->chart_context ? t->chart_context : NULL);
            // buffer_json_member_add_string(wb, "family", *t->family ? t->family : NULL);
            buffer_json_member_add_string(wb, "component", *t->component ? t->component : NULL);
            buffer_json_member_add_string(wb, "classification", *t->classification ? t->classification : NULL);
            buffer_json_member_add_string(wb, "type", *t->type ? t->type : NULL);

            buffer_json_member_add_time_t(wb, "when", t->when_key);
            buffer_json_member_add_string(wb, "info", *t->info ? t->info : "");
            buffer_json_member_add_string(wb, "summary", *t->summary ? t->summary : "");
            buffer_json_member_add_string(wb, "units", *t->units ? t->units : NULL);
            buffer_json_member_add_object(wb, "new");
            {
                buffer_json_member_add_string(wb, "status", rrdcalc_status2string(t->new_status));
                buffer_json_member_add_double(wb, "value", t->new_value);
            }
            buffer_json_object_close(wb); // new
            buffer_json_member_add_object(wb, "old");
            {
                buffer_json_member_add_string(wb, "status", rrdcalc_status2string(t->old_status));
                buffer_json_member_add_double(wb, "value", t->old_value);
                buffer_json_member_add_time_t(wb, "duration", t->duration);
                buffer_json_member_add_time_t(wb, "raised_duration", t->non_clear_duration);
            }
            buffer_json_object_close(wb); // old

            buffer_json_member_add_object(wb, "notification");
            {
                buffer_json_member_add_time_t(wb, "when", t->exec_run_timestamp);
                buffer_json_member_add_time_t(wb, "delay", t->delay);
                buffer_json_member_add_time_t(wb, "delay_up_to_time", t->delay_up_to_timestamp);
                health_entry_flags_to_json_array(wb, "flags", t->flags);
                buffer_json_member_add_string(wb, "exec", *t->exec ? t->exec : string2str(localhost->health.health_default_exec));
                buffer_json_member_add_uint64(wb, "exec_code", t->exec_code);
                buffer_json_member_add_string(wb, "to", *t->recipient ? t->recipient : string2str(localhost->health.health_default_recipient));
            }
            buffer_json_object_close(wb); // notification
        }
        buffer_json_object_close(wb); // a transition
    }
    buffer_json_array_close(wb); // all transitions

    if(ctl->options & CONTEXT_V2_OPTION_ALERTS_WITH_CONFIGURATIONS) {
        DICTIONARY *configs = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

        for(struct sql_alert_transition_fixed_size *t = data.base; t ; t = t->next) {
            char guid[UUID_STR_LEN];
            uuid_unparse_lower(t->config_hash_id, guid);
            dictionary_set(configs, guid, NULL, 0);
        }

        buffer_json_member_add_array(wb, "configurations");
        sql_get_alert_configuration(configs, contexts_v2_alert_config_to_json_from_sql_alert_config_data, &data, debug);
        buffer_json_array_close(wb);

        dictionary_destroy(configs);
    }

    while(data.base) {
        struct sql_alert_transition_fixed_size *t = data.base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(data.base, t, prev, next);
        contexts_v2_alert_transition_free(t);
    }

    for(size_t i = 0; i < ATF_TOTAL_ENTRIES ;i++) {
        dictionary_destroy(data.facets[i].dict);
        simple_pattern_free(data.facets[i].pattern);
    }

    buffer_json_member_add_object(wb, "items");
    {
        // all the items in the window, under the scope_nodes, ignoring the facets (filters)
        buffer_json_member_add_uint64(wb, "evaluated", data.items_evaluated);

        // all the items matching the query (if you didn't put anchor_gi and last, these are all the items you would get back)
        buffer_json_member_add_uint64(wb, "matched", data.items_matched);

        // the items included in this response
        buffer_json_member_add_uint64(wb, "returned", data.items_to_return);

        // same as last=X parameter
        buffer_json_member_add_uint64(wb, "max_to_return", data.max_items_to_return);

        // items before the first returned, this should be 0 if anchor_gi is not set
        buffer_json_member_add_uint64(wb, "before", data.operations.skips_before);

        // items after the last returned, when this is zero there aren't any items after the current list
        buffer_json_member_add_uint64(wb, "after", data.operations.skips_after + data.operations.shifts);
    }
    buffer_json_object_close(wb); // items

    if(debug) {
        buffer_json_member_add_object(wb, "stats");
        {
            buffer_json_member_add_uint64(wb, "first", data.operations.first);
            buffer_json_member_add_uint64(wb, "prepend", data.operations.prepend);
            buffer_json_member_add_uint64(wb, "append", data.operations.append);
            buffer_json_member_add_uint64(wb, "backwards", data.operations.backwards);
            buffer_json_member_add_uint64(wb, "forwards", data.operations.forwards);
            buffer_json_member_add_uint64(wb, "shifts", data.operations.shifts);
            buffer_json_member_add_uint64(wb, "skips_before", data.operations.skips_before);
            buffer_json_member_add_uint64(wb, "skips_after", data.operations.skips_after);
        }
        buffer_json_object_close(wb);
    }
}

int rrdcontext_to_json_v2(BUFFER *wb, struct api_v2_contexts_request *req, CONTEXTS_V2_MODE mode) {
    int resp = HTTP_RESP_OK;
    bool run = true;

    if(mode & CONTEXTS_V2_SEARCH)
        mode |= CONTEXTS_V2_CONTEXTS;

    if(mode & (CONTEXTS_V2_AGENTS_INFO))
        mode |= CONTEXTS_V2_AGENTS;

    if(mode & (CONTEXTS_V2_FUNCTIONS | CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH | CONTEXTS_V2_NODES_INFO | CONTEXTS_V2_NODE_INSTANCES))
        mode |= CONTEXTS_V2_NODES;

    if(mode & CONTEXTS_V2_ALERTS) {
        mode |= CONTEXTS_V2_NODES;
        req->options &= ~CONTEXT_V2_OPTION_ALERTS_WITH_CONFIGURATIONS;

        if(!(req->options & (CONTEXT_V2_OPTION_ALERTS_WITH_SUMMARY|CONTEXT_V2_OPTION_ALERTS_WITH_INSTANCES|CONTEXT_V2_OPTION_ALERTS_WITH_VALUES)))
            req->options |= CONTEXT_V2_OPTION_ALERTS_WITH_SUMMARY;
    }

    if(mode & CONTEXTS_V2_ALERT_TRANSITIONS) {
        mode |= CONTEXTS_V2_NODES;
        req->options &= ~CONTEXT_V2_OPTION_ALERTS_WITH_INSTANCES;
    }

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
            ctl.options |= CONTEXT_V2_OPTION_ALERTS_WITH_INSTANCES|CONTEXT_V2_OPTION_ALERTS_WITH_VALUES;
            run = sql_find_alert_transition(req->alerts.transition, rrdcontext_v2_set_transition_filter, &ctl);
            if(!run) {
                resp = HTTP_RESP_NOT_FOUND;
                goto cleanup;
            }
        }

        ctl.alerts.summary = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                       NULL, sizeof(struct alert_v2_entry));

        dictionary_register_insert_callback(ctl.alerts.summary, alerts_v2_insert_callback, &ctl);
        dictionary_register_conflict_callback(ctl.alerts.summary, alerts_v2_conflict_callback, &ctl);
        dictionary_register_delete_callback(ctl.alerts.summary, alerts_v2_delete_callback, &ctl);

        ctl.alerts.by_type = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                        NULL, sizeof(struct alert_by_x_entry));

        dictionary_register_insert_callback(ctl.alerts.by_type, alerts_by_x_insert_callback, NULL);
        dictionary_register_conflict_callback(ctl.alerts.by_type, alerts_by_x_conflict_callback, NULL);

        ctl.alerts.by_component = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                        NULL, sizeof(struct alert_by_x_entry));

        dictionary_register_insert_callback(ctl.alerts.by_component, alerts_by_x_insert_callback, NULL);
        dictionary_register_conflict_callback(ctl.alerts.by_component, alerts_by_x_conflict_callback, NULL);

        ctl.alerts.by_classification = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                          NULL, sizeof(struct alert_by_x_entry));

        dictionary_register_insert_callback(ctl.alerts.by_classification, alerts_by_x_insert_callback, NULL);
        dictionary_register_conflict_callback(ctl.alerts.by_classification, alerts_by_x_conflict_callback, NULL);

        ctl.alerts.by_recipient = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                          NULL, sizeof(struct alert_by_x_entry));

        dictionary_register_insert_callback(ctl.alerts.by_recipient, alerts_by_x_insert_callback, NULL);
        dictionary_register_conflict_callback(ctl.alerts.by_recipient, alerts_by_x_conflict_callback, NULL);

        ctl.alerts.by_module = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                             NULL, sizeof(struct alert_by_x_entry));

        dictionary_register_insert_callback(ctl.alerts.by_module, alerts_by_x_insert_callback, NULL);
        dictionary_register_conflict_callback(ctl.alerts.by_module, alerts_by_x_conflict_callback, NULL);

        if(ctl.options & (CONTEXT_V2_OPTION_ALERTS_WITH_INSTANCES | CONTEXT_V2_OPTION_ALERTS_WITH_VALUES)) {
            ctl.alerts.alert_instances = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                                    NULL, sizeof(struct sql_alert_instance_v2_entry));

            dictionary_register_insert_callback(ctl.alerts.alert_instances, alert_instances_v2_insert_callback, &ctl);
            dictionary_register_conflict_callback(ctl.alerts.alert_instances, alert_instances_v2_conflict_callback, &ctl);
            dictionary_register_delete_callback(ctl.alerts.alert_instances, alert_instances_delete_callback, &ctl);
        }
    }

    if(req->after || req->before) {
        ctl.window.relative = rrdr_relative_window_to_absolute_query(&ctl.window.after, &ctl.window.before, &ctl.now
                                                                     , false
                                                                    );
        ctl.window.enabled = !(mode & CONTEXTS_V2_ALERT_TRANSITIONS);
    }
    else
        ctl.now = now_realtime_sec();

    buffer_json_initialize(wb, "\"", "\"", 0, true,
                           ((req->options & CONTEXT_V2_OPTION_MINIFY) && !(req->options & CONTEXT_V2_OPTION_DEBUG)) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);

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

                if(mode & (CONTEXTS_V2_ALERTS | CONTEXTS_V2_ALERT_TRANSITIONS)) {
                    buffer_json_member_add_object(wb, "alerts");

                    if(mode & CONTEXTS_V2_ALERTS)
                        web_client_api_request_v2_contexts_alerts_status_to_buffer_json_array(wb, "status", req->alerts.status);

                    if(mode & CONTEXTS_V2_ALERT_TRANSITIONS) {
                        buffer_json_member_add_string(wb, "context", req->contexts);
                        buffer_json_member_add_uint64(wb, "anchor_gi", req->alerts.global_id_anchor);
                        buffer_json_member_add_uint64(wb, "last", req->alerts.last);
                    }

                    buffer_json_member_add_string(wb, "alert", req->alerts.alert);
                    buffer_json_member_add_string(wb, "transition", req->alerts.transition);
                    buffer_json_object_close(wb); // alerts
                }
            }
            buffer_json_object_close(wb); // selectors

            buffer_json_member_add_object(wb, "filters");
            {
                if (mode & CONTEXTS_V2_SEARCH)
                    buffer_json_member_add_string(wb, "q", req->q);

                buffer_json_member_add_time_t(wb, "after", req->after);
                buffer_json_member_add_time_t(wb, "before", req->before);
            }
            buffer_json_object_close(wb); // filters

            if(mode & CONTEXTS_V2_ALERT_TRANSITIONS) {
                buffer_json_member_add_object(wb, "facets");
                {
                    for (int i = 0; i < ATF_TOTAL_ENTRIES; i++) {
                        buffer_json_member_add_string(wb, alert_transition_facets[i].query_param, req->alerts.facets[i]);
                    }
                }
                buffer_json_object_close(wb); // facets
            }
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
            resp = HTTP_RESP_CLIENT_CLOSED_REQUEST;
        }
        goto cleanup;
    }

    ctl.timings.executed_ut = now_monotonic_usec();

    if(mode & CONTEXTS_V2_ALERT_TRANSITIONS) {
        contexts_v2_alert_transitions_to_json(wb, &ctl, debug);
    }
    else {
        if (mode & CONTEXTS_V2_NODES) {
            buffer_json_member_add_array(wb, "nodes");
            struct contexts_v2_node *t;
            dfe_start_read(ctl.nodes.dict, t) {
                rrdcontext_to_json_v2_rrdhost(wb, t->host, &ctl, t->ni);
            }
            dfe_done(t);
            buffer_json_array_close(wb);
        }

        if (mode & CONTEXTS_V2_FUNCTIONS) {
            buffer_json_member_add_array(wb, "functions");
            {
                struct function_v2_entry *t;
                dfe_start_read(ctl.functions.dict, t) {
                    buffer_json_add_array_item_object(wb);
                    {
                        buffer_json_member_add_string(wb, "name", t_dfe.name);
                        buffer_json_member_add_string(wb, "help", string2str(t->help));
                        buffer_json_member_add_array(wb, "ni");
                        {
                            for (size_t i = 0; i < t->used; i++)
                                buffer_json_add_array_item_uint64(wb, t->node_ids[i]);
                        }
                        buffer_json_array_close(wb);
                        buffer_json_member_add_string(wb, "tags", string2str(t->tags));
                        http_access2buffer_json_array(wb, "access", t->access);
                        buffer_json_member_add_uint64(wb, "priority", t->priority);
                    }
                    buffer_json_object_close(wb);
                }
                dfe_done(t);
            }
            buffer_json_array_close(wb);
        }

        if (mode & CONTEXTS_V2_CONTEXTS) {
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

        if (mode & CONTEXTS_V2_ALERTS)
            contexts_v2_alerts_to_json(wb, &ctl, debug);

        if (mode & CONTEXTS_V2_SEARCH) {
            buffer_json_member_add_object(wb, "searches");
            {
                buffer_json_member_add_uint64(wb, "strings", ctl.q.fts.string_searches);
                buffer_json_member_add_uint64(wb, "char", ctl.q.fts.char_searches);
                buffer_json_member_add_uint64(wb, "total", ctl.q.fts.searches);
            }
            buffer_json_object_close(wb);
        }

        if (mode & (CONTEXTS_V2_VERSIONS))
            version_hashes_api_v2(wb, &ctl.versions);

        if (mode & CONTEXTS_V2_AGENTS)
            buffer_json_agents_v2(wb, &ctl.timings, ctl.now, mode & (CONTEXTS_V2_AGENTS_INFO), true);
    }

    buffer_json_cloud_timings(wb, "timings", &ctl.timings);

    buffer_json_finalize(wb);

cleanup:
    dictionary_destroy(ctl.nodes.dict);
    dictionary_destroy(ctl.contexts.dict);
    dictionary_destroy(ctl.functions.dict);
    dictionary_destroy(ctl.alerts.summary);
    dictionary_destroy(ctl.alerts.alert_instances);
    dictionary_destroy(ctl.alerts.by_type);
    dictionary_destroy(ctl.alerts.by_component);
    dictionary_destroy(ctl.alerts.by_classification);
    dictionary_destroy(ctl.alerts.by_recipient);
    dictionary_destroy(ctl.alerts.by_module);
    simple_pattern_free(ctl.nodes.scope_pattern);
    simple_pattern_free(ctl.nodes.pattern);
    simple_pattern_free(ctl.contexts.pattern);
    simple_pattern_free(ctl.contexts.scope_pattern);
    simple_pattern_free(ctl.q.pattern);
    simple_pattern_free(ctl.alerts.alert_name_pattern);

    return resp;
}
