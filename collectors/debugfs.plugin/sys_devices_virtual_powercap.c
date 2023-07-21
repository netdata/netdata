// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"

#define METRIC_ID "system.power_consumption"

struct measurement_t {
    unsigned long long energy_uj;
    usec_t time_us;
};

struct zone_t {
    char name[FILENAME_MAX + 1];
    char path[FILENAME_MAX + 1];
    unsigned long long max_energy_range_uj;
    struct measurement_t measurement;
};

static int g_zones_count = 0;
static struct zone_t *g_zones = NULL;

static int get_measurement(const char *path, struct measurement_t *measurement)
{
    int result;
    if (likely((result = read_single_number_file(path, &measurement->energy_uj)) == 0)) {
        measurement->time_us = now_monotonic_high_precision_usec();
    }
    return result;
}

static double
calculate_watts(unsigned long long max_energy_range_uj, struct measurement_t *before, struct measurement_t *after)
{
    unsigned long long energy_uj = 0;
    usec_t delta_us = after->time_us - before->time_us;
    if (unlikely(delta_us == 0)) {
        return 0;
    }

    if (likely(after->energy_uj >= before->energy_uj)) {
        energy_uj = after->energy_uj - before->energy_uj;
    } else {
        energy_uj = after->energy_uj + (max_energy_range_uj - before->energy_uj);
    }
    return (double)energy_uj / delta_us;
}

static struct zone_t *
get_zone(const char *control_type, struct zone_t *parent, const char *dirname, struct zone_t **zones, size_t *count)
{
    char temp[FILENAME_MAX + 1];
    snprintfz(temp, FILENAME_MAX, "%s/%s", dirname, "name");

    char name[FILENAME_MAX + 1] = "";
    if (read_file(temp, name, sizeof(name) - 1) != 0) {
        return NULL;
    }

    char *trimmed = trim(name);
    if (unlikely(trimmed == NULL || trimmed[0] == 0)) {
        return NULL;
    }

    snprintfz(temp, FILENAME_MAX, "%s/%s", dirname, "max_energy_range_uj");
    unsigned long long max_energy_range_uj = 0;
    if (unlikely(read_single_number_file(temp, &max_energy_range_uj) != 0)) {
        collector_error("Cannot read %s", temp);
        return NULL;
    }

    snprintfz(temp, FILENAME_MAX, "%s/%s", dirname, "energy_uj");
    struct measurement_t measurement;
    if (unlikely(get_measurement(temp, &measurement) != 0)) {
        collector_error("%s: Cannot read %s", trimmed, temp);
        return NULL;
    }

    int parent_idx = -1;
    if (parent != NULL) {
        parent_idx = parent - *zones;
    }

    *zones = (struct zone_t *)reallocz(*zones, sizeof(struct zone_t) * (*count + 1));

    struct zone_t *zone = &(*zones)[*count];
    strcpy(zone->path, temp);
    if (parent_idx >= 0) {
        snprintfz(zone->name, FILENAME_MAX, "%s/%s", (*zones)[parent_idx].name, trimmed);
    } else {
        snprintfz(zone->name, FILENAME_MAX, "%s/%s", control_type, trimmed);
    }
    zone->max_energy_range_uj = max_energy_range_uj;
    zone->measurement = measurement;

    collector_info("Found zone: \"%s\"", zone->name);

    ++(*count);

    return zone;
}

static void
look_for_zones(const char *control_type, struct zone_t *parent, const char *path, struct zone_t **zones, size_t *count)
{
    DIR *dir = opendir(path);
    if (unlikely(dir == NULL)) {
        return;
    }

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if (de->d_type != DT_DIR || de->d_name[0] == '.') {
            continue;
        }

        char zone_path[FILENAME_MAX + 1];
        snprintfz(zone_path, FILENAME_MAX, "%s/%s", path, de->d_name);

        collector_info("Looking for zone in \"%s\"", zone_path);

        struct zone_t *zone = get_zone(control_type, parent, zone_path, zones, count);
        if (zone != NULL) {
            look_for_zones(control_type, zone, zone_path, zones, count);
        }
    }

    closedir(dir);
}

static int get_rapl_zones(struct zone_t **zones)
{
    *zones = NULL;

    char dirname[FILENAME_MAX + 1];
    snprintfz(dirname, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/virtual/powercap");

    DIR *dir = opendir(dirname);
    if (unlikely(dir == NULL)) {
        return 0;
    }

    size_t count = 0;

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if (de->d_type != DT_DIR || de->d_name[0] == '.') {
            continue;
        }

        char control_type_path[FILENAME_MAX + 1];
        snprintfz(control_type_path, FILENAME_MAX, "%s/%s", dirname, de->d_name);

        collector_info("Looking at control type \"%s\"", de->d_name);
        look_for_zones(de->d_name, NULL, control_type_path, zones, &count);
    }
    closedir(dir);

    return count;
}

static void send_chart(int update_every, const char *name)
{
    fprintf(
        stdout,
        "CHART %s '' 'Power Consumption' 'Watts' 'power consumption' '' '%s' %d %d '%s' 'debugfs.plugin' ''\n",
        METRIC_ID,
        debugfs_rrdset_type_name(RRDSET_TYPE_LINE),
        NETDATA_CHART_PRIO_POWERCAP,
        update_every,
        name);
}

static void send_dimension(const char *zone_name)
{
    fprintf(
        stdout,
        "DIMENSION '%s' '%s' %s 1 %d ''\n",
        zone_name,
        zone_name,
        debugfs_rrd_algorithm_name(RRD_ALGORITHM_ABSOLUTE),
        1000);
}

static void send_begin()
{
    fprintf(stdout, "BEGIN %s\n", METRIC_ID);
}

static void send_set(const char *zone_name, collected_number value)
{
    fprintf(stdout, "SET %s = %lld\n", zone_name, value);
}

static void send_end_and_flush()
{
    fprintf(stdout, "END\n");
    fflush(stdout);
}

int do_sys_devices_virtual_powercap(int update_every, const char *name)
{
    if (unlikely(g_zones_count <= 0)) {
        g_zones_count = get_rapl_zones(&g_zones);
        if (unlikely(g_zones_count < 0)) {
            collector_error("Failed to find powercap zones.");
            return 1;
        } else if (unlikely(g_zones_count == 0)) {
            collector_info("No powercap zones found.");
            return 1;
        }

        send_chart(update_every, name);

        for (int ii = 0; ii < g_zones_count; ++ii) {
            send_dimension(g_zones[ii].name);
        }

        // This measurement is a derivative so start reporting on the next call
        return 0;
    }

    send_begin();

    for (int ii = 0; ii < g_zones_count; ++ii) {
        struct measurement_t measurement;
        struct zone_t *zone = &g_zones[ii];

        if (unlikely(get_measurement(zone->path, &measurement) != 0)) {
            collector_error("%s: Cannot read %s", zone->name, zone->path);
            continue;
        }
        double watts = calculate_watts(zone->max_energy_range_uj, &zone->measurement, &measurement);
        zone->measurement = measurement;

        send_set(zone->name, (collected_number)(watts * 1000));
    }

    send_end_and_flush();

    return 0;
}
