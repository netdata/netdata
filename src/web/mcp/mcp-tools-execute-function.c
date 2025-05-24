// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-execute-function.h"
#include "database/contexts/rrdcontext.h"
#include "database/rrdfunctions.h"

// Define operator types for faster comparison
typedef enum {
    OP_EQUALS,           // ==
    OP_NOT_EQUALS,       // != or <>
    OP_LESS,             // <
    OP_LESS_EQUALS,      // <=
    OP_GREATER,          // >
    OP_GREATER_EQUALS,   // >=
    OP_MATCH,            // simple pattern
    OP_NOT_MATCH,        // not simple pattern
    OP_UNKNOWN           // unknown operator
} OPERATOR_TYPE;


// Maximum number of conditions we expect to handle
#define MAX_CONDITIONS 20
#define MAX_COLUMNS 300  // Maximum number of columns we can handle

// Result status for table processing
typedef enum {
    MCP_TABLE_OK,                                    // Success
    MCP_TABLE_ERROR_INVALID_CONDITIONS,              // Condition format/parsing error
    MCP_TABLE_ERROR_NO_MATCHES_WITH_MISSING_COLUMNS, // No matches, some columns not found
    MCP_TABLE_ERROR_NO_MATCHES,                      // No matches with valid columns
    MCP_TABLE_ERROR_INVALID_SORT_ORDER,              // Invalid sort order parameter
    MCP_TABLE_ERROR_COLUMNS_NOT_FOUND,               // Requested columns not found
    MCP_TABLE_ERROR_SORT_COLUMN_NOT_FOUND,           // Sort column not found
    MCP_TABLE_ERROR_TOO_MANY_COLUMNS,                // Exceeds MAX_COLUMNS
    MCP_TABLE_NOT_JSON,                              // Response is not valid JSON
    MCP_TABLE_NOT_PROCESSABLE,                       // JSON but not a processable table format
    MCP_TABLE_EMPTY_RESULT,                          // Function returned no rows
    MCP_TABLE_INFO_MISSING_COLUMNS_FOUND_RESULTS,    // Missing columns but found via wildcard
    MCP_TABLE_RESPONSE_TOO_BIG                        // Result too big, guidance added
} MCP_TABLE_RESULT_STATUS;

// Structure to hold table processing results
typedef struct {
    MCP_TABLE_RESULT_STATUS status;
    BUFFER *result;                    // The processed result or error details
    BUFFER *error_message;             // Detailed error message
    BUFFER *missing_columns;           // List of missing columns (comma-separated)
    size_t row_count;                  // Number of rows in result
    size_t column_count;               // Number of columns in result
    size_t result_size;                // Size of the result in bytes
    bool had_missing_columns;          // Whether any columns were missing
} MCP_TABLE_RESULT;

// Structure to hold preprocessed condition information
typedef struct condition_s {
    int column_index;                // Index of the column in the row
    char column_name[256];           // Name of the column (for error reporting) - fixed size array
    OPERATOR_TYPE op;                // Operator type
    struct json_object *value;       // Value to compare against (referenced, not owned - caller must keep alive)
    SIMPLE_PATTERN *pattern;         // Pre-compiled pattern for MATCH operations (owned - must be freed)
} CONDITION;

// Structure to hold an array of conditions
typedef struct {
    CONDITION items[MAX_CONDITIONS]; // Fixed array of conditions
    size_t count;                    // Number of conditions currently in use
} CONDITION_ARRAY;

// Helper function to create a filtered copy of a column definition
static struct json_object *create_filtered_column(struct json_object *col_obj) {
    struct json_object *col_copy = json_object_new_object();
    
    // Handle type conversion for better LLM understanding
    struct json_object *type_obj = NULL;
    if (json_object_object_get_ex(col_obj, "type", &type_obj) && 
        json_object_is_type(type_obj, json_type_string)) {
        const char *type_str = json_object_get_string(type_obj);
        RRDF_FIELD_TYPE field_type = RRDF_FIELD_TYPE_2id(type_str);
        
        // Replace the original type with a simplified scalar type for LLM
        json_object_object_add(col_copy, "type", 
                              json_object_new_string(field_type_to_json_scalar_type(field_type)));
    }
    
    // Copy only necessary properties
    struct json_object_iterator col_it = json_object_iter_begin(col_obj);
    struct json_object_iterator col_itEnd = json_object_iter_end(col_obj);
    
    while (!json_object_iter_equal(&col_it, &col_itEnd)) {
        const char *field_key = json_object_iter_peek_name(&col_it);
        struct json_object *field_val = json_object_iter_peek_value(&col_it);
        
        // Skip properties we don't need for LLM
        if (strcmp(field_key, "visible") != 0 &&
            strcmp(field_key, "visualization") != 0 &&
            strcmp(field_key, "value_options") != 0 &&
            strcmp(field_key, "sort") != 0 &&
            strcmp(field_key, "sortable") != 0 &&
            strcmp(field_key, "sticky") != 0 &&
            strcmp(field_key, "summary") != 0 &&
            strcmp(field_key, "filter") != 0 &&
            strcmp(field_key, "full_width") != 0 &&
            strcmp(field_key, "wrap") != 0 &&
            strcmp(field_key, "default_expanded_filter") != 0 &&
            strcmp(field_key, "unique_key") != 0 &&
            // Skip type as we've already handled it
            strcmp(field_key, "type") != 0) {
            json_object_object_add(col_copy, field_key, json_object_get(field_val));
        }
        
        json_object_iter_next(&col_it);
    }
    
    return col_copy;
}


// Structure to hold column type and transform information
typedef struct column_transform_info {
    RRDF_FIELD_TYPE type;
    RRDF_FIELD_TRANSFORM transform;
} COLUMN_TRANSFORM_INFO;

// Check if a column represents a timestamp
static inline bool is_transformable_timestamp(RRDF_FIELD_TYPE type, RRDF_FIELD_TRANSFORM transform) {
    return (type == RRDF_FIELD_TYPE_TIMESTAMP || 
            transform == RRDF_FIELD_TRANSFORM_DATETIME_MS ||
            transform == RRDF_FIELD_TRANSFORM_DATETIME_USEC);
}

// Check if a column represents a duration
static inline bool is_transformable_duration(RRDF_FIELD_TYPE type, RRDF_FIELD_TRANSFORM transform) {
    (void)transform; // unused for duration
    return (type == RRDF_FIELD_TYPE_DURATION);
}

// Check if a column type/transform combination should be transformed to string
static inline bool is_transformable_to_string(RRDF_FIELD_TYPE type, RRDF_FIELD_TRANSFORM transform) {
    return is_transformable_timestamp(type, transform) || is_transformable_duration(type, transform);
}

// Transform a value based on its type and transform settings
// Returns either a new object (caller owns) or NULL if no transformation needed
// IMPORTANT: When this returns a non-NULL value, it's a NEW object that the caller owns.
// When it returns NULL, the caller should use json_object_get() on the original value
// if they need to store it somewhere that takes ownership.
static struct json_object *transform_value_for_mcp(struct json_object *val, RRDF_FIELD_TYPE type, RRDF_FIELD_TRANSFORM transform) {
    if (!val || json_object_is_type(val, json_type_null))
        return NULL;
    
    // If it's not an integer, no transformation possible
    if (!json_object_is_type(val, json_type_int))
        return NULL;
    
    int64_t num_val = json_object_get_int64(val);
    
    if (is_transformable_timestamp(type, transform)) {
        // Convert to microseconds based on transform
        usec_t usec_val;
        if (transform == RRDF_FIELD_TRANSFORM_DATETIME_MS) {
            usec_val = (usec_t)num_val * USEC_PER_MS;
        } else if (transform == RRDF_FIELD_TRANSFORM_DATETIME_USEC) {
            usec_val = (usec_t)num_val;
        } else {
            // Default: seconds
            usec_val = (usec_t)num_val * USEC_PER_SEC;
        }
        
        // Format as RFC3339
        char datetime_buf[RFC3339_MAX_LENGTH];
        rfc3339_datetime_ut(datetime_buf, sizeof(datetime_buf), usec_val, 0, true);
        return json_object_new_string(datetime_buf);
    } else if (is_transformable_duration(type, transform)) {
        // Duration is always in seconds
        char duration_buf[256];
        duration_snprintf_time_t(duration_buf, sizeof(duration_buf), (time_t)num_val);
        return json_object_new_string(duration_buf);
    }
    
    // Value is 0 or negative, no transformation
    return NULL;
}

// Extract column transform information from column definitions
static void extract_column_transforms(struct json_object *columns_obj, 
                                    int *column_indices, 
                                    char **column_names,
                                    size_t selected_count,
                                    COLUMN_TRANSFORM_INFO *col_transforms) {
    for (size_t i = 0; i < selected_count; i++) {
        int col_idx = column_indices[i];
        const char *col_name = column_names[col_idx];
        
        struct json_object *col_obj = NULL;
        if (json_object_object_get_ex(columns_obj, col_name, &col_obj)) {
            // Get type
            struct json_object *type_obj = NULL;
            if (json_object_object_get_ex(col_obj, "type", &type_obj) && 
                json_object_is_type(type_obj, json_type_string)) {
                const char *type_str = json_object_get_string(type_obj);
                col_transforms[i].type = RRDF_FIELD_TYPE_2id(type_str);
            }
            
            // Get transform from value_options
            struct json_object *value_options_obj = NULL;
            if (json_object_object_get_ex(col_obj, "value_options", &value_options_obj) && 
                json_object_is_type(value_options_obj, json_type_object)) {
                // value_options is an object, extract the "transform" field
                struct json_object *transform_obj = NULL;
                if (json_object_object_get_ex(value_options_obj, "transform", &transform_obj) &&
                    json_object_is_type(transform_obj, json_type_string)) {
                    const char *transform_str = json_object_get_string(transform_obj);
                    col_transforms[i].transform = RRDF_FIELD_TRANSFORM_2id(transform_str);
                }
            }
        }
    }
}

// Convert string operator to enum type
static OPERATOR_TYPE string_to_operator(const char *op_str) {
    if (!op_str)
        return OP_UNKNOWN;
    
    if (strcmp(op_str, "==") == 0)
        return OP_EQUALS;
    
    if (strcmp(op_str, "!=") == 0 || strcmp(op_str, "<>") == 0)
        return OP_NOT_EQUALS;
    
    if (strcmp(op_str, "<") == 0)
        return OP_LESS;
    
    if (strcmp(op_str, "<=") == 0)
        return OP_LESS_EQUALS;
    
    if (strcmp(op_str, ">") == 0)
        return OP_GREATER;
    
    if (strcmp(op_str, ">=") == 0)
        return OP_GREATER_EQUALS;
    
    if (strcmp(op_str, "match") == 0 || strcmp(op_str, "like") == 0 || strcmp(op_str, "in") == 0)
        return OP_MATCH;
    
    if (strcmp(op_str, "not match") == 0 || strcmp(op_str, "not like") == 0 || strcmp(op_str, "not in") == 0)
        return OP_NOT_MATCH;
    
    return OP_UNKNOWN;
}

// Free patterns in the condition array
static void free_condition_patterns(CONDITION_ARRAY *condition_array) {
    if (!condition_array)
        return;
    
    for (size_t i = 0; i < condition_array->count; i++) {
        if (condition_array->items[i].pattern)
            simple_pattern_free(condition_array->items[i].pattern);
    }
}

// Check if a single value matches a condition
static bool value_matches_condition(struct json_object *value, const CONDITION *condition) {
    if (!value || !condition)
        return false;
    
    // Handle MATCH and NOT MATCH operators (pattern matching) - always convert to strings
    if (condition->op == OP_MATCH || condition->op == OP_NOT_MATCH) {
        if (condition->pattern) {
            const char *val_str = json_object_get_string(value);
            bool pattern_match = simple_pattern_matches(condition->pattern, val_str);
            return (condition->op == OP_MATCH) ? pattern_match : !pattern_match;
        }
        return false;
    }
    // Handle numeric comparisons - only if BOTH values are numeric
    else if ((json_object_is_type(value, json_type_int) || 
              json_object_is_type(value, json_type_double)) &&
             (json_object_is_type(condition->value, json_type_int) || 
              json_object_is_type(condition->value, json_type_double))) {
        
        double val_num = json_object_get_double(value);
        double cond_num = json_object_get_double(condition->value);
        
        switch (condition->op) {
            case OP_EQUALS:
                return (val_num == cond_num);
            case OP_NOT_EQUALS:
                return (val_num != cond_num);
            case OP_LESS:
                return (val_num < cond_num);
            case OP_LESS_EQUALS:
                return (val_num <= cond_num);
            case OP_GREATER:
                return (val_num > cond_num);
            case OP_GREATER_EQUALS:
                return (val_num >= cond_num);
            default:
                return false;
        }
    }
    // Boolean comparisons - convert to booleans if possible
    else if (json_object_is_type(value, json_type_boolean) || 
             json_object_is_type(condition->value, json_type_boolean)) {
        
        bool val_bool = json_object_get_boolean(value);
        bool cond_bool = json_object_get_boolean(condition->value);
        
        switch (condition->op) {
            case OP_EQUALS:
                return (val_bool == cond_bool);
            case OP_NOT_EQUALS:
                return (val_bool != cond_bool);
            default:
                return false;
        }
    }
    // String comparisons for everything else
    else {
        const char *val_str = json_object_get_string(value);
        const char *cond_str = json_object_get_string(condition->value);
        
        int cmp = strcmp(val_str, cond_str);
        
        switch (condition->op) {
            case OP_EQUALS:
                return (cmp == 0);
            case OP_NOT_EQUALS:
                return (cmp != 0);
            case OP_LESS:
                return (cmp < 0);
            case OP_LESS_EQUALS:
                return (cmp <= 0);
            case OP_GREATER:
                return (cmp > 0);
            case OP_GREATER_EQUALS:
                return (cmp >= 0);
            default:
                return false;
        }
    }
}

// Check if a row matches all the conditions
static bool row_matches_conditions(struct json_object *row, const CONDITION_ARRAY *conditions) {
    if (!conditions || conditions->count == 0)
        return true; // No conditions means everything matches
    
    for (size_t i = 0; i < conditions->count; i++) {
        const CONDITION *current = &conditions->items[i];
        bool condition_match = false;
        
        // Special case: column_index == -1 means search all columns
        if (current->column_index == -1) {
            // Search across all columns for a match
            size_t row_length = json_object_array_length(row);
            for (size_t col_idx = 0; col_idx < row_length; col_idx++) {
                struct json_object *row_val = json_object_array_get_idx(row, col_idx);
                if (!row_val) continue;
                
                // Check if this column value matches the condition
                if (value_matches_condition(row_val, current)) {
                    condition_match = true;
                    break; // Found a match, no need to check other columns
                }
            }
        }
        else {
            // Normal case: specific column index
            struct json_object *row_val = json_object_array_get_idx(row, current->column_index);
            
            // Handle null values
            if (!row_val) {
                return false;
            }
            
            condition_match = value_matches_condition(row_val, current);
        }
        
        // If any condition doesn't match, return false
        if (!condition_match) {
            return false;
        }
    }
    
    // All conditions matched
    return true;
}

// Process conditions array into an array of preprocessed conditions
// Returns:
//   > 0: Number of conditions successfully processed
//   = 0: No conditions provided
//   < 0: Error processing conditions (-1: generic error, -2: too many conditions, -3: invalid format,
//        -4: invalid element types, -5: column not found, -6: invalid operator)
// IMPORTANT: This function stores references to json_object values from conditions_array without
// incrementing their reference counts. The caller MUST keep conditions_array alive for as long
// as the returned CONDITION_ARRAY is used.
static int preprocess_conditions(struct json_object *conditions_array, 
                                struct json_object *columns_obj,
                                CONDITION_ARRAY *condition_array,
                                BUFFER *error_buffer,
                                bool *has_missing_columns) {
    if (!condition_array) {
        if (error_buffer)
            buffer_strcat(error_buffer, "Invalid condition array pointer");
        return -1; // Generic error
    }
    
    // Initialize the condition array
    memset(condition_array, 0, sizeof(CONDITION_ARRAY));
    
    // Initialize the missing columns flag
    if (has_missing_columns)
        *has_missing_columns = false;
    
    if (!conditions_array || !json_object_is_type(conditions_array, json_type_array))
        return 0; // No conditions is valid
    
    size_t conditions_count = json_object_array_length(conditions_array);
    if (conditions_count == 0)
        return 0; // Empty array is valid
    
    // Check if we have too many conditions
    if (conditions_count > MAX_CONDITIONS) {
        if (error_buffer)
            buffer_sprintf(error_buffer, "Too many conditions. Maximum is %d.", MAX_CONDITIONS);
        return -2; // Too many conditions
    }
    
    for (size_t i = 0; i < conditions_count; i++) {
        struct json_object *condition = json_object_array_get_idx(conditions_array, i);
        
        // Each condition should be an array of [column, operator, value]
        if (!condition || !json_object_is_type(condition, json_type_array) ||
            json_object_array_length(condition) != 3) {
            if (error_buffer)
                buffer_sprintf(error_buffer, "Invalid condition format at index %zu. Expected [column, operator, value]", i);
            free_condition_patterns(condition_array);
            return -3; // Invalid format
        }
        
        struct json_object *col_name_obj = json_object_array_get_idx(condition, 0);
        struct json_object *operator_obj = json_object_array_get_idx(condition, 1);
        struct json_object *value_obj = json_object_array_get_idx(condition, 2);
        
        if (!col_name_obj || !json_object_is_type(col_name_obj, json_type_string) ||
            !operator_obj || !json_object_is_type(operator_obj, json_type_string) ||
            !value_obj) {
            if (error_buffer)
                buffer_sprintf(error_buffer, "Invalid condition element types at index %zu. Expected [string, string, any]", i);
            free_condition_patterns(condition_array);
            return -4; // Invalid element types
        }
        
        // Get column name and operator
        const char *col_name = json_object_get_string(col_name_obj);
        const char *op_str = json_object_get_string(operator_obj);
        
        // Initialize condition values
        CONDITION *curr = &condition_array->items[condition_array->count];
        
        // Copy column name (with bounds checking)
        strncpy(curr->column_name, col_name, sizeof(curr->column_name) - 1);
        curr->column_name[sizeof(curr->column_name) - 1] = '\0'; // Ensure null termination
        
        curr->op = string_to_operator(op_str);
        if (curr->op == OP_UNKNOWN) {
            if (error_buffer)
                buffer_sprintf(error_buffer, "Invalid operator '%s' at index %zu. Valid operators are: ==, !=, <>, <, <=, >, >=, match, not match", 
                              op_str, i);
            free_condition_patterns(condition_array);
            return -6; // Invalid operator
        }
        
        curr->value = value_obj; // Store reference only - we don't own this, conditions_array must stay alive
        curr->pattern = NULL;
        
        // Find column in column definitions
        struct json_object *col_obj = NULL;
        if (!json_object_object_get_ex(columns_obj, col_name, &col_obj)) {
            // Column not found - mark it as a wildcard search (use -1 as special index)
            curr->column_index = -1;
            
            if (has_missing_columns)
                *has_missing_columns = true;
            
            if (error_buffer) {
                buffer_sprintf(error_buffer, "Column not found: '%s' at index %zu", col_name, i);
            }
            // Don't return error yet - we'll handle this specially
        } else {
            // Get column index
            struct json_object *index_obj = NULL;
            if (!json_object_object_get_ex(col_obj, "index", &index_obj)) {
                if (error_buffer) {
                    buffer_sprintf(error_buffer, "Column index not found for: '%s' at index %zu", col_name, i);
                }
                free_condition_patterns(condition_array);
                return -5; // Column not found (index missing)
            }
            
            curr->column_index = json_object_get_int(index_obj);
        }
        
        // Pre-compile patterns for MATCH operators
        if (curr->op == OP_MATCH || curr->op == OP_NOT_MATCH) {
            // Always convert to string for pattern matching, regardless of original type
            const char *pattern_str = json_object_get_string(value_obj);
            curr->pattern = string_to_simple_pattern_nocase_substring(pattern_str);
        }
        
        // Increment the count
        condition_array->count++;
    }
    
    return (int)condition_array->count; // Return the number of conditions processed
}



// Flags to indicate what additional content should be added for errors
typedef enum {
    MCP_TABLE_ADD_NOTHING = 0,
    MCP_TABLE_ADD_COLUMNS = 1 << 0,
    MCP_TABLE_ADD_RAW_DATA = 1 << 1,
    MCP_TABLE_ADD_FILTERING_INSTRUCTIONS = 1 << 2
} MCP_TABLE_ADDITIONAL_CONTENT;

// Helper to create a filtered columns object for error messages
static struct json_object *create_filtered_columns_for_errors(struct json_object *columns_obj) {
    if (!columns_obj) return NULL;
    
    struct json_object *filtered = json_object_new_object();
    struct json_object_iterator it = json_object_iter_begin(columns_obj);
    struct json_object_iterator itEnd = json_object_iter_end(columns_obj);
    
    while (!json_object_iter_equal(&it, &itEnd)) {
        const char *col_name = json_object_iter_peek_name(&it);
        struct json_object *col_obj = json_object_iter_peek_value(&it);
        
        if (col_obj) {
            struct json_object *filtered_col = create_filtered_column(col_obj);
            json_object_object_add(filtered, col_name, filtered_col);
        }
        
        json_object_iter_next(&it);
    }
    
    return filtered;
}

// Helper function to generate comprehensive error messages for LLMs
static MCP_TABLE_ADDITIONAL_CONTENT generate_table_error_message(MCP_TABLE_RESULT *result) {
    buffer_flush(result->error_message);
    MCP_TABLE_ADDITIONAL_CONTENT additional_content = MCP_TABLE_ADD_NOTHING;
    
    switch (result->status) {
        case MCP_TABLE_ERROR_INVALID_CONDITIONS:
            buffer_sprintf(result->error_message,
                "Error processing conditions: %s\n\n"
                "Conditions should be formatted as:\n"
                "```json\n"
                "\"conditions\": [\n"
                "    [\"column_name\", \"operator\", value],\n"
                "    [\"another_column\", \"another_operator\", another_value]\n"
                "]\n"
                "```",
                buffer_tostring(result->result)
            );
            additional_content = MCP_TABLE_ADD_FILTERING_INSTRUCTIONS;
            break;
            
        case MCP_TABLE_ERROR_NO_MATCHES_WITH_MISSING_COLUMNS:
            buffer_sprintf(result->error_message,
                "No rows matched the specified conditions.\n\n"
                "Note: The following column(s) were not found: %s\n"
                "A full-text search was performed across all columns, but no matches were found.",
                buffer_tostring(result->missing_columns)
            );
            additional_content = MCP_TABLE_ADD_COLUMNS | MCP_TABLE_ADD_FILTERING_INSTRUCTIONS;
            break;
            
        case MCP_TABLE_ERROR_NO_MATCHES:
            buffer_strcat(result->error_message,
                "No results match the specified conditions.\n\n"
                "Tips:\n"
                "• Verify the column names in your conditions\n"
                "• Check the values and operators used\n"
                "• For 'match' operators, ensure your pattern format is correct\n"
                "• To match multiple values, use 'match' with patterns separated by the pipe (|) character: '*value1*|*value2*'\n"
                "• Try broadening your filter criteria"
            );
            additional_content = MCP_TABLE_ADD_COLUMNS | MCP_TABLE_ADD_FILTERING_INSTRUCTIONS;
            break;
            
        case MCP_TABLE_ERROR_INVALID_SORT_ORDER:
            buffer_sprintf(result->error_message,
                "Invalid sort_order: '%s'. Valid options are 'asc' (ascending) or 'desc' (descending).\n\n"
                "Example:\n"
                "```json\n"
                "\"sort_order\": \"desc\"\n"
                "```",
                buffer_tostring(result->result)
            );
            additional_content = MCP_TABLE_ADD_NOTHING;
            break;
            
        case MCP_TABLE_ERROR_COLUMNS_NOT_FOUND:
            buffer_sprintf(result->error_message,
                "Column(s) not found: %s",
                buffer_tostring(result->missing_columns)
            );
            additional_content = MCP_TABLE_ADD_COLUMNS;
            break;
            
        case MCP_TABLE_ERROR_SORT_COLUMN_NOT_FOUND:
            buffer_sprintf(result->error_message,
                "Sort column '%s' not found.",
                buffer_tostring(result->result)
            );
            additional_content = MCP_TABLE_ADD_COLUMNS;
            break;
            
        case MCP_TABLE_ERROR_TOO_MANY_COLUMNS:
            buffer_sprintf(result->error_message,
                "Error: Table has %zu columns, which exceeds the maximum supported (%d). Showing raw output.",
                result->column_count, MAX_COLUMNS
            );
            additional_content = MCP_TABLE_ADD_RAW_DATA;
            break;
            
        case MCP_TABLE_NOT_JSON:
            buffer_strcat(result->error_message,
                "This response is not valid JSON. Showing raw output.");
            additional_content = MCP_TABLE_ADD_RAW_DATA;
            break;
            
        case MCP_TABLE_NOT_PROCESSABLE:
            buffer_strcat(result->error_message,
                "The function returned JSON but it's not a table format we can filter. Showing raw output.");
            additional_content = MCP_TABLE_ADD_RAW_DATA;
            break;
            
        case MCP_TABLE_EMPTY_RESULT:
            buffer_strcat(result->error_message,
                "The function returned an empty result (no rows).");
            additional_content = MCP_TABLE_ADD_RAW_DATA;
            break;
            
        case MCP_TABLE_INFO_MISSING_COLUMNS_FOUND_RESULTS:
            buffer_strcat(result->error_message,
                "Note: Not all columns in the conditions were found, so a full-text search was performed across all columns, and matching results were found.");
            additional_content = MCP_TABLE_ADD_NOTHING;
            break;
            
        case MCP_TABLE_RESPONSE_TOO_BIG:
            buffer_sprintf(result->error_message,
                "The response is too big (%zu bytes), having %zu rows and %zu columns. Limiting to 1 row for readability.",
                result->result_size, result->row_count, result->column_count
            );
            additional_content = MCP_TABLE_ADD_FILTERING_INSTRUCTIONS;
            break;
            
        default:
            additional_content = MCP_TABLE_ADD_NOTHING;
            break;
    }
    
    return additional_content;
}

// Helper to add filtering instructions as a separate content entry
static void add_filtering_instructions_to_mcp_result(MCP_CLIENT *mcpc) {
    buffer_json_add_array_item_object(mcpc->result);
    {
        buffer_json_member_add_string(mcpc->result, "type", "text");
        buffer_json_member_add_string(mcpc->result, "text",
            "FILTERING INSTRUCTIONS:\n"
            "• **columns**: Select specific columns to reduce width (e.g., [\"Column1\", \"Column2\", \"Column3\"])\n"
            "• **conditions**: Filter rows using [ [column1, operator1, value1], [column2, operator2, value2], ... ]\n"
            "• **limit**: Control number of rows returned (e.g., 10)\n"
            "• **sort_column** + **sort_order**: Order results by a column ('asc' or 'desc')\n"
            "\n"
            "Example filtering:\n"
            "```json\n"
            "{\n"
            "  \"columns\": [\"CmdLine\", \"CPU\", \"Memory\", \"Status\"],\n"
            "  \"conditions\": [\n"
            "    [\"Memory\", \">\", 1.0],\n"
            "    [\"CmdLine\", \"match\", \"*systemd*|*postgresql*|*docker*\"]\n"
            "  ],\n"
            "  \"sort_column\": \"CPU\",\n"
            "  \"sort_order\": \"desc\",\n"
            "  \"limit\": 10\n"
            "}\n"
            "```\n"
            "\n"
            "Operators: ==, !=, <, <=, >, >=, match (simple pattern), not match (simple pattern)\n"
            "Simple patterns: '*this*|*that*|*other*' (wildcard search to find strings that include 'this', or 'that', or 'other')"
        );
    }
    buffer_json_object_close(mcpc->result);
}

// Helper to add columns info as a separate content entry
static void add_columns_info_to_mcp_result(MCP_CLIENT *mcpc, struct json_object *columns_obj) {
    if (!columns_obj) return;
    
    struct json_object *filtered_columns = create_filtered_columns_for_errors(columns_obj);
    if (filtered_columns) {
        // Create wrapper object
        struct json_object *wrapper = json_object_new_object();
        json_object_object_add(wrapper, "available_columns", filtered_columns);
        
        const char *columns_json = json_object_to_json_string_ext(wrapper, JSON_C_TO_STRING_PRETTY);
        
        buffer_json_add_array_item_object(mcpc->result);
        {
            buffer_json_member_add_string(mcpc->result, "type", "text");
            buffer_json_member_add_string(mcpc->result, "text", columns_json);
        }
        buffer_json_object_close(mcpc->result);
        
        json_object_put(wrapper);
    }
}

// Helper to add messages to MCP result based on table result status
static void add_table_messages_to_mcp_result(MCP_CLIENT *mcpc, 
                                            MCP_TABLE_RESULT *table_result,
                                            struct json_object *columns_obj) {
    // Generate the appropriate error message and get additional content flags
    MCP_TABLE_ADDITIONAL_CONTENT additional_content = generate_table_error_message(table_result);
    
    // Add the message if there's an error or guidance
    if (table_result->status != MCP_TABLE_OK && buffer_strlen(table_result->error_message) > 0) {
        buffer_json_add_array_item_object(mcpc->result);
        {
            buffer_json_member_add_string(mcpc->result, "type", "text");
            buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(table_result->error_message));
        }
        buffer_json_object_close(mcpc->result);
    }
    
    // Add columns info if requested
    if ((additional_content & MCP_TABLE_ADD_COLUMNS) && columns_obj) {
        add_columns_info_to_mcp_result(mcpc, columns_obj);
    }
    
    // Add filtering instructions if requested
    if (additional_content & MCP_TABLE_ADD_FILTERING_INSTRUCTIONS) {
        add_filtering_instructions_to_mcp_result(mcpc);
    }
    
    // Add raw data if requested
    if ((additional_content & MCP_TABLE_ADD_RAW_DATA) && 
        buffer_strlen(table_result->result) > 0) {
        buffer_json_add_array_item_object(mcpc->result);
        {
            buffer_json_member_add_string(mcpc->result, "type", "text");
            buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(table_result->result));
        }
        buffer_json_object_close(mcpc->result);
    }
}

void mcp_tool_execute_function_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Execute a function on a specific node. Functions provide live information and they are automatically routed and executed to Netdata running on the given node.");

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    buffer_json_member_add_object(buffer, "node");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "The node on which to execute the function");
        buffer_json_member_add_string(buffer, "description", "The hostname or machine_guid or node_id of the node where the function should be executed. The node needs to be online (live) and reachable.");
    }
    buffer_json_object_close(buffer); // node

    buffer_json_member_add_object(buffer, "function");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "The name of the function to execute.");
        buffer_json_member_add_string(buffer, "description", "The function name, as available in the node_details tool output");
    }
    buffer_json_object_close(buffer); // function
    
    buffer_json_member_add_object(buffer, "timeout");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Execution timeout in seconds");
        buffer_json_member_add_string(buffer, "description", "Maximum time to wait for function execution (default: 60)");
        buffer_json_member_add_int64(buffer, "default", 60);
    }
    buffer_json_object_close(buffer); // timeout

    buffer_json_member_add_object(buffer, "columns");
    {
        buffer_json_member_add_string(buffer, "type", "array");
        buffer_json_member_add_string(buffer, "title", "Columns to include");
        buffer_json_member_add_string(buffer, "description", "Array of column names to include in the result. Each function has its own columns, so first check the function without this parameter.");
        
        buffer_json_member_add_object(buffer, "items");
        {
            buffer_json_member_add_string(buffer, "type", "string");
        }
        buffer_json_object_close(buffer); // items
    }
    buffer_json_object_close(buffer); // columns

    buffer_json_member_add_object(buffer, "sort_column");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Column to sort by");
        buffer_json_member_add_string(buffer, "description", "Name of the column to sort the results by.");
    }
    buffer_json_object_close(buffer); // sort_column

    buffer_json_member_add_object(buffer, "sort_order");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Sort order");
        buffer_json_member_add_string(buffer, "description", "Order to sort results: 'asc' for ascending, 'desc' for descending");
        buffer_json_member_add_string(buffer, "default", "asc");
        buffer_json_member_add_array(buffer, "enum");
        buffer_json_add_array_item_string(buffer, "asc");
        buffer_json_add_array_item_string(buffer, "desc");
        buffer_json_array_close(buffer);
    }
    buffer_json_object_close(buffer); // sort_order

    buffer_json_member_add_object(buffer, "limit");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Row limit");
        buffer_json_member_add_string(buffer, "description", "Maximum number of rows to return");
    }
    buffer_json_object_close(buffer); // limit
    
    buffer_json_member_add_object(buffer, "conditions");
    {
        buffer_json_member_add_string(buffer, "type", "array");
        buffer_json_member_add_string(buffer, "title", "Filter conditions");
        buffer_json_member_add_string(buffer, "description", "Array of conditions to filter rows. Each condition is an array of [column, operator, value] where operator can be ==, !=, <>, <, <=, >, >=, match, not match");
        
        buffer_json_member_add_object(buffer, "items");
        {
            buffer_json_member_add_string(buffer, "type", "array");
            buffer_json_member_add_object(buffer, "items");
            {
                buffer_json_member_add_array(buffer, "oneOf");
                
                // First item of the condition array - column name
                buffer_json_add_array_item_object(buffer);
                {
                    buffer_json_member_add_string(buffer, "type", "string");
                }
                buffer_json_object_close(buffer);
                
                // Second item - operator
                buffer_json_add_array_item_object(buffer);
                {
                    buffer_json_member_add_string(buffer, "type", "string");
                    buffer_json_member_add_array(buffer, "enum");
                    buffer_json_add_array_item_string(buffer, "==");
                    buffer_json_add_array_item_string(buffer, "!=");
                    buffer_json_add_array_item_string(buffer, "<>");
                    buffer_json_add_array_item_string(buffer, "<");
                    buffer_json_add_array_item_string(buffer, "<=");
                    buffer_json_add_array_item_string(buffer, ">");
                    buffer_json_add_array_item_string(buffer, ">=");
                    buffer_json_add_array_item_string(buffer, "match");
                    buffer_json_add_array_item_string(buffer, "not match");
                    buffer_json_array_close(buffer);
                }
                buffer_json_object_close(buffer);
                
                // Third item - value (can be string, number, or boolean)
                buffer_json_add_array_item_object(buffer);
                {
                    buffer_json_member_add_array(buffer, "type");
                    buffer_json_add_array_item_string(buffer, "string");
                    buffer_json_add_array_item_string(buffer, "number");
                    buffer_json_add_array_item_string(buffer, "boolean");
                    buffer_json_array_close(buffer);
                }
                buffer_json_object_close(buffer);
                
                buffer_json_array_close(buffer); // oneOf
            }
            buffer_json_object_close(buffer); // inner items
        }
        buffer_json_object_close(buffer); // items
    }
    buffer_json_object_close(buffer); // conditions

    buffer_json_object_close(buffer); // properties

    // Required fields
    buffer_json_member_add_array(buffer, "required");
    buffer_json_add_array_item_string(buffer, "node");
    buffer_json_add_array_item_string(buffer, "function");
    buffer_json_array_close(buffer); // required

    buffer_json_object_close(buffer); // inputSchema
}

/**
 * Filter and sort a table-formatted JSON result based on parameters
 * 
 * @param result_buffer The original JSON result buffer
 * @param columns_array JSON array of column names to include (or NULL for all)
 * @param sort_column_param Column name to sort by (or NULL for no sorting)
 * @param sort_order_param "asc" or "desc" for sort direction (defaults to "asc" if NULL)
 * @param limit_param Maximum number of rows to return (-1 for all)
 * @param conditions_array JSON array of condition arrays [column, op, value] (or NULL for no conditions)
 * @param max_size_threshold Maximum size in bytes before truncation is recommended
 * @param table_result Output structure containing the result and status
 * 
 * @return void (result is returned in table_result structure)
 */
static void mcp_process_table_result(BUFFER *result_buffer, struct json_object *columns_array, 
                                    const char *sort_column_param, const char *sort_order_param,
                                    int limit_param, struct json_object *conditions_array,
                                    size_t max_size_threshold, MCP_TABLE_RESULT *table_result)
{
    // Initialize result structure
    memset(table_result, 0, sizeof(MCP_TABLE_RESULT));
    table_result->result = buffer_create(0, NULL);
    table_result->error_message = buffer_create(0, NULL);
    table_result->missing_columns = buffer_create(0, NULL);
    table_result->status = MCP_TABLE_OK;
    
    struct json_object *json_result = NULL;

    // Parse the JSON result
    const char *json_str = buffer_tostring(result_buffer);
    size_t result_size = buffer_strlen(result_buffer);
    json_result = json_tokener_parse(json_str);

    if (!json_result) {
        buffer_strcat(table_result->result, json_str); // Return original if parsing fails
        return;
    }

    // Check if it's a table format
    struct json_object *type_obj = NULL;
    struct json_object *has_history_obj = NULL;

    if (!json_object_object_get_ex(json_result, "type", &type_obj) ||
        !json_object_object_get_ex(json_result, "has_history", &has_history_obj)) {
        buffer_strcat(table_result->result, json_str); // Not in correct format
        json_object_put(json_result);
        return;
    }

    const char *type = json_object_get_string(type_obj);
    bool has_history = json_object_get_boolean(has_history_obj);

    if (!type || strcmp(type, "table") != 0 || has_history) {
        buffer_strcat(table_result->result, json_str); // Not a table format
        json_object_put(json_result);
        return;
    }

    // Get data and columns
    struct json_object *data_obj = NULL;
    struct json_object *columns_obj = NULL;

    if (!json_object_object_get_ex(json_result, "data", &data_obj) ||
        !json_object_object_get_ex(json_result, "columns", &columns_obj)) {
        buffer_strcat(table_result->result, json_str); // Missing required elements
        json_object_put(json_result);
        return;
    }

    size_t row_count = json_object_array_length(data_obj);
    size_t column_count = json_object_object_length(columns_obj);
    
    // Store row and column counts
    table_result->row_count = row_count;
    table_result->column_count = column_count;

    // Check if we need to show guidance - only when no filtering is specified
    if (result_size > max_size_threshold && !columns_array && !sort_column_param && limit_param < 0 &&
        !conditions_array) {
        // Store first row in result for guidance
        table_result->status = MCP_TABLE_RESPONSE_TOO_BIG;
        limit_param = 1;
    }

    // Even with no filtering parameters, we need to process to remove unwanted fields

    // Process filtering parameters
    // Determine sort direction
    bool descending = false;
    if (sort_order_param) {
        if (strcasecmp(sort_order_param, "desc") == 0) {
            descending = true;
        }
        else if (strcasecmp(sort_order_param, "asc") == 0) {
            descending = false;
        }
        else {
            // Invalid sort order
            table_result->status = MCP_TABLE_ERROR_INVALID_SORT_ORDER;
            buffer_strcat(table_result->result, sort_order_param);
            json_object_put(json_result);
            return;
        }
    }

    // Use fixed-size arrays for column selection
    uint8_t column_selected[MAX_COLUMNS] = {0};  // 1 if column is selected, 0 if not
    char *column_names[MAX_COLUMNS] = {0};       // Names of selected columns (references)
    int column_indices[MAX_COLUMNS] = {0};       // Index mapping for ordered access
    size_t selected_count = 0;
    
    // Check if we have too many columns
    if (column_count > MAX_COLUMNS) {
        table_result->status = MCP_TABLE_ERROR_TOO_MANY_COLUMNS;
        table_result->column_count = column_count;
        json_object_put(json_result);
        return;
    }
    
    if (columns_array && json_object_is_type(columns_array, json_type_array)) {
        // Get column names from JSON array
        size_t count = json_object_array_length(columns_array);
        bool all_columns_found = true;
        CLEAN_BUFFER *missing_columns = buffer_create(0, NULL);
        
        for (size_t i = 0; i < count; i++) {
            struct json_object *col_name_obj = json_object_array_get_idx(columns_array, i);
            if (col_name_obj && json_object_is_type(col_name_obj, json_type_string)) {
                const char *col = json_object_get_string(col_name_obj);
                
                // Find column and add to selected set
                struct json_object *col_obj = NULL;
                if (json_object_object_get_ex(columns_obj, col, &col_obj)) {
                    // Get index of this column
                    struct json_object *index_obj = NULL;
                    if (json_object_object_get_ex(col_obj, "index", &index_obj)) {
                        int idx = json_object_get_int(index_obj);
                        if (idx >= 0 && idx < MAX_COLUMNS) {
                            column_selected[idx] = 1;
                            column_names[idx] = (char*)col; // Store reference only - columns_obj must stay alive
                        }
                    }
                } else {
                    // Column not found
                    all_columns_found = false;
                    if (buffer_strlen(missing_columns) > 0) {
                        buffer_strcat(missing_columns, ", ");
                    }
                    buffer_strcat(missing_columns, col);
                }
            }
        }
        
        // If any columns were not found, set error status
        if (!all_columns_found) {
            table_result->status = MCP_TABLE_ERROR_COLUMNS_NOT_FOUND;
            buffer_strcat(table_result->missing_columns, buffer_tostring(missing_columns));
            json_object_put(json_result);
            return;
        }
    } else {
        // If no columns specified, include all
        struct json_object_iterator it = json_object_iter_begin(columns_obj);
        struct json_object_iterator itEnd = json_object_iter_end(columns_obj);
        
        while (!json_object_iter_equal(&it, &itEnd)) {
            const char *col_key = json_object_iter_peek_name(&it);
            struct json_object *col_val = json_object_iter_peek_value(&it);
            
            struct json_object *index_obj = NULL;
            if (json_object_object_get_ex(col_val, "index", &index_obj)) {
                int idx = json_object_get_int(index_obj);
                if (idx >= 0 && idx < MAX_COLUMNS) {
                    column_selected[idx] = 1;
                    column_names[idx] = (char*)col_key; // Store reference only - columns_obj must stay alive
                }
            }
            
            json_object_iter_next(&it);
        }
    }
    
    // Create ordered index mapping for selected columns
    for (int i = 0; i < MAX_COLUMNS; i++) {
        if (column_selected[i]) {
            column_indices[selected_count] = i;
            selected_count++;
        }
    }

    // Find sort column index if sort requested
    int sort_idx = -1;

    if (sort_column_param) {
        struct json_object *sort_col_obj = NULL;
        if (json_object_object_get_ex(columns_obj, sort_column_param, &sort_col_obj)) {
            struct json_object *index_obj = NULL;
            if (json_object_object_get_ex(sort_col_obj, "index", &index_obj)) {
                sort_idx = json_object_get_int(index_obj);
            }
        } else {
            // Column not found
            table_result->status = MCP_TABLE_ERROR_SORT_COLUMN_NOT_FOUND;
            buffer_strcat(table_result->result, sort_column_param);
            json_object_put(json_result);
            return;
        }
    }

    // Create new arrays for filtered/sorted data
    struct json_object **rows = (struct json_object **)callocz(row_count, sizeof(struct json_object *));
    size_t row_idx = 0;

    // Preprocess conditions once for the whole request - using stack-based array
    CONDITION_ARRAY conditions; // Stack-allocated array of conditions
    int conditions_result = 0;
    bool has_missing_columns = false;
    
    if (conditions_array && json_object_is_type(conditions_array, json_type_array)) {
        CLEAN_BUFFER *error_buffer = buffer_create(0, NULL);
        conditions_result = preprocess_conditions(conditions_array, columns_obj, &conditions, error_buffer, &has_missing_columns);
        
        // Check if preprocessing failed (negative return value means error)
        if (conditions_result < 0 && conditions_result != -5) {  // -5 is column not found, which we now handle
            table_result->status = MCP_TABLE_ERROR_INVALID_CONDITIONS;
            buffer_strcat(table_result->result, buffer_tostring(error_buffer));
            
            freez((void *)rows);
            json_object_put(json_result);
            return;
        }
    }

    // Copy rows for filtering/sorting, applying filters if specified
    for (size_t i = 0; i < row_count; i++) {
        struct json_object *row = json_object_array_get_idx(data_obj, i);
        if (!row)
            continue;

        bool include_row = true;

        // Apply preprocessed conditions if they exist
        if (conditions_result > 0) {
            include_row = row_matches_conditions(row, &conditions);
        }

        if (include_row) {
            rows[row_idx++] = row;
        }
    }
    
    // If we have missing columns and found no matches, provide helpful error
    if (has_missing_columns && row_idx == 0) {
        table_result->status = MCP_TABLE_ERROR_NO_MATCHES_WITH_MISSING_COLUMNS;
        table_result->had_missing_columns = true;
        
        // Build missing columns list
        bool first = true;
        for (size_t i = 0; i < conditions.count; i++) {
            if (conditions.items[i].column_index == -1) {
                if (!first) buffer_strcat(table_result->missing_columns, ", ");
                buffer_sprintf(table_result->missing_columns, "\"%s\"", conditions.items[i].column_name);
                first = false;
            }
        }
        
        // Clean up and return
        if (conditions_result > 0) {
            free_condition_patterns(&conditions);
        }
        freez((void *)rows);
        json_object_put(json_result);
        return;
    }

    // Free pattern resources when we're done with all rows
    if (conditions_result > 0) {
        free_condition_patterns(&conditions);
    }

    // Sort if requested
    if (sort_idx >= 0) {
        // Simple bubble sort implementation
        for (size_t i = 0; i < row_idx; i++) {
            for (size_t j = i + 1; j < row_idx; j++) {
                bool should_swap = false;

                struct json_object *val_i = json_object_array_get_idx(rows[i], sort_idx);
                struct json_object *val_j = json_object_array_get_idx(rows[j], sort_idx);

                // Handle null values
                if (!val_i && !val_j) {
                    should_swap = false;
                } else if (!val_i) {
                    should_swap = !descending;
                } else if (!val_j) {
                    should_swap = descending;
                } else {
                    // Try to match based on apparent type
                    // Check if either value is a number type
                    if (json_object_is_type(val_i, json_type_int) || 
                        json_object_is_type(val_i, json_type_double) ||
                        json_object_is_type(val_j, json_type_int) || 
                        json_object_is_type(val_j, json_type_double)) {
                        
                        // Let json-c do the type conversion
                        double i_val = json_object_get_double(val_i);
                        double j_val = json_object_get_double(val_j);
                        should_swap = descending ? (i_val < j_val) : (i_val > j_val);
                        
                    } else if (json_object_is_type(val_i, json_type_boolean) || 
                               json_object_is_type(val_j, json_type_boolean)) {
                        
                        // Let json-c do the type conversion
                        bool i_val = json_object_get_boolean(val_i);
                        bool j_val = json_object_get_boolean(val_j);
                        should_swap = descending ? (i_val && !j_val) : (!i_val && j_val);
                        
                    } else {
                        // Default to string comparison for everything else
                        int cmp = strcmp(json_object_get_string(val_i), json_object_get_string(val_j));
                        should_swap = descending ? (cmp < 0) : (cmp > 0);
                    }
                }

                if (should_swap) {
                    struct json_object *temp = rows[i];
                    rows[i] = rows[j];
                    rows[j] = temp;
                }
            }
        }
    }

    // Apply row limit
    size_t limit = row_idx;
    if (limit_param >= 0 && (size_t)limit_param < limit) {
        limit = (size_t)limit_param;
    }

    // Create new filtered result
    struct json_object *filtered_result = json_object_new_object();
    struct json_object *filtered_data = json_object_new_array();
    struct json_object *filtered_columns = json_object_new_object();

    // Copy only specific metadata fields from original
    {
        // Keep only status, type, update_every, has_history + data and columns
        const char *keep_fields[] = {"status", "type", "update_every", "has_history"};
        size_t keep_count = sizeof(keep_fields) / sizeof(keep_fields[0]);

        for (size_t i = 0; i < keep_count; i++) {
            struct json_object *field_obj = NULL;

            if (json_object_object_get_ex(json_result, keep_fields[i], &field_obj)) {
                json_object_object_add(filtered_result, keep_fields[i], json_object_get(field_obj));
            }
        }
    }

    // Extract column transform information BEFORE creating filtered columns
    COLUMN_TRANSFORM_INFO col_transforms[MAX_COLUMNS] = {{0}};
    extract_column_transforms(columns_obj, column_indices, column_names, selected_count, col_transforms);

    // Create filtered data rows with transformation
    for (size_t i = 0; i < limit; i++) {
        struct json_object *row = rows[i];
        struct json_object *new_row = json_object_new_array();

        // Extract only selected columns
        for (size_t j = 0; j < selected_count; j++) {
            int col_idx = column_indices[j];
            struct json_object *val = json_object_array_get_idx(row, col_idx);
            
            // Try to transform the value
            struct json_object *transformed = transform_value_for_mcp(val, col_transforms[j].type, col_transforms[j].transform);
            
            if (transformed) {
                // Use the transformed value (we own it)
                json_object_array_add(new_row, transformed);
            } else if (val) {
                // No transformation needed, use original with ref count increase
                // json_object_array_add takes ownership, so we must increment ref count
                json_object_array_add(new_row, json_object_get(val));
            } else {
                // NULL value
                json_object_array_add(new_row, NULL);
            }
        }

        json_object_array_add(filtered_data, new_row);
    }

    // Create filtered column definitions AFTER processing data
    for (size_t i = 0; i < selected_count; i++) {
        int col_idx = column_indices[i];
        const char *col_name = column_names[col_idx];
        
        struct json_object *col_obj = NULL;
        if (json_object_object_get_ex(columns_obj, col_name, &col_obj)) {
            struct json_object *col_copy = create_filtered_column(col_obj);
            
            // Update index to match new position
            json_object_object_add(col_copy, "index", json_object_new_int((int)i));
            
            // Check if this column was transformed to string
            if (is_transformable_to_string(col_transforms[i].type, col_transforms[i].transform)) {
                // Override type to string since we transformed the value
                json_object_object_del(col_copy, "type");
                json_object_object_add(col_copy, "type", json_object_new_string("string"));
                
                // Remove numeric-specific properties that don't make sense for strings
                json_object_object_del(col_copy, "max");
                json_object_object_del(col_copy, "min");
                json_object_object_del(col_copy, "units");
            }
            
            json_object_object_add(filtered_columns, col_name, col_copy);
        }
    }

    // Add filtered data and columns to result
    json_object_object_add(filtered_result, "data", filtered_data);
    json_object_object_add(filtered_result, "columns", filtered_columns);

    // Check if we found any rows
    if (limit == 0 && conditions_array) {
        // No rows matched the conditions
        json_object_put(filtered_result);
        table_result->status = MCP_TABLE_ERROR_NO_MATCHES;
    } else {
        // Set flag if we used wildcard search and found results
        if (has_missing_columns && row_idx > 0) {
            table_result->had_missing_columns = true;
        }
        
        // Convert to string and store result
        const char *filtered_json = json_object_to_json_string_ext(filtered_result, JSON_C_TO_STRING_PRETTY);
        buffer_strcat(table_result->result, filtered_json);

        // Update actual counts from filtered result
        table_result->row_count = limit;
        table_result->column_count = selected_count;
        
        // Free the filtered result
        json_object_put(filtered_result);
    }

    // Clean up
    freez((void *)rows);
    json_object_put(json_result);
}

MCP_RETURN_CODE mcp_tool_execute_function_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id)
{
    if (!mcpc || id == 0 || !params)
        return MCP_RC_ERROR;

    // Extract required parameters
    const char *node_name = NULL;
    if (json_object_object_get_ex(params, "node", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "node", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            node_name = json_object_get_string(obj);
        }
    }

    if (!node_name || !*node_name) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'node'");
        return MCP_RC_BAD_REQUEST;
    }

    const char *function_name = NULL;
    if (json_object_object_get_ex(params, "function", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "function", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            function_name = json_object_get_string(obj);
        }
    }

    if (!function_name || !*function_name) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'function'");
        return MCP_RC_BAD_REQUEST;
    }

    int timeout = 60; // Default timeout 60 seconds
    if (json_object_object_get_ex(params, "timeout", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "timeout", &obj);
        if (obj && json_object_is_type(obj, json_type_int)) {
            timeout = json_object_get_int(obj);
            if (timeout <= 0)
                timeout = 60;
        }
    }

    // Find the host by hostname first
    RRDHOST *host = rrdhost_find_by_hostname(node_name);
    
    // If not found by hostname, try by GUID (node ID)
    if (!host) {
        host = rrdhost_find_by_guid(node_name);
        if (!host) {
            host = rrdhost_find_by_node_id(node_name);
        }
    }

    if (!host) {
        buffer_sprintf(mcpc->error, "Node not found: %s", node_name);
        return MCP_RC_NOT_FOUND;
    }

    // Create a buffer for function result
    CLEAN_BUFFER *result_buffer = buffer_create(0, NULL);
    BUFFER *processed_result = NULL;  // Not CLEAN_BUFFER as it will be cleaned up explicitly
    
    // Create a unique transaction ID
    char transaction[UUID_STR_LEN];
    nd_uuid_t transaction_uuid;
    uuid_generate(transaction_uuid);
    uuid_unparse_lower(transaction_uuid, transaction);

#if 1
    mcpc->user_auth->access = HTTP_ACCESS_ALL;
    mcpc->user_auth->method = USER_AUTH_METHOD_CLOUD;
    mcpc->user_auth->user_role = HTTP_USER_ROLE_ADMIN;
#endif

    // Create source buffer from user_auth
    CLEAN_BUFFER *source = buffer_create(0, NULL);
    user_auth_to_source_buffer(mcpc->user_auth, source);
    buffer_strcat(source, ",modelcontextprotocol");

    // Execute the function with proper permissions from the client
    int ret = rrd_function_run(
        host,
        result_buffer,
        timeout,
        mcpc->user_auth->access,
        function_name,
        true,
        transaction,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        buffer_tostring(source),
        false
    );

    if (ret != HTTP_RESP_OK) {
        buffer_sprintf(mcpc->error,
                       "Failed to execute function '%s' on node '%s', "
                       "http error code %d (%s):\n"
                       "```json\n%s\n```",
                       function_name, node_name, ret,
                       http_response_code2string(ret),
                       buffer_tostring(result_buffer));

        return MCP_RC_ERROR;
    }

    // Extract filtering parameters
    const char *sort_column_param = NULL;
    const char *sort_order_param = NULL;
    int limit_param = -1;
    struct json_object *conditions_array = NULL;
    
    // Get columns parameter as JSON array
    struct json_object *columns_array = NULL;
    if (json_object_object_get_ex(params, "columns", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "columns", &obj);
        
        if (obj && json_object_is_type(obj, json_type_array)) {
            // Only set if array has elements
            if (json_object_array_length(obj) > 0) {
                columns_array = obj;
            }
        }
    }
    
    // Get sort_column parameter
    if (json_object_object_get_ex(params, "sort_column", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "sort_column", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            const char *value = json_object_get_string(obj);
            // Only set if not empty
            if (value && *value) {
                sort_column_param = value;
            }
        }
    }
    
    // Get sort_order parameter
    if (json_object_object_get_ex(params, "sort_order", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "sort_order", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            sort_order_param = json_object_get_string(obj);
        }
    }
    
    // Get limit parameter
    if (json_object_object_get_ex(params, "limit", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "limit", &obj);
        if (obj && json_object_is_type(obj, json_type_int)) {
            int limit = json_object_get_int(obj);
            // Only set if positive
            if (limit > 0) {
                limit_param = limit;
            }
        }
    }
    
    // Get conditions parameter (array of [column, op, value] arrays)
    if (json_object_object_get_ex(params, "conditions", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "conditions", &obj);
        if (obj && json_object_is_type(obj, json_type_array)) {
            // Only set if array has elements
            if (json_object_array_length(obj) > 0) {
                conditions_array = obj;
            }
        }
    }
    
    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Start building content array for the result
    buffer_json_member_add_array(mcpc->result, "content");
    
    // Parse the original result to determine how to handle it
    struct json_object *json_check = json_tokener_parse(buffer_tostring(result_buffer));
    
    if (!json_check) {
        // Not valid JSON - return raw output with message
        MCP_TABLE_RESULT result = {0};
        result.status = MCP_TABLE_NOT_JSON;
        result.result = buffer_dup(result_buffer);
        result.error_message = buffer_create(0, NULL);
        result.missing_columns = buffer_create(0, NULL);
        
        add_table_messages_to_mcp_result(mcpc, &result, NULL);
        
        buffer_free(result.result);
        buffer_free(result.error_message);
        buffer_free(result.missing_columns);
    }
    else {
        // We have valid JSON - check if it's processable
        struct json_object *type_obj = NULL;
        struct json_object *has_history_obj = NULL;
        struct json_object *status_obj = NULL;
        struct json_object *data_obj = NULL;
        struct json_object *columns_obj = NULL;
        
        bool is_processable = false;
        if (json_object_object_get_ex(json_check, "type", &type_obj) &&
            json_object_object_get_ex(json_check, "has_history", &has_history_obj)) {
            
            const char *type = json_object_get_string(type_obj);
            bool has_history = json_object_get_boolean(has_history_obj);
            
            // Check status if exists
            int status = 200; // default
            if (json_object_object_get_ex(json_check, "status", &status_obj)) {
                status = json_object_get_int(status_obj);
            }
            
            is_processable = (type && strcmp(type, "table") == 0 && !has_history && status == 200);
        }
        
        if (!is_processable) {
            // Not a processable table format
            MCP_TABLE_RESULT result = {0};
            result.status = MCP_TABLE_NOT_PROCESSABLE;
            result.result = buffer_dup(result_buffer);
            result.error_message = buffer_create(0, NULL);
            result.missing_columns = buffer_create(0, NULL);
            
            add_table_messages_to_mcp_result(mcpc, &result, NULL);
            
            buffer_free(result.result);
            buffer_free(result.error_message);
            buffer_free(result.missing_columns);
        }
        else {
            // It's a processable table - get data and columns
            if (!json_object_object_get_ex(json_check, "data", &data_obj) ||
                !json_object_object_get_ex(json_check, "columns", &columns_obj)) {
                // Missing required fields, treat as not processable
                MCP_TABLE_RESULT result = {0};
                result.status = MCP_TABLE_NOT_PROCESSABLE;
                result.result = buffer_dup(result_buffer);
                result.error_message = buffer_create(0, NULL);
                result.missing_columns = buffer_create(0, NULL);
                
                add_table_messages_to_mcp_result(mcpc, &result, NULL);
                
                buffer_free(result.result);
                buffer_free(result.error_message);
                buffer_free(result.missing_columns);
            }
            else {
                // Check if data is empty
                size_t original_row_count = json_object_array_length(data_obj);
                
                if (original_row_count == 0) {
                    // Empty result
                    MCP_TABLE_RESULT result = {0};
                    result.status = MCP_TABLE_EMPTY_RESULT;
                    result.result = buffer_dup(result_buffer);
                    result.error_message = buffer_create(0, NULL);
                    result.missing_columns = buffer_create(0, NULL);
                    
                    add_table_messages_to_mcp_result(mcpc, &result, NULL);
                    
                    buffer_free(result.result);
                    buffer_free(result.error_message);
                    buffer_free(result.missing_columns);
                }
                else {
                    // We have data - process with user parameters
                    const size_t max_size_threshold = 20UL * 1024;
                    MCP_TABLE_RESULT first_result = {0};
                    
                    // First pass to check size and conditions
                    mcp_process_table_result(
                        result_buffer, 
                        columns_array, 
                        sort_column_param,
                        sort_order_param,
                        limit_param,
                        conditions_array,
                        SIZE_MAX, // Don't trigger guidance in first pass
                        &first_result
                    );
                    
                    // Check for errors
                    if (first_result.status != MCP_TABLE_OK && 
                        first_result.status != MCP_TABLE_RESPONSE_TOO_BIG) {
                        // Handle errors
                        add_table_messages_to_mcp_result(mcpc, &first_result, columns_obj);
                        
                        // Cleanup
                        buffer_free(first_result.result);
                        buffer_free(first_result.error_message);
                        buffer_free(first_result.missing_columns);
                    }
                    else {
                        // Check if we need to add any informational messages
                        
                        // Missing columns but found results
                        if (first_result.had_missing_columns && first_result.row_count > 0) {
                            MCP_TABLE_RESULT info_result = {0};
                            info_result.status = MCP_TABLE_INFO_MISSING_COLUMNS_FOUND_RESULTS;
                            info_result.result = buffer_create(0, NULL);
                            info_result.error_message = buffer_create(0, NULL);
                            info_result.missing_columns = buffer_create(0, NULL);
                            
                            add_table_messages_to_mcp_result(mcpc, &info_result, columns_obj);
                            
                            buffer_free(info_result.result);
                            buffer_free(info_result.error_message);
                            buffer_free(info_result.missing_columns);
                        }
                        
                        // Check if response is too big
                        size_t processed_size = buffer_strlen(first_result.result);
                        bool need_reprocess = false;
                        int final_limit = limit_param;
                        
                        if (processed_size > max_size_threshold && first_result.row_count > 1) {
                            need_reprocess = true;
                            final_limit = 1;
                            
                            // Create a result structure for the guidance message
                            MCP_TABLE_RESULT guidance_result = {0};
                            guidance_result.status = MCP_TABLE_RESPONSE_TOO_BIG;
                            guidance_result.result = buffer_create(0, NULL);
                            guidance_result.error_message = buffer_create(0, NULL);
                            guidance_result.missing_columns = buffer_create(0, NULL);
                            guidance_result.row_count = first_result.row_count;
                            guidance_result.column_count = first_result.column_count;
                            guidance_result.result_size = processed_size;
                            
                            // Use centralized message generation
                            add_table_messages_to_mcp_result(mcpc, &guidance_result, columns_obj);
                            
                            // Cleanup
                            buffer_free(guidance_result.result);
                            buffer_free(guidance_result.error_message);
                            buffer_free(guidance_result.missing_columns);
                        }
                        
                        // Get final result (reprocess if needed)
                        if (need_reprocess) {
                            MCP_TABLE_RESULT final_result = {0};
                            mcp_process_table_result(
                                result_buffer, 
                                columns_array, 
                                sort_column_param,
                                sort_order_param,
                                final_limit,
                                conditions_array,
                                max_size_threshold,
                                &final_result
                            );
                            
                            // Use final result
                            processed_result = final_result.result;
                            
                            // Cleanup other buffers
                            buffer_free(final_result.error_message);
                            buffer_free(final_result.missing_columns);
                            buffer_free(first_result.result);
                        } else {
                            processed_result = first_result.result;
                        }
                        
                        // Cleanup first_result buffers we're not using
                        buffer_free(first_result.error_message);
                        buffer_free(first_result.missing_columns);
                    }
                    
                    // Add the final result if we have one
                    if (processed_result) {
                        buffer_json_add_array_item_object(mcpc->result);
                        {
                            buffer_json_member_add_string(mcpc->result, "type", "text");
                            buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(processed_result));
                        }
                        buffer_json_object_close(mcpc->result);
                        
                        // Cleanup processed_result
                        buffer_free(processed_result);
                    }
                }
                
                json_object_put(json_check);
            }
        }
        buffer_json_array_close(mcpc->result);  // Close content array
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON

    return MCP_RC_OK;
}
