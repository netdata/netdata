// SPDX-License-Identifier: GPL-3.0-or-later

#include "web/api/web_api_v1.h"
#include "database/storage-engine.h"

void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb)
{
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    rrdset2json(st, wb, NULL, NULL);
    buffer_json_finalize(wb);
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
        for(size_t t = 0; t < nd_profile.storage_tiers;t++)
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

    if(qt->request.version == 2) {
        wrapper_begin = rrdr_json_wrapper_begin2;
        wrapper_end = rrdr_json_wrapper_end2;
    }

    stream_control_user_data_query_started();
    RRDR *r = rrd2rrdr(owa, qt);
    stream_control_user_data_query_finished();

    if(!r) {
        buffer_strcat(wb, "Cannot generate output with these parameters on this chart.");
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    if (r->view.flags & RRDR_RESULT_FLAG_CANCEL) {
        rrdr_free(owa, r);
        return HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(r->view.flags & RRDR_RESULT_FLAG_RELATIVE)
        buffer_no_cacheable(wb);
    else if(r->view.flags & RRDR_RESULT_FLAG_ABSOLUTE)
        buffer_cacheable(wb);

    if(latest_timestamp && rrdr_rows(r) > 0)
        *latest_timestamp = r->view.before;

    DATASOURCE_FORMAT format = qt->request.format;
    RRDR_OPTIONS options = qt->window.options;

    switch(format) {
    case DATASOURCE_SSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2ssv(r, wb, options, "", " ", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", " ", "");
        }
        break;

    case DATASOURCE_SSV_COMMA:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2ssv(r, wb, options, "", ",", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", ",", "");
        }
        break;

    case DATASOURCE_JS_ARRAY:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb);
            buffer_json_member_add_array(wb, "result");
            rrdr2ssv(r, wb, options, "", ",", "");
            buffer_json_array_close(wb);
            wrapper_end(r, wb);
        }
        else {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr2ssv(r, wb, options, "[", ",", "]");
        }
        break;

    case DATASOURCE_CSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2csv(r, wb, format, options, "", ",", "\\n", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", ",", "\r\n", "");
        }
        break;

    case DATASOURCE_CSV_MARKDOWN:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2csv(r, wb, format, options, "", "|", "\\n", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "|", "\r\n", "");
        }
        break;

    case DATASOURCE_CSV_JSON_ARRAY:
        wb->content_type = CT_APPLICATION_JSON;
        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb);
            buffer_json_member_add_array(wb, "result");
            rrdr2csv(r, wb, format, options + RRDR_OPTION_LABEL_QUOTES, "[", ",", "]", ",\n");
            buffer_json_array_close(wb);
            wrapper_end(r, wb);
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
            wrapper_begin(r, wb);
            buffer_json_member_add_string_open(wb, "result");
            rrdr2csv(r, wb, format, options, "", "\t", "\\n", "");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "\t", "\r\n", "");
        }
        break;

    case DATASOURCE_HTML:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            wrapper_begin(r, wb);
            buffer_json_member_add_string_open(wb, "result");
            buffer_strcat(wb, "<html>\\n<center>\\n<table border=\\\"0\\\" cellpadding=\\\"5\\\" cellspacing=\\\"5\\\">\\n");
            rrdr2csv(r, wb, format, options, "<tr><td>", "</td><td>", "</td></tr>\\n", "");
            buffer_strcat(wb, "</table>\\n</center>\\n</html>\\n");
            buffer_json_member_add_string_close(wb);
            wrapper_end(r, wb);
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
            wrapper_begin(r, wb);
            buffer_json_member_add_key_only(wb, "result");
        }

        rrdr2json(r, wb, options, 1);

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_end(r, wb);

        break;

    case DATASOURCE_DATATABLE_JSON:
        wb->content_type = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb);
            buffer_json_member_add_key_only(wb, "result");
        }

        rrdr2json(r, wb, options, 1);

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_end(r, wb);

        break;

    case DATASOURCE_JSONP:
        wb->content_type = CT_APPLICATION_X_JAVASCRIPT;
        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb);
            buffer_json_member_add_key_only(wb, "result");
        }

        rrdr2json(r, wb, options, 0);

        if(options & RRDR_OPTION_JSON_WRAP)
            wrapper_end(r, wb);

        break;

    case DATASOURCE_JSON:
    default:
        wb->content_type = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP) {
            wrapper_begin(r, wb);
            buffer_json_member_add_key_only(wb, "result");
        }

        rrdr2json(r, wb, options, 0);

        if(options & RRDR_OPTION_JSON_WRAP) {
            if (options & RRDR_OPTION_RETURN_JWAR) {
                buffer_json_member_add_key_only(wb, "anomaly_rates");
                rrdr2json(r, wb, options | RRDR_OPTION_INTERNAL_AR, false);
            }
            wrapper_end(r, wb);
        }
        break;

    case DATASOURCE_JSON2:
        wb->content_type = CT_APPLICATION_JSON;
        wrapper_begin(r, wb);
        rrdr2json_v2(r, wb);
        wrapper_end(r, wb);
        break;
    }

    rrdr_free(owa, r);
    return HTTP_RESP_OK;
}
