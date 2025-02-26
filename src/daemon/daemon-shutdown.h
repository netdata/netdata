// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_SHUTDOWN_H
#define NETDATA_DAEMON_SHUTDOWN_H

#include "libnetdata/libnetdata.h"

void cancel_main_threads(void);

void abort_on_fatal_disable(void);
void abort_on_fatal_enable(void);

#ifdef OS_WINDOWS
void netdata_cleanup_and_exit(EXIT_REASON reason, const char *action, const char *action_result, const char *action_data);
#else
void netdata_cleanup_and_exit(EXIT_REASON reason, const char *action, const char *action_result, const char *action_data) NORETURN;
#endif

#endif //NETDATA_DAEMON_SHUTDOWN_H
