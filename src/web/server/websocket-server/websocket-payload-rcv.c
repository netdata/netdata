// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

#include "libnetdata/json/json-c-parser-inline.h"

// Echo a client's uncompressed message back to the client - useful for testing
int websocket_payload_echo(WS_CLIENT *wsc, WS_BUF *wsb) {
    if (!wsc || !wsb)
        return -1;

    websocket_debug(wsc, "Echoing payload: type=%s, length=%zu",
                    (wsc->opcode == WS_OPCODE_BINARY) ? "binary" : "text",
                    wsb_length(wsb));

    int result;

    // Handle empty message case properly
    if (wsb_is_empty(wsb)) {
        websocket_debug(wsc, "Echoing empty %s message",
                       (wsc->opcode == WS_OPCODE_BINARY) ? "binary" : "text");

        result = websocket_protocol_send_frame(wsc, NULL, 0, wsc->opcode, true);
    }
    else {
        // Ensure the buffer is null-terminated for text messages
        wsb_append_padding(wsb, "", 1);

        result = websocket_protocol_send_frame(wsc, wsb_data(wsb), wsb_length(wsb), wsc->opcode, true);
    }

    websocket_debug(wsc, "Echo response result: %d", result);

    if (result < 0) {
        websocket_error(wsc, "Failed to echo payload");
        return result;
    }

    return result;
}

// Parse JSON from the client's uncompressed message
struct json_object *websocket_client_parse_json(WS_CLIENT *wsc) {
    if (!wsc || !wsb_has_data(&wsc->u_payload))
        return NULL;

    // Make sure it's a text message
    if (wsc->opcode != WS_OPCODE_TEXT) {
        websocket_error(wsc, "Attempted to parse binary data as JSON");
        return NULL;
    }

    // Ensure the text data is null-terminated
    wsb_null_terminate(&wsc->u_payload);

    // Parse JSON using json-c
    struct json_object *json = json_tokener_parse(wsb_data(&wsc->u_payload));
    if (!json) {
        websocket_error(wsc, "Failed to parse JSON message: %s", wsb_data(&wsc->u_payload));
        return NULL;
    }

    return json;
}

// Send an error response back to the client
void websocket_payload_error(WS_CLIENT *wsc, const char *error_message) {
    if (!wsc || !error_message)
        return;

    websocket_error(wsc, "Sending error response: %s", error_message);

    // Create a JSON error response using json-c
    struct json_object *error_obj = json_object_new_object();

    // Add an error object with message
    json_object_object_add(error_obj, "error", json_object_new_string(error_message));

    // Add status
    json_object_object_add(error_obj, "status", json_object_new_string("error"));

    // Send JSON to client
    int result = websocket_client_send_json(wsc, error_obj);

    if (result < 0) {
        websocket_error(wsc, "Failed to send error response");
    }

    // Free JSON object
    json_object_put(error_obj);
}


// Handle a client's message based on protocol
bool websocket_payload_handle_message(WS_CLIENT *wsc, WS_BUF *wsb) {
    if (!wsc)
        return false;

    websocket_debug(wsc, "Handling message: type=%s, length=%zu, strlen=%zu",
                    (wsc->opcode == WS_OPCODE_BINARY) ? "binary" : "text",
                    wsb_length(wsb));

    // For now, we just echo back any message
    // This can be enhanced to handle specific operations based on the protocol

    // Call the message callback if set
    if (wsc->on_message) {
        websocket_debug(wsc, "Calling client message handler");
        wsc->on_message(wsc, wsb_data(wsb), wsb_length(wsb), wsc->opcode);
    }
    else {
        // Echo the message
        int result = websocket_payload_echo(wsc, wsb);
        if (result <= 0) {
            websocket_error(wsc, "Failed to echo payload");
            return false;
        }
    }

    return true;
}
