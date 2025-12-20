// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_LOG_QUEUE_H
#define NETDATA_ND_LOG_QUEUE_H

#include "../libnetdata.h"
#include "nd_log-internals.h"

// Inline buffer size for short messages (most messages fit here)
#define ND_LOG_QUEUE_INLINE_SIZE 512

// Maximum size for dynamically allocated messages
#define ND_LOG_QUEUE_MESSAGE_MAX_SIZE 16384

// Default queue size (number of entries)
#define ND_LOG_QUEUE_DEFAULT_SIZE 1024

// Maximum queue size before dropping messages
#define ND_LOG_QUEUE_MAX_SIZE 8192

// Structure for a queued log entry (pre-formatted)
struct nd_log_queue_entry {
    ND_LOG_SOURCES source;
    ND_LOG_FIELD_PRIORITY priority;
    ND_LOG_METHOD method;
    ND_LOG_FORMAT format;
    FILE *fp;                                       // target file pointer (for NDLM_FILE)
    int fd;                                         // target fd (for journal direct)
    size_t message_len;
    char *message_allocated;                        // mallocz'd for messages > INLINE_SIZE (NULL if inline)
    char message_inline[ND_LOG_QUEUE_INLINE_SIZE];  // inline buffer for short messages
};

// Queue statistics
struct nd_log_queue_stats {
    size_t entries_queued;      // total entries added to queue
    size_t entries_processed;   // total entries written
    size_t entries_dropped;     // entries dropped due to full queue
    size_t entries_allocated;   // entries that needed malloc (> inline size)
    size_t bytes_queued;        // total bytes queued
    size_t bytes_written;       // total bytes written
    size_t queue_high_water;    // maximum queue depth seen
};

// Initialize the async logging queue and start the logger thread
// Returns true on success, false on failure
bool nd_log_queue_init(void);

// Shutdown the async logging queue
// If flush is true, waits for all pending messages to be written
void nd_log_queue_shutdown(bool flush);

// Check if async logging is enabled
bool nd_log_queue_enabled(void);

// Enqueue a pre-formatted log message
// Returns true if queued, false if dropped (queue full)
bool nd_log_queue_enqueue(struct nd_log_queue_entry *entry);

// Get queue statistics
void nd_log_queue_get_stats(struct nd_log_queue_stats *stats);

// Flush all pending log messages (blocks until queue is empty)
void nd_log_queue_flush(void);

// For critical messages (ALERT and above) - write synchronously
// This bypasses the queue for messages that must be written immediately
void nd_log_queue_write_sync(struct nd_log_queue_entry *entry);

#endif // NETDATA_ND_LOG_QUEUE_H
