// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CONNECT_TO_H
#define NETDATA_CONNECT_TO_H

void foreach_entry_in_connection_string(const char *destination, bool (*callback)(char *entry, void *data), void *data);
int connect_to_this_ip46(
    int protocol,
    int socktype,
    const char *host,
    uint32_t scope_id,
    const char *service,
    struct timeval *timeout,
    bool *fallback_ipv4);
int connect_to_this(const char *definition, int default_port, struct timeval *timeout);
int connect_to_one_of(const char *destination, int default_port, struct timeval *timeout, size_t *reconnects_counter, char *connected_to, size_t connected_to_size);
int connect_to_one_of_urls(const char *destination, int default_port, struct timeval *timeout, size_t *reconnects_counter, char *connected_to, size_t connected_to_size);

#endif //NETDATA_CONNECT_TO_H
