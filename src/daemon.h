#ifndef NETDATA_DAEMON_H
#define NETDATA_DAEMON_H 1

/**
 * @file daemon.h
 * @brief Netdata deamon functions.
 *
 * This should only be used from the main thread.
 */

/**
 * Signal handler which tells netdata to stop.
 *
 * Set `netdata_exit` to 1.
 *
 * @param signo recieved
 */
extern void sig_handler_exit(int signo);
/**
 * Signal handler which saves the round robin database to disk.
 *
 * @param signo recieved
 */
extern void sig_handler_save(int signo);
/**
 * Signal handler which reopens all logfiles.
 *
 * @param signo recieved
 */
extern void sig_handler_logrotate(int signo);
/**
 * Signal handler which reloads health status.
 *
 * @param signo recieved
 */
extern void sig_handler_reload_health(int signo);

/**
 * After this the process is owned by `username`
 *
 * @param username which should own the process
 * @param pid_fd File descriptor to pidfile of the process.
 * @return 0 on success, -1 on error
 */
extern int become_user(const char *username, int pid_fd);

/**
 * Initialize the daemon process.
 *
 * Only fork if `dont_fork` is false.
 * After this the process is owned by `username`
 *
 * @param dont_fork boolean
 * @param user which should own the process
 * @return 0 on success, -1 on error
 */
extern int become_daemon(int dont_fork, const char *user);

/**
 * Exit netdata the save way.
 *
 * This calls exit().
 *
 * @param i Return code to set.
 */
extern void netdata_cleanup_and_exit(int i);

/** Patho of the pidfile of the process */
extern char pidfile[];

#endif /* NETDATA_DAEMON_H */
