#include "libnetdata/libnetdata.h"
#include "spawn_server_internals.h"

#define ENV_VAR_KEY "SPAWN_TESTER"
#define ENV_VAR_VALUE "1234567890"

size_t warnings = 0;

void child_check_environment(void) {
    const char *s = getenv(ENV_VAR_KEY);
    if(!s || !*s || strcmp(s, ENV_VAR_VALUE) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Wrong environment. Variable '%s' should have value '%s' but it has '%s'",
               ENV_VAR_KEY, ENV_VAR_VALUE, s ? s : "(unset)");

        exit(1);
    }
}

static bool is_valid_fd(int fd) {
    errno_clear();
    return fcntl(fd, F_GETFD) != -1 || errno != EBADF;
}

void child_check_fds(void) {
    for(int fd = 0; fd < 3; fd++) {
        if(!is_valid_fd(fd)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "fd No %d should be a valid file descriptor - but it isn't.", fd);

            exit(1);
        }
    }

    for(int fd = 3; fd < /* os_get_fd_open_max() */ 1024; fd++) {
        if(is_valid_fd(fd)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "fd No %d is a valid file descriptor - it shouldn't.", fd);

            exit(1);
        }
    }

    errno_clear();
}

#if defined(SPAWN_SERVER_VERSION_NOFORK)
#define SPAWN_CONTROL_CONCURRENT_CALLERS 24

typedef struct {
    SPAWN_SERVER *server;
    const char *argv0;
    size_t index;
    int result;
    bool done;
} SPAWN_CONTROL_TEST_CALL;

static int plugin_echo_argument(int argc, const char **argv) {
    child_check_fds();
    child_check_environment();
    if(argc != 3)
        return 1;
    return fputs(argv[2], stdout) == EOF ? 1 : 0;
}

static void *test_control_concurrent_call(void *arg) {
    SPAWN_CONTROL_TEST_CALL *call = arg;
    char token[32], output[sizeof(token)] = { 0 };
    snprintfz(token, sizeof(token), "request-%zu", call->index);
    const char *argv[] = { call->argv0, "plugin-echo-argument", token, NULL };

    SPAWN_INSTANCE *si = spawn_server_exec(call->server, STDERR_FILENO, 0, argv, NULL, 0,
                                           SPAWN_INSTANCE_TYPE_EXEC);
    if(!si) {
        __atomic_store_n(&call->done, true, __ATOMIC_RELEASE);
        return NULL;
    }

    close(spawn_server_instance_write_fd(si));
    spawn_server_instance_write_fd_unset(si);
    size_t used = 0;
    ssize_t bytes;
    while(used < sizeof(output) &&
          (bytes = read(spawn_server_instance_read_fd(si), output + used, sizeof(output) - used)) > 0)
        used += (size_t)bytes;

    int status = spawn_server_exec_wait(call->server, si);
    call->result = used == strlen(token) && memcmp(output, token, used) == 0 &&
                   WIFEXITED(status) && WEXITSTATUS(status) == 0;
    __atomic_store_n(&call->done, true, __ATOMIC_RELEASE);
    return NULL;
}

// Stop the server and fill its control queue. A caller must wait for room
// instead of failing, and complete once the server runs again.
static void test_control_queue_full(SPAWN_SERVER *server, const char *argv0) {
    pid_t pid = spawn_server_pid(server);

#if defined(OS_LINUX)
    // A small send buffer bounds the queue. BSD and macOS bound it by the receive
    // buffer, and a tiny send buffer there would also cap the size of the
    // request's own datagram.
    int sndbuf = 0;
    socklen_t sndbuf_len = sizeof(sndbuf);
    int small_sndbuf = 1;
    if(getsockopt(server->control_fd, SOL_SOCKET, SO_SNDBUF, &sndbuf, &sndbuf_len) != 0 ||
       setsockopt(server->control_fd, SOL_SOCKET, SO_SNDBUF, &small_sndbuf, sizeof(small_sndbuf)) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot shrink the control send buffer");
        exit(1);
    }
#endif

    if(kill(pid, SIGSTOP) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot stop the spawn server");
        exit(1);
    }

    // The server logs and discards each filler, as they carry no descriptor, so
    // fill with 1 KiB datagrams (under the 2 KiB macOS limit) and top off with
    // smaller ones until not even 1 byte fits: the request must find no room.
    char filler[1024];
    memset(filler, 'F', sizeof(filler));
    size_t queued = 0;
    for(size_t size = sizeof(filler); size > 0; ) {
        if(queued > 100000) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Control queue did not fill");
            exit(1);
        }
        if(send(server->control_fd, filler, size, 0) == (ssize_t)size) {
            queued++;
            continue;
        }
        if(errno == EAGAIN || errno == EWOULDBLOCK || errno == ENOBUFS) {
            size /= 2;
            continue;
        }
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot fill the control queue");
        exit(1);
    }

    pthread_t thread;
    SPAWN_CONTROL_TEST_CALL call = { .server = server, .argv0 = argv0, .index = 1000 };
    if(pthread_create(&thread, NULL, test_control_concurrent_call, &call) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot start the control queue test thread");
        exit(1);
    }

    sleep_usec(300 * USEC_PER_MS);
    bool waited = !__atomic_load_n(&call.done, __ATOMIC_ACQUIRE);

    if(kill(pid, SIGCONT) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot resume the spawn server");
        exit(1);
    }
    pthread_join(thread, NULL);
#if defined(OS_LINUX)
    // Linux reports twice the value that was set, and doubles what it is given.
    sndbuf /= 2;
    if(setsockopt(server->control_fd, SOL_SOCKET, SO_SNDBUF, &sndbuf, sizeof(sndbuf)) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot restore the control send buffer");
        exit(1);
    }
#endif

    if(!waited || !call.result) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Request with a full control queue (%zu queued) %s", queued,
               waited ? "failed after the server resumed" : "did not wait for room");
        exit(1);
    }
}

static void test_private_control_channel(int argc, const char **argv) {
    SPAWN_SERVER *server = spawn_server_create(SPAWN_SERVER_OPTION_EXEC, "test-control", NULL, argc, argv);
    if(!server) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot create private-channel test server");
        exit(1);
    }

    // A forked descendant has a copy of the descriptor, but the public API
    // must reject it as a caller of this server.
    pid_t child = fork();
    if(child == 0) {
        const char *args[] = { argv[0], "plugin-echo-and-exit", NULL };
        SPAWN_INSTANCE *si = spawn_server_exec(server, STDERR_FILENO, 0, args, NULL, 0,
                                               SPAWN_INSTANCE_TYPE_EXEC);
        _exit(!si && errno == EPERM ? 0 : 1);
    }
    int status;
    if(child < 0 || waitpid(child, &status, 0) != child || !WIFEXITED(status) || WEXITSTATUS(status) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Forked child was allowed to submit a spawn request");
        spawn_server_destroy(server);
        exit(1);
    }

    // An extra transferred FD is invalid. The server must discard the entire
    // datagram, close its received copies, and remain able to process requests.
    char marker = 'R';
    int bad_fds[2] = { STDIN_FILENO, STDIN_FILENO };
    char control[CMSG_SPACE(sizeof(bad_fds))] __attribute__((aligned(sizeof(size_t))));
    struct iovec iov = { .iov_base = &marker, .iov_len = sizeof(marker) };
    struct msghdr msg = { .msg_iov = &iov, .msg_iovlen = 1,
                          .msg_control = control, .msg_controllen = sizeof(control) };
    struct cmsghdr *cmsg = CMSG_FIRSTHDR(&msg);
    cmsg->cmsg_level = SOL_SOCKET;
    cmsg->cmsg_type = SCM_RIGHTS;
    cmsg->cmsg_len = CMSG_LEN(sizeof(bad_fds));
    memcpy(CMSG_DATA(cmsg), bad_fds, sizeof(bad_fds));
    if(sendmsg(server->control_fd, &msg, 0) != sizeof(marker)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot send malformed control test message");
        spawn_server_destroy(server);
        exit(1);
    }

    test_control_queue_full(server, argv[0]);

    pthread_t threads[SPAWN_CONTROL_CONCURRENT_CALLERS];
    SPAWN_CONTROL_TEST_CALL calls[SPAWN_CONTROL_CONCURRENT_CALLERS] = { 0 };
    for(size_t i = 0; i < SPAWN_CONTROL_CONCURRENT_CALLERS; i++) {
        calls[i] = (SPAWN_CONTROL_TEST_CALL){ .server = server, .argv0 = argv[0], .index = i };
        if(pthread_create(&threads[i], NULL, test_control_concurrent_call, &calls[i]) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot start private-channel test thread %zu", i);
            exit(1);
        }
    }
    for(size_t i = 0; i < SPAWN_CONTROL_CONCURRENT_CALLERS; i++) {
        pthread_join(threads[i], NULL);
        if(!calls[i].result) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Private-channel request %zu failed or received another result", i);
            exit(1);
        }
    }

    // Requests must fail promptly when the forked server is already gone.
    pid_t server_pid = spawn_server_pid(server);
    if(kill(server_pid, SIGKILL) != 0 || waitpid(server_pid, &status, 0) != server_pid) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot stop private-channel test server");
        exit(1);
    }
    server->server_pid = 0; // already reaped; destroy must not signal a recycled PID
    const char *args[] = { argv[0], "plugin-echo-and-exit", NULL };
    SPAWN_INSTANCE *si = spawn_server_exec(server, STDERR_FILENO, 0, args, NULL, 0,
                                           SPAWN_INSTANCE_TYPE_EXEC);
    if(si) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Spawn request succeeded after server death");
        spawn_server_exec_wait(server, si);
        exit(1);
    }

    spawn_server_destroy(server);
}
#endif

// --------------------------------------------------------------------------------------------------------------------

static void test_int_fds_echo_loop(SPAWN_INSTANCE *si, const char *msg, size_t iterations) {
    if(!msg || !*msg) return;

    const size_t max_msg_len = (size_t)(SSIZE_MAX / 2);
    size_t ulen = strnlen(msg, max_msg_len + 1);
    if(unlikely(ulen > max_msg_len))
        return;

    ssize_t len = (ssize_t)ulen;
    size_t buffer_size = ulen * 2;
    CLEAN_CHAR_P *buffer = mallocz(buffer_size);

    for(size_t j = 0; j < iterations; j++) {
        fprintf(stderr, "-");
        memset(buffer, 0, buffer_size);

        ssize_t rc = write(spawn_server_instance_write_fd(si), msg, len);
        if (rc != len) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot write to plugin. Expected to write %zd bytes, wrote %zd bytes",
                   len, rc);
            exit(1);
        }

        rc = read(spawn_server_instance_read_fd(si), buffer, buffer_size);
        if (rc != len) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot read from plugin. Expected to read %zd bytes, read %zd bytes",
                   len, rc);
            exit(1);
        }

        if (memcmp(msg, buffer, len) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Read corrupted data. Expected '%s', Read '%s'",
                   msg, buffer);
            exit(1);
        }
    }
    fprintf(stderr, "\n");
}

static void test_popen_echo_loop(POPEN_INSTANCE *pi, const char *msg, size_t iterations) {
    if(!msg || !*msg) return;

    const size_t max_msg_len =
        ((size_t)(INT_MAX / 2) < (size_t)(SSIZE_MAX / 2)) ? (size_t)(INT_MAX / 2) : (size_t)(SSIZE_MAX / 2);
    size_t len = strnlen(msg, max_msg_len + 1);
    if(unlikely(len > max_msg_len))
        return;

    size_t buffer_size = len * 2;
    CLEAN_CHAR_P *buffer = mallocz(buffer_size);

    FILE *child_stdin = spawn_popen_stdin(pi);
    FILE *child_stdout = child_stdin ? spawn_popen_stdout(pi) : NULL;
    if(!child_stdin || !child_stdout) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open popen child streams");
        spawn_popen_kill(pi, 0);
        netdata_main_spawn_server_cleanup();
        exit(1);
    }

    for(size_t j = 0; j < iterations; j++) {
        fprintf(stderr, "-");
        memset(buffer, 0, buffer_size);

        size_t rc = fwrite(msg, 1, len, child_stdin);
        if (rc != len) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot write to plugin. Expected to write %zu bytes, wrote %zu bytes",
                   len, rc);
            exit(1);
        }
        if(fflush(child_stdin) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot flush to plugin: %s", strerror(errno));
            exit(1);
        }

        char *s = fgets(buffer, (int)buffer_size, child_stdout);
        if (!s || strlen(s) != len) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot read from plugin. Expected to read %zu bytes, read %zu bytes",
                   len, (size_t)(s ? strlen(s) : 0));
            exit(1);
        }
        if (memcmp(msg, buffer, len) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Read corrupted data. Expected '%s', Read '%s'",
                   msg, buffer);
            exit(1);
        }
    }
    fprintf(stderr, "\n");
}

// --------------------------------------------------------------------------------------------------------------------
// kill to stop

int plugin_kill_to_stop() {
    child_check_fds();
    child_check_environment();

    char buffer[1024];
    while (fgets(buffer, sizeof(buffer), stdin) != NULL) {
        fprintf(stderr, "+");
        printf("%s", buffer);
        fflush(stdout);
    }

    return 0;
}

void test_int_fds_plugin_kill_to_stop(SPAWN_SERVER *server, const char *argv0) {
    const char *params[] = {
        argv0,
        "plugin-kill-to-stop",
        NULL,
    };

    SPAWN_INSTANCE *si = spawn_server_exec(server, STDERR_FILENO, 0, params, NULL, 0, SPAWN_INSTANCE_TYPE_EXEC);
    if(!si) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (spawn)");
        exit(1);
    }

    test_int_fds_echo_loop(si, "Hello World!\n", 30);

    int code = spawn_server_exec_kill(server, si, 0);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child exited with code %d",
           code);

    if(code != 15 && code != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "child should exit with code 0 or 15, but exited with code %d", code);
        warnings++;
    }
}

void test_popen_plugin_kill_to_stop(const char *argv0) {
    char cmd[FILENAME_MAX + 100];
    snprintfz(cmd, sizeof(cmd), "exec %s plugin-kill-to-stop", argv0);
    POPEN_INSTANCE *pi = spawn_popen_run(cmd);
    if(!pi) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (popen)");
        exit(1);
    }

    test_popen_echo_loop(pi, "Hello World!\n", 30);

    int code = spawn_popen_kill(pi, 0);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child exited with code %d",
           code);

    if(code != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "child should exit with code 0, but exited with code %d", code);
        warnings++;
    }
}

// --------------------------------------------------------------------------------------------------------------------
// close to stop

int plugin_close_to_stop() {
    child_check_fds();
    child_check_environment();

    char buffer[1024];
    while (fgets(buffer, sizeof(buffer), stdin) != NULL) {
        fprintf(stderr, "+");
        printf("%s", buffer);
        fflush(stdout);
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "child detected a closed pipe.");
    exit(1);
}

void test_int_fds_plugin_close_to_stop(SPAWN_SERVER *server, const char *argv0) {
    const char *params[] = {
        argv0,
        "plugin-close-to-stop",
        NULL,
    };

    SPAWN_INSTANCE *si = spawn_server_exec(server, STDERR_FILENO, 0, params, NULL, 0, SPAWN_INSTANCE_TYPE_EXEC);
    if(!si) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (spawn)");
        exit(1);
    }

    test_int_fds_echo_loop(si, "Hello World!\n", 1);

    int code = spawn_server_exec_wait(server, si);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child exited with code %d",
           code);

    if(!WIFEXITED(code) || WEXITSTATUS(code) != 1) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "child should exit with code 1, but exited with code %d", code);
        warnings++;
    }
}

void test_popen_plugin_close_to_stop(const char *argv0) {
    char cmd[FILENAME_MAX + 100];
    snprintfz(cmd, sizeof(cmd), "exec %s plugin-close-to-stop", argv0);
    POPEN_INSTANCE *pi = spawn_popen_run(cmd);
    if(!pi) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (popen)");
        exit(1);
    }

    test_popen_echo_loop(pi, "Hello World!\n", 1);

    int code = spawn_popen_wait(pi);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child exited with code %d",
           code);

    if(code != 1) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "child should exit with code 1, but exited with code %d", code);
        warnings++;
    }
}

// --------------------------------------------------------------------------------------------------------------------
// echo and exit

#define ECHO_AND_EXIT_MSG "GOODBYE\n"

int plugin_echo_and_exit() {
    child_check_fds();
    child_check_environment();

    printf(ECHO_AND_EXIT_MSG);
    exit(0);
}

void test_int_fds_plugin_echo_and_exit(SPAWN_SERVER *server, const char *argv0) {
    const char *params[] = {
        argv0,
        "plugin-echo-and-exit",
        NULL,
    };

    SPAWN_INSTANCE *si = spawn_server_exec(server, STDERR_FILENO, 0, params, NULL, 0, SPAWN_INSTANCE_TYPE_EXEC);
    if(!si) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (spawn)");
        exit(1);
    }

    char buffer[1024];
    size_t reads = 0;

    for(size_t j = 0; j < 30 ;j++) {
        fprintf(stderr, "-");
        memset(buffer, 0, sizeof(buffer));

        ssize_t rc = read(spawn_server_instance_read_fd(si), buffer, sizeof(buffer));
        if(rc <= 0)
            break;

        reads++;

        if (rc != strlen(ECHO_AND_EXIT_MSG)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot read from plugin. Expected to read %zu bytes, read %zd bytes",
                   strlen(ECHO_AND_EXIT_MSG), rc);
            exit(1);
        }
        if (memcmp(ECHO_AND_EXIT_MSG, buffer, strlen(ECHO_AND_EXIT_MSG)) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Read corrupted data. Expected '%s', Read '%s'",
                   ECHO_AND_EXIT_MSG, buffer);
            exit(1);
        }
    }
    fprintf(stderr, "\n");

    if(reads != 1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Cannot read from plugin. Expected to read %d times, but read %zu",
               1, reads);
        exit(1);
    }

    int code = spawn_server_exec_wait(server, si);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child exited with code %d",
           code);

    if(code != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "child should exit with code 0, but exited with code %d", code);
        warnings++;
    }
}

void test_popen_plugin_echo_and_exit(const char *argv0) {
    char cmd[FILENAME_MAX + 100];
    snprintfz(cmd, sizeof(cmd), "exec %s plugin-echo-and-exit", argv0);
    POPEN_INSTANCE *pi = spawn_popen_run(cmd);
    if(!pi) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (popen)");
        exit(1);
    }

    char buffer[1024];
    FILE *child_stdout = spawn_popen_stdout(pi);
    if(!child_stdout) {
        spawn_popen_kill(pi, 0);
        netdata_main_spawn_server_cleanup();
        exit(1);
    }

    size_t reads = 0;
    for(size_t j = 0; j < 30 ;j++) {
        fprintf(stderr, "-");
        memset(buffer, 0, sizeof(buffer));

        char *s = fgets(buffer, sizeof(buffer), child_stdout);
        if(!s) break;
        reads++;
        if (strlen(s) != strlen(ECHO_AND_EXIT_MSG)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot read from plugin. Expected to read %zu bytes, read %zu bytes",
                   strlen(ECHO_AND_EXIT_MSG), (size_t)(s ? strlen(s) : 0));
            exit(1);
        }
        if (memcmp(ECHO_AND_EXIT_MSG, buffer, strlen(ECHO_AND_EXIT_MSG)) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Read corrupted data. Expected '%s', Read '%s'",
                   ECHO_AND_EXIT_MSG, buffer);
            exit(1);
        }
    }
    fprintf(stderr, "\n");

    if(reads != 1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Cannot read from plugin. Expected to read %d times, but read %zu",
               1, reads);
        exit(1);
    }

    int code = spawn_popen_wait(pi);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child exited with code %d",
           code);

    if(code != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "child should exit with code 0, but exited with code %d", code);
        warnings++;
    }
}

// --------------------------------------------------------------------------------------------------------------------
// termination contract: closing a child's stdout must kill it
//
// Signal HANDLERS are reset by exec, but IGNORED signals are not - SIG_IGN survives execve().
// netdata ignores SIGPIPE, so unless the spawn server forces SIGPIPE back to default in the child,
// the child inherits the ignore, gets EPIPE instead of dying, and keeps running after we stop
// reading it. For a child we cannot signal (a setuid-root helper that exec'd its target) that is a
// permanent leak, which is exactly how netdata/netdata#23730 stranded 624 powermetrics processes.
//
// This is the automated guard for that bug: it exercises netdata's own spawn path and fails if
// SIGPIPE is not both reset to default AND deliverable in the child. It deliberately does not
// involve powermetrics - whether Apple's tool dies on a broken pipe is an external premise,
// measured separately (see tests/manual/spawn-termination-macos.sh and the note next to
// macos_powermetrics_loop_kill_grace_ms()); it is not something this repository can assert.

#if !defined(OS_WINDOWS)
static int plugin_stream_until_pipe_breaks(void) {
    child_check_fds();
    child_check_environment();

    // Never exit voluntarily while writes succeed: this child must only stop when the kernel kills
    // it with SIGPIPE. Reaching a write error at all means SIGPIPE was not deliverable, so the
    // error is reported through a distinct exit code (below) rather than treated as normal
    // shutdown. Unbuffered write() - stdio without a flush would never touch the pipe.
    static const char chunk[1024] = { 0 };
    for(;;) {
        ssize_t rc = write(STDOUT_FILENO, chunk, sizeof(chunk));
        if(rc == -1) {
            if(errno == EINTR)
                continue;

            // Distinguish the finding from noise: only EPIPE means "we survived a broken pipe",
            // i.e. SIGPIPE was inherited as SIG_IGN or left blocked. Any other errno is an
            // unrelated I/O failure and must not be reported as a contract regression.
            if(errno == EPIPE) {
                fprintf(stderr, "child survived a broken stdout - SIGPIPE is not deliverable\n");
                return 66;
            }

            fprintf(stderr, "child's stdout write failed with errno %d, unrelated to the contract\n", errno);
            return 67;
        }
    }
}

// spawn_server_destroy() SIGTERMs the server then waits for it with an unbounded waitpid(), and
// A broken server can leave a request read blocked; kill the dedicated test server first.
// Dedicated test servers are therefore SIGKILLed first so cleanup cannot hang.
static void test_server_destroy(SPAWN_SERVER *server) {
    if(server && spawn_server_pid(server) > 0)
        (void)kill(spawn_server_pid(server), SIGKILL);

    if(server)
        spawn_server_destroy(server);
}

static void test_exec_child_dies_when_stdout_closes(int argc, const char **argv) {
    // The daemon's ignored SIGPIPE - the thing the child must not inherit - is installed
    // process-wide by main() (see ignore_sigpipe_like_the_daemon()), so it is already in place here.
    SPAWN_SERVER *server = spawn_server_create(SPAWN_SERVER_OPTION_EXEC, "test-sigpipe", NULL, argc, argv);
    if(!server) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot create spawn server for the termination contract test");
        exit(1);
    }

    const char *params[] = {
        argv[0],
        "plugin-stream-until-pipe-breaks",
        NULL,
    };

    SPAWN_INSTANCE *si = spawn_server_exec(server, STDERR_FILENO, 0, params, NULL, 0, SPAWN_INSTANCE_TYPE_EXEC);
    if(!si) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (spawn)");
        test_server_destroy(server);
        exit(1);
    }

    // spawn_server_exec_timedwait() closes both pipe ends itself and sends no signal, so whatever
    // kills the child here is the closed pipe and nothing else. The child blocks in write() once
    // the pipe fills, so the close makes its pending write fail at once - no timing assumptions.
    int status = 0;
    SPAWN_TIMEDWAIT_RESULT r = SPAWN_TIMEDWAIT_RUNNING;
    for(size_t slices = 0; slices < 50 && r == SPAWN_TIMEDWAIT_RUNNING; slices++)
        r = spawn_server_exec_timedwait(server, si, 100, &status);

    if(r == SPAWN_TIMEDWAIT_RUNNING) {
        // si is still owned on RUNNING - reclaim the child before failing.
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "child did not exit after its stdout was closed - the termination contract is broken");
        spawn_server_exec_kill(server, si, 0);
        test_server_destroy(server);
        exit(1);
    }

    if(r == SPAWN_TIMEDWAIT_ERROR) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "spawn_server_exec_timedwait() returned ERROR");
        spawn_server_exec_kill(server, si, 0);
        test_server_destroy(server);
        exit(1);
    }

    if(!WIFSIGNALED(status) || WTERMSIG(status) != SIGPIPE) {
        if(WIFEXITED(status) && WEXITSTATUS(status) == 66)
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "child survived its broken stdout: SIGPIPE was not deliverable in the child "
                   "(inherited as SIG_IGN, or left blocked, across exec)");
        else if(WIFEXITED(status) && WEXITSTATUS(status) == 67)
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "child hit an unrelated write error - this test could not assess the contract");
        else
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "child did not die of SIGPIPE when its stdout closed (raw status %d)", status);
        // si was already reclaimed by the EXITED result; only the server is still ours.
        test_server_destroy(server);
        exit(1);
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "child died of SIGPIPE when its stdout closed, as it must");

    test_server_destroy(server);
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// timed wait

int plugin_sleep_to_stop(void) {
    child_check_fds();
    child_check_environment();

    // ignore the pipes - only a kill can stop us within the test's lifetime
    sleep_usec(3600 * USEC_PER_SEC);
    return 0;
}

void test_popen_plugin_timedwait_exits(const char *argv0) {
    // a child that exits on its own must be reaped by spawn_popen_timedwait()
    char cmd[FILENAME_MAX + 100];
    snprintfz(cmd, sizeof(cmd), "exec %s plugin-echo-and-exit", argv0);
    POPEN_INSTANCE *pi = spawn_popen_run(cmd);
    if(!pi) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (popen)");
        exit(1);
    }

    int code = -1;
    size_t slices = 0;
    for(;;) {
        SPAWN_TIMEDWAIT_RESULT r = spawn_popen_timedwait(pi, 100, &code);
        if(r == SPAWN_TIMEDWAIT_EXITED)
            break;

        if(r == SPAWN_TIMEDWAIT_ERROR) {
            // ERROR must never be looped over; for a cleanly-exiting child it should not happen at all.
            // pi is still owned on ERROR, so reclaim it before bailing out.
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "spawn_popen_timedwait() returned ERROR for a child that should exit cleanly");
            spawn_popen_kill(pi, 0);
            exit(1);
        }

        // SPAWN_TIMEDWAIT_RUNNING - pi is still owned
        if(++slices > 100) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "spawn_popen_timedwait() did not reap a child that exits immediately");
            spawn_popen_kill(pi, 0);
            exit(1);
        }
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child exited with code %d (after %zu timedwait slices)",
           code, slices);

    if(code != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "child should exit with code 0, but exited with code %d", code);
        warnings++;
    }
}

void test_popen_plugin_timedwait_kill(const char *argv0) {
    // a child that never exits must be reported still-running on every slice, then killed
    char cmd[FILENAME_MAX + 100];
    snprintfz(cmd, sizeof(cmd), "exec %s plugin-sleep-to-stop", argv0);
    POPEN_INSTANCE *pi = spawn_popen_run(cmd);
    if(!pi) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (popen)");
        exit(1);
    }

    int code = 0;
    for(size_t i = 0; i < 5; i++) {
        SPAWN_TIMEDWAIT_RESULT r = spawn_popen_timedwait(pi, 200, &code);
        if(r != SPAWN_TIMEDWAIT_RUNNING) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "spawn_popen_timedwait() did not report RUNNING for a sleeping child");
            // on ERROR we still own pi and must reclaim it; on EXITED it was already freed
            if(r == SPAWN_TIMEDWAIT_ERROR) spawn_popen_kill(pi, 0);
            exit(1);
        }
    }

    code = spawn_popen_kill(pi, 0);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child killed, exited with code %d",
           code);

    if(code != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "killed child should report code 0, but reported code %d", code);
        warnings++;
    }
}

// --------------------------------------------------------------------------------------------------------------------

#if !defined(OS_WINDOWS)
static int callback_wait_for_sigterm(SPAWN_REQUEST *request) {
    struct sigaction sigterm_action, sigchld_action, sigpipe_action;
    sigset_t mask;

    // SIGPIPE is checked on both axes deliberately. netdata ignores it AND blocks it, and fork()
    // inherits both; a callback child needs the disposition reset AND the signal unblocked, because
    // a blocked SIGPIPE leaves write() returning EPIPE with the signal pending - the child then
    // survives a closed pipe just as if it were still ignored.
    if(sigaction(SIGTERM, NULL, &sigterm_action) == -1 ||
       sigaction(SIGCHLD, NULL, &sigchld_action) == -1 ||
       sigaction(SIGPIPE, NULL, &sigpipe_action) == -1 ||
       pthread_sigmask(SIG_BLOCK, NULL, &mask) != 0 ||
       sigterm_action.sa_handler != SIG_DFL ||
       sigchld_action.sa_handler != SIG_DFL ||
       sigpipe_action.sa_handler != SIG_DFL ||
       sigismember(&mask, SIGTERM) != 0 ||
       sigismember(&mask, SIGCHLD) != 0 ||
       sigismember(&mask, SIGPIPE) != 0)
        return EXIT_FAILURE;

    static const char ready = 'R';
    if(write(request->fds[1], &ready, sizeof(ready)) != sizeof(ready))
        return EXIT_FAILURE;

    for(;;)
        pause();
}

static void test_callback_signal_lifecycle(int argc, const char **argv) {
    SPAWN_SERVER *server = spawn_server_create(
        SPAWN_SERVER_OPTION_CALLBACK, "test-callback", callback_wait_for_sigterm, argc, argv);
    if(!server) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot create callback spawn server");
        exit(1);
    }

    SPAWN_INSTANCE *si = spawn_server_exec(
        server, STDERR_FILENO, STDIN_FILENO, NULL, NULL, 0, SPAWN_INSTANCE_TYPE_CALLBACK);
    if(!si) {
        spawn_server_destroy(server);
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run callback signal lifecycle test");
        exit(1);
    }

    char ready = 0;
    if(read(spawn_server_instance_read_fd(si), &ready, sizeof(ready)) != sizeof(ready) || ready != 'R') {
        spawn_server_exec_kill(server, si, 0);
        spawn_server_destroy(server);
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Callback child did not enter with default unblocked lifecycle signals");
        exit(1);
    }

    int status = spawn_server_exec_kill(server, si, 0);
    if(!WIFSIGNALED(status) || WTERMSIG(status) != SIGTERM) {
        spawn_server_destroy(server);
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Callback child should terminate with SIGTERM, raw status is %d", status);
        exit(1);
    }

    si = spawn_server_exec(server, STDERR_FILENO, STDIN_FILENO, NULL, NULL, 0, SPAWN_INSTANCE_TYPE_CALLBACK);
    if(!si) {
        spawn_server_destroy(server);
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run immediate callback signal lifecycle test");
        exit(1);
    }

    status = spawn_server_exec_kill(server, si, 0);
    spawn_server_destroy(server);

    if(!WIFSIGNALED(status) || WTERMSIG(status) != SIGTERM) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Immediately killed callback child should terminate with SIGTERM, raw status is %d", status);
        exit(1);
    }
}
#endif

// --------------------------------------------------------------------------------------------------------------------

#if !defined(OS_WINDOWS)
// Model the daemon for the whole suite: netdata sets SIGPIPE to SIG_IGN before it creates its spawn
// server (NETDATA_SIGNAL_IGNORE in src/daemon/signal-handler.c), and SIG_IGN survives execve(), so
// an ignored SIGPIPE is exactly what a spawned child must NOT inherit.
//
// This has to be process-wide, and it has to happen before the FIRST test that asserts anything
// about a child's SIGPIPE state. Doing it per-test is what made the callback assertion in
// callback_wait_for_sigterm() depend on the invoking shell: with SIGPIPE left at its default, the
// callback child sees SIG_DFL no matter what spawn_server_run_callback() does, so the assertion
// passed even with the reset removed. It only bit on CI, where the runner leaks SIG_IGN into every
// step's shell (GitHub Actions is Node-based and Node ignores SIGPIPE - the same inheritance the
// fix is about, which tests/manual/spawn-termination-macos.sh documents). Coverage that depends on
// the caller's signal state is not coverage.
//
// Only the DISPOSITION needs modelling here. The daemon blocks SIGPIPE too, but that half needs no
// help: spawn_server_event_loop() calls signals_block_all() and unblocks only SIGTERM and SIGCHLD,
// so the callback child's inherited mask always carries SIGPIPE blocked and the sigdelset() in
// spawn_server_run_callback() is exercised on every run, whatever our own mask is.
//
// It also makes the tester itself behave like the daemon: a write to a dead child's pipe returns
// EPIPE and hits our own error path with a diagnostic, instead of killing the tester silently.
static void ignore_sigpipe_like_the_daemon(void) {
    struct sigaction ignore_pipe = {
        .sa_handler = SIG_IGN,
    };
    sigemptyset(&ignore_pipe.sa_mask);

    if(sigaction(SIGPIPE, &ignore_pipe, NULL) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot ignore SIGPIPE to model the daemon");
        exit(1);
    }
}
#endif

#if defined(SPAWN_SERVER_VERSION_NOFORK)
// The descendant deliberately ignores TERM and retains stdout. A readiness pipe ensures
// the leader-exits-first case cannot pass merely because fork() had not happened yet.
static int plugin_group_descendant(bool leader_exits) {
    int ready[2];
    if(pipe(ready) != 0) return 80;
    if(signal(SIGTERM, SIG_IGN) == SIG_ERR) return 81;
    pid_t child = fork();
    if(child == -1) return 82;
    if(child == 0) {
        close(ready[0]);
        if(write(ready[1], "R", 1) != 1) _exit(83);
        close(ready[1]);
        for(;;) pause();
    }
    close(ready[1]);
    char byte;
    if(read(ready[0], &byte, 1) != 1) return 84;
    close(ready[0]);
    if(write(STDOUT_FILENO, "R", 1) != 1) return 85;
    if(leader_exits) return 0;
    for(;;) pause();
}

static void group_test_require(bool ok, const char *what) {
    if(ok) return;
    nd_log(NDLS_COLLECTORS, NDLP_ERR, "Process-group test failed: %s", what);
    // The test owns this server. Normal destruction cancels its remaining owned groups.
    netdata_main_spawn_server_cleanup();
    exit(1);
}

static void test_popen_groups(const char *argv0) {
    struct {
        const char *script;
        int code;
    } cases[] = {
        { "exit 0", 0 },
        { "exit 7", 7 },
        { "kill -TERM $$", -1 },
        { "kill -PIPE $$", -1 },
    };
    for(size_t i = 0; i < _countof(cases); i++) {
        const char *args[] = { "/bin/sh", "-c", cases[i].script, NULL };
        POPEN_INSTANCE *pi = spawn_popen_run_argv_group(args);
        group_test_require(pi != NULL, "spawn status fixture");
        int code;
        SPAWN_TIMEDWAIT_RESULT result = spawn_popen_timedwait(pi, 5000, &code);
        if(result != SPAWN_TIMEDWAIT_EXITED) spawn_popen_kill(pi, 0);
        group_test_require(result == SPAWN_TIMEDWAIT_EXITED && code == cases[i].code,
                           "preserve normal, nonzero, and signal exit status");
    }

    // An unrelated request must remain alive throughout both kinds of group cleanup.
    const char *sentinel_args[] = { "/bin/sh", "-c", "echo R; read value; test \"$value\" = done", NULL };
    POPEN_INSTANCE *sentinel = spawn_popen_run_argv_group(sentinel_args);
    group_test_require(sentinel != NULL, "spawn unrelated request");
    struct pollfd sentinel_fd = { .fd = spawn_popen_read_fd(sentinel), .events = POLLIN };
    group_test_require(poll(&sentinel_fd, 1, 5000) > 0, "unrelated request readiness");
    char sentinel_ready[2];
    group_test_require(read(sentinel_fd.fd, sentinel_ready, 2) == 2, "read unrelated readiness");

    for(size_t exits_first = 0; exits_first < 2; exits_first++) {
        const char *args[] = { argv0, exits_first ? "plugin-group-leader-exits" : "plugin-group-hangs", NULL };
        POPEN_INSTANCE *pi = spawn_popen_run_argv_group(args);
        group_test_require(pi != NULL, "spawn descendant fixture");
        // timedwait/kill close their own pipes. Keep another reader to observe real EOF,
        // proving the descendant has stopped, rather than merely that the leader was reaped.
        int output = dup(spawn_popen_read_fd(pi));
        group_test_require(output != -1, "duplicate descendant stdout");
        struct pollfd output_fd = { .fd = output, .events = POLLIN };
        group_test_require(poll(&output_fd, 1, 5000) > 0, "descendant readiness");
        char byte;
        group_test_require(read(output, &byte, 1) == 1 && byte == 'R', "descendant started");
        usec_t start = now_monotonic_usec();
        int code;
        if(exits_first) {
            // Detection reads to EOF before it waits for status. Cleanup must happen even
            // when no client wait/kill has been called and a descendant keeps stdout open.
            group_test_require(poll(&output_fd, 1, 5000) > 0 && read(output, &byte, 1) == 0,
                               "leader exit proactively closes descendant output");
            SPAWN_TIMEDWAIT_RESULT result = spawn_popen_timedwait(pi, 5000, &code);
            if(result != SPAWN_TIMEDWAIT_EXITED) spawn_popen_kill(pi, 0);
            group_test_require(result == SPAWN_TIMEDWAIT_EXITED && code == 0, "leader exits first");
        }
        else {
            group_test_require(getpgid(spawn_popen_pid(pi)) == spawn_popen_pid(pi), "private process group");
            SPAWN_TIMEDWAIT_RESULT result = spawn_popen_timedwait(pi, 20, &code);
            group_test_require(result == SPAWN_TIMEDWAIT_RUNNING, "hanging leader reaches deadline");
            code = spawn_popen_kill(pi, 0);
            group_test_require(code == -1, "forced termination is failure");
        }
        group_test_require(now_monotonic_usec() - start < 6 * USEC_PER_SEC, "bounded cleanup");
        group_test_require(poll(&output_fd, 1, 5000) > 0 && read(output, &byte, 1) == 0,
                           "descendant releases stdout after leader exit or timeout");
        close(output);
    }
    group_test_require(write(spawn_popen_write_fd(sentinel), "done\n", 5) == 5,
                       "unrelated request survives group cleanup");
    int code;
    SPAWN_TIMEDWAIT_RESULT result = spawn_popen_timedwait(sentinel, 5000, &code);
    if(result != SPAWN_TIMEDWAIT_EXITED) spawn_popen_kill(sentinel, 0);
    group_test_require(result == SPAWN_TIMEDWAIT_EXITED && code == 0,
                       "unrelated request exits normally");
}
#endif

int main(int argc, const char **argv) {
#if defined(SPAWN_SERVER_VERSION_NOFORK)
    if(argc > 1 && strcmp(argv[1], "plugin-group-hangs") == 0)
        return plugin_group_descendant(false);
    if(argc > 1 && strcmp(argv[1], "plugin-group-leader-exits") == 0)
        return plugin_group_descendant(true);
#endif
    if(argc > 1 && strcmp(argv[1], "plugin-kill-to-stop") == 0)
        return plugin_kill_to_stop();

    if(argc > 1 && strcmp(argv[1], "plugin-sleep-to-stop") == 0)
        return plugin_sleep_to_stop();

    if(argc > 1 && strcmp(argv[1], "plugin-echo-and-exit") == 0)
        return plugin_echo_and_exit();

#if defined(SPAWN_SERVER_VERSION_NOFORK)
    if(argc > 1 && strcmp(argv[1], "plugin-echo-argument") == 0)
        return plugin_echo_argument(argc, (const char **)argv);
#endif

    if(argc > 1 && strcmp(argv[1], "plugin-close-to-stop") == 0)
        return plugin_close_to_stop();

#if !defined(OS_WINDOWS)
    if(argc > 1 && strcmp(argv[1], "plugin-stream-until-pipe-breaks") == 0)
        return plugin_stream_until_pipe_breaks();
#endif

    if(argc <= 1 || strcmp(argv[1], "test") != 0) {
        fprintf(stderr, "Run me with 'test' parameter!\n");
        exit(1);
    }

    nd_setenv(ENV_VAR_KEY, ENV_VAR_VALUE, 1);

#if !defined(OS_WINDOWS)
    // Only for the test driver, and it MUST stay below the plugin-* dispatch above: those early
    // returns are the exec'd children. If they came through here they would ignore SIGPIPE
    // themselves, after exec, and test_exec_child_dies_when_stdout_closes() could never fail.
    ignore_sigpipe_like_the_daemon();
#endif

#if defined(SPAWN_SERVER_VERSION_NOFORK)
    fprintf(stderr, "\n\nTESTING private control channel\n\n");
    test_private_control_channel(argc, argv);
#endif

    fprintf(stderr, "\n\nTESTING fds\n\n");
    SPAWN_SERVER *server = spawn_server_create(SPAWN_SERVER_OPTION_EXEC, "test", NULL, argc, argv);
    if(!server) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot create spawn server");
        exit(1);
    }
    for(size_t i = 0; i < 5; i++) {
        fprintf(stderr, "\n\nTESTING fds No %zu (kill to stop)\n\n", i + 1);
        test_int_fds_plugin_kill_to_stop(server, argv[0]);
    }
    for(size_t i = 0; i < 5; i++) {
        fprintf(stderr, "\n\nTESTING fds No %zu (echo and exit)\n\n", i + 1);
        test_int_fds_plugin_echo_and_exit(server, argv[0]);
    }
    for(size_t i = 0; i < 5; i++) {
        fprintf(stderr, "\n\nTESTING fds No %zu (close to stop)\n\n", i + 1);
        test_int_fds_plugin_close_to_stop(server, argv[0]);
    }
    spawn_server_destroy(server);

#if !defined(OS_WINDOWS)
    fprintf(stderr, "\n\nTESTING callback signal lifecycle\n\n");
    test_callback_signal_lifecycle(argc, argv);

    fprintf(stderr, "\n\nTESTING termination contract (closing stdout must kill the child)\n\n");
    test_exec_child_dies_when_stdout_closes(argc, argv);
#endif

    fprintf(stderr, "\n\nTESTING popen\n\n");
    netdata_main_spawn_server_init("test", argc, argv);
    for(size_t i = 0; i < 5; i++) {
        fprintf(stderr, "\n\nTESTING popen No %zu (kill to stop)\n\n", i + 1);
        test_popen_plugin_kill_to_stop(argv[0]);
    }
    for(size_t i = 0; i < 5; i++) {
        fprintf(stderr, "\n\nTESTING popen No %zu (echo and exit)\n\n", i + 1);
        test_popen_plugin_echo_and_exit(argv[0]);
    }
    for(size_t i = 0; i < 5; i++) {
        fprintf(stderr, "\n\nTESTING popen No %zu (close to stop)\n\n", i + 1);
        test_popen_plugin_close_to_stop(argv[0]);
    }
    for(size_t i = 0; i < 5; i++) {
        fprintf(stderr, "\n\nTESTING popen No %zu (timedwait exits)\n\n", i + 1);
        test_popen_plugin_timedwait_exits(argv[0]);
    }
    for(size_t i = 0; i < 5; i++) {
        fprintf(stderr, "\n\nTESTING popen No %zu (timedwait kill)\n\n", i + 1);
        test_popen_plugin_timedwait_kill(argv[0]);
    }
#if defined(SPAWN_SERVER_VERSION_NOFORK)
    fprintf(stderr, "\n\nTESTING owned process groups\n\n");
    test_popen_groups(argv[0]);
#endif
    netdata_main_spawn_server_cleanup();

    fprintf(stderr, "\n\nTests passed! (%zu warnings)\n\n", warnings);

    exit(0);
}
