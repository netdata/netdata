// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "daemon/daemon-status-file.h"

typedef enum signal_action {
    NETDATA_SIGNAL_IGNORE,
    NETDATA_SIGNAL_EXIT_CLEANLY,
    NETDATA_SIGNAL_REOPEN_LOGS,
    NETDATA_SIGNAL_RELOAD_HEALTH,
    NETDATA_SIGNAL_DEADLY,
} SIGNAL_ACTION;

static struct {
    int signo;              // the signal
    const char *name;       // the name of the signal
    size_t count;           // the number of signals received
    SIGNAL_ACTION action;   // the action to take
    EXIT_REASON reason;
} signals_waiting[] = {
    { SIGPIPE, "SIGPIPE", 0, NETDATA_SIGNAL_IGNORE, EXIT_REASON_NONE },
    { SIGINT , "SIGINT",  0, NETDATA_SIGNAL_EXIT_CLEANLY, EXIT_REASON_SIGINT },
    { SIGQUIT, "SIGQUIT", 0, NETDATA_SIGNAL_EXIT_CLEANLY, EXIT_REASON_SIGQUIT },
    { SIGTERM, "SIGTERM", 0, NETDATA_SIGNAL_EXIT_CLEANLY, EXIT_REASON_SIGTERM },
    { SIGHUP,  "SIGHUP",  0, NETDATA_SIGNAL_REOPEN_LOGS, EXIT_REASON_NONE },
    { SIGUSR2, "SIGUSR2", 0, NETDATA_SIGNAL_RELOAD_HEALTH, EXIT_REASON_NONE },
    { SIGBUS,  "SIGBUS",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGBUS },
    { SIGSEGV, "SIGSEGV", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGSEGV },
    { SIGFPE,  "SIGFPE",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGFPE },
    { SIGILL,  "SIGILL",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGILL },
    { SIGABRT, "SIGABRT", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGABRT },
};

static void signal_handler(int signo) {
    for(size_t i = 0; i < _countof(signals_waiting) ; i++) {
        if(signals_waiting[i].signo != signo)
            continue;

        signals_waiting[i].count++;

        if(signals_waiting[i].action == NETDATA_SIGNAL_DEADLY) {
            // Update the status file
            daemon_status_file_deadly_signal_received(signals_waiting[i].reason);

            // log it
            char buffer[200 + 1];
            snprintfz(buffer, sizeof(buffer) - 1, "\nSIGNAL HANDLER: received: %s in thread %d!\n",
                      signals_waiting[i].name, gettid_cached());

            if(write(STDERR_FILENO, buffer, strlen(buffer)) == -1) {
                // nothing to do - we cannot write but there is no way to complain about it
                ;
            }

            // Reset the signal's disposition to the default handler.

            struct sigaction sa;
            sa.sa_handler = SIG_DFL;
            sigemptyset(&sa.sa_mask);
            sa.sa_flags = 0;
            sigaction(signo, &sa, NULL);

            // Re-raise the signal, which now uses the default action.
            raise(signo);
        }

        break;
    }
}

// Unmask all signals the netdata main signal handler uses.
// All other signals remain masked.
static void posix_unmask_my_signals(void) {
    sigset_t sigset;
    sigemptyset(&sigset);

    for (size_t i = 0; i < _countof(signals_waiting) ; i++)
        sigaddset(&sigset, signals_waiting[i].signo);

    if (pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) != 0)
        netdata_log_error("SIGNAL: cannot unmask netdata signals");
}

void nd_initialize_signals(void) {
    signals_block_all_except_deadly();

    // Catch signals which we want to use
    struct sigaction sa;
    sa.sa_flags = 0;

    // ignore all signals while we run in a signal handler
    sigfillset(&sa.sa_mask);

    for (size_t i = 0; i < _countof(signals_waiting) ; i++) {
        switch (signals_waiting[i].action) {
        case NETDATA_SIGNAL_IGNORE:
            sa.sa_handler = SIG_IGN;
            break;
        default:
            sa.sa_handler = signal_handler;
            break;
        }

        if(sigaction(signals_waiting[i].signo, &sa, NULL) == -1)
            netdata_log_error("SIGNAL: Failed to change signal handler for: %s", signals_waiting[i].name);
    }
}

void nd_process_signals(void) {
    posix_unmask_my_signals();

    while(true) {
        // pause()  causes  the calling process (or thread) to sleep until a signal
        // is delivered that either terminates the process or causes the invocation
        // of a signal-catching function.
        if(pause() == -1 && errno == EINTR) {
            daemon_status_file_update_status(DAEMON_STATUS_NONE);
            errno_clear();

            // loop once, but keep looping while signals are coming in,
            // this is needed because a few operations may take some time
            // so we need to check for new signals before pausing again
            size_t found;
            do {
                found = 0;
                for (size_t i = 0; i < _countof(signals_waiting) ; i++) {
                    if (!signals_waiting[i].count)
                        continue;

                    found++;
                    signals_waiting[i].count = 0;
                    const char *name = signals_waiting[i].name;

                    switch (signals_waiting[i].action) {
                        case NETDATA_SIGNAL_RELOAD_HEALTH:
                            nd_log_limits_unlimited();
                            netdata_log_info("SIGNAL: Received %s. Reloading HEALTH configuration...", name);
                            nd_log_limits_reset();
                            execute_command(CMD_RELOAD_HEALTH, NULL, NULL);
                            break;

                        case NETDATA_SIGNAL_REOPEN_LOGS:
                            nd_log_limits_unlimited();
                            netdata_log_info("SIGNAL: Received %s. Reopening all log files...", name);
                            nd_log_limits_reset();
                            execute_command(CMD_REOPEN_LOGS, NULL, NULL);
                            break;

                        case NETDATA_SIGNAL_EXIT_CLEANLY:
                            nd_log_limits_unlimited();
                            netdata_log_info("SIGNAL: Received %s. Cleaning up to exit...", name);
                            commands_exit();
                            netdata_cleanup_and_exit(signals_waiting[i].reason, NULL, NULL, NULL);
                            exit(0);
                            break;

                        case NETDATA_SIGNAL_DEADLY:
                            nd_log_limits_unlimited();
                            daemon_status_file_deadly_signal_received(signals_waiting[i].reason);
                            _exit(1);
                            break;

                        default:
                            netdata_log_info("SIGNAL: Received %s. No signal handler configured. Ignoring it.", name);
                            break;
                    }
                }
            } while(found);
        }
        else
            netdata_log_error("SIGNAL: pause() returned but it was not interrupted by a signal.");
    }
}
