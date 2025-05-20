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
 * Standard methods in the MCP specification:
 * 
 * 1. tools/list - Lists available tools with their schemas and metadata
 *    - Returns a list of all available tools
 *    - Includes each tool's name, description, and input schema
 * 
 * 2. tools/call - Executes a specific tool with provided parameters
 *    - Takes a tool name and arguments
 *    - Returns the result of the tool execution
 *    - May include resource references, text, images, or other content types
 * 
 * In the Netdata context, tools provide access to operations like:
 *    - Exploring metrics and their relationships
 *    - Analyzing time-series data patterns
 *    - Finding correlations between metrics
 *    - Root cause analysis for anomalies
 *    - Summarizing system health
 */

#include "mcp-tools.h"
#include "mcp-initialize.h"
#include "database/contexts/rrdcontext.h"

// Include tool-specific header files
#include "mcp-tools-metric-contexts.h"
#include "mcp-tools-metric-context-categories.h"
#include "mcp-tools-context-details.h"
#include "mcp-tools-contexts-search.h"
#include "mcp-tools-list-nodes.h"
#include "mcp-tools-node-details.h"
#include "mcp-tools-execute-function.h"
#include "mcp-tools-metrics-query.h"

// Tool handler function prototypes
typedef MCP_RETURN_CODE (*mcp_tool_execute_t)(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);
typedef void (*mcp_tool_schema_t)(BUFFER *schema_buffer);

// Tool definition structure
typedef struct {
    const char *name;         // Tool name
    const char *description;  // Tool description
    mcp_tool_execute_t execute_callback;  // Tool execution callback
    mcp_tool_schema_t schema_callback;    // Tool schema definition callback
    
    // UI/UX annotations
    const char *title;        // Human-readable title
    bool read_only_hint;      // If true, tool doesn't modify state
    bool open_world_hint;     // If true, tool interacts with external world
} MCP_TOOL_DEF;

// Static array of tool definitions
static const MCP_TOOL_DEF mcp_tools[] = {
    {
        .name = MCP_TOOL_METRIC_CONTEXT_CATEGORIES,
        .title = "Aggregated view of what's being monitored by Netdata.",
        .description = "Metric Contexts are the equivalent of Charts on Netdata dashboards.\n"
                       "Provides a summarized view of monitoring domains by grouping contexts by their prefix.\n"
                       "Useful for getting a quick overview of what's being monitored without fetching the whole list.\n",
        .execute_callback = mcp_tool_metric_context_categories_execute,
        .schema_callback = mcp_tool_metric_context_categories_schema,
        .read_only_hint = true,
        .open_world_hint = false
    },
    {
        .name = MCP_TOOL_METRIC_CONTEXTS,
        .title = "Primary discovery mechanism for what's being monitored by Netdata.",
        .description = "Metric Contexts are the equivalent of Charts on Netdata dashboards.\n"
                       "Contexts are multi-node, multi-instance and multi-dimensional, usually\n"
                       "aggregating metrics with common/similar labels and dimensions.\n",
        .execute_callback = mcp_tool_metric_contexts_execute,
        .schema_callback = mcp_tool_metric_contexts_schema,
        .read_only_hint = true,
        .open_world_hint = false
    },
    {
        .name = MCP_TOOL_CONTEXT_DETAILS,
        .title = "Get additional information for specific metric contexts.",
        .description = "Given a time-frame, a list of nodes and contexts, this tool\n"
                       "provides their names, titles, families, units, retention,\n"
                       "and whether they are currently collected or not.\n",
        .execute_callback = mcp_tool_context_details_execute,
        .schema_callback = mcp_tool_context_details_schema,
        .read_only_hint = true,
        .open_world_hint = false
    },
    {
        .name = MCP_TOOL_CONTEXT_SEARCH,
        .title = "Find relevant metric contexts using full text search",
        .description = "Search for contexts given search pattern matching context names, instances,\n"
                       "dimensions, label keys, label values, titles and related metadata.\n",
        .execute_callback = mcp_tool_contexts_search_execute,
        .schema_callback = mcp_tool_contexts_search_schema,
        .read_only_hint = true,
        .open_world_hint = false
    },
    {
        .name = MCP_TOOL_LIST_NODES,
        .title = "List all monitored nodes in the Netdata ecosystem",
        .description = "Provides a list of all nodes/agents in the Netdata infrastructure.\n"
                       "Includes information such as node IDs, hostnames, and connection status.\n"
                       "Useful for discovering the monitoring topology and available endpoints.\n",
        .execute_callback = mcp_tool_list_nodes_execute,
        .schema_callback = mcp_tool_list_nodes_schema,
        .read_only_hint = true,
        .open_world_hint = false
    },
    {
        .name = MCP_TOOL_NODE_DETAILS,
        .title = "Get detailed information about monitored nodes",
        .description = "Provides detailed information about nodes/agents in the Netdata infrastructure.\n"
                       "Includes comprehensive node metadata, system information, and configuration details.\n"
                       "Useful for deeper analysis of node properties and capabilities.\n",
        .execute_callback = mcp_tool_node_details_execute,
        .schema_callback = mcp_tool_node_details_schema,
        .read_only_hint = true,
        .open_world_hint = false
    },
    {
        .name = MCP_TOOL_EXECUTE_FUNCTION,
        .title = "Execute a function on a specific node",
        .description = "Execute a registered function on a specific node to get live information from it.\n"
                       "The node needs to be online/live/reachable.\n"
                       "Functions usually include `processes`, `network-connections`, `mount-points`, \n"
                       "`containers-vms`, `netdata-streaming` and more.\n"
                       "Use the " MCP_TOOL_NODE_DETAILS " tool to discover available functions for each node.",
        .execute_callback = mcp_tool_execute_function_execute,
        .schema_callback = mcp_tool_execute_function_schema,
        .read_only_hint = false, // This tool can modify state
        .open_world_hint = false
    },
    
    {
        .name = MCP_TOOL_METRICS_QUERY,
        .title = "Query metrics data",
        .description = "Query time-series metrics data from Netdata's database.\n"
                       "Provides querying capabilities with filtering by nodes, instances, and dimensions for a specific context.\n"
                       "Supports various data formats, time aggregation, and grouping options similar to the API v3 data endpoint.\n"
                       "NOTE: You must first use the " MCP_TOOL_METRIC_CONTEXTS " tool to discover available contexts.",
        .execute_callback = mcp_tool_metrics_query_execute,
        .schema_callback = mcp_tool_metrics_query_schema,
        .read_only_hint = true,
        .open_world_hint = false
    },

    // Add more tools here
    
    // Terminator
    {
        .name = NULL
    }
};

// Return a list of available tools
static MCP_RETURN_CODE mcp_tools_method_list(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id) {
    if (!mcpc || id == 0) return MCP_RC_ERROR;

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Create tools array
    buffer_json_member_add_array(mcpc->result, "tools");
    
    // Iterate through all defined tools and add them to the response
    for (size_t i = 0; mcp_tools[i].name != NULL; i++) {
        const MCP_TOOL_DEF *tool = &mcp_tools[i];
        
        // Add tool object
        buffer_json_add_array_item_object(mcpc->result);
        
        // Add basic properties
        buffer_json_member_add_string(mcpc->result, "name", tool->name);
        buffer_json_member_add_string(mcpc->result, "description", tool->description);
        
        // Add schema using the tool's schema callback
        tool->schema_callback(mcpc->result);
        
        // Add annotations if available
        buffer_json_member_add_object(mcpc->result, "annotations");
        if (tool->title) {
            buffer_json_member_add_string(mcpc->result, "title", tool->title);
        }
        buffer_json_member_add_boolean(mcpc->result, "readOnlyHint", tool->read_only_hint);
        buffer_json_member_add_boolean(mcpc->result, "openWorldHint", tool->open_world_hint);
        buffer_json_object_close(mcpc->result); // Close annotations
        
        buffer_json_object_close(mcpc->result); // Close tool object
    }
    
    buffer_json_array_close(mcpc->result); // Close tools array
    buffer_json_finalize(mcpc->result); // Finalize the JSON
    
    return MCP_RC_OK;
}

// Main execute method that routes to specific tool handlers
static MCP_RETURN_CODE mcp_tools_method_call(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !params || id == 0) return MCP_RC_ERROR;
    
    // Extract tool name
    struct json_object *name_obj = NULL;
    if (!json_object_object_get_ex(params, "name", &name_obj)) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'name'");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_is_type(name_obj, json_type_string)) {
        buffer_sprintf(mcpc->error, "Parameter 'name' must be a string");
        return MCP_RC_BAD_REQUEST;
    }
    
    const char *tool_name = json_object_get_string(name_obj);
    
    // Get arguments if present
    struct json_object *args_obj = NULL;
    json_object_object_get_ex(params, "arguments", &args_obj);
    
    // Search for the tool in our static array
    for (size_t i = 0; mcp_tools[i].name != NULL; i++) {
        if (strcmp(tool_name, mcp_tools[i].name) == 0) {
            // Found the tool, execute its callback
            return mcp_tools[i].execute_callback(mcpc, args_obj, id);
        }
    }
    
    // Tool not found
    buffer_sprintf(mcpc->error, "Unknown tool: %s", tool_name);
    return MCP_RC_BAD_REQUEST;
}

// The MCP specification only defines list and call methods for tools
// Other methods are not part of the standard specification

// Tools namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_tools_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;

    netdata_log_debug(D_MCP, "MCP tools method: %s", method);

    // Flush previous buffers
    buffer_flush(mcpc->result);
    buffer_flush(mcpc->error);
    
    MCP_RETURN_CODE rc;

    if (strcmp(method, "list") == 0) {
        // List available tools - standard method in MCP specification
        rc = mcp_tools_method_list(mcpc, params, id);
    }
    else if (strcmp(method, "call") == 0) {
        // Execute a tool - standard method in MCP specification
        rc = mcp_tools_method_call(mcpc, params, id);
    }
    else {
        // Method not found in tools namespace
        buffer_sprintf(mcpc->error, "Method 'tools/%s' not supported. The MCP specification only defines 'list' and 'call' methods.", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}
