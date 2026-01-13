// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_LOG_QUEUE_H
#define NETDATA_ND_LOG_QUEUE_H

#include "../libnetdata.h"
#include "nd_log-internals.h"

// ----------------------------------------------------------------------------
// Configuration

// Inline buffer size for short messages (most messages fit here)
#define ND_LOG_QUEUE_INLINE_SIZE 512

// Maximum size for dynamically allocated messages
#define ND_LOG_QUEUE_MESSAGE_MAX_SIZE 16384

// Command pool size (number of command slots)
#define ND_LOG_QUEUE_CMD_POOL_SIZE 4096

// ----------------------------------------------------------------------------
// Opcodes for the logger event loop

typedef enum __attribute__((packed)) {
    ND_LOG_OP_NOOP = 0,         // No operation (empty slot)
    ND_LOG_OP_ENTRY,            // Log a message
    ND_LOG_OP_FLUSH,            // Flush queue and signal completion
    ND_LOG_OP_REOPEN,           // Reopen all log files (handled in logger thread)
    ND_LOG_OP_SHUTDOWN,         // Drain queue and exit
} ND_LOG_OPCODE;

// ----------------------------------------------------------------------------
// Log entry structure (pre-formatted message)

struct nd_log_queue_entry {
    ND_LOG_SOURCES source;
    ND_LOG_FIELD_PRIORITY priority;
    ND_LOG_METHOD method;
    ND_LOG_FORMAT format;
    FILE *fp;                                       // target file pointer (for NDLM_FILE)
    int fd;                                         // target fd (for journal direct)
    size_t message_len;
    bool journal_direct_initialized;                // captured at enqueue time
    bool journal_libsystemd_initialized;            // captured at enqueue time
    bool syslog_initialized;                        // captured at enqueue time
    char *message_allocated;                        // mallocz'd for messages > INLINE_SIZE (NULL if inline)
    char message_inline[ND_LOG_QUEUE_INLINE_SIZE];  // inline buffer for short messages
};

// ----------------------------------------------------------------------------
// Command structure for the event loop

struct nd_log_queue_cmd {
    ND_LOG_OPCODE opcode;
    union {
        struct nd_log_queue_entry entry;    // for OP_ENTRY
        struct {
            struct completion *completion;  // for OP_FLUSH, OP_REOPEN, OP_SHUTDOWN
        } sync;
    };
};

// ----------------------------------------------------------------------------
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

// ----------------------------------------------------------------------------
// Public API

// Initialize the async logging queue and start the logger thread
// Returns true on success, false on failure
bool nd_log_queue_init(void);

// Shutdown the async logging queue
// Drains all pending messages before returning
void nd_log_queue_shutdown(void);

// Check if async logging is enabled and accepting entries
bool nd_log_queue_enabled(void);

// Enqueue a pre-formatted log message
// Returns true if queued, false if dropped (queue full or not initialized)
bool nd_log_queue_enqueue(struct nd_log_queue_entry *entry);

// Get queue statistics
void nd_log_queue_get_stats(struct nd_log_queue_stats *stats);

// Flush all pending log messages (blocks until queue is empty)
void nd_log_queue_flush(void);

// Reopen all log files (blocks until complete)
// This is handled entirely in the logger thread, avoiding FILE* races
void nd_log_queue_reopen(void);

#endif // NETDATA_ND_LOG_QUEUE_H
