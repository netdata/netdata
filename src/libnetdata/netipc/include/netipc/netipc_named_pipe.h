/*
 * netipc_named_pipe.h - L1 Windows Named Pipe transport.
 *
 * Connection lifecycle, handshake, send/receive with transparent chunking
 * over Win32 Named Pipes in message mode. Uses the wire envelope from
 * netipc_protocol.h for all framing.
 *
 * Pipe name derivation:
 *   \\.\pipe\netipc-{FNV1a64(run_dir):016llx}-{service_name}
 */

#ifndef NETIPC_NAMED_PIPE_H
#define NETIPC_NAMED_PIPE_H

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc_protocol.h"
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <windows.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ------------------------------------------------------------------ */
/*  Constants                                                         */
/* ------------------------------------------------------------------ */

/* Maximum pipe name length (\\.\pipe\ prefix + hash + service) */
#define NIPC_NP_MAX_PIPE_NAME 256

/* Default pipe buffer size and packet size */
#define NIPC_NP_DEFAULT_PIPE_BUF_SIZE 65536
#define NIPC_NP_DEFAULT_PACKET_SIZE   65536
#define NIPC_NP_DEFAULT_BATCH_ITEMS   1

/* Max concurrent pipe instances */
#define NIPC_NP_MAX_INSTANCES PIPE_UNLIMITED_INSTANCES

/* How long a connect waits for a free pipe instance when every instance is
 * busy (ERROR_PIPE_BUSY). */
#define NIPC_NP_CONNECT_BUSY_WAIT_MS 1000

/* FNV-1a 64-bit constants */
#define NIPC_FNV1A_OFFSET_BASIS 0xcbf29ce484222325ULL
#define NIPC_FNV1A_PRIME        0x00000100000001B3ULL

/* ------------------------------------------------------------------ */
/*  Error codes (transport-level)                                      */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_NP_OK = 0,
    NIPC_NP_ERR_PIPE_NAME,          /* pipe name derivation failed */
    NIPC_NP_ERR_CREATE_PIPE,        /* CreateNamedPipeW failed */
    NIPC_NP_ERR_CONNECT,            /* ConnectNamedPipe / CreateFileW failed */
    NIPC_NP_ERR_ACCEPT,             /* ConnectNamedPipe failed waiting for client */
    NIPC_NP_ERR_SEND,               /* WriteFile failed */
    NIPC_NP_ERR_RECV,               /* ReadFile failed / peer disconnected */
    NIPC_NP_ERR_HANDSHAKE,          /* handshake protocol error */
    NIPC_NP_ERR_AUTH_FAILED,        /* auth token rejected */
    NIPC_NP_ERR_NO_PROFILE,         /* no common profile */
    NIPC_NP_ERR_INCOMPATIBLE,       /* protocol/layout version mismatch */
    NIPC_NP_ERR_PROTOCOL,           /* wire protocol violation */
    NIPC_NP_ERR_ADDR_IN_USE,        /* pipe name already in use */
    NIPC_NP_ERR_CHUNK,              /* chunk header mismatch */
    NIPC_NP_ERR_ALLOC,              /* memory allocation failed */
    NIPC_NP_ERR_LIMIT_EXCEEDED,     /* payload/batch exceeds negotiated */
    NIPC_NP_ERR_BAD_PARAM,          /* invalid argument */
    NIPC_NP_ERR_DUPLICATE_MSG_ID,   /* message_id already in-flight */
    NIPC_NP_ERR_UNKNOWN_MSG_ID,     /* response message_id not in-flight */
    NIPC_NP_ERR_DISCONNECTED,       /* peer disconnected (graceful) */
    NIPC_NP_ERR_TIMEOUT,            /* receive deadline expired */
    NIPC_NP_ERR_ABORTED,            /* receive aborted by caller */
    NIPC_NP_ERR_UNTRUSTED_SERVER,   /* pipe owner is not this account,
                                       SYSTEM or Administrators */
} nipc_np_error_t;

/* ------------------------------------------------------------------ */
/*  Role                                                               */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_NP_ROLE_CLIENT = 1,
    NIPC_NP_ROLE_SERVER = 2,
} nipc_np_role_t;

/* ------------------------------------------------------------------ */
/*  Client connect configuration                                       */
/* ------------------------------------------------------------------ */

typedef struct {
    uint32_t supported_profiles;
    uint32_t preferred_profiles;
    uint32_t max_request_payload_bytes;  /* 0 = use default */
    uint32_t max_request_batch_items;    /* 0 = use default (1) */
    uint32_t max_response_payload_bytes; /* 0 = use default */
    uint32_t max_response_batch_items;   /* 0 = use default (1) */
    uint64_t auth_token;
    uint32_t packet_size;               /* 0 = use default (65536) */
} nipc_np_client_config_t;

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
    uint64_t auth_token;
    uint32_t packet_size;               /* 0 = use default (65536) */
} nipc_np_server_config_t;

/* ------------------------------------------------------------------ */
/*  Session                                                            */
/* ------------------------------------------------------------------ */

typedef struct {
    HANDLE          pipe;    /* connected pipe handle */
    nipc_np_role_t  role;

    /* Negotiated limits */
    uint32_t max_request_payload_bytes;
    uint32_t max_request_batch_items;
    uint32_t max_response_payload_bytes;
    uint32_t max_response_batch_items;
    uint32_t packet_size;
    uint32_t selected_profile;
    uint64_t session_id;             /* server-assigned, for per-session SHM */

    /* Internal receive buffer for chunked reassembly */
    uint8_t *recv_buf;
    size_t   recv_buf_size;

    /* In-flight message_id set (client-side only, dynamically grown) */
    uint64_t *inflight_ids;
    uint32_t  inflight_count;
    uint32_t  inflight_capacity;

    /* Events of the overlapped client handle's reads and writes; NULL on
     * server sessions, whose handles are synchronous. */
    HANDLE read_event;
    HANDLE write_event;

    /* Messages read while a pipelined send waited for its write, each a
     * 4-byte length then its bytes; receive returns them first. */
    uint8_t *pending_buf;
    size_t   pending_len;
    size_t   pending_head;
    size_t   pending_cap;
    /* Queue bound of the send in progress; 0 writes without draining. */
    size_t   progress_bound;
} nipc_np_session_t;

/* ------------------------------------------------------------------ */
/*  Listener                                                           */
/* ------------------------------------------------------------------ */

typedef struct {
    HANDLE                  pipe;   /* current listening pipe instance */
    nipc_np_server_config_t config;
    wchar_t                 pipe_name[NIPC_NP_MAX_PIPE_NAME]; /* stored for new instances */
} nipc_np_listener_t;

/* ------------------------------------------------------------------ */
/*  FNV-1a 64-bit hash                                                 */
/* ------------------------------------------------------------------ */

/* Compute FNV-1a 64-bit hash of data[0..len). */
uint64_t nipc_fnv1a_64(const void *data, size_t len);

/* ------------------------------------------------------------------ */
/*  Pipe name derivation                                               */
/* ------------------------------------------------------------------ */

/*
 * Build pipe name into dst (wide string).
 * Format: \\.\pipe\netipc-{FNV1a64(run_dir):016llx}-{service_name}
 * Returns 0 on success, -1 on error.
 */
int nipc_np_build_pipe_name(wchar_t *dst, size_t dst_chars,
                             const char *run_dir,
                             const char *service_name);

/* ------------------------------------------------------------------ */
/*  Connection lifecycle                                               */
/* ------------------------------------------------------------------ */

/*
 * Create a listener on a Named Pipe derived from run_dir + service_name.
 * Creates the first pipe instance and prepares for client connections.
 */
nipc_np_error_t nipc_np_listen(const char *run_dir,
                                const char *service_name,
                                const nipc_np_server_config_t *config,
                                nipc_np_listener_t *out);

/*
 * Accept one client on a listener. Performs the full handshake.
 * session_id is placed into the hello-ack so the client can attach
 * to the correct per-session SHM region.
 * Blocks until a client connects; the handshake then waits at most
 * NIPC_SERVER_HANDSHAKE_TIMEOUT_MS for HELLO.
 * After accepting, creates a new pipe instance for subsequent clients.
 */
nipc_np_error_t nipc_np_accept(nipc_np_listener_t *listener,
                                uint64_t session_id,
                                nipc_np_session_t *out);

/*
 * Accept one connection without handshaking, so the caller can run the
 * handshake on another thread. Blocks until a client connects, then
 * creates a new pipe instance for subsequent clients. The caller owns
 * *pipe_out and passes it to nipc_np_accept_handshake().
 */
nipc_np_error_t nipc_np_accept_pending(nipc_np_listener_t *listener,
                                       HANDLE *pipe_out);

/*
 * Complete the server side of the handshake on a pipe returned by
 * nipc_np_accept_pending(). Waits at most timeout_ms for HELLO
 * (NIPC_NP_ERR_TIMEOUT); 0 waits without a deadline. Takes ownership of
 * pipe: on success it belongs to *out, on failure it is disconnected
 * and closed.
 */
nipc_np_error_t nipc_np_accept_handshake(HANDLE pipe,
                                         const nipc_np_server_config_t *config,
                                         uint64_t session_id,
                                         uint32_t timeout_ms,
                                         nipc_np_session_t *out);

/*
 * Called by the server handshake once a profile is selected and the
 * payload limits are agreed, before HELLO_ACK is sent, so the caller can
 * set up profile resources (an SHM region) only for sessions that use
 * them. Returning false drops that profile and the handshake selects
 * again; with no profile left the client is rejected as unsupported.
 */
typedef bool (*nipc_np_profile_prepare_fn)(void *user, uint32_t profile,
                                           uint32_t max_request_payload_bytes,
                                           uint32_t max_response_payload_bytes);

/* nipc_np_accept_handshake() with a prepare step; prepare may be NULL. */
nipc_np_error_t nipc_np_accept_handshake_prepared(
    HANDLE pipe, const nipc_np_server_config_t *config, uint64_t session_id,
    uint32_t timeout_ms, nipc_np_profile_prepare_fn prepare, void *user,
    nipc_np_session_t *out);

/*
 * Connect to a server pipe derived from run_dir + service_name.
 * Performs the full handshake. Blocks until connected + handshake done.
 * When every pipe instance is busy, waits up to NIPC_NP_CONNECT_BUSY_WAIT_MS
 * for a free one.
 */
nipc_np_error_t nipc_np_connect(const char *run_dir,
                                 const char *service_name,
                                 const nipc_np_client_config_t *config,
                                 nipc_np_session_t *out);

/*
 * nipc_np_connect() bounded by timeout_ms: the busy-pipe wait and the wait
 * for HELLO_ACK together end with NIPC_NP_ERR_TIMEOUT after timeout_ms
 * (0 = no deadline), and with NIPC_NP_ERR_ABORTED once abort_event is
 * signaled (NULL = no abort).
 */
nipc_np_error_t nipc_np_connect_timeout(const char *run_dir,
                                        const char *service_name,
                                        const nipc_np_client_config_t *config,
                                        uint32_t timeout_ms,
                                        HANDLE abort_event,
                                        nipc_np_session_t *out);

/*
 * Close a session. Releases pipe handle and internal buffers.
 * Safe to call on a zero-initialized session (no-op).
 */
void nipc_np_close_session(nipc_np_session_t *session);

/*
 * Close a listener. Closes the pipe handle.
 */
void nipc_np_close_listener(nipc_np_listener_t *listener);

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
 */
nipc_np_error_t nipc_np_send(nipc_np_session_t *session,
                              nipc_header_t *hdr,
                              const void *payload,
                              size_t payload_len);

/*
 * nipc_np_send() with an optional deadline and abort event for a client
 * session. timeout_ms == 0 waits without a deadline; abort_event == NULL
 * disables abort waiting. If the deadline passes or abort_event is signaled
 * before a write completes, the write is cancelled, the session is closed
 * (later calls fail with NIPC_NP_ERR_BAD_PARAM) and the call returns
 * NIPC_NP_ERR_TIMEOUT or NIPC_NP_ERR_ABORTED, since part of the message may
 * already be on the wire.
 */
nipc_np_error_t nipc_np_send_timeout(nipc_np_session_t *session,
                                      nipc_header_t *hdr,
                                      const void *payload,
                                      size_t payload_len,
                                      uint32_t timeout_ms,
                                      HANDLE abort_event);

/*
 * Receive one logical message. Blocks until a complete message arrives.
 *
 * On success:
 *   - hdr_out is filled with the decoded outer header.
 *   - *payload_out points to the payload bytes (inside session->recv_buf
 *     or buf). Valid until the next receive call.
 *   - *payload_len_out is the payload length.
 *
 * buf/buf_size: caller-provided buffer for the first packet.
 */
nipc_np_error_t nipc_np_receive(nipc_np_session_t *session,
                                 void *buf, size_t buf_size,
                                 nipc_header_t *hdr_out,
                                 const void **payload_out,
                                 size_t *payload_len_out);

/*
 * Receive one logical message with an optional deadline and abort event.
 *
 * timeout_ms == 0 waits indefinitely. abort_event == NULL disables abort
 * waiting. If abort_event is signaled before a complete message arrives,
 * returns NIPC_NP_ERR_ABORTED.
 */
nipc_np_error_t nipc_np_receive_timeout(nipc_np_session_t *session,
                                         void *buf, size_t buf_size,
                                         nipc_header_t *hdr_out,
                                         const void **payload_out,
                                         size_t *payload_len_out,
                                         uint32_t timeout_ms,
                                         HANDLE abort_event);

/*
 * Poll until a session becomes readable or the timeout expires.
 *
 * On success:
 *   - returns NIPC_NP_OK
 *   - sets *readable_out to true if bytes are available or a pipelined
 *     send queued messages (nipc_np_session_has_pending)
 *   - sets *readable_out to false on timeout with no pending bytes
 *
 * Returns NIPC_NP_ERR_DISCONNECTED if the peer has gone away while waiting.
 */
nipc_np_error_t nipc_np_wait_readable(nipc_np_session_t *session,
                                       uint32_t timeout_ms,
                                       bool *readable_out);

/* ------------------------------------------------------------------ */
/*  Utility                                                            */
/* ------------------------------------------------------------------ */

/*
 * Whether the next receive can return a message read earlier by a
 * pipelined send, without the pipe becoming readable.
 */
static inline bool nipc_np_session_has_pending(const nipc_np_session_t *s) {
    return s->pending_head < s->pending_len;
}

/*
 * The raw pipe HANDLE. It is not a readability signal: a pipe handle is
 * signalled by I/O completion, not by arriving data. Use
 * nipc_np_wait_readable(), and check nipc_np_session_has_pending() first: a
 * pipelined send can read responses into the session, where the pipe no
 * longer shows them.
 */
static inline HANDLE nipc_np_session_handle(const nipc_np_session_t *s) {
    return s->pipe;
}

static inline HANDLE nipc_np_listener_handle(const nipc_np_listener_t *l) {
    return l->pipe;
}

#ifdef NIPC_INTERNAL_TESTING
/* Call hook(user) whenever a connect finds every pipe instance busy, before
 * it waits for one. NULL removes the hook. */
void nipc_np_set_connect_busy_hook_for_tests(void (*hook)(void *user),
                                             void *user);

/* Call hook(user) whenever WaitNamedPipeW reports a free instance, before
 * the connect retries CreateFileW, so a test can take that instance first.
 * NULL removes the hook. */
void nipc_np_set_connect_waited_hook_for_tests(void (*hook)(void *user),
                                               void *user);

/* Fail the next `count` creations of a replacement pipe instance in accept,
 * as CreateNamedPipeW can under resource exhaustion. */
void nipc_np_fail_next_instances_for_tests(int count);

/* Clients trust only pipes owned by this SID string instead of their own
 * account, SYSTEM and Administrators. NULL restores the default. */
void nipc_np_set_trusted_owner_for_tests(const char *sid);
#endif

#ifdef __cplusplus
}
#endif

#endif /* _WIN32 || __MSYS__ */

#endif /* NETIPC_NAMED_PIPE_H */
