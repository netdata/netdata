// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"

static inline long jsonwrap_v1_chart_ids(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i;

    buffer_json_member_add_array(wb, key);
    for (c = 0, i = 0; c < query_used; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        QUERY_METRIC *qm = query_metric(qt, c);
        QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
        buffer_json_add_array_item_string(wb, rrdinstance_acquired_id(qi->ria));
        i++;
    }
    buffer_json_array_close(wb);

    return i;
}

static inline long query_target_chart_labels_filter_v1(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i = 0;

    buffer_json_member_add_object(wb, key);

    SIMPLE_PATTERN *pattern = qt->instances.chart_label_key_pattern;
    char *label_key = NULL;
    while (pattern && (label_key = simple_pattern_iterate(&pattern))) {
        buffer_json_member_add_array(wb, label_key);

        for (c = 0, i = 0; c < query_used; c++) {
            if(!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            QUERY_METRIC *qm = query_metric(qt, c);
            QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
            rrdlabels_value_to_buffer_array_item_or_null(rrdinstance_acquired_labels(qi->ria), wb, label_key);
            i++;
        }
        buffer_json_array_close(wb);
    }

    buffer_json_object_close(wb);

    return i;
}

static inline long query_target_metrics_latest_values(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i;

    buffer_json_member_add_array(wb, key);

    for(c = 0, i = 0; c < query_used ;c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        QUERY_METRIC *qm = query_metric(qt, c);
        QUERY_DIMENSION *qd = query_dimension(qt, qm->link.query_dimension_id);
        buffer_json_add_array_item_double(wb, rrdmetric_acquired_last_stored_value(qd->rma));
        i++;
    }

    buffer_json_array_close(wb);

    return i;
}

static inline size_t rrdr_dimension_view_latest_values(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    buffer_json_member_add_array(wb, key);

    size_t c, i;
    for(c = 0, i = 0; c < r->d ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        i++;

        NETDATA_DOUBLE *cn = &r->v[ (rrdr_rows(r) - 1) * r->d ];
        RRDR_VALUE_FLAGS *co = &r->o[ (rrdr_rows(r) - 1) * r->d ];
        NETDATA_DOUBLE n = cn[c];

        if(co[c] & RRDR_VALUE_EMPTY) {
            if(options & RRDR_OPTION_NULL2ZERO)
                buffer_json_add_array_item_double(wb, 0.0);
            else
                buffer_json_add_array_item_double(wb, NAN);
        }
        else
            buffer_json_add_array_item_double(wb, n);
    }

    buffer_json_array_close(wb);

    return i;
}

void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb) {
    QUERY_TARGET *qt = r->internal.qt;
    DATASOURCE_FORMAT format = qt->request.format;
    RRDR_OPTIONS options = qt->window.options;

    long rows = rrdr_rows(r);

    char kq[2] = "",                    // key quote
        sq[2] = "";                    // string quote

    if( options & RRDR_OPTION_GOOGLE_JSON ) {
        kq[0] = '\0';
        sq[0] = '\'';
    }
    else {
        kq[0] = '"';
        sq[0] = '"';
    }

    buffer_json_initialize(
        wb, kq, sq, 0, true, (options & RRDR_OPTION_MINIFY) ? BUFFER_JSON_OPTIONS_MINIFY : BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_uint64(wb, "api", 1);
    buffer_json_member_add_string(wb, "id", qt->id);
    buffer_json_member_add_string(wb, "name", qt->id);
    buffer_json_member_add_time_t(wb, "view_update_every", r->view.update_every);
    buffer_json_member_add_time_t(wb, "update_every", qt->db.minimum_latest_update_every_s);
    buffer_json_member_add_time_t(wb, "first_entry", qt->db.first_time_s);
    buffer_json_member_add_time_t(wb, "last_entry", qt->db.last_time_s);
    buffer_json_member_add_time_t(wb, "after", r->view.after);
    buffer_json_member_add_time_t(wb, "before", r->view.before);
    buffer_json_member_add_string(wb, "group", time_grouping_tostring(qt->request.time_group_method));
    rrdr_options_to_buffer_json_array(wb, "options", options);

    if(!rrdr_dimension_names(wb, "dimension_names", r, options))
        rows = 0;

    if(!rrdr_dimension_ids(wb, "dimension_ids", r, options))
        rows = 0;

    if (options & RRDR_OPTION_ALL_DIMENSIONS) {
        query_target_summary_instances_v1(wb, qt, "full_chart_list");
        query_target_summary_dimensions_v12(wb, qt, "full_dimension_list", false, NULL);
        query_target_summary_labels_v12(wb, qt, "full_chart_labels", false, NULL, NULL);
    }

    query_target_functions(wb, "functions", r);

    if (!qt->request.st && !jsonwrap_v1_chart_ids(wb, "chart_ids", r, options))
        rows = 0;

    if (qt->instances.chart_label_key_pattern && !query_target_chart_labels_filter_v1(wb, "chart_labels", r, options))
        rows = 0;

    if(!query_target_metrics_latest_values(wb, "latest_values", r, options))
        rows = 0;

    size_t dimensions = rrdr_dimension_view_latest_values(wb, "view_latest_values", r, options);
    if(!dimensions)
        rows = 0;

    buffer_json_member_add_uint64(wb, "dimensions", dimensions);
    buffer_json_member_add_uint64(wb, "points", rows);
    buffer_json_member_add_string(wb, "format", rrdr_format_to_string(format));

    buffer_json_member_add_array(wb, "db_points_per_tier");
    for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++)
        buffer_json_add_array_item_uint64(wb, qt->db.tiers[tier].points);
    buffer_json_array_close(wb);

    if(options & RRDR_OPTION_DEBUG)
        jsonwrap_query_plan(r, wb);
}

void rrdr_json_wrapper_end(RRDR *r, BUFFER *wb) {
    buffer_json_member_add_double(wb, "min", r->view.min);
    buffer_json_member_add_double(wb, "max", r->view.max);

    buffer_json_query_timings(wb, "timings", &r->internal.qt->timings);
    buffer_json_finalize(wb);
}
