// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_H
#define NETDATA_DAEMON_H 1

int become_daemon(int dont_fork, const char *user);

void netdata_cleanup_and_exit(int ret, const char *action, const char *action_result, const char *action_data);

void get_netdata_execution_path(void);

extern char *pidfile;
extern char *netdata_exe_path;

#endif /* NETDATA_DAEMON_H */
