//
// Created by costa on 24/12/2016.
//

#ifndef NETDATA_SOCKET_H
#define NETDATA_SOCKET_H

extern int connect_to(const char *definition, int default_port, struct timeval *timeout);
extern int connect_to_one_of(const char *destination, int default_port, struct timeval *timeout, size_t *reconnects_counter, char *connected_to, size_t connected_to_size);

extern ssize_t recv_timeout(int sockfd, void *buf, size_t len, int flags, int timeout);
extern ssize_t send_timeout(int sockfd, void *buf, size_t len, int flags, int timeout);

#endif //NETDATA_SOCKET_H
