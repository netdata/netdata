// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"
#include "libnetdata/required_dummies.h"

#define SYSTEMD_JOURNAL_WORKER_THREADS          5

netdata_mutex_t stdout_mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;

int main(int argc __maybe_unused, char **argv __maybe_unused) {
    clocks_init();
    nd_log_initialize_for_external_plugins("systemd-journal.plugin");

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix() == -1) exit(1);

    // ------------------------------------------------------------------------
    // initialization

    netdata_systemd_journal_message_ids_init();
    journal_init_query_status();
    journal_init_files_and_directories();

    // ------------------------------------------------------------------------
    // debug

    if(argc == 2 && strcmp(argv[1], "debug") == 0) {
        bool cancelled = false;
        char buf[] = "systemd-journal after:-8640000 before:0 direction:backward last:200 data_only:false slice:true source:all";
        // char buf[] = "systemd-journal after:1695332964 before:1695937764 direction:backward last:100 slice:true source:all DHKucpqUoe1:PtVoyIuX.MU";
        // char buf[] = "systemd-journal after:1694511062 before:1694514662 anchor:1694514122024403";
        function_systemd_journal("123", buf, 600, &cancelled);
//        function_systemd_units("123", "systemd-units", 600, &cancelled);
        exit(1);
    }
#ifdef ENABLE_SYSTEMD_DBUS
    if(argc == 2 && strcmp(argv[1], "debug-units") == 0) {
        bool cancelled = false;
        function_systemd_units("123", "systemd-units", 600, &cancelled);
        exit(1);
    }
#endif

    // ------------------------------------------------------------------------
    // the event loop for functions

    struct functions_evloop_globals *wg =
            functions_evloop_init(SYSTEMD_JOURNAL_WORKER_THREADS, "SDJ", &stdout_mutex, &plugin_should_exit);

    functions_evloop_add_function(wg, SYSTEMD_JOURNAL_FUNCTION_NAME, function_systemd_journal,
            SYSTEMD_JOURNAL_DEFAULT_TIMEOUT);

#ifdef ENABLE_SYSTEMD_DBUS
    functions_evloop_add_function(wg, SYSTEMD_UNITS_FUNCTION_NAME, function_systemd_units,
            SYSTEMD_UNITS_DEFAULT_TIMEOUT);
#endif

    // ------------------------------------------------------------------------
    // register functions to netdata

    netdata_mutex_lock(&stdout_mutex);

    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\"\n",
            SYSTEMD_JOURNAL_FUNCTION_NAME, SYSTEMD_JOURNAL_DEFAULT_TIMEOUT, SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);

#ifdef ENABLE_SYSTEMD_DBUS
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\"\n",
            SYSTEMD_UNITS_FUNCTION_NAME, SYSTEMD_UNITS_DEFAULT_TIMEOUT, SYSTEMD_UNITS_FUNCTION_DESCRIPTION);
#endif

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);

    // ------------------------------------------------------------------------

    usec_t step_ut = 100 * USEC_PER_MS;
    usec_t send_newline_ut = 0;
    usec_t since_last_scan_ut = 1000 * USEC_PER_SEC; // something big to trigger scanning at start
    bool tty = isatty(fileno(stderr)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!plugin_should_exit) {

        if(since_last_scan_ut > 60 * USEC_PER_SEC) {
            journal_files_registry_update();
            since_last_scan_ut = 0;
        }

        usec_t dt_ut = heartbeat_next(&hb, step_ut);
        since_last_scan_ut += dt_ut;
        send_newline_ut += dt_ut;

        if(!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush();
            send_newline_ut = 0;
        }
    }

    exit(0);
}
