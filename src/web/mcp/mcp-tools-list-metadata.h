// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_TOOLS_LIST_METADATA_H
#define NETDATA_MCP_TOOLS_LIST_METADATA_H

#include "mcp.h"
#include "web/api/maps/contexts_options.h"
#include "web/api/maps/contexts_alert_statuses.h"
#include "database/contexts/rrdcontext.h"

// Tool output types - what kind of data the tool returns
typedef enum {
    MCP_LIST_OUTPUT_NODES,      // Tool returns nodes
    MCP_LIST_OUTPUT_METRICS,    // Tool returns metrics (contexts)
    MCP_LIST_OUTPUT_FUNCTIONS,  // Tool returns functions
    MCP_LIST_OUTPUT_ALERTS,     // Tool returns alerts
} MCP_LIST_OUTPUT_TYPE;

// Configuration structure for unified list tools
typedef struct mcp_list_tool_config {
    const char *name;
    const char *title;
    const char *description;
    
    // What type of output this tool returns
    MCP_LIST_OUTPUT_TYPE output_type;
    
    // Mode for rrdcontext_to_json_v2
    CONTEXTS_V2_MODE mode;
    
    // Additional options (MCP is always added)
    CONTEXTS_OPTIONS options;
    
    // Parameters configuration
    struct {
        bool has_q;           // Full-text search
        bool has_metrics;     // Metrics pattern
        bool has_nodes;       // Nodes pattern
        bool has_instances;   // Instances pattern
        bool has_dimensions;  // Dimensions pattern
        bool has_time_range;  // Has after/before time parameters
        bool has_cardinality_limit; // Has cardinality limit parameter
        bool has_alert_status; // Has alert status filter parameter
        bool has_alert_pattern; // Has alert name pattern parameter
        bool has_last_transitions; // Has last N transitions parameter
        
        bool metrics_required; // Is metrics parameter required?
        bool nodes_required;   // Is nodes parameter required?
        bool alerts_required;  // Is alerts parameter required?
        bool nodes_as_array;   // Should nodes be an array instead of pattern?
        bool metrics_as_array; // Should metrics be an array instead of pattern?
        bool alerts_as_array;  // Should alerts be an array instead of pattern?
    } params;
    
    // Tool-specific defaults (0 means use global default)
    struct {
        size_t cardinality_limit;
        uint32_t alert_status;  // Default alert status filter (CONTEXTS_ALERT_STATUS)
    } defaults;
    
} MCP_LIST_TOOL_CONFIG;

// Get tool configuration by name
const MCP_LIST_TOOL_CONFIG *mcp_get_list_tool_config(const char *name);

// Iterate through all list tools
const MCP_LIST_TOOL_CONFIG *mcp_get_list_tool_by_index(size_t index);
size_t mcp_get_list_tools_count(void);

// Unified functions
void mcp_unified_list_tool_schema(BUFFER *buffer, const MCP_LIST_TOOL_CONFIG *config);
MCP_RETURN_CODE mcp_unified_list_tool_execute(MCP_CLIENT *mcpc, const MCP_LIST_TOOL_CONFIG *config, 
                                               struct json_object *params, MCP_REQUEST_ID id);

#endif //NETDATA_MCP_TOOLS_LIST_METADATA_H