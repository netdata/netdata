// SPDX-License-Identifier: GPL-3.0-or-later
#include "libnetdata/libnetdata.h"
#include "database/rrd.h"

#ifdef ACLK_NG
#include "aclk.h"
#endif
#ifdef ACLK_LEGACY
#include "legacy/agent_cloud_link.h"
#endif

int aclk_connected = 0;
int aclk_kill_link = 0;

usec_t aclk_session_us = 0;
time_t aclk_session_sec = 0;

int aclk_disable_runtime = 0;
int aclk_disable_single_updates = 0;

int aclk_stats_enabled;

#ifdef ACLK_NG
int aclk_ng = 1;
#else
int aclk_ng = 0;
#endif

#define ACLK_IMPL_KEY_NAME "aclk implementation"

#ifdef ENABLE_ACLK
void *aclk_starter(void *ptr) {
    char *aclk_impl_req = config_get(CONFIG_SECTION_CLOUD, ACLK_IMPL_KEY_NAME, "ng");

    if (!strcasecmp(aclk_impl_req, "ng")) {
        aclk_ng = 1;
    } else if (!strcasecmp(aclk_impl_req, "legacy")) {
        aclk_ng = 0;
    } else {
        error("Unknown value \"%s\" of key \"" ACLK_IMPL_KEY_NAME "\" in section \"" CONFIG_SECTION_CLOUD "\". Trying default ACLK %s.", aclk_impl_req, aclk_ng ? "NG" : "Legacy");
    }

#ifndef ACLK_NG
    if (aclk_ng) {
        error("Configuration requests ACLK-NG but it is not available in this agent. Switching to Legacy.");
        aclk_ng = 0;
    }
#endif

#ifndef ACLK_LEGACY
    if (!aclk_ng) {
        error("Configuration requests ACLK Legacy but it is not available in this agent. Switching to NG.");
        aclk_ng = 1;
    }
#endif

#ifdef ACLK_NG
    if (aclk_ng) {
        info("Starting ACLK-NG");
        return aclk_main(ptr);
    }
#endif
#ifdef ACLK_LEGACY
    if (!aclk_ng) {
        info("Starting ACLK Legacy");
        return legacy_aclk_main(ptr);
    }
#endif
    error_report("No ACLK could be started");
    return NULL;
}

void aclk_single_update_disable()
{
    aclk_disable_single_updates = 1;
}

void aclk_single_update_enable()
{
    aclk_disable_single_updates = 0;
}

void aclk_alarm_reload(void)
{
#ifdef ACLK_NG
    if (aclk_ng)
        ng_aclk_alarm_reload();
#endif
#ifdef ACLK_LEGACY
    if (!aclk_ng)
        legacy_aclk_alarm_reload();
#endif
}

int aclk_update_chart(RRDHOST *host, char *chart_name, int create)
{
#ifdef ACLK_NG
    if (aclk_ng)
        return ng_aclk_update_chart(host, chart_name, create);
#endif
#ifdef ACLK_LEGACY
    if (!aclk_ng)
        return legacy_aclk_update_chart(host, chart_name, create);
#endif
    error_report("No usable aclk_update_chart implementation");
    return 1;
}

int aclk_update_alarm(RRDHOST *host, ALARM_ENTRY *ae)
{
#ifdef ACLK_NG
    if (aclk_ng)
        return ng_aclk_update_alarm(host, ae);
#endif
#ifdef ACLK_LEGACY
    if (!aclk_ng)
        return legacy_aclk_update_alarm(host, ae);
#endif
    error_report("No usable aclk_update_alarm implementation");
    return 1;
}

void aclk_add_collector(RRDHOST *host, const char *plugin_name, const char *module_name)
{
#ifdef ACLK_NG
    if (aclk_ng)
        return ng_aclk_add_collector(host, plugin_name, module_name);
#endif
#ifdef ACLK_LEGACY
    if (!aclk_ng)
        return legacy_aclk_add_collector(host, plugin_name, module_name);
#endif
    error_report("No usable aclk_add_collector implementation");
}

void aclk_del_collector(RRDHOST *host, const char *plugin_name, const char *module_name)
{
#ifdef ACLK_NG
    if (aclk_ng)
        return ng_aclk_del_collector(host, plugin_name, module_name);
#endif
#ifdef ACLK_LEGACY
    if (!aclk_ng)
        return legacy_aclk_del_collector(host, plugin_name, module_name);
#endif
    error_report("No usable aclk_del_collector implementation");
}

#endif /* ENABLE_ACLK */

struct label *add_aclk_host_labels(struct label *label) {
#ifdef ACLK_NG
    label = add_label_to_list(label, "_aclk_ng_available", "true", LABEL_SOURCE_AUTO);
#else
    label = add_label_to_list(label, "_aclk_ng_available", "false", LABEL_SOURCE_AUTO);
#endif
#ifdef ACLK_LEGACY
    label = add_label_to_list(label, "_aclk_legacy_available", "true", LABEL_SOURCE_AUTO);
#else
    label = add_label_to_list(label, "_aclk_legacy_available", "false", LABEL_SOURCE_AUTO);
#endif
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

    label = add_label_to_list(label, "_aclk_impl", aclk_ng ? "Next Generation" : "Legacy", LABEL_SOURCE_AUTO);
    label = add_label_to_list(label, "_aclk_proxy", proxy_str, LABEL_SOURCE_AUTO);
#endif
    return label;
}
