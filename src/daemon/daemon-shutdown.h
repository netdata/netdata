// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_SHUTDOWN_H
#define NETDATA_DAEMON_SHUTDOWN_H

#include "libnetdata/libnetdata.h"

void cancel_main_threads(void);

void abort_on_fatal_disable(void);
void abort_on_fatal_enable(void);

void netdata_cleanup_and_exit_gracefully(EXIT_REASON reason);
void netdata_cleanup_and_exit_fatal(EXIT_REASON reason);

#endif //NETDATA_DAEMON_SHUTDOWN_H
