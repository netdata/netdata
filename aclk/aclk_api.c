// SPDX-License-Identifier: GPL-3.0-or-later
#include "libnetdata/libnetdata.h"
#include "database/rrd.h"

#ifdef ENABLE_ACLK
#include "aclk.h"
#endif

int aclk_connected = 0;
int aclk_kill_link = 0;

usec_t aclk_session_us = 0;
time_t aclk_session_sec = 0;

int aclk_disable_runtime = 0;
int aclk_disable_single_updates = 0;

int aclk_stats_enabled;
int use_mqtt_5 = 0;

#define ACLK_IMPL_KEY_NAME "aclk implementation"

#ifdef ENABLE_ACLK
void *aclk_starter(void *ptr) {
    char *aclk_impl_req = config_get(CONFIG_SECTION_CLOUD, ACLK_IMPL_KEY_NAME, "ng");

    if (!strcasecmp(aclk_impl_req, "ng")) {
        return aclk_main(ptr);
    } else if (!strcasecmp(aclk_impl_req, "legacy")) {
        error("Legacy ACLK is not supported anymore key \"" ACLK_IMPL_KEY_NAME "\" in section \"" CONFIG_SECTION_CLOUD "\" ignored. Using ACLK-NG.");
    } else {
        error("Unknown value \"%s\" of key \"" ACLK_IMPL_KEY_NAME "\" in section \"" CONFIG_SECTION_CLOUD "\". Using ACLK-NG. This config key will be deprecated.", aclk_impl_req);
    }
    return aclk_main(ptr);
}

void aclk_single_update_disable()
{
    aclk_disable_single_updates = 1;
}

void aclk_single_update_enable()
{
    aclk_disable_single_updates = 0;
}
#endif /* ENABLE_ACLK */

struct label *add_aclk_host_labels(struct label *label) {
#ifdef ENABLE_ACLK
    label = add_label_to_list(label, "_aclk_ng_available", "true", LABEL_SOURCE_AUTO);
#else
    label = add_label_to_list(label, "_aclk_ng_available", "false", LABEL_SOURCE_AUTO);
#endif
    label = add_label_to_list(label, "_aclk_legacy_available", "false", LABEL_SOURCE_AUTO);
#ifdef ENABLE_ACLK
    ACLK_PROXY_TYPE aclk_proxy;
    char *proxy_str;
    aclk_get_proxy(&aclk_proxy);

    switch(aclk_proxy) {
        case PROXY_TYPE_SOCKS5:
            proxy_str = "SOCKS5";
            break;
        case PROXY_TYPE_HTTP:
            proxy_str = "HTTP";
            break;
        default:
            proxy_str = "none";
            break;
    }

    label = add_label_to_list(label, "_aclk_impl", "Next Generation", LABEL_SOURCE_AUTO);
    label = add_label_to_list(label, "_aclk_proxy", proxy_str, LABEL_SOURCE_AUTO);
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    label = add_label_to_list(label, "_aclk_ng_new_cloud_protocol", "true", LABEL_SOURCE_AUTO);
#else
    label = add_label_to_list(label, "_aclk_ng_new_cloud_protocol", "false", LABEL_SOURCE_AUTO);
#endif
#endif
    return label;
}

char *aclk_state(void) {
#ifndef ENABLE_ACLK
    return strdupz("ACLK Available: No");
#else
    return ng_aclk_state();
#endif
}

char *aclk_state_json(void) {
#ifndef ENABLE_ACLK
    return strdupz("{\"aclk-available\":false}");
#else
    return ng_aclk_state_json();
#endif
}
