// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_NETIPC_H
#define NETDATA_CGROUP_NETIPC_H 1

#ifdef OS_LINUX
void cgroup_netipc_init(void);
void cgroup_netipc_cleanup(void);
#else
static inline void cgroup_netipc_init(void) {}
static inline void cgroup_netipc_cleanup(void) {}
#endif

#endif // NETDATA_CGROUP_NETIPC_H
