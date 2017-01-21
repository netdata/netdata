#ifndef NETDATA_NFACCT_H
#define NETDATA_NFACCT_H 1

/**
 * @file plugin_nfacct.h
 * @brief Thread to collect nfacct data.
 */

/**
 * Main method of the nfacct thread.
 *
 * This thread collects nfacct data.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
extern void *nfacct_main(void *ptr);

#endif /* NETDATA_NFACCT_H */

