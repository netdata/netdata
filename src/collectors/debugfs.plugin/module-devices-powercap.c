// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

struct zone_t {
    char *zone_chart_id;
    char *subzone_chart_id;
    char *name;
    char *path;

    unsigned long long max_energy_range_uj;
    unsigned long long energy_uj;

    struct zone_t *subzones;

    struct zone_t *prev, *next;
};

static struct zone_t *rapl_zones = NULL;

static bool get_measurement(const char *path, unsigned long long *energy_uj) {
    return read_single_number_file(path, energy_uj) == 0;
}

static struct zone_t *get_rapl_zone(const char *control_type __maybe_unused, struct zone_t *parent __maybe_unused, const char *dirname) {
    char temp[FILENAME_MAX + 1];
    snprintfz(temp, FILENAME_MAX, "%s/%s", dirname, "name");

    char name[FILENAME_MAX + 1] = "";
    if (read_txt_file(temp, name, sizeof(name)) != 0)
        return NULL;

    char *trimmed = trim(name);
    if (unlikely(trimmed == NULL || trimmed[0] == 0))
        return NULL;

    snprintfz(temp, FILENAME_MAX, "%s/%s", dirname, "max_energy_range_uj");
    unsigned long long max_energy_range_uj = 0;
    if (unlikely(read_single_number_file(temp, &max_energy_range_uj) != 0)) {
        collector_error("Cannot read %s", temp);
        return NULL;
    }

    snprintfz(temp, FILENAME_MAX, "%s/%s", dirname, "energy_uj");
    unsigned long long energy_uj;
    if (unlikely(!get_measurement(temp, &energy_uj))) {
        collector_info("%s: Cannot read %s", trimmed, temp);
        return NULL;
    }

    struct zone_t *zone = callocz(1, sizeof(*zone));

    zone->name = strdupz(trimmed);
    zone->path = strdupz(temp);

    zone->max_energy_range_uj = max_energy_range_uj;
    zone->energy_uj = energy_uj;

    collector_info("Found zone: \"%s\"", zone->name);

    return zone;
}

static struct zone_t *look_for_rapl_zones(const char *control_type, struct zone_t *parent, const char *path, int depth) {
    if(depth > 2)
        return NULL;

    struct zone_t *base = NULL;

    DIR *dir = opendir(path);
    if (unlikely(dir == NULL))
        return NULL;

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if (de->d_type != DT_DIR || de->d_name[0] == '.')
            continue;

        if(strncmp(de->d_name, "intel-rapl:", 11) != 0)
            continue;

        char zone_path[FILENAME_MAX + 1];
        snprintfz(zone_path, FILENAME_MAX, "%s/%s", path, de->d_name);

        struct zone_t *zone = get_rapl_zone(control_type, parent, zone_path);
        if(zone) {
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(base, zone, prev, next);

            if(!parent)
                zone->subzones = look_for_rapl_zones(control_type, zone, zone_path, depth + 1);
        }
    }

    closedir(dir);
    return base;
}

static struct zone_t *get_main_rapl_zones(void) {
    struct zone_t *base = NULL;

    char dirname[FILENAME_MAX + 1];
    snprintfz(dirname, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/virtual/powercap");

    DIR *dir = opendir(dirname);
    if (unlikely(dir == NULL))
        return 0;

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if (de->d_type != DT_DIR || de->d_name[0] == '.')
            continue;

        if(strncmp(de->d_name, "intel-rapl", 10) != 0)
            continue;

        char control_type_path[FILENAME_MAX + 1];
        snprintfz(control_type_path, FILENAME_MAX, "%s/%s", dirname, de->d_name);

        collector_info("Looking at control type \"%s\"", de->d_name);
        struct zone_t *zone = look_for_rapl_zones(de->d_name, NULL, control_type_path, 0);
        if(zone)
            DOUBLE_LINKED_LIST_APPEND_LIST_UNSAFE(base, zone, prev, next);
    }
    closedir(dir);

    return base;
}

int do_module_devices_powercap(int update_every, const char *name __maybe_unused) {

    if (unlikely(!rapl_zones)) {
        rapl_zones = get_main_rapl_zones();
        if (unlikely(!rapl_zones)) {
            collector_info("Failed to find powercap zones.");
            return 1;
        }
    }

    netdata_mutex_lock(&stdout_mutex);

    for(struct zone_t *zone = rapl_zones; zone ; zone = zone->next) {
        if(!zone->zone_chart_id) {
            char id[1000 + 1];
            snprintf(id, 1000, "cpu.powercap_intel_rapl_zone_%s", zone->name);
            zone->zone_chart_id = strdupz(id);

            printf(PLUGINSD_KEYWORD_CHART " '%s' '' 'Intel RAPL Zone Power Consumption' 'Watts' 'powercap' '%s' '%s' %d %d '' 'debugfs.plugin' 'intel_rapl'\n",
                    zone->zone_chart_id,
                    "cpu.powercap_intel_rapl_zone",
                    debugfs_rrdset_type_name(RRDSET_TYPE_LINE),
                    NETDATA_CHART_PRIO_POWERCAP,
                    update_every);

            printf(PLUGINSD_KEYWORD_CLABEL " 'zone' '%s' 1\n"
                    PLUGINSD_KEYWORD_CLABEL_COMMIT "\n",
                    zone->name);

            printf(PLUGINSD_KEYWORD_DIMENSION " 'power' '' %s 1 1000000 ''\n",
                    debugfs_rrd_algorithm_name(RRD_ALGORITHM_INCREMENTAL));

            // the sub-zones
            snprintf(id, 1000, "cpu.powercap_intel_rapl_subzones_%s", zone->name);
            zone->subzone_chart_id = strdupz(id);
            printf(PLUGINSD_KEYWORD_CHART " '%s' '' 'Intel RAPL Subzones Power Consumption' 'Watts' 'powercap' '%s' '%s' %d %d '' 'debugfs.plugin' 'intel_rapl'\n",
                    zone->subzone_chart_id,
                    "cpu.powercap_intel_rapl_subzones",
                    debugfs_rrdset_type_name(RRDSET_TYPE_LINE),
                    NETDATA_CHART_PRIO_POWERCAP + 1,
                    update_every);

            printf(PLUGINSD_KEYWORD_CLABEL " 'zone' '%s' 1\n"
                   PLUGINSD_KEYWORD_CLABEL_COMMIT "\n",
                    zone->name);

            for(struct zone_t *subzone = zone->subzones; subzone ; subzone = subzone->next) {
                printf(PLUGINSD_KEYWORD_DIMENSION " '%s' '' %s 1 1000000 ''\n",
                        subzone->name,
                        debugfs_rrd_algorithm_name(RRD_ALGORITHM_INCREMENTAL));
            }
        }

        if(get_measurement(zone->path, &zone->energy_uj)) {
            printf(PLUGINSD_KEYWORD_BEGIN " '%s'\n"
                    PLUGINSD_KEYWORD_SET " power = %llu\n"
                    PLUGINSD_KEYWORD_END "\n"
                    , zone->zone_chart_id
                    , zone->energy_uj);
        }

        if(zone->subzones) {
            printf(PLUGINSD_KEYWORD_BEGIN " '%s'\n",
                    zone->subzone_chart_id);

            for (struct zone_t *subzone = zone->subzones; subzone; subzone = subzone->next) {
                if(get_measurement(subzone->path, &subzone->energy_uj)) {
                    printf(PLUGINSD_KEYWORD_SET " '%s' = %llu\n",
                            subzone->name,
                            subzone->energy_uj);
                }
            }

            printf(PLUGINSD_KEYWORD_END "\n");
        }

    }

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);

    return 0;
}
