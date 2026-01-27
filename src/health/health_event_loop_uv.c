// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"
#include "health_event_loop_uv.h"
#include "health-alert-entry.h"
#include "database/sqlite/sqlite_functions.h"
#include "database/sqlite/sqlite_health.h"
#include "database/sqlite/sqlite_aclk_alert.h"
#include "database/sqlite/sqlite_aclk.h"

#define HEALTH_EVENT_LOOP_NAME "HEALTH"
#define HEALTH_CMD_POOL_SIZE (512)
#define HEALTH_TIMER_INITIAL_PERIOD_MS (1000)
#define HEALTH_TIMER_REPEAT_PERIOD_MS (1000)
#define HEALTH_SHUTDOWN_SLEEP_INTERVAL_MS (100)
#define HEALTH_MAX_SHUTDOWN_TIMEOUT_SECONDS (60)
#define HEALTH_CLEANUP_FIRST_RUN_DELAY (1800)     // First cleanup 30 min after startup
#define HEALTH_CLEANUP_INTERVAL (3600)            // Cleanup every hour

// Worker job types
#define WORKER_HEALTH_JOB_RRD_LOCK              0
#define WORKER_HEALTH_JOB_HOST_LOCK             1
#define WORKER_HEALTH_JOB_DB_QUERY              2
#define WORKER_HEALTH_JOB_CALC_EVAL             3
#define WORKER_HEALTH_JOB_WARNING_EVAL          4
#define WORKER_HEALTH_JOB_CRITICAL_EVAL         5
#define WORKER_HEALTH_JOB_ALARM_LOG_ENTRY       6
#define WORKER_HEALTH_JOB_ALARM_LOG_PROCESS     7
#define WORKER_HEALTH_JOB_ALARM_LOG_QUEUE       8
#define WORKER_HEALTH_JOB_WAIT_EXEC             9
#define WORKER_HEALTH_JOB_DELAYED_INIT_RRDSET   10
#define WORKER_HEALTH_JOB_DELAYED_INIT_RRDDIM   11
#define WORKER_HEALTH_JOB_SAVE_ALERT_TRANSITION 12
#define WORKER_HEALTH_JOB_CLEANUP               13
#define WORKER_HEALTH_JOB_DELETE_ALERT          14

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 15
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 15
#endif

// Global configuration structure
static struct health_event_loop_config health_config = { 0 };

// External declaration for the per-host processing function
extern void health_event_loop_for_host(RRDHOST *host, bool apply_hibernation_delay, time_t now, time_t *next_run, struct health_stmt_set *stmts);
extern uint64_t health_evloop_iteration;

// ---------------------------------------------------------------------------------------------------------------------
// Statement set pool management

struct health_stmt_set *health_stmt_set_acquire(struct health_event_loop_config *config) {
    spinlock_lock(&config->stmt_pool_lock);
    for (size_t i = 0; i < config->max_concurrent_workers; i++) {
        if (!config->stmt_pool[i].in_use) {
            config->stmt_pool[i].in_use = true;
            spinlock_unlock(&config->stmt_pool_lock);
            return &config->stmt_pool[i];
        }
    }
    spinlock_unlock(&config->stmt_pool_lock);
    return NULL;  // Pool exhausted
}

void health_stmt_set_release(struct health_event_loop_config *config, struct health_stmt_set *set) {
    if (!set)
        return;

    // Reset all statements before releasing back to pool
    if (set->stmt_process_alert_pending_queue)
        SQLITE_RESET(set->stmt_process_alert_pending_queue);
    if (set->stmt_insert_alert_to_submit_queue)
        SQLITE_RESET(set->stmt_insert_alert_to_submit_queue);
    if (set->stmt_update_alert_version_transition)
        SQLITE_RESET(set->stmt_update_alert_version_transition);
    if (set->stmt_cloud_status_matches)
        SQLITE_RESET(set->stmt_cloud_status_matches);
    if (set->stmt_delete_alert_from_pending_queue)
        SQLITE_RESET(set->stmt_delete_alert_from_pending_queue);
    if (set->stmt_is_event_from_alert_variable_config)
        SQLITE_RESET(set->stmt_is_event_from_alert_variable_config);
    if (set->stmt_health_log_update)
        SQLITE_RESET(set->stmt_health_log_update);
    if (set->stmt_health_log_insert)
        SQLITE_RESET(set->stmt_health_log_insert);
    if (set->stmt_health_log_insert_detail)
        SQLITE_RESET(set->stmt_health_log_insert_detail);
    if (set->stmt_alert_queue_insert)
        SQLITE_RESET(set->stmt_alert_queue_insert);
    if (set->stmt_health_get_last_executed_event)
        SQLITE_RESET(set->stmt_health_get_last_executed_event);

    spinlock_lock(&config->stmt_pool_lock);
    set->in_use = false;
    spinlock_unlock(&config->stmt_pool_lock);
}

static void health_stmt_set_finalize(struct health_stmt_set *set) {
    if (!set)
        return;

    SQLITE_FINALIZE(set->stmt_process_alert_pending_queue);
    SQLITE_FINALIZE(set->stmt_insert_alert_to_submit_queue);
    SQLITE_FINALIZE(set->stmt_update_alert_version_transition);
    SQLITE_FINALIZE(set->stmt_cloud_status_matches);
    SQLITE_FINALIZE(set->stmt_delete_alert_from_pending_queue);
    SQLITE_FINALIZE(set->stmt_is_event_from_alert_variable_config);
    SQLITE_FINALIZE(set->stmt_health_log_update);
    SQLITE_FINALIZE(set->stmt_health_log_insert);
    SQLITE_FINALIZE(set->stmt_health_log_insert_detail);
    SQLITE_FINALIZE(set->stmt_alert_queue_insert);
    SQLITE_FINALIZE(set->stmt_health_get_last_executed_event);

    set->stmt_process_alert_pending_queue = NULL;
    set->stmt_insert_alert_to_submit_queue = NULL;
    set->stmt_update_alert_version_transition = NULL;
    set->stmt_cloud_status_matches = NULL;
    set->stmt_delete_alert_from_pending_queue = NULL;
    set->stmt_is_event_from_alert_variable_config = NULL;
    set->stmt_health_log_update = NULL;
    set->stmt_health_log_insert = NULL;
    set->stmt_health_log_insert_detail = NULL;
    set->stmt_alert_queue_insert = NULL;
    set->stmt_health_get_last_executed_event = NULL;
}

// ---------------------------------------------------------------------------------------------------------------------
// Command queue helpers

static cmd_data_t health_deq_cmd(void) {
    cmd_data_t ret = { 0 };
    ret.opcode = HEALTH_NOOP;
    (void)pop_cmd(&health_config.cmd_pool, &ret);
    return ret;
}

static bool health_enq_cmd(cmd_data_t *cmd, bool wait_on_full) {
    if (unlikely(!__atomic_load_n(&health_config.initialized, __ATOMIC_ACQUIRE)))
        return false;

    bool added = push_cmd(&health_config.cmd_pool, cmd, wait_on_full);
    if (added)
        (void)uv_async_send(&health_config.async);
    return added;
}

// ---------------------------------------------------------------------------------------------------------------------
// Alert transition save queue

bool health_queue_alert_save(RRDHOST *host, ALARM_ENTRY *ae) {
    if (unlikely(!host || !ae))
        return false;

    __atomic_add_fetch(&host->health.pending_transitions, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ae->pending_save_count, 1, __ATOMIC_RELAXED);

    cmd_data_t cmd = { 0 };
    cmd.opcode = HEALTH_SAVE_ALERT_TRANSITION;
    cmd.param[0] = host;
    cmd.param[1] = ae;

    if (unlikely(!health_enq_cmd(&cmd, false))) {
        // Failed to queue, reset counters
        __atomic_sub_fetch(&host->health.pending_transitions, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&ae->pending_save_count, 1, __ATOMIC_RELAXED);
        return false;
    }
    return true;
}

bool health_queue_alert_deletion(ALARM_ENTRY *ae) {
    if (unlikely(!ae))
        return false;

    cmd_data_t cmd = { 0 };
    cmd.opcode = HEALTH_DELETE_ALERT_ENTRY;
    cmd.param[0] = ae;

    if (!health_enq_cmd(&cmd, false)) {
        // Queue failed (shutdown in progress or queue full)
        // Free directly to avoid memory leak
        health_alarm_entry_free_direct(ae);
        return false;
    }
    return true;
}

static void health_process_pending_deletions(struct health_event_loop_config *config) {
    if (!config->ae_pending_deletion)
        return;

    worker_is_busy(WORKER_HEALTH_JOB_DELETE_ALERT);

    Word_t Index = 0;
    Pvoid_t *Pvalue;
    bool first = true;

    while ((Pvalue = JudyLFirstThenNext(config->ae_pending_deletion, &Index, &first))) {
        ALARM_ENTRY *ae = (ALARM_ENTRY *)*Pvalue;
        // Use ACQUIRE to synchronize with the RELEASE in health_process_pending_alerts()
        // This ensures all save operations are complete before we free the entry
        if (!__atomic_load_n(&ae->pending_save_count, __ATOMIC_ACQUIRE)) {
            // No more pending saves, safe to free.
            // Use health_alarm_entry_free_direct() instead of health_alarm_log_free_one_nochecks_nounlink()
            // to avoid recursive re-queueing for deletion.
            health_alarm_entry_free_direct(ae);
            (void)JudyLDel(&config->ae_pending_deletion, Index, PJE0);
            // Restart iteration since we modified the array
            first = true;
            Index = 0;
        }
    }

    worker_is_idle();
}

static void health_process_pending_alerts(struct health_event_loop_config *config) {
    struct health_pending_alerts *pending = config->pending_alerts;
    if (!pending || !pending->count)
        return;

    worker_is_busy(WORKER_HEALTH_JOB_SAVE_ALERT_TRANSITION);

    usec_t started = now_monotonic_usec();
    size_t entries = pending->count;

    Word_t Index = 0;
    Pvoid_t *Pvalue;
    bool first = true;

    while ((Pvalue = JudyLFirstThenNext(pending->JudyL, &Index, &first))) {
        RRDHOST *host = (RRDHOST *)*Pvalue;

        Pvalue = JudyLGet(pending->JudyL, ++Index, PJE0);
        if (!Pvalue)
            break;

        ALARM_ENTRY *ae = (ALARM_ENTRY *)*Pvalue;

        sql_health_alarm_log_save(host, ae, &config->main_loop_stmts);

        __atomic_sub_fetch(&host->health.pending_transitions, 1, __ATOMIC_RELAXED);
        // Use RELEASE to ensure sql_health_alarm_log_save() writes are visible
        // before the counter decrement is observed by deletion checks
        __atomic_sub_fetch(&ae->pending_save_count, 1, __ATOMIC_RELEASE);
    }

    usec_t ended = now_monotonic_usec();

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "HEALTH: Stored %zu alert transitions in %.2f ms",
           entries / 2, (double)(ended - started) / USEC_PER_MS);

    (void)JudyLFreeArray(&pending->JudyL, PJE0);
    freez(pending);
    config->pending_alerts = NULL;
}

// ---------------------------------------------------------------------------------------------------------------------
// Health log cleanup

#define SQL_DELETE_ORPHAN_HEALTH_LOG \
    "DELETE FROM health_log WHERE host_id NOT IN (SELECT host_id FROM host)"

#define SQL_DELETE_ORPHAN_HEALTH_LOG_DETAIL \
    "DELETE FROM health_log_detail WHERE health_log_id NOT IN (SELECT health_log_id FROM health_log)"

#define SQL_DELETE_ORPHAN_ALERT_VERSION \
    "DELETE FROM alert_version WHERE health_log_id NOT IN (SELECT health_log_id FROM health_log)"

static void health_cleanup_log(struct health_event_loop_config *config) {
    time_t now = now_realtime_sec();

    // Initialize next cleanup time on first call
    if (!config->next_cleanup_time)
        config->next_cleanup_time = now + HEALTH_CLEANUP_FIRST_RUN_DELAY;

    // Check if it's time to run cleanup
    if (now < config->next_cleanup_time)
        return;

    config->next_cleanup_time = now + HEALTH_CLEANUP_INTERVAL;

    worker_is_busy(WORKER_HEALTH_JOB_CLEANUP);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: Starting health log cleanup");

    // Cleanup each host's health log
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host) {
        sql_health_alarm_log_cleanup(host);
        if (unlikely(health_should_stop()))
            break;
    }
    dfe_done(host);

    if (unlikely(health_should_stop())) {
        worker_is_idle();
        return;
    }

    // Delete orphan records
    (void)db_execute(db_meta, SQL_DELETE_ORPHAN_HEALTH_LOG, NULL);
    (void)db_execute(db_meta, SQL_DELETE_ORPHAN_HEALTH_LOG_DETAIL, NULL);
    (void)db_execute(db_meta, SQL_DELETE_ORPHAN_ALERT_VERSION, NULL);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: Health log cleanup completed");
    worker_is_idle();
}

// ---------------------------------------------------------------------------------------------------------------------
// Libuv callbacks

static void health_async_cb(uv_async_t *handle __maybe_unused)
{
    ;
}

static void health_timer_cb(uv_timer_t *handle __maybe_unused)
{
    // Queue a timer tick command to process hosts
    cmd_data_t cmd = { 0 };
    cmd.opcode = HEALTH_TIMER_TICK;
    (void)health_enq_cmd(&cmd, false);
}

// ---------------------------------------------------------------------------------------------------------------------
// Hibernation detection

static inline bool check_if_resumed_from_suspension(struct health_event_loop_config *config) {
    usec_t realtime = now_realtime_usec();
    usec_t monotonic = now_monotonic_usec();
    bool ret = false;

    // Detect if monotonic and realtime have twice the difference
    // in which case we assume the system was just woken from hibernation
    if (config->last_realtime && config->last_monotonic &&
        realtime - config->last_realtime > 2 * (monotonic - config->last_monotonic))
        ret = true;

    config->last_realtime = realtime;
    config->last_monotonic = monotonic;

    return ret;
}

// ---------------------------------------------------------------------------------------------------------------------
// Per-host work callbacks (run in libuv thread pool)

static void health_host_work_cb(uv_work_t *req) {
    struct health_host_work *work = req->data;

    // Check for shutdown before starting work
    if (__atomic_load_n(&work->config->shutdown_requested, __ATOMIC_RELAXED)) {
        // Shutdown requested - skip processing
        return;
    }

    // Process the host (health_event_loop_for_host checks health_should_stop() internally)
    health_event_loop_for_host(work->host, work->apply_hibernation_delay, work->now, &work->host_next_run, work->stmts);
}

static void health_host_after_work_cb(uv_work_t *req, int status) {
    struct health_host_work *work = req->data;
    struct health_event_loop_config *config = work->config;

    // Update host's next run time (atomic for visibility to main loop)
    __atomic_store_n(&work->host->health.next_run, work->host_next_run, __ATOMIC_RELEASE);

    // Mark host as no longer processing
    __atomic_store_n(&work->host->health.processing, false, __ATOMIC_RELEASE);

    // Release statement set back to pool
    health_stmt_set_release(config, work->stmts);

    // Decrement active worker count
    __atomic_sub_fetch(&config->active_workers, 1, __ATOMIC_RELAXED);

    // Free the work item
    freez(work);

    if (status != 0) {
        nd_log(NDLS_DAEMON, NDLP_WARNING, "HEALTH: host work callback returned status %d", status);
    }
}

// ---------------------------------------------------------------------------------------------------------------------
// Host processing dispatch

static void health_queue_host_work(struct health_event_loop_config *config, RRDHOST *host,
                                   time_t now, bool apply_hibernation_delay) {
    // Try to acquire a statement set
    struct health_stmt_set *stmts = health_stmt_set_acquire(config);
    if (!stmts) {
        // Pool exhausted - host will be retried on next timer tick
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "HEALTH: Statement pool exhausted (%zu workers active), host '%s' will be processed on next tick",
               __atomic_load_n(&config->active_workers, __ATOMIC_RELAXED), rrdhost_hostname(host));
        return;
    }

    // Mark host as processing
    __atomic_store_n(&host->health.processing, true, __ATOMIC_RELEASE);

    // Allocate work item
    struct health_host_work *work = callocz(1, sizeof(*work));
    work->request.data = work;
    work->config = config;
    work->host = host;
    work->stmts = stmts;
    work->now = now;
    work->apply_hibernation_delay = apply_hibernation_delay;
    work->host_next_run = now + health_globals.config.run_at_least_every_seconds;

    // Increment active worker count
    __atomic_add_fetch(&config->active_workers, 1, __ATOMIC_RELAXED);

    // Queue work to thread pool
    int rc = uv_queue_work(&config->loop, &work->request, health_host_work_cb, health_host_after_work_cb);
    if (rc != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "HEALTH: failed to queue work for host %s: %s",
               rrdhost_hostname(host), uv_strerror(rc));

        // Cleanup on failure
        __atomic_store_n(&host->health.processing, false, __ATOMIC_RELEASE);
        health_stmt_set_release(config, stmts);
        __atomic_sub_fetch(&config->active_workers, 1, __ATOMIC_RELAXED);
        freez(work);
    }
}

static void health_process_timer_tick(struct health_event_loop_config *config) {
    if (!stream_control_health_should_be_running()) {
        return;
    }

    time_t now = now_realtime_sec();
    bool apply_hibernation_delay = false;

    if (unlikely(check_if_resumed_from_suspension(config))) {
        apply_hibernation_delay = true;

        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "HEALTH: Postponing alarm checks for %"PRId32" seconds, "
               "because it seems that the system was just resumed from suspension.",
               (int32_t)health_globals.config.postpone_alarms_during_hibernation_for_seconds);
        schedule_node_state_update(localhost, 10);
    }

    if (unlikely(silencers->all_alarms && silencers->stype == STYPE_DISABLE_ALARMS)) {
        static int logged = 0;
        if (!logged) {
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "HEALTH: Skipping health checks, because all alarms are disabled via API command.");
            logged = 1;
        }
    }

    // Increment global iteration counter
    __atomic_add_fetch(&health_evloop_iteration, 1, __ATOMIC_RELAXED);

    // Iterate over all hosts and queue those that need processing
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host) {
        if (unlikely(health_should_stop()))
            break;

        // Skip if host is already being processed
        if (__atomic_load_n(&host->health.processing, __ATOMIC_ACQUIRE))
            continue;

        // Skip if not time to run yet (use atomic load since workers update this)
        if (__atomic_load_n(&host->health.next_run, __ATOMIC_ACQUIRE) > now)
            continue;

        // Skip if we've reached max concurrent workers
        if (__atomic_load_n(&config->active_workers, __ATOMIC_RELAXED) >= config->max_concurrent_workers)
            break;

        health_queue_host_work(config, host, now, apply_hibernation_delay);
    }
    dfe_done(host);
}

// ---------------------------------------------------------------------------------------------------------------------
// Prepared statement finalization

static void health_finalize_all_statements(struct health_event_loop_config *config) {
    // Finalize main loop statements (used for alert saves)
    health_stmt_set_finalize(&config->main_loop_stmts);

    // Finalize worker pool statements
    if (!config->stmt_pool)
        return;

    for (size_t i = 0; i < config->max_concurrent_workers; i++) {
        health_stmt_set_finalize(&config->stmt_pool[i]);
    }
    freez(config->stmt_pool);
    config->stmt_pool = NULL;
}

// ---------------------------------------------------------------------------------------------------------------------
// Main event loop

static void health_event_loop(void *arg) {
    struct health_event_loop_config *config = arg;
    uv_thread_set_name_np(HEALTH_EVENT_LOOP_NAME);
    worker_register(HEALTH_EVENT_LOOP_NAME);

    init_cmd_pool(&config->cmd_pool, HEALTH_CMD_POOL_SIZE);

    // Register worker job names
    worker_register_job_name(WORKER_HEALTH_JOB_RRD_LOCK, "rrd lock");
    worker_register_job_name(WORKER_HEALTH_JOB_HOST_LOCK, "host lock");
    worker_register_job_name(WORKER_HEALTH_JOB_DB_QUERY, "db lookup");
    worker_register_job_name(WORKER_HEALTH_JOB_CALC_EVAL, "calc eval");
    worker_register_job_name(WORKER_HEALTH_JOB_WARNING_EVAL, "warning eval");
    worker_register_job_name(WORKER_HEALTH_JOB_CRITICAL_EVAL, "critical eval");
    worker_register_job_name(WORKER_HEALTH_JOB_ALARM_LOG_ENTRY, "alert log entry");
    worker_register_job_name(WORKER_HEALTH_JOB_ALARM_LOG_PROCESS, "alert log process");
    worker_register_job_name(WORKER_HEALTH_JOB_ALARM_LOG_QUEUE, "alert log queue");
    worker_register_job_name(WORKER_HEALTH_JOB_WAIT_EXEC, "alert wait exec");
    worker_register_job_name(WORKER_HEALTH_JOB_DELAYED_INIT_RRDSET, "rrdset init");
    worker_register_job_name(WORKER_HEALTH_JOB_DELAYED_INIT_RRDDIM, "rrddim init");
    worker_register_job_name(WORKER_HEALTH_JOB_SAVE_ALERT_TRANSITION, "alert save");
    worker_register_job_name(WORKER_HEALTH_JOB_CLEANUP, "health cleanup");
    worker_register_job_name(WORKER_HEALTH_JOB_DELETE_ALERT, "alert delete");

    // Initialize statement pool (size from config, fall back to default)
    config->max_concurrent_workers = health_globals.config.max_concurrent_workers;
    if (config->max_concurrent_workers == 0)
        config->max_concurrent_workers = HEALTH_DEFAULT_CONCURRENT_WORKERS;

    spinlock_init(&config->stmt_pool_lock);
    config->stmt_pool = callocz(config->max_concurrent_workers, sizeof(struct health_stmt_set));

    nd_log(NDLS_DAEMON, NDLP_INFO, "HEALTH: initialized with %zu concurrent workers",
           config->max_concurrent_workers);

    // Initialize libuv event loop
    uv_loop_t *loop = &config->loop;
    fatal_assert(0 == uv_loop_init(loop));
    fatal_assert(0 == uv_async_init(loop, &config->async, health_async_cb));
    fatal_assert(0 == uv_timer_init(loop, &config->timer_req));
    fatal_assert(0 == uv_timer_start(&config->timer_req, health_timer_cb,
                                     HEALTH_TIMER_INITIAL_PERIOD_MS, HEALTH_TIMER_REPEAT_PERIOD_MS));

    loop->data = config;
    config->async.data = config;
    config->timer_req.data = config;

    // Mark as initialized and signal completion
    // Use RELEASE to ensure all initialization (stmt_pool, loop, etc.) is visible
    // before other threads see initialized=true
    __atomic_store_n(&config->shutdown_requested, false, __ATOMIC_RELAXED);
    __atomic_store_n(&config->initialized, true, __ATOMIC_RELEASE);
    completion_mark_complete(&config->start_stop_complete);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: event loop started");

    // Main event loop
    while (likely(!health_should_stop())) {
        enum health_opcode opcode;

        worker_is_idle();
        uv_run(loop, UV_RUN_ONCE);

        // Process commands from queue
        do {
            cmd_data_t cmd = health_deq_cmd();
            opcode = cmd.opcode;

            switch (opcode) {
                case HEALTH_NOOP:
                    break;

                case HEALTH_TIMER_TICK:
                    // Process any pending alert saves first
                    health_process_pending_alerts(config);
                    // Process any pending deletions
                    health_process_pending_deletions(config);
                    health_process_timer_tick(config);
                    // Run periodic cleanup if it's time
                    health_cleanup_log(config);
                    break;

                case HEALTH_HOST_COMPLETED:
                    // Host completion is handled in after_work callback
                    break;

                case HEALTH_SAVE_ALERT_TRANSITION: {
                    // Collect alert transitions for batch processing
                    RRDHOST *host = (RRDHOST *)cmd.param[0];
                    ALARM_ENTRY *ae = (ALARM_ENTRY *)cmd.param[1];

                    if (!config->pending_alerts)
                        config->pending_alerts = callocz(1, sizeof(*config->pending_alerts));

                    Pvoid_t *Pvalue = JudyLIns(&config->pending_alerts->JudyL,
                                               ++config->pending_alerts->count, PJE0);
                    if (unlikely(Pvalue == PJERR))
                        fatal("HEALTH: Failed to insert host into pending_alerts Judy array");
                    *Pvalue = (void *)host;

                    Pvalue = JudyLIns(&config->pending_alerts->JudyL,
                                      ++config->pending_alerts->count, PJE0);
                    if (unlikely(Pvalue == PJERR))
                        fatal("HEALTH: Failed to insert ae into pending_alerts Judy array");
                    *Pvalue = (void *)ae;
                    break;
                }

                case HEALTH_DELETE_ALERT_ENTRY: {
                    // Track alert entry for deletion when pending saves complete
                    // Use counter as index (not pointer) to avoid use-after-free if
                    // memory is reused before deletion completes
                    ALARM_ENTRY *ae = (ALARM_ENTRY *)cmd.param[0];
                    Pvoid_t *Pvalue = JudyLIns(&config->ae_pending_deletion, ++config->ae_deletion_next_id, PJE0);
                    if (Pvalue == PJERR)
                        nd_log(NDLS_DAEMON, NDLP_ERR, "HEALTH: Failed to track alert entry for deletion");
                    else
                        *Pvalue = ae;
                    break;
                }

                case HEALTH_SYNC_SHUTDOWN:
                    __atomic_store_n(&config->shutdown_requested, true, __ATOMIC_RELAXED);
                    break;

                default:
                    nd_log(NDLS_DAEMON, NDLP_ERR, "HEALTH: Unknown opcode %d", opcode);
                    break;
            }

            if (likely(opcode != HEALTH_NOOP))
                uv_run(loop, UV_RUN_NOWAIT);

        } while (opcode != HEALTH_NOOP && !health_should_stop());
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: event loop shutting down");

    // Process any remaining pending alerts before shutdown
    health_process_pending_alerts(config);

    // Wait for all in-flight notifications to complete before freeing alert entries
    // This prevents use-after-free when notification processes access alert data
    worker_is_busy(WORKER_HEALTH_JOB_WAIT_EXEC);
    wait_for_all_notifications_to_finish_before_allowing_health_to_be_cleaned_up();
    worker_is_idle();

    // Free any remaining pending deletions directly (bypass pending_save_count check since we're shutting down)
    if (config->ae_pending_deletion) {
        Word_t Index = 0;
        Pvoid_t *Pvalue;
        bool first = true;
        size_t count = 0;

        while ((Pvalue = JudyLFirstThenNext(config->ae_pending_deletion, &Index, &first))) {
            ALARM_ENTRY *ae = (ALARM_ENTRY *)*Pvalue;
            health_alarm_entry_free_direct(ae);
            count++;
        }

        (void)JudyLFreeArray(&config->ae_pending_deletion, PJE0);
        config->ae_pending_deletion = NULL;

        if (count)
            nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: freed %zu pending alert deletions at shutdown", count);
    }

    // Shutdown sequence
    // Use RELEASE to ensure any pending work sees this before we tear down
    __atomic_store_n(&config->initialized, false, __ATOMIC_RELEASE);

    // Stop timer
    if (!uv_timer_stop(&config->timer_req))
        uv_close((uv_handle_t *)&config->timer_req, NULL);

    // Close async handle
    uv_close((uv_handle_t *)&config->async, NULL);

    // Walk and close all handles
    uv_walk(loop, libuv_close_callback, NULL);

    // Wait for pending workers and callbacks with timeout
    size_t loop_count = (HEALTH_MAX_SHUTDOWN_TIMEOUT_SECONDS * 1000) / HEALTH_SHUTDOWN_SLEEP_INTERVAL_MS;
    while ((__atomic_load_n(&config->active_workers, __ATOMIC_RELAXED) > 0 || uv_run(loop, UV_RUN_NOWAIT)) &&
           loop_count > 0) {
        sleep_usec(HEALTH_SHUTDOWN_SLEEP_INTERVAL_MS * USEC_PER_MS);
        loop_count--;
    }

    if (__atomic_load_n(&config->active_workers, __ATOMIC_RELAXED) > 0) {
        nd_log(NDLS_DAEMON, NDLP_WARNING, "HEALTH: %zu workers still active at shutdown",
               __atomic_load_n(&config->active_workers, __ATOMIC_RELAXED));
    }

    // Close the loop
    int rc = uv_loop_close(loop);
    if (rc != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING, "HEALTH: uv_loop_close returned %d", rc);

    // Finalize all prepared statements in the pool
    health_finalize_all_statements(config);

    // Release command pool
    release_cmd_pool(&config->cmd_pool);

    worker_unregister();
    service_exits();
    completion_mark_complete(&config->start_stop_complete);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: event loop shutdown complete");
}

// ---------------------------------------------------------------------------------------------------------------------
// Public API

void health_event_loop_init(void) {
    memset(&health_config, 0, sizeof(health_config));
    completion_init(&health_config.start_stop_complete);

    health_config.thread = nd_thread_create(HEALTH_EVENT_LOOP_NAME, NETDATA_THREAD_OPTION_DEFAULT,
                                            health_event_loop, &health_config);
    fatal_assert(NULL != health_config.thread);

    // Wait for initialization to complete
    completion_wait_for(&health_config.start_stop_complete);
    completion_reset(&health_config.start_stop_complete);

    nd_log(NDLS_DAEMON, NDLP_INFO, "HEALTH: event loop initialized");
}

void health_event_loop_shutdown(void) {
    if (!__atomic_load_n(&health_config.initialized, __ATOMIC_ACQUIRE)) {
        nd_log(NDLS_DAEMON, NDLP_WARNING, "HEALTH: event loop not initialized, skipping shutdown");
        return;
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: requesting shutdown");

    // Send shutdown command
    cmd_data_t cmd = { 0 };
    cmd.opcode = HEALTH_SYNC_SHUTDOWN;

    if (!health_enq_cmd(&cmd, true)) {
        // Command queue failed - signal shutdown directly and wake the event loop
        nd_log(NDLS_DAEMON, NDLP_WARNING, "HEALTH: Failed to queue shutdown command, signaling directly");
        __atomic_store_n(&health_config.shutdown_requested, true, __ATOMIC_RELAXED);
        (void)uv_async_send(&health_config.async);
    }

    // Wait for shutdown to complete
    completion_wait_for(&health_config.start_stop_complete);
    completion_destroy(&health_config.start_stop_complete);

    // Join the thread
    int rc = nd_thread_join(health_config.thread);
    if (rc)
        nd_log(NDLS_DAEMON, NDLP_ERR, "HEALTH: Failed to join event loop thread");
    else
        nd_log(NDLS_DAEMON, NDLP_INFO, "HEALTH: event loop shutdown completed");
}

bool health_event_loop_is_initialized(void) {
    return __atomic_load_n(&health_config.initialized, __ATOMIC_ACQUIRE);
}

struct health_event_loop_config *health_event_loop_get_config(void) {
    return &health_config;
}

bool health_should_stop(void) {
    // Check the UV loop shutdown flag
    return __atomic_load_n(&health_config.shutdown_requested, __ATOMIC_RELAXED);
}
