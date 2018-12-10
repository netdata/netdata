// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_STAT_NAME "/proc/stat"

struct per_core_single_number_file {
    unsigned char found:1;
    const char *filename;
    int fd;
    collected_number value;
    RRDDIM *rd;
};

struct last_ticks {
    collected_number frequency;
    collected_number ticks;
};

// This is an extension of struct per_core_single_number_file at CPU_FREQ_INDEX.
// Either scaling_cur_freq or time_in_state file is used at one time.
struct per_core_time_in_state_file {
    const char *filename;
    procfile *ff;
    size_t last_ticks_len;
    struct last_ticks *last_ticks;
};

#define CORE_THROTTLE_COUNT_INDEX    0
#define PACKAGE_THROTTLE_COUNT_INDEX 1
#define CPU_FREQ_INDEX               2
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

    struct per_core_time_in_state_file time_in_state_files;
};

static int keep_per_core_fds_open = CONFIG_BOOLEAN_YES;
static int keep_cpuidle_fds_open = CONFIG_BOOLEAN_YES;

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
        if(unlikely(ret < 0)) {
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
        if(likely(f->value != 0))
            files_nonzero++;
    }

    if(files_read == 0)
        return -1;

    if(files_nonzero == 0)
        return 0;

    return (int)files_nonzero;
}

static int read_per_core_time_in_state_files(struct cpu_chart *all_cpu_charts, size_t len, size_t index) {
    size_t x, files_read = 0, files_nonzero = 0;

    for(x = 0; x < len ; x++) {
        struct per_core_single_number_file *f = &all_cpu_charts[x].files[index];
        struct per_core_time_in_state_file *tsf = &all_cpu_charts[x].time_in_state_files;

        f->found = 0;

        if(unlikely(!tsf->filename))
            continue;

        if(unlikely(!tsf->ff)) {
            tsf->ff = procfile_open(tsf->filename, " \t:", PROCFILE_FLAG_DEFAULT);
            if(unlikely(!tsf->ff))
            {
                error("Cannot open file '%s'", tsf->filename);
                continue;
            }
        }

        tsf->ff = procfile_readall(tsf->ff);
        if(unlikely(!tsf->ff)) {
            error("Cannot read file '%s'", tsf->filename);
            procfile_close(tsf->ff);
            tsf->ff = NULL;
            continue;
        }
        else {
            // successful read

            size_t lines = procfile_lines(tsf->ff), l;
            size_t words;
            unsigned long long total_ticks_since_last = 0, avg_freq = 0;

            // Check if there is at least one frequency in time_in_state
            if (procfile_word(tsf->ff, 0)[0] == '\0') {
                if(unlikely(keep_per_core_fds_open != CONFIG_BOOLEAN_YES)) {
                    procfile_close(tsf->ff);
                    tsf->ff = NULL;
                }
                // TODO: Is there a better way to avoid spikes than calculating the average over
                // the whole period under schedutil governor?
                // freez(tsf->last_ticks);
                // tsf->last_ticks = NULL;
                // tsf->last_ticks_len = 0;
                continue;
            }

            if (unlikely(tsf->last_ticks_len < lines || tsf->last_ticks == NULL)) {
                tsf->last_ticks = reallocz(tsf->last_ticks, sizeof(struct last_ticks) * lines);
                memset(tsf->last_ticks, 0, sizeof(struct last_ticks) * lines);
                tsf->last_ticks_len = lines;
            }

            f->value = 0;

            for(l = 0; l < lines - 1 ;l++) {
                unsigned long long frequency = 0, ticks = 0, ticks_since_last = 0;

                words = procfile_linewords(tsf->ff, l);
                if(unlikely(words < 2)) {
                    error("Cannot read time_in_state line. Expected 2 params, read %zu.", words);
                    continue;
                }
                frequency = str2ull(procfile_lineword(tsf->ff, l, 0));
                ticks     = str2ull(procfile_lineword(tsf->ff, l, 1));

                // It is assumed that frequencies are static and sorted
                ticks_since_last = ticks - tsf->last_ticks[l].ticks;
                tsf->last_ticks[l].frequency = frequency;
                tsf->last_ticks[l].ticks = ticks;

                total_ticks_since_last += ticks_since_last;
                avg_freq += frequency * ticks_since_last;

            }

            if (likely(total_ticks_since_last)) {
                avg_freq /= total_ticks_since_last;
                f->value = avg_freq;
            }

            if(unlikely(keep_per_core_fds_open != CONFIG_BOOLEAN_YES)) {
                procfile_close(tsf->ff);
                tsf->ff = NULL;
            }
        }

        files_read++;

        f->found = 1;

        if(likely(f->value != 0))
            files_nonzero++;
    }

    if(unlikely(files_read == 0))
        return -1;

    if(unlikely(files_nonzero == 0))
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

struct cpuidle_state {
    char *name;

    char *time_filename;
    int time_fd;

    collected_number value;

    RRDDIM *rd;
};

struct per_core_cpuidle_chart {
    RRDSET *st;

    RRDDIM *active_time_rd;
    collected_number active_time;
    collected_number last_active_time;

    struct cpuidle_state *cpuidle_state;
    size_t cpuidle_state_len;
    int rescan_cpu_states;
};

static void* wake_cpu_thread(void* core) {
    pthread_t thread;
    cpu_set_t cpu_set;
    static size_t cpu_wakeups = 0;

    CPU_ZERO(&cpu_set);
    CPU_SET(*(int*)core, &cpu_set);

    thread = pthread_self();
    if(unlikely(pthread_setaffinity_np(thread, sizeof(cpu_set_t), &cpu_set)))
        error("Cannot set CPU affinity");

    // Make the CPU core do something
    cpu_wakeups++;

    return 0;
}

static int read_schedstat(char* schedstat_filename, struct per_core_cpuidle_chart **cpuidle_charts_address, size_t cores_found) {
    static size_t cpuidle_charts_len = 0;
    static procfile *ff = NULL;
    struct per_core_cpuidle_chart *cpuidle_charts = *cpuidle_charts_address;

    if(unlikely(!ff)) {
        ff = procfile_open(schedstat_filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 1;

    size_t lines = procfile_lines(ff), l;
    size_t words;

    if(unlikely(cpuidle_charts_len < cores_found)) {
        cpuidle_charts = reallocz(cpuidle_charts, sizeof(struct per_core_cpuidle_chart) * cores_found);
        *cpuidle_charts_address = cpuidle_charts;
        memset(cpuidle_charts + cpuidle_charts_len, 0, sizeof(struct per_core_cpuidle_chart) * (cores_found - cpuidle_charts_len));
        cpuidle_charts_len = cores_found;
    }

    for(l = 0; l < lines ;l++) {
        char *row_key = procfile_lineword(ff, l, 0);

        // faster strncmp(row_key, "cpu", 3) == 0
        if(likely(row_key[0] == 'c' && row_key[1] == 'p' && row_key[2] == 'u')) {
            words = procfile_linewords(ff, l);
            if(unlikely(words < 10)) {
                error("Cannot read /proc/schedstat cpu line. Expected 9 params, read %zu.", words);
                return 1;
            }

            size_t core = str2ul(&row_key[3]);
            if(unlikely(core >= cores_found)) {
                // Temporary workaround for issue 3945
                // error("Core %zu found but no more than %zu cores were expected.", core, cores_found);
                return 1;
            }
            cpuidle_charts[core].active_time = str2ull(procfile_lineword(ff, l, 7)) / 1000;
        }
    }

    return 0;
}

static int read_one_state(char *buf, const char *filename, int *fd) {
    ssize_t ret = read(*fd, buf, 50);

    if(unlikely(ret <= 0)) {
        // cannot read that file
        error("Cannot read file '%s'", filename);
        close(*fd);
        *fd = -1;
        return 0;
    }
    else {
        // successful read

        // terminate the buffer
        buf[ret - 1] = '\0';

        if(unlikely(keep_cpuidle_fds_open != CONFIG_BOOLEAN_YES)) {
            close(*fd);
            *fd = -1;
        }
        else if(lseek(*fd, 0, SEEK_SET) == -1) {
            error("Cannot seek in file '%s'", filename);
            close(*fd);
            *fd = -1;
        }
    }

    return 1;
}

static int read_cpuidle_states(char *cpuidle_name_filename , char *cpuidle_time_filename, struct per_core_cpuidle_chart *cpuidle_charts, size_t core) {
    char filename[FILENAME_MAX + 1];
    static char next_state_filename[FILENAME_MAX + 1];
    struct stat stbuf;
    struct per_core_cpuidle_chart *cc = &cpuidle_charts[core];
    size_t state;

    if(unlikely(!cc->cpuidle_state_len || cc->rescan_cpu_states)) {
        int state_file_found = 1; // check at least one state

        if(cc->cpuidle_state_len) {
            for(state = 0; state < cc->cpuidle_state_len; state++) {
                freez(cc->cpuidle_state[state].name);

                freez(cc->cpuidle_state[state].time_filename);
                close(cc->cpuidle_state[state].time_fd);
                cc->cpuidle_state[state].time_fd = -1;
            }

            freez(cc->cpuidle_state);
            cc->cpuidle_state = NULL;
            cc->cpuidle_state_len = 0;

            cc->active_time_rd = NULL;
            cc->st = NULL;
        }

        while(likely(state_file_found)) {
            snprintfz(filename, FILENAME_MAX, cpuidle_name_filename, core, cc->cpuidle_state_len);
            if (stat(filename, &stbuf) == 0)
                cc->cpuidle_state_len++;
            else
                state_file_found = 0;
        }
        snprintfz(next_state_filename, FILENAME_MAX, cpuidle_name_filename, core, cc->cpuidle_state_len);

        cc->cpuidle_state = callocz(cc->cpuidle_state_len, sizeof(struct cpuidle_state));
        memset(cc->cpuidle_state, 0, sizeof(struct cpuidle_state) * cc->cpuidle_state_len);

        for(state = 0; state < cc->cpuidle_state_len; state++) {
            char name_buf[50 + 1];
            snprintfz(filename, FILENAME_MAX, cpuidle_name_filename, core, state);

            int fd = open(filename, O_RDONLY, 0666);
            if(unlikely(fd == -1)) {
                error("Cannot open file '%s'", filename);
                cc->rescan_cpu_states = 1;
                return 1;
            }

            ssize_t r = read(fd, name_buf, 50);
            if(unlikely(r < 1)) {
                error("Cannot read file '%s'", filename);
                close(fd);
                cc->rescan_cpu_states = 1;
                return 1;
            }

            name_buf[r - 1] = '\0'; // erase extra character
            cc->cpuidle_state[state].name = strdupz(name_buf);
            close(fd);

            snprintfz(filename, FILENAME_MAX, cpuidle_time_filename, core, state);
            cc->cpuidle_state[state].time_filename = strdupz(filename);
            cc->cpuidle_state[state].time_fd = -1;
        }

        cc->rescan_cpu_states = 0;
    }

    for(state = 0; state < cc->cpuidle_state_len; state++) {

        struct cpuidle_state *cs = &cc->cpuidle_state[state];

        if(unlikely(cs->time_fd == -1)) {
            cs->time_fd = open(cs->time_filename, O_RDONLY);
            if (unlikely(cs->time_fd == -1)) {
                error("Cannot open file '%s'", cs->time_filename);
                cc->rescan_cpu_states = 1;
                return 1;
            }
        }

        char time_buf[50 + 1];
        if(likely(read_one_state(time_buf, cs->time_filename, &cs->time_fd))) {
            cs->value = str2ll(time_buf, NULL);
        }
        else {
            cc->rescan_cpu_states = 1;
            return 1;
        }
    }

    // check if the number of states was increased
    if(unlikely(stat(next_state_filename, &stbuf) == 0)) {
        cc->rescan_cpu_states = 1;
        return 1;
    }

    return 0;
}

int do_proc_stat(int update_every, usec_t dt) {
    (void)dt;

    static struct cpu_chart *all_cpu_charts = NULL;
    static size_t all_cpu_charts_size = 0;
    static procfile *ff = NULL;
    static int do_cpu = -1, do_cpu_cores = -1, do_interrupts = -1, do_context = -1, do_forks = -1, do_processes = -1,
           do_core_throttle_count = -1, do_package_throttle_count = -1, do_cpu_freq = -1, do_cpuidle = -1;
    static uint32_t hash_intr, hash_ctxt, hash_processes, hash_procs_running, hash_procs_blocked;
    static char *core_throttle_count_filename = NULL, *package_throttle_count_filename = NULL, *scaling_cur_freq_filename = NULL,
           *time_in_state_filename = NULL, *schedstat_filename = NULL, *cpuidle_name_filename = NULL, *cpuidle_time_filename = NULL;
    static RRDVAR *cpus_var = NULL;
    static int accurate_freq_avail = 0, accurate_freq_is_used = 0;
    size_t cores_found = (size_t)processors;

    if(unlikely(do_cpu == -1)) {
        do_cpu                    = config_get_boolean("plugin:proc:/proc/stat", "cpu utilization", CONFIG_BOOLEAN_YES);
        do_cpu_cores              = config_get_boolean("plugin:proc:/proc/stat", "per cpu core utilization", CONFIG_BOOLEAN_YES);
        do_interrupts             = config_get_boolean("plugin:proc:/proc/stat", "cpu interrupts", CONFIG_BOOLEAN_YES);
        do_context                = config_get_boolean("plugin:proc:/proc/stat", "context switches", CONFIG_BOOLEAN_YES);
        do_forks                  = config_get_boolean("plugin:proc:/proc/stat", "processes started", CONFIG_BOOLEAN_YES);
        do_processes              = config_get_boolean("plugin:proc:/proc/stat", "processes running", CONFIG_BOOLEAN_YES);

        // give sane defaults based on the number of processors
        if(unlikely(processors > 50)) {
            // the system has too many processors
            keep_per_core_fds_open = CONFIG_BOOLEAN_NO;
            do_core_throttle_count = CONFIG_BOOLEAN_NO;
            do_package_throttle_count = CONFIG_BOOLEAN_NO;
            do_cpu_freq = CONFIG_BOOLEAN_NO;
            do_cpuidle = CONFIG_BOOLEAN_NO;
        }
        else {
            // the system has a reasonable number of processors
            keep_per_core_fds_open = CONFIG_BOOLEAN_YES;
            do_core_throttle_count = CONFIG_BOOLEAN_AUTO;
            do_package_throttle_count = CONFIG_BOOLEAN_NO;
            do_cpu_freq = CONFIG_BOOLEAN_YES;
            do_cpuidle = CONFIG_BOOLEAN_YES;
        }
        if(unlikely(processors > 24)) {
            // the system has too many processors
            keep_cpuidle_fds_open = CONFIG_BOOLEAN_NO;
        }
        else {
            // the system has a reasonable number of processors
            keep_cpuidle_fds_open = CONFIG_BOOLEAN_YES;
        }

        keep_per_core_fds_open    = config_get_boolean("plugin:proc:/proc/stat", "keep per core files open", keep_per_core_fds_open);
        keep_cpuidle_fds_open     = config_get_boolean("plugin:proc:/proc/stat", "keep cpuidle files open", keep_cpuidle_fds_open);
        do_core_throttle_count    = config_get_boolean_ondemand("plugin:proc:/proc/stat", "core_throttle_count", do_core_throttle_count);
        do_package_throttle_count = config_get_boolean_ondemand("plugin:proc:/proc/stat", "package_throttle_count", do_package_throttle_count);
        do_cpu_freq               = config_get_boolean_ondemand("plugin:proc:/proc/stat", "cpu frequency", do_cpu_freq);
        do_cpuidle                = config_get_boolean_ondemand("plugin:proc:/proc/stat", "cpu idle states", do_cpuidle);

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

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/cpu/%s/cpufreq/stats/time_in_state");
        time_in_state_filename = config_get("plugin:proc:/proc/stat", "time_in_state filename to monitor", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/schedstat");
        schedstat_filename = config_get("plugin:proc:/proc/stat", "schedstat filename to monitor", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/cpu/cpu%zu/cpuidle/state%zu/name");
        cpuidle_name_filename = config_get("plugin:proc:/proc/stat", "cpuidle name filename to monitor", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/system/cpu/cpu%zu/cpuidle/state%zu/time");
        cpuidle_time_filename = config_get("plugin:proc:/proc/stat", "cpuidle time filename to monitor", filename);
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
            if(likely(core > 0)) cores_found = core;

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

                if(unlikely(core >= all_cpu_charts_size)) {
                    size_t old_cpu_charts_size = all_cpu_charts_size;
                    all_cpu_charts_size = core + 1;
                    all_cpu_charts = reallocz(all_cpu_charts, sizeof(struct cpu_chart) * all_cpu_charts_size);
                    memset(&all_cpu_charts[old_cpu_charts_size], 0, sizeof(struct cpu_chart) * (all_cpu_charts_size - old_cpu_charts_size));
                }
                struct cpu_chart *cpu_chart = &all_cpu_charts[core];

                if(unlikely(!cpu_chart->st)) {
                    cpu_chart->id = strdupz(id);

                    if(unlikely(core == 0)) {
                        title = "Total CPU utilization";
                        type = "system";
                        context = "system.cpu";
                        family = id;
                        priority = NETDATA_CHART_PRIO_SYSTEM_CPU;
                    }
                    else {
                        title = "Core utilization";
                        type = "cpu";
                        context = "cpu.cpu";
                        family = "utilization";
                        priority = NETDATA_CHART_PRIO_CPU_PER_CORE;

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

                        if(do_cpu_freq != CONFIG_BOOLEAN_NO) {

                            snprintfz(filename, FILENAME_MAX, scaling_cur_freq_filename, id);

                            if (stat(filename, &stbuf) == 0) {
                                cpu_chart->files[CPU_FREQ_INDEX].filename = strdupz(filename);
                                cpu_chart->files[CPU_FREQ_INDEX].fd = -1;
                                do_cpu_freq = CONFIG_BOOLEAN_YES;
                            }

                            snprintfz(filename, FILENAME_MAX, time_in_state_filename, id);

                            if (stat(filename, &stbuf) == 0) {
                                cpu_chart->time_in_state_files.filename = strdupz(filename);
                                cpu_chart->time_in_state_files.ff = NULL;
                                do_cpu_freq = CONFIG_BOOLEAN_YES;
                                accurate_freq_avail = 1;
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
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_STAT_NAME
                            , priority + core
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

                    if(unlikely(core == 0 && cpus_var == NULL))
                        cpus_var = rrdvar_custom_host_variable_create(localhost, "active_processors");
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
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_STAT_NAME
                            , NETDATA_CHART_PRIO_SYSTEM_INTR
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
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_STAT_NAME
                            , NETDATA_CHART_PRIO_SYSTEM_CTXT
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
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_STAT_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_FORKS
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
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_STAT_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_PROCESSES
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
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_STAT_NAME
                            , NETDATA_CHART_PRIO_CORE_THROTTLING
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
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_STAT_NAME
                            , NETDATA_CHART_PRIO_PACKAGE_THROTTLING
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                else
                    rrdset_next(st_package_throttle_count);

                chart_per_core_files(&all_cpu_charts[1], all_cpu_charts_size - 1, PACKAGE_THROTTLE_COUNT_INDEX, st_package_throttle_count, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrdset_done(st_package_throttle_count);
            }
        }

        if(likely(do_cpu_freq != CONFIG_BOOLEAN_NO)) {
            char filename[FILENAME_MAX + 1];
            int r = 0;

            if (accurate_freq_avail) {
                r = read_per_core_time_in_state_files(&all_cpu_charts[1], all_cpu_charts_size - 1, CPU_FREQ_INDEX);
                if(r > 0 && !accurate_freq_is_used) {
                    accurate_freq_is_used = 1;
                    snprintfz(filename, FILENAME_MAX, time_in_state_filename, "cpu*");
                    info("cpufreq is using %s", filename);
                }
            }
            if (r < 1) {
                r = read_per_core_files(&all_cpu_charts[1], all_cpu_charts_size - 1, CPU_FREQ_INDEX);
                if(accurate_freq_is_used) {
                    accurate_freq_is_used = 0;
                    snprintfz(filename, FILENAME_MAX, scaling_cur_freq_filename, "cpu*");
                    info("cpufreq fell back to %s", filename);
                }
            }

            if(likely(r != -1 && (do_cpu_freq == CONFIG_BOOLEAN_YES || r > 0))) {
                do_cpu_freq = CONFIG_BOOLEAN_YES;

                static RRDSET *st_scaling_cur_freq = NULL;

                if(unlikely(!st_scaling_cur_freq))
                    st_scaling_cur_freq = rrdset_create_localhost(
                            "cpu"
                            , "cpufreq"
                            , NULL
                            , "cpufreq"
                            , "cpufreq.cpufreq"
                            , "Current CPU Frequency"
                            , "MHz"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_STAT_NAME
                            , NETDATA_CHART_PRIO_CPUFREQ_SCALING_CUR_FREQ
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                else
                    rrdset_next(st_scaling_cur_freq);

                chart_per_core_files(&all_cpu_charts[1], all_cpu_charts_size - 1, CPU_FREQ_INDEX, st_scaling_cur_freq, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                rrdset_done(st_scaling_cur_freq);
            }
        }
    }

    // --------------------------------------------------------------------

    static struct per_core_cpuidle_chart *cpuidle_charts = NULL;

    if(likely(do_cpuidle != CONFIG_BOOLEAN_NO && !read_schedstat(schedstat_filename, &cpuidle_charts, cores_found))) {
        int cpu_states_updated = 0;
        size_t core, state;


        // proc.plugin runs on Linux systems only. Multi-platform compatibility is not needed here,
        // so bare pthread functions are used to avoid unneeded overheads.
        for(core = 0; core < cores_found; core++) {
            if(unlikely(!(cpuidle_charts[core].active_time - cpuidle_charts[core].last_active_time))) {
                pthread_t thread;

                if(unlikely(pthread_create(&thread, NULL, wake_cpu_thread, (void *)&core)))
                    error("Cannot create wake_cpu_thread");
                else if(unlikely(pthread_join(thread, NULL)))
                    error("Cannot join wake_cpu_thread");
                cpu_states_updated = 1;
            }
        }

        if(unlikely(!cpu_states_updated || !read_schedstat(schedstat_filename, &cpuidle_charts, cores_found))) {
            for(core = 0; core < cores_found; core++) {
                cpuidle_charts[core].last_active_time = cpuidle_charts[core].active_time;

                int r = read_cpuidle_states(cpuidle_name_filename, cpuidle_time_filename, cpuidle_charts, core);
                if(likely(r != -1 && (do_cpuidle == CONFIG_BOOLEAN_YES || r > 0))) {
                    do_cpuidle = CONFIG_BOOLEAN_YES;

                    char cpuidle_chart_id[RRD_ID_LENGTH_MAX + 1];
                    snprintfz(cpuidle_chart_id, RRD_ID_LENGTH_MAX, "cpu%zu_cpuidle", core);

                    if(unlikely(!cpuidle_charts[core].st)) {
                        cpuidle_charts[core].st = rrdset_create_localhost(
                                "cpu"
                                , cpuidle_chart_id
                                , NULL
                                , "cpuidle"
                                , "cpuidle.cpuidle"
                                , "C-state residency"
                                , "time%"
                                , PLUGIN_PROC_NAME
                                , PLUGIN_PROC_MODULE_STAT_NAME
                                , NETDATA_CHART_PRIO_CPUIDLE + core
                                , update_every
                                , RRDSET_TYPE_STACKED
                        );

                        char cpuidle_dim_id[RRD_ID_LENGTH_MAX + 1];
                        snprintfz(cpuidle_dim_id, RRD_ID_LENGTH_MAX, "cpu%zu_active_time", core);
                        cpuidle_charts[core].active_time_rd = rrddim_add(cpuidle_charts[core].st, cpuidle_dim_id, "C0 (active)", 1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                        for(state = 0; state < cpuidle_charts[core].cpuidle_state_len; state++) {
                            snprintfz(cpuidle_dim_id, RRD_ID_LENGTH_MAX, "cpu%zu_cpuidle_state%zu_time", core, state);
                            cpuidle_charts[core].cpuidle_state[state].rd = rrddim_add(cpuidle_charts[core].st, cpuidle_dim_id,
                                                                                      cpuidle_charts[core].cpuidle_state[state].name,
                                                                                      1, 1, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL);
                        }
                    }
                    else
                        rrdset_next(cpuidle_charts[core].st);

                    rrddim_set_by_pointer(cpuidle_charts[core].st, cpuidle_charts[core].active_time_rd, cpuidle_charts[core].active_time);
                    for(state = 0; state < cpuidle_charts[core].cpuidle_state_len; state++) {
                        rrddim_set_by_pointer(cpuidle_charts[core].st, cpuidle_charts[core].cpuidle_state[state].rd, cpuidle_charts[core].cpuidle_state[state].value);
                    }
                    rrdset_done(cpuidle_charts[core].st);
                }
            }
        }
    }

    if(cpus_var)
        rrdvar_custom_host_variable_set(localhost, cpus_var, cores_found);

    return 0;
}
