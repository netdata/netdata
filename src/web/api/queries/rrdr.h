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

typedef enum __attribute__ ((__packed__)) rrdr_value_flag {

    // IMPORTANT:
    // THIS IS AN AGREED BIT MAP BETWEEN AGENT, CLOUD FRONT-END AND CLOUD BACK-END
    // DO NOT CHANGE THE MAPPINGS !

    RRDR_VALUE_NOTHING      = 0,            // no flag set (a good default)
    RRDR_VALUE_EMPTY        = (1 << 0),     // the database value is empty
    RRDR_VALUE_RESET        = (1 << 1),     // the database value is marked as reset (overflown)
    RRDR_VALUE_PARTIAL      = (1 << 2),     // the database provides partial data about this point (used in group-by)
} RRDR_VALUE_FLAGS;

typedef enum __attribute__ ((__packed__)) rrdr_dimension_flag {
    RRDR_DIMENSION_DEFAULT  = 0,
    RRDR_DIMENSION_HIDDEN   = (1 << 0), // the dimension is hidden (not to be presented to callers)
    RRDR_DIMENSION_NONZERO  = (1 << 1), // the dimension is non zero (contains non-zero values)
    RRDR_DIMENSION_SELECTED = (1 << 2), // the dimension has been selected for query
    RRDR_DIMENSION_QUERIED  = (1 << 3), // the dimension has been queried
    RRDR_DIMENSION_FAILED   = (1 << 4), // the dimension failed to be queried
    RRDR_DIMENSION_GROUPED  = (1 << 5), // the dimension has been grouped in this RRDR
} RRDR_DIMENSION_FLAGS;

// RRDR result options
typedef enum __attribute__ ((__packed__)) rrdr_result_flags {
    RRDR_RESULT_FLAG_ABSOLUTE      = (1 << 0), // the query uses absolute time-frames
                                               // (can be cached by browsers and proxies)
    RRDR_RESULT_FLAG_RELATIVE      = (1 << 1), // the query uses relative time-frames
                                               // (should not to be cached by browsers and proxies)
    RRDR_RESULT_FLAG_CANCEL        = (1 << 2), // the query needs to be cancelled
} RRDR_RESULT_FLAGS;

#define RRDR_DVIEW_ANOMALY_COUNT_MULTIPLIER 1000.0

typedef struct rrdresult {
    size_t d;                 // the number of dimensions
    size_t n;                 // the number of values in the arrays (number of points per dimension)
    size_t rows;              // the number of actual rows used

    RRDR_DIMENSION_FLAGS *od; // the options for the dimensions

    STRING **di;              // array of d dimension ids
    STRING **dn;              // array of d dimension names
    STRING **du;              // array of d dimension units
    uint32_t *dgbs;           // array of d dimension group by slots - NOT ALLOCATED when RRDR is created
    uint32_t *dgbc;           // array of d dimension group by counts - NOT ALLOCATED when RRDR is created
    uint32_t *dp;             // array of d dimension priority - NOT ALLOCATED when RRDR is created
    DICTIONARY **dl;          // array of d dimension labels - NOT ALLOCATED when RRDR is created
    STORAGE_POINT *dqp;       // array of d dimensions query points - NOT ALLOCATED when RRDR is created
    STORAGE_POINT *dview;     // array of d dimensions group by view - NOT ALLOCATED when RRDR is created
    NETDATA_DOUBLE *vh;       // array of n x d hidden values, while grouping - NOT ALLOCATED when RRDR is created

    DICTIONARY *label_keys;

    time_t *t;                // array of n timestamps
    NETDATA_DOUBLE *v;        // array n x d values
    RRDR_VALUE_FLAGS *o;      // array n x d options for each value returned
    NETDATA_DOUBLE *ar;       // array n x d of anomaly rates (0 - 100)
    uint32_t *gbc;            // array n x d of group by count - NOT ALLOCATED when RRDR is created

    struct {
        size_t group;         // how many collected values were grouped for each row - NEEDED BY GROUPING FUNCTIONS
        time_t after;
        time_t before;
        time_t update_every;  // what is the suggested update frequency in seconds
        NETDATA_DOUBLE min;
        NETDATA_DOUBLE max;
        RRDR_RESULT_FLAGS flags; // RRDR_RESULT_FLAG_*
    } view;

    struct {
        size_t db_points_read;
        size_t result_points_generated;
    } stats;

    struct {
        void *data;                         // the internal data of the grouping function

        // grouping function pointers
        RRDR_TIME_GROUPING add_flush;
        void (*create)(struct rrdresult *r, const char *options);
        void (*reset)(struct rrdresult *r);
        void (*free)(struct rrdresult *r);
        void (*add)(struct rrdresult *r, NETDATA_DOUBLE value);
        NETDATA_DOUBLE (*flush)(struct rrdresult *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr);

        TIER_QUERY_FETCH tier_query_fetch;  // which value to use from STORAGE_POINT

        size_t points_wanted;               // used by SES and DES
        size_t resampling_group;            // used by AVERAGE
        NETDATA_DOUBLE resampling_divisor;  // used by AVERAGE
    } time_grouping;

    struct {
        struct rrdresult *r;
    } group_by;

    struct {
        time_t max_update_every;
        time_t expected_after;
        time_t trimmed_after;
    } partial_data_trimming;

    struct {
        ONEWAYALLOC *owa;           // the allocator used
        struct query_target *qt;    // the QUERY_TARGET
        size_t contexts;            // temp needed between json_wrapper_begin2() and json_wrapper_end2()
        size_t queries_count;       // temp needed to know if a query is the first executed

#ifdef NETDATA_INTERNAL_CHECKS
        const char *log;
#endif

        struct query_target *release_with_rrdr_qt;
    } internal;
} RRDR;

#define rrdr_rows(r) ((r)->rows)

#include "database/rrd.h"
void rrdr_free(ONEWAYALLOC *owa, RRDR *r);
RRDR *rrdr_create(ONEWAYALLOC *owa, struct query_target *qt, size_t dimensions, size_t points);

#include "../web_api_v1.h"
#include "web/api/queries/query.h"

RRDR *rrd2rrdr_legacy(
        ONEWAYALLOC *owa,
        RRDSET *st, size_t points, time_t after, time_t before,
        RRDR_TIME_GROUPING group_method, time_t resampling_time, RRDR_OPTIONS options, const char *dimensions,
        const char *group_options, time_t timeout_ms, size_t tier, QUERY_SOURCE query_source,
        STORAGE_PRIORITY priority);

RRDR *rrd2rrdr(ONEWAYALLOC *owa, struct query_target *qt);
bool query_target_calculate_window(struct query_target *qt);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_QUERIES_RRDR_H
