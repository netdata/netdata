// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GET_SYSTEM_CPUS_H
#define NETDATA_GET_SYSTEM_CPUS_H

#include "../libnetdata.h"

long os_get_system_cpus_cached(bool cache, bool for_netdata);

#endif //NETDATA_GET_SYSTEM_CPUS_H
