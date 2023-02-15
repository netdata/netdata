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
    RRDR_OPTION_NONZERO         = 0x00000001, // don't output dimensions with just zero values
    RRDR_OPTION_REVERSED        = 0x00000002, // output the rows in reverse order (oldest to newest)
    RRDR_OPTION_ABSOLUTE        = 0x00000004, // values positive, for DATASOURCE_SSV before summing
    RRDR_OPTION_MIN2MAX         = 0x00000008, // when adding dimensions, use max - min, instead of sum
    RRDR_OPTION_SECONDS         = 0x00000010, // output seconds, instead of dates
    RRDR_OPTION_MILLISECONDS    = 0x00000020, // output milliseconds, instead of dates
    RRDR_OPTION_NULL2ZERO       = 0x00000040, // do not show nulls, convert them to zeros
    RRDR_OPTION_OBJECTSROWS     = 0x00000080, // each row of values should be an object, not an array
    RRDR_OPTION_GOOGLE_JSON     = 0x00000100, // comply with google JSON/JSONP specs
    RRDR_OPTION_JSON_WRAP       = 0x00000200, // wrap the response in a JSON header with info about the result
    RRDR_OPTION_LABEL_QUOTES    = 0x00000400, // in CSV output, wrap header labels in double quotes
    RRDR_OPTION_PERCENTAGE      = 0x00000800, // give values as percentage of total
    RRDR_OPTION_NOT_ALIGNED     = 0x00001000, // do not align charts for persistent timeframes
    RRDR_OPTION_DISPLAY_ABS     = 0x00002000, // for badges, display the absolute value, but calculate colors with sign
    RRDR_OPTION_MATCH_IDS       = 0x00004000, // when filtering dimensions, match only IDs
    RRDR_OPTION_MATCH_NAMES     = 0x00008000, // when filtering dimensions, match only names
    RRDR_OPTION_NATURAL_POINTS  = 0x00020000, // return the natural points of the database
    RRDR_OPTION_VIRTUAL_POINTS  = 0x00040000, // return virtual points
    RRDR_OPTION_ANOMALY_BIT     = 0x00080000, // Return the anomaly bit stored in each collected_number
    RRDR_OPTION_RETURN_RAW      = 0x00100000, // Return raw data for aggregating across multiple nodes
    RRDR_OPTION_RETURN_JWAR     = 0x00200000, // Return anomaly rates in jsonwrap
    RRDR_OPTION_SELECTED_TIER   = 0x00400000, // Use the selected tier for the query
    RRDR_OPTION_ALL_DIMENSIONS  = 0x00800000, // Return the full dimensions list
    RRDR_OPTION_SHOW_PLAN       = 0x01000000, // Return the query plan in jsonwrap

    // internal ones - not to be exposed to the API
    RRDR_OPTION_INTERNAL_AR     = 0x10000000, // internal use only, to let the formatters we want to render the anomaly rate
    RRDR_OPTION_HEALTH_RSRVD1   = 0x80000000, // reserved for RRDCALC_OPTION_NO_CLEAR_NOTIFICATION
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
    RRDR_DIMENSION_QUERIED = 0x10, // the dimension is selected for evaluation in this RRDR
} RRDR_DIMENSION_FLAGS;

// RRDR result options
typedef enum rrdr_result_flags {
    RRDR_RESULT_OPTION_ABSOLUTE      = 0x00000001, // the query uses absolute time-frames
                                                   // (can be cached by browsers and proxies)
    RRDR_RESULT_OPTION_RELATIVE      = 0x00000002, // the query uses relative time-frames
                                                   // (should not to be cached by browsers and proxies)
    RRDR_RESULT_OPTION_VARIABLE_STEP = 0x00000004, // the query uses variable-step time-frames
    RRDR_RESULT_OPTION_CANCEL        = 0x00000008, // the query needs to be cancelled
} RRDR_RESULT_OPTIONS;

typedef struct rrdresult {
    RRDR_RESULT_OPTIONS result_options; // RRDR_RESULT_OPTION_*

    size_t d;                 // the number of dimensions
    size_t n;                 // the number of values in the arrays
    size_t rows;              // the number of rows used

    RRDR_DIMENSION_FLAGS *od; // the options for the dimensions

    time_t *t;                // array of n timestamps
    NETDATA_DOUBLE *v;        // array n x d values
    RRDR_VALUE_FLAGS *o;      // array n x d options for each value returned
    NETDATA_DOUBLE *ar;       // array n x d of anomaly rates (0 - 100)

    size_t group;             // how many collected values were grouped for each row
    time_t update_every;      // what is the suggested update frequency in seconds

    NETDATA_DOUBLE min;
    NETDATA_DOUBLE max;

    time_t before;
    time_t after;

    // internal rrd2rrdr() members below this point
    struct {
        ONEWAYALLOC *owa;                   // the allocator used
        struct query_target *qt;            // the QUERY_TARGET

        RRDR_OPTIONS query_options;         // RRDR_OPTION_* (as run by the query)

        size_t points_wanted;               // used by SES and DES
        size_t resampling_group;            // used by AVERAGE
        NETDATA_DOUBLE resampling_divisor;  // used by AVERAGE

        // grouping function pointers
        void (*grouping_create)(struct rrdresult *r, const char *options);
        void (*grouping_reset)(struct rrdresult *r);
        void (*grouping_free)(struct rrdresult *r);
        void (*grouping_add)(struct rrdresult *r, NETDATA_DOUBLE value);
        NETDATA_DOUBLE (*grouping_flush)(struct rrdresult *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

        TIER_QUERY_FETCH tier_query_fetch;  // which value to use from STORAGE_POINT
        void *grouping_data;                // the internal data of the grouping function

#ifdef NETDATA_INTERNAL_CHECKS
        const char *log;
#endif

        // statistics
        size_t db_points_read;
        size_t result_points_generated;
        size_t tier_points_read[RRD_STORAGE_TIERS];
    } internal;
} RRDR;

#define rrdr_rows(r) ((r)->rows)

#include "database/rrd.h"
void rrdr_free(ONEWAYALLOC *owa, RRDR *r);
RRDR *rrdr_create(ONEWAYALLOC *owa, struct query_target *qt);

#include "../web_api_v1.h"
#include "web/api/queries/query.h"

RRDR *rrd2rrdr_legacy(
        ONEWAYALLOC *owa,
        RRDSET *st, size_t points, time_t after, time_t before,
        RRDR_TIME_GROUPING group_method, time_t resampling_time, RRDR_OPTIONS options, const char *dimensions,
        const char *group_options, time_t timeout, size_t tier, QUERY_SOURCE query_source,
        STORAGE_PRIORITY priority);

RRDR *rrd2rrdr(ONEWAYALLOC *owa, struct query_target *qt);
bool query_target_calculate_window(struct query_target *qt);

bool rrdr_relative_window_to_absolute(time_t *after, time_t *before);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_QUERIES_RRDR_H
