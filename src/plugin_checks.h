#ifndef NETDATA_PLUGIN_CHECKS_H
#define NETDATA_PLUGIN_CHECKS_H 1

/**
 * @file plugin_checks.h
 * @brief Thread fills some example charts.
 */

/**
 * Main method of checks thread.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
void *checks_main(void *ptr);

#endif /* NETDATA_PLUGIN_PROC_H */
