// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP System Namespace
 * 
 * The MCP System namespace provides methods for querying and managing the server system.
 * These methods provide information about the server's state, health, and performance,
 * and allow for basic administrative operations.
 * 
 * Key features of the system namespace:
 * 
 * 1. System Information:
 *    - Get server health status (system/health)
 *    - Get detailed version information (system/version)
 *    - Get server performance metrics (system/metrics)
 *    - Get current system status (system/status)
 * 
 * 2. System Management:
 *    - Request server restart (system/restart)
 * 
 * System methods typically require elevated permissions, as they can affect
 * the operation of the server and may provide sensitive information.
 * 
 * In the Netdata context, system methods provide:
 *    - Netdata Agent version and build information
 *    - Server metrics (CPU, memory usage, uptime, etc.)
 *    - Runtime configuration status
 *    - Agent health and operational status
 *    - Administrative operations for authorized users
 * 
 * These methods are particularly useful for:
 *    - System administrators monitoring Netdata servers
 *    - Tools that need to check for version compatibility
 *    - Health monitoring systems tracking Netdata itself
 *    - Administrative interfaces
 */

#include "mcp-system.h"
#include "mcp-initialize.h"
#include "config.h" // Include config.h for NETDATA_VERSION

// Stub implementations for system methods (transport-agnostic)
static int mcp_system_method_health(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "system/health", id);
}

static int mcp_system_method_version(MCP_CLIENT *ctx, struct json_object *params __maybe_unused, uint64_t id) {
    if (!ctx || id == 0) return -1;
    
    struct json_object *result = json_object_new_object();
    
    // Add version information
    json_object_object_add(result, "name", json_object_new_string("Netdata"));
    json_object_object_add(result, "version", json_object_new_string(NETDATA_VERSION));
    json_object_object_add(result, "mcpVersion", json_object_new_string(MCP_PROTOCOL_VERSION_2str(MCP_PROTOCOL_VERSION_LATEST)));
    
    // Send success response and free the result object
    int ret = mcp_send_success_response(ctx, result, id);
    json_object_put(result);
    
    return ret;
}

static int mcp_system_method_metrics(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "system/metrics", id);
}

static int mcp_system_method_restart(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "system/restart", id);
}

static int mcp_system_method_status(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    return mcp_method_not_implemented_generic(mcpc, "system/status", id);
}

// System namespace method dispatcher (transport-agnostic)
int mcp_system_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return -1;

    netdata_log_debug(D_MCP, "MCP system method: %s", method);

    if (strcmp(method, "health") == 0) {
        return mcp_system_method_health(mcpc, params, id);
    }
    else if (strcmp(method, "version") == 0) {
        return mcp_system_method_version(mcpc, params, id);
    }
    else if (strcmp(method, "metrics") == 0) {
        return mcp_system_method_metrics(mcpc, params, id);
    }
    else if (strcmp(method, "restart") == 0) {
        return mcp_system_method_restart(mcpc, params, id);
    }
    else if (strcmp(method, "status") == 0) {
        return mcp_system_method_status(mcpc, params, id);
    }
    else {
        // Method not found in system namespace
        char full_method[256];
        snprintf(full_method, sizeof(full_method), "system/%s", method);
        return mcp_method_not_implemented_generic(mcpc, full_method, id);
    }
}
