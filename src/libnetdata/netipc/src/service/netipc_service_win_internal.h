#ifndef NETIPC_SERVICE_WIN_INTERNAL_H
#define NETIPC_SERVICE_WIN_INTERNAL_H

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc_service_platform.h"

#include <process.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <windows.h>

#define SERVER_POLL_TIMEOUT_MS 100
#define CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS 5u
#define CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS 5000u
#define CLIENT_CALL_RECONNECT_RETRY_INTERVAL_MS 5u
#define CLIENT_CALL_RECONNECT_DRAIN_MS (SERVER_POLL_TIMEOUT_MS + 50u)
#define CLIENT_CALL_RECONNECT_RETRIES 20u

enum {
    NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC_INTERNAL = 1,
    NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_CLIENT_SHM_CTX_CALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_SERVER_RECV_BUF_MALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_SERVER_RESP_BUF_MALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SESSIONS_CALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_SERVER_THREAD_CREATE_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_CACHE_BUCKETS_CALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_CACHE_ITEMS_CALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_CACHE_ITEM_NAME_MALLOC_INTERNAL,
    NIPC_WIN_SERVICE_TEST_FAULT_CACHE_ITEM_PATH_MALLOC_INTERNAL,
};

const nipc_service_common_client_ops_t *nipc_service_win_client_ops(void);
void nipc_service_win_client_disconnect(nipc_client_ctx_t *ctx);
nipc_client_state_t nipc_service_win_client_try_connect(nipc_client_ctx_t *ctx);
bool nipc_service_win_client_reconnect_for_call(nipc_client_ctx_t *ctx);
void *nipc_service_win_malloc(size_t size, int fault_site);
void *nipc_service_win_calloc(size_t count, size_t size, int fault_site);
bool nipc_service_win_ensure_buffer(uint8_t **buf, size_t *buf_size,
                                    size_t need, int fault_site);
unsigned __stdcall nipc_service_win_session_handler_thread(void *arg);
void nipc_service_win_server_reap_sessions_locked(nipc_managed_server_t *server);
uintptr_t nipc_service_win_beginthreadex(void *security,
                                         unsigned stack_size,
                                         unsigned (__stdcall *start_address)(void *),
                                         void *arglist,
                                         unsigned initflag,
                                         unsigned *thrdaddr);

#endif /* _WIN32 || __MSYS__ */

#endif /* NETIPC_SERVICE_WIN_INTERNAL_H */
