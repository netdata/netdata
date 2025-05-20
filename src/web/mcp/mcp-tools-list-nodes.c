// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-list-nodes.h"
#include "database/contexts/rrdcontext.h"

void mcp_tool_list_nodes_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Filter monitored nodes");

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    buffer_json_member_add_object(buffer, "nodes");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Pipe separated list of nodes or node patterns to be returned");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on nodes for slicing the metadata database of Netdata. Examples: node1|node2, or even *db*|*dns*, to match against hostnames");
        buffer_json_member_add_string(buffer, "default", "*");
    }
    buffer_json_object_close(buffer); // nodes

    buffer_json_member_add_object(buffer, "contexts");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Pipe separated list of contexts to select only the node that collect these contexts.");
        buffer_json_member_add_string(buffer, "description", "Glob-like pattern matching on context names. Examples: context1|context2, or even *word1*|*word2*, to match against contexts identifiers.");
        buffer_json_member_add_string(buffer, "default", "*");
    }
    buffer_json_object_close(buffer); // contexts

    buffer_json_member_add_object(buffer, "after");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Unix Epoch Timestamp, or negative number of seconds relative to parameter before");
        buffer_json_member_add_string(buffer, "description", "Limit the results to nodes that were connected after this timestamp. If negative, it will be interpreted as a number of seconds relative to the before parameter");
        buffer_json_member_add_int64(buffer, "default", 0);
    }
    buffer_json_object_close(buffer); // after

    buffer_json_member_add_object(buffer, "before");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Unix Epoch Timestamp, or negative number of seconds relative to now");
        buffer_json_member_add_string(buffer, "description", "Limit the results to nodes that were connected before this timestamp. If negative, it will be interpreted as a number of seconds relative now");
        buffer_json_member_add_int64(buffer, "default", 0);
    }
    buffer_json_object_close(buffer); // before

    buffer_json_object_close(buffer); // properties

    // No required fields
    buffer_json_object_close(buffer); // inputSchema
}

MCP_RETURN_CODE mcp_tool_list_nodes_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id)
{
    if (!mcpc || id == 0)
        return MCP_RC_ERROR;

    const char *nodes_pattern = NULL;
    if (params && json_object_object_get_ex(params, "nodes", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "nodes", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            nodes_pattern = json_object_get_string(obj);
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

    int code = rrdcontext_to_json_v2(t, &req, CONTEXTS_V2_NODES | CONTEXTS_V2_MCP);
    if (code != HTTP_RESP_OK) {
        buffer_sprintf(mcpc->error, "Failed to fetch nodes, query returned http error code %d", code);
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