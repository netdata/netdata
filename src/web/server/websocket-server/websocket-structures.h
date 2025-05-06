// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBSOCKET_STRUCTURES_H
#define NETDATA_WEBSOCKET_STRUCTURES_H

#include "websocket.h"
#include "websocket-compression.h"

// WebSocket protocol constants
#define WS_GUID "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WebSocket close codes (RFC 6455)
#define WS_CLOSE_NORMAL            1000
#define WS_CLOSE_GOING_AWAY        1001
#define WS_CLOSE_PROTOCOL_ERROR    1002
#define WS_CLOSE_UNSUPPORTED_DATA  1003
#define WS_CLOSE_INVALID_PAYLOAD   1007  // Frame payload data is invalid
#define WS_CLOSE_POLICY_VIOLATION  1008
#define WS_CLOSE_MESSAGE_TOO_BIG   1009
#define WS_CLOSE_INTERNAL_ERROR    1011

// WebSocket frame constants
#define WS_FIN                     0x80  // Final frame bit
#define WS_RSV1                    0x40  // Reserved bit 1 (used for compression)
#define WS_MASK                    0x80  // Mask bit
// Frame size limit - affects fragmentation but not total message size
#define WS_MAX_FRAME_LENGTH        (20 * 1024 * 1024) // 20MB max frame size

// Total message size limits - these prevent resource exhaustion
#define WEBSOCKET_MAX_COMPRESSED_SIZE    (20ULL * 1024 * 1024) // 20MB max compressed message
#define WEBSOCKET_MAX_UNCOMPRESSED_SIZE  (200ULL * 1024 * 1024) // 200MB max uncompressed message

// WebSocket frame header structure - used for processing frame headers
typedef struct websocket_frame_header {
    unsigned char fin:1;
    unsigned char rsv1:1;
    unsigned char rsv2:1;
    unsigned char rsv3:1;
    unsigned char opcode:4;
    unsigned char mask:1;
    unsigned char len:7;

    unsigned char mask_key[4];      // Masking key (if present)
    size_t frame_size;              // Size of the entire frame
    size_t header_size;             // Size of the header
    size_t payload_length;          // Length of the payload data
    void *payload;                  // Pointer to the payload data
} WEBSOCKET_FRAME_HEADER;

// Buffer for message data (used for reassembly of fragmented messages)
typedef struct websocket_buffer {
    char *data;                     // Buffer holding message data
    size_t length;                  // Current buffer length
    size_t size;                    // Allocated buffer size
} WS_BUF;

// Forward declaration for client structure
struct websocket_server_client;

// Function prototypes for buffer handling

// Message and payload processing functions
void websocket_client_message_reset(struct websocket_server_client *wsc);
bool websocket_client_process_message(struct websocket_server_client *wsc);
bool websocket_client_decompress_message(struct websocket_server_client *wsc);

// Additional helper functions
bool websocket_frame_is_control_opcode(WEBSOCKET_OPCODE opcode);
bool websocket_validate_utf8(const char *data, size_t length);

#include "websocket-buffer.h"

#endif // NETDATA_WEBSOCKET_STRUCTURES_H