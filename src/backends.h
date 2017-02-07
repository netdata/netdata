#ifndef NETDATA_BACKENDS_H
#define NETDATA_BACKENDS_H 1

/**
 * @file backends.h
 * @brief Thread sending data to backends.
 */
 
/** 
 * Main function of backend thread.
 *
 * This method sends data to backends.
 *
 * @param ptr to struct netdata_static_thread
 */
void *backends_main(struct netdata_static_thread *ptr);

#endif /* NETDATA_BACKENDS_H */
