// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBSOCKET_JSONRPC_H
#define NETDATA_WEBSOCKET_JSONRPC_H

#include "websocket-internal.h"

// JSON-RPC 2.0 protocol constants
#define JSONRPC_VERSION "2.0"

// JSON-RPC error codes as per specification
typedef enum {
    // Official JSON-RPC 2.0 error codes
    JSONRPC_ERROR_PARSE_ERROR = -32700,      // Invalid JSON was received by the server
    JSONRPC_ERROR_INVALID_REQUEST = -32600,  // The JSON sent is not a valid Request object
    JSONRPC_ERROR_METHOD_NOT_FOUND = -32601, // The method does not exist / is not available
    JSONRPC_ERROR_INVALID_PARAMS = -32602,   // Invalid method parameter(s)
    JSONRPC_ERROR_INTERNAL_ERROR = -32603,   // Internal JSON-RPC error

    // -32000 to -32099 are reserved for implementation-defined server-errors
    JSONRPC_ERROR_SERVER_ERROR = -32000,     // Generic server error
    
    // Netdata specific error codes (using reserved server-error range)
    JSONRPC_ERROR_NETDATA_PERMISSION_DENIED = -32030, // Permission denied
    JSONRPC_ERROR_NETDATA_NOT_SUPPORTED = -32031,     // Feature not supported
    JSONRPC_ERROR_NETDATA_RATE_LIMIT = -32032,        // Rate limit exceeded
} JSONRPC_ERROR_CODE;

// Method handler function type
typedef void (*jsonrpc_method_handler)(WS_CLIENT *wsc, struct json_object *request, uint64_t id);

// Initialize WebSocket JSON-RPC protocol
void websocket_jsonrpc_initialize(void);

// Process a WebSocket message as JSON-RPC
bool websocket_jsonrpc_process_message(WS_CLIENT *wsc, const char *message, size_t length);

// WebSocket protocol handler callbacks for JSON-RPC
void jsonrpc_on_connect(struct websocket_server_client *wsc);
void jsonrpc_on_message_callback(struct websocket_server_client *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode);
void jsonrpc_on_close(struct websocket_server_client *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason);
void jsonrpc_on_disconnect(struct websocket_server_client *wsc);

// Response functions
void websocket_jsonrpc_response_result(WS_CLIENT *wsc, struct json_object *result, uint64_t id);
void websocket_jsonrpc_response_error(WS_CLIENT *wsc, JSONRPC_ERROR_CODE code, const char *message, uint64_t id);
void websocket_jsonrpc_response_error_with_data(WS_CLIENT *wsc, JSONRPC_ERROR_CODE code, const char *message, 
                                               struct json_object *data, uint64_t id);

// Helper functions
bool websocket_jsonrpc_validate_request(struct json_object *request);

// Utility function to extract parameters from a request
struct json_object *websocket_jsonrpc_get_params(struct json_object *request);

#endif // NETDATA_WEBSOCKET_JSONRPC_H
