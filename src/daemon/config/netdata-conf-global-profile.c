// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-global-profile.h"
#include "streaming/stream-conf.h"
#include "netdata-conf.h"

ENUM_STR_MAP_DEFINE(ND_CONF_PROFILE) = {
    { .id = ND_CONF_PROFILE_STANDALONE, .name = "standalone" },
    { .id = ND_CONF_PROFILE_PARENT,     .name = "parent" },
    { .id = ND_CONF_PROFILE_CHILD,      .name = "child" },
    { .id = ND_CONF_PROFILE_IOT,        .name = "iot" },

    // terminator
    { . id = 0, .name = NULL }
};

BITMAP_STR_DEFINE_FUNCTIONS(ND_CONF_PROFILE, ND_CONF_PROFILE_NONE, "");

static inline ND_CONF_PROFILE prefer_profile(ND_CONF_PROFILE setting, ND_CONF_PROFILE preferred, ND_CONF_PROFILE out_of) {
    if(setting & preferred) {
        setting &= ~out_of;
        setting |= preferred;
    }
    return setting;
}

ND_CONF_PROFILE netdata_conf_global_profile(bool recheck) {
    static ND_CONF_PROFILE profile = ND_CONF_PROFILE_NONE;

    if(!recheck && profile != ND_CONF_PROFILE_NONE)
        return profile;

    ND_CONF_PROFILE def_profile = ND_CONF_PROFILE_NONE;

    OS_SYSTEM_MEMORY mem = os_system_memory(true);
    size_t cpus = os_get_system_cpus_uncached();

    if(cpus <= 1 || (mem.ram_total_bytes && mem.ram_total_bytes < 1 * 1024 * 1024 * 1024))
        def_profile = ND_CONF_PROFILE_IOT;

    else if(stream_conf_is_parent(true))
        def_profile = ND_CONF_PROFILE_PARENT;

    else if(stream_conf_is_child())
        def_profile = ND_CONF_PROFILE_CHILD;

    else
        def_profile = ND_CONF_PROFILE_STANDALONE;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    ND_CONF_PROFILE_2buffer(wb, def_profile, " ");

    CLEAN_CHAR_P *s = strdupz(config_get(CONFIG_SECTION_GLOBAL, "profile", buffer_tostring(wb)));

    char *words[100];
    size_t n = quoted_strings_splitter(s, words, _countof(words), isspace_map_whitespace);
    ND_CONF_PROFILE pt = ND_CONF_PROFILE_NONE;
    for(size_t i = 0; i < n ;i++) {
        ND_CONF_PROFILE ptt = ND_CONF_PROFILE_2id_one(words[i]);
        if(ptt == ND_CONF_PROFILE_NONE)
            nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot understand netdata.conf [global].profile = %s", words[i]);
        pt |= ptt;
    }

    // sanity checks

    ND_CONF_PROFILE started = pt;

    if(!(pt & ND_CONF_PROFILES_SYSTEM))
        // system profile is missing from the settings
        pt |= (def_profile & ND_CONF_PROFILES_SYSTEM);

    pt = prefer_profile(pt, ND_CONF_PROFILE_PARENT, ND_CONF_PROFILES_SYSTEM);
    pt = prefer_profile(pt, ND_CONF_PROFILE_STANDALONE, ND_CONF_PROFILES_SYSTEM);
    pt = prefer_profile(pt, ND_CONF_PROFILE_CHILD, ND_CONF_PROFILES_SYSTEM);
    pt = prefer_profile(pt, ND_CONF_PROFILE_IOT, ND_CONF_PROFILES_SYSTEM);

    if(pt != started) {
        buffer_flush(wb);
        ND_CONF_PROFILE_2buffer(wb, pt, " ");
        config_set(CONFIG_SECTION_GLOBAL, "profile", buffer_tostring(wb));

        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "The netdata.conf setting [global].profile has been overwritten to '%s'",
               buffer_tostring(wb));
    }

    profile = pt;
    return profile;
}

void netdata_conf_apply_profile(void) {
    ND_CONF_PROFILE profile = netdata_conf_global_profile(true); (void)profile;
    if(netdata_conf_is_iot()) {
        storage_tiers = 1;
        default_rrd_update_every = 2;
        // web server threads = 6
        // aclk query threads = 6
        // backfill threads = 0
        // replication threads = 1 // can we disable them completely?
        // ml enabled = false
        // health enabled = true if there is no parent, otherwise false
        // disable internal plugins: tc, idlejitter, cgroups
        // disable external plugins: all except apps, network_viewer, systemd-journal
        // disable sqlite
    }
    else if(netdata_conf_is_parent()) {
        storage_tiers = 3;
        default_rrd_update_every = 1;
        // web server threads = dynamic
        // aclk query threads = dynamic
        // backfill threads = dynamic
        // replication threads = dynamic
        // ml enabled = true
        // health enabled = true
    }
    else if(netdata_conf_is_child()) {
        storage_tiers = 3;
        default_rrd_update_every = 1;
        // web server threads = 6
        // aclk query threads = 6
        // backfill threads = 0
        // replication threads = 1
        // ml enabled = false
        // health enabled = false
    }
    else /* if(netdata_conf_is_standalone()) */ {
        storage_tiers = 3;
        default_rrd_update_every = 1;
        // web server threads = 6
        // aclk query threads = 6
        // backfill threads = 0
        // replication threads = 1 // can we disable them completely?
        // ml enabled = true
        // health enabled = true
    }
}
