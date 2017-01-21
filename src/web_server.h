#ifndef NETDATA_WEB_SERVER_H
#define NETDATA_WEB_SERVER_H 1

/**
 * @file web_server.h
 * @brief API of the web server.
 */

#define WEB_PATH_FILE "file"             ///< web path file string
#define WEB_PATH_DATA "data"             ///< web path data string
#define WEB_PATH_DATASOURCE "datasource" ///< web path datasource string
#define WEB_PATH_GRAPH "graph"           ///< web path graph string

#define LISTEN_PORT 19999  ///< default port to listen
#define LISTEN_BACKLOG 100 ///< default backlog to listen

#ifndef MAX_LISTEN_FDS
#define MAX_LISTEN_FDS 100 ///< default maximum file descriptors to listen to
#endif

#define WEB_SERVER_MODE_MULTI_THREADED 0  ///< Multi threaded mode.
#define WEB_SERVER_MODE_SINGLE_THREADED 1 ///< Single threaded mode.
/// WEB_SERVER_MODE_*
extern int web_server_mode;

/**
 * Main method of thread of the web server.
 *
 * This starts a multi threaded web server.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
extern void *socket_listen_main_multi_threaded(void *ptr);
/**
 * Main method of thread of the web server.
 *
 * This starts a single threaded web server.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
extern void *socket_listen_main_single_threaded(void *ptr);
/**
 * Create sockets to listen to.
 *
 * @return the number of open sockets.
 */
extern int create_listen_sockets(void);
/**
 * Check if `fd` is in the list sockets listening for web server requests.
 *
 * @param fd to check
 * @return boolean
 */
extern int is_listen_socket(int fd);

#ifndef HAVE_ACCEPT4
/**
 * Replacement of `accept4()` for systems that do not have one.
 *
 * @see man 2 accept4
 *
 * Extracts the first connection request on the queue of pending connections 
 * for the listening socket, creates a new connected socket, and returns a new
 * file descriptor referring to that socket.
 *
 * The newly created socket is not in the listening state.
 * The original socket sockfd is unaffected by this call.
 *
 * @param sock created with socket(2), bound to a local address with bind(2), and is listening for connections after a listen(2).
 * @param addr to store the createt storage
 * @param addrlen size of `addr` in bytes.
 * @param flags SOCK_*
 * @return nonnegative integer on success. -1 on error.
 */
extern int accept4(int sock, struct sockaddr *addr, socklen_t *addrlen, int flags);

#ifndef SOCK_NONBLOCK
/**
 * Set the O_NONBLOCK file status flag on the new open file description.
 * Using this flag saves extra calls to fcntl(2) to achieve the same result.
 *
 * @see man 2 accept4
 */ 
#define SOCK_NONBLOCK 00004000
#endif                         /* #ifndef SOCK_NONBLOCK */

#ifndef SOCK_CLOEXEC
/**
 * Set the close-on-exec (FD_CLOEXEC) flag on the new file descriptor.
 * See the description of the O_CLOEXEC flag in open(2) for reasons 
 * why this may be useful.
 *
 * @see man 2 accept4
 */
#define SOCK_CLOEXEC 02000000 ///< ktsaou: Your help needed
#endif                        /* #ifndef SOCK_CLOEXEC */

#endif /* #ifndef HAVE_ACCEPT4 */

#endif /* NETDATA_WEB_SERVER_H */
