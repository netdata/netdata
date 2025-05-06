// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

// Send a JSON object to client
int websocket_client_send_json(struct websocket_server_client *wsc, struct json_object *json) {
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

// Send multiple text messages as fragments
int websocket_client_send_text_fragmented(WS_CLIENT *wsc, const char **fragments, int count) {
    if (!wsc || !fragments || count <= 0)
        return -1;

    websocket_debug(wsc, "Sending fragmented text message with %d fragments", count);

    int total_bytes = 0;

    // Send fragments with appropriate FIN and opcode settings
    for (int i = 0; i < count; i++) {
        const char *fragment = fragments[i];
        if (!fragment)
            continue;

        size_t length = strlen(fragment);
        bool is_first = (i == 0);
        WEBSOCKET_OPCODE opcode = is_first ? WS_OPCODE_TEXT : WS_OPCODE_CONTINUATION;

        // Only compress fragments larger than minimum size
        bool compress = wsc->compression.enabled && length >= WS_COMPRESS_MIN_SIZE;

        websocket_debug(wsc, "Sending fragment %d/%d: length=%zu, opcode=%d, compress=%d",
                   i+1, count, length, opcode, compress);

        // Use the protocol send frame function directly
        int result = websocket_protocol_send_frame(wsc, fragment, length, opcode, compress);

        if (result < 0) {
            websocket_error(wsc, "Failed to send fragment %d/%d", i+1, count);
            return result;
        }

        total_bytes += result;
    }

    websocket_debug(wsc, "Completed sending fragmented text message, total bytes=%d", total_bytes);
    return total_bytes;
}

// Send a large binary payload as fragments
int websocket_payload_send_binary_fragmented(WS_CLIENT *wsc, const void *data,
                                            size_t length, size_t fragment_size) {
    if (!wsc || !data || length == 0 || fragment_size == 0)
        return -1;

    // Use reasonable default fragment size if not specified
    if (fragment_size == 0)
        fragment_size = 64 * 1024; // 64KB fragments

    // Calculate number of fragments
    int count = (length + fragment_size - 1) / fragment_size;
    int total_bytes = 0;

    websocket_debug(wsc, "Sending fragmented binary message: total_length=%zu, fragments=%d, fragment_size=%zu",
               length, count, fragment_size);

    // Send fragments
    const char *ptr = (const char *)data;
    size_t remaining = length;

    for (int i = 0; i < count; i++) {
        bool is_first = (i == 0);
        size_t this_size = (remaining < fragment_size) ? remaining : fragment_size;
        WEBSOCKET_OPCODE opcode = is_first ? WS_OPCODE_BINARY : WS_OPCODE_CONTINUATION;

        // Only compress fragments larger than minimum size
        bool compress = wsc->compression.enabled && this_size >= WS_COMPRESS_MIN_SIZE;

        websocket_debug(wsc, "Sending binary fragment %d/%d: size=%zu, opcode=%d, compress=%d",
                   i+1, count, this_size, opcode, compress);

        // Send the fragment
        int result = websocket_protocol_send_frame(wsc, ptr, this_size, opcode, compress);

        if (result < 0) {
            websocket_error(wsc, "Failed to send binary fragment %d/%d", i+1, count);
            return result;
        }

        total_bytes += result;
        ptr += this_size;
        remaining -= this_size;
    }

    websocket_debug(wsc, "Completed sending fragmented binary message, total bytes=%d", total_bytes);
    return total_bytes;
}