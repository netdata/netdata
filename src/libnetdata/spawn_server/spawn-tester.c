#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#define ENV_VAR_KEY "SPAWN_TESTER"
#define ENV_VAR_VALUE "1234567890"

int external_plugin() {
    const char *s = getenv(ENV_VAR_KEY);
    if(!s || !*s || strcmp(s, ENV_VAR_VALUE) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Wrong environment. Variable '%s' should have value '%s' but it has '%s'",
               ENV_VAR_KEY, ENV_VAR_VALUE, s ? s : "(unset)");

        exit(1);
    }

    char buffer[1024];
    while (fgets(buffer, sizeof(buffer), stdin) != NULL) {
        fprintf(stderr, "+");
        printf("%s", buffer);
        fflush(stdout);
    }

    return 0;
}

void test_int_fds(SPAWN_SERVER *server, int argc __maybe_unused, const char **argv) {
    const char *params[] = {
        argv[0],
        "plugin",
        NULL,
    };

    SPAWN_INSTANCE *si = spawn_server_exec(server, STDERR_FILENO, 0, params, NULL, 0, SPAWN_INSTANCE_TYPE_EXEC);
    if(!si) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (spawn)");
        exit(1);
    }

    const char *msg = "Hello World!\n";
    ssize_t len = strlen(msg);
    char buffer[len * 2];

    for(size_t j = 0; j < 30 ;j++) {
        fprintf(stderr, "-");
        memset(buffer, 0, sizeof(buffer));

        ssize_t rc = write(spawn_server_instance_write_fd(si), msg, len);
        if (rc != len) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot write to plugin. Expected to write %zd bytes, wrote %zd bytes",
                   len, rc);
            exit(1);
        }

        rc = read(spawn_server_instance_read_fd(si), buffer, sizeof(buffer));
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

    int code = spawn_server_exec_kill(server, si);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child exited with code %d",
           code);
}

void test_popen(int argc __maybe_unused, const char **argv) {
    char cmd[FILENAME_MAX + 100];
    snprintfz(cmd, sizeof(cmd), "exec %s plugin", argv[0]);
    POPEN_INSTANCE *pi = spawn_popen_run(cmd);
    if(!pi) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot run myself as plugin (popen)");
        exit(1);
    }

    const char *msg = "Hello World!\n";
    size_t len = strlen(msg);
    char buffer[len * 2];

    for(size_t j = 0; j < 30 ;j++) {
        fprintf(stderr, "-");
        memset(buffer, 0, sizeof(buffer));

        size_t rc = fwrite(msg, 1, len, spawn_popen_stdin(pi));
        if (rc != len) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "Cannot write to plugin. Expected to write %zu bytes, wrote %zu bytes",
                   len, rc);
            exit(1);
        }
        fflush(spawn_popen_stdin(pi));

        char *s = fgets(buffer, sizeof(buffer), spawn_popen_stdout(pi));
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

    int code = spawn_popen_kill(pi);

    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "child exited with code %d",
           code);
}

int main(int argc, const char **argv) {
    if(argc > 1 && strcmp(argv[1], "plugin") == 0)
        return external_plugin();

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
    for(size_t i = 0; i < 10; i++) {
        fprintf(stderr, "\n\nTESTING fds No %zu\n\n", i + 1);
        test_int_fds(server, argc, argv);
    }
    spawn_server_destroy(server);

    fprintf(stderr, "\n\nTESTING popen\n\n");
    netdata_main_spawn_server_init("test", argc, argv);
    for(size_t i = 0; i < 10; i++) {
        fprintf(stderr, "\n\nTESTING popen No %zu\n\n", i + 1);
        test_popen(argc, argv);
    }
    netdata_main_spawn_server_cleanup();

    exit(0);
}
