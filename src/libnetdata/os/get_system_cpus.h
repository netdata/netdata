// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GET_SYSTEM_CPUS_H
#define NETDATA_GET_SYSTEM_CPUS_H

#include "../libnetdata.h"

/*
 * The functions return the actual cpu cores of the system.
 * For configuring netdata workers, please use netdata_conf_cpus()
 * which is based on these settings, but it allows users to override it.
 *
 * External plugins can use the environment variable NETDATA_CONF_CPUS
 * to get the user configured setting.
 *
 */

#define os_get_system_cpus() os_get_system_cpus_cached(true)
#define os_get_system_cpus_uncached() os_get_system_cpus_cached(false)
size_t os_get_system_cpus_cached(bool cache);

#if defined(OS_LINUX)
size_t os_read_cpuset_cpus(const char *filename, size_t system_cpus);
#endif

#endif //NETDATA_GET_SYSTEM_CPUS_H
