// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// Mask all signals, to ensure they will only be unmasked at the threads that can handle them.
// This means that all third party libraries (including libuv) cannot use signals anymore.
// The signals they are interested must be unblocked at their corresponding event loops.
void signals_block_all_except_deadly(void) {
    sigset_t sigset;
    sigfillset(&sigset);

    // Don't mask fatal signals - we want these to be handled in any thread
    sigdelset(&sigset, SIGBUS);
    sigdelset(&sigset, SIGSEGV);
    sigdelset(&sigset, SIGFPE);
    sigdelset(&sigset, SIGILL);
    sigdelset(&sigset, SIGABRT);

    if(pthread_sigmask(SIG_BLOCK, &sigset, NULL) != 0)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "SIGNALS: cannot apply the default mask for signals");
}

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
