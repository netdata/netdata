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

/*
 * Wait for data on a file descriptor with periodic shutdown checks.
 * Returns: 1 = data ready, 0 = server stopping, -1 = error/hangup.
 */
static void server_destroy_precreated_shm(nipc_shm_ctx_t **shm) {
  if (!shm || !*shm)
    return;
  nipc_shm_destroy(*shm);
  free(*shm);
  *shm = NULL;
}

static bool server_prepare_accept_config(nipc_managed_server_t *server,
                                         uint64_t sid,
                                         nipc_uds_server_config_t *cfg_out,
                                         nipc_shm_ctx_t **shm_out) {
  *cfg_out = server->base_config;
  cfg_out->max_request_payload_bytes =
      __atomic_load_n(&server->learned_request_payload_bytes, __ATOMIC_ACQUIRE);
  cfg_out->max_response_payload_bytes = __atomic_load_n(
      &server->learned_response_payload_bytes, __ATOMIC_ACQUIRE);
  *shm_out = NULL;

  uint32_t shm_profiles = cfg_out->supported_profiles &
                          (NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX);
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

  nipc_shm_ctx_t *shm = nipc_service_posix_calloc(
      1, sizeof(nipc_shm_ctx_t),
      NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC_INTERNAL);
  if (!shm)
    return false;

  /* HELLO has not been read yet, so the request segment must cover any
   * client proposal the handshake may legally echo back. */
  nipc_shm_error_t serr =
      nipc_shm_server_create(server->run_dir, server->service_name, sid,
                             request_capacity, response_capacity, shm);
  if (serr == NIPC_SHM_OK) {
    *shm_out = shm;
    return true;
  }

  free(shm);
  cfg_out->supported_profiles &=
      ~(NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX);
  cfg_out->preferred_profiles &=
      ~(NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX);
  return cfg_out->supported_profiles != 0;
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

void nipc_server_run(nipc_managed_server_t *server) {
  __atomic_store_n(&server->running, true, __ATOMIC_RELEASE);

  while (__atomic_load_n(&server->running, __ATOMIC_RELAXED)) {
    /* Poll the listener fd before blocking on accept */
    int pr = nipc_service_posix_poll_with_shutdown(server->listener.fd,
                                                   &server->running);
    if (pr <= 0)
      break; /* shutdown or error */

    /* Accept one client via L1 */
    nipc_uds_session_t session;
    memset(&session, 0, sizeof(session));
    session.fd = -1;

    uint64_t sid = server->next_session_id++;
    nipc_uds_server_config_t accept_cfg;
    nipc_shm_ctx_t *prepared_shm = NULL;
    if (!server_prepare_accept_config(server, sid, &accept_cfg,
                                      &prepared_shm)) {
      nipc_service_posix_sleep_us(10000);
      continue;
    }

    server->listener.config = accept_cfg;
    nipc_uds_error_t uerr = nipc_uds_accept(&server->listener, sid, &session);
    if (uerr != NIPC_UDS_OK) {
      server_destroy_precreated_shm(&prepared_shm);
      if (!__atomic_load_n(&server->running, __ATOMIC_RELAXED))
        break;
      nipc_service_posix_sleep_us(10000);
      continue;
    }

    nipc_service_common_server_note_request_capacity(
        server, session.max_request_payload_bytes);
    nipc_service_common_server_note_response_capacity(
        server, session.max_response_payload_bytes);

    /* Enforce worker_count limit: reap finished sessions, check count */
    pthread_mutex_lock(&server->sessions_lock);
    nipc_service_posix_server_reap_sessions_locked(server);

    if (server->session_count >= server->worker_count) {
      /* At capacity: reject this client by closing the session */
      pthread_mutex_unlock(&server->sessions_lock);
      server_destroy_precreated_shm(&prepared_shm);
      nipc_uds_close_session(&session);
      continue;
    }

    /* SHM profile guarantee: only negotiate SHM for sessions that already
     * have a prepared per-session SHM region. */
    nipc_shm_ctx_t *shm = prepared_shm;
    if (session.selected_profile == NIPC_PROFILE_SHM_HYBRID ||
        session.selected_profile == NIPC_PROFILE_SHM_FUTEX) {
      if (!shm) {
        pthread_mutex_unlock(&server->sessions_lock);
        nipc_uds_close_session(&session);
        continue;
      }
    } else {
      server_destroy_precreated_shm(&prepared_shm);
      shm = NULL;
    }

    /* Create session context */
    nipc_session_ctx_t *sctx = nipc_service_posix_calloc(
        1, sizeof(nipc_session_ctx_t),
        NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC_INTERNAL);
    if (!sctx) {
      if (shm) {
        nipc_shm_destroy(shm);
        free(shm);
      }
      pthread_mutex_unlock(&server->sessions_lock);
      nipc_uds_close_session(&session);
      continue;
    }

    /* The caller-owned server object stays live until destroy joins sessions.
     */
    // codeql[cpp/stack-address-escape]
    sctx->server = server;
    sctx->session = session;
    sctx->shm = shm;
    sctx->id = sid;
    __atomic_store_n(&sctx->active, true, __ATOMIC_RELAXED);

    server->sessions[server->session_count++] = sctx;
    pthread_mutex_unlock(&server->sessions_lock);

    /* Spawn handler thread for this session */
    int rc = nipc_service_posix_pthread_create(
        &sctx->thread, NULL, nipc_service_posix_session_handler_thread, sctx);
    if (rc != 0) {
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

      if (shm) {
        nipc_shm_destroy(shm);
        free(shm);
      }
      nipc_uds_close_session(&session);
      free(sctx);
    }
  }
}

void nipc_server_stop(nipc_managed_server_t *server) {
  __atomic_store_n(&server->running, false, __ATOMIC_RELEASE);
}

bool nipc_server_drain(nipc_managed_server_t *server, uint32_t timeout_ms) {
  /* 1. Stop accepting new clients.
   * Do NOT close the listener here — the run loop may still be
   * polling on listener.fd. Setting the flag is enough; the run
   * loop will exit on its next poll timeout (100ms). The listener
   * is closed later by nipc_server_destroy(). */
  __atomic_store_n(&server->running, false, __ATOMIC_RELEASE);

  /* 2. Wait for in-flight sessions to complete */
  bool all_drained = true;
  if (server->sessions) {
    struct timespec deadline;
    clock_gettime(CLOCK_MONOTONIC, &deadline);
    deadline.tv_sec += timeout_ms / 1000;
    deadline.tv_nsec += (timeout_ms % 1000) * 1000000L;
    if (deadline.tv_nsec >= 1000000000L) {
      deadline.tv_sec++;
      deadline.tv_nsec -= 1000000000L;
    }

    /* Poll until all sessions are inactive or timeout */
    while (1) {
      pthread_mutex_lock(&server->sessions_lock);
      int active_count = 0;
      for (int i = 0; i < server->session_count; i++) {
        if (__atomic_load_n(&server->sessions[i]->active, __ATOMIC_ACQUIRE))
          active_count++;
      }
      pthread_mutex_unlock(&server->sessions_lock);

      if (active_count == 0)
        break;

      struct timespec now;
      clock_gettime(CLOCK_MONOTONIC, &now);
      if (now.tv_sec > deadline.tv_sec ||
          (now.tv_sec == deadline.tv_sec && now.tv_nsec >= deadline.tv_nsec)) {
        /* Timeout: force-close session fds to unblock recv.
         * Closing the fd causes poll/recv to return error,
         * which terminates the session handler loop. */
        pthread_mutex_lock(&server->sessions_lock);
        for (int i = 0; i < server->session_count; i++) {
          nipc_session_ctx_t *s = server->sessions[i];
          if (__atomic_load_n(&s->active, __ATOMIC_ACQUIRE)) {
            if (s->session.fd >= 0) {
              shutdown(s->session.fd, SHUT_RDWR);
            }
          }
        }
        pthread_mutex_unlock(&server->sessions_lock);
        all_drained = false;
        break;
      }

      nipc_service_posix_sleep_us(5000); /* 5ms poll interval */
    }

    /* 3. Join all session threads (finished or not) */
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

  server->worker_count = 0;
  return all_drained;
}

void nipc_server_destroy(nipc_managed_server_t *server) {
  __atomic_store_n(&server->running, false, __ATOMIC_RELEASE);
  nipc_uds_close_listener(&server->listener);

  /* Join all active session threads */
  if (server->sessions) {
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

  server->worker_count = 0;
}
