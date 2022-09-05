// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_INTERRUPTS_NAME "/proc/interrupts"
#define CONFIG_SECTION_PLUGIN_PROC_INTERRUPTS "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_INTERRUPTS_NAME

#define MAX_INTERRUPT_NAME 50

struct cpu_interrupt {
    unsigned long long value;
    RRDDIM *rd;
};

struct interrupt {
    int used;
    char *id;
    char name[MAX_INTERRUPT_NAME + 1];
    RRDDIM *rd;
    unsigned long long total;
    struct cpu_interrupt cpu[];
};

// since each interrupt is variable in size
// we use this to calculate its record size
#define recordsize(cpus) (sizeof(struct interrupt) + ((cpus) * sizeof(struct cpu_interrupt)))

// given a base, get a pointer to each record
#define irrindex(base, line, cpus) ((struct interrupt *)&((char *)(base))[(line) * recordsize(cpus)])

static inline struct interrupt *get_interrupts_array(size_t lines, int cpus) {
    static struct interrupt *irrs = NULL;
    static size_t allocated = 0;

    if(unlikely(lines != allocated)) {
        size_t l;
        int c;

        irrs = (struct interrupt *)reallocz(irrs, lines * recordsize(cpus));

        // reset all interrupt RRDDIM pointers as any line could have shifted
        for(l = 0; l < lines ;l++) {
            struct interrupt *irr = irrindex(irrs, l, cpus);
            irr->rd = NULL;
            irr->name[0] = '\0';
            for(c = 0; c < cpus ;c++)
                irr->cpu[c].rd = NULL;
        }

        allocated = lines;
    }

    return irrs;
}

int do_proc_interrupts(int update_every, usec_t dt) {
    (void)dt;
    static procfile *ff = NULL;
    static int cpus = -1, do_per_core = CONFIG_BOOLEAN_INVALID;
    struct interrupt *irrs = NULL;

    if(unlikely(do_per_core == CONFIG_BOOLEAN_INVALID))
        do_per_core = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_INTERRUPTS, "interrupts per core", CONFIG_BOOLEAN_AUTO);

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/interrupts");
        ff = procfile_open(config_get(CONFIG_SECTION_PLUGIN_PROC_INTERRUPTS, "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
    }
    if(unlikely(!ff))
        return 1;

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;
    size_t words = procfile_linewords(ff, 0);

    if(unlikely(!lines)) {
        error("Cannot read /proc/interrupts, zero lines reported.");
        return 1;
    }

    // find how many CPUs are there
    if(unlikely(cpus == -1)) {
        uint32_t w;
        cpus = 0;
        for(w = 0; w < words ; w++) {
            if(likely(strncmp(procfile_lineword(ff, 0, w), "CPU", 3) == 0))
                cpus++;
        }
    }

    if(unlikely(!cpus)) {
        error("PLUGIN: PROC_INTERRUPTS: Cannot find the number of CPUs in /proc/interrupts");
        return 1;
    }

    // allocate the size we need;
    irrs = get_interrupts_array(lines, cpus);
    irrs[0].used = 0;

    // loop through all lines
    for(l = 1; l < lines ;l++) {
        struct interrupt *irr = irrindex(irrs, l, cpus);
        irr->used = 0;
        irr->total = 0;

        words = procfile_linewords(ff, l);
        if(unlikely(!words)) continue;

        irr->id = procfile_lineword(ff, l, 0);
        if(unlikely(!irr->id || !irr->id[0])) continue;

        size_t idlen = strlen(irr->id);
        if(irr->id[idlen - 1] == ':')
            irr->id[--idlen] = '\0';

        int c;
        for(c = 0; c < cpus ;c++) {
            if(likely((c + 1) < (int)words))
                irr->cpu[c].value = str2ull(procfile_lineword(ff, l, (uint32_t)(c + 1)));
            else
                irr->cpu[c].value = 0;

            irr->total += irr->cpu[c].value;
        }

        if(unlikely(isdigit(irr->id[0]) && (uint32_t)(cpus + 2) < words)) {
            strncpyz(irr->name, procfile_lineword(ff, l, words - 1), MAX_INTERRUPT_NAME);
            size_t nlen = strlen(irr->name);
            if(likely(nlen + 1 + idlen <= MAX_INTERRUPT_NAME)) {
                irr->name[nlen] = '_';
                strncpyz(&irr->name[nlen + 1], irr->id, MAX_INTERRUPT_NAME - nlen - 1);
            }
            else {
                irr->name[MAX_INTERRUPT_NAME - idlen - 1] = '_';
                strncpyz(&irr->name[MAX_INTERRUPT_NAME - idlen], irr->id, idlen);
            }
        }
        else {
            strncpyz(irr->name, irr->id, MAX_INTERRUPT_NAME);
        }

        irr->used = 1;
    }

    // --------------------------------------------------------------------

    static RRDSET *st_system_interrupts = NULL;
    if(unlikely(!st_system_interrupts))
        st_system_interrupts = rrdset_create_localhost(
                "system"
                , "interrupts"
                , NULL
                , "interrupts"
                , NULL
                , "System interrupts"
                , "interrupts/s"
                , PLUGIN_PROC_NAME
                , PLUGIN_PROC_MODULE_INTERRUPTS_NAME
                , NETDATA_CHART_PRIO_SYSTEM_INTERRUPTS
                , update_every
                , RRDSET_TYPE_STACKED
        );
    else
        rrdset_next(st_system_interrupts);

    for(l = 0; l < lines ;l++) {
        struct interrupt *irr = irrindex(irrs, l, cpus);
        if(irr->used && irr->total) {
            // some interrupt may have changed without changing the total number of lines
            // if the same number of interrupts have been added and removed between two
            // calls of this function.
            if(unlikely(!irr->rd || strncmp(rrddim_name(irr->rd), irr->name, MAX_INTERRUPT_NAME) != 0)) {
                irr->rd = rrddim_add(st_system_interrupts, irr->id, irr->name, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_set_name(st_system_interrupts, irr->rd, irr->name);

                // also reset per cpu RRDDIMs to avoid repeating strncmp() in the per core loop
                if(likely(do_per_core != CONFIG_BOOLEAN_NO)) {
                    int c;
                    for(c = 0; c < cpus; c++) irr->cpu[c].rd = NULL;
                }
            }

            rrddim_set_by_pointer(st_system_interrupts, irr->rd, irr->total);
        }
    }

    rrdset_done(st_system_interrupts);

    // --------------------------------------------------------------------

    if(likely(do_per_core != CONFIG_BOOLEAN_NO)) {
        static RRDSET **core_st = NULL;
        static int old_cpus = 0;

        if(old_cpus < cpus) {
            core_st = reallocz(core_st, sizeof(RRDSET *) * cpus);
            memset(&core_st[old_cpus], 0, sizeof(RRDSET *) * (cpus - old_cpus));
            old_cpus = cpus;
        }

        int c;

        for(c = 0; c < cpus ;c++) {
            if(unlikely(!core_st[c])) {
                char id[50+1];
                snprintfz(id, 50, "cpu%d_interrupts", c);

                char title[100+1];
                snprintfz(title, 100, "CPU Interrupts");
                core_st[c] = rrdset_create_localhost(
                        "cpu"
                        , id
                        , NULL
                        , "interrupts"
                        , "cpu.interrupts"
                        , title
                        , "interrupts/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_INTERRUPTS_NAME
                        , NETDATA_CHART_PRIO_INTERRUPTS_PER_CORE + c
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                char core[50+1];
                snprintfz(core, 50, "cpu%d", c);
                rrdlabels_add(core_st[c]->state->chart_labels, "cpu", core, RRDLABEL_SRC_AUTO);
            }
            else rrdset_next(core_st[c]);

            for(l = 0; l < lines ;l++) {
                struct interrupt *irr = irrindex(irrs, l, cpus);
                if(irr->used && (do_per_core == CONFIG_BOOLEAN_YES || irr->cpu[c].value)) {
                    if(unlikely(!irr->cpu[c].rd)) {
                        irr->cpu[c].rd = rrddim_add(core_st[c], irr->id, irr->name, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rrddim_set_name(core_st[c], irr->cpu[c].rd, irr->name);
                    }

                    rrddim_set_by_pointer(core_st[c], irr->cpu[c].rd, irr->cpu[c].value);
                }
            }

            rrdset_done(core_st[c]);
        }
    }

    return 0;
}
