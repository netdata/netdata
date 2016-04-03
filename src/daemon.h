#ifndef NETDATA_DAEMON_H
#define NETDATA_DAEMON_H 1

extern void sig_handler(int signo);

extern int become_user(const char *username);

extern int become_daemon(int dont_fork, int close_all_files, const char *user, const char *input, const char *output, const char *error, const char *access, int *access_fd, FILE **access_fp);

extern void netdata_cleanup_and_exit(int i);

extern char pidfile[];
extern int pidfd;

#endif /* NETDATA_DAEMON_H */
