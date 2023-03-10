// SPDX-License-Identifier: GPL-3.0-or-later

#include "web/api/web_api_v1.h"
#include "database/storage_engine.h"

inline bool query_target_has_percentage_units(struct query_target *qt) {
    if(qt->request.options & RRDR_OPTION_PERCENTAGE ||
       qt->request.time_group_method == RRDR_GROUPING_CV)
        return true;

    return false;
}

bool query_target_aggregatable(struct query_target *qt) {
    return (qt->request.options & RRDR_OPTION_RETURN_RAW);
}

void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb) {
    rrdset2json(st, wb, NULL, NULL, 0);
}

const char *rrdr_format_to_string(DATASOURCE_FORMAT format)  {
    switch(format) {
        case DATASOURCE_JSON:
            return DATASOURCE_FORMAT_JSON;

        case DATASOURCE_JSON2:
            return DATASOURCE_FORMAT_JSON2;

        case DATASOURCE_DATATABLE_JSON:
            return DATASOURCE_FORMAT_DATATABLE_JSON;

        case DATASOURCE_DATATABLE_JSONP:
            return DATASOURCE_FORMAT_DATATABLE_JSONP;

        case DATASOURCE_JSONP:
            return DATASOURCE_FORMAT_JSONP;

        case DATASOURCE_SSV:
            return DATASOURCE_FORMAT_SSV;

        case DATASOURCE_CSV:
            return DATASOURCE_FORMAT_CSV;

        case DATASOURCE_TSV:
            return DATASOURCE_FORMAT_TSV;

        case DATASOURCE_HTML:
            return DATASOURCE_FORMAT_HTML;

        case DATASOURCE_JS_ARRAY:
            return DATASOURCE_FORMAT_JS_ARRAY;

        case DATASOURCE_SSV_COMMA:
            return DATASOURCE_FORMAT_SSV_COMMA;

        default:
            return "unknown";
    }
}

int rrdset2value_api_v1(
        RRDSET *st
        , BUFFER *wb
        , NETDATA_DOUBLE *n
        , const char *dimensions
        , size_t points
        , time_t after
        , time_t before
        , RRDR_TIME_GROUPING group_method
        , const char *group_options
        , time_t resampling_time
        , uint32_t options
        , time_t *db_after
        , time_t *db_before
        , size_t *db_points_read
        , size_t *db_points_per_tier
        , size_t *result_points_generated
        , int *value_is_null
        , NETDATA_DOUBLE *anomaly_rate
        , time_t timeout
        , size_t tier
        , QUERY_SOURCE query_source
        , STORAGE_PRIORITY priority
) {
    int ret = HTTP_RESP_INTERNAL_SERVER_ERROR;

    ONEWAYALLOC *owa = onewayalloc_create(0);
    RRDR *r = rrd2rrdr_legacy(
            owa,
            st,
            points,
            after,
            before,
            group_method,
            resampling_time,
            options,
            dimensions,
            group_options,
            timeout,
            tier,
            query_source,
            priority);

    if(!r) {
        if(value_is_null) *value_is_null = 1;
        ret = HTTP_RESP_INTERNAL_SERVER_ERROR;
        goto cleanup;
    }

    if(db_points_read)
        *db_points_read += r->stats.db_points_read;

    if(db_points_per_tier) {
        for(size_t t = 0; t < storage_tiers ;t++)
            db_points_per_tier[t] += r->internal.qt->db.tiers[t].points;
    }

    if(result_points_generated)
        *result_points_generated += r->stats.result_points_generated;

    if(rrdr_rows(r) == 0) {
        if(db_after)  *db_after  = 0;
        if(db_before) *db_before = 0;
        if(value_is_null) *value_is_null = 1;

        ret = HTTP_RESP_BAD_REQUEST;
        goto cleanup;
    }

    if(wb) {
        if (r->view.flags & RRDR_RESULT_FLAG_RELATIVE)
            buffer_no_cacheable(wb);
        else if (r->view.flags & RRDR_RESULT_FLAG_ABSOLUTE)
            buffer_cacheable(wb);
    }

    if(db_after)  *db_after  = r->view.after;
    if(db_before) *db_before = r->view.before;

    long i = (!(options & RRDR_OPTION_REVERSED))?(long)rrdr_rows(r) - 1:0;
    *n = rrdr2value(r, i, options, value_is_null, anomaly_rate);
    ret = HTTP_RESP_OK;

cleanup:
    rrdr_free(owa, r);
    onewayalloc_destroy(owa);
    return ret;
}

struct group_by_entry {
    size_t priority;
    size_t count;
    STRING *id;
    STRING *name;
    STRING *units;
    RRDR_DIMENSION_FLAGS od;
};

static int group_by_label_is_space(char c) {
    if(c == ',' || c == '|')
        return 1;

    return 0;
}

static RRDR *data_query_group_by(RRDR *r) {
    QUERY_TARGET *qt = r->internal.qt;
    RRDR_OPTIONS options = qt->request.options;
    size_t rows = rrdr_rows(r);

    if(qt->request.group_by == RRDR_GROUP_BY_NONE || !rows)
        return r;

    struct group_by_entry *entries = onewayalloc_callocz(r->internal.owa, qt->query.used, sizeof(struct group_by_entry));
    DICTIONARY *groups = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

    if(qt->request.group_by & RRDR_GROUP_BY_LABEL && qt->request.group_by_label && *qt->request.group_by_label)
        qt->group_by.used = quoted_strings_splitter(qt->request.group_by_label, qt->group_by.label_keys, GROUP_BY_MAX_LABEL_KEYS, group_by_label_is_space);

    if(!qt->group_by.used)
        qt->request.group_by &= ~RRDR_GROUP_BY_LABEL;

    if(!(qt->request.group_by & (RRDR_GROUP_BY_NODE | RRDR_GROUP_BY_INSTANCE | RRDR_GROUP_BY_DIMENSION | RRDR_GROUP_BY_LABEL | RRDR_GROUP_BY_SELECTED)))
        qt->request.group_by = RRDR_GROUP_BY_DIMENSION;

    int added = 0;
    BUFFER *key = buffer_create(0, NULL);
    QUERY_INSTANCE *last_qi = NULL;
    size_t priority = 0;
    time_t update_every_max = 0;
    for(size_t d = 0; d < qt->query.used ; d++) {
        if(unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
            continue;

        QUERY_METRIC *qm = query_metric(qt, d);
        QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
        QUERY_NODE *qn = query_node(qt, qm->link.query_node_id);

        if(qi != last_qi) {
            priority = 0;
            last_qi = qi;

            time_t update_every = rrdinstance_acquired_update_every(qi->ria);
            if(update_every > update_every_max)
                update_every_max = update_every;
        }
        else
            priority++;

        // --------------------------------------------------------------------
        // generate the group by key

        buffer_flush(key);
        if(unlikely(r->od[d] & RRDR_DIMENSION_HIDDEN)) {
            buffer_strcat(key, "__hidden_dimensions__");
        }
        else if(unlikely(qt->request.group_by & RRDR_GROUP_BY_SELECTED)) {
            buffer_strcat(key, "selected");
        }
        else {
            if (qt->request.group_by & RRDR_GROUP_BY_DIMENSION) {
                buffer_fast_strcat(key, "|", 1);
                buffer_strcat(key, query_metric_id(qt, qm));
            }

            if (qt->request.group_by & RRDR_GROUP_BY_INSTANCE) {
                buffer_fast_strcat(key, "|", 1);
                buffer_strcat(key, string2str(query_instance_id_fqdn(qt, qi)));
            }

            if (qt->request.group_by & RRDR_GROUP_BY_LABEL) {
                DICTIONARY *labels = rrdinstance_acquired_labels(qi->ria);
                for (size_t l = 0; l < qt->group_by.used; l++) {
                    buffer_fast_strcat(key, "|", 1);
                    rrdlabels_get_value_to_buffer_or_unset(labels, key, qt->group_by.label_keys[l], "[unset]");
                }
            }

            if (qt->request.group_by & RRDR_GROUP_BY_NODE) {
                buffer_fast_strcat(key, "|", 1);
                buffer_strcat(key, qn->rrdhost->machine_guid);
            }

            // append the units
            if (query_target_has_percentage_units(qt)) {
                buffer_fast_strcat(key, "|%", 2);
            } else {
                buffer_fast_strcat(key, "|", 1);
                buffer_strcat(key, rrdinstance_acquired_units(qi->ria));
            }
        }

        // lookup the key in the dictionary

        int pos = -1;
        int *set = dictionary_set(groups, buffer_tostring(key), &pos, sizeof(pos));
        if(*set == -1) {
            // the key just added to the dictionary

            *set = pos = added++;

            // ----------------------------------------------------------------
            // generate the dimension id

            buffer_flush(key);
            if(unlikely(r->od[d] & RRDR_DIMENSION_HIDDEN)) {
                buffer_strcat(key, "__hidden_dimensions__");
            }
            else if(unlikely(qt->request.group_by & RRDR_GROUP_BY_SELECTED)) {
                buffer_strcat(key, "selected");
            }
            else {
                if (qt->request.group_by & RRDR_GROUP_BY_DIMENSION) {
                    buffer_strcat(key, query_metric_id(qt, qm));
                }

                if (qt->request.group_by & RRDR_GROUP_BY_INSTANCE) {
                    if (buffer_strlen(key) != 0)
                        buffer_fast_strcat(key, ",", 1);

                    if (qt->request.group_by & RRDR_GROUP_BY_NODE)
                        buffer_strcat(key, rrdinstance_acquired_id(qi->ria));
                    else
                        buffer_strcat(key, string2str(query_instance_id_fqdn(qt, qi)));
                }

                if (qt->request.group_by & RRDR_GROUP_BY_LABEL) {
                    DICTIONARY *labels = rrdinstance_acquired_labels(qi->ria);
                    for (size_t l = 0; l < qt->group_by.used; l++) {
                        if (buffer_strlen(key) != 0)
                            buffer_fast_strcat(key, ",", 1);
                        rrdlabels_get_value_to_buffer_or_unset(labels, key, qt->group_by.label_keys[l], "[unset]");
                    }
                }

                if (qt->request.group_by & RRDR_GROUP_BY_NODE) {
                    if (buffer_strlen(key) != 0)
                        buffer_fast_strcat(key, ",", 1);

                    buffer_strcat(key, qn->rrdhost->machine_guid);
                }
            }

            entries[pos].id = string_strdupz(buffer_tostring(key));

            // ----------------------------------------------------------------
            // generate the dimension name

            buffer_flush(key);
            if(unlikely(r->od[d] & RRDR_DIMENSION_HIDDEN)) {
                buffer_strcat(key, "__hidden_dimensions__");
            }
            else if(unlikely(qt->request.group_by & RRDR_GROUP_BY_SELECTED)) {
                buffer_strcat(key, "selected");
            }
            else {
                if (qt->request.group_by & RRDR_GROUP_BY_DIMENSION) {
                    buffer_strcat(key, query_metric_name(qt, qm));
                }

                if (qt->request.group_by & RRDR_GROUP_BY_INSTANCE) {
                    if (buffer_strlen(key) != 0)
                        buffer_fast_strcat(key, ",", 1);

                    if (qt->request.group_by & RRDR_GROUP_BY_NODE)
                        buffer_strcat(key, rrdinstance_acquired_name(qi->ria));
                    else
                        buffer_strcat(key, string2str(query_instance_name_fqdn(qt, qi)));
                }

                if (qt->request.group_by & RRDR_GROUP_BY_LABEL) {
                    DICTIONARY *labels = rrdinstance_acquired_labels(qi->ria);
                    for (size_t l = 0; l < qt->group_by.used; l++) {
                        if (buffer_strlen(key) != 0)
                            buffer_fast_strcat(key, ",", 1);
                        rrdlabels_get_value_to_buffer_or_unset(labels, key, qt->group_by.label_keys[l], "[unset]");
                    }
                }

                if (qt->request.group_by & RRDR_GROUP_BY_NODE) {
                    if (buffer_strlen(key) != 0)
                        buffer_fast_strcat(key, ",", 1);

                    buffer_strcat(key, rrdhost_hostname(qn->rrdhost));
                }
            }

            entries[pos].name = string_strdupz(buffer_tostring(key));

            // add the rest of the info
            entries[pos].units = rrdinstance_acquired_units_dup(qi->ria);
            entries[pos].priority = priority;
        }
        else {
            // the key found in the dictionary
            pos = *set;
        }

        entries[pos].count++;

        if(unlikely(priority < entries[pos].priority))
            entries[pos].priority = priority;

        qm->grouped_as.slot = pos;
        qm->grouped_as.id = entries[pos].id;
        qm->grouped_as.name = entries[pos].name;
        qm->grouped_as.units = entries[pos].units;

        // copy the dimension flags decided by the query target
        // we need this, because if a dimension is explicitly selected
        // the query target adds to it the non-zero flag
        qm->status |= RRDR_DIMENSION_GROUPED | r->od[d];
        entries[pos].od |= RRDR_DIMENSION_GROUPED | r->od[d];
    }

    // check if we have multiple units
    bool multiple_units = false;
    for(int i = 1; i < added ; i++) {
        if(entries[i].units != entries[0].units) {
            multiple_units = true;
            break;
        }
    }

    if(multiple_units) {
        // include the units into the id and name of the dimensions
        for(int i = 0; i < added ; i++) {
            buffer_flush(key);
            buffer_strcat(key, string2str(entries[i].id));
            buffer_fast_strcat(key, ",", 1);
            buffer_strcat(key, string2str(entries[i].units));
            STRING *u = string_strdupz(buffer_tostring(key));
            string_freez(entries[i].id);
            entries[i].id = u;
        }
    }

    RRDR *r2 = rrdr_create(r->internal.owa, qt, added, rows);
    if(!r2)
        goto cleanup;

    r2->dp = onewayalloc_callocz(r2->internal.owa, r2->d, sizeof(*r2->dp));
    r2->dgbc = onewayalloc_callocz(r2->internal.owa, r2->d, sizeof(*r2->dgbc));
    r2->gbc = onewayalloc_callocz(r2->internal.owa, r2->n * r2->d, sizeof(*r2->gbc));

    // copy from previous rrdr
    r2->view = r->view;
    r2->stats = r->stats;
    r2->rows = rows;
    r2->stats.result_points_generated = r2->d * r2->n;

    // initialize r2 (dimension options, names, and ids)
    for(size_t d2 = 0; d2 < r2->d ; d2++) {
        r2->od[d2] = entries[d2].od;
        r2->di[d2] = entries[d2].id;
        r2->dn[d2] = entries[d2].name;
        r2->du[d2] = entries[d2].units;
        r2->dp[d2] = entries[d2].priority;
        r2->dgbc[d2] = entries[d2].count;
    }

    r2->partial_data_trimming.max_update_every = update_every_max;
    r2->partial_data_trimming.expected_after =
            (!(qt->request.options & RRDR_OPTION_RETURN_RAW) && qt->window.before >= qt->window.now - update_every_max) ?
            qt->window.before - update_every_max :
            qt->window.before;
    r2->partial_data_trimming.trimmed_after = qt->window.before;

    // initialize r2 (timestamps and value flags)
    for(size_t i = 0; i != rows ;i++) {
        // copy the timestamp
        r2->t[i] = r->t[i];

        // make all values empty
        NETDATA_DOUBLE *cn2 = &r2->v[ i * r2->d ];
        RRDR_VALUE_FLAGS *co2 = &r2->o[ i * r2->d ];
        NETDATA_DOUBLE *ar2 = &r2->ar[ i * r2->d ];
        for (size_t d2 = 0; d2 < r2->d; d2++) {
            cn2[d2] = 0.0;
            ar2[d2] = 0.0;
            co2[d2] = RRDR_VALUE_EMPTY;
        }
    }

    // do the group_by
    size_t last_row_gbc = 0;
    for(size_t i = 0; i != rows ;i++) {

        size_t idx = i * r->d;
        NETDATA_DOUBLE *cn_base = &r->v[ idx ];
        RRDR_VALUE_FLAGS *co_base = &r->o[ idx ];
        NETDATA_DOUBLE *ar_base = &r->ar[ idx ];

        size_t idx2 = i * r2->d;
        NETDATA_DOUBLE *cn2_base = &r2->v[ idx2 ];
        RRDR_VALUE_FLAGS *co2_base = &r2->o[ idx2 ];
        NETDATA_DOUBLE *ar2_base = &r2->ar[ idx2 ];
        uint32_t *gbc2_base = &r2->gbc[ idx2 ];

        size_t row_gbc = 0;
        for(size_t d = 0; d < r->d ; d++) {
            if(unlikely(!(r->od[d] & RRDR_DIMENSION_QUERIED)))
                continue;

            NETDATA_DOUBLE n = cn_base[d];
            RRDR_VALUE_FLAGS o = co_base[d];
            NETDATA_DOUBLE ar = ar_base[d];

            if(o & RRDR_VALUE_EMPTY) {
                if(options & RRDR_OPTION_NULL2ZERO)
                    n = 0.0;
                else
                    continue;
            }

            if(unlikely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                n = -n;

            QUERY_METRIC *qm = query_metric(qt, d);
            size_t d2 = qm->grouped_as.slot;

            NETDATA_DOUBLE *cn2 = &cn2_base[d2];
            RRDR_VALUE_FLAGS *co2 = &co2_base[d2];
            NETDATA_DOUBLE *ar2 = &ar2_base[d2];
            uint32_t *gbc2 = &gbc2_base[d2];

            switch(qt->request.group_by_aggregate_function) {
                default:
                case RRDR_GROUP_BY_FUNCTION_AVERAGE:
                case RRDR_GROUP_BY_FUNCTION_SUM:
                    *cn2 += n;
                    break;

                case RRDR_GROUP_BY_FUNCTION_MIN:
                    if(n < *cn2)
                        *cn2 = n;
                    break;

                case RRDR_GROUP_BY_FUNCTION_MAX:
                    if(n > *cn2)
                        *cn2 = n;
                    break;
            }

            *co2 |= (o & (RRDR_VALUE_RESET|RRDR_VALUE_PARTIAL));
            *ar2 += ar;
            (*gbc2)++;

            row_gbc++;
        }

        if(unlikely(r->t[i] > r2->partial_data_trimming.expected_after && row_gbc < last_row_gbc)) {
            // discard the rest of the points
            r2->partial_data_trimming.trimmed_after = r->t[i];
            r2->rows = i;
            rows = i;
            break;
        }
        else
            last_row_gbc = row_gbc;
    }

    // apply averaging, remove RRDR_VALUE_EMPTY, find the non-zero dimensions, min and max
    size_t min_max_values = 0;
    NETDATA_DOUBLE min = NAN, max = NAN;
    for (size_t d2 = 0; d2 < r2->d; d2++) {
        size_t non_zero = 0;

        for(size_t i = 0; i != rows ;i++) {
            size_t idx2 = i * r2->d + d2;

            NETDATA_DOUBLE *cn2 = &r2->v[ idx2 ];
            RRDR_VALUE_FLAGS *co2 = &r2->o[ idx2 ];
            NETDATA_DOUBLE *ar2 = &r2->ar[ idx2 ];
            uint32_t gbc2 = r2->gbc[ idx2 ];

            if(likely(gbc2)) {
                *co2 &= ~RRDR_VALUE_EMPTY;

                if(gbc2 != r2->dgbc[d2])
                    *co2 |= RRDR_VALUE_PARTIAL;

                NETDATA_DOUBLE n;

                if(qt->request.group_by_aggregate_function == RRDR_GROUP_BY_FUNCTION_AVERAGE)
                    n = (*cn2 /= gbc2);
                else
                    n = *cn2;

                if(!query_target_aggregatable(qt))
                    *ar2 /= gbc2;

                if(islessgreater(n, 0.0))
                    non_zero++;

                if(unlikely(!min_max_values++)) {
                    min = n;
                    max = n;
                }
                else {
                    if(n < min)
                        min = n;

                    if(n > max)
                        max = n;
                }
            }
        }

        if(non_zero)
            r2->od[d2] |= RRDR_DIMENSION_NONZERO;
    }

    r2->view.min = min;
    r2->view.max = max;

cleanup:
    buffer_free(key);

    if(!r2 && entries && added) {
        for(int d2 = 0; d2 < added ; d2++) {
            string_freez(entries[d2].id);
            string_freez(entries[d2].name);
        }
    }
    onewayalloc_freez(r->internal.owa, entries);
    dictionary_destroy(groups);

    return r2;
}

static inline void buffer_json_member_add_key_only(BUFFER *wb, const char *key) {
    buffer_print_json_comma_newline_spacing(wb);
    buffer_print_json_key(wb, key);
    buffer_fast_strcat(wb, ":", 1);
    wb->json.stack[wb->json.depth].count++;
}

static inline void buffer_json_member_add_string_open(BUFFER *wb, const char *key) {
    buffer_json_member_add_key_only(wb, key);
    buffer_strcat(wb, wb->json.value_quote);
}

static inline void buffer_json_member_add_string_close(BUFFER *wb) {
    buffer_strcat(wb, wb->json.value_quote);
}

int data_query_execute(ONEWAYALLOC *owa, BUFFER *wb, QUERY_TARGET *qt, time_t *latest_timestamp) {
    wrapper_begin_t wrapper_begin = rrdr_json_wrapper_begin;
    wrapper_end_t wrapper_end = rrdr_json_wrapper_end;

    if(qt->request.version == 2)
        wrapper_begin = rrdr_json_wrapper_begin2;

    RRDR *r1 = rrd2rrdr(owa, qt);
    qt->timings.executed_ut = now_monotonic_usec();

    if(!r1) {
        buffer_strcat(wb, "Cannot generate output with these parameters on this chart.");
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    if (r1->view.flags & RRDR_RESULT_FLAG_CANCEL) {
        rrdr_free(owa, r1);
        return HTTP_RESP_BACKEND_FETCH_FAILED;
    }

    if(r1->view.flags & RRDR_RESULT_FLAG_RELATIVE)
        buffer_no_cacheable(wb);
    else if(r1->view.flags & RRDR_RESULT_FLAG_ABSOLUTE)
        buffer_cacheable(wb);

    if(latest_timestamp && rrdr_rows(r1) > 0)
        *latest_timestamp = r1->view.before;

    DATASOURCE_FORMAT format = qt->request.format;
    RRDR_OPTIONS options = qt->request.options;
    RRDR_TIME_GROUPING group_method = qt->request.time_group_method;

    RRDR *r = data_query_group_by(r1);
    qt->timings.group_by_ut = now_monotonic_usec();

    switch(format) {
    case DATASOURCE_SSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2ssv(r, wb, options, "", " ", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb, format, options);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", " ", "");
        }
        break;

    case DATASOURCE_SSV_COMMA:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2ssv(r, wb, options, "", ",", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb, format, options);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", ",", "");
        }
        break;

    case DATASOURCE_JS_ARRAY:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_array(wb, "result");
            rrdr2ssv(r, wb, options, "", ",", "");
            buffer_json_array_close(wb);
            wrapper_end(r, wb, format, options);
        }
        else {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr2ssv(r, wb, options, "[", ",", "]");
        }
        break;

    case DATASOURCE_CSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2csv(r, wb, format, options, "", ",", "\\n", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb, format, options);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", ",", "\r\n", "");
        }
        break;

    case DATASOURCE_CSV_MARKDOWN:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2csv(r, wb, format, options, "", "|", "\\n", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb, format, options);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "|", "\r\n", "");
        }
        break;

    case DATASOURCE_CSV_JSON_ARRAY:
        wb->content_type = CT_APPLICATION_JSON;
        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_array(wb, "result");
            rrdr2csv(r, wb, format, options + RRDR_OPTION_LABEL_QUOTES, "[", ",", "]", ",\n");
            buffer_json_array_close(wb);
            wrapper_end(r, wb, format, options);
        }
        else {
            wb->content_type = CT_APPLICATION_JSON;
            buffer_strcat(wb, "[\n");
            rrdr2csv(r, wb, format, options + RRDR_OPTION_LABEL_QUOTES, "[", ",", "]", ",\n");
            buffer_strcat(wb, "\n]");
        }
        break;

    case DATASOURCE_TSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2csv(r, wb, format, options, "", "\t", "\\n", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb, format, options);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "\t", "\r\n", "");
        }
        break;

    case DATASOURCE_HTML:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_string_open(wb, "result");
            buffer_strcat(wb, "<html>\\n<center>\\n<table border=\\\"0\\\" cellpadding=\\\"5\\\" cellspacing=\\\"5\\\">\\n");
            rrdr2csv(r, wb, format, options, "<tr><td>", "</td><td>", "</td></tr>\\n", "");
            buffer_strcat(wb, "</table>\\n</center>\\n</html>\\n");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb, format, options);
        }
        else {
            wb->content_type = CT_TEXT_HTML;
            buffer_strcat(wb, "<html>\n<center>\n<table border=\"0\" cellpadding=\"5\" cellspacing=\"5\">\n");
            rrdr2csv(r, wb, format, options, "<tr><td>", "</td><td>", "</td></tr>\n", "");
            buffer_strcat(wb, "</table>\n</center>\n</html>\n");
        }
        break;

    case DATASOURCE_DATATABLE_JSONP:
        wb->content_type = CT_APPLICATION_X_JAVASCRIPT;

        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_key_only(wb, "result");
        }

        rrdr2json(r, wb, options, 1);

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_end(r, wb, format, options);

        break;

    case DATASOURCE_DATATABLE_JSON:
        wb->content_type = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_key_only(wb, "result");
        }

        rrdr2json(r, wb, options, 1);

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_end(r, wb, format, options);

        break;

    case DATASOURCE_JSONP:
        wb->content_type = CT_APPLICATION_X_JAVASCRIPT;
        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_key_only(wb, "result");
        }

        rrdr2json(r, wb, options, 0);

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_end(r, wb, format, options);

        break;

    case DATASOURCE_JSON:
    default:
        wb->content_type = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb, format, options, group_method);
            buffer_json_member_add_key_only(wb, "result");
        }

        rrdr2json(r, wb, options, 0);

        if(options & RRDR_OPTION_JSON_WRAP) {
            if (query_target_aggregatable(qt)) {
                buffer_json_member_add_key_only(wb, "group_by_count");
                rrdr2json(r, wb, options | RRDR_OPTION_INTERNAL_GBC, false);
            }
            if (options & RRDR_OPTION_RETURN_JWAR) {
                buffer_json_member_add_key_only(wb, "anomaly_rates");
                rrdr2json(r, wb, options | RRDR_OPTION_INTERNAL_AR, false);
            }
            wrapper_end(r, wb, format, options);
        }
        break;

    case DATASOURCE_JSON2:
        wb->content_type = CT_APPLICATION_JSON;
        wrapper_begin(r, wb, format, options, group_method);
        rrdr2json_v2(r, wb, format, options);
        wrapper_end(r, wb, format, options);
        break;
    }

    if(r != r1)
        rrdr_free(owa, r);

    rrdr_free(owa, r1);
    return HTTP_RESP_OK;
}
