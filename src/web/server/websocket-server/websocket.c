// SPDX-License-Identifier: GPL-3.0-or-later

#include "websocket-internal.h"

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
    // Initialize thread system
    websocket_threads_init();

    // debug_flags |= D_WEBSOCKET;
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
        nd_poll_del(wsc->wth->ndpl, wsc->sock.fd);

    // Close socket using ND_SOCK abstraction
    nd_sock_close(&wsc->sock);

    // Free circular buffers
    cbuffer_cleanup(&wsc->in_buffer);
    cbuffer_cleanup(&wsc->out_buffer);

    // Cleanup pre-allocated message and uncompressed buffers
    wsb_cleanup(&wsc->payload);
    wsb_cleanup(&wsc->u_payload);

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
    
    // Use the appropriate protocol function based on opcode
    if (opcode == WS_OPCODE_TEXT) {
        return websocket_protocol_send_text(wsc, message);
    } else if (opcode == WS_OPCODE_BINARY) {
        return websocket_protocol_send_binary(wsc, message, length);
    } else {
        // For other opcodes, use the generic frame sender
        bool use_compression = wsc->compression.enabled && 
                             !websocket_frame_is_control_opcode(opcode) &&
                             length >= WS_COMPRESS_MIN_SIZE;
        
        return websocket_protocol_send_frame(wsc, message, length, opcode, use_compression);
    }
}
