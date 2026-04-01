// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_PARENT_H
#define NETDATA_DAEMON_PARENT_H

#include "libnetdata/libnetdata.h"

/**
 * Start a parent process to monitor netdata
 *
 * This function forks the current process. The parent process will monitor
 * the child (netdata) process and update the status file if netdata exits
 * abnormally or is terminated by a signal that netdata's signal handler
 * cannot catch.
 *
 * @param argc The original argc from main()
 * @param argv The original argv from main()
 * @return 0 for the child process (netdata), -1 on error, does not return for the parent process
 */
int daemon_parent_start(int argc, char **argv);

#endif // NETDATA_DAEMON_PARENT_H