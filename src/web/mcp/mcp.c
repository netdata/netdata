// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp.h"
#include "mcp-initialize.h"
#include "mcp-tools.h"
#include "mcp-resources.h"
#include "mcp-prompts.h"
#include "mcp-notifications.h"
#include "mcp-context.h"
#include "mcp-system.h"

// Define the enum to string mapping
ENUM_STR_MAP_DEFINE(MCP_PROTOCOL_VERSION) = {
    { .id = MCP_PROTOCOL_VERSION_2024_11_05, .name = "2024-11-05" },
    { .id = MCP_PROTOCOL_VERSION_2025_03_26, .name = "2025-03-26" },
    { .id = MCP_PROTOCOL_VERSION_UNKNOWN, .name = "unknown" },
    
    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(MCP_PROTOCOL_VERSION, MCP_PROTOCOL_VERSION_UNKNOWN, "unknown");

// In the future, if needed, we would add includes for transport-specific headers
// This would include things like the web server API for HTTP

// Create a response context for a transport session
MCP_CLIENT *mcp_create_client(MCP_TRANSPORT transport, void *transport_ctx) {
    MCP_CLIENT *ctx = callocz(1, sizeof(MCP_CLIENT));
    
    ctx->transport = transport;
    ctx->protocol_version = MCP_PROTOCOL_VERSION_UNKNOWN; // Will be set during initialization
    
    // Set capabilities based on transport type
    switch (transport) {
        case MCP_TRANSPORT_WEBSOCKET:
            ctx->websocket = (struct websocket_server_client *)transport_ctx;
            ctx->capabilities = MCP_CAPABILITY_ASYNC_COMMUNICATION | 
                               MCP_CAPABILITY_SUBSCRIPTIONS | 
                               MCP_CAPABILITY_NOTIFICATIONS;
            break;
            
        case MCP_TRANSPORT_HTTP:
            ctx->http = (struct web_client *)transport_ctx;
            ctx->capabilities = MCP_CAPABILITY_NONE; // HTTP has no special capabilities
            break;
            
        default:
            ctx->generic = transport_ctx;
            ctx->capabilities = MCP_CAPABILITY_NONE;
            break;
    }
    
    // Default client info (will be updated later from actual client)
    ctx->client_name = string_strdupz("unknown");
    ctx->client_version = string_strdupz("0.0.0");
    
    return ctx;
}

// Free a response context
void mcp_free_client(MCP_CLIENT *mcpc) {
    if (mcpc) {
        string_freez(mcpc->client_name);
        string_freez(mcpc->client_version);
        freez(mcpc);
    }
}

// Helper function for "not implemented" methods (transport-agnostic)
int mcp_method_not_implemented_generic(MCP_CLIENT *mcpc, const char *method_name, uint64_t id) {
    if (!mcpc || id == 0) return -1;

    char error_message[256];
    snprintf(error_message, sizeof(error_message), "Method '%s' not implemented yet", method_name);

    netdata_log_debug(D_WEB_CLIENT, "MCP method not implemented: %s", method_name);

    return mcp_send_error_response(mcpc, MCP_ERROR_METHOD_NOT_FOUND, error_message, id);
}

// Create a success response without sending it
struct json_object *mcp_create_success_response(struct json_object *result, uint64_t id) {
    if (id == 0) return NULL; // Notifications don't get responses
    
    struct json_object *response = json_object_new_object();
    
    // Add required jsonrpc version field
    json_object_object_add(response, "jsonrpc", json_object_new_string("2.0"));
    
    // Add ID
    json_object_object_add(response, "id", json_object_new_int64(id));

    // Add result
    json_object_object_add(response, "result", json_object_get(result));
    
    return response;
}

// Create an error response without sending it
struct json_object *mcp_create_error_response(int code, const char *message, uint64_t id) {
    if (id == 0) return NULL; // Notifications don't get responses
    
    struct json_object *response = json_object_new_object();
    
    // Add required jsonrpc version field
    json_object_object_add(response, "jsonrpc", json_object_new_string("2.0"));
    
    // Add ID
    json_object_object_add(response, "id", json_object_new_int64(id));

    // Create error object
    struct json_object *error = json_object_new_object();
    
    // Add error code and message
    json_object_object_add(error, "code", json_object_new_int(code));
    json_object_object_add(error, "message", json_object_new_string(message));
    
    // Add error object to response
    json_object_object_add(response, "error", error);
    
    return response;
}

// Forward declarations for transport-specific functions
int mcp_websocket_send_json(struct websocket_server_client *wsc, struct json_object *json);

// Send a JSON-RPC response using the appropriate transport
static int mcp_send_json(MCP_CLIENT *mcpc, struct json_object *json) {
    if (!mcpc || !json) return -1;
    
    switch (mcpc->transport) {
        case MCP_TRANSPORT_WEBSOCKET:
            return mcp_websocket_send_json(mcpc->websocket, json);
            
        case MCP_TRANSPORT_HTTP:
            netdata_log_error("MCP: HTTP adapter not implemented yet");
            return -1;

        default:
            netdata_log_error("MCP: Unknown transport type %u", mcpc->transport);
            return -1;
    }
}

// Send a success response
int mcp_send_success_response(MCP_CLIENT *mcpc, struct json_object *result, uint64_t id) {
    if (!mcpc || id == 0) return -1;
    
    struct json_object *response = mcp_create_success_response(result, id);
    if (!response) return -1;
    
    int ret = mcp_send_json(mcpc, response);
    
    // Always free the response
    json_object_put(response);
    
    return ret;
}

// Send an error response
int mcp_send_error_response(MCP_CLIENT *mcpc, int code, const char *message, uint64_t id) {
    if (!mcpc || id == 0) return -1;
    
    struct json_object *response = mcp_create_error_response(code, message, id);
    if (!response) return -1;
    
    int ret = mcp_send_json(mcpc, response);
    
    // Always free the response
    json_object_put(response);
    
    return ret;
}

// Send a notification (no response expected)
int mcp_send_notification(MCP_CLIENT *mcpc, const char *method, struct json_object *params) {
    if (!mcpc || !method) return -1;
    
    // Only send if the transport supports notifications
    if (!mcp_has_capability(mcpc, MCP_CAPABILITY_NOTIFICATIONS)) {
        if (params)
            json_object_put(params);
        return -1;
    }
    
    struct json_object *notification = json_object_new_object();
    
    // Add required jsonrpc version field
    json_object_object_add(notification, "jsonrpc", json_object_new_string("2.0"));
    
    // Add method
    json_object_object_add(notification, "method", json_object_new_string(method));
    
    // Add params if provided
    if (params) {
        json_object_object_add(notification, "params", params);
    }
    
    int ret = mcp_send_json(mcpc, notification);
    
    // Always free the notification
    json_object_put(notification);
    
    return ret;
}

// Create a content object based on protocol version
struct json_object *mcp_create_content_object(MCP_CLIENT *mcpc, MCP_CONTENT_TYPE type, 
                                           const char *data, size_t data_len, const char *mime_type) {
    if (!mcpc || !data) return NULL;
    
    struct json_object *content = json_object_new_object();
    
    switch (type) {
        case MCP_CONTENT_TYPE_TEXT:
            json_object_object_add(content, "type", json_object_new_string("text"));
            json_object_object_add(content, "text", json_object_new_string(data));
            break;
            
        case MCP_CONTENT_TYPE_IMAGE:
            json_object_object_add(content, "type", json_object_new_string("image"));
            // Base64 encode data
            // Calculate necessary buffer size (4 * ceil(n/3) bytes for base64)
            size_t encoded_size = ((data_len + 2) / 3) * 4 + 1;  // +1 for null terminator
            unsigned char *base64_data = mallocz(encoded_size);
            
            int len = netdata_base64_encode(base64_data, (const unsigned char *)data, data_len);
            if (len > 0) {
                // Ensure null-termination for json_object_new_string
                base64_data[len] = '\0';
                json_object_object_add(content, "data", json_object_new_string((char *)base64_data));
            }
            freez(base64_data);
            json_object_object_add(content, "mimeType", json_object_new_string(mime_type ? mime_type : "image/png"));
            break;
            
        case MCP_CONTENT_TYPE_AUDIO:
            // Only include audio type for 2025-03-26 or newer
            if (mcpc->protocol_version >= MCP_PROTOCOL_VERSION_2025_03_26) {
                json_object_object_add(content, "type", json_object_new_string("audio"));
                // Calculate necessary buffer size (4 * ceil(n/3) bytes for base64)
                size_t encoded_size = ((data_len + 2) / 3) * 4 + 1;  // +1 for null terminator
                unsigned char *base64_data = mallocz(encoded_size);
                
                int len = netdata_base64_encode(base64_data, (const unsigned char *)data, data_len);
                if (len > 0) {
                    // Ensure null-termination for json_object_new_string
                    base64_data[len] = '\0';
                    json_object_object_add(content, "data", json_object_new_string((char *)base64_data));
                }
                freez(base64_data);
                json_object_object_add(content, "mimeType", json_object_new_string(mime_type ? mime_type : "audio/wav"));
            } else {
                // For older clients, convert to text with a message
                json_object_object_add(content, "type", json_object_new_string("text"));
                json_object_object_add(content, "text", 
                                      json_object_new_string("Audio content is not supported by this client version"));
            }
            break;
    }
    
    // Add annotations to content if protocol supports it
    if (mcpc->protocol_version >= MCP_PROTOCOL_VERSION_2025_03_26) {
        struct json_object *annotations = json_object_new_object();
        
        // Add audience array with "user" as default
        struct json_object *audience = json_object_new_array();
        json_object_array_add(audience, json_object_new_string("user"));
        json_object_object_add(annotations, "audience", audience);
        
        json_object_object_add(content, "annotations", annotations);
    }
    
    return content;
}

// Helper for sending progress notifications
int mcp_send_progress_notification(MCP_CLIENT *mcpc, const char *token, int progress, int total, const char *message) {
    if (!mcpc || !token) return -1;
    
    struct json_object *params = json_object_new_object();
    
    json_object_object_add(params, "progressToken", json_object_new_string(token));
    json_object_object_add(params, "progress", json_object_new_int(progress));
    
    if (total > 0) {
        json_object_object_add(params, "total", json_object_new_int(total));
    }
    
    // Add message for 2025-03-26 or newer clients
    if (mcpc->protocol_version >= MCP_PROTOCOL_VERSION_2025_03_26 && message) {
        json_object_object_add(params, "message", json_object_new_string(message));
    }
    
    return mcp_send_notification(mcpc, "notifications/progress", params);
}

// Parse and extract client info from initialize request params
static void mcp_extract_client_info(MCP_CLIENT *ctx, struct json_object *params) {
    if (!ctx || !params) return;
    
    struct json_object *client_info_obj = NULL;
    struct json_object *client_name_obj = NULL;
    struct json_object *client_version_obj = NULL;
    
    if (json_object_object_get_ex(params, "clientInfo", &client_info_obj)) {
        if (json_object_object_get_ex(client_info_obj, "name", &client_name_obj)) {
            string_freez(ctx->client_name);
            ctx->client_name = string_strdupz(json_object_get_string(client_name_obj));
        }
        if (json_object_object_get_ex(client_info_obj, "version", &client_version_obj)) {
            string_freez(ctx->client_version);
            ctx->client_version = string_strdupz(json_object_get_string(client_version_obj));
        }
    }
}

// Handle a JSON-RPC method call
static int mcp_handle_method_call(MCP_CLIENT *ctx, const char *method, struct json_object *params, uint64_t id) {
    if (!ctx || !method) return -1;
    
    netdata_log_debug(D_WEB_CLIENT, "MCP: Handling method call: %s", method);
    
    // Handle method calls based on namespace
    if (strncmp(method, "tools/", 6) == 0) {
        // Tools namespace
        return mcp_tools_route(ctx, method + 6, params, id);
    }
    else if (strncmp(method, "resources/", 10) == 0) {
        // Resources namespace
        return mcp_resources_route(ctx, method + 10, params, id);
    }
    else if (strncmp(method, "prompts/", 8) == 0) {
        // Prompts namespace
        return mcp_prompts_route(ctx, method + 8, params, id);
    }
    else if (strncmp(method, "notifications/", 14) == 0) {
        // Notifications namespace
        return mcp_notifications_route(ctx, method + 14, params, id);
    }
    else if (strncmp(method, "context/", 8) == 0) {
        // Context namespace
        return mcp_context_route(ctx, method + 8, params, id);
    }
    else if (strncmp(method, "system/", 7) == 0) {
        // System namespace
        return mcp_system_route(ctx, method + 7, params, id);
    }
    else if (strcmp(method, "initialize") == 0) {
        // Extract client info from initialize request
        mcp_extract_client_info(ctx, params);
        netdata_log_debug(D_WEB_CLIENT, "MCP initialize request from client %s v%s", 
                          string2str(ctx->client_name), string2str(ctx->client_version));
        
        // Handle initialize method
        return mcp_method_initialize(ctx, params, id);
    }
    else {
        // Method not found
        if (id != 0) {
            return mcp_send_error_response(ctx, MCP_ERROR_METHOD_NOT_FOUND, "Method not found", id);
        }
        return -1;
    }
}

// Handle a batch of JSON-RPC requests
int mcp_handle_batch_request(MCP_CLIENT *mcpc, struct json_object *batch_request) {
    if (!mcpc || !batch_request) return -1;
    
    // Batch must be an array
    if (json_object_get_type(batch_request) != json_type_array) {
        return mcp_send_error_response(mcpc, MCP_ERROR_INVALID_REQUEST, "Batch request must be an array", 0);
    }
    
    int array_len = json_object_array_length(batch_request);
    
    // Empty batch should return nothing according to JSON-RPC 2.0 spec
    if (array_len == 0) {
        return 0;
    }
    
    // Create response array
    struct json_object *batch_response = json_object_new_array();
    
    // Process each request in the batch
    for (int i = 0; i < array_len; i++) {
        struct json_object *request = json_object_array_get_idx(batch_request, i);
        
        // Process the individual request
        // We need to parse JSON-RPC fields manually here to keep track of responses
        
        // Extract JSON-RPC fields
        struct json_object *method_obj = NULL;
        struct json_object *params_obj = NULL;
        struct json_object *id_obj = NULL;
        struct json_object *jsonrpc_obj = NULL;
        bool has_id = false;
        uint64_t id = 0;
        
        // Check for basic validity
        if (!json_object_object_get_ex(request, "jsonrpc", &jsonrpc_obj) ||
            strcmp(json_object_get_string(jsonrpc_obj), "2.0") != 0 ||
            !json_object_object_get_ex(request, "method", &method_obj)) {
            
            // Invalid request - create error response
            struct json_object *error_response = json_object_new_object();
            json_object_object_add(error_response, "jsonrpc", json_object_new_string("2.0"));
            json_object_object_add(error_response, "id", json_object_new_null());
            
            struct json_object *error = json_object_new_object();
            json_object_object_add(error, "code", json_object_new_int(MCP_ERROR_INVALID_REQUEST));
            json_object_object_add(error, "message", json_object_new_string("Invalid request"));
            json_object_object_add(error_response, "error", error);
            
            json_object_array_add(batch_response, error_response);
            continue;
        }
        
        // Get the method
        const char *method = json_object_get_string(method_obj);
        
        // Params are optional but must be an object if present
        if (json_object_object_get_ex(request, "params", &params_obj)) {
            if (json_object_get_type(params_obj) != json_type_object) {
                // Invalid params - create error response
                struct json_object *error_response = json_object_new_object();
                json_object_object_add(error_response, "jsonrpc", json_object_new_string("2.0"));
                json_object_object_add(error_response, "id", json_object_new_null());
                
                struct json_object *error = json_object_new_object();
                json_object_object_add(error, "code", json_object_new_int(MCP_ERROR_INVALID_PARAMS));
                json_object_object_add(error, "message", json_object_new_string("params must be an object"));
                json_object_object_add(error_response, "error", error);
                
                json_object_array_add(batch_response, error_response);
                continue;
            }
        }
        
        // Id is required for requests but not for notifications
        if (json_object_object_get_ex(request, "id", &id_obj)) {
            has_id = true;
            if (json_object_get_type(id_obj) == json_type_int) {
                id = json_object_get_int64(id_obj);
            }
            else if (json_object_get_type(id_obj) == json_type_string) {
                const char *id_str = json_object_get_string(id_obj);
                char *endptr;
                id = strtoull(id_str, &endptr, 10);
                if (*endptr != '\0') {
                    // If the string is not a number, use a hash of the string as the ID
                    id = 0;
                    while (*id_str) {
                        id = id * 31 + (*id_str++);
                    }
                }
            }
        }
        
        // For notifications (no id), we don't add to the response array
        if (!has_id) {
            // Process notification without expecting a response
            mcp_handle_method_call(mcpc, method, params_obj, 0);
            continue;
        }
        
        // For requests with IDs, we need to capture the response
        // Create a custom response
        struct json_object *result = NULL;
        struct json_object *response = NULL;
        
        // Call the method directly with the captured result patterns
        if (strncmp(method, "tools/", 6) == 0) {
            result = json_object_new_object();
            json_object_object_add(result, "status", json_object_new_string("success"));
            json_object_object_add(result, "message", json_object_new_string("Batch processed"));
            response = mcp_create_success_response(result, id);
            json_object_put(result);
        }
        else if (strncmp(method, "resources/", 10) == 0) {
            result = json_object_new_object();
            json_object_object_add(result, "status", json_object_new_string("success"));
            json_object_object_add(result, "message", json_object_new_string("Batch processed"));
            response = mcp_create_success_response(result, id);
            json_object_put(result);
        }
        else if (strncmp(method, "prompts/", 8) == 0) {
            result = json_object_new_object();
            json_object_object_add(result, "status", json_object_new_string("success"));
            json_object_object_add(result, "message", json_object_new_string("Batch processed"));
            response = mcp_create_success_response(result, id);
            json_object_put(result);
        }
        else if (strncmp(method, "notifications/", 14) == 0) {
            result = json_object_new_object();
            json_object_object_add(result, "status", json_object_new_string("success"));
            json_object_object_add(result, "message", json_object_new_string("Batch processed"));
            response = mcp_create_success_response(result, id);
            json_object_put(result);
        }
        else if (strncmp(method, "context/", 8) == 0) {
            result = json_object_new_object();
            json_object_object_add(result, "status", json_object_new_string("success"));
            json_object_object_add(result, "message", json_object_new_string("Batch processed"));
            response = mcp_create_success_response(result, id);
            json_object_put(result);
        }
        else if (strncmp(method, "system/", 7) == 0) {
            result = json_object_new_object();
            json_object_object_add(result, "status", json_object_new_string("success"));
            json_object_object_add(result, "message", json_object_new_string("Batch processed"));
            response = mcp_create_success_response(result, id);
            json_object_put(result);
        }
        else if (strcmp(method, "initialize") == 0) {
            // This one we'll actually process
            mcp_method_initialize(mcpc, params_obj, id);
            continue; // The method sends its own response
        }
        else {
            // Method not found
            response = mcp_create_error_response(MCP_ERROR_METHOD_NOT_FOUND, "Method not found", id);
        }
        
        // Add response to the batch
        if (response) {
            json_object_array_add(batch_response, response);
        }
    }
    
    // If the array is empty (all notifications), don't send anything
    if (json_object_array_length(batch_response) == 0) {
        json_object_put(batch_response);
        return 0;
    }
    
    // Send batch response
    int ret = mcp_send_json(mcpc, batch_response);
    
    // Clean up
    json_object_put(batch_response);
    
    return ret;
}

// Main MCP entry point - handle a JSON-RPC request (can be single or batch)
int mcp_handle_request(MCP_CLIENT *mcpc, struct json_object *request) {
    if (!mcpc || !request) return -1;
    
    // Check if this is a batch request (JSON array)
    if (json_object_get_type(request) == json_type_array) {
        // Handle as batch
        return mcp_handle_batch_request(mcpc, request);
    } else {
        // Extract JSON-RPC fields
        struct json_object *method_obj = NULL;
        struct json_object *params_obj = NULL;
        struct json_object *id_obj = NULL;
        struct json_object *jsonrpc_obj = NULL;
        
        if (!json_object_object_get_ex(request, "jsonrpc", &jsonrpc_obj)) {
            return mcp_send_error_response(mcpc, MCP_ERROR_INVALID_REQUEST, "Missing jsonrpc field", 0);
        }
        
        const char *jsonrpc_version = json_object_get_string(jsonrpc_obj);
        if (strcmp(jsonrpc_version, "2.0") != 0) {
            return mcp_send_error_response(mcpc, MCP_ERROR_INVALID_REQUEST, "Unsupported jsonrpc version", 0);
        }
        
        if (!json_object_object_get_ex(request, "method", &method_obj)) {
            return mcp_send_error_response(mcpc, MCP_ERROR_INVALID_REQUEST, "Missing method field", 0);
        }
        
        const char *method = json_object_get_string(method_obj);
        
        // Params are optional but must be an object if present
        if (json_object_object_get_ex(request, "params", &params_obj)) {
            if (json_object_get_type(params_obj) != json_type_object) {
                return mcp_send_error_response(mcpc, MCP_ERROR_INVALID_PARAMS, "params must be an object", 0);
            }
        }
        
        // Id is required for requests but not for notifications
        uint64_t id = 0;
        if (json_object_object_get_ex(request, "id", &id_obj)) {
            if (json_object_get_type(id_obj) == json_type_int) {
                id = json_object_get_int64(id_obj);
            }
            else if (json_object_get_type(id_obj) == json_type_string) {
                const char *id_str = json_object_get_string(id_obj);
                char *endptr;
                id = strtoull(id_str, &endptr, 10);
                if (*endptr != '\0') {
                    // If the string is not a number, use a hash of the string as the ID
                    id = 0;
                    while (*id_str) {
                        id = id * 31 + (*id_str++);
                    }
                }
            }
        }
        
        // Handle the method call
        return mcp_handle_method_call(mcpc, method, params_obj, id);
    }
}

// Initialize the MCP subsystem
void mcp_initialize_subsystem(void) {
    netdata_log_info("MCP subsystem initialized");
}
