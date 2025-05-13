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
    netdata_log_debug(D_WEBSOCKET, "WEBSOCKET: C=%u M=%zu F=%zu %s",
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
    netdata_log_info("WEBSOCKET: C=%u M=%zu F=%zu %s",
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
    netdata_log_error("WEBSOCKET: C=%u M=%zu F=%zu %s",
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

    // Handle empty payloads explicitly (log message but no hex dump)
    if (payload_length == 0) {
        netdata_log_debug(D_WEBSOCKET, "WEBSOCKET: C=%u M=%zu F=%zu %s (EMPTY PAYLOAD - 0 bytes)",
                      wsc->id, wsc->message_id, wsc->frame_id, formatted_message);
        return;
    }

    // If payload is provided and not empty, create and log a hex dump
    if (payload && payload_length > 0) {
        size_t bytes_to_dump = (payload_length < 32) ? payload_length : 32;

        char hex_dump[bytes_to_dump * 2 + 1];
        char ascii_dump[bytes_to_dump + 1];

        // Payload check is redundant as we already have it in the outer if

        // Create the hex dump
        size_t i = 0;
        for (i = 0; i < bytes_to_dump; i++) {
            unsigned char c = (unsigned char)payload[i];
            sprintf(hex_dump + i * 2, "%02x", c);
            ascii_dump[i] = (isprint(c) ? c : '.');
        }

        hex_dump[i * 2] = '\0';
        ascii_dump[i] = '\0';

        // Log the hex dump
        netdata_log_debug(D_WEBSOCKET, "WEBSOCKET: C=%u M=%zu F=%zu %s DUMP %zu/%zu: HEX:[%s]%s, ASCII:[%s]%s",
                          wsc->id, wsc->message_id, wsc->frame_id,
                          formatted_message,
                          bytes_to_dump, payload_length,
                          hex_dump, payload_length > bytes_to_dump ? "..." : "",
                          ascii_dump, payload_length > bytes_to_dump ? "..." : "");
    }
#endif /* NETDATA_INTERNAL_CHECKS */
}
