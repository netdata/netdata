// SPDX-License-Identifier: GPL-3.0-or-later

#include "json_wrapper.h"

void jsonwrap_query_plan(RRDR *r, BUFFER *wb) {
    QUERY_TARGET *qt = r->internal.qt;

    buffer_json_member_add_object(wb, "query_plan");
    for(size_t m = 0; m < qt->query.used; m++) {
        QUERY_METRIC *qm = &qt->query.array[m];

        buffer_json_member_add_object(wb, string2str(qm->dimension.id));
        {
            buffer_json_member_add_array(wb, "plans");
            for (size_t p = 0; p < qm->plan.used; p++) {
                QUERY_PLAN_ENTRY *qp = &qm->plan.array[p];

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_uint64(wb, "tier", qp->tier);
                buffer_json_member_add_time_t(wb, "after", qp->after);
                buffer_json_member_add_time_t(wb, "before", qp->before);
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);

            buffer_json_member_add_array(wb, "tiers");
            for (size_t tier = 0; tier < storage_tiers; tier++) {
                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_uint64(wb, "tier", tier);
                buffer_json_member_add_time_t(wb, "db_first_time", qm->tiers[tier].db_first_time_s);
                buffer_json_member_add_time_t(wb, "db_last_time", qm->tiers[tier].db_last_time_s);
                buffer_json_member_add_int64(wb, "weight", qm->tiers[tier].weight);
                buffer_json_object_close(wb);
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

static inline long jsonwrap_dimension_names(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i;

    buffer_json_member_add_array(wb, key);
    for(c = 0, i = 0; c < query_used ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_string(wb, string2str(qt->query.array[c].dimension.name));
        i++;
    }
    buffer_json_array_close(wb);

    return i;
}

static inline long jsonwrap_dimension_ids(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i;

    buffer_json_member_add_array(wb, key);
    for(c = 0, i = 0; c < query_used ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_json_add_array_item_string(wb, string2str(qt->query.array[c].dimension.id));
        i++;
    }
    buffer_json_array_close(wb);

    return i;
}

static inline long jsonwrap_chart_ids(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i;

    buffer_json_member_add_array(wb, key);
    for (c = 0, i = 0; c < query_used; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        QUERY_METRIC *qm = &qt->query.array[c];
        buffer_json_add_array_item_string(wb, string2str(qm->chart.id));
        i++;
    }
    buffer_json_array_close(wb);

    return i;
}

struct rrdlabels_formatting_v2 {
    BUFFER *wb;
    DICTIONARY *dict;
};

static int rrdlabels_formatting_v2(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct rrdlabels_formatting_v2 *t = data;
    BUFFER *wb = t->wb;
    DICTIONARY *dict = t->dict;

    char n[RRD_ID_LENGTH_MAX * 2 + 2];
    snprintfz(n, RRD_ID_LENGTH_MAX * 2, "%s:%s", name, value);

    bool existing = 0;
    bool *set = dictionary_set(dict, n, &existing, sizeof(bool));
    if(!*set) {
        *set = true;
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, name);
        buffer_json_add_array_item_string(wb, value);
        buffer_json_array_close(wb);
    }

    return 1;
}

static inline void jsonwrap_full_dimension_list(BUFFER *wb, RRDR *r) {
    QUERY_TARGET *qt = r->internal.qt;

    char name[RRD_ID_LENGTH_MAX * 2 + 2];

    buffer_json_member_add_array(wb, "full_dimension_list");
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
    for (long c = 0; c < (long)qt->metrics.used ;c++) {
        RRDMETRIC_ACQUIRED *rma = qt->metrics.array[c];

        snprintfz(name, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                  rrdmetric_acquired_id(rma),
                  rrdmetric_acquired_name(rma));

        bool existing = 0;
        bool *set = dictionary_set(dict, name, &existing, sizeof(bool));
        if(!*set) {
            *set = true;
            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, rrdmetric_acquired_id(rma));
            buffer_json_add_array_item_string(wb, rrdmetric_acquired_name(rma));
            buffer_json_array_close(wb);
        }
    }
    dictionary_destroy(dict);
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "full_chart_list");
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
    for (long c = 0; c < (long)qt->instances.used ; c++) {
        RRDINSTANCE_ACQUIRED *ria = qt->instances.array[c];

        snprintfz(name, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                  rrdinstance_acquired_id(ria),
                  rrdinstance_acquired_name(ria));

        bool existing = 0;
        bool *set = dictionary_set(dict, name, &existing, sizeof(bool));
        if(!*set) {
            *set = true;
            buffer_json_add_array_item_array(wb);
            buffer_json_add_array_item_string(wb, rrdinstance_acquired_id(ria));
            buffer_json_add_array_item_string(wb, rrdinstance_acquired_name(ria));
            buffer_json_array_close(wb);
        }
    }
    dictionary_destroy(dict);
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "full_chart_labels");
    struct rrdlabels_formatting_v2 t = {
            .wb = wb,
            .dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE),
    };
    for (long c = 0; c < (long)qt->instances.used ; c++) {
        RRDINSTANCE_ACQUIRED *ria = qt->instances.array[c];
        rrdlabels_walkthrough_read(rrdinstance_acquired_labels(ria), rrdlabels_formatting_v2, &t);
    }
    dictionary_destroy(t.dict);
    buffer_json_array_close(wb);
}

static inline void jsonwrap_functions(BUFFER *wb, const char *key, RRDR *r) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;

    DICTIONARY *funcs = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
    RRDINSTANCE_ACQUIRED *ria = NULL;
    for (long c = 0; c < query_used ; c++) {
        QUERY_METRIC *qm = &qt->query.array[c];
        if(qm->link.ria == ria)
            continue;

        ria = qm->link.ria;
        chart_functions_to_dict(rrdinstance_acquired_functions(ria), funcs);
    }

    buffer_json_member_add_array(wb, key);
    void *t; (void)t;
    dfe_start_read(funcs, t)
        buffer_json_add_array_item_string(wb, t_dfe.name);
    dfe_done(t);
    dictionary_destroy(funcs);
    buffer_json_array_close(wb);
}

static inline long jsonwrap_chart_labels_filter(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
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

            QUERY_METRIC *qm = &qt->query.array[c];
            rrdlabels_value_to_buffer_array_item_or_null(rrdinstance_acquired_labels(qm->link.ria), wb, label_key);
            i++;
        }
        buffer_json_array_close(wb);
    }

    buffer_json_object_close(wb);

    return i;
}

static inline long jsonwrap_latest_values(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i;

    buffer_json_member_add_array(wb, key);

    for(c = 0, i = 0; c < query_used ;c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        QUERY_METRIC *qm = &qt->query.array[c];
        buffer_json_add_array_item_double(wb, rrdmetric_acquired_last_stored_value(qm->link.rma));
        i++;
    }

    buffer_json_array_close(wb);

    return i;
}

static inline long jsonwrap_view_latest_values(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    QUERY_TARGET *qt = r->internal.qt;
    const long query_used = qt->query.used;
    long c, i;

    buffer_json_member_add_array(wb, key);

    NETDATA_DOUBLE total = 1;

    if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
        total = 0;
        for(c = 0; c < query_used ;c++) {
            if(unlikely(!(r->od[c] & RRDR_DIMENSION_QUERIED))) continue;

            NETDATA_DOUBLE *cn = &r->v[ (rrdr_rows(r) - 1) * r->d ];
            NETDATA_DOUBLE n = cn[c];

            if(likely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                n = -n;

            total += n;
        }
        // prevent a division by zero
        if(total == 0) total = 1;
    }

    for(c = 0, i = 0; c < query_used ;c++) {
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
        else {
            if(unlikely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                n = -n;

            if(unlikely(options & RRDR_OPTION_PERCENTAGE))
                n = n * 100 / total;

            buffer_json_add_array_item_double(wb, n);
        }
    }

    buffer_json_array_close(wb);

    return i;
}

void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb, uint32_t format, RRDR_OPTIONS options, int string_value,
                             RRDR_TIME_GROUPING group_method)
{
    QUERY_TARGET *qt = r->internal.qt;

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

    buffer_json_initialize(wb, kq, sq, 0, true);

    buffer_json_member_add_uint64(wb, "api", 1);
    buffer_json_member_add_string(wb, "id", qt->id);
    buffer_json_member_add_string(wb, "name", qt->id);
    buffer_json_member_add_time_t(wb, "view_update_every", r->update_every);
    buffer_json_member_add_time_t(wb, "update_every", qt->db.minimum_latest_update_every_s);
    buffer_json_member_add_time_t(wb, "first_entry", qt->db.first_time_s);
    buffer_json_member_add_time_t(wb, "last_entry", qt->db.last_time_s);
    buffer_json_member_add_time_t(wb, "after", r->after);
    buffer_json_member_add_time_t(wb, "before", r->before);
    buffer_json_member_add_string(wb, "group", time_grouping_tostring(group_method));
    web_client_api_request_v1_data_options_to_buffer_json_array(wb, "options", r->internal.query_options);

    if(!jsonwrap_dimension_names(wb, "dimension_names", r, options))
        rows = 0;

    if(!jsonwrap_dimension_ids(wb, "dimension_ids", r, options))
        rows = 0;

    if (r->internal.query_options & RRDR_OPTION_ALL_DIMENSIONS)
        jsonwrap_full_dimension_list(wb, r);

    jsonwrap_functions(wb, "functions", r);

    if (!qt->request.st && !jsonwrap_chart_ids(wb, "chart_ids", r, options))
        rows = 0;

    if (qt->instances.chart_label_key_pattern && !jsonwrap_chart_labels_filter(wb, "chart_labels", r, options))
        rows = 0;

    if(!jsonwrap_latest_values(wb, "latest_values", r, options))
        rows = 0;

    long dimensions = jsonwrap_view_latest_values(wb, "view_latest_values", r, options);
    if(!dimensions)
        rows = 0;

    buffer_json_member_add_uint64(wb, "dimensions", dimensions);
    buffer_json_member_add_uint64(wb, "points", rows);
    buffer_json_member_add_string(wb, "format", rrdr_format_to_string(format));

    buffer_json_member_add_array(wb, "db_points_per_tier");
    for(size_t tier = 0; tier < storage_tiers ; tier++)
        buffer_json_add_array_item_uint64(wb, r->internal.tier_points_read[tier]);
    buffer_json_array_close(wb);

    if(options & RRDR_OPTION_SHOW_PLAN)
        jsonwrap_query_plan(r, wb);

    buffer_sprintf(wb, ",\n %sresult%s:", kq, kq);
    if(string_value) buffer_strcat(wb, sq);
}

void rrdr_json_wrapper_anomaly_rates(RRDR *r, BUFFER *wb, uint32_t format, uint32_t options, int string_value) {
    (void)r;
    (void)format;

    char kq[2] = "",                    // key quote
        sq[2] = "";                     // string quote

    if( options & RRDR_OPTION_GOOGLE_JSON ) {
        kq[0] = '\0';
        sq[0] = '\'';
    }
    else {
        kq[0] = '"';
        sq[0] = '"';
    }

    if(string_value) buffer_strcat(wb, sq);

    buffer_sprintf(wb, ",\n   %sanomaly_rates%s: ", kq, kq);
}

void rrdr_json_wrapper_end(RRDR *r, BUFFER *wb, uint32_t format, uint32_t options, int string_value) {
    (void)format;

    char sq[2] = "";                     // string quote

    if( options & RRDR_OPTION_GOOGLE_JSON ) {
        sq[0] = '\'';
    }
    else {
        sq[0] = '"';
    }

    if(string_value) buffer_strcat(wb, sq);

    buffer_json_member_add_double(wb, "min", r->min);
    buffer_json_member_add_double(wb, "max", r->max);
    buffer_json_finalize(wb);
}
