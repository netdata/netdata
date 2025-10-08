// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Completion Namespace
 * 
 * The MCP Completion namespace provides methods for handling input argument completions.
 * In the MCP protocol, completion methods allow clients to request suggestions
 * for input fields, providing a better user experience when interacting with tools,
 * resources, or prompts.
 * 
 * Standard methods in the MCP specification:
 * 
 * 1. completion/complete - Requests completion suggestions for an input argument
 *    - Takes an argument name, current value, and reference context
 *    - Reference can be to a tool, resource, or prompt
 *    - Returns an array of possible completion values
 *    - May include pagination information for large result sets
 * 
 * The completion namespace enhances user interactions by providing
 * real-time suggestions for input fields, improving usability
 * and reducing errors when working with complex arguments.
 * 
 * Completions are context-aware, meaning they take into account what the
 * completion is for (which tool, resource, or prompt) and provide
 * relevant suggestions based on that context.
 */

#include "mcp-completion.h"

// Implementation of completion/complete (transport-agnostic)
static MCP_RETURN_CODE mcp_completion_method_complete(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc) return MCP_RC_ERROR;
    
    // Extract argument and ref parameters
    struct json_object *argument_obj = NULL;
    struct json_object *ref_obj = NULL;
    
    if (!json_object_object_get_ex(params, "argument", &argument_obj)) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'argument'");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_object_get_ex(params, "ref", &ref_obj)) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'ref'");
        return MCP_RC_BAD_REQUEST;
    }
    
    // Extract argument name and value
    struct json_object *name_obj = NULL;
    struct json_object *value_obj = NULL;
    
    if (!json_object_object_get_ex(argument_obj, "name", &name_obj)) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'argument.name'");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_object_get_ex(argument_obj, "value", &value_obj)) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'argument.value'");
        return MCP_RC_BAD_REQUEST;
    }
    
    const char *name = json_object_get_string(name_obj);
    const char *value = json_object_get_string(value_obj);
    
    // Log that we received this request (actual implementation would generate completion options)
    netdata_log_info("MCP received completion/complete request for argument '%s' with value '%s'", name, value);
    
    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Add completion data (sample implementation with a few static options)
    buffer_json_member_add_object(mcpc->result, "completion");
    
    // Add values-array with sample completion options
    buffer_json_member_add_array(mcpc->result, "values");
    buffer_json_add_array_item_string(mcpc->result, "option1");
    buffer_json_add_array_item_string(mcpc->result, "option2");
    buffer_json_add_array_item_string(mcpc->result, "option3");
    buffer_json_array_close(mcpc->result); // Close values array
    
    // Add optional fields
    buffer_json_member_add_boolean(mcpc->result, "hasMore", false);
    buffer_json_member_add_int64(mcpc->result, "total", 3);
    
    buffer_json_object_close(mcpc->result); // Close completion object
    buffer_json_finalize(mcpc->result);
    
    return MCP_RC_OK;
}

// Completion namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_completion_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;

    netdata_log_debug(D_MCP, "MCP completion method: %s", method);

    MCP_RETURN_CODE rc;
    
    if (strcmp(method, "complete") == 0) {
        rc = mcp_completion_method_complete(mcpc, params, id);
    }
    else {
        // Method not found in completion namespace
        buffer_sprintf(mcpc->error, "Method 'completion/%s' not supported. The MCP specification only defines 'complete' method.", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}
