// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"
#include "websocket-echo.h"
#include "websocket-jsonrpc.h"
#include "../mcp/adapters/mcp-websocket.h"

ENUM_STR_MAP_DEFINE(WEBSOCKET_PROTOCOL) = {
    { .id = WS_PROTOCOL_JSONRPC, .name = "jsonrpc" },
    { .id = WS_PROTOCOL_ECHO,    .name = "echo" },
    { .id = WS_PROTOCOL_MCP,     .name = "mcp" },
    { .id = WS_PROTOCOL_UNKNOWN, .name = "unknown" },

    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(WEBSOCKET_PROTOCOL, WS_PROTOCOL_UNKNOWN, "unknown");

ENUM_STR_MAP_DEFINE(WEBSOCKET_STATE) = {
    { .id = WS_STATE_HANDSHAKE,      .name = "handshake" },
    { .id = WS_STATE_OPEN,           .name = "open" },
    { .id = WS_STATE_CLOSING_SERVER, .name = "closing_server" },
    { .id = WS_STATE_CLOSING_CLIENT, .name = "closing_client" },
    { .id = WS_STATE_CLOSED,         .name = "closed" },

    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(WEBSOCKET_STATE, WS_STATE_CLOSED, "closed");

ENUM_STR_MAP_DEFINE(WEBSOCKET_OPCODE) = {
    { .id = WS_OPCODE_CONTINUATION, .name = "continuation" },
    { .id = WS_OPCODE_TEXT,         .name = "text" },
    { .id = WS_OPCODE_BINARY,       .name = "binary" },
    { .id = WS_OPCODE_CLOSE,        .name = "close" },
    { .id = WS_OPCODE_PING,         .name = "ping" },
    { .id = WS_OPCODE_PONG,         .name = "pong" },

    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(WEBSOCKET_OPCODE, WS_OPCODE_TEXT, "text");

ENUM_STR_MAP_DEFINE(WEBSOCKET_CLOSE_CODE) = {
    // Standard WebSocket close codes
    { .id = WS_CLOSE_NORMAL,            .name = "normal" },
    { .id = WS_CLOSE_GOING_AWAY,        .name = "going_away" },
    { .id = WS_CLOSE_PROTOCOL_ERROR,    .name = "protocol_error" },
    { .id = WS_CLOSE_UNSUPPORTED_DATA,  .name = "unsupported_data" },
    { .id = WS_CLOSE_RESERVED,          .name = "reserved" },
    { .id = WS_CLOSE_NO_STATUS,         .name = "no_status" },
    { .id = WS_CLOSE_ABNORMAL,          .name = "abnormal" },
    { .id = WS_CLOSE_INVALID_PAYLOAD,   .name = "invalid_payload" },
    { .id = WS_CLOSE_POLICY_VIOLATION,  .name = "policy_violation" },
    { .id = WS_CLOSE_MESSAGE_TOO_BIG,   .name = "message_too_big" },
    { .id = WS_CLOSE_EXTENSION_MISSING, .name = "extension_missing" },
    { .id = WS_CLOSE_INTERNAL_ERROR,    .name = "internal_error" },
    { .id = WS_CLOSE_TLS_HANDSHAKE,     .name = "tls_handshake_error" },

    // Netdata-specific close codes
    { .id = WS_CLOSE_NETDATA_TIMEOUT,   .name = "timeout" },
    { .id = WS_CLOSE_NETDATA_SHUTDOWN,  .name = "shutdown" },
    { .id = WS_CLOSE_NETDATA_REJECTED,  .name = "rejected" },
    { .id = WS_CLOSE_NETDATA_RATE_LIMIT,.name = "rate_limit" },

    // terminator
    { .name = NULL, .id = 0 }
};
ENUM_STR_DEFINE_FUNCTIONS(WEBSOCKET_CLOSE_CODE, WS_CLOSE_NORMAL, "normal");

// Private structure for WebSocket server state
struct websocket_server {
    WS_CLIENTS_JudyLSet clients;     // JudyL array of WebSocket clients
    size_t client_id_counter;        // Counter for generating unique client IDs
    size_t active_clients;           // Number of active clients
    SPINLOCK spinlock;               // Spinlock to protect the registry
};

// The global (but private) instance of the WebSocket server state
static struct websocket_server ws_server = (struct websocket_server){
    .clients = { 0 },
    .client_id_counter = 0,
    .active_clients = 0,
    .spinlock = SPINLOCK_INITIALIZER
};

// Initialize WebSocket subsystem
void websocket_initialize(void) {
    // debug_flags |= D_WEBSOCKET;

    // Initialize thread system
    websocket_threads_init();

    // Initialize protocol handlers
    websocket_jsonrpc_initialize();
    websocket_echo_initialize();
    mcp_websocket_adapter_initialize();

    netdata_log_info("WebSocket server subsystem initialized");
}

// Create a new WebSocket client with a unique ID
NEVERNULL
WS_CLIENT *websocket_client_create(void) {
    WS_CLIENT *wsc = callocz(1, sizeof(WS_CLIENT));

    spinlock_lock(&ws_server.spinlock);
    wsc->id = ++ws_server.client_id_counter; // Generate unique ID
    spinlock_unlock(&ws_server.spinlock);

    wsc->connected_t = now_realtime_sec();
    wsc->last_activity_t = wsc->connected_t;
    wsc->max_outbound_frame_size = WS_MAX_OUTGOING_FRAME_SIZE; // Default value

    // initialize callbacks to NULL
    wsc->on_connect = NULL;
    wsc->on_message = NULL;
    wsc->on_close = NULL;
    wsc->on_disconnect = NULL;

    // initialize the ND_SOCK with the web server's SSL context
    nd_sock_init(&wsc->sock, netdata_ssl_web_server_ctx, false);

    // Initialize circular buffers for I/O with WebSocket-specific sizes and max limits
    cbuffer_init(&wsc->in_buffer, WEBSOCKET_IN_BUFFER_INITIAL_SIZE, WEBSOCKET_IN_BUFFER_MAX_SIZE, NULL);
    cbuffer_init(&wsc->out_buffer, WEBSOCKET_OUT_BUFFER_INITIAL_SIZE, WEBSOCKET_OUT_BUFFER_MAX_SIZE, NULL);

    // Initialize pre-allocated message buffer
    wsb_init(&wsc->payload, WEBSOCKET_PAYLOAD_INITIAL_SIZE);

    // Initialize uncompressed buffer with a larger size since decompressed data can expand
    // For compressed content, the expanded data can be much larger than the input
    wsb_init(&wsc->u_payload, WEBSOCKET_UNPACKED_INITIAL_SIZE);
    
    // Initialize compressed output buffer for outbound messages
    wsb_init(&wsc->c_payload, WEBSOCKET_PAYLOAD_INITIAL_SIZE);

    // Set the initial message state
    wsc->opcode = WS_OPCODE_TEXT; // Default opcode
    wsc->is_compressed = false;
    wsc->message_complete = true; // Not in a fragmented sequence initially
    wsc->frame_id = 0;
    wsc->message_id = 0;
    wsc->compression = WEBSOCKET_COMPRESSION_DEFAULTS;

    return wsc;
}

// Free a WebSocket client
void websocket_client_free(WS_CLIENT *wsc) {
    if (!wsc)
        return;

    // First unregister from the client registry
    websocket_client_unregister(wsc);

    // We MUST make sure the socket is not in the poll before closing it
    // otherwise kernel structures may be corrupted due to socket reuse
    if(wsc->wth && wsc->wth->ndpl && wsc->sock.fd >= 0)
        (void) nd_poll_del(wsc->wth->ndpl, wsc->sock.fd);

    // Close socket using ND_SOCK abstraction
    nd_sock_close(&wsc->sock);

    // Free circular buffers
    cbuffer_cleanup(&wsc->in_buffer);
    cbuffer_cleanup(&wsc->out_buffer);

    // Cleanup pre-allocated message, uncompressed, and compressed buffers
    wsb_cleanup(&wsc->payload);
    wsb_cleanup(&wsc->u_payload);
    wsb_cleanup(&wsc->c_payload);

    // Clean up compression resources if needed
    websocket_compression_cleanup(wsc);

    freez(wsc);
}

// Register a WebSocket client in the registry
bool websocket_client_register(WS_CLIENT *wsc) {
    if (!wsc || wsc->id == 0)
        return false;
    
    spinlock_lock(&ws_server.spinlock);
    
    int added = WS_CLIENTS_SET(&ws_server.clients, wsc->id, wsc);
    if (!added) {
        ws_server.active_clients++;
        websocket_debug(wsc, "WebSocket client registered, total clients: %u", ws_server.active_clients);
    }
    
    spinlock_unlock(&ws_server.spinlock);
    
    return added;
}

// Unregister a WebSocket client from the registry
void websocket_client_unregister(WS_CLIENT *wsc) {
    if (!wsc || wsc->id == 0)
        return;
    
    spinlock_lock(&ws_server.spinlock);

    WS_CLIENT *existing = WS_CLIENTS_GET(&ws_server.clients, wsc->id);
    if (existing && existing == wsc) {
        WS_CLIENTS_DEL(&ws_server.clients, wsc->id);
        if (ws_server.active_clients > 0)
            ws_server.active_clients--;
        
        websocket_debug(wsc,"WebSocket client unregistered, total clients: %zu", ws_server.active_clients);
    }
    
    spinlock_unlock(&ws_server.spinlock);
}

// Find a WebSocket client by ID
ALWAYS_INLINE
WS_CLIENT *websocket_client_find_by_id(size_t id) {
    if (id == 0)
        return NULL;

    WS_CLIENT *wsc = NULL;

    spinlock_lock(&ws_server.spinlock);
    wsc = WS_CLIENTS_GET(&ws_server.clients, id);
    spinlock_unlock(&ws_server.spinlock);

    return wsc;
}

// Broadcast a message to all connected WebSocket clients
int websocket_broadcast_message(const char *message, WEBSOCKET_OPCODE opcode) {
    if (!message || (opcode != WS_OPCODE_TEXT && opcode != WS_OPCODE_BINARY))
        return -1;

    int success_count = 0;

    // Send broadcast command to all active threads
    for(size_t i = 0; i < WEBSOCKET_MAX_THREADS; i++) {
        if(websocket_threads[i].thread && websocket_threads[i].running) {
            if(websocket_thread_send_broadcast(&websocket_threads[i], opcode, message)) {
                success_count++;
            }
        }
    }
    
    return success_count;
}

// Send a WebSocket message to the client
int websocket_send_message(WS_CLIENT *wsc, const char *message, size_t length, WEBSOCKET_OPCODE opcode) {
    if (!wsc || !message || wsc->state != WS_STATE_OPEN)
        return -1;

    // For other opcodes, use the generic frame sender
    bool use_compression = wsc->compression.enabled &&
                           !websocket_frame_is_control_opcode(opcode) &&
                           length >= WS_COMPRESS_MIN_SIZE;

    return websocket_protocol_send_payload(wsc, message, length, opcode, use_compression);
}
