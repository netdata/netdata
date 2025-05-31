// SPDX-License-Identifier: GPL-3.0-or-later

#include "functions_fields.h"

// Define mapping tables for the enums using the template macros

// RRDF_FIELD_TYPE mapping
ENUM_STR_MAP_DEFINE(RRDF_FIELD_TYPE) = {
    {RRDF_FIELD_TYPE_NONE, "none"},
    {RRDF_FIELD_TYPE_INTEGER, "integer"},
    {RRDF_FIELD_TYPE_BOOLEAN, "boolean"},
    {RRDF_FIELD_TYPE_STRING, "string"},
    {RRDF_FIELD_TYPE_DETAIL_STRING, "detail-string"},
    {RRDF_FIELD_TYPE_BAR_WITH_INTEGER, "bar-with-integer"},
    {RRDF_FIELD_TYPE_DURATION, "duration"},
    {RRDF_FIELD_TYPE_TIMESTAMP, "timestamp"},
    {RRDF_FIELD_TYPE_ARRAY, "array"},
    {0, NULL}
};

// Define the conversion functions
ENUM_STR_DEFINE_FUNCTIONS(RRDF_FIELD_TYPE, RRDF_FIELD_TYPE_NONE, "none")

// Convert an RRDF_FIELD_TYPE to a simplified JSON scalar type
const char *field_type_to_json_scalar_type(RRDF_FIELD_TYPE type) {
    switch (type) {
        case RRDF_FIELD_TYPE_INTEGER:
        case RRDF_FIELD_TYPE_BAR_WITH_INTEGER:
        case RRDF_FIELD_TYPE_DURATION:
        case RRDF_FIELD_TYPE_TIMESTAMP:
            return "number";
            
        case RRDF_FIELD_TYPE_BOOLEAN:
            return "boolean";
            
        case RRDF_FIELD_TYPE_STRING:
        case RRDF_FIELD_TYPE_DETAIL_STRING:
            return "string";
            
        case RRDF_FIELD_TYPE_ARRAY:
            return "array";
            
        case RRDF_FIELD_TYPE_NONE:
        default:
            return "unknown";
    }
}

// RRDF_FIELD_VISUAL mapping
ENUM_STR_MAP_DEFINE(RRDF_FIELD_VISUAL) = {
    {RRDF_FIELD_VISUAL_VALUE, "value"},
    {RRDF_FIELD_VISUAL_BAR, "bar"},
    {RRDF_FIELD_VISUAL_PILL, "pill"},
    {RRDF_FIELD_VISUAL_RICH, "richValue"},
    {RRDR_FIELD_VISUAL_ROW_OPTIONS, "rowOptions"},
    {0, NULL}
};

// Define the conversion functions
ENUM_STR_DEFINE_FUNCTIONS(RRDF_FIELD_VISUAL, RRDF_FIELD_VISUAL_VALUE, "value")

// RRDF_FIELD_TRANSFORM mapping
ENUM_STR_MAP_DEFINE(RRDF_FIELD_TRANSFORM) = {
    {RRDF_FIELD_TRANSFORM_NONE, "none"},
    {RRDF_FIELD_TRANSFORM_NUMBER, "number"},
    {RRDF_FIELD_TRANSFORM_DURATION_S, "duration"},
    {RRDF_FIELD_TRANSFORM_DATETIME_MS, "datetime"},
    {RRDF_FIELD_TRANSFORM_DATETIME_USEC, "datetime_usec"},
    {RRDF_FIELD_TRANSFORM_XML, "xml"},
    {0, NULL}
};

// Define the conversion functions
ENUM_STR_DEFINE_FUNCTIONS(RRDF_FIELD_TRANSFORM, RRDF_FIELD_TRANSFORM_NONE, "none")

// RRDF_FIELD_SORT mapping
ENUM_STR_MAP_DEFINE(RRDF_FIELD_SORT) = {
    {RRDF_FIELD_SORT_ASCENDING, "ascending"},
    {RRDF_FIELD_SORT_DESCENDING, "descending"},
    {0, NULL}
};

// Define the conversion functions
ENUM_STR_DEFINE_FUNCTIONS(RRDF_FIELD_SORT, RRDF_FIELD_SORT_ASCENDING, "ascending")

// RRDF_FIELD_SUMMARY mapping
ENUM_STR_MAP_DEFINE(RRDF_FIELD_SUMMARY) = {
    {RRDF_FIELD_SUMMARY_COUNT, "count"},
    {RRDF_FIELD_SUMMARY_UNIQUECOUNT, "uniqueCount"},
    {RRDF_FIELD_SUMMARY_SUM, "sum"},
    {RRDF_FIELD_SUMMARY_MIN, "min"},
    {RRDF_FIELD_SUMMARY_MAX, "max"},
    {RRDF_FIELD_SUMMARY_MEAN, "mean"},
    {RRDF_FIELD_SUMMARY_MEDIAN, "median"},
    {0, NULL}
};

// Define the conversion functions
ENUM_STR_DEFINE_FUNCTIONS(RRDF_FIELD_SUMMARY, RRDF_FIELD_SUMMARY_COUNT, "count")

// RRDF_FIELD_FILTER mapping
ENUM_STR_MAP_DEFINE(RRDF_FIELD_FILTER) = {
    {RRDF_FIELD_FILTER_NONE, "none"},
    {RRDF_FIELD_FILTER_RANGE, "range"},
    {RRDF_FIELD_FILTER_MULTISELECT, "multiselect"},
    {RRDF_FIELD_FILTER_FACET, "facet"},
    {0, NULL}
};

// Define the conversion functions
ENUM_STR_DEFINE_FUNCTIONS(RRDF_FIELD_FILTER, RRDF_FIELD_FILTER_NONE, "none")

void buffer_rrdf_table_add_field(BUFFER *wb, size_t field_id, const char *key, const char *name, RRDF_FIELD_TYPE type,
                                RRDF_FIELD_VISUAL visual, RRDF_FIELD_TRANSFORM transform, size_t decimal_points,
                                const char *units, NETDATA_DOUBLE max, RRDF_FIELD_SORT sort, const char *pointer_to,
                                RRDF_FIELD_SUMMARY summary, RRDF_FIELD_FILTER filter, RRDF_FIELD_OPTIONS options,
                                const char *default_value) {

    buffer_json_member_add_object(wb, key);
    {
        buffer_json_member_add_uint64(wb, "index", field_id);
        buffer_json_member_add_boolean(wb, "unique_key", options & RRDF_FIELD_OPTS_UNIQUE_KEY);
        buffer_json_member_add_string(wb, "name", name);
        buffer_json_member_add_boolean(wb, "visible", options & RRDF_FIELD_OPTS_VISIBLE);
        buffer_json_member_add_string(wb, "type", RRDF_FIELD_TYPE_2str(type));
        buffer_json_member_add_string_or_omit(wb, "units", units);
        buffer_json_member_add_string(wb, "visualization", RRDF_FIELD_VISUAL_2str(visual));

        buffer_json_member_add_object(wb, "value_options");
        {
            buffer_json_member_add_string_or_omit(wb, "units", units);
            buffer_json_member_add_string(wb, "transform", RRDF_FIELD_TRANSFORM_2str(transform));
            buffer_json_member_add_uint64(wb, "decimal_points", decimal_points);
            buffer_json_member_add_string(wb, "default_value", default_value);
        }
        buffer_json_object_close(wb);

        if (!isnan((NETDATA_DOUBLE) (max)))
            buffer_json_member_add_double(wb, "max", (NETDATA_DOUBLE) (max));

        buffer_json_member_add_string_or_omit(wb, "pointer_to", pointer_to);
        buffer_json_member_add_string(wb, "sort", RRDF_FIELD_SORT_2str(sort));
        buffer_json_member_add_boolean(wb, "sortable", !(sort & RRDF_FIELD_SORT_FIXED));
        buffer_json_member_add_boolean(wb, "sticky", options & RRDF_FIELD_OPTS_STICKY);
        buffer_json_member_add_string(wb, "summary", RRDF_FIELD_SUMMARY_2str(summary));
        buffer_json_member_add_string(wb, "filter", RRDF_FIELD_FILTER_2str(filter));

        buffer_json_member_add_boolean(wb, "full_width", options & RRDF_FIELD_OPTS_FULL_WIDTH);
        buffer_json_member_add_boolean(wb, "wrap", options & RRDF_FIELD_OPTS_WRAP);
        buffer_json_member_add_boolean(wb, "default_expanded_filter", options & RRDF_FIELD_OPTS_EXPANDED_FILTER);

        if(options & RRDF_FIELD_OPTS_DUMMY)
            buffer_json_member_add_boolean(wb, "dummy", true);
    }
    buffer_json_object_close(wb);
}