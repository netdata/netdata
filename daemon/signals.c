// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

static int reaper_enabled = 0;

typedef enum signal_action {
    NETDATA_SIGNAL_END_OF_LIST,
    NETDATA_SIGNAL_IGNORE,
    NETDATA_SIGNAL_EXIT_CLEANLY,
    NETDATA_SIGNAL_SAVE_DATABASE,
    NETDATA_SIGNAL_REOPEN_LOGS,
    NETDATA_SIGNAL_RELOAD_HEALTH,
    NETDATA_SIGNAL_FATAL,
    NETDATA_SIGNAL_CHILD,
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
        { SIGUSR1, "SIGUSR1", 0, NETDATA_SIGNAL_SAVE_DATABASE },
        { SIGUSR2, "SIGUSR2", 0, NETDATA_SIGNAL_RELOAD_HEALTH },
        { SIGBUS,  "SIGBUS",  0, NETDATA_SIGNAL_FATAL         },
        { SIGCHLD, "SIGCHLD", 0, NETDATA_SIGNAL_CHILD         },

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
                snprintfz(buffer, 200, "\nSIGNAL HANLDER: received: %s. Oops! This is bad!\n", signals_waiting[i].name);
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
        error("SIGNAL: Could not block signals for threads");
}

void signals_unblock(void) {
    sigset_t sigset;
    sigfillset(&sigset);

    if(pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) == -1) {
        error("SIGNAL: Could not unblock signals for threads");
    }
}

void signals_init(void) {
    // Catch signals which we want to use
    struct sigaction sa;
    sa.sa_flags = 0;

    // Enable process tracking / reaper if running as init (pid == 1).
    // This prevents zombie processes when running in a container.
    if (getpid() == 1) {
        info("SIGNAL: Enabling reaper");
        myp_init();
        reaper_enabled = 1;
    } else {
        info("SIGNAL: Not enabling reaper");
    }

    // ignore all signals while we run in a signal handler
    sigfillset(&sa.sa_mask);

    int i;
    for (i = 0; signals_waiting[i].action != NETDATA_SIGNAL_END_OF_LIST; i++) {
        switch (signals_waiting[i].action) {
        case NETDATA_SIGNAL_IGNORE:
            sa.sa_handler = SIG_IGN;
            break;
        case NETDATA_SIGNAL_CHILD:
            if (reaper_enabled == 0)
                continue;
            // FALLTHROUGH
        default:
            sa.sa_handler = signal_handler;
            break;
        }

        if(sigaction(signals_waiting[i].signo, &sa, NULL) == -1)
            error("SIGNAL: Failed to change signal handler for: %s", signals_waiting[i].name);
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
            error("SIGNAL: Failed to reset signal handler for: %s", signals_waiting[i].name);
    }

    if (reaper_enabled == 1)
        myp_free();
}

// reap_child reaps the child identified by pid.
static void reap_child(pid_t pid) {
    siginfo_t i;

    errno = 0;
    debug(D_CHILDS, "SIGNAL: Reaping pid: %d...", pid);
    if (waitid(P_PID, (id_t)pid, &i, WEXITED|WNOHANG) == -1) {
        if (errno != ECHILD)
            error("SIGNAL: Failed to wait for: %d", pid);
        else
            debug(D_CHILDS, "SIGNAL: Already reaped: %d", pid);
        return;
    } else if (i.si_pid == 0) {
        // Process didn't exit, this shouldn't happen.
        return;
    }

    switch (i.si_code) {
    case CLD_EXITED:
        debug(D_CHILDS, "SIGNAL: Child %d exited: %d", pid, i.si_status);
        break;
    case CLD_KILLED:
        debug(D_CHILDS, "SIGNAL: Child %d killed by signal: %d", pid, i.si_status);
        break;
    case CLD_DUMPED:
        debug(D_CHILDS, "SIGNAL: Child %d dumped core by signal: %d", pid, i.si_status);
        break;
    case CLD_STOPPED:
        debug(D_CHILDS, "SIGNAL: Child %d stopped by signal: %d", pid, i.si_status);
        break;
    case CLD_TRAPPED:
        debug(D_CHILDS, "SIGNAL: Child %d trapped by signal: %d", pid, i.si_status);
        break;
    case CLD_CONTINUED:
        debug(D_CHILDS, "SIGNAL: Child %d continued by signal: %d", pid, i.si_status);
        break;
    default:
        debug(D_CHILDS, "SIGNAL: Child %d gave us a SIGCHLD with code %d and status %d.", pid, i.si_code, i.si_status);
    }
}

// reap_children reaps all pending children which are not managed by myp.
static void reap_children() {
    siginfo_t i;

    while (1 == 1) {
        // Identify which process caused the signal so we can determine
        // if we need to reap a re-parented process.
        i.si_pid = 0;
        if (waitid(P_ALL, (id_t)0, &i, WEXITED|WNOHANG|WNOWAIT) == -1) {
            if (errno != ECHILD) // This shouldn't happen with WNOHANG but does.
                error("SIGNAL: Failed to wait");
            return;
        } else if (i.si_pid == 0) {
            // No child exited.
            return;
        } else if (myp_reap(i.si_pid) == 0) {
            // myp managed, sleep for a short time to avoid busy wait while
            // this is handled by myp.
            usleep(10000);
        } else {
            // Unknown process, likely a re-parented child, reap it.
            reap_child(i.si_pid);
        }
    }
}

void signals_handle(void) {
    while(1) {

        // pause()  causes  the calling process (or thread) to sleep until a signal
        // is delivered that either terminates the process or causes the invocation
        // of a signal-catching function.
        if(pause() == -1 && errno == EINTR) {

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
                                error_log_limit_unlimited();
                                info("SIGNAL: Received %s. Reloading HEALTH configuration...", name);
                                error_log_limit_reset();
                                execute_command(CMD_RELOAD_HEALTH, NULL, NULL);
                                break;

                            case NETDATA_SIGNAL_SAVE_DATABASE:
                                error_log_limit_unlimited();
                                info("SIGNAL: Received %s. Saving databases...", name);
                                error_log_limit_reset();
                                execute_command(CMD_SAVE_DATABASE, NULL, NULL);
                                break;

                            case NETDATA_SIGNAL_REOPEN_LOGS:
                                error_log_limit_unlimited();
                                info("SIGNAL: Received %s. Reopening all log files...", name);
                                error_log_limit_reset();
                                execute_command(CMD_REOPEN_LOGS, NULL, NULL);
                                break;

                            case NETDATA_SIGNAL_EXIT_CLEANLY:
                                error_log_limit_unlimited();
                                info("SIGNAL: Received %s. Cleaning up to exit...", name);
                                commands_exit();
                                netdata_cleanup_and_exit(0);
                                exit(0);
                                break;

                            case NETDATA_SIGNAL_FATAL:
                                fatal("SIGNAL: Received %s. netdata now exits.", name);
                                break;

                            case NETDATA_SIGNAL_CHILD:
                                debug(D_CHILDS, "SIGNAL: Received %s. Reaping...", name);
                                reap_children();
                                break;

                            default:
                                info("SIGNAL: Received %s. No signal handler configured. Ignoring it.", name);
                                break;
                        }
                    }
                }
            }
        }
        else
            error("SIGNAL: pause() returned but it was not interrupted by a signal.");
    }
}
