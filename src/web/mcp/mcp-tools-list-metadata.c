// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-list-metadata.h"
#include "mcp-tools.h"
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
            .nodes_as_array = true,    // get_metrics_details uses array for nodes
            .metrics_as_array = true,  // get_metrics_details uses array for metrics
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
            .metrics_as_array = true,  // list_nodes uses an array for metrics filter
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
            .nodes_as_array = true,   // list_functions uses array for nodes
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
            .metrics_as_array = true, // get_nodes_details uses array for metrics
        },
    },
    {
        .name = MCP_TOOL_LIST_RAISED_ALERTS,
        .title = "List raised alerts",
        .description = "List currently active alerts (WARNING and CRITICAL status)",
        .output_type = MCP_LIST_OUTPUT_ALERTS,
        .mode = CONTEXTS_V2_ALERTS,
        .options = CONTEXTS_OPTION_INSTANCES | CONTEXTS_OPTION_VALUES,
        .params = {
            .has_nodes = true,
            .has_metrics = true,  // Filter by context pattern
            .has_alert_pattern = true,
            .has_time_range = false,  // Raised alerts are current, not historical
            .has_cardinality_limit = true,
            .nodes_as_array = true,
            .metrics_as_array = true,  // Metrics should be an array for alerts
        },
        .defaults = {
            .alert_status = CONTEXT_ALERT_RAISED,  // Only raised alerts
            .cardinality_limit = 200,
        },
    },
    {
        .name = MCP_TOOL_LIST_ALL_ALERTS,
        .title = "List all alerts",
        .description = "List all currently running alerts",
        .output_type = MCP_LIST_OUTPUT_ALERTS,
        .mode = CONTEXTS_V2_ALERTS,
        .options = CONTEXTS_OPTION_SUMMARY,
        .params = {
            .has_nodes = true,
            .has_metrics = true,  // Filter by context pattern
            .has_alert_pattern = true,
            .has_time_range = true,
            .has_cardinality_limit = true,
            .nodes_as_array = true,
            .metrics_as_array = true,  // Metrics should be an array for alerts
        },
        .defaults = {
            .alert_status = CONTEXTS_ALERT_STATUSES,  // All statuses
            .cardinality_limit = 200,
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
        case MCP_LIST_OUTPUT_ALERTS:
            output_name = "alerts";
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

    // Add a metrics pattern if supported
    if (config->params.has_metrics) {
        if (config->params.metrics_as_array) {
            // Use an array for metrics
            if (config->output_type != MCP_LIST_OUTPUT_METRICS) {
                // This is a node/function/alert query - metrics acts as a filter
                if (config->output_type == MCP_LIST_OUTPUT_ALERTS) {
                    // For alerts, metrics refers to contexts
                    mcp_schema_add_array_param(buffer, "metrics",
                        config->params.metrics_required ? "Specify the contexts to filter by" : "Filter by contexts",
                        config->params.metrics_required ?
                            "Array of specific context names to filter alerts by. This parameter is required. "
                            "Each context must be an exact match - no wildcards or patterns allowed. "
                            "Use '" MCP_TOOL_LIST_METRICS "' to discover available contexts. "
                            "Examples: [\"system.cpu\", \"disk.space\"], [\"mysql.queries\", \"redis.memory\"]" :
                            "Array of specific context names to filter alerts by. "
                            "Each context must be an exact match - no wildcards or patterns allowed. "
                            "Use '" MCP_TOOL_LIST_METRICS "' to discover available contexts. "
                            "If not specified, alerts from all contexts are included. "
                            "Examples: [\"system.cpu\", \"disk.space\"], [\"mysql.queries\", \"redis.memory\"]");
                } else {
                    // For nodes/functions, metrics is a filter
                    mcp_schema_add_array_param(buffer, "metrics",
                        config->params.metrics_required ? "Specify the metrics to filter by" : "Filter by metrics",
                        config->params.metrics_required ?
                            "Array of specific metric names to filter by. This parameter is required. "
                            "Each metric must be an exact match - no wildcards or patterns allowed. "
                            "Use '" MCP_TOOL_LIST_METRICS "' to discover available metrics. "
                            "Examples: [\"system.cpu\", \"system.load\"], [\"disk.io\", \"disk.space\"]" :
                            "Array of specific metric names to filter by. "
                            "Each metric must be an exact match - no wildcards or patterns allowed. "
                            "Use '" MCP_TOOL_LIST_METRICS "' to discover available metrics. "
                            "If not specified, all metrics are included. "
                            "Examples: [\"system.cpu\", \"system.load\"], [\"disk.io\", \"disk.space\"]");
                }
            } else {
                // This is a metrics query
                mcp_schema_add_array_param(buffer, "metrics",
                    config->params.metrics_required ? "Specify the metrics" : "Filter metrics",
                    config->params.metrics_required ?
                        "Array of specific metric names to retrieve details for. This parameter is required. "
                        "Each metric must be an exact match - no wildcards or patterns allowed. "
                        "Examples: [\"system.cpu\", \"system.load\", \"system.ram\"]" :
                        "Array of specific metric names to filter. "
                        "Each metric must be an exact match - no wildcards or patterns allowed. "
                        "If not specified, all metrics are included. "
                        "Examples: [\"system.cpu\", \"system.load\", \"system.ram\"]");
            }
        } else {
            // Use string pattern for metrics
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
    }

    // Add full-text search if supported
    if (config->params.has_q) {
        buffer_json_member_add_object(buffer, "q");
        {
            buffer_json_member_add_string(buffer, "type", "string");
            buffer_json_member_add_string(buffer, "title", "Full-text search on metrics metadata");
            buffer_json_member_add_string(buffer, "description",
                "Filter metrics by searching across all their metadata (names, titles, instances, dimensions, labels). "
                "Use pipe (|) to separate multiple search terms. Examples: 'memory|pressure', 'cpu|load|system'");
        }
        buffer_json_object_close(buffer); // q
    }

    // Add a nodes' pattern if supported
    if (config->params.has_nodes) {
        if (config->params.nodes_as_array) {
            // Use an array for nodes
            mcp_schema_add_array_param(buffer, "nodes",
                config->params.nodes_required ? "Specify the nodes" : "Filter by nodes",
                config->params.nodes_required ?
                    "Array of specific node names to query. This parameter is required because this tool produces detailed output. "
                    "Each node must be an exact match - no wildcards or patterns allowed. "
                    "Use '" MCP_TOOL_LIST_NODES "' to discover available nodes. "
                    "Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]" :
                    "Array of specific node names to filter by. "
                    "Each node must be an exact match - no wildcards or patterns allowed. "
                    "Use '" MCP_TOOL_LIST_NODES "' to discover available nodes. "
                    "If not specified, all nodes are included. "
                    "Examples: [\"node1\", \"node2\"], [\"web-server-01\", \"db-server-01\"]");
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
                          "Examples: 'node1|node2' (exact names), '*web*' (contains 'web'), 'prod-*' (starts with 'prod-'), '*db*|*cache*' (contains 'db' or 'cache')");
            } else {
                // This is a metrics query - 'nodes' acts as a filter
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
        // Build description prefix for metadata queries
        char description_prefix[256];
        snprintfz(description_prefix, sizeof(description_prefix),
                  "%s with data collected", output_name ? output_name : "results");
        mcp_schema_add_time_params(buffer, description_prefix, false);
    }

    // Add cardinality limit if supported
    if (config->params.has_cardinality_limit) {
        size_t default_cardinality = config->defaults.cardinality_limit ?: MCP_METADATA_CARDINALITY_LIMIT;
        mcp_schema_add_cardinality_limit(buffer,
            "Maximum number of items to return per category (dimensions, instances, labels, etc.). "
            "Prevents response explosion. When exceeded, the response will indicate how many items were omitted.",
            default_cardinality,
            1,  // minimum
            MAX(default_cardinality, MCP_METADATA_CARDINALITY_LIMIT_MAX));
    }
    
    // Add alert-specific parameters
    if (config->params.has_alert_pattern) {
        // Use string pattern for alerts
        buffer_json_member_add_object(buffer, "alerts");
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Filter alerts");
        buffer_json_member_add_string(buffer, "description",
            "Pattern matching on alert names. Use pipe (|) to separate multiple patterns. "
            "Supports wildcards. Examples: 'disk_*', '*cpu*|*memory*', 'health.*'");
        buffer_json_member_add_string(buffer, "default", "*");
        buffer_json_object_close(buffer); // alerts
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
    
    // Handle metrics - either as array or string pattern
    const char *metrics_pattern = NULL;
    CLEAN_BUFFER *metrics_buffer = NULL;
    
    if (config->params.has_metrics) {
        if (config->params.metrics_as_array) {
            // Parse metrics as an array
            metrics_buffer = mcp_params_parse_array_to_pattern(params, "metrics", false, false, MCP_TOOL_LIST_METRICS, mcpc->error);
            if (buffer_strlen(mcpc->error) > 0) {
                return MCP_RC_BAD_REQUEST;
            }
            metrics_pattern = buffer_tostring(metrics_buffer);
        } else {
            // Parse metrics as a string pattern
            metrics_pattern = mcp_params_extract_string(params, "metrics", NULL);
        }
        if(metrics_pattern && !*metrics_pattern) {
            metrics_pattern = NULL; // Treat empty string as no metrics specified
        }
    }
    
    // Handle nodes - either as array or string pattern
    const char *nodes_pattern = NULL;
    CLEAN_BUFFER *nodes_buffer = NULL;
    
    if (config->params.has_nodes) {
        if (config->params.nodes_as_array) {
            // Parse nodes as array
            nodes_buffer = mcp_params_parse_array_to_pattern(params, "nodes", false, false, MCP_TOOL_LIST_NODES, mcpc->error);
            if (buffer_strlen(mcpc->error) > 0) {
                return MCP_RC_BAD_REQUEST;
            }
            nodes_pattern = buffer_tostring(nodes_buffer);
        } else {
            // Parse nodes as a string pattern
            nodes_pattern = mcp_params_extract_string(params, "nodes", NULL);
        }
        if(nodes_pattern && !*nodes_pattern) {
            nodes_pattern = NULL; // Treat empty string as no nodes specified
        }
    }
    
    // Check required parameters
    if (config->params.metrics_required && !metrics_pattern) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'metrics'. Use '" MCP_TOOL_LIST_METRICS "' to discover available metrics.");
        return MCP_RC_ERROR;
    }
    
    if (config->params.nodes_required && !nodes_pattern) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'nodes'. Use '" MCP_TOOL_LIST_NODES "' to discover available nodes.");
        return MCP_RC_ERROR;
    }

    // Extract time parameters (only if the tool supports them)
    time_t after = 0;
    time_t before = 0;
    if (config->params.has_time_range) {
        if (!mcp_params_parse_time_window(params, &after, &before, 
                                          MCP_DEFAULT_AFTER_TIME, MCP_DEFAULT_BEFORE_TIME, 
                                          false, mcpc->error)) {
            return MCP_RC_BAD_REQUEST;
        }
    }
    
    // Extract cardinality limit if supported
    size_t cardinality_limit = 0;
    if (config->params.has_cardinality_limit) {
        size_t default_cardinality = config->defaults.cardinality_limit ?: MCP_METADATA_CARDINALITY_LIMIT;
        cardinality_limit = mcp_params_extract_size(params, "cardinality_limit", default_cardinality, 1, 500, mcpc->error);
        if (buffer_strlen(mcpc->error) > 0) {
            return MCP_RC_BAD_REQUEST;
        }
    }
    
    // Extract alert-specific parameters
    const char *alert_pattern = NULL;
    
    if (config->params.has_alert_pattern) {
        // Parse alerts as a string pattern
        alert_pattern = mcp_params_extract_string(params, "alerts", NULL);
        if(alert_pattern && !*alert_pattern) {
            alert_pattern = NULL; // Treat empty string as no alerts specified
        }
    }
    
    CLEAN_BUFFER *t = buffer_create(0, NULL);

    struct api_v2_contexts_request req = {
        .scope_contexts = metrics_pattern,
        .scope_nodes = nodes_pattern,
        .contexts = NULL,
        .nodes = NULL,
        .q = q,
        .after = after,
        .before = before,
        .cardinality_limit = cardinality_limit,
        .options = config->options | CONTEXTS_OPTION_MCP | CONTEXTS_OPTION_RFC3339 | CONTEXTS_OPTION_JSON_LONG_KEYS | CONTEXTS_OPTION_MINIFY,
        .alerts = {
            .alert = alert_pattern,
            .status = config->defaults.alert_status,
        },
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
        // Start building a content array for the result
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