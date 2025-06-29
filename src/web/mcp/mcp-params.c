// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-params.h"

// Utility function to check if a string contains wildcards
static bool contains_wildcards(const char *str) {
    if (!str || !*str) return false;

    if(*str == '!') {
        // simple patterns negative match (only if it starts with '!')
        return true;
    }

    // Check for common wildcard characters
    for (const char *p = str; *p; p++) {
        if (*p == '*') {
            // simple patterns wildcard match
            return true;
        }

        // let's check for simple pattern separators
        for (const char *c = SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS; *c ; c++) {
            if (*p == *c)
                return true;
        }
    }
    return false;
}

// Convert JSON array to pipe-separated string
static int mcp_params_array_to_pipe_string(
    struct json_object *array_obj,
    BUFFER *output_buffer,
    bool allow_wildcards
) {
    if (!array_obj || !output_buffer)
        return -1;
        
    if (!json_object_is_type(array_obj, json_type_array))
        return -1;
        
    size_t array_len = json_object_array_length(array_obj);
    if (array_len == 0)
        return 0;  // Empty array is valid
        
    for (size_t i = 0; i < array_len; i++) {
        struct json_object *item = json_object_array_get_idx(array_obj, i);
        if (!json_object_is_type(item, json_type_string))
            return -1;
            
        const char *str = json_object_get_string(item);
        if (!str || !*str)
            return -1;
            
        // Check for wildcards if not allowed
        if (!allow_wildcards && contains_wildcards(str))
            return -2;
            
        if (i > 0)
            buffer_strcat(output_buffer, "|");
        buffer_strcat(output_buffer, str);
    }
    
    return 0;
}

// Parse array parameters (nodes, instances, dimensions)
BUFFER *mcp_params_parse_array_to_pattern(
    struct json_object *params,
    const char *param_name,
    bool required,
    bool allow_wildcards,
    const char *list_tool,
    BUFFER *error
) {
    struct json_object *array_obj = NULL;
    
    if (!json_object_object_get_ex(params, param_name, &array_obj) || !array_obj) {
        if (required && error) {
            buffer_flush(error);
            buffer_sprintf(error, "Missing required parameter '%s'", param_name);
        }
        return NULL;  // Parameter not provided
    }
        
    if (!json_object_is_type(array_obj, json_type_array)) {
        if (error) {
            buffer_flush(error);
            buffer_sprintf(error, "%s must be an array of strings, not %s", 
                    param_name, json_type_to_name(json_object_get_type(array_obj)));
        }
        return NULL;
    }
    
    // Check for an empty array if required
    if (required && json_object_array_length(array_obj) == 0) {
        if (error) {
            buffer_flush(error);
            buffer_sprintf(error, "The '%s' parameter cannot be an empty array", param_name);
        }
        return NULL;
    }
        
    BUFFER *wb = buffer_create(256, NULL);
    int result = mcp_params_array_to_pipe_string(array_obj, wb, allow_wildcards);
    
    if (result == -2) {
        buffer_free(wb);
        if (error) {
            buffer_flush(error);
            if (allow_wildcards) {
                buffer_strcat(error, "invalid array format");
            } else {
                if (list_tool) {
                    buffer_sprintf(error,
                            "must contain exact values, not patterns. "
                            "Wildcards are not supported. "
                            "Use the '%s' tool to discover exact values.", list_tool);
                } else {
                    buffer_strcat(error,
                            "must contain exact values, not patterns. Wildcards are not supported.");
                }
            }
        }
        return NULL;
    } else if (result != 0) {
        buffer_free(wb);
        if (error) {
            buffer_flush(error);
            buffer_sprintf(error, "%s must be an array of strings", param_name);
        }
        return NULL;
    }
    
    return wb;  // Success - return the buffer
}

// Parse labels object parameter
BUFFER *mcp_params_parse_labels_object(
    struct json_object *params,
    const char *list_tool,
    BUFFER *error
) {
    struct json_object *labels_obj = NULL;
    
    if (!json_object_object_get_ex(params, "labels", &labels_obj) || !labels_obj)
        return NULL;  // Parameter not provided
        
    if (!json_object_is_type(labels_obj, json_type_object)) {
        if (error) {
            buffer_flush(error);
            buffer_strcat(error, "must be an object where each key maps to an array of string values");
        }
        return NULL;
    }
        
    BUFFER *wb = buffer_create(256, NULL);
    int first = 1;
    
    struct json_object_iterator it = json_object_iter_begin(labels_obj);
    struct json_object_iterator itEnd = json_object_iter_end(labels_obj);
    
    while (!json_object_iter_equal(&it, &itEnd)) {
        const char *key = json_object_iter_peek_name(&it);
        struct json_object *val = json_object_iter_peek_value(&it);
        
        if (!json_object_is_type(val, json_type_array)) {
            buffer_free(wb);
            if (error) {
                buffer_flush(error);
                buffer_strcat(error, "each label key must map to an array of string values");
            }
            return NULL;
        }
        
        size_t array_len = json_object_array_length(val);
        for (size_t i = 0; i < array_len; i++) {
            struct json_object *item = json_object_array_get_idx(val, i);
            if (!json_object_is_type(item, json_type_string)) {
                buffer_free(wb);
                if (error) {
                    buffer_flush(error);
                    buffer_strcat(error, "label values must be strings");
                }
                return NULL;
            }
            
            const char *value = json_object_get_string(item);
            if (!value || !*value) {
                buffer_free(wb);
                if (error) {
                    buffer_flush(error);
                    buffer_strcat(error, "label values cannot be empty");
                }
                return NULL;
            }
            
            // Check for wildcards in label values
            if (contains_wildcards(value)) {
                buffer_free(wb);
                if (error) {
                    buffer_flush(error);
                    if (list_tool) {
                        buffer_sprintf(error,
                                "label values must be exact values, not patterns. "
                                "Wildcards are not supported. "
                                "Use the %s tool to discover available label values.", list_tool);
                    } else {
                        buffer_strcat(error,
                                "label values must be exact values, not patterns. Wildcards are not supported.");
                    }
                }
                return NULL;
            }
            
            if (!first) buffer_strcat(wb, "|");
            buffer_sprintf(wb, "%s:%s", key, value);
            first = 0;
        }
        
        json_object_iter_next(&it);
    }
    
    return wb;  // Success
}


// Schema generation functions

// Add array parameter schema
void mcp_schema_add_array_param(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description
) {
    buffer_json_member_add_object(buffer, param_name);
    {
        buffer_json_member_add_string(buffer, "type", "array");
        buffer_json_member_add_string(buffer, "title", title);
        buffer_json_member_add_string(buffer, "description", description);
        buffer_json_member_add_object(buffer, "items");
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_object_close(buffer); // items
    }
    buffer_json_object_close(buffer);
}

// Add labels object parameter schema
void mcp_schema_add_labels_object(
    BUFFER *buffer,
    const char *title,
    const char *description
) {
    buffer_json_member_add_object(buffer, "labels");
    {
        buffer_json_member_add_string(buffer, "type", "object");
        buffer_json_member_add_string(buffer, "title", 
            title ? title : "Filter by labels");
        buffer_json_member_add_string(buffer, "description", 
            description ? description :
                "Filter using labels where each key maps to an array of exact values. "
                "Values in the same array are ORed, different keys are ANDed. "
                "Example: {\"disk_type\": [\"ssd\", \"nvme\"], \"mount_point\": [\"/\"]}\n"
                "Note: Wildcards are not supported. Use exact label keys and values only.");
        buffer_json_member_add_object(buffer, "additionalProperties");
        {
            buffer_json_member_add_string(buffer, "type", "array");
            buffer_json_member_add_object(buffer, "items");
            buffer_json_member_add_string(buffer, "type", "string");
            buffer_json_object_close(buffer); // items
        }
        buffer_json_object_close(buffer); // additionalProperties
    }
    buffer_json_object_close(buffer); // labels
}

// Add time window parameters to the schema
void mcp_schema_add_time_params(
    BUFFER *buffer,
    const char *time_description_prefix,
    bool required
) {
    // Build descriptions for after and before
    BUFFER *after_desc = buffer_create(256, NULL);
    BUFFER *before_desc = buffer_create(256, NULL);
    
    if (time_description_prefix && *time_description_prefix) {
        buffer_sprintf(after_desc, "Start time for %s.", time_description_prefix);
        buffer_sprintf(before_desc, "End time for %s.", time_description_prefix);
    } else {
        buffer_strcat(after_desc, "Start time for the query.");
        buffer_strcat(before_desc, "End time for the query.");
    }
    
    // Use the new function for both parameters
    mcp_schema_add_time_param(buffer, "after", "Start time", buffer_tostring(after_desc), 
                              "'before'", required ? 0 : MCP_DEFAULT_AFTER_TIME, required);
    
    mcp_schema_add_time_param(buffer, "before", "End time", buffer_tostring(before_desc),
                              "now", required ? 0 : MCP_DEFAULT_BEFORE_TIME, required);
    
    buffer_free(after_desc);
    buffer_free(before_desc);
}

// Add cardinality limit parameter to schema
void mcp_schema_add_cardinality_limit(
    BUFFER *buffer,
    const char *description,
    size_t default_value,
    size_t min_value,
    size_t max_value
) {
    buffer_json_member_add_object(buffer, "cardinality_limit");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Cardinality Limit");
        buffer_json_member_add_string(
            buffer, "description",
            description ? description : "When multiple nodes, instances, dimensions, labels are queried, "
                                        "limit their numbers to prevent response size explosion.");
        
        if (default_value > 0)
            buffer_json_member_add_uint64(buffer, "default", default_value);
        
        buffer_json_member_add_uint64(buffer, "minimum", min_value > 0 ? min_value : 1);
        
        if (max_value > 0)
            buffer_json_member_add_uint64(buffer, "maximum", max_value);
    }
    buffer_json_object_close(buffer);
}

// Extract string parameter with optional default
const char *mcp_params_extract_string(
    struct json_object *params,
    const char *param_name,
    const char *default_value
) {
    struct json_object *obj = NULL;
    
    if (json_object_object_get_ex(params, param_name, &obj) && obj) {
        if (json_object_is_type(obj, json_type_string)) {
            const char *value = json_object_get_string(obj);
            if (value && *value) {
                return value;
            }
        }
    }
    
    return default_value;
}

// Extract numeric size parameter with bounds checking
size_t mcp_params_extract_size(
    struct json_object *params,
    const char *param_name,
    size_t default_value,
    size_t min_value,
    size_t max_value,
    BUFFER *error
) {
    struct json_object *obj = NULL;
    
    if (json_object_object_get_ex(params, param_name, &obj) && obj) {
        if (json_object_is_type(obj, json_type_int)) {
            int64_t value = json_object_get_int64(obj);
            if (value < 0) {
                if (error) {
                    buffer_flush(error);
                    buffer_sprintf(error, "%s must be a positive number", param_name);
                }
                return default_value;
            }
            size_t size_value = (size_t)value;
            if (size_value < min_value || (max_value > 0 && size_value > max_value)) {
                if (error) {
                    buffer_flush(error);
                    if (max_value > 0) {
                        buffer_sprintf(error,
                                "%s must be between %zu and %zu",
                                param_name, min_value, max_value);
                    } else {
                        buffer_sprintf(error,
                                "%s must be at least %zu",
                                param_name, min_value);
                    }
                }
                return default_value;
            }
            return size_value;
        }
    }
    
    return default_value;
}

// Extract timeout parameter (in seconds)
int mcp_params_extract_timeout(
    struct json_object *params,
    const char *param_name,
    int default_seconds,
    int min_seconds,
    int max_seconds,
    BUFFER *error
) {
    struct json_object *obj = NULL;
    
    if (json_object_object_get_ex(params, param_name, &obj) && obj) {
        if (json_object_is_type(obj, json_type_int)) {
            int value = json_object_get_int(obj);
            if (value < min_seconds || (max_seconds > 0 && value > max_seconds)) {
                if (error) {
                    buffer_flush(error);
                    if (max_seconds > 0) {
                        buffer_sprintf(error,
                                "%s must be between %d and %d seconds",
                                param_name, min_seconds, max_seconds);
                    } else {
                        buffer_sprintf(error,
                                "%s must be at least %d seconds",
                                param_name, min_seconds);
                    }
                }
                return default_seconds;
            }
            return value;
        }
    }
    
    return default_seconds;
}

// Schema generation for timeout parameter
void mcp_schema_add_timeout(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description,
    int default_seconds,
    int min_seconds,
    int max_seconds,
    bool required
) {
    buffer_json_member_add_object(buffer, param_name);
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", title);
        buffer_json_member_add_string(buffer, "description", description);
        
        if (!required && default_seconds >= 0)
            buffer_json_member_add_int64(buffer, "default", default_seconds);
            
        if (min_seconds >= 0)
            buffer_json_member_add_int64(buffer, "minimum", min_seconds);
            
        if (max_seconds > 0)
            buffer_json_member_add_int64(buffer, "maximum", max_seconds);
    }
    buffer_json_object_close(buffer);
}

// Schema generation for generic string parameter
void mcp_schema_add_string_param(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description,
    const char *default_value,
    bool required
) {
    buffer_json_member_add_object(buffer, param_name);
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", title);
        buffer_json_member_add_string(buffer, "description", description);
        
        if (!required && default_value)
            buffer_json_member_add_string(buffer, "default", default_value);
    }
    buffer_json_object_close(buffer);
}

// Schema generation for size parameter
void mcp_schema_add_size_param(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description,
    size_t default_value,
    size_t min_value,
    size_t max_value,
    bool required
) {
    buffer_json_member_add_object(buffer, param_name);
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", title);
        buffer_json_member_add_string(buffer, "description", description);
        
        if (!required)
            buffer_json_member_add_uint64(buffer, "default", default_value);
            
        if (min_value > 0)
            buffer_json_member_add_uint64(buffer, "minimum", min_value);
            
        if (max_value < SIZE_MAX)
            buffer_json_member_add_uint64(buffer, "maximum", max_value);
    }
    buffer_json_object_close(buffer);
}

// Validate and auto-correct time window parameters
// Contract: 'after' is relative to 'before', 'before' is relative to 'now'
// Since we're a monitoring solution, we almost always work in the past
void mcp_params_validate_time_window(time_t *after, time_t *before, time_t now) {
    if (!now) now = now_realtime_sec();
    
    // Check if both are relative times (within 3 years of zero)
    bool after_is_relative = (ABS(*after) <= API_RELATIVE_TIME_MAX);
    bool before_is_relative = (ABS(*before) <= API_RELATIVE_TIME_MAX);
    
    if (after_is_relative && before_is_relative) {
        // Case 1: Both are relative and positive - assistant didn't read instructions
        if (*after > 0 && *before > 0) {
            *after = -*after;
            *before = -*before;
        }
        // Case 2: After is positive, before is negative, check if result makes sense
        else if (*after > 0 && *before <= 0) {
            // If after + before > 0, the assistant is confused about relative time
            if (*after + *before > 0) {
                *after = -*after;
            }
        }
    }
}

// Parse and validate time window parameters (after and before) together
// This ensures consistent parsing and validation across all MCP tools
bool mcp_params_parse_time_window(
    struct json_object *params,
    time_t *after,
    time_t *before,
    time_t default_after,
    time_t default_before,
    bool allow_both_zero,
    BUFFER *error
) {
    if (!after || !before) {
        if (error) {
            buffer_flush(error);
            buffer_strcat(error, "Internal error: after and before pointers cannot be NULL");
        }
        return false;
    }
    
    // Parse both time parameters
    *after = mcp_params_parse_time(params, "after", default_after);
    *before = mcp_params_parse_time(params, "before", default_before);
    
    // Apply validation and auto-correction
    mcp_params_validate_time_window(after, before, 0);
    
    // Basic validation - both cannot be zero (unless explicitly allowed)
    if (*after == 0 && *before == 0 && !allow_both_zero) {
        if (error) {
            buffer_flush(error);
            buffer_strcat(error, "Invalid time range: both 'after' and 'before' cannot be zero. "
                                 "Use negative values for relative times (e.g., after=-3600, before=0 for the last hour) "
                                 "or specific timestamps for absolute times.");
        }
        return false;
    }
    
    // Check if after is later than before (when both are absolute timestamps)
    bool after_is_absolute = (ABS(*after) > API_RELATIVE_TIME_MAX);
    bool before_is_absolute = (ABS(*before) > API_RELATIVE_TIME_MAX);
    
    if (after_is_absolute && before_is_absolute && *after >= *before) {
        if (error) {
            buffer_flush(error);
            buffer_sprintf(error, "Invalid time range: 'after' (%ld) must be earlier than 'before' (%ld) "
                                  "when both are absolute timestamps.", *after, *before);
        }
        return false;
    }
    
    return true;
}

time_t mcp_params_parse_time(struct json_object *params, const char *name, time_t default_value) {
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
        // Note: We check endptr to see if the entire string was consumed
        // We don't check timestamp_ut > 0 because dates before 1970 are valid
        if (endptr && (*endptr == '\0' || isspace((unsigned char)*endptr))) {
            // Successfully parsed as RFC3339, convert to seconds
            return (time_t)(timestamp_ut / USEC_PER_SEC);
        }

        // Check for special "now" keyword first
        if (strcasecmp(val_str, "now") == 0) {
            return 0;  // "now" means no offset from current time
        }
        
        // Try duration parsing for human-readable durations
        // This handles human-readable durations like:
        // - "7d", "7 days", "2h", "30m"
        // - "7 days ago", "2h ago" (negative)
        // - Complex expressions: "1d12h", "2h30m ago"
        int64_t duration_seconds;
        if (duration_parse(val_str, &duration_seconds, "s", "s")) {
            return (time_t)duration_seconds;
        }
        
        // Duration parsing failed, fall back to parsing as integer
        // This handles:
        // - Unix timestamps as strings: "1705318200"
        // - Relative times as strings: "-3600", "-86400"
        return str2l(val_str);
    }

    return default_value;
}

// Schema generation for individual time parameter
void mcp_schema_add_time_param(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description,
    const char *relative_to,
    time_t default_value,
    bool required
) {
    buffer_json_member_add_object(buffer, param_name);
    {
        buffer_json_member_add_string(buffer, "title", title);

        if (description && *description)
            buffer_json_member_add_string(buffer, "description", description);
        else
            buffer_json_member_add_string(
                buffer, "description",
                "Unix epoch timestamp in seconds (e.g. 1705318200), "
                "number of seconds (use NEGATIVE for past times), "
                "human-readable duration (e.g. '-7d', '-2h', '-30m', '7 days ago'), "
                "or RFC3339 datetime string");

        // Use anyOf for multiple types
        buffer_json_member_add_array(buffer, "anyOf");
        {
            buffer_json_add_array_item_object(buffer);
            {
                buffer_json_member_add_string(buffer, "type", "number");
                buffer_json_member_add_sprintf(buffer, "description",
                    "Unix epoch timestamp in seconds (e.g. 1705318200), or number of seconds relative to %s (e.g. -3600 for an hour before %s). NOTE: Use NEGATIVE values for past times.",
                    relative_to ? relative_to : "now", relative_to ? relative_to : "now");
            }
            buffer_json_object_close(buffer);
            
            buffer_json_add_array_item_object(buffer);
            {
                buffer_json_member_add_string(buffer, "type", "string");
                buffer_json_member_add_string(buffer, "description", 
                    "RFC3339 datetime string (e.g., \"2024-01-15T10:30:00Z\", \"2024-01-15T10:30:00-05:00\"), "
                    "or human-readable duration (e.g., \"-7d\", \"-2h\", \"-30m\", \"7 days ago\", \"now\"). NOTE: Use NEGATIVE values for past times.");
            }
            buffer_json_object_close(buffer);
        }
        buffer_json_array_close(buffer);

        if (!required && default_value != 0) {
            buffer_json_member_add_int64(buffer, "default", default_value);
        }
    }
    buffer_json_object_close(buffer);
}
