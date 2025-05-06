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
    WS_EXTENSION_NONE      = 0,      // No extensions
    WS_EXTENSION_PERMESSAGE_DEFLATE = 1 << 0,  // permessage-deflate (RFC 7692)
    // Additional extensions can be added here in the future
} WEBSOCKET_EXTENSION;


// Forward declarations for websocket client
typedef struct websocket_server_client WS_CLIENT;

// Public WebSocket API functions
struct web_client;

// WebSocket detection and handshake
bool websocket_detect_handshake_request(struct web_client *w);
short int websocket_handle_handshake(struct web_client *w);

// WebSocket socket takeover
void websocket_takeover_web_connection(struct web_client *w, WS_CLIENT *wsc);

// Initialize the WebSocket subsystem
void websocket_initialize(void);

#endif // NETDATA_WEB_SERVER_WEBSOCKET_H