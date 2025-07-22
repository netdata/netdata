// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"
#include "libnetdata/required_dummies.h"

#define ND_SD_JOURNAL_WORKER_THREADS 5

netdata_mutex_t stdout_mutex;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&stdout_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&stdout_mutex);
}

static bool plugin_should_exit = false;

static bool journal_data_directories_exist()
{
    struct stat st;
    for (unsigned i = 0; i < MAX_JOURNAL_DIRECTORIES && journal_directories[i].path; i++) {
        if ((stat(string2str(journal_directories[i].path), &st) == 0) && S_ISDIR(st.st_mode))
            return true;
    }
    return false;
}

int main(int argc __maybe_unused, char **argv __maybe_unused)
{
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
        functions_evloop_init(ND_SD_JOURNAL_WORKER_THREADS, "SDJ", &stdout_mutex, &plugin_should_exit);

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
    while (!plugin_should_exit) {
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
