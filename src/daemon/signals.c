// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

/*
 * IMPORTANT: Libuv uv_spawn() uses SIGCHLD internally:
 * https://github.com/libuv/libuv/blob/cc51217a317e96510fbb284721d5e6bc2af31e33/src/unix/process.c#L485
 * Extreme care is needed when mixing and matching POSIX and libuv.
 */

typedef enum signal_action {
    NETDATA_SIGNAL_END_OF_LIST,
    NETDATA_SIGNAL_IGNORE,
    NETDATA_SIGNAL_EXIT_CLEANLY,
    NETDATA_SIGNAL_REOPEN_LOGS,
    NETDATA_SIGNAL_RELOAD_HEALTH,
    NETDATA_SIGNAL_FATAL,
} SIGNAL_ACTION;

static struct {
    int signo;              // the signal
    const char *name;       // the name of the signal
    size_t count;           // the number of signals received
    SIGNAL_ACTION action;   // the action to take
} signals_waiting[] = {
        { SIGPIPE, "SIGPIPE", 0, NETDATA_SIGNAL_IGNORE        },
        { SIGINT , "SIGINT",  0, NETDATA_SIGNAL_EXIT_CLEANLY  },
        { SIGQUIT, "SIGQUIT", 0, NETDATA_SIGNAL_EXIT_CLEANLY  },
        { SIGTERM, "SIGTERM", 0, NETDATA_SIGNAL_EXIT_CLEANLY  },
        { SIGHUP,  "SIGHUP",  0, NETDATA_SIGNAL_REOPEN_LOGS   },
        { SIGUSR2, "SIGUSR2", 0, NETDATA_SIGNAL_RELOAD_HEALTH },
        { SIGBUS,  "SIGBUS",  0, NETDATA_SIGNAL_FATAL         },

        // terminator
        { 0,       "NONE",    0, NETDATA_SIGNAL_END_OF_LIST   }
};

static void signal_handler(int signo) {
    // find the entry in the list
    int i;
    for(i = 0; signals_waiting[i].action != NETDATA_SIGNAL_END_OF_LIST ; i++) {
        if(unlikely(signals_waiting[i].signo == signo)) {
            signals_waiting[i].count++;

            if(signals_waiting[i].action == NETDATA_SIGNAL_FATAL) {
                char buffer[200 + 1];
                snprintfz(buffer, sizeof(buffer) - 1, "\nSIGNAL HANDLER: received: %s. Oops! This is bad!\n", signals_waiting[i].name);
                if(write(STDERR_FILENO, buffer, strlen(buffer)) == -1) {
                    // nothing to do - we cannot write but there is no way to complain about it
                    ;
                }
            }

            return;
        }
    }
}

void signals_block(void) {
    sigset_t sigset;
    sigfillset(&sigset);

    if(pthread_sigmask(SIG_BLOCK, &sigset, NULL) == -1)
        netdata_log_error("SIGNAL: Could not block signals for threads");
}

void signals_unblock(void) {
    sigset_t sigset;
    sigfillset(&sigset);

    if(pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) == -1) {
        netdata_log_error("SIGNAL: Could not unblock signals for threads");
    }
}

void signals_init(void) {
    // Catch signals which we want to use
    struct sigaction sa;
    sa.sa_flags = 0;

    // ignore all signals while we run in a signal handler
    sigfillset(&sa.sa_mask);

    int i;
    for (i = 0; signals_waiting[i].action != NETDATA_SIGNAL_END_OF_LIST; i++) {
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

void signals_reset(void) {
    struct sigaction sa;
    sigemptyset(&sa.sa_mask);
    sa.sa_handler = SIG_DFL;
    sa.sa_flags = 0;

    int i;
    for (i = 0; signals_waiting[i].action != NETDATA_SIGNAL_END_OF_LIST; i++) {
        if(sigaction(signals_waiting[i].signo, &sa, NULL) == -1)
            netdata_log_error("SIGNAL: Failed to reset signal handler for: %s", signals_waiting[i].name);
    }
}

void signals_handle(void) {
    while(1) {

        // pause()  causes  the calling process (or thread) to sleep until a signal
        // is delivered that either terminates the process or causes the invocation
        // of a signal-catching function.
        if(pause() == -1 && errno == EINTR) {
            errno_clear();

            // loop once, but keep looping while signals are coming in
            // this is needed because a few operations may take some time
            // so we need to check for new signals before pausing again
            int found = 1;
            while(found) {
                found = 0;

                // execute the actions of the signals
                int i;
                for (i = 0; signals_waiting[i].action != NETDATA_SIGNAL_END_OF_LIST; i++) {
                    if (signals_waiting[i].count) {
                        found = 1;
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
                                netdata_cleanup_and_exit(0, NULL, NULL, NULL);
                                exit(0);
                                break;

                            case NETDATA_SIGNAL_FATAL:
                                fatal("SIGNAL: Received %s. netdata now exits.", name);
                                break;

                            default:
                                netdata_log_info("SIGNAL: Received %s. No signal handler configured. Ignoring it.", name);
                                break;
                        }
                    }
                }
            }
        }
        else
            netdata_log_error("SIGNAL: pause() returned but it was not interrupted by a signal.");
    }
}
