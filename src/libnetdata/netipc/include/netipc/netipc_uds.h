/*
 * netipc_uds.h - L1 POSIX UDS SEQPACKET transport.
 *
 * Connection lifecycle, handshake, send/receive with transparent chunking.
 * Uses the wire envelope from netipc_protocol.h for all framing.
 */

#ifndef NETIPC_UDS_H
#define NETIPC_UDS_H

#include "netipc_protocol.h"
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ------------------------------------------------------------------ */
/*  Error codes (transport-level)                                      */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_UDS_OK = 0,
    NIPC_UDS_ERR_PATH_TOO_LONG,     /* socket path exceeds sun_path */
    NIPC_UDS_ERR_SOCKET,            /* socket()/bind()/listen() failed */
    NIPC_UDS_ERR_CONNECT,           /* connect() failed */
    NIPC_UDS_ERR_ACCEPT,            /* accept() failed */
    NIPC_UDS_ERR_SEND,              /* send() failed */
    NIPC_UDS_ERR_RECV,              /* recv() failed / peer disconnected */
    NIPC_UDS_ERR_HANDSHAKE,         /* handshake protocol error */
    NIPC_UDS_ERR_AUTH_FAILED,       /* auth token rejected */
    NIPC_UDS_ERR_NO_PROFILE,        /* no common profile */
    NIPC_UDS_ERR_INCOMPATIBLE,      /* protocol/layout version mismatch */
    NIPC_UDS_ERR_PROTOCOL,          /* wire protocol violation */
    NIPC_UDS_ERR_ADDR_IN_USE,       /* live server on the socket path */
    NIPC_UDS_ERR_CHUNK,             /* chunk header mismatch */
    NIPC_UDS_ERR_ALLOC,             /* memory allocation failed */
    NIPC_UDS_ERR_LIMIT_EXCEEDED,    /* payload/batch exceeds negotiated */
    NIPC_UDS_ERR_BAD_PARAM,         /* invalid argument */
    NIPC_UDS_ERR_DUPLICATE_MSG_ID,  /* message_id already in-flight */
    NIPC_UDS_ERR_UNKNOWN_MSG_ID,    /* response message_id not in-flight */
    NIPC_UDS_ERR_TIMEOUT,           /* receive deadline expired */
    NIPC_UDS_ERR_ABORTED,           /* receive aborted by caller */
} nipc_uds_error_t;

/* ------------------------------------------------------------------ */
/*  Role                                                               */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_UDS_ROLE_CLIENT = 1,
    NIPC_UDS_ROLE_SERVER = 2,
} nipc_uds_role_t;

/* ------------------------------------------------------------------ */
/*  Client connect configuration                                       */
/* ------------------------------------------------------------------ */

typedef struct {
    uint32_t supported_profiles;        /* bitmask */
    uint32_t preferred_profiles;        /* bitmask */
    uint32_t max_request_payload_bytes; /* 0 = use default */
    uint32_t max_request_batch_items;   /* 0 = use default (1) */
    uint32_t max_response_payload_bytes;/* 0 = use default */
    uint32_t max_response_batch_items;  /* 0 = use default (1) */
    uint64_t auth_token;
    uint32_t packet_size;              /* 0 = transport limit (SEQPACKET: SO_SNDBUF-32; stream: 256 KiB); capped at 256 KiB */
} nipc_uds_client_config_t;

/* ------------------------------------------------------------------ */
/*  Server configuration (for listen + accept)                         */
/* ------------------------------------------------------------------ */

typedef struct {
    uint32_t supported_profiles;
    uint32_t preferred_profiles;
    uint32_t max_request_payload_bytes;  /* 0 = use default */
    uint32_t max_request_batch_items;    /* 0 = use default (1) */
    uint32_t max_response_payload_bytes; /* 0 = use default */
    uint32_t max_response_batch_items;   /* 0 = use default (1) */
    uint64_t auth_token;                 /* expected token from clients */
    uint32_t packet_size;               /* 0 = transport limit (SEQPACKET: SO_SNDBUF-32; stream: 256 KiB); capped at 256 KiB */
    int      backlog;                   /* listen backlog, 0 = default (16) */
} nipc_uds_server_config_t;

/* ------------------------------------------------------------------ */
/*  Session                                                            */
/* ------------------------------------------------------------------ */

typedef struct {
    int             fd;             /* connected socket (native wait object) */
    nipc_uds_role_t role;

    /* Negotiated limits */
    uint32_t max_request_payload_bytes;
    uint32_t max_request_batch_items;
    uint32_t max_response_payload_bytes;
    uint32_t max_response_batch_items;
    uint32_t packet_size;
    uint32_t selected_profile;

    /* Server-assigned session ID (from hello-ack) */
    uint64_t session_id;

    /* Internal receive buffer for chunked reassembly */
    uint8_t *recv_buf;
    size_t   recv_buf_size;

    /* In-flight message_id set (client-side only, dynamically grown) */
    uint64_t *inflight_ids;
    uint32_t  inflight_count;
    uint32_t  inflight_capacity;

    /* Inbound packets read while a pipelined client send waited for
     * buffer space; receive consumes them before reading the socket. */
    uint8_t *pending_buf;
    size_t   pending_cap;
    size_t   pending_head;   /* next unconsumed byte */
    size_t   pending_tail;   /* end of the stored bytes */

    /* Stream baseline: a receive failed after reading part of a message,
     * so the stream is off a packet boundary and every later receive
     * fails. */
    bool recv_failed;
} nipc_uds_session_t;

/* ------------------------------------------------------------------ */
/*  Listener                                                           */
/* ------------------------------------------------------------------ */

typedef struct {
    int                      fd;      /* listening socket */
    nipc_uds_server_config_t config;
    char                     path[108]; /* stored for cleanup */
    bool                     has_lock; /* lock_fd holds the endpoint lock */
    int                      lock_fd;  /* {service_name}.lock, flock'ed */
} nipc_uds_listener_t;

/* ------------------------------------------------------------------ */
/*  Connection lifecycle                                               */
/* ------------------------------------------------------------------ */

/*
 * Create a listener on {run_dir}/{service_name}.sock.
 * First takes an exclusive flock on {run_dir}/{service_name}.lock, held
 * until nipc_uds_close_listener(); a held lock means another server owns
 * the endpoint (NIPC_UDS_ERR_ADDR_IN_USE). With the lock held, an existing
 * socket file no live server accepts on is stale: it is unlinked and
 * recreated.
 */
nipc_uds_error_t nipc_uds_listen(const char *run_dir,
                                  const char *service_name,
                                  const nipc_uds_server_config_t *config,
                                  nipc_uds_listener_t *out);

/*
 * Accept one client on a listener. Performs the full handshake.
 * session_id is placed into the hello-ack so the client can attach
 * to the correct per-session SHM region.
 * Blocks until a client connects; the handshake then waits at most
 * NIPC_SERVER_HANDSHAKE_TIMEOUT_MS for HELLO.
 */
nipc_uds_error_t nipc_uds_accept(nipc_uds_listener_t *listener,
                                  uint64_t session_id,
                                  nipc_uds_session_t *out);

/*
 * Accept one connection without handshaking, so the caller can run the
 * handshake on another thread. Blocks until a client connects. The
 * caller owns *fd_out and passes it to nipc_uds_accept_handshake().
 */
nipc_uds_error_t nipc_uds_accept_pending(nipc_uds_listener_t *listener,
                                          int *fd_out);

/*
 * Complete the server side of the handshake on a connection returned by
 * nipc_uds_accept_pending(). Waits at most timeout_ms for HELLO
 * (NIPC_UDS_ERR_TIMEOUT); 0 waits without a deadline. Takes ownership
 * of fd: on success it belongs to *out, on failure it is closed.
 */
nipc_uds_error_t nipc_uds_accept_handshake(int fd,
                                            const nipc_uds_server_config_t *config,
                                            uint64_t session_id,
                                            uint32_t timeout_ms,
                                            nipc_uds_session_t *out);

/*
 * Called by the server handshake once a profile is selected and the
 * payload limits are agreed, before HELLO_ACK is sent, so the caller can
 * set up profile resources (an SHM region) only for sessions that use
 * them. Returning false drops that profile and the handshake selects
 * again; with no profile left the client is rejected as unsupported.
 */
typedef bool (*nipc_uds_profile_prepare_fn)(void *user, uint32_t profile,
                                            uint32_t max_request_payload_bytes,
                                            uint32_t max_response_payload_bytes);

/* nipc_uds_accept_handshake() with a prepare step; prepare may be NULL. */
nipc_uds_error_t nipc_uds_accept_handshake_prepared(
    int fd, const nipc_uds_server_config_t *config, uint64_t session_id,
    uint32_t timeout_ms, nipc_uds_profile_prepare_fn prepare, void *user,
    nipc_uds_session_t *out);

/*
 * Connect to a server at {run_dir}/{service_name}.sock.
 * Performs the full handshake. Blocks until connected + handshake done.
 */
nipc_uds_error_t nipc_uds_connect(const char *run_dir,
                                   const char *service_name,
                                   const nipc_uds_client_config_t *config,
                                   nipc_uds_session_t *out);

/*
 * nipc_uds_connect() bounded by timeout_ms: connect and the wait for
 * HELLO_ACK together end with NIPC_UDS_ERR_TIMEOUT after timeout_ms
 * (0 = no deadline), and with NIPC_UDS_ERR_ABORTED once abort_fd becomes
 * readable (-1 = no abort).
 */
nipc_uds_error_t nipc_uds_connect_timeout(const char *run_dir,
                                           const char *service_name,
                                           const nipc_uds_client_config_t *config,
                                           uint32_t timeout_ms,
                                           int abort_fd,
                                           nipc_uds_session_t *out);

/*
 * Close a session. Releases socket and internal buffers.
 * Safe to call on a zero-initialized session (no-op).
 */
void nipc_uds_close_session(nipc_uds_session_t *session);

/*
 * Stop accepting: close the listening socket and unlink the socket file,
 * keeping the endpoint lock, so no other server takes the endpoint while
 * this one still serves its sessions. Idempotent.
 */
void nipc_uds_stop_listening(nipc_uds_listener_t *listener);

/*
 * Close a listener: stop listening if still listening, then release the
 * endpoint lock. Idempotent.
 */
void nipc_uds_close_listener(nipc_uds_listener_t *listener);

/* ------------------------------------------------------------------ */
/*  Message send / receive                                             */
/* ------------------------------------------------------------------ */

/*
 * Send one logical message. hdr is the 32-byte outer header (caller fills
 * kind, code, flags, payload_len, item_count, message_id; this function
 * sets magic/version/header_len). payload is the opaque payload bytes.
 *
 * If the total message (32 + payload_len) exceeds packet_size, the
 * message is chunked transparently.
 *
 * Blocks until all chunks are sent or an error occurs. A client with other
 * requests in flight keeps reading their responses into the session while
 * it waits for buffer space, so pipelining never deadlocks with a server
 * that is blocked sending to it; a later nipc_uds_receive() returns them.
 * Inbound bytes beyond what the in-flight responses can occupy fail the
 * session with NIPC_UDS_ERR_PROTOCOL.
 */
nipc_uds_error_t nipc_uds_send(nipc_uds_session_t *session,
                                nipc_header_t *hdr,
                                const void *payload,
                                size_t payload_len);

/*
 * nipc_uds_send() with an optional deadline and abort fd for a client
 * session. timeout_ms == 0 waits without a deadline; abort_fd < 0 disables
 * abort waiting. While the socket lacks buffer space the send reads the
 * peer's bytes as a pipelined send does. If the deadline passes or abort_fd
 * becomes readable before the message is sent, returns NIPC_UDS_ERR_TIMEOUT
 * or NIPC_UDS_ERR_ABORTED and shuts the session down, since part of the
 * message may already be on the wire.
 */
nipc_uds_error_t nipc_uds_send_timeout(nipc_uds_session_t *session,
                                        nipc_header_t *hdr,
                                        const void *payload,
                                        size_t payload_len,
                                        uint32_t timeout_ms,
                                        int abort_fd);

/*
 * Receive one logical message. Blocks until a complete message arrives.
 *
 * On success:
 *   - hdr_out is filled with the decoded outer header.
 *   - *payload_out points to the payload bytes (inside session->recv_buf
 *     or buf). Valid until the next nipc_uds_receive call.
 *   - *payload_len_out is the payload length.
 *
 * buf/buf_size: caller-provided buffer. If the message fits, it is
 * placed here. If the message is chunked and exceeds buf_size, the
 * session's internal recv_buf is grown to fit (allocated via realloc).
 */
nipc_uds_error_t nipc_uds_receive(nipc_uds_session_t *session,
                                   void *buf, size_t buf_size,
                                   nipc_header_t *hdr_out,
                                   const void **payload_out,
                                   size_t *payload_len_out);

/*
 * Receive one logical message with an optional deadline and abort fd.
 *
 * timeout_ms == 0 waits indefinitely. abort_fd < 0 disables abort waiting.
 * If abort_fd becomes readable before a complete message arrives, returns
 * NIPC_UDS_ERR_ABORTED.
 */
nipc_uds_error_t nipc_uds_receive_timeout(nipc_uds_session_t *session,
                                           void *buf, size_t buf_size,
                                           nipc_header_t *hdr_out,
                                           const void **payload_out,
                                           size_t *payload_len_out,
                                           uint32_t timeout_ms,
                                           int abort_fd);

/* ------------------------------------------------------------------ */
/*  Utility                                                            */
/* ------------------------------------------------------------------ */

/* Get the fd for poll/epoll integration. A pipelined send can read
 * responses into the session's pending queue, where the fd no longer shows
 * them: check nipc_uds_session_has_pending() before waiting on the fd. */
static inline int nipc_uds_session_fd(const nipc_uds_session_t *s) {
    return s->fd;
}

/* True when the next receive can return data read earlier by a pipelined
 * send, without the fd becoming readable. */
static inline bool nipc_uds_session_has_pending(const nipc_uds_session_t *s) {
    return !s->recv_failed && s->pending_tail > s->pending_head;
}

static inline int nipc_uds_listener_fd(const nipc_uds_listener_t *l) {
    return l->fd;
}

#ifdef NIPC_INTERNAL_TESTING
/* Run hook(user) inside nipc_uds_listen() between bind() and listen(), so
 * tests can hold a listener in that window. Process-wide; NULL clears. */
void nipc_uds_set_listen_hook_for_tests(void (*hook)(void *user), void *user);

/* Run hook(tmp_path, user) just before a new endpoint lock file is linked to
 * its final name. Process-wide; NULL clears. */
void nipc_uds_set_lock_publish_hook_for_tests(
    void (*hook)(const char *tmp_path, void *user), void *user);
#endif

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_UDS_H */
