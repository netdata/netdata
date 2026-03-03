// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#include "netdata_win_driver.h"

static const char *srv_name = "NetdataDriver";
static const char *drv_path = "%SystemRoot%\\system32\\netdata_driver.sys";

struct cpu_data {
    RRDDIM *rd_cpu_temp;

    collected_number cpu_temp;
    collected_number last_valid_temp;
    int read_errors;
};

struct cpu_data *cpus = NULL;
size_t ncpus = 0;
static ND_THREAD *hardware_info_thread = NULL;
static collected_number (*temperature_fcnt)(MSR_REQUEST *) = NULL;
static CRITICAL_SECTION cpus_lock;
bool cpus_lock_initialized = false;
static HANDLE msr_device = INVALID_HANDLE_VALUE;
static CRITICAL_SECTION device_lock;
bool device_lock_initialized = false;
static int consecutive_errors = 0;
static const int MAX_CONSECUTIVE_ERRORS = 5;
static const int IOCTL_RETRIES = 3;
static const int IOCTL_RETRY_DELAY_MS = 10;
#define INVALID_TEMP ((collected_number)(-1))

static void netdata_stop_driver()
{
    SC_HANDLE scm = OpenSCManager(NULL, NULL, SC_MANAGER_ALL_ACCESS);
    if (scm == NULL) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open Service Manager. Error= %lu \n", GetLastError());
        return;
    }

    SC_HANDLE service = OpenService(scm, srv_name, SERVICE_STOP | SERVICE_QUERY_STATUS | DELETE);
    if (service == NULL) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open the service. Error= %lu \n", GetLastError());
        CloseServiceHandle(scm);
        return;
    }

    SERVICE_STATUS ss_status = {};
    if (ControlService(service, SERVICE_CONTROL_STOP, &ss_status) == 0) {
        DWORD err = GetLastError();
        if (err != ERROR_SERVICE_NOT_ACTIVE) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot stop the service. Error= %lu \n", err);
        }
    }

    if (!DeleteService(service)) {
        DWORD err = GetLastError();
        if (err != ERROR_SERVICE_MARKED_FOR_DELETE) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot delete service. Error= %lu \n", err);
        }
    }

    CloseServiceHandle(service);
    CloseServiceHandle(scm);
}

int netdata_install_driver()
{
    SC_HANDLE scm = OpenSCManager(NULL, NULL, SC_MANAGER_CREATE_SERVICE);

    if (unlikely(!scm)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open Service Manager. Error= %lu \n", GetLastError());
        return -1;
    }

    char expanded_path[MAX_PATH];
    if (ExpandEnvironmentStringsA(drv_path, expanded_path, sizeof(expanded_path)) == 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot expand environment strings. Error= %lu \n", GetLastError());
        CloseServiceHandle(scm);
        return -1;
    }

    // Create the service entry for the driver
    SC_HANDLE service = CreateServiceA(
        scm,
        srv_name,
        srv_name,
        SERVICE_START | SERVICE_STOP | DELETE,
        SERVICE_KERNEL_DRIVER,
        SERVICE_DEMAND_START,
        SERVICE_ERROR_NORMAL,
        expanded_path,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL);

    if (unlikely(!service)) {
        if (GetLastError() == ERROR_SERVICE_EXISTS) {
            CloseServiceHandle(scm);
            return 0;
        }

        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot create Service. Error= %lu \n", GetLastError());
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
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open Service Manager. Error= %lu \n", GetLastError());
        return -1;
    }

    SC_HANDLE service = OpenServiceA(scm, srv_name, SERVICE_START | SERVICE_QUERY_STATUS);
    if (unlikely(!service)) {
        CloseServiceHandle(scm);
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open Service. Error= %lu \n", GetLastError());
        return -1;
    }

    int ret = 0;
    if (!StartServiceA(service, 0, NULL)) {
        DWORD err = GetLastError();
        if (err != ERROR_SERVICE_ALREADY_RUNNING) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot start Service. Error= %lu \n", err);
            ret = -1;
        }
    }

    CloseServiceHandle(service);
    CloseServiceHandle(scm);
    return ret;
}

static inline HANDLE netdata_open_device()
{
    HANDLE msr_h =
        CreateFileA(MSR_USER_PATH, GENERIC_READ | GENERIC_WRITE, 0, NULL, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL);
    if (msr_h == INVALID_HANDLE_VALUE) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open device. GetLastError= %lu \n", GetLastError());
    }
    return msr_h;
}

static bool netdata_reopen_device_if_needed()
{
    if (msr_device != INVALID_HANDLE_VALUE) {
        return true;
    }

    msr_device = netdata_open_device();
    return (msr_device != INVALID_HANDLE_VALUE);
}

static bool netdata_read_msr(MSR_REQUEST *req)
{
    if (!req || msr_device == INVALID_HANDLE_VALUE) {
        return false;
    }

    for (int retry = 0; retry < IOCTL_RETRIES; retry++) {
        DWORD bytes = 0;
        if (DeviceIoControl(msr_device, IOCTL_MSR_READ, req, sizeof(*req), req, sizeof(*req), &bytes, NULL)) {
            return true;
        }

        if (retry < IOCTL_RETRIES - 1) {
            Sleep(IOCTL_RETRY_DELAY_MS);
        }
    }

    return false;
}

static collected_number netdata_intel_cpu_temp(MSR_REQUEST *req)
{
    if (!req)
        return INVALID_TEMP;

    const ULONG TJMAX = 100;
    ULONG digital_readout = (req->low >> 16) & 0x7F; // bits [22:16]

    collected_number temp = (collected_number)(TJMAX - digital_readout);

    if (temp < 0 || temp > 150)
        return INVALID_TEMP;

    return temp;
}

static collected_number netdata_amd_cpu_temp(MSR_REQUEST *req)
{
    if (!req)
        return INVALID_TEMP;

    ULONG amd_temp = (req->low >> 21) & 0x7FF;
    collected_number temp = (collected_number)amd_temp / 8;

    if (temp < 0 || temp > 150)
        return INVALID_TEMP;

    return temp;
}

void netdata_collect_cpu_chart()
{
    if (!netdata_reopen_device_if_needed()) {
        consecutive_errors++;
        if (consecutive_errors >= MAX_CONSECUTIVE_ERRORS) {
            nd_log(
                NDLS_COLLECTORS, NDLP_ERR, "MSR device unavailable for %d consecutive attempts\n", consecutive_errors);
        }
        return;
    }

    consecutive_errors = 0;
    const uint32_t MSR_THERM_STATUS = 0x19C;

    EnterCriticalSection(&cpus_lock);
    for (size_t cpu = 0; cpu < ncpus; cpu++) {
        MSR_REQUEST req = {MSR_THERM_STATUS, (ULONG)cpu, 0, 0};

        if (netdata_read_msr(&req)) {
            collected_number temp = 0;
            if (temperature_fcnt) {
                temp = temperature_fcnt(&req);
            }

            if (temp != INVALID_TEMP) {
                cpus[cpu].last_valid_temp = temp;
                cpus[cpu].cpu_temp = temp;
                cpus[cpu].read_errors = 0;
            } else {
                cpus[cpu].cpu_temp = cpus[cpu].last_valid_temp;
                cpus[cpu].read_errors++;
            }
        } else {
            cpus[cpu].cpu_temp = cpus[cpu].last_valid_temp;
            cpus[cpu].read_errors++;
        }
    }
    LeaveCriticalSection(&cpus_lock);
}

static void get_hardware_info_thread(void *ptr __maybe_unused)
{
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    while (service_running(SERVICE_COLLECTORS)) {
        (void)heartbeat_next(&hb);

        netdata_collect_cpu_chart();
    }
}

static void netdata_detect_cpu()
{
    SYSTEM_INFO sysInfo;
    GetSystemInfo(&sysInfo);

    WORD test = sysInfo.wProcessorArchitecture;
    if (test != PROCESSOR_ARCHITECTURE_AMD64 && test != PROCESSOR_ARCHITECTURE_IA64) {
        return;
    }

    int cpuInfo[4];
    __cpuid(cpuInfo, 0);

    char vendorID[13];
    memcpy(vendorID, &cpuInfo[1], 4);
    memcpy(&vendorID[4], &cpuInfo[3], 4);
    memcpy(&vendorID[8], &cpuInfo[2], 4);
    vendorID[12] = '\0';

    if (!strcmp(vendorID, "GenuineIntel"))
        temperature_fcnt = netdata_intel_cpu_temp;
    else if (!strcmp(vendorID, "AuthenticAMD"))
        temperature_fcnt = netdata_amd_cpu_temp;
}

static int initialize()
{
    char expanded_path[MAX_PATH];
    if (ExpandEnvironmentStringsA(drv_path, expanded_path, sizeof(expanded_path)) == 0) {
        nd_log(
            NDLS_COLLECTORS, NDLP_ERR, "Cannot expand driver path environment strings. Error= %lu \n", GetLastError());
        return -1;
    }

    if (GetFileAttributesA(expanded_path) == INVALID_FILE_ATTRIBUTES) {
        nd_log(
            NDLS_COLLECTORS,
            NDLP_ERR,
            "Driver not found at '%s'. Please ensure the driver is properly installed.\n",
            expanded_path);
        return -1;
    }

    InitializeCriticalSection(&cpus_lock);
    cpus_lock_initialized = true;

    InitializeCriticalSection(&device_lock);
    device_lock_initialized = true;

    netdata_detect_cpu();
    if (!temperature_fcnt) {
        return -1;
    }

    if (netdata_install_driver()) {
        return -1;
    }

    if (netdata_start_driver()) {
        return -1;
    }

    ncpus = os_get_system_cpus();
    cpus = callocz(ncpus, sizeof(struct cpu_data));

    for (size_t i = 0; i < ncpus; i++) {
        cpus[i].cpu_temp = INVALID_TEMP;
        cpus[i].last_valid_temp = INVALID_TEMP;
        cpus[i].read_errors = 0;
    }

    hardware_info_thread =
        nd_thread_create("hw_info_thread", NETDATA_THREAD_OPTION_DEFAULT, get_hardware_info_thread, NULL);

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
            "Celsius",
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

    EnterCriticalSection(&cpus_lock);
    for (int i = 0; i < (int)ncpus; i++) {
        struct cpu_data *lcpu = &cpus[i];
        if (!lcpu->rd_cpu_temp) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "cpu%d.temp", i);
            lcpu->rd_cpu_temp = rrddim_add(chart, id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        if (lcpu->cpu_temp != INVALID_TEMP) {
            rrddim_set_by_pointer(chart, lcpu->rd_cpu_temp, lcpu->cpu_temp);
        } else {
            rrddim_set_by_pointer(chart, lcpu->rd_cpu_temp, 0);
        }
    }
    LeaveCriticalSection(&cpus_lock);

    rrdset_done(chart);
}

int do_GetHardwareInfo(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;
    if (unlikely(!initialized)) {
        initialized = true;
        if (initialize()) {
            return -1;
        }
    }

    netdata_loop_cpu_chart(update_every);

    return 0;
}

void do_GetHardwareInfo_cleanup()
{
    if (hardware_info_thread) {
        if (nd_thread_join(hardware_info_thread))
            nd_log_daemon(NDLP_ERR, "Failed to join Get Hardware Info thread");
    }

    if (msr_device != INVALID_HANDLE_VALUE) {
        CloseHandle(msr_device);
        msr_device = INVALID_HANDLE_VALUE;
    }

    netdata_stop_driver();

    if (cpus_lock_initialized)
        DeleteCriticalSection(&cpus_lock);

    if (device_lock_initialized)
        DeleteCriticalSection(&device_lock);

    if (cpus) {
        freez(cpus);
        cpus = NULL;
        ncpus = 0;
    }
}
