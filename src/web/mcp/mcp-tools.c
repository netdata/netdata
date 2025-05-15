// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Tools Namespace
 * 
 * The MCP Tools namespace provides methods for discovering and executing tools offered by the server.
 * In the MCP protocol, tools are discrete operations that clients can invoke to perform specific actions.
 * 
 * Tools are model-controlled actions - meaning the AI decides when and how to use them based on context.
 * Each tool has a defined input schema that specifies required and optional parameters.
 * 
 * Key features of the tools namespace:
 * 
 * 1. Tool Discovery:
 *    - Clients can list available tools (tools/list)
 *    - Get detailed descriptions of specific tools (tools/describe)
 *    - Understand what parameters a tool requires (through JSON Schema)
 * 
 * 2. Tool Execution:
 *    - Execute tools with specific parameters (tools/execute)
 *    - Validate parameters without execution (tools/validate)
 *    - Asynchronous execution is supported for long-running tools
 * 
 * 3. Execution Management:
 *    - Check execution status (tools/status)
 *    - Cancel running executions (tools/cancel)
 * 
 * In the Netdata context, tools provide access to operations like:
 *    - Exploring metrics and their relationships
 *    - Analyzing time-series data patterns
 *    - Finding correlations between metrics
 *    - Root cause analysis for anomalies
 *    - Summarizing system health
 * 
 * Each tool execution is assigned a unique ID, allowing clients to track and manage executions.
 */

#include "mcp-tools.h"
#include "mcp-initialize.h"

// Return a list of available tools (transport-agnostic)
static MCP_RETURN_CODE mcp_tools_method_list(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    if (!mcpc || id == 0) return MCP_RC_ERROR;

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Create tools array
    buffer_json_member_add_array(mcpc->result, "tools");
    
    // Add explore_metrics tool
    buffer_json_add_array_item_object(mcpc->result);
    buffer_json_member_add_string(mcpc->result, "name", "explore_metrics");
    buffer_json_member_add_string(mcpc->result, "description", 
        "Explore Netdata's time-series metrics with support for high-resolution data");
    
    // Add input schema for metrics tool
    buffer_json_member_add_object(mcpc->result, "inputSchema");
    buffer_json_member_add_string(mcpc->result, "type", "object");
    buffer_json_member_add_string(mcpc->result, "title", "MetricsQuery");
    
    // Properties
    buffer_json_member_add_object(mcpc->result, "properties");
    
    // Context property
    buffer_json_member_add_object(mcpc->result, "context");
    buffer_json_member_add_string(mcpc->result, "type", "string");
    buffer_json_member_add_string(mcpc->result, "title", "Context");
    buffer_json_object_close(mcpc->result); // Close context
    
    // After property
    buffer_json_member_add_object(mcpc->result, "after");
    buffer_json_member_add_string(mcpc->result, "type", "integer");
    buffer_json_member_add_string(mcpc->result, "title", "After");
    buffer_json_object_close(mcpc->result); // Close after
    
    // Before property
    buffer_json_member_add_object(mcpc->result, "before");
    buffer_json_member_add_string(mcpc->result, "type", "integer");
    buffer_json_member_add_string(mcpc->result, "title", "Before");
    buffer_json_object_close(mcpc->result); // Close before
    
    // Points property
    buffer_json_member_add_object(mcpc->result, "points");
    buffer_json_member_add_string(mcpc->result, "type", "integer");
    buffer_json_member_add_string(mcpc->result, "title", "Points");
    buffer_json_object_close(mcpc->result); // Close points
    
    // Group property
    buffer_json_member_add_object(mcpc->result, "group");
    buffer_json_member_add_string(mcpc->result, "type", "string");
    buffer_json_member_add_string(mcpc->result, "title", "Group");
    buffer_json_object_close(mcpc->result); // Close group
    
    buffer_json_object_close(mcpc->result); // Close properties
    
    // Required properties
    buffer_json_member_add_array(mcpc->result, "required");
    buffer_json_add_array_item_string(mcpc->result, "context");
    buffer_json_array_close(mcpc->result); // Close required
    
    buffer_json_object_close(mcpc->result); // Close inputSchema
    buffer_json_object_close(mcpc->result); // Close explore_metrics tool
    
    // Add explore_nodes tool
    buffer_json_add_array_item_object(mcpc->result);
    buffer_json_member_add_string(mcpc->result, "name", "explore_nodes");
    buffer_json_member_add_string(mcpc->result, "description", 
        "Discover and explore all monitored nodes in your infrastructure");
    
    // Add input schema for nodes tool
    buffer_json_member_add_object(mcpc->result, "inputSchema");
    buffer_json_member_add_string(mcpc->result, "type", "object");
    buffer_json_member_add_string(mcpc->result, "title", "NodesQuery");
    
    // Properties
    buffer_json_member_add_object(mcpc->result, "properties");
    
    // Filter property
    buffer_json_member_add_object(mcpc->result, "filter");
    buffer_json_member_add_string(mcpc->result, "type", "string");
    buffer_json_member_add_string(mcpc->result, "title", "Filter");
    buffer_json_object_close(mcpc->result); // Close filter
    
    buffer_json_object_close(mcpc->result); // Close properties
    buffer_json_object_close(mcpc->result); // Close inputSchema
    buffer_json_object_close(mcpc->result); // Close explore_nodes tool
    
    buffer_json_array_close(mcpc->result); // Close tools array
    buffer_json_finalize(mcpc->result); // Finalize the JSON
    
    return MCP_RC_OK;
}

// Stub implementations for other tools methods (transport-agnostic)
static MCP_RETURN_CODE mcp_tools_method_execute(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'tools/execute' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_tools_method_cancel(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'tools/cancel' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_tools_method_status(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'tools/status' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_tools_method_validate(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'tools/validate' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_tools_method_describe(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'tools/describe' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_tools_method_getCapabilities(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    if (!mcpc || id == 0) return MCP_RC_ERROR;
    
    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Add capabilities as result object properties
    buffer_json_member_add_boolean(mcpc->result, "listChanged", false);
    buffer_json_member_add_boolean(mcpc->result, "asyncExecution", true);
    buffer_json_member_add_boolean(mcpc->result, "batchExecution", true);
    
    // Close the result object
    buffer_json_finalize(mcpc->result);
    
    return MCP_RC_OK;
}

// Tools namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_tools_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;

    netdata_log_debug(D_MCP, "MCP tools method: %s", method);

    // Flush previous buffers
    buffer_flush(mcpc->result);
    buffer_flush(mcpc->error);
    
    MCP_RETURN_CODE rc;

    if (strcmp(method, "list") == 0) {
        rc = mcp_tools_method_list(mcpc, params, id);
    }
    else if (strcmp(method, "execute") == 0) {
        rc = mcp_tools_method_execute(mcpc, params, id);
    }
    else if (strcmp(method, "cancel") == 0) {
        rc = mcp_tools_method_cancel(mcpc, params, id);
    }
    else if (strcmp(method, "status") == 0) {
        rc = mcp_tools_method_status(mcpc, params, id);
    }
    else if (strcmp(method, "validate") == 0) {
        rc = mcp_tools_method_validate(mcpc, params, id);
    }
    else if (strcmp(method, "describe") == 0) {
        rc = mcp_tools_method_describe(mcpc, params, id);
    }
    else if (strcmp(method, "getCapabilities") == 0) {
        rc = mcp_tools_method_getCapabilities(mcpc, params, id);
    }
    else {
        // Method not found in tools namespace
        buffer_sprintf(mcpc->error, "Method 'tools/%s' not implemented yet", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}
