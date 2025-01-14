// SPDX-License-Identifier: GPL-3.0-or-later

#include "internal.h"

static void rrd_flags_to_buffer_json_array_items(RRD_FLAGS flags, BUFFER *wb) {
    if(flags & RRD_FLAG_QUEUED_FOR_HUB)
        buffer_json_add_array_item_string(wb, "QUEUED");

    if(flags & RRD_FLAG_DELETED)
        buffer_json_add_array_item_string(wb, "DELETED");

    if(flags & RRD_FLAG_COLLECTED)
        buffer_json_add_array_item_string(wb, "COLLECTED");

    if(flags & RRD_FLAG_UPDATED)
        buffer_json_add_array_item_string(wb, "UPDATED");

    if(flags & RRD_FLAG_ARCHIVED)
        buffer_json_add_array_item_string(wb, "ARCHIVED");

    if(flags & RRD_FLAG_OWN_LABELS)
        buffer_json_add_array_item_string(wb, "OWN_LABELS");

    if(flags & RRD_FLAG_LIVE_RETENTION)
        buffer_json_add_array_item_string(wb, "LIVE_RETENTION");

    if(flags & RRD_FLAG_HIDDEN)
        buffer_json_add_array_item_string(wb, "HIDDEN");

    if(flags & RRD_FLAG_QUEUED_FOR_PP)
        buffer_json_add_array_item_string(wb, "PENDING_UPDATES");
}

// ----------------------------------------------------------------------------
// /api/v1/context(s) API

struct rrdcontext_to_json {
    BUFFER *wb;
    RRDCONTEXT_TO_JSON_OPTIONS options;
    time_t after;
    time_t before;
    SIMPLE_PATTERN *chart_label_key;
    SIMPLE_PATTERN *chart_labels_filter;
    SIMPLE_PATTERN *chart_dimensions;
    size_t written;
    time_t now;
    time_t combined_first_time_s;
    time_t combined_last_time_s;
    RRD_FLAGS combined_flags;
};

static inline int rrdmetric_to_json_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *id = dictionary_acquired_item_name(item);
    struct rrdcontext_to_json * t = data;
    RRDMETRIC *rm = value;
    BUFFER *wb = t->wb;
    RRDCONTEXT_TO_JSON_OPTIONS options = t->options;
    time_t after = t->after;
    time_t before = t->before;

    if(unlikely(rrd_flag_is_deleted(rm) && !(options & RRDCONTEXT_OPTION_SHOW_DELETED)))
        return 0;

    if(after && (!rm->last_time_s || after > rm->last_time_s))
        return 0;

    if(before && (!rm->first_time_s || before < rm->first_time_s))
        return 0;

    if(t->chart_dimensions
       && !simple_pattern_matches_string(t->chart_dimensions, rm->id)
       && rm->name != rm->id
       && !simple_pattern_matches_string(t->chart_dimensions, rm->name))
        return 0;

    if(t->written) {
        t->combined_first_time_s = MIN(t->combined_first_time_s, rm->first_time_s);
        t->combined_last_time_s = MAX(t->combined_last_time_s, rm->last_time_s);
        t->combined_flags |= rrd_flags_get(rm);
    }
    else {
        t->combined_first_time_s = rm->first_time_s;
        t->combined_last_time_s = rm->last_time_s;
        t->combined_flags = rrd_flags_get(rm);
    }

    buffer_json_member_add_object(wb, id);

    if(options & RRDCONTEXT_OPTION_SHOW_UUIDS) {
        char uuid[UUID_STR_LEN];
        uuid_unparse(*uuidmap_uuid_ptr(rm->uuid), uuid);
        buffer_json_member_add_string(wb, "uuid", uuid);
    }

    buffer_json_member_add_string(wb, "name", string2str(rm->name));
    buffer_json_member_add_time_t(wb, "first_time_t", rm->first_time_s);
    buffer_json_member_add_time_t(wb, "last_time_t", rrd_flag_is_collected(rm) ? (long long)t->now : (long long)rm->last_time_s);
    buffer_json_member_add_boolean(wb, "collected", rrd_flag_is_collected(rm));

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED)
        buffer_json_member_add_boolean(wb, "deleted", rrd_flag_is_deleted(rm));

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_json_member_add_array(wb, "flags");
        rrd_flags_to_buffer_json_array_items(rrd_flags_get(rm), wb);
        buffer_json_array_close(wb);
    }

    buffer_json_object_close(wb);
    t->written++;
    return 1;
}

static inline int rrdinstance_to_json_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *id = dictionary_acquired_item_name(item);

    struct rrdcontext_to_json *t_parent = data;
    RRDINSTANCE *ri = value;
    BUFFER *wb = t_parent->wb;
    RRDCONTEXT_TO_JSON_OPTIONS options = t_parent->options;
    time_t after = t_parent->after;
    time_t before = t_parent->before;
    bool has_filter = t_parent->chart_label_key || t_parent->chart_labels_filter || t_parent->chart_dimensions;

    if(unlikely(rrd_flag_is_deleted(ri) && !(options & RRDCONTEXT_OPTION_SHOW_DELETED)))
        return 0;

    if(after && (!ri->last_time_s || after > ri->last_time_s))
        return 0;

    if(before && (!ri->first_time_s || before < ri->first_time_s))
        return 0;

    if(t_parent->chart_label_key && rrdlabels_match_simple_pattern_parsed(ri->rrdlabels, t_parent->chart_label_key,
                                                                           '\0', NULL) != SP_MATCHED_POSITIVE)
        return 0;

    if(t_parent->chart_labels_filter && rrdlabels_match_simple_pattern_parsed(ri->rrdlabels,
                                                                               t_parent->chart_labels_filter, ':',
                                                                               NULL) != SP_MATCHED_POSITIVE)
        return 0;

    time_t first_time_s = ri->first_time_s;
    time_t last_time_s = ri->last_time_s;
    RRD_FLAGS flags = rrd_flags_get(ri);

    BUFFER *wb_metrics = NULL;
    if(options & RRDCONTEXT_OPTION_SHOW_METRICS || t_parent->chart_dimensions) {

        wb_metrics = buffer_create(4096, &netdata_buffers_statistics.buffers_api);
        buffer_json_initialize(wb_metrics, "\"", "\"", wb->json.depth + 2, false, BUFFER_JSON_OPTIONS_DEFAULT);

        struct rrdcontext_to_json t_metrics = {
                .wb = wb_metrics,
                .options = options,
                .chart_label_key = t_parent->chart_label_key,
                .chart_labels_filter = t_parent->chart_labels_filter,
                .chart_dimensions = t_parent->chart_dimensions,
                .after = after,
                .before = before,
                .written = 0,
                .now = t_parent->now,
        };
        dictionary_walkthrough_read(ri->rrdmetrics, rrdmetric_to_json_callback, &t_metrics);

        if(has_filter && !t_metrics.written) {
            buffer_free(wb_metrics);
            return 0;
        }

        first_time_s = t_metrics.combined_first_time_s;
        last_time_s = t_metrics.combined_last_time_s;
        flags = t_metrics.combined_flags;
    }

    if(t_parent->written) {
        t_parent->combined_first_time_s = MIN(t_parent->combined_first_time_s, first_time_s);
        t_parent->combined_last_time_s = MAX(t_parent->combined_last_time_s, last_time_s);
        t_parent->combined_flags |= flags;
    }
    else {
        t_parent->combined_first_time_s = first_time_s;
        t_parent->combined_last_time_s = last_time_s;
        t_parent->combined_flags = flags;
    }

    buffer_json_member_add_object(wb, id);

    if(options & RRDCONTEXT_OPTION_SHOW_UUIDS) {
        char uuid[UUID_STR_LEN];
        uuid_unparse(*uuidmap_uuid_ptr(ri->uuid), uuid);
        buffer_json_member_add_string(wb, "uuid", uuid);
    }

    buffer_json_member_add_string(wb, "name", string2str(ri->name));
    buffer_json_member_add_string(wb, "context", string2str(ri->rc->id));
    buffer_json_member_add_string(wb, "title", string2str(ri->title));
    buffer_json_member_add_string(wb, "units", string2str(ri->units));
    buffer_json_member_add_string(wb, "family", string2str(ri->family));
    buffer_json_member_add_string(wb, "chart_type", rrdset_type_name(ri->chart_type));
    buffer_json_member_add_uint64(wb, "priority", ri->priority);
    buffer_json_member_add_time_t(wb, "update_every", ri->update_every_s);
    buffer_json_member_add_time_t(wb, "first_time_t", first_time_s);
    buffer_json_member_add_time_t(wb, "last_time_t", (flags & RRD_FLAG_COLLECTED) ? (long long)t_parent->now : (long long)last_time_s);
    buffer_json_member_add_boolean(wb, "collected", flags & RRD_FLAG_COLLECTED);

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED)
        buffer_json_member_add_boolean(wb, "deleted", rrd_flag_is_deleted(ri));

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_json_member_add_array(wb, "flags");
        rrd_flags_to_buffer_json_array_items(rrd_flags_get(ri), wb);
        buffer_json_array_close(wb);
    }

    if(options & RRDCONTEXT_OPTION_SHOW_LABELS && ri->rrdlabels && rrdlabels_entries(ri->rrdlabels)) {
        buffer_json_member_add_object(wb, "labels");
        rrdlabels_to_buffer_json_members(ri->rrdlabels, wb);
        buffer_json_object_close(wb);
    }

    if(wb_metrics) {
        buffer_json_member_add_object(wb, "dimensions");
        buffer_fast_strcat(wb, buffer_tostring(wb_metrics), buffer_strlen(wb_metrics));
        buffer_json_object_close(wb);

        buffer_free(wb_metrics);
    }

    buffer_json_object_close(wb);
    t_parent->written++;
    return 1;
}

static inline int rrdcontext_to_json_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *id = dictionary_acquired_item_name(item);
    struct rrdcontext_to_json *t_parent = data;
    RRDCONTEXT *rc = value;
    BUFFER *wb = t_parent->wb;
    RRDCONTEXT_TO_JSON_OPTIONS options = t_parent->options;
    time_t after = t_parent->after;
    time_t before = t_parent->before;
    bool has_filter = t_parent->chart_label_key || t_parent->chart_labels_filter || t_parent->chart_dimensions;

    if(unlikely(rrd_flag_check(rc, RRD_FLAG_HIDDEN) && !(options & RRDCONTEXT_OPTION_SHOW_HIDDEN)))
        return 0;

    if(unlikely(rrd_flag_is_deleted(rc) && !(options & RRDCONTEXT_OPTION_SHOW_DELETED)))
        return 0;

    if(options & RRDCONTEXT_OPTION_DEEPSCAN)
        rrdcontext_recalculate_context_retention(rc, RRD_FLAG_NONE, false);

    if(after && (!rc->last_time_s || after > rc->last_time_s))
        return 0;

    if(before && (!rc->first_time_s || before < rc->first_time_s))
        return 0;

    time_t first_time_s = rc->first_time_s;
    time_t last_time_s = rc->last_time_s;
    RRD_FLAGS flags = rrd_flags_get(rc);

    BUFFER *wb_instances = NULL;
    if((options & (RRDCONTEXT_OPTION_SHOW_LABELS|RRDCONTEXT_OPTION_SHOW_INSTANCES|RRDCONTEXT_OPTION_SHOW_METRICS))
       || t_parent->chart_label_key
       || t_parent->chart_labels_filter
       || t_parent->chart_dimensions) {

        wb_instances = buffer_create(4096, &netdata_buffers_statistics.buffers_api);
        buffer_json_initialize(wb_instances, "\"", "\"", wb->json.depth + 2, false, BUFFER_JSON_OPTIONS_DEFAULT);

        struct rrdcontext_to_json t_instances = {
                .wb = wb_instances,
                .options = options,
                .chart_label_key = t_parent->chart_label_key,
                .chart_labels_filter = t_parent->chart_labels_filter,
                .chart_dimensions = t_parent->chart_dimensions,
                .after = after,
                .before = before,
                .written = 0,
                .now = t_parent->now,
        };
        dictionary_walkthrough_read(rc->rrdinstances, rrdinstance_to_json_callback, &t_instances);

        if(has_filter && !t_instances.written) {
            buffer_free(wb_instances);
            return 0;
        }

        first_time_s = t_instances.combined_first_time_s;
        last_time_s = t_instances.combined_last_time_s;
        flags = t_instances.combined_flags;
    }

    if(!(options & RRDCONTEXT_OPTION_SKIP_ID))
        buffer_json_member_add_object(wb, id);

    rrdcontext_lock(rc);

    buffer_json_member_add_string(wb, "title", string2str(rc->title));
    buffer_json_member_add_string(wb, "units", string2str(rc->units));
    buffer_json_member_add_string(wb, "family", string2str(rc->family));
    buffer_json_member_add_string(wb, "chart_type", rrdset_type_name(rc->chart_type));
    buffer_json_member_add_uint64(wb, "priority", rc->priority);
    buffer_json_member_add_time_t(wb, "first_time_t", first_time_s);
    buffer_json_member_add_time_t(wb, "last_time_t", (flags & RRD_FLAG_COLLECTED) ? (long long)t_parent->now : (long long)last_time_s);
    buffer_json_member_add_boolean(wb, "collected", (flags & RRD_FLAG_COLLECTED));

    if(options & RRDCONTEXT_OPTION_SHOW_DELETED)
        buffer_json_member_add_boolean(wb, "deleted", rrd_flag_is_deleted(rc));

    if(options & RRDCONTEXT_OPTION_SHOW_FLAGS) {
        buffer_json_member_add_array(wb, "flags");
        rrd_flags_to_buffer_json_array_items(rrd_flags_get(rc), wb);
        buffer_json_array_close(wb);
    }

    if(options & RRDCONTEXT_OPTION_SHOW_QUEUED) {
        buffer_json_member_add_array(wb, "queued_reasons");
        rrd_reasons_to_buffer_json_array_items(rc->queue.queued_flags, wb);
        buffer_json_array_close(wb);

        buffer_json_member_add_time_t(wb, "last_queued", (time_t)(rc->queue.queued_ut / USEC_PER_SEC));
        buffer_json_member_add_time_t(wb, "scheduled_dispatch", (time_t)(rc->queue.scheduled_dispatch_ut / USEC_PER_SEC));
        buffer_json_member_add_time_t(wb, "last_dequeued", (time_t)(rc->queue.dequeued_ut / USEC_PER_SEC));
        buffer_json_member_add_uint64(wb, "dispatches", rc->queue.dispatches);
        buffer_json_member_add_uint64(wb, "hub_version", rc->hub.version);
        buffer_json_member_add_uint64(wb, "version", rc->version);

        buffer_json_member_add_array(wb, "pp_reasons");
        rrd_reasons_to_buffer_json_array_items(rc->pp.queued_flags, wb);
        buffer_json_array_close(wb);

        buffer_json_member_add_time_t(wb, "pp_last_queued", (time_t)(rc->pp.queued_ut / USEC_PER_SEC));
        buffer_json_member_add_time_t(wb, "pp_last_dequeued", (time_t)(rc->pp.dequeued_ut / USEC_PER_SEC));
        buffer_json_member_add_uint64(wb, "pp_executed", rc->pp.executions);
    }

    rrdcontext_unlock(rc);

    if(wb_instances) {
        buffer_json_member_add_object(wb, "charts");
        buffer_fast_strcat(wb, buffer_tostring(wb_instances), buffer_strlen(wb_instances));
        buffer_json_object_close(wb);

        buffer_free(wb_instances);
    }

    if(!(options & RRDCONTEXT_OPTION_SKIP_ID))
        buffer_json_object_close(wb);

    t_parent->written++;
    return 1;
}

int rrdcontext_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options, const char *context, SIMPLE_PATTERN *chart_label_key, SIMPLE_PATTERN *chart_labels_filter, SIMPLE_PATTERN *chart_dimensions) {
    if(!host->rrdctx.contexts) {
        netdata_log_error("%s(): request for host '%s' that does not have rrdcontexts initialized.", __FUNCTION__, rrdhost_hostname(host));
        return HTTP_RESP_NOT_FOUND;
    }

    RRDCONTEXT_ACQUIRED *rca = (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdctx.contexts, context);
    if(!rca) return HTTP_RESP_NOT_FOUND;

    RRDCONTEXT *rc = rrdcontext_acquired_value(rca);

    if(after != 0 && before != 0)
        rrdr_relative_window_to_absolute_query(&after, &before, NULL, false);

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    struct rrdcontext_to_json t_contexts = {
            .wb = wb,
            .options = options|RRDCONTEXT_OPTION_SKIP_ID,
            .chart_label_key = chart_label_key,
            .chart_labels_filter = chart_labels_filter,
            .chart_dimensions = chart_dimensions,
            .after = after,
            .before = before,
            .written = 0,
            .now = now_realtime_sec(),
    };
    rrdcontext_to_json_callback((DICTIONARY_ITEM *)rca, rc, &t_contexts);
    buffer_json_finalize(wb);

    rrdcontext_release(rca);

    if(!t_contexts.written)
        return HTTP_RESP_NOT_FOUND;

    return HTTP_RESP_OK;
}

int rrdcontexts_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options, SIMPLE_PATTERN *chart_label_key, SIMPLE_PATTERN *chart_labels_filter, SIMPLE_PATTERN *chart_dimensions) {
    if(!host->rrdctx.contexts) {
        netdata_log_error("%s(): request for host '%s' that does not have rrdcontexts initialized.", __FUNCTION__, rrdhost_hostname(host));
        return HTTP_RESP_NOT_FOUND;
    }

    char node_uuid[UUID_STR_LEN] = "";

    if(!UUIDiszero(host->node_id))
        uuid_unparse_lower(host->node_id.uuid, node_uuid);

    if(after != 0 && before != 0)
        rrdr_relative_window_to_absolute_query(&after, &before, NULL, false);

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));
    buffer_json_member_add_string(wb, "machine_guid", host->machine_guid);
    buffer_json_member_add_string(wb, "node_id", node_uuid);
    CLAIM_ID claim_id = rrdhost_claim_id_get(host);
    buffer_json_member_add_string(wb, "claim_id", claim_id.str);

    if(options & RRDCONTEXT_OPTION_SHOW_LABELS) {
        buffer_json_member_add_object(wb, "host_labels");
        rrdlabels_to_buffer_json_members(host->rrdlabels, wb);
        buffer_json_object_close(wb);
    }

    buffer_json_member_add_object(wb, "contexts");
    struct rrdcontext_to_json t_contexts = {
            .wb = wb,
            .options = options,
            .chart_label_key = chart_label_key,
            .chart_labels_filter = chart_labels_filter,
            .chart_dimensions = chart_dimensions,
            .after = after,
            .before = before,
            .written = 0,
            .now = now_realtime_sec(),
    };
    dictionary_walkthrough_read(host->rrdctx.contexts, rrdcontext_to_json_callback, &t_contexts);
    buffer_json_object_close(wb);

    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

