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
#endif /* ENABLE_ACLK */

void add_aclk_host_labels(void) {
    DICTIONARY *labels = localhost->host_labels;

#ifdef ENABLE_ACLK
    rrdlabels_add(labels, "_aclk_ng_available", "true", RRDLABEL_SRC_AUTO|RRDLABEL_SRC_ACLK);
#else
    rrdlabels_add(labels, "_aclk_ng_available", "false", RRDLABEL_SRC_AUTO|RRDLABEL_SRC_ACLK);
#endif
    rrdlabels_add(labels, "_aclk_legacy_available", "false", RRDLABEL_SRC_AUTO|RRDLABEL_SRC_ACLK);
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

    int mqtt5 = config_get_boolean(CONFIG_SECTION_CLOUD, "mqtt5", CONFIG_BOOLEAN_NO);

    rrdlabels_add(labels, "_mqtt_version", mqtt5 ? "5" : "3", RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_aclk_impl", "Next Generation", RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_aclk_proxy", proxy_str, RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_aclk_ng_new_cloud_protocol", "true", RRDLABEL_SRC_AUTO|RRDLABEL_SRC_ACLK);
#endif
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
