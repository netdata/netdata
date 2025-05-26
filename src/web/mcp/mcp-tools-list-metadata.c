// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-list-metadata.h"
#include "mcp-tools.h"
#include "mcp-time-utils.h"
#include "mcp-params.h"
#include "database/contexts/rrdcontext.h"

// Static configuration for all list tools
static const MCP_LIST_TOOL_CONFIG mcp_list_tools[] = {
    {
        .name = MCP_TOOL_LIST_METRICS,
        .title = "List available metrics",
        .description = "Search and list available metrics to query, across some or all nodes, for any time-frame",
        .output_type = MCP_LIST_OUTPUT_METRICS,
        .mode = CONTEXTS_V2_CONTEXTS,
        .options = 0,  // Just MCP will be added
        .params = {
            .has_q = true,
            .has_metrics = true,
            .has_nodes = true,
            .has_time_range = true,
            .has_cardinality_limit = true,
            .nodes_as_array = true,  // list_metrics uses array for nodes
        },
    },
    {
        .name = MCP_TOOL_GET_METRICS_DETAILS,
        .title = "Get metrics details",
        .description = "Get retention and cardinality information about specific metrics",
        .output_type = MCP_LIST_OUTPUT_METRICS,
        .mode = CONTEXTS_V2_CONTEXTS,
        .options = CONTEXTS_OPTION_TITLES | CONTEXTS_OPTION_INSTANCES | 
                   CONTEXTS_OPTION_DIMENSIONS | CONTEXTS_OPTION_LABELS | 
                   CONTEXTS_OPTION_RETENTION | CONTEXTS_OPTION_LIVENESS | 
                   CONTEXTS_OPTION_FAMILY | CONTEXTS_OPTION_UNITS,
        .params = {
            .has_metrics = true,
            .has_nodes = true,
            .has_time_range = true,
            .has_cardinality_limit = true,
            .metrics_required = true,
        },
    },
    {
        .name = MCP_TOOL_LIST_NODES,
        .title = "List monitored nodes",
        .description = "Search for and list monitored nodes by hostname patterns. Use the 'nodes' parameter to search for specific nodes instead of retrieving all nodes",
        .output_type = MCP_LIST_OUTPUT_NODES,
        .mode = CONTEXTS_V2_NODES,
        .options = 0,  // Just MCP will be added
        .params = {
            .has_nodes = true,
            .has_metrics = true,  // Filters nodes that collect these metrics
            .has_time_range = true,
            .has_cardinality_limit = true,
        },
    },
    {
        .name = MCP_TOOL_LIST_FUNCTIONS,
        .title = "List available functions",
        .description = "List available Netdata functions that can be executed on specific nodes",
        .output_type = MCP_LIST_OUTPUT_FUNCTIONS,
        .mode = CONTEXTS_V2_FUNCTIONS,
        .options = 0,
        .params = {
            .has_nodes = true,
            .has_time_range = false,  // Functions are live, not historical
            .has_cardinality_limit = false,  // Functions list is small, no limit needed
            .nodes_required = true,   // Must specify which nodes to query
        },
    },
    {
        .name = MCP_TOOL_GET_NODES_DETAILS,
        .title = "Get detailed information about monitored nodes",
        .description = "Gets comprehensive node information including hardware specs, OS details, capabilities, health status, available functions, streaming and monitoring configuration",
        .output_type = MCP_LIST_OUTPUT_NODES,
        .mode = CONTEXTS_V2_NODES | CONTEXTS_V2_NODES_INFO | CONTEXTS_V2_NODE_INSTANCES,
        .options = 0,  // Just MCP will be added
        .params = {
            .has_nodes = true,
            .has_metrics = true,  // Filters nodes that collect these metrics
            .has_time_range = true,
            .has_cardinality_limit = true,
            .nodes_required = true,   // Must specify nodes due to large output
            .nodes_as_array = true,   // get_nodes_details uses array for nodes
        },
    },
};

// Get tool configuration by name
const MCP_LIST_TOOL_CONFIG *mcp_get_list_tool_config(const char *name) {
    for (size_t i = 0; i < sizeof(mcp_list_tools) / sizeof(mcp_list_tools[0]); i++) {
        if (strcmp(mcp_list_tools[i].name, name) == 0) {
            return &mcp_list_tools[i];
        }
    }
    return NULL;
}

// Get tool by index
const MCP_LIST_TOOL_CONFIG *mcp_get_list_tool_by_index(size_t index) {
    if (index >= sizeof(mcp_list_tools) / sizeof(mcp_list_tools[0])) {
        return NULL;
    }
    return &mcp_list_tools[index];
}

// Get total count of list tools
size_t mcp_get_list_tools_count(void) {
    return sizeof(mcp_list_tools) / sizeof(mcp_list_tools[0]);
}

// Unified schema generation
void mcp_unified_list_tool_schema(BUFFER *buffer, const MCP_LIST_TOOL_CONFIG *config) {
    if (!buffer || !config) return;
    
    // Determine output type name
    const char *output_name;
    switch (config->output_type) {
        case MCP_LIST_OUTPUT_NODES:
            output_name = "nodes";
            break;
        case MCP_LIST_OUTPUT_METRICS:
            output_name = "metrics";
            break;
        case MCP_LIST_OUTPUT_FUNCTIONS:
            output_name = "functions";
            break;
        default:
            output_name = "items";
            break;
    }
    
    // Buffers for composing strings
    char title[256];
    char description[1024];
    
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", config->title);

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    // Add metrics pattern if supported
    if (config->params.has_metrics) {
        buffer_json_member_add_object(buffer, "metrics");
        buffer_json_member_add_string(buffer, "type", "string");
        
        // Compose title and description based on context
        if (config->output_type != MCP_LIST_OUTPUT_METRICS) {
            // This is a node/function query - metrics acts as a filter
            snprintfz(title, sizeof(title), "%s metrics", 
                      config->params.metrics_required ? "Specify the" : "Filter");
            snprintfz(description, sizeof(description),
                      "Filter %s to only those collecting these metrics. "
                      "Use pipe (|) to separate multiple patterns. Supports wildcards. "
                      "Examples: 'system.*', '*cpu*|*memory*', 'disk.*|net.*'", 
                      output_name);
        } else {
            // This is a metrics query
            snprintfz(title, sizeof(title), "%s metrics", 
                      config->params.metrics_required ? "Specify the" : "Filter");
            if (config->params.metrics_required) {
                snprintfz(description, sizeof(description),
                          "Pipe-separated list of metric names. "
                          "Example: 'system.cpu|system.load|system.ram'");
            } else {
                snprintfz(description, sizeof(description),
                          "Pattern matching on metric names. Use pipe (|) to separate multiple patterns. "
                          "Supports wildcards. Examples: 'system.*', '*cpu*|*memory*', 'disk.*|net.*|system.*'");
            }
        }
        
        buffer_json_member_add_string(buffer, "title", title);
        buffer_json_member_add_string(buffer, "description", description);
        if (!config->params.metrics_required) {
            buffer_json_member_add_string(buffer, "default", "*");
        }
        buffer_json_object_close(buffer); // metrics
    }

    // Add full-text search if supported
    if (config->params.has_q) {
        buffer_json_member_add_object(buffer, "q");
        {
            buffer_json_member_add_string(buffer, "type", "string");
            buffer_json_member_add_string(buffer, "title", "Full-text search");
            buffer_json_member_add_string(buffer, "description",
                "Filter metrics by searching across all their metadata (names, titles, instances, dimensions, labels). "
                "Use pipe (|) to separate multiple search terms. Examples: 'memory|pressure', 'cpu|load|system'");
            buffer_json_member_add_string(buffer, "default", NULL);
        }
        buffer_json_object_close(buffer); // q
    }

    // Add nodes pattern if supported
    if (config->params.has_nodes) {
        if (config->params.nodes_as_array) {
            // Use array for nodes
            mcp_schema_add_array_param(buffer, "nodes",
                config->params.nodes_required ? "Specify the nodes" : "Filter nodes",
                config->params.nodes_required ?
                    "Array of specific node names to query. This parameter is required because this tool produces detailed output. "
                    "Each node must be an exact match - no wildcards or patterns allowed. "
                    "Use '" MCP_TOOL_LIST_NODES "' to discover available nodes. "
                    "Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]" :
                    "Array of specific node names to filter by. "
                    "Each node must be an exact match - no wildcards or patterns allowed. "
                    "Use '" MCP_TOOL_LIST_NODES "' to discover available nodes. "
                    "If not specified, all nodes are included. "
                    "Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]",
                config->params.nodes_required);
        } else {
            // Use string pattern for nodes
            buffer_json_member_add_object(buffer, "nodes");
            buffer_json_member_add_string(buffer, "type", "string");
            
            // Compose title and description based on context
            snprintfz(title, sizeof(title), "%s nodes", 
                      config->params.nodes_required ? "Specify the" : "Filter");
            
            if (config->params.nodes_required) {
                snprintfz(description, sizeof(description),
                          "Specify which nodes to query. This parameter is required because this tool produces detailed output. "
                          "Use pipe (|) to separate multiple patterns. Examples: 'node1|node2', '*web*|*db*', 'prod-*'");
            } else if (config->output_type == MCP_LIST_OUTPUT_NODES || config->output_type == MCP_LIST_OUTPUT_FUNCTIONS) {
                // This is a node/function query - direct filtering
                snprintfz(description, sizeof(description),
                          "Search for nodes by hostname patterns. This is the primary way to find specific nodes without retrieving the full list. "
                          "Use pipe (|) to separate multiple patterns. Wildcards (*) are supported for flexible matching. "
                          "Examples: 'node1|node2' (exact names), '*web*' (contains 'web'), 'prod-*' (starts with 'prod-'), '*db*|*cache*' (contains 'db' OR 'cache')");
            } else {
                // This is a metrics query - nodes acts as a filter
                snprintfz(description, sizeof(description),
                          "Filter %s to only those collected by these nodes. "
                          "Use pipe (|) to separate multiple patterns. "
                          "Examples: 'node1|node2', '*web*|*db*', 'prod-*|staging-*'", 
                          output_name);
            }
            
            buffer_json_member_add_string(buffer, "title", title);
            buffer_json_member_add_string(buffer, "description", description);
            if (!config->params.nodes_required) {
                buffer_json_member_add_string(buffer, "default", "*");
            }
            buffer_json_object_close(buffer); // nodes
        }
    }

    // Add time range parameters if supported
    if (config->params.has_time_range) {
        mcp_schema_params_add_time_window(buffer, output_name, false);
    }

    // Add cardinality limit if supported
    if (config->params.has_cardinality_limit) {
        mcp_schema_params_add_cardinality_limit(buffer, output_name, false);
    }

    buffer_json_object_close(buffer); // properties

    // Required fields
    if (config->params.metrics_required || config->params.nodes_required) {
        buffer_json_member_add_array(buffer, "required");
        if (config->params.metrics_required) {
            buffer_json_add_array_item_string(buffer, "metrics");
        }
        if (config->params.nodes_required) {
            buffer_json_add_array_item_string(buffer, "nodes");
        }
        buffer_json_array_close(buffer);
    }
    
    buffer_json_object_close(buffer); // inputSchema
}

// Removed extract_string_param and extract_size_param - now using mcp-params functions

// Unified execution
MCP_RETURN_CODE mcp_unified_list_tool_execute(MCP_CLIENT *mcpc, const MCP_LIST_TOOL_CONFIG *config,
                                               struct json_object *params, MCP_REQUEST_ID id)
{
    if (!mcpc || !config || id == 0)
        return MCP_RC_ERROR;

    // Extract parameters based on configuration
    const char *q = config->params.has_q ? mcp_params_extract_string(params, "q", NULL) : NULL;
    const char *metrics_pattern = config->params.has_metrics ? mcp_params_extract_string(params, "metrics", NULL) : NULL;
    
    // Handle nodes - either as array or string pattern
    const char *nodes_pattern = NULL;
    CLEAN_BUFFER *nodes_buffer = NULL;
    
    if (config->params.has_nodes) {
        if (config->params.nodes_as_array) {
            // Parse nodes as array
            const char *error_message = NULL;
            nodes_buffer = mcp_params_parse_array_to_pattern(params, "nodes", false, MCP_TOOL_LIST_NODES, &error_message);
            if (error_message) {
                buffer_sprintf(mcpc->error, "%s", error_message);
                return MCP_RC_BAD_REQUEST;
            }
            nodes_pattern = buffer_tostring(nodes_buffer);
        } else {
            // Parse nodes as string pattern
            nodes_pattern = mcp_params_extract_string(params, "nodes", NULL);
        }
    }
    
    // Check required parameters
    if (config->params.metrics_required && !metrics_pattern) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'metrics'");
        return MCP_RC_ERROR;
    }
    
    if (config->params.nodes_required && !nodes_pattern) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'nodes'. This tool produces detailed output - please specify which nodes to query.");
        return MCP_RC_ERROR;
    }

    // Extract time parameters (only if tool supports them)
    time_t after = 0;
    time_t before = 0;
    if (config->params.has_time_range) {
        after = mcp_extract_time_param(params, "after", MCP_DEFAULT_AFTER_TIME);
        before = mcp_extract_time_param(params, "before", MCP_DEFAULT_BEFORE_TIME);
    }
    
    // Extract cardinality limit if supported
    size_t cardinality_limit = 0;
    if (config->params.has_cardinality_limit) {
        size_t default_cardinality = config->defaults.cardinality_limit ?: MCP_METADATA_CARDINALITY_LIMIT;
        const char *size_error = NULL;
        cardinality_limit = mcp_params_extract_size(params, "cardinality_limit", default_cardinality, 1, 500, &size_error);
        if (size_error) {
            buffer_sprintf(mcpc->error, "%s", size_error);
            return MCP_RC_BAD_REQUEST;
        }
    }

    CLEAN_BUFFER *t = buffer_create(0, NULL);

    struct api_v2_contexts_request req = {
        .scope_contexts = metrics_pattern,
        .scope_nodes = nodes_pattern,
        .contexts = metrics_pattern,
        .nodes = nodes_pattern,
        .q = q,
        .after = after,
        .before = before,
        .cardinality_limit = cardinality_limit,
        .options = config->options | CONTEXTS_OPTION_MCP | CONTEXTS_OPTION_RFC3339,
    };

    // Determine mode - add SEARCH if q is provided
    CONTEXTS_V2_MODE mode = config->mode;
    if (config->params.has_q && q) {
        mode = CONTEXTS_V2_SEARCH;
        req.options |= CONTEXTS_OPTION_FAMILY | CONTEXTS_OPTION_UNITS | CONTEXTS_OPTION_TITLES |
            CONTEXTS_OPTION_LABELS | CONTEXTS_OPTION_INSTANCES | CONTEXTS_OPTION_DIMENSIONS;
    }

    int code = rrdcontext_to_json_v2(t, &req, mode);
    if (code != HTTP_RESP_OK) {
        buffer_sprintf(mcpc->error, "Failed to fetch %s, query returned http error code %d", config->name, code);
        return MCP_RC_ERROR;
    }

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    {
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
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON

    return MCP_RC_OK;
}