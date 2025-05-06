// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_SERVER_WEBSOCKET_H
#define NETDATA_WEB_SERVER_WEBSOCKET_H 1

#include "libnetdata/libnetdata.h"

// WebSocket subprotocols supported by Netdata
typedef enum __attribute__((packed)) {
    WS_PROTOCOL_UNKNOWN    = 0,     // Unknown or unsupported protocol
    WS_PROTOCOL_NETDATA_JSON = 1,   // Basic JSON protocol
    // Additional protocols will be added here in the future
} WEBSOCKET_PROTOCOL;

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


// Public WebSocket API functions
struct web_client;

// WebSocket detection and handshake
short int websocket_handle_handshake(struct web_client *w);

// Initialize the WebSocket subsystem
void websocket_initialize(void);

#endif // NETDATA_WEB_SERVER_WEBSOCKET_H
