// SPDX-License-Identifier: GPL-3.0-or-later

#include "web/api/web_api_v1.h"
#include "database/storage_engine.h"

static inline void free_single_rrdrim(ONEWAYALLOC *owa, RRDDIM *temp_rd, int archive_mode)
{
    if (unlikely(!temp_rd))
        return;

    string_freez(temp_rd->id);
    string_freez(temp_rd->name);

    if (unlikely(archive_mode)) {
        temp_rd->rrdset->counter--;
        if (!temp_rd->rrdset->counter) {
            string_freez(temp_rd->rrdset->id);
            string_freez(temp_rd->rrdset->name);
            string_freez(temp_rd->rrdset->context);
            onewayalloc_freez(owa, temp_rd->rrdset);
        }
    }

    for(int tier = 0; tier < storage_tiers ;tier++) {
        if(!temp_rd->tiers[tier]) continue;

        if(archive_mode) {
            STORAGE_ENGINE *eng = storage_engine_get(temp_rd->tiers[tier]->mode);
            if (eng)
                eng->api.free(temp_rd->tiers[tier]->db_metric_handle);
        }

        onewayalloc_freez(owa, temp_rd->tiers[tier]);
    }

    onewayalloc_freez(owa, temp_rd);
}

static inline void free_rrddim_list(ONEWAYALLOC *owa, RRDDIM *temp_rd, int archive_mode)
{
    if (unlikely(!temp_rd))
        return;

    RRDDIM *t;
    while (temp_rd) {
        t = temp_rd->next;
        free_single_rrdrim(owa, temp_rd, archive_mode);
        temp_rd = t;
    }
}

void free_context_param_list(ONEWAYALLOC *owa, struct context_param **param_list)
{
    if (unlikely(!param_list || !*param_list))
        return;

    free_rrddim_list(owa, ((*param_list)->rd), (*param_list)->flags & CONTEXT_FLAGS_ARCHIVE);
    onewayalloc_freez(owa, (*param_list));
    *param_list = NULL;
}

void rebuild_context_param_list(ONEWAYALLOC *owa, struct context_param *context_param_list, time_t after_requested)
{
    RRDDIM *temp_rd = context_param_list->rd;
    RRDDIM *new_rd_list = NULL, *t;
    int is_archived = (context_param_list->flags & CONTEXT_FLAGS_ARCHIVE);

    RRDSET *st = temp_rd->rrdset;
    RRDSET *last_st = st;
    time_t last_entry_t = is_archived ? st->last_entry_t : rrdset_last_entry_t(st);
    time_t last_last_entry_t = last_entry_t;
    while (temp_rd) {
        t = temp_rd->next;

        st = temp_rd->rrdset;
        if (st == last_st) {
            last_entry_t = last_last_entry_t;
        }else {
            last_entry_t = is_archived ? st->last_entry_t : rrdset_last_entry_t(st);
            last_last_entry_t = last_entry_t;
            last_st = st;
        }

        if (last_entry_t >= after_requested) {
            temp_rd->next = new_rd_list;
            new_rd_list = temp_rd;
        } else
            free_single_rrdrim(owa, temp_rd, is_archived);
        temp_rd = t;
    }
    context_param_list->rd = new_rd_list;
};

void build_context_param_list(ONEWAYALLOC *owa, struct context_param **param_list, RRDSET *st)
{
    if (unlikely(!param_list || !st))
        return;

    if (unlikely(!(*param_list))) {
        *param_list = onewayalloc_mallocz(owa, sizeof(struct context_param));
        (*param_list)->first_entry_t = LONG_MAX;
        (*param_list)->last_entry_t = 0;
        (*param_list)->flags = CONTEXT_FLAGS_CONTEXT;
        (*param_list)->rd = NULL;
    }

    RRDDIM *rd1;
    st->last_accessed_time = now_realtime_sec();
    rrdset_rdlock(st);

    (*param_list)->first_entry_t = MIN((*param_list)->first_entry_t, rrdset_first_entry_t_nolock(st));
    (*param_list)->last_entry_t  = MAX((*param_list)->last_entry_t, rrdset_last_entry_t_nolock(st));

    rrddim_foreach_read(rd1, st) {
        RRDDIM *rd = onewayalloc_memdupz(owa, rd1, sizeof(RRDDIM));
        rd->id = string_dup(rd1->id);
        rd->name = string_dup(rd1->name);
        for(int tier = 0; tier < storage_tiers ;tier++) {
            if(rd1->tiers[tier])
                rd->tiers[tier] = onewayalloc_memdupz(owa, rd1->tiers[tier], sizeof(*rd->tiers[tier]));
            else
                rd->tiers[tier] = NULL;
        }
        rd->next = (*param_list)->rd;
        (*param_list)->rd = rd;
    }

    rrdset_unlock(st);
}

void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb) {
    rrdset2json(st, wb, NULL, NULL, 0);
}

void rrdr_buffer_print_format(BUFFER *wb, uint32_t format)  {
    switch(format) {
        case DATASOURCE_JSON:
            buffer_strcat(wb, DATASOURCE_FORMAT_JSON);
            break;

        case DATASOURCE_DATATABLE_JSON:
            buffer_strcat(wb, DATASOURCE_FORMAT_DATATABLE_JSON);
            break;

        case DATASOURCE_DATATABLE_JSONP:
            buffer_strcat(wb, DATASOURCE_FORMAT_DATATABLE_JSONP);
            break;

        case DATASOURCE_JSONP:
            buffer_strcat(wb, DATASOURCE_FORMAT_JSONP);
            break;

        case DATASOURCE_SSV:
            buffer_strcat(wb, DATASOURCE_FORMAT_SSV);
            break;

        case DATASOURCE_CSV:
            buffer_strcat(wb, DATASOURCE_FORMAT_CSV);
            break;

        case DATASOURCE_TSV:
            buffer_strcat(wb, DATASOURCE_FORMAT_TSV);
            break;

        case DATASOURCE_HTML:
            buffer_strcat(wb, DATASOURCE_FORMAT_HTML);
            break;

        case DATASOURCE_JS_ARRAY:
            buffer_strcat(wb, DATASOURCE_FORMAT_JS_ARRAY);
            break;

        case DATASOURCE_SSV_COMMA:
            buffer_strcat(wb, DATASOURCE_FORMAT_SSV_COMMA);
            break;

        default:
            buffer_strcat(wb, "unknown");
            break;
    }
}

int rrdset2value_api_v1(
          RRDSET *st
        , BUFFER *wb
        , NETDATA_DOUBLE *n
        , const char *dimensions
        , long points
        , long long after
        , long long before
        , int group_method
        , const char *group_options
        , long group_time
        , uint32_t options
        , time_t *db_after
        , time_t *db_before
        , size_t *db_points_read
        , size_t *db_points_per_tier
        , size_t *result_points_generated
        , int *value_is_null
        , NETDATA_DOUBLE *anomaly_rate
        , int timeout
        , int tier
) {
    int ret = HTTP_RESP_INTERNAL_SERVER_ERROR;

    ONEWAYALLOC *owa = onewayalloc_create(0);

    RRDR *r = rrd2rrdr(owa, st, points, after, before,
                       group_method, group_time, options, dimensions, NULL,
                       group_options, timeout, tier);

    if(!r) {
        if(value_is_null) *value_is_null = 1;
        ret = HTTP_RESP_INTERNAL_SERVER_ERROR;
        goto cleanup;
    }

    if(db_points_read)
        *db_points_read += r->internal.db_points_read;

    if(db_points_per_tier) {
        for(int t = 0; t < storage_tiers ;t++)
            db_points_per_tier[t] += r->internal.tier_points_read[t];
    }

    if(result_points_generated)
        *result_points_generated += r->internal.result_points_generated;

    if(rrdr_rows(r) == 0) {
        if(db_after)  *db_after  = 0;
        if(db_before) *db_before = 0;
        if(value_is_null) *value_is_null = 1;

        ret = HTTP_RESP_BAD_REQUEST;
        goto cleanup;
    }

    if(wb) {
        if (r->result_options & RRDR_RESULT_OPTION_RELATIVE)
            buffer_no_cacheable(wb);
        else if (r->result_options & RRDR_RESULT_OPTION_ABSOLUTE)
            buffer_cacheable(wb);
    }

    if(db_after)  *db_after  = r->after;
    if(db_before) *db_before = r->before;

    long i = (!(options & RRDR_OPTION_REVERSED))?rrdr_rows(r) - 1:0;
    *n = rrdr2value(r, i, options, value_is_null, anomaly_rate, NULL);
    ret = HTTP_RESP_OK;

cleanup:
    if(r) rrdr_free(owa, r);
    onewayalloc_destroy(owa);
    return ret;
}

int rrdset2anything_api_v1(
          ONEWAYALLOC *owa
        , RRDSET *st
        , QUERY_PARAMS *query_params
        , BUFFER *dimensions
        , uint32_t format
        , long points
        , long long after
        , long long before
        , int group_method
        , const char *group_options
        , long group_time
        , uint32_t options
        , time_t *latest_timestamp
        , int tier
)
{
    BUFFER *wb = query_params->wb;
    if (query_params->context_param_list && !(query_params->context_param_list->flags & CONTEXT_FLAGS_ARCHIVE))
        st->last_accessed_time = now_realtime_sec();

    RRDR *r = rrd2rrdr(
        owa,
        st,
        points,
        after,
        before,
        group_method,
        group_time,
        options,
        dimensions ? buffer_tostring(dimensions) : NULL,
        query_params->context_param_list,
        group_options,
        query_params->timeout, tier);
    if(!r) {
        buffer_strcat(wb, "Cannot generate output with these parameters on this chart.");
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    if (r->result_options & RRDR_RESULT_OPTION_CANCEL) {
        rrdr_free(owa, r);
        return HTTP_RESP_BACKEND_FETCH_FAILED;
    }

    if (st->state && st->state->is_ar_chart)
        ml_process_rrdr(r, query_params->max_anomaly_rates);

    RRDDIM *temp_rd = query_params->context_param_list ? query_params->context_param_list->rd : NULL;

    if(r->result_options & RRDR_RESULT_OPTION_RELATIVE)
        buffer_no_cacheable(wb);
    else if(r->result_options & RRDR_RESULT_OPTION_ABSOLUTE)
        buffer_cacheable(wb);

    if(latest_timestamp && rrdr_rows(r) > 0)
        *latest_timestamp = r->before;

    switch(format) {
    case DATASOURCE_SSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method, query_params);
            rrdr2ssv(r, wb, options, "", " ", "", temp_rd);
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->contenttype = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", " ", "", temp_rd);
        }
        break;

    case DATASOURCE_SSV_COMMA:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method, query_params);
            rrdr2ssv(r, wb, options, "", ",", "", temp_rd);
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->contenttype = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", ",", "", temp_rd);
        }
        break;

    case DATASOURCE_JS_ARRAY:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method, query_params);
            rrdr2ssv(r, wb, options, "[", ",", "]", temp_rd);
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        }
        else {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr2ssv(r, wb, options, "[", ",", "]", temp_rd);
        }
        break;

    case DATASOURCE_CSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method, query_params);
            rrdr2csv(r, wb, format, options, "", ",", "\\n", "", temp_rd);
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->contenttype = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", ",", "\r\n", "", temp_rd);
        }
        break;

    case DATASOURCE_CSV_MARKDOWN:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method, query_params);
            rrdr2csv(r, wb, format, options, "", "|", "\\n", "", temp_rd);
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->contenttype = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "|", "\r\n", "", temp_rd);
        }
        break;

    case DATASOURCE_CSV_JSON_ARRAY:
        wb->contenttype = CT_APPLICATION_JSON;
        if(options & RRDR_OPTION_JSON_WRAP) {
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method, query_params);
            buffer_strcat(wb, "[\n");
            rrdr2csv(r, wb, format, options + RRDR_OPTION_LABEL_QUOTES, "[", ",", "]", ",\n", temp_rd);
            buffer_strcat(wb, "\n]");
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        }
        else {
            wb->contenttype = CT_APPLICATION_JSON;
            buffer_strcat(wb, "[\n");
            rrdr2csv(r, wb, format, options + RRDR_OPTION_LABEL_QUOTES, "[", ",", "]", ",\n", temp_rd);
            buffer_strcat(wb, "\n]");
        }
        break;

    case DATASOURCE_TSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method, query_params);
            rrdr2csv(r, wb, format, options, "", "\t", "\\n", "", temp_rd);
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->contenttype = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "\t", "\r\n", "", temp_rd);
        }
        break;

    case DATASOURCE_HTML:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method, query_params);
            buffer_strcat(wb, "<html>\\n<center>\\n<table border=\\\"0\\\" cellpadding=\\\"5\\\" cellspacing=\\\"5\\\">\\n");
            rrdr2csv(r, wb, format, options, "<tr><td>", "</td><td>", "</td></tr>\\n", "", temp_rd);
            buffer_strcat(wb, "</table>\\n</center>\\n</html>\\n");
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->contenttype = CT_TEXT_HTML;
            buffer_strcat(wb, "<html>\n<center>\n<table border=\"0\" cellpadding=\"5\" cellspacing=\"5\">\n");
            rrdr2csv(r, wb, format, options, "<tr><td>", "</td><td>", "</td></tr>\n", "", temp_rd);
            buffer_strcat(wb, "</table>\n</center>\n</html>\n");
        }
        break;

    case DATASOURCE_DATATABLE_JSONP:
        wb->contenttype = CT_APPLICATION_X_JAVASCRIPT;

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method, query_params);

        rrdr2json(r, wb, options, 1, query_params->context_param_list);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;

    case DATASOURCE_DATATABLE_JSON:
        wb->contenttype = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method, query_params);

        rrdr2json(r, wb, options, 1, query_params->context_param_list);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;

    case DATASOURCE_JSONP:
        wb->contenttype = CT_APPLICATION_X_JAVASCRIPT;
        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method, query_params);

        rrdr2json(r, wb, options, 0, query_params->context_param_list);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;

    case DATASOURCE_JSON:
    default:
        wb->contenttype = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method, query_params);

        rrdr2json(r, wb, options, 0, query_params->context_param_list);

        if(options & RRDR_OPTION_JSON_WRAP) {
            if(options & RRDR_OPTION_RETURN_JWAR) {
                rrdr_json_wrapper_anomaly_rates(r, wb, format, options, 0);
                rrdr2json(r, wb, options | RRDR_OPTION_INTERNAL_AR, 0, query_params->context_param_list);
            }
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        }
        break;
    }

    rrdr_free(owa, r);
    return HTTP_RESP_OK;
}
