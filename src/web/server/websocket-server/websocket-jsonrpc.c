// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-jsonrpc.h"

#include "libnetdata/json/json-c-parser-inline.h"

static int websocket_client_send_json(struct websocket_server_client *wsc, struct json_object *json) {
    if (!wsc || !json)
        return -1;

    websocket_debug(wsc, "Sending JSON message");

    // Convert JSON to string
    const char *json_str = json_object_to_json_string_ext(json, JSON_C_TO_STRING_PLAIN);
    if (!json_str) {
        websocket_error(wsc, "Failed to convert JSON to string");
        return -1;
    }

    // Send as text message
    int result = websocket_protocol_send_text(wsc, json_str);

    websocket_debug(wsc, "Sent JSON message, result=%d", result);
    return result;
}

// Called when a client is connected and ready to exchange messages
void jsonrpc_on_connect(struct websocket_server_client *wsc) {
    if (!wsc) return;

    websocket_debug(wsc, "JSON-RPC client connected");

    // Future implementation - can send a welcome message or initialize client state
}

// Called when a client is about to be disconnected
void jsonrpc_on_disconnect(struct websocket_server_client *wsc) {
    if (!wsc) return;

    websocket_debug(wsc, "JSON-RPC client disconnected");

    // Future implementation - can clean up any client-specific resources
}

// Called before sending a close frame to the client
void jsonrpc_on_close(struct websocket_server_client *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason) {
    if (!wsc) return;

    websocket_debug(wsc, "JSON-RPC client closing with code %d (%s): %s",
                   code,
                   code == WS_CLOSE_NORMAL ? "Normal" :
                   code == WS_CLOSE_GOING_AWAY ? "Going Away" :
                   code == WS_CLOSE_PROTOCOL_ERROR ? "Protocol Error" :
                   code == WS_CLOSE_INTERNAL_ERROR ? "Internal Error" : "Other",
                   reason ? reason : "No reason provided");

    // Future implementation - can send a final message before closing
}

// Adapter function for the on_message callback to match WS_CLIENT callback signature
void jsonrpc_on_message_callback(struct websocket_server_client *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode) {
    if (!wsc || !message || length == 0)
        return;

    // JSON-RPC only works with text messages
    if (opcode != WS_OPCODE_TEXT) {
        websocket_error(wsc, "JSON-RPC protocol received non-text message, ignoring");
        return;
    }

    websocket_debug(wsc, "JSON-RPC callback processing message: length=%zu", length);

    // Process the message
    websocket_jsonrpc_process_message(wsc, message, length);
}

// Utility function to extract parameters from a request
struct json_object *websocket_jsonrpc_get_params(struct json_object *request) {
    if (!request)
        return NULL;

    struct json_object *params = NULL;
    if (json_object_object_get_ex(request, "params", &params)) {
        return params;  // Return the params object if it exists
    }

    return NULL;  // No params found
}

// Handler for the "echo" method - simply returns the params as the result
static void jsonrpc_echo_handler(WS_CLIENT *wsc, struct json_object *request, uint64_t id) {
    // Get the params if available
    struct json_object *params = websocket_jsonrpc_get_params(request);

    // Clone the params to avoid ownership issues
    struct json_object *result = params ? json_object_get(params) : json_object_new_object();

    // Send response
    websocket_jsonrpc_response_result(wsc, result, id);
}

// Define a fixed array of method handlers
static struct {
    const char *method;
    jsonrpc_method_handler handler;
} jsonrpc_methods[] = {
    { "echo", jsonrpc_echo_handler },

    // Add more methods here as needed
    // { "method_name", method_handler_function },

    // Terminator
    { NULL, NULL }
};

// Initialize the JSON-RPC protocol
void websocket_jsonrpc_initialize(void) {
    netdata_log_info("JSON-RPC protocol initialized with built-in methods");
}

// Find a method handler
static jsonrpc_method_handler find_method_handler(const char *method) {
    if (!method)
        return NULL;

    // Simple linear search through the fixed array
    for (int i = 0; jsonrpc_methods[i].method != NULL; i++) {
        if (strcmp(jsonrpc_methods[i].method, method) == 0) {
            return jsonrpc_methods[i].handler;
        }
    }

    return NULL;
}

// Validate JSON-RPC request according to specification
bool websocket_jsonrpc_validate_request(struct json_object *request) {
    if (!request || json_object_get_type(request) != json_type_object)
        return false;
    
    // Check for required fields
    struct json_object *jsonrpc, *method;
    
    if (!json_object_object_get_ex(request, "jsonrpc", &jsonrpc) ||
        !json_object_object_get_ex(request, "method", &method))
        return false;
    
    // Validate jsonrpc version
    if (json_object_get_type(jsonrpc) != json_type_string ||
        strcmp(json_object_get_string(jsonrpc), JSONRPC_VERSION) != 0)
        return false;
    
    // Validate method
    if (json_object_get_type(method) != json_type_string)
        return false;
    
    return true;
}

// Process a JSON-RPC request
static void process_jsonrpc_request(WS_CLIENT *wsc, struct json_object *request) {
    if (!websocket_jsonrpc_validate_request(request)) {
        websocket_jsonrpc_response_error(wsc, JSONRPC_ERROR_INVALID_REQUEST, 
                                        "Invalid JSON-RPC request", 0);
        return;
    }
    
    // Extract request components
    struct json_object *method_obj, *id_obj = NULL;
    
    json_object_object_get_ex(request, "method", &method_obj);
    const char *method = json_object_get_string(method_obj);
    
    // Get ID if present (0 indicates a notification that requires no response)
    uint64_t id = 0;
    bool has_id = json_object_object_get_ex(request, "id", &id_obj);
    if (has_id && id_obj && json_object_get_type(id_obj) != json_type_null) {
        if (json_object_get_type(id_obj) == json_type_int)
            id = json_object_get_int64(id_obj);
        else if (json_object_get_type(id_obj) == json_type_string) {
            // Try to convert string ID to integer if possible
            const char *id_str = json_object_get_string(id_obj);
            char *endptr;
            id = (uint64_t)strtoll(id_str, &endptr, 10);
            if (*endptr != '\0') {
                // Not a number, just hash the string for an ID
                id = simple_hash(id_str);
            }
        }
    }
    
    // Find handler for the requested method
    jsonrpc_method_handler handler = find_method_handler(method);
    if (!handler) {
        if (has_id) {
            websocket_jsonrpc_response_error(wsc, JSONRPC_ERROR_METHOD_NOT_FOUND, 
                                           "Method not found", id);
        }
        return;
    }
    
    // Call the handler with the request
    handler(wsc, request, id);
}

// Process a WebSocket message as JSON-RPC
bool websocket_jsonrpc_process_message(WS_CLIENT *wsc, const char *message, size_t length) {
    if (!wsc || !message || length == 0)
        return false;

    websocket_debug(wsc, "Processing JSON-RPC message: length=%zu", length);

    // Parse the JSON
    struct json_object *json = json_tokener_parse(message);
    if (!json) {
        websocket_error(wsc, "Failed to parse JSON-RPC message");
        websocket_jsonrpc_response_error(wsc, JSONRPC_ERROR_PARSE_ERROR,
                                       "Parse error", 0);
        return false;
    }

    // Process based on message type
    if (json_object_get_type(json) == json_type_array) {
        // Batch request
        websocket_debug(wsc, "Processing JSON-RPC batch request");

        int array_len = json_object_array_length(json);
        for (int i = 0; i < array_len; i++) {
            struct json_object *request = json_object_array_get_idx(json, i);
            process_jsonrpc_request(wsc, request);
        }
    }
    else if (json_object_get_type(json) == json_type_object) {
        // Single request
        process_jsonrpc_request(wsc, json);
    }
    else {
        // Invalid request
        websocket_jsonrpc_response_error(wsc, JSONRPC_ERROR_INVALID_REQUEST,
                                      "Invalid request", 0);
        json_object_put(json);
        return false;
    }

    json_object_put(json);
    return true;
}

// Create and send a JSON-RPC success response
void websocket_jsonrpc_response_result(WS_CLIENT *wsc, struct json_object *result, uint64_t id) {
    if (!wsc || id == 0) // No response for notifications (id == 0)
        return;
    
    struct json_object *response = json_object_new_object();
    
    // Add required fields
    json_object_object_add(response, "jsonrpc", json_object_new_string(JSONRPC_VERSION));
    
    // Add result (takes ownership of the result object)
    if (result) {
        json_object_object_add(response, "result", result);
    } else {
        json_object_object_add(response, "result", json_object_new_object());
    }
    
    // Add ID
    json_object_object_add(response, "id", json_object_new_int64(id));
    
    // Send the response
    websocket_client_send_json(wsc, response);
    
    // Free the response object
    json_object_put(response);
}

// Create and send a JSON-RPC error response
void websocket_jsonrpc_response_error(WS_CLIENT *wsc, JSONRPC_ERROR_CODE code, const char *message, uint64_t id) {
    websocket_jsonrpc_response_error_with_data(wsc, code, message, NULL, id);
}

// Create and send a JSON-RPC error response with additional data
void websocket_jsonrpc_response_error_with_data(WS_CLIENT *wsc, JSONRPC_ERROR_CODE code, const char *message, 
                                              struct json_object *data, uint64_t id) {
    if (!wsc || id == 0) // No response for notifications (id == 0)
        return;
    
    struct json_object *response = json_object_new_object();
    struct json_object *error = json_object_new_object();
    
    // Add required fields
    json_object_object_add(response, "jsonrpc", json_object_new_string(JSONRPC_VERSION));
    
    // Add error code and message
    json_object_object_add(error, "code", json_object_new_int(code));
    json_object_object_add(error, "message", json_object_new_string(message ? message : "Unknown error"));
    
    // Add error data if provided
    if (data) {
        json_object_object_add(error, "data", data);
    }
    
    // Add error object to response
    json_object_object_add(response, "error", error);
    
    // Add ID
    json_object_object_add(response, "id", json_object_new_int64(id));
    
    // Send the response
    websocket_client_send_json(wsc, response);
    
    // Free the response object
    json_object_put(response);
}
