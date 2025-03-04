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

#ifdef __APPLE__
#include <crt_externs.h>
#define environ (*_NSGetEnviron())
#else
extern char **environ;
#endif

static size_t spawn_server_id = 0;
static volatile bool spawn_server_exit = false;
static volatile bool spawn_server_sigchld = false;
static SPAWN_REQUEST *spawn_server_requests = NULL;

// --------------------------------------------------------------------------------------------------------------------

static int connect_to_spawn_server(const char *path, bool log) {
    int sock = -1;

    if ((sock = socket(AF_UNIX, SOCK_STREAM, 0)) == -1) {
        if(log)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: cannot create socket() to connect to spawn server.");
        return -1;
    }

    struct sockaddr_un server_addr = {
        .sun_family = AF_UNIX,
    };
    strcpy(server_addr.sun_path, path);

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
static void* argv_encode(const char **argv, size_t *out_size) {
    size_t buffer_size = 1024; // Initial buffer size
    size_t buffer_used = 0;
    char *buffer = mallocz(buffer_size);

    if(argv) {
        for (const char **p = argv; *p != NULL; p++) {
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

// Function to decode argv or envp
static const char** argv_decode(const char *buffer, size_t size) {
    size_t count = 0;
    const char *ptr = buffer;
    while (ptr < buffer + size) {
        if(ptr && *ptr) {
            count++;
            ptr += strlen(ptr) + 1;
        }
        else
            break;
    }

    const char **argv = mallocz((count + 1) * sizeof(char *));

    ptr = buffer;
    for (size_t i = 0; i < count; i++) {
        argv[i] = ptr;
        ptr += strlen(ptr) + 1;
    }
    argv[count] = NULL; // Null-terminate the array

    return argv;
}

// --------------------------------------------------------------------------------------------------------------------
// status reports

typedef enum __attribute__((packed)) {
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

    pid_t pid = fork();
    if (pid < 0) {
        // fork failed

        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to fork() child for callback.");
        return false;
    }
    else if (pid == 0) {
        // the child

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

        // overwrite the process environment
        environ = (char **)rq->envp;

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

static bool spawn_server_is_running(const char *path) {
    struct msghdr msg = {0};
    struct iovec iov[7];
    SPAWN_SERVER_MSG msg_type = SPAWN_SERVER_MSG_PING;
    size_t dummy_size = 0;
    SPAWN_INSTANCE_TYPE dummy_type = 0;
    ND_UUID magic = UUID_ZERO;
    char cmsgbuf[CMSG_SPACE(sizeof(int))];

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

    msg.msg_iov = iov;
    msg.msg_iovlen = 7;
    msg.msg_control = cmsgbuf;
    msg.msg_controllen = sizeof(cmsgbuf);

    int sock = connect_to_spawn_server(path, false);
    if(sock == -1)
        return false;

    int rc = sendmsg(sock, &msg, 0);
    if (rc < 0) {
        // cannot send the message
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

static bool spawn_server_send_request(ND_UUID *magic, SPAWN_REQUEST *request) {
    bool ret = false;

    size_t env_size = 0;
    size_t argv_size = 0;

    void *encoded_env = argv_encode(request->envp, &env_size);
    void *encoded_argv = argv_encode(request->argv, &argv_size);

    struct msghdr msg = {0};
    struct cmsghdr *cmsg;
    SPAWN_SERVER_MSG msg_type = SPAWN_SERVER_MSG_REQUEST;
    char cmsgbuf[CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)];
    struct iovec iov[11];

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

    iov[10].iov_base = NULL;
    iov[10].iov_len = 0;

    msg.msg_iov = iov;
    msg.msg_iovlen = 11;
    msg.msg_control = cmsgbuf;
    msg.msg_controllen = CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS);

    cmsg = CMSG_FIRSTHDR(&msg);
    cmsg->cmsg_level = SOL_SOCKET;
    cmsg->cmsg_type = SCM_RIGHTS;
    cmsg->cmsg_len = CMSG_LEN(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS);

    memcpy(CMSG_DATA(cmsg), request->fds, sizeof(int) * SPAWN_SERVER_TRANSFER_FDS);

    int rc = sendmsg(request->sock, &msg, 0);

    if (rc < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: Failed to sendmsg() request to spawn server using socket %d.", request->sock);
        goto cleanup;
    }
    else {
        ret = true;
        // fprintf(stderr, "PARENT: sent request %zu on socket %d (fds: %d, %d, %d, %d) from tid %d\n",
        //     request->request_id, request->socket, request->fds[0], request->fds[1], request->fds[2], request->fds[3], os_gettid());
    }

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
    char cmsgbuf[CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)];
    char *envp_encoded = NULL, *argv_encoded = NULL, *data = NULL;
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

    if (recvmsg(sock, &msg, 0) < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: failed to recvmsg() the first part of the request.");
        close(sock);
        return;
    }

    if(msg_type == SPAWN_SERVER_MSG_PING) {
        spawn_server_send_status_ping(sock);
        close(sock);
        return;
    }

    if(!UUIDeq(magic, server->magic)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Invalid authorization key for request %zu. "
            "Rejecting request.",
            request_id);
        close(sock);
        return;
    }

    if(type == SPAWN_INSTANCE_TYPE_EXEC && !(server->options & SPAWN_SERVER_OPTION_EXEC)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Request %zu wants to exec, but exec is not allowed for this spawn server. "
            "Rejecting request.",
            request_id);
        close(sock);
        return;
    }

    if(type == SPAWN_INSTANCE_TYPE_CALLBACK && !(server->options & SPAWN_SERVER_OPTION_CALLBACK)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
            "SPAWN SERVER: Request %zu wants to run a callback, but callbacks are not allowed for this spawn server. "
            "Rejecting request.",
            request_id);
        close(sock);
        return;
    }

    // Extract file descriptors from control message
    struct cmsghdr *cmsg = CMSG_FIRSTHDR(&msg);
    if (cmsg == NULL || cmsg->cmsg_len != CMSG_LEN(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: Received invalid control message (expected %zu bytes, received %zu bytes)",
               (size_t)(CMSG_LEN(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)), (size_t)(cmsg?cmsg->cmsg_len:0));
        close(sock);
        return;
    }

    if (cmsg->cmsg_level != SOL_SOCKET || cmsg->cmsg_type != SCM_RIGHTS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Received unexpected control message type.");
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

    // Second recvmsg() to read buffer contents
    iov[0].iov_base = envp_encoded = mallocz(env_size);
    iov[0].iov_len = env_size;
    iov[1].iov_base = argv_encoded = mallocz(argv_size);
    iov[1].iov_len = argv_size;
    iov[2].iov_base = data = mallocz(data_size);
    iov[2].iov_len = data_size;

    msg.msg_iov = iov;
    msg.msg_iovlen = 3;
    msg.msg_control = NULL;
    msg.msg_controllen = 0;

    ssize_t total_bytes_received = recvmsg(sock, &msg, 0);
    if (total_bytes_received < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: failed to recvmsg() the second part of the request.");
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
        .envp = argv_decode(envp_encoded, env_size),
        .argv = argv_decode(argv_encoded, argv_size),
        .data = data,
        .data_size = data_size,
        .type = type
    };

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
        // nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: killing all children...");
        size_t killed = 0;
        for(SPAWN_REQUEST *rq = spawn_server_requests; rq ; rq = rq->next) {
            kill(rq->pid, SIGTERM);
            killed++;
        }
        while(spawn_server_requests) {
            spawn_server_process_sigchld();
            tinysleep();
        }
        // nd_log(NDLS_COLLECTORS, NDLP_INFO, "SPAWN SERVER: all %zu children finished", killed);
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

    if ((server->sock = socket(AF_UNIX, SOCK_STREAM, 0)) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Failed to create socket()");
        return false;
    }

    struct sockaddr_un server_addr = {
        .sun_family = AF_UNIX,
    };
    strcpy(server_addr.sun_path, server->path);
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

SPAWN_SERVER* spawn_server_create(SPAWN_SERVER_OPTIONS options, const char *name, spawn_request_callback_t child_callback, int argc, const char **argv) {
    SPAWN_SERVER *server = callocz(1, sizeof(SPAWN_SERVER));
    server->pipe[0] = -1;
    server->pipe[1] = -1;
    server->sock = -1;
    server->cb = child_callback;
    server->argc = argc;
    server->argv = argv;
    server->options = options;
    server->id = __atomic_add_fetch(&spawn_server_id, 1, __ATOMIC_RELAXED);
    os_uuid_generate_random(server->magic.uuid);

    const char *runtime_directory = getenv("NETDATA_RUN_DIR");
    if(!runtime_directory || !*runtime_directory)
        runtime_directory = os_run_dir(true);

    if (runtime_directory) {
        struct stat statbuf;

        if(!*runtime_directory)
            // it is empty
                runtime_directory = NULL;

        else if (stat(runtime_directory, &statbuf) == 0 && S_ISDIR(statbuf.st_mode)) {
            // it exists and it is a directory

            if (access(runtime_directory, W_OK) != 0) {
                // it is not writable by us
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "Runtime directory '%s' is not writable, falling back to '/tmp'", runtime_directory);
                runtime_directory = NULL;
            }
        }
        else {
            // it does not exist
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Runtime directory '%s' does not exist, falling back to '/tmp'", runtime_directory);
            runtime_directory = NULL;
        }
    }
    if(!runtime_directory)
        runtime_directory = "/tmp";

    char path[1024];
    if(name && *name) {
        server->name = strdupz(name);
        snprintf(path, sizeof(path), "%s/netdata-spawn-%s.sock", runtime_directory, name);
    }
    else {
        server->name = strdupz("unnamed");
        snprintf(path, sizeof(path), "%s/netdata-spawn-%d-%zu.sock", runtime_directory, getpid(), server->id);
    }

    server->path = strdupz(path);

    if (!spawn_server_create_listening_socket(server))
        goto cleanup;

    if (pipe(server->pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: Cannot create status pipe()");
        goto cleanup;
    }

    pid_t pid = fork();
    if (pid == 0) {
        // the child - the spawn server

        char buf[16];
        snprintfz(buf, sizeof(buf), "spawn-%s", server->name);
        os_setproctitle(buf, server->argc, server->argv);

        replace_stdio_with_dev_null();
        int fds_to_keep[] = {
            server->sock,
            server->pipe[1],
            nd_log_systemd_journal_fd(),
        };
        os_close_all_non_std_open_fds_except(fds_to_keep, _countof(fds_to_keep), 0);
        nd_log_reopen_log_files_for_spawn_server(buf);
        _exit(spawn_server_event_loop(server));
    }
    else if (pid > 0) {
        // the parent
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
    if(instance->child_pid)
        kill(instance->child_pid, SIGTERM);

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
        .envp = (const char **)environ,
        .argv = argv,
        .data = data,
        .data_size = data_size,
        .type = type
    };

    if(!spawn_server_send_request(&server->magic, &request))
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
