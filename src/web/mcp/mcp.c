// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp.h"
#include "mcp-initialize.h"
#include "mcp-ping.h"
#include "mcp-tools.h"
#include "mcp-resources.h"
#include "mcp-prompts.h"
#include "mcp-logging.h"
#include "mcp-completion.h"
#include "adapters/mcp-websocket.h"

// Define the enum to string mapping for protocol versions
ENUM_STR_MAP_DEFINE(MCP_PROTOCOL_VERSION) = {
    { .id = MCP_PROTOCOL_VERSION_2024_11_05, .name = "2024-11-05" },
    { .id = MCP_PROTOCOL_VERSION_2025_03_26, .name = "2025-03-26" },
    { .id = MCP_PROTOCOL_VERSION_UNKNOWN, .name = "unknown" },
    
    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(MCP_PROTOCOL_VERSION, MCP_PROTOCOL_VERSION_UNKNOWN, "unknown");

// Define the enum to string mapping for return codes
ENUM_STR_MAP_DEFINE(MCP_RETURN_CODE) = {
    { .id = MCP_RC_OK, .name = "OK" },
    { .id = MCP_RC_ERROR, .name = "ERROR" },
    { .id = MCP_RC_INVALID_PARAMS, .name = "INVALID_PARAMS" },
    { .id = MCP_RC_NOT_FOUND, .name = "NOT_FOUND" },
    { .id = MCP_RC_INTERNAL_ERROR, .name = "INTERNAL_ERROR" },
    { .id = MCP_RC_NOT_IMPLEMENTED, .name = "NOT_IMPLEMENTED" },
    { .id = MCP_RC_BAD_REQUEST, .name = "BAD_REQUEST" },
    
    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(MCP_RETURN_CODE, MCP_RC_ERROR, "ERROR");

// Define the enum to string mapping for logging levels
ENUM_STR_MAP_DEFINE(MCP_LOGGING_LEVEL) = {
    { .id = MCP_LOGGING_LEVEL_DEBUG, .name = "debug" },
    { .id = MCP_LOGGING_LEVEL_INFO, .name = "info" },
    { .id = MCP_LOGGING_LEVEL_NOTICE, .name = "notice" },
    { .id = MCP_LOGGING_LEVEL_WARNING, .name = "warning" },
    { .id = MCP_LOGGING_LEVEL_ERROR, .name = "error" },
    { .id = MCP_LOGGING_LEVEL_CRITICAL, .name = "critical" },
    { .id = MCP_LOGGING_LEVEL_ALERT, .name = "alert" },
    { .id = MCP_LOGGING_LEVEL_EMERGENCY, .name = "emergency" },
    { .id = MCP_LOGGING_LEVEL_UNKNOWN, .name = "unknown" },
    
    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(MCP_LOGGING_LEVEL, MCP_LOGGING_LEVEL_UNKNOWN, "unknown");

// Decode a URI component using mcpc's pre-allocated buffer
// Returns a pointer to the decoded string which is valid until the next call
const char *mcp_uri_decode(MCP_CLIENT *mcpc, const char *src) {
    if(!mcpc || !src || !*src)
        return src;

    // Prepare the buffer
    buffer_flush(mcpc->uri);
    buffer_need_bytes(mcpc->uri, strlen(src) + 1);

    // Perform URL decoding
    char *d = url_decode_r(mcpc->uri->buffer, src, mcpc->uri->size);
    if (!d || !*d)
        return src;

    // Ensure the buffer's length is updated
    mcpc->uri->len = strlen(d);

    return buffer_tostring(mcpc->uri);
}

// Create a response context for a transport session
MCP_CLIENT *mcp_create_client(MCP_TRANSPORT transport, void *transport_ctx) {
    MCP_CLIENT *mcpc = callocz(1, sizeof(MCP_CLIENT));

    mcpc->transport = transport;
    mcpc->protocol_version = MCP_PROTOCOL_VERSION_UNKNOWN; // Will be set during initialization
    mcpc->ready = false; // Client is not ready until initialized notification is received
    
    // Set capabilities based on transport type
    switch (transport) {
        case MCP_TRANSPORT_WEBSOCKET:
            mcpc->websocket = (struct websocket_server_client *)transport_ctx;
            mcpc->capabilities = MCP_CAPABILITY_ASYNC_COMMUNICATION |
                               MCP_CAPABILITY_SUBSCRIPTIONS | 
                               MCP_CAPABILITY_NOTIFICATIONS;
            break;
            
        case MCP_TRANSPORT_HTTP:
            mcpc->http = (struct web_client *)transport_ctx;
            mcpc->capabilities = MCP_CAPABILITY_NONE; // HTTP has no special capabilities
            break;
            
        default:
            mcpc->generic = transport_ctx;
            mcpc->capabilities = MCP_CAPABILITY_NONE;
            break;
    }
    
    // Default client info (will be updated later from actual client)
    mcpc->client_name = string_strdupz("unknown");
    mcpc->client_version = string_strdupz("0.0.0");
    
    // Set default logging level to info
    mcpc->logging_level = MCP_LOGGING_LEVEL_INFO;
    
    // Initialize response buffers
    mcpc->result = buffer_create(4096, NULL);
    mcpc->error = buffer_create(1024, NULL);
    
    // Initialize utility buffers
    mcpc->uri = buffer_create(1024, NULL);
    
    // Initialize request IDs tracking
    mcpc->request_id_counter = 0;
    mcpc->request_ids = NULL;
    
    return mcpc;
}

// Free a response context
void mcp_free_client(MCP_CLIENT *mcpc) {
    if (mcpc) {
        string_freez(mcpc->client_name);
        string_freez(mcpc->client_version);
        
        // Free response buffers
        buffer_free(mcpc->result);
        buffer_free(mcpc->error);
        
        // Free utility buffers
        buffer_free(mcpc->uri);
        
        // Free request IDs
        mcp_request_id_cleanup_all(mcpc);
        
        freez(mcpc);
    }
}

// Map internal MCP_RETURN_CODE to JSON-RPC error code
static int mcp_map_return_code_to_jsonrpc_error(MCP_RETURN_CODE rc) {
    switch (rc) {
        case MCP_RC_OK:
            return 0; // Not an error
        case MCP_RC_INVALID_PARAMS:
            return -32602; // JSON-RPC Invalid params
        case MCP_RC_NOT_FOUND:
            return -32601; // JSON-RPC Method not found
        case MCP_RC_INTERNAL_ERROR:
            return -32603; // JSON-RPC Internal error
        case MCP_RC_NOT_IMPLEMENTED:
            return -32601; // Use method not found for not implemented
        case MCP_RC_BAD_REQUEST:
            return -32600; // JSON-RPC Invalid request
        case MCP_RC_ERROR:
        default:
            return -32000; // JSON-RPC Server error
    }
}

void mcp_init_success_result(MCP_CLIENT *mcpc, MCP_REQUEST_ID id) {
    buffer_flush(mcpc->result);
    buffer_json_initialize(mcpc->result, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_string(mcpc->result, "jsonrpc", "2.0");

    // Add the ID using our request ID system
    mcp_request_id_to_buffer(mcpc, mcpc->result, "id", id);
    buffer_json_member_add_object(mcpc->result, "result");

    buffer_flush(mcpc->error);
}

MCP_RETURN_CODE mcp_error_result(MCP_CLIENT *mcpc, MCP_REQUEST_ID id, MCP_RETURN_CODE rc) {
    if (!mcpc) return rc;
    
    buffer_flush(mcpc->result);
    buffer_json_initialize(mcpc->result, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_string(mcpc->result, "jsonrpc", "2.0");
    
    // Add the ID using our request ID system
    mcp_request_id_to_buffer(mcpc, mcpc->result, "id", id);

    buffer_json_member_add_object(mcpc->result, "error");
    buffer_json_member_add_int64(mcpc->result, "code", mcp_map_return_code_to_jsonrpc_error(rc));
    
    const char *error_message = buffer_strlen(mcpc->error) 
                              ? buffer_tostring(mcpc->error) 
                              : MCP_RETURN_CODE_2str(rc);
    
    if(error_message && *error_message)
        buffer_json_member_add_string(mcpc->result, "message", error_message);
    
    buffer_json_object_close(mcpc->result); // Close error
    
    buffer_json_finalize(mcpc->result);
    return rc;
}

// No longer needed - we're using mcp_request_id_del directly in mcp_single_request

// Send the content of a buffer using the appropriate transport
int mcp_send_response_buffer(MCP_CLIENT *mcpc) {
    if (!mcpc || !mcpc->result || !buffer_strlen(mcpc->result))
        return -1;
    
    switch (mcpc->transport) {
        case MCP_TRANSPORT_WEBSOCKET:
            return mcp_websocket_send_buffer(mcpc->websocket, mcpc->result);
            
        case MCP_TRANSPORT_HTTP:
            netdata_log_error("MCP: HTTP adapter not implemented yet");
            return -1;

        default:
            netdata_log_error("MCP: Unknown transport type %u", mcpc->transport);
            return -1;
    }
}

// Parse and extract client info from initialize request params
static void mcp_extract_client_info(MCP_CLIENT *mcpc, struct json_object *params) {
    if (!mcpc || !params) return;
    
    struct json_object *client_info_obj = NULL;
    struct json_object *client_name_obj = NULL;
    struct json_object *client_version_obj = NULL;
    
    if (json_object_object_get_ex(params, "clientInfo", &client_info_obj)) {
        if (json_object_object_get_ex(client_info_obj, "name", &client_name_obj)) {
            string_freez(mcpc->client_name);
            mcpc->client_name = string_strdupz(json_object_get_string(client_name_obj));
        }
        if (json_object_object_get_ex(client_info_obj, "version", &client_version_obj)) {
            string_freez(mcpc->client_version);
            mcpc->client_version = string_strdupz(json_object_get_string(client_version_obj));
        }
    }
}

// Handle a JSON-RPC method call - the result is always filled with a jsonrpc response
static MCP_RETURN_CODE mcp_single_request(MCP_CLIENT *mcpc, struct json_object *request) {
    if (!mcpc || !request) {
        return MCP_RC_ERROR;
    }

    // Flush buffers before processing the request
    buffer_reset(mcpc->result);
    buffer_reset(mcpc->error);
    
    // Extract JSON-RPC fields
    struct json_object *method_obj = NULL;
    struct json_object *params_obj = NULL;
    struct json_object *jsonrpc_obj = NULL;
    
    // Validate jsonrpc version
    if (!json_object_object_get_ex(request, "jsonrpc", &jsonrpc_obj) ||
        strcmp(json_object_get_string(jsonrpc_obj), "2.0") != 0) {
        buffer_strcat(mcpc->error, "Invalid or missing jsonrpc version");
        mcp_error_result(mcpc, 0, MCP_RC_INVALID_PARAMS);
        return MCP_RC_INVALID_PARAMS;
    }
    
    // Extract method
    if (!json_object_object_get_ex(request, "method", &method_obj)) {
        buffer_strcat(mcpc->error, "Missing method field");
        mcp_error_result(mcpc, 0, MCP_RC_INVALID_PARAMS);
        return MCP_RC_INVALID_PARAMS;
    }
    
    const char *method = json_object_get_string(method_obj);
    
    // Extract params (optional)
    bool params_created = false;
    if (json_object_object_get_ex(request, "params", &params_obj)) {
        if (json_object_get_type(params_obj) != json_type_object) {
            buffer_strcat(mcpc->error, "params must be an object");
            mcp_error_result(mcpc, 0, MCP_RC_INVALID_PARAMS);
            return MCP_RC_INVALID_PARAMS;
        }
    } else {
        // Create an empty params object if none provided
        params_obj = json_object_new_object();
        params_created = true;
    }
    
    // Extract and register the request ID
    MCP_REQUEST_ID id = mcp_request_id_add(mcpc, request);
    bool has_id = (id != 0);

    // If we have a request ID, log it
    if (has_id) {
        netdata_log_debug(D_WEB_CLIENT, "MCP: Handling method call: %s (request_id: %zu)", method, id);
    } else {
        netdata_log_debug(D_WEB_CLIENT, "MCP: Handling notification: %s (no id)", method);
    }
    
    // Handle method calls based on namespace
    MCP_RETURN_CODE rc;

    // Check for notifications/initialized method which marks client as ready
    if(!method || !*method) {
        buffer_strcat(mcpc->error, "Empty method name");
        rc = MCP_RC_INVALID_PARAMS;
    }
    else if (strcmp(method, "notifications/initialized") == 0) {
        mcpc->ready = true;
        netdata_log_debug(D_WEB_CLIENT, "MCP client %s v%s is now ready", 
                         string2str(mcpc->client_name), string2str(mcpc->client_version));
        rc = MCP_RC_OK;
    }
    else if (strncmp(method, "tools/", 6) == 0) {
        // Tools namespace
        rc = mcp_tools_route(mcpc, method + 6, params_obj, id);
        // Mark client as ready if not already
        if (!mcpc->ready) {
            mcpc->ready = true;
        }
    }
    else if (strncmp(method, "resources/", 10) == 0) {
        // Resources namespace
        rc = mcp_resources_route(mcpc, method + 10, params_obj, id);
        // Mark client as ready if not already
        if (!mcpc->ready) {
            mcpc->ready = true;
        }
    }
    else if (strncmp(method, "prompts/", 8) == 0) {
        // Prompts namespace
        rc = mcp_prompts_route(mcpc, method + 8, params_obj, id);
        // Mark client as ready if not already
        if (!mcpc->ready) {
            mcpc->ready = true;
        }
    }
    else if (strncmp(method, "logging/", 8) == 0) {
        // Logging namespace - don't alter ready state
        rc = mcp_logging_route(mcpc, method + 8, params_obj, id);
    }
    else if (strncmp(method, "completion/", 11) == 0) {
        // Completion namespace
        rc = mcp_completion_route(mcpc, method + 11, params_obj, id);
        // Mark client as ready if not already
        if (!mcpc->ready) {
            mcpc->ready = true;
        }
    }
    else if (strcmp(method, "initialize") == 0) {
        // Extract client info from initialize request
        mcp_extract_client_info(mcpc, params_obj);
        netdata_log_debug(D_WEB_CLIENT, "MCP initialize request from client %s v%s", 
                          string2str(mcpc->client_name), string2str(mcpc->client_version));
        
        // Handle initialize method
        rc = mcp_method_initialize(mcpc, params_obj, id);
    }
    else if (strcmp(method, "ping") == 0) {
        // Handle ping method - simple connection health check
        // Don't alter ready state for ping requests
        rc = mcp_method_ping(mcpc, params_obj, id);
    }
    else {
        buffer_sprintf(mcpc->error, "Method '%s' not found", method);
        rc = MCP_RC_NOT_FOUND;
        // Method not found shouldn't alter ready state
    }

    // If this is a notification (no ID), don't generate a response
    if (!has_id) {
        // Clean up the params object if we created it
        if (params_created) {
            json_object_put(params_obj);
        }
        return rc;
    }

    // For requests with IDs, ensure we have a valid response
    if (rc != MCP_RC_OK && !buffer_strlen(mcpc->result)) {
        mcp_error_result(mcpc, id, rc);
    }

    if (!buffer_strlen(mcpc->result)) {
        buffer_strcat(mcpc->error, "method generated empty result");
        mcp_error_result(mcpc, id, MCP_RC_INTERNAL_ERROR);
    }

    // Clean up the request ID
    mcp_request_id_del(mcpc, id);

    // Clean up the params object if we created it
    if (params_created) {
        json_object_put(params_obj);
    }

    return rc;
}

// Main MCP entry point - handle a JSON-RPC request (can be single or batch)
MCP_RETURN_CODE mcp_handle_request(MCP_CLIENT *mcpc, struct json_object *request) {
    if (!mcpc || !request)
        return MCP_RC_INTERNAL_ERROR;
    
    // Clear previous response buffers
    buffer_flush(mcpc->result);
    buffer_flush(mcpc->error);
    
    // Check if this is a batch request (JSON array)
    if (json_object_get_type(request) == json_type_array) {
        int array_len = json_object_array_length(request);
        
        // Empty batch should return nothing according to JSON-RPC 2.0 spec
        if (array_len == 0) {
            return MCP_RC_OK;
        }
        
        // Create a temporary buffer for building the batch response
        BUFFER *batch_buffer = buffer_create(4096, NULL);
        buffer_flush(batch_buffer);
        
        // Start the JSON array for batch response
        buffer_strcat(batch_buffer, "[");
        
        // Track if we've added any responses (for comma handling)
        size_t responses_added = 0;
        
        // Process each request in the batch
        for (int i = 0; i < array_len; i++) {
            struct json_object *req_item = json_object_array_get_idx(request, i);
            
            // Process the individual request
            buffer_flush(mcpc->result);
            buffer_flush(mcpc->error);
            
            // Call the single request handler
            mcp_single_request(mcpc, req_item);
            
            // For notifications (no id), don't add to response
            if (buffer_strlen(mcpc->result) == 0) {
                continue;
            }
            
            // Add comma if this isn't the first response
            if (responses_added) {
                buffer_strcat(batch_buffer, ", ");
            }
            
            // Add the response to the batch
            buffer_strcat(batch_buffer, buffer_tostring(mcpc->result));
            responses_added++;
        }
        
        // If no responses were added (all notifications), don't send anything per JSON-RPC spec
        if (!responses_added) {
            buffer_free(batch_buffer);
            return MCP_RC_OK;
        }
        
        // Close the JSON array
        buffer_strcat(batch_buffer, "]");
        
        // Copy batch response to client's result buffer
        buffer_flush(mcpc->result);
        buffer_strcat(mcpc->result, buffer_tostring(batch_buffer));
        buffer_free(batch_buffer);
        
        // Send the batch response
        mcp_send_response_buffer(mcpc);
        
        return MCP_RC_OK;
    } 
    else {
        // Handle single request
        MCP_RETURN_CODE rc = mcp_single_request(mcpc, request);
        mcp_send_response_buffer(mcpc);
        return rc;
    }
}

// Initialize the MCP subsystem
void mcp_initialize_subsystem(void) {
    netdata_log_info("MCP subsystem initialized");

    debug_flags |= D_MCP;
}
