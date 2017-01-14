#ifndef NETDATA_PLUGIN_TC_H
#define NETDATA_PLUGIN_TC_H 1

extern volatile pid_t tc_child_pid;
extern void *tc_main(void *ptr);

#endif /* NETDATA_PLUGIN_TC_H */

