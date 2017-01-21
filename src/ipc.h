#ifndef NETDATA_PLUGIN_IPC_H
#define NETDATA_PLUGIN_IPC_H 1

/**
 * @file ipc.h
 * @brief Inter process communication statistics.
 */

/**
 * Data collector of ipc statistics
 *
 * @param update_every intervall in seconds this is called.
 * @param dt microseconds passed since the last call.
 * @return 0 on success. 1 on error.
 */
extern int do_ipc(int update_every, usec_t dt);

#endif /* NETDATA_PLUGIN_IPC_H */

