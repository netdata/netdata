// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_PRESSURE_NAME "/proc/pressure"
#define CONFIG_SECTION_PLUGIN_PROC_PRESSURE "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_PRESSURE_NAME

// linux calculates this every 2 seconds, see kernel/sched/psi.c PSI_FREQ
#define MIN_PRESSURE_UPDATE_EVERY 2


static struct pressure resources[PRESSURE_NUM_RESOURCES] = {
        {
            .some = { .id = "cpu_pressure", .title = "CPU Pressure" },
        },
        {
            .some = { .id = "memory_some_pressure", .title = "Memory Pressure" },
            .full = { .id = "memory_full_pressure", .title = "Memory Full Pressure" },
        },
        {
            .some = { .id = "io_some_pressure", .title = "I/O Pressure" },
            .full = { .id = "io_full_pressure", .title = "I/O Full Pressure" },
        },
};

static struct {
    procfile *pf;
    const char *name;       // metric file name
    const char *family;     // webui section name
    int section_priority;
} resource_info[PRESSURE_NUM_RESOURCES] = {
        { .name = "cpu",    .family = "cpu",    .section_priority = NETDATA_CHART_PRIO_SYSTEM_CPU },
        { .name = "memory", .family = "ram",    .section_priority = NETDATA_CHART_PRIO_SYSTEM_RAM },
        { .name = "io",     .family = "disk",   .section_priority = NETDATA_CHART_PRIO_SYSTEM_IO },
};

void update_pressure_chart(struct pressure_chart *chart) {
    rrddim_set_by_pointer(chart->st, chart->rd10, (collected_number)(chart->value10 * 100));
    rrddim_set_by_pointer(chart->st, chart->rd60, (collected_number) (chart->value60 * 100));
    rrddim_set_by_pointer(chart->st, chart->rd300, (collected_number) (chart->value300 * 100));

    rrdset_done(chart->st);
}

int do_proc_pressure(int update_every, usec_t dt) {
    int fail_count = 0;
    int i;

    static usec_t next_pressure_dt = 0;
    static char *base_path = NULL;

    update_every = (update_every < MIN_PRESSURE_UPDATE_EVERY) ? MIN_PRESSURE_UPDATE_EVERY : update_every;

    if (next_pressure_dt <= dt) {
        next_pressure_dt = update_every * USEC_PER_SEC;
    } else {
        next_pressure_dt -= dt;
        return 0;
    }

    if (unlikely(!base_path)) {
        base_path = config_get(CONFIG_SECTION_PLUGIN_PROC_PRESSURE, "base path of pressure metrics", "/proc/pressure");
    }

    for (i = 0; i < PRESSURE_NUM_RESOURCES; i++) {
        procfile *ff = resource_info[i].pf;
        int do_some = resources[i].some.enabled, do_full = resources[i].full.enabled;

        if (unlikely(!ff)) {
            char filename[FILENAME_MAX + 1];
            char config_key[CONFIG_MAX_NAME + 1];

            snprintfz(filename
                      , FILENAME_MAX
                      , "%s%s/%s"
                      , netdata_configured_host_prefix
                      , base_path
                      , resource_info[i].name);

            snprintfz(config_key, CONFIG_MAX_NAME, "enable %s some pressure", resource_info[i].name);
            do_some = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_PRESSURE, config_key, CONFIG_BOOLEAN_YES);
            resources[i].some.enabled = do_some;
            if (resources[i].full.id) {
                snprintfz(config_key, CONFIG_MAX_NAME, "enable %s full pressure", resource_info[i].name);
                do_full = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_PRESSURE, config_key, CONFIG_BOOLEAN_YES);
                resources[i].full.enabled = do_full;
            }

            ff = procfile_open(filename, " =", PROCFILE_FLAG_DEFAULT);
            if (unlikely(!ff)) {
                error("Cannot read pressure information from %s.", filename);
                fail_count++;
                continue;
            }
        }

        ff = procfile_readall(ff);
        resource_info[i].pf = ff;
        if (unlikely(!ff)) {
            fail_count++;
            continue;
        }

        size_t lines = procfile_lines(ff);
        if (unlikely(lines < 1)) {
            error("%s has no lines.", procfile_filename(ff));
            fail_count++;
            continue;
        }

        struct pressure_chart *chart;
        if (do_some) {
            chart = &resources[i].some;
            if (unlikely(!chart->st)) {
                chart->st = rrdset_create_localhost(
                        "system"
                        , chart->id
                        , NULL
                        , resource_info[i].family
                        , NULL
                        , chart->title
                        , "percentage"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_PRESSURE_NAME
                        , resource_info[i].section_priority + 40
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                chart->rd10 = rrddim_add(chart->st, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                chart->rd60 = rrddim_add(chart->st, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                chart->rd300 = rrddim_add(chart->st, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrdset_next(chart->st);
            }

            chart->value10 = strtod(procfile_lineword(ff, 0, 2), NULL);
            chart->value60 = strtod(procfile_lineword(ff, 0, 4), NULL);
            chart->value300 = strtod(procfile_lineword(ff, 0, 6), NULL);
            update_pressure_chart(chart);
        }

        if (do_full && lines > 2) {
            chart = &resources[i].full;
            if (unlikely(!chart->st)) {
                chart->st = rrdset_create_localhost(
                        "system"
                        , chart->id
                        , NULL
                        , resource_info[i].family
                        , NULL
                        , chart->title
                        , "percentage"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_PRESSURE_NAME
                        , resource_info[i].section_priority + 45
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                chart->rd10 = rrddim_add(chart->st, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                chart->rd60 = rrddim_add(chart->st, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                chart->rd300 = rrddim_add(chart->st, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrdset_next(chart->st);
            }

            chart->value10 = strtod(procfile_lineword(ff, 1, 2), NULL);
            chart->value60 = strtod(procfile_lineword(ff, 1, 4), NULL);
            chart->value300 = strtod(procfile_lineword(ff, 1, 6), NULL);
            update_pressure_chart(chart);
        }
    }

    if (PRESSURE_NUM_RESOURCES == fail_count) {
        return 1;
    }

    return 0;
}
