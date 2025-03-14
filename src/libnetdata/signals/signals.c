// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

void signals_block_all(void) {
    sigset_t sigset;
    sigfillset(&sigset);

    if(pthread_sigmask(SIG_BLOCK, &sigset, NULL) != 0)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "SIGNALS: cannot apply the default mask for signals");
}

void signals_unblock_one(int signo) {
    sigset_t sigset;
    sigemptyset(&sigset);  // Initialize the signal set to empty
    sigaddset(&sigset, signo);  // Add our signal to the set

    if(pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) != 0)
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SIGNALS: cannot unmask signal %d", signo);
}

void signals_unblock(int signals[], size_t count) {
    sigset_t sigset;
    sigemptyset(&sigset);  // Initialize the signal set to empty

    // Add each signal from the array to the signal set
    for (size_t i = 0; i < count; i++)
        sigaddset(&sigset, signals[i]);

    // Unblock all signals in the set
    if (pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) != 0)
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SIGNALS: cannot unmask signals");
}

void signals_unblock_deadly(void) {
    int deadly_signals[] = {
        SIGBUS,
        SIGSEGV,
        SIGFPE,
        SIGILL,
        SIGABRT,
        SIGSYS,
        SIGXCPU,
        SIGXFSZ,
    };
    signals_unblock(deadly_signals, _countof(deadly_signals));
}

void signals_block_all_except_deadly(void) {
    signals_block_all();
    signals_unblock_deadly();
}
