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

static inline void proc_interrupts_keep_colon_in_words(procfile *ff) {
    if(ff)
        ff->separators[(uint8_t)':'] = PF_CHAR_IS_WORD;
}

static inline char *proc_interrupts_lineword(procfile *ff, size_t line, size_t word) {
    if(!ff)
        return NULL;

    if(!ff->lines)
        return NULL;

    if(!ff->words)
        return NULL;

    if(line >= ff->lines->len)
        return NULL;

    ffline *pfline = &ff->lines->lines[line];
    if(word >= pfline->words)
        return NULL;

    size_t word_index = (size_t)pfline->first + word;
    if(word_index >= ff->words->len)
        return NULL;

    return ff->words->words[word_index];
}

static inline bool proc_interrupts_word_is_number(const char *word) {
    if(word == NULL)
        return false;

    const unsigned char *p = (const unsigned char *)word;
    if(*p == '\0')
        return false;

    while(*p) {
        if(!isdigit(*p))
            return false;

        p++;
    }

    return true;
}

static inline bool proc_interrupts_word_is_trigger(const char *word) {
    if(!word)
        return false;

    return (
        strcasecmp(word, "edge") == 0 ||
        strcasecmp(word, "level") == 0 ||
        strcasecmp(word, "fasteoi") == 0);
}

static inline bool proc_interrupts_word_has_trigger_suffix(const char *word) {
    const char *separator = word ? strrchr(word, '-') : NULL;
    return separator && proc_interrupts_word_is_trigger(separator + 1);
}

static size_t proc_interrupts_name_first_word(procfile *ff, size_t line, int cpus, size_t words) {
    size_t first = (size_t)cpus + 1;
    if(unlikely(first >= words))
        return words;

    // Skip the interrupt controller/chip column.
    first++;

    while(first < words) {
        const char *word = proc_interrupts_lineword(ff, line, first);
        if(proc_interrupts_word_is_number(word) ||
            proc_interrupts_word_is_trigger(word) ||
            proc_interrupts_word_has_trigger_suffix(word))
            first++;
        else
            break;
    }

    return first;
}

static inline char proc_interrupts_name_char(char c) {
    return (c == ':' || isspace((uint8_t)c)) ? '_' : c;
}

static size_t proc_interrupts_strnlen(const char *s, size_t max) {
    size_t len = 0;

    if(!s)
        return 0;

    while(len < max && s[len])
        len++;

    return len;
}

static inline void proc_interrupts_append_char(char *dst, size_t *len, char c) {
    if(*len < MAX_INTERRUPT_NAME) {
        dst[*len] = c;
        (*len)++;
        dst[*len] = '\0';
    }
}

static void proc_interrupts_append_id(char *name, const char *id, size_t idlen) {
    if(!id)
        return;

    if(!idlen)
        return;

    if(unlikely(idlen >= MAX_INTERRUPT_NAME)) {
        strncpyz(name, id, MAX_INTERRUPT_NAME);
        return;
    }

    size_t nlen = proc_interrupts_strnlen(name, MAX_INTERRUPT_NAME);
    if(likely(nlen + 1 + idlen <= MAX_INTERRUPT_NAME)) {
        name[nlen] = '_';
        strncpyz(&name[nlen + 1], id, MAX_INTERRUPT_NAME - nlen - 1);
    }
    else {
        name[MAX_INTERRUPT_NAME - idlen - 1] = '_';
        strncpyz(&name[MAX_INTERRUPT_NAME - idlen], id, idlen);
    }
}

static void proc_interrupts_build_name(
    char *dst,
    procfile *ff,
    size_t line,
    size_t first_name_word,
    size_t words,
    const char *id,
    size_t idlen) {
    dst[0] = '\0';

    size_t len = 0;
    for(size_t w = first_name_word; w < words ;w++) {
        const char *word = proc_interrupts_lineword(ff, line, w);
        if(!word)
            continue;

        if(!*word)
            continue;

        if(len && dst[len - 1] != '_')
            proc_interrupts_append_char(dst, &len, '_');

        while(*word) {
            char c = proc_interrupts_name_char(*word++);
            if(c != '_' || !len || dst[len - 1] != '_')
                proc_interrupts_append_char(dst, &len, c);
        }
    }

    if(likely(len))
        proc_interrupts_append_id(dst, id, idlen);
    else
        strncpyz(dst, id, MAX_INTERRUPT_NAME);
}

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
        do_per_core = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_INTERRUPTS, "interrupts per core", CONFIG_BOOLEAN_NO);

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/interrupts");
        ff = procfile_open(inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_INTERRUPTS, "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        proc_interrupts_keep_colon_in_words(ff);
    }
    if(unlikely(!ff))
        return 1;

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;
    size_t words = procfile_linewords(ff, 0);

    if(unlikely(!lines)) {
        collector_error("Cannot read /proc/interrupts, zero lines reported.");
        return 1;
    }

    // find how many CPUs are there
    if(unlikely(cpus == -1)) {
        uint32_t w;
        cpus = 0;
        for(w = 0; w < words ; w++) {
            const char *word = proc_interrupts_lineword(ff, 0, w);
            if(!word)
                continue;

            if(likely(strncmp(word, "CPU", 3) == 0))
                cpus++;
        }
    }

    if(unlikely(!cpus)) {
        collector_error("PLUGIN: PROC_INTERRUPTS: Cannot find the number of CPUs in /proc/interrupts");
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

        irr->id = proc_interrupts_lineword(ff, l, 0);
        if(unlikely(!irr->id)) continue;
        if(unlikely(!irr->id[0])) continue;

        size_t idlen = strlen(irr->id);
        if(irr->id[idlen - 1] == ':')
            irr->id[--idlen] = '\0';

        int c;
        for(c = 0; c < cpus ;c++) {
            char *word = NULL;
            if(likely((c + 1) < (int)words))
                word = proc_interrupts_lineword(ff, l, (uint32_t)(c + 1));

            irr->cpu[c].value = word ? str2ull(word, NULL) : 0;

            irr->total += irr->cpu[c].value;
        }

        if(unlikely(isdigit(irr->id[0]))) {
            size_t first_name_word = proc_interrupts_name_first_word(ff, l, cpus, words);
            proc_interrupts_build_name(irr->name, ff, l, first_name_word, words, irr->id, idlen);
        }
        else {
            strncpyz(irr->name, irr->id, MAX_INTERRUPT_NAME);
        }

        irr->used = 1;
    }

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

    for(l = 0; l < lines ;l++) {
        struct interrupt *irr = irrindex(irrs, l, cpus);
        if(irr->used && irr->total) {
            // some interrupt may have changed without changing the total number of lines
            // if the same number of interrupts have been added and removed between two
            // calls of this function.
            if(unlikely(!irr->rd || strncmp(rrddim_name(irr->rd), irr->name, MAX_INTERRUPT_NAME) != 0)) {
                irr->rd = rrddim_add(st_system_interrupts, irr->id, irr->name, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrddim_reset_name(st_system_interrupts, irr->rd, irr->name);

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
                snprintfz(id, sizeof(id) - 1, "cpu%d_interrupts", c);

                char title[100+1];
                snprintfz(title, sizeof(title) - 1, "CPU Interrupts");
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
                snprintfz(core, sizeof(core) - 1, "cpu%d", c);
                rrdlabels_add(core_st[c]->rrdlabels, "cpu", core, RRDLABEL_SRC_AUTO);
            }

            for(l = 0; l < lines ;l++) {
                struct interrupt *irr = irrindex(irrs, l, cpus);
                if(irr->used && (do_per_core == CONFIG_BOOLEAN_YES || irr->cpu[c].value)) {
                    if(unlikely(!irr->cpu[c].rd)) {
                        irr->cpu[c].rd = rrddim_add(core_st[c], irr->id, irr->name, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rrddim_reset_name(core_st[c], irr->cpu[c].rd, irr->name);
                    }

                    rrddim_set_by_pointer(core_st[c], irr->cpu[c].rd, irr->cpu[c].value);
                }
            }

            rrdset_done(core_st[c]);
        }
    }

    return 0;
}

int proc_interrupts_unittest(void) {
#if !defined(__NR_memfd_create) || !defined(MFD_CLOEXEC)
    fprintf(stderr, "%s skipped, memfd_create() is not available.\n", __FUNCTION__);
    return 0;
#else
    int fd = (int)syscall(__NR_memfd_create, "netdata-proc-interrupts", MFD_CLOEXEC);
    if(fd == -1) {
        fprintf(stderr, "Cannot create in-memory /proc/interrupts fixture: %s\n", strerror(errno));
        return 1;
    }

    static const char fixture[] =
        "           CPU0       CPU1\n"
        "240:          1          2   PCI-MSI 1572864-edge      mlx5_comp40@pci:0000:86:00.0\n"
        "250:          3          4   PCI-MSI 5242880-edge      nvme 0 io5\n"
        "  0:          5          6   IO-APIC   2-edge          timer\n"
        "  1:          7          8   XT-PIC-XT                 keyboard\n"
        " 27:          9         10   GICv3     27 Level        arch_timer\n";

    if(write(fd, fixture, sizeof(fixture) - 1) != (ssize_t)sizeof(fixture) - 1) {
        fprintf(stderr, "Cannot write in-memory /proc/interrupts fixture: %s\n", strerror(errno));
        close(fd);
        return 1;
    }

    if(lseek(fd, 0, SEEK_SET) == -1) {
        fprintf(stderr, "Cannot rewind in-memory /proc/interrupts fixture: %s\n", strerror(errno));
        close(fd);
        return 1;
    }

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "/proc/self/fd/%d", fd);

    procfile *ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
    proc_interrupts_keep_colon_in_words(ff);
    close(fd);

    if(!ff)
        return 1;

    ff = procfile_readall(ff);
    if(!ff)
        return 1;

    size_t words = procfile_linewords(ff, 0);
    int cpus = 0;
    for(uint32_t w = 0; w < words ;w++) {
        const char *word = proc_interrupts_lineword(ff, 0, w);
        if(!word)
            continue;

        if(strncmp(word, "CPU", 3) == 0)
            cpus++;
    }

    static const char *expected[] = {
        "mlx5_comp40@pci_0000_86_00.0_240",
        "nvme_0_io5_250",
        "timer_0",
        "keyboard_1",
        "arch_timer_27",
    };

    int rc = 0;
    size_t expected_idx = 0;
    for(size_t line = 1; line < procfile_lines(ff) ;line++) {
        words = procfile_linewords(ff, line);
        if(!words)
            continue;

        if(expected_idx >= _countof(expected)) {
            fprintf(stderr, "proc_interrupts_unittest found unexpected parsed line %zu\n", line);
            rc = 1;
            break;
        }

        char id[MAX_INTERRUPT_NAME + 1];
        const char *id_word = proc_interrupts_lineword(ff, line, 0);
        if(!id_word) {
            rc = 1;
            break;
        }

        strncpyz(id, id_word, MAX_INTERRUPT_NAME);
        size_t idlen = proc_interrupts_strnlen(id, MAX_INTERRUPT_NAME);
        if(idlen && id[idlen - 1] == ':')
            id[--idlen] = '\0';

        size_t first_name_word = proc_interrupts_name_first_word(ff, line, cpus, words);

        char name[MAX_INTERRUPT_NAME + 1];
        proc_interrupts_build_name(name, ff, line, first_name_word, words, id, idlen);

        if(strcmp(name, expected[expected_idx]) != 0) {
            fprintf(
                stderr,
                "proc_interrupts_unittest line %zu expected '%s', got '%s'\n",
                line,
                expected[expected_idx],
                name);
            rc = 1;
            break;
        }

        expected_idx++;
    }

    if(!rc && expected_idx != _countof(expected)) {
        fprintf(
            stderr,
            "proc_interrupts_unittest expected %zu parsed lines, got %zu\n",
            _countof(expected),
            expected_idx);
        rc = 1;
    }

    procfile_close(ff);

    return rc;
#endif
}
