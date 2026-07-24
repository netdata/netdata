#include "libnetdata/libnetdata.h"

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
    struct sigaction sigterm_action, sigchld_action;
    sigset_t mask;

    if(sigaction(SIGTERM, NULL, &sigterm_action) == -1 ||
       sigaction(SIGCHLD, NULL, &sigchld_action) == -1 ||
       pthread_sigmask(SIG_BLOCK, NULL, &mask) != 0 ||
       sigterm_action.sa_handler != SIG_DFL ||
       sigchld_action.sa_handler != SIG_DFL ||
       sigismember(&mask, SIGTERM) != 0 ||
       sigismember(&mask, SIGCHLD) != 0)
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

    if(argc <= 1 || strcmp(argv[1], "test") != 0) {
        fprintf(stderr, "Run me with 'test' parameter!\n");
        exit(1);
    }

    nd_setenv(ENV_VAR_KEY, ENV_VAR_VALUE, 1);

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
