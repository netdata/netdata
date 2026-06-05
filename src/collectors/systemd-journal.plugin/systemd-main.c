// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

#include <errno.h>
#include <limits.h>

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
    const char *request_path;
    uint64_t timeout_seconds;
    bool timeout_seconds_set;
};

static void systemd_journal_test_usage(FILE *stream)
{
    fprintf(
        stream,
        "usage: systemd-journal.plugin --test systemd-journal --dir <journal-dir> --request <payload.json> [--timeout <seconds>]\n");
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
            if (++i >= argc)
                return set_required_option_once(&cmd->request_path, NULL, "--request");

            int rc = set_required_option_once(&cmd->request_path, argv[i], "--request");
            if (rc)
                return rc;
        }
        else if (strncmp(arg, "--request=", strlen("--request=")) == 0) {
            int rc = set_required_option_once(&cmd->request_path, arg + strlen("--request="), "--request");
            if (rc)
                return rc;
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

    if (!cmd->request_path) {
        fprintf(stderr, "missing required --request\n");
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

static int validate_systemd_journal_test_executable_permissions(void)
{
    CLEAN_CHAR_P *plugin_path = os_get_process_path();
    if (!plugin_path || !*plugin_path) {
        fprintf(stderr, "refusing systemd journal test mode: failed to resolve plugin executable path\n");
        return 2;
    }

    struct stat st;
    if (stat(plugin_path, &st) != 0) {
        fprintf(
            stderr,
            "refusing systemd journal test mode: failed to inspect plugin executable '%s': %s\n",
            plugin_path,
            strerror(errno));
        return 2;
    }

    if (st.st_mode & S_IXOTH) {
        fprintf(
            stderr,
            "refusing systemd journal test mode: plugin executable '%s' is world-executable\n",
            plugin_path);
        return 2;
    }

    return 0;
}

static bool path_is_directory(const char *path)
{
    struct stat st;
    return path && stat(path, &st) == 0 && S_ISDIR(st.st_mode);
}

static BUFFER *read_request_payload(const char *filename)
{
#if !defined(O_NOFOLLOW) || !defined(O_NONBLOCK)
    fprintf(
        stderr,
        "refusing request payload '%s': platform does not support safe request payload open flags\n",
        filename);
    return NULL;
#else

    int flags = O_RDONLY | O_CLOEXEC | O_NOFOLLOW | O_NONBLOCK;
    int fd = open(filename, flags);
    if (fd == -1) {
        fprintf(stderr, "failed to open request payload '%s': %s\n", filename, strerror(errno));
        return NULL;
    }

    CLEAN_CHAR_P *resolved = realpath(filename, NULL);
    const char *display_path = resolved ? resolved : filename;

    struct stat st;
    if (fstat(fd, &st) == -1) {
        fprintf(stderr, "failed to inspect request payload '%s': %s\n", display_path, strerror(errno));
        close(fd);
        return NULL;
    }

    if (!S_ISREG(st.st_mode)) {
        fprintf(stderr, "request payload '%s' is not a regular file\n", display_path);
        close(fd);
        return NULL;
    }

    if (st.st_size <= 0) {
        fprintf(stderr, "request payload '%s' is empty\n", display_path);
        close(fd);
        return NULL;
    }

    if ((uint64_t)st.st_size > ND_SD_JOURNAL_TEST_MAX_REQUEST_BYTES) {
        fprintf(
            stderr,
            "request payload '%s' is too large: %llu bytes, max %llu bytes\n",
            display_path,
            (unsigned long long)st.st_size,
            (unsigned long long)ND_SD_JOURNAL_TEST_MAX_REQUEST_BYTES);
        close(fd);
        return NULL;
    }

    BUFFER *payload = buffer_create((size_t)st.st_size + 1, NULL);
    size_t total = 0;
    while (total < (size_t)st.st_size) {
        char buffer[8192];
        size_t remaining = (size_t)st.st_size - total;
        size_t wanted = MIN(remaining, sizeof(buffer));
        ssize_t bytes_read = read(fd, buffer, wanted);
        if (bytes_read == -1) {
            if (errno == EINTR)
                continue;

            fprintf(stderr, "failed to read request payload '%s': %s\n", display_path, strerror(errno));
            buffer_free(payload);
            close(fd);
            return NULL;
        }

        if (bytes_read == 0) {
            fprintf(stderr, "request payload '%s' changed while reading\n", display_path);
            buffer_free(payload);
            close(fd);
            return NULL;
        }

        buffer_memcat(payload, buffer, (size_t)bytes_read);
        total += (size_t)bytes_read;
    }

    close(fd);
    payload->content_type = CT_APPLICATION_JSON;

    return payload;
#endif
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

    if (!path_is_directory(cmd->backend_dir)) {
        fprintf(stderr, "systemd journal backend directory '%s' does not exist\n", cmd->backend_dir);
        return 1;
    }

    // The request path is intentionally caller-selected for offline fixtures. Test mode refuses world-executable
    // privileged plugin binaries, and the path is opened with no-final-symlink and nonblocking flags, size-limited,
    // and checked as a regular file.
    //
    // codeql[cpp/path-injection]
    CLEAN_BUFFER *payload = read_request_payload(cmd->request_path);
    if (!payload)
        return 1;

    nd_journal_set_scan_progress_enabled(false);
    nd_journal_use_single_directory(cmd->backend_dir);
    nd_journal_files_registry_update();

    bool cancelled = false;
    usec_t stop_monotonic_ut = systemd_journal_test_stop_monotonic_usec(cmd->timeout_seconds);
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

    if (test_command.enabled) {
        int permissions_rc = validate_systemd_journal_test_executable_permissions();
        if (permissions_rc)
            exit(permissions_rc);
    }

    nd_thread_tag_set("sd-jrnl.plugin");
    nd_log_initialize_for_external_plugins("systemd-journal.plugin");
    netdata_threads_init_for_external_plugins(0);

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
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
        char buf[] =
            "systemd-journal after:-8640000 before:0 direction:backward last:200 data_only:false slice:true facets: source:all";
        function_systemd_journal("123", buf, &stop_monotonic_ut, &cancelled, NULL, HTTP_ACCESS_ALL, NULL, NULL);
        exit(1);
    }

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
