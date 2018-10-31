// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_QUERIES_RRDR_H
#define NETDATA_QUERIES_RRDR_H

#include "libnetdata/libnetdata.h"

typedef enum rrdr_options {
    RRDR_OPTION_NONZERO      = 0x00000001, // don't output dimensions will just zero values
    RRDR_OPTION_REVERSED     = 0x00000002, // output the rows in reverse order (oldest to newest)
    RRDR_OPTION_ABSOLUTE     = 0x00000004, // values positive, for DATASOURCE_SSV before summing
    RRDR_OPTION_MIN2MAX      = 0x00000008, // when adding dimensions, use max - min, instead of sum
    RRDR_OPTION_SECONDS      = 0x00000010, // output seconds, instead of dates
    RRDR_OPTION_MILLISECONDS = 0x00000020, // output milliseconds, instead of dates
    RRDR_OPTION_NULL2ZERO    = 0x00000040, // do not show nulls, convert them to zeros
    RRDR_OPTION_OBJECTSROWS  = 0x00000080, // each row of values should be an object, not an array
    RRDR_OPTION_GOOGLE_JSON  = 0x00000100, // comply with google JSON/JSONP specs
    RRDR_OPTION_JSON_WRAP    = 0x00000200, // wrap the response in a JSON header with info about the result
    RRDR_OPTION_LABEL_QUOTES = 0x00000400, // in CSV output, wrap header labels in double quotes
    RRDR_OPTION_PERCENTAGE   = 0x00000800, // give values as percentage of total
    RRDR_OPTION_NOT_ALIGNED  = 0x00001000, // do not align charts for persistant timeframes
    RRDR_OPTION_DISPLAY_ABS  = 0x00002000, // for badges, display the absolute value, but calculate colors with sign
    RRDR_OPTION_MATCH_IDS    = 0x00004000, // when filtering dimensions, match only IDs
    RRDR_OPTION_MATCH_NAMES  = 0x00008000, // when filtering dimensions, match only names
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
    RRDR_RESULT_OPTION_ABSOLUTE = 0x00000001, // the query uses absolute time-frames (can be cached by browsers and proxies)
    RRDR_RESULT_OPTION_RELATIVE = 0x00000002, // the query uses relative time-frames (should not to be cached by browsers and proxies)
} RRDR_RESULT_FLAGS;

typedef struct rrdresult {
    struct rrdset *st;         // the chart this result refers to

    RRDR_RESULT_FLAGS result_options; // RRDR_RESULT_OPTION_*

    int d;                    // the number of dimensions
    long n;                   // the number of values in the arrays
    long rows;                // the number of rows used

    RRDR_DIMENSION_FLAGS *od; // the options for the dimensions

    time_t *t;                // array of n timestamps
    calculated_number *v;     // array n x d values
    RRDR_VALUE_FLAGS *o;      // array n x d options for each value returned

    long group;               // how many collected values were grouped for each row
    int update_every;         // what is the suggested update frequency in seconds

    calculated_number min;
    calculated_number max;

    time_t before;
    time_t after;

    int has_st_lock;        // if st is read locked by us

    // internal rrd2rrdr() members below this point
    struct {
        long points_wanted;
        long resampling_group;
        calculated_number resampling_divisor;

        void *(*grouping_create)(struct rrdresult *r);
        void (*grouping_reset)(struct rrdresult *r);
        void (*grouping_free)(struct rrdresult *r);
        void (*grouping_add)(struct rrdresult *r, calculated_number value);
        calculated_number (*grouping_flush)(struct rrdresult *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);
        void *grouping_data;

        #ifdef NETDATA_INTERNAL_CHECKS
        const char *log;
        #endif

        size_t db_points_read;
        size_t result_points_generated;
    } internal;
} RRDR;

#define rrdr_rows(r) ((r)->rows)

extern void rrdr_free(RRDR *r);
extern RRDR *rrdr_create(struct rrdset *st, long n);

#include "../web_api_v1.h"
#include "web/api/queries/query.h"

extern RRDR *rrd2rrdr(RRDSET *st, long points_requested, long long after_requested, long long before_requested, RRDR_GROUPING group_method, long resampling_time_requested, RRDR_OPTIONS options, const char *dimensions);

#include "query.h"

#endif //NETDATA_QUERIES_RRDR_H
