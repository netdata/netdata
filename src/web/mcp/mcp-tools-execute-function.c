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

// Structure to hold preprocessed condition information
typedef struct condition_s {
    int column_index;                // Index of the column in the row
    char column_name[256];           // Name of the column (for error reporting) - fixed size array
    OPERATOR_TYPE op;                // Operator type
    struct json_object *value;       // Value to compare against (referenced, not owned)
    SIMPLE_PATTERN *pattern;         // Pre-compiled pattern for MATCH operations
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
            // Skip type as we've already handled it
            strcmp(field_key, "type") != 0) {
            json_object_object_add(col_copy, field_key, json_object_get(field_val));
        }
        
        json_object_iter_next(&col_it);
    }
    
    return col_copy;
}

// Helper function to output columns as JSON with proper filtering
static void output_columns_as_json(BUFFER *output, struct json_object *columns_obj) {
    struct json_object *filtered_columns = json_object_new_object();
    
    struct json_object_iterator it = json_object_iter_begin(columns_obj);
    struct json_object_iterator itEnd = json_object_iter_end(columns_obj);
    
    while (!json_object_iter_equal(&it, &itEnd)) {
        const char *col_name = json_object_iter_peek_name(&it);
        struct json_object *col_obj = json_object_iter_peek_value(&it);
        
        struct json_object *col_copy = create_filtered_column(col_obj);
        json_object_object_add(filtered_columns, col_name, col_copy);
        
        json_object_iter_next(&it);
    }
    
    buffer_strcat(output, "\"columns\": ");
    const char *columns_json = json_object_to_json_string_ext(filtered_columns, JSON_C_TO_STRING_PRETTY);
    buffer_strcat(output, columns_json);
    
    json_object_put(filtered_columns);
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
    // Handle numeric comparisons - try to convert to numbers if comparing with numbers
    else if (json_object_is_type(value, json_type_int) || 
             json_object_is_type(value, json_type_double) ||
             json_object_is_type(condition->value, json_type_int) || 
             json_object_is_type(condition->value, json_type_double)) {
        
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
        
        curr->value = value_obj; // we don't own this, just reference
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
 * @param function_name The name of the function for generating examples
 * @param node_name The node name for generating examples
 * 
 * @return A newly allocated buffer with filtered/processed result or guidance message
 */
BUFFER *mcp_process_table_result(BUFFER *result_buffer, struct json_object *columns_array, 
                                 const char *sort_column_param, const char *sort_order_param,
                                 int limit_param, struct json_object *conditions_array,
                                 size_t max_size_threshold, const char *function_name, const char *node_name,
                                 bool *had_missing_columns_but_found_results)
{
    BUFFER *output = buffer_create(0, NULL);
    struct json_object *json_result = NULL;
    
    // Initialize the output flag
    if (had_missing_columns_but_found_results)
        *had_missing_columns_but_found_results = false;

    // Parse the JSON result
    const char *json_str = buffer_tostring(result_buffer);
    size_t result_size = buffer_strlen(result_buffer);
    json_result = json_tokener_parse(json_str);

    if (!json_result) {
        buffer_strcat(output, json_str); // Return original if parsing fails
        return output;
    }

    // Check if it's a table format
    struct json_object *type_obj = NULL;
    struct json_object *has_history_obj = NULL;

    if (!json_object_object_get_ex(json_result, "type", &type_obj) ||
        !json_object_object_get_ex(json_result, "has_history", &has_history_obj)) {
        buffer_strcat(output, json_str); // Not in correct format
        json_object_put(json_result);
        return output;
    }

    const char *type = json_object_get_string(type_obj);
    bool has_history = json_object_get_boolean(has_history_obj);

    if (!type || strcmp(type, "table") != 0 || has_history) {
        buffer_strcat(output, json_str); // Not a table format
        json_object_put(json_result);
        return output;
    }

    // Get data and columns
    struct json_object *data_obj = NULL;
    struct json_object *columns_obj = NULL;

    if (!json_object_object_get_ex(json_result, "data", &data_obj) ||
        !json_object_object_get_ex(json_result, "columns", &columns_obj)) {
        buffer_strcat(output, json_str); // Missing required elements
        json_object_put(json_result);
        return output;
    }

    size_t row_count = json_object_array_length(data_obj);
    size_t column_count = json_object_object_length(columns_obj);

    // Check if we need to show guidance - only when no filtering is specified
    bool guidance_added = false;
    if (result_size > max_size_threshold && !columns_array && !sort_column_param && limit_param < 0 &&
        !conditions_array) {
        buffer_sprintf(output,
                       "The response is too big (%zu bytes), having %zu rows and %zu columns.\n"
                       "Here is the first row for your reference:\n"
                       "\n"
                       "```json\n",
                       result_size, row_count, column_count);

        limit_param = 1;
        guidance_added = true;
    }

    // If no filtering parameters are provided, just return the original result
    if (!columns_array && !sort_column_param && limit_param <= 0 && !conditions_array) {
        buffer_flush(output);
        buffer_strcat(output, json_str);
        json_object_put(json_result);
        return output;
    }

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
            // Invalid sort order - return error message
            buffer_flush(output);
            buffer_sprintf(output, 
                "Invalid sort_order: '%s'. Valid options are 'asc' (ascending) or 'desc' (descending).",
                sort_order_param);
                
            json_object_put(json_result);
            return output;
        }
    }

    // Use fixed-size arrays for column selection
    uint8_t column_selected[MAX_COLUMNS] = {0};  // 1 if column is selected, 0 if not
    char *column_names[MAX_COLUMNS] = {0};       // Names of selected columns (references)
    int column_indices[MAX_COLUMNS] = {0};       // Index mapping for ordered access
    size_t selected_count = 0;
    
    // Check if we have too many columns
    if (column_count > MAX_COLUMNS) {
        buffer_sprintf(output, 
            "Error: Table has %zu columns, which exceeds the maximum supported (%d).\n"
            "Please use the 'columns' parameter to select specific columns.",
            column_count, MAX_COLUMNS);
        json_object_put(json_result);
        return output;
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
                            column_names[idx] = (char*)col; // Just store a reference
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
        
        // If any columns were not found, return error message
        if (!all_columns_found) {
            buffer_flush(output);
            
            // Create JSON error response
            buffer_strcat(output, "{\n");
            buffer_sprintf(output, 
                "  \"error\": \"Column(s) not found: %s\",\n", 
                buffer_tostring(missing_columns));
            buffer_strcat(output, "  ");
            output_columns_as_json(output, columns_obj);
            buffer_strcat(output, "\n}");
            
            json_object_put(json_result);
            return output;
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
                    column_names[idx] = (char*)col_key; // Just store a reference
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
            // Column not found - return a helpful error message with available columns
            buffer_flush(output);
            
            // Create JSON error response
            buffer_strcat(output, "{\n");
            buffer_sprintf(output, 
                "  \"error\": \"Sort column '%s' not found\",\n", sort_column_param);
            buffer_strcat(output, "  ");
            output_columns_as_json(output, columns_obj);
            buffer_strcat(output, "\n}");
            
            json_object_put(json_result);
            return output;
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
            // Return a helpful error message with guidance
            buffer_flush(output);
            buffer_sprintf(output, 
                "Error processing conditions: %s\n\n"
                "Conditions should be formatted as:\n"
                "```json\n"
                "\"conditions\": [\n"
                "    [\"column_name\", \"operator\", value],\n"
                "    [\"another_column\", \"another_operator\", another_value]\n"
                "]\n"
                "```\n\n"
                "Available operators: ==, !=, <>, <, <=, >, >=, match, not match\n\n"
                "Examples:\n"
                "- [\"cpu\", \"==\", 0] - Match where cpu equals 0\n"
                "- [\"name\", \"match\", \"*sys*\"] - Match where name contains 'sys'\n"
                "- [\"value\", \">\", 100] - Match where value is greater than 100\n"
                "- [\"state\", \"match\", \"*running*|*stopped*\"] - Match multiple values using the pipe (|) character\n\n"
                "Note: To match multiple values for a field, use the 'match' operator with patterns separated by the pipe (|) character.",
                buffer_tostring(error_buffer)
            );
            
            freez((void *)rows);
            json_object_put(json_result);
            return output;
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
        buffer_flush(output);
        
        // Create JSON error response
        buffer_strcat(output, "{\n");
        buffer_strcat(output, "  \"error\": \"No rows matched the specified conditions\",\n");
        buffer_strcat(output, "  \"note\": \"The following column(s) were not found, so all columns were searched, although no matches were found\",\n");
        buffer_strcat(output, "  \"columns_not_found\": [");
        
        bool first = true;
        for (size_t i = 0; i < conditions.count; i++) {
            if (conditions.items[i].column_index == -1) {
                if (!first) buffer_strcat(output, ", ");
                buffer_sprintf(output, "\"%s\"", conditions.items[i].column_name);
                first = false;
            }
        }
        buffer_strcat(output, "],\n");
        
        buffer_strcat(output, "  ");
        output_columns_as_json(output, columns_obj);
        buffer_strcat(output, "\n}");
        
        // Clean up and return
        if (conditions_result > 0) {
            free_condition_patterns(&conditions);
        }
        freez((void *)rows);
        json_object_put(json_result);
        return output;
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

    // Create filtered column definitions
    for (size_t i = 0; i < selected_count; i++) {
        int col_idx = column_indices[i];
        const char *col_name = column_names[col_idx];
        
        struct json_object *col_obj = NULL;
        if (json_object_object_get_ex(columns_obj, col_name, &col_obj)) {
            struct json_object *col_copy = create_filtered_column(col_obj);
            
            // Update index to match new position
            json_object_object_add(col_copy, "index", json_object_new_int((int)i));
            
            json_object_object_add(filtered_columns, col_name, col_copy);
        }
    }

    // Create filtered data rows
    for (size_t i = 0; i < limit; i++) {
        struct json_object *row = rows[i];
        struct json_object *new_row = json_object_new_array();

        // Extract only selected columns
        for (size_t j = 0; j < selected_count; j++) {
            int col_idx = column_indices[j];
            struct json_object *val = json_object_array_get_idx(row, col_idx);
            if (val) {
                json_object_array_add(new_row, json_object_get(val));
            } else {
                json_object_array_add(new_row, NULL);
            }
        }

        json_object_array_add(filtered_data, new_row);
    }

    // Add filtered data and columns to result
    json_object_object_add(filtered_result, "data", filtered_data);
    json_object_object_add(filtered_result, "columns", filtered_columns);

    // Check if we found any rows
    if (limit == 0 && conditions_array) {
        // No rows matched the conditions
        json_object_put(filtered_result);

        buffer_flush(output);

        // Generate error message
        buffer_strcat(output, "{\n");
        buffer_strcat(output, "  \"error\": \"No results match the specified conditions\",\n");
        buffer_strcat(output, "  \"tips\": [\n");
        buffer_strcat(output, "    \"Verify the column names in your conditions\",\n");
        buffer_strcat(output, "    \"Check the values and operators used\",\n");
        buffer_strcat(output, "    \"For 'match' operators, ensure your pattern format is correct\",\n");
        buffer_strcat(output, "    \"To match multiple values, use 'match' with patterns separated by the pipe (|) character: '*value1*|*value2*'\",\n");
        buffer_strcat(output, "    \"Try broadening your filter criteria\"\n");
        buffer_strcat(output, "  ],\n");
        buffer_strcat(output, "  \"examples\": [\n");
        buffer_strcat(output, "    {\"condition\": [\"cpu\", \"==\", 0], \"description\": \"Match where cpu equals 0\"},\n");
        buffer_strcat(output, "    {\"condition\": [\"name\", \"match\", \"*sys*\"], \"description\": \"Match where name contains 'sys'\"},\n");
        buffer_strcat(output, "    {\"condition\": [\"value\", \">\", 100], \"description\": \"Match where value is greater than 100\"},\n");
        buffer_strcat(output, "    {\"condition\": [\"state\", \"match\", \"*running*|*stopped*\"], \"description\": \"Match multiple values using the pipe (|) character\"}\n");
        buffer_strcat(output, "  ],\n");
        buffer_strcat(output, "  ");
        output_columns_as_json(output, columns_obj);
        buffer_strcat(output, "\n}");
    } else {
        // Set flag if we used wildcard search and found results
        if (has_missing_columns && row_idx > 0 && had_missing_columns_but_found_results) {
            *had_missing_columns_but_found_results = true;
        }
        
        // Convert to string
        const char *filtered_json = json_object_to_json_string_ext(filtered_result, JSON_C_TO_STRING_PRETTY);
        buffer_strcat(output, filtered_json);

        // Free the filtered result
        json_object_put(filtered_result);
    }

    if(guidance_added) {
        buffer_strcat(output, "\n```\n\n");

        // Get column names for example suggestions
        char *column_names[MAX_COLUMNS] = {0};
        size_t column_name_count = 0;
        
        // Use local variables for the foreach to avoid redefinition conflicts
        {
            struct json_object_iterator it = json_object_iter_begin(columns_obj);
            struct json_object_iterator itEnd = json_object_iter_end(columns_obj);
            
            while (!json_object_iter_equal(&it, &itEnd)) {
                const char *col_key = json_object_iter_peek_name(&it);
                column_names[column_name_count++] = (char*)col_key;  // Just store a reference, no allocation
                if (column_name_count >= column_count)
                    break;
                
                json_object_iter_next(&it);
            }
        }
        
        // Create example column list as JSON array items
        char example_columns[256] = {0};
        size_t example_count = column_count > 3 ? 3 : column_count;
        
        for (size_t i = 0; i < example_count; i++) {
            if (i > 0) strcat(example_columns, ", ");
            char quoted_name[128] = {0};
            snprintf(quoted_name, sizeof(quoted_name), "\"%s\"", column_names[i]);
            strcat(example_columns, quoted_name);
        }
        
        // Append the guidance message directly
        buffer_sprintf(output, 
            "\n\nNext Steps:\n"
            "To filter this large result, provide filtering parameters in your request:\n"
            "```json\n"
            "\"params\": {\n"
            "    \"name\": \"execute_function\",\n"
            "    \"arguments\": {\n"
            "        \"node\": \"%s\",\n"
            "        \"function\": \"%s\",\n"
            "        \"timeout\": 60,\n"
            "        \"columns\": [%s],\n"
            "        \"sort_column\": \"%s\",\n"
            "        \"sort_order\": \"desc\",\n"
            "        \"limit\": 10,\n"
            "        \"conditions\": [\n"
            "            [\"%s\", \">\", 100],\n"
            "            [\"%s\", \"==\", \"value\"],\n"
            "            [\"%s\", \"match\", \"*pattern1*|*pattern2*\"]\n"
            "        ]\n"
            "    }\n"
            "}\n"
            "```\n"
            "Filtering options:\n"
            "1. `columns`: Array of column names to include\n"
            "2. `sort_column` and `sort_order`: Sort results by column\n"
            "3. `limit`: Limit number of rows returned\n"
            "4. `conditions`: Array of [column, operator, value] conditions\n"
            "   - Operators: ==, !=, <>, <, <=, >, >=, match, not match\n"
            "   - Values can be strings, numbers, or booleans\n"
            "   - 'match' and 'not match' use Netdata's simple pattern format for string matching\n"
            "   - To match multiple values, use 'match' with patterns separated by pipe (|): \"*value1*|*value2*\"\n",
            node_name, function_name, 
            example_columns,  // This will be quoted column names
            column_names[0],
            column_names[0],  // For numeric example
            column_names[0],
            column_names[0]);
    }

    // Clean up
    freez((void *)rows);

    json_object_put(json_result);
    return output;
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
    
    // Process and filter the results if needed
    const size_t max_size_threshold = 20UL * 1024;
    bool had_missing_columns_but_found_results = false;
    CLEAN_BUFFER *processed_result = mcp_process_table_result(
        result_buffer, 
        columns_array, 
        sort_column_param,
        sort_order_param,
        limit_param,
        conditions_array,
        max_size_threshold,
        function_name,
        node_name,
        &had_missing_columns_but_found_results
    );
    
    // Initialize success response
    mcp_init_success_result(mcpc, id);
    {
        // Start building content array for the result
        buffer_json_member_add_array(mcpc->result, "content");
        {
            // Add note if we had missing columns but found results
            if (had_missing_columns_but_found_results) {
                buffer_json_add_array_item_object(mcpc->result);
                {
                    buffer_json_member_add_string(mcpc->result, "type", "text");
                    buffer_json_member_add_string(mcpc->result, "text", 
                        "Note: Not all columns in the conditions were found, so a full-text search was performed across all columns, and matching results were found.");
                }
                buffer_json_object_close(mcpc->result); // Close note content
            }
            
            // Add the function execution result as text content
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");
                buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(processed_result));
            }
            buffer_json_object_close(mcpc->result); // Close text content
        }
        buffer_json_array_close(mcpc->result);  // Close content array
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON

    return MCP_RC_OK;
}