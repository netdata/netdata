// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_NAME_CONFIG_H
#define NETDATA_CGROUP_NAME_CONFIG_H

#include <stdbool.h>
#include <time.h>

bool cgroup_name_is_legacy_default_helper(const char *configured, const char *legacy_default);
int cgroup_name_timeout_ms_from_seconds(time_t timeout_s);

#endif // NETDATA_CGROUP_NAME_CONFIG_H
