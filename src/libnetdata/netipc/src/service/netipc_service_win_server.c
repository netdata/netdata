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

/*
 * Handle one client session: read requests, dispatch to handler,
 * send responses. Each session gets its own response buffer.
 * Runs until the client disconnects or server stops.
 */
typedef struct {
  nipc_win_shm_ctx_t *hybrid;
  nipc_win_shm_ctx_t *busywait;
} prepared_win_shm_t;

static void server_destroy_prepared_win_shm(prepared_win_shm_t *prepared) {
  if (!prepared)
    return;
  if (prepared->hybrid) {
    nipc_win_shm_destroy(prepared->hybrid);
    free(prepared->hybrid);
    prepared->hybrid = NULL;
  }
  if (prepared->busywait) {
    nipc_win_shm_destroy(prepared->busywait);
    free(prepared->busywait);
    prepared->busywait = NULL;
  }
}

static nipc_win_shm_ctx_t *
server_take_prepared_win_shm(prepared_win_shm_t *prepared, uint32_t profile) {
  if (!prepared)
    return NULL;
  if (profile == NIPC_WIN_SHM_PROFILE_HYBRID) {
    nipc_win_shm_ctx_t *ctx = prepared->hybrid;
    prepared->hybrid = NULL;
    return ctx;
  }
  if (profile == NIPC_WIN_SHM_PROFILE_BUSYWAIT) {
    nipc_win_shm_ctx_t *ctx = prepared->busywait;
    prepared->busywait = NULL;
    return ctx;
  }
  return NULL;
}

static bool server_prepare_accept_config(nipc_managed_server_t *server,
                                         uint64_t sid,
                                         nipc_np_server_config_t *cfg_out,
                                         prepared_win_shm_t *prepared) {
  *cfg_out = server->base_config;
  cfg_out->max_request_payload_bytes = server->learned_request_payload_bytes;
  cfg_out->max_response_payload_bytes = server->learned_response_payload_bytes;
  memset(prepared, 0, sizeof(*prepared));

  uint32_t shm_profiles =
      cfg_out->supported_profiles &
      (NIPC_WIN_SHM_PROFILE_HYBRID | NIPC_WIN_SHM_PROFILE_BUSYWAIT);
  if (shm_profiles == 0)
    return true;

  uint32_t request_capacity;
  uint32_t response_capacity;
  if (!nipc_service_common_header_payload_len_u32(
          cfg_out->max_request_payload_bytes, &request_capacity) ||
      !nipc_service_common_header_payload_len_u32(
          cfg_out->max_response_payload_bytes, &response_capacity)) {
    cfg_out->supported_profiles &= ~shm_profiles;
    cfg_out->preferred_profiles &= ~shm_profiles;
    return cfg_out->supported_profiles != 0;
  }

  const uint32_t profiles[] = {
      NIPC_WIN_SHM_PROFILE_HYBRID,
      NIPC_WIN_SHM_PROFILE_BUSYWAIT,
  };
  for (size_t i = 0; i < sizeof(profiles) / sizeof(profiles[0]); i++) {
    uint32_t profile = profiles[i];
    if (!(cfg_out->supported_profiles & profile))
      continue;

    nipc_win_shm_ctx_t *ctx = nipc_service_win_calloc(
        1, sizeof(nipc_win_shm_ctx_t),
        NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC_INTERNAL);
    if (!ctx)
      continue;

    /* HELLO has not been read yet, so the request segment must cover any
     * client proposal the handshake may legally echo back. */
    nipc_win_shm_error_t serr = nipc_win_shm_server_create(
        server->run_dir, server->service_name, server->auth_token, sid, profile,
        request_capacity, response_capacity, ctx);
    if (serr == NIPC_WIN_SHM_OK) {
      if (profile == NIPC_WIN_SHM_PROFILE_HYBRID)
        prepared->hybrid = ctx;
      else
        prepared->busywait = ctx;
      continue;
    }

    free(ctx);
    cfg_out->supported_profiles &= ~profile;
    cfg_out->preferred_profiles &= ~profile;
  }

  return cfg_out->supported_profiles != 0;
}

static void server_wake_listener(nipc_managed_server_t *server) {
  if (server->listener.pipe == INVALID_HANDLE_VALUE ||
      server->listener.pipe == NULL || server->listener.pipe_name[0] == L'\0')
    return;

  HANDLE wake =
      CreateFileW(server->listener.pipe_name, GENERIC_READ | GENERIC_WRITE, 0,
                  NULL, OPEN_EXISTING, 0, NULL);
  if (wake != INVALID_HANDLE_VALUE)
    CloseHandle(wake);
}

/* Ask active session threads to leave synchronous ReadFile/WriteFile waits.
 * This is safer than targeting pipe handles from another thread because the
 * thread handle stays valid until the owner joins it. */
static void
server_cancel_active_session_io_locked(nipc_managed_server_t *server) {
  for (int i = 0; i < server->session_count; i++) {
    nipc_session_ctx_t *s = server->sessions[i];
    if (!s)
      continue;
    if (!InterlockedCompareExchange((volatile LONG *)&s->active, 0, 0))
      continue;
    if (s->thread == NULL || s->thread == INVALID_HANDLE_VALUE)
      continue;
    CancelSynchronousIo(s->thread);
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

void nipc_server_run(nipc_managed_server_t *server) {
  InterlockedExchange(&server->accept_loop_active, 1);
  InterlockedExchange(&server->running, 1);

  while (InterlockedCompareExchange(&server->running, 0, 0)) {
    /* Accept one client via L1 (blocking with internal timeout) */
    nipc_np_session_t session;
    memset(&session, 0, sizeof(session));
    session.pipe = INVALID_HANDLE_VALUE;

    uint64_t sid = server->next_session_id++;
    nipc_np_server_config_t accept_cfg;
    prepared_win_shm_t prepared_shm;
    if (!server_prepare_accept_config(server, sid, &accept_cfg,
                                      &prepared_shm)) {
      Sleep(10);
      continue;
    }

    server->listener.config = accept_cfg;
    nipc_np_error_t uerr = nipc_np_accept(&server->listener, sid, &session);
    if (uerr != NIPC_NP_OK) {
      server_destroy_prepared_win_shm(&prepared_shm);
      if (!InterlockedCompareExchange(&server->running, 0, 0))
        break;
      Sleep(10);
      continue;
    }

    nipc_service_common_server_note_request_capacity(
        server, session.max_request_payload_bytes);
    nipc_service_common_server_note_response_capacity(
        server, session.max_response_payload_bytes);

    /* Enforce worker_count limit: reap finished sessions, check count */
    EnterCriticalSection(&server->sessions_lock);
    nipc_service_win_server_reap_sessions_locked(server);

    if (server->session_count >= server->worker_count) {
      /* At capacity: reject this client by closing the session */
      LeaveCriticalSection(&server->sessions_lock);
      server_destroy_prepared_win_shm(&prepared_shm);
      nipc_np_close_session(&session);
      continue;
    }

    /* SHM profile guarantee: only negotiate SHM for sessions backed by
     * prepared per-session kernel objects for the selected profile. */
    nipc_win_shm_ctx_t *shm = NULL;
    if (session.selected_profile == NIPC_WIN_SHM_PROFILE_HYBRID ||
        session.selected_profile == NIPC_WIN_SHM_PROFILE_BUSYWAIT) {
      shm =
          server_take_prepared_win_shm(&prepared_shm, session.selected_profile);
      if (!shm) {
        server_destroy_prepared_win_shm(&prepared_shm);
        LeaveCriticalSection(&server->sessions_lock);
        nipc_np_close_session(&session);
        continue;
      }
      server_destroy_prepared_win_shm(&prepared_shm);
    } else {
      server_destroy_prepared_win_shm(&prepared_shm);
    }

    /* Create session context */
    nipc_session_ctx_t *sctx = nipc_service_win_calloc(
        1, sizeof(nipc_session_ctx_t),
        NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC_INTERNAL);
    if (!sctx) {
      LeaveCriticalSection(&server->sessions_lock);
      if (shm) {
        nipc_win_shm_destroy(shm);
        free(shm);
      }
      nipc_np_close_session(&session);
      continue;
    }

    sctx->server = server;
    sctx->session = session;
    sctx->shm = shm;
    sctx->id = sid;
    InterlockedExchange((volatile LONG *)&sctx->active, 1);

    server->sessions[server->session_count++] = sctx;
    LeaveCriticalSection(&server->sessions_lock);

    /* Spawn handler thread for this session */
    unsigned tid_unused;
    sctx->thread = (HANDLE)nipc_service_win_beginthreadex(
        NULL, 0, nipc_service_win_session_handler_thread, sctx, 0, &tid_unused);
    if (sctx->thread == 0) {
      /* Thread creation failed: clean up */
      EnterCriticalSection(&server->sessions_lock);
      for (int i = 0; i < server->session_count; i++) {
        if (server->sessions[i] == sctx) {
          server->sessions[i] = server->sessions[server->session_count - 1];
          server->session_count--;
          break;
        }
      }
      LeaveCriticalSection(&server->sessions_lock);

      if (shm) {
        nipc_win_shm_destroy(shm);
        free(shm);
      }
      nipc_np_close_session(&session);
      free(sctx);
    }
  }

  InterlockedExchange(&server->accept_loop_active, 0);
  nipc_np_close_listener(&server->listener);
}

void nipc_server_stop(nipc_managed_server_t *server) {
  InterlockedExchange(&server->running, 0);
  if (InterlockedCompareExchange(&server->accept_loop_active, 0, 0))
    server_wake_listener(server);
  else
    nipc_np_close_listener(&server->listener);

  if (server->sessions) {
    EnterCriticalSection(&server->sessions_lock);
    server_cancel_active_session_io_locked(server);
    LeaveCriticalSection(&server->sessions_lock);
  }
}

bool nipc_server_drain(nipc_managed_server_t *server, uint32_t timeout_ms) {
  /* 1. Stop accepting new clients */
  InterlockedExchange(&server->running, 0);
  if (InterlockedCompareExchange(&server->accept_loop_active, 0, 0))
    server_wake_listener(server);
  else
    nipc_np_close_listener(&server->listener);

  /* 2. Wait for in-flight sessions to complete */
  bool all_drained = true;
  if (server->sessions) {
    ULONGLONG deadline = GetTickCount64() + timeout_ms;

    /* Poll until all sessions are inactive or timeout */
    while (1) {
      EnterCriticalSection(&server->sessions_lock);
      int active_count = 0;
      for (int i = 0; i < server->session_count; i++) {
        if (InterlockedCompareExchange(
                (volatile LONG *)&server->sessions[i]->active, 0, 0))
          active_count++;
      }
      LeaveCriticalSection(&server->sessions_lock);

      if (active_count == 0)
        break;

      if (GetTickCount64() >= deadline) {
        /* Timeout: cancel synchronous session I/O to unblock threads. */
        EnterCriticalSection(&server->sessions_lock);
        server_cancel_active_session_io_locked(server);
        LeaveCriticalSection(&server->sessions_lock);
        all_drained = false;
        break;
      }

      Sleep(5); /* 5ms poll interval */
    }

    /* 3. Join all session threads */
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

  server->worker_count = 0;

  return all_drained;
}

void nipc_server_destroy(nipc_managed_server_t *server) {
  InterlockedExchange(&server->running, 0);
  if (InterlockedCompareExchange(&server->accept_loop_active, 0, 0))
    server_wake_listener(server);
  else
    nipc_np_close_listener(&server->listener);

  /* Join all active session threads */
  if (server->sessions) {
    EnterCriticalSection(&server->sessions_lock);
    server_cancel_active_session_io_locked(server);
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

  server->worker_count = 0;
}

#endif /* _WIN32 || __MSYS__ */
