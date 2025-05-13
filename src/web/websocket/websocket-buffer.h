// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBSOCKET_BUFFER_H
#define NETDATA_WEBSOCKET_BUFFER_H

#include "websocket-internal.h"

ALWAYS_INLINE
static void websocket_unmask(char *dst, const char *src, size_t length, const unsigned char *mask_key) {
    for (size_t i = 0; i < length; i++)
        dst[i] = (char)((unsigned char)src[i] ^ mask_key[i % 4]);
}

// Initialize an already allocated buffer structure
ALWAYS_INLINE
static void wsb_init(WS_BUF *wsb, size_t initial_size) {
    if (!wsb) return;
    wsb->data = mallocz(initial_size);
    wsb->size = initial_size;
    wsb->length = 0;
}

// Clean up an embedded buffer (free data but not the buffer structure itself)
ALWAYS_INLINE
static void wsb_cleanup(WS_BUF *wsb) {
    if (!wsb) return;
    freez(wsb->data);
    wsb->data = NULL;
    wsb->size = 0;
    wsb->length = 0;
}

// Initialize buffer structure
ALWAYS_INLINE
static WS_BUF *wsb_create(size_t initial_size) {
    WS_BUF *buffer = mallocz(sizeof(WS_BUF));
    wsb_init(buffer, MAX(initial_size, 1024));
    return buffer;
}

// Free buffer structure
ALWAYS_INLINE
static void wsb_free(WS_BUF *wsb) {
    if (!wsb) return;
    wsb_cleanup(wsb);
    freez(wsb);
}

// Resize buffer to a new size
ALWAYS_INLINE
static void wsb_resize(WS_BUF *wsb, size_t new_size) {
    if (new_size <= wsb->size) return;
    wsb->data = reallocz(wsb->data, new_size);
    wsb->size = new_size;
}

// Append data to buffer
ALWAYS_INLINE
static void wsb_need_bytes(WS_BUF *wsb, size_t bytes) {
    if (!wsb) return;

    // 1 for null + 4 for the final decompression padding
    size_t wanted_size = wsb->length + bytes + 1 + 4;
    if (wanted_size < wsb->size)
        return;

    size_t new_size = wsb->size * 2;
    if (new_size < wanted_size)
        new_size = wanted_size;

    wsb_resize(wsb, new_size);
}

// Reset buffer
ALWAYS_INLINE
static void wsb_reset(WS_BUF *wsb) {
    if (!wsb) return;
    wsb->length = 0;
}

// Ensure buffer has null termination for text data
ALWAYS_INLINE
static void wsb_null_terminate(WS_BUF *wsb) {
    if (!wsb) return;
    wsb_need_bytes(wsb, 1);
    wsb->data[wsb->length] = '\0';
}

// Check if buffer is empty
ALWAYS_INLINE
static bool wsb_is_empty(const WS_BUF *wsb) {
    return (!wsb || wsb->length == 0);
}

// Check if buffer has data
ALWAYS_INLINE
static bool wsb_has_data(const WS_BUF *wsb) {
    return (wsb && wsb->data && wsb->length > 0);
}

// Get pointer to buffer data
ALWAYS_INLINE
static char *wsb_data(WS_BUF *wsb) {
    return wsb ? wsb->data : NULL;
}

// Get current buffer length
ALWAYS_INLINE
static size_t wsb_length(const WS_BUF *wsb) {
    return wsb ? wsb->length : 0;
}

// Get allocated buffer size
ALWAYS_INLINE
static size_t wsb_size(const WS_BUF *wsb) {
    return wsb ? wsb->size : 0;
}

// Set buffer length (must be <= buffer size)
ALWAYS_INLINE
static void wsb_set_length(WS_BUF *wsb, size_t length) {
    if (!wsb) return;

    if (length > wsb->size)
        fatal("WEBSOCKET: trying to set length to %zu, but buffer size is %zu", length, wsb->size);

    wsb->length = length;
}

// Append data to a buffer
ALWAYS_INLINE
static char *wsb_append(WS_BUF *wsb, const void *data, size_t length) {
    if (!wsb || !data || !length)
        return NULL;

    // Ensure buffer is large enough
    wsb_need_bytes(wsb, length);

    char *dst = wsb->data + wsb->length;

    // Copy data to end of buffer
    memcpy(dst, data, length);

    // Update length
    wsb->length += length;

    return dst;
}

// Unmask and append binary data to a buffer, returns pointer to beginning of the unmasked data
ALWAYS_INLINE
static char *wsb_unmask_and_append(WS_BUF *wsb, const void *masked_data,
                                          size_t length, const unsigned char *mask_key) {
    if (!wsb || !masked_data || !length || !mask_key)
        return NULL;

    // Ensure buffer is large enough for the new data
    wsb_need_bytes(wsb, length);

    // Get a pointer to the destination in the expanded buffer
    char *dst = wsb->data + wsb->length;

    // Unmask the data directly into the buffer by calling websocket_unmask
    websocket_unmask(dst, (const char *)masked_data, length, mask_key);

    // Update buffer length
    wsb->length += length;

    return dst;
}

// Append data to a buffer but don't change the length (use for padding)
// Returns pointer to beginning of the appended data area
ALWAYS_INLINE
static char *wsb_append_padding(WS_BUF *wsb, const void *data, size_t length) {
    if (!wsb || !data || !length)
        return NULL;

    // Ensure buffer is large enough for the new data
    wsb_need_bytes(wsb, length);

    // Get pointer to where the data will be stored
    char *dst = wsb->data + wsb->length;

    // Copy data to end of buffer
    memcpy(dst, data, length);

    // Don't update length - this is the difference from wsb_append()
    // This allows adding "padding" data after the logical end of the buffer

    return dst;
}

// Remove bytes from the front of the buffer, shifting remaining content forward
// Returns the number of bytes actually trimmed (may be less than requested if buffer is smaller)
ALWAYS_INLINE
static size_t wsb_trim_front(WS_BUF *wsb, size_t bytes_to_trim) {
    if (!wsb || !wsb->data || bytes_to_trim == 0 || wsb->length == 0)
        return 0;

    // Cap the trim size to the actual buffer length
    size_t actual_trim = (bytes_to_trim > wsb->length) ? wsb->length : bytes_to_trim;

    if (actual_trim < wsb->length) {
        // More data in buffer - shift remaining data to beginning
        size_t remaining = wsb->length - actual_trim;

        // Shift the remaining data to the beginning of the buffer
        memmove(wsb->data, wsb->data + actual_trim, remaining);

        // Update buffer length to reflect the shift
        wsb->length = remaining;
    } else {
        // All data was trimmed or the buffer is empty - reset length
        wsb->length = 0;
    }

    return actual_trim;
}

#endif //NETDATA_WEBSOCKET_BUFFER_H
