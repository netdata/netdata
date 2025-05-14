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
static MCP_RETURN_CODE mcp_system_method_health(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'system/health' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_system_method_version(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    if (!mcpc || id == 0) return MCP_RC_ERROR;
    
    // Initialize success response
    mcp_init_success_result(mcpc, id);
    
    // Add version information
    buffer_json_member_add_string(mcpc->result, "name", "Netdata");
    buffer_json_member_add_string(mcpc->result, "version", NETDATA_VERSION);
    buffer_json_member_add_string(mcpc->result, "mcpVersion", MCP_PROTOCOL_VERSION_2str(MCP_PROTOCOL_VERSION_LATEST));
    
    // Close the result object
    buffer_json_finalize(mcpc->result);
    
    return MCP_RC_OK;
}

static MCP_RETURN_CODE mcp_system_method_metrics(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'system/metrics' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_system_method_restart(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'system/restart' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

static MCP_RETURN_CODE mcp_system_method_status(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, uint64_t id) {
    buffer_sprintf(mcpc->error, "Method 'system/status' not implemented yet");
    return MCP_RC_NOT_IMPLEMENTED;
}

// System namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_system_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, uint64_t id) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;

    netdata_log_debug(D_MCP, "MCP system method: %s", method);
    
    // Flush previous buffers
    buffer_flush(mcpc->result);
    buffer_flush(mcpc->error);
    
    MCP_RETURN_CODE rc;

    if (strcmp(method, "health") == 0) {
        rc = mcp_system_method_health(mcpc, params, id);
    }
    else if (strcmp(method, "version") == 0) {
        rc = mcp_system_method_version(mcpc, params, id);
    }
    else if (strcmp(method, "metrics") == 0) {
        rc = mcp_system_method_metrics(mcpc, params, id);
    }
    else if (strcmp(method, "restart") == 0) {
        rc = mcp_system_method_restart(mcpc, params, id);
    }
    else if (strcmp(method, "status") == 0) {
        rc = mcp_system_method_status(mcpc, params, id);
    }
    else {
        // Method not found in system namespace
        buffer_sprintf(mcpc->error, "Method 'system/%s' not implemented yet", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}
