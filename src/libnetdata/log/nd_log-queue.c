// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-queue.h"
#include "nd_log-internals.h"
#include "systemd-journal-helpers.h"

// ----------------------------------------------------------------------------
// Command pool (ring buffer for commands)

struct nd_log_cmd_pool {
    struct nd_log_queue_cmd *cmds;
    size_t size;
    size_t head;    // next position to write
    size_t tail;    // next position to read
    SPINLOCK spinlock;
};

// ----------------------------------------------------------------------------
// Event loop configuration

struct nd_log_event_loop {
    ND_THREAD *thread;
    uv_loop_t loop;
    uv_async_t async;

    // State (accessed atomically)
    bool initialized;
    bool shutdown_requested;

    // Command pool
    struct nd_log_cmd_pool cmd_pool;

    // Synchronization for init/shutdown
    struct completion start_stop_complete;

    // Statistics (atomic)
    size_t entries_queued;
    size_t entries_processed;
    size_t entries_dropped;
    size_t entries_allocated;
    size_t bytes_queued;
    size_t bytes_written;
    size_t queue_high_water;
    size_t current_queue_depth;
};

static struct nd_log_event_loop log_ev = {
    .thread = NULL,
    .initialized = false,
    .shutdown_requested = false,
    .entries_queued = 0,
    .entries_processed = 0,
    .entries_dropped = 0,
    .entries_allocated = 0,
    .bytes_queued = 0,
    .bytes_written = 0,
    .queue_high_water = 0,
    .current_queue_depth = 0,
};

// ----------------------------------------------------------------------------
// Command pool functions

static void cmd_pool_init(struct nd_log_cmd_pool *pool, size_t size) {
    pool->cmds = callocz(size, sizeof(struct nd_log_queue_cmd));
    pool->size = size;
    pool->head = 0;
    pool->tail = 0;
    spinlock_init(&pool->spinlock);
}

static void cmd_pool_destroy(struct nd_log_cmd_pool *pool) {
    // Free any allocated message buffers in remaining commands
    // Only iterate through valid entries (between tail and head)
    size_t idx = pool->tail;
    while (idx != pool->head) {
        if (pool->cmds[idx].opcode == ND_LOG_OP_ENTRY && pool->cmds[idx].entry.message_allocated) {
            freez(pool->cmds[idx].entry.message_allocated);
            pool->cmds[idx].entry.message_allocated = NULL;
        }
        idx = (idx + 1) % pool->size;
    }
    freez(pool->cmds);
    pool->cmds = NULL;
    pool->size = 0;
}

static bool cmd_pool_push(struct nd_log_cmd_pool *pool, struct nd_log_queue_cmd *cmd) {
    spinlock_lock(&pool->spinlock);

    size_t next_head = (pool->head + 1) % pool->size;
    if (next_head == pool->tail) {
        // Queue full
        spinlock_unlock(&pool->spinlock);
        return false;
    }

    pool->cmds[pool->head] = *cmd;
    pool->head = next_head;

    spinlock_unlock(&pool->spinlock);
    return true;
}

static struct nd_log_queue_cmd cmd_pool_pop(struct nd_log_cmd_pool *pool) {
    struct nd_log_queue_cmd cmd = { .opcode = ND_LOG_OP_NOOP };

    spinlock_lock(&pool->spinlock);

    if (pool->tail != pool->head) {
        cmd = pool->cmds[pool->tail];
        // Clear the entire slot to prevent stale data in the union
        memset(&pool->cmds[pool->tail], 0, sizeof(pool->cmds[pool->tail]));
        pool->tail = (pool->tail + 1) % pool->size;
    }

    spinlock_unlock(&pool->spinlock);
    return cmd;
}

// ----------------------------------------------------------------------------
// Internal: Get pointer to message content

static inline const char *entry_message(struct nd_log_queue_entry *entry) {
    return entry->message_allocated ? entry->message_allocated : entry->message_inline;
}

static inline void entry_free_allocated(struct nd_log_queue_entry *entry) {
    if (entry->message_allocated) {
        freez(entry->message_allocated);
        entry->message_allocated = NULL;
    }
}

// ----------------------------------------------------------------------------
// Internal: Write a single entry to its destination

static void write_entry(struct nd_log_queue_entry *entry) {
    if (!entry || entry->message_len == 0)
        return;

    // Bounds check on source index to prevent out-of-bounds access
    if (entry->source >= _NDLS_MAX) {
        fprintf(stderr, "async-logger: invalid source index %d, dropping message\n", entry->source);
        return;
    }

    const char *message = entry_message(entry);

    switch (entry->method) {
        case NDLM_FILE:
        case NDLM_STDOUT:
        case NDLM_STDERR: {
            // Lookup FILE* at write time to handle log rotation correctly
            FILE *fp = nd_log.sources[entry->source].fp;
            if (fp) {
                fprintf(fp, "%s\n", message);
                fflush(fp);
            }
            break;
        }

        case NDLM_SYSLOG:
            if (entry->syslog_initialized) {
                int syslog_priority = LOG_INFO;
                switch (entry->priority) {
                    case NDLP_EMERG:   syslog_priority = LOG_EMERG; break;
                    case NDLP_ALERT:   syslog_priority = LOG_ALERT; break;
                    case NDLP_CRIT:    syslog_priority = LOG_CRIT; break;
                    case NDLP_ERR:     syslog_priority = LOG_ERR; break;
                    case NDLP_WARNING: syslog_priority = LOG_WARNING; break;
                    case NDLP_NOTICE:  syslog_priority = LOG_NOTICE; break;
                    case NDLP_INFO:    syslog_priority = LOG_INFO; break;
                    case NDLP_DEBUG:   syslog_priority = LOG_DEBUG; break;
                    default:           syslog_priority = LOG_INFO; break;
                }
                syslog(syslog_priority, "%s", message);
            } else {
                fprintf(stderr, "%s\n", message);
                fflush(stderr);
            }
            break;

        case NDLM_DISABLED:
        case NDLM_DEVNULL:
        case NDLM_DEFAULT:
            break;

        default:
            fprintf(stderr, "%s\n", message);
            fflush(stderr);
            break;
    }

    __atomic_add_fetch(&log_ev.bytes_written, entry->message_len, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// Internal: Reopen log files (called from logger thread)

static void do_reopen_log_files(void) {
    // This runs in the logger thread, so no race with FILE* usage
    for (size_t i = 0; i < _NDLS_MAX; i++) {
        nd_log_open(&nd_log.sources[i], i);
    }
}

// ----------------------------------------------------------------------------
// Event loop callbacks

static void async_cb(uv_async_t *handle __maybe_unused) {
    // Just wake up the loop - actual processing happens in the main loop
}

// ----------------------------------------------------------------------------
// Logger thread main function

static void logger_event_loop(void *arg) {
    struct nd_log_event_loop *ev = arg;
    uv_loop_t *loop = &ev->loop;

    // Initialize libuv loop
    int rc = uv_loop_init(loop);
    if (rc) {
        netdata_log_error("LOGGER: Failed to initialize event loop: %s", uv_strerror(rc));
        completion_mark_complete(&ev->start_stop_complete);
        return;
    }

    // Initialize async handle for wakeups
    rc = uv_async_init(loop, &ev->async, async_cb);
    if (rc) {
        netdata_log_error("LOGGER: Failed to initialize async handle: %s", uv_strerror(rc));
        uv_loop_close(loop);
        completion_mark_complete(&ev->start_stop_complete);
        return;
    }

    // Mark as initialized and signal completion
    __atomic_store_n(&ev->initialized, true, __ATOMIC_RELEASE);
    completion_mark_complete(&ev->start_stop_complete);

    // Main event loop
    while (likely(!__atomic_load_n(&ev->shutdown_requested, __ATOMIC_ACQUIRE))) {
        // Run event loop (will block until async signal)
        uv_run(loop, UV_RUN_ONCE);

        // Process all pending commands
        struct nd_log_queue_cmd cmd;
        while ((cmd = cmd_pool_pop(&ev->cmd_pool)).opcode != ND_LOG_OP_NOOP) {

            switch (cmd.opcode) {
                case ND_LOG_OP_ENTRY:
                    write_entry(&cmd.entry);
                    entry_free_allocated(&cmd.entry);
                    __atomic_add_fetch(&ev->entries_processed, 1, __ATOMIC_RELAXED);
                    __atomic_sub_fetch(&ev->current_queue_depth, 1, __ATOMIC_RELAXED);
                    break;

                case ND_LOG_OP_FLUSH:
                    // All entries before this have been processed
                    if (cmd.sync.completion)
                        completion_mark_complete(cmd.sync.completion);
                    break;

                case ND_LOG_OP_REOPEN:
                    // Reopen all log files - safe because we're in the logger thread
                    do_reopen_log_files();
                    if (cmd.sync.completion)
                        completion_mark_complete(cmd.sync.completion);
                    break;

                case ND_LOG_OP_SHUTDOWN:
                    __atomic_store_n(&ev->shutdown_requested, true, __ATOMIC_RELEASE);
                    if (cmd.sync.completion)
                        completion_mark_complete(cmd.sync.completion);
                    break;

                case ND_LOG_OP_NOOP:
                default:
                    break;
            }
        }
    }

    // Drain any remaining entries
    struct nd_log_queue_cmd cmd;
    while ((cmd = cmd_pool_pop(&ev->cmd_pool)).opcode != ND_LOG_OP_NOOP) {
        if (cmd.opcode == ND_LOG_OP_ENTRY) {
            write_entry(&cmd.entry);
            entry_free_allocated(&cmd.entry);
            __atomic_add_fetch(&ev->entries_processed, 1, __ATOMIC_RELAXED);
            __atomic_sub_fetch(&ev->current_queue_depth, 1, __ATOMIC_RELAXED);
        } else if (cmd.sync.completion) {
            completion_mark_complete(cmd.sync.completion);
        }
    }

    // Close handles
    uv_close((uv_handle_t *)&ev->async, NULL);

    // Run loop until all handles are closed
    while (uv_loop_close(loop) == UV_EBUSY) {
        uv_run(loop, UV_RUN_NOWAIT);
    }

    __atomic_store_n(&ev->initialized, false, __ATOMIC_RELEASE);
}

// ----------------------------------------------------------------------------
// Public API

void nd_log_queue_init(void) {
    FUNCTION_RUN_ONCE();

    if (__atomic_load_n(&log_ev.initialized, __ATOMIC_ACQUIRE))
        return;

    // Initialize command pool
    cmd_pool_init(&log_ev.cmd_pool, ND_LOG_QUEUE_CMD_POOL_SIZE);

    // Initialize synchronization
    completion_init(&log_ev.start_stop_complete);

    // Reset state
    __atomic_store_n(&log_ev.shutdown_requested, false, __ATOMIC_RELEASE);
    __atomic_store_n(&log_ev.entries_queued, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&log_ev.entries_processed, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&log_ev.entries_dropped, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&log_ev.entries_allocated, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&log_ev.bytes_queued, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&log_ev.bytes_written, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&log_ev.queue_high_water, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&log_ev.current_queue_depth, 0, __ATOMIC_RELEASE);

    // Create logger thread
    log_ev.thread = nd_thread_create(
        "LOGGER",
        NETDATA_THREAD_OPTION_DONT_LOG,
        logger_event_loop,
        &log_ev
    );

    if (!log_ev.thread) {
        cmd_pool_destroy(&log_ev.cmd_pool);
        completion_destroy(&log_ev.start_stop_complete);
        return;
    }

    // Wait for initialization to complete
    completion_wait_for(&log_ev.start_stop_complete);
    completion_reset(&log_ev.start_stop_complete);
}

// Timeout in seconds for shutdown wait - prevents indefinite hang if logger thread is dead
#define ND_LOG_SHUTDOWN_TIMEOUT_S 5

void nd_log_queue_shutdown(void) {
    if (!__atomic_load_n(&log_ev.initialized, __ATOMIC_ACQUIRE))
        return;

    // Send shutdown command and wait
    struct completion shutdown_complete;
    completion_init(&shutdown_complete);

    struct nd_log_queue_cmd cmd = {
        .opcode = ND_LOG_OP_SHUTDOWN,
        .sync.completion = &shutdown_complete
    };

    if (cmd_pool_push(&log_ev.cmd_pool, &cmd)) {
        uv_async_send(&log_ev.async);
        if (!completion_timedwait_for(&shutdown_complete, ND_LOG_SHUTDOWN_TIMEOUT_S)) {
            // Timeout - logger thread may be dead, proceed with cleanup anyway
            fprintf(stderr, "LOGGER: shutdown wait timed out after %d seconds\n", ND_LOG_SHUTDOWN_TIMEOUT_S);
        }
    }
    else {
        // Queue is full - signal shutdown directly via atomic flag.
        // The logger thread checks this flag in its main loop and will exit.
        __atomic_store_n(&log_ev.shutdown_requested, true, __ATOMIC_RELEASE);
        uv_async_send(&log_ev.async);
    }

    // Join thread BEFORE destroying completion - a slow thread might still
    // call completion_mark_complete() after timeout but before exiting
    if (log_ev.thread) {
        nd_thread_join(log_ev.thread);
        log_ev.thread = NULL;
    }

    // Now safe to destroy - thread has exited and won't access completion
    completion_destroy(&shutdown_complete);

    // Cleanup
    cmd_pool_destroy(&log_ev.cmd_pool);
    completion_destroy(&log_ev.start_stop_complete);
}

bool nd_log_queue_enabled(void) {
    return __atomic_load_n(&log_ev.initialized, __ATOMIC_ACQUIRE) &&
           !__atomic_load_n(&log_ev.shutdown_requested, __ATOMIC_ACQUIRE);
}

bool nd_log_queue_enqueue(struct nd_log_queue_entry *entry) {
    if (!nd_log_queue_enabled()) {
        // Not accepting entries - free any allocated buffer to prevent leak
        if (entry->message_allocated) {
            freez(entry->message_allocated);
            entry->message_allocated = NULL;
        }
        return false;
    }

    struct nd_log_queue_cmd cmd = {
        .opcode = ND_LOG_OP_ENTRY,
        .entry = *entry  // Copy the entry
    };

    // Transfer ownership of allocated pointer
    if (entry->message_allocated) {
        entry->message_allocated = NULL;
        __atomic_add_fetch(&log_ev.entries_allocated, 1, __ATOMIC_RELAXED);
    }

    if (!cmd_pool_push(&log_ev.cmd_pool, &cmd)) {
        // Queue full - free allocated buffer if any
        entry_free_allocated(&cmd.entry);
        __atomic_add_fetch(&log_ev.entries_dropped, 1, __ATOMIC_RELAXED);
        return false;
    }

    // Update statistics
    __atomic_add_fetch(&log_ev.entries_queued, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&log_ev.bytes_queued, entry->message_len, __ATOMIC_RELAXED);

    size_t depth = __atomic_add_fetch(&log_ev.current_queue_depth, 1, __ATOMIC_RELAXED);

    // Update high water mark using CAS loop to handle concurrent updates correctly
    size_t high = __atomic_load_n(&log_ev.queue_high_water, __ATOMIC_RELAXED);
    while (depth > high) {
        if (__atomic_compare_exchange_n(&log_ev.queue_high_water, &high, depth,
                                        false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
            break;
        // CAS failed - 'high' now contains the current value, loop will re-check
    }

    // Wake up logger thread, but only if not shutting down.
    // If shutdown is in progress, the async handle may be closed/invalid,
    // and the drain loop will process this entry anyway.
    if (!__atomic_load_n(&log_ev.shutdown_requested, __ATOMIC_ACQUIRE))
        uv_async_send(&log_ev.async);

    return true;
}

void nd_log_queue_get_stats(struct nd_log_queue_stats *stats) {
    if (!stats)
        return;

    stats->entries_queued = __atomic_load_n(&log_ev.entries_queued, __ATOMIC_RELAXED);
    stats->entries_processed = __atomic_load_n(&log_ev.entries_processed, __ATOMIC_RELAXED);
    stats->entries_dropped = __atomic_load_n(&log_ev.entries_dropped, __ATOMIC_RELAXED);
    stats->entries_allocated = __atomic_load_n(&log_ev.entries_allocated, __ATOMIC_RELAXED);
    stats->bytes_queued = __atomic_load_n(&log_ev.bytes_queued, __ATOMIC_RELAXED);
    stats->bytes_written = __atomic_load_n(&log_ev.bytes_written, __ATOMIC_RELAXED);
    stats->queue_high_water = __atomic_load_n(&log_ev.queue_high_water, __ATOMIC_RELAXED);
}

void nd_log_queue_flush(void) {
    if (!nd_log_queue_enabled())
        return;

    struct completion flush_complete;
    completion_init(&flush_complete);

    struct nd_log_queue_cmd cmd = {
        .opcode = ND_LOG_OP_FLUSH,
        .sync.completion = &flush_complete
    };

    if (cmd_pool_push(&log_ev.cmd_pool, &cmd)) {
        uv_async_send(&log_ev.async);
        completion_wait_for(&flush_complete);
    }

    completion_destroy(&flush_complete);
}

void nd_log_queue_reopen(void) {
    if (!nd_log_queue_enabled())
        return;

    struct completion reopen_complete;
    completion_init(&reopen_complete);

    struct nd_log_queue_cmd cmd = {
        .opcode = ND_LOG_OP_REOPEN,
        .sync.completion = &reopen_complete
    };

    if (cmd_pool_push(&log_ev.cmd_pool, &cmd)) {
        uv_async_send(&log_ev.async);
        completion_wait_for(&reopen_complete);
    }

    completion_destroy(&reopen_complete);
}
