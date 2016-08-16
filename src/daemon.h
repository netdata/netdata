#ifndef NETDATA_DAEMON_H
#define NETDATA_DAEMON_H 1

extern void sig_handler_exit(int signo);
extern void sig_handler_save(int signo);
extern void sig_handler_logrotate(int signo);
extern void sig_handler_reload_health(int signo);

extern int become_user(const char *username, int pid_fd);

extern int become_daemon(int dont_fork, const char *user);

extern void netdata_cleanup_and_exit(int i);

extern char pidfile[];

#endif /* NETDATA_DAEMON_H */
