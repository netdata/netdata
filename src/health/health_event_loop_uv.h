// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_EVENT_LOOP_UV_H
#define NETDATA_HEALTH_EVENT_LOOP_UV_H

#include "daemon/common.h"

// Forward declaration for sqlite3_stmt (to avoid including sqlite3.h)
struct sqlite3_stmt;

// Default/fallback for max concurrent workers (actual value from config)
#define HEALTH_DEFAULT_CONCURRENT_WORKERS 4

// Worker job IDs for the health main event loop thread only.
// Per-host processing jobs that run on libuv pool threads use UV_EVENT_HEALTH_*
// from libuv_workers.h (registered via register_libuv_worker_jobs()).
#define WORKER_HEALTH_JOB_SAVE_ALERT_TRANSITION 0
#define WORKER_HEALTH_JOB_DELETE_ALERT          1
#define WORKER_HEALTH_JOB_WAIT_EXEC             2

// Opcodes for the health event loop
enum health_opcode {
    HEALTH_NOOP = 0,
    HEALTH_TIMER_TICK,                // Timer fired; pop due hosts off the ready list
    HEALTH_SAVE_ALERT_TRANSITION,     // Save an alert transition (param[0] = ae)
    HEALTH_DELETE_ALERT_ENTRY,        // Delete an alert entry when saves complete (param[0] = ae)
    HEALTH_ENABLE_HOST_PROCESSING,    // Register a host with the ready list (param[0] = host)
    HEALTH_DISABLE_HOST_PROCESSING,   // Unregister a host from the ready list (param[0] = host)
    HEALTH_SYNC_SHUTDOWN,             // Clean shutdown request
    HEALTH_MAX_ENUMERATIONS_DEFINED
};

// Set of prepared statements for health operations. One set per worker,
// owned for the worker's lifetime — no pool, no contention.
struct health_stmt_set {
    // Prepared statements for alert queue processing (sqlite_aclk_alert.c)
    struct sqlite3_stmt *stmt_process_alert_pending_queue;
    struct sqlite3_stmt *stmt_insert_alert_to_submit_queue;
    struct sqlite3_stmt *stmt_update_alert_version_transition;
    struct sqlite3_stmt *stmt_cloud_status_matches;
    struct sqlite3_stmt *stmt_delete_alert_from_pending_queue;
    struct sqlite3_stmt *stmt_is_event_from_alert_variable_config;

    // Prepared statements for health log operations (sqlite_health.c)
    struct sqlite3_stmt *stmt_health_log_update;
    struct sqlite3_stmt *stmt_health_log_insert;
    struct sqlite3_stmt *stmt_health_log_insert_detail;
    struct sqlite3_stmt *stmt_alert_queue_insert;
    struct sqlite3_stmt *stmt_health_get_last_executed_event;
};

// Forward declaration
struct health_event_loop_config;

// Pending alert list for batching saves
struct health_pending_alerts {
    Pvoid_t JudyL;
    size_t count;
};

// Per-worker context. Each worker is a reusable uv_work_t that keeps its
// own dedicated stmt set. The main thread submits all N via uv_queue_work
// at batch start; each work_cb loops on batch.next until drained.
struct health_batch_worker {
    uv_work_t request;
    struct health_event_loop_config *config;
    size_t worker_id;
    struct health_stmt_set stmts;
    // Per-worker arena reused across all hosts this worker processes in one
    // batch. Reset between alerts inside health_event_loop_for_host, destroyed
    // when the worker pool is finalized.
    ONEWAYALLOC *owa;
};

// Batch of hosts to process on each tick. Filled by the main thread, pulled
// atomically by workers via __atomic_fetch_add(&next, 1).
struct health_batch {
    RRDHOST **hosts;
    size_t count;
    size_t capacity;
    size_t next;             // atomic cursor
    time_t now;
    bool apply_hibernation_delay;
};

// Health event loop configuration structure
struct health_event_loop_config {
    ND_THREAD *thread;
    uv_loop_t loop;
    uv_async_t async;
    uv_timer_t timer_req;

    bool initialized;           // true while accepting commands (toggled during drain)
    bool started;               // true once initialized, cleared atomically on first shutdown call
    bool shutdown_requested;

    struct completion start_stop_complete;
    CmdPool cmd_pool;

    // Persistent worker pool — N reusable uv_work_t slots, each with its
    // own stmt set. Allocated once at loop init, reused every tick.
    struct health_batch_worker *workers;
    size_t num_workers;            // = max_concurrent_workers
    size_t max_concurrent_workers;

    // Batch currently being processed (or just finished). Main thread
    // rebuilds batch.hosts each TIMER_TICK before dispatching workers.
    struct health_batch batch;

    // Counter of workers still in flight for the current batch.
    // Decremented by each after_work_cb; when it hits zero the batch is
    // complete and the main thread can start the next one.
    size_t batch_workers_active;
    bool batch_in_progress;

    // Dedicated statement set for main loop operations (alert saves)
    struct health_stmt_set main_loop_stmts;

    // Pending alert transitions to save (collected in main loop)
    struct health_pending_alerts *pending_alerts;

    // Pending alert entries to delete (when pending_save_count reaches 0)
    // Uses counter as index (not pointer) to avoid use-after-free issues
    Pvoid_t ae_pending_deletion;
    Word_t ae_deletion_next_id;

    // Hibernation detection state
    usec_t last_realtime;
    usec_t last_monotonic;

    // Health log cleanup timing and state
    time_t next_cleanup_time;
    bool cleanup_running;  // true while cleanup work is queued/executing on a worker

    // Shutdown drain loop iteration counter
    size_t drain_iterations;

};

// Initialize and start the health event loop
void health_event_loop_init(void);

// Shutdown the health event loop gracefully
void health_event_loop_shutdown(void);

// Check if health event loop is initialized
bool health_event_loop_is_initialized(void);

// Get the config pointer (for statement set access from SQLite functions)
struct health_event_loop_config *health_event_loop_get_config(void);

// Check if health processing should stop (for use in worker threads and loops)
// Returns true if shutdown has been requested
bool health_should_stop(void);

// Queue an alert transition to be saved asynchronously by the health event loop
// Returns true if queued successfully, false if health loop is not running
bool health_queue_alert_save(ALARM_ENTRY *ae);

// Queue an alert entry for deletion (will be freed when pending saves complete)
// Returns true if queued successfully, false if health loop is not running
bool health_queue_alert_deletion(ALARM_ENTRY *ae);

// Register a host with the health ready list. Safe to call from any thread
// (posts an opcode to the event loop). Idempotent — redundant calls are no-ops.
// Must be called for every host that should have its health evaluated.
void health_enable_host_processing(RRDHOST *host);

// Unregister a host from the health ready list. Safe to call from any thread.
// MUST be called before a host is freed. Callers that free the host should
// additionally wait for `host->health.processing == false` after posting.
// Idempotent — redundant calls are no-ops.
void health_disable_host_processing(RRDHOST *host);

// Global health iteration counter (defined in health_event_loop.c)
extern uint64_t health_evloop_iteration;

// Per-host health processing function (defined in health_event_loop.c, called from UV workers)
void health_event_loop_for_host(RRDHOST *host, bool apply_hibernation_delay, time_t now, time_t *next_run, struct health_stmt_set *stmts, ONEWAYALLOC *owa);

#endif //NETDATA_HEALTH_EVENT_LOOP_UV_H
