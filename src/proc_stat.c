#include "common.h"

struct per_core_single_number_file {
    char found:1;
    const char *filename;
    int fd;
    collected_number value;
    RRDDIM *rd;
};

#define CORE_THROTTLE_COUNT_INDEX    0
#define PACKAGE_THROTTLE_COUNT_INDEX 1
#define SCALING_CUR_FREQ_INDEX       2
#define PER_CORE_FILES               3

struct cpu_chart {
    const char *id;

    RRDSET *st;
    RRDDIM *rd_user;
    RRDDIM *rd_nice;
    RRDDIM *rd_system;
    RRDDIM *rd_idle;
    RRDDIM *rd_iowait;
    RRDDIM *rd_irq;
    RRDDIM *rd_softirq;
    RRDDIM *rd_steal;
    RRDDIM *rd_guest;
    RRDDIM *rd_guest_nice;

    struct per_core_single_number_file files[PER_CORE_FILES];
};

static int keep_per_core_fds_open = CONFIG_BOOLEAN_YES;

static int read_per_core_files(struct cpu_chart *all_cpu_charts, size_t len, size_t index) {
    char buf[50 + 1];
    size_t x, files_read = 0, files_nonzero = 0;

    for(x = 0; x < len ; x++) {
        struct per_core_single_number_file *f = &all_cpu_charts[x].files[index];

        f->found = 0;

        if(unlikely(!f->filename))
            continue;

        if(unlikely(f->fd == -1)) {
            f->fd = open(f->filename, O_RDONLY);
            if (unlikely(f->fd == -1)) {
                error("Cannot open file '%s'", f->filename);
                continue;
            }
        }

        ssize_t ret = read(f->fd, buf, 50);
        if(unlikely(ret == -1)) {
            // cannot read that file

            error("Cannot read file '%s'", f->filename);
            close(f->fd);
            f->fd = -1;
            continue;
        }
        else {
            // successful read

            // terminate the buffer
            buf[ret] = '\0';

            if(unlikely(keep_per_core_fds_open != CONFIG_BOOLEAN_YES)) {
                close(f->fd);
                f->fd = -1;
            }
            else if(lseek(f->fd, 0, SEEK_SET) == -1) {
                error("Cannot seek in file '%s'", f->filename);
                close(f->fd);
                f->fd = -1;
            }
        }

        files_read++;
        f->found = 1;

        f->value = str2ll(buf, NULL);
        // info("read '%s', parsed as " COLLECTED_NUMBER_FORMAT, buf, f->value);
        if(likely(f->value != 0))
            files_nonzero++;
    }

    if(files_read == 0)
        return -1;

    if(files_nonzero == 0)
        return 0;

    return (int)files_nonzero;
}

static void chart_per_core_files(struct cpu_chart *all_cpu_charts, size_t len, size_t index, RRDSET *st, collected_number multiplier, collected_number divisor, RRD_ALGORITHM algorithm) {
    size_t x;
    for(x = 0; x < len ; x++) {
        struct per_core_single_number_file *f = &all_cpu_charts[x].files[index];

        if(unlikely(!f->found))
            continue;

        if(unlikely(!f->rd))
            f->rd = rrddim_add(st, all_cpu_charts[x].id, NULL, multiplier, divisor, algorithm);

        rrddim_set_by_pointer(st, f->rd, f->value);
    }
}

int do_proc_stat(int update_every, usec_t dt) {
    (void)dt;

    static struct cpu_chart *all_cpu_charts = NULL;
    static size_t all_cpu_charts_size = 0;
    static procfile *ff = NULL;
    static int do_cpu = -1, do_cpu_cores = -1, do_interrupts = -1, do_context = -1, do_forks = -1, do_processes = -1, do_core_throttle_count = -1, do_package_throttle_count = -1, do_scaling_cur_freq = -1;
    static uint32_t hash_intr, hash_ctxt, hash_processes, hash_procs_running, hash_procs_blocked;
    static char *core_throttle_count_filename = NULL, *package_throttle_count_filename = NULL, *scaling_cur_freq_filename = NULL;

    if(unlikely(do_cpu == -1)) {
        do_cpu                    = config_get_boolean("plugin:proc:/proc/stat", "cpu utilization", CONFIG_BOOLEAN_YES);
        do_cpu_cores              = config_get_boolean("plugin:proc:/proc/stat", "per cpu core utilization", CONFIG_BOOLEAN_YES);
        do_interrupts             = config_get_boolean("plugin:proc:/proc/stat", "cpu interrupts", CONFIG_BOOLEAN_YES);
        do_context                = config_get_boolean("plugin:proc:/proc/stat", "context switches", CONFIG_BOOLEAN_YES);
        do_forks                  = config_get_boolean("plugin:proc:/proc/stat", "processes started", CONFIG_BOOLEAN_YES);
        do_processes              = config_get_boolean("plugin:proc:/proc/stat", "processes running", CONFIG_BOOLEAN_YES);

        // give sane defaults based on the number of processors
        if(processors > 50) {
            // the system has too many processors
            keep_per_core_fds_open = CONFIG_BOOLEAN_NO;
            do_core_throttle_count = CONFIG_BOOLEAN_NO;
            do_package_throttle_count = CONFIG_BOOLEAN_NO;
            do_scaling_cur_freq = CONFIG_BOOLEAN_NO;
        }
        else {
            // the system has a reasonable number of processors
            keep_per_core_fds_open = CONFIG_BOOLEAN_YES;
            do_core_throttle_count = CONFIG_BOOLEAN_AUTO;
            do_package_throttle_count = CONFIG_BOOLEAN_NO;
            do_scaling_cur_freq = CONFIG_BOOLEAN_NO;
        }

        keep_per_core_fds_open    = config_get_boolean("plugin:proc:/proc/stat", "keep per core files open", keep_per_core_fds_open);
        do_core_throttle_count    = config_get_boolean_ondemand("plugin:proc:/proc/stat", "core_throttle_count", do_core_throttle_count);
        do_package_throttle_count = config_get_boolean_ondemand("plugin:proc:/proc/stat", "package_throttle_count", do_package_throttle_count);
        do_scaling_cur_freq       = config_get_boolean_ondemand("plugin:proc:/proc/stat", "scaling_cur_freq", do_scaling_cur_freq);

        hash_intr = simple_hash("intr");
        hash_ctxt = simple_hash("ctxt");
        hash_processes = simple_hash("processes");
        hash_procs_running = simple_hash("procs_running");
        hash_procs_blocked = simple_hash("procs_blocked");

        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/cpu/%s/thermal_throttle/core_throttle_count");
        core_throttle_count_filename = config_get("plugin:proc:/proc/stat", "core_throttle_count filename to monitor", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/cpu/%s/thermal_throttle/package_throttle_count");
        package_throttle_count_filename = config_get("plugin:proc:/proc/stat", "package_throttle_count filename to monitor", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/cpu/%s/cpufreq/scaling_cur_freq");
        scaling_cur_freq_filename = config_get("plugin:proc:/proc/stat", "scaling_cur_freq filename to monitor", filename);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/stat");
        ff = procfile_open(config_get("plugin:proc:/proc/stat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;
    size_t words;

    unsigned long long processes = 0, running = 0 , blocked = 0;

    for(l = 0; l < lines ;l++) {
        char *row_key = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(row_key);

        // faster strncmp(row_key, "cpu", 3) == 0
        if(likely(row_key[0] == 'c' && row_key[1] == 'p' && row_key[2] == 'u')) {
            words = procfile_linewords(ff, l);
            if(unlikely(words < 9)) {
                error("Cannot read /proc/stat cpu line. Expected 9 params, read %zu.", words);
                continue;
            }

            size_t core    = (row_key[3] == '\0') ? 0 : str2ul(&row_key[3]) + 1;

            if(likely((core == 0 && do_cpu) || (core > 0 && do_cpu_cores))) {
                char *id;
                unsigned long long user = 0, nice = 0, system = 0, idle = 0, iowait = 0, irq = 0, softirq = 0, steal = 0, guest = 0, guest_nice = 0;

                id          = row_key;
                user        = str2ull(procfile_lineword(ff, l, 1));
                nice        = str2ull(procfile_lineword(ff, l, 2));
                system      = str2ull(procfile_lineword(ff, l, 3));
                idle        = str2ull(procfile_lineword(ff, l, 4));
                iowait      = str2ull(procfile_lineword(ff, l, 5));
                irq         = str2ull(procfile_lineword(ff, l, 6));
                softirq     = str2ull(procfile_lineword(ff, l, 7));
                steal       = str2ull(procfile_lineword(ff, l, 8));

                guest       = str2ull(procfile_lineword(ff, l, 9));
                user -= guest;

                guest_nice  = str2ull(procfile_lineword(ff, l, 10));
                nice -= guest_nice;

                char *title, *type, *context, *family;
                long priority;

                if(core >= all_cpu_charts_size) {
                    size_t old_cpu_charts_size = all_cpu_charts_size;
                    all_cpu_charts_size = core + 1;
                    all_cpu_charts = reallocz(all_cpu_charts, sizeof(struct cpu_chart) * all_cpu_charts_size);
                    memset(&all_cpu_charts[old_cpu_charts_size], 0, sizeof(struct cpu_chart) * (all_cpu_charts_size - old_cpu_charts_size));
                }
                struct cpu_chart *cpu_chart = &all_cpu_charts[core];

                if(unlikely(!cpu_chart->st)) {
                    cpu_chart->id = strdupz(id);

                    if(core == 0) {
                        title = "Total CPU utilization";
                        type = "system";
                        context = "system.cpu";
                        family = id;
                        priority = 100;
                    }
                    else {
                        title = "Core utilization";
                        type = "cpu";
                        context = "cpu.cpu";
                        family = "utilization";
                        priority = 1000;

                        // FIXME: check for /sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq
                        // FIXME: check for /sys/devices/system/cpu/cpu*/cpufreq/stats/time_in_state

                        char filename[FILENAME_MAX + 1];
                        struct stat stbuf;

                        if(do_core_throttle_count != CONFIG_BOOLEAN_NO) {
                            snprintfz(filename, FILENAME_MAX, core_throttle_count_filename, id);
                            if (stat(filename, &stbuf) == 0) {
                                cpu_chart->files[CORE_THROTTLE_COUNT_INDEX].filename = strdupz(filename);
                                cpu_chart->files[CORE_THROTTLE_COUNT_INDEX].fd = -1;
                                do_core_throttle_count = CONFIG_BOOLEAN_YES;
                            }
                        }

                        if(do_package_throttle_count != CONFIG_BOOLEAN_NO) {
                            snprintfz(filename, FILENAME_MAX, package_throttle_count_filename, id);
                            if (stat(filename, &stbuf) == 0) {
                                cpu_chart->files[PACKAGE_THROTTLE_COUNT_INDEX].filename = strdupz(filename);
                                cpu_chart->files[PACKAGE_THROTTLE_COUNT_INDEX].fd = -1;
                                do_package_throttle_count = CONFIG_BOOLEAN_YES;
                            }
                        }

                        if(do_scaling_cur_freq != CONFIG_BOOLEAN_NO) {
                            snprintfz(filename, FILENAME_MAX, scaling_cur_freq_filename, id);
                            if (stat(filename, &stbuf) == 0) {
                                cpu_chart->files[SCALING_CUR_FREQ_INDEX].filename = strdupz(filename);
                                cpu_chart->files[SCALING_CUR_FREQ_INDEX].fd = -1;
                                do_scaling_cur_freq = CONFIG_BOOLEAN_YES;
                            }
                        }
                    }

                    cpu_chart->st = rrdset_create_localhost(
                            type
                            , id
                            , NULL
                            , family
                            , context
                            , title
                            , "percentage"
                            , priority
                            , update_every
                            , RRDSET_TYPE_STACKED
                    );

                    long multiplier = 1;
                    long divisor = 1; // sysconf(_SC_CLK_TCK);

                    cpu_chart->rd_guest_nice = rrddim_add(cpu_chart->st, "guest_nice", NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    cpu_chart->rd_guest      = rrddim_add(cpu_chart->st, "guest",      NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    cpu_chart->rd_steal      = rrddim_add(cpu_chart->st, "steal",      NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    cpu_chart->rd_softirq    = rrddim_add(cpu_chart->st, "softirq",    NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    cpu_chart->rd_irq        = rrddim_add(cpu_chart->st, "irq",        NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    cpu_chart->rd_user       = rrddim_add(cpu_chart->st, "user",       NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    cpu_chart->rd_system     = rrddim_add(cpu_chart->st, "system",     NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    cpu_chart->rd_nice       = rrddim_add(cpu_chart->st, "nice",       NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    cpu_chart->rd_iowait     = rrddim_add(cpu_chart->st, "iowait",     NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    cpu_chart->rd_idle       = rrddim_add(cpu_chart->st, "idle",       NULL, multiplier, divisor, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                    rrddim_hide(cpu_chart->st, "idle");
                }
                else rrdset_next(cpu_chart->st);

                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_user, user);
                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_nice, nice);
                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_system, system);
                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_idle, idle);
                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_iowait, iowait);
                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_irq, irq);
                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_softirq, softirq);
                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_steal, steal);
                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_guest, guest);
                rrddim_set_by_pointer(cpu_chart->st, cpu_chart->rd_guest_nice, guest_nice);
                rrdset_done(cpu_chart->st);
            }
        }
        else if(unlikely(hash == hash_intr && strcmp(row_key, "intr") == 0)) {
            if(likely(do_interrupts)) {
                static RRDSET *st_intr = NULL;
                static RRDDIM *rd_interrupts = NULL;
                unsigned long long value = str2ull(procfile_lineword(ff, l, 1));

                if(unlikely(!st_intr)) {
                    st_intr = rrdset_create_localhost(
                            "system"
                            , "intr"
                            , NULL
                            , "interrupts"
                            , NULL
                            , "CPU Interrupts"
                            , "interrupts/s"
                            , 900
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdset_flag_set(st_intr, RRDSET_FLAG_DETAIL);

                    rd_interrupts = rrddim_add(st_intr, "interrupts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st_intr);

                rrddim_set_by_pointer(st_intr, rd_interrupts, value);
                rrdset_done(st_intr);
            }
        }
        else if(unlikely(hash == hash_ctxt && strcmp(row_key, "ctxt") == 0)) {
            if(likely(do_context)) {
                static RRDSET *st_ctxt = NULL;
                static RRDDIM *rd_switches = NULL;
                unsigned long long value = str2ull(procfile_lineword(ff, l, 1));

                if(unlikely(!st_ctxt)) {
                    st_ctxt = rrdset_create_localhost(
                            "system"
                            , "ctxt"
                            , NULL
                            , "processes"
                            , NULL
                            , "CPU Context Switches"
                            , "context switches/s"
                            , 800
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_switches = rrddim_add(st_ctxt, "switches", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st_ctxt);

                rrddim_set_by_pointer(st_ctxt, rd_switches, value);
                rrdset_done(st_ctxt);
            }
        }
        else if(unlikely(hash == hash_processes && !processes && strcmp(row_key, "processes") == 0)) {
            processes = str2ull(procfile_lineword(ff, l, 1));
        }
        else if(unlikely(hash == hash_procs_running && !running && strcmp(row_key, "procs_running") == 0)) {
            running = str2ull(procfile_lineword(ff, l, 1));
        }
        else if(unlikely(hash == hash_procs_blocked && !blocked && strcmp(row_key, "procs_blocked") == 0)) {
            blocked = str2ull(procfile_lineword(ff, l, 1));
        }
    }

    // --------------------------------------------------------------------

    if(likely(do_forks)) {
        static RRDSET *st_forks = NULL;
        static RRDDIM *rd_started = NULL;

        if(unlikely(!st_forks)) {
            st_forks = rrdset_create_localhost(
                    "system"
                    , "forks"
                    , NULL
                    , "processes"
                    , NULL
                    , "Started Processes"
                    , "processes/s"
                    , 700
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st_forks, RRDSET_FLAG_DETAIL);

            rd_started = rrddim_add(st_forks, "started", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st_forks);

        rrddim_set_by_pointer(st_forks, rd_started, processes);
        rrdset_done(st_forks);
    }

    // --------------------------------------------------------------------

    if(likely(do_processes)) {
        static RRDSET *st_processes = NULL;
        static RRDDIM *rd_running = NULL;
        static RRDDIM *rd_blocked = NULL;

        if(unlikely(!st_processes)) {
            st_processes = rrdset_create_localhost(
                    "system"
                    , "processes"
                    , NULL
                    , "processes"
                    , NULL
                    , "System Processes"
                    , "processes"
                    , 600
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_running = rrddim_add(st_processes, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_blocked = rrddim_add(st_processes, "blocked", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st_processes);

        rrddim_set_by_pointer(st_processes, rd_running, running);
        rrddim_set_by_pointer(st_processes, rd_blocked, blocked);
        rrdset_done(st_processes);
    }

    if(likely(all_cpu_charts_size > 1)) {
        if(likely(do_core_throttle_count != CONFIG_BOOLEAN_NO)) {
            int r = read_per_core_files(&all_cpu_charts[1], all_cpu_charts_size - 1, CORE_THROTTLE_COUNT_INDEX);
            if(likely(r != -1 && (do_core_throttle_count == CONFIG_BOOLEAN_YES || r > 0))) {
                do_core_throttle_count = CONFIG_BOOLEAN_YES;

                static RRDSET *st_core_throttle_count = NULL;

                if (unlikely(!st_core_throttle_count))
                    st_core_throttle_count = rrdset_create_localhost(
                            "cpu"
                            , "core_throttling"
                            , NULL
                            , "throttling"
                            , "cpu.core_throttling"
                            , "Core Thermal Throttling Events"
                            , "events/s"
                            , 5001
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                else
                    rrdset_next(st_core_throttle_count);

                chart_per_core_files(&all_cpu_charts[1], all_cpu_charts_size - 1, CORE_THROTTLE_COUNT_INDEX, st_core_throttle_count, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrdset_done(st_core_throttle_count);
            }
        }

        if(likely(do_package_throttle_count != CONFIG_BOOLEAN_NO)) {
            int r = read_per_core_files(&all_cpu_charts[1], all_cpu_charts_size - 1, PACKAGE_THROTTLE_COUNT_INDEX);
            if(likely(r != -1 && (do_package_throttle_count == CONFIG_BOOLEAN_YES || r > 0))) {
                do_package_throttle_count = CONFIG_BOOLEAN_YES;

                static RRDSET *st_package_throttle_count = NULL;

                if(unlikely(!st_package_throttle_count))
                    st_package_throttle_count = rrdset_create_localhost(
                            "cpu"
                            , "package_throttling"
                            , NULL
                            , "throttling"
                            , "cpu.package_throttling"
                            , "Package Thermal Throttling Events"
                            , "events/s"
                            , 5002
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                else
                    rrdset_next(st_package_throttle_count);

                chart_per_core_files(&all_cpu_charts[1], all_cpu_charts_size - 1, PACKAGE_THROTTLE_COUNT_INDEX, st_package_throttle_count, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrdset_done(st_package_throttle_count);
            }
        }

        if(likely(do_scaling_cur_freq != CONFIG_BOOLEAN_NO)) {
            int r = read_per_core_files(&all_cpu_charts[1], all_cpu_charts_size - 1, SCALING_CUR_FREQ_INDEX);
            if(likely(r != -1 && (do_scaling_cur_freq == CONFIG_BOOLEAN_YES || r > 0))) {
                do_scaling_cur_freq = CONFIG_BOOLEAN_YES;

                static RRDSET *st_scaling_cur_freq = NULL;

                if(unlikely(!st_scaling_cur_freq))
                    st_scaling_cur_freq = rrdset_create_localhost(
                            "cpu"
                            , "scaling_cur_freq"
                            , NULL
                            , "cpufreq"
                            , "cpu.scaling_cur_freq"
                            , "Per CPU Core, Current CPU Scaling Frequency"
                            , "MHz"
                            , 5003
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                else
                    rrdset_next(st_scaling_cur_freq);

                chart_per_core_files(&all_cpu_charts[1], all_cpu_charts_size - 1, SCALING_CUR_FREQ_INDEX, st_scaling_cur_freq, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                rrdset_done(st_scaling_cur_freq);
            }
        }
    }

    return 0;
}
