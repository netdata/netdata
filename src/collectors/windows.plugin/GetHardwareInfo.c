// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include "netdata_win_driver.h"

static const char *srv_name = "NetdataDriver";

struct cpu_data {
    RRDSET *st_cpu_temp;
    RRDDIM *rd_cpu_temp;

    collected_number cpu_temp;
};

struct cpu_data *cpus;
size_t ncpus;

static void netdata_unload_driver()
{
    SC_HANDLE scm = OpenSCManager(NULL, NULL, SC_MANAGER_ALL_ACCESS);
    if (scm == NULL) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open Service Manager. GetLastError= %lu \n", GetLastError());
        return;
    }

    SC_HANDLE service = OpenService(scm, srv_name, SERVICE_STOP | SERVICE_QUERY_STATUS | DELETE);
    if (service == NULL) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open the service. GetLastError= %lu \n", GetLastError());
        CloseServiceHandle(scm);
        return;
    }

    SERVICE_STATUS_PROCESS ss_status;
    if (ControlService(service, SERVICE_CONTROL_STOP, (LPSERVICE_STATUS)&ss_status) == 0) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot stop the service. GetLastError= %lu \n", GetLastError());
    }

    if (!DeleteService(service)) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot delete the service. GetLastError= %lu \n", GetLastError());
    }

    CloseServiceHandle(service);
    CloseServiceHandle(scm);
}

static void netdata_unload_hardware()
{
    netdata_unload_driver();
}

int netdata_load_driver()
{
    const char *drv_path = "C:\\Windows\\System32\\netdata_driver.sys";
    SC_HANDLE scm = OpenSCManager(
            NULL,
            NULL,
            SC_MANAGER_ALL_ACCESS
    );

    if (unlikely(!scm)) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open Service Manager. GetLastError= %lu \n", GetLastError());
        return -1;
    }

    // Create the service entry for the driver
    SC_HANDLE service = CreateService(
            scm,
            srv_name,
            srv_name,
            SERVICE_ALL_ACCESS,
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
            service = OpenService(scm, srv_name, SERVICE_ALL_ACCESS);
            if (unlikely(!service)) {
                CloseServiceHandle(scm);
                nd_log(
                        NDLS_COLLECTORS,
                        NDLP_ERR,
                        "Cannot open Service. GetLastError= %lu \n", GetLastError());
                return -1;
            }
        } else {
            CloseServiceHandle(scm);
            nd_log(
                    NDLS_COLLECTORS,
                    NDLP_ERR,
                    "Cannot create Service. GetLastError= %lu \n", GetLastError());
            CloseServiceHandle(scm);
            return -1;
        }
    }

    if (!StartService(service, 0, NULL)) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot start Service. GetLastError= %lu \n", GetLastError());
        DeleteService(service);
        CloseServiceHandle(service);
        CloseServiceHandle(scm);
        return -1;
    }

    CloseServiceHandle(service);
    CloseServiceHandle(scm);

    return 0;
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

static int initialize(int update_every)
{
    if (netdata_load_driver()) {
        return -1;
    }

    ncpus = os_get_system_cpus();
    cpus = callocz(ncpus, sizeof(struct cpu_data));

    return 0;
}

void netdata_collect_sensor_chart(int update_every)
{
    HANDLE device = netdata_open_device();
    if (device == INVALID_HANDLE_VALUE) {
        return;
    }

    const uint32_t MSR_THERM_STATUS = 0x19C;
    const int TJMAX = 100;
    DWORD bytes;

    for (size_t cpu = 0; cpu < ncpus; cpu++) {
        MSR_REQUEST req = { MSR_THERM_STATUS, (uint32_t)cpu, 0, 0 };

        if (DeviceIoControl(device, IOCTL_MSR_READ, &req, sizeof(req), &req, sizeof(req), &bytes, NULL)) {
            int digital_readout = (req.low >> 16) & 0x7F;  // bits [22:16]
            cpus[cpu].cpu_temp = (collected_number)(TJMAX - digital_readout);
        }
    }

    CloseHandle(device);
}

int do_GetHardwareInfo(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        if (initialize(update_every)) {
            return -1;
        }

        initialized = true;
    }

    netdata_collect_sensor_chart(update_every);

    return 0;
}

void do_GetHardwareInfo_cleanup()
{
    netdata_unload_hardware();
}
