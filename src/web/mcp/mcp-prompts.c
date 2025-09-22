// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Prompts Namespace
 * 
 * The MCP Prompts namespace provides methods for managing prompts.
 * In the MCP protocol, prompts are text templates that guide AI generation for specific tasks.
 * Prompts are user-controlled interactions that leverage AI capabilities in predefined ways.
 * 
 * Standard methods in the MCP specification:
 * 
 * 1. prompts/list - Lists available prompts
 *    - Returns a collection of prompts the server offers
 *    - Includes prompt names, descriptions, and potentially argument information
 *    - Can be paginated for large prompt collections
 * 
 * 2. prompts/get - Gets details about a specific prompt
 *    - Takes a prompt name and returns its full definition
 *    - Returns the prompt template, messages, and metadata
 *    - May include information about required arguments
 * 
 * Prompts differ from tools in that they are:
 *    - More flexible and text-oriented
 *    - Designed for natural language processing
 *    - Often used for analysis and summarization
 *    - Usually invoked explicitly by users rather than by the model
 * 
 * In the Netdata context, prompts might include:
 *    - Analyzing a time period of metrics for anomalies
 *    - Summarizing system health
 *    - Creating natural language explanations of charts
 *    - Helping users create custom alert configurations
 *    - Generating analysis reports
 * 
 * Prompts typically use templating to insert user-provided context into predefined templates,
 * making them powerful for specific analysis tasks while maintaining predictable outputs.
 */

#include "mcp-prompts.h"

// Implementation of prompts/list (transport-agnostic)
static MCP_RETURN_CODE mcp_prompts_method_list(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc)
        return MCP_RC_ERROR;

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Add an empty prompts array
    buffer_json_member_add_array(mcpc->result, "prompts");
    buffer_json_array_close(mcpc->result); // Close prompts array
    
    // Close the result object
    buffer_json_finalize(mcpc->result);
    
    return MCP_RC_OK;
}

// Stub implementations for other prompts methods (transport-agnostic)

static MCP_RETURN_CODE mcp_prompts_method_get(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id __maybe_unused) {
    buffer_sprintf(mcpc->error, "Method 'prompts/get' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}





// Prompts namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_prompts_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;

    netdata_log_debug(D_MCP, "MCP prompts method: %s", method);

    MCP_RETURN_CODE rc;
    
    if (strcmp(method, "list") == 0) {
        rc = mcp_prompts_method_list(mcpc, params, id);
    }
    else if (strcmp(method, "get") == 0) {
        rc = mcp_prompts_method_get(mcpc, params, id);
    }
    else {
        // Method not found in prompts namespace
        buffer_sprintf(mcpc->error, "Method 'prompts/%s' not implemented yet", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}
