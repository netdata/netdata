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
static int mcp_prompts_method_list(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    if (!mcpc || id == 0) return -1;

    // Currently we don't support prompts, so return an empty list
    struct json_object *result = json_object_new_object();
    struct json_object *prompts_array = json_object_new_array();
    
    json_object_object_add(result, "prompts", prompts_array);
    
    // Send success response (and free the result object)
    int ret = mcp_send_success_response(mcpc, result, id);
    json_object_put(result);
    
    return ret;
}

// Stub implementations for other prompts methods (transport-agnostic)
static int mcp_prompts_method_execute(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "prompts/execute", id);
}

static int mcp_prompts_method_get(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "prompts/get", id);
}

static int mcp_prompts_method_save(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "prompts/save", id);
}

static int mcp_prompts_method_delete(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "prompts/delete", id);
}

static int mcp_prompts_method_getCategories(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "prompts/getCategories", id);
}

static int mcp_prompts_method_getHistory(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "prompts/getHistory", id);
}

// Prompts namespace method dispatcher (transport-agnostic)
int mcp_prompts_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return -1;
    
    netdata_log_debug(D_MCP, "MCP prompts method: %s", method);
    
    if (strcmp(method, "list") == 0) {
        return mcp_prompts_method_list(mcpc, params, id);
    }
    else if (strcmp(method, "execute") == 0) {
        return mcp_prompts_method_execute(mcpc, params, id);
    }
    else if (strcmp(method, "get") == 0) {
        return mcp_prompts_method_get(mcpc, params, id);
    }
    else if (strcmp(method, "save") == 0) {
        return mcp_prompts_method_save(mcpc, params, id);
    }
    else if (strcmp(method, "delete") == 0) {
        return mcp_prompts_method_delete(mcpc, params, id);
    }
    else if (strcmp(method, "getCategories") == 0) {
        return mcp_prompts_method_getCategories(mcpc, params, id);
    }
    else if (strcmp(method, "getHistory") == 0) {
        return mcp_prompts_method_getHistory(mcpc, params, id);
    }
    else {
        // Method not found in prompts namespace
        char full_method[256];
        snprintf(full_method, sizeof(full_method), "prompts/%s", method);
        return mcp_method_not_implemented_generic(mcpc, full_method, id);
    }
}
