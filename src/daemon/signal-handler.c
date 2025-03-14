// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "daemon/daemon-status-file.h"

static void (*original_handlers[NSIG])(int) = {0};
static void (*original_sigactions[NSIG])(int, siginfo_t *, void *) = {0};

typedef enum signal_action {
    NETDATA_SIGNAL_IGNORE,
    NETDATA_SIGNAL_EXIT_CLEANLY,
#if defined(FSANITIZE_ADDRESS)
    NETDATA_SIGNAL_EXIT_NOW,
#endif
    NETDATA_SIGNAL_REOPEN_LOGS,
    NETDATA_SIGNAL_RELOAD_HEALTH,
    NETDATA_SIGNAL_DEADLY,
} SIGNAL_ACTION;

static struct {
    int signo;              // the signal
    int flags;              // the sigaction flags to use
    const char *name;       // the name of the signal
    size_t count;           // the number of signals received
    SIGNAL_ACTION action;   // the action to take
    EXIT_REASON reason;
} signals_waiting[] = {
    { SIGPIPE, 0, "SIGPIPE", 0, NETDATA_SIGNAL_IGNORE, EXIT_REASON_NONE },
    { SIGINT , 0, "SIGINT",  0, NETDATA_SIGNAL_EXIT_CLEANLY, EXIT_REASON_SIGINT },
    { SIGQUIT, 0, "SIGQUIT", 0, NETDATA_SIGNAL_EXIT_CLEANLY, EXIT_REASON_SIGQUIT },
    { SIGTERM, 0, "SIGTERM", 0, NETDATA_SIGNAL_EXIT_CLEANLY, EXIT_REASON_SIGTERM },
    { SIGHUP,  0, "SIGHUP",  0, NETDATA_SIGNAL_REOPEN_LOGS, EXIT_REASON_NONE },
#if defined(FSANITIZE_ADDRESS)
    { SIGUSR1, 0, "SIGUSR1", 0, NETDATA_SIGNAL_EXIT_NOW, EXIT_REASON_NONE },
#endif
    { SIGUSR2, 0, "SIGUSR2", 0, NETDATA_SIGNAL_RELOAD_HEALTH, EXIT_REASON_NONE },
    { SIGBUS,  SA_SIGINFO, "SIGBUS",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGBUS },
    { SIGSEGV, SA_SIGINFO, "SIGSEGV", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGSEGV },
    { SIGFPE,  SA_SIGINFO, "SIGFPE",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGFPE },
    { SIGILL,  SA_SIGINFO, "SIGILL",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGILL },
    { SIGABRT, 0, "SIGABRT", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGABRT },
    { SIGSYS,  0, "SIGSYS",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGSYS },
    { SIGXCPU, 0, "SIGXCPU", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGXCPU },
    { SIGXFSZ, 0, "SIGXFSZ", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGXFSZ },
#ifdef SIGEMT
    { SIGEMT,  "SIGEMT",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGEMT },
#endif
};

void signal_handler_with_info(int signo, siginfo_t *info, void *context __maybe_unused) {
    for(size_t i = 0; i < _countof(signals_waiting) ; i++) {
        if(signals_waiting[i].signo != signo)
            continue;

        signals_waiting[i].count++;

#if defined(FSANITIZE_ADDRESS)
        if(signals_waiting[i].action == NETDATA_SIGNAL_EXIT_NOW)
            exit(1);
#endif

        if(signals_waiting[i].action == NETDATA_SIGNAL_DEADLY) {
            // Update the status file
            SIGNAL_CODE sc = info ? signal_code(signo, info->si_code) : 0;
            daemon_status_file_deadly_signal_received(signals_waiting[i].reason, sc);

            // log it
            char b[512];
            strncpyz(b, "SIGNAL HANDLER: received deadly signal: ", sizeof(b) - 1);
            strcat(b, signals_waiting[i].name);
            if(sc) {
                strcat(b, " (");
                strcat(b, SIGNAL_CODE_2str(sc));
                strcat(b, ")");
            }
            strcat(b, " in thread ");
            print_uint64(&b[strlen(b)], gettid_cached());
            strcat(b, " ");
            strcat(b, nd_thread_tag_async_safe());
            strcat(b, "!\n");

            if(write(STDERR_FILENO, b, strlen(b)) == -1) {
                // nothing to do - we cannot write but there is no way to complain about it
                ;
            }

            // Chain to the original handler if it exists
            if (original_sigactions[signo] && info) {
                original_sigactions[signo](signo, info, context);
                return; // Original handler should handle the signal
            }

            if (original_handlers[signo] && original_handlers[signo] != SIG_IGN && original_handlers[signo] != SIG_DFL) {
                original_handlers[signo](signo);
                return; // Original handler should handle the signal
            }

            // If there's no original handler or we can't chain, reset to default and re-raise
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

static void signal_handler(int signo) {
    signal_handler_with_info(signo, NULL, NULL);
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

void nd_initialize_signals(bool chain_existing) {
    signals_block_all_except_deadly();

    struct sigaction act;
    memset(&act, 0, sizeof(struct sigaction));

    // ignore all signals while we run in a signal handler
    sigfillset(&act.sa_mask);

    for (size_t i = 0; i < _countof(signals_waiting); i++) {
        int signo = signals_waiting[i].signo;

        // If chaining is requested, get the current handler first
        struct sigaction old_act;
        if (chain_existing &&
            sigaction(signo, NULL, &old_act) == 0 &&
            (uintptr_t)old_act.sa_handler != (uintptr_t)signal_handler &&
            (uintptr_t)old_act.sa_handler != (uintptr_t)signal_handler_with_info) {
            // Save the original handlers for chaining
            if (old_act.sa_flags & SA_SIGINFO)
                original_sigactions[signo] = old_act.sa_sigaction;
            else
                original_handlers[signo] = old_act.sa_handler;
        }

        act.sa_flags = signals_waiting[i].flags;

        switch (signals_waiting[i].action) {
            case NETDATA_SIGNAL_IGNORE:
                act.sa_handler = SIG_IGN;
                break;
            default:
                if (act.sa_flags == SA_SIGINFO)
                    act.sa_sigaction = signal_handler_with_info;
                else
                    act.sa_handler = signal_handler;
                break;
        }

        if (sigaction(signals_waiting[i].signo, &act, NULL) == -1)
            netdata_log_error("SIGNAL: Failed to change signal handler for: %s", signals_waiting[i].name);
    }
}

static void process_triggered_signals(void) {
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
                    daemon_status_file_deadly_signal_received(signals_waiting[i].reason, 0);
                    _exit(1);
                    break;

                default:
                    netdata_log_info("SIGNAL: Received %s. No signal handler configured. Ignoring it.", name);
                    break;
            }
        }
    } while(found);
}

void nd_process_signals(void) {
    posix_unmask_my_signals();
    const usec_t save_every_ut = 15 * 60 * USEC_PER_SEC;
    usec_t last_update_mt = now_monotonic_usec();

    while (true) {
        usec_t mt = now_monotonic_usec();
        if ((mt - last_update_mt) >= save_every_ut) {
            daemon_status_file_update_status(DAEMON_STATUS_NONE);
            last_update_mt += save_every_ut;
        }

        poll(NULL, 0, 13 * MSEC_PER_SEC + 379);
        process_triggered_signals();
    }
}
