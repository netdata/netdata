#ifndef NETDATA_PLUGIN_TC_H
#define NETDATA_PLUGIN_TC_H 1

/**
 * @file plugin_tc.h
 * @brief Thread to collect network tc classes data.
 */

/// PID of tc child
extern volatile pid_t tc_child_pid;
/**
 * Main function of TC thread.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
extern void *tc_main(void *ptr);

#endif /* NETDATA_PLUGIN_TC_H */

