/*
 * netipc_service_win.c - L2 orchestration for Windows.
 *
 * Pure composition of L1 (Named Pipe / Win SHM) + Codec.
 * Identical state machine and retry logic as the POSIX implementation,
 * using Windows transport calls instead of UDS/POSIX SHM.
 *
 * Client context manages connection lifecycle with at-least-once retry.
 * Managed server handles accept, read, dispatch, respond.
 */

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc/netipc_named_pipe.h"
#include "netipc/netipc_protocol.h"
#include "netipc/netipc_service.h"
#include "netipc/netipc_win_shm.h"
#include "netipc_service_common.h"
#include "netipc_service_platform.h"
#include "netipc_service_win_internal.h"

#include <process.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <windows.h>

#ifdef __MSYS__
#include <pthread.h>
#endif

static uint64_t g_win_service_test_fault_state = 0;

static uint64_t win_service_fault_state_make(int site, uint32_t skip_matches) {
  return ((uint64_t)skip_matches << 32) | (uint32_t)site;
}

void nipc_win_service_test_fault_set(int site, uint32_t skip_matches) {
  __atomic_store_n(&g_win_service_test_fault_state,
                   win_service_fault_state_make(site, skip_matches),
                   __ATOMIC_RELEASE);
}

void nipc_win_service_test_fault_clear(void) {
  __atomic_store_n(&g_win_service_test_fault_state, 0, __ATOMIC_RELEASE);
}

static bool service_test_should_fail(int site) {
  for (;;) {
    uint64_t state =
        __atomic_load_n(&g_win_service_test_fault_state, __ATOMIC_ACQUIRE);
    uint32_t current_site = (uint32_t)state;
    uint32_t current_skip = (uint32_t)(state >> 32);
    uint64_t next_state;

    if (current_site != (uint32_t)site)
      return false;

    if (current_skip > 0)
      next_state = win_service_fault_state_make(site, current_skip - 1u);
    else
      next_state = 0;

    if (__atomic_compare_exchange_n(&g_win_service_test_fault_state, &state,
                                    next_state, false, __ATOMIC_ACQ_REL,
                                    __ATOMIC_ACQUIRE))
      return current_skip == 0;
  }
}

void *nipc_service_win_malloc(size_t size, int fault_site) {
  if (service_test_should_fail(fault_site))
    return NULL;
  return malloc(size);
}

void *nipc_service_platform_malloc(size_t size, int fault_site) {
  return nipc_service_win_malloc(size, fault_site);
}

void *nipc_service_win_calloc(size_t count, size_t size, int fault_site) {
  if (service_test_should_fail(fault_site))
    return NULL;
  return calloc(count, size);
}

void *nipc_service_platform_calloc(size_t count, size_t size, int fault_site) {
  return nipc_service_win_calloc(count, size, fault_site);
}

static void *service_realloc(void *ptr, size_t size, int fault_site) {
  if (service_test_should_fail(fault_site))
    return NULL;
  return realloc(ptr, size);
}

#ifdef __MSYS__
/* The MSYS runtime does not support threads started with CreateThread: under
 * load such a thread may never run and the process dies silently. Threads
 * start through pthread_create instead, and each one duplicates its own
 * handle for the creator, so callers keep using CancelSynchronousIo,
 * WaitForSingleObject and CloseHandle on a real thread handle. */
typedef struct {
  unsigned(__stdcall *start_address)(void *);
  void *arglist;
  HANDLE started;
  HANDLE thread;
  DWORD thread_id;
} service_win_thread_start_t;

static void *service_win_thread_trampoline(void *p) {
  service_win_thread_start_t *start = (service_win_thread_start_t *)p;
  unsigned(__stdcall *start_address)(void *) = start->start_address;
  void *arglist = start->arglist;
  HANDLE thread = NULL;
  if (!DuplicateHandle(GetCurrentProcess(), GetCurrentThread(),
                       GetCurrentProcess(), &thread, 0, FALSE,
                       DUPLICATE_SAME_ACCESS))
    thread = NULL;
  start->thread = thread;
  start->thread_id = GetCurrentThreadId();
  /* start lives on the creator's stack: no access after this. */
  SetEvent(start->started);
  if (thread)
    start_address(arglist);
  return NULL;
}
#endif

uintptr_t
nipc_service_win_beginthreadex(void *security, unsigned stack_size,
                               unsigned(__stdcall *start_address)(void *),
                               void *arglist, unsigned initflag,
                               unsigned *thrdaddr) {
  if (service_test_should_fail(
          NIPC_WIN_SERVICE_TEST_FAULT_SERVER_THREAD_CREATE_INTERNAL))
    return 0;
#ifdef __MSYS__
  {
    /* Callers pass no security attributes, stack size or creation flags. */
    (void)security;
    (void)stack_size;
    (void)initflag;
    service_win_thread_start_t start = {
        .start_address = start_address,
        .arglist = arglist,
        .started = CreateEventW(NULL, TRUE, FALSE, NULL),
    };
    if (!start.started)
      return 0;
    pthread_t thread;
    if (pthread_create(&thread, NULL, service_win_thread_trampoline, &start) !=
        0) {
      CloseHandle(start.started);
      return 0;
    }
    pthread_detach(thread);
    /* A created pthread always runs, so this wait ends. */
    WaitForSingleObject(start.started, INFINITE);
    CloseHandle(start.started);
    if (thrdaddr)
      *thrdaddr = (unsigned)start.thread_id;
    return (uintptr_t)start.thread;
  }
#else
  return _beginthreadex(security, stack_size, start_address, arglist, initflag,
                        thrdaddr);
#endif
}

bool nipc_service_win_ensure_buffer(uint8_t **buf, size_t *buf_size,
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

static uint64_t monotonic_time_ms(void) { return GetTickCount64(); }

uint64_t nipc_service_platform_monotonic_ms(void) {
  return monotonic_time_ms();
}

bool nipc_service_platform_ensure_client_send_buffer(nipc_client_ctx_t *ctx,
                                                     size_t need) {
  return nipc_service_win_ensure_buffer(
      &ctx->send_buf, &ctx->send_buf_size, need,
      NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC_INTERNAL);
}

bool nipc_service_platform_ensure_client_response_buffer(nipc_client_ctx_t *ctx,
                                                         size_t need) {
  return nipc_service_win_ensure_buffer(
      &ctx->response_buf, &ctx->response_buf_size, need,
      NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC_INTERNAL);
}

/* Managed server implementation lives in netipc_service_win_server.c. */

#endif /* _WIN32 || __MSYS__ */
