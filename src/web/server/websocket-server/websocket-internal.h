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

// WebSocket frame opcodes as per RFC 6455
typedef enum __attribute__((packed)) {
    WS_OPCODE_CONTINUATION = 0x0,
    WS_OPCODE_TEXT         = 0x1,
    WS_OPCODE_BINARY       = 0x2,
    WS_OPCODE_CLOSE        = 0x8,
    WS_OPCODE_PING         = 0x9,
    WS_OPCODE_PONG         = 0xA
} WEBSOCKET_OPCODE;

// WebSocket connection states
typedef enum __attribute__((packed)) {
    WS_STATE_HANDSHAKE         = 0, // Initial handshake in progress
    WS_STATE_OPEN              = 1, // Connection established
    WS_STATE_CLOSING_SERVER    = 2, // Server initiated closing handshake
    WS_STATE_CLOSING_CLIENT    = 3, // Client initiated closing handshake
    WS_STATE_CLOSED            = 4  // Connection closed
} WEBSOCKET_STATE;

// Forward declaration for thread structure
struct websocket_thread;

#include "websocket-compression.h"
#include "websocket-structures.h"

// WebSocket connection context - full structure definition
struct websocket_server_client {
    WEBSOCKET_STATE state;
    ND_SOCK sock;        // Socket with SSL abstraction
    uint32_t id;         // Unique client ID
    size_t max_message_size;
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

    // Thread management
    struct websocket_thread *wth; // The thread handling this client
    struct websocket_server_client *prev; // Linked list for thread's client management
    struct websocket_server_client *next; // Linked list for thread's client management

    // Message processing state
    WS_BUF payload;                     // Pre-allocated buffer for message data
    WS_BUF u_payload;                   // Pre-allocated buffer for uncompressed message data
    WEBSOCKET_OPCODE opcode;            // Current message opcode
    bool is_compressed;                 // Whether the current message is compressed
    bool message_complete;              // Whether the current message is complete
    size_t message_id;                  // Sequential ID for messages, starting from 0
    size_t frame_id;                    // Sequential ID for frames within current message

    // Compression state
    WEBSOCKET_COMPRESSION_CTX compression;

    // Connection closing state
    bool flush_and_remove_client;       // Flag to indicate we're just flushing buffer before close

    // Message handling callbacks
    void (*on_message)(struct websocket_server_client *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode);
    void (*on_close)(struct websocket_server_client *wsc, int code, const char *reason);
    void (*on_error)(struct websocket_server_client *wsc, const char *error);

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
void *websocket_thread(void *ptr);
void websocket_thread_enqueue_client(WEBSOCKET_THREAD *wth, struct websocket_server_client *wsc);
bool websocket_thread_update_client_poll_flags(struct websocket_server_client *wsc);

// Client registry internals
void websocket_client_free(WS_CLIENT *wsc);
bool websocket_client_register(struct websocket_server_client *wsc);
void websocket_client_unregister(struct websocket_server_client *wsc);
struct websocket_server_client *websocket_client_find_by_id(size_t id);

// Utility functions
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
void websocket_protocol_exception(WS_CLIENT *wsc, uint16_t reason_code, const char *reason_txt);

// Protocol receiver functions - websocket-protocol-rcv.c
ssize_t websocket_protocol_got_data(WS_CLIENT *wsc, char *data, size_t length);

// Protocol sender functions - websocket-protocol-snd.c
int websocket_protocol_send_frame(
    WS_CLIENT *wsc, const char *payload,
                                  size_t payload_len, WEBSOCKET_OPCODE opcode, bool use_compression);
int websocket_protocol_send_text(WS_CLIENT *wsc, const char *text);
int websocket_protocol_send_binary(WS_CLIENT *wsc, const void *data, size_t length);
int websocket_protocol_send_close(WS_CLIENT *wsc, uint16_t code, const char *reason);
int websocket_protocol_send_ping(WS_CLIENT *wsc, const char *data, size_t length);
int websocket_protocol_send_pong(WS_CLIENT *wsc, const char *data, size_t length);

// Payload handler functions - websocket-payload-rcv.c
bool websocket_payload_handle_message(WS_CLIENT *wsc, WS_BUF *wsb);
int websocket_payload_echo(struct websocket_server_client *wsc, WS_BUF *wsb);
struct json_object *websocket_client_parse_json(struct websocket_server_client *wsc);
void websocket_payload_error(struct websocket_server_client *wsc, const char *error_message);

// JSON payload functions - websocket-payload-snd.c
int websocket_client_send_json(struct websocket_server_client *wsc, struct json_object *json);

// IO functions from old implementation - will be refactored
ssize_t websocket_receive_data(struct websocket_server_client *wsc);
ssize_t websocket_write_data(struct websocket_server_client *wsc);

// WebSocket message sending functions
int websocket_send_message(WS_CLIENT *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode);
int websocket_broadcast_message(const char *message, WEBSOCKET_OPCODE opcode);
int websocket_client_send_text_fragmented(WS_CLIENT *wsc, const char **fragments, int count);

bool websocket_protocol_parse_header_from_buffer(const char *buffer, size_t length,
                                                 WEBSOCKET_FRAME_HEADER *header);
#endif // NETDATA_WEBSOCKET_INTERNAL_H