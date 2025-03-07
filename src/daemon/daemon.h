// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_H
#define NETDATA_DAEMON_H 1

int become_daemon(int dont_fork, const char *user);

void get_netdata_execution_path(void);

extern char *pidfile;

void verify_required_directory(const char *env, const char *dir, bool create_it, int perms);

#endif /* NETDATA_DAEMON_H */
