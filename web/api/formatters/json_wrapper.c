// SPDX-License-Identifier: GPL-3.0-or-later

#include "json_wrapper.h"

struct value_output {
    int c;
    BUFFER *wb;
};

static int value_list_output_callback(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *data) {
    struct value_output *ap = (struct value_output *)data;
    BUFFER *wb = ap->wb;
    char *output = (char *) entry;
    if(ap->c) buffer_strcat(wb, ",");
    buffer_strcat(wb, output);
    (ap->c)++;
    return 0;
}

static int fill_formatted_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    (void)ls;
    DICTIONARY *dict = (DICTIONARY *)data;
    char n[RRD_ID_LENGTH_MAX * 2 + 2];
    char output[RRD_ID_LENGTH_MAX * 2 + 8];
    char v[RRD_ID_LENGTH_MAX * 2 + 1];

    sanitize_json_string(v, (char *)value, RRD_ID_LENGTH_MAX * 2);
    int len = snprintfz(output, RRD_ID_LENGTH_MAX * 2 + 7, "[\"%s\", \"%s\"]", name, v);
    snprintfz(n, RRD_ID_LENGTH_MAX * 2, "%s:%s", name, v);
    dictionary_set(dict, n, output, len + 1);

    return 1;
}

void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb, uint32_t format, RRDR_OPTIONS options, int string_value,
    RRDR_GROUPING group_method)
{
    QUERY_TARGET *qt = r->internal.qt;

    long rows = rrdr_rows(r);
    long c, i;
    const long query_used = qt->query.used;

    //info("JSONWRAPPER(): %s: BEGIN", r->st->id);
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

    buffer_sprintf(wb, "{\n"
                       "   %sapi%s: 1,\n"
                       "   %sid%s: %s%s%s,\n"
                       "   %sname%s: %s%s%s,\n"
                       "   %sview_update_every%s: %lld,\n"
                       "   %supdate_every%s: %lld,\n"
                       "   %sfirst_entry%s: %lld,\n"
                       "   %slast_entry%s: %lld,\n"
                       "   %sbefore%s: %lld,\n"
                       "   %safter%s: %lld,\n"
                       "   %sgroup%s: %s%s%s,\n"
                       "   %soptions%s: %s"
                   , kq, kq
                   , kq, kq, sq, qt->id, sq
                   , kq, kq, sq, qt->id, sq
                   , kq, kq, (long long)r->update_every
                   , kq, kq, (long long)qt->db.minimum_latest_update_every
                   , kq, kq, (long long)qt->db.first_time_t
                   , kq, kq, (long long)qt->db.last_time_t
                   , kq, kq, (long long)r->before
                   , kq, kq, (long long)r->after
                   , kq, kq, sq, web_client_api_request_v1_data_group_to_string(group_method), sq
                   , kq, kq, sq);

    web_client_api_request_v1_data_options_to_buffer(wb, r->internal.query_options);

    buffer_sprintf(wb, "%s,\n   %sdimension_names%s: [", sq, kq, kq);

    for(c = 0, i = 0; c < query_used ; c++) {
        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        if(i) buffer_strcat(wb, ", ");
        buffer_strcat(wb, sq);
        buffer_strcat(wb, string2str(qt->query.array[c].dimension.name));
        buffer_strcat(wb, sq);
        i++;
    }
    if(!i) {
#ifdef NETDATA_INTERNAL_CHECKS
        error("QUERY: '%s', RRDR is empty, %zu dimensions, options is 0x%08x", qt->id, r->d, options);
#endif
        rows = 0;
        buffer_strcat(wb, sq);
        buffer_strcat(wb, "no data");
        buffer_strcat(wb, sq);
    }

    buffer_sprintf(wb, "],\n"
                       "   %sdimension_ids%s: ["
                   , kq, kq);

    for(c = 0, i = 0; c < query_used ; c++) {
        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        if(i) buffer_strcat(wb, ", ");
        buffer_strcat(wb, sq);
        buffer_strcat(wb, string2str(qt->query.array[c].dimension.id));
        buffer_strcat(wb, sq);
        i++;
    }
    if(!i) {
        rows = 0;
        buffer_strcat(wb, sq);
        buffer_strcat(wb, "no data");
        buffer_strcat(wb, sq);
    }
    buffer_strcat(wb, "],\n");

    if (r->internal.query_options & RRDR_OPTION_ALL_DIMENSIONS) {
        buffer_sprintf(wb, "   %sfull_dimension_list%s: [", kq, kq);

        char name[RRD_ID_LENGTH_MAX * 2 + 2];
        char output[RRD_ID_LENGTH_MAX * 2 + 8];

        struct value_output co = {.c = 0, .wb = wb};

        DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
        for (c = 0; c < (long)qt->metrics.used ;c++) {
            snprintfz(name, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                      rrdmetric_acquired_id(qt->metrics.array[c]),
                      rrdmetric_acquired_name(qt->metrics.array[c]));

            int len = snprintfz(output, RRD_ID_LENGTH_MAX * 2 + 7, "[\"%s\",\"%s\"]",
                                rrdmetric_acquired_id(qt->metrics.array[c]),
                                rrdmetric_acquired_name(qt->metrics.array[c]));

            dictionary_set(dict, name, output, len + 1);
        }
        dictionary_walkthrough_read(dict, value_list_output_callback, &co);
        dictionary_destroy(dict);

        co.c = 0;
        buffer_sprintf(wb, "],\n   %sfull_chart_list%s: [", kq, kq);
        dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
        for (c = 0; c < (long)qt->instances.used ; c++) {
            RRDINSTANCE_ACQUIRED *ria = qt->instances.array[c];

            snprintfz(name, RRD_ID_LENGTH_MAX * 2 + 1, "%s:%s",
                      rrdinstance_acquired_id(ria),
                      rrdinstance_acquired_name(ria));

            int len = snprintfz(output, RRD_ID_LENGTH_MAX * 2 + 7, "[\"%s\",\"%s\"]",
                                rrdinstance_acquired_id(ria),
                                rrdinstance_acquired_name(ria));

            dictionary_set(dict, name, output, len + 1);
        }
        dictionary_walkthrough_read(dict, value_list_output_callback, &co);
        dictionary_destroy(dict);

        co.c = 0;
        buffer_sprintf(wb, "],\n   %sfull_chart_labels%s: [", kq, kq);
        dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
        for (c = 0; c < (long)qt->instances.used ; c++) {
            RRDINSTANCE_ACQUIRED *ria = qt->instances.array[c];
            rrdlabels_walkthrough_read(rrdinstance_acquired_labels(ria), fill_formatted_callback, dict);
        }
        dictionary_walkthrough_read(dict, value_list_output_callback, &co);
        dictionary_destroy(dict);
        buffer_strcat(wb, "],\n");
    }

    // functions
    {
        DICTIONARY *funcs = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
        RRDINSTANCE_ACQUIRED *ria = NULL;
        for (c = 0; c < query_used ; c++) {
            QUERY_METRIC *qm = &qt->query.array[c];
            if(qm->link.ria == ria)
                continue;

            ria = qm->link.ria;
            chart_functions_to_dict(rrdinstance_acquired_functions(ria), funcs);
        }

        buffer_sprintf(wb, "   %sfunctions%s: [", kq, kq);
        void *t; (void)t;
        dfe_start_read(funcs, t) {
            const char *comma = "";
            if(t_dfe.counter) comma = ", ";
            buffer_sprintf(wb, "%s%s%s%s", comma, sq, t_dfe.name, sq);
        }
        dfe_done(t);
        dictionary_destroy(funcs);
        buffer_strcat(wb, "],\n");
    }

    // context query
    if (!qt->request.st) {
        buffer_sprintf(
            wb,
            "   %schart_ids%s: [",
            kq, kq);

        for (c = 0, i = 0; c < query_used; c++) {
            QUERY_METRIC *qm = &qt->query.array[c];

            if (unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN))
                continue;

            if (unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO)))
                continue;

            if (i)
                buffer_strcat(wb, ", ");
            buffer_strcat(wb, sq);
            buffer_strcat(wb, string2str(qm->chart.id));
            buffer_strcat(wb, sq);
            i++;
        }
        if (!i) {
            rows = 0;
            buffer_strcat(wb, sq);
            buffer_strcat(wb, "no data");
            buffer_strcat(wb, sq);
        }
        buffer_strcat(wb, "],\n");
        if (qt->instances.chart_label_key_pattern) {
            buffer_sprintf(wb, "   %schart_labels%s: { ", kq, kq);

            SIMPLE_PATTERN *pattern = qt->instances.chart_label_key_pattern;
            char *label_key = NULL;
            int keys = 0;
            while (pattern && (label_key = simple_pattern_iterate(&pattern))) {
                if (keys)
                    buffer_strcat(wb, ", ");
                buffer_sprintf(wb, "%s%s%s : [", kq, label_key, kq);
                keys++;

                for (c = 0, i = 0; c < query_used; c++) {
                    QUERY_METRIC *qm = &qt->query.array[c];

                    if (unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN))
                        continue;
                    if (unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO)))
                        continue;

                    if (i)
                        buffer_strcat(wb, ", ");
                    rrdlabels_get_value_to_buffer_or_null(rrdinstance_acquired_labels(qm->link.ria), wb, label_key, sq, "null");
                    i++;
                }
                if (!i) {
                    rows = 0;
                    buffer_strcat(wb, sq);
                    buffer_strcat(wb, "no data");
                    buffer_strcat(wb, sq);
                }
                buffer_strcat(wb, "]");
            }
            buffer_strcat(wb, "},\n");
        }
    }

    buffer_sprintf(wb, "   %slatest_values%s: ["
                   , kq, kq);

    for(c = 0, i = 0; c < query_used ;c++) {
        QUERY_METRIC *qm = &qt->query.array[c];

        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        if(i) buffer_strcat(wb, ", ");
        i++;

        NETDATA_DOUBLE value = rrdmetric_acquired_last_stored_value(qm->link.rma);
        if (NAN == value)
            buffer_strcat(wb, "null");
        else
            buffer_rrd_value(wb, value);
    }
    if(!i) {
        rows = 0;
        buffer_strcat(wb, "null");
    }

    buffer_sprintf(wb, "],\n"
                       "   %sview_latest_values%s: ["
                   , kq, kq);

    i = 0;
    if(rows) {
        NETDATA_DOUBLE total = 1;

        if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
            total = 0;
            for(c = 0; c < query_used ;c++) {
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
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

            if(i) buffer_strcat(wb, ", ");
            i++;

            NETDATA_DOUBLE *cn = &r->v[ (rrdr_rows(r) - 1) * r->d ];
            RRDR_VALUE_FLAGS *co = &r->o[ (rrdr_rows(r) - 1) * r->d ];
            NETDATA_DOUBLE n = cn[c];

            if(co[c] & RRDR_VALUE_EMPTY) {
                if(options & RRDR_OPTION_NULL2ZERO)
                    buffer_strcat(wb, "0");
                else
                    buffer_strcat(wb, "null");
            }
            else {
                if(unlikely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                    n = -n;

                if(unlikely(options & RRDR_OPTION_PERCENTAGE))
                    n = n * 100 / total;

                buffer_rrd_value(wb, n);
            }
        }
    }
    if(!i) {
        rows = 0;
        buffer_strcat(wb, "null");
    }

    buffer_sprintf(wb, "],\n"
                       "   %sdimensions%s: %ld,\n"
                       "   %spoints%s: %ld,\n"
                       "   %sformat%s: %s"
                   , kq, kq, i
                   , kq, kq, rows
                   , kq, kq, sq
    );

    rrdr_buffer_print_format(wb, format);

    buffer_sprintf(wb, "%s,\n"
                       "   %sdb_points_per_tier%s: [ "
                   , sq
                   , kq, kq
                   );

    for(size_t tier = 0; tier < storage_tiers ; tier++)
        buffer_sprintf(wb, "%s%zu", tier>0?", ":"", r->internal.tier_points_read[tier]);

    buffer_strcat(wb, " ]");

    buffer_sprintf(wb, ",\n   %sresult%s: ", kq, kq);

    if(string_value) buffer_strcat(wb, sq);
    //info("JSONWRAPPER(): %s: END", r->st->id);
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

    buffer_sprintf(wb, ",\n %smin%s: ", kq, kq);
    buffer_rrd_value(wb, r->min);
    buffer_sprintf(wb, ",\n %smax%s: ", kq, kq);
    buffer_rrd_value(wb, r->max);
    buffer_strcat(wb, "\n}\n");
}
