/**
 * @file poll.h
 * @brief API to poll files.
 * @author Simon Nagl
 */

#ifndef NETDATA_POLL_H
#define NETDATA_POLL_H

#include <time.h>

#define POLLIN 1
#define POLLOUT 2
#define POLLERR 3

extern void *poll_main(void *ptr);

/**
 * @brief Add file to the list of polled files.
 *
 * poll_file_register opens a file desciptor for path and adds it to the list
 * of polled files. The file get's polled for event type. You can check if a 
 * file got updated with poll_is_updated().
 *
 * To stop polling a file call poll_unregister().
 *
 * @param path Absolute paht to a file.
 * @param type One of POLLIN, POLLOUT or POLLERR
 *
 * @return A poll descriptor. Only use it for poll_occured() and 
 * poll_file_unregister().
 */
extern void *poll_file_register(char *path, int type);

/**
 * @brief Check if event occured since the last call of poll_occured or 
 * poll_file_register.
 *
 * @param poll_descriptor Poll descriptor returned by poll_file_register().
 *
 * @return Time passed since event occurred in microseconds. 0 for false.
 */
extern unsigned long long poll_occured(void *poll_descriptor);

/**
 * @brief Delete a file from the list of polled files
 *
 * Before deleting this function does the same as poll_occured.
 * Do not use poll_occured on poll_descriptor after calling this function.
 *
 * @param poll_descriptor Poll descriptor returned by poll_file_register.
 *
 * @return Time passed since event occurred in microseconds. 0 for false.
 */
extern unsigned long long poll_file_unregister(void *poll_descriptor);

#endif /* NETDATA_POLL_H */
