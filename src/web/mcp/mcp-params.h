// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_PARAMS_H
#define NETDATA_MCP_PARAMS_H

#include "mcp.h"
#include <json-c/json.h>

// Common parameter parsing functions

// Parse array parameters (nodes, instances, dimensions) and convert to pipe-separated string
// Returns newly allocated BUFFER on success, or NULL if not provided/on error
// Writes error message to error buffer on failure
// required: if true, validates parameter exists, is array type, and is non-empty
// list_tool: tool name to recommend for discovering exact values (e.g., MCP_TOOL_LIST_NODES)
BUFFER *mcp_params_parse_array_to_pattern(
    struct json_object *params,
    const char *param_name,
    bool required,
    bool allow_wildcards,
    const char *list_tool,
    BUFFER *error
);

// Parse labels object parameter and convert to query string format
// Returns newly allocated BUFFER on success, or NULL if not provided/on error
// Writes error message to error buffer on failure
// list_tool: tool name to recommend for discovering exact values (e.g., MCP_TOOL_GET_METRICS_DETAILS)
BUFFER *mcp_params_parse_labels_object(
    struct json_object *params,
    const char *list_tool,
    BUFFER *error
);

// Add array parameter schema (for nodes, instances, dimensions)
void mcp_schema_add_array_param(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description
);

// Add labels object parameter schema
void mcp_schema_add_labels_object(
    BUFFER *buffer,
    const char *title,
    const char *description
);

// Add time window parameters (after, before) to schema
void mcp_schema_add_time_params(
    BUFFER *buffer,
    const char *time_description_prefix,
    bool required
);

// Add cardinality limit parameter to schema
void mcp_schema_add_cardinality_limit(
    BUFFER *buffer,
    const char *description,
    size_t default_value,
    size_t min_value,
    size_t max_value
);

// Extract string parameter with optional default
// Returns the string value or default_value if not found
const char *mcp_params_extract_string(
    struct json_object *params,
    const char *param_name,
    const char *default_value
);

// Extract numeric size parameter with bounds checking
// Returns the size value or default_value if not found
// Writes error message to error buffer if value is out of bounds
size_t mcp_params_extract_size(
    struct json_object *params,
    const char *param_name,
    size_t default_value,
    size_t min_value,
    size_t max_value,
    BUFFER *error
);

// Extract timeout parameter (in seconds)
// Returns the timeout value or default_value if not found
// Writes error message to error buffer if value is out of bounds
int mcp_params_extract_timeout(
    struct json_object *params,
    const char *param_name,
    int default_seconds,
    int min_seconds,
    int max_seconds,
    BUFFER *error
);

// Schema generation for timeout parameter
void mcp_schema_add_timeout(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description,
    int default_seconds,
    int min_seconds,
    int max_seconds,
    bool required
);

// Schema generation for generic string parameter
void mcp_schema_add_string_param(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description,
    const char *default_value,
    bool required
);

// Schema generation for size parameter
void mcp_schema_add_size_param(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description,
    size_t default_value,
    size_t min_value,
    size_t max_value,
    bool required
);

time_t mcp_params_parse_time(
    struct json_object *params,
    const char *name,
    time_t default_value);

// Validate and auto-correct time window parameters
// Contract: 'after' is relative to 'before', 'before' is relative to 'now'
// Handles common AI assistant mistakes:
// - Both positive relative times → make both negative
// - Positive after + negative before that results in future time → make after negative
void mcp_params_validate_time_window(time_t *after, time_t *before, time_t now);

// Parse and validate time window parameters (after and before) together
// This ensures consistent parsing and validation across all MCP tools
// Returns true on success, false on error (with error message in error buffer)
// allow_both_zero: set to true for baseline times that use 0,0 for auto-calculation
bool mcp_params_parse_time_window(
    struct json_object *params,
    time_t *after,
    time_t *before,
    time_t default_after,
    time_t default_before,
    bool allow_both_zero,
    BUFFER *error
);

// Schema generation for individual time parameter
void mcp_schema_add_time_param(
    BUFFER *buffer,
    const char *param_name,
    const char *title,
    const char *description,
    const char *relative_to,
    time_t default_value,
    bool required
);

#endif // NETDATA_MCP_PARAMS_H