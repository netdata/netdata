// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-alert-transitions.h"
#include "mcp-params.h"
#include "database/contexts/api_v2_contexts.h"
#include "database/contexts/rrdcontext.h"

// Schema for alert transitions
void mcp_tool_list_alert_transitions_schema(BUFFER *buffer) {
    // Tool metadata
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "List alert transitions");
    
    buffer_json_member_add_object(buffer, "properties");
    
    // Nodes filter
    mcp_schema_add_array_param(buffer, "nodes", "Filter nodes",
        "Array of specific node names to filter by. "
        "Each node must be an exact match - no wildcards or patterns allowed. "
        "Use 'list_nodes' to discover available nodes. "
        "If not specified, all nodes are included. "
        "Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]",
        false);
    
    // Time range
    mcp_schema_add_time_params(buffer, 
        "transitions that occurred",
        false);
    
    // Last N transitions
    mcp_schema_add_size_param(buffer, "last", 
        "Number of transitions",
        "Number of most recent alert transitions to return. Must be between 1 and 100.",
        1, 1, 100, false);
    
    // Facet filters
    mcp_schema_add_string_param(buffer, "status",
        "Filter by status",
        "Filter by alert status. Examples: 'WARNING', 'CRITICAL', 'CLEAR'",
        NULL, false);
    
    mcp_schema_add_string_param(buffer, "class",
        "Filter by classification", 
        "Filter by alert classification. Examples: 'Errors', 'Latency', 'Utilization'",
        NULL, false);
    
    mcp_schema_add_string_param(buffer, "type",
        "Filter by type",
        "Filter by alert type. Examples: 'System', 'Web Server', 'Database'",
        NULL, false);
    
    mcp_schema_add_string_param(buffer, "component",
        "Filter by component",
        "Filter by component. Examples: 'Network', 'Disk', 'Memory'",
        NULL, false);
    
    mcp_schema_add_string_param(buffer, "role",
        "Filter by role",
        "Filter by role. Examples: 'sysadmin', 'webmaster', 'dba'",
        NULL, false);
    
    mcp_schema_add_string_param(buffer, "alert",
        "Filter by alert name",
        "Filter by alert name pattern. Supports wildcards.",
        NULL, false);
    
    mcp_schema_add_string_param(buffer, "instance",
        "Filter by metric instance name",
        "Filter by chart name pattern. Supports wildcards.",
        NULL, false);
    
    mcp_schema_add_string_param(buffer, "context",
        "Filter by context",
        "Filter by context pattern. Supports wildcards.",
        NULL, false);
    
    // Pagination cursor
    mcp_schema_add_string_param(buffer, "cursor",
        "Pagination cursor",
        "Pagination cursor from previous response. Use the 'nextCursor' value from the previous response to get the next page of results.",
        NULL, false);
    
    // Timeout parameter
    mcp_schema_add_timeout(buffer, "timeout",
        "Query timeout",
        "Maximum time to wait for the query to complete (in seconds)",
        60, 1, 3600, false);
    
    buffer_json_object_close(buffer); // properties
    buffer_json_object_close(buffer); // inputSchema
}

// Execute alert transitions query
MCP_RETURN_CODE mcp_tool_list_alert_transitions_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || id == 0)
        return MCP_RC_ERROR;
    
    // Extract nodes array
    const char *nodes_pattern = NULL;
    CLEAN_BUFFER *nodes_buffer = NULL;
    const char *error_message = NULL;
    
    nodes_buffer = mcp_params_parse_array_to_pattern(params, "nodes", false, MCP_TOOL_LIST_NODES, &error_message);
    if (error_message) {
        buffer_sprintf(mcpc->error, "%s", error_message);
        return MCP_RC_BAD_REQUEST;
    }
    if (nodes_buffer)
        nodes_pattern = buffer_tostring(nodes_buffer);
    
    // Extract time parameters
    time_t after = mcp_params_parse_time(params, "after", MCP_DEFAULT_AFTER_TIME);
    time_t before = mcp_params_parse_time(params, "before", MCP_DEFAULT_BEFORE_TIME);
    
    // Extract last transitions count
    const char *size_error = NULL;
    size_t last_transitions = mcp_params_extract_size(params, "last", 1, 1, 100, &size_error);
    if (size_error) {
        buffer_sprintf(mcpc->error, "%s", size_error);
        return MCP_RC_BAD_REQUEST;
    }
    
    // Extract pagination cursor (global_id_anchor)
    usec_t global_id_anchor = 0;
    const char *cursor = mcp_params_extract_string(params, "cursor", NULL);
    if (cursor) {
        // Parse cursor as usec_t
        char *endptr;
        unsigned long long value = strtoull(cursor, &endptr, 10);
        if (*endptr != '\0' || endptr == cursor) {
            buffer_sprintf(mcpc->error, "Invalid cursor value");
            return MCP_RC_BAD_REQUEST;
        }
        global_id_anchor = (usec_t)value;
    }
    
    // Extract timeout parameter
    const char *timeout_error = NULL;
    int timeout = mcp_params_extract_timeout(params, "timeout", 60, 1, 3600, &timeout_error);
    if (timeout_error) {
        buffer_sprintf(mcpc->error, "%s", timeout_error);
        return MCP_RC_BAD_REQUEST;
    }
    
    // Create request structure
    CLEAN_BUFFER *t = buffer_create(0, NULL);
    struct api_v2_contexts_request req = {
        .scope_nodes = nodes_pattern,
        .scope_contexts = NULL,
        .nodes = NULL,
        .contexts = NULL,
        .after = after,
        .before = before,
        .timeout_ms = timeout * 1000,  // Convert seconds to milliseconds
        .options = CONTEXTS_OPTION_CONFIGURATIONS | CONTEXTS_OPTION_MCP | CONTEXTS_OPTION_RFC3339 | CONTEXTS_OPTION_JSON_LONG_KEYS | CONTEXTS_OPTION_MINIFY,
    };
    
    // Set up alerts section
    req.alerts.last = (uint32_t)last_transitions;
    req.alerts.global_id_anchor = global_id_anchor;
    
    // Extract facet filters
    req.alerts.facets[ATF_STATUS] = mcp_params_extract_string(params, "status", NULL);
    req.alerts.facets[ATF_CLASS] = mcp_params_extract_string(params, "class", NULL);
    req.alerts.facets[ATF_TYPE] = mcp_params_extract_string(params, "type", NULL);
    req.alerts.facets[ATF_COMPONENT] = mcp_params_extract_string(params, "component", NULL);
    req.alerts.facets[ATF_ROLE] = mcp_params_extract_string(params, "role", NULL);
    req.alerts.facets[ATF_NODE] = NULL; // Already handled via nodes parameter
    req.alerts.facets[ATF_ALERT_NAME] = mcp_params_extract_string(params, "alert", NULL);
    req.alerts.facets[ATF_CHART_NAME] = mcp_params_extract_string(params, "instance", NULL);
    req.alerts.facets[ATF_CONTEXT] = mcp_params_extract_string(params, "context", NULL);
    
    // Execute the query
    buffer_flush(t);
    buffer_json_initialize(t, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    
    CONTEXTS_V2_MODE mode = CONTEXTS_V2_NODES | CONTEXTS_V2_ALERT_TRANSITIONS;
    int response = rrdcontext_to_json_v2(t, &req, mode);
    
    if (response != HTTP_RESP_OK) {
        buffer_sprintf(mcpc->error, "Query failed with response code %d", response);
        return MCP_RC_ERROR;
    }
    
    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Start building content array for the result
    buffer_json_member_add_array(mcpc->result, "content");
    {
        // Return text content for LLM compatibility
        buffer_json_add_array_item_object(mcpc->result);
        {
            buffer_json_member_add_string(mcpc->result, "type", "text");
            buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(t));
        }
        buffer_json_object_close(mcpc->result); // Close text content
    }
    buffer_json_array_close(mcpc->result);  // Close content array
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON
    
    return MCP_RC_OK;
}