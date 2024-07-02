// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#include "spawn_server.h"

static size_t spawn_server_id = 0;

// --------------------------------------------------------------------------------------------------------------------

static void set_process_name(SPAWN_SERVER *server, const char *comm __maybe_unused) {

#ifdef HAVE_SYS_PRCTL_H
    // Set the process name (comm)
    prctl(PR_SET_NAME, comm, 0, 0, 0);
#endif

    if(server->argv && server->argv[0] && server->argv0_size) {
        size_t len = strlen(comm);
        strncpyz(server->argv[0], comm, MIN(server->argv0_size, len));
        while(len < server->argv0_size)
            server->argv[0][len++] = ' ';
    }
}

// --------------------------------------------------------------------------------------------------------------------
// the child created by the spawn server

#define STATUS_REPORT_OK     0x0C0FFEE0
#define STATUS_REPORT_FAILED 0xDEADBEEF

struct status_report {
    uint32_t status;
    union {
        pid_t pid;
        int err_no;
    };
};

static void spawn_server_send_status_success(int fd) {
    const struct status_report sr = {
        .status = STATUS_REPORT_OK,
        .pid = getpid(),
    };

    if(write(fd, &sr, sizeof(sr)) != sizeof(sr))
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: Cannot send initialize status report");
}

static void spawn_server_send_status_failure(int fd) {
    struct status_report sr = {
        .status = STATUS_REPORT_FAILED,
        .err_no = errno,
    };

    if(write(fd, &sr, sizeof(sr)) != sizeof(sr))
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: Cannot send initialize status report");
}

static void spawn_server_run_child(SPAWN_SERVER *server, SPAWN_REQUEST *request) {
    fprintf(stderr, "CHILD: running request %zu on pid %d\n", request->request_id, getpid());

    int stdin_fd = request->fds[0];
    int stdout_fd = request->fds[1];
    int stderr_fd = request->fds[2];
    int custom_fd = request->fds[3]; (void)custom_fd;

    if (dup2(stdin_fd, STDIN_FILENO) == -1) {
        spawn_server_send_status_failure(stdout_fd);
        exit(1);
    }
    if (dup2(stdout_fd, STDOUT_FILENO) == -1) {
        spawn_server_send_status_failure(stdout_fd);
        exit(1);
    }
    if (dup2(stderr_fd, STDERR_FILENO) == -1) {
        spawn_server_send_status_failure(stdout_fd);
        exit(1);
    }

    close(stdin_fd); stdin_fd = request->fds[0] = STDIN_FILENO;
    close(stdout_fd); stdout_fd = request->fds[1] = STDOUT_FILENO;
    close(stderr_fd); stderr_fd = request->fds[2] = STDERR_FILENO;

    environ = (char **)request->environment;

    // Perform different actions based on the type
    switch (request->type) {

        case SPAWN_INSTANCE_TYPE_EXEC:
            spawn_server_send_status_success(stdout_fd);
            execvp(request->argv[0], (char **)request->argv);
            exit(1);
            break;

        case SPAWN_INSTANCE_TYPE_CALLBACK:
            if(server->cb == NULL) {
                errno = ENOENT;
                spawn_server_send_status_failure(stdout_fd);
                exit(1);
            }
            spawn_server_send_status_success(stdout_fd);
            server->cb(request);
            exit(0);
            break;

        default:
            nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: unknown request type %u", request->type);
            exit(1);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// Encoding and decoding of spawn server request argv type of data

// Function to encode argv or envp
static void* encode_argv(const char **argv, size_t *out_size) {
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
static const char** decode_argv(const char *buffer, size_t size) {
    size_t count = 0;
    const char *ptr = buffer;
    while (ptr < buffer + size) {
        count++;
        ptr += strlen(ptr) + 1;
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
// Sending and receiving requests

static bool spawn_server_send_request_unsafe(SPAWN_REQUEST *request) {
    bool ret = false;

    size_t env_size = 0;
    void *encoded_env = encode_argv(request->environment, &env_size);
    if (!encoded_env)
        goto cleanup;

    size_t argv_size = 0;
    void *encoded_argv = encode_argv(request->argv, &argv_size);
    if (!encoded_argv)
        goto cleanup;

    struct msghdr msg = {0};
    struct cmsghdr *cmsg;
    char buf[1] = {0};
    char cmsgbuf[CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)];
    struct iovec iov[10];


    // We send 1 request with 10 iovec in it
    // The request will be received in 2 parts
    // 1. the first 6 iovec which include the sizes of the memory allocations required
    // 2. the last 4 iovec which require the memory allocations to be received

    iov[0].iov_base = buf;
    iov[0].iov_len = 1;

    iov[1].iov_base = &request->request_id;
    iov[1].iov_len = sizeof(request->request_id);

    iov[2].iov_base = &env_size;
    iov[2].iov_len = sizeof(env_size);

    iov[3].iov_base = &argv_size;
    iov[3].iov_len = sizeof(argv_size);

    iov[4].iov_base = &request->data_size;
    iov[4].iov_len = sizeof(request->data_size);

    iov[5].iov_base = &request->type;  // Added this line
    iov[5].iov_len = sizeof(request->type);

    iov[6].iov_base = encoded_env;
    iov[6].iov_len = env_size;

    iov[7].iov_base = encoded_argv;
    iov[7].iov_len = argv_size;

    iov[8].iov_base = (char *)request->data;
    iov[8].iov_len = request->data_size;

    iov[9].iov_base = NULL;
    iov[9].iov_len = 0;

    msg.msg_iov = iov;
    msg.msg_iovlen = 10;
    msg.msg_control = cmsgbuf;
    msg.msg_controllen = CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS);

    cmsg = CMSG_FIRSTHDR(&msg);
    cmsg->cmsg_level = SOL_SOCKET;
    cmsg->cmsg_type = SCM_RIGHTS;
    cmsg->cmsg_len = CMSG_LEN(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS);

    memcpy(CMSG_DATA(cmsg), request->fds, sizeof(int) * SPAWN_SERVER_TRANSFER_FDS);

    int rc = sendmsg(request->socket, &msg, 0);

    if (rc < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN PARENT: Failed to sendmsg() request to spawn server.");
        goto cleanup;
    }
    else {
        ret = true;
        fprintf(stderr, "PARENT: sent request %zu on socket %d (fds: %d, %d, %d, %d) from pid %d\n",
            request->request_id, request->socket, request->fds[0], request->fds[1], request->fds[2], request->fds[3], gettid());
    }

cleanup:
    freez(encoded_env);
    freez(encoded_argv);
    return ret;
}

static void spawn_server_receive_request(int sock, SPAWN_SERVER *server) {
    struct msghdr msg = {0};
    struct iovec iov[6];  // Increased size to 6
    char buf[1];
    size_t request_id;
    size_t env_size;
    size_t argv_size;
    size_t data_size;
    SPAWN_INSTANCE_TYPE type;  // Added this line
    char cmsgbuf[CMSG_SPACE(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)];
    char *envp = NULL, *argv = NULL, *data = NULL;
    int stdin_fd = -1, stdout_fd = -1, stderr_fd = -1, custom_fd = -1;

    // First recvmsg() to read sizes and control message
    iov[0].iov_base = buf;
    iov[0].iov_len = 1;
    iov[1].iov_base = &request_id;
    iov[1].iov_len = sizeof(request_id);
    iov[2].iov_base = &env_size;
    iov[2].iov_len = sizeof(env_size);
    iov[3].iov_base = &argv_size;
    iov[3].iov_len = sizeof(argv_size);
    iov[4].iov_base = &data_size;
    iov[4].iov_len = sizeof(data_size);
    iov[5].iov_base = &type;  // Added this line
    iov[5].iov_len = sizeof(type);

    msg.msg_iov = iov;
    msg.msg_iovlen = 6;
    msg.msg_control = cmsgbuf;
    msg.msg_controllen = sizeof(cmsgbuf);

    if (recvmsg(sock, &msg, 0) < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: failed to recvmsg() the first part of the request.");
        return;
    }

    // Extract file descriptors from control message
    struct cmsghdr *cmsg = CMSG_FIRSTHDR(&msg);
    if (cmsg == NULL || cmsg->cmsg_len != CMSG_LEN(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
            "SPAWN SERVER: Received invalid control message (expected %zu bytes, received %zu bytes)",
            CMSG_LEN(sizeof(int) * SPAWN_SERVER_TRANSFER_FDS), cmsg?cmsg->cmsg_len:0);
        return;
    }

    if (cmsg->cmsg_level != SOL_SOCKET || cmsg->cmsg_type != SCM_RIGHTS) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: Received unexpected control message type.");
        return;
    }

    int *fds = (int *)CMSG_DATA(cmsg);
    stdin_fd = fds[0];
    stdout_fd = fds[1];
    stderr_fd = fds[2];
    custom_fd = fds[3];

    if (stdin_fd < 0 || stdout_fd < 0 || stderr_fd < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
            "SPAWN SERVER: invalid file descriptors received, stdin = %d, stdout = %d, stderr = %d",
            stdin_fd, stdout_fd, stderr_fd);
        goto cleanup;
    }

    // Second recvmsg() to read buffer contents
    iov[0].iov_base = envp = mallocz(env_size);
    iov[0].iov_len = env_size;
    iov[1].iov_base = argv = mallocz(argv_size);
    iov[1].iov_len = argv_size;
    iov[2].iov_base = data = mallocz(data_size);
    iov[2].iov_len = data_size;

    msg.msg_iov = iov;
    msg.msg_iovlen = 3;
    msg.msg_control = NULL;
    msg.msg_controllen = 0;

    ssize_t total_bytes_received = recvmsg(sock, &msg, 0);
    if (total_bytes_received < 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: failed to recvmsg() the second part of the request.");
        goto cleanup;
    }

    fprintf(stderr, "SPAWN SERVER: received request %zu (fds: %d, %d, %d, %d)\n", request_id,
        stdin_fd, stdout_fd, stderr_fd, custom_fd);

    pid_t pid = fork();
    if (pid == 0) {
        // In child process
        close(server->server_sock); server->server_sock = -1;

        {
            char buf[15];
            snprintfz(buf, sizeof(buf), "chld-%zu-r%zu", server->id, request_id);
            set_process_name(server, buf);
        }

        SPAWN_REQUEST request = {
            .request_id = request_id,
            .socket = sock,
            .fds = {
                [0] = stdin_fd,
                [1] = stdout_fd,
                [2] = stderr_fd,
                [3] = custom_fd,
            },
            .environment = decode_argv(envp, env_size),
            .argv = decode_argv(argv, argv_size),
            .data = data,
            .data_size = data_size,
            .type = type
        };

        spawn_server_run_child(server, &request);
        exit(1);

    }
    else if (pid > 0) {
        // the parent
        // the child will send success to the parent
        ;
    }
    else {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: Failed to fork() child.");
        spawn_server_send_status_failure(stdout_fd);
    }

cleanup:
    if(stdin_fd != -1) close(stdin_fd);
    if(stdout_fd != -1) close(stdout_fd);
    if(stderr_fd != -1) close(stderr_fd);
    if(custom_fd != -1) close(custom_fd);
    freez(envp);
    freez(argv);
    freez(data);
}

// --------------------------------------------------------------------------------------------------------------------
// the spawn server main event loop

static void spawn_server_sigchld_handler(int signo __maybe_unused) {
    int status;
    pid_t pid;

    // Loop to check for exited child processes
    while ((pid = waitpid((pid_t)(-1), &status, WNOHANG)) != -1) {
        if (pid > 0) {
            if (WIFEXITED(status))
                fprintf(stderr, "Child %d exited with status %d\n", pid, WEXITSTATUS(status));

            else if (WIFSIGNALED(status))
                fprintf(stderr, "Child %d killed by signal %d\n", pid, WTERMSIG(status));
        }
    }
}

static void spawn_server_event_loop(SPAWN_SERVER *server) {
    int pipe_fd = server->pipe[1];
    close(server->pipe[0]);

    // Set up the signal handler for SIGCHLD
    struct sigaction sa;
    sa.sa_handler = spawn_server_sigchld_handler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = SA_RESTART | SA_NOCLDSTOP;
    if (sigaction(SIGCHLD, &sa, NULL) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: sigaction() failed");
        exit(1);
    }

    struct status_report sr = {
        .status = STATUS_REPORT_OK,
        .pid = getpid(),
    };
    if(write(pipe_fd, &sr, sizeof(sr)) != sizeof(sr)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: failed to write initial status report.");
        exit(1);
    }
    close(pipe_fd); pipe_fd = server->pipe[1] = -1;

    while (1) {
        int client_sock = accept(server->server_sock, NULL, NULL);
        if (client_sock == -1) {
            if (errno == EINTR)
                continue;
            else {
                nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: accept() failed");
                continue;
            }
        }

        spawn_server_receive_request(client_sock, server);
        close(client_sock);
    }

    exit(1);
}

// --------------------------------------------------------------------------------------------------------------------
// management of the spawn server

void spawn_server_destroy(SPAWN_SERVER *server) {
    if(server->pipe[0] != -1) close(server->pipe[0]);
    if(server->pipe[1] != -1) close(server->pipe[1]);
    if(server->server_sock != -1) close(server->server_sock);

    if(server->path) {
        unlink(server->path);
        free(server->path);
    }

    if(server->server_pid) {
        kill(server->server_pid, SIGTERM);
        waitpid(server->server_pid, NULL, 0);
    }

    free(server);
}

static bool spawn_server_create_listening_socket(SPAWN_SERVER *server) {
    char path[256];
    server->id = __atomic_add_fetch(&spawn_server_id, 1, __ATOMIC_RELAXED);
    snprintf(path, sizeof(path), "/tmp/.netdata-spawn-server-%d-%zu.sock", getpid(), server->id);
    server->path = strdup(path);

    if ((server->server_sock = socket(AF_UNIX, SOCK_STREAM, 0)) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to create socket()");
        return false;
    }

    struct sockaddr_un server_addr = {
        .sun_family = AF_UNIX,
    };
    strcpy(server_addr.sun_path, server->path);
    unlink(server->path);
    errno = 0;

    if (bind(server->server_sock, (struct sockaddr *)&server_addr, sizeof(server_addr)) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to bind()");
        return false;
    }

    if (listen(server->server_sock, 5) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to listen()");
        return false;
    }

    return true;
}

SPAWN_SERVER* spawn_server_create(spawn_request_callback_t child_callback, int argc, char **argv) {
    SPAWN_SERVER *server = callocz(1, sizeof(SPAWN_SERVER));
    server->pipe[0] = -1;
    server->pipe[1] = -1;
    server->server_sock = -1;
    server->cb = child_callback;
    server->argv = argv;
    server->argv0_size = (argv && argv[0]) ? strlen(argv[0]) : 0;

    if (!spawn_server_create_listening_socket(server))
        goto cleanup;

    if (pipe(server->pipe) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot create status pipe");
        goto cleanup;
    }

    pid_t pid = fork();
    if (pid == 0) {
        // the child - the spawn server
        if(argc && argv) {
            for(int i = 1; i < argc ;i++)
                argv[i][0] = '\0';
        }

        {
            char buf[15];
            snprintfz(buf, sizeof(buf), "spawn-server-%zu", server->id);
            set_process_name(server, buf);
        }
        spawn_server_event_loop(server);
    }
    else if (pid > 0) {
        // the parent
        server->server_pid = pid;
        close(server->server_sock); server->server_sock = -1;
        close(server->pipe[1]); server->pipe[1] = -1;

        struct status_report sr = { 0 };
        if (read(server->pipe[0], &sr, sizeof(sr)) != sizeof(sr)) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: cannot read() initial status report from spawn server");
            goto cleanup;
        }
        close(server->pipe[0]); server->pipe[0] = -1;

        if(sr.status != STATUS_REPORT_OK) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: server did not respond with success.");
            goto cleanup;
        }

        if(sr.pid != server->server_pid) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: server sent pid %d but we have created %d.", sr.pid, server->server_pid);
            goto cleanup;
        }

        // Initialize the mutex
        spinlock_init(&server->spinlock);
        return server;
    }

    nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot fork()");

cleanup:
    spawn_server_destroy(server);
    return NULL;
}

// --------------------------------------------------------------------------------------------------------------------
// creating spawn server instances

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd, int custom_fd, const char **argv, const void *data, size_t data_size, SPAWN_INSTANCE_TYPE type) {
    int pipe_stdin[2] = { -1, -1 }, pipe_stdout[2] = { -1, -1 };

    SPAWN_INSTANCE *instance = callocz(1, sizeof(SPAWN_INSTANCE));
    instance->read_fd = -1;
    instance->write_fd = -1;

    // Initialize the client socket and connect to the server
    if ((instance->client_sock = socket(AF_UNIX, SOCK_STREAM, 0)) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: cannot create socket() to connect to spawn server.");
        goto cleanup;
    }

    struct sockaddr_un server_addr = {
        .sun_family = AF_UNIX,
    };
    strcpy(server_addr.sun_path, server->path);

    if (connect(instance->client_sock, (struct sockaddr *)&server_addr, sizeof(server_addr)) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "SPAWN SERVER: Cannot connect() to spawn server.");
        goto cleanup;
    }

    if (pipe(pipe_stdin) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot create stdin pipe()");
        goto cleanup;
    }

    if (pipe(pipe_stdout) == -1) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot create stdout pipe()");
        goto cleanup;
    }

    spinlock_lock(&server->spinlock);

    SPAWN_REQUEST request = {
        .request_id = ++server->request_id,
        .socket = instance->client_sock,
        .fds = {
            [0] = pipe_stdin[0],
            [1] = pipe_stdout[1],
            [2] = stderr_fd,
            [3] = custom_fd,
        },
        .environment = (const char **)environ,
        .argv = argv,
        .data = data,
        .data_size = data_size,
        .type = type
    };

    bool sent = spawn_server_send_request_unsafe(&request);

    spinlock_unlock(&server->spinlock);

    if (!sent)
        goto cleanup;

    close(pipe_stdin[0]); pipe_stdin[0] = -1;
    instance->write_fd = pipe_stdin[1];

    close(pipe_stdout[1]); pipe_stdout[1] = -1;
    instance->read_fd = pipe_stdout[0];

    struct status_report sr = { 0 };
    if(read(instance->read_fd, &sr, sizeof(sr)) != sizeof(sr)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to exec spawn request %zu (cannot read child stdout)", request.request_id);
        goto cleanup;
    }

    if(sr.status != STATUS_REPORT_OK && sr.status != STATUS_REPORT_FAILED) {
        errno = 0;
        nd_log(NDLS_DAEMON, NDLP_ERR, "Invalid status report to exec spawn request %zu (received invalid data)", request.request_id);
        goto cleanup;
    }

    if(sr.status == STATUS_REPORT_FAILED) {
        errno = sr.err_no;
        nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to exec spawn request %zu (check errno)", request.request_id);
        errno = 0;
        goto cleanup;
    }

    instance->child_pid = sr.pid;
    return instance;

cleanup:
    if (pipe_stdin[0] >= 0) close(pipe_stdin[0]);
    if (pipe_stdin[1] >= 0) close(pipe_stdin[1]);
    if (pipe_stdout[0] >= 0) close(pipe_stdout[0]);
    if (pipe_stdout[1] >= 0) close(pipe_stdout[1]);
    freez(instance);
    return NULL;
}

void spawn_server_stop(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *instance) {
    if(instance->child_pid) kill(instance->child_pid, SIGTERM);
    if(instance->write_fd != -1) close(instance->write_fd);
    if(instance->read_fd != -1) close(instance->read_fd);
    if(instance->client_sock != -1) close(instance->client_sock);

    freez(instance);
}
