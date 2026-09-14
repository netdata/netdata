/*
 * netipc_service.c - L2 orchestration implementation.
 *
 * Pure composition of L1 (UDS/SHM) + Codec. No direct socket/mmap calls
 * for data framing. Uses poll() for timeout-based shutdown detection
 * in the managed server.
 *
 * Client context manages connection lifecycle with at-least-once retry.
 * Managed server handles accept, read, dispatch, respond.
 */

#include "netipc/netipc_service.h"
#include "netipc/netipc_protocol.h"
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

#define CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS 5u
#define CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS 5000u
#define CLIENT_CALL_RECONNECT_RETRY_INTERVAL_MS 5u
#define CLIENT_CALL_RECONNECT_DRAIN_MS (SERVER_POLL_TIMEOUT_MS + 50u)
#define CLIENT_CALL_RECONNECT_RETRIES 20u

static uint64_t g_posix_service_test_fault_state = 0;

void nipc_service_posix_sleep_us(unsigned int usec) {
  struct timespec req = {
      .tv_sec = (time_t)(usec / 1000000u),
      .tv_nsec = (long)(usec % 1000000u) * 1000L,
  };

  while (nanosleep(&req, &req) == -1 && errno == EINTR) {
    // Retry with the remaining interval after signal interruption.
  }
}

static uint64_t posix_service_fault_state_make(int site,
                                               uint32_t skip_matches) {
  return ((uint64_t)skip_matches << 32) | (uint32_t)site;
}

void nipc_posix_service_test_fault_set(int site, uint32_t skip_matches) {
  __atomic_store_n(&g_posix_service_test_fault_state,
                   posix_service_fault_state_make(site, skip_matches),
                   __ATOMIC_RELEASE);
}

void nipc_posix_service_test_fault_clear(void) {
  __atomic_store_n(&g_posix_service_test_fault_state, 0, __ATOMIC_RELEASE);
}

static bool service_test_should_fail(int site) {
  for (;;) {
    uint64_t state =
        __atomic_load_n(&g_posix_service_test_fault_state, __ATOMIC_ACQUIRE);
    uint32_t current_site = (uint32_t)state;
    uint32_t current_skip = (uint32_t)(state >> 32);
    uint64_t next_state;

    if (current_site != (uint32_t)site)
      return false;

    if (current_skip > 0)
      next_state = posix_service_fault_state_make(site, current_skip - 1u);
    else
      next_state = 0;

    if (__atomic_compare_exchange_n(&g_posix_service_test_fault_state, &state,
                                    next_state, false, __ATOMIC_ACQ_REL,
                                    __ATOMIC_ACQUIRE))
      return current_skip == 0;
  }
}

void *nipc_service_posix_malloc(size_t size, int fault_site) {
  if (service_test_should_fail(fault_site))
    return NULL;
  return malloc(size);
}

void *nipc_service_platform_malloc(size_t size, int fault_site) {
  return nipc_service_posix_malloc(size, fault_site);
}

void *nipc_service_posix_calloc(size_t count, size_t size, int fault_site) {
  if (service_test_should_fail(fault_site))
    return NULL;
  return calloc(count, size);
}

void *nipc_service_platform_calloc(size_t count, size_t size, int fault_site) {
  return nipc_service_posix_calloc(count, size, fault_site);
}

static void *service_realloc(void *ptr, size_t size, int fault_site) {
  if (service_test_should_fail(fault_site))
    return NULL;
  return realloc(ptr, size);
}

int nipc_service_posix_pthread_create(pthread_t *thread,
                                      const pthread_attr_t *attr,
                                      void *(*start_routine)(void *),
                                      void *arg) {
  if (service_test_should_fail(
          NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_THREAD_CREATE_INTERNAL))
    return EAGAIN;
  return pthread_create(thread, attr, start_routine, arg);
}

static uint64_t monotonic_time_ms(void) {
  struct timespec ts;
  clock_gettime(CLOCK_MONOTONIC, &ts);
  return (uint64_t)ts.tv_sec * 1000u + (uint64_t)ts.tv_nsec / 1000000u;
}

uint64_t nipc_service_platform_monotonic_ms(void) {
  return monotonic_time_ms();
}

bool nipc_service_posix_ensure_buffer(uint8_t **buf, size_t *buf_size,
                                      size_t need, int fault_site) {
  if (*buf && *buf_size >= need)
    return true;

  uint8_t *new_buf = service_realloc(*buf, need, fault_site);
  if (!new_buf)
    return false;

  *buf = new_buf;
  *buf_size = need;
  return true;
}

bool nipc_service_platform_ensure_client_send_buffer(nipc_client_ctx_t *ctx,
                                                     size_t need) {
  return nipc_service_posix_ensure_buffer(
      &ctx->send_buf, &ctx->send_buf_size, need,
      NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC_INTERNAL);
}

bool nipc_service_platform_ensure_client_response_buffer(nipc_client_ctx_t *ctx,
                                                         size_t need) {
  return nipc_service_posix_ensure_buffer(
      &ctx->response_buf, &ctx->response_buf_size, need,
      NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC_INTERNAL);
}

/* Managed server implementation lives in netipc_service_posix_server.c. */
