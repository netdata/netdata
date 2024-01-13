// SPDX-License-Identifier: GPL-3.0-or-later

#include "silencers.h"
#include "daemon/common.h"

SILENCERS *silencers;
char *silencers_filename;

/**
 * Health Silencers add
 *
 * Add more one silencer to the list of silencers.
 *
 * @param silencer
 */
void health_silencers_add(SILENCER *silencer) {
    // Add the created instance to the linked list in silencers
    silencer->next = silencers->silencers;
    silencers->silencers = silencer;
    netdata_log_debug(
        D_HEALTH,
        "HEALTH command API: Added silencer %s:%s:%s:%s",
        silencer->alarms,
        silencer->charts,
        silencer->contexts,
        silencer->hosts);
}

int health_initialize_global_silencers()
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/health.silencers.json", netdata_configured_varlib_dir);
    silencers_filename = config_get(CONFIG_SECTION_HEALTH, "silencers file", filename);

    silencers = mallocz(sizeof(SILENCERS));
    silencers->all_alarms = 0;
    silencers->stype = STYPE_NONE;
    silencers->silencers = NULL;
    return 0;
}

void health_silencers2file(BUFFER *wb) {
    if (wb->len == 0) return;

    FILE *fd = fopen(silencers_filename, "wb");
    if(fd) {
        size_t written = (size_t)fprintf(fd, "%s", wb->buffer) ;
        if (written == wb->len ) {
            netdata_log_info("Silencer changes written to %s", silencers_filename);
        }
        fclose(fd);
        return;
    }
    netdata_log_error("Silencer changes could not be written to %s. Error %s", silencers_filename, strerror(errno));
}

/**
 * Silencers init
 *
 * Function used to initialize the silencer structure.
 */
void health_silencers_init(void) {
    load_health_silencers(silencers_filename);
    netdata_log_info("Parsed health silencers file %s", silencers_filename);
}
