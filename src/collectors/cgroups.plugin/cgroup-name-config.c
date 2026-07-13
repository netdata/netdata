// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-name-config.h"

#include <string.h>

bool cgroup_name_is_legacy_default_helper(const char *configured, const char *legacy_default) {
    return configured && legacy_default && strcmp(configured, legacy_default) == 0;
}
