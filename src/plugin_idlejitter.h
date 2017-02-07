#ifndef NETDATA_PLUGIN_IDLEJITTER_H
#define NETDATA_PLUGIN_IDLEJITTER_H 1

/**
 * @file plugin_idlejitter.h
 * @brief Thread to collect IDLEJITTER metrics.
 */

/**
 * Main function of IDLEJITTER thread.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
extern void *cpuidlejitter_main(void *ptr);

#endif /* NETDATA_PLUGIN_IDLEJITTER_H */
