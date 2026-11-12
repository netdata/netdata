/*
 * netipc_service_win_server.c - Windows managed server orchestration.
 */

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc/netipc_named_pipe.h"
#include "netipc/netipc_protocol.h"
#include "netipc/netipc_service.h"
#include "netipc/netipc_win_shm.h"
#include "netipc_service_common.h"
#include "netipc_service_platform.h"
#include "netipc_service_win_internal.h"

#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <windows.h>

/* ------------------------------------------------------------------ */
/*  Internal: managed server session handler                           */
/* ------------------------------------------------------------------ */

/* Wake an accept loop blocked in ConnectNamedPipe by connecting to it.
 * Uses only the pipe name, which is fixed after listen; the pipe handle
 * changes on every accept. */
static void server_wake_listener(nipc_managed_server_t *server) {
  if (server->listener.pipe_name[0] == L'\0')
    return;

  HANDLE wake =
      CreateFileW(server->listener.pipe_name, GENERIC_READ | GENERIC_WRITE, 0,
                  NULL, OPEN_EXISTING,
                  SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION, NULL);
  if (wake != INVALID_HANDLE_VALUE)
    CloseHandle(wake);
}

/* Wait for the accept loop to exit, re-sending the wake: a real client can
 * take the free pipe instance before the wake connection does. */
static void server_wait_accept_loop_inactive(nipc_managed_server_t *server) {
  LONG current_thread_id = (LONG)GetCurrentThreadId();

  for (unsigned i = 0;
       InterlockedCompareExchange(&server->accept_loop_active, 0, 0); i++) {
    LONG owner = InterlockedCompareExchange(&server->accept_loop_thread_id, 0, 0);
    if (owner == current_thread_id)
      return;
    if (i % 50 == 49)
      server_wake_listener(server);
    Sleep(1);
  }
}

/* ------------------------------------------------------------------ */
/*  Public API: managed server                                         */
/* ------------------------------------------------------------------ */

nipc_error_t nipc_service_platform_server_init_raw(
    nipc_managed_server_t *server, const char *run_dir,
    const char *service_name,
    const nipc_service_platform_server_config_t *config, int worker_count,
    uint16_t expected_method_code, nipc_server_handler_fn handler, void *user) {
  if (!server)
    return NIPC_ERR_BAD_LAYOUT;

  memset(server, 0, sizeof(*server));
  server->listener.pipe = INVALID_HANDLE_VALUE;
  InterlockedExchange(&server->running, 0);

  if (!config)
    return NIPC_ERR_BAD_LAYOUT;

  nipc_error_t ierr = nipc_service_common_server_init_base(
      server, run_dir, service_name, worker_count, expected_method_code,
      handler, user, config->max_request_payload_bytes,
      config->max_response_payload_bytes);
  if (ierr != NIPC_OK)
    return ierr;
  server->base_config = *config;
  /* 0 selects the baseline profile, as in the transport handshake; the
   * accept path masks profiles and would otherwise reject every client. */
  if (server->base_config.supported_profiles == 0)
    server->base_config.supported_profiles = NIPC_PROFILE_BASELINE;
  server->auth_token = config->auth_token;
  ierr = nipc_service_common_server_alloc_sessions(
      server, nipc_service_win_calloc,
      NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SESSIONS_CALLOC_INTERNAL);
  if (ierr != NIPC_OK)
    return ierr;
  InitializeCriticalSection(&server->sessions_lock);

  /* Clean up stale SHM kernel objects from previous crashes (no-op on
   * Windows but maintains API symmetry with the POSIX transport). */
  nipc_win_shm_cleanup_stale(run_dir, service_name);

  /* Start listening via L1 */
  nipc_np_error_t uerr =
      nipc_np_listen(run_dir, service_name, config, &server->listener);
  if (uerr != NIPC_NP_OK) {
    free(server->sessions);
    server->sessions = NULL;
    DeleteCriticalSection(&server->sessions_lock);
    return NIPC_ERR_BAD_LAYOUT;
  }

  return NIPC_OK;
}

nipc_error_t
nipc_server_init_raw_for_tests(nipc_managed_server_t *server,
                               const char *run_dir, const char *service_name,
                               const nipc_np_server_config_t *config,
                               int worker_count, uint16_t expected_method_code,
                               nipc_server_handler_fn handler, void *user) {
  return nipc_service_platform_server_init_raw(
      server, run_dir, service_name, config, worker_count, expected_method_code,
      handler, user);
}

static void server_request_stop(nipc_managed_server_t *server) {
  InterlockedExchange(&server->stop_requested, 1);
  InterlockedExchange(&server->running, 0);
}

static bool server_stop_requested(nipc_managed_server_t *server) {
  return InterlockedCompareExchange(&server->stop_requested, 0, 0) != 0;
}

/* Takes sessions_lock and keeps it on return. When all workers are busy,
 * waits up to two poll slices for one to finish (a worker whose client left
 * exits within one), so a client that
 * reconnects right after closing is not refused while its old worker exits.
 * Clients queued behind this one wait as well, but while every worker
 * stays busy they would be refused too. */
static bool server_wait_for_slot_locked(nipc_managed_server_t *server) {
  ULONGLONG deadline = GetTickCount64() + 2u * (ULONGLONG)server->poll_ms;
  EnterCriticalSection(&server->sessions_lock);
  for (;;) {
    nipc_service_win_server_reap_sessions_locked(server);
    if (server->session_count < server->worker_count)
      return true;
    if (!InterlockedCompareExchange(&server->running, 0, 0) ||
        GetTickCount64() >= deadline)
      return false;
    LeaveCriticalSection(&server->sessions_lock);
    Sleep(1);
    EnterCriticalSection(&server->sessions_lock);
  }
}

void nipc_server_run(nipc_managed_server_t *server) {
  /* A stop, drain or destroy before run must win: they may already have
   * released the listener and the session lock. */
  if (server_stop_requested(server))
    return;
  /* running is set before the stop check: a stop the check misses clears
   * it afterwards. The loop is published, then the stop is checked; drain
   * and destroy publish the stop, then check the loop. Interlocked calls
   * are full barriers, so at least one side sees the other: either they
   * wait for this loop, or it sees the stop and leaves the listener to
   * them. */
  InterlockedExchange(&server->accept_loop_thread_id, (LONG)GetCurrentThreadId());
  InterlockedExchange(&server->running, 1);
  InterlockedExchange(&server->accept_loop_active, 1);
  if (server_stop_requested(server)) {
    InterlockedExchange(&server->running, 0);
    InterlockedExchange(&server->accept_loop_active, 0);
    InterlockedExchange(&server->accept_loop_thread_id, 0);
    return;
  }

  while (InterlockedCompareExchange(&server->running, 0, 0)) {
    /* The handshake runs on the session thread, so a client that never
     * sends HELLO cannot stall this loop. */
    HANDLE pipe = INVALID_HANDLE_VALUE;
    nipc_np_error_t uerr = nipc_np_accept_pending(&server->listener, &pipe);
    if (uerr != NIPC_NP_OK) {
      if (!InterlockedCompareExchange(&server->running, 0, 0))
        break;
      Sleep(10);
      continue;
    }

    /* Stop wakes ConnectNamedPipe with a throwaway connection. */
    if (!InterlockedCompareExchange(&server->running, 0, 0)) {
      DisconnectNamedPipe(pipe);
      CloseHandle(pipe);
      break;
    }

    /* Enforce worker_count limit: a connection holds its slot from accept,
     * handshake included. Refuse before HELLO_ACK so the client's connect
     * fails instead of a later call. */
    if (!server_wait_for_slot_locked(server)) {
      LeaveCriticalSection(&server->sessions_lock);
      DisconnectNamedPipe(pipe);
      CloseHandle(pipe);
      continue;
    }

    nipc_session_ctx_t *sctx = nipc_service_win_calloc(
        1, sizeof(nipc_session_ctx_t),
        NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC_INTERNAL);
    if (!sctx) {
      LeaveCriticalSection(&server->sessions_lock);
      DisconnectNamedPipe(pipe);
      CloseHandle(pipe);
      continue;
    }

    sctx->server = server;
    /* The thread takes the pending pipe from here and replaces it with
     * the established session. */
    sctx->session.pipe = pipe;
    sctx->id = server->next_session_id++;
    InterlockedExchange(&sctx->active, 1);

    /* Create the thread under the lock so stop paths never see a session
     * without its thread handle. */
    unsigned tid_unused;
    sctx->thread = (HANDLE)nipc_service_win_beginthreadex(
        NULL, 0, nipc_service_win_session_handler_thread, sctx, 0, &tid_unused);
    if (sctx->thread == 0) {
      LeaveCriticalSection(&server->sessions_lock);
      DisconnectNamedPipe(pipe);
      CloseHandle(pipe);
      free(sctx);
      continue;
    }

    server->sessions[server->session_count++] = sctx;
    LeaveCriticalSection(&server->sessions_lock);
  }

  /* Close before marking inactive: drain/destroy close the listener only
   * after the loop is gone, so the two never race. */
  nipc_np_close_listener(&server->listener);
  InterlockedExchange(&server->accept_loop_active, 0);
  InterlockedExchange(&server->accept_loop_thread_id, 0);
}

void nipc_server_stop(nipc_managed_server_t *server) {
  server_request_stop(server);
  if (InterlockedCompareExchange(&server->accept_loop_active, 0, 0))
    server_wake_listener(server);
}

/* Stop the accept loop and release the listener. */
static void server_stop_accepting(nipc_managed_server_t *server) {
  server_request_stop(server);
  if (InterlockedCompareExchange(&server->accept_loop_active, 0, 0))
    server_wake_listener(server);
  server_wait_accept_loop_inactive(server);
  nipc_np_close_listener(&server->listener);
}

static void server_join_sessions(nipc_managed_server_t *server) {
  EnterCriticalSection(&server->sessions_lock);
  for (int i = 0; i < server->session_count; i++) {
    nipc_session_ctx_t *s = server->sessions[i];
    LeaveCriticalSection(&server->sessions_lock);
    WaitForSingleObject(s->thread, INFINITE);
    CloseHandle(s->thread);
    free(s);
    EnterCriticalSection(&server->sessions_lock);
  }
  server->session_count = 0;
  LeaveCriticalSection(&server->sessions_lock);

  free(server->sessions);
  server->sessions = NULL;
  server->session_capacity = 0;
  DeleteCriticalSection(&server->sessions_lock);
}

bool nipc_server_drain(nipc_managed_server_t *server, uint32_t timeout_ms) {
  /* 1. Stop accepting new clients */
  server_stop_accepting(server);

  /* 2. Wait for in-flight sessions, 3. join them */
  bool all_drained = true;
  if (server->sessions) {
    all_drained = nipc_service_win_server_wait_sessions(server, timeout_ms);
    server_join_sessions(server);
  }

  server->worker_count = 0;

  return all_drained;
}

void nipc_server_destroy(nipc_managed_server_t *server) {
  server_stop_accepting(server);

  if (server->sessions) {
    (void)nipc_service_win_server_wait_sessions(server, server->stop_grace_ms);
    server_join_sessions(server);
  }

  server->worker_count = 0;
}

#endif /* _WIN32 || __MSYS__ */
