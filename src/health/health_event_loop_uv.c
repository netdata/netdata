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
#define HEALTH_MAX_DRAIN_ITERATIONS (50)          // Max drain loop iterations during shutdown
// Global configuration structure
static struct health_event_loop_config health_config = { 0 };

// ---------------------------------------------------------------------------------------------------------------------
// Statement set lifecycle

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

bool health_queue_alert_save(ALARM_ENTRY *ae) {
    if (unlikely(!ae || !ae->host))
        return false;

    __atomic_add_fetch(&ae->host->health.pending_transitions, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ae->pending_save_count, 1, __ATOMIC_RELAXED);

    cmd_data_t cmd = { 0 };
    cmd.opcode = HEALTH_SAVE_ALERT_TRANSITION;
    cmd.param[0] = ae;

    if (unlikely(!health_enq_cmd(&cmd, false))) {
        // Failed to queue (pool full or shutting down), reset counters.
        // Caller will fall back to a synchronous save.
        __atomic_sub_fetch(&ae->host->health.pending_transitions, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&ae->pending_save_count, 1, __ATOMIC_RELAXED);
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "HEALTH: alert save queue full or shutting down, falling back to sync save");
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

    if (!health_enq_cmd(&cmd, true)) {
        // Enqueue failed — the loop is either not initialized or shutting down.
        // In both cases no in-flight save still references the entry:
        //   - Not initialized: no saves were ever queued.
        //   - Shutting down: initialized is cleared only after
        //     health_process_pending_alerts() has drained every queued save, so all
        //     pending_save_count decrements have already happened.
        // Free unconditionally to avoid leaking unlinked entries whose caller
        // (health_alarm_log_free_one_nochecks_nounlink) has no other owner path.
        health_alarm_entry_free_direct(ae);
        return false;
    }
    return true;
}

// ---------------------------------------------------------------------------------------------------------------------
// Host registration API (callable from any thread)

void health_enable_host_processing(RRDHOST *host) {
    if (unlikely(!host))
        return;

    cmd_data_t cmd = { 0 };
    cmd.opcode = HEALTH_ENABLE_HOST_PROCESSING;
    cmd.param[0] = host;
    (void)health_enq_cmd(&cmd, false);
}

void health_disable_host_processing(RRDHOST *host) {
    if (unlikely(!host))
        return;

    cmd_data_t cmd = { 0 };
    cmd.opcode = HEALTH_DISABLE_HOST_PROCESSING;
    cmd.param[0] = host;
    (void)health_enq_cmd(&cmd, false);
}

static size_t health_cmd_queue_depth(struct health_event_loop_config *config) {
    size_t count;

    netdata_mutex_lock(&config->cmd_pool.lock);
    count = (size_t)config->cmd_pool.count;
    netdata_mutex_unlock(&config->cmd_pool.lock);

    return count;
}

static bool health_has_pending_work(struct health_event_loop_config *config) {
    if (health_cmd_queue_depth(config) > 0)
        return true;

    if (__atomic_load_n(&config->batch_workers_active, __ATOMIC_RELAXED) > 0)
        return true;

    if (config->pending_alerts && config->pending_alerts->count)
        return true;

    if (config->ae_pending_deletion)
        return true;

    if (config->cleanup_running)
        return true;

    return false;
}

static void health_process_pending_deletions(struct health_event_loop_config *config) {
    if (!config->ae_pending_deletion)
        return;

    worker_is_busy(WORKER_HEALTH_JOB_DELETE_ALERT);

    // First pass: collect indices that are ready to delete
    Word_t ready_indices[128];
    size_t ready_count;

    do {
        ready_count = 0;
        Word_t Index = 0;
        Pvoid_t *Pvalue;
        bool first = true;

        while ((Pvalue = JudyLFirstThenNext(config->ae_pending_deletion, &Index, &first))) {
            ALARM_ENTRY *ae = (ALARM_ENTRY *)*Pvalue;
            // Use ACQUIRE to synchronize with the RELEASE in health_process_pending_alerts()
            // This ensures all save operations are complete before we free the entry
            if (!__atomic_load_n(&ae->pending_save_count, __ATOMIC_ACQUIRE)) {
                ready_indices[ready_count++] = Index;
                if (ready_count >= 128)
                    break;
            }
        }

        // Second pass: delete collected entries
        for (size_t i = 0; i < ready_count; i++) {
            Pvalue = JudyLGet(config->ae_pending_deletion, ready_indices[i], PJE0);
            if (Pvalue) {
                ALARM_ENTRY *ae = (ALARM_ENTRY *)*Pvalue;
                // Use health_alarm_entry_free_direct() instead of
                // health_alarm_log_free_one_nochecks_nounlink() to avoid
                // recursive re-queueing for deletion.  These entries have
                // already been unlinked from the host's alarm log.
                health_alarm_entry_free_direct(ae);
                (void)JudyLDel(&config->ae_pending_deletion, ready_indices[i], PJE0);
            }
        }
    } while (ready_count == 128);  // Continue if batch was full (more may be ready)

    worker_is_idle();
}

static void health_track_pending_deletion(struct health_event_loop_config *config, ALARM_ENTRY *ae) {
    Pvoid_t *Pvalue = JudyLIns(&config->ae_pending_deletion, ++config->ae_deletion_next_id, PJE0);
    if (likely(Pvalue != PJERR)) {
        *Pvalue = ae;
        return;
    }

    // These entries have already been unlinked from the host's alarm log.
    // If no save still references the entry, free it directly; otherwise we
    // cannot safely continue because the entry would be lost without a valid
    // deletion-tracking path.
    if (!__atomic_load_n(&ae->pending_save_count, __ATOMIC_ACQUIRE)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "HEALTH: Failed to track alert entry for deletion, freeing it immediately");
        health_alarm_entry_free_direct(ae);
        return;
    }

    fatal("HEALTH: Failed to track alert entry for deletion with pending saves still in flight");
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
        ALARM_ENTRY *ae = (ALARM_ENTRY *)*Pvalue;
        RRDHOST *host = ae->host;

        sql_health_alarm_log_save(host, ae, &config->main_loop_stmts);

        __atomic_sub_fetch(&host->health.pending_transitions, 1, __ATOMIC_RELAXED);
        // Use RELEASE to ensure sql_health_alarm_log_save() writes are visible
        // before the counter decrement is observed by deletion checks
        __atomic_sub_fetch(&ae->pending_save_count, 1, __ATOMIC_RELEASE);
    }

    usec_t ended = now_monotonic_usec();

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "HEALTH: Stored %zu alert transitions in %.2f ms",
           entries, (double)(ended - started) / USEC_PER_MS);

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

// Work item for cleanup running on a UV worker thread
struct health_cleanup_work {
    uv_work_t request;
    struct health_event_loop_config *config;
};

static void health_cleanup_work_cb(uv_work_t *req) {
    UNUSED(req);
    register_libuv_worker_jobs();
    worker_is_busy(UV_EVENT_HEALTH_CLEANUP);
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

static void health_cleanup_after_work_cb(uv_work_t *req, int status __maybe_unused) {
    struct health_cleanup_work *work = req->data;
    work->config->cleanup_running = false;
    freez(work);
}

static void health_cleanup_log(struct health_event_loop_config *config) {
    time_t now = now_realtime_sec();

    // Initialize next cleanup time on first call
    if (!config->next_cleanup_time)
        config->next_cleanup_time = now + HEALTH_CLEANUP_FIRST_RUN_DELAY;

    // Check if it's time to run cleanup, and not already running
    if (now < config->next_cleanup_time || config->cleanup_running)
        return;

    config->next_cleanup_time = now + HEALTH_CLEANUP_INTERVAL;
    config->cleanup_running = true;

    struct health_cleanup_work *work = callocz(1, sizeof(*work));
    work->request.data = work;
    work->config = config;

    int rc = uv_queue_work(&config->loop, &work->request, health_cleanup_work_cb, health_cleanup_after_work_cb);
    if (rc != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "HEALTH: failed to queue cleanup work: %s", uv_strerror(rc));
        config->cleanup_running = false;
        freez(work);
    }
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
// Batch worker callbacks — run on libuv thread pool
//
// Each tick, the main thread fills config->batch and dispatches all N
// workers via uv_queue_work. Each work_cb loops on an atomic cursor,
// pulling hosts until the batch is drained. After-work runs on the main
// loop thread and bumps a counter; the last worker kicks the next tick.

static void health_batch_work_cb(uv_work_t *req) {
    struct health_batch_worker *w = req->data;
    struct health_event_loop_config *config = w->config;

    // Register this pool thread with the LIBUV worker utilization system
    // (one-time per thread via a thread-local guard inside the helper).
    register_libuv_worker_jobs();

    for (;;) {
        if (unlikely(__atomic_load_n(&config->shutdown_requested, __ATOMIC_RELAXED)))
            return;

        // Atomic pull: each worker grabs the next index until drained.
        size_t idx = __atomic_fetch_add(&config->batch.next, 1, __ATOMIC_RELAXED);
        if (idx >= config->batch.count)
            return;

        RRDHOST *host = config->batch.hosts[idx];

        // Mark the host as processing so that rrdhost teardown can wait for
        // us to release it before freeing. Cleared by the main thread after
        // the whole batch drains (see health_batch_after_work_cb).
        __atomic_store_n(&host->health.processing, true, __ATOMIC_RELEASE);

        time_t host_next_run = config->batch.now + health_globals.config.run_at_least_every_seconds;
        health_event_loop_for_host(host, config->batch.apply_hibernation_delay,
                                   config->batch.now, &host_next_run, &w->stmts, w->owa);
        __atomic_store_n(&host->health.next_run, host_next_run, __ATOMIC_RELEASE);

        __atomic_store_n(&host->health.processing, false, __ATOMIC_RELEASE);
    }
}

static void health_batch_after_work_cb(uv_work_t *req, int status) {
    struct health_batch_worker *w = req->data;
    struct health_event_loop_config *config = w->config;

    if (status != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "HEALTH: batch worker %zu returned status %d", w->worker_id, status);

    size_t remaining = __atomic_sub_fetch(&config->batch_workers_active, 1, __ATOMIC_ACQ_REL);
    if (remaining == 0) {
        // Last worker out — batch is complete. We deliberately do NOT
        // immediately start another batch here; the next one will be kicked
        // off by the periodic timer (health_timer_cb). That caps us at one
        // batch per HEALTH_TIMER_REPEAT_PERIOD_MS and avoids spinning through
        // the whole host set in a tight loop when eval is cheap.
        config->batch_in_progress = false;
    }
}

// ---------------------------------------------------------------------------------------------------------------------
// Batch dispatch (main thread)

static void health_batch_grow_to(struct health_event_loop_config *config, size_t needed) {
    if (needed <= config->batch.capacity)
        return;
    size_t new_cap = config->batch.capacity ? config->batch.capacity : 128;
    while (new_cap < needed) new_cap *= 2;
    config->batch.hosts = reallocz(config->batch.hosts, new_cap * sizeof(RRDHOST *));
    config->batch.capacity = new_cap;
}

static void health_process_timer_tick(struct health_event_loop_config *config) {
    // Skip if the previous batch is still running — we'll start the next one
    // from the last worker's after_work_cb.
    if (config->batch_in_progress)
        return;

    if (!stream_control_health_should_be_running())
        return;

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

    __atomic_add_fetch(&health_evloop_iteration, 1, __ATOMIC_RELAXED);

    // Build the batch: walk the host index, collect everyone that's registered
    // for health processing. Per-alert scheduling happens inside
    // health_event_loop_for_host (via rc->next_update), so we do not filter
    // by host->health.next_run here — matches the master branch's "check all
    // hosts every cycle" model.
    config->batch.count = 0;
    config->batch.now = now;
    config->batch.apply_hibernation_delay = apply_hibernation_delay;

    RRDHOST *host;
    dfe_start_read(rrdhost_root_index, host) {
        if (!__atomic_load_n(&host->health.evloop_registered, __ATOMIC_ACQUIRE))
            continue;
        health_batch_grow_to(config, config->batch.count + 1);
        config->batch.hosts[config->batch.count++] = host;
    }
    dfe_done(host);

    if (config->batch.count == 0)
        return;  // no hosts to process — next timer tick will retry

    // Dispatch all workers. They pull atomically from batch.next.
    __atomic_store_n(&config->batch.next, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&config->batch_workers_active, config->num_workers, __ATOMIC_RELEASE);
    config->batch_in_progress = true;

    for (size_t i = 0; i < config->num_workers; i++) {
        int rc = uv_queue_work(&config->loop, &config->workers[i].request,
                                health_batch_work_cb, health_batch_after_work_cb);
        if (rc != 0) {
            // Extremely unlikely (libuv only fails on allocation). Count the
            // worker as complete so we don't wedge the batch.
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "HEALTH: failed to dispatch batch worker %zu: %s",
                   i, uv_strerror(rc));
            size_t remaining = __atomic_sub_fetch(&config->batch_workers_active, 1, __ATOMIC_ACQ_REL);
            if (remaining == 0) {
                config->batch_in_progress = false;
            }
        }
    }
}

// ---------------------------------------------------------------------------------------------------------------------
// Prepared statement finalization

static void health_finalize_all_statements(struct health_event_loop_config *config, bool finalize_workers) {
    // Main loop statements (alert saves)
    health_stmt_set_finalize(&config->main_loop_stmts);

    if (!finalize_workers || !config->workers)
        return;

    for (size_t i = 0; i < config->num_workers; i++) {
        health_stmt_set_finalize(&config->workers[i].stmts);
        if (config->workers[i].owa) {
            onewayalloc_destroy(config->workers[i].owa);
            config->workers[i].owa = NULL;
        }
    }
}

// ---------------------------------------------------------------------------------------------------------------------
// Shutdown helpers

// Drain all remaining commands from the cmd_pool, collecting save/delete
// entries so they can be processed before final teardown. Commands other
// than SAVE/DELETE (e.g. stale TIMER_TICKs) are harmlessly discarded.
static void health_drain_remaining_commands(struct health_event_loop_config *config) {
    size_t saved = 0, deleted = 0, discarded = 0;

    while (true) {
        cmd_data_t cmd = { 0 };
        cmd.opcode = HEALTH_NOOP;
        (void)pop_cmd(&config->cmd_pool, &cmd);

        if (cmd.opcode == HEALTH_NOOP)
            break;

        switch (cmd.opcode) {
            case HEALTH_SAVE_ALERT_TRANSITION: {
                ALARM_ENTRY *ae = (ALARM_ENTRY *)cmd.param[0];
                if (!config->pending_alerts)
                    config->pending_alerts = callocz(1, sizeof(*config->pending_alerts));

                Pvoid_t *Pvalue = JudyLIns(&config->pending_alerts->JudyL,
                                           ++config->pending_alerts->count, PJE0);
                if (unlikely(Pvalue == PJERR))
                    fatal("HEALTH: Failed to insert ae into pending_alerts Judy array");
                *Pvalue = (void *)ae;
                saved++;
                break;
            }

            case HEALTH_DELETE_ALERT_ENTRY: {
                ALARM_ENTRY *ae = (ALARM_ENTRY *)cmd.param[0];
                health_track_pending_deletion(config, ae);
                deleted++;
                break;
            }

            default:
                discarded++;
                break;
        }
    }

    if (saved || deleted || discarded)
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "HEALTH: drained remaining commands at shutdown: %zu saves, %zu deletes, %zu discarded",
               saved, deleted, discarded);
}

// ---------------------------------------------------------------------------------------------------------------------
// Main event loop

static void health_event_loop(void *arg) {
    struct health_event_loop_config *config = arg;
    uv_thread_set_name_np(HEALTH_EVENT_LOOP_NAME);
    worker_register(HEALTH_EVENT_LOOP_NAME);

    init_cmd_pool(&config->cmd_pool, HEALTH_CMD_POOL_SIZE);

    // Register worker job names for the main event loop thread.
    // Per-host processing jobs on libuv pool threads are registered
    // via register_libuv_worker_jobs() in the work callbacks.
    worker_register_job_name(WORKER_HEALTH_JOB_SAVE_ALERT_TRANSITION, "alert save");
    worker_register_job_name(WORKER_HEALTH_JOB_DELETE_ALERT, "alert delete");
    worker_register_job_name(WORKER_HEALTH_JOB_WAIT_EXEC, "alert wait exec");

    // Initialize the persistent worker pool. Each worker owns a uv_work_t
    // and a dedicated stmt set; workers are allocated once and reused on
    // every batch dispatch.
    config->max_concurrent_workers = health_globals.config.max_concurrent_workers;
    if (config->max_concurrent_workers == 0)
        config->max_concurrent_workers = HEALTH_DEFAULT_CONCURRENT_WORKERS;
    config->num_workers = config->max_concurrent_workers;

    config->workers = callocz(config->num_workers, sizeof(struct health_batch_worker));
    for (size_t i = 0; i < config->num_workers; i++) {
        config->workers[i].config = config;
        config->workers[i].worker_id = i;
        config->workers[i].request.data = &config->workers[i];
        config->workers[i].owa = onewayalloc_create(0);
    }

    nd_log(NDLS_DAEMON, NDLP_INFO, "HEALTH: initialized with %zu batch workers",
           config->num_workers);

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
    __atomic_store_n(&config->started, true, __ATOMIC_RELEASE);
    completion_mark_complete(&config->start_stop_complete);

    // Bootstrap: any host that was created before the loop started posted
    // HEALTH_ENABLE_HOST_PROCESSING before `initialized` was set, so its opcode
    // was dropped. Walk the dict once here to register those hosts. Future
    // hosts register via opcodes and we never scan the dict again.
    {
        RRDHOST *host;
        size_t registered = 0;
        dfe_start_read(rrdhost_root_index, host) {
            if (host->health.evloop_registered)
                continue;
            __atomic_store_n(&host->health.evloop_registered, true, __ATOMIC_RELEASE);
            registered++;
        }
        dfe_done(host);

        if (registered)
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "HEALTH: bootstrap registered %zu existing hosts", registered);
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: event loop started");

    // Main event loop
    while (true) {
        enum health_opcode opcode;
        bool shutdown_requested = __atomic_load_n(&config->shutdown_requested, __ATOMIC_RELAXED);

        worker_is_idle();
        uv_run(loop, shutdown_requested ? UV_RUN_NOWAIT : UV_RUN_ONCE);

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

                case HEALTH_SAVE_ALERT_TRANSITION: {
                    // Collect alert transitions for batch processing
                    ALARM_ENTRY *ae = (ALARM_ENTRY *)cmd.param[0];

                    if (!config->pending_alerts)
                        config->pending_alerts = callocz(1, sizeof(*config->pending_alerts));

                    Pvoid_t *Pvalue = JudyLIns(&config->pending_alerts->JudyL,
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
                    health_track_pending_deletion(config, ae);
                    break;
                }

                case HEALTH_ENABLE_HOST_PROCESSING: {
                    RRDHOST *host = (RRDHOST *)cmd.param[0];
                    __atomic_store_n(&host->health.evloop_registered, true, __ATOMIC_RELEASE);
                    break;
                }

                case HEALTH_DISABLE_HOST_PROCESSING: {
                    RRDHOST *host = (RRDHOST *)cmd.param[0];
                    __atomic_store_n(&host->health.evloop_registered, false, __ATOMIC_RELEASE);
                    // Subsequent batches will skip this host. If a worker is
                    // currently processing it, the caller of DISABLE (teardown)
                    // waits for host->health.processing to go false before
                    // freeing the host.
                    break;
                }

                case HEALTH_SYNC_SHUTDOWN:
                    __atomic_store_n(&config->shutdown_requested, true, __ATOMIC_RELAXED);
                    if (!uv_is_closing((uv_handle_t *)&config->timer_req) &&
                        !uv_timer_stop(&config->timer_req))
                        uv_close((uv_handle_t *)&config->timer_req, NULL);
                    break;

                default:
                    nd_log(NDLS_DAEMON, NDLP_ERR, "HEALTH: Unknown opcode %u", opcode);
                    break;
            }

            if (likely(opcode != HEALTH_NOOP))
                uv_run(loop, UV_RUN_NOWAIT);

        } while (opcode != HEALTH_NOOP);

        if (__atomic_load_n(&config->shutdown_requested, __ATOMIC_RELAXED)) {
            health_process_pending_alerts(config);
            health_process_pending_deletions(config);

            if (!health_has_pending_work(config)) {
                // Stop accepting new commands before the final drained check so no
                // producer can enqueue after we decide to leave the loop.
                __atomic_store_n(&config->initialized, false, __ATOMIC_RELEASE);
                uv_run(loop, UV_RUN_NOWAIT);
                health_process_pending_alerts(config);
                health_process_pending_deletions(config);

                if (!health_has_pending_work(config))
                    break;

                if (++config->drain_iterations >= HEALTH_MAX_DRAIN_ITERATIONS) {
                    nd_log(NDLS_DAEMON, NDLP_WARNING,
                           "HEALTH: shutdown drain loop reached %zu iterations, forcing exit",
                           config->drain_iterations);
                    break;
                }

                // Work appeared after we sealed the queue (e.g. a worker completion
                // callback produced new alerts).  Re-arm so producers can enqueue
                // during the next drain iteration.
                __atomic_store_n(&config->initialized, true, __ATOMIC_RELEASE);
            }
        }
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: event loop shutting down");

    // Ensure initialized is sealed so no new commands can be enqueued.
    // (The drain loop sets it to false before breaking, but if we exited via
    // the outer while(true) break path without entering the drain, seal it now.)
    __atomic_store_n(&config->initialized, false, __ATOMIC_RELEASE);

    // Stop timer so no new HEALTH_TIMER_TICK commands are enqueued
    if (!uv_is_closing((uv_handle_t *)&config->timer_req) &&
        !uv_timer_stop(&config->timer_req))
        uv_close((uv_handle_t *)&config->timer_req, NULL);

    // Close async handle
    uv_close((uv_handle_t *)&config->async, NULL);

    // Wait for any in-flight batch to complete, pumping the event loop so
    // after_work callbacks fire and any resulting commands land in the queue.
    size_t loop_count = (HEALTH_MAX_SHUTDOWN_TIMEOUT_SECONDS * 1000) / HEALTH_SHUTDOWN_SLEEP_INTERVAL_MS;
    while (__atomic_load_n(&config->batch_workers_active, __ATOMIC_RELAXED) > 0 && loop_count > 0) {
        uv_run(loop, UV_RUN_NOWAIT);
        sleep_usec(HEALTH_SHUTDOWN_SLEEP_INTERVAL_MS * USEC_PER_MS);
        loop_count--;
    }

    size_t active_workers = __atomic_load_n(&config->batch_workers_active, __ATOMIC_RELAXED);
    if (active_workers > 0) {
        nd_log(NDLS_DAEMON, NDLP_WARNING, "HEALTH: %zu workers still active at shutdown",
               active_workers);
    }

    // Drain any remaining commands from the queue.  This picks up
    // HEALTH_SAVE_ALERT_TRANSITION / HEALTH_DELETE_ALERT_ENTRY commands that
    // were enqueued before we sealed, or produced by after_work callbacks above.
    health_drain_remaining_commands(config);

    // Process all collected alert saves and deletions
    health_process_pending_alerts(config);
    health_process_pending_deletions(config);

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

    // Walk and close all remaining handles (e.g. timer/async close callbacks)
    uv_walk(loop, libuv_close_callback, NULL);

    // Run the loop until all close callbacks have fired
    loop_count = (HEALTH_MAX_SHUTDOWN_TIMEOUT_SECONDS * 1000) / HEALTH_SHUTDOWN_SLEEP_INTERVAL_MS;
    while (uv_run(loop, UV_RUN_NOWAIT) && loop_count > 0) {
        sleep_usec(HEALTH_SHUTDOWN_SLEEP_INTERVAL_MS * USEC_PER_MS);
        loop_count--;
    }

    // Close the loop
    int rc = uv_loop_close(loop);
    if (rc != 0)
        nd_log(NDLS_DAEMON, NDLP_WARNING, "HEALTH: uv_loop_close returned %d", rc);

    // If a batch is still active, leaking the per-worker statements is safer
    // than finalising them while in use during process exit.
    health_finalize_all_statements(config, active_workers == 0);
    freez(config->workers);
    config->workers = NULL;
    config->num_workers = 0;

    freez(config->batch.hosts);
    config->batch.hosts = NULL;
    config->batch.count = 0;
    config->batch.capacity = 0;

    // Release command pool
    release_cmd_pool(&config->cmd_pool);

    worker_unregister();
    service_exits();
    completion_mark_complete(&config->start_stop_complete);

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: event loop shutdown complete");
}

// ---------------------------------------------------------------------------------------------------------------------
// Public API

void health_event_loop_init(void)
{
    FUNCTION_RUN_ONCE();

    memset(&health_config, 0, sizeof(health_config));
    health_notifications_init();
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
    // Use atomic exchange: 'started' is set once during init and cleared here on
    // first shutdown call.  This avoids the race where the drain loop temporarily
    // sets initialized=false (which the old check used), and also prevents a second
    // shutdown call from driving teardown against already-destroyed state.
    if (!__atomic_exchange_n(&health_config.started, false, __ATOMIC_ACQ_REL)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "HEALTH: event loop not initialized or already shut down, skipping");
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
