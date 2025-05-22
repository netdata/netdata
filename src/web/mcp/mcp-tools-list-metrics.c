// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-list-metrics.h"
#include "database/contexts/rrdcontext.h"

void mcp_tool_list_metrics_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "List available metrics");

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    buffer_json_member_add_object(buffer, "q");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Full-text search");
        buffer_json_member_add_string(buffer, "description", "Search across all metadata (names, titles, instances, dimensions, labels). Example: 'memory pressure'");
        buffer_json_member_add_string(buffer, "default", "");
    }
    buffer_json_object_close(buffer); // q

    buffer_json_member_add_object(buffer, "metrics");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Metric name pattern");
        buffer_json_member_add_string(buffer, "description", "Pattern matching on metric names only. Supports wildcards like 'system.*' or '*cpu*|*memory*'");
        buffer_json_member_add_string(buffer, "default", "");
    }
    buffer_json_object_close(buffer); // metrics

    buffer_json_member_add_object(buffer, "nodes");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Node filter");
        buffer_json_member_add_string(buffer, "description", "Filter by specific nodes. Supports patterns like 'node1|node2' or '*web*|*db*' on their hostnames");
        buffer_json_member_add_string(buffer, "default", "");
    }
    buffer_json_object_close(buffer); // nodes

    buffer_json_member_add_object(buffer, "after");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Start time");
        buffer_json_member_add_string(buffer, "description", "Limit to metrics collected after this time. Unix timestamp or negative seconds relative to before");
        buffer_json_member_add_int64(buffer, "default", -3600);
    }
    buffer_json_object_close(buffer); // after

    buffer_json_member_add_object(buffer, "before");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "End time");
        buffer_json_member_add_string(buffer, "description", "Limit to metrics collected before this time. Unix timestamp or negative seconds relative to now");
        buffer_json_member_add_int64(buffer, "default", 0);
    }
    buffer_json_object_close(buffer); // before

    buffer_json_member_add_object(buffer, "limit");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Maximum results");
        buffer_json_member_add_string(buffer, "description", "Maximum number of metrics to return");
        buffer_json_member_add_int64(buffer, "default", 100);
        buffer_json_member_add_int64(buffer, "minimum", 1);
        buffer_json_member_add_int64(buffer, "maximum", 500);
    }
    buffer_json_object_close(buffer); // limit

    buffer_json_object_close(buffer); // properties

    // No required fields
    buffer_json_object_close(buffer); // inputSchema
}

MCP_RETURN_CODE mcp_tool_list_metrics_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id)
{
    if (!mcpc || id == 0)
        return MCP_RC_ERROR;

    const char *q = NULL;
    if (params && json_object_object_get_ex(params, "q", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "q", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            q = json_object_get_string(obj);
            if (q && strlen(q) == 0) q = NULL;
        }
    }

    const char *metrics_pattern = NULL;
    if (params && json_object_object_get_ex(params, "metrics", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "metrics", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            metrics_pattern = json_object_get_string(obj);
            if (metrics_pattern && strlen(metrics_pattern) == 0) metrics_pattern = NULL;
        }
    }

    const char *nodes_pattern = NULL;
    if (params && json_object_object_get_ex(params, "nodes", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "nodes", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            nodes_pattern = json_object_get_string(obj);
            if (nodes_pattern && strlen(nodes_pattern) == 0) nodes_pattern = NULL;
        }
    }

    time_t after = -3600;
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

    size_t limit = 100;
    if (params && json_object_object_get_ex(params, "limit", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "limit", &obj);
        if (obj && json_object_is_type(obj, json_type_int)) {
            limit = json_object_get_int64(obj);
            if (limit > 500) limit = 500;
            if (limit < 1) limit = 1;
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
        .cardinality_limit = limit,
        .options = CONTEXTS_OPTION_MCP,
    };

    int code = rrdcontext_to_json_v2(t, &req, q ? CONTEXTS_V2_CONTEXTS | CONTEXTS_V2_SEARCH : CONTEXTS_V2_CONTEXTS);
    if (code != HTTP_RESP_OK) {
        buffer_sprintf(mcpc->error, "Failed to fetch metrics, query returned http error code %d", code);
        return MCP_RC_ERROR;
    }

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
