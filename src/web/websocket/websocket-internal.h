// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBSOCKET_INTERNAL_H
#define NETDATA_WEBSOCKET_INTERNAL_H

#include "websocket.h"

// Maximum number of WebSocket threads
#define WEBSOCKET_MAX_THREADS 2

#define WORKERS_WEBSOCKET_POLL              0
#define WORKERS_WEBSOCKET_CMD_READ          1
#define WORKERS_WEBSOCKET_CMD_EXIT          2
#define WORKERS_WEBSOCKET_CMD_ADD           3
#define WORKERS_WEBSOCKET_CMD_DEL           4
#define WORKERS_WEBSOCKET_CMD_BROADCAST     5
#define WORKERS_WEBSOCKET_CMD_UNKNOWN       6
#define WORKERS_WEBSOCKET_SOCK_RECEIVE      7
#define WORKERS_WEBSOCKET_SOCK_SEND         8
#define WORKERS_WEBSOCKET_SOCK_ERROR        9
#define WORKERS_WEBSOCKET_CLIENT_TIMEOUT    10
#define WORKERS_WEBSOCKET_SEND_PING         11
#define WORKERS_WEBSOCKET_CLIENT_STUCK      12

#define WORKERS_WEBSOCKET_INCOMPLETE_FRAME  13
#define WORKERS_WEBSOCKET_COMPLETE_FRAME    14
#define WORKERS_WEBSOCKET_MESSAGE           15
#define WORKERS_WEBSOCKET_MSG_PING          16
#define WORKERS_WEBSOCKET_MSG_PONG          17
#define WORKERS_WEBSOCKET_MSG_CLOSE         18
#define WORKERS_WEBSOCKET_MSG_INVALID       19

// Forward declaration for thread structure
struct websocket_thread;

#include "websocket-compression.h"

// WebSocket protocol constants
#define WS_GUID "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WebSocket frame constants
#define WS_FIN                      0x80  // Final frame bit
#define WS_RSV1                     0x40  // Reserved bit 1 (used for compression)
#define WS_MASK                     0x80  // Mask bit

// Frame size limits for protection against DoS and browser compatibility
#define WS_MAX_INCOMING_FRAME_SIZE  (20ULL * 1024 * 1024) // 20MB max incoming frame size (browsers have ~16MiB)
#define WS_MAX_OUTGOING_FRAME_SIZE  (4ULL * 1024 * 1024)  // 4MB max outgoing frame size for browser compatibility
#define WS_MAX_DECOMPRESSED_SIZE    (200ULL * 1024 * 1024) // 200MB max inbound uncompressed message

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

// WebSocket connection context - full structure definition
struct websocket_server_client {
    WEBSOCKET_STATE state;
    ND_SOCK sock;        // Socket with SSL abstraction
    uint32_t id;         // Unique client ID
    size_t max_message_size;
    size_t max_outbound_frame_size;     // Maximum size of outgoing frames for this client
    time_t connected_t;  // Connection timestamp
    time_t last_activity_t; // Last activity timestamp

    // Buffer for I/O data
    struct circular_buffer in_buffer;   // Incoming raw data (circular buffer)
    struct circular_buffer out_buffer;  // Outgoing raw data (circular buffer)
    size_t next_frame_size;             // The size of the next complete frame to read

    // Connection info
    char client_ip[INET6_ADDRSTRLEN];
    char client_port[NI_MAXSERV];
    WEBSOCKET_PROTOCOL protocol; // The negotiated subprotocol

    // Authentication info
    USER_AUTH user_auth;          // Authentication information copied from web_client

    // Thread management
    struct websocket_thread *wth; // The thread handling this client
    struct websocket_server_client *prev; // Linked list for thread's client management
    struct websocket_server_client *next; // Linked list for thread's client management

    // Message processing state
    WS_BUF payload;                     // Pre-allocated buffer for message data
    WS_BUF u_payload;                   // Pre-allocated buffer for uncompressed message data
    WS_BUF c_payload;                   // Pre-allocated buffer for outbound compressed data
    WEBSOCKET_OPCODE opcode;            // Current message opcode
    bool is_compressed;                 // Whether the current message is compressed
    bool message_complete;              // Whether the current message is complete
    size_t message_id;                  // Sequential ID for messages, starting from 0
    size_t frame_id;                    // Sequential ID for frames within current message

    // Compression state
    WEBSOCKET_COMPRESSION_CTX compression;

    // Connection closing state
    bool flush_and_remove_client;       // Flag to indicate we're just flushing buffer before close

    // Protocol handler callbacks
    void (*on_connect)(struct websocket_server_client *wsc);                                            // Called when a client is successfully connected
    void (*on_message)(struct websocket_server_client *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode); // Called when a message is received
    void (*on_close)(struct websocket_server_client *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason); // Called BEFORE sending close frame
    void (*on_disconnect)(struct websocket_server_client *wsc);                                         // Called when a client is disconnected

    // User data for application use
    void *user_data;
};

// Forward declarations for websocket client
typedef struct websocket_server_client WS_CLIENT;

// WebSocket thread structure
typedef struct websocket_thread {
    size_t id;                       // Thread ID
    pid_t tid;

    struct {
        ND_THREAD *thread;           // Thread handle
        bool running;                // Thread running status
        SPINLOCK spinlock;           // Thread spinlock
    };

    size_t clients_current;                   // Current number of clients in the thread
    SPINLOCK clients_spinlock;                // Spinlock for client operations
    struct websocket_server_client *clients;  // Head of the clients double-linked list

    nd_poll_t *ndpl;             // Poll instance

    struct {
        int pipe[2];                 // Command pipe [0] = read, [1] = write
    } cmd;

} WEBSOCKET_THREAD;

// Global array of WebSocket threads
extern WEBSOCKET_THREAD websocket_threads[WEBSOCKET_MAX_THREADS];

// Define JudyL typed structure for WebSocket clients
DEFINE_JUDYL_TYPED(WS_CLIENTS, struct websocket_server_client *);

// WebSocket thread commands
#define WEBSOCKET_THREAD_CMD_EXIT            1
#define WEBSOCKET_THREAD_CMD_ADD_CLIENT      2
#define WEBSOCKET_THREAD_CMD_REMOVE_CLIENT   3
#define WEBSOCKET_THREAD_CMD_BROADCAST       4

// Buffer size definitions for WebSocket operations
#define WEBSOCKET_RECEIVE_BUFFER_SIZE 4096  // Size used for network read operations

// Initial buffer sizes
#define WEBSOCKET_IN_BUFFER_INITIAL_SIZE  8192UL  // Initial size for incoming data buffer
#define WEBSOCKET_OUT_BUFFER_INITIAL_SIZE 16384UL // Initial size for outgoing data buffer
#define WEBSOCKET_PAYLOAD_INITIAL_SIZE    8192UL  // Initial size for message payload buffer
#define WEBSOCKET_UNPACKED_INITIAL_SIZE   16384UL // Initial size for uncompressed message buffer

// Maximum buffer sizes to protect against memory exhaustion
#define WEBSOCKET_IN_BUFFER_MAX_SIZE  (20UL * 1024 * 1024)  // 10MiB max for incoming data buffer
#define WEBSOCKET_OUT_BUFFER_MAX_SIZE (20UL * 1024 * 1024)  // 10MiB max for outgoing data buffer

NEVERNULL
WS_CLIENT *websocket_client_create(void);

// Thread management
void websocket_threads_init(void);
void websocket_threads_join(void);
bool websocket_thread_send_command(WEBSOCKET_THREAD *wth, uint8_t cmd, uint32_t id);
bool websocket_thread_send_broadcast(WEBSOCKET_THREAD *wth, WEBSOCKET_OPCODE opcode, const char *message);
void websocket_thread(void *ptr);
void websocket_thread_enqueue_client(WEBSOCKET_THREAD *wth, struct websocket_server_client *wsc);
bool websocket_thread_update_client_poll_flags(struct websocket_server_client *wsc);

// Client registry internals
void websocket_client_free(WS_CLIENT *wsc);
bool websocket_client_register(struct websocket_server_client *wsc);
void websocket_client_unregister(struct websocket_server_client *wsc);
struct websocket_server_client *websocket_client_find_by_id(size_t id);

// Utility functions
// Validates a WebSocket close code according to RFC 6455
bool websocket_validate_close_code(uint16_t code);
void websocket_debug(WS_CLIENT *wsc, const char *format, ...);
void websocket_info(WS_CLIENT *wsc, const char *format, ...);
void websocket_error(WS_CLIENT *wsc, const char *format, ...);
void websocket_dump_debug(WS_CLIENT *wsc, const char *payload, size_t payload_length, const char *format, ...);

// Frame processing result codes
typedef enum {
    WS_FRAME_ERROR = -1,       // Processing error occurred
    WS_FRAME_COMPLETE = 0,     // Frame processing completed successfully
    WS_FRAME_NEED_MORE_DATA = 1,    // Need more data to complete frame processing
    WS_FRAME_MESSAGE_READY = 2 // A complete message is ready to be processed
} WEBSOCKET_FRAME_RESULT;

// Centralized protocol validation functions
void websocket_protocol_exception(WS_CLIENT *wsc, WEBSOCKET_CLOSE_CODE reason_code, const char *reason_txt);

// Protocol receiver functions - websocket-protocol-rcv.c
ssize_t websocket_protocol_got_data(WS_CLIENT *wsc, char *data, size_t length);

// Payload sender - breaks large messages into multiple frames
int websocket_protocol_send_payload(
    WS_CLIENT *wsc, const char *payload,
                                  size_t payload_len, WEBSOCKET_OPCODE opcode, bool use_compression);
int websocket_protocol_send_text(WS_CLIENT *wsc, const char *text);
int websocket_protocol_send_binary(WS_CLIENT *wsc, const void *data, size_t length);
int websocket_protocol_send_close(WS_CLIENT *wsc, WEBSOCKET_CLOSE_CODE code, const char *reason);
int websocket_protocol_send_ping(WS_CLIENT *wsc, const char *data, size_t length);
int websocket_protocol_send_pong(WS_CLIENT *wsc, const char *data, size_t length);

// IO functions from old implementation - will be refactored
ssize_t websocket_receive_data(struct websocket_server_client *wsc);
ssize_t websocket_write_data(struct websocket_server_client *wsc);

// WebSocket message sending functions
int websocket_send_message(WS_CLIENT *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode);
int websocket_broadcast_message(const char *message, WEBSOCKET_OPCODE opcode);

bool websocket_protocol_parse_header_from_buffer(const char *buffer, size_t length,
                                                 WEBSOCKET_FRAME_HEADER *header);
#endif // NETDATA_WEBSOCKET_INTERNAL_H