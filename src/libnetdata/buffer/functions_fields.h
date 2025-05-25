// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTIONS_FIELDS_H
#define NETDATA_FUNCTIONS_FIELDS_H 1

#include "../libnetdata.h"
#include "buffer.h"
#include "../template-enum.h"

typedef enum __attribute__((packed)) {
    RRDF_FIELD_OPTS_NONE         = 0,
    RRDF_FIELD_OPTS_UNIQUE_KEY   = (1 << 0), // the field is the unique key of the row
    RRDF_FIELD_OPTS_VISIBLE      = (1 << 1), // the field should be visible by default
    RRDF_FIELD_OPTS_STICKY       = (1 << 2), // the field should be sticky
    RRDF_FIELD_OPTS_FULL_WIDTH   = (1 << 3), // the field should get full width
    RRDF_FIELD_OPTS_WRAP         = (1 << 4), // the field should wrap
    RRDF_FIELD_OPTS_DUMMY        = (1 << 5), // not a presentable field
    RRDF_FIELD_OPTS_EXPANDED_FILTER = (1 << 6), // show the filter expanded
} RRDF_FIELD_OPTIONS;

typedef enum __attribute__((packed)) {
    RRDF_FIELD_TYPE_NONE,
    RRDF_FIELD_TYPE_INTEGER,
    RRDF_FIELD_TYPE_BOOLEAN,
    RRDF_FIELD_TYPE_STRING,
    RRDF_FIELD_TYPE_DETAIL_STRING,
    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
    RRDF_FIELD_TYPE_DURATION,
    RRDF_FIELD_TYPE_TIMESTAMP,
    RRDF_FIELD_TYPE_ARRAY,
} RRDF_FIELD_TYPE;

// Declare enum string mapping functions
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDF_FIELD_TYPE)

// Convert an RRDF_FIELD_TYPE to a simplified JSON scalar type string 
// for easier LLM comprehension
const char *field_type_to_json_scalar_type(RRDF_FIELD_TYPE type);

typedef enum __attribute__((packed)) {
    RRDF_FIELD_VISUAL_VALUE,        // show the value, possibly applying a transformation
    RRDF_FIELD_VISUAL_BAR,          // show the value and a bar, respecting the max field to fill the bar at 100%
    RRDF_FIELD_VISUAL_PILL,         //
    RRDF_FIELD_VISUAL_RICH,         //
    RRDR_FIELD_VISUAL_ROW_OPTIONS,  // this is a dummy column that is used for row options
} RRDF_FIELD_VISUAL;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDF_FIELD_VISUAL)

typedef enum __attribute__((packed)) {
    RRDF_FIELD_TRANSFORM_NONE,      // show the value as-is
    RRDF_FIELD_TRANSFORM_NUMBER,    // show the value respecting the decimal_points
    RRDF_FIELD_TRANSFORM_DURATION_S,  // transform as duration in second to a human-readable duration
    RRDF_FIELD_TRANSFORM_DATETIME_MS,  // UNIX epoch timestamp in ms
    RRDF_FIELD_TRANSFORM_DATETIME_USEC,  // UNIX epoch timestamp in usec
    RRDF_FIELD_TRANSFORM_XML,   // format the field with an XML prettifier
} RRDF_FIELD_TRANSFORM;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDF_FIELD_TRANSFORM)

typedef enum __attribute__((packed)) {
    RRDF_FIELD_SORT_ASCENDING  = (1 << 0),
    RRDF_FIELD_SORT_DESCENDING = (1 << 1),

    RRDF_FIELD_SORT_FIXED      = (1 << 7),
} RRDF_FIELD_SORT;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDF_FIELD_SORT)

typedef enum __attribute__((packed)) {
    RRDF_FIELD_SUMMARY_UNIQUECOUNT,     // Finds the number of unique values of a group of rows
    RRDF_FIELD_SUMMARY_SUM,             // Sums the values of a group of rows
    RRDF_FIELD_SUMMARY_MIN,             // Finds the minimum value of a group of rows
    RRDF_FIELD_SUMMARY_MAX,             // Finds the maximum value of a group of rows
    // RRDF_FIELD_SUMMARY_EXTENT,          // Finds the minimum and maximum values of a group of rows
    RRDF_FIELD_SUMMARY_MEAN,            // Finds the mean/average value of a group of rows
    RRDF_FIELD_SUMMARY_MEDIAN,          // Finds the median value of a group of rows
    // RRDF_FIELD_SUMMARY_UNIQUE,         // Finds the unique values of a group of rows
    RRDF_FIELD_SUMMARY_COUNT,           // Calculates the number of rows in a group
} RRDF_FIELD_SUMMARY;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDF_FIELD_SUMMARY)

typedef enum __attribute__((packed)) {
    RRDF_FIELD_FILTER_NONE = 0,
    RRDF_FIELD_FILTER_RANGE,
    RRDF_FIELD_FILTER_MULTISELECT,
    RRDF_FIELD_FILTER_FACET,
} RRDF_FIELD_FILTER;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(RRDF_FIELD_FILTER)

void buffer_rrdf_table_add_field(BUFFER *wb, size_t field_id, const char *key, const char *name, RRDF_FIELD_TYPE type,
                                RRDF_FIELD_VISUAL visual, RRDF_FIELD_TRANSFORM transform, size_t decimal_points,
                                const char *units, NETDATA_DOUBLE max, RRDF_FIELD_SORT sort, const char *pointer_to,
                                RRDF_FIELD_SUMMARY summary, RRDF_FIELD_FILTER filter, RRDF_FIELD_OPTIONS options,
                                const char *default_value);

#endif /* NETDATA_FUNCTIONS_FIELDS_H */