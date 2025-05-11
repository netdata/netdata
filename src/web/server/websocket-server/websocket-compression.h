// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBSOCKET_COMPRESSION_H
#define NETDATA_WEBSOCKET_COMPRESSION_H

#include "daemon/common.h"
#include <zlib.h>

// WebSocket compression constants
#define WS_COMPRESS_WINDOW_BITS      15     // Default window bits (RFC 7692)
#define WS_COMPRESS_MEMLEVEL         8      // Default memory level for zlib
#define WS_COMPRESS_DEFAULT_LEVEL    Z_DEFAULT_COMPRESSION  // Default compression level
#define WS_COMPRESS_MIN_SIZE         64     // Don't compress payloads smaller than this

// WebSocket compression extension types
typedef enum {
    WS_COMPRESS_NONE = 0,      // No compression
    WS_COMPRESS_DEFLATE = 1    // permessage-deflate extension
} WEBSOCKET_COMPRESSION_TYPE;

// WebSocket compression context structure
typedef struct websocket_compression_context {
    WEBSOCKET_COMPRESSION_TYPE type;        // Compression type
    bool enabled;                           // Whether compression is enabled
    bool client_context_takeover;           // Client context takeover
    bool server_context_takeover;           // Server context takeover
    int window_bits;                        // Window bits (8-15)
    int compression_level;                  // Compression level
    z_stream* deflate_stream;               // Deflate context for outgoing messages
    z_stream* inflate_stream;               // Inflate context for incoming messages
} WEBSOCKET_COMPRESSION_CTX;

// Forward declaration
struct websocket_server_client;

// Function declarations
bool websocket_compression_init(struct websocket_server_client *wsc, const char *extensions);
void websocket_compression_cleanup(struct websocket_server_client *wsc);
char *websocket_compression_negotiate(struct websocket_server_client *wsc, const char *requested_extensions);
bool websocket_compression_supported(void);

// Decompression-specific functions
void websocket_decompression_cleanup(struct websocket_server_client *wsc);
bool websocket_decompression_init(struct websocket_server_client *wsc);
bool websocket_decompression_reset(struct websocket_server_client *wsc);

#endif // NETDATA_WEBSOCKET_COMPRESSION_H