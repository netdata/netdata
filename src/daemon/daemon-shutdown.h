// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_SHUTDOWN_H
#define NETDATA_DAEMON_SHUTDOWN_H

#include "libnetdata/libnetdata.h"

void cancel_main_threads(void);

void abort_on_fatal_disable(void);
void abort_on_fatal_enable(void);

#if !defined(OS_WINDOWS)
#define ND_EXIT_NORETURN NORETURN
#else
#define ND_EXIT_NORETURN
#endif

void netdata_exit_gracefully(EXIT_REASON reason, bool exit_when_done);
void netdata_exit_fatal(void);

#endif //NETDATA_DAEMON_SHUTDOWN_H
