#ifndef NETDATA_MAIN_H
#define NETDATA_MAIN_H 1

extern int netdata_exit;

extern void kill_childs(void);
extern int killpid(pid_t pid, int signal);
extern void netdata_cleanup_and_exit(int ret);

#endif /* NETDATA_MAIN_H */
