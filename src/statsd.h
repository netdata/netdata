// SPDX-License-Identifier: GPL-3.0+

#ifndef NETDATA_STATSD_H
#define NETDATA_STATSD_H 1

#define STATSD_LISTEN_PORT 8125
#define STATSD_LISTEN_BACKLOG 4096

extern void *statsd_main(void *ptr);

#endif //NETDATA_STATSD_H
