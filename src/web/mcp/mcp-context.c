// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Context Namespace
 * 
 * The MCP Context namespace provides methods for managing contextual information
 * exchanged between clients and servers. Context represents stateful information
 * that enhances the interaction between components.
 * 
 * Key features of the context namespace:
 * 
 * 1. Context Management:
 *    - Provide contextual information to the server (context/provide)
 *    - Clear specific context data (context/clear)
 *    - Check the status of current context (context/status)
 * 
 * 2. Context Persistence:
 *    - Save context for future use (context/save)
 *    - Load previously saved context (context/load)
 * 
 * Context in MCP can include:
 *    - User preferences and settings
 *    - Session-specific information
 *    - Authentication and authorization details
 *    - Client capabilities and limitations
 *    - Conversation or interaction history
 * 
 * In the Netdata environment, context might include:
 *    - User display preferences (theme, date formats, etc.)
 *    - View configurations (dashboard layouts, chart settings)
 *    - Filtering and query preferences
 *    - Historical interaction patterns
 *    - Authentication tokens and permissions
 * 
 * Context can be transient (per session) or persistent (saved across sessions),
 * and may be scoped to specific interactions or broadly applied.
 */

#include "mcp-context.h"
#include "mcp-initialize.h"

// Stub implementations for all context namespace methods (transport-agnostic)
static int mcp_context_method_provide(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "context/provide", id);
}

static int mcp_context_method_clear(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "context/clear", id);
}

static int mcp_context_method_status(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "context/status", id);
}

static int mcp_context_method_save(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "context/save", id);
}

static int mcp_context_method_load(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "context/load", id);
}

// Context namespace method dispatcher (transport-agnostic)
int mcp_context_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return -1;
    
    netdata_log_debug(D_MCP, "MCP context method: %s", method);
    
    if (strcmp(method, "provide") == 0) {
        return mcp_context_method_provide(mcpc, params, id);
    }
    else if (strcmp(method, "clear") == 0) {
        return mcp_context_method_clear(mcpc, params, id);
    }
    else if (strcmp(method, "status") == 0) {
        return mcp_context_method_status(mcpc, params, id);
    }
    else if (strcmp(method, "save") == 0) {
        return mcp_context_method_save(mcpc, params, id);
    }
    else if (strcmp(method, "load") == 0) {
        return mcp_context_method_load(mcpc, params, id);
    }
    else {
        // Method not found in context namespace
        char full_method[256];
        snprintf(full_method, sizeof(full_method), "context/%s", method);
        return mcp_method_not_implemented_generic(mcpc, full_method, id);
    }
}
