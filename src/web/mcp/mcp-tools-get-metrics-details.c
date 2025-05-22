// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-get-metrics-details.h"
#include "database/contexts/rrdcontext.h"

void mcp_tool_get_metrics_details_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Get metrics details");

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    buffer_json_member_add_object(buffer, "metrics");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Metrics to get details for");
        buffer_json_member_add_string(buffer, "description", "Pipe-separated list of metric names. Maximum 20 metrics per request. Example: 'system.cpu|system.load|system.ram'");
    }
    buffer_json_object_close(buffer); // metrics

    buffer_json_member_add_object(buffer, "nodes");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "Node filter");
        buffer_json_member_add_string(buffer, "description", "Filter details by specific nodes. Leave empty for all nodes");
        buffer_json_member_add_string(buffer, "default", "");
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

    buffer_json_member_add_object(buffer, "cardinality_limit");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Maximum number of dimensions, instances, and label values to return per context");
        buffer_json_member_add_string(buffer, "description", "Limits the number of dimensions, instances, and label values returned.");
        buffer_json_member_add_int64(buffer, "default", 50);
    }
    buffer_json_object_close(buffer); // cardinality_limit

    buffer_json_object_close(buffer); // properties

    // Required fields
    buffer_json_member_add_array(buffer, "required");
    buffer_json_add_array_item_string(buffer, "metrics");
    buffer_json_array_close(buffer);
    
    buffer_json_object_close(buffer); // inputSchema
}

MCP_RETURN_CODE mcp_tool_get_metrics_details_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id)
{
    if (!mcpc || id == 0)
        return MCP_RC_ERROR;

    // Extract the 'metrics' parameter (required)
    const char *metrics_pattern = NULL;
    if (params && json_object_object_get_ex(params, "metrics", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "metrics", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            metrics_pattern = json_object_get_string(obj);
        }
    }
    
    if (!metrics_pattern || strlen(metrics_pattern) == 0) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'metrics'");
        return MCP_RC_ERROR;
    }
    
    // Count the number of metrics requested (pipe-separated)
    size_t metric_count = 1;
    const char *p = metrics_pattern;
    while (*p) {
        if (*p == '|') metric_count++;
        p++;
    }
    
    if (metric_count > 20) {
        buffer_sprintf(mcpc->error, "Too many metrics requested. Maximum 20 metrics per request (got %zu)", metric_count);
        return MCP_RC_ERROR;
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

    size_t cardinality_limit = 50;
    if (params && json_object_object_get_ex(params, "cardinality_limit", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "cardinality_limit", &obj);
        if (obj && json_object_is_type(obj, json_type_int)) {
            cardinality_limit = json_object_get_int64(obj);
        }
    }

    CLEAN_BUFFER *t = buffer_create(0, NULL);

    struct api_v2_contexts_request req = {
        .scope_nodes = nodes_pattern,
        .scope_contexts = metrics_pattern,
        .after = after,
        .before = before,
        .cardinality_limit = cardinality_limit,
        .options = CONTEXTS_OPTION_TITLES | CONTEXTS_OPTION_INSTANCES | CONTEXTS_OPTION_DIMENSIONS | 
                   CONTEXTS_OPTION_LABELS | CONTEXTS_OPTION_MCP | CONTEXTS_OPTION_RETENTION | 
                   CONTEXTS_OPTION_LIVENESS | CONTEXTS_OPTION_FAMILY | CONTEXTS_OPTION_UNITS,
    };

    int code = rrdcontext_to_json_v2(t, &req, CONTEXTS_V2_CONTEXTS);
    if (code != HTTP_RESP_OK) {
        buffer_sprintf(mcpc->error, "Failed to fetch metrics details, query returned http error code %d", code);
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
