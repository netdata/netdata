#ifndef NETIPC_SERVICE_POSIX_INTERNAL_H
#define NETIPC_SERVICE_POSIX_INTERNAL_H

#include "netipc_service_platform.h"

#include <pthread.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define SERVER_POLL_TIMEOUT_MS 100
#define CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS 5u
#define CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS 5000u
#define CLIENT_CALL_RECONNECT_RETRY_INTERVAL_MS 5u
#define CLIENT_CALL_RECONNECT_DRAIN_MS (SERVER_POLL_TIMEOUT_MS + 50u)
#define CLIENT_CALL_RECONNECT_RETRIES 20u

typedef struct {
    pthread_mutex_t lock;
    pthread_cond_t copied_cond;
    bool copied;
    nipc_managed_server_t *server;
    nipc_session_ctx_t *session;
} nipc_posix_session_start_arg_t;

enum {
    NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_RESPONSE_BUF_REALLOC_INTERNAL = 1,
    NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SEND_BUF_REALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CLIENT_SHM_CTX_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SHM_CTX_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RECV_BUF_MALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_RESP_BUF_MALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSIONS_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_SESSION_CTX_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_SERVER_THREAD_CREATE_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_BUCKETS_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEMS_CALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEM_NAME_MALLOC_INTERNAL,
    NIPC_POSIX_SERVICE_TEST_FAULT_CACHE_ITEM_PATH_MALLOC_INTERNAL,
};

void nipc_service_posix_sleep_us(unsigned int usec);
const nipc_service_common_client_ops_t *nipc_service_posix_client_ops(void);
void nipc_service_posix_client_disconnect(nipc_client_ctx_t *ctx);
nipc_client_state_t nipc_service_posix_client_try_connect(nipc_client_ctx_t *ctx);
bool nipc_service_posix_client_reconnect_for_call(nipc_client_ctx_t *ctx);
void *nipc_service_posix_malloc(size_t size, int fault_site);
void *nipc_service_posix_calloc(size_t count, size_t size, int fault_site);
bool nipc_service_posix_ensure_buffer(uint8_t **buf, size_t *buf_size,
                                      size_t need, int fault_site);
int nipc_service_posix_poll_with_shutdown(int fd, bool *running);
void *nipc_service_posix_session_handler_thread(void *arg);
void nipc_service_posix_server_reap_sessions_locked(nipc_managed_server_t *server);
int nipc_service_posix_pthread_create(pthread_t *thread,
                                      const pthread_attr_t *attr,
                                      void *(*start_routine)(void *),
                                      void *arg);

#endif /* NETIPC_SERVICE_POSIX_INTERNAL_H */
