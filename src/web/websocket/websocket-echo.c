// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-echo.h"

// Called when a client is connected and ready to exchange messages
void echo_on_connect(struct websocket_server_client *wsc) {
    if (!wsc) return;
    
    websocket_debug(wsc, "Echo protocol client connected");
    
    // Send a welcome message
    // websocket_protocol_send_text(wsc, "Welcome to Netdata Echo WebSocket Server");
}

// Called when a message is received from the client
void echo_on_message_callback(struct websocket_server_client *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode) {
    if (!wsc || !message)
        return;

    websocket_debug(wsc, "Echo protocol handling message: type=%s, length=%zu",
                    (opcode == WS_OPCODE_BINARY) ? "binary" : "text",
                    length);

    // Simply echo back the same message with the same opcode
    websocket_protocol_send_frame(wsc, message, length, opcode, true);
}

// Called before sending a close frame to the client
void echo_on_close(struct websocket_server_client *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason) {
    if (!wsc) return;

    websocket_debug(wsc, "Echo protocol client closing with code %d (%s): %s",
                    code,
                    code == WS_CLOSE_NORMAL ? "Normal" :
                    code == WS_CLOSE_GOING_AWAY ? "Going Away" :
                    code == WS_CLOSE_PROTOCOL_ERROR ? "Protocol Error" :
                    code == WS_CLOSE_INTERNAL_ERROR ? "Internal Error" : "Other",
                    reason ? reason : "No reason provided");
    
    // Optional: Send a goodbye message
    // websocket_protocol_send_text(wsc, "Goodbye from Netdata Echo WebSocket Server");
}

// Called when a client is about to be disconnected
void echo_on_disconnect(struct websocket_server_client *wsc) {
    if (!wsc) return;
    
    websocket_debug(wsc, "Echo protocol client disconnected");
    
    // No cleanup needed for the Echo protocol since it doesn't maintain any state
}

// Initialize the Echo protocol
void websocket_echo_initialize(void) {
    netdata_log_info("Echo protocol initialized");
}