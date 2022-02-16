// SPDX-License-Identifier: GPL-3.0-or-later

#include "web/api/web_api_v1.h"

static inline void free_single_rrdrim(RRDDIM *temp_rd, int archive_mode)
{
    if (unlikely(!temp_rd))
        return;

    freez((char *)temp_rd->id);
    freez((char *)temp_rd->name);

    if (unlikely(archive_mode)) {
        temp_rd->rrdset->counter--;
        if (!temp_rd->rrdset->counter) {
            freez((char *)temp_rd->rrdset->name);
            freez(temp_rd->rrdset->context);
            freez(temp_rd->rrdset);
        }
    }
    freez(temp_rd->state);
    freez(temp_rd);
}

static inline void free_rrddim_list(RRDDIM *temp_rd, int archive_mode)
{
    if (unlikely(!temp_rd))
        return;

    RRDDIM *t;
    while (temp_rd) {
        t = temp_rd->next;
        free_single_rrdrim(temp_rd, archive_mode);
        temp_rd = t;
    }
}

void free_context_param_list(struct context_param **param_list)
{
    if (unlikely(!param_list || !*param_list))
        return;

    free_rrddim_list(((*param_list)->rd), (*param_list)->flags & CONTEXT_FLAGS_ARCHIVE);
    freez((*param_list));
    *param_list = NULL;
}

void rebuild_context_param_list(struct context_param *context_param_list, time_t after_requested)
{
    RRDDIM *temp_rd = context_param_list->rd;
    RRDDIM *new_rd_list = NULL, *t;
    int is_archived = (context_param_list->flags & CONTEXT_FLAGS_ARCHIVE);
    while (temp_rd) {
        t = temp_rd->next;
        RRDSET *st = temp_rd->rrdset;
        time_t last_entry_t = is_archived ? st->last_entry_t : rrdset_last_entry_t(st);

        if (last_entry_t >= after_requested) {
            temp_rd->next = new_rd_list;
            new_rd_list = temp_rd;
        } else
            free_single_rrdrim(temp_rd, is_archived);
        temp_rd = t;
    }
    context_param_list->rd = new_rd_list;
};

void build_context_param_list(struct context_param **param_list, RRDSET *st)
{
    if (unlikely(!param_list || !st))
        return;

    if (unlikely(!(*param_list))) {
        *param_list = mallocz(sizeof(struct context_param));
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
        RRDDIM *rd = mallocz(rd1->memsize);
        memcpy(rd, rd1, rd1->memsize);
        rd->id = strdupz(rd1->id);
        rd->name = strdupz(rd1->name);
        rd->state = mallocz(sizeof(*rd->state));
        memcpy(rd->state, rd1->state, sizeof(*rd->state));
        memcpy(&rd->state->collect_ops, &rd1->state->collect_ops, sizeof(struct rrddim_collect_ops));
        memcpy(&rd->state->query_ops, &rd1->state->query_ops, sizeof(struct rrddim_query_ops));
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
        , calculated_number *n
        , const char *dimensions
        , long points
        , long long after
        , long long before
        , int group_method
        , long group_time
        , uint32_t options
        , time_t *db_after
        , time_t *db_before
        , int *value_is_null
) {

    RRDR *r = rrd2rrdr(st, points, after, before, group_method, group_time, options, dimensions, NULL);

    if(!r) {
        if(value_is_null) *value_is_null = 1;
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    if(rrdr_rows(r) == 0) {
        rrdr_free(r);

        if(db_after)  *db_after  = 0;
        if(db_before) *db_before = 0;
        if(value_is_null) *value_is_null = 1;

        return HTTP_RESP_BAD_REQUEST;
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
    *n = rrdr2value(r, i, options, value_is_null);

    rrdr_free(r);
    return HTTP_RESP_OK;
}

int rrdset2anything_api_v1(
          RRDSET *st
        , BUFFER *wb
        , BUFFER *dimensions
        , uint32_t format
        , long points
        , long long after
        , long long before
        , int group_method
        , long group_time
        , uint32_t options
        , time_t *latest_timestamp
        , struct context_param *context_param_list
        , char *chart_label_key
) {

    if (context_param_list && !(context_param_list->flags & CONTEXT_FLAGS_ARCHIVE))
        st->last_accessed_time = now_realtime_sec();

    RRDR *r = rrd2rrdr(st, points, after, before, group_method, group_time, options, dimensions?buffer_tostring(dimensions):NULL, context_param_list);
    if(!r) {
        buffer_strcat(wb, "Cannot generate output with these parameters on this chart.");
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    RRDDIM *temp_rd = context_param_list ? context_param_list->rd : NULL;

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
            rrdr_json_wrapper_begin(r, wb, format, options, 1, context_param_list, chart_label_key);
            rrdr2ssv(r, wb, options, "", " ", "");
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->contenttype = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", " ", "");
        }
        break;

    case DATASOURCE_SSV_COMMA:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, context_param_list, chart_label_key);
            rrdr2ssv(r, wb, options, "", ",", "");
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->contenttype = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", ",", "");
        }
        break;

    case DATASOURCE_JS_ARRAY:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 0, context_param_list, chart_label_key);
            rrdr2ssv(r, wb, options, "[", ",", "]");
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        }
        else {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr2ssv(r, wb, options, "[", ",", "]");
        }
        break;

    case DATASOURCE_CSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->contenttype = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, context_param_list, chart_label_key);
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
            rrdr_json_wrapper_begin(r, wb, format, options, 1, context_param_list, chart_label_key);
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
            rrdr_json_wrapper_begin(r, wb, format, options, 0, context_param_list, chart_label_key);
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
            rrdr_json_wrapper_begin(r, wb, format, options, 1, context_param_list, chart_label_key);
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
            rrdr_json_wrapper_begin(r, wb, format, options, 1, context_param_list, chart_label_key);
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
            rrdr_json_wrapper_begin(r, wb, format, options, 0, context_param_list, chart_label_key);

        rrdr2json(r, wb, options, 1, context_param_list);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;

    case DATASOURCE_DATATABLE_JSON:
        wb->contenttype = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, context_param_list, chart_label_key);

        rrdr2json(r, wb, options, 1, context_param_list);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;

    case DATASOURCE_JSONP:
        wb->contenttype = CT_APPLICATION_X_JAVASCRIPT;
        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, context_param_list, chart_label_key);

        rrdr2json(r, wb, options, 0, context_param_list);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;

    case DATASOURCE_JSON:
    default:
        wb->contenttype = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, context_param_list, chart_label_key);

        rrdr2json(r, wb, options, 0, context_param_list);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;
    }

    rrdr_free(r);
    return HTTP_RESP_OK;
}
