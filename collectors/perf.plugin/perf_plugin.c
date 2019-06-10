// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata/libnetdata.h"

#include <linux/perf_event.h>

#define PLUGIN_PERF_NAME "perf.plugin"

// Hardware counters
#define NETDATA_CHART_PRIO_PERF_CPU_CYCLES            8800
#define NETDATA_CHART_PRIO_PERF_INSTRUCTIONS          8801
#define NETDATA_CHART_PRIO_PERF_BRANCH_INSTRUSTIONS   8802
#define NETDATA_CHART_PRIO_PERF_CACHE                 8803
#define NETDATA_CHART_PRIO_PERF_BUS_CYCLES            8804
#define NETDATA_CHART_PRIO_PERF_FRONT_BACK_CYCLES     8805

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

// callback required by fatal()
void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

void send_statistics( const char *action, const char *action_result, const char *action_data) {
    (void) action;
    (void) action_result;
    (void) action_data;
    return;
}

// callbacks required by popen()
void signals_block(void) {};
void signals_unblock(void) {};
void signals_reset(void) {};

// callback required by eval()
int health_variable_lookup(const char *variable, uint32_t hash, struct rrdcalc *rc, calculated_number *result) {
    (void)variable;
    (void)hash;
    (void)rc;
    (void)result;
    return 0;
};

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

// Variables

#define RRD_TYPE_PERF "perf"
#define RRD_FAMILY_HW "hardware"
#define RRD_FAMILY_SW "software"
#define RRD_FAMILY_CACHE "cache"

#define NO_FD -1
#define ALL_PIDS -1
#define UINT64_SIZE 8

static int debug = 0;

static int update_every = 1;

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
    EV_GROUP_0,
    EV_GROUP_1,
    EV_GROUP_2,
    EV_GROUP_3,
    EV_GROUP_4,
    EV_GROUP_5,

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
} perf_events[] = {
    // Hardware counters
    {EV_ID_CPU_CYCLES,              PERF_TYPE_HARDWARE, PERF_COUNT_HW_CPU_CYCLES,              &group_leader_fds[EV_GROUP_0], NULL, 0, 0, 0},
    {EV_ID_INSTRUCTIONS,            PERF_TYPE_HARDWARE, PERF_COUNT_HW_INSTRUCTIONS,            &group_leader_fds[EV_GROUP_1], NULL, 0, 0, 0},
    {EV_ID_CACHE_REFERENCES,        PERF_TYPE_HARDWARE, PERF_COUNT_HW_CACHE_REFERENCES,        &group_leader_fds[EV_GROUP_1], NULL, 0, 0, 0},
    {EV_ID_CACHE_MISSES,            PERF_TYPE_HARDWARE, PERF_COUNT_HW_CACHE_MISSES,            &group_leader_fds[EV_GROUP_1], NULL, 0, 0, 0},
    {EV_ID_BRANCH_INSTRUCTIONS,     PERF_TYPE_HARDWARE, PERF_COUNT_HW_BRANCH_INSTRUCTIONS,     &group_leader_fds[EV_GROUP_1], NULL, 0, 0, 0},
    {EV_ID_BRANCH_MISSES,           PERF_TYPE_HARDWARE, PERF_COUNT_HW_BRANCH_MISSES,           &group_leader_fds[EV_GROUP_1], NULL, 0, 0, 0},
    {EV_ID_BUS_CYCLES,              PERF_TYPE_HARDWARE, PERF_COUNT_HW_BUS_CYCLES,              &group_leader_fds[EV_GROUP_0], NULL, 0, 0, 0},
    {EV_ID_STALLED_CYCLES_FRONTEND, PERF_TYPE_HARDWARE, PERF_COUNT_HW_STALLED_CYCLES_FRONTEND, &group_leader_fds[EV_GROUP_0], NULL, 0, 0, 0},
    {EV_ID_STALLED_CYCLES_BACKEND,  PERF_TYPE_HARDWARE, PERF_COUNT_HW_STALLED_CYCLES_BACKEND,  &group_leader_fds[EV_GROUP_0], NULL, 0, 0, 0},
    {EV_ID_REF_CPU_CYCLES,          PERF_TYPE_HARDWARE, PERF_COUNT_HW_REF_CPU_CYCLES,          &group_leader_fds[EV_GROUP_0], NULL, 0, 0, 0},

    // Software counters
    // {EV_ID_CPU_CLOCK,        PERF_TYPE_SOFTWARE, PERF_COUNT_SW_CPU_CLOCK,        &group_leader_fds[EV_GROUP_2], NULL, 0, 0, 0},
    // {EV_ID_TASK_CLOCK,       PERF_TYPE_SOFTWARE, PERF_COUNT_SW_TASK_CLOCK,       &group_leader_fds[EV_GROUP_2], NULL, 0, 0, 0},
    // {EV_ID_PAGE_FAULTS,      PERF_TYPE_SOFTWARE, PERF_COUNT_SW_PAGE_FAULTS,      &group_leader_fds[EV_GROUP_2], NULL, 0, 0, 0},
    // {EV_ID_CONTEXT_SWITCHES, PERF_TYPE_SOFTWARE, PERF_COUNT_SW_CONTEXT_SWITCHES, &group_leader_fds[EV_GROUP_2], NULL, 0, 0, 0},
    {EV_ID_CPU_MIGRATIONS,   PERF_TYPE_SOFTWARE, PERF_COUNT_SW_CPU_MIGRATIONS,   &group_leader_fds[EV_GROUP_2], NULL, 0, 0, 0},
    // {EV_ID_PAGE_FAULTS_MIN,  PERF_TYPE_SOFTWARE, PERF_COUNT_SW_PAGE_FAULTS_MIN,  &group_leader_fds[EV_GROUP_2], NULL, 0, 0, 0},
    // {EV_ID_PAGE_FAULTS_MAJ,  PERF_TYPE_SOFTWARE, PERF_COUNT_SW_PAGE_FAULTS_MAJ,  &group_leader_fds[EV_GROUP_2], NULL, 0, 0, 0},
    {EV_ID_ALIGNMENT_FAULTS, PERF_TYPE_SOFTWARE, PERF_COUNT_SW_ALIGNMENT_FAULTS, &group_leader_fds[EV_GROUP_2], NULL, 0, 0, 0},
    {EV_ID_EMULATION_FAULTS, PERF_TYPE_SOFTWARE, PERF_COUNT_SW_EMULATION_FAULTS, &group_leader_fds[EV_GROUP_2], NULL, 0, 0, 0},

    // Hardware cache counters
    {EV_ID_L1D_READ_ACCESS,     PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_L1D)  | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_3], NULL, 0, 0, 0},
    {EV_ID_L1D_READ_MISS,       PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_L1D)  | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS   << 16), &group_leader_fds[EV_GROUP_3], NULL, 0, 0, 0},
    {EV_ID_L1D_WRITE_ACCESS,    PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_L1D)  | (PERF_COUNT_HW_CACHE_OP_WRITE    << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_3], NULL, 0, 0, 0},
    {EV_ID_L1D_WRITE_MISS,      PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_L1D)  | (PERF_COUNT_HW_CACHE_OP_WRITE    << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS   << 16), &group_leader_fds[EV_GROUP_3], NULL, 0, 0, 0},
    {EV_ID_L1D_PREFETCH_ACCESS, PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_L1D)  | (PERF_COUNT_HW_CACHE_OP_PREFETCH << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_3], NULL, 0, 0, 0},

    {EV_ID_L1I_READ_ACCESS,     PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_L1I)  | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_4], NULL, 0, 0, 0},
    {EV_ID_L1I_READ_MISS,       PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_L1I)  | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS   << 16), &group_leader_fds[EV_GROUP_4], NULL, 0, 0, 0},

    {EV_ID_LL_READ_ACCESS,      PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_LL)   | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_4], NULL, 0, 0, 0},
    {EV_ID_LL_READ_MISS,        PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_LL)   | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS   << 16), &group_leader_fds[EV_GROUP_4], NULL, 0, 0, 0},
    {EV_ID_LL_WRITE_ACCESS,     PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_LL)   | (PERF_COUNT_HW_CACHE_OP_WRITE    << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_4], NULL, 0, 0, 0},
    {EV_ID_LL_WRITE_MISS,       PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_LL)   | (PERF_COUNT_HW_CACHE_OP_WRITE    << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS   << 16), &group_leader_fds[EV_GROUP_4], NULL, 0, 0, 0},

    {EV_ID_DTLB_READ_ACCESS,    PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_DTLB) | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_4], NULL, 0, 0, 0},
    {EV_ID_DTLB_READ_MISS,      PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_DTLB) | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS   << 16), &group_leader_fds[EV_GROUP_4], NULL, 0, 0, 0},
    {EV_ID_DTLB_WRITE_ACCESS,   PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_DTLB) | (PERF_COUNT_HW_CACHE_OP_WRITE    << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_4], NULL, 0, 0, 0},
    {EV_ID_DTLB_WRITE_MISS,     PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_DTLB) | (PERF_COUNT_HW_CACHE_OP_WRITE    << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS   << 16), &group_leader_fds[EV_GROUP_5], NULL, 0, 0, 0},

    {EV_ID_ITLB_READ_ACCESS,    PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_ITLB) | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_5], NULL, 0, 0, 0},
    {EV_ID_ITLB_READ_MISS,      PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_ITLB) | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_MISS   << 16), &group_leader_fds[EV_GROUP_5], NULL, 0, 0, 0},

    {EV_ID_PBU_READ_ACCESS,     PERF_TYPE_HW_CACHE, (PERF_COUNT_HW_CACHE_BPU)  | (PERF_COUNT_HW_CACHE_OP_READ     << 8) | (PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16), &group_leader_fds[EV_GROUP_5], NULL, 0, 0, 0},

    {EV_ID_END, 0, 0, NULL, NULL, 0, 0, 0}
};

static int perf_init() {
    int cpu;
    struct perf_event_attr perf_event_attr;
    struct perf_event *current_event = NULL;
    unsigned long flags = 0;

    number_of_cpus = (int)get_system_cpus();

    // initialize all perf event file descriptors
    for(current_event = &perf_events[0]; current_event->id != EV_ID_END; current_event++) {
        current_event->fd = mallocz(number_of_cpus * sizeof(int));
        memset(current_event->fd, NO_FD, number_of_cpus * sizeof(int));

        *current_event->group_leader_fd = mallocz(number_of_cpus * sizeof(int));
        memset(*current_event->group_leader_fd, NO_FD, number_of_cpus * sizeof(int));
    }

    memset(&perf_event_attr, 0, sizeof(perf_event_attr));

    for(cpu = 0; cpu < number_of_cpus; cpu++) {
        for(current_event = &perf_events[0]; current_event->id != EV_ID_END; current_event++) {
            if(unlikely(current_event->disabled)) continue;

            perf_event_attr.type = current_event->type;
            perf_event_attr.config = current_event->config;

            int fd, group_leader_fd = *(*current_event->group_leader_fd + cpu);

            fd = syscall(
                __NR_perf_event_open,
                &perf_event_attr,
                ALL_PIDS,
                cpu,
                group_leader_fd,
                flags
            );

            if(group_leader_fd == NO_FD) group_leader_fd = fd;

            if(fd < 0) {
                switch errno {
                    case EACCES:
                        error("PERF: Cannot access to the PMU: Permission denied");
                        break;
                    case EBUSY:
                        error("PERF: Another event already has exclusive access to the PMU");
                        break;
                    default:
                        error("PERF: Cannot open perf event");
                }
                error("PERF: Disabling event %u", current_event->id);
                current_event->disabled = 1;
            }

            *(current_event->fd + cpu) = fd;
            *(*current_event->group_leader_fd + cpu) = group_leader_fd;

            if(unlikely(debug)) fprintf(stderr, "perf.plugin: event id = %u, cpu = %d, fd = %d, leader_fd = %d\n", current_event->id, cpu, fd, group_leader_fd);
        }
    }

    return 0;
}

static void perf_free(void) {
    struct perf_event *current_event = NULL;

    for(current_event = &perf_events[0]; current_event->id != EV_ID_END; current_event++) {
        free(current_event->fd);
        free(*current_event->group_leader_fd);
    }
}

static int perf_collect() {
    int cpu;
    struct perf_event *current_event = NULL;
    uint64_t value;

    for(current_event = &perf_events[0]; current_event->id != EV_ID_END; current_event++) {
        current_event->updated = 0;
        current_event->value = 0;

        if(unlikely(current_event->disabled)) continue;

        for(cpu = 0; cpu < number_of_cpus; cpu++) {

            ssize_t read_size = read(current_event->fd[cpu], &value, UINT64_SIZE);

            if(likely(read_size == UINT64_SIZE)) {
                current_event->value += value;
                current_event->updated = 1;
            }
            else {
                error("Cannot update value for event %u", current_event->id);
                return 1;
            }
        }

        if(unlikely(debug)) fprintf(stderr, "perf.plugin: successfully read event id = %u, value = %lu\n", current_event->id, current_event->value);
    }

    return 0;
}

static void perf_send_metrics() {
    static int cpu_cycles_chart_generated = 0, instructions_chart_generated = 0, branch_chart_generated = 0,
               cache_chart_generated = 0, bus_cycles_chart_generated = 0, front_back_cycles_chart_generated = 0;
    static int migrations_chart_generated = 0, alighnment_chart_generated = 0, emulation_chart_generated = 0;
    static int L1D_chart_generated = 0, L1D_prefetch_chart_generated = 0, L1I_chart_generated = 0, LL_chart_generated = 0,
               DTLB_chart_generated = 0, ITLB_chart_generated = 0, PBU_chart_generated = 0;

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_CPU_CYCLES].updated || perf_events[EV_ID_REF_CPU_CYCLES].updated)) {
        if(unlikely(!cpu_cycles_chart_generated)) {
            cpu_cycles_chart_generated = 1;

            printf("CHART %s.%s '' 'CPU cycles' 'cycles/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "cpu_cycles"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_CPU_CYCLES
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "cpu");
            printf("DIMENSION %s '' incremental 1 1\n", "ref_cpu");
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

            printf("CHART %s.%s '' 'Instructions' 'instructions/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "instructions"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_INSTRUCTIONS
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "instructions");
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

    if(likely(perf_events[EV_ID_BRANCH_INSTRUCTIONS].updated || perf_events[EV_ID_BRANCH_MISSES].updated)) {
        if(unlikely(!branch_chart_generated)) {
            branch_chart_generated = 1;

            printf("CHART %s.%s '' 'Branch instructions' 'instructions/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "branch_instructions"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_BRANCH_INSTRUSTIONS
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "instructions");
            printf("DIMENSION %s '' incremental 1 1\n", "misses");
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

            printf("CHART %s.%s '' 'Cache operations' 'operations/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "cache"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_CACHE
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "references");
            printf("DIMENSION %s '' incremental 1 1\n", "misses");
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

            printf("CHART %s.%s '' 'Bus cycles' 'cycles/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "bus_cycles"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_BUS_CYCLES
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "bus");
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
        if(unlikely(!front_back_cycles_chart_generated)) {
            front_back_cycles_chart_generated = 1;

            printf("CHART %s.%s '' 'Stalled frontend and backebd cycles' 'cycles/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "front_back_cycles"
                   , RRD_FAMILY_HW
                   , NETDATA_CHART_PRIO_PERF_FRONT_BACK_CYCLES
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "stalled_frontend");
            printf("DIMENSION %s '' incremental 1 1\n", "stalled_backend");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "front_back_cycles"
        );
        if(likely(perf_events[EV_ID_STALLED_CYCLES_FRONTEND].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "stalled_frontend"
                   , (collected_number) perf_events[EV_ID_STALLED_CYCLES_FRONTEND].value
            );
        }
        if(likely(perf_events[EV_ID_STALLED_CYCLES_BACKEND].updated)) {
            printf(
                   "SET %s = %lld\n"
                   , "stalled_backend"
                   , (collected_number) perf_events[EV_ID_STALLED_CYCLES_BACKEND].value
            );
        }
        printf("END\n");
    }

    // ------------------------------------------------------------------------

    if(likely(perf_events[EV_ID_CPU_MIGRATIONS].updated)) {
        if(unlikely(!migrations_chart_generated)) {
            migrations_chart_generated = 1;

            printf("CHART %s.%s '' 'CPU migrations' 'migrations' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "migrations"
                   , RRD_FAMILY_SW
                   , NETDATA_CHART_PRIO_PERF_MIGRATIONS
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "migrations");
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
        if(unlikely(!alighnment_chart_generated)) {
            alighnment_chart_generated = 1;

            printf("CHART %s.%s '' 'Alighnment faults' 'faults' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "alighnment_faults"
                   , RRD_FAMILY_SW
                   , NETDATA_CHART_PRIO_PERF_ALIGNMENT
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "faults");
        }

        printf(
               "BEGIN %s.%s\n"
               , RRD_TYPE_PERF
               , "alighnment_faults"
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

            printf("CHART %s.%s '' 'Emulation faults' 'faults' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "emulation_faults"
                   , RRD_FAMILY_SW
                   , NETDATA_CHART_PRIO_PERF_EMULATION
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "faults");
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

            printf("CHART %s.%s '' 'L1D cache operations' 'events/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "l1d_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_L1D
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "read_access");
            printf("DIMENSION %s '' incremental 1 1\n", "read_misses");
            printf("DIMENSION %s '' incremental -1 1\n", "write_access");
            printf("DIMENSION %s '' incremental -1 1\n", "write_misses");
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

            printf("CHART %s.%s '' 'L1D prefetch cache operations' 'prefetches/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "l1d_cache_prefetch"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_L1D_PREFETCH
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "prefetches");
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

            printf("CHART %s.%s '' 'L1I cache operations' 'events/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "l1i_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_L1I
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "read_access");
            printf("DIMENSION %s '' incremental 1 1\n", "read_misses");
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

            printf("CHART %s.%s '' 'LL cache operations' 'events/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "ll_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_LL
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "read_access");
            printf("DIMENSION %s '' incremental 1 1\n", "read_misses");
            printf("DIMENSION %s '' incremental -1 1\n", "write_access");
            printf("DIMENSION %s '' incremental -1 1\n", "write_misses");
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

            printf("CHART %s.%s '' 'DTLB cache operations' 'events/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "dtlb_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_DTLB
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "read_access");
            printf("DIMENSION %s '' incremental 1 1\n", "read_misses");
            printf("DIMENSION %s '' incremental -1 1\n", "write_access");
            printf("DIMENSION %s '' incremental -1 1\n", "write_misses");
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

            printf("CHART %s.%s '' 'ITLB cache operations' 'events/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "itlb_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_ITLB
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "read_access");
            printf("DIMENSION %s '' incremental 1 1\n", "read_misses");
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

            printf("CHART %s.%s '' 'PBU cache operations' 'events/s' %s '' line %d %d %s\n"
                   , RRD_TYPE_PERF
                   , "pbu_cache"
                   , RRD_FAMILY_CACHE
                   , NETDATA_CHART_PRIO_PERF_PBU
                   , update_every
                   , PLUGIN_PERF_NAME
            );
            printf("DIMENSION %s '' incremental 1 1\n", "read_access");
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

int main(int argc, char **argv) {

    // ------------------------------------------------------------------------
    // initialization of netdata plugin

    program_name = "perf.plugin";

    // disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    // ------------------------------------------------------------------------
    // parse command line parameters

    int i, freq = 0;
    for(i = 1; i < argc ; i++) {
        if(isdigit(*argv[i]) && !freq) {
            int n = str2i(argv[i]);
            if(n > 0 && n < 86400) {
                freq = n;
                continue;
            }
        }
        else if(strcmp("version", argv[i]) == 0 || strcmp("-version", argv[i]) == 0 || strcmp("--version", argv[i]) == 0 || strcmp("-v", argv[i]) == 0 || strcmp("-V", argv[i]) == 0) {
            printf("perf.plugin %s\n", VERSION);
            exit(0);
        }
        else if(strcmp("debug", argv[i]) == 0) {
            debug = 1;
            continue;
        }
        else if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata perf.plugin %s\n"
                    " Copyright (C) 2019 Netdata Inc.\n"
                    " Released under GNU General Public License v3 or later.\n"
                    " All rights reserved.\n"
                    "\n"
                    " This program is a data collector plugin for netdata.\n"
                    "\n"
                    " Available command line options:\n"
                    "\n"
                    "  COLLECTION_FREQUENCY    data collection frequency in seconds\n"
                    "                          minimum: %d\n"
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
                    " https://github.com/netdata/netdata/tree/master/collectors/perf.plugin\n"
                    "\n"
                    , VERSION
                    , update_every
            );
            exit(1);
        }

        error("perf.plugin: ignoring parameter '%s'", argv[i]);
    }

    errno = 0;

    if(freq >= update_every)
        update_every = freq;
    else if(freq)
        error("update frequency %d seconds is too small for PERF. Using %d.", freq, update_every);

    if(debug) fprintf(stderr, "perf.plugin: calling perf_init()\n");
    int perf = !perf_init();

    // ------------------------------------------------------------------------
    // the main loop

    if(debug) fprintf(stderr, "perf.plugin: starting data collection\n");

    time_t started_t = now_monotonic_sec();

    size_t iteration;
    usec_t step = update_every * USEC_PER_SEC;

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1; iteration++) {
        usec_t dt = heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        if(debug && iteration)
            fprintf(stderr, "perf.plugin: iteration %zu, dt %llu usec\n"
                    , iteration
                    , dt
            );

        if(likely(perf)) {
            if(debug) fprintf(stderr, "perf.plugin: calling perf_collect()\n");
            perf = !perf_collect();

            if(likely(perf)) {
                if(debug) fprintf(stderr, "perf.plugin: calling perf_send_metrics()\n");
                perf_send_metrics();
            }
        }

        fflush(stdout);

        // restart check (14400 seconds)
        if(now_monotonic_sec() - started_t > 14400) break;
    }

    info("PERF process exiting");
    perf_free();
}
