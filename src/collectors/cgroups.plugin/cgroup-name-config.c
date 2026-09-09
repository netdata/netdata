// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-name-config.h"

#include <limits.h>
#include <stdint.h>
#include <string.h>

bool cgroup_name_is_legacy_default_helper(const char *configured, const char *legacy_default) {
    return configured && legacy_default && strcmp(configured, legacy_default) == 0;
}

int cgroup_name_timeout_ms_from_seconds(time_t timeout_s) {
    if (timeout_s <= 0)
        return 0;

    if ((uintmax_t)timeout_s > (uintmax_t)INT_MAX / 1000)
        return INT_MAX;

    return (int)((uintmax_t)timeout_s * 1000);
}
