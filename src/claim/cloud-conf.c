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

void load_cloud_conf(int silent) {
    char *nd_disable_cloud = getenv("NETDATA_DISABLE_CLOUD");

    if (nd_disable_cloud && !strncmp(nd_disable_cloud, "1", 1))
        netdata_cloud_enabled = CONFIG_BOOLEAN_NO;

    errno_clear();
    char *filename = strdupz_path_subpath(netdata_configured_varlib_dir, "cloud.d/cloud.conf");
    int ret = appconfig_load(&cloud_config, filename, 1, NULL);

    if(!ret && !silent)
        netdata_log_info("CONFIG: cannot load cloud config '%s'. Running with internal defaults.", filename);

    freez(filename);

    // --------------------------------------------------------------------
    // Check if the cloud is enabled

#if defined( DISABLE_CLOUD ) || !defined( ENABLE_ACLK )

    netdata_cloud_enabled = CONFIG_BOOLEAN_NO;

#else

    netdata_cloud_enabled = appconfig_get_boolean_ondemand(
        &cloud_config, CONFIG_SECTION_GLOBAL, "enabled", netdata_cloud_enabled);

#endif

    // This must be set before any point in the code that accesses it. Do not move it from this function.
    appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", DEFAULT_CLOUD_BASE_URL);
}
