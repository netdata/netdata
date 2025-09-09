// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_dev.h"

#define _COMMON_PLUGIN_NAME "dev.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "dev"
#include "../common-contexts/common-contexts.h"

#define NETDATA_MSR_THERM_STATUS 0x19C
#define NETDATA_MSR_TEMPERATURE_TARGET 0x1A2

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

static void dev_main_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    worker_unregister();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

static bool is_msr_enabled() {
    return (netdata_dev_read_msr(0, NETDATA_MSR_THERM_STATUS));
}

void dev_main(void *ptr)
{
    CLEANUP_FUNCTION_REGISTER(dev_main_cleanup) cleanup_ptr = ptr;

    if (!is_msr_enabled())
        return;

    worker_register("DEV");

    rrd_collector_started();
    int number_of_cpus = (int)os_get_system_cpus();
    RRDDIM **rd_pcpu_temperature = callocz(sizeof(RRDDIM *), number_of_cpus);

    heartbeat_t hb;
    int update_every = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_init(&hb, (usec_t )update_every);

    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        (void)heartbeat_next(&hb);

        if(unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        RRDSET *st = common_cpu_temperature(update_every) ;
        for (int i = 0; i < number_of_cpus; i++) {
            if (unlikely(!rd_pcpu_temperature[i])) {
                char char_rd[64];
                sprintf(char_rd, "cpu%d.temp", i);
                rd_pcpu_temperature[i] = rrddim_add(st, char_rd, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            collected_number temperature = netadata_read_cpu_temp(i);
            rrddim_set_by_pointer(st, rd_pcpu_temperature[i], temperature);
        }
        rrdset_done(st);
    }
}
