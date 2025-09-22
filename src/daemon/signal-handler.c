// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "daemon/status-file.h"
#include "protected-access.h"

#ifdef ENABLE_SENTRY
#include "sentry-native/sentry-native.h"
#endif

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
#if defined(FSANITIZE_ADDRESS)
    { SIGUSR1, "SIGUSR1", 0, NETDATA_SIGNAL_EXIT_NOW, EXIT_REASON_NONE },
#endif
    { SIGUSR2, "SIGUSR2", 0, NETDATA_SIGNAL_RELOAD_HEALTH, EXIT_REASON_NONE },
    { SIGBUS,  "SIGBUS",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGBUS },
    { SIGSEGV, "SIGSEGV", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGSEGV },
    { SIGFPE,  "SIGFPE",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGFPE },
    { SIGILL,  "SIGILL",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGILL },
    { SIGABRT, "SIGABRT", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGABRT },
    { SIGSYS,  "SIGSYS",  0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGSYS },
    { SIGXCPU, "SIGXCPU", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGXCPU },
    { SIGXFSZ, "SIGXFSZ", 0, NETDATA_SIGNAL_DEADLY, EXIT_REASON_SIGXFSZ },
};

static void (*original_handlers[NSIG])(int) = {0};
static void (*original_sigactions[NSIG])(int, siginfo_t *, void *) = {0};

NEVER_INLINE
void nd_signal_handler(int signo, siginfo_t *info, void *context __maybe_unused) {
    signal_protected_access_check(signo, info, context);

    for(size_t i = 0; i < _countof(signals_waiting) ; i++) {
        if(signals_waiting[i].signo != signo)
            continue;

        signals_waiting[i].count++;

#if defined(FSANITIZE_ADDRESS)
        if(signals_waiting[i].action == NETDATA_SIGNAL_EXIT_NOW)
            exit(1);
#endif

        if(signals_waiting[i].action == NETDATA_SIGNAL_DEADLY) {
            bool chained_handler = original_sigactions[signo] || (original_handlers[signo] && original_handlers[signo] != SIG_IGN && original_handlers[signo] != SIG_DFL);

            // Update the status file
            SIGNAL_CODE sc = info ? signal_code(signo, info->si_code) : 0;

            // Get fault address based on signal type
            void *fault_address = NULL;
            if (info && (signo == SIGSEGV || signo == SIGBUS || signo == SIGILL || signo == SIGFPE))
                fault_address = info->si_addr;

            if(daemon_status_file_deadly_signal_received(signals_waiting[i].reason, sc, fault_address, chained_handler)) {
                // this is a duplicate event, do not send it to sentry
#ifdef ENABLE_SENTRY
                nd_sentry_crash_report(false);
#else
                chained_handler = false;
#endif
            }

            // log it
            char b[1024];
            size_t len = 0;
            len = strcatz(b, len, "SIGNAL HANDLER: received deadly signal: ", sizeof(b));
            len = strcatz(b, len, signals_waiting[i].name, sizeof(b));
            if(sc) {
                char buf[128];
                SIGNAL_CODE_2str_h(sc, buf, sizeof(buf));
                len = strcatz(b, len, " (", sizeof(b));
                len = strcatz(b, len, buf, sizeof(b));
                len = strcatz(b, len, ")", sizeof(b));
            }
            len = strcatz(b, len, " in thread ", sizeof(b));
            print_uint64(&b[len], gettid_cached());
            len = strcatz(b, len, " ", sizeof(b));
            len = strcatz(b, len, nd_thread_tag_async_safe(), sizeof(b));
            len = strcatz(b, len, "!\n", sizeof(b));

            if(write(STDERR_FILENO, b, strlen(b)) == -1) {
                // nothing to do - we cannot write but there is no way to complain about it
                ;
            }

            // Chain to the original handler if it exists
            if(chained_handler) {
                if (original_sigactions[signo]) {
                    original_sigactions[signo](signo, info, context);
                    return; // Original handler should handle the signal
                }

                if (original_handlers[signo]) {
                    original_handlers[signo](signo);
                    return; // Original handler should handle the signal
                }
            }

            // If there's no original handler or we can't chain, reset to default and re-raise
            struct sigaction sa;
            sa.sa_handler = SIG_DFL;
            sigemptyset(&sa.sa_mask);
            sa.sa_flags = 0;
            if(sigaction(signo, &sa, NULL) < 0) { ; }

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

void nd_cleanup_deadly_signals(void) {
    struct sigaction act;
    memset(&act, 0, sizeof(struct sigaction));

    // ignore all signals while we run in a signal handler
    sigfillset(&act.sa_mask);

    for (size_t i = 0; i < _countof(signals_waiting); i++) {
        if(signals_waiting[i].action != NETDATA_SIGNAL_DEADLY)
            continue;

        act.sa_flags = 0;
        act.sa_handler = SIG_DFL;

        if (sigaction(signals_waiting[i].signo, &act, NULL) == -1)
            netdata_log_error("SIGNAL: Failed to cleanup signal handler for: %s", signals_waiting[i].name);
    }

    memset(original_handlers, 0, sizeof(original_handlers));
    memset(original_sigactions, 0, sizeof(original_sigactions));
}

void nd_initialize_signals(bool chain_existing) {
    signals_block_all_except_deadly();
    
    // Set the signal handler name for stack trace filtering
#ifdef HAVE_LIBBACKTRACE
    stacktrace_set_signal_handler_function("nd_signal_handler");
#endif

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
            (uintptr_t)old_act.sa_handler != (uintptr_t)nd_signal_handler) {
            // Save the original handlers for chaining
            if (old_act.sa_flags & SA_SIGINFO)
                original_sigactions[signo] = old_act.sa_sigaction;
            else
                original_handlers[signo] = old_act.sa_handler;
        }

        switch (signals_waiting[i].action) {
            case NETDATA_SIGNAL_IGNORE:
                act.sa_flags = 0;
                act.sa_handler = SIG_IGN;
                break;
            default:
                act.sa_flags = SA_SIGINFO;
                act.sa_sigaction = nd_signal_handler;
                break;
        }

        if (sigaction(signals_waiting[i].signo, &act, NULL) == -1)
            netdata_log_error("SIGNAL: Failed to change signal handler for: %s", signals_waiting[i].name);
    }
}

NEVER_INLINE
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
                    if(exit_initiated_get())
                        netdata_log_info("SIGNAL: Received %s. Ignoring it, as we are exiting...", name);
                    else {
                        nd_log_limits_unlimited();
                        netdata_log_info("SIGNAL: Received %s. Reloading HEALTH configuration...", name);
                        nd_log_limits_reset();
                        execute_command(CMD_RELOAD_HEALTH, NULL, NULL);
                    }
                    break;

                case NETDATA_SIGNAL_REOPEN_LOGS:
                    if(exit_initiated_get())
                        netdata_log_info("SIGNAL: Received %s. Ignoring it, as we are exiting...", name);
                    else {
                        nd_log_limits_unlimited();
                        netdata_log_info("SIGNAL: Received %s. Reopening all log files...", name);
                        nd_log_limits_reset();
                        execute_command(CMD_REOPEN_LOGS, NULL, NULL);
                    }
                    break;

                case NETDATA_SIGNAL_EXIT_CLEANLY:
                    nd_log_limits_unlimited();
                    netdata_log_info("SIGNAL: Received %s. Cleaning up to exit...", name);
                    commands_exit();
                    netdata_exit_gracefully(signals_waiting[i].reason, true);
                    break;

                case NETDATA_SIGNAL_DEADLY:
                    _exit(1);
                    break;

                default:
                    netdata_log_info("SIGNAL: Received %s. No signal handler configured. Ignoring it.", name);
                    break;
            }
        }
    } while(found);
}

static inline bool threshold_trigger_smaller(bool *last, double threshold, double hysteresis, double free_mem) {
    bool triggered = *last;

    if (free_mem < threshold)
        *last = true;

    if (free_mem >= (threshold + hysteresis))
        *last = false;

    return !triggered && *last;
}

NEVER_INLINE
void nd_process_signals(void) {
    posix_unmask_my_signals();
    const usec_t save_every_ut = 15 * 60 * USEC_PER_SEC;
    usec_t last_update_mt = now_monotonic_usec();
    bool triggered1 = false, triggered5 = false, triggered10 = false;

    while (true) {
        bool save_again = false;
        double free_mem = os_system_memory_available_percent(os_system_memory(false));

        save_again =
            threshold_trigger_smaller(&triggered1, 1.0, 1.0, free_mem) ||
            threshold_trigger_smaller(&triggered5, 5.0, 1.0, free_mem) ||
            threshold_trigger_smaller(&triggered10, 10.0, 1.0, free_mem);

        usec_t mt = now_monotonic_usec();
        if ((mt - last_update_mt) >= save_every_ut || save_again) {
            daemon_status_file_update_status(DAEMON_STATUS_NONE);
            last_update_mt += save_every_ut;
        }

        if(poll(NULL, 0, 13 * MSEC_PER_SEC + 379) < 0) { ; }

        process_triggered_signals();
    }
}
