// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools.h"
#include "database/contexts/rrdcontext.h"
#include "database/rrdfunctions.h"


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
    
    buffer_json_member_add_object(buffer, "q");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Full text search pattern");
        buffer_json_member_add_string(buffer, "description", "Simple pattern to search across all row values. Uses Netdata's simple pattern format (wildcards, prefixes, etc.)");
    }
    buffer_json_object_close(buffer); // q

    buffer_json_member_add_object(buffer, "conditions");
    {
        buffer_json_member_add_string(buffer, "type", "array");
        buffer_json_member_add_string(buffer, "title", "Filter conditions");
        buffer_json_member_add_string(buffer, "description", "Array of conditions to filter rows. Each condition is an array of [column, operator, value] where operator can be ==, !=, <>, <, <=, >, >=");
        
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
 * @param q_param Simple pattern for full text search across all values (or NULL for no search)
 * @param conditions_array JSON array of condition arrays [column, op, value] (or NULL for no conditions)
 * @param max_size_threshold Maximum size in bytes before truncation is recommended
 * @param function_name The name of the function for generating examples
 * @param node_name The node name for generating examples
 * 
 * @return A newly allocated buffer with filtered/processed result or guidance message
 */
BUFFER *mcp_process_table_result(BUFFER *result_buffer, struct json_object *columns_array, 
                                 const char *sort_column_param, const char *sort_order_param,
                                 int limit_param, const char *q_param, struct json_object *conditions_array,
                                 size_t max_size_threshold, const char *function_name, const char *node_name) {
    BUFFER *output = buffer_create(0, NULL);
    struct json_object *json_result = NULL;
    
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
    if (result_size > max_size_threshold && 
        !columns_array && !sort_column_param && limit_param < 0 && 
        !q_param && !conditions_array) {
        // Response is too large and no filtering applied - generate guidance
        
        // Create a sample with the first row
        BUFFER *sample_buffer = buffer_create(0, NULL);
        buffer_json_initialize(sample_buffer, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
        
        if (row_count > 0) {
            // Get column names for example suggestions
            char **column_names = calloc(column_count, sizeof(char*));
            size_t column_name_count = 0;
            
            // Use local variables for the foreach to avoid redefinition conflicts
            {
                struct json_object_iterator it = json_object_iter_begin(columns_obj);
                struct json_object_iterator itEnd = json_object_iter_end(columns_obj);
                
                while (!json_object_iter_equal(&it, &itEnd)) {
                    const char *col_key = json_object_iter_peek_name(&it);
                    column_names[column_name_count++] = strdup(col_key);
                    if (column_name_count >= column_count)
                        break;
                    
                    json_object_iter_next(&it);
                }
            }
            
            // Add only essential table metadata
            buffer_json_member_add_string(sample_buffer, "type", "table");
            buffer_json_member_add_boolean(sample_buffer, "has_history", false);
            
            // Add status and update_every if present in the original
            struct json_object *status_obj = NULL;
            if (json_object_object_get_ex(json_result, "status", &status_obj)) {
                buffer_json_member_add_string(sample_buffer, "status", 
                                             json_object_get_string(status_obj));
            }
            
            struct json_object *update_every_obj = NULL;
            if (json_object_object_get_ex(json_result, "update_every", &update_every_obj)) {
                buffer_json_member_add_int64(sample_buffer, "update_every", 
                                            json_object_get_int64(update_every_obj));
            }
            
            // Add the first row
            struct json_object *first_row = json_object_array_get_idx(data_obj, 0);
            if (first_row) {
                buffer_json_member_add_array(sample_buffer, "data");
                buffer_json_add_array_item_array(sample_buffer);
                
                for (size_t i = 0; i < json_object_array_length(first_row); i++) {
                    struct json_object *val = json_object_array_get_idx(first_row, i);
                    
                    // Handle different value types
                    if (!val || json_object_is_type(val, json_type_null)) {
                        buffer_json_add_array_item_string(sample_buffer, NULL);
                    } 
                    else if (json_object_is_type(val, json_type_string)) {
                        buffer_json_add_array_item_string(sample_buffer, json_object_get_string(val));
                    } 
                    else if (json_object_is_type(val, json_type_int)) {
                        buffer_json_add_array_item_int64(sample_buffer, json_object_get_int64(val));
                    } 
                    else if (json_object_is_type(val, json_type_double)) {
                        buffer_json_add_array_item_double(sample_buffer, json_object_get_double(val));
                    } 
                    else if (json_object_is_type(val, json_type_boolean)) {
                        buffer_json_add_array_item_boolean(sample_buffer, json_object_get_boolean(val));
                    }
                }
                buffer_json_array_close(sample_buffer);
                buffer_json_array_close(sample_buffer);
            }
            
            // Add column definitions (simplified for sample)
            buffer_json_member_add_object(sample_buffer, "columns");
            
            // Use iterators to avoid variable redefinition in macro
            {
                struct json_object_iterator it = json_object_iter_begin(columns_obj);
                struct json_object_iterator itEnd = json_object_iter_end(columns_obj);
                
                while (!json_object_iter_equal(&it, &itEnd)) {
                    const char *col_key = json_object_iter_peek_name(&it);
                    struct json_object *col_val = json_object_iter_peek_value(&it);
                    
                    buffer_json_member_add_object(sample_buffer, col_key);
                    
                    struct json_object *name_obj = NULL;
                    struct json_object *title_obj = NULL;
                    struct json_object *type_obj = NULL;
                    
                    if (json_object_object_get_ex(col_val, "name", &name_obj)) {
                        buffer_json_member_add_string(sample_buffer, "name", json_object_get_string(name_obj));
                    }
                    
                    if (json_object_object_get_ex(col_val, "title", &title_obj)) {
                        buffer_json_member_add_string(sample_buffer, "title", json_object_get_string(title_obj));
                    }
                    
                    if (json_object_object_get_ex(col_val, "type", &type_obj)) {
                        buffer_json_member_add_string(sample_buffer, "type", json_object_get_string(type_obj));
                    }
                    
                    buffer_json_object_close(sample_buffer);
                    
                    json_object_iter_next(&it);
                }
            }
            
            buffer_json_object_close(sample_buffer);
            
            buffer_json_finalize(sample_buffer);
            
            // Create example column list as JSON array items
            char example_columns[256] = {0};
            size_t example_count = column_count > 3 ? 3 : column_count;
            
            for (size_t i = 0; i < example_count; i++) {
                if (i > 0) strcat(example_columns, ", ");
                char quoted_name[128] = {0};
                snprintf(quoted_name, sizeof(quoted_name), "\"%s\"", column_names[i]);
                strcat(example_columns, quoted_name);
            }
            
            // Construct guidance message with proper JSON structure
            buffer_sprintf(output, 
                "The response is too big (%zu bytes), having %zu rows and %zu columns. "
                "Here is the first row for your reference:\n\n%s\n\n"
                "Next Steps:\n"
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
                "        \"q\": \"*search*pattern*\",\n"
                "        \"conditions\": [\n"
                "            [\"%s\", \">\", 100],\n"
                "            [\"%s\", \"==\", \"value\"]\n"
                "        ]\n"
                "    }\n"
                "}\n"
                "```\n"
                "Filtering options:\n"
                "1. `columns`: Array of column names to include\n"
                "2. `sort_column` and `sort_order`: Sort results by column\n"
                "3. `limit`: Limit number of rows returned\n"
                "4. `q`: Full text search pattern (Netdata simple pattern format)\n"
                "5. `conditions`: Array of [column, operator, value] conditions\n"
                "   - Operators: ==, !=, <>, <, <=, >, >=\n"
                "   - Values can be strings, numbers, or booleans\n",
                result_size, row_count, column_count, 
                buffer_tostring(sample_buffer),
                node_name, function_name, 
                example_columns,  // This will be quoted column names
                column_names[0],
                column_names[0],  // For numeric example
                column_names[0]);
            
            // Clean up
            for (size_t i = 0; i < column_name_count; i++) {
                free(column_names[i]);
            }
            free(column_names);
            buffer_free(sample_buffer);
            json_object_put(json_result);
            
            return output;
        }
        
        // If we can't create a sample (no rows)
        buffer_free(sample_buffer);
        buffer_strcat(output, json_str); // Return original
        json_object_put(json_result);
        return output;
    }
    
    // Process filtering parameters if provided
    if (columns_array || sort_column_param || limit_param >= 0 || q_param || conditions_array) {
        // Determine sort direction
        bool descending = false;
        if (sort_order_param && strcasecmp(sort_order_param, "desc") == 0) {
            descending = true;
        }
        
        // Store selected column indices
        int *selected_indices = NULL;
        size_t selected_count = 0;
        char **selected_names = NULL;
        
        // Prepare full text search pattern if specified
        SIMPLE_PATTERN *search_pattern = NULL;
        if (q_param) {
            search_pattern = string_to_simple_pattern_nocase_substring(q_param);
        }
        
        if (columns_array && json_object_is_type(columns_array, json_type_array)) {
            // Get column names from JSON array
            size_t count = json_object_array_length(columns_array);
            
            // Allocate arrays
            selected_indices = calloc(count, sizeof(int));
            selected_names = calloc(count, sizeof(char*));
            
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
                            selected_indices[selected_count] = idx;
                            selected_names[selected_count] = strdup(col);
                            selected_count++;
                        }
                    }
                }
            }
            
            // Sort indices for proper extraction
            // Simple bubble sort since count is typically small
            for (size_t i = 0; i < selected_count; i++) {
                for (size_t j = i + 1; j < selected_count; j++) {
                    if (selected_indices[i] > selected_indices[j]) {
                        int temp_idx = selected_indices[i];
                        selected_indices[i] = selected_indices[j];
                        selected_indices[j] = temp_idx;
                        
                        char *temp_name = selected_names[i];
                        selected_names[i] = selected_names[j];
                        selected_names[j] = temp_name;
                    }
                }
            }
        } else {
            // If no columns specified, include all
            selected_indices = calloc(column_count, sizeof(int));
            selected_names = calloc(column_count, sizeof(char*));
            
            // Use iterator to avoid macro variable redefinition
            struct json_object_iterator it = json_object_iter_begin(columns_obj);
            struct json_object_iterator itEnd = json_object_iter_end(columns_obj);
            
            while (!json_object_iter_equal(&it, &itEnd)) {
                const char *col_key = json_object_iter_peek_name(&it);
                struct json_object *col_val = json_object_iter_peek_value(&it);
                
                struct json_object *index_obj = NULL;
                if (json_object_object_get_ex(col_val, "index", &index_obj)) {
                    int idx = json_object_get_int(index_obj);
                    selected_indices[selected_count] = idx;
                    selected_names[selected_count] = strdup(col_key);
                    selected_count++;
                }
                
                json_object_iter_next(&it);
            }
            
            // Sort indices
            for (size_t i = 0; i < selected_count; i++) {
                for (size_t j = i + 1; j < selected_count; j++) {
                    if (selected_indices[i] > selected_indices[j]) {
                        int temp_idx = selected_indices[i];
                        selected_indices[i] = selected_indices[j];
                        selected_indices[j] = temp_idx;
                        
                        char *temp_name = selected_names[i];
                        selected_names[i] = selected_names[j];
                        selected_names[j] = temp_name;
                    }
                }
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
            }
        }
        
        // Create new arrays for filtered/sorted data
        struct json_object **rows = calloc(row_count, sizeof(struct json_object*));
        size_t row_idx = 0;
        
        // Copy rows for filtering/sorting, applying filters if specified
        for (size_t i = 0; i < row_count; i++) {
            struct json_object *row = json_object_array_get_idx(data_obj, i);
            if (!row)
                continue;
            
            bool include_row = true;
            
            // Apply full text search if specified
            if (search_pattern && include_row) {
                bool match_found = false;
                
                // Search all values in the row
                for (size_t j = 0; j < json_object_array_length(row); j++) {
                    struct json_object *val = json_object_array_get_idx(row, j);
                    
                    // Only search string values
                    if (val && json_object_is_type(val, json_type_string)) {
                        const char *str_val = json_object_get_string(val);
                        if (str_val && simple_pattern_matches(search_pattern, str_val)) {
                            match_found = true;
                            break;
                        }
                    }
                    // Convert numbers to strings for searching
                    else if (val && (json_object_is_type(val, json_type_int) || 
                                     json_object_is_type(val, json_type_double))) {
                        char num_str[64];
                        if (json_object_is_type(val, json_type_int)) {
                            snprintf(num_str, sizeof(num_str), "%lld", (long long)json_object_get_int64(val));
                        } else {
                            snprintf(num_str, sizeof(num_str), "%g", json_object_get_double(val));
                        }
                        
                        if (simple_pattern_matches(search_pattern, num_str)) {
                            match_found = true;
                            break;
                        }
                    }
                    // Convert booleans to strings for searching
                    else if (val && json_object_is_type(val, json_type_boolean)) {
                        const char *bool_str = json_object_get_boolean(val) ? "true" : "false";
                        if (simple_pattern_matches(search_pattern, bool_str)) {
                            match_found = true;
                            break;
                        }
                    }
                }
                
                include_row = match_found;
            }
            
            // Apply column conditions if specified
            if (conditions_array && include_row && json_object_is_type(conditions_array, json_type_array)) {
                size_t conditions_count = json_object_array_length(conditions_array);
                
                for (size_t j = 0; j < conditions_count; j++) {
                    struct json_object *condition = json_object_array_get_idx(conditions_array, j);
                    
                    // Each condition should be an array of [column, operator, value]
                    if (!condition || !json_object_is_type(condition, json_type_array) || 
                        json_object_array_length(condition) != 3) {
                        continue;
                    }
                    
                    struct json_object *col_name_obj = json_object_array_get_idx(condition, 0);
                    struct json_object *operator_obj = json_object_array_get_idx(condition, 1);
                    struct json_object *value_obj = json_object_array_get_idx(condition, 2);
                    
                    if (!col_name_obj || !json_object_is_type(col_name_obj, json_type_string) ||
                        !operator_obj || !json_object_is_type(operator_obj, json_type_string) ||
                        !value_obj) {
                        continue;
                    }
                    
                    // Get column name and its index
                    const char *col_name = json_object_get_string(col_name_obj);
                    const char *op_str = json_object_get_string(operator_obj);
                    
                    // Find column in column definitions
                    struct json_object *col_obj = NULL;
                    int col_idx = -1;
                    
                    if (json_object_object_get_ex(columns_obj, col_name, &col_obj)) {
                        struct json_object *index_obj = NULL;
                        if (json_object_object_get_ex(col_obj, "index", &index_obj)) {
                            col_idx = json_object_get_int(index_obj);
                        }
                    }
                    
                    // Skip this condition if column not found
                    if (col_idx < 0) {
                        continue;
                    }
                    
                    // Get the value from the row
                    struct json_object *row_val = json_object_array_get_idx(row, col_idx);
                    
                    // Handle null values
                    if (!row_val) {
                        include_row = false;
                        break;
                    }
                    
                    bool condition_match = false;
                    
                    // Compare based on types
                    if (json_object_is_type(row_val, json_type_string) && 
                        json_object_is_type(value_obj, json_type_string)) {
                        
                        const char *row_str = json_object_get_string(row_val);
                        const char *val_str = json_object_get_string(value_obj);
                        int cmp = strcmp(row_str, val_str);
                        
                        if (strcmp(op_str, "==") == 0) {
                            condition_match = (cmp == 0);
                        } else if (strcmp(op_str, "!=") == 0 || strcmp(op_str, "<>") == 0) {
                            condition_match = (cmp != 0);
                        } else if (strcmp(op_str, "<") == 0) {
                            condition_match = (cmp < 0);
                        } else if (strcmp(op_str, "<=") == 0) {
                            condition_match = (cmp <= 0);
                        } else if (strcmp(op_str, ">") == 0) {
                            condition_match = (cmp > 0);
                        } else if (strcmp(op_str, ">=") == 0) {
                            condition_match = (cmp >= 0);
                        }
                    }
                    else if ((json_object_is_type(row_val, json_type_int) || 
                              json_object_is_type(row_val, json_type_double)) &&
                             (json_object_is_type(value_obj, json_type_int) || 
                              json_object_is_type(value_obj, json_type_double))) {
                        
                        double row_num = json_object_is_type(row_val, json_type_int) ? 
                                       (double)json_object_get_int64(row_val) : 
                                       json_object_get_double(row_val);
                                       
                        double val_num = json_object_is_type(value_obj, json_type_int) ? 
                                       (double)json_object_get_int64(value_obj) : 
                                       json_object_get_double(value_obj);
                        
                        if (strcmp(op_str, "==") == 0) {
                            condition_match = (row_num == val_num);
                        } else if (strcmp(op_str, "!=") == 0 || strcmp(op_str, "<>") == 0) {
                            condition_match = (row_num != val_num);
                        } else if (strcmp(op_str, "<") == 0) {
                            condition_match = (row_num < val_num);
                        } else if (strcmp(op_str, "<=") == 0) {
                            condition_match = (row_num <= val_num);
                        } else if (strcmp(op_str, ">") == 0) {
                            condition_match = (row_num > val_num);
                        } else if (strcmp(op_str, ">=") == 0) {
                            condition_match = (row_num >= val_num);
                        }
                    }
                    else if (json_object_is_type(row_val, json_type_boolean) && 
                             json_object_is_type(value_obj, json_type_boolean)) {
                        
                        bool row_bool = json_object_get_boolean(row_val);
                        bool val_bool = json_object_get_boolean(value_obj);
                        
                        if (strcmp(op_str, "==") == 0) {
                            condition_match = (row_bool == val_bool);
                        } else if (strcmp(op_str, "!=") == 0 || strcmp(op_str, "<>") == 0) {
                            condition_match = (row_bool != val_bool);
                        }
                    }
                    
                    if (!condition_match) {
                        include_row = false;
                        break;
                    }
                }
            }
            
            if (include_row) {
                rows[row_idx++] = row;
            }
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
                        // Compare based on type
                        if (json_object_is_type(val_i, json_type_string) && 
                            json_object_is_type(val_j, json_type_string)) {
                            int cmp = strcmp(json_object_get_string(val_i), json_object_get_string(val_j));
                            should_swap = descending ? (cmp < 0) : (cmp > 0);
                        }
                        else if ((json_object_is_type(val_i, json_type_int) || json_object_is_type(val_i, json_type_double)) &&
                                 (json_object_is_type(val_j, json_type_int) || json_object_is_type(val_j, json_type_double))) {
                            // Convert both to double for comparison
                            double i_val = json_object_is_type(val_i, json_type_int) ? 
                                          (double)json_object_get_int64(val_i) : json_object_get_double(val_i);
                            double j_val = json_object_is_type(val_j, json_type_int) ? 
                                          (double)json_object_get_int64(val_j) : json_object_get_double(val_j);
                            
                            should_swap = descending ? (i_val < j_val) : (i_val > j_val);
                        }
                        else if (json_object_is_type(val_i, json_type_boolean) && 
                                 json_object_is_type(val_j, json_type_boolean)) {
                            bool i_val = json_object_get_boolean(val_i);
                            bool j_val = json_object_get_boolean(val_j);
                            
                            should_swap = descending ? (i_val && !j_val) : (!i_val && j_val);
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
            struct json_object *col_obj = NULL;
            if (json_object_object_get_ex(columns_obj, selected_names[i], &col_obj)) {
                struct json_object *col_copy = json_object_new_object();
                
                // Copy all properties
                struct json_object_iterator it = json_object_iter_begin(col_obj);
                struct json_object_iterator itEnd = json_object_iter_end(col_obj);
                
                while (!json_object_iter_equal(&it, &itEnd)) {
                    const char *field_key = json_object_iter_peek_name(&it);
                    struct json_object *field_val = json_object_iter_peek_value(&it);
                    
                    // Update index to match new position
                    if (strcmp(field_key, "index") == 0) {
                        json_object_object_add(col_copy, field_key, json_object_new_int((int)i));
                    } else {
                        json_object_object_add(col_copy, field_key, json_object_get(field_val));
                    }
                    
                    json_object_iter_next(&it);
                }
                
                json_object_object_add(filtered_columns, selected_names[i], col_copy);
            }
        }
        
        // Create filtered data rows
        for (size_t i = 0; i < limit; i++) {
            struct json_object *row = rows[i];
            struct json_object *new_row = json_object_new_array();
            
            // Extract only selected columns
            for (size_t j = 0; j < selected_count; j++) {
                struct json_object *val = json_object_array_get_idx(row, selected_indices[j]);
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
        
        // Convert to string
        const char *filtered_json = json_object_to_json_string_ext(filtered_result, JSON_C_TO_STRING_PRETTY);
        buffer_strcat(output, filtered_json);
        
        // Clean up
        json_object_put(filtered_result);
        free(rows);
        
        for (size_t i = 0; i < selected_count; i++) {
            free(selected_names[i]);
        }
        free(selected_names);
        free(selected_indices);
        
        // Free search pattern if created
        if (search_pattern) {
            simple_pattern_free(search_pattern);
        }
    } else {
        // No filtering, return original
        buffer_strcat(output, json_str);
    }
    
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
    BUFFER *result_buffer = buffer_create(0, NULL);
    
    // Create a unique transaction ID
    char transaction[UUID_STR_LEN];
    nd_uuid_t transaction_uuid;
    uuid_generate(transaction_uuid);
    uuid_unparse_lower(transaction_uuid, transaction);

#if 0
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

    if (ret != 200) {
        buffer_sprintf(mcpc->error, "Failed to execute function: %s on node %s, error code %d", 
                       function_name, node_name, ret);
        buffer_free(result_buffer);
        return MCP_RC_ERROR;
    }

    // Extract filtering parameters
    const char *sort_column_param = NULL;
    const char *sort_order_param = NULL;
    int limit_param = -1;
    const char *q_param = NULL;
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
    
    // Get q parameter (full text search pattern)
    if (json_object_object_get_ex(params, "q", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "q", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            const char *value = json_object_get_string(obj);
            // Only set if not empty
            if (value && *value) {
                q_param = value;
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
    BUFFER *processed_result = mcp_process_table_result(
        result_buffer, 
        columns_array, 
        sort_column_param,
        sort_order_param,
        limit_param,
        q_param,
        conditions_array,
        max_size_threshold,
        function_name,
        node_name
    );
    
    // Initialize success response
    mcp_init_success_result(mcpc, id);
    {
        // Start building content array for the result
        buffer_json_member_add_array(mcpc->result, "content");
        {
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

    buffer_free(processed_result);
    buffer_free(result_buffer);
    
    return MCP_RC_OK;
}