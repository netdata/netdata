// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD2JSON_H
#define NETDATA_RRD2JSON_H 1

#include "web_api_v1.h"
#include "exporters/allmetrics.h"
#include "queries/rrdr.h"

#define HOSTNAME_MAX 1024

#define API_RELATIVE_TIME_MAX (3 * 365 * 86400)

// type of JSON generations
#define DATASOURCE_INVALID (-1)
#define DATASOURCE_JSON 0
#define DATASOURCE_DATATABLE_JSON 1
#define DATASOURCE_DATATABLE_JSONP 2
#define DATASOURCE_SSV 3
#define DATASOURCE_CSV 4
#define DATASOURCE_JSONP 5
#define DATASOURCE_TSV 6
#define DATASOURCE_HTML 7
#define DATASOURCE_JS_ARRAY 8
#define DATASOURCE_SSV_COMMA 9
#define DATASOURCE_CSV_JSON_ARRAY 10

#define DATASOURCE_FORMAT_JSON "json"
#define DATASOURCE_FORMAT_DATATABLE_JSON "datatable"
#define DATASOURCE_FORMAT_DATATABLE_JSONP "datasource"
#define DATASOURCE_FORMAT_JSONP "jsonp"
#define DATASOURCE_FORMAT_SSV "ssv"
#define DATASOURCE_FORMAT_CSV "csv"
#define DATASOURCE_FORMAT_TSV "tsv"
#define DATASOURCE_FORMAT_HTML "html"
#define DATASOURCE_FORMAT_JS_ARRAY "array"
#define DATASOURCE_FORMAT_SSV_COMMA "ssvcomma"
#define DATASOURCE_FORMAT_CSV_JSON_ARRAY "csvjsonarray"

extern void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb);
extern void rrd_stats_api_v1_charts(RRDHOST *host, BUFFER *wb);

extern int rrdset2anything_api_v1(
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
);

extern int rrdset2value_api_v1(
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
);

#endif /* NETDATA_RRD2JSON_H */
