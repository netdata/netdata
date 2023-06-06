// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD2JSON_H
#define NETDATA_RRD2JSON_H 1

// type of JSON generations
typedef enum {
    DATASOURCE_JSON             = 0,
    DATASOURCE_DATATABLE_JSON   = 1,
    DATASOURCE_DATATABLE_JSONP  = 2,
    DATASOURCE_SSV              = 3,
    DATASOURCE_CSV              = 4,
    DATASOURCE_JSONP            = 5,
    DATASOURCE_TSV              = 6,
    DATASOURCE_HTML             = 7,
    DATASOURCE_JS_ARRAY         = 8,
    DATASOURCE_SSV_COMMA        = 9,
    DATASOURCE_CSV_JSON_ARRAY   = 10,
    DATASOURCE_CSV_MARKDOWN     = 11,
    DATASOURCE_JSON2            = 12,
} DATASOURCE_FORMAT;

#include "web/api/web_api_v1.h"

#include "web/api/exporters/allmetrics.h"
#include "web/api/queries/rrdr.h"

#include "web/api/formatters/csv/csv.h"
#include "web/api/formatters/ssv/ssv.h"
#include "web/api/formatters/json/json.h"
#include "web/api/formatters/value/value.h"

#include "web/api/formatters/rrdset2json.h"
#include "web/api/formatters/charts2json.h"
#include "web/api/formatters/json_wrapper.h"

#include "web/server/web_client.h"

#define HOSTNAME_MAX 1024

#define API_RELATIVE_TIME_MAX (3 * 365 * 86400)

#define DATASOURCE_FORMAT_JSON "json"
#define DATASOURCE_FORMAT_JSON2 "json2"
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
#define DATASOURCE_FORMAT_CSV_MARKDOWN "markdown"

void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb);
const char *rrdr_format_to_string(DATASOURCE_FORMAT format);

int data_query_execute(ONEWAYALLOC *owa, BUFFER *wb, struct query_target *qt, time_t *latest_timestamp);

void rrdr_json_group_by_labels(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options);

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
);

static inline bool rrdr_dimension_should_be_exposed(RRDR_DIMENSION_FLAGS rrdr_dim_flags, RRDR_OPTIONS options) {
    if(unlikely((options & RRDR_OPTION_RETURN_RAW) && (rrdr_dim_flags & RRDR_DIMENSION_QUERIED)))
        return true;

    if(unlikely(rrdr_dim_flags & RRDR_DIMENSION_HIDDEN)) return false;
    if(unlikely(!(rrdr_dim_flags & RRDR_DIMENSION_QUERIED))) return false;
    if(unlikely((options & RRDR_OPTION_NONZERO) && !(rrdr_dim_flags & RRDR_DIMENSION_NONZERO))) return false;

    return true;
}

#endif /* NETDATA_RRD2JSON_H */
