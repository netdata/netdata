// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-profile.h"
#include "streaming/stream-conf.h"
#include "netdata-conf.h"

ENUM_STR_MAP_DEFINE(ND_PROFILE) = {
    { .id = ND_PROFILE_STANDALONE, .name = "standalone" },
    { .id = ND_PROFILE_PARENT,     .name = "parent" },
    { .id = ND_PROFILE_CHILD,      .name = "child" },
    { .id = ND_PROFILE_IOT,        .name = "iot" },

    // terminator
    { . id = 0, .name = NULL }
};

BITMAP_STR_DEFINE_FUNCTIONS(ND_PROFILE, ND_PROFILE_NONE, "");

static inline ND_PROFILE prefer_profile(ND_PROFILE setting, ND_PROFILE preferred, ND_PROFILE out_of) {
    if(setting & preferred) {
        setting &= ~out_of;
        setting |= preferred;
    }
    return setting;
}

ND_PROFILE nd_profile_detect_and_configure(bool recheck) {
    static ND_PROFILE profile = ND_PROFILE_NONE;
    if(!recheck && profile != ND_PROFILE_NONE)
        return profile;

    // required for detecting the profile
    stream_conf_load();
    netdata_conf_section_directories();

    ND_PROFILE def_profile = ND_PROFILE_NONE;

    OS_SYSTEM_MEMORY mem = os_system_memory(true);
    size_t cpus = os_get_system_cpus_uncached();

    if(cpus <= 1 || (OS_SYSTEM_MEMORY_OK(mem) && mem.ram_total_bytes < 1ULL * 1024 * 1024 * 1024))
        def_profile = ND_PROFILE_IOT;

    else if(stream_conf_is_parent(true))
        def_profile = ND_PROFILE_PARENT;

    else if(stream_conf_is_child())
        def_profile = ND_PROFILE_CHILD;

    else
        def_profile = ND_PROFILE_STANDALONE;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    ND_PROFILE_2buffer(wb, def_profile, " ");

    CLEAN_CHAR_P *s = strdupz(inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "profile", buffer_tostring(wb)));

    char *words[100];
    size_t n = quoted_strings_splitter(s, words, _countof(words), isspace_map_whitespace);
    ND_PROFILE pt = ND_PROFILE_NONE;
    for(size_t i = 0; i < n ;i++) {
        ND_PROFILE ptt = ND_PROFILE_2id_one(words[i]);
        if(ptt == ND_PROFILE_NONE)
            nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot understand netdata.conf [global].profile = %s", words[i]);
        pt |= ptt;
    }

    // sanity checks

    ND_PROFILE started = pt;

    if(!(pt & ND_CONF_PROFILES_SYSTEM))
        // system profile is missing from the settings
        pt |= (def_profile & ND_CONF_PROFILES_SYSTEM);

    pt = prefer_profile(pt, ND_PROFILE_PARENT, ND_CONF_PROFILES_SYSTEM);
    pt = prefer_profile(pt, ND_PROFILE_STANDALONE, ND_CONF_PROFILES_SYSTEM);
    pt = prefer_profile(pt, ND_PROFILE_CHILD, ND_CONF_PROFILES_SYSTEM);
    pt = prefer_profile(pt, ND_PROFILE_IOT, ND_CONF_PROFILES_SYSTEM);

    if(pt != started) {
        buffer_flush(wb);
        ND_PROFILE_2buffer(wb, pt, " ");
        inicfg_set(&netdata_config, CONFIG_SECTION_GLOBAL, "profile", buffer_tostring(wb));

        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "The netdata.conf setting [global].profile has been overwritten to '%s'",
               buffer_tostring(wb));
    }

    profile = pt;
    return profile;
}

struct nd_profile_t nd_profile = { 0 };

void nd_profile_setup(void) {
    FUNCTION_RUN_ONCE();

    ND_PROFILE profile = nd_profile_detect_and_configure(true); (void)profile;
    if(netdata_conf_is_iot()) {
        nd_profile.storage_tiers = 3;       // MUST BE 1
        nd_profile.update_every = 1;        // MUST BE 2
        nd_profile.malloc_arenas = 1;
        nd_profile.malloc_trim = 16 * 1024;
        nd_profile.stream_sender_compression = ND_COMPRESSION_FASTEST;
        nd_profile.dbengine_journal_v2_unmount_time = 120;
        nd_profile.max_page_size = 16 * 1024;
        nd_profile.ml_enabled = CONFIG_BOOLEAN_NO;
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
        nd_profile.storage_tiers = 3;
        nd_profile.update_every = 1;
        nd_profile.malloc_arenas = 4;
        nd_profile.malloc_trim = 128 * 1024;
        nd_profile.stream_sender_compression = ND_COMPRESSION_FASTEST;
        nd_profile.dbengine_journal_v2_unmount_time = 0;
        nd_profile.max_page_size = 2 * 1024 * 1024; // 2MB for THP
        nd_profile.ml_enabled = CONFIG_BOOLEAN_AUTO;
        // web server threads = dynamic
        // aclk query threads = dynamic
        // backfill threads = dynamic
        // replication threads = dynamic
        // ml enabled = true
        // health enabled = true
    }
    else if(netdata_conf_is_child()) {
        nd_profile.storage_tiers = 3;
        nd_profile.update_every = 1;
        nd_profile.malloc_arenas = 1;
        nd_profile.malloc_trim = 32 * 1024;
        nd_profile.stream_sender_compression = ND_COMPRESSION_DEFAULT;
        nd_profile.dbengine_journal_v2_unmount_time = 120;
        nd_profile.max_page_size = 32 * 1024;
        nd_profile.ml_enabled = CONFIG_BOOLEAN_AUTO;
        // web server threads = 6
        // aclk query threads = 6
        // backfill threads = 0
        // replication threads = 1
        // ml enabled = false
        // health enabled = false
    }
    else /* if(netdata_conf_is_standalone()) */ {
        nd_profile.storage_tiers = 3;
        nd_profile.update_every = 1;
        nd_profile.malloc_arenas = 1;
        nd_profile.malloc_trim = 64 * 1024;
        nd_profile.stream_sender_compression = ND_COMPRESSION_DEFAULT;
        nd_profile.dbengine_journal_v2_unmount_time = 120;
        nd_profile.max_page_size = 64 * 1024;
        nd_profile.ml_enabled = CONFIG_BOOLEAN_AUTO;
        // web server threads = 6
        // aclk query threads = 6
        // backfill threads = 0
        // replication threads = 1 // can we disable them completely?
        // ml enabled = true
        // health enabled = true
    }

    aral_optimal_malloc_page_size_set(nd_profile.max_page_size);
    netdata_conf_glibc_malloc_initialize(nd_profile.malloc_arenas, nd_profile.malloc_trim);
    stream_conf_set_sender_compression_levels(nd_profile.stream_sender_compression);
}
