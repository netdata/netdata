/**
 * @file socket.h
 * @brief API to connect to a socket
 * @author costa
 * @since 24/12/2016.
 */

#ifndef NETDATA_SOCKET_H
#define NETDATA_SOCKET_H

/**
 * Connect to a socket.
 *
 * Definition format: `[PROTOCOL:]IP[%INTERFACE][:PORT]`
 *
 * - PROTOCOL  = tcp or udp
 * - IP        = IPv4 or IPv6 IP or hostname, optionally enclosed in [] (required for IPv6)
 * - INTERFACE = for IPv6 only, the network interface to use
 * - PORT      = port number or service name
 *
 * @param definition format
 * @param default_port if not in `definition_format`
 * @param timeout Timeout
 * @return file descriptor connected to
 */
extern int connect_to(const char *definition, int default_port, struct timeval *timeout);

#endif //NETDATA_SOCKET_H
