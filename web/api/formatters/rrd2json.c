// SPDX-License-Identifier: GPL-3.0-or-later

#include "web/api/web_api_v1.h"
#include "database/storage_engine.h"

void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb) {
    rrdset2json(st, wb, NULL, NULL, 0);
}

const char *rrdr_format_to_string(uint32_t format)  {
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
        *db_points_read += r->internal.db_points_read;

    if(db_points_per_tier) {
        for(size_t t = 0; t < storage_tiers ;t++)
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

    long i = (!(options & RRDR_OPTION_REVERSED))?(long)rrdr_rows(r) - 1:0;
    *n = rrdr2value(r, i, options, value_is_null, anomaly_rate);
    ret = HTTP_RESP_OK;

cleanup:
    rrdr_free(owa, r);
    onewayalloc_destroy(owa);
    return ret;
}

int data_query_execute(ONEWAYALLOC *owa, BUFFER *wb, QUERY_TARGET *qt, time_t *latest_timestamp) {

    RRDR *r = rrd2rrdr(owa, qt);
    if(!r) {
        buffer_strcat(wb, "Cannot generate output with these parameters on this chart.");
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    if (r->result_options & RRDR_RESULT_OPTION_CANCEL) {
        rrdr_free(owa, r);
        return HTTP_RESP_BACKEND_FETCH_FAILED;
    }

    if(r->result_options & RRDR_RESULT_OPTION_RELATIVE)
        buffer_no_cacheable(wb);
    else if(r->result_options & RRDR_RESULT_OPTION_ABSOLUTE)
        buffer_cacheable(wb);

    if(latest_timestamp && rrdr_rows(r) > 0)
        *latest_timestamp = r->before;

    DATASOURCE_FORMAT format = qt->request.format;
    RRDR_OPTIONS options = qt->request.options;
    RRDR_TIME_GROUPING group_method = qt->request.time_group_method;

    switch(format) {
    case DATASOURCE_SSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method);
            rrdr2ssv(r, wb, options, "", " ", "");
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", " ", "");
        }
        break;

    case DATASOURCE_SSV_COMMA:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method);
            rrdr2ssv(r, wb, options, "", ",", "");
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2ssv(r, wb, options, "", ",", "");
        }
        break;

    case DATASOURCE_JS_ARRAY:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method);
            rrdr2ssv(r, wb, options, "[", ",", "]");
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        }
        else {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr2ssv(r, wb, options, "[", ",", "]");
        }
        break;

    case DATASOURCE_CSV:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method);
            rrdr2csv(r, wb, format, options, "", ",", "\\n", "");
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", ",", "\r\n", "");
        }
        break;

    case DATASOURCE_CSV_MARKDOWN:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method);
            rrdr2csv(r, wb, format, options, "", "|", "\\n", "");
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "|", "\r\n", "");
        }
        break;

    case DATASOURCE_CSV_JSON_ARRAY:
        wb->content_type = CT_APPLICATION_JSON;
        if(options & RRDR_OPTION_JSON_WRAP) {
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method);
            buffer_strcat(wb, "[\n");
            rrdr2csv(r, wb, format, options + RRDR_OPTION_LABEL_QUOTES, "[", ",", "]", ",\n");
            buffer_strcat(wb, "\n]");
            rrdr_json_wrapper_end(r, wb, format, options, 0);
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
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method);
            rrdr2csv(r, wb, format, options, "", "\t", "\\n", "");
            rrdr_json_wrapper_end(r, wb, format, options, 1);
        }
        else {
            wb->content_type = CT_TEXT_PLAIN;
            rrdr2csv(r, wb, format, options, "", "\t", "\r\n", "");
        }
        break;

    case DATASOURCE_HTML:
        if(options & RRDR_OPTION_JSON_WRAP) {
            wb->content_type = CT_APPLICATION_JSON;
            rrdr_json_wrapper_begin(r, wb, format, options, 1, group_method);
            buffer_strcat(wb, "<html>\\n<center>\\n<table border=\\\"0\\\" cellpadding=\\\"5\\\" cellspacing=\\\"5\\\">\\n");
            rrdr2csv(r, wb, format, options, "<tr><td>", "</td><td>", "</td></tr>\\n", "");
            buffer_strcat(wb, "</table>\\n</center>\\n</html>\\n");
            rrdr_json_wrapper_end(r, wb, format, options, 1);
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
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method);

        rrdr2json(r, wb, options, 1);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;

    case DATASOURCE_DATATABLE_JSON:
        wb->content_type = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method);

        rrdr2json(r, wb, options, 1);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;

    case DATASOURCE_JSONP:
        wb->content_type = CT_APPLICATION_X_JAVASCRIPT;
        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method);

        rrdr2json(r, wb, options, 0);

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        break;

    case DATASOURCE_JSON:
    default:
        wb->content_type = CT_APPLICATION_JSON;

        if(options & RRDR_OPTION_JSON_WRAP)
            rrdr_json_wrapper_begin(r, wb, format, options, 0, group_method);

        rrdr2json(r, wb, options, 0);

        if(options & RRDR_OPTION_JSON_WRAP) {
            if(options & RRDR_OPTION_RETURN_JWAR) {
                rrdr_json_wrapper_anomaly_rates(r, wb, format, options, 0);
                rrdr2json(r, wb, options | RRDR_OPTION_INTERNAL_AR, 0);
            }
            rrdr_json_wrapper_end(r, wb, format, options, 0);
        }
        break;
    }

    rrdr_free(owa, r);
    return HTTP_RESP_OK;
}
