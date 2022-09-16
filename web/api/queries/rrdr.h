// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_QUERIES_RRDR_H
#define NETDATA_QUERIES_RRDR_H

#include "libnetdata/libnetdata.h"
#include "web/api/queries/query.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef enum tier_query_fetch {
    TIER_QUERY_FETCH_SUM,
    TIER_QUERY_FETCH_MIN,
    TIER_QUERY_FETCH_MAX,
    TIER_QUERY_FETCH_AVERAGE
} TIER_QUERY_FETCH;

typedef enum rrdr_options {
    RRDR_OPTION_NONZERO        = 0x00000001, // don't output dimensions with just zero values
    RRDR_OPTION_REVERSED       = 0x00000002, // output the rows in reverse order (oldest to newest)
    RRDR_OPTION_ABSOLUTE       = 0x00000004, // values positive, for DATASOURCE_SSV before summing
    RRDR_OPTION_MIN2MAX        = 0x00000008, // when adding dimensions, use max - min, instead of sum
    RRDR_OPTION_SECONDS        = 0x00000010, // output seconds, instead of dates
    RRDR_OPTION_MILLISECONDS   = 0x00000020, // output milliseconds, instead of dates
    RRDR_OPTION_NULL2ZERO      = 0x00000040, // do not show nulls, convert them to zeros
    RRDR_OPTION_OBJECTSROWS    = 0x00000080, // each row of values should be an object, not an array
    RRDR_OPTION_GOOGLE_JSON    = 0x00000100, // comply with google JSON/JSONP specs
    RRDR_OPTION_JSON_WRAP      = 0x00000200, // wrap the response in a JSON header with info about the result
    RRDR_OPTION_LABEL_QUOTES   = 0x00000400, // in CSV output, wrap header labels in double quotes
    RRDR_OPTION_PERCENTAGE     = 0x00000800, // give values as percentage of total
    RRDR_OPTION_NOT_ALIGNED    = 0x00001000, // do not align charts for persistent timeframes
    RRDR_OPTION_DISPLAY_ABS    = 0x00002000, // for badges, display the absolute value, but calculate colors with sign
    RRDR_OPTION_MATCH_IDS      = 0x00004000, // when filtering dimensions, match only IDs
    RRDR_OPTION_MATCH_NAMES    = 0x00008000, // when filtering dimensions, match only names
    RRDR_OPTION_CUSTOM_VARS    = 0x00010000, // when wrapping response in a JSON, return custom variables in response
    RRDR_OPTION_NATURAL_POINTS = 0x00020000, // return the natural points of the database
    RRDR_OPTION_VIRTUAL_POINTS = 0x00040000, // return virtual points
    RRDR_OPTION_ANOMALY_BIT    = 0x00080000, // Return the anomaly bit stored in each collected_number
    RRDR_OPTION_RETURN_RAW     = 0x00100000, // Return raw data for aggregating across multiple nodes
    RRDR_OPTION_RETURN_JWAR    = 0x00200000, // Return anomaly rates in jsonwrap
    RRDR_OPTION_SELECTED_TIER  = 0x00400000, // Use the selected tier for the query

    // internal ones - not to be exposed to the API
    RRDR_OPTION_INTERNAL_AR    = 0x10000000, // internal use only, to let the formatters we want to render the anomaly rate
} RRDR_OPTIONS;

typedef enum rrdr_value_flag {
    RRDR_VALUE_NOTHING      = 0x00, // no flag set (a good default)
    RRDR_VALUE_EMPTY        = 0x01, // the database value is empty
    RRDR_VALUE_RESET        = 0x02, // the database value is marked as reset (overflown)
} RRDR_VALUE_FLAGS;

typedef enum rrdr_dimension_flag {
    RRDR_DIMENSION_DEFAULT  = 0x00,
    RRDR_DIMENSION_HIDDEN   = 0x04, // the dimension is hidden (not to be presented to callers)
    RRDR_DIMENSION_NONZERO  = 0x08, // the dimension is non zero (contains non-zero values)
    RRDR_DIMENSION_SELECTED = 0x10, // the dimension is selected for evaluation in this RRDR
} RRDR_DIMENSION_FLAGS;

// RRDR result options
typedef enum rrdr_result_flags {
    RRDR_RESULT_OPTION_ABSOLUTE      = 0x00000001, // the query uses absolute time-frames
                                                   // (can be cached by browsers and proxies)
    RRDR_RESULT_OPTION_RELATIVE      = 0x00000002, // the query uses relative time-frames
                                                   // (should not to be cached by browsers and proxies)
    RRDR_RESULT_OPTION_VARIABLE_STEP = 0x00000004, // the query uses variable-step time-frames
    RRDR_RESULT_OPTION_CANCEL        = 0x00000008, // the query needs to be cancelled
} RRDR_RESULT_FLAGS;

typedef struct rrdresult {
    struct rrdset *st;         // the chart this result refers to

    RRDR_RESULT_FLAGS result_options; // RRDR_RESULT_OPTION_*

    int d;                    // the number of dimensions
    long n;                   // the number of values in the arrays
    long rows;                // the number of rows used

    RRDR_DIMENSION_FLAGS *od; // the options for the dimensions

    time_t *t;                // array of n timestamps
    NETDATA_DOUBLE *v;        // array n x d values
    RRDR_VALUE_FLAGS *o;      // array n x d options for each value returned
    NETDATA_DOUBLE *ar;       // array n x d of anomaly rates (0 - 100)

    long group;               // how many collected values were grouped for each row
    int update_every;         // what is the suggested update frequency in seconds

    NETDATA_DOUBLE min;
    NETDATA_DOUBLE max;

    time_t before;
    time_t after;

    bool st_locked_by_rrdr_create;        // if st is read locked by us

    // internal rrd2rrdr() members below this point
    struct {
        int query_tier;                         // the selected tier
        RRDR_OPTIONS query_options;       // RRDR_OPTION_* (as run by the query)

        long points_wanted;
        long resampling_group;
        NETDATA_DOUBLE resampling_divisor;

        void (*grouping_create)(struct rrdresult *r, const char *options);
        void (*grouping_reset)(struct rrdresult *r);
        void (*grouping_free)(struct rrdresult *r);
        void (*grouping_add)(struct rrdresult *r, NETDATA_DOUBLE value);
        NETDATA_DOUBLE (*grouping_flush)(struct rrdresult *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);
        void *grouping_data;

        TIER_QUERY_FETCH tier_query_fetch;
        #ifdef NETDATA_INTERNAL_CHECKS
        const char *log;
        #endif

        size_t db_points_read;
        size_t result_points_generated;
        size_t tier_points_read[RRD_STORAGE_TIERS];
        ONEWAYALLOC *owa;
    } internal;
} RRDR;

#define rrdr_rows(r) ((r)->rows)

#include "database/rrd.h"
extern void rrdr_free(ONEWAYALLOC *owa, RRDR *r);
extern RRDR *rrdr_create(ONEWAYALLOC *owa, struct rrdset *st, long n, struct context_param *context_param_list);
extern RRDR *rrdr_create_for_x_dimensions(ONEWAYALLOC *owa, int dimensions, long points);

#include "../web_api_v1.h"
#include "web/api/queries/query.h"

extern RRDR *rrd2rrdr(
    ONEWAYALLOC *owa,
    RRDSET *st, long points_wanted, long long after_wanted, long long before_wanted,
    RRDR_GROUPING group_method, long resampling_time_requested, RRDR_OPTIONS options, const char *dimensions,
    struct context_param *context_param_list, const char *group_options, int timeout, int tier);

extern int rrdr_relative_window_to_absolute(long long *after, long long *before);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_QUERIES_RRDR_H
