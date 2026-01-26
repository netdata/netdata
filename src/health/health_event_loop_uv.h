// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_HEALTH_EVENT_LOOP_UV_H
#define NETDATA_HEALTH_EVENT_LOOP_UV_H

#include "daemon/common.h"

// Forward declaration for sqlite3_stmt (to avoid including sqlite3.h)
struct sqlite3_stmt;
typedef struct sqlite3_stmt sqlite3_stmt;

// Default/fallback for max concurrent workers (actual value from config)
#define HEALTH_DEFAULT_CONCURRENT_WORKERS 4

// Opcodes for the health event loop
enum health_opcode {
    HEALTH_NOOP = 0,
    HEALTH_TIMER_TICK,           // Timer fired, check which hosts need processing
    HEALTH_HOST_COMPLETED,       // A host finished processing (param[0] = work ptr)
    HEALTH_SAVE_ALERT_TRANSITION,// Save an alert transition (param[0] = host, param[1] = ae)
    HEALTH_DELETE_ALERT_ENTRY,   // Delete an alert entry when saves complete (param[0] = ae)
    HEALTH_SYNC_SHUTDOWN,        // Clean shutdown request
    HEALTH_MAX_ENUMERATIONS_DEFINED
};

// Set of prepared statements for health operations
// Each worker gets exclusive access to one set from the pool
struct health_stmt_set {
    bool in_use;

    // Prepared statements for alert queue processing (sqlite_aclk_alert.c)
    sqlite3_stmt *stmt_process_alert_pending_queue;
    sqlite3_stmt *stmt_insert_alert_to_submit_queue;
    sqlite3_stmt *stmt_update_alert_version_transition;
    sqlite3_stmt *stmt_cloud_status_matches;
    sqlite3_stmt *stmt_delete_alert_from_pending_queue;
    sqlite3_stmt *stmt_is_event_from_alert_variable_config;

    // Prepared statements for health log operations (sqlite_health.c)
    sqlite3_stmt *stmt_health_log_update;
    sqlite3_stmt *stmt_health_log_insert;
    sqlite3_stmt *stmt_health_log_insert_detail;
    sqlite3_stmt *stmt_alert_queue_insert;
    sqlite3_stmt *stmt_health_get_last_executed_event;
};

// Forward declaration
struct health_event_loop_config;

// Work item for processing a single host
struct health_host_work {
    uv_work_t request;
    struct health_event_loop_config *config;
    RRDHOST *host;
    struct health_stmt_set *stmts;   // Acquired statement set (exclusive access)

    // Input parameters
    time_t now;
    bool apply_hibernation_delay;

    // Output: when this host should run again
    time_t host_next_run;
};

// Pending alert list for batching saves
struct health_pending_alerts {
    Pvoid_t JudyL;
    size_t count;
};

// Health event loop configuration structure
struct health_event_loop_config {
    ND_THREAD *thread;
    uv_loop_t loop;
    uv_async_t async;
    uv_timer_t timer_req;

    bool initialized;
    bool shutdown_requested;

    struct completion start_stop_complete;
    CmdPool cmd_pool;

    // Statement pool for parallel workers (dynamically allocated)
    SPINLOCK stmt_pool_lock;
    struct health_stmt_set *stmt_pool;
    size_t max_concurrent_workers;

    // Track active workers
    size_t active_workers;

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

    // Health log cleanup timing
    time_t next_cleanup_time;
};

// Initialize and start the health event loop
void health_event_loop_init(void);

// Shutdown the health event loop gracefully
void health_event_loop_shutdown(void);

// Check if health event loop is initialized
bool health_event_loop_is_initialized(void);

// Get the config pointer (for statement set access from SQLite functions)
struct health_event_loop_config *health_event_loop_get_config(void);

// Statement set pool management (called from worker context)
struct health_stmt_set *health_stmt_set_acquire(struct health_event_loop_config *config);
void health_stmt_set_release(struct health_event_loop_config *config, struct health_stmt_set *set);

// Check if health processing should stop (for use in worker threads and loops)
// Returns true if shutdown has been requested
bool health_should_stop(void);

// Queue an alert transition to be saved asynchronously by the health event loop
// Returns true if queued successfully, false if health loop is not running
bool health_queue_alert_save(RRDHOST *host, ALARM_ENTRY *ae);

// Queue an alert entry for deletion (will be freed when pending saves complete)
// Returns true if queued successfully, false if health loop is not running
bool health_queue_alert_deletion(ALARM_ENTRY *ae);

#endif //NETDATA_HEALTH_EVENT_LOOP_UV_H
