// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DAEMON_WATCHER_H
#define DAEMON_WATCHER_H

#include "libnetdata/libnetdata.h"

typedef enum {
    WATCHER_STEP_ID_CLOSE_WEBRTC_CONNECTIONS,
    WATCHER_STEP_ID_DISABLE_MAINTENANCE_NEW_QUERIES_NEW_WEB_REQUESTS_NEW_STREAMING_CONNECTIONS,
    WATCHER_STEP_ID_STOP_MAINTENANCE_THREAD,
    WATCHER_STEP_ID_STOP_EXPORTERS_HEALTH_AND_WEB_SERVERS_THREADS,
    WATCHER_STEP_ID_STOP_WEBSOCKET_THREADS,
    WATCHER_STEP_ID_STOP_COLLECTORS_AND_STREAMING_THREADS,
    WATCHER_STEP_ID_STOP_REPLICATION_THREADS,
    WATCHER_STEP_ID_DISABLE_ML_DETEC_AND_TRAIN_THREADS,
    WATCHER_STEP_ID_STOP_CONTEXT_THREAD,
    WATCHER_STEP_ID_CLEAR_WEB_CLIENT_CACHE,
    WATCHER_STEP_ID_STOP_ACLK_SYNC_THREAD,
    WATCHER_STEP_ID_STOP_ACLK_MQTT_THREAD,
    WATCHER_STEP_ID_STOP_ALL_REMAINING_WORKER_THREADS,
    WATCHER_STEP_ID_CANCEL_MAIN_THREADS,
    WATCHER_STEP_ID_STOP_COLLECTION_FOR_ALL_HOSTS,
    WATCHER_STEP_ID_WAIT_FOR_DBENGINE_COLLECTORS_TO_FINISH,
    WATCHER_STEP_ID_STOP_DBENGINE_TIERS,
    WATCHER_STEP_ID_STOP_METASYNC_THREADS,
    WATCHER_STEP_ID_JOIN_STATIC_THREADS,
    WATCHER_STEP_ID_CLOSE_SQL_DATABASES,
    WATCHER_STEP_ID_REMOVE_PID_FILE,
    WATCHER_STEP_ID_FREE_OPENSSL_STRUCTURES,

    // Always keep this as the last enum value
    WATCHER_STEP_ID_MAX
} watcher_step_id_t;

typedef struct {
    const char *msg;
    struct completion p;
} watcher_step_t;

extern watcher_step_t *watcher_steps;

void watcher_thread_start(void);
void watcher_thread_stop(void);

void watcher_shutdown_begin(void);
void watcher_shutdown_end(void);

void watcher_step_complete(watcher_step_id_t step_id);

// Register a callback invoked once, before abort(), when the watcher times out.
// On Windows this is used by winsvc.cc to report SERVICE_STOPPED to the SCM
// so the service does not appear to crash rather than stop. Pass NULL to clear.
void nd_register_shutdown_timeout_cb(void (*cb)(void));

#ifdef OS_WINDOWS
// Stops the Windows stop-pending heartbeat thread and publishes
// SERVICE_STOPPED to the SCM, all while holding the svc_status lock so the
// heartbeat cannot publish SERVICE_STOP_PENDING after the SCM has been told
// the service is stopped. The watcher invokes this before calling the
// registered shutdown-timeout callback so the SCM sees a coherent final
// state before the process aborts.
//
// Exposed here (rather than only inside winsvc.cc) so daemon-shutdown-watcher.c
// can call it without taking a C++ dependency on the service entry point.
// extern "C" so the C source file links against the C++ definition without
// name-mangling surprises.
#ifdef __cplusplus
extern "C" {
#endif
void netdata_svc_shutdown_aborted(void);
#ifdef __cplusplus
}
#endif
#endif

#endif /* DAEMON_WATCHER_H */
