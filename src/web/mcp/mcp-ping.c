// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Ping Method
 * 
 * The ping method is a core part of the Model Context Protocol (MCP),
 * allowing connection health checks between client and server.
 * 
 * Standard method in the MCP specification:
 * 
 * 1. ping - Simple connection health check
 *    - Takes no parameters (empty params object)
 *    - The receiver must respond promptly with an empty result object
 *    - Either client or server can initiate a ping
 *    - If no response is received within a reasonable timeout, the connection may be considered stale
 * 
 * According to the MCP specification:
 * - The ping method is mandatory for all MCP implementations
 * - It serves as a basic mechanism to verify the connection is still active
 * - Implementations should handle ping requests promptly to ensure accurate health checks
 * 
 * This implementation provides a simple handler for ping requests that responds with an
 * empty result object, as required by the specification.
 */

#include "mcp-ping.h"

/**
 * Handle a ping request from a client or server
 * 
 * @param mcpc The MCP client context
 * @param params The JSON params object (should be empty for ping)
 * @param id The request ID
 * @return MCP_RETURN_CODE - MCP_RC_OK if successful
 */
MCP_RETURN_CODE mcp_method_ping(MCP_CLIENT *mcpc, struct json_object *params __maybe_unused, MCP_REQUEST_ID id) {
    if (!mcpc) {
        return MCP_RC_ERROR;
    }
    
    // Initialize success response with empty result object
    mcp_init_success_result(mcpc, id);
    buffer_json_finalize(mcpc->result);
    
    // Log the ping for debugging
    netdata_log_debug(D_MCP, "Received ping request (ID: %zu), responded", id);
    
    return MCP_RC_OK;
}