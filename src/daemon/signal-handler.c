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

_Static_assert(__atomic_always_lock_free(sizeof(signals_waiting[0].count),
                                         &signals_waiting[0].count),
               "signal pending counters must be lock-free");

typedef void (*SIGNAL_HANDLER)(int);
typedef void (*SIGNAL_SIGACTION)(int, siginfo_t *, void *);

static SIGNAL_HANDLER original_handlers[NSIG] = {0};
static SIGNAL_SIGACTION original_sigactions[NSIG] = {0};

#if defined(OS_WINDOWS)
static HANDLE windows_console_signal_event = NULL;
static HANDLE windows_console_shutdown_complete_event = NULL;
static bool windows_console_handler_registered = false;

static DWORD windows_console_shutdown_timeout_ms(DWORD control_type) {
    UINT parameter;
    DWORD fallback_timeout;

    switch (control_type) {
        case CTRL_CLOSE_EVENT:
            parameter = SPI_GETHUNGAPPTIMEOUT;
            fallback_timeout = 5000;
            break;
        case CTRL_SHUTDOWN_EVENT:
            parameter = SPI_GETWAITTOKILLSERVICETIMEOUT;
            fallback_timeout = 20000;
            break;
        default:
            return 0;
    }

    DWORD timeout = fallback_timeout;
    if (!SystemParametersInfoW(parameter, 0, &timeout, 0) || !timeout)
        timeout = fallback_timeout;

    // Keep the wait finite even if a local policy sets an extreme value, and
    // return before the system's own termination deadline.
    if (timeout > 120000)
        timeout = 120000;
    DWORD safety_margin = timeout / 10;
    if (safety_margin < 100)
        safety_margin = 100;
    return timeout > safety_margin ? timeout - safety_margin : 1;
}

// Queues the signal and wakes nd_process_signals(), which performs the actual
// cleanup outside this callback.
//
// Ctrl+C/Break can return after queuing because the process remains active.
// Close/shutdown callbacks wait for teardown completion, bounded by the
// applicable Windows shutdown timeout, before returning to the OS. Logoff is
// delegated to the service-aware default handler so a user logoff cannot stop
// the service.
static BOOL WINAPI windows_console_control_handler(DWORD control_type) {
    int signo;
    bool wait_for_shutdown = false;
    switch (control_type) {
        case CTRL_C_EVENT:
            signo = SIGINT;
            break;
        case CTRL_BREAK_EVENT:
            signo = SIGQUIT;
            break;
        case CTRL_CLOSE_EVENT:
            signo = SIGTERM;
            wait_for_shutdown = true;
            break;
        case CTRL_LOGOFF_EVENT:
            // Services must remain running when an interactive user logs off.
            // Let Windows' service-aware default handler process this event.
            return FALSE;
        case CTRL_SHUTDOWN_EVENT:
            signo = SIGTERM;
            wait_for_shutdown = true;
            break;
        default:
            return FALSE;
    }

    // Without both events, the signal loop cannot be woken and the close-class
    // callback cannot confirm completion. Leave termination to Windows.
    if (wait_for_shutdown &&
        (!windows_console_signal_event || !windows_console_shutdown_complete_event))
        return FALSE;

    for (size_t i = 0; i < _countof(signals_waiting); i++) {
        if (signals_waiting[i].signo == signo) {
            __atomic_fetch_add(&signals_waiting[i].count, 1, __ATOMIC_RELAXED);
            break;
        }
    }

    if (windows_console_signal_event)
        SetEvent(windows_console_signal_event);

    if (wait_for_shutdown) {
        WaitForSingleObject(windows_console_shutdown_complete_event,
                            windows_console_shutdown_timeout_ms(control_type));
    }

    return TRUE;
}
#endif

static inline bool signal_number_supported(int signo) {
    return signo >= 0 && signo < NSIG;
}

// Signal-handler atomics must never fall back to a locking runtime helper.
_Static_assert(__atomic_always_lock_free(sizeof(original_handlers[0]), original_handlers),
               "signal handler pointers must be lock-free");
_Static_assert(__atomic_always_lock_free(sizeof(original_sigactions[0]), original_sigactions),
               "signal sigaction pointers must be lock-free");

NEVER_INLINE
void nd_signal_handler(int signo, siginfo_t *info, void *context __maybe_unused) {
    signal_protected_access_check(signo, info, context);

    for(size_t i = 0; i < _countof(signals_waiting) ; i++) {
        if(signals_waiting[i].signo != signo)
            continue;

        __atomic_fetch_add(&signals_waiting[i].count, 1, __ATOMIC_RELAXED);

        if(signals_waiting[i].action == NETDATA_SIGNAL_DEADLY) {
            SIGNAL_SIGACTION original_sigaction = NULL;
            SIGNAL_HANDLER original_handler = NULL;
            if (signal_number_supported(signo)) {
                original_sigaction =
                    __atomic_load_n(&original_sigactions[signo], __ATOMIC_ACQUIRE);
                original_handler =
                    __atomic_load_n(&original_handlers[signo], __ATOMIC_ACQUIRE);
            }
            bool chained_handler = original_sigaction ||
                (original_handler && original_handler != SIG_IGN && original_handler != SIG_DFL);

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
            char tid[UINT64_MAX_LENGTH];
            print_uint64(tid, gettid_cached());
            len = strcatz(b, len, tid, sizeof(b));
            len = strcatz(b, len, " ", sizeof(b));
            len = strcatz(b, len, nd_thread_tag_async_safe(), sizeof(b));
            len = strcatz(b, len, "!\n", sizeof(b));

            if(write(STDERR_FILENO, b, len) == -1) {
                // nothing to do - we cannot write but there is no way to complain about it
                ;
            }

            // Chain to the original handler if it exists
            if(chained_handler) {
                if (original_sigaction) {
                    original_sigaction(signo, info, context);
                    return; // Original handler should handle the signal
                }

                if (original_handler) {
                    original_handler(signo);
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
#if defined(OS_WINDOWS)
    return;
#else
    sigset_t sigset;
    sigemptyset(&sigset);

    for (size_t i = 0; i < _countof(signals_waiting) ; i++)
        sigaddset(&sigset, signals_waiting[i].signo);

    if (pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) != 0)
        netdata_log_error("SIGNAL: cannot unmask netdata signals");
#endif
}

void nd_cleanup_deadly_signals(void) {
#if defined(OS_WINDOWS)
    return;
#else
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

    for(size_t signo = 0; signo < NSIG; signo++) {
        __atomic_store_n(&original_handlers[signo], (SIGNAL_HANDLER)0, __ATOMIC_RELEASE);
        __atomic_store_n(&original_sigactions[signo], (SIGNAL_SIGACTION)0, __ATOMIC_RELEASE);
    }
#endif
}

void nd_initialize_signals(bool chain_existing) {
#if defined(OS_WINDOWS)
    (void)chain_existing;
    if (!windows_console_signal_event)
        windows_console_signal_event = CreateEvent(NULL, FALSE, FALSE, NULL);
    if (!windows_console_shutdown_complete_event)
        windows_console_shutdown_complete_event = CreateEvent(NULL, TRUE, FALSE, NULL);

    if (!windows_console_handler_registered) {
        if (SetConsoleCtrlHandler(windows_console_control_handler, TRUE))
            windows_console_handler_registered = true;
        else
            fprintf(stderr, "SIGNAL: Failed to register Windows console control handler (error %lu)\n",
                    (unsigned long)GetLastError());
    }
    return;
#else
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
            if (signal_number_supported(signo)) {
                if (old_act.sa_flags & SA_SIGINFO)
                    __atomic_store_n(&original_sigactions[signo], old_act.sa_sigaction, __ATOMIC_RELEASE);
                else
                    __atomic_store_n(&original_handlers[signo], old_act.sa_handler, __ATOMIC_RELEASE);
            }
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
#endif
}

#if defined(OS_WINDOWS)
void nd_windows_signal_shutdown_complete(void) {
    if (windows_console_shutdown_complete_event)
        SetEvent(windows_console_shutdown_complete_event);
}
#endif

NEVER_INLINE
static void process_triggered_signals(void) {
    size_t found;
    do {
        found = 0;
        for (size_t i = 0; i < _countof(signals_waiting) ; i++) {
            if (!__atomic_exchange_n(&signals_waiting[i].count, 0, __ATOMIC_RELAXED))
                continue;

            found++;
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

#if defined(FSANITIZE_ADDRESS)
                case NETDATA_SIGNAL_EXIT_NOW:
                    exit(1);
                    break;
#endif

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

#if defined(OS_WINDOWS)
        if (windows_console_signal_event)
            WaitForSingleObject(windows_console_signal_event, 13 * MSEC_PER_SEC + 379);
        else
            Sleep(13 * MSEC_PER_SEC + 379);
#else
        if(poll(NULL, 0, 13 * MSEC_PER_SEC + 379) < 0) { ; }
#endif

        process_triggered_signals();
    }
}
