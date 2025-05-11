// SPDX-License-Identifier: GPL-3.0-or-later

#include "web/server/web_client.h"
#include "websocket-internal.h"
#include <openssl/sha.h>

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

// Forward declarations for internal functions
static WS_CLIENT *websocket_client_create(void);

// Check if the current HTTP request is a WebSocket handshake request
bool websocket_detect_handshake_request(struct web_client *w) {
    // We need a valid key and to be flagged as a WebSocket request
    if (!web_client_is_websocket(w) || !w->websocket.key)
        return false;
    
    return true;
}

// Generate the WebSocket accept key as per RFC 6455
char *websocket_generate_handshake_key(const char *client_key) {
    if (!client_key)
        return NULL;
    
    // Concatenate the key with the WebSocket GUID
    char concat_key[256];
    snprintfz(concat_key, sizeof(concat_key), "%s%s", client_key, WS_GUID);
    
    // Create SHA-1 hash
    unsigned char sha_hash[SHA_DIGEST_LENGTH];
    SHA1((unsigned char *)concat_key, strlen(concat_key), sha_hash);
    
    // Convert to base64
    char *accept_key = mallocz(33); // Base64 of SHA-1 is 28 chars + null term
    netdata_base64_encode((unsigned char *)accept_key, sha_hash, SHA_DIGEST_LENGTH);
    
    return accept_key;
}

bool websocket_send_first_response(WS_CLIENT *wsc, const char *accept_key, const char *extensions) {
    CLEAN_BUFFER *header = buffer_create(1024, NULL);

    buffer_sprintf(header,
                   "HTTP/1.1 101 Switching Protocols\r\n"
                   "Upgrade: websocket\r\n"
                   "Connection: Upgrade\r\n"
                   "Sec-WebSocket-Accept: %s\r\n",
                   accept_key
    );

    // Add the selected subprotocol
    if (wsc->protocol == WS_PROTOCOL_NETDATA_JSON)
        buffer_strcat(header, "Sec-WebSocket-Protocol: netdata-json\r\n");

    // Check for WebSocket compression extension
    if (extensions && websocket_compression_supported()) {
        // Negotiate compression if extensions are requested
        char *used_extensions = websocket_compression_negotiate(wsc, extensions);
        if (used_extensions) {
            buffer_sprintf(header, "Sec-WebSocket-Extensions: %s\r\n", used_extensions);
            freez(used_extensions);
        }
    }

    // End of headers
    buffer_strcat(header, "\r\n");

    // Send the handshake response using ND_SOCK - we're still in the web server thread,
    // so we need to use the persist version to ensure the complete handshake is sent
    const char *header_str = buffer_tostring(header);
    size_t header_len = buffer_strlen(header);
    ssize_t bytes = nd_sock_write_persist(&wsc->sock, header_str, header_len, 20); // Try up to 20 chunks

    return bytes == (ssize_t)header_len;
}

// Handle the WebSocket handshake procedure
short int websocket_handle_handshake(struct web_client *w) {
    if (!websocket_detect_handshake_request(w))
        return HTTP_RESP_BAD_REQUEST;

    // Generate the accept key
    char *accept_key = websocket_generate_handshake_key(w->websocket.key);
    if (!accept_key)
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    
    // Create the WebSocket client object early so we can set up compression
    WS_CLIENT *wsc = websocket_client_create();

    // Copy client information
    strncpyz(wsc->client_ip, w->client_ip, sizeof(wsc->client_ip));
    strncpyz(wsc->client_port, w->client_port, sizeof(wsc->client_port));
    wsc->protocol = w->websocket.protocol;

    // Take over the connection immediately
    websocket_takeover_web_connection(w, wsc);

    if(!websocket_send_first_response(wsc, accept_key, w->websocket.extensions)) {
        netdata_log_error("WEBSOCKET: Failed to send complete WebSocket handshake response"); // No client yet
        freez(accept_key);
        websocket_client_free(wsc);
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    freez(accept_key);

    // Now that we've sent the handshake response successfully, set the connection state to open
    wsc->state = WS_STATE_OPEN;

    // Register the client in our registry
    if (!websocket_client_register(wsc)) {
        websocket_error(wsc, "Failed to register WebSocket client");
        websocket_client_free(wsc);
        return HTTP_RESP_WEBSOCKET_HANDSHAKE;
    }

    // Message structures are already initialized in websocket_client_create()

    // Set socket to non-blocking mode
    if (fcntl(wsc->sock.fd, F_SETFL, O_NONBLOCK) == -1) {
        websocket_error(wsc, "Failed to set WebSocket socket to non-blocking mode");
        websocket_client_free(wsc);
        return HTTP_RESP_WEBSOCKET_HANDSHAKE;
    }

    // Assign to a thread
    WEBSOCKET_THREAD *wth = websocket_thread_assign_client(wsc);
    if (!wth) {
        websocket_error(wsc, "Failed to assign WebSocket client to a thread");
        websocket_client_free(wsc);
        return HTTP_RESP_WEBSOCKET_HANDSHAKE;
    }

    // Initialize compression if permessage-deflate extension is enabled
    if (w->websocket.ext_flags & WS_EXTENSION_PERMESSAGE_DEFLATE) {
        // Initialize the compression with the original extension string for parameter parsing
        if (!websocket_compression_init(wsc, w->websocket.extensions))
            wsc->compression.enabled = false;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "WebSocket connection established with %s:%s using protocol: %s (client ID: %zu, thread: %zu), compression: %s",
           wsc->client_ip, wsc->client_port,
           (wsc->protocol == WS_PROTOCOL_NETDATA_JSON) ? "netdata-json" : "unknown",
           wsc->id, wth->id,
           wsc->compression.enabled ? "enabled" : "disabled");

    // Important: This code doesn't actually get sent to the client since we've already
    // taken over the socket. It's just used by the caller to identify what happened.
    return HTTP_RESP_WEBSOCKET_HANDSHAKE;
}

// Initialize WebSocket subsystem
void websocket_initialize(void) {
    // Initialize thread system
    websocket_threads_init();

    debug_flags |= D_WEBSOCKET;
    netdata_log_info("WebSocket server subsystem initialized");
}

// Create a new WebSocket client with a unique ID
NEVERNULL
static WS_CLIENT *websocket_client_create(void) {
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

    return wsc;
}

// Free a WebSocket client
void websocket_client_free(WS_CLIENT *wsc) {
    if (!wsc)
        return;

    // First unregister from the client registry
    websocket_client_unregister(wsc);

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
        netdata_log_debug(D_WEB_CLIENT, "WebSocket client %zu registered, total clients: %zu", 
                         wsc->id, ws_server.active_clients);
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
        
        netdata_log_debug(D_WEB_CLIENT, "WebSocket client %zu unregistered, total clients: %zu", 
                         wsc->id, ws_server.active_clients);
    }
    
    spinlock_unlock(&ws_server.spinlock);
}

// Find a WebSocket client by ID
WS_CLIENT *websocket_client_find_by_id(size_t id) {
    if (id == 0)
        return NULL;

    WS_CLIENT *wsc = NULL;
    
    spinlock_lock(&ws_server.spinlock);
    wsc = WS_CLIENTS_GET(&ws_server.clients, id);
    spinlock_unlock(&ws_server.spinlock);
    
    return wsc;
}

// Validate a WebSocket close code according to RFC 6455
bool websocket_validate_close_code(uint16_t code) {
    // 1000-2999 are reserved for the WebSocket protocol
    // 3000-3999 are reserved for use by libraries, frameworks, and applications
    // 4000-4999 are reserved for private use

    // Check if code is in valid ranges
    if ((code >= 1000 && code <= 1011) || // Protocol-defined codes
        (code >= 3000 && code <= 4999))   // Application/library/private codes
    {
        // Codes 1004, 1005, and 1006 must not be used in a Close frame by an endpoint
        if (code != 1004 && code != 1005 && code != 1006)
            return true;
    }

    return false;
}

// Helper structure for foreach callback
struct foreach_callback_params {
    void (*callback)(WS_CLIENT *wsc, void *data);
    void *data;
};

// Iterate through all WebSocket clients using JudyL FIRST/NEXT
void websocket_clients_foreach(void (*callback)(WS_CLIENT *wsc, void *data), void *data) {
    if (!callback)
        return;
    
    spinlock_lock(&ws_server.spinlock);
    
    Word_t index = 0;
    WS_CLIENT *wsc;
    
    // Get the first entry
    wsc = WS_CLIENTS_FIRST(&ws_server.clients, &index);
    
    // Iterate through all entries
    while (wsc) {
        callback(wsc, data);
        wsc = WS_CLIENTS_NEXT(&ws_server.clients, &index);
    }
    
    spinlock_unlock(&ws_server.spinlock);
}

// Get the count of active WebSocket clients
size_t websocket_clients_count(void) {
    size_t count;
    
    spinlock_lock(&ws_server.spinlock);
    count = ws_server.active_clients;
    spinlock_unlock(&ws_server.spinlock);
    
    return count;
}

// Helper structure for broadcast
struct broadcast_params {
    const char *message;
    size_t length;
    WEBSOCKET_OPCODE opcode;
    int success_count;
};

// Callback function for broadcast
static void websocket_broadcast_callback(WS_CLIENT *wsc, void *data) {
    struct broadcast_params *params = (struct broadcast_params *)data;
    
    if (wsc && wsc->state == WS_STATE_OPEN) {
        if (websocket_protocol_send_text(wsc, params->message) > 0)
            params->success_count++;
    }
}

// Broadcast a message to all connected WebSocket clients
int websocket_broadcast_message(const char *message, size_t length, WEBSOCKET_OPCODE opcode) {
    if (!message || length == 0 || (opcode != WS_OPCODE_TEXT && opcode != WS_OPCODE_BINARY))
        return -1;
    
    // Check if there are any active clients
    size_t client_count = websocket_clients_count();
    if (client_count == 0)
        return 0;
    
    // First, try the older method for backward compatibility - useful for testing
    if(!websocket_threads[0].thread) {
        struct broadcast_params params = {
            .message = message,
            .length = length,
            .opcode = opcode,
            .success_count = 0
        };
        
        // Use the foreach function to iterate through all clients
        websocket_clients_foreach(websocket_broadcast_callback, &params);
        
        return params.success_count;
    }
    
    // New approach: send broadcast command to all active threads
    int success_count = 0;
    
    // Prepare the broadcast command data
    size_t cmd_size = sizeof(size_t) + sizeof(WEBSOCKET_OPCODE) + length;
    char *cmd_data = mallocz(cmd_size);
    
    // Set message length
    memcpy(cmd_data, &length, sizeof(size_t));
    
    // Set opcode
    memcpy(cmd_data + sizeof(size_t), &opcode, sizeof(WEBSOCKET_OPCODE));
    
    // Set message
    memcpy(cmd_data + sizeof(size_t) + sizeof(WEBSOCKET_OPCODE), message, length);
    
    // Send broadcast command to all active threads
    for(size_t i = 0; i < WEBSOCKET_MAX_THREADS; i++) {
        if(websocket_threads[i].thread && websocket_threads[i].running) {
            if(websocket_thread_send_command(&websocket_threads[i], WEBSOCKET_THREAD_CMD_BROADCAST, 
                                           cmd_data, cmd_size)) {
                success_count++;
            }
        }
    }
    
    freez(cmd_data);
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
