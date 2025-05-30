// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-execute-function.h"
#include "mcp-tools-execute-function-internal.h"
#include "mcp-tools-execute-function-logs.h"
#include "mcp-params.h"
#include "database/rrdfunctions.h"

// Analyze the JSON response and determine its type
MCP_FUNCTION_TYPE mcp_functions_analyze_response(struct json_object *json_obj, int *out_status) {
    if (!json_obj) return FN_TYPE_UNKNOWN;
    
    struct json_object *type_obj = NULL;
    struct json_object *has_history_obj = NULL;
    struct json_object *status_obj = NULL;
    
    // Type is required
    if (!json_object_object_get_ex(json_obj, "type", &type_obj)) {
        return FN_TYPE_NOT_TABLE;
    }
    
    const char *type = json_object_get_string(type_obj);
    if (!type || strcmp(type, "table") != 0) {
        return FN_TYPE_NOT_TABLE;
    }
    
    // has_history is optional - assume false if missing
    bool has_history = false;
    if (json_object_object_get_ex(json_obj, "has_history", &has_history_obj)) {
        has_history = json_object_get_boolean(has_history_obj);
    }
    
    // Status is optional - assume 200 if missing
    int status = 200;
    if (json_object_object_get_ex(json_obj, "status", &status_obj)) {
        status = json_object_get_int(status_obj);
    }
    
    if (out_status) *out_status = status;
    
    // Return appropriate type based on has_history
    if (has_history) {
        return FN_TYPE_TABLE_WITH_HISTORY;
    } else {
        return FN_TYPE_TABLE;
    }
}

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
OPERATOR_TYPE mcp_functions_string_to_operator(const char *op_str) {
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
void mcp_functions_free_condition_patterns(CONDITION_ARRAY *condition_array) {
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
    
    // Handle NULL values
    if (json_object_is_type(value, json_type_null)) {
        if (condition->v_type == COND_VALUE_NULL) {
            return (condition->op == OP_EQUALS);
        } else {
            return (condition->op == OP_NOT_EQUALS);
        }
    }
    
    // Handle MATCH and NOT MATCH operators (pattern matching) - always convert to strings
    if (condition->op == OP_MATCH || condition->op == OP_NOT_MATCH) {
        if (condition->pattern) {
            const char *val_str = json_object_get_string(value);
            bool pattern_match = simple_pattern_matches(condition->pattern, val_str);
            return (condition->op == OP_MATCH) ? pattern_match : !pattern_match;
        }
        return false;
    }
    
    // Handle comparisons based on condition value type
    if (condition->v_type == COND_VALUE_NUMBER) {
        // Try to get numeric value from JSON
        double val_num = 0.0;
        if (json_object_is_type(value, json_type_int) || json_object_is_type(value, json_type_double)) {
            val_num = json_object_get_double(value);
        } else if (json_object_is_type(value, json_type_string)) {
            // Try to parse string as number
            char *endptr;
            const char *str = json_object_get_string(value);
            val_num = strtod(str, &endptr);
            if (endptr == str || *endptr != '\0') {
                // Not a valid number, do string comparison
                goto string_compare;
            }
        } else {
            // Can't convert to number, treat as not equal
            return (condition->op == OP_NOT_EQUALS);
        }
        
        switch (condition->op) {
            case OP_EQUALS:
                return (val_num == condition->v_num);
            case OP_NOT_EQUALS:
                return (val_num != condition->v_num);
            case OP_LESS:
                return (val_num < condition->v_num);
            case OP_LESS_EQUALS:
                return (val_num <= condition->v_num);
            case OP_GREATER:
                return (val_num > condition->v_num);
            case OP_GREATER_EQUALS:
                return (val_num >= condition->v_num);
            default:
                return false;
        }
    }
    else if (condition->v_type == COND_VALUE_BOOLEAN) {
        bool val_bool = json_object_get_boolean(value);
        
        switch (condition->op) {
            case OP_EQUALS:
                return (val_bool == condition->v_bool);
            case OP_NOT_EQUALS:
                return (val_bool != condition->v_bool);
            default:
                // Boolean doesn't support ordering comparisons
                return false;
        }
    }
    
string_compare:
    // String comparisons (including when condition is string or as fallback)
    const char *val_str = json_object_get_string(value);
    const char *cond_str = condition->v_str;
    
    // Handle NULL condition string
    if (!cond_str) {
        if (condition->v_type == COND_VALUE_NULL) {
            return (condition->op == OP_EQUALS && !val_str);
        }
        // Use empty string for comparison
        cond_str = "";
    }
    
    if (!val_str) val_str = "";
    
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

// Resolve column indices for conditions that were parsed during request parsing
static void resolve_condition_columns(CONDITION_ARRAY *condition_array, struct json_object *columns_obj) {
    if (!condition_array || !columns_obj || condition_array->count == 0)
        return;
    
    condition_array->has_missing_columns = false;
    
    for (size_t i = 0; i < condition_array->count; i++) {
        CONDITION *curr = &condition_array->items[i];
        
        // Check for special column names that mean "search all columns"
        bool is_all_columns = (strcmp(curr->column_name, "*") == 0 || strcmp(curr->column_name, "") == 0);
        
        // Find column in column definitions
        struct json_object *col_obj = NULL;
        if (is_all_columns || !json_object_object_get_ex(columns_obj, curr->column_name, &col_obj)) {
            // Column not found or explicitly all columns - mark it as a wildcard search (use -1 as special index)
            curr->column_index = -1;
            
            // Only report as missing if it's not a special "all columns" indicator
            if (!is_all_columns) {
                condition_array->has_missing_columns = true;
            }
        } else {
            // Get column index
            struct json_object *index_obj = NULL;
            if (json_object_object_get_ex(col_obj, "index", &index_obj)) {
                curr->column_index = json_object_get_int(index_obj);
            } else {
                // Column found but no index - treat as missing
                curr->column_index = -1;
                condition_array->has_missing_columns = true;
            }
        }
    }
}

// Flags to indicate what additional content should be added for errors
typedef enum {
    MCP_TABLE_ADD_NOTHING = 0,
    MCP_TABLE_ADD_COLUMNS = 1 << 0,
    MCP_TABLE_ADD_RAW_DATA = 1 << 1,
    MCP_TABLE_ADD_FILTERING_INSTRUCTIONS = 1 << 2
} MCP_TABLE_ADDITIONAL_CONTENT;

// Initialize MCP_FUNCTION_DATA structure
void mcp_functions_data_init(MCP_FUNCTION_DATA *data) {
    memset(data, 0, sizeof(MCP_FUNCTION_DATA));
    data->output.result = buffer_create(0, NULL);
}

// Clean up MCP_FUNCTION_DATA structure
void mcp_functions_data_cleanup(MCP_FUNCTION_DATA *data) {
    if (!data) return;
    
    // Free the parsed JSON object
    if (data->input.jobj) {
        json_object_put(data->input.jobj);
        data->input.jobj = NULL;
    }
    
    // Free the output result buffer
    if (data->output.result) {
        buffer_free(data->output.result);
        data->output.result = NULL;
    }
    
    // Free condition patterns
    mcp_functions_free_condition_patterns(&data->request.conditions);
    
    // Free the input JSON buffer
    if (data->input.json) {
        buffer_free(data->input.json);
        data->input.json = NULL;
    }
}

// Reset output for reprocessing
static void mcp_function_data_reset_output(MCP_FUNCTION_DATA *data) {
    buffer_flush(data->output.result);
    data->output.status = MCP_TABLE_OK;
    data->output.rows = 0;
    data->output.columns = 0;
}

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
static MCP_TABLE_ADDITIONAL_CONTENT generate_table_error_message(MCP_FUNCTION_DATA *data) {
    buffer_flush(data->request.mcpc->error);
    MCP_TABLE_ADDITIONAL_CONTENT additional_content = MCP_TABLE_ADD_NOTHING;
    
    switch (data->output.status) {
        case MCP_TABLE_ERROR_INVALID_CONDITIONS:
            buffer_sprintf(data->request.mcpc->error,
                "Error processing conditions: %s\n\n"
                "Conditions should be formatted as:\n"
                "```json\n"
                "\"conditions\": [\n"
                "    [\"column_name\", \"operator\", value],\n"
                "    [\"another_column\", \"another_operator\", another_value]\n"
                "]\n"
                "```",
                buffer_tostring(data->output.result)
            );
            additional_content = MCP_TABLE_ADD_FILTERING_INSTRUCTIONS;
            break;
            
        case MCP_TABLE_ERROR_NO_MATCHES_WITH_MISSING_COLUMNS:
            buffer_strcat(data->request.mcpc->error,
                "No rows matched the specified conditions.\n\n"
                "Note: Some columns were not found, so a full-text search was performed across all columns, but no matches were found."
            );
            additional_content = MCP_TABLE_ADD_COLUMNS | MCP_TABLE_ADD_FILTERING_INSTRUCTIONS;
            break;
            
        case MCP_TABLE_ERROR_NO_MATCHES:
            buffer_strcat(data->request.mcpc->error,
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
            buffer_sprintf(data->request.mcpc->error,
                "Invalid sort_order: '%s'. Valid options are 'asc' (ascending) or 'desc' (descending).\n\n"
                "Example:\n"
                "```json\n"
                "\"sort_order\": \"desc\"\n"
                "```",
                buffer_tostring(data->output.result)
            );
            additional_content = MCP_TABLE_ADD_NOTHING;
            break;
            
        case MCP_TABLE_ERROR_COLUMNS_NOT_FOUND:
            buffer_sprintf(data->request.mcpc->error,
                "Column(s) not found: %s",
                buffer_tostring(data->output.result)
            );
            additional_content = MCP_TABLE_ADD_COLUMNS;
            break;
            
        case MCP_TABLE_ERROR_SORT_COLUMN_NOT_FOUND:
            buffer_sprintf(data->request.mcpc->error,
                "Sort column '%s' not found.",
                buffer_tostring(data->output.result)
            );
            additional_content = MCP_TABLE_ADD_COLUMNS;
            break;
            
        case MCP_TABLE_ERROR_TOO_MANY_COLUMNS:
            buffer_sprintf(data->request.mcpc->error,
                "Error: Table has %zu columns, which exceeds the maximum supported (%d). Showing raw output.",
                data->input.columns, MAX_COLUMNS
            );
            additional_content = MCP_TABLE_ADD_RAW_DATA;
            break;
            
        case MCP_TABLE_NOT_JSON:
            buffer_strcat(data->request.mcpc->error,
                "This response is not valid JSON. Showing raw output.");
            additional_content = MCP_TABLE_ADD_RAW_DATA;
            break;
            
        case MCP_TABLE_NOT_PROCESSABLE:
            buffer_strcat(data->request.mcpc->error,
                "The function returned JSON but it's not a table format we can filter. Showing raw output.");
            additional_content = MCP_TABLE_ADD_RAW_DATA;
            break;
            
        case MCP_TABLE_EMPTY_RESULT:
            buffer_strcat(data->request.mcpc->error,
                "The function returned an empty result (no rows).");
            additional_content = MCP_TABLE_ADD_RAW_DATA;
            break;
            
        case MCP_TABLE_INFO_MISSING_COLUMNS_FOUND_RESULTS:
            buffer_strcat(data->request.mcpc->error,
                "Note: Not all columns in the conditions were found, so a full-text search was performed across all columns, and matching results were found.");
            additional_content = MCP_TABLE_ADD_NOTHING;
            break;
            
        case MCP_TABLE_RESPONSE_TOO_BIG:
            buffer_sprintf(data->request.mcpc->error,
                "The response is too big, having %zu rows and %zu columns. Limiting to 1 row for readability.",
                data->input.rows, data->input.columns
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
            "    [\"CmdLine\", \"match\", \"*systemd*|*postgresql*|*docker*\"],\n"
            "    [\"*\", \"match\", \"*error*|*warning*\"]  // Search all columns\n"
            "  ],\n"
            "  \"sort_column\": \"CPU\",\n"
            "  \"sort_order\": \"desc\",\n"
            "  \"limit\": 10\n"
            "}\n"
            "```\n"
            "\n"
            "Operators: ==, !=, <, <=, >, >=, match (simple pattern), not match (simple pattern)\n"
            "Simple patterns: '*this*|*that*|*other*' (wildcard search to find strings that include 'this', or 'that', or 'other')\n"
            "Full-text search: Use '*' or '' as column name to search across all columns"
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
static void add_table_messages_to_mcp_result(MCP_FUNCTION_DATA *data,
                                            struct json_object *columns_obj) {
    // Generate the appropriate error message and get additional content flags
    MCP_TABLE_ADDITIONAL_CONTENT additional_content = generate_table_error_message(data);
    
    // Add the message if there's an error or guidance
    if (data->output.status != MCP_TABLE_OK && buffer_strlen(data->request.mcpc->error) > 0) {
        buffer_json_add_array_item_object(data->request.mcpc->result);
        {
            buffer_json_member_add_string(data->request.mcpc->result, "type", "text");
            buffer_json_member_add_string(data->request.mcpc->result, "text", buffer_tostring(data->request.mcpc->error));
        }
        buffer_json_object_close(data->request.mcpc->result);
    }
    
    // Add columns info if requested
    if ((additional_content & MCP_TABLE_ADD_COLUMNS) && columns_obj) {
        add_columns_info_to_mcp_result(data->request.mcpc, columns_obj);
    }
    
    // Add filtering instructions if requested
    if (additional_content & MCP_TABLE_ADD_FILTERING_INSTRUCTIONS) {
        add_filtering_instructions_to_mcp_result(data->request.mcpc);
    }
    
    // Add raw data if requested
    if ((additional_content & MCP_TABLE_ADD_RAW_DATA) && 
        buffer_strlen(data->output.result) > 0) {
        buffer_json_add_array_item_object(data->request.mcpc->result);
        {
            buffer_json_member_add_string(data->request.mcpc->result, "type", "text");
            buffer_json_member_add_string(data->request.mcpc->result, "text", buffer_tostring(data->output.result));
        }
        buffer_json_object_close(data->request.mcpc->result);
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
    
    mcp_schema_add_timeout(buffer, "timeout", "Execution timeout in seconds",
                          "Maximum time to wait for function execution (default: 60)",
                          60, 1, 3600, false);

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

    mcp_schema_add_string_param(buffer, "sort_column", "Column to sort by",
                               "Name of the column to sort the results by.",
                               NULL, false);

    buffer_json_member_add_object(buffer, "sort_order");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Sort order");
        buffer_json_member_add_string(buffer, "description", "Order to sort results: 'asc' for ascending, 'desc' for descending");
        buffer_json_member_add_string(buffer, "default", "desc");
        buffer_json_member_add_array(buffer, "enum");
        buffer_json_add_array_item_string(buffer, "asc");
        buffer_json_add_array_item_string(buffer, "desc");
        buffer_json_array_close(buffer);
    }
    buffer_json_object_close(buffer); // sort_order

    mcp_schema_add_size_param(buffer, "limit", "Row limit",
                             "Maximum number of rows to return",
                             0, 0, SIZE_MAX, false);
    
    buffer_json_member_add_object(buffer, "conditions");
    {
        buffer_json_member_add_string(buffer, "type", "array");
        buffer_json_member_add_string(buffer, "title", "Filter conditions");
        buffer_json_member_add_string(buffer, "description",
            "Array of conditions to filter rows. "
            "Each condition is an array of [column, operator, value] where operator "
            "can be ==, !=, <>, <, <=, >, >=, match, not match. "
            "Use '*' or '' (empty string) as column name to search across all columns.");
        
        buffer_json_member_add_object(buffer, "items");
        {
            buffer_json_member_add_string(buffer, "type", "array");
            buffer_json_member_add_object(buffer, "items");
            {
                buffer_json_member_add_array(buffer, "oneOf");
                {
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
                        {
                            buffer_json_add_array_item_string(buffer, "==");
                            buffer_json_add_array_item_string(buffer, "!=");
                            buffer_json_add_array_item_string(buffer, "<>");
                            buffer_json_add_array_item_string(buffer, "<");
                            buffer_json_add_array_item_string(buffer, "<=");
                            buffer_json_add_array_item_string(buffer, ">");
                            buffer_json_add_array_item_string(buffer, ">=");
                            buffer_json_add_array_item_string(buffer, "match");
                            buffer_json_add_array_item_string(buffer, "not match");
                        }
                        buffer_json_array_close(buffer);
                    }
                    buffer_json_object_close(buffer);

                    // Third item - value (can be string, number, or boolean)
                    buffer_json_add_array_item_object(buffer);
                    {
                        buffer_json_member_add_array(buffer, "oneOf");
                        {
                            buffer_json_add_array_item_object(buffer);
                            {
                                buffer_json_member_add_string(buffer, "type", "string");
                            }
                            buffer_json_object_close(buffer);

                            buffer_json_add_array_item_object(buffer);
                            {
                                buffer_json_member_add_string(buffer, "type", "number");
                            }
                            buffer_json_object_close(buffer);

                            buffer_json_add_array_item_object(buffer);
                            {
                                buffer_json_member_add_string(buffer, "type", "boolean");
                            }
                            buffer_json_object_close(buffer);
                        }
                        buffer_json_array_close(buffer);
                    }
                    buffer_json_object_close(buffer); // third item
                }
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
 * @param data Function data structure with request context, input and output
 * @param max_size_threshold Maximum size in bytes before truncation is recommended
 * 
 * @return void (result is returned in data->output)
 */
static void mcp_process_table_result(MCP_FUNCTION_DATA *data, size_t max_size_threshold)
{
    // Reset output for processing
    mcp_function_data_reset_output(data);
    
    // Clear error buffer for any error messages that might be generated
    buffer_flush(data->request.mcpc->error);
    
    // Get original JSON string from input
    const char *json_str = buffer_tostring(data->input.json);
    size_t result_size = buffer_strlen(data->input.json);

    if (!data->input.jobj) {
        buffer_strcat(data->output.result, json_str); // Return original if JSON is NULL
        data->output.status = MCP_TABLE_NOT_JSON;
        return;
    }

    // Check if it's a processable table format
    if (data->input.type != FN_TYPE_TABLE) {
        buffer_strcat(data->output.result, json_str); // Not a processable table format
        data->output.status = MCP_TABLE_NOT_PROCESSABLE;
        return;
    }

    // Get data and columns
    struct json_object *data_obj = NULL;
    struct json_object *columns_obj = NULL;

    if (!json_object_object_get_ex(data->input.jobj, "data", &data_obj) ||
        !json_object_object_get_ex(data->input.jobj, "columns", &columns_obj)) {
        buffer_strcat(data->output.result, json_str); // Missing required elements
        return;
    }

    size_t row_count = json_object_array_length(data_obj);
    size_t column_count = json_object_object_length(columns_obj);
    
    // Store original counts in input
    data->input.rows = row_count;
    data->input.columns = column_count;

    // Check if we need to show guidance - only when no filtering is specified
    if (result_size > max_size_threshold && data->request.columns.count == 0 && !data->request.sort.column && 
        data->request.limit == 0 && data->request.conditions.count == 0) {
        // Store first row in result for guidance
        data->output.status = MCP_TABLE_RESPONSE_TOO_BIG;
        data->request.limit = 1;
    }

    // Even with no filtering parameters, we need to process to remove unwanted fields

    // Sort direction is already parsed in data->request.sort.descending

    // Use fixed-size arrays for column selection
    uint8_t column_selected[MAX_COLUMNS] = {0};  // 1 if column is selected, 0 if not
    char *column_names[MAX_COLUMNS] = {0};       // Names of selected columns (references)
    int column_indices[MAX_COLUMNS] = {0};       // Index mapping for ordered access
    size_t selected_count = 0;
    
    // Check if we have too many columns
    if (column_count > MAX_COLUMNS) {
        data->output.status = MCP_TABLE_ERROR_TOO_MANY_COLUMNS;
        data->output.columns = column_count;
        
        return;
    }
    
    if (data->request.columns.count > 0) {
        // Process the pre-parsed column names
        bool all_columns_found = true;
        CLEAN_BUFFER *missing_columns = buffer_create(0, NULL);
        
        for (size_t i = 0; i < data->request.columns.count; i++) {
            const char *col = data->request.columns.array[i];
            
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
        
        // If any columns were not found, set error status
        if (!all_columns_found) {
            data->output.status = MCP_TABLE_ERROR_COLUMNS_NOT_FOUND;
            buffer_strcat(data->output.result, buffer_tostring(missing_columns));
            
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

    if (data->request.sort.column) {
        struct json_object *sort_col_obj = NULL;
        if (json_object_object_get_ex(columns_obj, data->request.sort.column, &sort_col_obj)) {
            struct json_object *index_obj = NULL;
            if (json_object_object_get_ex(sort_col_obj, "index", &index_obj)) {
                sort_idx = json_object_get_int(index_obj);
            }
        } else {
            // Column not found
            data->output.status = MCP_TABLE_ERROR_SORT_COLUMN_NOT_FOUND;
            buffer_strcat(data->output.result, data->request.sort.column);
            
            return;
        }
    }

    // Create new arrays for filtered/sorted data
    struct json_object **rows = (struct json_object **)callocz(row_count, sizeof(struct json_object *));
    size_t row_idx = 0;

    // Resolve column indices for pre-parsed conditions if they exist
    if (data->request.conditions.count > 0) {
        resolve_condition_columns(&data->request.conditions, columns_obj);
    }

    // Copy rows for filtering/sorting, applying filters if specified
    for (size_t i = 0; i < row_count; i++) {
        struct json_object *row = json_object_array_get_idx(data_obj, i);
        if (!row)
            continue;

        bool include_row = true;

        // Apply preprocessed conditions if they exist
        if (data->request.conditions.count > 0) {
            include_row = row_matches_conditions(row, &data->request.conditions);
        }

        if (include_row) {
            rows[row_idx++] = row;
        }
    }
    
    // If we have missing columns and found no matches, provide helpful error
    if (data->request.conditions.has_missing_columns && row_idx == 0) {
        data->output.status = MCP_TABLE_ERROR_NO_MATCHES_WITH_MISSING_COLUMNS;
        
        // Clean up and return
        freez((void *)rows);
        
        return;
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
                    should_swap = !data->request.sort.descending;
                } else if (!val_j) {
                    should_swap = data->request.sort.descending;
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
                        should_swap = data->request.sort.descending ? (i_val < j_val) : (i_val > j_val);
                        
                    } else if (json_object_is_type(val_i, json_type_boolean) || 
                               json_object_is_type(val_j, json_type_boolean)) {
                        
                        // Let json-c do the type conversion
                        bool i_val = json_object_get_boolean(val_i);
                        bool j_val = json_object_get_boolean(val_j);
                        should_swap = data->request.sort.descending ? (i_val && !j_val) : (!i_val && j_val);
                        
                    } else {
                        // Default to string comparison for everything else
                        int cmp = strcmp(json_object_get_string(val_i), json_object_get_string(val_j));
                        should_swap = data->request.sort.descending ? (cmp < 0) : (cmp > 0);
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
    if (data->request.limit > 0 && data->request.limit < limit) {
        limit = data->request.limit;
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

            if (json_object_object_get_ex(data->input.jobj, keep_fields[i], &field_obj)) {
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
    if (limit == 0 && data->request.conditions.count > 0) {
        // No rows matched the conditions
        json_object_put(filtered_result);
        data->output.status = MCP_TABLE_ERROR_NO_MATCHES;
    } else {
        // Set status flag if we used wildcard search and found results
        if (data->request.conditions.has_missing_columns && row_idx > 0) {
            data->output.status = MCP_TABLE_INFO_MISSING_COLUMNS_FOUND_RESULTS;
        }
        
        // Convert to string and store result
        const char *filtered_json = json_object_to_json_string_ext(filtered_result, JSON_C_TO_STRING_PRETTY);
        buffer_strcat(data->output.result, filtered_json);

        // Update actual counts from filtered result
        data->output.rows = limit;
        data->output.columns = selected_count;
        
        // Free the filtered result
        json_object_put(filtered_result);
    }

    // Clean up
    freez((void *)rows);
    
}

// Parse conditions without column information (early parsing during request)
static MCP_RETURN_CODE mcp_parse_conditions_early(CONDITION_ARRAY *condition_array, struct json_object *conditions_json, BUFFER *error_buffer) {
    if (!condition_array || !conditions_json)
        return MCP_RC_ERROR;
    
    // Initialize the condition array
    memset(condition_array, 0, sizeof(CONDITION_ARRAY));
    
    if (!json_object_is_type(conditions_json, json_type_array))
        return MCP_RC_INVALID_PARAMS;
    
    size_t conditions_count = json_object_array_length(conditions_json);
    if (conditions_count == 0)
        return MCP_RC_OK; // Empty array is valid
    
    if (conditions_count > MAX_CONDITIONS) {
        if (error_buffer)
            buffer_sprintf(error_buffer, "Too many conditions. Maximum is %d.", MAX_CONDITIONS);
        return MCP_RC_INVALID_PARAMS;
    }
    
    for (size_t i = 0; i < conditions_count; i++) {
        struct json_object *condition = json_object_array_get_idx(conditions_json, i);
        
        // Each condition should be an array of [column, operator, value]
        if (!condition || !json_object_is_type(condition, json_type_array) ||
            json_object_array_length(condition) != 3) {
            if (error_buffer)
                buffer_sprintf(error_buffer, "Invalid condition format at index %zu. Expected [column, operator, value]", i);
            mcp_functions_free_condition_patterns(condition_array);
            return MCP_RC_INVALID_PARAMS;
        }
        
        struct json_object *col_name_obj = json_object_array_get_idx(condition, 0);
        struct json_object *operator_obj = json_object_array_get_idx(condition, 1);
        struct json_object *value_obj = json_object_array_get_idx(condition, 2);
        
        if (!col_name_obj || !json_object_is_type(col_name_obj, json_type_string) ||
            !operator_obj || !json_object_is_type(operator_obj, json_type_string) ||
            !value_obj) {
            if (error_buffer)
                buffer_sprintf(error_buffer, "Invalid condition element types at index %zu. Expected [string, string, any]", i);
            mcp_functions_free_condition_patterns(condition_array);
            return MCP_RC_INVALID_PARAMS;
        }
        
        CONDITION *curr = &condition_array->items[condition_array->count];
        
        // Get column name and operator
        curr->column_name = json_object_get_string(col_name_obj);
        const char *op_str = json_object_get_string(operator_obj);
        
        curr->op = mcp_functions_string_to_operator(op_str);
        if (curr->op == OP_UNKNOWN) {
            if (error_buffer)
                buffer_sprintf(error_buffer, "Invalid operator '%s' at index %zu. Valid operators are: ==, !=, <>, <, <=, >, >=, match, not match", 
                              op_str, i);
            mcp_functions_free_condition_patterns(condition_array);
            return MCP_RC_INVALID_PARAMS;
        }
        
        // Parse and store the value based on its type
        if (json_object_is_type(value_obj, json_type_null)) {
            curr->v_type = COND_VALUE_NULL;
        } else if (json_object_is_type(value_obj, json_type_boolean)) {
            curr->v_type = COND_VALUE_BOOLEAN;
            curr->v_bool = json_object_get_boolean(value_obj);
        } else if (json_object_is_type(value_obj, json_type_int) || json_object_is_type(value_obj, json_type_double)) {
            curr->v_type = COND_VALUE_NUMBER;
            curr->v_num = json_object_get_double(value_obj);
        } else {
            // Everything else is treated as string
            curr->v_type = COND_VALUE_STRING;
            curr->v_str = json_object_get_string(value_obj);
        }
        
        // Pre-compile patterns for MATCH operators
        if (curr->op == OP_MATCH || curr->op == OP_NOT_MATCH) {
            const char *pattern_str = NULL;
            if (curr->v_type == COND_VALUE_STRING) {
                pattern_str = curr->v_str;
            } else {
                // For non-string types, use json-c's string conversion
                pattern_str = json_object_get_string(value_obj);
            }
            if (pattern_str) {
                curr->pattern = string_to_simple_pattern_nocase_substring(pattern_str);
            }
        }
        
        // Mark column index as unknown for now (-1)
        curr->column_index = -1;
        
        condition_array->count++;
    }
    
    return MCP_RC_OK;
}

// Parse the request parameters and populate the MCP_FUNCTION_DATA structure
static MCP_RETURN_CODE mcp_parse_function_request(MCP_FUNCTION_DATA *data, MCP_CLIENT *mcpc, struct json_object *params) {
    if (!data || !mcpc || !params)
        return MCP_RC_ERROR;
    
    // Set request context
    data->request.mcpc = mcpc;
    data->request.params = params;
    data->request.auth = mcpc->user_auth;
    
    // Parse required parameter: node
    struct json_object *obj = NULL;
    if (json_object_object_get_ex(params, "node", &obj) && 
        json_object_is_type(obj, json_type_string)) {
        data->request.node = json_object_get_string(obj);
    }
    
    if (!data->request.node || !*data->request.node) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'node'");
        return MCP_RC_BAD_REQUEST;
    }
    
    // Parse required parameter: function
    if (json_object_object_get_ex(params, "function", &obj) && 
        json_object_is_type(obj, json_type_string)) {
        data->request.function = json_object_get_string(obj);
    }
    
    if (!data->request.function || !*data->request.function) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'function'");
        return MCP_RC_BAD_REQUEST;
    }
    
    // Parse timeout parameter
    data->request.timeout = (time_t)mcp_params_extract_timeout(params, "timeout", 60, 1, 3600, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // Parse optional filtering parameters
    
    // columns array - parse it early
    if (json_object_object_get_ex(params, "columns", &obj) && 
        json_object_is_type(obj, json_type_array) &&
        json_object_array_length(obj) > 0) {
        size_t count = json_object_array_length(obj);
        if (count > MAX_SELECTED_COLUMNS) {
            buffer_sprintf(mcpc->error, "Too many columns requested. Maximum is %d.", MAX_SELECTED_COLUMNS);
            return MCP_RC_BAD_REQUEST;
        }
        
        data->request.columns.count = 0;
        for (size_t i = 0; i < count; i++) {
            struct json_object *col_obj = json_object_array_get_idx(obj, i);
            if (col_obj && json_object_is_type(col_obj, json_type_string)) {
                data->request.columns.array[data->request.columns.count++] = json_object_get_string(col_obj);
            }
        }
    }
    
    // sort_column
    if (json_object_object_get_ex(params, "sort_column", &obj) && 
        json_object_is_type(obj, json_type_string)) {
        const char *value = json_object_get_string(obj);
        if (value && *value) {
            data->request.sort.column = value;
        }
    }
    
    // sort_order and parse it to boolean
    data->request.sort.descending = true; // default to DESC
    if (json_object_object_get_ex(params, "sort_order", &obj) && 
        json_object_is_type(obj, json_type_string)) {
        const char *sort_order = json_object_get_string(obj);
        if (sort_order) {
            if (strcasecmp(sort_order, "asc") == 0) {
                data->request.sort.descending = false;
            } else if (strcasecmp(sort_order, "desc") != 0) {
                // Invalid sort order
                buffer_sprintf(mcpc->error, "Invalid sort_order: '%s'. Valid options are 'asc' or 'desc'.", sort_order);
                return MCP_RC_BAD_REQUEST;
            }
        }
    }
    
    // limit
    data->request.limit = 0; // 0 means no limit
    if (json_object_object_get_ex(params, "limit", &obj) && 
        json_object_is_type(obj, json_type_int)) {
        int limit = json_object_get_int(obj);
        if (limit > 0) {
            data->request.limit = (size_t)limit;
        }
    }
    
    // conditions array - parse it early
    if (json_object_object_get_ex(params, "conditions", &obj) && 
        json_object_is_type(obj, json_type_array) &&
        json_object_array_length(obj) > 0) {
        // Parse conditions early
        MCP_RETURN_CODE rc = mcp_parse_conditions_early(&data->request.conditions, obj, mcpc->error);
        if (rc != MCP_RC_OK) {
            return rc;
        }
    }
    
    // Find the host
    data->request.host = rrdhost_find_by_hostname(data->request.node);
    if (!data->request.host) {
        data->request.host = rrdhost_find_by_guid(data->request.node);
        if (!data->request.host) {
            data->request.host = rrdhost_find_by_node_id(data->request.node);
        }
    }
    
    if (!data->request.host) {
        buffer_sprintf(mcpc->error, "Node not found: %s", data->request.node);
        return MCP_RC_NOT_FOUND;
    }
    
    // Generate transaction UUID
    uuid_generate(data->request.transaction_uuid);
    uuid_unparse_lower(data->request.transaction_uuid, data->request.transaction);
    
    return MCP_RC_OK;
}

// Execute the function and populate the input section of data
static MCP_RETURN_CODE mcp_function_run(MCP_FUNCTION_DATA *data) {
    if (!data || !data->request.host || !data->request.function)
        return MCP_RC_ERROR;
    
#if 1
    // Override auth for testing
    data->request.auth->access = HTTP_ACCESS_ALL;
    data->request.auth->method = USER_AUTH_METHOD_CLOUD;
    data->request.auth->user_role = HTTP_USER_ROLE_ADMIN;
#endif
    
    // Create source buffer from user_auth
    CLEAN_BUFFER *source = buffer_create(0, NULL);
    user_auth_to_source_buffer(data->request.auth, source);
    buffer_strcat(source, ",modelcontextprotocol");
    
    // Create result buffer (will be owned by data->input.json)
    BUFFER *result_buffer = buffer_create(0, NULL);
    
    // Execute the function
    int ret = rrd_function_run(
        data->request.host,
        result_buffer,
        data->request.timeout,
        data->request.auth->access,
        data->request.function,
        true,
        data->request.transaction,
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
        buffer_sprintf(data->request.mcpc->error,
                       "Failed to execute function '%s' on node '%s', "
                       "http error code %d (%s):\n"
                       "```json\n%s\n```",
                       data->request.function, data->request.node, ret,
                       http_response_code2string(ret),
                       buffer_tostring(result_buffer));
        buffer_free(result_buffer);
        return MCP_RC_ERROR;
    }
    
    // Store the result in data->input
    data->input.json = result_buffer;
    data->input.jobj = json_tokener_parse(buffer_tostring(result_buffer));
    
    // Analyze the response type
    int status = 0;
    data->input.type = mcp_functions_analyze_response(data->input.jobj, &status);
    
    return MCP_RC_OK;
}


// Process table response
static MCP_RETURN_CODE mcp_functions_process_table(MCP_FUNCTION_DATA *data, MCP_REQUEST_ID id) {
    const size_t max_size_threshold = 20UL * 1024;
    
    // Initialize success response
    mcp_init_success_result(data->request.mcpc, id);
    
    // Start building content array for the result
    buffer_json_member_add_array(data->request.mcpc->result, "content");
    
    if (!data->input.jobj) {
        // Not valid JSON - return raw output with message
        data->output.status = MCP_TABLE_NOT_JSON;
        buffer_strcat(data->output.result, buffer_tostring(data->input.json));
        add_table_messages_to_mcp_result(data, NULL);
    }
    else if (data->input.type == FN_TYPE_NOT_TABLE) {
        // Not a processable table format
        data->output.status = MCP_TABLE_NOT_PROCESSABLE;
        buffer_strcat(data->output.result, buffer_tostring(data->input.json));
        add_table_messages_to_mcp_result(data, NULL);
    }
    else {
        // It's a processable table - get data and columns
        struct json_object *data_obj = NULL;
        struct json_object *columns_obj = NULL;
        
        if (!json_object_object_get_ex(data->input.jobj, "data", &data_obj) ||
            !json_object_object_get_ex(data->input.jobj, "columns", &columns_obj)) {
            // Missing required fields, treat as not processable
            data->output.status = MCP_TABLE_NOT_PROCESSABLE;
            buffer_strcat(data->output.result, buffer_tostring(data->input.json));
            add_table_messages_to_mcp_result(data, NULL);
        }
        else {
            // Check if data is empty
            size_t original_row_count = json_object_array_length(data_obj);
            
            if (original_row_count == 0) {
                // Empty result
                data->output.status = MCP_TABLE_EMPTY_RESULT;
                buffer_strcat(data->output.result, buffer_tostring(data->input.json));
                add_table_messages_to_mcp_result(data, NULL);
            }
            else {
                // Process the table with filtering
                mcp_process_table_result(data, max_size_threshold);
                
                // Check for errors or special conditions
                if (data->output.status != MCP_TABLE_OK && 
                    data->output.status != MCP_TABLE_RESPONSE_TOO_BIG &&
                    data->output.status != MCP_TABLE_INFO_MISSING_COLUMNS_FOUND_RESULTS) {
                    // Handle errors
                    add_table_messages_to_mcp_result(data, columns_obj);
                }
                else {
                    // Check if we need to add any informational messages
                    if (data->output.status == MCP_TABLE_INFO_MISSING_COLUMNS_FOUND_RESULTS) {
                        add_table_messages_to_mcp_result(data, columns_obj);
                        data->output.status = MCP_TABLE_OK;
                    }
                    else if (data->output.status == MCP_TABLE_RESPONSE_TOO_BIG) {
                        // The table was already processed with limit=1 due to size
                        // Add the guidance message
                        add_table_messages_to_mcp_result(data, columns_obj);
                    }
                    
                    // Add the final result
                    buffer_json_add_array_item_object(data->request.mcpc->result);
                    {
                        buffer_json_member_add_string(data->request.mcpc->result, "type", "text");
                        buffer_json_member_add_string(data->request.mcpc->result, "text", buffer_tostring(data->output.result));
                    }
                    buffer_json_object_close(data->request.mcpc->result);
                }
            }
        }
    }
    
    buffer_json_array_close(data->request.mcpc->result);  // Close content array
    buffer_json_object_close(data->request.mcpc->result); // Close result object
    buffer_json_finalize(data->request.mcpc->result); // Finalize the JSON
    
    return MCP_RC_OK;
}

MCP_RETURN_CODE mcp_tool_execute_function_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id)
{
    if (!mcpc || id == 0 || !params)
        return MCP_RC_ERROR;

    // Create and initialize function data structure
    MCP_FUNCTION_DATA data;
    mcp_functions_data_init(&data);
    
    // Parse the request
    MCP_RETURN_CODE rc = mcp_parse_function_request(&data, mcpc, params);
    if (rc != MCP_RC_OK) {
        mcp_functions_data_cleanup(&data);
        return rc;
    }
    
    // Execute the function
    rc = mcp_function_run(&data);
    if (rc != MCP_RC_OK) {
        mcp_functions_data_cleanup(&data);
        return rc;
    }
    
    // Process based on response type
    if (data.input.type == FN_TYPE_TABLE_WITH_HISTORY) {
        rc = mcp_functions_process_logs(&data, id);
    } else {
        rc = mcp_functions_process_table(&data, id);
    }
    
    // Cleanup
    mcp_functions_data_cleanup(&data);
    
    return rc;
}
