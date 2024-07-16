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
    const char *url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "url", DEFAULT_CLOUD_BASE_URL);

    const char *from_env = getenv("NETDATA_CLAIM_URL");
    if(from_env && *from_env)
        url = appconfig_set(&cloud_config, CONFIG_SECTION_GLOBAL, "url", from_env);

    return url;
}

const char *cloud_proxy(void) {
    // load cloud.conf or internal default
    const char *proxy = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "proxy", "env");

    // backwards compatibility, from when proxy was in netdata.conf
    // netdata.conf has bigger priority
    if (config_exists(CONFIG_SECTION_CLOUD, "proxy")) {
        // get it from netdata.conf
        proxy = config_get(CONFIG_SECTION_CLOUD, "proxy", proxy);

        // update cloud.conf
        proxy = appconfig_set(&cloud_config, CONFIG_SECTION_GLOBAL, "proxy", proxy);
    }
    else {
        // set in netdata.conf the proxy of cloud.conf
        config_set(CONFIG_SECTION_CLOUD, "proxy", proxy);
    }

    const char *from_env = getenv("NETDATA_CLAIM_PROXY");
    if(from_env && *from_env) {
        // set it in netdata.conf
        config_set(CONFIG_SECTION_CLOUD, "proxy", proxy);

        // set it in cloud.conf
        proxy = appconfig_set(&cloud_config, CONFIG_SECTION_GLOBAL, "proxy", from_env);
    }

    return proxy;
}

bool cloud_insecure(void) {
    // load it from cloud.conf or use internal default
    bool insecure = appconfig_get_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "insecure", CONFIG_BOOLEAN_NO);

    const char *from_env = getenv("NETDATA_EXTRA_CLAIM_OPTS");
    if(from_env && *from_env && strstr(from_env, "-insecure") == 0)
        insecure = appconfig_set_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "insecure", CONFIG_BOOLEAN_YES);

    return insecure;
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
