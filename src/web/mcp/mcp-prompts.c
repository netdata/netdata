// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Prompts Namespace
 * 
 * The MCP Prompts namespace provides methods for managing and executing prompts.
 * In the MCP protocol, prompts are text templates that guide AI generation for specific tasks.
 * Prompts are user-controlled interactions that leverage AI capabilities in predefined ways.
 * 
 * Key features of the prompts namespace:
 * 
 * 1. Prompt Management:
 *    - List available prompts (prompts/list)
 *    - Get details about specific prompts (prompts/get)
 *    - Save custom prompts (prompts/save)
 *    - Delete prompts (prompts/delete)
 *    - Organize prompts into categories (prompts/getCategories)
 * 
 * 2. Prompt Execution:
 *    - Execute prompts with input parameters (prompts/execute)
 *    - View execution history (prompts/getHistory)
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
#include "mcp-initialize.h"

// Implementation of prompts/list (transport-agnostic)
static MCP_RETURN_CODE mcp_prompts_method_list(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    if (!mcpc || id == 0) return MCP_RC_ERROR;

    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Add empty prompts array
    buffer_json_member_add_array(mcpc->result, "prompts");
    buffer_json_array_close(mcpc->result); // Close prompts array
    
    // Close the result object
    buffer_json_finalize(mcpc->result);
    
    return MCP_RC_OK;
}

// Stub implementations for other prompts methods (transport-agnostic)
static MCP_RETURN_CODE mcp_prompts_method_execute(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'prompts/execute' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_prompts_method_get(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'prompts/get' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_prompts_method_save(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'prompts/save' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_prompts_method_delete(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'prompts/delete' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_prompts_method_getCategories(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'prompts/getCategories' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_prompts_method_getHistory(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'prompts/getHistory' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

// Prompts namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_prompts_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;
    
    netdata_log_debug(D_MCP, "MCP prompts method: %s", method);
    
    // Flush previous buffers
    buffer_flush(mcpc->result);
    buffer_flush(mcpc->error);
    
    MCP_RETURN_CODE rc;
    
    if (strcmp(method, "list") == 0) {
        rc = mcp_prompts_method_list(mcpc, params, id);
    }
    else if (strcmp(method, "execute") == 0) {
        rc = mcp_prompts_method_execute(mcpc, params, id);
    }
    else if (strcmp(method, "get") == 0) {
        rc = mcp_prompts_method_get(mcpc, params, id);
    }
    else if (strcmp(method, "save") == 0) {
        rc = mcp_prompts_method_save(mcpc, params, id);
    }
    else if (strcmp(method, "delete") == 0) {
        rc = mcp_prompts_method_delete(mcpc, params, id);
    }
    else if (strcmp(method, "getCategories") == 0) {
        rc = mcp_prompts_method_getCategories(mcpc, params, id);
    }
    else if (strcmp(method, "getHistory") == 0) {
        rc = mcp_prompts_method_getHistory(mcpc, params, id);
    }
    else {
        // Method not found in prompts namespace
        buffer_sprintf(mcpc->error, "Method 'prompts/%s' not implemented yet", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}
