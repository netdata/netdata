// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-directories.h"
#include "daemon/common.h"

void netdata_conf_section_directories(void) {
    FUNCTION_RUN_ONCE();

    nd_runtime_paths_load_directories_from_inicfg();

    pluginsd_initialize_plugin_directories();
    netdata_configured_primary_plugins_dir = plugin_directories[PLUGINSD_STOCK_PLUGINS_DIRECTORY_PATH];
}
