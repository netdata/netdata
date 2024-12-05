// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SOCKET_H
#define NETDATA_SOCKET_H

#include "../libnetdata.h"

#define ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS 100

ssize_t send_timeout(NETDATA_SSL *ssl,int sockfd, void *buf, size_t len, int flags, time_t timeout);
int wait_on_socket_or_cancel_with_timeout(NETDATA_SSL *ssl, int fd, int timeout_ms, short int poll_events, short int *revents);

bool fd_is_socket(int fd);
bool is_socket_closed(int fd);

int sock_setnonblock(int fd);
int sock_delnonblock(int fd);
int sock_setreuse(int fd, int reuse);
void sock_setcloexec(int fd);
int sock_setreuse_port(int fd, int reuse);
int sock_enlarge_in(int fd);
int sock_enlarge_out(int fd);

int connection_allowed(int fd, char *client_ip, char *client_host, size_t hostsize,
                              SIMPLE_PATTERN *access_list, const char *patname, int allow_dns);
int accept_socket(int fd, int flags, char *client_ip, size_t ipsize, char *client_port, size_t portsize,
                         char *client_host, size_t hostsize, SIMPLE_PATTERN *access_list, int allow_dns);

#ifndef HAVE_ACCEPT4
int accept4(int sock, struct sockaddr *addr, socklen_t *addrlen, int flags);
#endif /* #ifndef HAVE_ACCEPT4 */

#ifdef SOCK_CLOEXEC
#define DEFAULT_SOCKET_FLAGS SOCK_CLOEXEC
#else
#define DEFAULT_SOCKET_FLAGS 0
#endif


bool ip_to_hostname(const char *ip, char *dst, size_t dst_len);

#endif //NETDATA_SOCKET_H
