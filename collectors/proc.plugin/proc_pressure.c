// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_PRESSURE_NAME "/proc/pressure"
#define CONFIG_SECTION_PLUGIN_PROC_PRESSURE "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_PRESSURE_NAME
#define NUM_RESOURCES 3

// linux calculates this every 2 seconds, see kernel/sched/psi.c PSI_FREQ
#define MIN_PRESSURE_UPDATE_EVERY 2


struct pressure_chart {
    int enabled;

    const char *id;
    const char *title;
    RRDSET *st;
    RRDDIM *rd10;
    RRDDIM *rd60;
    RRDDIM *rd300;
};

static struct {
    const char *family;
    procfile *pf;
    struct {
        struct pressure_chart some;
        struct pressure_chart full;
    } charts;
} resources[NUM_RESOURCES] = {
        { .family = "cpu", .charts = {
                .some = { .id = "cpu", .title = "CPU Pressure" },
        }},

        { .family = "memory", .charts = {
                .some = { .id = "memory", .title = "Memory Pressure" },
                .full = { .id = "memory_full", .title = "Memory Full Pressure" },
        }},

        { .family = "io", .charts = {
                .some = { .id = "io", .title = "I/O Pressure" },
                .full = { .id = "io_full", .title = "I/O Full Pressure" },
        }},
};

void update_pressure_chart(struct pressure_chart *chart, double value10, double value60, double value300) {
    rrddim_set_by_pointer(chart->st, chart->rd10, (collected_number) (value10 * 100));
    rrddim_set_by_pointer(chart->st, chart->rd60, (collected_number) (value60 * 100));
    rrddim_set_by_pointer(chart->st, chart->rd300, (collected_number) (value300 * 100));

    rrdset_done(chart->st);
}

int do_proc_pressure(int update_every, usec_t dt) {
    int fail_count = 0;
    int i;

    static usec_t next_pressure_dt = 0;

    update_every = (update_every < MIN_PRESSURE_UPDATE_EVERY) ? MIN_PRESSURE_UPDATE_EVERY : update_every;

    if (next_pressure_dt <= dt) {
        next_pressure_dt = update_every * USEC_PER_SEC;
    } else {
        next_pressure_dt -= dt;
        return 0;
    }

    for (i = 0; i < NUM_RESOURCES; i++) {
        procfile *ff = resources[i].pf;
        int do_some = resources[i].charts.some.enabled, do_full = resources[i].charts.full.enabled;

        if (unlikely(!ff)) {
            char filename[FILENAME_MAX + 1];
            char config_key[CONFIG_MAX_NAME + 1];
            char *pressure_filename;

            snprintfz(filename
                      , FILENAME_MAX
                      , "%s%s/%s"
                      , netdata_configured_host_prefix
                      , PLUGIN_PROC_MODULE_PRESSURE_NAME
                      , resources[i].family);

            snprintfz(config_key, CONFIG_MAX_NAME, "path to %s pressure", resources[i].family);
            pressure_filename = config_get(CONFIG_SECTION_PLUGIN_PROC_PRESSURE, config_key, filename);

            snprintfz(config_key, CONFIG_MAX_NAME, "enable %s some pressure", resources[i].family);
            do_some = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_PRESSURE, config_key, CONFIG_BOOLEAN_YES);
            resources[i].charts.some.enabled = do_some;
            if (resources[i].charts.full.id) {
                snprintfz(config_key, CONFIG_MAX_NAME, "enable %s full pressure", resources[i].family);
                do_full = config_get_boolean(CONFIG_SECTION_PLUGIN_PROC_PRESSURE, config_key, CONFIG_BOOLEAN_YES);
                resources[i].charts.full.enabled = do_full;
            }

            ff = procfile_open(pressure_filename, " =", PROCFILE_FLAG_DEFAULT);
            if (unlikely(!ff)) {
                error("Cannot read pressure information from %s.", pressure_filename);
                fail_count++;
                continue;
            }
        }

        ff = procfile_readall(ff);
        resources[i].pf = ff;
        if (unlikely(!ff)) {
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
            chart = &resources[i].charts.some;
            if (unlikely(!chart->st)) {
                chart->st = rrdset_create_localhost(
                        "pressure"
                        , chart->id
                        , NULL
                        , resources[i].family
                        , NULL
                        , chart->title
                        , "percentage"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_PRESSURE_NAME
                        , NETDATA_CHART_PRIO_SYSTEM_PRESSURE + 2 * i
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                chart->rd10 = rrddim_add(chart->st, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                chart->rd60 = rrddim_add(chart->st, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                chart->rd300 = rrddim_add(chart->st, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrdset_next(chart->st);
            }

            double some10 = strtod(procfile_lineword(ff, 0, 2), NULL);
            double some60 = strtod(procfile_lineword(ff, 0, 4), NULL);
            double some300 = strtod(procfile_lineword(ff, 0, 6), NULL);
            update_pressure_chart(chart, some10, some60, some300);
        }

        if (do_full && lines > 2) {
            chart = &resources[i].charts.full;
            if (unlikely(!chart->st)) {
                chart->st = rrdset_create_localhost(
                        "pressure"
                        , chart->id
                        , NULL
                        , resources[i].family
                        , NULL
                        , chart->title
                        , "percentage"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_PRESSURE_NAME
                        , NETDATA_CHART_PRIO_SYSTEM_PRESSURE + 2 * i + 1
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                chart->rd10 = rrddim_add(chart->st, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                chart->rd60 = rrddim_add(chart->st, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
                chart->rd300 = rrddim_add(chart->st, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
            } else {
                rrdset_next(chart->st);
            }

            double full10 = strtod(procfile_lineword(ff, 1, 2), NULL);
            double full60 = strtod(procfile_lineword(ff, 1, 4), NULL);
            double full300 = strtod(procfile_lineword(ff, 1, 6), NULL);
            update_pressure_chart(&resources[i].charts.full, full10, full60, full300);
        }
    }

    if (NUM_RESOURCES == fail_count) {
        return 1;
    }

    return 0;
}
