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

#include "mcp-tools-query-metrics.h"
#include "mcp-tools.h"
#include "mcp-params.h"
#include "web/api/formatters/rrd2json.h"


// JSON schema for the metrics query tool
void mcp_tool_query_metrics_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Query Metrics Data");

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    // Selection parameters
    mcp_schema_add_string_param(
        buffer, "metric", "Metric Name",
        "The exact metric (context) to query.\n"
        "Use the '" MCP_TOOL_LIST_METRICS "' tool to discover available metrics.",
        NULL, true);

    mcp_schema_add_array_param(
        buffer, "dimensions",
        "Dimensions Filter",
        "Array of dimensions to include in the query.\n"
        "Examples: [\"read\", \"write\"] or [\"in\", \"out\"] or [\"used\", \"free\", \"cached\"]\n"
        "Use the '" MCP_TOOL_GET_METRICS_DETAILS "' tool to discover the available dimensions for a metric.");

    mcp_schema_add_labels_object(
        buffer, "Labels Filter",
        "Query only the instances with the given labels. "
        "Example: {\"disk_type\": [\"ssd\", \"nvme\"], \"mount_point\": [\"/\"]}\n"
        "Values in the same array are ORed, different keys are ANDed. "
        "Use the '" MCP_TOOL_GET_METRICS_DETAILS "' tool to discover available labels and values for a metric.");

    mcp_schema_add_array_param(
        buffer, "instances",
        "Instances Filter",
        "Query only the given instances.\n"
        "Use the '" MCP_TOOL_GET_METRICS_DETAILS "' tool to discover available instances for a metric.\n"
        "If no instances are specified, all instances of the metric are queried.\n"
        "Example: [\"instance1\", \"instance2\", \"instance3\"]\n."
        "IMPORTANT: when you have a choice, prefer to filter by labels instead of instances, because many monitored "
        "components may change instance names over time.");

    mcp_schema_add_array_param(
        buffer, "nodes",
        "Nodes Filter",
        "Array of nodes to include in the query.\n"
        "If no nodes are specified, all nodes having data for the given metrics in the specified time-frame will be queried.\n"
        "Examples: [\"node1\", \"node2\", \"node3\"]\n"
        "Use the '" MCP_TOOL_LIST_NODES "' tool to discover the available nodes.");

    // Add cardinality limit
    mcp_schema_add_cardinality_limit(buffer,
        "Limit the response cardinality (number of dimensions, instances, labels, etc.). "
        "When the limit is exceeded, the response will indicate how many items were omitted.",
        MCP_DATA_CARDINALITY_LIMIT,
        1,  // minimum
        MAX(MCP_DATA_CARDINALITY_LIMIT, MCP_DATA_CARDINALITY_LIMIT_MAX));

    // Time parameters
    mcp_schema_add_time_params(buffer, "query window", true);

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
        buffer_json_member_add_string(buffer, "description", "Query timeout in seconds.");
        buffer_json_member_add_uint64(buffer, "default", 60);
    }
    buffer_json_object_close(buffer); // timeout

    buffer_json_member_add_object(buffer, "options");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Query Options");
        buffer_json_member_add_string(
            buffer, "description",
            "Space-separated list of additional query options:\n"
            "'percentage': Return values as percentages of total\n"
            "'absolute' or 'absolute-sum': Return absolute values for stacked charts\n"
            "'display-absolute': Convert percentage values to absolute before application of grouping functions\n"
            "'all-dimensions': Include all dimensions, even those with just zero values\n"
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
        buffer_json_member_add_string(
            buffer, "description",
            "Additional options for time grouping.\n"
            "For 'percentile', specify a percentage (0-100).\n"
            "For 'countif', specify a comparison operator and value (e.g., '>0', '=0', '!=0', '<=10').");
    }
    buffer_json_object_close(buffer); // time_group_options

    // Tier selection
    buffer_json_member_add_object(buffer, "tier");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Storage Tier");
        buffer_json_member_add_string(
            buffer, "description",
            "Storage tier to query from.\n"
            "If not specified, Netdata will automatically pick the best tier based on the time-frame and points requested.\n"
            "CAUTION: specifying a high-resolution tier (like 0) over long time-frames (like days) may consume significant system resources.");
    }
    buffer_json_object_close(buffer); // tier

    // Group by parameters
    buffer_json_member_add_object(buffer, "group_by");
    {
        buffer_json_member_add_string(buffer, "type", "array");
        buffer_json_member_add_string(buffer, "title", "Group By");
        buffer_json_member_add_string(
            buffer, "description",
            "Specifies how to group metrics across different time-series.\n"
            "- 'dimension': Groups by dimension name across all instances/nodes. Example: for disks it provides the aggregate of reads and writes across all disks of all nodes.\n"
            "- 'instance': Groups by instance across all nodes. Example: for disks, it provides the aggregate per disk name (sda, sdb, etc), aggregating their reads and writes, across all nodes.\n"
            "- 'node': Groups by node. Example: for disks, it provides one metric per node, aggregating reads and writes across all its disks.\n"
            "- 'label': Groups by the given label key (use the parameter 'group_by_label' to set the key). Example: for disks, aggregate over key 'disk_type' to get an group all 'physical', 'virtual' and 'partition' separately.\n"
            "Multiple groupings can be combined. Example: '[\"dimension\", \"label\"]'.");
        buffer_json_member_add_array(buffer, "default");
        buffer_json_add_array_item_string(buffer, "dimension");
        buffer_json_array_close(buffer);
        
        // Define items schema with enum values
        buffer_json_member_add_object(buffer, "items");
        {
            buffer_json_member_add_string(buffer, "type", "string");
            buffer_json_member_add_array(buffer, "enum");
            buffer_json_add_array_item_string(buffer, "dimension");
            buffer_json_add_array_item_string(buffer, "instance");
            buffer_json_add_array_item_string(buffer, "node");
            buffer_json_add_array_item_string(buffer, "label");

            // we don't offer these to MCP clients.
            // buffer_json_add_array_item_string(buffer, "context");
            // buffer_json_add_array_item_string(buffer, "units");
            buffer_json_array_close(buffer);
        }
        buffer_json_object_close(buffer); // items
    }
    buffer_json_object_close(buffer); // group_by

    mcp_schema_add_string_param(
        buffer, "group_by_label",
        "Group By Label",
        "When 'group_by' includes 'label', this parameter specifies the label key to group by.\n"
        "Example: if metrics have an 'interface_type' label with values like 'real' or 'virtual', "
        "setting 'group_by_label' to 'interface_type' would aggregate metrics separately for physical and virtual network interfaces.",
        NULL, false);

    buffer_json_member_add_object(buffer, "aggregation");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Aggregation Method");
        buffer_json_member_add_string(
            buffer, "description",
            "Method to use when aggregating grouped metrics.\n"
            "- 'sum': Sum of all grouped metrics (useful for additive metrics like bytes transferred, operations, etc.)\n"
            "- 'min': Minimum value among all grouped metrics (useful for finding best performance metrics)\n"
            "- 'max': Maximum value among all grouped metrics (useful for finding worst performance metrics, peak resource usage)\n"
            "- 'extremes': When values are both positive and negative, shows the maximum value for positive metrics and the minimum value for negative metrics\n"
            "- 'average': Average of all grouped metrics (CAUTION: When 'group_by' doesn't include 'dimension', this averages different metric types together - e.g., CPU user + system + idle - which is rarely meaningful)\n"
            "- 'percentage': Expresses each grouped metric as a percentage of its group's total (useful for seeing proportional contributions)\n");

        // Define enum of possible values
        buffer_json_member_add_array(buffer, "enum");
        buffer_json_add_array_item_string(buffer, "sum");
        buffer_json_add_array_item_string(buffer, "min");
        buffer_json_add_array_item_string(buffer, "max");
        buffer_json_add_array_item_string(buffer, "extremes");
        buffer_json_add_array_item_string(buffer, "average");
        buffer_json_add_array_item_string(buffer, "percentage");
        buffer_json_array_close(buffer);
    }
    buffer_json_object_close(buffer); // aggregation

    buffer_json_object_close(buffer); // properties

    // Required fields
    buffer_json_member_add_array(buffer, "required");
    buffer_json_add_array_item_string(buffer, "metric");
    buffer_json_add_array_item_string(buffer, "dimensions");
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

// Removed extract_string_param and extract_size_param - now using mcp-params functions

// Execute the metrics query
MCP_RETURN_CODE mcp_tool_query_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || id == 0)
        return MCP_RC_ERROR;

    buffer_flush(mcpc->result);

    usec_t received_ut = now_monotonic_usec();
    
    // Extract and validate context parameter
    const char *context = mcp_params_extract_string(params, "metric", NULL);
    
    // Validate required parameters with detailed error messages
    if (!context || !*context) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'metric'. Use the '" MCP_TOOL_LIST_METRICS "' tool to discover available metrics/contexts.");
        return MCP_RC_BAD_REQUEST;
    }
    
    // Check if context contains patterns
    if (simple_pattern_contains_wildcards(context, SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS)) {
        buffer_sprintf(mcpc->error, "The 'context' parameter must be an exact context name, not a pattern. "
                                    "Wildcards or pattern separators are not supported. "
                                    "Use the " MCP_TOOL_LIST_METRICS " tool to discover exact context names.");
        return MCP_RC_BAD_REQUEST;
    }
    
    // Check if all required parameters are provided
    struct json_object *after_obj = NULL, *before_obj = NULL, *points_obj = NULL, *time_group_obj = NULL;
    struct json_object *group_by_obj = NULL, *aggregation_obj = NULL, *dimensions_obj = NULL;
    
    if (!json_object_object_get_ex(params, "dimensions", &dimensions_obj) || !dimensions_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'dimensions'. Use the '" MCP_TOOL_LIST_METRICS "' to get the list of dimensions for this metric/context.");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_object_get_ex(params, "after", &after_obj) || !after_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'after'. This parameter defines the start time for your query (Unix epoch timestamp in seconds, or negative value relative to 'before', or RFC3339 datetime string).");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_object_get_ex(params, "before", &before_obj) || !before_obj) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'before'. This parameter defines the end time for your query (Unix epoch timestamp in seconds, or negative value relative to now, or RFC3339 datetime string).");
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
    
    // Get time_group value to check if it's a percentile or countif
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
    
    // Handle nodes array parameter
    CLEAN_BUFFER *nodes_buffer = NULL;
    
    nodes_buffer = mcp_params_parse_array_to_pattern(params, "nodes", false, false, MCP_TOOL_LIST_NODES, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // Handle instances array parameter
    CLEAN_BUFFER *instances_buffer = NULL;
    
    instances_buffer = mcp_params_parse_array_to_pattern(params, "instances", false, false, MCP_TOOL_GET_METRICS_DETAILS, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // Handle dimensions array parameter
    CLEAN_BUFFER *dimensions_buffer = NULL;
    
    dimensions_buffer = mcp_params_parse_array_to_pattern(params, "dimensions", true, false, MCP_TOOL_GET_METRICS_DETAILS, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        buffer_strcat(mcpc->error, ". You must explicitly list every dimension you want to query. "
                                   "Use the '" MCP_TOOL_GET_METRICS_DETAILS "' tool to discover available dimensions for the context.");
        return MCP_RC_BAD_REQUEST;
    }
    // Handle labels - expects a structured object only
    CLEAN_BUFFER *labels_buffer = NULL;
    
    labels_buffer = mcp_params_parse_labels_object(params, MCP_TOOL_GET_METRICS_DETAILS, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // Removed alerts parameter - not used in query_metrics
    
    // Time parameters - parse and validate together
    time_t after, before;
    if (!mcp_params_parse_time_window(params, &after, &before, 
                                      MCP_DEFAULT_AFTER_TIME, MCP_DEFAULT_BEFORE_TIME, 
                                      false, mcpc->error)) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // No need to check aggregation_obj here - we already have a default in group_by struct
    
    // Other parameters
    size_t points = mcp_params_extract_size(params, "points", 0, 0, SIZE_MAX, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    size_t cardinality_limit = mcp_params_extract_size(params, "cardinality_limit", MCP_DATA_CARDINALITY_LIMIT, 1, 1000, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }

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
    
    long timeout = mcp_params_extract_timeout(params, "timeout", 0, 0, 3600, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    const char *options_str = mcp_params_extract_string(params, "options", NULL);
    RRDR_OPTIONS options = 0;
    if (options_str && *options_str)
        options |= rrdr_options_parse(options_str);
    
    // Time grouping
    RRDR_TIME_GROUPING time_group = RRDR_GROUPING_AVERAGE;
    if (time_group_str && *time_group_str)
        time_group = time_grouping_parse(time_group_str, RRDR_GROUPING_AVERAGE);
    
    const char *time_group_options = mcp_params_extract_string(params, "time_group_options", NULL);
    
    // Tier selection (give an invalid default to now the caller added a tier to the query)
    size_t tier = mcp_params_extract_size(params, "tier", nd_profile.storage_tiers + 1, 0, SIZE_MAX, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    if (tier < nd_profile.storage_tiers)
        options |= RRDR_OPTION_SELECTED_TIER;
    else
        tier = 0;
    
    // Group by parameters (simplified - in real implementation handle multiple passes)
    struct group_by_pass group_by[MAX_QUERY_GROUP_BY_PASSES] = {
        {
            .group_by = RRDR_GROUP_BY_NONE,
            .group_by_label = NULL,
            .aggregation = RRDR_GROUP_BY_FUNCTION_AVERAGE,
        },
    };
    
    // Handle group_by array parameter and convert to comma-separated string
    CLEAN_BUFFER *group_by_buffer = NULL;
    const char *group_by_str = NULL;
    
    group_by_buffer = mcp_params_parse_array_to_pattern(params, "group_by", true, false, MCP_TOOL_GET_METRICS_DETAILS, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    if (group_by_buffer && buffer_strlen(group_by_buffer) > 0) {
        group_by_str = buffer_tostring(group_by_buffer);
        group_by[0].group_by = group_by_parse(group_by_str);
    }
    
    const char *group_by_label = mcp_params_extract_string(params, "group_by_label", NULL);
    if (group_by_label && *group_by_label) {
        group_by[0].group_by_label = (char *)group_by_label;
        group_by[0].group_by |= RRDR_GROUP_BY_LABEL;
    }
    
    const char *aggregation_str = mcp_params_extract_string(params, "aggregation", NULL);
    if (aggregation_str && *aggregation_str)
        group_by[0].aggregation = group_by_aggregate_function_parse(aggregation_str);
    
    // Create interrupt callback data
    mcp_query_interrupt_data interrupt_data = {
        .mcpc = mcpc,
        .id = id
    };
    
    // Prepare a query target request
    QUERY_TARGET_REQUEST qtr = {
        .version = 3,
        .scope_nodes = buffer_tostring(nodes_buffer),       // Use nodes as scope_nodes
        .scope_contexts = context,  // Use the single context as scope_contexts
        .scope_instances = buffer_tostring(instances_buffer), // Use instances as scope_instances for MCP
        .scope_labels = buffer_tostring(labels_buffer),     // Use labels as scope_labels for MCP
        .scope_dimensions = buffer_tostring(dimensions_buffer), // Use dimensions as scope_dimensions for MCP
        .after = after,
        .before = before,
        .host = NULL,
        .st = NULL,
        .nodes = NULL,              // Don't use the 'nodes' parameter here (we use scope_nodes)
        .contexts = NULL,           // Don't use the 'contexts' parameter here (we use scope_contexts)
        .instances = NULL,          // Don't use the 'instances' parameter here (we use scope_instances)
        .dimensions = NULL,         // Don't use the 'dimensions' parameter here (we use scope_dimensions)
        .alerts = NULL,
        .timeout_ms = (int)(timeout * MSEC_PER_SEC),
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
        .labels = NULL,             // Don't use labels parameter here (we use scope_labels)
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
    
    // Create a query target
    QUERY_TARGET *qt = query_target_create(&qtr);
    if (!qt) {
        buffer_sprintf(mcpc->error, "Failed to prepare the query.");
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
                error_desc = "metric/context not found";
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
    
    // Check if instance filtering or grouping is used
    bool using_instances = (instances_buffer && buffer_strlen(instances_buffer) > 0) || 
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
            
            // Add a warning about potentially misleading aggregation
            bool warn_aggregation = false;
            // Only warn if using average without dimension grouping AND multiple dimensions selected
            int dimensions_count = (int)json_object_array_length(dimensions_obj);
            if (dimensions_count > 1 &&
                group_by[0].aggregation == RRDR_GROUP_BY_FUNCTION_AVERAGE && 
                !(group_by[0].group_by & RRDR_GROUP_BY_DIMENSION)) {
                warn_aggregation = true;
            }
            
            if (warn_aggregation) {
                buffer_json_add_array_item_object(mcpc->result);
                {
                    buffer_json_member_add_string(mcpc->result, "type", "text");
                    buffer_json_member_add_string(mcpc->result, "text",
                        "⚠️ WARNING: Potentially Misleading Aggregation\n\n"
                        "You are using 'average' aggregation without including 'dimension' in group_by. "
                        "This means different metric types are being averaged together, which rarely produces meaningful results.\n\n"
                        "For example:\n"
                        "- For CPU metrics: averaging user, system, idle, wait states together\n"
                        "- For network metrics: averaging in/out traffic together\n"
                        "- For disk I/O: averaging reads and writes together\n\n"
                        "Check the 'aggregated' field in view.dimensions to see how many time-series were combined. "
                        "Values greater than 1 indicate multiple different metrics were averaged together.\n\n"
                        "Consider using:\n"
                        "- 'sum' aggregation for additive metrics\n"
                        "- Include 'dimension' in group_by (e.g., 'instance,dimension')\n"
                        "- Review the summary section to understand what's being aggregated");
                }
                buffer_json_object_close(mcpc->result);
            }
            
            // Add an instance usage warning if applicable
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
    
    return MCP_RC_OK;
}