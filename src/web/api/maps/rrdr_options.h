// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDR_OPTIONS_H
#define NETDATA_RRDR_OPTIONS_H

#include "libnetdata/libnetdata.h"

typedef enum rrdr_options {
    RRDR_OPTION_NONZERO         = (1 << 0), // don't output dimensions with just zero values
    RRDR_OPTION_REVERSED        = (1 << 1), // output the rows in reverse order (oldest to newest)
    RRDR_OPTION_ABSOLUTE        = (1 << 2), // values positive, for DATASOURCE_SSV before summing
    RRDR_OPTION_DIMS_MIN2MAX    = (1 << 3), // when adding dimensions, use max - min, instead of sum
    RRDR_OPTION_DIMS_AVERAGE    = (1 << 4), // when adding dimensions, use average, instead of sum
    RRDR_OPTION_DIMS_MIN        = (1 << 5), // when adding dimensions, use minimum, instead of sum
    RRDR_OPTION_DIMS_MAX        = (1 << 6), // when adding dimensions, use maximum, instead of sum
    RRDR_OPTION_SECONDS         = (1 << 7), // output seconds, instead of dates
    RRDR_OPTION_MILLISECONDS    = (1 << 8), // output milliseconds, instead of dates
    RRDR_OPTION_NULL2ZERO       = (1 << 9), // do not show nulls, convert them to zeros
    RRDR_OPTION_OBJECTSROWS     = (1 << 10), // each row of values should be an object, not an array
    RRDR_OPTION_GOOGLE_JSON     = (1 << 11), // comply with google JSON/JSONP specs
    RRDR_OPTION_JSON_WRAP       = (1 << 12), // wrap the response in a JSON header with info about the result
    RRDR_OPTION_LABEL_QUOTES    = (1 << 13), // in CSV output, wrap header labels in double quotes
    RRDR_OPTION_PERCENTAGE      = (1 << 14), // give values as percentage of total
    RRDR_OPTION_NOT_ALIGNED     = (1 << 15), // do not align charts for persistent timeframes
    RRDR_OPTION_DISPLAY_ABS     = (1 << 16), // for badges, display the absolute value, but calculate colors with sign
    RRDR_OPTION_MATCH_IDS       = (1 << 17), // when filtering dimensions, match only IDs
    RRDR_OPTION_MATCH_NAMES     = (1 << 18), // when filtering dimensions, match only names
    RRDR_OPTION_NATURAL_POINTS  = (1 << 19), // return the natural points of the database
    RRDR_OPTION_VIRTUAL_POINTS  = (1 << 20), // return virtual points
    RRDR_OPTION_ANOMALY_BIT     = (1 << 21), // Return the anomaly bit stored in each collected_number
    RRDR_OPTION_RETURN_RAW      = (1 << 22), // Return raw data for aggregating across multiple nodes
    RRDR_OPTION_RETURN_JWAR     = (1 << 23), // Return anomaly rates in jsonwrap
    RRDR_OPTION_SELECTED_TIER   = (1 << 24), // Use the selected tier for the query
    RRDR_OPTION_ALL_DIMENSIONS  = (1 << 25), // Return the full dimensions list
    RRDR_OPTION_SHOW_DETAILS    = (1 << 26), // v2 returns detailed object tree
    RRDR_OPTION_DEBUG           = (1 << 27), // v2 returns request description
    RRDR_OPTION_MINIFY          = (1 << 28), // remove JSON spaces and newlines from JSON output
    RRDR_OPTION_GROUP_BY_LABELS = (1 << 29), // v2 returns flattened labels per dimension of the chart

    // internal ones - not to be exposed to the API
    RRDR_OPTION_INTERNAL_AR              = (1 << 31), // internal use only, to let the formatters know we want to render the anomaly rate
} RRDR_OPTIONS;

void rrdr_options_to_buffer(BUFFER *wb, RRDR_OPTIONS options);
void rrdr_options_to_buffer_json_array(BUFFER *wb, const char *key, RRDR_OPTIONS options);
void web_client_api_request_data_vX_options_to_string(char *buf, size_t size, RRDR_OPTIONS options);
void rrdr_options_init(void);

RRDR_OPTIONS rrdr_options_parse(char *o);
RRDR_OPTIONS rrdr_options_parse_one(const char *o);

#endif //NETDATA_RRDR_OPTIONS_H
