// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETDATA_CONF_PROFILE_H
#define NETDATA_NETDATA_CONF_PROFILE_H

#include "libnetdata/libnetdata.h"

typedef enum __attribute__((packed)) {
    ND_PROFILE_NONE = (0),

    // system profiles
    ND_PROFILE_PARENT       = (1 << 30),
    ND_PROFILE_STANDALONE   = (1 << 29),
    ND_PROFILE_CHILD        = (1 << 28),
    ND_PROFILE_IOT          = (1 << 27),

    // optional attributed to profiles

} ND_PROFILE;

typedef enum __attribute__((packed)) {
    ND_COMPRESSION_DEFAULT = 0,
    ND_COMPRESSION_FASTEST,
} ND_COMPRESSION_PROFILE;

#define ND_CONF_PROFILES_SYSTEM (ND_PROFILE_STANDALONE | ND_PROFILE_PARENT | ND_PROFILE_CHILD | ND_PROFILE_IOT)

BITMAP_STR_DEFINE_FUNCTIONS_EXTERN(ND_PROFILE);

ND_PROFILE nd_profile_detect_and_configure(bool recheck);

void nd_profile_setup(void);

static inline bool netdata_conf_is_iot(void) {
    return (nd_profile_detect_and_configure(false) & ND_CONF_PROFILES_SYSTEM) == ND_PROFILE_IOT;
}

static inline bool netdata_conf_is_standalone(void) {
    return (nd_profile_detect_and_configure(false) & ND_CONF_PROFILES_SYSTEM) == ND_PROFILE_STANDALONE;
}

static inline bool netdata_conf_is_child(void) {
    return (nd_profile_detect_and_configure(false) & ND_CONF_PROFILES_SYSTEM) == ND_PROFILE_CHILD;
}

static inline bool netdata_conf_is_parent(void) {
    return (nd_profile_detect_and_configure(false) & ND_CONF_PROFILES_SYSTEM) == ND_PROFILE_PARENT;
}

struct nd_profile_t {
    size_t storage_tiers;
    time_t update_every;
    size_t malloc_arenas;
    size_t malloc_trim;
    size_t max_page_size;
    time_t dbengine_journal_v2_unmount_time;
    ND_COMPRESSION_PROFILE stream_sender_compression;
    int ml_enabled;
};

extern struct nd_profile_t nd_profile;

#endif //NETDATA_NETDATA_CONF_PROFILE_H
