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
static int mcp_tools_method_list(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    if (!mcpc || id == 0) return -1;

    struct json_object *result = json_object_new_object();
    struct json_object *tools_array = json_object_new_array();
    
    // Tool: explore_metrics
    struct json_object *metrics_tool = json_object_new_object();
    json_object_object_add(metrics_tool, "name", json_object_new_string("explore_metrics"));
    json_object_object_add(metrics_tool, "description", json_object_new_string(
        "Explore Netdata's time-series metrics with support for high-resolution data"
    ));
    
    // Input schema for metrics tool
    struct json_object *metrics_schema = json_object_new_object();
    json_object_object_add(metrics_schema, "type", json_object_new_string("object"));
    json_object_object_add(metrics_schema, "title", json_object_new_string("MetricsQuery"));
    
    struct json_object *metrics_props = json_object_new_object();
    
    // Context property
    struct json_object *context_prop = json_object_new_object();
    json_object_object_add(context_prop, "type", json_object_new_string("string"));
    json_object_object_add(context_prop, "title", json_object_new_string("Context"));
    json_object_object_add(metrics_props, "context", context_prop);
    
    // After property
    struct json_object *after_prop = json_object_new_object();
    json_object_object_add(after_prop, "type", json_object_new_string("integer"));
    json_object_object_add(after_prop, "title", json_object_new_string("After"));
    json_object_object_add(metrics_props, "after", after_prop);
    
    // Before property
    struct json_object *before_prop = json_object_new_object();
    json_object_object_add(before_prop, "type", json_object_new_string("integer"));
    json_object_object_add(before_prop, "title", json_object_new_string("Before"));
    json_object_object_add(metrics_props, "before", before_prop);
    
    // Points property
    struct json_object *points_prop = json_object_new_object();
    json_object_object_add(points_prop, "type", json_object_new_string("integer"));
    json_object_object_add(points_prop, "title", json_object_new_string("Points"));
    json_object_object_add(metrics_props, "points", points_prop);
    
    // Group property
    struct json_object *group_prop = json_object_new_object();
    json_object_object_add(group_prop, "type", json_object_new_string("string"));
    json_object_object_add(group_prop, "title", json_object_new_string("Group"));
    json_object_object_add(metrics_props, "group", group_prop);
    
    json_object_object_add(metrics_schema, "properties", metrics_props);
    
    // Required properties
    struct json_object *required = json_object_new_array();
    json_object_array_add(required, json_object_new_string("context"));
    json_object_object_add(metrics_schema, "required", required);
    
    json_object_object_add(metrics_tool, "inputSchema", metrics_schema);
    json_object_array_add(tools_array, metrics_tool);
    
    // Tool: explore_nodes
    struct json_object *nodes_tool = json_object_new_object();
    json_object_object_add(nodes_tool, "name", json_object_new_string("explore_nodes"));
    json_object_object_add(nodes_tool, "description", json_object_new_string(
        "Discover and explore all monitored nodes in your infrastructure"
    ));
    
    // Input schema for nodes tool
    struct json_object *nodes_schema = json_object_new_object();
    json_object_object_add(nodes_schema, "type", json_object_new_string("object"));
    json_object_object_add(nodes_schema, "title", json_object_new_string("NodesQuery"));
    
    struct json_object *nodes_props = json_object_new_object();
    
    // Filter property
    struct json_object *filter_prop = json_object_new_object();
    json_object_object_add(filter_prop, "type", json_object_new_string("string"));
    json_object_object_add(filter_prop, "title", json_object_new_string("Filter"));
    json_object_object_add(nodes_props, "filter", filter_prop);
    
    json_object_object_add(nodes_schema, "properties", nodes_props);
    json_object_object_add(nodes_tool, "inputSchema", nodes_schema);
    json_object_array_add(tools_array, nodes_tool);
    
    // Add tools array to result
    json_object_object_add(result, "tools", tools_array);
    
    // Send success response (and free the result object)
    int ret = mcp_send_success_response(mcpc, result, id);
    json_object_put(result);
    
    return ret;
}

// Stub implementations for other tools methods (transport-agnostic)
static int mcp_tools_method_execute(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "tools/execute", id);
}

static int mcp_tools_method_cancel(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "tools/cancel", id);
}

static int mcp_tools_method_status(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "tools/status", id);
}

static int mcp_tools_method_validate(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "tools/validate", id);
}

static int mcp_tools_method_describe(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "tools/describe", id);
}

static int mcp_tools_method_getCapabilities(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    if (!mcpc || id == 0) return -1;
    
    // Create a result object with tool capabilities
    struct json_object *result = json_object_new_object();
    
    // Add capabilities
    json_object_object_add(result, "listChanged", json_object_new_boolean(false));
    json_object_object_add(result, "asyncExecution", json_object_new_boolean(true));
    json_object_object_add(result, "batchExecution", json_object_new_boolean(true));
    
    // Send success response (and free the result object)
    int ret = mcp_send_success_response(mcpc, result, id);
    json_object_put(result);
    
    return ret;
}

// Tools namespace method dispatcher (transport-agnostic)
int mcp_tools_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return -1;

    netdata_log_debug(D_MCP, "MCP tools method: %s", method);

    if (strcmp(method, "list") == 0) {
        return mcp_tools_method_list(mcpc, params, id);
    }
    else if (strcmp(method, "execute") == 0) {
        return mcp_tools_method_execute(mcpc, params, id);
    }
    else if (strcmp(method, "cancel") == 0) {
        return mcp_tools_method_cancel(mcpc, params, id);
    }
    else if (strcmp(method, "status") == 0) {
        return mcp_tools_method_status(mcpc, params, id);
    }
    else if (strcmp(method, "validate") == 0) {
        return mcp_tools_method_validate(mcpc, params, id);
    }
    else if (strcmp(method, "describe") == 0) {
        return mcp_tools_method_describe(mcpc, params, id);
    }
    else if (strcmp(method, "getCapabilities") == 0) {
        return mcp_tools_method_getCapabilities(mcpc, params, id);
    }
    else {
        // Method not found in tools namespace
        char full_method[256];
        snprintf(full_method, sizeof(full_method), "tools/%s", method);
        return mcp_method_not_implemented_generic(mcpc, full_method, id);
    }
}
