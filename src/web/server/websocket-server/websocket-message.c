// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

// Helper function to determine if an opcode is a control opcode
bool websocket_frame_is_control_opcode(WEBSOCKET_OPCODE opcode) {
    return (opcode == WS_OPCODE_CLOSE ||
            opcode == WS_OPCODE_PING ||
            opcode == WS_OPCODE_PONG);
}

// Validates that a buffer contains valid UTF-8 encoded data
// Returns true if the data is valid UTF-8, false otherwise
bool websocket_validate_utf8(const char *data, size_t length) {
    if (!data)
        return length == 0; // Empty data is valid

    const unsigned char *bytes = (const unsigned char *)data;
    size_t i = 0;

    while (i < length) {
        // Check for ASCII (single-byte character)
        if (bytes[i] <= 0x7F) {
            i++;
            continue;
        }

        // Check for 2-byte sequence
        else if ((bytes[i] & 0xE0) == 0xC0) {
            // Need at least 2 bytes
            if (i + 1 >= length)
                return false;

            // Second byte must be a continuation byte
            if ((bytes[i+1] & 0xC0) != 0x80)
                return false;

            // Must not be overlong encoding
            if (bytes[i] < 0xC2)
                return false;

            i += 2;
        }

        // Check for 3-byte sequence
        else if ((bytes[i] & 0xF0) == 0xE0) {
            // Need at least 3 bytes
            if (i + 2 >= length)
                return false;

            // Second and third bytes must be continuation bytes
            if ((bytes[i+1] & 0xC0) != 0x80 || (bytes[i+2] & 0xC0) != 0x80)
                return false;

            // Check for overlong encoding
            if (bytes[i] == 0xE0 && (bytes[i+1] & 0xE0) == 0x80)
                return false;

            // Check for UTF-16 surrogates (not allowed in UTF-8)
            if (bytes[i] == 0xED && (bytes[i+1] & 0xE0) == 0xA0)
                return false;

            i += 3;
        }

        // Check for 4-byte sequence
        else if ((bytes[i] & 0xF8) == 0xF0) {
            // Need at least 4 bytes
            if (i + 3 >= length)
                return false;

            // Second, third, and fourth bytes must be continuation bytes
            if ((bytes[i+1] & 0xC0) != 0x80 ||
                (bytes[i+2] & 0xC0) != 0x80 ||
                (bytes[i+3] & 0xC0) != 0x80)
                return false;

            // Check for overlong encoding
            if (bytes[i] == 0xF0 && (bytes[i+1] & 0xF0) == 0x80)
                return false;

            // Check for values outside Unicode range
            if (bytes[i] > 0xF4 || (bytes[i] == 0xF4 && bytes[i+1] > 0x8F))
                return false;

            i += 4;
        }

        // Invalid UTF-8 leading byte
        else {
            return false;
        }
    }

    return true;
}

// Reset a client's message state for a new message
void websocket_client_message_reset(WS_CLIENT *wsc) {
    if (!wsc)
        return;

    // Reset message buffer
    wsb_reset(&wsc->payload);

    // Also reset uncompressed buffer to avoid keeping stale data
    wsb_reset(&wsc->u_payload);

    // Reset client's message state
    // We set message_complete to true by default (no fragmented message in progress),
    // but this will be overridden based on the FIN bit for actual frames
    wsc->message_complete = true;
    wsc->is_compressed = false;
    wsc->opcode = WS_OPCODE_TEXT; // Default opcode
    wsc->frame_id = 0;
}

// Process a complete message (decompress if needed and call handler)
bool websocket_client_process_message(WS_CLIENT *wsc) {
    if (!wsc || !wsc->message_complete)
        return false;

    worker_is_busy(WORKERS_WEBSOCKET_MESSAGE);

    websocket_debug(wsc, "Processing message (opcode=0x%x, is_compressed=%d, length=%zu)",
               wsc->opcode, wsc->is_compressed,
        wsb_length(&wsc->payload));

    // Handle control frames immediately
    if (wsc->opcode != WS_OPCODE_TEXT && wsc->opcode != WS_OPCODE_BINARY) {
        websocket_debug(wsc, "Control frame (opcode=0x%x) should not be handled by %s()", wsc->opcode, __FUNCTION__);
        return false;
    }

    // At this point, we know we're dealing with a data frame (text or binary)
    WS_BUF *wsb;

    // Handle decompression if needed
    if (wsc->is_compressed) {
        if (!websocket_client_decompress_message(wsc)) {
            websocket_protocol_exception(wsc, WS_CLOSE_INTERNAL_ERROR, "Decompression failed");
            return false;
        }
        wsb = &wsc->u_payload;
    }
    else
        wsb = &wsc->payload;

   // For uncompressed messages, we just use payload buffer directly
   if (wsc->opcode == WS_OPCODE_TEXT) {
       wsb_null_terminate(wsb);

       if (!websocket_validate_utf8(wsb_data(wsb), wsb_length(wsb))) {
           websocket_protocol_exception(wsc, WS_CLOSE_INVALID_PAYLOAD,
                                        "Invalid UTF-8 data in text message");
           return false;
       }
   }

    // Now handle the uncompressed message - using the new function
    // that contains the actual handler logic

   websocket_debug(wsc, "Handling message: type=%s, length=%zu, protocol=%d",
                   (wsc->opcode == WS_OPCODE_BINARY) ? "binary" : "text",
                   wsb_length(wsb), wsc->protocol);

   // Ensure text messages are null-terminated
   if (wsc->opcode == WS_OPCODE_TEXT)
       wsb_null_terminate(wsb);

   // Call the message callback if set - this allows protocols to be handled dynamically
   if (wsc->on_message) {
       websocket_debug(wsc, "Calling client message handler for protocol %d", wsc->protocol);
       wsc->on_message(wsc, wsb_data(wsb), wsb_length(wsb), wsc->opcode);
   }
   else {
       // No handler registered - this should not happen as we check during handshake
       websocket_error(wsc, "No message handler registered for protocol %d", wsc->protocol);
       return false;
   }

    // Update client message stats
    wsc->message_id++;
    wsc->frame_id = 0;

    // Reset for the next message
    websocket_client_message_reset(wsc);

    return true;
}
