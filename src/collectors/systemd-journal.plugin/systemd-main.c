// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

#define ND_SD_JOURNAL_WORKER_THREADS 5
#define ND_SD_JOURNAL_TEST_TIMEOUT_DISABLED_SECONDS (100ULL * 365ULL * 24ULL * 60ULL * 60ULL)
#define ND_SD_JOURNAL_TEST_MAX_REQUEST_BYTES (16ULL * 1024ULL * 1024ULL)

netdata_mutex_t stdout_mutex;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&stdout_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&stdout_mutex);
}

static bool plugin_should_exit = false;

struct systemd_journal_test_command {
    bool enabled;
    const char *function_name;
    const char *backend_dir;
    uint64_t timeout_seconds;
    bool timeout_seconds_set;
};

static void systemd_journal_test_usage(FILE *stream)
{
    fprintf(
        stream,
        "usage: systemd-journal.plugin --test systemd-journal --dir <journal-dir> [--timeout <seconds>] < payload.json\n");
}

static bool test_option_present(int argc, char **argv)
{
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--test") == 0 || strncmp(argv[i], "--test=", strlen("--test=")) == 0)
            return true;
    }

    return false;
}

static int set_required_option_once(const char **slot, const char *value, const char *option)
{
    if (*slot) {
        fprintf(stderr, "duplicate %s\n", option);
        systemd_journal_test_usage(stderr);
        return 2;
    }

    if (!value || !*value) {
        fprintf(stderr, "missing value for %s\n", option);
        systemd_journal_test_usage(stderr);
        return 2;
    }

    *slot = value;
    return 0;
}

static int set_timeout_option_once(uint64_t *slot, bool *slot_set, const char *value)
{
    if (*slot_set) {
        fprintf(stderr, "duplicate --timeout\n");
        systemd_journal_test_usage(stderr);
        return 2;
    }

    if (!value || !*value) {
        fprintf(stderr, "missing value for --timeout\n");
        systemd_journal_test_usage(stderr);
        return 2;
    }

    for (const char *s = value; *s; s++) {
        if (*s < '0' || *s > '9') {
            fprintf(stderr, "invalid value for --timeout '%s'; expected seconds\n", value);
            systemd_journal_test_usage(stderr);
            return 2;
        }
    }

    errno = 0;
    unsigned long long parsed = strtoull(value, NULL, 10);
    if (errno == ERANGE) {
        fprintf(stderr, "invalid value for --timeout '%s'; expected seconds\n", value);
        systemd_journal_test_usage(stderr);
        return 2;
    }

#if ULLONG_MAX > UINT64_MAX
    if (parsed > UINT64_MAX) {
        fprintf(stderr, "invalid value for --timeout '%s'; expected seconds\n", value);
        systemd_journal_test_usage(stderr);
        return 2;
    }
#endif

    *slot = (uint64_t)parsed;
    *slot_set = true;
    return 0;
}

static int reject_request_option(void)
{
    fprintf(stderr, "--request is no longer supported; pass the request payload on stdin\n");
    systemd_journal_test_usage(stderr);
    return 2;
}

static int parse_systemd_journal_test_command(int argc, char **argv, struct systemd_journal_test_command *cmd)
{
    *cmd = (struct systemd_journal_test_command){0};
    if (!test_option_present(argc, argv))
        return 0;

    cmd->enabled = true;

    for (int i = 1; i < argc; i++) {
        const char *arg = argv[i];

        if (strcmp(arg, "--test") == 0) {
            if (++i >= argc)
                return set_required_option_once(&cmd->function_name, NULL, "--test");

            int rc = set_required_option_once(&cmd->function_name, argv[i], "--test");
            if (rc)
                return rc;
        }
        else if (strncmp(arg, "--test=", strlen("--test=")) == 0) {
            int rc = set_required_option_once(&cmd->function_name, arg + strlen("--test="), "--test");
            if (rc)
                return rc;
        }
        else if (strcmp(arg, "--dir") == 0) {
            if (++i >= argc)
                return set_required_option_once(&cmd->backend_dir, NULL, "--dir");

            int rc = set_required_option_once(&cmd->backend_dir, argv[i], "--dir");
            if (rc)
                return rc;
        }
        else if (strncmp(arg, "--dir=", strlen("--dir=")) == 0) {
            int rc = set_required_option_once(&cmd->backend_dir, arg + strlen("--dir="), "--dir");
            if (rc)
                return rc;
        }
        else if (strcmp(arg, "--request") == 0) {
            return reject_request_option();
        }
        else if (strncmp(arg, "--request=", strlen("--request=")) == 0) {
            return reject_request_option();
        }
        else if (strcmp(arg, "--timeout") == 0) {
            if (++i >= argc)
                return set_timeout_option_once(&cmd->timeout_seconds, &cmd->timeout_seconds_set, NULL);

            int rc = set_timeout_option_once(&cmd->timeout_seconds, &cmd->timeout_seconds_set, argv[i]);
            if (rc)
                return rc;
        }
        else if (strncmp(arg, "--timeout=", strlen("--timeout=")) == 0) {
            int rc = set_timeout_option_once(
                &cmd->timeout_seconds, &cmd->timeout_seconds_set, arg + strlen("--timeout="));
            if (rc)
                return rc;
        }
        else if (strcmp(arg, "-h") == 0 || strcmp(arg, "--help") == 0) {
            systemd_journal_test_usage(stderr);
            return 2;
        }
        else {
            fprintf(stderr, "unsupported systemd journal test option '%s'\n", arg);
            systemd_journal_test_usage(stderr);
            return 2;
        }
    }

    if (!cmd->function_name) {
        fprintf(stderr, "missing required --test\n");
        systemd_journal_test_usage(stderr);
        return 2;
    }

    if (!cmd->backend_dir) {
        fprintf(stderr, "missing required --dir\n");
        systemd_journal_test_usage(stderr);
        return 2;
    }

    if (!cmd->timeout_seconds_set)
        cmd->timeout_seconds = ND_SD_JOURNAL_DEFAULT_TIMEOUT;

    return 0;
}

static uint64_t systemd_journal_effective_timeout_seconds(uint64_t timeout_seconds)
{
    return timeout_seconds ? timeout_seconds : ND_SD_JOURNAL_TEST_TIMEOUT_DISABLED_SECONDS;
}

static usec_t systemd_journal_test_stop_monotonic_usec(uint64_t timeout_seconds)
{
    usec_t now_ut = now_monotonic_usec();
    uint64_t effective_timeout_seconds = systemd_journal_effective_timeout_seconds(timeout_seconds);
    uint64_t max_timeout_seconds = (UINT64_MAX - now_ut) / USEC_PER_SEC;

    if (effective_timeout_seconds > max_timeout_seconds)
        return UINT64_MAX;

    return now_ut + effective_timeout_seconds * USEC_PER_SEC;
}

static DIR *open_systemd_journal_test_backend_directory(const char *path, char *fd_path, size_t fd_path_size)
{
    struct stat path_st, dir_st;

    // Pin the explicit --dir backend root; symlinked journal trees are handled by the shared scanner.
    if (!path || !*path) {
        errno = EINVAL;
        return NULL;
    }

    if (lstat(path, &path_st) == -1)
        return NULL;

    if (S_ISLNK(path_st.st_mode)) {
        errno = ELOOP;
        return NULL;
    }

    if (!S_ISDIR(path_st.st_mode)) {
        errno = ENOTDIR;
        return NULL;
    }

    DIR *dir = opendir(path);
    if (!dir)
        return NULL;

    int fd = dirfd(dir);
    if (fd == -1) {
        int saved_errno = errno;
        closedir(dir);
        errno = saved_errno;
        return NULL;
    }

    if (fstat(fd, &dir_st) == -1) {
        int saved_errno = errno;
        closedir(dir);
        errno = saved_errno;
        return NULL;
    }

    if (!S_ISDIR(dir_st.st_mode)) {
        closedir(dir);
        errno = ENOTDIR;
        return NULL;
    }

    if (path_st.st_dev != dir_st.st_dev || path_st.st_ino != dir_st.st_ino) {
        closedir(dir);
        errno = EAGAIN;
        return NULL;
    }

    int written = snprintfz(fd_path, fd_path_size, "/proc/self/fd/%d", fd);
    if (written < 0 || (size_t)written >= fd_path_size) {
        closedir(dir);
        errno = ENAMETOOLONG;
        return NULL;
    }

    return dir;
}

static BUFFER *read_request_payload_from_stdin(void)
{
    BUFFER *payload = buffer_create(8192, NULL);
    size_t total = 0;
    while (true) {
        char buffer[8192];
        ssize_t bytes_read = read(STDIN_FILENO, buffer, sizeof(buffer));
        if (bytes_read == -1) {
            if (errno == EINTR)
                continue;

            fprintf(stderr, "failed to read request payload from stdin: %s\n", strerror(errno));
            buffer_free(payload);
            return NULL;
        }

        if (bytes_read == 0)
            break;

        if ((uint64_t)total + (uint64_t)bytes_read > ND_SD_JOURNAL_TEST_MAX_REQUEST_BYTES) {
            fprintf(
                stderr,
                "request payload from stdin is too large: max %llu bytes\n",
                (unsigned long long)ND_SD_JOURNAL_TEST_MAX_REQUEST_BYTES);
            buffer_free(payload);
            return NULL;
        }

        buffer_memcat(payload, buffer, (size_t)bytes_read);
        total += (size_t)bytes_read;
    }

    if (total == 0) {
        fprintf(stderr, "request payload from stdin is empty\n");
        buffer_free(payload);
        return NULL;
    }

    payload->content_type = CT_APPLICATION_JSON;

    return payload;
}

static int run_systemd_journal_test_command(const struct systemd_journal_test_command *cmd)
{
    if (strcmp(cmd->function_name, ND_SD_JOURNAL_FUNCTION_NAME) != 0) {
        fprintf(
            stderr,
            "unsupported systemd journal test function '%s' (expected '%s')\n",
            cmd->function_name,
            ND_SD_JOURNAL_FUNCTION_NAME);
        return 2;
    }

    char backend_dir_path[FILENAME_MAX];
    DIR *backend_dir =
        open_systemd_journal_test_backend_directory(cmd->backend_dir, backend_dir_path, sizeof(backend_dir_path));
    if (!backend_dir) {
        fprintf(
            stderr,
            "systemd journal backend directory '%s' cannot be opened: %s\n",
            cmd->backend_dir,
            strerror(errno));
        return 1;
    }

    CLEAN_BUFFER *payload = read_request_payload_from_stdin();
    if (!payload) {
        closedir(backend_dir);
        return 1;
    }

    bool cancelled = false;
    usec_t stop_monotonic_ut = systemd_journal_test_stop_monotonic_usec(cmd->timeout_seconds);

    nd_journal_set_scan_progress_enabled(false);
    nd_journal_use_single_directory(backend_dir_path);
    nd_journal_files_registry_update();

    char *function = strdupz(cmd->function_name);
    BUFFER *result = function_systemd_journal_result(
        "test", function, &stop_monotonic_ut, &cancelled, payload, HTTP_ACCESS_ALL, "test-cli", NULL);
    freez(function);

    int rc = 1;
    if (result) {
        if (buffer_strlen(result))
            fwrite(buffer_tostring(result), buffer_strlen(result), 1, stdout);
        fprintf(stdout, "\n");
        fflush(stdout);

        if (result->response_code >= HTTP_RESP_OK && result->response_code < 300)
            rc = 0;

        buffer_free(result);
    }
    else {
        fprintf(stderr, "systemd journal test function returned no result\n");
    }

    closedir(backend_dir);
    return rc;
}

static bool journal_data_directories_exist()
{
    struct stat st;
    for (unsigned i = 0; i < MAX_JOURNAL_DIRECTORIES && journal_directories[i].path; i++) {
        if ((stat(string2str(journal_directories[i].path), &st) == 0) && S_ISDIR(st.st_mode))
            return true;
    }
    return false;
}

int main(int argc, char **argv)
{
    struct systemd_journal_test_command test_command = {0};
    int test_parse_rc = parse_systemd_journal_test_command(argc, argv, &test_command);
    if (test_parse_rc)
        exit(test_parse_rc);

    nd_thread_tag_set("sd-jrnl.plugin");
    nd_log_initialize_for_external_plugins("systemd-journal.plugin");

    if(!netdata_configured_host_prefix)
        netdata_configured_host_prefix = "";
    if (verify_netdata_host_prefix(true) == -1)
        exit(1);

    // ------------------------------------------------------------------------
    // initialization

    nd_sd_journal_annotations_init();
    nd_journal_init_files_and_directories();

    if (test_command.enabled)
        exit(run_systemd_journal_test_command(&test_command));

    if (!journal_data_directories_exist()) {
        nd_log_collector(NDLP_INFO, "unable to locate journal data directories. Exiting...");
        fprintf(stdout, "DISABLE\n");
        fflush(stdout);
        exit(0);
    }

    // ------------------------------------------------------------------------
    // debug

    if (argc == 2 && strcmp(argv[1], "debug") == 0) {
        nd_journal_files_registry_update();

        bool cancelled = false;
        usec_t stop_monotonic_ut = now_monotonic_usec() + 600 * USEC_PER_SEC;
        // "__logs_sources" is LQS_PARAMETER_SOURCE (logs_query_status.h); a plain
        // "source:" is parsed as a filter on a user field named "source" and matches nothing
        char buf[] =
            "systemd-journal after:-8640000 before:0 direction:backward last:200 data_only:false slice:true facets: __logs_sources:all";
        function_systemd_journal("123", buf, &stop_monotonic_ut, &cancelled, NULL, HTTP_ACCESS_ALL, NULL, NULL);
        exit(1);
    }

    if(nd_environment_freeze_process() != 0)
        fatal("Cannot freeze the process environment: %s", strerror(errno));

    netdata_threads_init_for_external_plugins(0);

    // ------------------------------------------------------------------------
    // watcher thread

    nd_thread_create("SDWATCH", NETDATA_THREAD_OPTION_DONT_LOG, nd_journal_watcher_main, NULL);

    // ------------------------------------------------------------------------
    // the event loop for functions

    struct functions_evloop_globals *wg =
        functions_evloop_init(ND_SD_JOURNAL_WORKER_THREADS, "SDJ", &stdout_mutex, &plugin_should_exit, NULL);

    functions_evloop_add_function(
        wg, ND_SD_JOURNAL_FUNCTION_NAME, function_systemd_journal, ND_SD_JOURNAL_DEFAULT_TIMEOUT, NULL);

    nd_systemd_journal_dyncfg_init(wg);

    // ------------------------------------------------------------------------
    // register functions to netdata

    netdata_mutex_lock(&stdout_mutex);

    fprintf(
        stdout,
        PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"logs\" " HTTP_ACCESS_FORMAT " %d\n",
        ND_SD_JOURNAL_FUNCTION_NAME,
        ND_SD_JOURNAL_DEFAULT_TIMEOUT,
        ND_SD_JOURNAL_FUNCTION_DESCRIPTION,
        (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
        RRDFUNCTIONS_PRIORITY_DEFAULT);

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);

    // ------------------------------------------------------------------------

    usec_t send_newline_ut = 0;
    usec_t since_last_scan_ut =
        ND_SD_JOURNAL_ALL_FILES_SCAN_EVERY_USEC * 2; // something big to trigger scanning at start
    const bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!__atomic_load_n(&plugin_should_exit, __ATOMIC_ACQUIRE)) {
        if (since_last_scan_ut > ND_SD_JOURNAL_ALL_FILES_SCAN_EVERY_USEC) {
            nd_journal_files_registry_update();
            since_last_scan_ut = 0;
        }

        usec_t dt_ut = heartbeat_next(&hb);
        since_last_scan_ut += dt_ut;
        send_newline_ut += dt_ut;

        if (!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    exit(0);
}
