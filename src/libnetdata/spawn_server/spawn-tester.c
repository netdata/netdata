#include "libnetdata/libnetdata.h"
#include "spawn_server_internals.h"

#if defined(OS_FREEBSD) || defined(OS_MACOS)
#include <sys/sysctl.h>
#endif

#define ENV_VAR_KEY "SPAWN_TESTER"
#define ENV_VAR_VALUE "1234567890"

size_t warnings = 0;

#if defined(SPAWN_SERVER_VERSION_NOFORK)
#define SPAWN_SERVER_BACKLOG_TEST_CONNECTIONS 64

static size_t test_listener_backlog_cap(void) {
    size_t cap = SOMAXCONN;

#if defined(OS_LINUX)
    char buffer[32];
    if(read_txt_file("/proc/sys/net/core/somaxconn", buffer, sizeof(buffer)) == 0) {
        size_t kernel_cap = str2ull(buffer, NULL);
        if(kernel_cap && kernel_cap < cap)
            cap = kernel_cap;
    }
#elif defined(OS_FREEBSD) || defined(OS_MACOS)
    int kernel_cap;
    size_t kernel_cap_size = sizeof(kernel_cap);
    if(sysctlbyname("kern.ipc.somaxconn", &kernel_cap, &kernel_cap_size, NULL, 0) == 0 && kernel_cap > 0 &&
       (size_t)kernel_cap < cap)
        cap = (size_t)kernel_cap;
#endif

    return cap;
}

static void test_listener_backlog_cleanup(SPAWN_SERVER *server, int *clients, size_t clients_count) {
    // SIGTERM restarts the server's blocked recvmsg() (SA_RESTART), while
    // spawn_server_destroy() waits for it without a timeout. SIGKILL the
    // dedicated test server before destruction so cleanup cannot hang.
    if(server && spawn_server_pid(server) > 0)
        (void)kill(spawn_server_pid(server), SIGKILL);

    for(size_t i = 0; i < clients_count; i++)
        if(clients[i] != -1)
            close(clients[i]);

    if(server)
        spawn_server_destroy(server);
}

static void test_listener_backlog(SPAWN_SERVER *server) {
    const size_t configured_cap =
        SOMAXCONN < SPAWN_SERVER_BACKLOG_TEST_CONNECTIONS ? (size_t)SOMAXCONN : SPAWN_SERVER_BACKLOG_TEST_CONNECTIONS;
    const size_t kernel_cap = test_listener_backlog_cap();
    const size_t connections = kernel_cap < configured_cap ? kernel_cap : configured_cap;
    size_t historical_backlog_admission = 5;
#if defined(OS_MACOS)
    historical_backlog_admission = 3 * historical_backlog_admission / 2;
#endif
    int clients[SPAWN_SERVER_BACKLOG_TEST_CONNECTIONS];
    struct sockaddr_un server_addr = {
        .sun_family = AF_UNIX,
    };

    for(size_t i = 0; i < connections; i++)
        clients[i] = -1;

    if(connections <= historical_backlog_admission) {
        test_listener_backlog_cleanup(server, clients, connections);
        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
               "Skipping spawn-server backlog test: runtime listener cap %zu cannot distinguish the historical admission capacity of %zu.",
               kernel_cap, historical_backlog_admission);
        return;
    }

    if(strlen(server->path) >= sizeof(server_addr.sun_path)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Cannot test spawn-server backlog: socket path '%s' is too long.", server->path);
        test_listener_backlog_cleanup(server, clients, connections);
        exit(1);
    }

    strncpyz(server_addr.sun_path, server->path, sizeof(server_addr.sun_path) - 1);

    // Keep every client idle. The server blocks in its untimed first recvmsg(),
    // so the remaining connections exercise the listener's pending queue. If
    // request reception becomes timed or asynchronous, replace this mechanism:
    // otherwise listen(..., 5) can drain and make this regression test pass.
    for(size_t i = 0; i < connections; i++) {
        clients[i] = socket(AF_UNIX, SOCK_STREAM, 0);
        if(clients[i] == -1) {
            int error = errno;
            test_listener_backlog_cleanup(server, clients, connections);

            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot create spawn-server backlog test client %zu/%zu: %s (%d).",
                   i + 1, connections, strerror(error), error);
            exit(1);
        }

        if(sock_setnonblock(clients[i], true) != 1) {
            int error = errno;
            test_listener_backlog_cleanup(server, clients, connections);

            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot make spawn-server backlog test client %zu/%zu nonblocking: %s (%d).",
                   i + 1, connections, strerror(error), error);
            exit(1);
        }

        if(connect(clients[i], (struct sockaddr *)&server_addr, sizeof(server_addr)) == -1) {
            int error = errno;

            // AF_UNIX stream connect() completes synchronously on every supported platform, so an
            // in-progress result is unreachable today. Accept it anyway: it means the listener queued
            // the connection, which is what this test asserts - never a backlog rejection.
            if(error == EINPROGRESS || error == EALREADY)
                continue;

            test_listener_backlog_cleanup(server, clients, connections);

            if(error != EAGAIN && error != EWOULDBLOCK && error != ECONNREFUSED) {
                nd_log(NDLS_COLLECTORS, NDLP_ERR,
                       "Cannot connect spawn-server backlog test client %zu/%zu: %s (%d).",
                       i + 1, connections, strerror(error), error);
                exit(1);
            }

            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Spawn-server listener rejected backlog test connection %zu/%zu: %s (%d).",
                   i + 1, connections, strerror(error), error);
            exit(1);
        }
    }

    test_listener_backlog_cleanup(server, clients, connections);
}
#endif

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
        fflush(child_stdin);

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
// SA_RESTART can restart the server's blocked recvmsg() - see test_listener_backlog_cleanup().
// Dedicated test servers are therefore SIGKILLed first so cleanup cannot hang.
static void test_server_destroy(SPAWN_SERVER *server) {
    if(server && spawn_server_pid(server) > 0)
        (void)kill(spawn_server_pid(server), SIGKILL);

    if(server)
        spawn_server_destroy(server);
}

static void test_exec_child_dies_when_stdout_closes(int argc, const char **argv) {
    // Model the daemon: netdata ignores SIGPIPE before it creates its spawn server, and that is
    // what the child must not inherit. Without this the test would pass trivially, because the
    // tester's own SIGPIPE is already default.
    struct sigaction ignore_pipe, previous_pipe;
    memset(&ignore_pipe, 0, sizeof(ignore_pipe));
    ignore_pipe.sa_handler = SIG_IGN;
    sigemptyset(&ignore_pipe.sa_mask);
    if(sigaction(SIGPIPE, &ignore_pipe, &previous_pipe) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot ignore SIGPIPE for the termination contract test");
        exit(1);
    }

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

    if(sigaction(SIGPIPE, &previous_pipe, NULL) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot restore SIGPIPE after the termination contract test");
        exit(1);
    }
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

int main(int argc, const char **argv) {
    if(argc > 1 && strcmp(argv[1], "plugin-kill-to-stop") == 0)
        return plugin_kill_to_stop();

    if(argc > 1 && strcmp(argv[1], "plugin-sleep-to-stop") == 0)
        return plugin_sleep_to_stop();

    if(argc > 1 && strcmp(argv[1], "plugin-echo-and-exit") == 0)
        return plugin_echo_and_exit();

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

#if defined(SPAWN_SERVER_VERSION_NOFORK)
    fprintf(stderr, "\n\nTESTING spawn-server listener backlog\n\n");
    SPAWN_SERVER *backlog_server = spawn_server_create(SPAWN_SERVER_OPTION_EXEC, NULL, NULL, argc, argv);
    if(!backlog_server) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot create spawn server for backlog test");
        exit(1);
    }
    test_listener_backlog(backlog_server);
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
    netdata_main_spawn_server_cleanup();

    fprintf(stderr, "\n\nTests passed! (%zu warnings)\n\n", warnings);

    exit(0);
}
