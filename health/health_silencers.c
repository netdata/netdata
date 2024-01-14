// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

const char *health_silencers_filename(void) {
    return string2str(health_globals.config.silencers_filename);
}

void health_set_silencers_filename(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/health.silencers.json", netdata_configured_varlib_dir);

    health_globals.config.silencers_filename =
        string_strdupz(config_get(CONFIG_SECTION_HEALTH, "silencers file", filename));
}
