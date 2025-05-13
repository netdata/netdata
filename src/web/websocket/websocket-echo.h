// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBSOCKET_ECHO_H
#define NETDATA_WEBSOCKET_ECHO_H

#include "websocket-internal.h"

// WebSocket protocol handler callbacks for Echo protocol
void echo_on_connect(struct websocket_server_client *wsc);
void echo_on_message_callback(struct websocket_server_client *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode);
void echo_on_close(struct websocket_server_client *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason);
void echo_on_disconnect(struct websocket_server_client *wsc);

// Initialize Echo protocol - called during WebSocket subsystem initialization
void websocket_echo_initialize(void);

#endif // NETDATA_WEBSOCKET_ECHO_H