// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_contexts_alerts.h"

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

#define SQL_TRANSITION_DATA_SMALL_STRING (6 * 8)
#define SQL_TRANSITION_DATA_MEDIUM_STRING (12 * 8)
#define SQL_TRANSITION_DATA_BIG_STRING 512

struct sql_alert_transition_fixed_size {
    usec_t global_id;
    nd_uuid_t transition_id;
    nd_uuid_t host_id;
    nd_uuid_t config_hash_id;
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

struct facet_entry {
    uint32_t count;
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
        [ATF_ROLE] = t->recipient && *t->recipient ? t->recipient : string2str(localhost->health.default_recipient),
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

void contexts_v2_alert_transitions_to_json(BUFFER *wb, struct rrdcontext_to_json_v2_data *ctl, bool debug) {
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
            buffer_json_member_add_uuid(wb, "transition_id", t->transition_id);
            buffer_json_member_add_uuid(wb, "config_hash_id", t->config_hash_id);
            buffer_json_member_add_string(wb, "machine_guid", t->machine_guid);

            if(host) {
                buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));

                if(!UUIDiszero(host->node_id))
                    buffer_json_member_add_uuid(wb, "node_id", host->node_id.uuid);
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
                buffer_json_member_add_string(wb, "exec", *t->exec ? t->exec : string2str(localhost->health.default_exec));
                buffer_json_member_add_uint64(wb, "exec_code", t->exec_code);
                buffer_json_member_add_string(wb, "to", *t->recipient ? t->recipient : string2str(localhost->health.default_recipient));
            }
            buffer_json_object_close(wb); // notification
        }
        buffer_json_object_close(wb); // a transition
    }
    buffer_json_array_close(wb); // all transitions

    if(ctl->options & CONTEXTS_OPTION_ALERTS_WITH_CONFIGURATIONS) {
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
