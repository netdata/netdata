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
#include "web/api/formatters/rrd2json.h"

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
    }
    buffer_json_object_close(buffer); // nodes

    buffer_json_member_add_object(buffer, "context");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Context Name");
        buffer_json_member_add_string(buffer, "description", "The specific context name to query. This parameter is required.\n"
                                                             "To discover available contexts, first use the " MCP_TOOL_METRIC_CONTEXTS " tool.");
    }
    buffer_json_object_close(buffer); // context


    buffer_json_member_add_object(buffer, "dimensions");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Dimensions Pattern");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on dimensions to include in the query.\n"
                                                             "If no dimensions are specified, all dimensions of the context are queried.");
    }
    buffer_json_object_close(buffer); // dimensions

    buffer_json_member_add_object(buffer, "labels");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Labels Filter");
        buffer_json_member_add_string(buffer, "description", "Filter for charts having specified labels (key:value pairs).");
    }
    buffer_json_object_close(buffer); // labels

    buffer_json_member_add_object(buffer, "alerts");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Alerts Filter");
        buffer_json_member_add_string(buffer, "description", "Filter for charts having specified alert states.");
    }
    buffer_json_object_close(buffer); // alerts
    
    buffer_json_member_add_object(buffer, "cardinality_limit");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Cardinality Limit");
        buffer_json_member_add_string(buffer, "description", "Limits the number of nodes, instances, dimensions, and label values returned in the results. "
                                                           "When the number of items exceeds this limit, only the top N items by contribution are returned, "
                                                           "with the remaining items aggregated into a 'remaining X dimensions' entry. "
                                                           "This helps keep response sizes manageable for high-cardinality queries. "
                                                           "The default limit is 10.");
    }
    buffer_json_object_close(buffer); // cardinality_limit

    // Time parameters
    buffer_json_member_add_object(buffer, "after");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "After Timestamp");
        buffer_json_member_add_string(buffer, "description", "Start time for the query in seconds since epoch. Negative values indicate relative time from `before`.");
    }
    buffer_json_object_close(buffer); // after

    buffer_json_member_add_object(buffer, "before");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Before Timestamp");
        buffer_json_member_add_string(buffer, "description", "End time for the query in seconds since epoch. Negative values indicate relative time from now.");
    }
    buffer_json_object_close(buffer); // before

    buffer_json_member_add_object(buffer, "points");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Data Points");
        buffer_json_member_add_string(buffer, "description", "Number of data points to return.");
    }
    buffer_json_object_close(buffer); // points

    buffer_json_member_add_object(buffer, "timeout");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Timeout");
        buffer_json_member_add_string(buffer, "description", "Query timeout in milliseconds.");
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
    }
    buffer_json_object_close(buffer); // time_group_options


    // Tier selection
    buffer_json_member_add_object(buffer, "tier");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Storage Tier");
        buffer_json_member_add_string(buffer, "description", "Storage tier to query from.\n"
                                                             "If not specified, Netdata will automatically pick the best tier based on the time-frame and points requested.");
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

// Extract time_t parameter from json object
static time_t extract_time_param(struct json_object *params, const char *name, time_t default_val) {
    if (!params)
        return default_val;
        
    struct json_object *obj = NULL;
    if (!json_object_object_get_ex(params, name, &obj) || !obj)
        return default_val;
        
    if (json_object_is_type(obj, json_type_int))
        return json_object_get_int64(obj);
        
    if (json_object_is_type(obj, json_type_string)) {
        const char *val_str = json_object_get_string(obj);
        if (val_str && *val_str)
            return str2l(val_str);
    }
    
    return default_val;
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
    const char *dimensions = extract_string_param(params, "dimensions");
    const char *labels = extract_string_param(params, "labels");
    const char *alerts = extract_string_param(params, "alerts");
    
    // Time parameters
    time_t after = extract_time_param(params, "after", -600);
    time_t before = extract_time_param(params, "before", 0);
    
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
    size_t cardinality_limit = extract_size_param(params, "cardinality_limit", 10);

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
        .version = 2,
        .scope_nodes = nodes,     // Use nodes as scope_nodes
        .scope_contexts = context, // Use the single context as scope_contexts
        .after = after,
        .before = before,
        .host = NULL,
        .st = NULL,
        .nodes = NULL,              // Don't use nodes parameter here (we use scope_nodes)
        .contexts = NULL,           // Don't use contexts parameter here (we use scope_contexts)
        .instances = NULL,
        .dimensions = dimensions,
        .alerts = alerts,
        .timeout_ms = timeout,
        .points = points,
        .format = DATASOURCE_JSON2,
        .options = options |
                   RRDR_OPTION_ABSOLUTE | RRDR_OPTION_JSON_WRAP | RRDR_OPTION_RETURN_JWAR |
                   RRDR_OPTION_VIRTUAL_POINTS | RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_NONZERO |
                   RRDR_OPTION_MINIFY | RRDR_OPTION_MINIMAL_STATS,
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
        return MCP_RC_INTERNAL_ERROR;
    }

    // Create a temporary buffer for the query result
    CLEAN_BUFFER *tmp_buffer = buffer_create(0, NULL);
    
    // Prepare onewayalloc for query execution
    ONEWAYALLOC *owa = onewayalloc_create(0);
    
    // Variables for metadata processing
    struct json_object *metadata = NULL;
    
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
        return MCP_RC_INTERNAL_ERROR;
    }
    
    // Initialize the success response
    mcp_init_success_result(mcpc, id);
    {
        // Start building content array for the result
        buffer_json_member_add_array(mcpc->result, "content");
        {
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");

                // Parse the result as JSON and remove some fields
                const char *tmp_buffer_str = buffer_tostring(tmp_buffer);
                struct json_object *json_result = json_tokener_parse(tmp_buffer_str ? tmp_buffer_str : "{}");
                if (json_result) {
                    // Check if the "result.data" array is empty
                    struct json_object *result_obj = NULL;
                    struct json_object *data_obj = NULL;
                    bool empty_data = true;
                    
                    // Access the "result" object
                    if (json_object_object_get_ex(json_result, "result", &result_obj)) {
                        // Access the "data" array in "result"
                        if (json_object_object_get_ex(result_obj, "data", &data_obj)) {
                            // Check if the array is empty or has elements
                            if (json_object_is_type(data_obj, json_type_array)) {
                                int data_length = json_object_array_length(data_obj);
                                if (data_length > 0) {
                                    empty_data = false;
                                }
                            }
                        }
                    }
                    
                    if (empty_data) {
                        buffer_flush(mcpc->result);

                        // Free the JSON object
                        json_object_put(json_result);
                        
                        // Return a more detailed error message about empty data
                        buffer_sprintf(mcpc->error, 
                            "Query matched no data. Possible reasons:\n"
                            "1. The context '%s' exists but has no data in the requested time range (%lld to %lld)\n"
                            "2. The nodes filter '%s' doesn't match any nodes that collect this context\n"
                            "3. The dimensions filter '%s' doesn't match any available dimensions\n"
                            "4. The labels filter '%s' doesn't match any metrics with those labels\n"
                            "Try using the context_details tool to verify the context exists and which nodes collect it.",
                            context, 
                            (long long)after, 
                            (long long)before, 
                            nodes ? nodes : "*",
                            dimensions ? dimensions : "*",
                            labels ? labels : "none"
                        );
                        return MCP_RC_BAD_REQUEST;
                    }
                    
                    // Extract important summary information before removing fields
                    metadata = json_object_new_object();
                    struct json_object *summary_obj = NULL;
                    
                    // Find the summary object and extract counts
                    if (json_object_object_get_ex(json_result, "summary", &summary_obj) && summary_obj) {
                        // Create a counts object for metadata
                        struct json_object *counts = json_object_new_object();
                        
                        // Extract node count
                        struct json_object *nodes_obj = NULL;
                        if (json_object_object_get_ex(summary_obj, "nodes", &nodes_obj) && nodes_obj) {
                            if (json_object_is_type(nodes_obj, json_type_array)) {
                                int count = json_object_array_length(nodes_obj);
                                json_object_object_add(counts, "nodes", json_object_new_int(count));
                            }
                        }
                        
                        // Extract instance count
                        struct json_object *instances_obj = NULL;
                        if (json_object_object_get_ex(summary_obj, "instances", &instances_obj) && instances_obj) {
                            if (json_object_is_type(instances_obj, json_type_array)) {
                                int count = json_object_array_length(instances_obj);
                                json_object_object_add(counts, "instances", json_object_new_int(count));
                            }
                        }
                        
                        // Extract dimension count
                        struct json_object *dimensions_obj = NULL;
                        if (json_object_object_get_ex(summary_obj, "dimensions", &dimensions_obj) && dimensions_obj) {
                            if (json_object_is_type(dimensions_obj, json_type_array)) {
                                int count = json_object_array_length(dimensions_obj);
                                json_object_object_add(counts, "dimensions", json_object_new_int(count));
                            }
                        }
                        
                        // Add counts to metadata
                        json_object_object_add(metadata, "counts", counts);
                        
                        // Extract labels information
                        struct json_object *labels_obj = NULL;
                        if (json_object_object_get_ex(summary_obj, "labels", &labels_obj) && labels_obj) {
                            if (json_object_is_type(labels_obj, json_type_array)) {
                                int labels_count = json_object_array_length(labels_obj);
                                struct json_object *metadata_labels = json_object_new_array();
                                
                                // Process each label key
                                for (int i = 0; i < labels_count; i++) {
                                    struct json_object *label = json_object_array_get_idx(labels_obj, i);
                                    if (label && json_object_is_type(label, json_type_object)) {
                                        // Get label key
                                        struct json_object *id_obj = NULL;
                                        if (json_object_object_get_ex(label, "id", &id_obj) && id_obj) {
                                            const char *label_key = json_object_get_string(id_obj);
                                            
                                            // Skip common labels that are always present with a single value
                                            if (label_key && (
                                                strcmp(label_key, "_collect_plugin") == 0 || 
                                                strcmp(label_key, "_collect_module") == 0)) {
                                                continue;
                                            }
                                            
                                            struct json_object *label_meta = json_object_new_object();
                                            json_object_object_add(label_meta, "key", json_object_get(id_obj));
                                            
                                            // Get label values
                                            struct json_object *values_obj = NULL;
                                            if (json_object_object_get_ex(label, "vl", &values_obj) && values_obj) {
                                                if (json_object_is_type(values_obj, json_type_array)) {
                                                    int values_count = json_object_array_length(values_obj);
                                                    json_object_object_add(label_meta, "count", json_object_new_int(values_count));
                                                    
                                                    // Add a sample of values (first 2)
                                                    struct json_object *sample_values = json_object_new_array();
                                                    int sample_size = values_count > 2 ? 2 : values_count;
                                                    
                                                    for (int j = 0; j < sample_size; j++) {
                                                        struct json_object *value = json_object_array_get_idx(values_obj, j);
                                                        if (value && json_object_is_type(value, json_type_object)) {
                                                            struct json_object *value_id = NULL;
                                                            if (json_object_object_get_ex(value, "id", &value_id) && value_id) {
                                                                json_object_array_add(sample_values, json_object_get(value_id));
                                                            }
                                                        }
                                                    }
                                                    
                                                    json_object_object_add(label_meta, "values", sample_values);
                                                }
                                            }
                                            
                                            json_object_array_add(metadata_labels, label_meta);
                                        }
                                    }
                                }
                                
                                json_object_object_add(metadata, "labels", metadata_labels);
                            }
                        }
                    }
                    
                    // We'll use the metadata object directly later
                    // No need to convert to string in advance
                    
                    // Now remove unwanted fields
                    json_object_object_del(json_result, "api");
                    json_object_object_del(json_result, "versions");
                    json_object_object_del(json_result, "summary");
                    json_object_object_del(json_result, "totals");
                    json_object_object_del(json_result, "timings");
                    json_object_object_del(json_result, "agents");
                    json_object_object_del(json_result, "functions");

                    // Convert back to string
                    const char *modified_json = json_object_to_json_string_ext(json_result, JSON_C_TO_STRING_PRETTY);

                    // Write it to the result buffer (should be after the opening brace for "result")
                    buffer_json_member_add_string(mcpc->result, "text", modified_json);

                    // Free the JSON object
                    json_object_put(json_result);
                } else {
                    // If parsing failed, just use an empty object as the result
                    buffer_json_member_add_string(mcpc->result, "text", "{}");
                }
            }
            buffer_json_object_close(mcpc->result);

            // Add metadata as text
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");
                
                // Add metadata as plain text, with careful handling of the JSON to avoid format issues
                BUFFER *plain_text = buffer_create(0, NULL);
                
                // First add the explanation text
                buffer_strcat(plain_text, "QUERY METADATA\n");

                // Now manually add a cleaner version of the counts
                if (metadata) {
                    struct json_object *counts_obj = NULL;
                    if (json_object_object_get_ex(metadata, "counts", &counts_obj)) {
                        struct json_object *nodes_count = NULL;
                        if (json_object_object_get_ex(counts_obj, "nodes", &nodes_count)) {
                            buffer_sprintf(plain_text, "- Nodes Aggregated: %d\n", json_object_get_int(nodes_count));
                        }
                        
                        struct json_object *instances_count = NULL;
                        if (json_object_object_get_ex(counts_obj, "instances", &instances_count)) {
                            buffer_sprintf(plain_text, "- Instances Aggregated (across all nodes): %d\n", json_object_get_int(instances_count));
                        }
                        
                        struct json_object *dimensions_count = NULL;
                        if (json_object_object_get_ex(counts_obj, "dimensions", &dimensions_count)) {
                            buffer_sprintf(plain_text, "- Unique dimension names (across all instances): %d\n", json_object_get_int(dimensions_count));
                        }
                    }
                    
                    // Process labels array
                    struct json_object *labels_array = NULL;
                    if (json_object_object_get_ex(metadata, "labels", &labels_array) && 
                        json_object_is_type(labels_array, json_type_array)) {
                        
                        int num_labels = json_object_array_length(labels_array);
                        if (num_labels > 0) {
                            buffer_strcat(plain_text, "\nLabel information:\n");
                            
                            for (int i = 0; i < num_labels; i++) {
                                struct json_object *label = json_object_array_get_idx(labels_array, i);
                                if (!label) continue;
                                
                                struct json_object *key_obj = NULL;
                                struct json_object *count_obj = NULL;
                                struct json_object *values_obj = NULL;
                                
                                if (json_object_object_get_ex(label, "key", &key_obj) && 
                                    json_object_object_get_ex(label, "count", &count_obj) &&
                                    json_object_object_get_ex(label, "values", &values_obj)) {
                                    
                                    const char *key = json_object_get_string(key_obj);
                                    int count = json_object_get_int(count_obj);
                                    
                                    buffer_sprintf(plain_text, "- '%s': %d unique label values", key, count);
                                    
                                    if (json_object_is_type(values_obj, json_type_array)) {
                                        int values_length = json_object_array_length(values_obj);
                                        if (values_length > 0) {
                                            buffer_strcat(plain_text, " (sample: ");
                                            
                                            for (int j = 0; j < values_length; j++) {
                                                struct json_object *value = json_object_array_get_idx(values_obj, j);
                                                if (value) {
                                                    if (j > 0) buffer_strcat(plain_text, ", ");
                                                    buffer_sprintf(plain_text, "'%s'", json_object_get_string(value));
                                                }
                                            }
                                            
                                            buffer_strcat(plain_text, ")");
                                        }
                                    }
                                    
                                    buffer_strcat(plain_text, "\n");
                                }
                            }
                        }
                    }
                }
                
                buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(plain_text));
                
                // Free the temporary buffer
                buffer_free(plain_text);
            }
            buffer_json_object_close(mcpc->result);
            
            // Add explanation of data points
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");
                buffer_json_member_add_string(
                    mcpc->result, "text",
                    "Each point in the `result` has an array of 3 values:\n"
                    "- `value`: is the Value based on the samples\n"
                    "- `arp`: is the Anomaly Rate Percentage\n"
                    "- `pa`: is a bitmap of Point Annotations:\n"
                    "   1 = `EMPTY` the point has no value.\n"
                    "   2 = `RESET` at least one metric aggregated experienced an overflow (a counter that wrapped or restarted).\n"
                    "   4 = `PARTIAL` this point should have more metrics aggregated into it, but not all metrics had data.\n"
                    "\n"
                    "`db` is about the raw data in the db (before aggregations).\n"
                    "\n"
                    "`view` is about the `result` (after all aggregations).\n"
                    "\n"
                    "FIELDS\n"
                    "`sts` stands for Statistics.\n"
                    "\n"
                    "`arp` is Anomaly Rate Percentage.\n"
                    "      The percentage of samples in the time series that were detected as anomalies.\n"
                    "\n"
                    "`con` is Contribution Percentage.\n"
                    "      The percentage each time series is contributing to the total volume of the chart.");
            }
            buffer_json_object_close(mcpc->result);
        }
        buffer_json_array_close(mcpc->result);  // Close content array
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON
    
    // Free the metadata object if it was allocated
    if (metadata) {
        json_object_put(metadata);
    }

    return MCP_RC_OK;
}