// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

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

