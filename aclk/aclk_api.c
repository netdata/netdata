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

#ifdef ENABLE_ACLK
void *aclk_starter(void *ptr) {
    //TODO read config
#ifdef ACLK_NG
    if (aclk_ng)
        return aclk_main(ptr);
#endif
#ifdef ACLK_LEGACY
    if (!aclk_ng)
        return legacy_aclk_main(ptr);
#endif
    fatal("ACLK couldn't be started");
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
    fatal("ACLK_UPDATE_CHART");
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
    fatal("ACLK_UPDATE_ALARM");
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
    fatal("ACLK_ADD_COLLECTOR");
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
    fatal("ACLK_DEL_COLLECTOR");
}

#endif /* ENABLE_ACLK */

struct label *add_aclk_host_labels(struct label *label) {
#ifdef ACLK_NG
    label = add_label_to_list(label, "_aclk_ng_available", "true", LABEL_SOURCE_AUTO);
#else
    label = add_label_to_list(label, "_aclk_ng_available", "false", LABEL_SOURCE_AUTO);
#endif
#ifdef ACLK_LEGACY
    label = add_label_to_list(label, "_aclk_legacy", "true", LABEL_SOURCE_AUTO);
#else
    label = add_label_to_list(label, "_aclk_legacy", "false", LABEL_SOURCE_AUTO);
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
