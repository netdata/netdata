// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn_server_internals.h"

#if defined(SPAWN_SERVER_VERSION_NOFORK)

// the child's output pipe, reading side
int spawn_server_instance_read_fd(SPAWN_INSTANCE *si) { return si->read_fd; }

// the child's input pipe, writing side
int spawn_server_instance_write_fd(SPAWN_INSTANCE *si) { return si->write_fd; }

void spawn_server_instance_read_fd_unset(SPAWN_INSTANCE *si) { si->read_fd = -1; }
void spawn_server_instance_write_fd_unset(SPAWN_INSTANCE *si) { si->write_fd = -1; }
pid_t spawn_server_instance_pid(SPAWN_INSTANCE *si) { return si->child_pid; }

pid_t spawn_server_pid(SPAWN_SERVER *server) { return server->server_pid; }

static size_t spawn_server_id = 0;
static volatile sig_atomic_t spawn_server_exit = false;
static volatile sig_atomic_t spawn_server_sigchld = false;
static SPAWN_REQUEST *spawn_server_requests = NULL;

#define SPAWN_SERVER_IOV_MAX 10

static size_t spawn_server_max_unix_socket_path_length(void) {
    struct sockaddr_un server_addr = { 0 };
    return sizeof(server_addr.sun_path) - 1;
}

static bool spawn_server_set_unix_socket_path(struct sockaddr_un *server_addr, const char *path, bool log, const char *action) {
    const size_t max_path_length = sizeof(server_addr->sun_path) - 1;

    if(strlen(path) > max_path_length) {
        errno = ENAMETOOLONG;
        if(log)
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "%s '%s': exceeds the %zu-byte AF_UNIX limit",
                   action, path, max_path_length);
        return false;
    }

    strncpyz(server_addr->sun_path, path, max_path_length);
    return true;
}

// --------------------------------------------------------------------------------------------------------------------

static int connect_to_spawn_server(const char *path, bool log) {
    int sock = -1;
    struct sockaddr_un server_addr = {
        .sun_family = AF_UNIX,
    };

    if(!spawn_server_set_unix_socket_path(&server_addr, path, log,
                                          "SPAWN PARENT: Cannot connect() to spawn server on path"))
        return -1;

    if ((sock = socket(AF_UNIX, SOCK_STREAM, 0)) == -1) {
        if(log)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: cannot create socket() to connect to spawn server.");
        return -1;
    }

    if (connect(sock, (struct sockaddr *)&server_addr, sizeof(server_addr)) == -1) {
        if(log)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: Cannot connect() to spawn server on path '%s'.", path);
        close(sock);
        return -1;
    }

    return sock;
}

// --------------------------------------------------------------------------------------------------------------------
// Encoding and decoding of spawn server request argv type of data

// Function to encode argv or envp
static void *argv_encode(const char *const *argv, size_t *out_size) {
    size_t buffer_size = 1024; // Initial buffer size
    size_t buffer_used = 0;
    char *buffer = mallocz(buffer_size);

    if(argv) {
        for (const char *const *p = argv; *p != NULL; p++) {
            if (strlen(*p) == 0)
                continue; // Skip empty strings

            size_t len = strlen(*p) + 1;
            size_t wanted_size = buffer_used + len + 1;

            if (wanted_size >= buffer_size) {
                buffer_size *= 2;

                if(buffer_size < wanted_size)
                    buffer_size = wanted_size;

                buffer = reallocz(buffer, buffer_size);
            }

            memcpy(&buffer[buffer_used], *p, len);
            buffer_used += len;
        }
    }

    buffer[buffer_used++] = '\0'; // Final empty string
    *out_size = buffer_used;

    return buffer;
}

static const char *argv_find_next(const char *ptr, const char *end) {
    return memchr(ptr, '\0', (size_t)(end - ptr));
}

// Function to decode argv or envp
static const char** argv_decode(const char *buffer, size_t size) {
    if(!buffer || !size || buffer[size - 1] != '\0')
        return NULL;

    size_t count = 0;
    const char *ptr = buffer;
    const char *end = buffer + size;
    bool found_final_empty_string = false;
    while (ptr < end) {
        if(*ptr) {
            const char *next = argv_find_next(ptr, end);
            if(!next)
                return NULL;

            count++;
            ptr = next + 1;
        }
        else {
            if(ptr != end - 1)
                return NULL;

            found_final_empty_string = true;
            break;
        }
    }

    if(!found_final_empty_string)
        return NULL;

    const char **argv = mallocz((count + 1) * sizeof(char *));

    ptr = buffer;
    for (size_t i = 0; i < count; i++) {
        argv[i] = ptr;
        ptr = argv_find_next(ptr, end) + 1;
    }
    argv[count] = NULL; // Null-terminate the array

    return argv;
}

static char **environment_vector_dup(ND_ENVIRONMENT *environment) {
    ND_ENV_SNAPSHOT *snapshot = nd_environment_snapshot_acquire(environment);
    if(!snapshot)
        return NULL;

    size_t entries = nd_environment_snapshot_entries(snapshot);
    const char *const *envp = nd_environment_snapshot_envp(snapshot);
    char **copy = callocz(entries + 1, sizeof(*copy));
    for(size_t i = 0; i < entries; i++)
        copy[i] = strdupz(envp[i]);

    nd_environment_snapshot_release(snapshot);
    return copy;
}

static void environment_vector_free(char **envp) {
    if(!envp)
        return;

    for(size_t i = 0; envp[i]; i++)
        freez(envp[i]);
    freez(envp);
}

// --------------------------------------------------------------------------------------------------------------------
// status reports

typedef enum {
    STATUS_REPORT_NONE = 0,
    STATUS_REPORT_STARTED,
    STATUS_REPORT_FAILED,
    STATUS_REPORT_EXITED,
    STATUS_REPORT_PING,
} STATUS_REPORT;

#define STATUS_REPORT_MAGIC 0xBADA55EE

struct status_report {
    uint32_t magic;
    STATUS_REPORT status;
    union {
        struct {
            pid_t pid;
        } started;

        struct {
            int err_no;
        } failed;

        struct {
            int waitpid_status;
        } exited;
    };
};

static void spawn_server_send_status_ping(int sock) {
    struct status_report sr = {
        .magic = STATUS_REPORT_MAGIC,
        .status = STATUS_REPORT_PING,
    };

    if(write(sock, &sr, sizeof(sr)) != sizeof(sr))
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Cannot send ping reply.");
}

static void spawn_server_send_status_success(SPAWN_REQUEST *rq) {
    const struct status_report sr = {
        .magic = STATUS_REPORT_MAGIC,
        .status = STATUS_REPORT_STARTED,
        .started = {
            .pid = rq->pid,
        },
    };

    if(write(rq->sock, &sr, sizeof(sr)) != sizeof(sr))
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Cannot send success status report for pid %d, request %zu: %s",
            rq->pid, rq->request_id, rq->cmdline);
}

static void spawn_server_send_status_failure(SPAWN_REQUEST *rq) {
    struct status_report sr = {
        .magic = STATUS_REPORT_MAGIC,
        .status = STATUS_REPORT_FAILED,
        .failed = {
            .err_no = errno,
        },
    };

    if(write(rq->sock, &sr, sizeof(sr)) != sizeof(sr))
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Cannot send failure status report for request %zu: %s",
            rq->request_id, rq->cmdline);
}

static void spawn_server_send_status_exit(SPAWN_REQUEST *rq, int waitpid_status) {
    struct status_report sr = {
        .magic = STATUS_REPORT_MAGIC,
        .status = STATUS_REPORT_EXITED,
        .exited = {
            .waitpid_status = waitpid_status,
        },
    };

    if(write(rq->sock, &sr, sizeof(sr)) != sizeof(sr))
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Cannot send exit status (%d) report for pid %d, request %zu: %s",
            waitpid_status, rq->pid, rq->request_id, rq->cmdline);
}

// --------------------------------------------------------------------------------------------------------------------
// execute a received request

static void request_free(SPAWN_REQUEST *rq) {
    if(rq->fds[0] != -1) close(rq->fds[0]);
    if(rq->fds[1] != -1) close(rq->fds[1]);
    if(rq->fds[2] != -1) close(rq->fds[2]);
    if(rq->fds[3] != -1) close(rq->fds[3]);
    if(rq->sock != -1) close(rq->sock);
    freez((void *)rq->argv);
    freez((void *)rq->envp);
    freez((void *)rq->data);
    freez((void *)rq->cmdline);
    freez((void *)rq);
}

static bool spawn_external_command(SPAWN_SERVER *server __maybe_unused, SPAWN_REQUEST *rq) {
    // Close custom_fd - it is not needed for exec mode
    if(rq->fds[3] != -1) { close(rq->fds[3]); rq->fds[3] = -1; }

    if(!rq->argv) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: there is no argv pointer to exec");
        return false;
    }

    if(rq->fds[0] == -1 || rq->fds[1] == -1 || rq->fds[2] == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: stdio fds are missing from the request");
        return false;
    }

    CLEAN_BUFFER *wb = argv_to_cmdline_buffer(rq->argv);
    rq->cmdline = strdupz(buffer_tostring(wb));

    posix_spawn_file_actions_t file_actions;
    if (posix_spawn_file_actions_init(&file_actions) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawn_file_actions_init() failed: %s", rq->cmdline);
        return false;
    }

    posix_spawn_file_actions_adddup2(&file_actions, rq->fds[0], STDIN_FILENO);
    posix_spawn_file_actions_adddup2(&file_actions, rq->fds[1], STDOUT_FILENO);
    posix_spawn_file_actions_adddup2(&file_actions, rq->fds[2], STDERR_FILENO);
    posix_spawn_file_actions_addclose(&file_actions, rq->fds[0]);
    posix_spawn_file_actions_addclose(&file_actions, rq->fds[1]);
    posix_spawn_file_actions_addclose(&file_actions, rq->fds[2]);

    posix_spawnattr_t attr;
    if (posix_spawnattr_init(&attr) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawnattr_init() failed: %s", rq->cmdline);
        posix_spawn_file_actions_destroy(&file_actions);
        return false;
    }

    // Set the flags to reset the signal mask and signal actions
    sigset_t empty_mask;
    sigemptyset(&empty_mask);
    if (posix_spawnattr_setsigmask(&attr, &empty_mask) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawnattr_setsigmask() failed: %s", rq->cmdline);
        posix_spawn_file_actions_destroy(&file_actions);
        posix_spawnattr_destroy(&attr);
        return false;
    }

    short flags = POSIX_SPAWN_SETSIGMASK | POSIX_SPAWN_SETSIGDEF;
    if (posix_spawnattr_setflags(&attr, flags) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: posix_spawnattr_setflags() failed: %s", rq->cmdline);
        posix_spawn_file_actions_destroy(&file_actions);
        posix_spawnattr_destroy(&attr);
        return false;
    }

    int fds_to_keep[] = {
        rq->fds[0],
        rq->fds[1],
        rq->fds[2],
        nd_log_systemd_journal_fd(),
    };
    os_close_all_non_std_open_fds_except(fds_to_keep, _countof(fds_to_keep), CLOSE_RANGE_CLOEXEC);

    errno_clear();
    if (posix_spawn(&rq->pid, rq->argv[0], &file_actions, &attr, (char * const *)rq->argv, (char * const *)rq->envp) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: posix_spawn() failed: %s", rq->cmdline);

        posix_spawnattr_destroy(&attr);
        posix_spawn_file_actions_destroy(&file_actions);
        return false;
    }

    // Destroy the posix_spawnattr_t and posix_spawn_file_actions_t structures
    posix_spawnattr_destroy(&attr);
    posix_spawn_file_actions_destroy(&file_actions);

    // Close the read end of the stdin pipe and the write end of the stdout pipe in the parent process
    close(rq->fds[0]); rq->fds[0] = -1;
    close(rq->fds[1]); rq->fds[1] = -1;
    close(rq->fds[2]); rq->fds[2] = -1;

    nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "SPAWN SERVER: process created with pid %d: %s", rq->pid, rq->cmdline);
    return true;
}

static bool spawn_server_run_callback(SPAWN_SERVER *server __maybe_unused, SPAWN_REQUEST *rq) {
    rq->cmdline = strdupz("callback() function");

    if(server->cb == NULL) {
        errno = ENOSYS;
        return false;
    }

    // Do not let the callback child run the spawn server's private signal handlers before it can reset them.
    sigset_t callback_signals, previous_mask;
    sigemptyset(&callback_signals);
    sigaddset(&callback_signals, SIGTERM);
    sigaddset(&callback_signals, SIGCHLD);

    int mask_rc = pthread_sigmask(SIG_BLOCK, &callback_signals, &previous_mask);
    if(mask_rc != 0) {
        errno = mask_rc;
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to block callback child signals before fork().");
        return false;
    }

    pid_t pid = fork();
    int fork_errno = errno;

    if(pid != 0) {
        mask_rc = pthread_sigmask(SIG_SETMASK, &previous_mask, NULL);
        if(mask_rc != 0) {
            if(pid > 0) {
                kill(pid, SIGKILL);

                while(waitpid(pid, NULL, 0) == -1 && errno == EINTR) {
                    // Retry until the child is reaped or waitpid() fails permanently.
                }
            }

            spawn_server_exit = true;
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to restore signal mask after callback fork().");
            errno = mask_rc;
            return false;
        }
    }

    if (pid < 0) {
        // fork failed

        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to fork() child for callback.");
        errno = fork_errno;
        return false;
    }
    else if (pid == 0) {
        // the child

        struct sigaction sa = {
            .sa_handler = SIG_DFL,
        };
        sigemptyset(&sa.sa_mask);

        if(sigaction(SIGTERM, &sa, NULL) == -1 || sigaction(SIGCHLD, &sa, NULL) == -1) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to reset callback child signal handlers.");
            _exit(EXIT_FAILURE);
        }

        if(nd_environment_fork_child_reset_from_native() != 0)
            _exit(EXIT_FAILURE);

        mask_rc = pthread_sigmask(SIG_SETMASK, &previous_mask, NULL);
        if(mask_rc != 0) {
            errno = mask_rc;
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to restore callback child signal mask.");
            _exit(EXIT_FAILURE);
        }

        // close the server sockets;
        close(server->sock); server->sock = -1;
        if(server->pipe[0] != -1) { close(server->pipe[0]); server->pipe[0] = -1; }
        if(server->pipe[1] != -1) { close(server->pipe[1]); server->pipe[1] = -1; }

        // set the process name
        os_setproctitle("spawn-callback", server->argc, server->argv);

        // close all open file descriptors of the parent, but keep ours
        int fds_to_keep[] = {
            rq->fds[0],
            rq->fds[1],
            rq->fds[2],
            rq->fds[3],
            nd_log_systemd_journal_fd(),
        };
        os_close_all_non_std_open_fds_except(fds_to_keep, _countof(fds_to_keep), 0);
        nd_log_reopen_log_files_for_spawn_server("spawn-callback");

        // get the fds from the request
        int stdin_fd = rq->fds[0];
        int stdout_fd = rq->fds[1];
        int stderr_fd = rq->fds[2];
        int custom_fd = rq->fds[3]; (void)custom_fd;

        // change stdio fds to the ones in the request
        if (dup2(stdin_fd, STDIN_FILENO) == -1) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: cannot dup2(%d) stdin of request No %zu: %s",
                   stdin_fd, rq->request_id, rq->cmdline);
            exit(EXIT_FAILURE);
        }
        if (dup2(stdout_fd, STDOUT_FILENO) == -1) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: cannot dup2(%d) stdin of request No %zu: %s",
                   stdout_fd, rq->request_id, rq->cmdline);
            exit(EXIT_FAILURE);
        }
        if (dup2(stderr_fd, STDERR_FILENO) == -1) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: cannot dup2(%d) stderr of request No %zu: %s",
                   stderr_fd, rq->request_id, rq->cmdline);
            exit(EXIT_FAILURE);
        }

        // close the excess fds
        close(stdin_fd); stdin_fd = rq->fds[0] = STDIN_FILENO;
        close(stdout_fd); stdout_fd = rq->fds[1] = STDOUT_FILENO;
        close(stderr_fd); stderr_fd = rq->fds[2] = STDERR_FILENO;

        if(nd_environment_fork_child_replace(rq->envp) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: cannot adopt the environment for request No %zu",
                   rq->request_id);
            exit(EXIT_FAILURE);
        }

        if(nd_environment_freeze_process() != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: cannot freeze the callback environment for request No %zu",
                   rq->request_id);
            exit(EXIT_FAILURE);
        }

        // run the callback and return its code
        exit(server->cb(rq));
    }

    // the parent
    rq->pid = pid;

    return true;
}

static void spawn_server_execute_request(SPAWN_SERVER *server, SPAWN_REQUEST *rq) {
    bool done;
    switch(rq->type) {
        case SPAWN_INSTANCE_TYPE_EXEC:
            done = spawn_external_command(server, rq);
            break;

        case SPAWN_INSTANCE_TYPE_CALLBACK:
            done = spawn_server_run_callback(server, rq);
            break;

        default:
            errno = EINVAL;
            done = false;
            break;
    }

    if(!done) {
        spawn_server_send_status_failure(rq);
        request_free(rq);
        return;
    }

    // let the parent know
    spawn_server_send_status_success(rq);

    // do not keep data we don't need at the parent
    freez((void *)rq->envp); rq->envp = NULL;
    freez((void *)rq->argv); rq->argv = NULL;
    freez((void *)rq->data); rq->data = NULL;
    rq->data_size = 0;

    // do not keep fds we don't need at the parent
    if(rq->fds[0] != -1) { close(rq->fds[0]); rq->fds[0] = -1; }
    if(rq->fds[1] != -1) { close(rq->fds[1]); rq->fds[1] = -1; }
    if(rq->fds[2] != -1) { close(rq->fds[2]); rq->fds[2] = -1; }
    if(rq->fds[3] != -1) { close(rq->fds[3]); rq->fds[3] = -1; }

    // keep it in the list
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(spawn_server_requests, rq, prev, next);
}

// --------------------------------------------------------------------------------------------------------------------
// Sending and receiving requests

typedef enum __attribute__((packed)) {
    SPAWN_SERVER_MSG_INVALID = 0,
    SPAWN_SERVER_MSG_REQUEST,
    SPAWN_SERVER_MSG_PING,
} SPAWN_SERVER_MSG;

static void spawn_server_close_received_fds(void *control, size_t controllen) {
    const size_t header_len = CMSG_LEN(0);

    if(!control || controllen < header_len)
        return;

    unsigned char *current = control;
    size_t remaining = controllen;

    while(remaining >= header_len) {
        struct cmsghdr *cmsg = (struct cmsghdr *)current;
        if(cmsg->cmsg_len < header_len)
            break;

        if(cmsg->cmsg_level != SOL_SOCKET || cmsg->cmsg_type != SCM_RIGHTS ||
           cmsg->cmsg_len < CMSG_LEN(sizeof(int)))
            goto next_cmsg;

        uintptr_t data_start = (uintptr_t)CMSG_DATA(cmsg);
        uintptr_t control_end = (uintptr_t)current + remaining;
        if(data_start < (uintptr_t)current || data_start >= control_end)
            goto next_cmsg;

        size_t fd_bytes = cmsg->cmsg_len - CMSG_LEN(0);
        size_t available_bytes = control_end - data_start;
        if(fd_bytes > available_bytes)
            fd_bytes = available_bytes;

        int *fds = (int *)(void *)data_start;
        for(size_t i = 0; i < fd_bytes / sizeof(int); i++)
            close(fds[i]);

next_cmsg:
        if(cmsg->cmsg_len > remaining)
            break;

        size_t next_cmsg_len = CMSG_SPACE(cmsg->cmsg_len - header_len);
        if(!next_cmsg_len || next_cmsg_len > remaining)
            break;

        current += next_cmsg_len;
        remaining -= next_cmsg_len;
    }
}

static bool spawn_server_iov_total_size(const struct iovec *iov, size_t iovlen, size_t *total) {
    size_t bytes = 0;

    for(size_t i = 0; i < iovlen; i++) {
        if(unlikely(iov[i].iov_len > SIZE_MAX - bytes))
            return false;

        bytes += iov[i].iov_len;
    }

    *total = bytes;
    return true;
}

static void spawn_server_iov_advance(struct iovec *iov, size_t iovlen, size_t *first, size_t bytes) {
    while(bytes && *first < iovlen) {
        if(bytes < iov[*first].iov_len) {
            iov[*first].iov_base = (char *)iov[*first].iov_base + bytes;
            iov[*first].iov_len -= bytes;
            return;
        }

        bytes -= iov[*first].iov_len;
        (*first)++;
    }
}

static bool spawn_server_recvmsg_fully(int sock, const struct iovec *iov, size_t iovlen,
                                       void *control, size_t controllen, size_t *received_controllen,
                                       const char *description) {
    if(received_controllen)
        *received_controllen = 0;

    if(unlikely(iovlen > SPAWN_SERVER_IOV_MAX)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: cannot recvmsg() %s: too many iovecs (%zu)",
               description, iovlen);
        return false;
    }

    struct iovec pending[SPAWN_SERVER_IOV_MAX];
    memcpy(pending, iov, sizeof(*iov) * iovlen);

    size_t remaining = 0;
    if(unlikely(!spawn_server_iov_total_size(pending, iovlen, &remaining))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: cannot recvmsg() %s: iovec byte count overflows",
               description);
        return false;
    }

    size_t first = 0;
    size_t control_received = 0;

    while(remaining) {
        struct msghdr msg = {
            .msg_iov = &pending[first],
            .msg_iovlen = iovlen - first,
        };

        if(control && !control_received) {
            msg.msg_control = control;
            msg.msg_controllen = controllen;
        }

        ssize_t bytes = recvmsg(sock, &msg, 0);
        if(bytes < 0) {
            if(errno == EINTR)
                continue;

            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: failed to recvmsg() %s.",
                   description);
            spawn_server_close_received_fds(control, control_received);
            return false;
        }

        if(bytes == 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: peer closed socket while receiving %s.",
                   description);
            spawn_server_close_received_fds(control, control_received);
            return false;
        }

        if(msg.msg_flags & MSG_CTRUNC) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: received truncated control message while receiving %s.",
                   description);
            spawn_server_close_received_fds(control, msg.msg_controllen);
            return false;
        }

        if(control && !control_received && msg.msg_controllen)
            control_received = msg.msg_controllen;

        if(unlikely((size_t)bytes > remaining)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: recvmsg() returned more bytes than requested while receiving %s.",
                   description);
            spawn_server_close_received_fds(control, control_received);
            return false;
        }

        spawn_server_iov_advance(pending, iovlen, &first, (size_t)bytes);
        remaining -= (size_t)bytes;
    }

    if(received_controllen)
        *received_controllen = control_received;

    return true;
}

static bool spawn_server_sendmsg_fully(int sock, const struct iovec *iov, size_t iovlen,
                                       void *control, size_t controllen, const char *description) {
    if(unlikely(iovlen > SPAWN_SERVER_IOV_MAX)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: cannot sendmsg() %s: too many iovecs (%zu)",
               description, iovlen);
        return false;
    }

    struct iovec pending[SPAWN_SERVER_IOV_MAX];
    memcpy(pending, iov, sizeof(*iov) * iovlen);

    size_t remaining = 0;
    if(unlikely(!spawn_server_iov_total_size(pending, iovlen, &remaining))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: cannot sendmsg() %s: iovec byte count overflows",
               description);
        return false;
    }

    size_t first = 0;
    bool control_sent = !control || !controllen;

    while(remaining) {
        struct msghdr msg = {
            .msg_iov = &pending[first],
            .msg_iovlen = iovlen - first,
        };

        if(!control_sent) {
            msg.msg_control = control;
            msg.msg_controllen = controllen;
        }

        ssize_t bytes = sendmsg(sock, &msg, 0);
        if(bytes < 0) {
            if(errno == EINTR)
                continue;

            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN PARENT: failed to sendmsg() %s.",
                   description);
            return false;
        }

        if(bytes == 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN PARENT: sendmsg() made no progress while sending %s.",
                   description);
            return false;
        }

        control_sent = true;

        if(unlikely((size_t)bytes > remaining)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN PARENT: sendmsg() returned more bytes than requested while sending %s.",
                   description);
            return false;
        }

        spawn_server_iov_advance(pending, iovlen, &first, (size_t)bytes);
        remaining -= (size_t)bytes;
    }

    return true;
}

static bool spawn_server_is_running(const char *path) {
    struct iovec iov[7];
    SPAWN_SERVER_MSG msg_type = SPAWN_SERVER_MSG_PING;
    size_t dummy_size = 0;
    SPAWN_INSTANCE_TYPE dummy_type = 0;
    ND_UUID magic = UUID_ZERO;

    iov[0].iov_base = &msg_type;
    iov[0].iov_len = sizeof(msg_type);

    iov[1].iov_base = magic.uuid;
    iov[1].iov_len = sizeof(magic.uuid);

    iov[2].iov_base = &dummy_size;
    iov[2].iov_len = sizeof(dummy_size);

    iov[3].iov_base = &dummy_size;
    iov[3].iov_len = sizeof(dummy_size);

    iov[4].iov_base = &dummy_size;
    iov[4].iov_len = sizeof(dummy_size);

    iov[5].iov_base = &dummy_size;
    iov[5].iov_len = sizeof(dummy_size);

    iov[6].iov_base = &dummy_type;
    iov[6].iov_len = sizeof(dummy_type);

    int sock = connect_to_spawn_server(path, false);
    if(sock == -1)
        return false;

    if(!spawn_server_sendmsg_fully(sock, iov, 7, NULL, 0, "spawn server ping")) {
        close(sock);
        return false;
    }

    // Receive response
    struct status_report sr = { 0 };
    if (read(sock, &sr, sizeof(sr)) != sizeof(sr)) {
        // cannot receive a ping reply
        close(sock);
        return false;
    }

    close(sock);
    return sr.status == STATUS_REPORT_PING;
}

static bool spawn_server_send_request(ND_UUID *magic, ND_ENVIRONMENT *environment, SPAWN_REQUEST *request) {
    bool ret = false;

    size_t env_size = 0;
    size_t argv_size = 0;

    ND_ENV_SNAPSHOT *snapshot = nd_environment_snapshot_acquire(environment);
    if(!snapshot)
        return false;

    void *encoded_env = argv_encode(nd_environment_snapshot_envp(snapshot), &env_size);
    nd_environment_snapshot_release(snapshot);

    void *encoded_argv = argv_encode(request->argv, &argv_size);

    struct msghdr msg = {0};
    struct cmsghdr *cmsg;
    SPAWN_SERVER_MSG msg_type = SPAWN_SERVER_MSG_REQUEST;
    char cmsgbuf[CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)] __attribute__((aligned(sizeof(size_t))));
    struct iovec iov[SPAWN_SERVER_IOV_MAX];

    // We send 1 request with 10 iovec in it
    // The request will be received in 2 parts
    // 1. the first 6 iovec which include the sizes of the memory allocations required
    // 2. the last 4 iovec which require the memory allocations to be received

    iov[0].iov_base = &msg_type;
    iov[0].iov_len = sizeof(msg_type);

    iov[1].iov_base = magic->uuid;
    iov[1].iov_len = sizeof(magic->uuid);

    iov[2].iov_base = &request->request_id;
    iov[2].iov_len = sizeof(request->request_id);

    iov[3].iov_base = &env_size;
    iov[3].iov_len = sizeof(env_size);

    iov[4].iov_base = &argv_size;
    iov[4].iov_len = sizeof(argv_size);

    iov[5].iov_base = &request->data_size;
    iov[5].iov_len = sizeof(request->data_size);

    iov[6].iov_base = &request->type;  // Added this line
    iov[6].iov_len = sizeof(request->type);

    iov[7].iov_base = encoded_env;
    iov[7].iov_len = env_size;

    iov[8].iov_base = encoded_argv;
    iov[8].iov_len = argv_size;

    iov[9].iov_base = (char *)request->data;
    iov[9].iov_len = request->data_size;

    msg.msg_control = cmsgbuf;
    msg.msg_controllen = CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS);

    cmsg = CMSG_FIRSTHDR(&msg);
    cmsg->cmsg_level = SOL_SOCKET;
    cmsg->cmsg_type = SCM_RIGHTS;
    cmsg->cmsg_len = CMSG_LEN(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS);

    memcpy(CMSG_DATA(cmsg), request->fds, sizeof(int) * SPAWN_SERVER_TRANSFER_FDS);

    if(!spawn_server_sendmsg_fully(request->sock, iov, 10, cmsgbuf, CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS),
                                   "request to spawn server"))
        goto cleanup;

    ret = true;
    // fprintf(stderr, "PARENT: sent request %zu on socket %d (fds: %d, %d, %d, %d) from tid %d\n",
    //     request->request_id, request->socket, request->fds[0], request->fds[1], request->fds[2], request->fds[3], os_gettid());

cleanup:
    freez(encoded_env);
    freez(encoded_argv);
    return ret;
}

static void spawn_server_receive_request(int sock, SPAWN_SERVER *server) {
    struct msghdr msg = {0};
    struct iovec iov[7];
    SPAWN_SERVER_MSG msg_type = SPAWN_SERVER_MSG_INVALID;
    size_t request_id;
    size_t env_size;
    size_t argv_size;
    size_t data_size;
    ND_UUID magic = UUID_ZERO;
    SPAWN_INSTANCE_TYPE type;
    char cmsgbuf[CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)] __attribute__((aligned(sizeof(size_t))));
    char *envp_encoded = NULL, *argv_encoded = NULL, *data = NULL;
    const char **envp_decoded = NULL, **argv_decoded = NULL;
    int stdin_fd = -1, stdout_fd = -1, stderr_fd = -1, custom_fd = -1;

    // First recvmsg() to read sizes and control message
    iov[0].iov_base = &msg_type;
    iov[0].iov_len = sizeof(msg_type);

    iov[1].iov_base = magic.uuid;
    iov[1].iov_len = sizeof(magic.uuid);

    iov[2].iov_base = &request_id;
    iov[2].iov_len = sizeof(request_id);

    iov[3].iov_base = &env_size;
    iov[3].iov_len = sizeof(env_size);

    iov[4].iov_base = &argv_size;
    iov[4].iov_len = sizeof(argv_size);

    iov[5].iov_base = &data_size;
    iov[5].iov_len = sizeof(data_size);

    iov[6].iov_base = &type;
    iov[6].iov_len = sizeof(type);

    msg.msg_iov = iov;
    msg.msg_iovlen = 7;
    msg.msg_control = cmsgbuf;
    msg.msg_controllen = sizeof(cmsgbuf);

    memset(cmsgbuf, 0, sizeof(cmsgbuf));
    size_t received_controllen = 0;
    if(!spawn_server_recvmsg_fully(sock, iov, 7, cmsgbuf, sizeof(cmsgbuf), &received_controllen,
                                   "the first part of the request")) {
        close(sock);
        return;
    }

    if(msg_type == SPAWN_SERVER_MSG_PING) {
        spawn_server_close_received_fds(cmsgbuf, received_controllen);
        spawn_server_send_status_ping(sock);
        close(sock);
        return;
    }

    if(msg_type != SPAWN_SERVER_MSG_REQUEST) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: invalid request message type %u. Rejecting request.",
               (unsigned)msg_type);
        spawn_server_close_received_fds(cmsgbuf, received_controllen);
        close(sock);
        return;
    }

    msg.msg_controllen = received_controllen;

    // Extract file descriptors from control message
    struct cmsghdr *cmsg = CMSG_FIRSTHDR(&msg);
    if (cmsg == NULL || cmsg->cmsg_len != CMSG_LEN(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: Received invalid control message (expected %zu bytes, received %zu bytes)",
               (size_t)(CMSG_LEN(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)), (size_t)(cmsg?cmsg->cmsg_len:0));
        spawn_server_close_received_fds(cmsgbuf, received_controllen);
        close(sock);
        return;
    }

    if (cmsg->cmsg_level != SOL_SOCKET || cmsg->cmsg_type != SCM_RIGHTS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Received unexpected control message type.");
        spawn_server_close_received_fds(cmsgbuf, received_controllen);
        close(sock);
        return;
    }

    int *fds = (int *)CMSG_DATA(cmsg);
    stdin_fd = fds[0];
    stdout_fd = fds[1];
    stderr_fd = fds[2];
    custom_fd = fds[3];

    if (stdin_fd < 0 || stdout_fd < 0 || stderr_fd < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: invalid file descriptors received, stdin = %d, stdout = %d, stderr = %d",
            stdin_fd, stdout_fd, stderr_fd);
        goto cleanup;
    }

    if(!UUIDeq(magic, server->magic)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Invalid authorization key for request %zu. "
            "Rejecting request.",
            request_id);
        goto cleanup;
    }

    if(type == SPAWN_INSTANCE_TYPE_EXEC && !(server->options & SPAWN_SERVER_OPTION_EXEC)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Request %zu wants to exec, but exec is not allowed for this spawn server. "
            "Rejecting request.",
            request_id);
        goto cleanup;
    }

    if(type == SPAWN_INSTANCE_TYPE_CALLBACK && !(server->options & SPAWN_SERVER_OPTION_CALLBACK)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Request %zu wants to run a callback, but callbacks are not allowed for this spawn server. "
            "Rejecting request.",
            request_id);
        goto cleanup;
    }

    if(env_size == 0 || argv_size == 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: invalid encoded request sizes, env = %zu, argv = %zu",
               env_size, argv_size);
        goto cleanup;
    }

    // Second recvmsg() to read buffer contents
    iov[0].iov_base = envp_encoded = mallocz(env_size);
    iov[0].iov_len = env_size;
    iov[1].iov_base = argv_encoded = mallocz(argv_size);
    iov[1].iov_len = argv_size;
    iov[2].iov_base = data = data_size ? mallocz(data_size) : NULL;
    iov[2].iov_len = data_size;

    msg.msg_iov = iov;
    msg.msg_iovlen = data_size ? 3 : 2;
    msg.msg_control = NULL;
    msg.msg_controllen = 0;

    if(!spawn_server_recvmsg_fully(sock, iov, msg.msg_iovlen, NULL, 0, NULL,
                                   "the second part of the request"))
        goto cleanup;

    envp_decoded = argv_decode(envp_encoded, env_size);
    argv_decoded = argv_decode(argv_encoded, argv_size);
    if(!envp_decoded || !argv_decoded) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: received malformed encoded argv/envp buffers for request %zu",
               request_id);
        goto cleanup;
    }

    // fprintf(stderr, "SPAWN SERVER: received request %zu (fds: %d, %d, %d, %d)\n", request_id,
    //     stdin_fd, stdout_fd, stderr_fd, custom_fd);

    SPAWN_REQUEST *rq = mallocz(sizeof(*rq));
    *rq = (SPAWN_REQUEST){
        .pid = 0,
        .request_id = request_id,
        .sock = sock,
        .fds = {
            [0] = stdin_fd,
            [1] = stdout_fd,
            [2] = stderr_fd,
            [3] = custom_fd,
        },
        .envp = envp_decoded,
        .argv = argv_decoded,
        .data = data,
        .data_size = data_size,
        .type = type
    };
    envp_decoded = NULL;
    argv_decoded = NULL;

    // all allocations given to the request are now handled by this
    spawn_server_execute_request(server, rq);

    // since we make rq->argv and rq->environment NULL when we keep it,
    // we don't need these anymore.
    freez(envp_encoded);
    freez(argv_encoded);
    return;

cleanup:
    close(sock);
    if(stdin_fd != -1) close(stdin_fd);
    if(stdout_fd != -1) close(stdout_fd);
    if(stderr_fd != -1) close(stderr_fd);
    if(custom_fd != -1) close(custom_fd);
    freez((void *)envp_decoded);
    freez((void *)argv_decoded);
    freez(envp_encoded);
    freez(argv_encoded);
    freez(data);
}

// --------------------------------------------------------------------------------------------------------------------
// the spawn server main event loop

static void spawn_server_sigchld_handler(int signo __maybe_unused) {
    spawn_server_sigchld = true;
}

static void spawn_server_sigterm_handler(int signo __maybe_unused) {
    spawn_server_exit = true;
}

static SPAWN_REQUEST *find_request_by_pid(pid_t pid) {
    for(SPAWN_REQUEST *rq = spawn_server_requests; rq ;rq = rq->next)
        if(rq->pid == pid)
            return rq;

    return NULL;
}

static void spawn_server_process_sigchld(void) {
    // nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: checking for exited children");

    spawn_server_sigchld = false;

    int status;
    pid_t pid;

    // Loop to check for exited child processes
    while ((pid = waitpid((pid_t)(-1), &status, WNOHANG)) != 0) {
        if(pid == -1)
            break;

        errno_clear();

        SPAWN_REQUEST *rq = find_request_by_pid(pid);
        size_t request_id = rq ? rq->request_id : 0;
        bool send_report_remove_request = false;

        if(WIFEXITED(status)) {
            if(WEXITSTATUS(status))
                nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                    "SPAWN SERVER: child with pid %d (request %zu) exited with exit code %d: %s",
                    pid, request_id, WEXITSTATUS(status), rq ? rq->cmdline : "[request not found]");
            send_report_remove_request = true;
        }
        else if(WIFSIGNALED(status)) {
            if(WCOREDUMP(status))
                nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                    "SPAWN SERVER: child with pid %d (request %zu) coredump'd due to signal %d: %s",
                    pid, request_id, WTERMSIG(status), rq ? rq->cmdline : "[request not found]");
            else
                nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                    "SPAWN SERVER: child with pid %d (request %zu) killed by signal %d: %s",
                    pid, request_id, WTERMSIG(status), rq ? rq->cmdline : "[request not found]");
            send_report_remove_request = true;
        }
        else if(WIFSTOPPED(status)) {
            nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                "SPAWN SERVER: child with pid %d (request %zu) stopped due to signal %d: %s",
                pid, request_id, WSTOPSIG(status), rq ? rq->cmdline : "[request not found]");
            send_report_remove_request = false;
        }
        else if(WIFCONTINUED(status)) {
            nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                "SPAWN SERVER: child with pid %d (request %zu) continued due to signal %d: %s",
                pid, request_id, SIGCONT, rq ? rq->cmdline : "[request not found]");
            send_report_remove_request = false;
        }
        else {
            nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                "SPAWN SERVER: child with pid %d (request %zu) reports unhandled status: %s",
                pid, request_id, rq ? rq->cmdline : "[request not found]");
            send_report_remove_request = false;
        }

        if(send_report_remove_request && rq) {
            spawn_server_send_status_exit(rq, status);
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(spawn_server_requests, rq, prev, next);
            request_free(rq);
        }
    }
}

static void spawn_server_signal_all_children(int signo) {
    for(SPAWN_REQUEST *rq = spawn_server_requests; rq ; rq = rq->next) {
        if(kill(rq->pid, signo) != 0)
            nd_log(NDLS_COLLECTORS, signo == SIGKILL ? NDLP_ERR : NDLP_WARNING,
                   "SPAWN SERVER: failed to send signal %d to child pid %d (request %zu): %s",
                   signo, rq->pid, rq->request_id, strerror(errno));
    }
}

static bool spawn_server_wait_for_children_to_exit(usec_t timeout_ut) {
    usec_t deadline_ut = now_monotonic_usec() + timeout_ut;

    while(spawn_server_requests) {
        spawn_server_process_sigchld();
        if(!spawn_server_requests)
            return true;

        if(now_monotonic_usec() >= deadline_ut)
            return false;

        sleep_usec(10 * USEC_PER_MS);
    }

    return true;
}

static int spawn_server_event_loop(SPAWN_SERVER *server) {
    int pipe_fd = server->pipe[1];
    close(server->pipe[0]); server->pipe[0] = -1;

    signals_block_all();
    int wanted_signals[] = {SIGTERM, SIGCHLD};
    signals_unblock(wanted_signals, _countof(wanted_signals));

    // Set up the signal handler for SIGCHLD and SIGTERM
    struct sigaction sa;
    sa.sa_handler = spawn_server_sigchld_handler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = SA_RESTART | SA_NOCLDSTOP;
    if (sigaction(SIGCHLD, &sa, NULL) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: sigaction() failed for SIGCHLD");
        return 1;
    }

    sa.sa_handler = spawn_server_sigterm_handler;
    if (sigaction(SIGTERM, &sa, NULL) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: sigaction() failed for SIGTERM");
        return 1;
    }

    struct status_report sr = {
        .status = STATUS_REPORT_STARTED,
        .started = {
            .pid = getpid(),
        },
    };
    if (write(pipe_fd, &sr, sizeof(sr)) != sizeof(sr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: failed to write initial status report.");
        return 1;
    }

    struct pollfd fds[2];
    fds[0].fd = server->sock;
    fds[0].events = POLLIN;
    fds[1].fd = pipe_fd;
    fds[1].events = POLLHUP | POLLERR;

    while(!spawn_server_exit) {
        int ret = poll(fds, 2, 500);
        if (spawn_server_sigchld || ret == 0) {
            spawn_server_process_sigchld();
            errno_clear();

            if(ret == -1 || ret == 0)
                continue;
        }

        if (ret == -1) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: poll() failed");
            break;
        }

        if (fds[1].revents & (POLLHUP|POLLERR)) {
            // Pipe has been closed (parent has exited)
            nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "SPAWN SERVER: Parent process closed socket (exited?)");
            break;
        }

        if (fds[0].revents & POLLIN) {
            int sock = accept(server->sock, NULL, NULL);
            if (sock == -1) {
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: accept() failed");
                continue;
            }

            // do not fork this socket
            sock_setcloexec(sock, true);

            // receive the request and process it
            spawn_server_receive_request(sock, server);
        }
    }

    // Cleanup before exiting
    unlink(server->path);

    // stop all children
    if(spawn_server_requests) {
        spawn_server_signal_all_children(SIGTERM);

        if(!spawn_server_wait_for_children_to_exit((usec_t)SPAWN_KILL_DEFAULT_GRACE_MS * USEC_PER_MS)) {
            nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                   "SPAWN SERVER: children did not exit after SIGTERM; sending SIGKILL");

            spawn_server_signal_all_children(SIGKILL);

            if(!spawn_server_wait_for_children_to_exit((usec_t)SPAWN_KILL_DEFAULT_GRACE_MS * USEC_PER_MS))
                nd_log(NDLS_COLLECTORS, NDLP_ERR,
                       "SPAWN SERVER: giving up waiting for children after SIGKILL");
        }
    }

    return 0;
}

// --------------------------------------------------------------------------------------------------------------------
// management of the spawn server

void spawn_server_destroy(SPAWN_SERVER *server) {
    if(server->pipe[0] != -1) close(server->pipe[0]);
    if(server->pipe[1] != -1) close(server->pipe[1]);
    if(server->sock != -1) close(server->sock);

    if(server->server_pid) {
        kill(server->server_pid, SIGTERM);
        waitpid(server->server_pid, NULL, 0);
    }

    if(server->path) {
        unlink(server->path);
        freez(server->path);
    }

    freez((void *)server->name);
    freez(server);
}

static bool spawn_server_create_listening_socket(SPAWN_SERVER *server) {
    if(spawn_server_is_running(server->path)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Server is already listening on path '%s'", server->path);
        return false;
    }

    struct sockaddr_un server_addr = {
        .sun_family = AF_UNIX,
    };

    if(!spawn_server_set_unix_socket_path(&server_addr, server->path, true,
                                          "SPAWN SERVER: Cannot listen on path"))
        return false;

    if ((server->sock = socket(AF_UNIX, SOCK_STREAM, 0)) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to create socket()");
        return false;
    }

    unlink(server->path);
    errno = 0;

    if (bind(server->sock, (struct sockaddr *)&server_addr, sizeof(server_addr)) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to bind()");
        return false;
    }

    if (listen(server->sock, 5) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to listen()");
        return false;
    }

    if(chmod(server->path, 0770) != 0)
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: failed to chmod '%s' to 0770", server->path);

    return true;
}

static void replace_stdio_with_dev_null() {
    // we cannot log in this function - the logger is not yet initialized after fork()

    int dev_null_fd = open("/dev/null", O_RDWR);
    if (dev_null_fd == -1) {
        // nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to open /dev/null: %s", strerror(errno));
        return;
    }

    // Redirect stdin (fd 0)
    if (dup2(dev_null_fd, STDIN_FILENO) == -1) {
        // nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to redirect stdin to /dev/null: %s", strerror(errno));
        close(dev_null_fd);
        return;
    }

    // Redirect stdout (fd 1)
    if (dup2(dev_null_fd, STDOUT_FILENO) == -1) {
        // nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to redirect stdout to /dev/null: %s", strerror(errno));
        close(dev_null_fd);
        return;
    }

    // Close the original /dev/null file descriptor
    close(dev_null_fd);
}

SPAWN_SERVER *spawn_server_create(SPAWN_SERVER_OPTIONS options, const char *name,
                                  spawn_request_callback_t child_callback, int argc, const char **argv,
                                  ND_ENVIRONMENT *environment, const char *runtime_directory) {
    if(nd_environment_is_process_frozen()) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: refusing to fork a nofork server after the environment freeze");
        return NULL;
    }

    if(!environment) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: environment context is required");
        return NULL;
    }

    if(!runtime_directory || !*runtime_directory) {
        errno = EINVAL;
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: runtime directory is required");
        return NULL;
    }

    struct stat runtime_directory_stat;
    if(stat(runtime_directory, &runtime_directory_stat) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: runtime directory '%s' is not accessible", runtime_directory);
        return NULL;
    }

    if(!S_ISDIR(runtime_directory_stat.st_mode)) {
        errno = ENOTDIR;
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: runtime directory '%s' is not a directory", runtime_directory);
        return NULL;
    }

    if(runtime_directory_stat.st_mode & S_IWOTH) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: runtime directory '%s' is publicly writable", runtime_directory);
        errno = EACCES;
        return NULL;
    }

    if(access(runtime_directory, W_OK | X_OK) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: runtime directory '%s' is not writable or searchable", runtime_directory);
        return NULL;
    }

    SPAWN_SERVER *server = callocz(1, sizeof(SPAWN_SERVER));
    char **isolated_child_environment = NULL;
    server->pipe[0] = -1;
    server->pipe[1] = -1;
    server->sock = -1;
    server->cb = child_callback;
    server->argc = argc;
    server->argv = argv;
    server->options = options;
    server->environment = environment;
    server->id = __atomic_add_fetch(&spawn_server_id, 1, __ATOMIC_RELAXED);
    os_uuid_generate_random(server->magic.uuid);

    char path[1024];
    int path_length;
    const size_t max_path_length = spawn_server_max_unix_socket_path_length();
    if(name && *name) {
        server->name = strdupz(name);
        path_length = snprintf(path, sizeof(path), "%s/netdata-spawn-%s.sock", runtime_directory, name);
    }
    else {
        server->name = strdupz("unnamed");
        path_length = snprintf(path, sizeof(path), "%s/netdata-spawn-%d-%zu.sock", runtime_directory, getpid(), server->id);
    }

    if(path_length < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: failed to generate socket path for '%s'",
               server->name);
        goto cleanup;
    }

    if((size_t)path_length >= sizeof(path)) {
        errno = ENAMETOOLONG;
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: socket path for '%s' in runtime directory '%s' was truncated (needed %d chars plus NUL, buffer is %zu bytes)",
               server->name, runtime_directory, path_length, sizeof(path));
        goto cleanup;
    }

    if((size_t)path_length > max_path_length) {
        errno = ENAMETOOLONG;
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: socket path for '%s' in runtime directory '%s' exceeds the %zu-byte AF_UNIX limit",
               server->name, runtime_directory, max_path_length);
        goto cleanup;
    }

    server->path = strdupz(path);

    if (!spawn_server_create_listening_socket(server))
        goto cleanup;

    if (pipe(server->pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Cannot create status pipe()");
        goto cleanup;
    }

    if(environment != nd_environment_process()) {
        isolated_child_environment = environment_vector_dup(environment);
        if(!isolated_child_environment) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN SERVER: cannot snapshot the isolated server environment");
            goto cleanup;
        }
    }

    pid_t pid = fork();
    if (pid == 0) {
        // the child - the spawn server

        int environment_rc = isolated_child_environment ?
            nd_environment_fork_child_replace((const char *const *)isolated_child_environment) :
            nd_environment_fork_child_reset_from_native();
        environment_vector_free(isolated_child_environment);
        isolated_child_environment = NULL;
        if(environment_rc != 0)
            _exit(EXIT_FAILURE);

        char buf[16];
        snprintfz(buf, sizeof(buf), "spawn-%s", server->name);
        os_setproctitle(buf, server->argc, server->argv);

        replace_stdio_with_dev_null();

        if(nd_log_collectors_fd() != STDERR_FILENO)
            dup2(nd_log_collectors_fd(), STDERR_FILENO);

        int fds_to_keep[] = {
            server->sock,
            server->pipe[1],
            nd_log_systemd_journal_fd(),
        };
        os_close_all_non_std_open_fds_except(fds_to_keep, _countof(fds_to_keep), 0);
        nd_log_reopen_log_files_for_spawn_server(buf);
        if(nd_environment_freeze_process() != 0)
            _exit(EXIT_FAILURE);
        _exit(spawn_server_event_loop(server));
    }
    else if (pid > 0) {
        // the parent
        environment_vector_free(isolated_child_environment);
        isolated_child_environment = NULL;
        server->server_pid = pid;
        close(server->sock); server->sock = -1;
        close(server->pipe[1]); server->pipe[1] = -1;

        struct status_report sr = { 0 };
        if (read(server->pipe[0], &sr, sizeof(sr)) != sizeof(sr)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: cannot read() initial status report from spawn server");
            goto cleanup;
        }

        if(sr.status != STATUS_REPORT_STARTED) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: server did not respond with success.");
            goto cleanup;
        }

        if(sr.started.pid != server->server_pid) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: server sent pid %d but we have created %d.", sr.started.pid, server->server_pid);
            goto cleanup;
        }

        nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "SPAWN SERVER: server created on pid %d", server->server_pid);

        return server;
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Cannot fork()");

cleanup:
    environment_vector_free(isolated_child_environment);
    spawn_server_destroy(server);
    return NULL;
}

// --------------------------------------------------------------------------------------------------------------------
// creating spawn server instances

void spawn_server_exec_destroy(SPAWN_INSTANCE *instance) {
    if(instance->child_pid) kill(instance->child_pid, SIGTERM);
    if(instance->write_fd != -1) close(instance->write_fd);
    if(instance->read_fd != -1) close(instance->read_fd);
    if(instance->sock != -1) close(instance->sock);
    freez(instance);
}

static void log_invalid_magic(SPAWN_INSTANCE *instance, struct status_report *sr) {
    unsigned char buf[sizeof(*sr) + 1];
    memcpy(buf, sr, sizeof(*sr));
    buf[sizeof(buf) - 1] = '\0';

    for(size_t i = 0; i < sizeof(buf) - 1; i++) {
        if (iscntrl(buf[i]) || !isprint(buf[i]))
            buf[i] = '_';
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "SPAWN PARENT: invalid final status report for child %d, request %zu (invalid magic %#x in response, reads like '%s')",
           instance->child_pid, instance->request_id, sr->magic, buf);
}

SPAWN_TIMEDWAIT_RESULT spawn_server_exec_timedwait(SPAWN_SERVER *server, SPAWN_INSTANCE *instance, int timeout_ms, int *status) {
    if(!instance) { if(status) *status = -1; return SPAWN_TIMEDWAIT_EXITED; }

    // close the child pipes, to make it exit (same as spawn_server_exec_wait)
    if(instance->write_fd != -1) { close(instance->write_fd); instance->write_fd = -1; }
    if(instance->read_fd != -1) { close(instance->read_fd); instance->read_fd = -1; }

    // a non-positive timeout means "wait forever" to wait_on_socket_or_cancel_with_timeout();
    // this primitive must always be bounded, so clamp to a minimal positive slice.
    if(timeout_ms <= 0) timeout_ms = 1;

    // the spawn server sends the final status report on instance->sock when the child exits
    short revents = 0;
    NETDATA_SSL ssl = { 0 };
    int rc = wait_on_socket_or_cancel_with_timeout(&ssl, instance->sock, timeout_ms, POLLIN, &revents);
    if(rc == -1 /* thread cancelled */ || rc == 1 /* timeout */)
        // the child is still running; the caller decides whether to keep waiting or kill it
        return SPAWN_TIMEDWAIT_RUNNING;

    if(rc == 2 /* error on the socket */) {
        // the status channel to the spawn server is broken (the spawn server itself died). We
        // cannot confirm the child exited, so we must NOT resolve as EXITED and free the instance
        // (that could leak a still-alive child). But this is terminal, not a transient "still
        // running" state, so we must NOT report RUNNING either (a caller looping on RUNNING with a
        // 0/"wait forever" timeout would spin forever). Report ERROR: the caller keeps the instance
        // and reclaims it by killing it.
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: status socket error for request No %zu, pid %d",
               instance->request_id, instance->child_pid);
        return SPAWN_TIMEDWAIT_ERROR;
    }

    // rc == 0: the status report is ready to read; the blocking wait returns immediately now.
    int st = spawn_server_exec_wait(server, instance);
    if(status) *status = st;
    return SPAWN_TIMEDWAIT_EXITED;
}

int spawn_server_exec_wait(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *instance) {
    int rc = -1;

    // close the child pipes, to make it exit
    if(instance->write_fd != -1) { close(instance->write_fd); instance->write_fd = -1; }
    if(instance->read_fd != -1) { close(instance->read_fd); instance->read_fd = -1; }

    // get the result
    struct status_report sr = { 0 };
    if(read(instance->sock, &sr, sizeof(sr)) != sizeof(sr))
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN PARENT: failed to read final status report for child %d, request %zu",
            instance->child_pid, instance->request_id);

    else if(sr.magic != STATUS_REPORT_MAGIC)
        log_invalid_magic(instance, &sr);
    else {
        switch (sr.status) {
            case STATUS_REPORT_EXITED:
                rc = sr.exited.waitpid_status;
                break;

            case STATUS_REPORT_STARTED:
            case STATUS_REPORT_FAILED:
            default:
                errno = 0;
                nd_log(
                    NDLS_COLLECTORS, NDLP_ERR,
                    "SPAWN PARENT: invalid status report to exec spawn request %zu for pid %d (status = %u)",
                    instance->request_id, instance->child_pid, sr.status);
                break;
        }
    }

    instance->child_pid = 0;
    spawn_server_exec_destroy(instance);
    return rc;
}

int spawn_server_exec_kill(SPAWN_SERVER *server, SPAWN_INSTANCE *instance, int timeout_ms) {
    if(instance->write_fd != -1) { close(instance->write_fd); instance->write_fd = -1; }
    if(instance->read_fd != -1) { close(instance->read_fd); instance->read_fd = -1; }

    if(timeout_ms > 0) {
        short revents;
        NETDATA_SSL ssl = { 0 };
        wait_on_socket_or_cancel_with_timeout(&ssl, instance->sock, timeout_ms, POLLIN, &revents);
    }

    // kill the child, if it is still running
    if(instance->child_pid) {
        kill(instance->child_pid, SIGTERM);

        // wait a bounded grace for the child to exit after SIGTERM. NOTE: timeout_ms is already
        // consumed above as the pre-kill grace (voluntary exit before SIGTERM); the post-SIGTERM
        // grace uses the fixed default so the caller's grace is not applied twice.
        // No PID-reuse race on the RUNNING path: the spawn server reaps the child and only then
        // sends the status report that makes timedwait return EXITED, so a RUNNING result means
        // the child has not been reaped yet and its PID is still held.
        int status;
        if(spawn_server_exec_timedwait(server, instance, SPAWN_KILL_DEFAULT_GRACE_MS, &status) == SPAWN_TIMEDWAIT_EXITED)
            return status;

        // still not gone: force-kill, then wait another bounded grace. We must NOT fall through to
        // an unbounded blocking wait here - a child we cannot signal (e.g. SIGKILL returns EPERM)
        // would otherwise hang the caller (and shutdown) forever, the very thing this path prevents.
        if(kill(instance->child_pid, SIGKILL) != 0)
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "SPAWN PARENT: SIGKILL of pid %d failed for request No %zu", instance->child_pid, instance->request_id);

        if(spawn_server_exec_timedwait(server, instance, SPAWN_KILL_DEFAULT_GRACE_MS, &status) == SPAWN_TIMEDWAIT_EXITED)
            return status;

        // could not confirm the child exited within the bounded waits; reclaim the instance so we
        // neither leak it nor block. The spawn server reaps the child if/when it actually dies.
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: giving up waiting for pid %d after SIGKILL (request No %zu) - reclaiming",
               instance->child_pid, instance->request_id);
        instance->child_pid = 0; // already signalled; skip the SIGTERM in destroy
        spawn_server_exec_destroy(instance);
        return -1;
    }

    return spawn_server_exec_wait(server, instance);
}

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd, int custom_fd, const char **argv, const void *data, size_t data_size, SPAWN_INSTANCE_TYPE type) {
    if(!server) return NULL;

    int pipe_stdin[2] = { -1, -1 }, pipe_stdout[2] = { -1, -1 };

    SPAWN_INSTANCE *instance = callocz(1, sizeof(SPAWN_INSTANCE));
    instance->read_fd = -1;
    instance->write_fd = -1;

    instance->sock = connect_to_spawn_server(server->path, true);
    if(instance->sock == -1)
        goto cleanup;

    if (pipe(pipe_stdin) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: Cannot create stdin pipe()");
        goto cleanup;
    }

    if (pipe(pipe_stdout) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: Cannot create stdout pipe()");
        goto cleanup;
    }

    SPAWN_REQUEST request = {
        .request_id = __atomic_add_fetch(&server->request_id, 1, __ATOMIC_RELAXED),
        .sock = instance->sock,
        .fds = {
            [0] = pipe_stdin[0],
            [1] = pipe_stdout[1],
            [2] = stderr_fd,
            [3] = custom_fd,
        },
        .argv = argv,
        .data = data,
        .data_size = data_size,
        .type = type
    };

    if(!spawn_server_send_request(&server->magic, server->environment, &request))
        goto cleanup;

    close(pipe_stdin[0]); pipe_stdin[0] = -1;
    instance->write_fd = pipe_stdin[1]; pipe_stdin[1] = -1;

    close(pipe_stdout[1]); pipe_stdout[1] = -1;
    instance->read_fd = pipe_stdout[0]; pipe_stdout[0] = -1;

    // copy the request id to the instance
    instance->request_id = request.request_id;

    struct status_report sr = { 0 };
    if(read(instance->sock, &sr, sizeof(sr)) != sizeof(sr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN PARENT: Failed to exec spawn request %zu (cannot get initial status report)",
            request.request_id);
        goto cleanup;
    }

    if(sr.magic != STATUS_REPORT_MAGIC) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN PARENT: Failed to exec spawn request %zu (invalid magic %#x in response)",
            request.request_id, sr.magic);
        goto cleanup;
    }

    switch(sr.status) {
        case STATUS_REPORT_STARTED:
            instance->child_pid = sr.started.pid;
            return instance;

        case STATUS_REPORT_FAILED:
            errno = sr.failed.err_no;
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                "SPAWN PARENT: Failed to exec spawn request %zu (server reports failure, errno is updated)",
                request.request_id);
            errno = 0;
            break;

        case STATUS_REPORT_EXITED:
            errno = ENOEXEC;
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                "SPAWN PARENT: Failed to exec spawn request %zu (server reports exit, errno is updated)",
                request.request_id);
            errno = 0;
            break;

        default:
            errno = 0;
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                "SPAWN PARENT: Invalid status report to exec spawn request %zu (received invalid data)",
                request.request_id);
            break;
    }

cleanup:
    if (pipe_stdin[0] >= 0) close(pipe_stdin[0]);
    if (pipe_stdin[1] >= 0) close(pipe_stdin[1]);
    if (pipe_stdout[0] >= 0) close(pipe_stdout[0]);
    if (pipe_stdout[1] >= 0) close(pipe_stdout[1]);
    spawn_server_exec_destroy(instance);
    return NULL;
}

#endif
