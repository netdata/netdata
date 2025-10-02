// SPDX-License-Identifier: GPL-3.0-or-later

/**
 * MCP Logging Namespace
 * 
 * The MCP Logging namespace provides methods for controlling server logging behavior.
 * In the MCP protocol, logging methods allow clients to control the verbosity
 * of logging information shared by the server through notifications.
 * 
 * Standard methods in the MCP specification:
 * 
 * 1. logging/setLevel - Sets the logging level for the server
 *    - Takes a logging level parameter (e.g., "debug", "info", "warning", "error")
 *    - Configures which severity of log messages the server should send
 *    - Does not affect internal server logging, only what's sent to the client
 *    - Server responds with an empty result on success
 * 
 * The logging namespace helps clients control debugging information
 * and monitor server activity by adjusting the amount of logging
 * information provided by the server. After setting the level,
 * clients may receive logging notifications matching that level.
 * 
 * The server logs are sent as notifications/message events, providing
 * real-time monitoring of server operations for debugging purposes.
 */

#include "mcp.h"
#include "mcp-logging.h"

// Implementation of logging/setLevel (transport-agnostic)
static MCP_RETURN_CODE mcp_logging_method_setLevel(MCP_CLIENT *mcpc, struct json_object *params, MCP_REQUEST_ID id __maybe_unused) {
    if (!mcpc)
        return MCP_RC_ERROR;
    
    // Extract level parameter
    struct json_object *level_obj = NULL;
    if (!json_object_object_get_ex(params, "level", &level_obj)) {
        buffer_sprintf(mcpc->error, "Missing required parameter 'level'");
        return MCP_RC_BAD_REQUEST;
    }
    
    if (!json_object_is_type(level_obj, json_type_string)) {
        buffer_sprintf(mcpc->error, "Parameter 'level' must be a string");
        return MCP_RC_BAD_REQUEST;
    }
    
    const char *level = json_object_get_string(level_obj);
    
    // Validate log level
    if (!level || !*level) {
        buffer_sprintf(mcpc->error, "Log level cannot be empty");
        return MCP_RC_BAD_REQUEST;
    }
    
    // Parse the log level using the enum string mapping
    MCP_LOGGING_LEVEL parsed_level = MCP_LOGGING_LEVEL_2id(level);
    if (parsed_level == MCP_LOGGING_LEVEL_UNKNOWN) {
        buffer_sprintf(mcpc->error, "Invalid log level: '%s'. Valid values are: debug, info, notice, warning, error, critical, alert, emergency", level);
        return MCP_RC_BAD_REQUEST;
    }
    
    // Store the parsed log level in the client context
    mcpc->logging_level = parsed_level;
    
    // Log that we received this request and set the level
    netdata_log_info("MCP client %s logging level set to: %s", string2str(mcpc->client_name), MCP_LOGGING_LEVEL_2str(parsed_level));
    
    // Initialize success' response with an empty result object
    mcp_init_success_result(mcpc, id);
    buffer_json_finalize(mcpc->result);
    
    return MCP_RC_OK;
}

// Logging namespace method dispatcher (transport-agnostic)
MCP_RETURN_CODE mcp_logging_route(MCP_CLIENT *mcpc, const char *method, struct json_object *params, MCP_REQUEST_ID id) {
    if (!mcpc || !method) return MCP_RC_INTERNAL_ERROR;

    netdata_log_debug(D_MCP, "MCP logging method: %s", method);

    MCP_RETURN_CODE rc;
    
    if (strcmp(method, "setLevel") == 0) {
        rc = mcp_logging_method_setLevel(mcpc, params, id);
    }
    else {
        // Method not found in logging namespace
        buffer_sprintf(mcpc->error, "Method 'logging/%s' not supported. The MCP specification only defines 'setLevel' method.", method);
        rc = MCP_RC_NOT_IMPLEMENTED;
    }
    
    return rc;
}
