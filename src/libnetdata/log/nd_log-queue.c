// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-queue.h"
#include "nd_log-internals.h"

// ----------------------------------------------------------------------------
// Queue structure

struct nd_log_queue {
    // Queue storage - array of entries for simplicity and cache efficiency
    struct nd_log_queue_entry *entries;
    size_t capacity;
    size_t head;            // next position to read from
    size_t tail;            // next position to write to
    size_t count;           // current number of entries in queue

    // Synchronization
    SPINLOCK enqueue_spinlock;      // protects queue modifications (very short hold time)
    netdata_mutex_t mutex;          // for condition variable wait
    netdata_cond_t cond;            // signals logger thread
    netdata_cond_t shutdown_ack_cond;   // signals shutdown acknowledgment
    netdata_mutex_t shutdown_ack_mutex; // protects shutdown_acknowledged

    // State
    bool initialized;
    bool shutdown_requested;
    bool flush_requested;
    bool shutdown_acknowledged;  // set by logger thread before exiting

    // Logger thread
    ND_THREAD *logger_thread;

    // Statistics (atomic)
    size_t entries_queued;
    size_t entries_processed;
    size_t entries_dropped;
    size_t entries_allocated;   // entries that needed malloc
    size_t bytes_queued;
    size_t bytes_written;
    size_t queue_high_water;
};

static struct nd_log_queue log_queue = {
    .entries = NULL,
    .capacity = 0,
    .head = 0,
    .tail = 0,
    .count = 0,
    .enqueue_spinlock = SPINLOCK_INITIALIZER,
    .initialized = false,
    .shutdown_requested = false,
    .flush_requested = false,
    .shutdown_acknowledged = false,
    .logger_thread = NULL,
    .entries_queued = 0,
    .entries_processed = 0,
    .entries_dropped = 0,
    .entries_allocated = 0,
    .bytes_queued = 0,
    .bytes_written = 0,
    .queue_high_water = 0,
};

// ----------------------------------------------------------------------------
// Internal: Get pointer to message content (inline or allocated)

static inline const char *nd_log_queue_entry_message(struct nd_log_queue_entry *entry) {
    return entry->message_allocated ? entry->message_allocated : entry->message_inline;
}

// Free any dynamically allocated message buffer
static inline void nd_log_queue_entry_free_allocated(struct nd_log_queue_entry *entry) {
    if (entry->message_allocated) {
        freez(entry->message_allocated);
        entry->message_allocated = NULL;
    }
}

// ----------------------------------------------------------------------------
// Internal: Write a single entry to its destination

static void nd_log_queue_write_entry(struct nd_log_queue_entry *entry) {
    if (!entry || entry->message_len == 0)
        return;

    const char *message = nd_log_queue_entry_message(entry);

    switch (entry->method) {
        case NDLM_FILE:
        case NDLM_STDOUT:
        case NDLM_STDERR:
            if (entry->fp) {
                fprintf(entry->fp, "%s\n", message);
                fflush(entry->fp);
            }
            break;

        case NDLM_JOURNAL:
            // For journal, the message should already be formatted appropriately
            // Use captured state from enqueue time to avoid data races
            if (entry->journal_direct_initialized && entry->fd >= 0) {
                // Journal direct write - message already formatted as journal fields
                struct iovec iov[1];
                iov[0].iov_base = (void *)message;
                iov[0].iov_len = entry->message_len;

                struct msghdr mh = {
                    .msg_iov = iov,
                    .msg_iovlen = 1,
                };
                ssize_t sent = sendmsg(entry->fd, &mh, MSG_NOSIGNAL);
                if (sent < 0) {
                    // Failed to write to journal - write to stderr as fallback
                    fprintf(stderr, "async-logger: sendmsg() to journal failed (fd=%d): %s\n",
                            entry->fd, strerror(errno));
                    fprintf(stderr, "%s\n", message);
                    fflush(stderr);
                }
            }
#ifdef HAVE_SYSTEMD
            else if (entry->journal_libsystemd_initialized) {
                // Fallback to libsystemd - for this we'd need structured fields
                // For now, just write to stderr as fallback
                fprintf(stderr, "%s\n", message);
                fflush(stderr);
            }
#endif
            else {
                // No journal available, fallback to stderr
                fprintf(stderr, "%s\n", message);
                fflush(stderr);
            }
            break;

        case NDLM_SYSLOG:
            // Use captured state from enqueue time to avoid data races
            if (entry->syslog_initialized) {
                // Convert priority to syslog priority
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
                // Syslog not initialized, fallback to stderr
                fprintf(stderr, "%s\n", message);
                fflush(stderr);
            }
            break;

        case NDLM_DISABLED:
        case NDLM_DEVNULL:
        case NDLM_DEFAULT:
            // Do nothing
            break;

        default:
            // Unknown method, try stderr as fallback
            fprintf(stderr, "%s\n", message);
            fflush(stderr);
            break;
    }

    __atomic_add_fetch(&log_queue.bytes_written, entry->message_len, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// Logger thread

static void nd_log_queue_thread_cleanup(void *ptr __maybe_unused) {
    // Drain any remaining entries on shutdown
    spinlock_lock(&log_queue.enqueue_spinlock);
    while (log_queue.count > 0) {
        // Copy entry out of queue (including the allocated pointer)
        struct nd_log_queue_entry entry_copy = log_queue.entries[log_queue.head];

        // Clear the allocated pointer in the queue entry so it won't be double-freed
        log_queue.entries[log_queue.head].message_allocated = NULL;

        log_queue.head = (log_queue.head + 1) % log_queue.capacity;
        log_queue.count--;
        spinlock_unlock(&log_queue.enqueue_spinlock);

        nd_log_queue_write_entry(&entry_copy);
        nd_log_queue_entry_free_allocated(&entry_copy);
        __atomic_add_fetch(&log_queue.entries_processed, 1, __ATOMIC_RELAXED);

        spinlock_lock(&log_queue.enqueue_spinlock);
    }
    spinlock_unlock(&log_queue.enqueue_spinlock);
}

static void nd_log_queue_thread(void *arg __maybe_unused) {
    netdata_thread_cleanup_push(nd_log_queue_thread_cleanup, NULL);

    while (true) {
        netdata_mutex_lock(&log_queue.mutex);

        // Wait for entries or shutdown signal
        while (log_queue.count == 0 && !log_queue.shutdown_requested) {
            netdata_cond_wait(&log_queue.cond, &log_queue.mutex);
        }

        // Check if we should exit
        if (log_queue.shutdown_requested && log_queue.count == 0) {
            // Signal acknowledgment before exiting
            netdata_mutex_lock(&log_queue.shutdown_ack_mutex);
            log_queue.shutdown_acknowledged = true;
            netdata_cond_signal(&log_queue.shutdown_ack_cond);
            netdata_mutex_unlock(&log_queue.shutdown_ack_mutex);

            netdata_mutex_unlock(&log_queue.mutex);
            break;
        }

        bool was_flush = log_queue.flush_requested;
        netdata_mutex_unlock(&log_queue.mutex);

        // Process entries - copy out with spinlock, write without any lock
        while (true) {
            struct nd_log_queue_entry entry_copy;
            bool has_entry = false;

            spinlock_lock(&log_queue.enqueue_spinlock);
            if (log_queue.count > 0) {
                // Copy entry out of queue (including the allocated pointer)
                entry_copy = log_queue.entries[log_queue.head];

                // Clear the allocated pointer in the queue entry so it won't be double-freed
                log_queue.entries[log_queue.head].message_allocated = NULL;

                log_queue.head = (log_queue.head + 1) % log_queue.capacity;
                log_queue.count--;
                has_entry = true;
            }
            spinlock_unlock(&log_queue.enqueue_spinlock);

            if (!has_entry)
                break;

            // Write entry without holding any lock
            nd_log_queue_write_entry(&entry_copy);

            // Free any allocated message buffer
            nd_log_queue_entry_free_allocated(&entry_copy);

            __atomic_add_fetch(&log_queue.entries_processed, 1, __ATOMIC_RELAXED);
        }

        // If flush was requested and queue is now empty, signal completion
        if (was_flush) {
            netdata_mutex_lock(&log_queue.mutex);
            log_queue.flush_requested = false;
            netdata_cond_broadcast(&log_queue.cond);
            netdata_mutex_unlock(&log_queue.mutex);
        }
    }

    netdata_thread_cleanup_pop(1);
}

// ----------------------------------------------------------------------------
// Public API

bool nd_log_queue_init(void) {
    if (log_queue.initialized)
        return true;

    // Allocate queue entries
    log_queue.capacity = ND_LOG_QUEUE_DEFAULT_SIZE;
    log_queue.entries = callocz(log_queue.capacity, sizeof(struct nd_log_queue_entry));

    log_queue.head = 0;
    log_queue.tail = 0;
    log_queue.count = 0;
    log_queue.shutdown_requested = false;
    log_queue.flush_requested = false;
    log_queue.shutdown_acknowledged = false;

    // Initialize synchronization primitives
    spinlock_init(&log_queue.enqueue_spinlock);
    if (netdata_mutex_init(&log_queue.mutex) != 0) {
        freez(log_queue.entries);
        log_queue.entries = NULL;
        return false;
    }

    if (netdata_cond_init(&log_queue.cond) != 0) {
        netdata_mutex_destroy(&log_queue.mutex);
        freez(log_queue.entries);
        log_queue.entries = NULL;
        return false;
    }

    if (netdata_mutex_init(&log_queue.shutdown_ack_mutex) != 0) {
        netdata_cond_destroy(&log_queue.cond);
        netdata_mutex_destroy(&log_queue.mutex);
        freez(log_queue.entries);
        log_queue.entries = NULL;
        return false;
    }

    if (netdata_cond_init(&log_queue.shutdown_ack_cond) != 0) {
        netdata_mutex_destroy(&log_queue.shutdown_ack_mutex);
        netdata_cond_destroy(&log_queue.cond);
        netdata_mutex_destroy(&log_queue.mutex);
        freez(log_queue.entries);
        log_queue.entries = NULL;
        return false;
    }

    // Start logger thread
    log_queue.logger_thread = nd_thread_create(
        "LOGGER",
        NETDATA_THREAD_OPTION_DONT_LOG,
        nd_log_queue_thread,
        NULL
    );

    if (!log_queue.logger_thread) {
        netdata_cond_destroy(&log_queue.shutdown_ack_cond);
        netdata_mutex_destroy(&log_queue.shutdown_ack_mutex);
        netdata_cond_destroy(&log_queue.cond);
        netdata_mutex_destroy(&log_queue.mutex);
        freez(log_queue.entries);
        log_queue.entries = NULL;
        return false;
    }

    log_queue.initialized = true;
    return true;
}

void nd_log_queue_shutdown(bool flush) {
    if (!log_queue.initialized)
        return;

    if (flush)
        nd_log_queue_flush();

    // Signal shutdown
    netdata_mutex_lock(&log_queue.mutex);
    log_queue.shutdown_requested = true;
    netdata_cond_signal(&log_queue.cond);
    netdata_mutex_unlock(&log_queue.mutex);

    // Wait for logger thread to acknowledge shutdown
    netdata_mutex_lock(&log_queue.shutdown_ack_mutex);
    while (!log_queue.shutdown_acknowledged) {
        netdata_cond_wait(&log_queue.shutdown_ack_cond, &log_queue.shutdown_ack_mutex);
    }
    netdata_mutex_unlock(&log_queue.shutdown_ack_mutex);

    // Now safe to join - thread has exited or is about to
    if (log_queue.logger_thread) {
        nd_thread_join(log_queue.logger_thread);
        log_queue.logger_thread = NULL;
    }

    // Cleanup
    netdata_cond_destroy(&log_queue.shutdown_ack_cond);
    netdata_mutex_destroy(&log_queue.shutdown_ack_mutex);
    netdata_cond_destroy(&log_queue.cond);
    netdata_mutex_destroy(&log_queue.mutex);
    freez(log_queue.entries);
    log_queue.entries = NULL;
    log_queue.initialized = false;
}

bool nd_log_queue_enabled(void) {
    return log_queue.initialized && !log_queue.shutdown_requested;
}

bool nd_log_queue_enqueue(struct nd_log_queue_entry *entry) {
    if (!log_queue.initialized || log_queue.shutdown_requested)
        return false;

    bool success = false;

    spinlock_lock(&log_queue.enqueue_spinlock);

    if (log_queue.count < log_queue.capacity) {
        struct nd_log_queue_entry *dest = &log_queue.entries[log_queue.tail];

        // Copy fixed fields
        dest->source = entry->source;
        dest->priority = entry->priority;
        dest->method = entry->method;
        dest->format = entry->format;
        dest->fp = entry->fp;
        dest->fd = entry->fd;
        dest->journal_direct_initialized = entry->journal_direct_initialized;
        dest->journal_libsystemd_initialized = entry->journal_libsystemd_initialized;
        dest->syslog_initialized = entry->syslog_initialized;
        dest->message_len = entry->message_len;

        // Handle message content - transfer ownership of allocated pointer
        if (entry->message_allocated) {
            // Large message - transfer the allocated pointer
            dest->message_allocated = entry->message_allocated;
            entry->message_allocated = NULL;  // caller no longer owns it
            __atomic_add_fetch(&log_queue.entries_allocated, 1, __ATOMIC_RELAXED);
        } else {
            // Small message - copy inline buffer
            dest->message_allocated = NULL;
            memcpy(dest->message_inline, entry->message_inline, entry->message_len + 1);
        }

        log_queue.tail = (log_queue.tail + 1) % log_queue.capacity;
        log_queue.count++;

        // Update statistics
        __atomic_add_fetch(&log_queue.entries_queued, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&log_queue.bytes_queued, entry->message_len, __ATOMIC_RELAXED);

        // Update high water mark
        size_t current_high = __atomic_load_n(&log_queue.queue_high_water, __ATOMIC_RELAXED);
        if (log_queue.count > current_high) {
            __atomic_store_n(&log_queue.queue_high_water, log_queue.count, __ATOMIC_RELAXED);
        }

        success = true;
    } else {
        // Queue full - drop message, but need to free any allocated buffer
        if (entry->message_allocated) {
            freez(entry->message_allocated);
            entry->message_allocated = NULL;
        }
        __atomic_add_fetch(&log_queue.entries_dropped, 1, __ATOMIC_RELAXED);
    }

    spinlock_unlock(&log_queue.enqueue_spinlock);

    if (success) {
        // Signal logger thread (don't hold spinlock while signaling)
        netdata_mutex_lock(&log_queue.mutex);
        netdata_cond_signal(&log_queue.cond);
        netdata_mutex_unlock(&log_queue.mutex);
    }

    return success;
}

void nd_log_queue_get_stats(struct nd_log_queue_stats *stats) {
    if (!stats)
        return;

    stats->entries_queued = __atomic_load_n(&log_queue.entries_queued, __ATOMIC_RELAXED);
    stats->entries_processed = __atomic_load_n(&log_queue.entries_processed, __ATOMIC_RELAXED);
    stats->entries_dropped = __atomic_load_n(&log_queue.entries_dropped, __ATOMIC_RELAXED);
    stats->entries_allocated = __atomic_load_n(&log_queue.entries_allocated, __ATOMIC_RELAXED);
    stats->bytes_queued = __atomic_load_n(&log_queue.bytes_queued, __ATOMIC_RELAXED);
    stats->bytes_written = __atomic_load_n(&log_queue.bytes_written, __ATOMIC_RELAXED);
    stats->queue_high_water = __atomic_load_n(&log_queue.queue_high_water, __ATOMIC_RELAXED);
}

void nd_log_queue_flush(void) {
    if (!log_queue.initialized)
        return;

    netdata_mutex_lock(&log_queue.mutex);
    log_queue.flush_requested = true;
    netdata_cond_signal(&log_queue.cond);

    // Wait until flush completes (queue empty)
    while (log_queue.flush_requested && !log_queue.shutdown_requested) {
        netdata_cond_wait(&log_queue.cond, &log_queue.mutex);
    }
    netdata_mutex_unlock(&log_queue.mutex);
}

void nd_log_queue_write_sync(struct nd_log_queue_entry *entry) {
    // Direct synchronous write - no queue, no locks (except file-level)
    // Used for critical messages that must be written immediately
    nd_log_queue_write_entry(entry);
}
