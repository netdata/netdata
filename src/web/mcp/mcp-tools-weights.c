// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-weights.h"
#include "mcp-params.h"
#include "web/api/web_api.h"
#include "web/api/queries/weights.h"
#include "web/api/queries/query.h"

// Common function to execute the 'weights' request
static MCP_RETURN_CODE execute_weights_request(
    MCP_CLIENT *mcpc,
    struct json_object *params,
    MCP_REQUEST_ID id,
    WEIGHTS_METHOD method,
    const char *default_time_group
) {
    // Extract time parameters using common parsing functions
    time_t after, before;
    if (!mcp_params_parse_time_window(params, &after, &before, 
                                      MCP_DEFAULT_AFTER_TIME, MCP_DEFAULT_BEFORE_TIME, 
                                      false, mcpc->error)) {
        return MCP_RC_BAD_REQUEST;
    }
    time_t baseline_after = 0;
    time_t baseline_before = 0;
    
    // For correlation methods (KS2, VOLUME), parse baseline times
    if (method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME) {
        if (!mcp_params_parse_time_window(params, &baseline_after, &baseline_before, 
                                           0, 0, true, mcpc->error)) {
            return MCP_RC_BAD_REQUEST;
        }
        
        // If baseline not specified, auto-calculate as 4x the query window before the query window
        if (baseline_after == 0 && baseline_before == 0) {
            time_t window = before - after;
            baseline_before = after;
            baseline_after = baseline_before - (window * 4);
        }
    }
    
    // Parse filter parameters using common parsing functions
    // Use CLEAN_BUFFER for automatic cleanup
    CLEAN_BUFFER *metrics_buffer = NULL;
    CLEAN_BUFFER *nodes_buffer = NULL;
    CLEAN_BUFFER *instances_buffer = NULL;
    CLEAN_BUFFER *dimensions_buffer = NULL;
    CLEAN_BUFFER *labels_buffer = NULL;
    
    // Parse metrics as an array
    metrics_buffer = mcp_params_parse_array_to_pattern(params, "metrics", false, false, MCP_TOOL_LIST_METRICS, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // Parse nodes as array
    nodes_buffer = mcp_params_parse_array_to_pattern(params, "nodes", false, false, MCP_TOOL_LIST_NODES, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // Parse instances as an array
    instances_buffer = mcp_params_parse_array_to_pattern(params, "instances", false, false, MCP_TOOL_GET_METRICS_DETAILS, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // Parse dimensions as array
    dimensions_buffer = mcp_params_parse_array_to_pattern(params, "dimensions", false, false, MCP_TOOL_GET_METRICS_DETAILS, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // Parse labels as object
    labels_buffer = mcp_params_parse_labels_object(params, MCP_TOOL_GET_METRICS_DETAILS, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
    
    // Get cardinality limit
    struct json_object *obj;
    size_t cardinality_limit = MCP_WEIGHTS_CARDINALITY_LIMIT;
    if (json_object_object_get_ex(params, "cardinality_limit", &obj) && json_object_is_type(obj, json_type_int))
        cardinality_limit = json_object_get_int(obj);
    
    // Extract timeout parameter
    int timeout = mcp_params_extract_timeout(params, "timeout", MCP_DEFAULT_TIMEOUT_WEIGHTS, 1, 3600, mcpc->error);
    if (buffer_strlen(mcpc->error) > 0) {
        return MCP_RC_BAD_REQUEST;
    }
        
    // Set time_group parameter based on the method
    const char *time_group_options = default_time_group;
    RRDR_TIME_GROUPING time_group_method = RRDR_GROUPING_AVERAGE;
    
    // For find_unstable_metrics (WEIGHTS_METHOD_VALUE with cv default), always use CV
    if (method == WEIGHTS_METHOD_VALUE && default_time_group && strcmp(default_time_group, "cv") == 0) {
        time_group_method = RRDR_GROUPING_CV;
        time_group_options = "cv";
    } else if (default_time_group) {
        // Use the default time grouping specified by the tool
        time_group_method = time_grouping_parse(default_time_group, RRDR_GROUPING_AVERAGE);
    }
    
    // Set options
    RRDR_OPTIONS options = RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_NULL2ZERO | RRDR_OPTION_ABSOLUTE | RRDR_OPTION_NONZERO;

    // Build the 'weights' request structure
    QUERY_WEIGHTS_REQUEST qwr = {
        .version = 2,
        .host = NULL,  // Will query all hosts
        .scope_nodes = buffer_tostring(nodes_buffer),
        .scope_contexts = buffer_tostring(metrics_buffer),
        .scope_instances = buffer_tostring(instances_buffer),
        .scope_labels = buffer_tostring(labels_buffer),
        .scope_dimensions = buffer_tostring(dimensions_buffer),
        .nodes = NULL,
        // exclude netdata internal metrics
        // exclude system interrupts and CPU interrupts, while are fragile
        .contexts = "!netdata.*|!system.interrupts|!system.intr|!cpu.interrupts|*",
        .instances = NULL,
        .dimensions = NULL,
        .labels = NULL,
        .alerts = NULL,
        .group_by = {
            .group_by = RRDR_GROUP_BY_NONE,
            .group_by_label = NULL,
            .aggregation = RRDR_GROUP_BY_FUNCTION_AVERAGE,
        },
        .method = method,
        .format = WEIGHTS_FORMAT_MCP,
        .time_group_method = time_group_method,
        .time_group_options = time_group_options,
        .baseline_after = baseline_after,
        .baseline_before = baseline_before,
        .after = after,
        .before = before,
        .points = 500,  // Default points for weights
        .options = options,
        .tier = 0,
        .timeout_ms = (int)(timeout * 1000),  // Convert seconds to milliseconds
        .cardinality_limit = cardinality_limit,
        .interrupt_callback = NULL,
        .interrupt_callback_data = NULL,
        .transaction = NULL,
    };
    
    // Create a temporary buffer for the 'weights' API response
    CLEAN_BUFFER *tmp_buffer = buffer_create(0, NULL);
    
    // Call the weights API function with the temporary buffer
    int http_code = web_api_v12_weights(tmp_buffer, &qwr);
    
    // Handle response
    if (http_code != HTTP_RESP_OK) {
        buffer_flush(mcpc->error);
        
        switch (http_code) {
            case HTTP_RESP_BAD_REQUEST:
                buffer_sprintf(mcpc->error, "Invalid request parameters");
                return MCP_RC_BAD_REQUEST;
                
            case HTTP_RESP_NOT_FOUND:
                buffer_sprintf(mcpc->error, "No results found");
                return MCP_RC_NOT_FOUND;
                
            case HTTP_RESP_GATEWAY_TIMEOUT:
                buffer_sprintf(mcpc->error, "Request timed out - repeat the request with a longer timeout");
                return MCP_RC_ERROR;
                
            default:
                buffer_sprintf(mcpc->error, "Internal error (HTTP %d)", http_code);
                return MCP_RC_INTERNAL_ERROR;
        }
    }
    
    // Initialize response
    mcp_init_success_result(mcpc, id);

    // Wrap the response in MCP JSON-RPC format
    buffer_json_member_add_array(mcpc->result, "content");
    {
        buffer_json_add_array_item_object(mcpc->result);
        {
            buffer_json_member_add_string(mcpc->result, "type", "text");
            buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(tmp_buffer));
        }
        buffer_json_object_close(mcpc->result);
    }
    buffer_json_array_close(mcpc->result);
    
    // Close the result object and finalize JSON
    buffer_json_object_close(mcpc->result); // Close the "result" object
    buffer_json_finalize(mcpc->result);
    
    return MCP_RC_OK;
}

// Schema helper for common time window parameters
static void add_weights_time_parameters(BUFFER *buffer, bool include_baseline, bool required) {

    // add 'after' and 'before' parameters
    mcp_schema_add_time_params(buffer, "metrics", required);
    
    if (include_baseline) {
        mcp_schema_add_time_param(
            buffer, "baseline_after",
            "Baseline start time",
            "Start time for the baseline period to compare against. If not specified, "
            "automatically set to 4x the query window before the query period.",
            "'baseline_before'",
            0,
            required);
        
        mcp_schema_add_time_param(
            buffer, "baseline_before",
            "Baseline end time",
            "End time for the baseline period. If not specified, automatically set to "
            "the start of the query period (adjacent to 'after').",
            "'after'",
            0,
            required);
    }
}

// Schema helper for common filter parameters
static void add_weights_filter_parameters(BUFFER *buffer) {
    mcp_schema_add_array_param(
        buffer, "metrics",
        "Filter by metrics",
        "Array of metrics (contexts) to filter (e.g., ['system.cpu', 'disk.io', 'mysql.queries']). Use '" MCP_TOOL_LIST_METRICS "' to discover available metrics.");
    
    mcp_schema_add_array_param(
        buffer, "nodes",
        "Filter by nodes",
        "Array of nodes to filter (e.g., ['web-server-1', 'database-primary']). Use '" MCP_TOOL_LIST_NODES "' to discover available nodes.");
    
    mcp_schema_add_array_param(
        buffer, "instances",
        "Filter by instances",
        "Array of metric instances to filter (e.g., ['eth0', 'sda', 'production_db']). Use '" MCP_TOOL_GET_METRICS_DETAILS "' to discover instances for a metric.");
    
    mcp_schema_add_array_param(buffer, "dimensions",
        "Filter by dimensions",
        "Array of dimension names to filter (e.g., ['user', 'writes', 'slow_queries']). Use '" MCP_TOOL_GET_METRICS_DETAILS "' to discover dimensions for a metric.");
    
    mcp_schema_add_labels_object(buffer,
        "Filter by labels",
        "Filter using labels where each key maps to an array of exact values. "
        "Values in the same array are ORed, different keys are ANDed. "
        "Example: {\"disk_type\": [\"ssd\", \"nvme\"], \"mount_point\": [\"/\"]}\n"
        "Note: Wildcards are not supported. Use exact label keys and values only. "
        "Use '" MCP_TOOL_GET_METRICS_DETAILS "' to discover available labels.");
}

static void add_weights_common_parameters(BUFFER *buffer) {
    mcp_schema_add_cardinality_limit(
        buffer, "Maximum number of results to return",
        MCP_WEIGHTS_CARDINALITY_LIMIT,
        30,  // minimum for weights
        MAX(MCP_WEIGHTS_CARDINALITY_LIMIT, MCP_WEIGHTS_CARDINALITY_LIMIT_MAX));

    // Timeout parameter
    mcp_schema_add_timeout(
        buffer, "timeout",
        "Query timeout",
        "Maximum time to wait for the query to complete (in seconds)",
        MCP_DEFAULT_TIMEOUT_WEIGHTS, 1, 3600, false);
}

// find_correlated_metrics implementation
MCP_RETURN_CODE mcp_tool_find_correlated_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    // Parse method parameter
    WEIGHTS_METHOD method = WEIGHTS_METHOD_MC_VOLUME; // Default to volume as per schema
    
    struct json_object *obj;
    if (json_object_object_get_ex(params, "method", &obj) && json_object_is_type(obj, json_type_string)) {
        const char *method_str = json_object_get_string(obj);
        if (strcmp(method_str, "ks2") == 0)
            method = WEIGHTS_METHOD_MC_KS2;
        else if (strcmp(method_str, "volume") == 0)
            method = WEIGHTS_METHOD_MC_VOLUME;
    }
    
    return execute_weights_request(mcpc, params, id, method, NULL);
}

void mcp_tool_find_correlated_metrics_schema(BUFFER *buffer) {
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Find metrics that changed during an incident");
    
    buffer_json_member_add_object(buffer, "properties");
    {
        add_weights_time_parameters(buffer, true, true); // include_baseline=true, required=true
        add_weights_filter_parameters(buffer);

        buffer_json_member_add_object(buffer, "method");
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Correlation method");
        buffer_json_member_add_string(
            buffer, "description",
            "Algorithm to use:\n"
            "- 'ks2': Statistical distribution comparison (slow, but intelligent)\n"
            "- 'volume': Percentage change in averages (fast, works well for most cases)");
        buffer_json_member_add_array(buffer, "enum");
        buffer_json_add_array_item_string(buffer, "ks2");
        buffer_json_add_array_item_string(buffer, "volume");
        buffer_json_array_close(buffer);
        buffer_json_member_add_string(buffer, "default", "volume");
        buffer_json_object_close(buffer); // method

        add_weights_common_parameters(buffer);
    }
    buffer_json_object_close(buffer); // properties
    
    buffer_json_member_add_array(buffer, "required");
    {
        buffer_json_add_array_item_string(buffer, "after");
        buffer_json_add_array_item_string(buffer, "before");
    }
    buffer_json_array_close(buffer);
    
    buffer_json_object_close(buffer); // inputSchema
}

// find_anomalous_metrics implementation
MCP_RETURN_CODE mcp_tool_find_anomalous_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    return execute_weights_request(mcpc, params, id, WEIGHTS_METHOD_ANOMALY_RATE, NULL);
}

void mcp_tool_find_anomalous_metrics_schema(BUFFER *buffer) {
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Find metrics with highest anomaly rates");
    
    buffer_json_member_add_object(buffer, "properties");
    {
        add_weights_time_parameters(buffer, false, true); // include_baseline=false, required=true
        add_weights_filter_parameters(buffer);
        add_weights_common_parameters(buffer);
    }
    buffer_json_object_close(buffer); // properties
    
    buffer_json_member_add_array(buffer, "required");
    {
        buffer_json_add_array_item_string(buffer, "after");
        buffer_json_add_array_item_string(buffer, "before");
    }
    buffer_json_array_close(buffer);
    
    buffer_json_object_close(buffer); // inputSchema
}

// find_unstable_metrics implementation
MCP_RETURN_CODE mcp_tool_find_unstable_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    // Use coefficient of variation for finding unstable metrics
    return execute_weights_request(mcpc, params, id, WEIGHTS_METHOD_VALUE, "cv");
}

void mcp_tool_find_unstable_metrics_schema(BUFFER *buffer) {
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Find metrics with high variability");
    
    buffer_json_member_add_object(buffer, "properties");
    {
        add_weights_time_parameters(buffer, false, true); // include_baseline=false, required=true
        add_weights_filter_parameters(buffer);
        add_weights_common_parameters(buffer);
    }
    buffer_json_object_close(buffer); // properties
    
    buffer_json_member_add_array(buffer, "required");
    {
        buffer_json_add_array_item_string(buffer, "after");
        buffer_json_add_array_item_string(buffer, "before");
    }
    buffer_json_array_close(buffer);
    
    buffer_json_object_close(buffer); // inputSchema
}
