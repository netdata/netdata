// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include "netdata_win_driver.h"

static const char *srv_name = "NetdataDriver";
const char *drv_path = "%SystemRoot%\\system32\\netdata_driver.sys";

struct cpu_data {
    RRDDIM *rd_cpu_temp;

    collected_number cpu_temp;
};

struct cpu_data *cpus = NULL;
size_t ncpus = 0 ;
static ND_THREAD *hardware_info_thread = NULL;

static void netdata_stop_driver()
{
    SC_HANDLE scm = OpenSCManager(NULL, NULL, SC_MANAGER_CONNECT);
    if (scm == NULL) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open Service Manager. Error= %lu \n", GetLastError());
        return;
    }

    SC_HANDLE service = OpenService(scm, srv_name, SERVICE_STOP | SERVICE_QUERY_STATUS);
    if (service == NULL) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open the service. Error= %lu \n", GetLastError());
        CloseServiceHandle(scm);
        return;
    }

    SERVICE_STATUS_PROCESS ss_status;
    if (ControlService(service, SERVICE_CONTROL_STOP, (LPSERVICE_STATUS)&ss_status) == 0) {
        if (GetLastError() != ERROR_SERVICE_NOT_ACTIVE) {
            nd_log(
                    NDLS_COLLECTORS,
                    NDLP_ERR,
                    "Cannot stop the service. Error= %lu \n", GetLastError());
        }
    }

    CloseServiceHandle(service);
    CloseServiceHandle(scm);
}

int netdata_install_driver()
{
    SC_HANDLE scm = OpenSCManager(
            NULL,
            NULL,
            SC_MANAGER_CREATE_SERVICE
    );

    if (unlikely(!scm)) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open Service Manager. Error= %lu \n", GetLastError());
        return -1;
    }

    // Create the service entry for the driver
    SC_HANDLE service = CreateService(
            scm,
            srv_name,
            srv_name,
            SERVICE_START | SERVICE_STOP | DELETE,
            SERVICE_KERNEL_DRIVER,
            SERVICE_DEMAND_START,
            SERVICE_ERROR_NORMAL,
            drv_path,
            NULL,
            NULL,
            NULL,
            NULL,
            NULL
    );

    if (unlikely(!service)) {
        if (GetLastError() == ERROR_SERVICE_EXISTS) {
            CloseServiceHandle(scm);
            return 0;
        }

        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot create Service. Error= %lu \n", GetLastError());
        CloseServiceHandle(scm);
        return -1;
    }

    CloseServiceHandle(service);
    CloseServiceHandle(scm);

    return 0;
}

int netdata_start_driver()
{
    SC_HANDLE scm = OpenSCManagerA(NULL, NULL, SC_MANAGER_CONNECT);
    if (unlikely(!scm)) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open Service Manager. Error= %lu \n", GetLastError());
        return -1;
    }

    SC_HANDLE service = OpenServiceA(scm, srv_name, SERVICE_START | SERVICE_QUERY_STATUS);
    if (unlikely(!service)) {
        CloseServiceHandle(scm);
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open Service. Error= %lu \n", GetLastError());
        return -1;
    }

    int ret = 0;
    if (!StartServiceA(service, 0, NULL)) {
        DWORD err = GetLastError();
        if (err != ERROR_SERVICE_EXISTS && err != ERROR_SERVICE_ALREADY_RUNNING) {
            nd_log(
                    NDLS_COLLECTORS,
                    NDLP_ERR,
                    "Cannot start Service. Error= %lu \n", err);
            ret = -1;
        }
    }

    CloseServiceHandle(service);
    CloseServiceHandle(scm);
    return ret;
}

static inline HANDLE netdata_open_device()
{
    HANDLE msr_h = CreateFileA(MSR_USER_PATH, GENERIC_READ | GENERIC_WRITE, 0,
                        NULL, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL);
    if (msr_h == INVALID_HANDLE_VALUE) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open device. GetLastError= %lu \n", GetLastError());
    }
    return msr_h;
}

void netdata_collect_cpu_chart()
{
    HANDLE device = netdata_open_device();
    if (device == INVALID_HANDLE_VALUE) {
        return;
    }

    const uint32_t MSR_THERM_STATUS = 0x19C;
    const ULONG TJMAX = 100;

    for (size_t cpu = 0; cpu < ncpus; cpu++) {
        DWORD bytes = 0;
        MSR_REQUEST req = { MSR_THERM_STATUS, (ULONG)cpu, 0, 0 };

        if (DeviceIoControl(device, IOCTL_MSR_READ, &req, sizeof(req), &req, sizeof(req), &bytes, NULL)) {
            ULONG digital_readout = (req.low >> 16) & 0x7F;  // bits [22:16]
            cpus[cpu].cpu_temp = (collected_number)(TJMAX - digital_readout);
        }
    }

    CloseHandle(device);
}

static void get_hardware_info_thread(void *ptr __maybe_unused)
{
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int update_every = UPDATE_EVERY_MIN;

    while (service_running(SERVICE_COLLECTORS)) {
        (void)heartbeat_next(&hb);

        netdata_collect_cpu_chart();
    }
}

static int initialize(int update_every)
{
    if (netdata_install_driver()) {
        return -1;
    }

    if (netdata_start_driver()) {
        return -1;
    }

    ncpus = os_get_system_cpus();
    cpus = callocz(ncpus, sizeof(struct cpu_data));

    hardware_info_thread = nd_thread_create("hi_threads", NETDATA_THREAD_OPTION_DEFAULT, get_hardware_info_thread, &update_every);

    return 0;
}

static RRDSET *netdata_publish_cpu_chart(int update_every)
{
    static RRDSET *st_cpu_temp = NULL;
    if (!st_cpu_temp) {
        st_cpu_temp = rrdset_create_localhost(
                "cpu",
                "temperature",
                NULL,
                "temperature",
                "cpu.temperature",
                "Core temperature",
                "Celcius",
                PLUGIN_WINDOWS_NAME,
                "GetHardwareInfo",
                NETDATA_CHART_PRIO_CPU_TEMPERATURE,
                update_every,
                RRDSET_TYPE_LINE);
    }

    return st_cpu_temp;
}

static void netdata_loop_cpu_chart(int update_every)
{
    RRDSET *chart = netdata_publish_cpu_chart(update_every);
    for (size_t i = 0; i < ncpus; i++) {
        struct cpu_data *lcpu = &cpus[i];
        if (!lcpu->rd_cpu_temp) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "cpu%lu.temp", i);
            lcpu->rd_cpu_temp = rrddim_add(chart, id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        rrddim_set_by_pointer(chart, lcpu->rd_cpu_temp, lcpu->cpu_temp);
    }
        rrdset_done(chart);
}

int do_GetHardwareInfo(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        initialized = true;
        if (initialize(update_every)) {
            return -1;
        }
    }

    netdata_loop_cpu_chart(update_every);

    return 0;
}

void do_GetHardwareInfo_cleanup()
{
    if (nd_thread_join(hardware_info_thread))
        nd_log_daemon(NDLP_ERR, "Failed to join mssql queries thread");

    netdata_stop_driver();
}
