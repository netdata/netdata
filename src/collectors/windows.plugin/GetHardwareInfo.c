// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include "netdata_win_driver.h"

static HANDLE msr_h = INVALID_HANDLE_VALUE;
static SC_HANDLE scm = NULL;
static SC_HANDLE service = NULL;

static void netdata_unload_driver()
{
    if (scm)
        CloseServiceHandle(scm);

    if (service)
        CloseServiceHandle(service);

    if (msr_h != INVALID_HANDLE_VALUE)
        CloseHandle(msr_h);
}

int netdata_load_driver()
{
    const char *srv_name = "NetdataDriver";
    const char* drv_path = "C:\\windows\\System32\\msys-netdata_driver.sys";

    scm = OpenSCManager(
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
    service = CreateService(
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

    return 0;
}

int netdata_open_device()
{
    msr_h = CreateFileA(MSR_USER_PATH, GENERIC_READ | GENERIC_WRITE, 0,
                        NULL, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL);
    if (msr_h == INVALID_HANDLE_VALUE) {
        nd_log(
                NDLS_COLLECTORS,
                NDLP_ERR,
                "Cannot open device. GetLastError= %lu \n", GetLastError());
        return -1;
    }
    return 0;
}

static int initialize(int update_every)
{
    if (netdata_open_device()) {
        return -1;
    }

    if (netdata_load_driver()) {
        return -1;
    }

    return 0;
}

int do_GetHardwareInfo(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        if (likely(initialize(update_every))) {
            return -1;
        }

        initialized = true;
    }

    return 0;
}
