// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXEC_SIGNALS_H
#define NETDATA_EXEC_SIGNALS_H

// Shared by the standalone exec wrappers (ndsudo, nd-run). These are deliberately dependency-free
// single-file binaries that do not link libnetdata, so this is a header-only helper.

#include <signal.h>
#include <stddef.h>
#include <string.h>

// Restore default dispositions for the signals netdata sets to SIG_IGN, immediately before exec.
//
// Signal HANDLERS are reset to default by exec automatically, but IGNORED signals are NOT: SIG_IGN
// survives execve() (POSIX exec). netdata ignores SIGPIPE (NETDATA_SIGNAL_IGNORE in
// src/daemon/signal-handler.c), so without this the command we exec inherits that ignore, gets EPIPE
// instead of dying when netdata closes its stdio, and keeps running. For a command exec'd with
// escalated privileges that is unrecoverable: netdata runs unprivileged and cannot signal it.
//
// Only signals netdata may have ignored are reset, so we do not override dispositions we never set
// (e.g. inherited from the service manager). Keep in sync with signals_waiting[] in
// src/daemon/signal-handler.c.
static inline void reset_signal_dispositions(void) {
    static const int signals[] = { SIGPIPE };

    struct sigaction sa;
    memset(&sa, 0, sizeof(sa));
    sa.sa_handler = SIG_DFL;
    sigemptyset(&sa.sa_mask);

    for (size_t i = 0; i < sizeof(signals) / sizeof(signals[0]); i++)
        // Best effort: a failure here must not stop us from running the command. The caller would
        // rather run with an inherited disposition than not run at all.
        (void)sigaction(signals[i], &sa, NULL);
}

#endif // NETDATA_EXEC_SIGNALS_H
