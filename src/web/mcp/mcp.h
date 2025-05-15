// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_MCP_H
#define NETDATA_MCP_H

#include "libnetdata/libnetdata.h"
#include <json-c/json.h>
#include "mcp-request-id.h"

// MCP protocol versions
typedef enum {
    MCP_PROTOCOL_VERSION_UNKNOWN = 0,
    MCP_PROTOCOL_VERSION_2024_11_05 = 20241105, // Using numeric date format for natural ordering
    MCP_PROTOCOL_VERSION_2025_03_26 = 20250326,
    // Add future versions here
    
    // Always keep this pointing to the latest version
    MCP_PROTOCOL_VERSION_LATEST = MCP_PROTOCOL_VERSION_2025_03_26
} MCP_PROTOCOL_VERSION;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(MCP_PROTOCOL_VERSION);

// JSON-RPC error codes (standard)
#define MCP_ERROR_PARSE_ERROR      -32700
#define MCP_ERROR_INVALID_REQUEST  -32600
#define MCP_ERROR_METHOD_NOT_FOUND -32601
#define MCP_ERROR_INVALID_PARAMS   -32602
#define MCP_ERROR_INTERNAL_ERROR   -32603
// Server error codes (implementation-defined)
#define MCP_ERROR_SERVER_ERROR_MIN -32099
#define MCP_ERROR_SERVER_ERROR_MAX -32000

// Content types (for messages and tool responses)
typedef enum {
    MCP_CONTENT_TYPE_TEXT = 0,
    MCP_CONTENT_TYPE_IMAGE = 1,
    MCP_CONTENT_TYPE_AUDIO = 2, // New in 2025-03-26
} MCP_CONTENT_TYPE;

// Forward declarations for transport-specific types
struct websocket_server_client;
struct web_client;

// Transport types for MCP
typedef enum {
    MCP_TRANSPORT_UNKNOWN = 0,
    MCP_TRANSPORT_WEBSOCKET,
    MCP_TRANSPORT_HTTP,
    // Add more as needed
} MCP_TRANSPORT;

// Transport capabilities
typedef enum {
    MCP_CAPABILITY_NONE = 0,
    MCP_CAPABILITY_ASYNC_COMMUNICATION = (1 << 0),  // Can send messages at any time
    MCP_CAPABILITY_SUBSCRIPTIONS = (1 << 1),        // Supports subscriptions
    MCP_CAPABILITY_NOTIFICATIONS = (1 << 2),        // Supports notifications
    // Add more as needed
} MCP_CAPABILITY;

// Return codes for MCP functions
typedef enum {
    MCP_RC_OK = 0,             // Success, result buffer contains valid response
    MCP_RC_ERROR = 1,          // Generic error, error buffer contains message
    MCP_RC_INVALID_PARAMS = 2, // Invalid parameters in request
    MCP_RC_NOT_FOUND = 3,      // Resource or method not found
    MCP_RC_INTERNAL_ERROR = 4, // Internal server error
    MCP_RC_NOT_IMPLEMENTED = 5 // Method not implemented
    // Can add more specific errors as needed
} MCP_RETURN_CODE;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(MCP_RETURN_CODE);

// Response handling context
typedef struct mcp_client {
    // Transport type and capabilities
    MCP_TRANSPORT transport;
    MCP_CAPABILITY capabilities;
    
    // Protocol version (detected during initialization)
    MCP_PROTOCOL_VERSION protocol_version;
    
    // Transport-specific context
    union {
        struct websocket_server_client *websocket;  // WebSocket client
        struct web_client *http;                    // HTTP client
        void *generic;                              // Generic context
    };
    
    // Client information
    STRING *client_name;                           // Client name (for logging, interned)
    STRING *client_version;                        // Client version (for logging, interned)
    
    // Response buffers
    BUFFER *result;                                // Pre-allocated buffer for success responses
    BUFFER *error;                                 // Pre-allocated buffer for error messages
    
    // Utility buffers
    BUFFER *uri;                                  // Pre-allocated buffer for URI decoding
    
    // Request IDs tracking
    size_t request_id_counter;                     // Counter for generating sequential request IDs
    Pvoid_t request_ids;                          // JudyL array for mapping internal IDs to client IDs
} MCP_CLIENT;

// Helper function to convert string version to numeric version
MCP_PROTOCOL_VERSION mcp_protocol_version_from_string(const char *version_str);

// Helper function to convert numeric version to string version
const char *mcp_protocol_version_to_string(MCP_PROTOCOL_VERSION version);

// Create a response context for a transport session
MCP_CLIENT *mcp_create_client(MCP_TRANSPORT transport, void *transport_ctx);

// Free a response context
void mcp_free_client(MCP_CLIENT *mcpc);

// Helper functions for creating and sending JSON-RPC responses

// Functions to initialize and build MCP responses
void mcp_init_success_result(MCP_CLIENT *mcpc, MCP_REQUEST_ID id);
MCP_RETURN_CODE mcp_error_result(MCP_CLIENT *mcpc, MCP_REQUEST_ID id, MCP_RETURN_CODE rc);

// Send prepared buffer content as response
int mcp_send_response_buffer(MCP_CLIENT *mcpc);

// Check if a capability is supported by the transport
static inline bool mcp_has_capability(MCP_CLIENT *mcpc, MCP_CAPABILITY capability) {
    return mcpc && (mcpc->capabilities & capability);
}

// Initialize the MCP subsystem
void mcp_initialize_subsystem(void);

// Main MCP entry point - handle a JSON-RPC request (single or batch)
MCP_RETURN_CODE mcp_handle_request(MCP_CLIENT *mcpc, struct json_object *request);

const char *mcp_uri_decode(MCP_CLIENT *mcpc, const char *src);

#endif // NETDATA_MCP_H
