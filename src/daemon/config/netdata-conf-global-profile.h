// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETDATA_CONF_GLOBAL_PROFILE_H
#define NETDATA_NETDATA_CONF_GLOBAL_PROFILE_H

#include "libnetdata/libnetdata.h"

typedef enum {
    ND_CONF_PROFILE_NONE        = (0),
    ND_CONF_PROFILE_STANDALONE  = (1 << 0),
    ND_CONF_PROFILE_CHILD       = (1 << 1),
    ND_CONF_PROFILE_PARENT      = (1 << 2),
} ND_CONF_PROFILE;

BITMAP_STR_DEFINE_FUNCTIONS_EXTERN(ND_CONF_PROFILE);

ND_CONF_PROFILE netdata_conf_global_profile(void);

static inline bool netdata_conf_is_child_only(void) {
    return (netdata_conf_global_profile() & (ND_CONF_PROFILE_CHILD | ND_CONF_PROFILE_PARENT)) == ND_CONF_PROFILE_CHILD;
}

static inline bool netdata_conf_is_parent(void) {
    return (netdata_conf_global_profile() & ND_CONF_PROFILE_PARENT) == ND_CONF_PROFILE_PARENT;
}

#endif //NETDATA_NETDATA_CONF_GLOBAL_PROFILE_H
