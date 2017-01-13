#ifndef NETDATA_BACKENDS_H
#define NETDATA_BACKENDS_H 1

/**
 * @file backends.h
 * @brief This file holds the API to start the netdata backend thread.
 */
 
/** 
 * Thread for netdata backends.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
void *backends_main(void *ptr);

#endif /* NETDATA_BACKENDS_H */
