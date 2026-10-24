/*
 * netipc_service.h - L2 orchestration: client context and managed server.
 *
 * Pure convenience layer. Uses L1 transport + Codec exclusively.
 * Adds zero wire behavior. Provides lifecycle management, typed
 * cgroups-snapshot calls, and managed multi-client worker dispatch for
 * one service kind per endpoint.
 *
 * The public L2 contract is service-oriented:
 *   - clients connect to a service kind, not a plugin identity
 *   - one server endpoint serves one request kind only
 *   - outer request codes remain part of the envelope for validation,
 *     not public multi-method dispatch
 *
 * L2 callers never see transports, handshakes, or chunking.
 *
 * Platform-specific transport types are selected at compile time:
 *   POSIX: UDS + SHM (netipc_uds.h, netipc_shm.h)
 *   Windows: Named Pipe + Win SHM (netipc_named_pipe.h, netipc_win_shm.h)
 */

#ifndef NETIPC_SERVICE_H
#define NETIPC_SERVICE_H

#include "netipc_protocol.h"

#if defined(_WIN32) || defined(__MSYS__)
#include "netipc_named_pipe.h"
#include "netipc_win_shm.h"
#include <windows.h>
#else
#include "netipc_shm.h"
#include "netipc_uds.h"
#include <pthread.h>
#endif

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NIPC_CLIENT_CALL_TIMEOUT_DEFAULT_MS 30000u
#define NIPC_CLIENT_ABORT_POLL_MS 100u
#define NIPC_SERVICE_MIN_PAYLOAD_BUFFER_BYTES 1024u
#define NIPC_LOGICAL_LOOKUP_ITEMS_DEFAULT 65536u
#define NIPC_LOGICAL_LOOKUP_SUBCALLS_DEFAULT 4096u
#define NIPC_LOGICAL_LOOKUP_RESPONSE_BYTES_DEFAULT (64u * 1024u * 1024u)

#ifdef __cplusplus
extern "C" {
#endif

/* ------------------------------------------------------------------ */
/*  Client context state                                               */
/* ------------------------------------------------------------------ */

typedef enum {
  NIPC_CLIENT_DISCONNECTED = 0,
  NIPC_CLIENT_CONNECTING,
  NIPC_CLIENT_READY,
  NIPC_CLIENT_NOT_FOUND,
  NIPC_CLIENT_AUTH_FAILED,
  NIPC_CLIENT_INCOMPATIBLE,
  NIPC_CLIENT_BROKEN,
} nipc_client_state_t;

/* ------------------------------------------------------------------ */
/*  Client status snapshot (for diagnostics, not hot path)              */
/* ------------------------------------------------------------------ */

typedef struct {
  nipc_client_state_t state;
  uint32_t connect_count;
  uint32_t reconnect_count;
  uint32_t call_count;
  uint32_t error_count;
} nipc_client_status_t;

/* ------------------------------------------------------------------ */
/*  Public L2/L3 service configuration                                 */
/* ------------------------------------------------------------------ */

/*
 * Public service-level client configuration shared by the L2 and L3 APIs.
 *
 * This configuration is intentionally transport-agnostic. Transport-only
 * tuning such as packet sizing stays below the public L2/L3 boundary.
 *
 * Zero-valued fields map to the normal library defaults.
 */
typedef struct {
  uint32_t supported_profiles;
  uint32_t preferred_profiles;
  uint32_t max_request_payload_bytes;
  uint32_t max_request_batch_items;
  uint32_t max_response_payload_bytes;
  uint64_t auth_token;
  uint32_t call_timeout_ms; /* 0 = use default (30000 ms) */
  uint32_t max_logical_lookup_items;
  uint32_t max_logical_lookup_subcalls;
  uint32_t max_logical_lookup_response_bytes;
} nipc_client_config_t;

/*
 * Public service-level server configuration shared by the typed L2 API.
 *
 * Transport-only tuning such as socket backlog or packet sizing stays in the
 * transport layer and is not part of the public L2/L3 contract.
 *
 * Zero-valued fields map to the normal library defaults.
 */
typedef struct {
  uint32_t supported_profiles;
  uint32_t preferred_profiles;
  uint32_t max_request_payload_bytes;
  uint32_t max_request_batch_items;
  uint32_t max_response_payload_bytes;
  uint64_t auth_token;
} nipc_server_config_t;

/* ------------------------------------------------------------------ */
/*  Client context                                                     */
/* ------------------------------------------------------------------ */

typedef struct {
  /* State */
  nipc_client_state_t state;

  /* Configuration (mutable learned sizing, updated across reconnects) */
  char run_dir[256];
  char service_name[128];

#if defined(_WIN32) || defined(__MSYS__)
  nipc_np_client_config_t transport_config;

  /* Connection (managed internally) */
  nipc_np_session_t session;
  bool session_valid;
  nipc_win_shm_ctx_t *shm; /* non-NULL if SHM profile negotiated */
  HANDLE abort_event;
#else
  nipc_uds_client_config_t transport_config;

  /* Connection (managed internally) */
  nipc_uds_session_t session;
  bool session_valid;
  nipc_shm_ctx_t *shm; /* non-NULL if SHM profile negotiated */
  int abort_pipe[2];
  bool abort_pipe_valid;
  pthread_mutex_t abort_pipe_lock;
  bool abort_pipe_lock_initialized;
#endif

  uint32_t call_timeout_ms;
  uint32_t max_logical_lookup_items;
  uint32_t max_logical_lookup_subcalls;
  uint32_t max_logical_lookup_response_bytes;
  uint32_t abort_requested;

  /* Stats */
  uint32_t connect_count;
  uint32_t reconnect_count;
  uint32_t call_count;
  uint32_t error_count;

  /* Internal reusable Level 2 scratch, allocated lazily from session sizes. */
  uint8_t *response_buf;
  size_t response_buf_size;
  uint8_t *send_buf;
  size_t send_buf_size;
} nipc_client_ctx_t;

/* ------------------------------------------------------------------ */
/*  Client API                                                         */
/* ------------------------------------------------------------------ */

/*
 * Initialize a client context. Does NOT connect. Does NOT require the
 * server to be running. State starts as DISCONNECTED.
 */
void nipc_client_init(nipc_client_ctx_t *ctx, const char *run_dir,
                      const char *service_name,
                      const nipc_client_config_t *config);

/*
 * Attempt connect if DISCONNECTED, reconnect if BROKEN.
 * Returns true if the state changed (e.g. DISCONNECTED -> READY),
 * false if unchanged.
 *
 * No hidden threads. Call from your own loop at your own cadence.
 */
bool nipc_client_refresh(nipc_client_ctx_t *ctx);

/*
 * Cheap cached boolean. No I/O, no syscalls.
 * Returns true only if state == READY.
 */
static inline bool nipc_client_ready(const nipc_client_ctx_t *ctx) {
  return ctx->state == NIPC_CLIENT_READY;
}

/*
 * Detailed status snapshot. For diagnostics and logging, not hot path.
 */
void nipc_client_status(const nipc_client_ctx_t *ctx,
                        nipc_client_status_t *out);

/*
 * Set the context-level default timeout used by typed calls when their
 * per-call timeout argument is zero. Passing zero restores the library default.
 */
void nipc_client_set_call_timeout(nipc_client_ctx_t *ctx, uint32_t timeout_ms);

/*
 * Abort an in-flight synchronous client call from another thread. Abort is
 * sticky: future calls fail with NIPC_ERR_ABORTED until
 * nipc_client_clear_abort() or nipc_client_close() is called.
 */
void nipc_client_abort(nipc_client_ctx_t *ctx);

/*
 * Clear a previously requested abort so the context can be refreshed/reused.
 */
void nipc_client_clear_abort(nipc_client_ctx_t *ctx);

/*
 * Tear down connection and release resources. Safe on a zero-init ctx.
 */
void nipc_client_close(nipc_client_ctx_t *ctx);

/* ------------------------------------------------------------------ */
/*  Typed cgroups snapshot call                                        */
/* ------------------------------------------------------------------ */

/*
 * Blocking typed call: encode request, send, receive, check
 * transport_status, decode response.
 *
 * view_out: on success, filled with the ephemeral snapshot view.
 *   Valid only until the next call on this context.
 *
 * Retry policy (per spec):
 *   If the call fails and the context was previously READY, the client
 *   disconnects, reconnects (full handshake), and retries.
 *   - Ordinary transport / peer failures retry once.
 *   - Overflow-driven resize recovery may reconnect more than once
 *     while negotiated capacities grow.
 *   If not previously READY, fails immediately.
 *
 * Returns NIPC_OK on success, or an error code.
 */
nipc_error_t
nipc_client_call_cgroups_snapshot(nipc_client_ctx_t *ctx,
                                  nipc_cgroups_resp_view_t *view_out);

nipc_error_t
nipc_client_call_cgroups_snapshot_timeout(nipc_client_ctx_t *ctx,
                                          nipc_cgroups_resp_view_t *view_out,
                                          uint32_t timeout_ms);

nipc_error_t nipc_client_call_cgroups_lookup(
    nipc_client_ctx_t *ctx, const nipc_str_view_t *paths, uint32_t path_count,
    nipc_cgroups_lookup_resp_view_t *view_out);

nipc_error_t nipc_client_call_cgroups_lookup_timeout(
    nipc_client_ctx_t *ctx, const nipc_str_view_t *paths, uint32_t path_count,
    nipc_cgroups_lookup_resp_view_t *view_out, uint32_t timeout_ms);

nipc_error_t
nipc_client_call_apps_lookup(nipc_client_ctx_t *ctx, const uint32_t *pids,
                             uint32_t pid_count,
                             nipc_apps_lookup_resp_view_t *view_out);

nipc_error_t nipc_client_call_apps_lookup_timeout(
    nipc_client_ctx_t *ctx, const uint32_t *pids, uint32_t pid_count,
    nipc_apps_lookup_resp_view_t *view_out, uint32_t timeout_ms);

/* ------------------------------------------------------------------ */
/*  Managed server                                                     */
/* ------------------------------------------------------------------ */

/* Internal dispatch callback shape used by the managed-server loop. */
typedef nipc_error_t (*nipc_server_handler_fn)(
    void *user, const nipc_header_t *request_hdr,
    const uint8_t *request_payload, size_t request_len, uint8_t *response_buf,
    size_t response_buf_size, size_t *response_len_out);

/* Typed managed-server callback surface for the cgroups-snapshot service. */
typedef struct {
  nipc_cgroups_handler_fn handle;

  /* Optional explicit reservation hint for snapshot builders. When 0,
   * the library derives a safe upper bound from negotiated response
   * limits. */
  uint32_t snapshot_max_items;

  void *user;
} nipc_cgroups_service_handler_t;

typedef struct {
  nipc_cgroups_lookup_handler_fn handle;
  void *user;
} nipc_cgroups_lookup_service_handler_t;

typedef struct {
  nipc_apps_lookup_handler_fn handle;
  void *user;
} nipc_apps_lookup_service_handler_t;

typedef union {
  nipc_cgroups_service_handler_t cgroups_snapshot;
  nipc_cgroups_lookup_service_handler_t cgroups_lookup;
  nipc_apps_lookup_service_handler_t apps_lookup;
} nipc_server_typed_handler_t;

typedef struct nipc_managed_server nipc_managed_server_t;

/* Per-session context for multi-client server */
typedef struct nipc_session_ctx {
  nipc_managed_server_t *server; /* back-pointer */
#if defined(_WIN32) || defined(__MSYS__)
  nipc_np_session_t session;
  nipc_win_shm_ctx_t *shm; /* non-NULL if SHM negotiated */
  HANDLE thread;
#else
  nipc_uds_session_t session;
  nipc_shm_ctx_t *shm; /* non-NULL if SHM negotiated */
  pthread_t thread;
#endif
  uint64_t id;
#if defined(_WIN32) || defined(__MSYS__)
  volatile LONG active; /* use Interlocked* for cross-thread access */
#else
  bool active; /* use __atomic builtins for cross-thread access */
#endif
} nipc_session_ctx_t;

struct nipc_managed_server {
  /* Listener */
#if defined(_WIN32) || defined(__MSYS__)
  nipc_np_listener_t listener;
  nipc_np_server_config_t base_config;
#else
  nipc_uds_listener_t listener;
  nipc_uds_server_config_t base_config;
#endif

  /* Concurrency control */
  int worker_count; /* max concurrent sessions */

  /* Callback */
  nipc_server_handler_fn handler;
  void *handler_user;
  nipc_server_typed_handler_t typed_handler;
  uint16_t expected_method_code;
  uint32_t learned_request_payload_bytes;
  uint32_t learned_response_payload_bytes;
  uint32_t request_payload_growth_ceiling;
  uint32_t response_payload_growth_ceiling;

  /* State — use __atomic builtins (POSIX) or Interlocked* (Windows) */
#if defined(_WIN32) || defined(__MSYS__)
  volatile LONG running;
  volatile LONG accept_loop_active;
  volatile LONG accept_loop_thread_id;
#else
  bool running;
#endif

  /* Session tracking */
  nipc_session_ctx_t **sessions; /* dynamic array of active sessions */
  int session_count;             /* current active session count */
  int session_capacity;          /* allocated slots */
  uint64_t next_session_id;      /* monotonic session ID counter */
#if defined(_WIN32) || defined(__MSYS__)
  CRITICAL_SECTION sessions_lock; /* protects session array + count */
#else
  pthread_mutex_t sessions_lock; /* protects session array + count */
  pthread_t acceptor_thread;
  bool acceptor_started;
#endif

  /* Configuration */
  char run_dir[256];
  char service_name[128];

#if defined(_WIN32) || defined(__MSYS__)
  /* Auth token needed for SHM kernel object naming */
  uint64_t auth_token;
#endif
};

/*
 * Initialize a managed server for the typed cgroups-snapshot service kind.
 * One endpoint serves one request kind only.
 * Does NOT start workers. Call nipc_server_run() to start the
 * acceptor+worker loop.
 */
nipc_error_t
nipc_server_init_typed(nipc_managed_server_t *server, const char *run_dir,
                       const char *service_name,
                       const nipc_server_config_t *config, int worker_count,
                       const nipc_cgroups_service_handler_t *service_handler);

nipc_error_t nipc_server_init_cgroups_lookup(
    nipc_managed_server_t *server, const char *run_dir,
    const char *service_name, const nipc_server_config_t *config,
    int worker_count,
    const nipc_cgroups_lookup_service_handler_t *service_handler);

nipc_error_t nipc_server_init_apps_lookup(
    nipc_managed_server_t *server, const char *run_dir,
    const char *service_name, const nipc_server_config_t *config,
    int worker_count,
    const nipc_apps_lookup_service_handler_t *service_handler);

#ifdef NIPC_INTERNAL_TESTING
/*
 * Internal compatibility entrypoint for repo tests and benchmarks that
 * intentionally exercise raw dispatch or malformed response paths. Not part
 * of the public Level 2 contract.
 */
nipc_error_t
nipc_server_init_raw_for_tests(nipc_managed_server_t *server,
                               const char *run_dir, const char *service_name,
#if defined(_WIN32) || defined(__MSYS__)
                               const nipc_np_server_config_t *config,
#else
                               const nipc_uds_server_config_t *config,
#endif
                               int worker_count, uint16_t expected_method_code,
                               nipc_server_handler_fn handler, void *user);

nipc_error_t nipc_apps_lookup_remaining_timeout_for_tests(
    nipc_client_ctx_t *ctx, uint64_t deadline_ms, uint32_t *timeout_out);
nipc_error_t nipc_cgroups_lookup_remaining_timeout_for_tests(
    nipc_client_ctx_t *ctx, uint64_t deadline_ms, uint32_t *timeout_out);

#if defined(_WIN32) || defined(__MSYS__)
typedef enum {
  NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC = 1,
  NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_SHM_CTX_CALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_SERVER_RECV_BUF_MALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_SERVER_RESP_BUF_MALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SESSIONS_CALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_SERVER_THREAD_CREATE,
  NIPC_WIN_SERVICE_TEST_FAULT_CACHE_BUCKETS_CALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_CACHE_ITEMS_CALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_CACHE_ITEM_NAME_MALLOC,
  NIPC_WIN_SERVICE_TEST_FAULT_CACHE_ITEM_PATH_MALLOC,
} nipc_win_service_test_fault_site_t;

void nipc_win_service_test_fault_set(int site, uint32_t skip_matches);
void nipc_win_service_test_fault_clear(void);
#else
typedef enum {
  NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC = 1,
  NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SHM_CTX_CALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RECV_BUF_MALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RESP_BUF_MALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSIONS_CALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_THREAD_CREATE,
  NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_BUCKETS_CALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEMS_CALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEM_NAME_MALLOC,
  NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEM_PATH_MALLOC,
} nipc_posix_service_test_fault_site_t;

void nipc_posix_service_test_fault_set(int site, uint32_t skip_matches);
void nipc_posix_service_test_fault_clear(void);
#endif

#define nipc_server_init nipc_server_init_raw_for_tests
#endif

/*
 * Run the acceptor loop. Blocking. Accepts clients, reads requests,
 * dispatches to the handler, sends responses.
 *
 * Returns when nipc_server_stop() is called or on fatal error.
 */
void nipc_server_run(nipc_managed_server_t *server);

/*
 * Signal shutdown. The acceptor loop will exit after current work.
 */
void nipc_server_stop(nipc_managed_server_t *server);

/*
 * Graceful drain: stop accepting new clients, wait for in-flight
 * sessions to complete (up to timeout_ms), then close everything.
 *
 * Combines stop + wait + destroy in one call. If sessions don't
 * finish within timeout_ms, they are forcibly closed.
 *
 * Returns true if all sessions completed within the timeout,
 * false if the timeout expired and sessions were forcibly closed.
 */
bool nipc_server_drain(nipc_managed_server_t *server, uint32_t timeout_ms);

/*
 * Cleanup: close listener, free workers. Safe after stop.
 */
void nipc_server_destroy(nipc_managed_server_t *server);

/* ------------------------------------------------------------------ */
/*  L3: Client-side cgroups snapshot cache                             */
/* ------------------------------------------------------------------ */

/* Default response buffer size for L3 cache refresh (when config is 0) */
#define NIPC_CGROUPS_CACHE_BUF_SIZE_DEFAULT 65536

/*
 * Borrowed view of a cached cgroup item.
 *
 * Returned by nipc_cgroups_cache_get() and valid only while the caller holds
 * the read guard used for that lookup. Do not store this pointer, free it, or
 * use it after nipc_cgroups_cache_read_unlock().
 */
typedef struct {
  uint32_t hash;
  uint32_t options;
  uint32_t enabled;
  const char *name; /* borrowed NUL-terminated string */
  const char *path; /* borrowed NUL-terminated string */
} nipc_cgroups_cache_item_view_t;

/*
 * Owned copy of a single cgroup item.
 *
 * Returned by nipc_cgroups_cache_item_dup(). The caller owns the returned item
 * and must release it with nipc_cgroups_cache_item_free().
 */
typedef struct {
  uint32_t hash;
  uint32_t options;
  uint32_t enabled;
  char *name; /* owned NUL-terminated copy */
  char *path; /* owned NUL-terminated copy */
} nipc_cgroups_cache_item_t;

/*
 * L3 cache status snapshot (for diagnostics, not hot path).
 */
typedef struct {
  bool populated;
  uint32_t item_count;
  uint32_t systemd_enabled;
  uint64_t generation;
  uint32_t refresh_success_count;
  uint32_t refresh_failure_count;
  nipc_client_state_t connection_state; /* current L2 connection state */
  uint64_t last_refresh_ts; /* monotonic timestamp of last successful refresh
                               (ms), 0 if never */
} nipc_cgroups_cache_status_t;

/*
 * L3 client-side cgroups snapshot cache.
 *
 * Wraps an L2 client context and maintains a local owned copy of the
 * most recent successful snapshot. Lookup by hash+name is O(1) via
 * an open-addressing hash table, with no I/O.
 *
 * On refresh failure, the previous cache is preserved. The cache
 * becomes empty only if no successful refresh has ever occurred.
 */

typedef struct nipc_cgroups_cache_snapshot nipc_cgroups_cache_snapshot_t;

typedef struct {
  nipc_client_ctx_t client;

  /*
   * Opaque immutable snapshot protected by cache_lock. Readers must acquire a
   * read guard before accessing borrowed item views. Refresh builds a new
   * snapshot privately and swaps it under the write lock.
   */
  nipc_cgroups_cache_snapshot_t *snapshot;

  /* Counters */
  uint32_t refresh_success_count;
  uint32_t refresh_failure_count;
  uint64_t last_refresh_ts; /* monotonic ms of last successful refresh */

  /* Internal: response buffer for L2 calls */
  uint8_t *response_buf;
  size_t response_buf_size;

#if defined(_WIN32) || defined(__MSYS__)
  SRWLOCK cache_lock;
  CRITICAL_SECTION writer_lock;
#else
  pthread_rwlock_t cache_lock;
  pthread_mutex_t writer_lock;
#endif
  bool cache_lock_initialized;
  bool writer_lock_initialized;
} nipc_cgroups_cache_t;

typedef struct {
  nipc_cgroups_cache_t *cache;
  const nipc_cgroups_cache_snapshot_t *snapshot;
  bool locked;
} nipc_cgroups_cache_read_guard_t;

/*
 * Initialize an L3 cache. Creates the underlying L2 client context.
 * Does NOT connect. Does NOT require the server to be running.
 * Cache starts empty (populated == false).
 */
void nipc_cgroups_cache_init(nipc_cgroups_cache_t *cache, const char *run_dir,
                             const char *service_name,
                             const nipc_client_config_t *config);

/*
 * Refresh the cache. Drives the L2 client (connect/reconnect as
 * needed) and requests a fresh snapshot. On success, rebuilds the
 * local cache from the response (copies strings). On failure,
 * preserves the previous cache.
 *
 * Caller-driven: call from your own loop at your own cadence.
 * Returns true if the cache was updated, false otherwise.
 */
bool nipc_cgroups_cache_refresh(nipc_cgroups_cache_t *cache);

/*
 * Returns true if at least one successful refresh has occurred.
 * Cheap cached boolean. No I/O, no syscalls.
 *
 * Note: ready means "has cached data", not "is connected."
 */
bool nipc_cgroups_cache_ready(nipc_cgroups_cache_t *cache);

/*
 * Acquire a read guard for borrowed cache access.
 *
 * While the guard is held, refresh can build a new snapshot in the background
 * but cannot publish and retire the current snapshot. Always release a
 * successful guard with nipc_cgroups_cache_read_unlock().
 */
bool nipc_cgroups_cache_read_lock(nipc_cgroups_cache_t *cache,
                                  nipc_cgroups_cache_read_guard_t *guard);

void nipc_cgroups_cache_read_unlock(nipc_cgroups_cache_read_guard_t *guard);

/*
 * Look up a cached item by hash + name. Pure in-memory, no I/O.
 *
 * Returns a borrowed immutable view, or NULL if not found or cache is empty.
 * The returned view is valid only until nipc_cgroups_cache_read_unlock() is
 * called on the guard.
 */
const nipc_cgroups_cache_item_view_t *
nipc_cgroups_cache_get(const nipc_cgroups_cache_read_guard_t *guard,
                       uint32_t hash, const char *name);

/*
 * Duplicate a borrowed view into an owned item. The caller must hold the same
 * read guard that protected the view. The returned item survives unlock and
 * must be freed with nipc_cgroups_cache_item_free().
 */
nipc_cgroups_cache_item_t *
nipc_cgroups_cache_item_dup(const nipc_cgroups_cache_read_guard_t *guard,
                            const nipc_cgroups_cache_item_view_t *view);

void nipc_cgroups_cache_item_free(nipc_cgroups_cache_item_t *item);

/*
 * Fill a status snapshot for diagnostics.
 */
void nipc_cgroups_cache_status(nipc_cgroups_cache_t *cache,
                               nipc_cgroups_cache_status_t *out);

#ifdef NIPC_INTERNAL_TESTING
/*
 * Internal test/benchmark helper. Seeds the cache from owned copies of the
 * supplied items and publishes the snapshot through the same write lock used by
 * refresh. Not part of the public L3 cache contract.
 */
bool nipc_cgroups_cache_seed_for_tests(nipc_cgroups_cache_t *cache,
                                       const nipc_cgroups_cache_item_t *items,
                                       uint32_t item_count,
                                       uint32_t systemd_enabled,
                                       uint64_t generation);
#endif

/*
 * Close the cache: free all cached items, close the L2 client.
 * Safe on a zero-initialized cache.
 */
void nipc_cgroups_cache_close(nipc_cgroups_cache_t *cache);

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_SERVICE_H */
