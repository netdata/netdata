// SPDX-License-Identifier: GPL-3.0-or-later

#include "web/server/web_client.h"
#include "websocket-internal.h"
#include "websocket-jsonrpc.h"
#include "websocket-echo.h"
#include "../mcp/adapters/mcp-websocket.h"
#include "web/api/mcp_auth.h"

// Global array of WebSocket threads
WEBSOCKET_THREAD websocket_threads[WEBSOCKET_MAX_THREADS];

// Initialize WebSocket thread system
void websocket_threads_init(void) {
    for(size_t i = 0; i < WEBSOCKET_MAX_THREADS; i++) {
        websocket_threads[i].id = i;
        websocket_threads[i].thread = NULL;
        websocket_threads[i].running = false;
        spinlock_init(&websocket_threads[i].spinlock);
        websocket_threads[i].clients_current = 0;
        spinlock_init(&websocket_threads[i].clients_spinlock);
        websocket_threads[i].clients = NULL;
        websocket_threads[i].ndpl = NULL;
        websocket_threads[i].cmd.pipe[PIPE_READ] = -1;
        websocket_threads[i].cmd.pipe[PIPE_WRITE] = -1;
    }
}

// Find the thread with the minimum client load and atomically increment its count
NEVERNULL
static WEBSOCKET_THREAD *websocket_thread_get_min_load(void) {
    // Static spinlock to protect the critical section of thread selection
    static SPINLOCK assign_spinlock = SPINLOCK_INITIALIZER;
    size_t slot = 0;

    // Critical section: find thread with minimum load and increment its count atomically
    spinlock_lock(&assign_spinlock);

    // Find the minimum load thread
    size_t min_clients = websocket_threads[0].clients_current;

    for(size_t i = 1; i < WEBSOCKET_MAX_THREADS; i++) {
        // Check if this thread has fewer clients
        if(websocket_threads[i].clients_current < min_clients) {
            min_clients = websocket_threads[i].clients_current;
            slot = i;
        }
    }

    // Preemptively increment the client count to prevent race conditions
    // This ensures concurrent client assignments will be properly distributed
    websocket_threads[slot].clients_current++;

    spinlock_unlock(&assign_spinlock);

    return &websocket_threads[slot];
}

// Handle socket takeover from web client - similar to stream_receiver_takeover_web_connection
static void websocket_takeover_web_connection(struct web_client *w, WS_CLIENT *wsc) {
    // Set the file descriptor and ssl from the web client
    wsc->sock.fd = w->fd;
    wsc->sock.ssl = w->ssl;

    w->ssl = NETDATA_SSL_UNSET_CONNECTION;

    WEB_CLIENT_IS_DEAD(w);

    if(web_server_mode == WEB_SERVER_MODE_STATIC_THREADED) {
        web_client_flag_set(w, WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET);
    }
    else {
        w->fd = -1;
    }

    // Clear web client buffer
    buffer_flush(w->response.data);

    web_server_remove_current_socket_from_poll();
}

// Initialize a thread's poll
static bool websocket_thread_init_poll(WEBSOCKET_THREAD *wth) {
    // Create poll instance
    if(!wth->ndpl) {
        wth->ndpl = nd_poll_create();
        if (!wth->ndpl) {
            netdata_log_error("WEBSOCKET[%zu]: Failed to create poll", wth->id);
            goto cleanup;
        }
    }

    // Create command pipe
    if(wth->cmd.pipe[PIPE_READ] == -1 || wth->cmd.pipe[PIPE_WRITE] == -1) {
        if (pipe(wth->cmd.pipe) == -1) {
            netdata_log_error("WEBSOCKET[%zu]: Failed to create command pipe: %s", wth->id, strerror(errno));
            goto cleanup;
        }

        // Set pipe to non-blocking
        if(fcntl(wth->cmd.pipe[PIPE_READ], F_SETFL, O_NONBLOCK) == -1) {
            netdata_log_error("WEBSOCKET[%zu]: Failed to set command pipe to non-blocking: %s", wth->id, strerror(errno));
            goto cleanup;
        }

        // Add command pipe to poll
        bool added = nd_poll_add(wth->ndpl, wth->cmd.pipe[PIPE_READ], ND_POLL_READ, &wth->cmd);
        if(!added) {
            netdata_log_error("WEBSOCKET[%zu]: Failed to add command pipe to poll", wth->id);
            goto cleanup;
        }
    }

    return true;

cleanup:
    if(wth->cmd.pipe[PIPE_READ] != -1) {
        close(wth->cmd.pipe[PIPE_READ]);
        wth->cmd.pipe[PIPE_READ] = -1;
    }
    if(wth->cmd.pipe[PIPE_WRITE] != -1) {
        close(wth->cmd.pipe[PIPE_WRITE]);
        wth->cmd.pipe[PIPE_WRITE] = -1;
    }
    if(wth->ndpl) {
        nd_poll_destroy(wth->ndpl);
        wth->ndpl = NULL;
    }
    return false;
}

// Assign a client to a thread
static WEBSOCKET_THREAD *websocket_thread_assign_client(WS_CLIENT *wsc) {
    // Get the thread with the minimum load
    // Note: client count is already atomically incremented inside this function
    WEBSOCKET_THREAD *wth = websocket_thread_get_min_load();

    // Lock the thread for initialization
    spinlock_lock(&wth->spinlock);

    // Start the thread if not running
    if(!wth->thread) {
        // Initialize poll
        if(!websocket_thread_init_poll(wth)) {
            spinlock_unlock(&wth->spinlock);
            netdata_log_error("WEBSOCKET[%zu]: Failed to initialize poll", wth->id);
            goto undo;
        }

        char thread_name[32];
        snprintf(thread_name, sizeof(thread_name), "WEBSOCK[%zu]", wth->id);
        wth->thread = nd_thread_create(thread_name, NETDATA_THREAD_OPTION_DEFAULT, websocket_thread, wth);
        wth->running = true;
    }

    // Release the thread lock
    spinlock_unlock(&wth->spinlock);

    // Link thread to client
    wsc->wth = wth;

    // Send command to add client
    if(!websocket_thread_send_command(wth, WEBSOCKET_THREAD_CMD_ADD_CLIENT, wsc->id)) {
        netdata_log_error("WEBSOCKET[%zu]: Failed to send add client command", wth->id);
        goto undo;
    }

    return wth;

undo:
    // Roll back the client count increment since assignment failed
    wsc->wth = NULL;

    spinlock_lock(&wth->clients_spinlock);
    if (wth->clients_current > 0)
        wth->clients_current--;
    spinlock_unlock(&wth->clients_spinlock);

    return NULL;
}

// Cancel all WebSocket threads
void websocket_threads_join(void) {
    for(size_t i = 0; i < WEBSOCKET_MAX_THREADS; i++) {
        if(websocket_threads[i].thread) {
            // Send exit command
            websocket_thread_send_command(&websocket_threads[i], WEBSOCKET_THREAD_CMD_EXIT, 0);

            // Signal thread to cancel
            nd_thread_signal_cancel(websocket_threads[i].thread);
        }
    }

    // Wait for all threads to exit
    for(size_t i = 0; i < WEBSOCKET_MAX_THREADS; i++) {
        if(websocket_threads[i].thread) {
            nd_thread_join(websocket_threads[i].thread);
            websocket_threads[i].thread = NULL;
            websocket_threads[i].running = false;
        }
    }
}

// Check if the current HTTP request is a WebSocket handshake request
static bool websocket_detect_handshake_request(struct web_client *w) {
    // We need a valid key and to be flagged as a WebSocket request
    if (!web_client_is_websocket(w) || !w->websocket.key)
        return false;

    return true;
}

// Generate the WebSocket accept key as per RFC 6455
static char *websocket_generate_handshake_key(const char *client_key) {
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

static bool websocket_send_first_response(WS_CLIENT *wsc, const char *accept_key, WEBSOCKET_EXTENSION ext_flags, bool url_protocol) {
    CLEAN_BUFFER *wb = buffer_create(1024, NULL);

    buffer_sprintf(wb,
                   "HTTP/1.1 101 Switching Protocols\r\n"
                   "Server: Netdata\r\n"
                   "Upgrade: websocket\r\n"
                   "Connection: Upgrade\r\n"
                   "Sec-WebSocket-Accept: %s\r\n",
                   accept_key
    );

    // Add the selected subprotocol
    if(!url_protocol && wsc->protocol != WS_PROTOCOL_UNKNOWN && wsc->protocol != WS_PROTOCOL_DEFAULT)
        buffer_sprintf(wb, "Sec-WebSocket-Protocol: %s\r\n", WEBSOCKET_PROTOCOL_2str(wsc->protocol));

    switch (wsc->compression.type) {
        case WS_COMPRESS_DEFLATE:
            buffer_strcat(wb, "Sec-WebSocket-Extensions: permessage-deflate");

            // Add parameters if different from defaults
            if (!wsc->compression.client_context_takeover)
                buffer_strcat(wb, "; client_no_context_takeover");

            if (!wsc->compression.server_context_takeover)
                buffer_strcat(wb, "; server_no_context_takeover");

            if(ext_flags & WS_EXTENSION_SERVER_MAX_WINDOW_BITS)
                buffer_sprintf(wb, "; server_max_window_bits=%d", wsc->compression.server_max_window_bits);

            if(ext_flags & WS_EXTENSION_CLIENT_MAX_WINDOW_BITS)
                buffer_sprintf(wb, "; client_max_window_bits=%d", wsc->compression.client_max_window_bits);

            buffer_strcat(wb, "\r\n");
            break;

        default:
            break;
    }

    // End of headers
    buffer_strcat(wb, "Sec-WebSocket-Version: 13\r\n");
    buffer_strcat(wb, "\r\n");

    // Send the handshake response using ND_SOCK - we're still in the web server thread,
    // so we need to use the persist version to ensure the complete handshake is sent
    const char *header_str = buffer_tostring(wb);
    size_t header_len = buffer_strlen(wb);
    ssize_t bytes = nd_sock_write_persist(&wsc->sock, header_str, header_len, 20);

    websocket_debug(wsc, "Sent WebSocket handshake response: %zd bytes out of %zu bytes", bytes, header_len);
    return bytes == (ssize_t)header_len;
}

// Handle the WebSocket handshake procedure
short int websocket_handle_handshake(struct web_client *w) {
    web_client_ensure_proper_authorization(w);

    if (!websocket_detect_handshake_request(w))
        return HTTP_RESP_BAD_REQUEST;

    // Generate the accept key
    char *accept_key = websocket_generate_handshake_key(w->websocket.key);
    if (!accept_key)
        return HTTP_RESP_INTERNAL_SERVER_ERROR;

    // Create the WebSocket client object early so we can set up compression
    WS_CLIENT *wsc = websocket_client_create();

    // Copy client information
    strncpyz(wsc->client_ip, w->user_auth.client_ip, sizeof(wsc->client_ip));
    strncpyz(wsc->client_port, w->client_port, sizeof(wsc->client_port));

    // Copy user authentication and authorization information
    wsc->user_auth = w->user_auth;

    // Check for max_frame_size parameter in the URL query string
    if (w->url_query_string_decoded && buffer_strlen(w->url_query_string_decoded) > 0) {
        const char *query = buffer_tostring(w->url_query_string_decoded);
        char *max_frame_size_str = strstr(query, "max_frame_size=");
        
        if (max_frame_size_str) {
            max_frame_size_str += strlen("max_frame_size=");
            
            char *end_ptr;
            size_t max_frame_size = strtoull(max_frame_size_str, &end_ptr, 10);
            
            // Validate the max frame size with reasonable bounds
            if (max_frame_size > 0) {
                // Set minimum and maximum limits
                if (max_frame_size < 1024) // Minimum 1KB
                    max_frame_size = 1024;
                else if (max_frame_size > (20ULL * 1024 * 1024)) // Maximum 20MB
                    max_frame_size = 20ULL * 1024 * 1024;
                
                // Set the client's max outbound frame size
                wsc->max_outbound_frame_size = max_frame_size;
                websocket_debug(wsc, "Setting custom max outbound frame size: %zu bytes", max_frame_size);
            }
        }
        
#ifdef NETDATA_MCP_DEV_PREVIEW_API_KEY
        if (web_client_has_mcp_preview_key(w)) {
            wsc->user_auth.access = HTTP_ACCESS_ALL;
            wsc->user_auth.method = USER_AUTH_METHOD_GOD;
            wsc->user_auth.user_role = HTTP_USER_ROLE_ADMIN;
            websocket_debug(wsc, "MCP developer preview API key verified via Authorization header - enabling full access");
        } else {
            // Check for api_key parameter for MCP developer preview
            char *api_key_str = strstr(query, "api_key=");
            if (api_key_str) {
                api_key_str += strlen("api_key=");

                // Extract the API key value (until & or end of string)
                char api_key_buffer[MCP_DEV_PREVIEW_API_KEY_LENGTH + 1];
                size_t i = 0;
                while (api_key_str[i] && api_key_str[i] != '&' && i < MCP_DEV_PREVIEW_API_KEY_LENGTH) {
                    api_key_buffer[i] = api_key_str[i];
                    i++;
                }
                api_key_buffer[i] = '\0';

                // Verify the API key
                if (mcp_api_key_verify(api_key_buffer)) {
                    // Override authentication with god mode
                    wsc->user_auth.access = HTTP_ACCESS_ALL;
                    wsc->user_auth.method = USER_AUTH_METHOD_GOD;
                    wsc->user_auth.user_role = HTTP_USER_ROLE_ADMIN;
                    websocket_debug(wsc, "MCP developer preview API key verified - enabling full access");
                } else {
                    websocket_debug(wsc, "Invalid MCP developer preview API key provided");
                }
            }
        }
#endif
    }

    bool url_protocol = false;
    wsc->protocol = w->websocket.protocol;

    if(wsc->protocol == WS_PROTOCOL_DEFAULT) {
        const char *path = buffer_tostring(w->url_path_decoded);
        if (path && path[0] == '/' && path[1])
            wsc->protocol = WEBSOCKET_PROTOCOL_2id(&path[1]);

        url_protocol = true;
    }

    // If no protocol is selected by either URL or subprotocol, reject the connection
    if(wsc->protocol == WS_PROTOCOL_UNKNOWN || wsc->protocol == WS_PROTOCOL_DEFAULT) {
        netdata_log_error("WEBSOCKET: No valid protocol selected by either URL or subprotocol");
        freez(accept_key);
        websocket_client_free(wsc);
        return HTTP_RESP_BAD_REQUEST;
    }

    // Take over the connection immediately
    websocket_takeover_web_connection(w, wsc);

    if((w->websocket.ext_flags & WS_EXTENSION_PERMESSAGE_DEFLATE)) {
        wsc->compression.enabled = true;
        wsc->compression.type = WS_COMPRESS_DEFLATE;

        if (w->websocket.ext_flags & WS_EXTENSION_CLIENT_NO_CONTEXT_TAKEOVER)
            wsc->compression.client_context_takeover = false;
        else
            wsc->compression.client_context_takeover = true;

        if (w->websocket.ext_flags & WS_EXTENSION_SERVER_NO_CONTEXT_TAKEOVER)
            wsc->compression.server_context_takeover = false;
        else
            wsc->compression.server_context_takeover = true;

        // Set window bits for both client-to-server and server-to-client directions
        wsc->compression.client_max_window_bits = w->websocket.client_max_window_bits ? w->websocket.client_max_window_bits : WS_COMPRESS_WINDOW_BITS;
        wsc->compression.server_max_window_bits = w->websocket.server_max_window_bits ? w->websocket.server_max_window_bits : WS_COMPRESS_WINDOW_BITS;
    }

    if(!websocket_send_first_response(wsc, accept_key, w->websocket.ext_flags, url_protocol)) {
        netdata_log_error("WEBSOCKET: Failed to send complete WebSocket handshake response"); // No client yet
        freez(accept_key);
        websocket_client_free(wsc);
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    freez(accept_key);

    // Now that we've sent the handshake response successfully, set the connection state to open
    wsc->state = WS_STATE_OPEN;

    // Set up protocol-specific callbacks based on the selected protocol
    switch (wsc->protocol) {
        case WS_PROTOCOL_MCP:
            // Set up callbacks for MCP protocol
            wsc->on_connect = mcp_websocket_on_connect;
            wsc->on_message = mcp_websocket_on_message;
            wsc->on_close = mcp_websocket_on_close;
            wsc->on_disconnect = mcp_websocket_on_disconnect;
            websocket_debug(wsc, "Setting up MCP protocol callbacks");
            break;

#ifdef NETDATA_INTERNAL_CHECKS
        case WS_PROTOCOL_JSONRPC:
            // Set up callbacks for jsonrpc protocol
            wsc->on_connect = jsonrpc_on_connect;
            wsc->on_message = jsonrpc_on_message_callback;
            wsc->on_close = jsonrpc_on_close;
            wsc->on_disconnect = jsonrpc_on_disconnect;
            websocket_debug(wsc, "Setting up jsonrpc protocol callbacks");
            break;

        case WS_PROTOCOL_ECHO:
            // Set up callbacks for echo protocol
            wsc->on_connect = echo_on_connect;
            wsc->on_message = echo_on_message_callback;
            wsc->on_close = echo_on_close;
            wsc->on_disconnect = echo_on_disconnect;
            websocket_debug(wsc, "Setting up echo protocol callbacks");
            break;
#endif

        default:
            // No protocol handler available - this shouldn't happen as we check earlier
            netdata_log_error("WEBSOCKET: No handler available for protocol %d", wsc->protocol);
            websocket_client_free(wsc);
            return HTTP_RESP_BAD_REQUEST;
    }

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

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "WebSocket connection established with %s:%s using protocol: %s (client ID: %u, thread: %zu), "
           "compression: %s (client context takeover: %s, server context takeover: %s, "
           "client window bits: %d, server window bits: %d), "
           "max outbound frame size: %zu bytes",
           wsc->client_ip, wsc->client_port,
           WEBSOCKET_PROTOCOL_2str(wsc->protocol),
           wsc->id, wth->id,
           wsc->compression.enabled ? "enabled" : "disabled",
           wsc->compression.client_context_takeover ? "enabled" : "disabled",
           wsc->compression.server_context_takeover ? "enabled" : "disabled",
           wsc->compression.client_max_window_bits,
           wsc->compression.server_max_window_bits,
           wsc->max_outbound_frame_size);

    // Important: This code doesn't actually get sent to the client since we've already
    // taken over the socket. It's just used by the caller to identify what happened.
    return HTTP_RESP_WEBSOCKET_HANDSHAKE;
}
