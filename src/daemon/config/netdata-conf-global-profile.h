// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETDATA_CONF_GLOBAL_PROFILE_H
#define NETDATA_NETDATA_CONF_GLOBAL_PROFILE_H

#include "libnetdata/libnetdata.h"

typedef enum {
    ND_CONF_PROFILE_NONE        = (0),

    // system profiles
    ND_CONF_PROFILE_PARENT      = (1 << 30),
    ND_CONF_PROFILE_STANDALONE  = (1 << 29),
    ND_CONF_PROFILE_CHILD       = (1 << 28),
    ND_CONF_PROFILE_IOT         = (1 << 27),

    // optional attributed to profiles

} ND_CONF_PROFILE;

#define ND_CONF_PROFILES_SYSTEM (ND_CONF_PROFILE_STANDALONE|ND_CONF_PROFILE_PARENT|ND_CONF_PROFILE_CHILD|ND_CONF_PROFILE_IOT)

BITMAP_STR_DEFINE_FUNCTIONS_EXTERN(ND_CONF_PROFILE);

ND_CONF_PROFILE netdata_conf_global_profile(bool recheck);

void netdata_conf_apply_profile(void);

static inline bool netdata_conf_is_iot(void) {
    return (netdata_conf_global_profile(false) & ND_CONF_PROFILES_SYSTEM) == ND_CONF_PROFILE_IOT;
}

static inline bool netdata_conf_is_standalone(void) {
    return (netdata_conf_global_profile(false) & ND_CONF_PROFILES_SYSTEM) == ND_CONF_PROFILE_STANDALONE;
}

static inline bool netdata_conf_is_child(void) {
    return (netdata_conf_global_profile(false) & ND_CONF_PROFILES_SYSTEM) == ND_CONF_PROFILE_CHILD;
}

static inline bool netdata_conf_is_parent(void) {
    return (netdata_conf_global_profile(false) & ND_CONF_PROFILES_SYSTEM) == ND_CONF_PROFILE_PARENT;
}

#endif //NETDATA_NETDATA_CONF_GLOBAL_PROFILE_H
