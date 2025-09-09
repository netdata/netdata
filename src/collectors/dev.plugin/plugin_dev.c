// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_dev.h"

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

    heartbeat_t hb;
    heartbeat_init(&hb, localhost->rrd_update_every * USEC_PER_SEC);

    while(service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb);

        if(unlikely(!service_running(SERVICE_COLLECTORS)))
            break;
    }
}

