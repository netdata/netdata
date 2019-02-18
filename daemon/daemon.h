// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_H
#define NETDATA_DAEMON_H 1

extern int become_user(const char *username, int pid_fd);

extern int become_daemon(int dont_fork, const char *user);

extern void netdata_cleanup_and_exit(int i);
extern void send_statistics(const char *action, const char *action_result, const char *action_data);

extern char pidfile[];

#endif /* NETDATA_DAEMON_H */
