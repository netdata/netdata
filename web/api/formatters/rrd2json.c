// SPDX-License-Identifier: GPL-3.0-or-later

#include "web/api/web_api_v1.h"
#include "database/storage_engine.h"

void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb) {
    rrdset2json(st, wb, NULL, NULL, 0);
}

const char *rrdr_format_to_string(DATASOURCE_FORMAT format)  {
    switch(format) {
        case DATASOURCE_JSON:
            return DATASOURCE_FORMAT_JSON;
            break;

        case DATASOURCE_DATATABLE_JSON:
            return DATASOURCE_FORMAT_DATATABLE_JSON;
            break;

        case DATASOURCE_DATATABLE_JSONP:
            return DATASOURCE_FORMAT_DATATABLE_JSONP;
            break;

        case DATASOURCE_JSONP:
            return DATASOURCE_FORMAT_JSONP;
            break;

        case DATASOURCE_SSV:
            return DATASOURCE_FORMAT_SSV;
            break;

        case DATASOURCE_CSV:
            return DATASOURCE_FORMAT_CSV;
            break;

        case DATASOURCE_TSV:
            return DATASOURCE_FORMAT_TSV;
            break;

        case DATASOURCE_HTML:
            return DATASOURCE_FORMAT_HTML;
            break;

        case DATASOURCE_JS_ARRAY:
            return DATASOURCE_FORMAT_JS_ARRAY;
            break;

        case DATASOURCE_SSV_COMMA:
            return DATASOURCE_FORMAT_SSV_COMMA;
            break;

        default:
            return "unknown";
            break;
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
            db_points_per_tier[t] += r->stats.tier_points_read[t];
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
};

static int group_by_label_is_space(char c) {
    if(c == ',' || c == '|')
        return 1;

    return 0;
}

RRDR *data_query_group_by(RRDR *r) {
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

    if(!(qt->request.group_by & (RRDR_GROUP_BY_NODE | RRDR_GROUP_BY_INSTANCE | RRDR_GROUP_BY_DIMENSION | RRDR_GROUP_BY_LABEL)))
        qt->request.group_by = RRDR_GROUP_BY_DIMENSION;

    int added = 0;
    BUFFER *key = buffer_create(0, NULL);
    QUERY_INSTANCE *last_qi = NULL;
    size_t priority = 0;
    for(size_t c = 0; c < qt->query.used ;c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        QUERY_METRIC *qm = query_metric(qt, c);
        QUERY_INSTANCE *qi = query_instance(qt, qm->link.query_instance_id);
        QUERY_HOST *qh = query_host(qt, qm->link.query_host_id);

        if(qi != last_qi) {
            priority = 0;
            last_qi = qi;
        }
        else
            priority++;

        // generate the group by key

        buffer_flush(key);
        if(qt->request.group_by & RRDR_GROUP_BY_DIMENSION) {
            buffer_fast_strcat(key, "|", 1);
            buffer_strcat(key, query_metric_id(qt, qm));
        }
        if(qt->request.group_by & RRDR_GROUP_BY_INSTANCE) {
            buffer_fast_strcat(key, "|", 1);
            buffer_strcat(key, string2str(qi->id_fqdn));
        }
        if(qt->request.group_by & RRDR_GROUP_BY_LABEL) {
            DICTIONARY *labels = rrdinstance_acquired_labels(qi->ria);
            for(size_t l = 0; l < qt->group_by.used ;l++) {
                buffer_fast_strcat(key, "|", 1);
                rrdlabels_get_value_to_buffer_or_unset(labels, key, qt->group_by.label_keys[l], "[unset]");
            }
        }
        if(qt->request.group_by & RRDR_GROUP_BY_NODE) {
            buffer_fast_strcat(key, "|", 1);
            buffer_strcat(key, qh->host->machine_guid);
        }
        buffer_fast_strcat(key, "|", 1);
        buffer_strcat(key, rrdinstance_acquired_units(qi->ria));

        // lookup the key in the dictionary

        int pos = -1;
        int *set = dictionary_set(groups, buffer_tostring(key), &pos, sizeof(pos));
        if(*set == -1) {
            // the key just added to the dictionary

            *set = pos = added++;

            // generate the dimension id

            buffer_flush(key);
            if(qt->request.group_by & RRDR_GROUP_BY_DIMENSION) {
                buffer_strcat(key, query_metric_id(qt, qm));
            }
            if(qt->request.group_by & RRDR_GROUP_BY_INSTANCE) {
                if(buffer_strlen(key) != 0)
                    buffer_fast_strcat(key, ",", 1);

                if(qt->request.group_by & RRDR_GROUP_BY_NODE)
                    buffer_strcat(key, rrdinstance_acquired_id(qi->ria));
                else
                    buffer_strcat(key, string2str(qi->id_fqdn));
            }
            if(qt->request.group_by & RRDR_GROUP_BY_LABEL) {
                DICTIONARY *labels = rrdinstance_acquired_labels(qi->ria);
                for(size_t l = 0; l < qt->group_by.used ;l++) {
                    if(buffer_strlen(key) != 0)
                        buffer_fast_strcat(key, ",", 1);
                    rrdlabels_get_value_to_buffer_or_unset(labels, key, qt->group_by.label_keys[l], "[unset]");
                }
            }
            if(qt->request.group_by & RRDR_GROUP_BY_NODE) {
                if(buffer_strlen(key) != 0)
                    buffer_fast_strcat(key, ",", 1);

                buffer_strcat(key, qh->host->machine_guid);
            }
            entries[pos].id = string_strdupz(buffer_tostring(key));

            // generate the dimension name

            buffer_flush(key);
            if(qt->request.group_by & RRDR_GROUP_BY_DIMENSION) {
                buffer_strcat(key, query_metric_name(qt, qm));
            }
            if(qt->request.group_by & RRDR_GROUP_BY_INSTANCE) {
                if(buffer_strlen(key) != 0)
                    buffer_fast_strcat(key, ",", 1);

                if(qt->request.group_by & RRDR_GROUP_BY_NODE)
                    buffer_strcat(key, rrdinstance_acquired_name(qi->ria));
                else
                    buffer_strcat(key, string2str(qi->name_fqdn));
            }
            if(qt->request.group_by & RRDR_GROUP_BY_LABEL) {
                DICTIONARY *labels = rrdinstance_acquired_labels(qi->ria);
                for(size_t l = 0; l < qt->group_by.used ;l++) {
                    if(buffer_strlen(key) != 0)
                        buffer_fast_strcat(key, ",", 1);
                    rrdlabels_get_value_to_buffer_or_unset(labels, key, qt->group_by.label_keys[l], "[unset]");
                }
            }
            if(qt->request.group_by & RRDR_GROUP_BY_NODE) {
                if(buffer_strlen(key) != 0)
                    buffer_fast_strcat(key, ",", 1);

                buffer_strcat(key, rrdhost_hostname(qh->host));
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
        qm->status |= RRDR_DIMENSION_GROUPED;
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
    for(size_t c2 = 0; c2 < r2->d ; c2++) {
        r2->od[c2] = RRDR_DIMENSION_QUERIED;
        r2->di[c2] = entries[c2].id;
        r2->dn[c2] = entries[c2].name;
        r2->du[c2] = entries[c2].units;
        r2->dp[c2] = entries[c2].priority;
        r2->dgbc[c2] = entries[c2].count;
    }

    // initialize r2 (timestamps and value flags)
    for(size_t i = 0; i != rows ;i++) {
        // copy the timestamp
        r2->t[i] = r->t[i];

        // make all values empty
        NETDATA_DOUBLE *cn2 = &r2->v[ i * r2->d ];
        RRDR_VALUE_FLAGS *co2 = &r2->o[ i * r2->d ];
        NETDATA_DOUBLE *ar2 = &r2->ar[ i * r2->d ];
        for (size_t c2 = 0; c2 < r2->d; c2++) {
            cn2[c2] = 0.0;
            ar2[c2] = 0.0;
            co2[c2] = RRDR_VALUE_EMPTY;
        }
    }

    // do the group_by
    for(size_t i = 0; i != rows ;i++) {
        size_t idx = i * r->d;
        size_t idx2 = i * r2->d;

        NETDATA_DOUBLE *cn = &r->v[ idx ];
        RRDR_VALUE_FLAGS *co = &r->o[ idx ];
        NETDATA_DOUBLE *ar = &r->ar[ idx ];

        NETDATA_DOUBLE *cn2 = &r2->v[ idx2 ];
        RRDR_VALUE_FLAGS *co2 = &r2->o[ idx2 ];
        NETDATA_DOUBLE *ar2 = &r2->ar[ idx2 ];
        uint32_t *gbc2 = &r2->gbc[ idx2 ];

        for(size_t c = 0; c < r->d ;c++) {
            if (!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            NETDATA_DOUBLE n = cn[c];
            RRDR_VALUE_FLAGS o = co[c];

            if(o & RRDR_VALUE_EMPTY) {
                if(options & RRDR_OPTION_NULL2ZERO)
                    n = 0.0;
                else
                    continue;
            }

            if(unlikely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                n = -n;

            QUERY_METRIC *qm = query_metric(qt, c);
            size_t c2 = qm->grouped_as.slot;

            switch(qt->request.group_by_aggregate_function) {
                default:
                case RRDR_GROUP_BY_FUNCTION_AVERAGE:
                case RRDR_GROUP_BY_FUNCTION_SUM:
                case RRDR_GROUP_BY_FUNCTION_SUM_COUNT:
                    cn2[c2] += n;
                    break;

                case RRDR_GROUP_BY_FUNCTION_MIN:
                    if(n < cn2[c2])
                        cn2[c2] = n;
                    break;

                case RRDR_GROUP_BY_FUNCTION_MAX:
                    if(n > cn2[c2])
                        cn2[c2] = n;
                    break;
            }

            if(o & RRDR_VALUE_RESET)
                co2[c2] |= RRDR_VALUE_RESET;

            ar2[c2] += ar[c];
            gbc2[c2]++;
        }
    }

    // apply averaging, remove RRDR_VALUE_EMPTY, find the non-zero dimensions, min and max
    size_t values = 0;
    NETDATA_DOUBLE min = NAN, max = NAN;
    for (size_t c2 = 0; c2 < r2->d; c2++) {
        size_t non_zero = 0;

        for(size_t i = 0; i != rows ;i++) {
            size_t idx2 = i * r2->d + c2;

            NETDATA_DOUBLE *cn2 = &r2->v[ idx2 ];
            RRDR_VALUE_FLAGS *co2 = &r2->o[ idx2 ];
            NETDATA_DOUBLE *ar2 = &r2->ar[ idx2 ];
            uint32_t *gbc2 = &r2->gbc[ idx2 ];

            if(likely(*gbc2)) {
                *co2 &= ~RRDR_VALUE_EMPTY;

                *ar2 /= *gbc2;

                NETDATA_DOUBLE n;

                if(qt->request.group_by_aggregate_function == RRDR_GROUP_BY_FUNCTION_SUM_COUNT) {
                    n = *cn2 / *gbc2;
                }
                else if(qt->request.group_by_aggregate_function == RRDR_GROUP_BY_FUNCTION_AVERAGE) {
                    n = *cn2 / *gbc2;
                    *cn2 = n;
                }
                else
                    n = *cn2;

                if(islessgreater(n, 0.0))
                    non_zero++;

                if(unlikely(!values++)) {
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
            r2->od[c2] |= RRDR_DIMENSION_NONZERO;
    }

    r2->view.min = min;
    r2->view.max = max;

cleanup:
    buffer_free(key);

    if(!r2 && entries && added) {
        for(long c = 0; c < added ;c++) {
            string_freez(entries[c].id);
            string_freez(entries[c].name);
        }
    }
    onewayalloc_freez(r->internal.owa, entries);
    dictionary_destroy(groups);

    return r2;
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
            wrapper_begin(r, wb, format, options, true, group_method);
            rrdr2ssv(r, wb, options, "", " ", "");
            wrapper_end(r, wb, format, options, true);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", " ", "");
        }
        break;

    case DATASOURCE_SSV_COMMA:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, true, group_method);
            rrdr2ssv(r, wb, options, "", ",", "");
            wrapper_end(r, wb, format, options, true);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", ",", "");
        }
        break;

    case DATASOURCE_JS_ARRAY:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, false, group_method);
            rrdr2ssv(r, wb, options, "[", ",", "]");
            wrapper_end(r, wb, format, options, false);
        }
        else {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr2ssv(r, wb, options, "[", ",", "]");
        }
        break;

    case DATASOURCE_CSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, true, group_method);
            rrdr2csv(r, wb, format, options, "", ",", "\\n", "");
            wrapper_end(r, wb, format, options, true);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", ",", "\r\n", "");
        }
        break;

    case DATASOURCE_CSV_MARKDOWN:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, true, group_method);
            rrdr2csv(r, wb, format, options, "", "|", "\\n", "");
            wrapper_end(r, wb, format, options, true);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "|", "\r\n", "");
        }
        break;

    case DATASOURCE_CSV_JSON_ARRAY:
        wb->content_type = CT_APPLICATION_JSON;
        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb, format, options, false, group_method);
            buffer_strcat(wb, "[\n");
            rrdr2csv(r, wb, format, options + RRDR_OPTION_LABEL_QUOTES, "[", ",", "]", ",\n");
            buffer_strcat(wb, "\n]");
            wrapper_end(r, wb, format, options, false);
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
            wrapper_begin(r, wb, format, options, true, group_method);
            rrdr2csv(r, wb, format, options, "", "\t", "\\n", "");
            wrapper_end(r, wb, format, options, true);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "\t", "\r\n", "");
        }
        break;

    case DATASOURCE_HTML:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb, format, options, true, group_method);
            buffer_strcat(wb, "<html>\\n<center>\\n<table border=\\\"0\\\" cellpadding=\\\"5\\\" cellspacing=\\\"5\\\">\\n");
            rrdr2csv(r, wb, format, options, "<tr><td>", "</td><td>", "</td></tr>\\n", "");
            buffer_strcat(wb, "</table>\\n</center>\\n</html>\\n");
            wrapper_end(r, wb, format, options, true);
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

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_begin(r, wb, format, options, false, group_method);

        rrdr2json(r, wb, options, 1);

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_end(r, wb, format, options, false);
        break;

    case DATASOURCE_DATATABLE_JSON:
        wb->content_type = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_begin(r, wb, format, options, false, group_method);

        rrdr2json(r, wb, options, 1);

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_end(r, wb, format, options, false);
        break;

    case DATASOURCE_JSONP:
        wb->content_type = CT_APPLICATION_X_JAVASCRIPT;
        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_begin(r, wb, format, options, false, group_method);

        rrdr2json(r, wb, options, 0);

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_end(r, wb, format, options, false);
        break;

    case DATASOURCE_JSON:
    default:
        wb->content_type = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_begin(r, wb, format, options, false, group_method);

        rrdr2json(r, wb, options, 0);

        if(options & RRDR_OPTION_JSON_WRAP) {
            if(qt->request.group_by_aggregate_function == RRDR_GROUP_BY_FUNCTION_SUM_COUNT) {
                rrdr_json_wrapper_group_by_count(r, wb, format, options, 0);
                rrdr2json(r, wb, options | RRDR_OPTION_INTERNAL_GBC, 0);
            }
            if(options & RRDR_OPTION_RETURN_JWAR) {
                rrdr_json_wrapper_anomaly_rates(r, wb, format, options, 0);
                rrdr2json(r, wb, options | RRDR_OPTION_INTERNAL_AR, 0);
            }
            wrapper_end(r, wb, format, options, false);
        }
        break;
    }

    if(r != r1)
        rrdr_free(owa, r);

    rrdr_free(owa, r1);
    return HTTP_RESP_OK;
}
