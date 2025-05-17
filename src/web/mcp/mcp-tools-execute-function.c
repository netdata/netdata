// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools.h"
#include "database/contexts/rrdcontext.h"
#include "database/rrdfunctions.h"


void mcp_tool_execute_function_schema(BUFFER *buffer) {
    // Tool input schema
    buffer_json_member_add_object(buffer, "inputSchema");
    buffer_json_member_add_string(buffer, "type", "object");
    buffer_json_member_add_string(buffer, "title", "Execute a function on a specific node");

    // Properties
    buffer_json_member_add_object(buffer, "properties");

    buffer_json_member_add_object(buffer, "node");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "The node on which to execute the function");
        buffer_json_member_add_string(buffer, "description", "The hostname or node ID where the function should be executed");
    }
    buffer_json_object_close(buffer); // node

    buffer_json_member_add_object(buffer, "function");
    {
        buffer_json_member_add_string(buffer, "type", "string");
        buffer_json_member_add_string(buffer, "title", "The name of the function to execute followed by a space and its parameters");
        buffer_json_member_add_string(buffer, "description", "The function name, as available in the node_details tool output");
    }
    buffer_json_object_close(buffer); // function
    
    buffer_json_member_add_object(buffer, "timeout");
    {
        buffer_json_member_add_string(buffer, "type", "number");
        buffer_json_member_add_string(buffer, "title", "Execution timeout in seconds");
        buffer_json_member_add_string(buffer, "description", "Maximum time to wait for function execution (default: 60)");
        buffer_json_member_add_int64(buffer, "default", 60);
    }
    buffer_json_object_close(buffer); // timeout

    buffer_json_object_close(buffer); // properties

    // Required fields
    buffer_json_member_add_array(buffer, "required");
    buffer_json_add_array_item_string(buffer, "node");
    buffer_json_add_array_item_string(buffer, "function");
    buffer_json_array_close(buffer); // required

    buffer_json_object_close(buffer); // inputSchema
}

MCP_RETURN_CODE mcp_tool_execute_function_execute(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id)
{
    if (!mcpc || id == 0 || !params)
        return MCP_RC_ERROR;

    // Extract required parameters
    const char *node_name = NULL;
    if (json_object_object_get_ex(params, "node", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "node", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            node_name = json_object_get_string(obj);
        }
    }

    if (!node_name || !*node_name) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'node'");
        return MCP_RC_BAD_REQUEST;
    }

    const char *function_name = NULL;
    if (json_object_object_get_ex(params, "function", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "function", &obj);
        if (obj && json_object_is_type(obj, json_type_string)) {
            function_name = json_object_get_string(obj);
        }
    }

    if (!function_name || !*function_name) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'function'");
        return MCP_RC_BAD_REQUEST;
    }

    int timeout = 60; // Default timeout 60 seconds
    if (json_object_object_get_ex(params, "timeout", NULL)) {
        struct json_object *obj = NULL;
        json_object_object_get_ex(params, "timeout", &obj);
        if (obj && json_object_is_type(obj, json_type_int)) {
            timeout = json_object_get_int(obj);
            if (timeout <= 0)
                timeout = 60;
        }
    }

    // Find the host by hostname first
    RRDHOST *host = rrdhost_find_by_hostname(node_name);
    
    // If not found by hostname, try by GUID (node ID)
    if (!host) {
        host = rrdhost_find_by_guid(node_name);
    }
    
    if (!host) {
        buffer_sprintf(mcpc->error, "Node not found: %s", node_name);
        return MCP_RC_NOT_FOUND;
    }

    // Create a buffer for function result
    BUFFER *result_buffer = buffer_create(0, NULL);
    
    // Create a unique transaction ID
    char transaction[UUID_STR_LEN];
    nd_uuid_t transaction_uuid;
    uuid_generate(transaction_uuid);
    uuid_unparse_lower(transaction_uuid, transaction);

    // Create source buffer from user_auth
    CLEAN_BUFFER *source = buffer_create(0, NULL);
    user_auth_to_source_buffer(mcpc->user_auth, source);
    buffer_strcat(source, ",modelcontextprotocol");

    // Execute the function with proper permissions from the client
    int ret = rrd_function_run(
        host,
        result_buffer,
        timeout,
        mcpc->user_auth->access,
        function_name,
        true,
        transaction,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        buffer_tostring(source),
        false
    );

    if (ret != 200) {
        buffer_sprintf(mcpc->error, "Failed to execute function: %s on node %s, error code %d", 
                       function_name, node_name, ret);
        buffer_free(result_buffer);
        return MCP_RC_ERROR;
    }

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    {
        // Start building content array for the result
        buffer_json_member_add_array(mcpc->result, "content");
        {
            // Add the function execution result as text content
            buffer_json_add_array_item_object(mcpc->result);
            {
                buffer_json_member_add_string(mcpc->result, "type", "text");
                buffer_json_member_add_string(mcpc->result, "text", buffer_tostring(result_buffer));
            }
            buffer_json_object_close(mcpc->result); // Close text content
        }
        buffer_json_array_close(mcpc->result);  // Close content array
    }
    buffer_json_object_close(mcpc->result); // Close result object
    buffer_json_finalize(mcpc->result); // Finalize the JSON

    buffer_free(result_buffer);
    
    return MCP_RC_OK;
}
