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

// Define schema for the list_netdata_metrics tool
static void mcp_tool_metric_contexts_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Filter Metric Contexts");

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

static void mcp_tool_metric_context_categories_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Filter Metric Context Categories");

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
static MCP_RETURN_CODE mcp_tool_metric_contexts_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
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

    CLEAN_BUFFER *t = buffer_create(0, NULL);
    buffer_json_initialize(t, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    SIMPLE_PATTERN *pattern = NULL;
    if(like_pattern && *like_pattern)
        pattern = simple_pattern_create(like_pattern, "|", SIMPLE_PATTERN_EXACT, false);

    rrdcontext_context_registry_json_mcp_array(t, pattern);
    buffer_json_finalize(t);

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    {
        // Start building content array for the result
        buffer_json_member_add_array(mcpc->result, "content");
        {
            // Instead of returning embedded resources, let's return a text explanation
            // that will be more compatible with most LLM clients
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");
                buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(t));
            }
            buffer_json_object_close(mcpc->result); // Close text content
        }
        buffer_json_array_close(mcpc->result);  // Close content array
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON

    simple_pattern_free(pattern);

    return MCP_RC_OK;
}

static MCP_RETURN_CODE mcp_tool_metric_context_categories_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
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

    CLEAN_BUFFER *t = buffer_create(0, NULL);
    buffer_json_initialize(t, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    SIMPLE_PATTERN *pattern = NULL;
    if(like_pattern && *like_pattern)
        pattern = simple_pattern_create(like_pattern, "|", SIMPLE_PATTERN_EXACT, false);

    rrdcontext_context_registry_json_mcp_categories_array(t, pattern);
    buffer_json_finalize(t);

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    {
        // Start building content array for the result
        buffer_json_member_add_array(mcpc->result, "content");
        {
            // Instead of returning embedded resources, let's return a text explanation
            // that will be more compatible with most LLM clients
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");
                buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(t));
            }
            buffer_json_object_close(mcpc->result); // Close text content
        }
        buffer_json_array_close(mcpc->result);  // Close content array
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON

    simple_pattern_free(pattern);

    return MCP_RC_OK;
}

static void mcp_tool_context_details_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Filter metric contexts");

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    buffer_json_member_add_object(buffer, "contexts");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Pipe separated list of contexts or context patterns to be returned");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on context names. Examples: context1|context2, or even *word1*|*word2*, to match against contexts identifiers.");
        buffer_json_member_add_string(buffer, "default", "*");
    }
    buffer_json_object_close(buffer); // contexts

    buffer_json_member_add_object(buffer, "nodes");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Pipe separated list of nodes or node patterns, for which the contexts should be returned");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on nodes for slicing the metadata database of Netdata. Examples: node1|node2, or even *db*|*dns*, to match against hostnames");
        buffer_json_member_add_string(buffer, "default", "*");
    }
    buffer_json_object_close(buffer); // nodes

    buffer_json_member_add_object(buffer, "after");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Unix Epoch Timestamp, or negative number of seconds relative to parameter before");
        buffer_json_member_add_string(buffer, "description", "Limit the results to contexts that were collected after this timestamp. If negative, it will be interpreted as a number of seconds relative to the before parameter");
        buffer_json_member_add_int64(buffer, "default", 0);
    }
    buffer_json_object_close(buffer); // after

    buffer_json_member_add_object(buffer, "before");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Unix Epoch Timestamp, or negative number of seconds relative to now");
        buffer_json_member_add_string(buffer, "description", "Limit the results to contexts that were collected before this timestamp. If negative, it will be interpreted as a number of seconds relative now");
        buffer_json_member_add_int64(buffer, "default", 0);
    }
    buffer_json_object_close(buffer); // before

    buffer_json_object_close(buffer); // properties

    // No required fields
    buffer_json_object_close(buffer); // inputSchema
}

static void mcp_tool_contexts_search_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Filter metric contexts");

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    buffer_json_member_add_object(buffer, "q");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Pipe separated glob-like patterns to search for in all metadata");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching any string related to contexts, including contexts, titles, instances, dimensions, label keys, label values, families. Examples: *eth0*|*sda*, to find all charts related to network interface eth0 and disk sda.");
        buffer_json_member_add_string(buffer, "default", "*");
    }
    buffer_json_object_close(buffer); // q

    buffer_json_member_add_object(buffer, "contexts");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Pipe separated list of contexts or context patterns to be returned");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on context names. Examples: context1|context2, or even *word1*|*word2*, to match against contexts identifiers.");
        buffer_json_member_add_string(buffer, "default", "*");
    }
    buffer_json_object_close(buffer); // contexts

    buffer_json_member_add_object(buffer, "nodes");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Pipe separated list of nodes or node patterns, for which the contexts should be returned");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on nodes for slicing the metadata database of Netdata. Examples: node1|node2, or even *db*|*dns*, to match against hostnames");
        buffer_json_member_add_string(buffer, "default", "*");
    }
    buffer_json_object_close(buffer); // nodes

    buffer_json_member_add_object(buffer, "after");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Unix Epoch Timestamp, or negative number of seconds relative to parameter before");
        buffer_json_member_add_string(buffer, "description", "Limit the results to contexts that were collected after this timestamp. If negative, it will be interpreted as a number of seconds relative to the before parameter");
        buffer_json_member_add_int64(buffer, "default", 0);
    }
    buffer_json_object_close(buffer); // after

    buffer_json_member_add_object(buffer, "before");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Unix Epoch Timestamp, or negative number of seconds relative to now");
        buffer_json_member_add_string(buffer, "description", "Limit the results to contexts that were collected before this timestamp. If negative, it will be interpreted as a number of seconds relative now");
        buffer_json_member_add_int64(buffer, "default", 0);
    }
    buffer_json_object_close(buffer); // before

    buffer_json_object_close(buffer); // properties

    // No required fields
    buffer_json_object_close(buffer); // inputSchema
}

static MCP_RETURN_CODE mcp_tool_context_details_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id)
{
    if (!mcpc || id == 0)
        return MCP_RC_ERROR;

    // Extract the 'like' parameter if present
    const char *contexts_pattern = NULL;
    if (params && json_object_object_get_ex(params, "contexts", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "contexts", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            contexts_pattern = json_object_get_string(obj);
        }
    }

    const char *nodes_pattern = NULL;
    if (params && json_object_object_get_ex(params, "nodes", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "nodes", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            nodes_pattern = json_object_get_string(obj);
        }
    }

    time_t after = 0;
    if (params && json_object_object_get_ex(params, "after", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "after", &obj);
        if (obj && json_object_is_type(obj, json_type_int)) {
            after = json_object_get_int64(obj);
        }
    }

    time_t before = 0;
    if (params && json_object_object_get_ex(params, "before", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "before", &obj);
        if (obj && json_object_is_type(obj, json_type_int)) {
            before = json_object_get_int64(obj);
        }
    }

    CLEAN_BUFFER *t = buffer_create(0, NULL);
    buffer_json_initialize(t, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    struct api_v2_contexts_request req = {
        .scope_nodes = nodes_pattern,
        .scope_contexts = contexts_pattern,
        .after = after,
        .before = before,
    };

    int code = rrdcontext_to_json_v2(t, &req, CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_CONTEXT_TITLES | CONTEXTS_V2_MCP);
    if (code != HTTP_RESP_OK) {
        buffer_sprintf(mcpc->error, "Failed to fetch contexts, query returned http error code %d", code);
        return MCP_RC_ERROR;
    }

    buffer_json_finalize(t);

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    {
        // Start building content array for the result
        buffer_json_member_add_array(mcpc->result, "content");
        {
            // Instead of returning embedded resources, let's return a text explanation
            // that will be more compatible with most LLM clients
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");
                buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(t));
            }
            buffer_json_object_close(mcpc->result); // Close text content
        }
        buffer_json_array_close(mcpc->result);  // Close content array
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON

    return MCP_RC_OK;
}

static MCP_RETURN_CODE mcp_tool_contexts_search_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id)
{
    if (!mcpc || id == 0)
        return MCP_RC_ERROR;

    const char *q = NULL;
    if (params && json_object_object_get_ex(params, "q", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "q", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            q = json_object_get_string(obj);
        }
    }

    const char *contexts_pattern = NULL;
    if (params && json_object_object_get_ex(params, "contexts", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "contexts", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            contexts_pattern = json_object_get_string(obj);
        }
    }

    const char *nodes_pattern = NULL;
    if (params && json_object_object_get_ex(params, "nodes", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "nodes", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            nodes_pattern = json_object_get_string(obj);
        }
    }

    time_t after = 0;
    if (params && json_object_object_get_ex(params, "after", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "after", &obj);
        if (obj && json_object_is_type(obj, json_type_int)) {
            after = json_object_get_int64(obj);
        }
    }

    time_t before = 0;
    if (params && json_object_object_get_ex(params, "before", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "before", &obj);
        if (obj && json_object_is_type(obj, json_type_int)) {
            before = json_object_get_int64(obj);
        }
    }

    CLEAN_BUFFER *t = buffer_create(0, NULL);
    buffer_json_initialize(t, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    struct api_v2_contexts_request req = {
        .scope_nodes = nodes_pattern,
        .scope_contexts = contexts_pattern,
        .after = after,
        .before = before,
        .q = q,
    };

    int code = rrdcontext_to_json_v2(t, &req, CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_CONTEXT_TITLES | CONTEXTS_V2_MCP | CONTEXTS_V2_SEARCH);
    if (code != HTTP_RESP_OK) {
        buffer_sprintf(mcpc->error, "Failed to fetch contexts, query returned http error code %d", code);
        return MCP_RC_ERROR;
    }

    buffer_json_finalize(t);

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    {
        // Start building content array for the result
        buffer_json_member_add_array(mcpc->result, "content");
        {
            // Instead of returning embedded resources, let's return a text explanation
            // that will be more compatible with most LLM clients
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");
                buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(t));
            }
            buffer_json_object_close(mcpc->result); // Close text content
        }
        buffer_json_array_close(mcpc->result);  // Close content array
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON

    return MCP_RC_OK;
}

// Static array of tool definitions
static const MCP_TOOL_DEF mcp_tools[] = {
    {
        .name = "list_metric_context_categories",
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
        .name = MCP_LIST_METRIC_CONTEXTS_METHOD,
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
        .name = MCP_CONTEXT_DETAILS_METHOD,
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
        .name = MCP_CONTEXT_SEARCH_METHOD,
        .title = "Find relevant metric contexts using full text search",
        .description = "Search for contexts given search pattern matching context names, instances,\n"
                       "dimensions, label keys, label values, titles and related metadata.\n",
        .execute_callback = mcp_tool_contexts_search_execute,
        .schema_callback = mcp_tool_contexts_search_schema,
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
