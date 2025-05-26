// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-weights.h"
#include "mcp-tools.h"
#include "mcp-time-utils.h"
#include "web/api/web_api.h"
#include "web/api/queries/weights.h"
#include "web/api/queries/query.h"

// Common function to execute the weights request
static MCP_RETURN_CODE execute_weights_request(
    MCP_CLIENT *mcpc,
    struct json_object *params,
    MCP_REQUEST_ID id,
    WEIGHTS_METHOD method,
    const char *default_time_group
) {
    // Initialize response
    mcp_init_success_result(mcpc, id);
    
    // Extract time parameters using MCP time utilities
    time_t after = mcp_extract_time_param(params, "after", MCP_DEFAULT_AFTER_TIME);
    time_t before = mcp_extract_time_param(params, "before", MCP_DEFAULT_BEFORE_TIME);
    time_t baseline_after = 0;
    time_t baseline_before = 0;
    
    // For correlation methods (KS2, VOLUME), parse baseline times
    if (method == WEIGHTS_METHOD_MC_KS2 || method == WEIGHTS_METHOD_MC_VOLUME) {
        baseline_after = mcp_extract_time_param(params, "baseline_after", 0);
        baseline_before = mcp_extract_time_param(params, "baseline_before", 0);
        
        // If baseline not specified, auto-calculate as 4x the query window before the query window
        if (baseline_after == 0 && baseline_before == 0) {
            time_t window = before - after;
            baseline_before = after;
            baseline_after = baseline_before - (window * 4);
        }
    }
    
    // Parse other parameters
    const char *contexts = NULL;
    const char *nodes = NULL;
    const char *instances = NULL;
    const char *dimensions = NULL;
    const char *labels = NULL;

    struct json_object *obj;
    if (json_object_object_get_ex(params, "contexts", &obj) && json_object_is_type(obj, json_type_string))
        contexts = json_object_get_string(obj);
        
    if (json_object_object_get_ex(params, "nodes", &obj) && json_object_is_type(obj, json_type_string))
        nodes = json_object_get_string(obj);
        
    if (json_object_object_get_ex(params, "instances", &obj) && json_object_is_type(obj, json_type_string))
        instances = json_object_get_string(obj);
        
    if (json_object_object_get_ex(params, "dimensions", &obj) && json_object_is_type(obj, json_type_string))
        dimensions = json_object_get_string(obj);
        
    // Handle labels conversion
    BUFFER *labels_buffer = NULL;
    if (json_object_object_get_ex(params, "labels", &obj) && json_object_is_type(obj, json_type_object)) {
        // Convert labels object to query string format
        struct json_object_iterator it = json_object_iter_begin(obj);
        struct json_object_iterator itEnd = json_object_iter_end(obj);
        
        labels_buffer = buffer_create(256, NULL);
        int first = 1;
        
        while (!json_object_iter_equal(&it, &itEnd)) {
            const char *key = json_object_iter_peek_name(&it);
            struct json_object *val = json_object_iter_peek_value(&it);
            
            if (json_object_is_type(val, json_type_array)) {
                size_t array_len = json_object_array_length(val);
                for (size_t i = 0; i < array_len; i++) {
                    struct json_object *item = json_object_array_get_idx(val, i);
                    if (json_object_is_type(item, json_type_string)) {
                        if (!first) buffer_strcat(labels_buffer, "|");
                        buffer_sprintf(labels_buffer, "%s:%s", key, json_object_get_string(item));
                        first = 0;
                    }
                }
            }
            json_object_iter_next(&it);
        }
        
        if (buffer_strlen(labels_buffer) > 0)
            labels = buffer_tostring(labels_buffer);
    }
    
    // Get cardinality limit
    size_t cardinality_limit = 50; // Default for MCP
    if (json_object_object_get_ex(params, "cardinality_limit", &obj) && json_object_is_type(obj, json_type_int))
        cardinality_limit = json_object_get_int(obj);
        
    // Set time_group parameter based on method
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

    // Build the weights request structure
    QUERY_WEIGHTS_REQUEST qwr = {
        .version = 2,
        .host = NULL,  // Will query all hosts
        .scope_nodes = nodes,
        .scope_contexts = contexts,
        .scope_instances = instances,
        .scope_labels = labels,
        .scope_dimensions = dimensions,
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
        .timeout_ms = 30000,  // 30 second timeout
        .cardinality_limit = cardinality_limit,
        .interrupt_callback = NULL,
        .interrupt_callback_data = NULL,
        .transaction = NULL,
    };
    
    // Create a temporary buffer for the weights API response
    BUFFER *tmp_buffer = buffer_create(0, NULL);
    
    // Call the weights API function with the temporary buffer
    int http_code = web_api_v12_weights(tmp_buffer, &qwr);
    
    // Clean up
    if (labels_buffer)
        buffer_free(labels_buffer);
    
    // Handle response
    if (http_code != HTTP_RESP_OK) {
        buffer_free(tmp_buffer);
        buffer_flush(mcpc->result);
        buffer_flush(mcpc->error);
        
        switch (http_code) {
            case HTTP_RESP_BAD_REQUEST:
                buffer_sprintf(mcpc->error, "Invalid request parameters");
                return MCP_RC_BAD_REQUEST;
                
            case HTTP_RESP_NOT_FOUND:
                buffer_sprintf(mcpc->error, "No results found");
                return MCP_RC_NOT_FOUND;
                
            case HTTP_RESP_GATEWAY_TIMEOUT:
                buffer_sprintf(mcpc->error, "Request timed out");
                return MCP_RC_ERROR;
                
            default:
                buffer_sprintf(mcpc->error, "Internal error (HTTP %d)", http_code);
                return MCP_RC_INTERNAL_ERROR;
        }
    }
    
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
    
    // Clean up the temporary buffer
    buffer_free(tmp_buffer);
    
    return MCP_RC_OK;
}

// Schema helper for common time window parameters
static void add_weights_time_parameters(BUFFER *buffer, bool include_baseline) {
    mcp_schema_params_add_time_window(buffer, "metrics", true);
    
    if (include_baseline) {
        buffer_json_member_add_object(buffer, "baseline_after");
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Baseline start time");
        buffer_json_member_add_string(buffer, "description", 
            "Start time for the baseline period to compare against. If not specified, "
            "automatically set to 4x the query window before the query period. Accepts:\n"
            "- Unix timestamp in seconds\n"
            "- Negative values for relative time\n"
            "- RFC3339 datetime string");
        buffer_json_object_close(buffer);
        
        buffer_json_member_add_object(buffer, "baseline_before");
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Baseline end time");
        buffer_json_member_add_string(buffer, "description", 
            "End time for the baseline period. If not specified, automatically set to "
            "the start of the query period.");
        buffer_json_object_close(buffer);
    }
}

// Schema helper for common filter parameters
static void add_weights_filter_parameters(BUFFER *buffer) {
    buffer_json_member_add_object(buffer, "contexts");
    buffer_json_member_add_string(buffer, "type", "string");
    buffer_json_member_add_string(buffer, "title", "Filter by metric contexts");
    buffer_json_member_add_string(buffer, "description", 
        "Filter to specific metric types (e.g., 'system.cpu', 'disk.*', 'mysql.*'). "
        "Use pipe (|) to separate multiple patterns. Supports wildcards.");
    buffer_json_object_close(buffer);
    
    buffer_json_member_add_object(buffer, "nodes");
    buffer_json_member_add_string(buffer, "type", "string");
    buffer_json_member_add_string(buffer, "title", "Filter by nodes");
    buffer_json_member_add_string(buffer, "description", 
        "Filter to specific nodes (e.g., 'web-server-1', 'database-*'). "
        "Use pipe (|) to separate multiple patterns. Supports wildcards.");
    buffer_json_object_close(buffer);
    
    buffer_json_member_add_object(buffer, "instances");
    buffer_json_member_add_string(buffer, "type", "string");
    buffer_json_member_add_string(buffer, "title", "Filter by instances");
    buffer_json_member_add_string(buffer, "description", 
        "Filter to specific instances (e.g., 'eth0', 'sda', 'production_db'). "
        "Use pipe (|) to separate multiple patterns. Supports wildcards.");
    buffer_json_object_close(buffer);
    
    buffer_json_member_add_object(buffer, "dimensions");
    buffer_json_member_add_string(buffer, "type", "string");
    buffer_json_member_add_string(buffer, "title", "Filter by dimensions");
    buffer_json_member_add_string(buffer, "description", 
        "Filter to specific dimensions (e.g., 'user', 'writes', 'slow_queries'). "
        "Use pipe (|) to separate multiple patterns. Supports wildcards.");
    buffer_json_object_close(buffer);
    
    buffer_json_member_add_object(buffer, "labels");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Filter by labels");
    buffer_json_member_add_string(buffer, "description", 
        "Filter using labels where each key maps to an array of values. "
        "Values in the same array are ORed, different keys are ANDed. "
        "Example: {\"disk_type\": [\"ssd\", \"nvme\"], \"mount_point\": [\"/\"]}");
    buffer_json_member_add_object(buffer, "additionalProperties");
    buffer_json_member_add_string(buffer, "type", "array");
    buffer_json_member_add_object(buffer, "items");
    buffer_json_member_add_string(buffer, "type", "string");
    buffer_json_object_close(buffer); // items
    buffer_json_object_close(buffer); // additionalProperties
    buffer_json_object_close(buffer); // labels
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
    
    add_weights_time_parameters(buffer, true);
    add_weights_filter_parameters(buffer);
    
    buffer_json_member_add_object(buffer, "method");
    buffer_json_member_add_string(buffer, "type", "string");
    buffer_json_member_add_string(buffer, "title", "Correlation method");
    buffer_json_member_add_string(buffer, "description", 
        "Algorithm to use:\n"
        "- 'ks2': Statistical distribution comparison (slow, but intelligent)\n"
        "- 'volume': Percentage change in averages (fast, works well for most cases)");
    buffer_json_member_add_array(buffer, "enum");
    buffer_json_add_array_item_string(buffer, "ks2");
    buffer_json_add_array_item_string(buffer, "volume");
    buffer_json_array_close(buffer);
    buffer_json_member_add_string(buffer, "default", "volume");
    buffer_json_object_close(buffer); // method
    
    buffer_json_member_add_object(buffer, "cardinality_limit");
    buffer_json_member_add_string(buffer, "type", "number");
    buffer_json_member_add_string(buffer, "title", "Maximum results");
    buffer_json_member_add_string(buffer, "description", "Maximum number of results to return");
    buffer_json_member_add_int64(buffer, "default", 50);
    buffer_json_member_add_int64(buffer, "minimum", 1);
    buffer_json_member_add_int64(buffer, "maximum", 200);
    buffer_json_object_close(buffer); // cardinality_limit
    
    buffer_json_object_close(buffer); // properties
    
    buffer_json_member_add_array(buffer, "required");
    buffer_json_add_array_item_string(buffer, "after");
    buffer_json_add_array_item_string(buffer, "before");
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
    
    add_weights_time_parameters(buffer, false);
    add_weights_filter_parameters(buffer);
    
    buffer_json_member_add_object(buffer, "cardinality_limit");
    buffer_json_member_add_string(buffer, "type", "number");
    buffer_json_member_add_string(buffer, "title", "Maximum results");
    buffer_json_member_add_string(buffer, "description", "Maximum number of results to return");
    buffer_json_member_add_int64(buffer, "default", 50);
    buffer_json_member_add_int64(buffer, "minimum", 1);
    buffer_json_member_add_int64(buffer, "maximum", 200);
    buffer_json_object_close(buffer); // cardinality_limit
    
    buffer_json_object_close(buffer); // properties
    
    buffer_json_member_add_array(buffer, "required");
    buffer_json_add_array_item_string(buffer, "after");
    buffer_json_add_array_item_string(buffer, "before");
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
    
    add_weights_time_parameters(buffer, false);
    add_weights_filter_parameters(buffer);
    
    buffer_json_member_add_object(buffer, "cardinality_limit");
    buffer_json_member_add_string(buffer, "type", "number");
    buffer_json_member_add_string(buffer, "title", "Maximum results");
    buffer_json_member_add_string(buffer, "description", "Maximum number of results to return");
    buffer_json_member_add_int64(buffer, "default", 50);
    buffer_json_member_add_int64(buffer, "minimum", 1);
    buffer_json_member_add_int64(buffer, "maximum", 200);
    buffer_json_object_close(buffer); // cardinality_limit
    
    buffer_json_object_close(buffer); // properties
    
    buffer_json_member_add_array(buffer, "required");
    buffer_json_add_array_item_string(buffer, "after");
    buffer_json_add_array_item_string(buffer, "before");
    buffer_json_array_close(buffer);
    
    buffer_json_object_close(buffer); // inputSchema
}