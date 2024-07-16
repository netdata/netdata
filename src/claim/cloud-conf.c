// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"

struct config cloud_config = {
    .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = {
        .avl_tree = {
            .root = NULL,
            .compar = appconfig_section_compare
        },
        .rwlock = AVL_LOCK_INITIALIZER
    }
};

const char *cloud_url(void) {
    return appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "url", DEFAULT_CLOUD_BASE_URL);
}

const char *cloud_proxy(void) {
    const char *proxy = "env";

    proxy = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "proxy", proxy);

    // backwards compatibility, from when proxy was in netdata.conf
    // netdata.conf has bigger priority
    if (config_exists(CONFIG_SECTION_CLOUD, "proxy"))
        proxy = config_get(CONFIG_SECTION_CLOUD, "proxy", proxy);
    else
        config_set(CONFIG_SECTION_CLOUD, "proxy", proxy);

    return proxy;
}

bool cloud_insecure(void) {
    return appconfig_get_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "insecure", CONFIG_BOOLEAN_NO);
}

void load_cloud_conf(int silent) {
    errno_clear();
    char *filename = strdupz_path_subpath(netdata_configured_varlib_dir, "cloud.d/cloud.conf");
    int ret = appconfig_load(&cloud_config, filename, 1, NULL);

    if(!ret && !silent)
        netdata_log_info("CONFIG: cannot load cloud config '%s'. Running with internal defaults.", filename);

    freez(filename);

    // --------------------------------------------------------------------
    // Check if the cloud is enabled

    appconfig_move(&cloud_config,
                   CONFIG_SECTION_GLOBAL, "cloud base url",
                   CONFIG_SECTION_GLOBAL, "url");

    // This must be set before any point in the code that accesses it. Do not move it from this function.
    cloud_url();
}
