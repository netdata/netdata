// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Metrics Query Tool
 *
 * This tool allows querying metrics data via the Model Context Protocol.
 * It provides an interface to the data query engine similar to the API v2 data endpoint.
 * 
 * Query Process:
 * 1. The query engine first determines all unique time-series to query by filtering based on context, nodes, 
 *    time-frame, and other supplied filters.
 * 
 * 2. It then queries each time-series, automatically applying over-time-aggregation. For example, if the 
 *    database has 1000 points for a time series and you request 10 points, the query engine reduces the 
 *    1000 points to 10 using the time_group aggregation function (average, max, min, etc.).
 * 
 * 3. After time aggregation, the query engine applies the group_by aggregation across metrics.
 *    For example, if querying disk I/O for 10 disks from 2 nodes with 2 dimensions each (read/write),
 *    you have 40 unique time-series. With group_by=dimension, the engine would:
 *    - Aggregate all 20 'read' dimensions (from all disks across all nodes) into a single 'read' dimension 
 *    - Aggregate all 20 'write' dimensions (from all disks across all nodes) into a single 'write' dimension
 *    - Use the specified aggregation function (sum, min, max, average) for this cross-metric aggregation
 * 
 * 4. The result will contain only the grouped dimensions, but with rich metadata:
 *    - Each data point contains: timestamp, aggregated value, anomaly rate, and quality flags
 *    - Quality flags indicate whether original data had gaps or counter overflows
 * 
 * 5. When 'jsonwrap' is included in options, the response includes comprehensive statistics about all 
 *    facets of the query, providing aggregated min, max, average, anomaly rate, and volume contribution 
 *    percentages per node, instance, dimension, and label.
 */

#include "mcp-tools-metrics-query.h"
#include "mcp-tools.h"
#include "mcp-time-utils.h"
#include "web/api/formatters/rrd2json.h"

/**
 * Convert structured labels object to Netdata's pipe-delimited format
 * 
 * MCP format (structured JSON):
 * {
 *   "disk_type": ["ssd", "nvme"],    // OR between values
 *   "mount_point": ["/", "/home"]    // AND between different keys
 * }
 * 
 * Netdata format (string):
 * "disk_type:ssd|disk_type:nvme|mount_point:/|mount_point:/home"
 * 
 * The backend automatically ORs values with the same key and ANDs different keys.
 * 
 * @param labels_obj JSON object containing structured labels
 * @param output_buffer Buffer to store the converted string
 * @return 0 on success, -1 on error
 */
static int convert_structured_labels_to_string(struct json_object *labels_obj, BUFFER *output_buffer) {
    if (!labels_obj || !output_buffer) {
        return -1;
    }
    
    if (!json_object_is_type(labels_obj, json_type_object)) {
        return -1;
    }
    
    buffer_flush(output_buffer);
    
    int first_pair = 1;
    struct json_object_iterator it = json_object_iter_begin(labels_obj);
    struct json_object_iterator itEnd = json_object_iter_end(labels_obj);
    
    while (!json_object_iter_equal(&it, &itEnd)) {
        const char *key = json_object_iter_peek_name(&it);
        struct json_object *value_obj = json_object_iter_peek_value(&it);
        
        if (json_object_is_type(value_obj, json_type_array)) {
            // Handle array of values
            int array_len = json_object_array_length(value_obj);
            for (int i = 0; i < array_len; i++) {
                struct json_object *array_item = json_object_array_get_idx(value_obj, i);
                if (json_object_is_type(array_item, json_type_string)) {
                    if (!first_pair) {
                        buffer_strcat(output_buffer, "|");
                    }
                    buffer_sprintf(output_buffer, "%s:%s", key, json_object_get_string(array_item));
                    first_pair = 0;
                }
            }
        } else if (json_object_is_type(value_obj, json_type_string)) {
            // Handle single string value
            if (!first_pair) {
                buffer_strcat(output_buffer, "|");
            }
            buffer_sprintf(output_buffer, "%s:%s", key, json_object_get_string(value_obj));
            first_pair = 0;
        }
        
        json_object_iter_next(&it);
    }
    
    return 0;
}

// JSON schema for the metrics query tool
void mcp_tool_metrics_query_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Query Metrics Data");

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    // Selection parameters
    buffer_json_member_add_object(buffer, "nodes");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Nodes Pattern");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on nodes to include in the query.\n"
                                                             "If no nodes are specified, all nodes having data for the context in the specified time-frame will be queried."
                                                             "Examples: `node1|node2|node3` or `node*` or `*db*|*dns*`\n"
                                                             "To discover available nodes, first use the " MCP_TOOL_LIST_NODES " tool.\n"
                                                             "If no nodes are specified, all nodes having data for the context in the specified time-frame will be queried.");
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // nodes

    buffer_json_member_add_object(buffer, "context");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Context Name");
        buffer_json_member_add_string(buffer, "description", "The specific context name to query. This parameter is required.\n"
                                                             "To discover available contexts, first use the " MCP_TOOL_LIST_METRICS " tool.");
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // context


    buffer_json_member_add_object(buffer, "instances");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Instances Pattern");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on instances to include in the query.\n"
                                                             "Use pipe (|) to separate multiple patterns. Examples: 'eth0|eth1', '*sda*|*nvme*', 'cpu0|cpu1|cpu2'\n"
                                                             "If no instances are specified, all instances of the context are queried.\n"
                                                             "Note: Instance behavior varies by collector type - see warning in response when used.");
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // instances

    buffer_json_member_add_object(buffer, "dimensions");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Dimensions Pattern");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on dimensions to include in the query.\n"
                                                             "Use pipe (|) to separate multiple patterns. Examples: 'read|write', 'in|out', 'used|free|cached'\n"
                                                             "If no dimensions are specified, all dimensions of the context are queried.");
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // dimensions

    buffer_json_member_add_object(buffer, "labels");
    {
        buffer_json_member_add_array(buffer, "oneOf");
        
        // Option 1: String format
        buffer_json_add_array_item_object(buffer);
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Labels Filter (String Format)");
        buffer_json_member_add_string(buffer, "description", "Filter using pipe-delimited format: 'key1:value1|key1:value2|key2:value3'");
        buffer_json_object_close(buffer);
        
        // Option 2: Structured object format
        buffer_json_add_array_item_object(buffer);
        buffer_json_member_add_string(buffer, "type", "object");
        buffer_json_member_add_string(buffer, "title", "Labels Filter (Structured Format)");
        buffer_json_member_add_string(buffer, "description", "Filter using structured format where each key maps to a value or array of values. "
                                                             "Values in the same array are ORed, different keys are ANDed. "
                                                             "Example: {\"disk_type\": [\"ssd\", \"nvme\"], \"mount_point\": [\"/\"]}");
        buffer_json_member_add_object(buffer, "additionalProperties");
        buffer_json_member_add_array(buffer, "oneOf");
        buffer_json_add_array_item_object(buffer);
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_object_close(buffer);
        buffer_json_add_array_item_object(buffer);
        buffer_json_member_add_string(buffer, "type", "array");
        buffer_json_member_add_object(buffer, "items");
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_object_close(buffer);
        buffer_json_object_close(buffer);
        buffer_json_array_close(buffer); // oneOf
        buffer_json_object_close(buffer); // additionalProperties
        buffer_json_object_close(buffer);
        
        buffer_json_array_close(buffer); // oneOf
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // labels

    buffer_json_member_add_object(buffer, "alerts");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Alerts Filter");
        buffer_json_member_add_string(buffer, "description", "Filter for charts having specified alert states.");
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // alerts
    
    // Add cardinality limit
    mcp_schema_params_add_cardinality_limit(buffer, NULL, true);

    // Time parameters
    mcp_schema_params_add_time_window(buffer, "data", true);

    buffer_json_member_add_object(buffer, "points");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Data Points");
        buffer_json_member_add_string(buffer, "description", "Number of data points to return.");
        buffer_json_member_add_uint64(buffer, "default", 60);
    }
    buffer_json_object_close(buffer); // points

    buffer_json_member_add_object(buffer, "timeout");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Timeout");
        buffer_json_member_add_string(buffer, "description", "Query timeout in milliseconds.");
        buffer_json_member_add_uint64(buffer, "default", 30000);
    }
    buffer_json_object_close(buffer); // timeout

    buffer_json_member_add_object(buffer, "options");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Query Options");
        buffer_json_member_add_string(buffer, "description", "Space-separated list of additional query options:\n\n"
                                                             "'percentage': Return values as percentages of total\n\n"
                                                             "'absolute' or 'absolute-sum': Return absolute values for stacked charts\n\n"
                                                             "'display-absolute': Convert percentage values to absolute before application of grouping functions\n\n"
                                                             "'all-dimensions': Include all dimensions, even those with just zero values\n\n"
                                                             "Example: 'absolute percentage'");
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // options

    // Time grouping
    buffer_json_member_add_object(buffer, "time_group");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Time Grouping Method");
        buffer_json_member_add_string(buffer, "description", "Method to group data points over time. The 'extremes' method returns the maximum value for positive numbers and the minimum value for negative numbers, which is particularly useful for showing the highest peaks in both directions on charts.");
        buffer_json_member_add_string(buffer, "default", "average");
        
        // Define enum of possible values
        buffer_json_member_add_array(buffer, "enum");
        buffer_json_add_array_item_string(buffer, "average");  // "avg" and "mean" are aliases
        buffer_json_add_array_item_string(buffer, "min");
        buffer_json_add_array_item_string(buffer, "max");
        buffer_json_add_array_item_string(buffer, "sum");
        buffer_json_add_array_item_string(buffer, "incremental-sum");  // "incremental_sum" is an alias
        buffer_json_add_array_item_string(buffer, "median");
        buffer_json_add_array_item_string(buffer, "trimmed-mean");
        buffer_json_add_array_item_string(buffer, "trimmed-median");
        buffer_json_add_array_item_string(buffer, "percentile");  // requires time_group_options parameter
        buffer_json_add_array_item_string(buffer, "stddev");  // standard deviation
        buffer_json_add_array_item_string(buffer, "coefficient-of-variation");    // relative standard deviation (cv)
        buffer_json_add_array_item_string(buffer, "ema");  // exponential moving average (alias "ses" or "ewma")
        buffer_json_add_array_item_string(buffer, "des");  // double exponential smoothing
        buffer_json_add_array_item_string(buffer, "countif");  // requires time_group_options parameter
        buffer_json_add_array_item_string(buffer, "extremes");  // for each time frame, returns max for positive values and min for negative values
        buffer_json_array_close(buffer);
    }
    buffer_json_object_close(buffer); // time_group

    buffer_json_member_add_object(buffer, "time_group_options");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Time Group Options");
        buffer_json_member_add_string(buffer, "description", "Additional options for time grouping. For 'percentile', specify a percentage (0-100). "
                                                             "For 'countif', specify a comparison operator and value (e.g., '>0', '=0', '!=0', '<=10').");
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // time_group_options

    // Tier selection
    buffer_json_member_add_object(buffer, "tier");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Storage Tier");
        buffer_json_member_add_string(buffer, "description", "Storage tier to query from.\n"
                                                             "If not specified, Netdata will automatically pick the best tier based on the time-frame and points requested.");
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // tier

    // Group by parameters
    buffer_json_member_add_object(buffer, "group_by");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Group By");
        buffer_json_member_add_string(buffer, "description", "Specifies how to group metrics across different time-series. Supports following options which can be combined (comma-separated):\n\n"
                                                             "'dimension': Groups metrics by dimension name across all instances/nodes. If monitoring disks having reads and writes, this will produce the aggregate read and writes for all disks of all nodes.\n\n"
                                                             "'instance': Groups metrics by instance across all nodes. If monitoring disks, the result will be 1 metric per disk, aggregating its reads and writes.\n\n"
                                                             "'node': Groups metrics from the same node. If monitoring disks, the result will be 1 metric per node, aggregating its reads and writes across all its disks.\n\n"
                                                             "'label': Groups metrics with the same value for the specified label (requires group_by_label). Example: if the label has 2 values: physical and virtual, the result will be 2 metrics: physical and virtual.\n\n"
                                                             "Multiple groupings can be combined, e.g., 'node,dimension' will produce separate read and write metrics for each node.");
        buffer_json_member_add_string(buffer, "default", "dimension");
    }
    buffer_json_object_close(buffer); // group_by

    buffer_json_member_add_object(buffer, "group_by_label");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Group By Label");
        buffer_json_member_add_string(buffer, "description", "When group_by includes 'label', this parameter specifies which label key to group by. For example, if metrics have a 'disk_type' label with values like 'ssd' or 'hdd', setting group_by_label to 'disk_type' would aggregate metrics separately for SSDs and HDDs.");
        buffer_json_member_add_string(buffer, "default", NULL);
    }
    buffer_json_object_close(buffer); // group_by_label

    buffer_json_member_add_object(buffer, "aggregation");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Aggregation Function");
        buffer_json_member_add_string(buffer, "description", "Function to use when aggregating grouped metrics:\n\n"
                                                             "'sum': Sum of all grouped metrics (useful for additive metrics like bytes transferred, operations, etc.)\n\n"
                                                             "'min': Minimum value among all grouped metrics (useful for finding best performance metrics)\n\n"
                                                             "'max': Maximum value among all grouped metrics (useful for finding worst performance metrics, peak resource usage)\n\n"
                                                             "'average': Average of all grouped metrics (useful for utilization and most ratio metrics)\n\n"
                                                             "'percentage': Expresses each grouped metric as a percentage of its group's total (useful for seeing proportional contributions)\n\n"
                                                             "'extremes': For each group, shows maximum value for positive metrics and minimum value for negative metrics (useful for showing both highest peaks and lowest dips)");
        buffer_json_member_add_string(buffer, "default", "average");
        
        // Define enum of possible values
        buffer_json_member_add_array(buffer, "enum");
        buffer_json_add_array_item_string(buffer, "sum");
        buffer_json_add_array_item_string(buffer, "min");
        buffer_json_add_array_item_string(buffer, "max");
        buffer_json_add_array_item_string(buffer, "average");
        buffer_json_add_array_item_string(buffer, "percentage");
        buffer_json_add_array_item_string(buffer, "extremes");
        buffer_json_array_close(buffer);
    }
    buffer_json_object_close(buffer); // aggregation

    buffer_json_object_close(buffer); // properties

    // Required fields
    buffer_json_member_add_array(buffer, "required");
    buffer_json_add_array_item_string(buffer, "context");
    buffer_json_add_array_item_string(buffer, "after");
    buffer_json_add_array_item_string(buffer, "before");
    buffer_json_add_array_item_string(buffer, "points");
    buffer_json_add_array_item_string(buffer, "time_group");
    buffer_json_add_array_item_string(buffer, "group_by");
    buffer_json_add_array_item_string(buffer, "aggregation");
    buffer_json_add_array_item_string(buffer, "cardinality_limit");
    buffer_json_array_close(buffer);
    
    buffer_json_object_close(buffer); // inputSchema
}

// Structure to hold interruption data
typedef struct {
    MCP_CLIENT *mcpc;
    MCP_REQUEST_ID id;
} mcp_query_interrupt_data;

// Interrupt callback for query execution
static bool mcp_query_interrupt_callback(void *data) {
    mcp_query_interrupt_data *int_data = (mcp_query_interrupt_data *)data;
    
    // Check if the MCP client is still valid and connected
    if (!int_data || !int_data->mcpc)
        return false;
        
    // Real implementations might check for client disconnection or timeout
    // Here we're just returning false to indicate "no interrupt"
    return false;
}

// Extract string parameter from json object
static const char *extract_string_param(struct json_object *params, const char *name) {
    if (!params)
        return NULL;
        
    struct json_object *obj = NULL;
    if (!json_object_object_get_ex(params, name, &obj) || !obj)
        return NULL;
        
    if (!json_object_is_type(obj, json_type_string))
        return NULL;
        
    return json_object_get_string(obj);
}


// Extract size_t parameter from json object
static size_t extract_size_param(struct json_object *params, const char *name, size_t default_val) {
    if (!params)
        return default_val;
        
    struct json_object *obj = NULL;
    if (!json_object_object_get_ex(params, name, &obj) || !obj)
        return default_val;
        
    if (json_object_is_type(obj, json_type_int))
        return (size_t)json_object_get_int64(obj);
        
    if (json_object_is_type(obj, json_type_string)) {
        const char *val_str = json_object_get_string(obj);
        if (val_str && *val_str)
            return str2u(val_str);
    }
    
    return default_val;
}

// Execute the metrics query
MCP_RETURN_CODE mcp_tool_metrics_query_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || id == 0)
        return MCP_RC_ERROR;

    buffer_flush(mcpc->result);

    usec_t received_ut = now_monotonic_usec();
    
    // Extract parameters
    const char *nodes = extract_string_param(params, "nodes");
    const char *context = extract_string_param(params, "context");
    
    // Validate required parameters with detailed error messages
    if (!context || !*context) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'context'. This parameter specifies which metric context to query. Use the list_metric_contexts tool to discover available contexts.");
        return MCP_RC_BAD_REQUEST;
    }
    
    // Check if all required parameters are provided
    struct json_object *after_obj = NULL, *before_obj = NULL, *points_obj = NULL, *time_group_obj = NULL;
    struct json_object *group_by_obj = NULL, *aggregation_obj = NULL;
    
    if (!json_object_object_get_ex(params, "after", &after_obj) || !after_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'after'. This parameter defines the start time for your query (epoch timestamp in seconds or negative value for relative time).");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_object_get_ex(params, "before", &before_obj) || !before_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'before'. This parameter defines the end time for your query (epoch timestamp in seconds or negative value for relative time).");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_object_get_ex(params, "points", &points_obj) || !points_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'points'. This parameter defines how many data points to return in your result set (e.g., 60 for minute-level granularity in an hour).");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_object_get_ex(params, "time_group", &time_group_obj) || !time_group_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'time_group'. This parameter defines how to aggregate data points over time (e.g., 'average', 'min', 'max', 'sum').");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_object_get_ex(params, "group_by", &group_by_obj) || !group_by_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'group_by'. This parameter defines how to group metrics (e.g., 'dimension', 'instance', 'node', or combinations like 'dimension,node').");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_object_get_ex(params, "aggregation", &aggregation_obj) || !aggregation_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'aggregation'. This parameter defines the function to use when aggregating metrics (e.g., 'sum', 'min', 'max', 'average').");
        return MCP_RC_BAD_REQUEST;
    }
    
    struct json_object *cardinality_limit_obj = NULL;
    if (!json_object_object_get_ex(params, "cardinality_limit", &cardinality_limit_obj) || !cardinality_limit_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'cardinality_limit'. This parameter limits the number of items returned to keep response sizes manageable (default: %d).", MCP_DATA_CARDINALITY_LIMIT);
        return MCP_RC_BAD_REQUEST;
    }
    
    // Get time_group value to check if it's percentile or countif
    const char *time_group_str = NULL;
    if (json_object_is_type(time_group_obj, json_type_string)) {
        time_group_str = json_object_get_string(time_group_obj);
        
        // Check if time_group_options is required based on time_group
        if (time_group_str && (
            strcmp(time_group_str, "percentile") == 0 || 
            strcmp(time_group_str, "countif") == 0)) {
            
            struct json_object *time_group_options_obj = NULL;
            if (!json_object_object_get_ex(params, "time_group_options", &time_group_options_obj) || !time_group_options_obj) {
                if (strcmp(time_group_str, "percentile") == 0) {
                    buffer_sprintf(mcpc->error, "Missing required parameter 'time_group_options' when using time_group='percentile'. You must specify a percentage value between 0-100 (e.g., '95' for 95th percentile).");
                } else {
                    buffer_sprintf(mcpc->error, "Missing required parameter 'time_group_options' when using time_group='countif'. You must specify a comparison operator and value (e.g., '>0', '=0', '!=0', '<=10').");
                }
                return MCP_RC_BAD_REQUEST;
            }
        }
    }
    const char *instances = extract_string_param(params, "instances");
    const char *dimensions = extract_string_param(params, "dimensions");
    
    // Handle labels - can be either string or structured object
    const char *labels = NULL;
    BUFFER *labels_buffer = NULL;
    struct json_object *labels_obj = NULL;
    
    if (json_object_object_get_ex(params, "labels", &labels_obj) && labels_obj) {
        if (json_object_is_type(labels_obj, json_type_string)) {
            // Direct string format
            labels = json_object_get_string(labels_obj);
        } else if (json_object_is_type(labels_obj, json_type_object)) {
            // Structured format - convert to string
            labels_buffer = buffer_create(256, NULL);
            if (convert_structured_labels_to_string(labels_obj, labels_buffer) == 0) {
                labels = buffer_tostring(labels_buffer);
            } else {
                buffer_free(labels_buffer);
                buffer_sprintf(mcpc->error, "Failed to convert structured labels to string format");
                return MCP_RC_BAD_REQUEST;
            }
        }
    }
    
    const char *alerts = extract_string_param(params, "alerts");
    
    // Time parameters
    time_t after = mcp_extract_time_param(params, "after", MCP_DEFAULT_AFTER_TIME);
    time_t before = mcp_extract_time_param(params, "before", MCP_DEFAULT_BEFORE_TIME);
    
    // Validate time range
    if (after == 0 && before == 0) {
        buffer_sprintf(mcpc->error, "Invalid time range: both 'after' and 'before' cannot be zero. Use negative values for relative times (e.g., after=-3600, before=-0 for the last hour) or specific timestamps for absolute times.");
        return MCP_RC_BAD_REQUEST;
    }
    
    // Check if after is later than before (when both are absolute timestamps)
    if (after > 0 && before > 0 && after >= before) {
        buffer_sprintf(mcpc->error, "Invalid time range: 'after' (%lld) must be earlier than 'before' (%lld). The query time range must be at least 1 second.", (long long)after, (long long)before);
        return MCP_RC_BAD_REQUEST;
    }
    
    // No need to check aggregation_obj here - we already have default in group_by struct
    
    // Other parameters
    size_t points = extract_size_param(params, "points", 0);
    size_t cardinality_limit = extract_size_param(params, "cardinality_limit", MCP_DATA_CARDINALITY_LIMIT);

    // Check if points is more than 1000
    if (points < 1) {
        buffer_sprintf(mcpc->error,
                       "Too few data points requested: %zu. The minimum allowed is 1 point.",
                       points);
        return MCP_RC_BAD_REQUEST;
    }

    // Check if points is more than 1000
    if (points > 1000) {
        buffer_sprintf(mcpc->error, 
            "Too many data points requested: %zu. The maximum allowed is 1000 points. Please reduce the 'points' parameter value to 1000 or less.\n"
            "This limit helps reduce response size and save context space when used with AI assistants.",
            points);
        return MCP_RC_BAD_REQUEST;
    }
    
    int timeout = (int)extract_size_param(params, "timeout", 0);
    
    const char *options_str = extract_string_param(params, "options");
    RRDR_OPTIONS options = 0;
    if (options_str && *options_str)
        options |= rrdr_options_parse(options_str);
    
    // Time grouping
    RRDR_TIME_GROUPING time_group = RRDR_GROUPING_AVERAGE;
    if (time_group_str && *time_group_str)
        time_group = time_grouping_parse(time_group_str, RRDR_GROUPING_AVERAGE);
    
    const char *time_group_options = extract_string_param(params, "time_group_options");
    
    // Tier selection (give an invalid default to now the caller added a tier to the query)
    size_t tier = extract_size_param(params, "tier", nd_profile.storage_tiers + 1);
    if (tier < nd_profile.storage_tiers)
        options |= RRDR_OPTION_SELECTED_TIER;
    else
        tier = 0;
    
    // Group by parameters (simplified - in real implementation handle multiple passes)
    struct group_by_pass group_by[MAX_QUERY_GROUP_BY_PASSES] = {
        {
            .group_by = RRDR_GROUP_BY_DIMENSION,
            .group_by_label = NULL,
            .aggregation = RRDR_GROUP_BY_FUNCTION_AVERAGE,
        },
    };
    
    const char *group_by_str = extract_string_param(params, "group_by");
    if (group_by_str && *group_by_str)
        group_by[0].group_by = group_by_parse(group_by_str);
    
    const char *group_by_label = extract_string_param(params, "group_by_label");
    if (group_by_label && *group_by_label) {
        group_by[0].group_by_label = (char *)group_by_label;
        group_by[0].group_by |= RRDR_GROUP_BY_LABEL;
    }
    
    const char *aggregation_str = extract_string_param(params, "aggregation");
    if (aggregation_str && *aggregation_str)
        group_by[0].aggregation = group_by_aggregate_function_parse(aggregation_str);
    else
        group_by[0].aggregation = RRDR_GROUP_BY_FUNCTION_AVERAGE; // Default to average if not specified
    
    if (group_by[0].group_by == RRDR_GROUP_BY_NONE)
        group_by[0].group_by = RRDR_GROUP_BY_DIMENSION;
    
    // Create interrupt callback data
    mcp_query_interrupt_data interrupt_data = {
        .mcpc = mcpc,
        .id = id
    };
    
    // Prepare query target request
    QUERY_TARGET_REQUEST qtr = {
        .version = 3,
        .scope_nodes = nodes,     // Use nodes as scope_nodes
        .scope_contexts = context, // Use the single context as scope_contexts
        .after = after,
        .before = before,
        .host = NULL,
        .st = NULL,
        .nodes = NULL,              // Don't use nodes parameter here (we use scope_nodes)
        .contexts = NULL,           // Don't use contexts parameter here (we use scope_contexts)
        .instances = instances,
        .dimensions = dimensions,
        .alerts = alerts,
        .timeout_ms = timeout,
        .points = points,
        .format = DATASOURCE_JSON2,
        .options = options |
                   RRDR_OPTION_ABSOLUTE | RRDR_OPTION_JSON_WRAP | RRDR_OPTION_RETURN_JWAR |
                   RRDR_OPTION_VIRTUAL_POINTS | RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_NONZERO |
                   RRDR_OPTION_MINIFY | RRDR_OPTION_MINIMAL_STATS | RRDR_OPTION_LONG_JSON_KEYS |
                   RRDR_OPTION_MCP_INFO | RRDR_OPTION_RFC3339,
        .time_group_method = time_group,
        .time_group_options = time_group_options,
        .resampling_time = 0,
        .tier = tier,
        .chart_label_key = NULL,
        .labels = labels,
        .query_source = QUERY_SOURCE_API_DATA,
        .priority = STORAGE_PRIORITY_NORMAL,
        .received_ut = received_ut,
        .cardinality_limit = cardinality_limit,
        
        .interrupt_callback = mcp_query_interrupt_callback,
        .interrupt_callback_data = &interrupt_data,
        
        .transaction = NULL, // No transaction for MCP
    };
    
    // Copy group_by structures
    for (size_t g = 0; g < MAX_QUERY_GROUP_BY_PASSES; g++)
        qtr.group_by[g] = group_by[g];
    
    // Create query target
    QUERY_TARGET *qt = query_target_create(&qtr);
    if (!qt) {
        buffer_sprintf(mcpc->error, "Failed to prepare the query.");
        if (labels_buffer) buffer_free(labels_buffer);
        return MCP_RC_INTERNAL_ERROR;
    }

    // Create a temporary buffer for the query result
    CLEAN_BUFFER *tmp_buffer = buffer_create(0, NULL);
    
    // Prepare onewayalloc for query execution
    ONEWAYALLOC *owa = onewayalloc_create(0);
    
    // Execute the query and get the data
    time_t latest_timestamp = 0;
    int ret = data_query_execute(owa, tmp_buffer, qt, &latest_timestamp);
    
    // Clean up
    query_target_release(qt);
    onewayalloc_destroy(owa);
    
    if (ret != HTTP_RESP_OK) {
        buffer_flush(mcpc->result);
        const char *error_desc = "unknown error";
        
        // Map common HTTP error codes to more descriptive messages
        switch (ret) {
            case HTTP_RESP_BAD_REQUEST:
                error_desc = "bad request parameters";
                break;
            case HTTP_RESP_NOT_FOUND:
                error_desc = "context or metrics not found";
                break;
            case HTTP_RESP_GATEWAY_TIMEOUT:
            case HTTP_RESP_SERVICE_UNAVAILABLE:
                error_desc = "timeout or service unavailable";
                break;
            case HTTP_RESP_INTERNAL_SERVER_ERROR:
                error_desc = "internal server error";
                break;
            default:
                break;
        }
        
        buffer_sprintf(mcpc->error, "Failed to execute query: %s (http error code: %d). The context '%s' might not exist, or no data is available for the specified time range.",
                      error_desc, ret, context);
        if (labels_buffer) buffer_free(labels_buffer);
        return MCP_RC_INTERNAL_ERROR;
    }
    
    // Check if instance filtering or grouping is used
    bool using_instances = (instances && *instances) || 
                          (group_by[0].group_by & RRDR_GROUP_BY_INSTANCE);
    
    // Return the raw query engine response as-is
    mcp_init_success_result(mcpc, id);
    {
        buffer_json_member_add_array(mcpc->result, "content");
        {
            // Main result content
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");
                buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(tmp_buffer));
            }
            buffer_json_object_close(mcpc->result);
            
            // Add instance usage warning if applicable
            if (using_instances) {
                buffer_json_add_array_item_object(mcpc->result);
                {
                    buffer_json_member_add_string(mcpc->result, "type", "text");
                    buffer_json_member_add_string(mcpc->result, "text", 
                        "⚠️ Instance Usage Notice: Instance filtering/grouping behavior varies by collector type:\n\n"
                        "- **Stable instances** (systemd services, cgroups): Instance names are typically stable and match their labels. "
                        "Filtering by instance works reliably.\n\n"
                        "- **Dynamic instances** (Kubernetes pods, containers, processes): Instance names often contain random IDs or session identifiers. "
                        "Each restart creates a new instance. For these, filtering/grouping by labels is recommended to see the complete picture across all instances.\n\n"
                        "- **Detecting restarts**: Grouping by labels and examining instance counts can reveal restart patterns - "
                        "multiple instances with the same labels but different names often indicate restarts or scaling events.\n\n"
                        "Best practice: Check if your target system uses stable or dynamic instances. When in doubt, group by labels for comprehensive data, "
                        "then examine instance patterns for additional insights.");
                }
                buffer_json_object_close(mcpc->result);
            }
        }
        buffer_json_array_close(mcpc->result);  // Close content array
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON
    
    // Clean up allocated memory
    if (labels_buffer) {
        buffer_free(labels_buffer);
    }
    
    return MCP_RC_OK;
}