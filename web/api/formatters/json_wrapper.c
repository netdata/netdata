// SPDX-License-Identifier: GPL-3.0-or-later

#include "json_wrapper.h"

struct value_output {
    int c;
    BUFFER *wb;
};

static int value_list_output(void *entry, void *data) {
    struct value_output *ap = (struct value_output *)data;
    BUFFER *wb = ap->wb;
    char *output = (char *) entry;
    if(ap->c) buffer_strcat(wb, ",");
    buffer_strcat(wb, output);
    (ap->c)++;
    return 0;
}

void rrdr_json_wrapper_begin(RRDR *r, BUFFER *wb, uint32_t format, RRDR_OPTIONS options, int string_value,
    QUERY_PARAMS *rrdset_query_data)
{
    struct context_param *context_param_list = rrdset_query_data->context_param_list;
    char *chart_label_key = rrdset_query_data->chart_label_key;

    RRDDIM *temp_rd = context_param_list ? context_param_list->rd : NULL;
    int should_lock = (!context_param_list || !(context_param_list->flags & CONTEXT_FLAGS_ARCHIVE));
    uint8_t context_mode = (!context_param_list || (context_param_list->flags & CONTEXT_FLAGS_CONTEXT));

    if (should_lock)
        rrdset_check_rdlock(r->st);

    long rows = rrdr_rows(r);
    long c, i;
    RRDDIM *rd;

    //info("JSONWRAPPER(): %s: BEGIN", r->st->id);
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

    if (should_lock)
        rrdset_rdlock(r->st);
    buffer_sprintf(wb, "{\n"
                       "   %sapi%s: 1,\n"
                       "   %sid%s: %s%s%s,\n"
                       "   %sname%s: %s%s%s,\n"
                       "   %sview_update_every%s: %d,\n"
                       "   %supdate_every%s: %d,\n"
                       "   %sfirst_entry%s: %u,\n"
                       "   %slast_entry%s: %u,\n"
                       "   %sbefore%s: %u,\n"
                       "   %safter%s: %u,\n"
                       "   %sdimension_names%s: ["
                   , kq, kq
                   , kq, kq, sq, context_mode && temp_rd?r->st->context:r->st->id, sq
                   , kq, kq, sq, context_mode && temp_rd?r->st->context:r->st->name, sq
                   , kq, kq, r->update_every
                   , kq, kq, r->st->update_every
                   , kq, kq, (uint32_t) (context_param_list ? context_param_list->first_entry_t : rrdset_first_entry_t_nolock(r->st))
                   , kq, kq, (uint32_t) (context_param_list ? context_param_list->last_entry_t : rrdset_last_entry_t_nolock(r->st))
                   , kq, kq, (uint32_t)r->before
                   , kq, kq, (uint32_t)r->after
                   , kq, kq);
    if (should_lock)
        rrdset_unlock(r->st);

    for(c = 0, i = 0, rd = temp_rd?temp_rd:r->st->dimensions; rd && c < r->d ;c++, rd = rd->next) {
        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        if(i) buffer_strcat(wb, ", ");
        buffer_strcat(wb, sq);
        buffer_strcat(wb, rd->name);
        buffer_strcat(wb, sq);
        i++;
    }
    if(!i) {
#ifdef NETDATA_INTERNAL_CHECKS
        error("RRDR is empty for %s (RRDR has %d dimensions, options is 0x%08x)", r->st->id, r->d, options);
#endif
        rows = 0;
        buffer_strcat(wb, sq);
        buffer_strcat(wb, "no data");
        buffer_strcat(wb, sq);
    }

    buffer_sprintf(wb, "],\n"
                       "   %sdimension_ids%s: ["
                   , kq, kq);

    for(c = 0, i = 0, rd = temp_rd?temp_rd:r->st->dimensions; rd && c < r->d ;c++, rd = rd->next) {
        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        if(i) buffer_strcat(wb, ", ");
        buffer_strcat(wb, sq);
        buffer_strcat(wb, rd->id);
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

    if (rrdset_query_data->show_dimensions) {
        buffer_sprintf(wb, "   %sfull_dimension_list%s: [", kq, kq);

        char name[RRD_ID_LENGTH_MAX * 2 + 2];
        char output[RRD_ID_LENGTH_MAX * 2 + 8];
        char value[RRD_ID_LENGTH_MAX * 2 + 1];

        struct value_output co = {.c = 0, .wb = wb};

        DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
        for (i = 0, rd = temp_rd ? temp_rd : r->st->dimensions; rd; rd = rd->next) {
            snprintfz(name, RRD_ID_LENGTH_MAX * 2, "%s:%s", rd->id, rd->name);
            int len = snprintfz(output, RRD_ID_LENGTH_MAX * 2 + 7, "[\"%s\",\"%s\"]", rd->id, rd->name);
            dictionary_set(dict, name, output, len+1);
        }
        dictionary_get_all(dict, value_list_output, &co);
        dictionary_destroy(dict);

        co.c = 0;
        buffer_sprintf(wb, "],\n   %sfull_chart_list%s: [", kq, kq);
        dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
        for (i = 0, rd = temp_rd ? temp_rd : r->st->dimensions; rd; rd = rd->next) {
            int len = snprintfz(output, RRD_ID_LENGTH_MAX * 2 + 7, "[\"%s\",\"%s\"]", rd->rrdset->id, rd->rrdset->name);
            snprintfz(name, RRD_ID_LENGTH_MAX * 2, "%s:%s", rd->rrdset->id, rd->rrdset->name);
            dictionary_set(dict, name, output, len + 1);
        }

        dictionary_get_all(dict, value_list_output, &co);
        dictionary_destroy(dict);

        RRDSET *st;
        co.c = 0;
        buffer_sprintf(wb, "],\n   %sfull_chart_labels%s: [", kq, kq);
        dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
        for (i = 0, rd = temp_rd ? temp_rd : r->st->dimensions; rd; rd = rd->next) {
            st = rd->rrdset;
            if (likely(st->state)) {
                struct label_index *labels = &st->state->labels;
                if (labels->head) {
                    netdata_rwlock_rdlock(&labels->labels_rwlock);
                    for (struct label *label = labels->head; label; label = label->next) {
                        sanitize_json_string(value, label->value, RRD_ID_LENGTH_MAX * 2);
                        int len = snprintfz(output, RRD_ID_LENGTH_MAX * 2 + 7, "[\"%s\", \"%s\"]", label->key, value);
                        snprintfz(name, RRD_ID_LENGTH_MAX * 2, "%s:%s", label->key, value);
                        dictionary_set(dict, name, output, len + 1);
                    }
                    netdata_rwlock_unlock(&labels->labels_rwlock);
                }
            }
        }
        dictionary_get_all(dict, value_list_output, &co);
        dictionary_destroy(dict);
        buffer_strcat(wb, "],\n");
    }

    // Composite charts
    if (context_mode && temp_rd) {
        buffer_sprintf(
            wb,
            "   %schart_ids%s: [",
            kq, kq);

        for (c = 0, i = 0, rd = temp_rd ; rd && c < r->d; c++, rd = rd->next) {
            if (unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN))
                continue;
            if (unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO)))
                continue;

            if (i)
                buffer_strcat(wb, ", ");
            buffer_strcat(wb, sq);
            buffer_strcat(wb, rd->rrdset->id);
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
        if (chart_label_key) {
            buffer_sprintf(wb, "   %schart_labels%s: { ", kq, kq);

            SIMPLE_PATTERN *pattern = simple_pattern_create(chart_label_key, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);
            SIMPLE_PATTERN *original_pattern = pattern;
            char *label_key = NULL;
            int keys = 0;
            while (pattern && (label_key = simple_pattern_iterate(&pattern))) {
                uint32_t key_hash = simple_hash(label_key);
                struct label *current_label;

                if (keys)
                    buffer_strcat(wb, ", ");
                buffer_sprintf(wb, "%s%s%s : [", kq, label_key, kq);
                keys++;

                for (c = 0, i = 0, rd = temp_rd; rd && c < r->d; c++, rd = rd->next) {
                    if (unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN))
                        continue;
                    if (unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO)))
                        continue;
                    if (i)
                        buffer_strcat(wb, ", ");

                    current_label = rrdset_lookup_label_key(rd->rrdset, label_key, key_hash);
                    if (current_label) {
                        buffer_strcat(wb, sq);
                        buffer_strcat(wb, current_label->value);
                        buffer_strcat(wb, sq);
                    } else
                        buffer_strcat(wb, "null");
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
            simple_pattern_free(original_pattern);
        }
    }

    buffer_sprintf(wb, "   %slatest_values%s: ["
                   , kq, kq);

    for(c = 0, i = 0, rd = temp_rd?temp_rd:r->st->dimensions; rd && c < r->d ;c++, rd = rd->next) {
        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        if(i) buffer_strcat(wb, ", ");
        i++;

        calculated_number value = rd->last_stored_value;
        if (NAN == value)
            buffer_strcat(wb, "null");
        else
            buffer_rrd_value(wb, value);
        /*
        storage_number n = rd->values[rrdset_last_slot(r->st)];

        if(!does_storage_number_exist(n))
            buffer_strcat(wb, "null");
        else
            buffer_rrd_value(wb, unpack_storage_number(n));
        */
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
        calculated_number total = 1;

        if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
            total = 0;
            for(c = 0, rd = temp_rd?temp_rd:r->st->dimensions; rd && c < r->d ;c++, rd = rd->next) {
                calculated_number *cn = &r->v[ (rrdr_rows(r) - 1) * r->d ];
                calculated_number n = cn[c];

                if(likely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                    n = -n;

                total += n;
            }
            // prevent a division by zero
            if(total == 0) total = 1;
        }

        for(c = 0, i = 0, rd = temp_rd?temp_rd:r->st->dimensions; rd && c < r->d ;c++, rd = rd->next) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

            if(i) buffer_strcat(wb, ", ");
            i++;

            calculated_number *cn = &r->v[ (rrdr_rows(r) - 1) * r->d ];
            RRDR_VALUE_FLAGS *co = &r->o[ (rrdr_rows(r) - 1) * r->d ];
            calculated_number n = cn[c];

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

    if((options & RRDR_OPTION_CUSTOM_VARS) && (options & RRDR_OPTION_JSON_WRAP)) {
        buffer_sprintf(wb, "%s,\n   %schart_variables%s: ", sq, kq, kq);
        health_api_v1_chart_custom_variables2json(r->st, wb);
    }
    else
        buffer_sprintf(wb, "%s", sq);

    buffer_sprintf(wb, ",\n   %sresult%s: ", kq, kq);

    if(string_value) buffer_strcat(wb, sq);
    //info("JSONWRAPPER(): %s: END", r->st->id);
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
