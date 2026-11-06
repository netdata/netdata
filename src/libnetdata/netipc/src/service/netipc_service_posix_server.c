/*
 * netipc_service_posix_server.c - POSIX managed server orchestration.
 */

#include "netipc/netipc_protocol.h"
#include "netipc/netipc_service.h"
#include "netipc/netipc_shm.h"
#include "netipc/netipc_uds.h"
#include "netipc_service_common.h"
#include "netipc_service_platform.h"
#include "netipc_service_posix_internal.h"

#include <errno.h>
#include <poll.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

/* ------------------------------------------------------------------ */
/*  Internal: managed server session handler                           */
/* ------------------------------------------------------------------ */

static void server_accept_loop_mark_active(nipc_managed_server_t *server) {
  if (!server || !server->sessions)
    return;

  pthread_mutex_lock(&server->sessions_lock);
  server->acceptor_thread = pthread_self();
  server->acceptor_started = true;
  pthread_mutex_unlock(&server->sessions_lock);
}

static void server_accept_loop_mark_inactive(nipc_managed_server_t *server) {
  if (!server || !server->sessions)
    return;

  pthread_mutex_lock(&server->sessions_lock);
  server->acceptor_started = false;
  pthread_mutex_unlock(&server->sessions_lock);
}

static void server_wait_accept_loop_inactive(nipc_managed_server_t *server) {
  if (!server || !server->sessions)
    return;

  for (;;) {
    pthread_mutex_lock(&server->sessions_lock);
    bool active = server->acceptor_started;
    bool current_thread =
        active && pthread_equal(server->acceptor_thread, pthread_self());
    pthread_mutex_unlock(&server->sessions_lock);

    if (!active || current_thread)
      return;
    nipc_service_posix_sleep_us(1000);
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
  server->listener.fd = -1;
  __atomic_store_n(&server->running, false, __ATOMIC_RELAXED);
  server->acceptor_started = false;

  if (!config)
    return NIPC_ERR_BAD_LAYOUT;

  nipc_error_t ierr = nipc_service_common_server_init_base(
      server, run_dir, service_name, worker_count, expected_method_code,
      handler, user, config->max_request_payload_bytes,
      config->max_response_payload_bytes);
  if (ierr != NIPC_OK)
    return ierr;
  server->base_config = *config;
  ierr = nipc_service_common_server_alloc_sessions(
      server, nipc_service_posix_calloc,
      NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSIONS_CALLOC_INTERNAL);
  if (ierr != NIPC_OK)
    return ierr;
  pthread_mutex_init(&server->sessions_lock, NULL);

  /* Clean up stale SHM regions from previous crashes (spec requirement:
   * runs once at server startup, before the listener begins accepting). */
  nipc_shm_cleanup_stale(run_dir, service_name);

  /* Start listening via L1 */
  nipc_uds_error_t uerr =
      nipc_uds_listen(run_dir, service_name, config, &server->listener);
  if (uerr != NIPC_UDS_OK) {
    free(server->sessions);
    server->sessions = NULL;
    pthread_mutex_destroy(&server->sessions_lock);
    return NIPC_ERR_BAD_LAYOUT;
  }

  return NIPC_OK;
}

nipc_error_t
nipc_server_init_raw_for_tests(nipc_managed_server_t *server,
                               const char *run_dir, const char *service_name,
                               const nipc_uds_server_config_t *config,
                               int worker_count, uint16_t expected_method_code,
                               nipc_server_handler_fn handler, void *user) {
  return nipc_service_platform_server_init_raw(
      server, run_dir, service_name, config, worker_count, expected_method_code,
      handler, user);
}

static void server_request_stop(nipc_managed_server_t *server) {
  __atomic_store_n(&server->stop_requested, true, __ATOMIC_RELEASE);
  __atomic_store_n(&server->running, false, __ATOMIC_RELEASE);
}

static bool server_stop_requested(nipc_managed_server_t *server) {
  return __atomic_load_n(&server->stop_requested, __ATOMIC_ACQUIRE);
}

/* Takes sessions_lock and keeps it on return. When all workers are busy,
 * waits up to two poll slices for one to finish (a worker whose client left
 * exits within one), so a client that
 * reconnects right after closing is not refused while its old worker exits.
 * The loop has nothing else to serve while full, so waiting costs nothing. */
static bool server_wait_for_slot_locked(nipc_managed_server_t *server) {
  uint64_t deadline =
      nipc_service_platform_monotonic_ms() + 2u * (uint64_t)server->poll_ms;
  pthread_mutex_lock(&server->sessions_lock);
  for (;;) {
    nipc_service_posix_server_reap_sessions_locked(server);
    if (server->session_count < server->worker_count)
      return true;
    if (!__atomic_load_n(&server->running, __ATOMIC_RELAXED) ||
        nipc_service_platform_monotonic_ms() >= deadline)
      return false;
    pthread_mutex_unlock(&server->sessions_lock);
    nipc_service_posix_sleep_us(1000);
    pthread_mutex_lock(&server->sessions_lock);
  }
}

void nipc_server_run(nipc_managed_server_t *server) {
  /* A stop, drain or destroy before run must win: drain and destroy may
   * already have released the listener and the session lock. */
  if (server_stop_requested(server))
    return;
  __atomic_store_n(&server->running, true, __ATOMIC_RELEASE);
  if (server_stop_requested(server)) {
    __atomic_store_n(&server->running, false, __ATOMIC_RELEASE);
    return;
  }
  server_accept_loop_mark_active(server);

  while (__atomic_load_n(&server->running, __ATOMIC_RELAXED)) {
    /* Poll the listener fd before blocking on accept */
    int pr = nipc_service_posix_poll_with_shutdown(server->listener.fd,
                                                   &server->running,
                                                   server->poll_ms);
    if (pr <= 0)
      break; /* shutdown or error */

    /* The handshake runs on the session thread, so a client that never
     * sends HELLO cannot stall this loop. */
    int fd = -1;
    if (nipc_uds_accept_pending(&server->listener, &fd) != NIPC_UDS_OK) {
      if (!__atomic_load_n(&server->running, __ATOMIC_RELAXED))
        break;
      nipc_service_posix_sleep_us(10000);
      continue;
    }

    /* Enforce worker_count limit: a connection holds its slot from accept,
     * handshake included. Refuse before HELLO_ACK so the client's connect
     * fails instead of a later call. */
    if (!server_wait_for_slot_locked(server)) {
      pthread_mutex_unlock(&server->sessions_lock);
      close(fd);
      continue;
    }

    nipc_session_ctx_t *sctx = nipc_service_posix_calloc(
        1, sizeof(nipc_session_ctx_t),
        NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC_INTERNAL);
    if (!sctx) {
      pthread_mutex_unlock(&server->sessions_lock);
      close(fd);
      continue;
    }

    sctx->session.fd = -1;
    sctx->id = server->next_session_id++;
    __atomic_store_n(&sctx->active, true, __ATOMIC_RELAXED);

    server->sessions[server->session_count++] = sctx;
    pthread_mutex_unlock(&server->sessions_lock);

    /* Spawn handler thread for this session */
    nipc_posix_session_start_arg_t start_arg = {
        .copied = false,
        .server = server,
        .session = sctx,
        .fd = fd,
    };
    int start_mutex_rc = pthread_mutex_init(&start_arg.lock, NULL);
    int start_cond_rc = start_mutex_rc == 0
                            ? pthread_cond_init(&start_arg.copied_cond, NULL)
                            : -1;
    int rc = -1;
    if (start_mutex_rc == 0 && start_cond_rc == 0) {
      rc = nipc_service_posix_pthread_create(
          &sctx->thread, NULL, nipc_service_posix_session_handler_thread,
          &start_arg);
    }
    if (rc == 0) {
      pthread_mutex_lock(&start_arg.lock);
      while (!start_arg.copied)
        pthread_cond_wait(&start_arg.copied_cond, &start_arg.lock);
      pthread_mutex_unlock(&start_arg.lock);
    }
    if (start_cond_rc == 0)
      pthread_cond_destroy(&start_arg.copied_cond);
    if (start_mutex_rc == 0)
      pthread_mutex_destroy(&start_arg.lock);

    if (start_mutex_rc != 0 || start_cond_rc != 0 || rc != 0) {
      /* Thread creation failed: clean up */
      pthread_mutex_lock(&server->sessions_lock);
      /* Remove the sctx we just added */
      for (int i = 0; i < server->session_count; i++) {
        if (server->sessions[i] == sctx) {
          server->sessions[i] = server->sessions[server->session_count - 1];
          server->session_count--;
          break;
        }
      }
      pthread_mutex_unlock(&server->sessions_lock);

      close(fd);
      free(sctx);
    }
  }

  server_accept_loop_mark_inactive(server);
}

void nipc_server_stop(nipc_managed_server_t *server) {
  server_request_stop(server);
}

static void server_join_sessions(nipc_managed_server_t *server) {
  pthread_mutex_lock(&server->sessions_lock);
  for (int i = 0; i < server->session_count; i++) {
    nipc_session_ctx_t *s = server->sessions[i];
    pthread_mutex_unlock(&server->sessions_lock);
    pthread_join(s->thread, NULL);
    free(s);
    pthread_mutex_lock(&server->sessions_lock);
  }
  server->session_count = 0;
  pthread_mutex_unlock(&server->sessions_lock);

  free(server->sessions);
  server->sessions = NULL;
  server->session_capacity = 0;
  pthread_mutex_destroy(&server->sessions_lock);
}

bool nipc_server_drain(nipc_managed_server_t *server, uint32_t timeout_ms) {
  /* 1. Stop accepting new clients.
   * Do NOT close the listener here — the run loop may still be
   * polling on listener.fd. Setting the flag is enough; the run
   * loop will exit on its next poll timeout (100ms). The listener
   * is closed later by nipc_server_destroy(). */
  server_request_stop(server);
  server_wait_accept_loop_inactive(server);

  /* 2. Wait for in-flight sessions, 3. join them (finished or not) */
  bool all_drained = true;
  if (server->sessions) {
    all_drained = nipc_service_posix_server_wait_sessions(server, timeout_ms);
    server_join_sessions(server);
  }

  server->worker_count = 0;
  return all_drained;
}

void nipc_server_destroy(nipc_managed_server_t *server) {
  server_request_stop(server);
  /* The accept loop polls the listener until it notices the stop. */
  server_wait_accept_loop_inactive(server);
  nipc_uds_close_listener(&server->listener);

  if (server->sessions) {
    (void)nipc_service_posix_server_wait_sessions(server, server->stop_grace_ms);
    server_join_sessions(server);
  }

  server->worker_count = 0;
}
