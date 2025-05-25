// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-time-utils.h"
#include "libnetdata/datetime/rfc3339.h"

time_t mcp_extract_time_param(struct json_object *params, const char *name, time_t default_value) {
    if (!params)
        return default_value;
        
    struct json_object *obj = NULL;
    if (!json_object_object_get_ex(params, name, &obj) || !obj)
        return default_value;
        
    // First try as integer
    if (json_object_is_type(obj, json_type_int))
        return json_object_get_int64(obj);
        
    // Then try as string
    if (json_object_is_type(obj, json_type_string)) {
        const char *val_str = json_object_get_string(obj);
        if (!val_str || !*val_str)
            return default_value;
            
        // Try to parse as RFC3339 first
        char *endptr = NULL;
        usec_t timestamp_ut = rfc3339_parse_ut(val_str, &endptr);
        
        // Check if RFC3339 parsing was successful
        if (timestamp_ut > 0 && endptr && (*endptr == '\0' || isspace((unsigned char)*endptr))) {
            // Successfully parsed as RFC3339, convert to seconds
            return (time_t)(timestamp_ut / USEC_PER_SEC);
        }
        
        // Fall back to parsing as integer (epoch seconds or relative time)
        return str2l(val_str);
    }
    
    return default_value;
}

size_t mcp_time_to_rfc3339(char *buffer, size_t len, time_t timestamp, bool utc) {
    if (!buffer || len == 0)
        return 0;
        
    // Convert to microseconds for rfc3339_datetime_ut
    usec_t timestamp_ut = (usec_t)timestamp * USEC_PER_SEC;
    
    // Use 0 fractional digits for time_t precision
    return rfc3339_datetime_ut(buffer, len, timestamp_ut, 0, utc);
}

size_t mcp_time_ut_to_rfc3339(char *buffer, size_t len, usec_t timestamp_ut, size_t fractional_digits, bool utc) {
    if (!buffer || len == 0)
        return 0;
        
    return rfc3339_datetime_ut(buffer, len, timestamp_ut, fractional_digits, utc);
}