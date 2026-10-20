// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXEC_SIGNALS_H
#define NETDATA_EXEC_SIGNALS_H

// Shared by the standalone exec wrappers (ndsudo, nd-run). These are deliberately dependency-free
// single-file binaries that do not link libnetdata, so this is a header-only helper.
//
// Requires POSIX declarations (struct sigaction, sigaction(), sigprocmask()). netdata compiles with
// gnu* C standards, which expose them; under a strict -std=cNN a caller must define _POSIX_C_SOURCE
// (or include config.h first, as nd-run.c does for setresuid()).

#include <signal.h>
#include <stddef.h>
#include <string.h>

// Make the signals netdata neutralises deliverable again, immediately before exec.
//
// Two things survive execve() and both have to be undone, or the command we exec cannot be stopped
// by closing its stdio:
//
//  - the DISPOSITION, when it is SIG_IGN. Handlers are reset by exec automatically; SIG_IGN is not
//    (POSIX exec). netdata sets SIGPIPE to SIG_IGN (NETDATA_SIGNAL_IGNORE in
//    src/daemon/signal-handler.c).
//  - the MASK. netdata also BLOCKS SIGPIPE (signals_block_all_except_deadly() unblocks only the
//    deadly signals). A blocked SIGPIPE makes write() return EPIPE with the signal left pending, so
//    the child survives a closed pipe exactly as if it were still ignored.
//
// Either one alone leaves the child running after netdata closes its stdio, and for a command exec'd
// with escalated privileges that is unrecoverable: netdata runs unprivileged and cannot signal it.
//
// Only signals netdata may have neutralised are touched, so we do not override state we never set
// (e.g. inherited from the service manager). Keep in sync with signals_waiting[] in
// src/daemon/signal-handler.c.
static inline void reset_signal_dispositions(void) {
    static const int signals[] = { SIGPIPE };

    struct sigaction sa;
    memset(&sa, 0, sizeof(sa));
    sa.sa_handler = SIG_DFL;
    sigemptyset(&sa.sa_mask);

    sigset_t unblock;
    sigemptyset(&unblock);

    for (size_t i = 0; i < sizeof(signals) / sizeof(signals[0]); i++) {
        // Best effort: a failure here must not stop us from running the command. The caller would
        // rather run with an inherited disposition than not run at all.
        (void)sigaction(signals[i], &sa, NULL);
        sigaddset(&unblock, signals[i]);
    }

    (void)sigprocmask(SIG_UNBLOCK, &unblock, NULL);
}

#endif // NETDATA_EXEC_SIGNALS_H
