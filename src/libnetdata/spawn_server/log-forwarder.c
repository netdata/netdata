// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "log-forwarder.h"

typedef struct LOG_FORWARDER_ENTRY {
    int fd;
    char *cmd;
    pid_t pid;
    BUFFER *wb;
    size_t pfds_idx;
    bool remove;

    struct LOG_FORWARDER_ENTRY *prev;
    struct LOG_FORWARDER_ENTRY *next;
} LOG_FORWARDER_ENTRY;

typedef struct LOG_FORWARDER {
    LOG_FORWARDER_ENTRY *entries;
    ND_THREAD *thread;
    SPINLOCK spinlock;
    int pipe_fds[2]; // Pipe for notifications
    bool running;
} LOG_FORWARDER;

static void *log_forwarder_thread_func(void *arg);

static inline LOG_FORWARDER_ENTRY *log_forwarder_find_entry_unsafe(LOG_FORWARDER *lf, int fd) {
    for (LOG_FORWARDER_ENTRY *entry = lf->entries; entry; entry = entry->next) {
        if (entry->fd == fd)
            return entry;
    }

    return NULL;
}

static inline void log_forwarder_del_entry_unsafe(LOG_FORWARDER *lf, LOG_FORWARDER_ENTRY *entry) {
    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(lf->entries, entry, prev, next);
    buffer_free(entry->wb);
    freez(entry->cmd);
    close(entry->fd);
    freez(entry);
}

static inline void log_forwarder_wake_up_worker(LOG_FORWARDER *lf) {
    char ch = 0;
    ssize_t bytes_written = write(lf->pipe_fds[PIPE_WRITE], &ch, 1);
    if (bytes_written != 1)
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Failed to write to notification pipe");
}

LOG_FORWARDER *log_forwarder_start(void) {
    LOG_FORWARDER *lf = callocz(1, sizeof(LOG_FORWARDER));

    spinlock_init(&lf->spinlock);
    if (pipe(lf->pipe_fds) != 0) {
        freez(lf);
        return NULL;
    }

    // make sure read() will not block on this pipe
    sock_setnonblock(lf->pipe_fds[PIPE_READ]);

    lf->running = true;
    lf->thread = nd_thread_create("log-fw", NETDATA_THREAD_OPTION_JOINABLE, log_forwarder_thread_func, lf);

    return lf;
}

void log_forwarder_stop(LOG_FORWARDER *lf) {
    if (!lf) return;

    // Signal the thread to stop
    spinlock_lock(&lf->spinlock);
    lf->running = false;

    // Send a byte to the pipe to wake up the thread
    char ch = 0;
    write(lf->pipe_fds[PIPE_WRITE], &ch, 1);
    spinlock_unlock(&lf->spinlock);

    // Wait for the thread to finish
    close(lf->pipe_fds[PIPE_WRITE]); // force it to quit
    nd_thread_join(lf->thread);
    close(lf->pipe_fds[PIPE_READ]);

    // Clean up
    while (lf->entries)
        log_forwarder_del_entry_unsafe(lf, lf->entries);

    freez(lf);
}

void log_forwarder_add_fd(LOG_FORWARDER *lf, int fd) {
    LOG_FORWARDER_ENTRY *entry = callocz(1, sizeof(LOG_FORWARDER_ENTRY));
    entry->fd = fd;
    entry->cmd = NULL;
    entry->pid = 0;
    entry->pfds_idx = 0;
    entry->remove = false;
    entry->wb = buffer_create(0, NULL);

    spinlock_lock(&lf->spinlock);

    // Append to the entries list
    DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(lf->entries, entry, prev, next);

    // Send a byte to the pipe to wake up the thread
    log_forwarder_wake_up_worker(lf);

    spinlock_unlock(&lf->spinlock);
}

bool log_forwarder_del_and_close_fd(LOG_FORWARDER *lf, int fd) {
    bool ret = false;

    spinlock_lock(&lf->spinlock);

    LOG_FORWARDER_ENTRY *entry = log_forwarder_find_entry_unsafe(lf, fd);
    if(entry) {
        log_forwarder_del_entry_unsafe(lf, entry);

        // Send a byte to the pipe to wake up the thread
        log_forwarder_wake_up_worker(lf);

        ret = true;
    }

    spinlock_unlock(&lf->spinlock);

    return ret;
}

void log_forwarder_annotate_fd_name(LOG_FORWARDER *lf, int fd, const char *cmd) {
    spinlock_lock(&lf->spinlock);

    LOG_FORWARDER_ENTRY *entry = log_forwarder_find_entry_unsafe(lf, fd);
    if (entry) {
        freez(entry->cmd);
        entry->cmd = strdupz(cmd);
    }

    spinlock_unlock(&lf->spinlock);
}

void log_forwarder_annotate_fd_pid(LOG_FORWARDER *lf, int fd, pid_t pid) {
    spinlock_lock(&lf->spinlock);

    LOG_FORWARDER_ENTRY *entry = log_forwarder_find_entry_unsafe(lf, fd);
    if (entry)
        entry->pid = pid;

    spinlock_unlock(&lf->spinlock);
}

static inline void log_forwarder_log(LOG_FORWARDER *lf __maybe_unused, LOG_FORWARDER_ENTRY *entry, const char *msg) {
    const char *s = msg;
    while(*s && isspace((uint8_t)*s)) s++;
    if(*s == '\0') return; // do not log empty lines

    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_SYSLOG_IDENTIFIER, entry->cmd ? entry->cmd : "unknown"),
            ND_LOG_FIELD_I64(NDF_TID, entry->pid),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_COLLECTORS, NDLP_WARNING, "STDERR: %s", msg);
}

static void *log_forwarder_thread_func(void *arg) {
    LOG_FORWARDER *lf = (LOG_FORWARDER *)arg;

    while (1) {
        spinlock_lock(&lf->spinlock);
        if (!lf->running) {
            spinlock_unlock(&lf->spinlock);
            break;
        }

        // Count the number of fds
        size_t nfds = 1; // For the notification pipe

        // count the entries
        for(LOG_FORWARDER_ENTRY *entry = lf->entries; entry ; entry = entry->next)
            nfds++;

        struct pollfd pfds[nfds];

        // First, the notification pipe
        pfds[0].fd = lf->pipe_fds[PIPE_READ];
        pfds[0].events = POLLIN;

        int idx = 1;
        for(LOG_FORWARDER_ENTRY *entry = lf->entries; entry ; entry = entry->next, idx++) {
            pfds[idx].fd = entry->fd;
            pfds[idx].events = POLLIN;
            entry->pfds_idx = idx;
        }

        spinlock_unlock(&lf->spinlock);

        int timeout = 200; // 200ms
        int ret = poll(pfds, nfds, timeout);

        if (ret > 0) {
            // Check the notification pipe
            if (pfds[0].revents & POLLIN) {
                // Read and discard the data
                char buf[256];
                ssize_t bytes_read = read(lf->pipe_fds[PIPE_READ], buf, sizeof(buf));
                // Ignore the data; proceed regardless of the result
                if (bytes_read == -1) {
                    if (errno != EAGAIN && errno != EWOULDBLOCK) {
                        // Handle read error if necessary
                        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Failed to read from notification pipe");
                        return NULL;
                    }
                }
            }

            // Now check the other fds
            spinlock_lock(&lf->spinlock);

            size_t to_remove = 0;

            // read or mark them for deletion
            for(LOG_FORWARDER_ENTRY *entry = lf->entries; entry ; entry = entry->next) {
                if (entry->pfds_idx < 1 || entry->pfds_idx >= nfds || !(pfds[entry->pfds_idx].revents & POLLIN))
                    continue;

                BUFFER *wb = entry->wb;
                buffer_need_bytes(wb, 1024);

                ssize_t bytes_read = read(entry->fd, &wb->buffer[wb->len], wb->size - wb->len - 1);
                if(bytes_read > 0)
                    wb->len += bytes_read;
                else if(bytes_read == 0 || (bytes_read == -1 && errno != EINTR && errno != EAGAIN)) {
                    // EOF or error
                    entry->remove = true;
                    to_remove++;
                }

                // log as many lines are they have been received
                char *start = (char *)buffer_tostring(wb);
                char *newline = strchr(start, '\n');
                while(newline) {
                    *newline = '\0';
                    log_forwarder_log(lf, entry, start);

                    start = ++newline;
                    newline = strchr(newline, '\n');
                }

                if(start != wb->buffer) {
                    wb->len = strlen(start);
                    if (wb->len)
                        memmove(wb->buffer, start, wb->len);
                }

                entry->pfds_idx = 0;
            }

            if(to_remove) {
                LOG_FORWARDER_ENTRY *entry = lf->entries;
                while(entry) {
                    LOG_FORWARDER_ENTRY *next = entry->next;
                    if(entry->remove) {
                        if (buffer_strlen(entry->wb))
                            log_forwarder_log(lf, entry, buffer_tostring(entry->wb));

                        log_forwarder_del_entry_unsafe(lf, entry);
                    }
                    entry = next;
                }
            }

            spinlock_unlock(&lf->spinlock);
        } else if (ret == 0) {
            // Timeout, nothing to do
            continue;

        }
        else {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Log forwarder poll() error: %s", strerror(errno));
        }
    }

    return NULL;
}
