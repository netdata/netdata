// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_dev.h"

#include "libnetdata/required_dummies.h"

#define _COMMON_PLUGIN_NAME "dev.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "dev"
#include "../common-contexts/common-contexts.h"

int update_every = 1;

#define NETDATA_MSR_THERM_STATUS 0x19C
#define NETDATA_MSR_TEMPERATURE_TARGET 0x1A2

struct dev_cpu_temp {
    char *dimension;
} *local_cpus;

static uint64_t netdata_dev_read_msr(int cpu, unsigned int reg) {
    char msr_file_name[FILENAME_MAX + 1];
    int fd;
    uint64_t data;

    sprintf(msr_file_name, "%s/dev/cpu/%d/msr", netdata_configured_host_prefix, cpu);
    fd = open(msr_file_name, O_RDONLY);
    if (fd < 0) {
        return 0;
    }

    if (pread(fd, &data, sizeof(data), reg) != sizeof(data)) {
        close(fd);
        return 0;
    }

    close(fd);
    return data;
}

static collected_number netadata_read_cpu_temp(int cpu)
{
    uint64_t therm_status = netdata_dev_read_msr(cpu, NETDATA_MSR_THERM_STATUS);
    uint64_t temp_target  = netdata_dev_read_msr(cpu, NETDATA_MSR_TEMPERATURE_TARGET);

    collected_number tjmax = (temp_target >> 16) & 0xFF; // TJMax value
    collected_number temp_offset = (therm_status >> 16) & 0x7F; // delta from TJMax
    return (tjmax - temp_offset);
}

static bool is_msr_enabled() {
    return (netdata_dev_read_msr(0, NETDATA_MSR_THERM_STATUS));
}

static void dev_parse_args(int argc, char **argv) {
    int i, freq = 0;

    for (i = 1; i < argc; i++) {
        if (!freq) {
            int n = (int) str2l(argv[i]);
            if (n > 0) {
                freq = n;
                continue;
            }
        }
    }

    if(freq > 0) update_every = freq;
}

static inline void dev_cpu_chart(int number_of_cpus)
{
    printf(
            "CHART cpu.temperature '' 'Core temperature' 'Celsius' 'temperature' 'cpu.temperature' 'line' %d %d '' '%s' 'dev.cpu.temperature'\n",
            NETDATA_CHART_PRIO_CPU_TEMPERATURE,
            update_every,
            _COMMON_PLUGIN_NAME);

    for (int i = 0; i < number_of_cpus; i++) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "cpu%d.temp", i);
        local_cpus[i].dimension = strdupz(id);
        printf("DIMENSION %s '' absolute 1 1\n", id);
    }
    fflush(stdout);
}

int main(int argc, char **argv)
{
    nd_log_initialize_for_external_plugins(PLUGIN_DEV_NAME);
    netdata_threads_init_for_external_plugins(0);

    if (!is_msr_enabled())
        return 1;

    dev_parse_args(argc, argv);

    int number_of_cpus = (int)os_get_system_cpus();
    local_cpus = callocz(sizeof(struct dev_cpu_temp), number_of_cpus);

    heartbeat_t hb;
    int local_update_every = update_every * USEC_PER_SEC;
    heartbeat_init(&hb, (usec_t )local_update_every);

    dev_cpu_chart(number_of_cpus);
    for (; !dev_plugin_stop();) {
        (void)heartbeat_next(&hb);

        printf("BEGIN cpu.temperature\n");
        for (int i = 0; i < number_of_cpus; i++) {
            collected_number temperature = netadata_read_cpu_temp(i);
            printf("SET %s %lld\n", local_cpus[i].dimension, temperature);
        }
        printf("END\n");
        fflush(stdout);
    }

    return 0;
}
