// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-params.h"
#include "mcp-time-utils.h"
#include "web/api/queries/rrdr.h"

// Utility function to check if a string contains wildcards
static bool contains_wildcards(const char *str) {
    if (!str) return false;
    
    // Check for common wildcard characters
    for (const char *p = str; *p; p++) {
        if (*p == '*' || *p == '?' || *p == '[' || *p == ']')
            return true;
    }
    return false;
}

// Convert JSON array to pipe-separated string
int mcp_params_array_to_pipe_string(
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
    bool allow_wildcards,
    const char *list_tool,
    const char **error_message
) {
    struct json_object *array_obj = NULL;
    
    if (!json_object_object_get_ex(params, param_name, &array_obj) || !array_obj)
        return NULL;  // Parameter not provided
        
    if (!json_object_is_type(array_obj, json_type_array)) {
        if (error_message)
            *error_message = "must be an array of strings";
        return NULL;
    }
        
    BUFFER *wb = buffer_create(256, NULL);
    int result = mcp_params_array_to_pipe_string(array_obj, wb, allow_wildcards);
    
    if (result == -2) {
        buffer_free(wb);
        if (error_message) {
            if (allow_wildcards) {
                *error_message = "invalid array format";
            } else {
                static char error_buffer[256];
                if (list_tool) {
                    snprintf(error_buffer, sizeof(error_buffer),
                            "must contain exact values, not patterns. "
                            "Wildcards are not supported. "
                            "Use the %s tool to discover exact values.", list_tool);
                } else {
                    snprintf(error_buffer, sizeof(error_buffer),
                            "must contain exact values, not patterns. Wildcards are not supported.");
                }
                *error_message = error_buffer;
            }
        }
        return NULL;
    } else if (result != 0) {
        buffer_free(wb);
        if (error_message)
            *error_message = "must be an array of strings";
        return NULL;
    }
    
    return wb;  // Success - return the buffer
}

// Parse contexts parameter as an array
BUFFER *mcp_params_parse_contexts_array(
    struct json_object *params,
    bool allow_wildcards,
    const char *list_tool,
    const char **error_message
) {
    return mcp_params_parse_array_to_pattern(params, "contexts", allow_wildcards, list_tool, error_message);
}

// Parse labels object parameter
BUFFER *mcp_params_parse_labels_object(
    struct json_object *params,
    const char *list_tool,
    const char **error_message
) {
    struct json_object *labels_obj = NULL;
    
    if (!json_object_object_get_ex(params, "labels", &labels_obj) || !labels_obj)
        return NULL;  // Parameter not provided
        
    if (!json_object_is_type(labels_obj, json_type_object)) {
        if (error_message)
            *error_message = "must be an object where each key maps to an array of string values";
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
            if (error_message)
                *error_message = "each label key must map to an array of string values";
            return NULL;
        }
        
        size_t array_len = json_object_array_length(val);
        for (size_t i = 0; i < array_len; i++) {
            struct json_object *item = json_object_array_get_idx(val, i);
            if (!json_object_is_type(item, json_type_string)) {
                buffer_free(wb);
                if (error_message)
                    *error_message = "label values must be strings";
                return NULL;
            }
            
            const char *value = json_object_get_string(item);
            if (!value || !*value) {
                buffer_free(wb);
                if (error_message)
                    *error_message = "label values cannot be empty";
                return NULL;
            }
            
            // Check for wildcards in label values
            if (contains_wildcards(value)) {
                buffer_free(wb);
                if (error_message) {
                    static char labels_error[256];
                    if (list_tool) {
                        snprintf(labels_error, sizeof(labels_error),
                                "label values must be exact values, not patterns. "
                                "Wildcards are not supported. "
                                "Use the %s tool to discover available label values.", list_tool);
                    } else {
                        snprintf(labels_error, sizeof(labels_error),
                                "label values must be exact values, not patterns. Wildcards are not supported.");
                    }
                    *error_message = labels_error;
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

// Parse time parameters
time_t mcp_params_parse_time(
    struct json_object *params,
    const char *param_name,
    time_t default_value
) {
    return mcp_extract_time_param(params, param_name, default_value);
}

// Schema generation functions

// Add array parameter schema
void mcp_schema_add_array_param(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description,
    bool required
) {
    buffer_json_member_add_object(buffer, param_name);
    {
        buffer_json_member_add_string(buffer, "type", "array");
        buffer_json_member_add_string(buffer, "title", title);
        buffer_json_member_add_string(buffer, "description", description);
        buffer_json_member_add_object(buffer, "items");
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_object_close(buffer); // items
        if (!required) {
            buffer_json_member_add_string(buffer, "default", NULL);
        }
    }
    buffer_json_object_close(buffer);
}

// Add contexts array parameter schema
void mcp_schema_add_contexts_array(
    BUFFER *buffer,
    const char *title,
    const char *description,
    bool required
) {
    mcp_schema_add_array_param(buffer, "contexts", 
        title ? title : "Filter by metric contexts",
        description ? description : 
            "Array of exact context names to include (e.g., ['system.cpu', 'disk.io']). "
            "Wildcards are not supported. Use exact context names only.",
        required);
}

// Add labels object parameter schema
void mcp_schema_add_labels_object(
    BUFFER *buffer,
    const char *title,
    const char *description,
    bool required
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
        if (!required) {
            buffer_json_member_add_string(buffer, "default", NULL);
        }
    }
    buffer_json_object_close(buffer); // labels
}

// Add time window parameters to schema
void mcp_schema_add_time_params(
    BUFFER *buffer,
    const char *time_description_prefix,
    bool required
) {
    // After parameter
    buffer_json_member_add_object(buffer, "after");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Start time");
        
        BUFFER *desc = buffer_create(512, NULL);
        buffer_sprintf(desc, "Start time for %s. Accepts:\n"
                            "- Unix timestamp in seconds\n"
                            "- Negative values for relative time (e.g., -3600 for 1 hour ago)\n"
                            "- RFC3339 datetime string",
                       time_description_prefix ? time_description_prefix : "the query");
        buffer_json_member_add_string(buffer, "description", buffer_tostring(desc));
        buffer_free(desc);
        
        if (!required) {
            buffer_json_member_add_int64(buffer, "default", MCP_DEFAULT_AFTER_TIME);
        }
    }
    buffer_json_object_close(buffer);
    
    // Before parameter
    buffer_json_member_add_object(buffer, "before");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "End time");
        
        BUFFER *desc = buffer_create(512, NULL);
        buffer_sprintf(desc, "End time for %s. Accepts:\n"
                            "- Unix timestamp in seconds\n"
                            "- Negative values for relative time\n"
                            "- RFC3339 datetime string",
                       time_description_prefix ? time_description_prefix : "the query");
        buffer_json_member_add_string(buffer, "description", buffer_tostring(desc));
        buffer_free(desc);
        
        if (!required) {
            buffer_json_member_add_int64(buffer, "default", MCP_DEFAULT_BEFORE_TIME);
        }
    }
    buffer_json_object_close(buffer);
}

// Add cardinality limit parameter to schema
void mcp_schema_add_cardinality_limit(
    BUFFER *buffer,
    const char *description,
    size_t default_value,
    size_t max_value
) {
    buffer_json_member_add_object(buffer, "cardinality_limit");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Maximum results");
        buffer_json_member_add_string(buffer, "description", 
            description ? description : "Maximum number of items to return");
        
        if (default_value > 0)
            buffer_json_member_add_uint64(buffer, "default", default_value);
        
        buffer_json_member_add_uint64(buffer, "minimum", 1);
        
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
    const char **error_message
) {
    struct json_object *obj = NULL;
    
    if (json_object_object_get_ex(params, param_name, &obj) && obj) {
        if (json_object_is_type(obj, json_type_int)) {
            int64_t value = json_object_get_int64(obj);
            if (value < 0) {
                if (error_message) {
                    static char error_buf[256];
                    snprintf(error_buf, sizeof(error_buf), 
                            "%s must be a positive number", param_name);
                    *error_message = error_buf;
                }
                return default_value;
            }
            size_t size_value = (size_t)value;
            if (size_value < min_value || (max_value > 0 && size_value > max_value)) {
                if (error_message) {
                    static char error_buf[256];
                    if (max_value > 0) {
                        snprintf(error_buf, sizeof(error_buf),
                                "%s must be between %zu and %zu",
                                param_name, min_value, max_value);
                    } else {
                        snprintf(error_buf, sizeof(error_buf),
                                "%s must be at least %zu",
                                param_name, min_value);
                    }
                    *error_message = error_buf;
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
    const char **error_message
) {
    struct json_object *obj = NULL;
    
    if (json_object_object_get_ex(params, param_name, &obj) && obj) {
        if (json_object_is_type(obj, json_type_int)) {
            int value = json_object_get_int(obj);
            if (value < min_seconds || (max_seconds > 0 && value > max_seconds)) {
                if (error_message) {
                    static char error_buf[256];
                    if (max_seconds > 0) {
                        snprintf(error_buf, sizeof(error_buf),
                                "%s must be between %d and %d seconds",
                                param_name, min_seconds, max_seconds);
                    } else {
                        snprintf(error_buf, sizeof(error_buf),
                                "%s must be at least %d seconds",
                                param_name, min_seconds);
                    }
                    *error_message = error_buf;
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
        
        if (!required)
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