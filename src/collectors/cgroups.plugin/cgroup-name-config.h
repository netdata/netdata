// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_NAME_CONFIG_H
#define NETDATA_CGROUP_NAME_CONFIG_H

#include <stdbool.h>

bool cgroup_name_is_legacy_default_helper(const char *configured, const char *legacy_default);

#endif // NETDATA_CGROUP_NAME_CONFIG_H
