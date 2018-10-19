// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_QUERIES_RRDR_H
#define NETDATA_QUERIES_RRDR_H

#include "../web_api_v1.h"

#define RRDR_OPTION_NONZERO         0x00000001 // don't output dimensions will just zero values
#define RRDR_OPTION_REVERSED        0x00000002 // output the rows in reverse order (oldest to newest)
#define RRDR_OPTION_ABSOLUTE        0x00000004 // values positive, for DATASOURCE_SSV before summing
#define RRDR_OPTION_MIN2MAX         0x00000008 // when adding dimensions, use max - min, instead of sum
#define RRDR_OPTION_SECONDS         0x00000010 // output seconds, instead of dates
#define RRDR_OPTION_MILLISECONDS    0x00000020 // output milliseconds, instead of dates
#define RRDR_OPTION_NULL2ZERO       0x00000040 // do not show nulls, convert them to zeros
#define RRDR_OPTION_OBJECTSROWS     0x00000080 // each row of values should be an object, not an array
#define RRDR_OPTION_GOOGLE_JSON     0x00000100 // comply with google JSON/JSONP specs
#define RRDR_OPTION_JSON_WRAP       0x00000200 // wrap the response in a JSON header with info about the result
#define RRDR_OPTION_LABEL_QUOTES    0x00000400 // in CSV output, wrap header labels in double quotes
#define RRDR_OPTION_PERCENTAGE      0x00000800 // give values as percentage of total
#define RRDR_OPTION_NOT_ALIGNED     0x00001000 // do not align charts for persistant timeframes
#define RRDR_OPTION_DISPLAY_ABS     0x00002000 // for badges, display the absolute value, but calculate colors with sign
#define RRDR_OPTION_MATCH_IDS       0x00004000 // when filtering dimensions, match only IDs
#define RRDR_OPTION_MATCH_NAMES     0x00008000 // when filtering dimensions, match only names

// RRDR dimension options
#define RRDR_EMPTY      0x01 // the dimension contains / the value is empty (null)
#define RRDR_RESET      0x02 // the dimension contains / the value is reset
#define RRDR_HIDDEN     0x04 // the dimension contains / the value is hidden
#define RRDR_NONZERO    0x08 // the dimension contains / the value is non-zero
#define RRDR_SELECTED   0x10 // the dimension is selected

// RRDR result options
#define RRDR_RESULT_OPTION_ABSOLUTE 0x00000001
#define RRDR_RESULT_OPTION_RELATIVE 0x00000002

typedef struct rrdresult {
    RRDSET *st;         // the chart this result refers to

    uint32_t result_options;    // RRDR_RESULT_OPTION_*

    int d;                  // the number of dimensions
    long n;                 // the number of values in the arrays
    long rows;              // the number of rows used

    uint8_t *od;            // the options for the dimensions

    time_t *t;              // array of n timestamps
    calculated_number *v;   // array n x d values
    uint8_t *o;             // array n x d options

    long group;             // how many collected values were grouped for each row
    int update_every;       // what is the suggested update frequency in seconds

    calculated_number min;
    calculated_number max;

    time_t before;
    time_t after;

    int has_st_lock;        // if st is read locked by us

    // internal rrd2rrdr() members below this point
    long group_points;
    calculated_number group_sum_divisor;
} RRDR;

#define rrdr_rows(r) ((r)->rows)

// formatters
extern void rrdr2json(RRDR *r, BUFFER *wb, uint32_t options, int datatable);
extern void rrdr2csv(RRDR *r, BUFFER *wb, uint32_t options, const char *startline, const char *separator, const char *endline, const char *betweenlines);
extern calculated_number rrdr2value(RRDR *r, long i, uint32_t options, int *all_values_are_null);
extern void rrdr2ssv(RRDR *r, BUFFER *wb, uint32_t options, const char *prefix, const char *separator, const char *suffix);

extern void rrdr_free(RRDR *r);
extern RRDR *rrdr_create(RRDSET *st, long n);

#include "query.h"

#endif //NETDATA_QUERIES_RRDR_H
