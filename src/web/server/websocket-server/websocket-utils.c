// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

// Debug log function with client, message, and frame IDs
void websocket_debug(WS_CLIENT *wsc __maybe_unused, const char *format __maybe_unused, ...) {
#ifdef NETDATA_INTERNAL_CHECKS
    if (!wsc || !format)
        return;
        
    char formatted_message[1024];
    va_list args;
    va_start(args, format);
    vsnprintf(formatted_message, sizeof(formatted_message), format, args);
    va_end(args);
    
    // Format the debug message with client, message, and frame IDs
    netdata_log_debug(D_WEBSOCKET, "WEBSOCKET: C=%zu M=%zu F=%zu %s",
                  wsc->id, wsc->message_id, wsc->frame_id, formatted_message);
#endif /* NETDATA_INTERNAL_CHECKS */
}

// Info log function with client, message, and frame IDs
void websocket_info(WS_CLIENT *wsc, const char *format, ...) {
    if (!wsc || !format)
        return;
        
    char formatted_message[1024];
    va_list args;
    va_start(args, format);
    vsnprintf(formatted_message, sizeof(formatted_message), format, args);
    va_end(args);
    
    // Format the info message with client, message, and frame IDs
    netdata_log_info("WEBSOCKET: C=%zu M=%zu F=%zu %s",
                 wsc->id, wsc->message_id, wsc->frame_id, formatted_message);
}

// Error log function with client, message, and frame IDs
void websocket_error(WS_CLIENT *wsc, const char *format, ...) {
    if (!wsc || !format)
        return;

    char formatted_message[1024];
    va_list args;
    va_start(args, format);
    vsnprintf(formatted_message, sizeof(formatted_message), format, args);
    va_end(args);

    // Format the error message with client, message, and frame IDs
    netdata_log_error("WEBSOCKET: C=%zu M=%zu F=%zu %s",
                  wsc->id, wsc->message_id, wsc->frame_id, formatted_message);
}

// Debug function that logs a message and dumps payload data for debugging
void websocket_dump_debug(WS_CLIENT *wsc __maybe_unused, const char *payload __maybe_unused,
    size_t payload_length __maybe_unused, const char *format __maybe_unused, ...) {
#ifdef NETDATA_INTERNAL_CHECKS
    if (!wsc || !format)
        return;

    // Format the primary message
    char formatted_message[1024];
    va_list args;
    va_start(args, format);
    vsnprintf(formatted_message, sizeof(formatted_message), format, args);
    va_end(args);

    // Log the primary message
    netdata_log_debug(D_WEBSOCKET, "WEBSOCKET: C=%zu M=%zu F=%zu %s",
                  wsc->id, wsc->message_id, wsc->frame_id, formatted_message);

    // Handle empty payloads explicitly (log message but no hex dump)
    if (payload_length == 0) {
        netdata_log_debug(D_WEBSOCKET, "WEBSOCKET: C=%zu M=%zu F=%zu %s (EMPTY PAYLOAD - 0 bytes)",
                      wsc->id, wsc->message_id, wsc->frame_id, formatted_message);
        return;
    }

    // If payload is provided and not empty, create and log a hex dump
    if (payload && payload_length > 0) {
        char hex_dump[256] = "";
        char ascii_dump[65] = "";
        size_t dump_length = 0;

        // Determine how many bytes to dump (max 32 bytes)
        size_t bytes_to_dump = (payload_length < 32) ? payload_length : 32;

        // Payload check is redundant as we already have it in the outer if

        // Create the hex dump
        for (size_t i = 0; i < bytes_to_dump && dump_length < sizeof(hex_dump) - 4; i++) {
            // Extra bounds check before accessing the payload
            if (i >= payload_length)
                break;

            int written = snprintf(hex_dump + dump_length,
                               sizeof(hex_dump) - dump_length,
                               "%02x ", (unsigned char)payload[i]);

            if (written > 0)
                dump_length += written;
            else
                break; // Error in snprintf

            // Also create ASCII representation for printable chars
            if (i < 64) {
                unsigned char c = (unsigned char)payload[i];
                ascii_dump[i] = (isprint(c) ? c : '.');
            }
        }

        // Null-terminate the ASCII dump
        if (bytes_to_dump < 64) {
            ascii_dump[bytes_to_dump] = '\0';
        } else {
            ascii_dump[64] = '\0';
        }

        // Log the hex dump
        netdata_log_debug(D_WEBSOCKET, "WEBSOCKET: C=%zu M=%zu F=%zu PAYLOAD HEX(%zu/%zu): [%s]%s",
                      wsc->id, wsc->message_id, wsc->frame_id,
                      bytes_to_dump, payload_length,
                      hex_dump, payload_length > 32 ? "..." : "");

        // If it looks like text data and is reasonably short, also show ASCII representation
        if (bytes_to_dump > 0) {
            netdata_log_debug(D_WEBSOCKET, "WEBSOCKET: C=%zu M=%zu F=%zu PAYLOAD ASCII: \"%s\"%s",
                          wsc->id, wsc->message_id, wsc->frame_id,
                          ascii_dump, payload_length > 64 ? "..." : "");
        }
    }
#endif /* NETDATA_INTERNAL_CHECKS */
}
