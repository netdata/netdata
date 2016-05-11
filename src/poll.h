/**
 * @file poll.h
 * @brief API to poll files.
 * @author Simon Nagl
 * @date 2016-05-10
 */

#ifndef NETDATA_POLL_H
#define NETDATA_POLL_H

#include <time.h>

#define POLLIN 1
#define POLLOUT 2
#define POLLERR 3

extern void *poll_main(void *ptr);

extern pthread_mutex_t poll_mtx;

/**
 * @brief Add file to the list of polled files.
 *
 * poll_file_register opens a file desciptor for filename and adds it to the list
 * of polled files. The file get's polled for event type. When a event occours, the 
 * timestamp get's updated.
 *
 * Use mutex poll_mtx to access the timestamp.
 *
 * To stop the polling of this file use poll_file_register.
 * It is important to also update timestamp.
 *
 * @param pathname Absolute paht to the file.
 * @param type One of POLLIN, POLLOUT or POLLERR
 *
 * @return A pointer to a timestamp. Always the timestamp of the last poll. NULL on failure.
 */
extern struct timeval* poll_file_register(char *pathname, int type);

/**
 * @brief Delete a file from the list of polled files
 *
 * poll_file_erase closes a the file descriptor for filename.
 *
 * @param pathname Absolute paht to the file.
 * @param type One of POLLIN, POLLOUT or POLLERR
 *
 * @return The timestamp of the last poll. NULL on failure.
 */
extern struct timeval* poll_file_erase(char *pathname, int type);

#endif /* NETDATA_POLL_H */
