// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools.h"
#include "database/contexts/rrdcontext.h"

// Define schema for the list_netdata_metrics tool
void mcp_tool_metric_contexts_schema(BUFFER *buffer) {
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

// Implementation of the list_netdata_metrics tool
MCP_RETURN_CODE mcp_tool_metric_contexts_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id) {
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
