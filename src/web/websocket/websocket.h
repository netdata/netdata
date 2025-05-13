// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_SERVER_WEBSOCKET_H
#define NETDATA_WEB_SERVER_WEBSOCKET_H 1

#include "libnetdata/libnetdata.h"

// WebSocket subprotocols supported by Netdata
typedef enum __attribute__((packed)) {
    WS_PROTOCOL_DEFAULT = 0,                            // the protocol is selected from the url
    WS_PROTOCOL_UNKNOWN,                                // Unknown or unsupported protocol
    WS_PROTOCOL_JSONRPC,                                // JSON-RPC protocol
    WS_PROTOCOL_ECHO,                                   // Echo protocol
    WS_PROTOCOL_MCP,                                    // Model Context Protocol
} WEBSOCKET_PROTOCOL;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(WEBSOCKET_PROTOCOL);

// WebSocket extensions supported by Netdata
typedef enum __attribute__((packed)) {
    // RFC 7692
    WS_EXTENSION_NONE                       = 0,        // No extensions
    WS_EXTENSION_PERMESSAGE_DEFLATE         = (1 << 0), // permessage-deflate
    WS_EXTENSION_CLIENT_NO_CONTEXT_TAKEOVER = (1 << 1), // client_no_context_takeover
    WS_EXTENSION_SERVER_NO_CONTEXT_TAKEOVER = (1 << 2), // server_no_context_takeover
    WS_EXTENSION_SERVER_MAX_WINDOW_BITS     = (1 << 3), // server_max_window_bits
    WS_EXTENSION_CLIENT_MAX_WINDOW_BITS     = (1 << 4)  // client_max_window_bits
} WEBSOCKET_EXTENSION;

// Forward declarations
struct web_client;
struct websocket_server_client;

// WebSocket connection state
typedef enum __attribute__((packed)) {
    WS_STATE_HANDSHAKE         = 0, // Initial handshake in progress
    WS_STATE_OPEN              = 1, // Connection established
    WS_STATE_CLOSING_SERVER    = 2, // Server initiated closing handshake
    WS_STATE_CLOSING_CLIENT    = 3, // Client initiated closing handshake
    WS_STATE_CLOSED            = 4  // Connection closed
} WEBSOCKET_STATE;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(WEBSOCKET_STATE);

// WebSocket message types (opcodes) as per RFC 6455
typedef enum __attribute__((packed)) {
    WS_OPCODE_CONTINUATION = 0x0,
    WS_OPCODE_TEXT         = 0x1,
    WS_OPCODE_BINARY       = 0x2,
    WS_OPCODE_CLOSE        = 0x8,
    WS_OPCODE_PING         = 0x9,
    WS_OPCODE_PONG         = 0xA
} WEBSOCKET_OPCODE;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(WEBSOCKET_OPCODE);

// WebSocket close codes as per RFC 6455
typedef enum __attribute__((packed)) {
    // Standard WebSocket close codes
    WS_CLOSE_NORMAL            = 1000, // Normal closure, meaning the purpose for which the connection was established has been fulfilled
    WS_CLOSE_GOING_AWAY        = 1001, // Server/client going away (such as server shutdown or browser navigating away)
    WS_CLOSE_PROTOCOL_ERROR    = 1002, // Protocol error
    WS_CLOSE_UNSUPPORTED_DATA  = 1003, // Client received data it couldn't accept (e.g., server sent binary data when client only supports text)
    WS_CLOSE_RESERVED          = 1004, // Reserved. Specific meaning might be defined in the future.
    WS_CLOSE_NO_STATUS         = 1005, // No status code was provided even though one was expected
    WS_CLOSE_ABNORMAL          = 1006, // Connection closed abnormally (no close frame received)
    WS_CLOSE_INVALID_PAYLOAD   = 1007, // Frame payload data is invalid (e.g., non-UTF-8 data in a text frame)
    WS_CLOSE_POLICY_VIOLATION  = 1008, // Generic message received that violates policy
    WS_CLOSE_MESSAGE_TOO_BIG   = 1009, // Message too big to process
    WS_CLOSE_EXTENSION_MISSING = 1010, // Client expected the server to negotiate one or more extensions, but server didn't
    WS_CLOSE_INTERNAL_ERROR    = 1011, // Server encountered an unexpected condition preventing it from fulfilling the request
    WS_CLOSE_TLS_HANDSHAKE     = 1015, // Transport Layer Security (TLS) handshake failure

    // Netdata-specific close codes (4000-4999 range is available for private use)
    WS_CLOSE_NETDATA_TIMEOUT   = 4000, // Client timed out due to inactivity
    WS_CLOSE_NETDATA_SHUTDOWN  = 4001, // Server is shutting down
    WS_CLOSE_NETDATA_REJECTED  = 4002, // Connection rejected by server
    WS_CLOSE_NETDATA_RATE_LIMIT= 4003  // Client exceeded rate limit
} WEBSOCKET_CLOSE_CODE;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(WEBSOCKET_CLOSE_CODE);

/**
 * WebSocket Protocol Handler Callbacks
 *
 * These callbacks are invoked when specific events occur during the WebSocket lifecycle:
 *
 * - on_connect: Called when a client successfully connects and is ready to exchange messages.
 *   This happens after the WebSocket handshake is complete and the client is added to a thread.
 *   Use this callback to welcome the client or initialize any protocol-specific state.
 *
 * - on_message: Called when a complete message is received from the client.
 *   This is where the protocol processes incoming messages from clients.
 *
 * - on_close: Called BEFORE sending a close frame to the client.
 *   This gives the protocol a chance to inject a final message before the connection closes.
 *
 * - on_disconnect: Called when a client is about to be disconnected.
 *   Use this callback to clean up any protocol-specific state for the client.
 */

// Public WebSocket API functions

// WebSocket detection and handshake
short int websocket_handle_handshake(struct web_client *w);

// Initialize the WebSocket subsystem
void websocket_initialize(void);

#endif // NETDATA_WEB_SERVER_WEBSOCKET_H
