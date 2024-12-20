// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <linux/perf_event.h>

#define PLUGIN_PERF_NAME "perf.plugin"

// Hardware counters
#define NETDATA_CHART_PRIO_PERF_CPU_CYCLES            8800
#define NETDATA_CHART_PRIO_PERF_INSTRUCTIONS          8801
#define NETDATA_CHART_PRIO_PERF_IPC                   8802
#define NETDATA_CHART_PRIO_PERF_BRANCH_INSTRUCTIONS   8803
#define NETDATA_CHART_PRIO_PERF_CACHE                 8804
#define NETDATA_CHART_PRIO_PERF_BUS_CYCLES            8805
#define NETDATA_CHART_PRIO_PERF_FRONT_BACK_CYCLES     8806

// Software counters
#define NETDATA_CHART_PRIO_PERF_MIGRATIONS            8810
#define NETDATA_CHART_PRIO_PERF_ALIGNMENT             8811
#define NETDATA_CHART_PRIO_PERF_EMULATION             8812

// Hardware cache counters
#define NETDATA_CHART_PRIO_PERF_L1D                   8820
#define NETDATA_CHART_PRIO_PERF_L1D_PREFETCH          8821
#define NETDATA_CHART_PRIO_PERF_L1I                   8822
#define NETDATA_CHART_PRIO_PERF_LL                    8823
#define NETDATA_CHART_PRIO_PERF_DTLB                  8824
#define NETDATA_CHART_PRIO_PERF_ITLB                  8825
#define NETDATA_CHART_PRIO_PERF_PBU                   8826

#define RRD_TYPE_PERF "perf"
#define RRD_FAMILY_HW "hardware"
#define RRD_FAMILY_SW "software"
#define RRD_FAMILY_CACHE "cache"

#define NO_FD (-1)
#define ALL_PIDS (-1)
#define RUNNING_THRESHOLD 100

static int debug = 0;

static int update_every = 1;
static int freq = 0;

typedef enum perf_event_id {
    // Hardware counters
    EV_ID_CPU_CYCLES,
    EV_ID_INSTRUCTIONS,
    EV_ID_CACHE_REFERENCES,
    EV_ID_CACHE_MISSES,
    EV_ID_BRANCH_INSTRUCTIONS,
    EV_ID_BRANCH_MISSES,
    EV_ID_BUS_CYCLES,
    EV_ID_STALLED_CYCLES_FRONTEND,
    EV_ID_STALLED_CYCLES_BACKEND,
    EV_ID_REF_CPU_CYCLES,

    // Software counters
    // EV_ID_CPU_CLOCK,
    // EV_ID_TASK_CLOCK,
    // EV_ID_PAGE_FAULTS,
    // EV_ID_CONTEXT_SWITCHES,
    EV_ID_CPU_MIGRATIONS,
    // EV_ID_PAGE_FAULTS_MIN,
    // EV_ID_PAGE_FAULTS_MAJ,
    EV_ID_ALIGNMENT_FAULTS,
    EV_ID_EMULATION_FAULTS,

    // Hardware cache counters
    EV_ID_L1D_READ_ACCESS,
    EV_ID_L1D_READ_MISS,
    EV_ID_L1D_WRITE_ACCESS,
    EV_ID_L1D_WRITE_MISS,
    EV_ID_L1D_PREFETCH_ACCESS,

    EV_ID_L1I_READ_ACCESS,
    EV_ID_L1I_READ_MISS,

    EV_ID_LL_READ_ACCESS,
    EV_ID_LL_READ_MISS,
    EV_ID_LL_WRITE_ACCESS,
    EV_ID_LL_WRITE_MISS,

    EV_ID_DTLB_READ_ACCESS,
    EV_ID_DTLB_READ_MISS,
    EV_ID_DTLB_WRITE_ACCESS,
    EV_ID_DTLB_WRITE_MISS,

    EV_ID_ITLB_READ_ACCESS,
    EV_ID_ITLB_READ_MISS,

    EV_ID_PBU_READ_ACCESS,

    EV_ID_END
} perf_event_id_t;

enum perf_event_group {
    EV_GROUP_CYCLES,
    EV_GROUP_INSTRUCTIONS_AND_CACHE,
    EV_GROUP_SOFTWARE,
    EV_GROUP_CACHE_L1D,
    EV_GROUP_CACHE_L1I_LL_DTLB,
    EV_GROUP_CACHE_ITLB_BPU,

    EV_GROUP_NUM
};

static int number_of_cpus;

static int *group_leader_fds[EV_GROUP_NUM];

static struct perf_event {
    perf_event_id_t id;

    int type;
    int config;

    int **group_leader_fd;
    int *fd;

    int disabled;
    int updated;

    uint64_t value;

    uint64_t *prev_value;
    uint64_t *prev_time_enabled;
    uint64_t *prev_time_running;
} perf_events[] = {
    // Hardware counters
    {EV_ID_CPU_CYCLES,              PERF_TYPE_HARDWARE, PERF_COUNT_HW_CPU_CYCLES,              &group_leader_fds[EV_GROUP_CYCLES], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_INSTRUCTIONS,            PERF_TYPE_HARDWARE, PERF_COUNT_HW_INSTRUCTIONS,            &group_leader_fds[EV_GROUP_INSTRUCTIONS_AND_CACHE], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_CACHE_REFERENCES,        PERF_TYPE_HARDWARE, PERF_COUNT_HW_CACHE_REFERENCES,        &group_leader_fds[EV_GROUP_INSTRUCTIONS_AND_CACHE], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_CACHE_MISSES,            PERF_TYPE_HARDWARE, PERF_COUNT_HW_CACHE_MISSES,            &group_leader_fds[EV_GROUP_INSTRUCTIONS_AND_CACHE], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_BRANCH_INSTRUCTIONS,     PERF_TYPE_HARDWARE, PERF_COUNT_HW_BRANCH_INSTRUCTIONS,     &group_leader_fds[EV_GROUP_INSTRUCTIONS_AND_CACHE], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_BRANCH_MISSES,           PERF_TYPE_HARDWARE, PERF_COUNT_HW_BRANCH_MISSES,           &group_leader_fds[EV_GROUP_INSTRUCTIONS_AND_CACHE], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_BUS_CYCLES,              PERF_TYPE_HARDWARE, PERF_COUNT_HW_BUS_CYCLES,              &group_leader_fds[EV_GROUP_CYCLES], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_STALLED_CYCLES_FRONTEND, PERF_TYPE_HARDWARE, PERF_COUNT_HW_STALLED_CYCLES_FRONTEND, &group_leader_fds[EV_GROUP_CYCLES], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_STALLED_CYCLES_BACKEND,  PERF_TYPE_HARDWARE, PERF_COUNT_HW_STALLED_CYCLES_BACKEND,  &group_leader_fds[EV_GROUP_CYCLES], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_REF_CPU_CYCLES,          PERF_TYPE_HARDWARE, PERF_COUNT_HW_REF_CPU_CYCLES,          &group_leader_fds[EV_GROUP_CYCLES], NULL, 1, 0, 0, NULL, NULL, NULL},

    // Software counters
    // {EV_ID_CPU_CLOCK,        PERF_TYPE_SOFTWARE, PERF_COUNT_SW_CPU_CLOCK,        &group_leader_fds[EV_GROUP_SOFTWARE], NULL, 1, 0, 0, NULL, NULL, NULL},
    // {EV_ID_TASK_CLOCK,       PERF_TYPE_SOFTWARE, PERF_COUNT_SW_TASK_CLOCK,       &group_leader_fds[EV_GROUP_SOFTWARE], NULL, 1, 0, 0, NULL, NULL, NULL},
    // {EV_ID_PAGE_FAULTS,      PERF_TYPE_SOFTWARE, PERF_COUNT_SW_PAGE_FAULTS,      &group_leader_fds[EV_GROUP_SOFTWARE], NULL, 1, 0, 0, NULL, NULL, NULL},
    // {EV_ID_CONTEXT_SWITCHES, PERF_TYPE_SOFTWARE, PERF_COUNT_SW_CONTEXT_SWITCHES, &group_leader_fds[EV_GROUP_SOFTWARE], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_CPU_MIGRATIONS,   PERF_TYPE_SOFTWARE, PERF_COUNT_SW_CPU_MIGRATIONS,   &group_leader_fds[EV_GROUP_SOFTWARE], NULL, 1, 0, 0, NULL, NULL, NULL},
    // {EV_ID_PAGE_FAULTS_MIN,  PERF_TYPE_SOFTWARE, PERF_COUNT_SW_PAGE_FAULTS_MIN,  &group_leader_fds[EV_GROUP_SOFTWARE], NULL, 1, 0, 0, NULL, NULL, NULL},
    // {EV_ID_PAGE_FAULTS_MAJ,  PERF_TYPE_SOFTWARE, PERF_COUNT_SW_PAGE_FAULTS_MAJ,  &group_leader_fds[EV_GROUP_SOFTWARE], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_ALIGNMENT_FAULTS, PERF_TYPE_SOFTWARE, PERF_COUNT_SW_ALIGNMENT_FAULTS, &group_leader_fds[EV_GROUP_SOFTWARE], NULL, 1, 0, 0, NULL, NULL, NULL},
    {EV_ID_EMULATION_FAULTS, PERF_TYPE_SOFTWARE, PERF_COUNT_SW_EMULATION_FAULTS, &group_leader_fds[EV_GROUP_SOFTWARE], NULL, 1, 0, 0, NULL, NULL, NULL},

    // Hardware cache counters
    {
        EV_ID_L1D_READ_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_L1D) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1D], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_L1D_READ_MISS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_L1D) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1D], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_L1D_WRITE_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_L1D) | (PERF_COUNT_HW_CACHE_OP_WRITE << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1D], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_L1D_WRITE_MISS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_L1D) | (PERF_COUNT_HW_CACHE_OP_WRITE << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1D], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_L1D_PREFETCH_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_L1D) | (PERF_COUNT_HW_CACHE_OP_PREFETCH << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1D], NULL, 1, 0, 0, NULL, NULL, NULL
    },

    {
        EV_ID_L1I_READ_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_L1I) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1I_LL_DTLB], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_L1I_READ_MISS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_L1I) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1I_LL_DTLB], NULL, 1, 0, 0, NULL, NULL, NULL
    },

    {
        EV_ID_LL_READ_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_LL) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1I_LL_DTLB], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_LL_READ_MISS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_LL) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1I_LL_DTLB], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_LL_WRITE_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_LL) | (PERF_COUNT_HW_CACHE_OP_WRITE << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1I_LL_DTLB], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_LL_WRITE_MISS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_LL) | (PERF_COUNT_HW_CACHE_OP_WRITE << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1I_LL_DTLB], NULL, 1, 0, 0, NULL, NULL, NULL
    },

    {
        EV_ID_DTLB_READ_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_DTLB) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1I_LL_DTLB], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_DTLB_READ_MISS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_DTLB) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1I_LL_DTLB], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_DTLB_WRITE_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_DTLB) | (PERF_COUNT_HW_CACHE_OP_WRITE << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_L1I_LL_DTLB], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_DTLB_WRITE_MISS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_DTLB) | (PERF_COUNT_HW_CACHE_OP_WRITE << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS << 16),
        &group_leader_fds[EV_GROUP_CACHE_ITLB_BPU], NULL, 1, 0, 0, NULL, NULL, NULL
    },

    {
        EV_ID_ITLB_READ_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_ITLB) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_ITLB_BPU], NULL, 1, 0, 0, NULL, NULL, NULL
    }, {
        EV_ID_ITLB_READ_MISS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_ITLB) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS << 16),
        &group_leader_fds[EV_GROUP_CACHE_ITLB_BPU], NULL, 1, 0, 0, NULL, NULL, NULL
    },

    {
        EV_ID_PBU_READ_ACCESS, PERF_TYPE_HW_CACHE,
        (PERF_COUNT_HW_CACHE_BPU) | (PERF_COUNT_HW_CACHE_OP_READ << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16),
        &group_leader_fds[EV_GROUP_CACHE_ITLB_BPU], NULL, 1, 0, 0, NULL, NULL, NULL
    },

    {EV_ID_END, 0, 0, NULL, NULL, 0, 0, 0, NULL, NULL, NULL}
};

static bool perf_init() {
    int cpu, group;
    struct perf_event_attr perf_event_attr;
    struct perf_event *current_event = NULL;
    unsigned long flags = 0;

    number_of_cpus = (int)os_get_system_cpus();

    // initialize all perf event file descriptors
    for(current_event = &perf_events[0]; current_event->id != EV_ID_END; current_event++) {
        current_event->fd = mallocz(number_of_cpus * sizeof(int));
        memset(current_event->fd, NO_FD, number_of_cpus * sizeof(int));

        current_event->prev_value = mallocz(number_of_cpus * sizeof(uint64_t));
        memset(current_event->prev_value, 0, number_of_cpus * sizeof(uint64_t));

        current_event->prev_time_enabled = mallocz(number_of_cpus * sizeof(uint64_t));
        memset(current_event->prev_time_enabled, 0, number_of_cpus * sizeof(uint64_t));

        current_event->prev_time_running = mallocz(number_of_cpus * sizeof(uint64_t));
        memset(current_event->prev_time_running, 0, number_of_cpus * sizeof(uint64_t));
    }

    for(group = 0; group < EV_GROUP_NUM; group++) {
        group_leader_fds[group] = mallocz(number_of_cpus * sizeof(int));
        memset(group_leader_fds[group], NO_FD, number_of_cpus * sizeof(int));
    }

    memset(&perf_event_attr, 0, sizeof(perf_event_attr));

    int enabled = 0;

    for(cpu = 0; cpu < number_of_cpus; cpu++) {
        for(current_event = &perf_events[0]; current_event->id != EV_ID_END; current_event++) {
            if(unlikely(current_event->disabled)) continue;

            perf_event_attr.type = current_event->type;
            perf_event_attr.config = current_event->config;
            perf_event_attr.read_format = PERF_FORMAT_TOTAL_TIME_ENABLED | PERF_FORMAT_TOTAL_TIME_RUNNING;

            int fd, group_leader_fd = *(*current_event->group_leader_fd + cpu);

            fd = syscall(
                __NR_perf_event_open,
                &perf_event_attr,
                ALL_PIDS,
                cpu,
                group_leader_fd,
                flags
            );

            if(unlikely(group_leader_fd == NO_FD)) group_leader_fd = fd;

            if(unlikely(fd < 0)) {
                switch errno {
                    case EACCES:
                        collector_error("Cannot access to the PMU: Permission denied");
                        break;
                    case EBUSY:
                        collector_error("Another event already has exclusive access to the PMU");
                        break;
                    default:
                        collector_error("Cannot open perf event");
                }
                collector_error("Disabling event %u", current_event->id);
                current_event->disabled = 1;
            } else {
                enabled++;
            }

            *(current_event->fd + cpu) = fd;
            *(*current_event->group_leader_fd + cpu) = group_leader_fd;

            if(unlikely(debug)) fprintf(stderr, "perf.plugin: event id = %u, cpu = %d, fd = %d, leader_fd = %d\n", current_event->id, cpu, fd, group_leader_fd);
        }
    }

    return enabled > 0;
}

static void perf_free(void) {
    int cpu, group;
    struct perf_event *current_event = NULL;

    for(current_event = &perf_events[0]; current_event->id != EV_ID_END; current_event++) {
        for(cpu = 0; cpu < number_of_cpus; cpu++)
            if(*(current_event->fd + cpu) != NO_FD) close(*(current_event->fd + cpu));

        free(current_event->fd);
        free(current_event->prev_value);
        free(current_event->prev_time_enabled);
        free(current_event->prev_time_running);
    }

    for(group = 0; group < EV_GROUP_NUM; group++)
        free(group_leader_fds[group]);
}

static void reenable_events() {
    int group, cpu;

    for(group = 0; group < EV_GROUP_NUM; group++) {
        for(cpu = 0; cpu < number_of_cpus; cpu++) {
            int current_fd = *(group_leader_fds[group] + cpu);

            if(unlikely(current_fd == NO_FD)) continue;

            if(ioctl(current_fd, PERF_EVENT_IOC_DISABLE, PERF_IOC_FLAG_GROUP) == -1
               || ioctl(current_fd, PERF_EVENT_IOC_ENABLE, PERF_IOC_FLAG_GROUP) == -1)
            {
                collector_error("Cannot reenable event group");
            }
        }
    }
}

static int perf_collect() {
    int cpu;
    struct perf_event *current_event = NULL;
    static uint64_t prev_cpu_cycles_value = 0;
    struct {
        uint64_t value;
        uint64_t time_enabled;
        uint64_t time_running;
    } read_result;

    for(current_event = &perf_events[0]; current_event->id != EV_ID_END; current_event++) {
        current_event->updated = 0;
        current_event->value = 0;

        if(unlikely(current_event->disabled)) continue;

        for(cpu = 0; cpu < number_of_cpus; cpu++) {

            ssize_t read_size = read(current_event->fd[cpu], &read_result, sizeof(read_result));

            if(likely(read_size == sizeof(read_result))) {
                if (likely(read_result.time_running
                           && read_result.time_running != *(current_event->prev_time_running + cpu)
                           && (read_result.time_enabled / read_result.time_running < RUNNING_THRESHOLD))) {
                    current_event->value += (read_result.value - *(current_event->prev_value + cpu)) \
                                             * (read_result.time_enabled - *(current_event->prev_time_enabled + cpu)) \
                                             / (read_result.time_running - *(current_event->prev_time_running + cpu));
                }

                *(current_event->prev_value + cpu) = read_result.value;
                *(current_event->prev_time_enabled + cpu) = read_result.time_enabled;
                *(current_event->prev_time_running + cpu) = read_result.time_running;

                current_event->updated = 1;
            }
            else {
                collector_error("Cannot update value for event %u", current_event->id);
                return 1;
            }
        }

        if(unlikely(debug)) fprintf(stderr, "perf.plugin: successfully read event id = %u, value = %"PRIu64"\n", current_event->id, current_event->value);
    }

    if(unlikely(perf_events[EV_ID_CPU_CYCLES].value == prev_cpu_cycles_value))
        reenable_events();
    prev_cpu_cycles_value = perf_events[EV_ID_CPU_CYCLES].value;

    return 0;
}

static void perf_send_metrics() {
    static int // Hardware counters
               cpu_cycles_chart_generated = 0,
               instructions_chart_generated = 0,
               ipc_chart_generated = 0,
               branch_chart_generated = 0,
               cache_chart_generated = 0,
               bus_cycles_chart_generated = 0,
               stalled_cycles_chart_generated = 0,

               // Software counters
               migrations_chart_generated = 0,
               alignment_chart_generated = 0,
               emulation_chart_generated = 0,

               // Hardware cache counters
               L1D_chart_generated = 0,
               L1D_prefetch_chart_generated = 0,
               L1I_chart_generated = 0,
               LL_chart_generated = 0,
               DTLB_chart_generated = 0,
               ITLB_chart_generated = 0,
               PBU_chart_generated = 0;

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_CPU_CYCLES].updated || perf_events[EV_ID_REF_CPU_CYCLES].updated)) {
        if(unlikely(!cpu_cycles_chart_generated)) {
            cpu_cycles_chart_generated = 1;

            printf("CHART %s.%s '' 'CPU cycles' 'cycles/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "cpu_cycles"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_CPU_CYCLES
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "cpu");
            printf("DIMENSION %s '' absolute 1 1\n", "ref_cpu");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "cpu_cycles"
        );
        if(likely(perf_events[EV_ID_CPU_CYCLES].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "cpu"
                   , (collected_number) perf_events[EV_ID_CPU_CYCLES].value
            );
        }
        if(likely(perf_events[EV_ID_REF_CPU_CYCLES].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "ref_cpu"
                   , (collected_number) perf_events[EV_ID_REF_CPU_CYCLES].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_INSTRUCTIONS].updated)) {
        if(unlikely(!instructions_chart_generated)) {
            instructions_chart_generated = 1;

            printf("CHART %s.%s '' 'Instructions' 'instructions/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "instructions"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_INSTRUCTIONS
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "instructions");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "instructions"
        );
        printf(
               "SET %s = %lld\n"
               , "instructions"
               , (collected_number) perf_events[EV_ID_INSTRUCTIONS].value
        );
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_INSTRUCTIONS].updated) && likely(perf_events[EV_ID_CPU_CYCLES].updated)) {
        if(unlikely(!ipc_chart_generated)) {
            ipc_chart_generated = 1;

            printf("CHART %s.%s '' '%s' 'instructions/cycle' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "instructions_per_cycle"
                   , "Instructions per Cycle(IPC)"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_IPC
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 100\n", "ipc");
        }

        printf("BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "instructions_per_cycle"
               );

        NETDATA_DOUBLE result = ((NETDATA_DOUBLE)perf_events[EV_ID_INSTRUCTIONS].value /
                                    (NETDATA_DOUBLE)perf_events[EV_ID_CPU_CYCLES].value) * 100.0;
        printf("SET %s = %lld\n"
               , "ipc"
               , (collected_number) result
               );
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_BRANCH_INSTRUCTIONS].updated || perf_events[EV_ID_BRANCH_MISSES].updated)) {
        if(unlikely(!branch_chart_generated)) {
            branch_chart_generated = 1;

            printf("CHART %s.%s '' 'Branch instructions' 'instructions/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "branch_instructions"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_BRANCH_INSTRUCTIONS
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "instructions");
            printf("DIMENSION %s '' absolute 1 1\n", "misses");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "branch_instructions"
        );
        if(likely(perf_events[EV_ID_BRANCH_INSTRUCTIONS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "instructions"
                   , (collected_number) perf_events[EV_ID_BRANCH_INSTRUCTIONS].value
            );
        }
        if(likely(perf_events[EV_ID_BRANCH_MISSES].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "misses"
                   , (collected_number) perf_events[EV_ID_BRANCH_MISSES].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_CACHE_REFERENCES].updated || perf_events[EV_ID_CACHE_MISSES].updated)) {
        if(unlikely(!cache_chart_generated)) {
            cache_chart_generated = 1;

            printf("CHART %s.%s '' 'Cache operations' 'operations/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "cache"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_CACHE
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "references");
            printf("DIMENSION %s '' absolute 1 1\n", "misses");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "cache"
        );
        if(likely(perf_events[EV_ID_CACHE_REFERENCES].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "references"
                   , (collected_number) perf_events[EV_ID_CACHE_REFERENCES].value
            );
        }
        if(likely(perf_events[EV_ID_CACHE_MISSES].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "misses"
                   , (collected_number) perf_events[EV_ID_CACHE_MISSES].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_BUS_CYCLES].updated)) {
        if(unlikely(!bus_cycles_chart_generated)) {
            bus_cycles_chart_generated = 1;

            printf("CHART %s.%s '' 'Bus cycles' 'cycles/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "bus_cycles"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_BUS_CYCLES
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "bus");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "bus_cycles"
        );
        printf(
               "SET %s = %lld\n"
               , "bus"
               , (collected_number) perf_events[EV_ID_BUS_CYCLES].value
        );
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_STALLED_CYCLES_FRONTEND].updated || perf_events[EV_ID_STALLED_CYCLES_BACKEND].updated)) {
        if(unlikely(!stalled_cycles_chart_generated)) {
            stalled_cycles_chart_generated = 1;

            printf("CHART %s.%s '' 'Stalled frontend and backend cycles' 'cycles/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "stalled_cycles"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_FRONT_BACK_CYCLES
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "frontend");
            printf("DIMENSION %s '' absolute 1 1\n", "backend");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "stalled_cycles"
        );
        if(likely(perf_events[EV_ID_STALLED_CYCLES_FRONTEND].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "frontend"
                   , (collected_number) perf_events[EV_ID_STALLED_CYCLES_FRONTEND].value
            );
        }
        if(likely(perf_events[EV_ID_STALLED_CYCLES_BACKEND].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "backend"
                   , (collected_number) perf_events[EV_ID_STALLED_CYCLES_BACKEND].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_CPU_MIGRATIONS].updated)) {
        if(unlikely(!migrations_chart_generated)) {
            migrations_chart_generated = 1;

            printf("CHART %s.%s '' 'CPU migrations' 'migrations' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "migrations"
                   , RRD_FAMILY_SW
                   , NETDATA_CHART_PRIO_PERF_MIGRATIONS
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "migrations");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "migrations"
        );
        printf(
               "SET %s = %lld\n"
               , "migrations"
               , (collected_number) perf_events[EV_ID_CPU_MIGRATIONS].value
        );
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_ALIGNMENT_FAULTS].updated)) {
        if(unlikely(!alignment_chart_generated)) {
            alignment_chart_generated = 1;

            printf("CHART %s.%s '' 'Alignment faults' 'faults' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "alignment_faults"
                   , RRD_FAMILY_SW
                   , NETDATA_CHART_PRIO_PERF_ALIGNMENT
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "faults");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "alignment_faults"
        );
        printf(
               "SET %s = %lld\n"
               , "faults"
               , (collected_number) perf_events[EV_ID_ALIGNMENT_FAULTS].value
        );
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_EMULATION_FAULTS].updated)) {
        if(unlikely(!emulation_chart_generated)) {
            emulation_chart_generated = 1;

            printf("CHART %s.%s '' 'Emulation faults' 'faults' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "emulation_faults"
                   , RRD_FAMILY_SW
                   , NETDATA_CHART_PRIO_PERF_EMULATION
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "faults");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "emulation_faults"
        );
        printf(
               "SET %s = %lld\n"
               , "faults"
               , (collected_number) perf_events[EV_ID_EMULATION_FAULTS].value
        );
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_L1D_READ_ACCESS].updated || perf_events[EV_ID_L1D_READ_MISS].updated
              || perf_events[EV_ID_L1D_WRITE_ACCESS].updated || perf_events[EV_ID_L1D_WRITE_MISS].updated)) {
        if(unlikely(!L1D_chart_generated)) {
            L1D_chart_generated = 1;

            printf("CHART %s.%s '' 'L1D cache operations' 'events/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "l1d_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_L1D
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "read_access");
            printf("DIMENSION %s '' absolute 1 1\n", "read_misses");
            printf("DIMENSION %s '' absolute -1 1\n", "write_access");
            printf("DIMENSION %s '' absolute -1 1\n", "write_misses");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "l1d_cache"
        );
        if(likely(perf_events[EV_ID_L1D_READ_ACCESS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_access"
                   , (collected_number) perf_events[EV_ID_L1D_READ_ACCESS].value
            );
        }
        if(likely(perf_events[EV_ID_L1D_READ_MISS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_misses"
                   , (collected_number) perf_events[EV_ID_L1D_READ_MISS].value
            );
        }
        if(likely(perf_events[EV_ID_L1D_WRITE_ACCESS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "write_access"
                   , (collected_number) perf_events[EV_ID_L1D_WRITE_ACCESS].value
            );
        }
        if(likely(perf_events[EV_ID_L1D_WRITE_MISS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "write_misses"
                   , (collected_number) perf_events[EV_ID_L1D_WRITE_MISS].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_L1D_PREFETCH_ACCESS].updated)) {
        if(unlikely(!L1D_prefetch_chart_generated)) {
            L1D_prefetch_chart_generated = 1;

            printf("CHART %s.%s '' 'L1D prefetch cache operations' 'prefetches/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "l1d_cache_prefetch"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_L1D_PREFETCH
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "prefetches");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "l1d_cache_prefetch"
        );
        printf(
               "SET %s = %lld\n"
               , "prefetches"
               , (collected_number) perf_events[EV_ID_L1D_PREFETCH_ACCESS].value
        );
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_L1I_READ_ACCESS].updated || perf_events[EV_ID_L1I_READ_MISS].updated)) {
        if(unlikely(!L1I_chart_generated)) {
            L1I_chart_generated = 1;

            printf("CHART %s.%s '' 'L1I cache operations' 'events/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "l1i_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_L1I
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "read_access");
            printf("DIMENSION %s '' absolute 1 1\n", "read_misses");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "l1i_cache"
        );
        if(likely(perf_events[EV_ID_L1I_READ_ACCESS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_access"
                   , (collected_number) perf_events[EV_ID_L1I_READ_ACCESS].value
            );
        }
        if(likely(perf_events[EV_ID_L1I_READ_MISS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_misses"
                   , (collected_number) perf_events[EV_ID_L1I_READ_MISS].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_LL_READ_ACCESS].updated || perf_events[EV_ID_LL_READ_MISS].updated
              || perf_events[EV_ID_LL_WRITE_ACCESS].updated || perf_events[EV_ID_LL_WRITE_MISS].updated)) {
        if(unlikely(!LL_chart_generated)) {
            LL_chart_generated = 1;

            printf("CHART %s.%s '' 'LL cache operations' 'events/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "ll_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_LL
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "read_access");
            printf("DIMENSION %s '' absolute 1 1\n", "read_misses");
            printf("DIMENSION %s '' absolute -1 1\n", "write_access");
            printf("DIMENSION %s '' absolute -1 1\n", "write_misses");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "ll_cache"
        );
        if(likely(perf_events[EV_ID_LL_READ_ACCESS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_access"
                   , (collected_number) perf_events[EV_ID_LL_READ_ACCESS].value
            );
        }
        if(likely(perf_events[EV_ID_LL_READ_MISS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_misses"
                   , (collected_number) perf_events[EV_ID_LL_READ_MISS].value
            );
        }
        if(likely(perf_events[EV_ID_LL_WRITE_ACCESS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "write_access"
                   , (collected_number) perf_events[EV_ID_LL_WRITE_ACCESS].value
            );
        }
        if(likely(perf_events[EV_ID_LL_WRITE_MISS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "write_misses"
                   , (collected_number) perf_events[EV_ID_LL_WRITE_MISS].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_DTLB_READ_ACCESS].updated || perf_events[EV_ID_DTLB_READ_MISS].updated
              || perf_events[EV_ID_DTLB_WRITE_ACCESS].updated || perf_events[EV_ID_DTLB_WRITE_MISS].updated)) {
        if(unlikely(!DTLB_chart_generated)) {
            DTLB_chart_generated = 1;

            printf("CHART %s.%s '' 'DTLB cache operations' 'events/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "dtlb_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_DTLB
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "read_access");
            printf("DIMENSION %s '' absolute 1 1\n", "read_misses");
            printf("DIMENSION %s '' absolute -1 1\n", "write_access");
            printf("DIMENSION %s '' absolute -1 1\n", "write_misses");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "dtlb_cache"
        );
        if(likely(perf_events[EV_ID_DTLB_READ_ACCESS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_access"
                   , (collected_number) perf_events[EV_ID_DTLB_READ_ACCESS].value
            );
        }
        if(likely(perf_events[EV_ID_DTLB_READ_MISS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_misses"
                   , (collected_number) perf_events[EV_ID_DTLB_READ_MISS].value
            );
        }
        if(likely(perf_events[EV_ID_DTLB_WRITE_ACCESS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "write_access"
                   , (collected_number) perf_events[EV_ID_DTLB_WRITE_ACCESS].value
            );
        }
        if(likely(perf_events[EV_ID_DTLB_WRITE_MISS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "write_misses"
                   , (collected_number) perf_events[EV_ID_DTLB_WRITE_MISS].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_ITLB_READ_ACCESS].updated || perf_events[EV_ID_ITLB_READ_MISS].updated)) {
        if(unlikely(!ITLB_chart_generated)) {
            ITLB_chart_generated = 1;

            printf("CHART %s.%s '' 'ITLB cache operations' 'events/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "itlb_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_ITLB
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "read_access");
            printf("DIMENSION %s '' absolute 1 1\n", "read_misses");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "itlb_cache"
        );
        if(likely(perf_events[EV_ID_ITLB_READ_ACCESS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_access"
                   , (collected_number) perf_events[EV_ID_ITLB_READ_ACCESS].value
            );
        }
        if(likely(perf_events[EV_ID_ITLB_READ_MISS].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "read_misses"
                   , (collected_number) perf_events[EV_ID_ITLB_READ_MISS].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_PBU_READ_ACCESS].updated)) {
        if(unlikely(!PBU_chart_generated)) {
            PBU_chart_generated = 1;

            printf("CHART %s.%s '' 'PBU cache operations' 'events/s' %s '' line %d %d '' %s\n"
                   , RRD_TYPE_PERF
                   , "pbu_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_PBU
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' absolute 1 1\n", "read_access");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "pbu_cache"
        );
        printf(
               "SET %s = %lld\n"
               , "read_access"
               , (collected_number) perf_events[EV_ID_PBU_READ_ACCESS].value
        );
        printf("END\n");
    }
}

void parse_command_line(int argc, char **argv) {
    int i, plugin_enabled = 0;

    for(i = 1; i < argc ; i++) {
        if(isdigit(*argv[i]) && !freq) {
            int n = str2i(argv[i]);
            if(n > 0 && n < 86400) {
                freq = n;
                continue;
            }
        }
        else if(strcmp("version", argv[i]) == 0 || strcmp("-version", argv[i]) == 0 || strcmp("--version", argv[i]) == 0 || strcmp("-v", argv[i]) == 0 || strcmp("-V", argv[i]) == 0) {
            printf("perf.plugin %s\n", NETDATA_VERSION);
            exit(0);
        }
        else if(strcmp("all", argv[i]) == 0) {
            struct perf_event *current_event = NULL;

            for(current_event = &perf_events[0]; current_event->id != EV_ID_END; current_event++)
                current_event->disabled = 0;

            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("cycles", argv[i]) == 0) {
            perf_events[EV_ID_CPU_CYCLES].disabled = 0;
            perf_events[EV_ID_REF_CPU_CYCLES].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("instructions", argv[i]) == 0) {
            perf_events[EV_ID_INSTRUCTIONS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("branch", argv[i]) == 0) {
            perf_events[EV_ID_BRANCH_INSTRUCTIONS].disabled = 0;
            perf_events[EV_ID_BRANCH_MISSES].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("cache", argv[i]) == 0) {
            perf_events[EV_ID_CACHE_REFERENCES].disabled = 0;
            perf_events[EV_ID_CACHE_MISSES].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("bus", argv[i]) == 0) {
            perf_events[EV_ID_BUS_CYCLES].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("stalled", argv[i]) == 0) {
            perf_events[EV_ID_STALLED_CYCLES_FRONTEND].disabled = 0;
            perf_events[EV_ID_STALLED_CYCLES_BACKEND].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("migrations", argv[i]) == 0) {
            perf_events[EV_ID_CPU_MIGRATIONS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("alignment", argv[i]) == 0) {
            perf_events[EV_ID_ALIGNMENT_FAULTS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("emulation", argv[i]) == 0) {
            perf_events[EV_ID_EMULATION_FAULTS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("L1D", argv[i]) == 0) {
            perf_events[EV_ID_L1D_READ_ACCESS].disabled = 0;
            perf_events[EV_ID_L1D_READ_MISS].disabled = 0;
            perf_events[EV_ID_L1D_WRITE_ACCESS].disabled = 0;
            perf_events[EV_ID_L1D_WRITE_MISS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("L1D-prefetch", argv[i]) == 0) {
            perf_events[EV_ID_L1D_PREFETCH_ACCESS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("L1I", argv[i]) == 0) {
            perf_events[EV_ID_L1I_READ_ACCESS].disabled = 0;
            perf_events[EV_ID_L1I_READ_MISS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("LL", argv[i]) == 0) {
            perf_events[EV_ID_LL_READ_ACCESS].disabled = 0;
            perf_events[EV_ID_LL_READ_MISS].disabled = 0;
            perf_events[EV_ID_LL_WRITE_ACCESS].disabled = 0;
            perf_events[EV_ID_LL_WRITE_MISS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("DTLB", argv[i]) == 0) {
            perf_events[EV_ID_DTLB_READ_ACCESS].disabled = 0;
            perf_events[EV_ID_DTLB_READ_MISS].disabled = 0;
            perf_events[EV_ID_DTLB_WRITE_ACCESS].disabled = 0;
            perf_events[EV_ID_DTLB_WRITE_MISS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("ITLB", argv[i]) == 0) {
            perf_events[EV_ID_ITLB_READ_ACCESS].disabled = 0;
            perf_events[EV_ID_ITLB_READ_MISS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("PBU", argv[i]) == 0) {
            perf_events[EV_ID_PBU_READ_ACCESS].disabled = 0;
            plugin_enabled = 1;
            continue;
        }
        else if(strcmp("debug", argv[i]) == 0) {
            debug = 1;
            continue;
        }
        else if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata perf.plugin %s\n"
                    " Copyright 2018-2025 Netdata Inc.\n"
                    " Released under GNU General Public License v3 or later.\n"
                    "\n"
                    " This program is a data collector plugin for netdata.\n"
                    "\n"
                    " Available command line options:\n"
                    "\n"
                    "  COLLECTION_FREQUENCY    data collection frequency in seconds\n"
                    "                          minimum: %d\n"
                    "\n"
                    "  all                     enable all charts\n"
                    "\n"
                    "  cycles                  enable CPU cycles chart\n"
                    "\n"
                    "  instructions            enable Instructions chart\n"
                    "\n"
                    "  branch                  enable Branch instructions chart\n"
                    "\n"
                    "  cache                   enable Cache operations chart\n"
                    "\n"
                    "  bus                     enable Bus cycles chart\n"
                    "\n"
                    "  stalled                 enable Stalled frontend and backend cycles chart\n"
                    "\n"
                    "  migrations              enable CPU migrations chart\n"
                    "\n"
                    "  alignment               enable Alignment faults chart\n"
                    "\n"
                    "  emulation               enable Emulation faults chart\n"
                    "\n"
                    "  L1D                     enable L1D cache operations chart\n"
                    "\n"
                    "  L1D-prefetch            enable L1D prefetch cache operations chart\n"
                    "\n"
                    "  L1I                     enable L1I cache operations chart\n"
                    "\n"
                    "  LL                      enable LL cache operations chart\n"
                    "\n"
                    "  DTLB                    enable DTLB cache operations chart\n"
                    "\n"
                    "  ITLB                    enable ITLB cache operations chart\n"
                    "\n"
                    "  PBU                     enable PBU cache operations chart\n"
                    "\n"
                    "  debug                   enable verbose output\n"
                    "                          default: disabled\n"
                    "\n"
                    "  -v\n"
                    "  -V\n"
                    "  --version               print version and exit\n"
                    "\n"
                    "  -h\n"
                    "  --help                  print this message and exit\n"
                    "\n"
                    " For more information:\n"
                    " https://github.com/netdata/netdata/tree/master/src/collectors/perf.plugin\n"
                    "\n"
                    , NETDATA_VERSION
                    , update_every
            );
            exit(1);
        }

        collector_error("ignoring parameter '%s'", argv[i]);
    }

    if(!plugin_enabled){
        collector_info("no charts enabled - nothing to do.");
        printf("DISABLE\n");
        exit(1);
    }
}

int main(int argc, char **argv) {
    nd_log_initialize_for_external_plugins("perf.plugin");
    netdata_threads_init_for_external_plugins(0);

    parse_command_line(argc, argv);

    errno_clear();

    if(freq >= update_every)
        update_every = freq;
    else if(freq)
        collector_error("update frequency %d seconds is too small for PERF. Using %d.", freq, update_every);

    if (unlikely(debug))
        fprintf(stderr, "perf.plugin: calling perf_init()\n");

    if (!perf_init()) {
        perf_free();
        collector_info("all perf counters are disabled");
        fprintf(stdout, "EXIT\n");
        fflush(stdout);
        exit(1);
    }

    // ------------------------------------------------------------------------
    // the main loop

    if(unlikely(debug)) fprintf(stderr, "perf.plugin: starting data collection\n");

    time_t started_t = now_monotonic_sec();

    size_t iteration;

    int perf = 1;

    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);
    for(iteration = 0; 1; iteration++) {
        usec_t dt = heartbeat_next(&hb);

        if (unlikely(netdata_exit))
            break;

        if (unlikely(debug && iteration))
            fprintf(stderr, "perf.plugin: iteration %zu, dt %" PRIu64 " usec\n", iteration, dt);

        if(likely(perf)) {
            if (unlikely(debug))
                fprintf(stderr, "perf.plugin: calling perf_collect()\n");

            perf = !perf_collect();

            if(likely(perf)) {
                if (unlikely(debug))
                    fprintf(stderr, "perf.plugin: calling perf_send_metrics()\n");

                perf_send_metrics();
            }
        }

        fflush(stdout);

        // restart check (14400 seconds)
        if (now_monotonic_sec() - started_t > 14400)
            break;
    }

    collector_info("process exiting");
    perf_free();
}
