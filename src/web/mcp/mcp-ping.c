// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Ping Functionality
 * 
 * This implements the ping functionality for the Model Context Protocol (MCP).
 * Ping is a simple request/response mechanism that allows either the client or server
 * to verify that their counterpart is still responsive and the connection is alive.
 * 
 * According to the MCP specification:
 * 1. Either client or server can initiate a ping by sending a ping request
 * 2. The receiver must respond promptly with an empty result
 * 3. If no response is received within a reasonable timeout, the connection may be considered stale
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