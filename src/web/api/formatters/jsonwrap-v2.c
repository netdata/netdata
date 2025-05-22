// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"

static void query_target_combined_chart_type(BUFFER *wb, QUERY_TARGET *qt, size_t contexts) {
    if(contexts >= 1)
        buffer_json_member_add_string(wb, "chart_type", rrdset_type_name(rrdcontext_acquired_chart_type(qt->contexts.array[0].rca)));
}

static void query_target_title(BUFFER *wb, QUERY_TARGET *qt, size_t contexts) {
    if(contexts == 1) {
        buffer_json_member_add_string(wb, "title", rrdcontext_acquired_title(qt->contexts.array[0].rca));
    }
    else if(contexts > 1) {
        BUFFER *t = buffer_create(0, NULL);
        DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

        buffer_strcat(t, "Chart for contexts: ");

        size_t added = 0;
        for(size_t c = 0; c < qt->contexts.used ;c++) {
            bool *set = dictionary_set(dict, rrdcontext_acquired_id(qt->contexts.array[c].rca), NULL, sizeof(*set));
            if(!*set) {
                *set = true;
                if(added)
                    buffer_fast_strcat(t, ", ", 2);

                buffer_strcat(t, rrdcontext_acquired_id(qt->contexts.array[c].rca));
                added++;
            }
        }
        buffer_json_member_add_string(wb, "title", buffer_tostring(t));
        dictionary_destroy(dict);
        buffer_free(t);
    }
}

void version_hashes_api_v2(BUFFER *wb, struct query_versions *versions) {
    buffer_json_member_add_object(wb, "versions");
    buffer_json_member_add_uint64(wb, "routing_hard_hash", 1);
    buffer_json_member_add_uint64(wb, "nodes_hard_hash", dictionary_version(rrdhost_root_index));
    buffer_json_member_add_uint64(wb, "contexts_hard_hash", versions->contexts_hard_hash);
    buffer_json_member_add_uint64(wb, "contexts_soft_hash", versions->contexts_soft_hash);
    buffer_json_member_add_uint64(wb, "alerts_hard_hash", versions->alerts_hard_hash);
    buffer_json_member_add_uint64(wb, "alerts_soft_hash", versions->alerts_soft_hash);
    buffer_json_object_close(wb);
}

static void query_target_combined_units_v2(BUFFER *wb, QUERY_TARGET *qt, size_t contexts, bool ignore_percentage) {
    if(!ignore_percentage && query_target_has_percentage_units(qt)) {
        buffer_json_member_add_string(wb, "units", "%");
    }
    else if(contexts == 1) {
        buffer_json_member_add_string(wb, "units", rrdcontext_acquired_units(qt->contexts.array[0].rca));
    }
    else if(contexts > 1) {
        DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
        for(size_t c = 0; c < qt->contexts.used ;c++)
            dictionary_set(dict, rrdcontext_acquired_units(qt->contexts.array[c].rca), NULL, 0);

        if(dictionary_entries(dict) == 1)
            buffer_json_member_add_string(wb, "units", rrdcontext_acquired_units(qt->contexts.array[0].rca));
        else {
            buffer_json_member_add_array(wb, "units");
            const char *s;
            dfe_start_read(dict, s)
                buffer_json_add_array_item_string(wb, s_dfe.name);
            dfe_done(s);
            buffer_json_array_close(wb);
        }
        dictionary_destroy(dict);
    }
}

static inline void rrdr_dimension_query_points_statistics(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options, bool dview) {
    STORAGE_POINT *sp = (dview) ? r->dview : r->dqp;
    NETDATA_DOUBLE anomaly_rate_multiplier = (dview) ? RRDR_DVIEW_ANOMALY_COUNT_MULTIPLIER : 1.0;

    if(unlikely(!sp))
        return;

    if(key)
        buffer_json_member_add_object(wb, key);

    buffer_json_member_add_array(wb, "min");
    for(size_t c = 0; c < r->d ; c++) {
        if (!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_double(wb, sp[c].min);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "max");
    for(size_t c = 0; c < r->d ; c++) {
        if (!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_double(wb, sp[c].max);
    }
    buffer_json_array_close(wb);

    if(options & RRDR_OPTION_RETURN_RAW) {
        buffer_json_member_add_array(wb, "sum");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_double(wb, sp[c].sum);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "cnt");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_uint64(wb, sp[c].count);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "arc");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_uint64(wb, storage_point_anomaly_rate(sp[c]) / anomaly_rate_multiplier / 100.0 * sp[c].count);
        }
        buffer_json_array_close(wb);
    }
    else {
        NETDATA_DOUBLE sum = 0.0;
        for(size_t c = 0; c < r->d ; c++) {
            if(!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            sum += ABS(sp[c].sum);
        }

        buffer_json_member_add_array(wb, "avg");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_double(wb, storage_point_average_value(sp[c]));
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "arp");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_json_add_array_item_double(wb, storage_point_anomaly_rate(sp[c]) / anomaly_rate_multiplier);
        }
        buffer_json_array_close(wb);

        buffer_json_member_add_array(wb, "con");
        for(size_t c = 0; c < r->d ; c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            NETDATA_DOUBLE con = (sum > 0.0) ? ABS(sp[c].sum) * 100.0 / sum : 0.0;
            buffer_json_add_array_item_double(wb, con);
        }
        buffer_json_array_close(wb);
    }

    if(key)
        buffer_json_object_close(wb);
}

static void rrdr_grouped_by_array_v2(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options __maybe_unused) {
    QUERY_TARGET *qt = r->internal.qt;

    buffer_json_member_add_array(wb, key);

    // find the deeper group-by
    ssize_t g = 0;
    for(g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
        if(qt->request.group_by[g].group_by == RRDR_GROUP_BY_NONE)
            break;
    }

    if(g > 0)
        g--;

    RRDR_GROUP_BY group_by = qt->request.group_by[g].group_by;

    if(group_by & RRDR_GROUP_BY_SELECTED)
        buffer_json_add_array_item_string(wb, "selected");

    else if(group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)
        buffer_json_add_array_item_string(wb, "percentage-of-instance");

    else {

        if(group_by & RRDR_GROUP_BY_DIMENSION)
            buffer_json_add_array_item_string(wb, "dimension");

        if(group_by & RRDR_GROUP_BY_INSTANCE)
            buffer_json_add_array_item_string(wb, "instance");

        if(group_by & RRDR_GROUP_BY_LABEL) {
            BUFFER *b = buffer_create(0, NULL);
            for (size_t l = 0; l < qt->group_by[g].used; l++) {
                buffer_flush(b);
                buffer_fast_strcat(b, "label:", 6);
                buffer_strcat(b, qt->group_by[g].label_keys[l]);
                buffer_json_add_array_item_string(wb, buffer_tostring(b));
            }
            buffer_free(b);
        }

        if(group_by & RRDR_GROUP_BY_NODE)
            buffer_json_add_array_item_string(wb, "node");

        if(group_by & RRDR_GROUP_BY_CONTEXT)
            buffer_json_add_array_item_string(wb, "context");

        if(group_by & RRDR_GROUP_BY_UNITS)
            buffer_json_add_array_item_string(wb, "units");
    }

    buffer_json_array_close(wb); // group_by_order
}

static void rrdr_dimension_units_array_v2(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options, bool ignore_percentage) {
    if(!r->du)
        return;

    bool percentage = !ignore_percentage && query_target_has_percentage_units(r->internal.qt);

    buffer_json_member_add_array(wb, key);
    for(size_t c = 0; c < r->d ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        if(percentage)
            buffer_json_add_array_item_string(wb, "%");
        else
            buffer_json_add_array_item_string(wb, string2str(r->du[c]));
    }
    buffer_json_array_close(wb);
}

static void rrdr_dimension_priority_array_v2(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    if(!r->dp)
        return;

    buffer_json_member_add_array(wb, key);
    for(size_t c = 0; c < r->d ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_uint64(wb, r->dp[c]);
    }
    buffer_json_array_close(wb);
}

static void rrdr_dimension_aggregated_array_v2(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    if(!r->dgbc)
        return;

    buffer_json_member_add_array(wb, key);
    for(size_t c = 0; c < r->d ;c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_uint64(wb, r->dgbc[c]);
    }
    buffer_json_array_close(wb);
}

void rrdr_json_wrapper_begin2(RRDR *r, BUFFER *wb) {
    QUERY_TARGET *qt = r->internal.qt;
    RRDR_OPTIONS options = qt->window.options;

    char kq[2] = "\"",                    // key quote
        sq[2] = "\"";                    // string quote

    if(unlikely(options & RRDR_OPTION_GOOGLE_JSON)) {
        kq[0] = '\0';
        sq[0] = '\'';
    }

    buffer_json_initialize(
        wb, kq, sq, 0, true, (options & RRDR_OPTION_MINIFY) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_uint64(wb, "api", 2);

    if(options & RRDR_OPTION_DEBUG) {
        buffer_json_member_add_string(wb, "id", qt->id);
        buffer_json_member_add_object(wb, "request");
        {
            buffer_json_member_add_string(wb, "format", rrdr_format_to_string(qt->request.format));
            rrdr_options_to_buffer_json_array(wb, "options", qt->request.options);

            buffer_json_member_add_object(wb, "scope");
            buffer_json_member_add_string(wb, "scope_nodes", qt->request.scope_nodes);
            buffer_json_member_add_string(wb, "scope_contexts", qt->request.scope_contexts);
            buffer_json_object_close(wb); // scope

            buffer_json_member_add_object(wb, "selectors");
            if (qt->request.host)
                buffer_json_member_add_string(wb, "nodes", rrdhost_hostname(qt->request.host));
            else
                buffer_json_member_add_string(wb, "nodes", qt->request.nodes);
            buffer_json_member_add_string(wb, "contexts", qt->request.contexts);
            buffer_json_member_add_string(wb, "instances", qt->request.instances);
            buffer_json_member_add_string(wb, "dimensions", qt->request.dimensions);
            buffer_json_member_add_string(wb, "labels", qt->request.labels);
            buffer_json_member_add_string(wb, "alerts", qt->request.alerts);
            buffer_json_object_close(wb); // selectors

            buffer_json_member_add_object(wb, "window");
            buffer_json_member_add_time_t(wb, "after", qt->request.after);
            buffer_json_member_add_time_t(wb, "before", qt->request.before);
            buffer_json_member_add_uint64(wb, "points", qt->request.points);
            if (qt->request.options & RRDR_OPTION_SELECTED_TIER)
                buffer_json_member_add_uint64(wb, "tier", qt->request.tier);
            else
                buffer_json_member_add_string(wb, "tier", NULL);
            buffer_json_object_close(wb); // window

            buffer_json_member_add_object(wb, "aggregations");
            {
                buffer_json_member_add_object(wb, "time");
                buffer_json_member_add_string(wb, "time_group", time_grouping_tostring(qt->request.time_group_method));
                buffer_json_member_add_string(wb, "time_group_options", qt->request.time_group_options);
                if (qt->request.resampling_time > 0)
                    buffer_json_member_add_time_t(wb, "time_resampling", qt->request.resampling_time);
                else
                    buffer_json_member_add_string(wb, "time_resampling", NULL);
                buffer_json_object_close(wb); // time

                buffer_json_member_add_array(wb, "metrics");
                for(size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES ;g++) {
                    if(qt->request.group_by[g].group_by == RRDR_GROUP_BY_NONE)
                        break;

                    buffer_json_add_array_item_object(wb);
                    {
                        buffer_json_member_add_array(wb, "group_by");
                        buffer_json_group_by_to_array(wb, qt->request.group_by[g].group_by);
                        buffer_json_array_close(wb);

                        buffer_json_member_add_array(wb, "group_by_label");
                        for (size_t l = 0; l < qt->group_by[g].used; l++)
                            buffer_json_add_array_item_string(wb, qt->group_by[g].label_keys[l]);
                        buffer_json_array_close(wb);

                        buffer_json_member_add_string(
                            wb, "aggregation",group_by_aggregate_function_to_string(qt->request.group_by[g].aggregation));
                    }
                    buffer_json_object_close(wb);
                }
                buffer_json_array_close(wb); // group_by
            }
            buffer_json_object_close(wb); // aggregations

            buffer_json_member_add_uint64(wb, "timeout", qt->request.timeout_ms);
        }
        buffer_json_object_close(wb); // request
    }

    version_hashes_api_v2(wb, &qt->versions);

    buffer_json_member_add_object(wb, "summary");
    struct summary_total_counts
        nodes_totals = { 0 },
        contexts_totals = { 0 },
        instances_totals = { 0 },
        metrics_totals = { 0 },
        label_key_totals = { 0 },
        label_key_value_totals = { 0 };
    {
        query_target_summary_nodes_v2(wb, qt, "nodes", &nodes_totals);
        r->internal.contexts = query_target_summary_contexts_v2(wb, qt, "contexts", &contexts_totals);
        query_target_summary_instances_v2(wb, qt, "instances", &instances_totals);
        query_target_summary_dimensions_v12(wb, qt, "dimensions", true, &metrics_totals);
        query_target_summary_labels_v12(wb, qt, "labels", true, &label_key_totals, &label_key_value_totals);
        query_target_summary_alerts_v2(wb, qt, "alerts");
    }
    if(query_target_aggregatable(qt)) {
        buffer_json_member_add_object(wb, "globals");
        query_target_points_statistics(wb, qt, &qt->query_points);
        buffer_json_object_close(wb); // globals
    }
    buffer_json_object_close(wb); // summary

    // Only include the totals section if MINIMAL_STATS option is not set
    if (!(options & RRDR_OPTION_MINIMAL_STATS)) {
        buffer_json_member_add_object(wb, "totals");
        query_target_total_counts(wb, "nodes", &nodes_totals);
        query_target_total_counts(wb, "contexts", &contexts_totals);
        query_target_total_counts(wb, "instances", &instances_totals);
        query_target_total_counts(wb, "dimensions", &metrics_totals);
        query_target_total_counts(wb, "label_keys", &label_key_totals);
        query_target_total_counts(wb, "label_key_values", &label_key_value_totals);
        buffer_json_object_close(wb); // totals
    }

    if(options & RRDR_OPTION_SHOW_DETAILS) {
        buffer_json_member_add_object(wb, "detailed");
        query_target_detailed_objects_tree(wb, r, options);
        buffer_json_object_close(wb); // detailed
    }

    query_target_functions(wb, "functions", r);
}

void rrdr_json_wrapper_end2(RRDR *r, BUFFER *wb) {
    QUERY_TARGET *qt = r->internal.qt;
    DATASOURCE_FORMAT format = qt->request.format;
    RRDR_OPTIONS options = qt->window.options;

    buffer_json_member_add_object(wb, "db");
    {
        buffer_json_member_add_uint64(wb, "tiers", nd_profile.storage_tiers);
        buffer_json_member_add_time_t(wb, "update_every", qt->db.minimum_latest_update_every_s);
        buffer_json_member_add_time_t(wb, "first_entry", qt->db.first_time_s);
        buffer_json_member_add_time_t(wb, "last_entry", qt->db.last_time_s);

        query_target_combined_units_v2(wb, qt, r->internal.contexts, true);
        buffer_json_member_add_object(wb, "dimensions");
        {
            rrdr_dimension_ids(wb, "ids", r, options);
            rrdr_dimension_units_array_v2(wb, "units", r, options, true);
            rrdr_dimension_query_points_statistics(wb, "sts", r, options, false);
        }
        buffer_json_object_close(wb); // dimensions

        buffer_json_member_add_array(wb, "per_tier");
        for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_uint64(wb, "tier", tier);
            buffer_json_member_add_uint64(wb, "queries", qt->db.tiers[tier].queries);
            buffer_json_member_add_uint64(wb, "points", qt->db.tiers[tier].points);
            buffer_json_member_add_time_t(wb, "update_every", qt->db.tiers[tier].update_every);
            buffer_json_member_add_time_t(wb, "first_entry", qt->db.tiers[tier].retention.first_time_s);
            buffer_json_member_add_time_t(wb, "last_entry", qt->db.tiers[tier].retention.last_time_s);
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb);
    }
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "view");
    {
        query_target_title(wb, qt, r->internal.contexts);
        buffer_json_member_add_time_t(wb, "update_every", r->view.update_every);
        buffer_json_member_add_time_t(wb, "after", r->view.after);
        buffer_json_member_add_time_t(wb, "before", r->view.before);

        if(options & RRDR_OPTION_DEBUG) {
            buffer_json_member_add_string(wb, "format", rrdr_format_to_string(format));
            rrdr_options_to_buffer_json_array(wb, "options", options);
            buffer_json_member_add_string(wb, "time_group", time_grouping_tostring(qt->request.time_group_method));
        }

        if(options & RRDR_OPTION_DEBUG) {
            buffer_json_member_add_object(wb, "partial_data_trimming");
            buffer_json_member_add_time_t(wb, "max_update_every", r->partial_data_trimming.max_update_every);
            buffer_json_member_add_time_t(wb, "expected_after", r->partial_data_trimming.expected_after);
            buffer_json_member_add_time_t(wb, "trimmed_after", r->partial_data_trimming.trimmed_after);
            buffer_json_object_close(wb);
        }

        if(options & RRDR_OPTION_RETURN_RAW)
            buffer_json_member_add_uint64(wb, "points", rrdr_rows(r));

        query_target_combined_units_v2(wb, qt, r->internal.contexts, false);
        query_target_combined_chart_type(wb, qt, r->internal.contexts);
        buffer_json_member_add_object(wb, "dimensions");
        {
            rrdr_grouped_by_array_v2(wb, "grouped_by", r, options);
            rrdr_dimension_ids(wb, "ids", r, options);
            rrdr_dimension_names(wb, "names", r, options);
            rrdr_dimension_units_array_v2(wb, "units", r, options, false);
            rrdr_dimension_priority_array_v2(wb, "priorities", r, options);
            rrdr_dimension_aggregated_array_v2(wb, "aggregated", r, options);
            rrdr_dimension_query_points_statistics(wb, "sts", r, options, true);
            rrdr_json_group_by_labels(wb, "labels", r, options);
        }
        buffer_json_object_close(wb); // dimensions
        buffer_json_member_add_double(wb, "min", r->view.min);
        buffer_json_member_add_double(wb, "max", r->view.max);
    }
    buffer_json_object_close(wb); // view

    buffer_json_agents_v2(wb, &r->internal.qt->timings, 0, false, true);
    buffer_json_cloud_timings(wb, "timings", &r->internal.qt->timings);
    buffer_json_finalize(wb);
}
