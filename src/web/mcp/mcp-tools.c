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

// Tool schema generator functions
static void mcp_tool_list_netdata_metrics_schema(BUFFER *buffer);

// Tool execution functions
static MCP_RETURN_CODE mcp_tool_list_netdata_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id);

// Static array of tool definitions
static const MCP_TOOL_DEF mcp_tools[] = {
    {
        .name = "list_netdata_metrics",
        .description = "List available metric contexts and context categories with optional pattern matching",
        .execute_callback = mcp_tool_list_netdata_metrics_execute,
        .schema_callback = mcp_tool_list_netdata_metrics_schema,
        .title = "Metric Contexts and Categories",
        .read_only_hint = true,
        .open_world_hint = false
    },
    
    // Add more tools here
    
    // Terminator
    {
        .name = NULL
    }
};

// Define schema for the list_netdata_metrics tool
static void mcp_tool_list_netdata_metrics_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "MetricsQuery");
    
    // Properties
    buffer_json_member_add_object(buffer, "properties");
    
    // Like property (optional)
    buffer_json_member_add_object(buffer, "like");
    buffer_json_member_add_string(buffer, "type", "string");
    buffer_json_member_add_string(buffer, "title", "Pattern");
    buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on context and category names");
    buffer_json_object_close(buffer); // Close like
    
    buffer_json_object_close(buffer); // Close properties
    
    // No required fields
    buffer_json_object_close(buffer); // Close inputSchema
}

// Implementation of the list_netdata_metrics tool
static MCP_RETURN_CODE mcp_tool_list_netdata_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || id == 0) return MCP_RC_ERROR;
    
    // Extract the 'like' parameter if present
    const char *like_pattern = NULL;
    if (params && json_object_object_get_ex(params, "like", NULL)) {
        struct json_object *like_obj = NULL;
        json_object_object_get_ex(params, "like", &like_obj);
        if (like_obj && json_object_is_type(like_obj, json_type_string)) {
            like_pattern = json_object_get_string(like_obj);
        }
    }
    
    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Start building content array for the result
    buffer_json_member_add_array(mcpc->result, "content");
    
    // Build query string for resource URIs
    char query_param[256] = "";
    if (like_pattern && *like_pattern) {
        snprintfz(query_param, sizeof(query_param), "?like=%s", like_pattern);
    }
    
    // Instead of returning embedded resources, let's return a text explanation
    // that will be more compatible with most LLM clients
    buffer_json_add_array_item_object(mcpc->result);
    buffer_json_member_add_string(mcpc->result, "type", "text");
    
    // Format an explanatory message with information about both resources
    char explanation[1024];
    snprintfz(explanation, sizeof(explanation), 
              "Available Netdata metrics information:\n\n"
              "1. Context list - Shows all available metric contexts in the system\n"
              "   URI: nd://contexts%s\n\n"
              "2. Context categories - Shows how metrics are organized by category\n"
              "   URI: nd://context_categories%s\n\n"
              "These metrics are used to monitor system performance and health.",
              query_param, query_param);
    
    buffer_json_member_add_string(mcpc->result, "text", explanation);
    buffer_json_object_close(mcpc->result); // Close text content
    
    // Still include the resources for clients that support them
    // Add resource reference for contexts
    buffer_json_add_array_item_object(mcpc->result);
    buffer_json_member_add_string(mcpc->result, "type", "resource");
    buffer_json_member_add_object(mcpc->result, "resource");
    char contexts_uri[300];
    snprintfz(contexts_uri, sizeof(contexts_uri), "nd://contexts%s", query_param);
    buffer_json_member_add_string(mcpc->result, "uri", contexts_uri);
    buffer_json_object_close(mcpc->result); // Close resource object
    buffer_json_object_close(mcpc->result); // Close first resource content
    
    // Add resource reference for context categories
    buffer_json_add_array_item_object(mcpc->result);
    buffer_json_member_add_string(mcpc->result, "type", "resource");
    buffer_json_member_add_object(mcpc->result, "resource");
    char categories_uri[300];
    snprintfz(categories_uri, sizeof(categories_uri), "nd://context_categories%s", query_param);
    buffer_json_member_add_string(mcpc->result, "uri", categories_uri);
    buffer_json_object_close(mcpc->result); // Close resource object
    buffer_json_object_close(mcpc->result); // Close second resource content
    
    buffer_json_array_close(mcpc->result); // Close content array
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON
    
    return MCP_RC_OK;
}

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