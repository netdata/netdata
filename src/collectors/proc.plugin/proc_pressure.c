// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_PRESSURE_NAME "/proc/pressure"
#define CONFIG_SECTION_PLUGIN_PROC_PRESSURE "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_PRESSURE_NAME

// linux calculates this every 2 seconds, see kernel/sched/psi.c PSI_FREQ
#define MIN_PRESSURE_UPDATE_EVERY 2

static int pressure_update_every = 0;

static struct pressure resources[PRESSURE_NUM_RESOURCES] = {
    {
        .some = {
                .available = true,
                .share_time = {.id = "cpu_some_pressure", .title = "CPU some pressure"},
                .total_time = {.id = "cpu_some_pressure_stall_time", .title = "CPU some pressure stall time"}
                },
        .full = {
                // Disable CPU full pressure.
                // See https://github.com/torvalds/linux/commit/890d550d7dbac7a31ecaa78732aa22be282bb6b8
                .available = false,
                .share_time = {.id = "cpu_full_pressure", .title = "CPU full pressure"},
                .total_time = {.id = "cpu_full_pressure_stall_time", .title = "CPU full pressure stall time"}
                },
    },
    {
        .some = {
                .available = true,
                .share_time = {.id = "memory_some_pressure", .title = "Memory some pressure"},
                .total_time = {.id = "memory_some_pressure_stall_time", .title = "Memory some pressure stall time"}
                },
        .full = {
                .available = true,
                .share_time = {.id = "memory_full_pressure", .title = "Memory full pressure"},
                .total_time = {.id = "memory_full_pressure_stall_time", .title = "Memory full pressure stall time"}
                },
    },
    {
        .some = {
                .available = true,
                .share_time = {.id = "io_some_pressure", .title = "I/O some pressure"},
                .total_time = {.id = "io_some_pressure_stall_time", .title = "I/O some pressure stall time"}
                },
        .full = {
                .available = true,
                .share_time = {.id = "io_full_pressure", .title = "I/O full pressure"},
                .total_time = {.id = "io_full_pressure_stall_time", .title = "I/O full pressure stall time"}
                },
    },
    {
        .some = {
                // this is not available
                .available = false,
                .share_time = {.id = "irq_some_pressure", .title = "IRQ some pressure"},
                .total_time = {.id = "irq_some_pressure_stall_time", .title = "IRQ some pressure stall time"}
                },
        .full = {
                .available = true,
                .share_time = {.id = "irq_full_pressure", .title = "IRQ full pressure"},
                .total_time = {.id = "irq_full_pressure_stall_time", .title = "IRQ full pressure stall time"}
                },
    },
};

static struct resource_info {
    procfile *pf;
    const char *name;       // metric file name
    const char *family;     // webui section name
    int section_priority;
} resource_info[PRESSURE_NUM_RESOURCES] = {
        { .name = "cpu",    .family = "cpu",    .section_priority = NETDATA_CHART_PRIO_SYSTEM_CPU },
        { .name = "memory", .family = "ram",    .section_priority = NETDATA_CHART_PRIO_SYSTEM_RAM },
        { .name = "io",     .family = "disk",   .section_priority = NETDATA_CHART_PRIO_SYSTEM_IO },
        { .name = "irq",    .family = "interrupts",  .section_priority = NETDATA_CHART_PRIO_SYSTEM_INTERRUPTS },
};

void update_pressure_charts(struct pressure_charts *pcs) {
    if (pcs->share_time.st) {
        rrddim_set_by_pointer(
            pcs->share_time.st, pcs->share_time.rd10, (collected_number)(pcs->share_time.value10 * 100));
        rrddim_set_by_pointer(
            pcs->share_time.st, pcs->share_time.rd60, (collected_number)(pcs->share_time.value60 * 100));
        rrddim_set_by_pointer(
            pcs->share_time.st, pcs->share_time.rd300, (collected_number)(pcs->share_time.value300 * 100));
        rrdset_done(pcs->share_time.st);
    }
    if (pcs->total_time.st) {
        rrddim_set_by_pointer(
            pcs->total_time.st, pcs->total_time.rdtotal, (collected_number)(pcs->total_time.value_total));
        rrdset_done(pcs->total_time.st);
    }
}

static void proc_pressure_do_resource(procfile *ff, int res_idx, size_t line, bool some) {
    struct pressure_charts *pcs;
    struct resource_info ri;
    pcs = some ? &resources[res_idx].some : &resources[res_idx].full;
    ri = resource_info[res_idx];

    if (unlikely(!pcs->share_time.st)) {
        pcs->share_time.st = rrdset_create_localhost(
            "system",
            pcs->share_time.id,
            NULL,
            ri.family,
            NULL,
            pcs->share_time.title,
            "percentage",
            PLUGIN_PROC_NAME,
            PLUGIN_PROC_MODULE_PRESSURE_NAME,
            ri.section_priority + (some ? 40 : 50),
            pressure_update_every,
            RRDSET_TYPE_LINE);
        pcs->share_time.rd10 =
            rrddim_add(pcs->share_time.st, some ? "some 10" : "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd60 =
            rrddim_add(pcs->share_time.st, some ? "some 60" : "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd300 =
            rrddim_add(pcs->share_time.st, some ? "some 300" : "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    pcs->share_time.value10 = strtod(procfile_lineword(ff, line, 2), NULL);
    pcs->share_time.value60 = strtod(procfile_lineword(ff, line, 4), NULL);
    pcs->share_time.value300 = strtod(procfile_lineword(ff, line, 6), NULL);

    if (unlikely(!pcs->total_time.st)) {
        pcs->total_time.st = rrdset_create_localhost(
            "system",
            pcs->total_time.id,
            NULL,
            ri.family,
            NULL,
            pcs->total_time.title,
            "ms",
            PLUGIN_PROC_NAME,
            PLUGIN_PROC_MODULE_PRESSURE_NAME,
            ri.section_priority + (some ? 45 : 55),
            pressure_update_every,
            RRDSET_TYPE_LINE);
        pcs->total_time.rdtotal = rrddim_add(pcs->total_time.st, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    pcs->total_time.value_total = str2ull(procfile_lineword(ff, line, 8), NULL) / 1000;
}

static void proc_pressure_do_resource_some(procfile *ff, int res_idx, size_t line) {
    proc_pressure_do_resource(ff, res_idx, line, true);
}

static void proc_pressure_do_resource_full(procfile *ff, int res_idx, size_t line) {
    proc_pressure_do_resource(ff, res_idx, line, false);
}

int do_proc_pressure(int update_every, usec_t dt) {
    int ok_count = 0;
    int i;

    static usec_t next_pressure_dt = 0;
    static const char *base_path = NULL;

    update_every = (update_every < MIN_PRESSURE_UPDATE_EVERY) ? MIN_PRESSURE_UPDATE_EVERY : update_every;
    pressure_update_every = update_every;

    if (next_pressure_dt <= dt) {
        next_pressure_dt = update_every * USEC_PER_SEC;
    } else {
        next_pressure_dt -= dt;
        return 0;
    }

    if (unlikely(!base_path))
        base_path = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_PRESSURE, "base path of pressure metrics", "/proc/pressure");

    for (i = 0; i < PRESSURE_NUM_RESOURCES; i++) {
        procfile *ff = resource_info[i].pf;
        int do_some = resources[i].some.enabled, do_full = resources[i].full.enabled;

        if (!resources[i].some.available && !resources[i].full.available)
            continue;

        if (unlikely(!ff)) {
            char filename[FILENAME_MAX + 1];
            char config_key[CONFIG_MAX_NAME + 1];

            snprintfz(filename
                      , FILENAME_MAX
                      , "%s%s/%s"
                      , netdata_configured_host_prefix
                      , base_path
                      , resource_info[i].name);

            do_some = resources[i].some.available ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;
            do_full = resources[i].full.available ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;

            snprintfz(config_key, CONFIG_MAX_NAME, "enable %s some pressure", resource_info[i].name);
            do_some = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_PRESSURE, config_key, do_some);
            resources[i].some.enabled = do_some;

            snprintfz(config_key, CONFIG_MAX_NAME, "enable %s full pressure", resource_info[i].name);
            do_full = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_PRESSURE, config_key, do_full);
            resources[i].full.enabled = do_full;

            if (!do_full && !do_some) {
                resources[i].some.available = false;
                resources[i].full.available = false;
                continue;
            }

            ff = procfile_open(filename, " =", PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
            if (unlikely(!ff)) {
                // PSI IRQ was added recently (https://github.com/torvalds/linux/commit/52b1364ba0b105122d6de0e719b36db705011ac1) 
                if (strcmp(resource_info[i].name, "irq") != 0)
                    collector_error("Cannot read pressure information from %s.", filename);
                resources[i].some.available = false;
                resources[i].full.available = false;
                continue;
            }
        }

        ff = procfile_readall(ff);
        resource_info[i].pf = ff;
        if (unlikely(!ff))
            continue;

        size_t lines = procfile_lines(ff);
        if (unlikely(lines < 1)) {
            collector_error("%s has no lines.", procfile_filename(ff));
            continue;
        }

        for(size_t l = 0; l < lines ;l++) {
            const char *key = procfile_lineword(ff, l, 0);
            if(strcmp(key, "some") == 0) {
                if(do_some) {
                    proc_pressure_do_resource_some(ff, i, l);
                    update_pressure_charts(&resources[i].some);
                    ok_count++;
                }
            }
            else if(strcmp(key, "full") == 0) {
                if(do_full) {
                    proc_pressure_do_resource_full(ff, i, l);
                    update_pressure_charts(&resources[i].full);
                    ok_count++;
                }
            }
        }
    }

    if(!ok_count)
        return 1;

    return 0;
}
