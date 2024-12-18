// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-global-profile.h"
#include "streaming/stream-conf.h"
#include "netdata-conf.h"

ENUM_STR_MAP_DEFINE(ND_CONF_PROFILE) = {
    { .id = ND_CONF_PROFILE_NONE,       .name = "default" },
    { .id = ND_CONF_PROFILE_STANDALONE, .name = "standalone" },
    { .id = ND_CONF_PROFILE_CHILD,      .name = "child" },
    { .id = ND_CONF_PROFILE_PARENT,     .name = "parent" },

    // terminator
    { . id = 0, .name = NULL }
};

BITMAP_STR_DEFINE_FUNCTIONS(ND_CONF_PROFILE, ND_CONF_PROFILE_NONE, "");

ND_CONF_PROFILE netdata_conf_global_profile(void) {
    static ND_CONF_PROFILE profile = ND_CONF_PROFILE_NONE;

    if(profile != ND_CONF_PROFILE_NONE)
        return profile;

    ND_CONF_PROFILE def_profile = ND_CONF_PROFILE_NONE;

    if(stream_conf_is_parent(true))
        def_profile |= ND_CONF_PROFILE_PARENT;

    if(stream_conf_is_child())
        def_profile |= ND_CONF_PROFILE_CHILD;

    if(def_profile == ND_CONF_PROFILE_NONE)
        def_profile = ND_CONF_PROFILE_STANDALONE;

    CLEAN_BUFFER *def_wb = buffer_create(0, NULL);
    ND_CONF_PROFILE_2buffer(def_wb, def_profile, " ");

    CLEAN_CHAR_P *s = strdupz(config_get(CONFIG_SECTION_GLOBAL, "profile", buffer_tostring(def_wb)));

    char *words[100];
    size_t n = quoted_strings_splitter(s, words, _countof(words), isspace_map_whitespace);
    ND_CONF_PROFILE pt = ND_CONF_PROFILE_NONE;
    for(size_t i = 0; i < n ;i++) {
        ND_CONF_PROFILE ptt = ND_CONF_PROFILE_2id_one(words[i]);
        if(ptt == ND_CONF_PROFILE_NONE)
            nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot understand netdata.conf [global].profile = %s", words[i]);
        pt |= ptt;
    }

    if(pt == ND_CONF_PROFILE_NONE || !(pt & (ND_CONF_PROFILE_CHILD|ND_CONF_PROFILE_PARENT|ND_CONF_PROFILE_STANDALONE))) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "Cannot understand the netdata.conf [global].profile set, falling back to: %s", buffer_tostring(def_wb));
        pt = def_profile;
    }
    else {
        if((pt & (ND_CONF_PROFILE_PARENT|ND_CONF_PROFILE_STANDALONE)) == (ND_CONF_PROFILE_PARENT|ND_CONF_PROFILE_STANDALONE) ||
            (pt & (ND_CONF_PROFILE_CHILD|ND_CONF_PROFILE_STANDALONE)) == (ND_CONF_PROFILE_CHILD|ND_CONF_PROFILE_STANDALONE))
            // standalone cannot be combined with parent or child
            pt &= ~(ND_CONF_PROFILE_STANDALONE);
    }

    profile = pt;

    return profile;
}
