// SPDX-License-Identifier: GPL-3.0-or-later

#include "win_system-info.h"

#ifdef OS_WINDOWS

// Hardware
static char *netdata_windows_arch(DWORD value)
{
    switch (value) {
        case 9:
            return "x86_64";
        case 5:
            return "ARM";
        case 12:
            return "ARM64";
        case 6:
            return "Intel Intaniun-based";
        case 0:
            return "x86";
        default:
            return NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN;
    }
}

static DWORD netdata_windows_cpu_frequency(HKEY lKey)
{
    DWORD freq = 0;
    long ret = netdata_registry_get_dword_from_open_key(&freq, lKey, "~MHz");
    if (ret != ERROR_SUCCESS)
        return freq;

    freq *= 1000000;
    return freq;
}

static void netdata_windows_cpu_from_system_info(struct rrdhost_system_info *systemInfo)
{
    SYSTEM_INFO sysInfo;
    GetSystemInfo(&sysInfo);

    char cpuData[256];
    (void)snprintf(cpuData, 255, "%d", sysInfo.dwNumberOfProcessors);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", cpuData);

    char *arch = netdata_windows_arch(sysInfo.wProcessorArchitecture);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_ARCHITECTURE", arch);

    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_SYSTEM_VIRTUALIZATION", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);

    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_SYSTEM_VIRT_DETECTION", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);

    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_SYSTEM_CONTAINER", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);

    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_SYSTEM_CONTAINER_DETECTION", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);

}

static void netdata_windows_cpu_vendor_model(struct rrdhost_system_info *systemInfo,
                                                    HKEY lKey,
                                                    char *variable,
                                                    char *key)
{
    char cpuData[256];
    long ret = netdata_registry_get_string_from_open_key(cpuData, 255, lKey, key);
    (void)rrdhost_set_system_info_variable(systemInfo,
                                           variable,
                                           (ret == ERROR_SUCCESS) ? cpuData : NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);
}

static void netdata_windows_cpu_from_registry(struct rrdhost_system_info *systemInfo)
{
    HKEY lKey;
    long ret = RegOpenKeyEx(HKEY_LOCAL_MACHINE,
                            "HARDWARE\\DESCRIPTION\\System\\CentralProcessor\\0",
                            0,
                            KEY_READ,
                            &lKey);
    if (ret != ERROR_SUCCESS)
        return;

    ULONGLONG cpuFreq = netdata_windows_cpu_frequency(lKey);
    char cpuData[256];
    if (cpuFreq)
        (void)snprintf(cpuData, 255, "%lu", (unsigned long)cpuFreq);

    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_SYSTEM_CPU_FREQ",
                                           (!cpuFreq) ? NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN : cpuData);

    netdata_windows_cpu_vendor_model(systemInfo, lKey, "NETDATA_SYSTEM_CPU_VENDOR", "VendorIdentifier");
    netdata_windows_cpu_vendor_model(systemInfo, lKey, "NETDATA_SYSTEM_CPU_MODEL", "ProcessorNameString");
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_CPU_DETECTION", NETDATA_WIN_DETECTION_METHOD);
}

static void netdata_windows_get_cpu(struct rrdhost_system_info *systemInfo)
{
    netdata_windows_cpu_from_system_info(systemInfo);

    netdata_windows_cpu_from_registry(systemInfo);
}

static void netdata_windows_get_mem(struct rrdhost_system_info *systemInfo)
{
    ULONGLONG size;
    char memSize[256];
    if (!GetPhysicallyInstalledSystemMemory(&size))
        size = 0;
    else
        (void)snprintf(memSize, 255, "%llu", size);

    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_SYSTEM_TOTAL_RAM",
                                           (!size) ? NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN : memSize);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_RAM_DETECTION", NETDATA_WIN_DETECTION_METHOD);
}

static ULONGLONG netdata_windows_get_disk_size(char *cVolume)
{
    HANDLE disk = CreateFile(cVolume, GENERIC_READ, FILE_SHARE_VALID_FLAGS, 0, OPEN_EXISTING, 0, 0);
    if (!disk)
        return 0;

    GET_LENGTH_INFORMATION length;
    DWORD ret;

    if (!DeviceIoControl(disk, IOCTL_DISK_GET_LENGTH_INFO, 0, 0, &length, sizeof(length), &ret, 0))
        return 0;

    CloseHandle(disk);

    return length.Length.QuadPart;
}

static void netdata_windows_get_total_disk_size(struct rrdhost_system_info *systemInfo)
{
    ULONGLONG total = 0;
    char cVolume[8];
    snprintf(cVolume, 7, "\\\\.\\C:");

    DWORD lDrives = GetLogicalDrives();
    if (!lDrives) {
        return;
    }

    int i;
#define ND_POSSIBLE_VOLUMES 26
    for (i = 0; i < ND_POSSIBLE_VOLUMES; i++) {
        if (!(lDrives & 1 << i))
            continue;

        cVolume[4] = 'A' + i;
        total += netdata_windows_get_disk_size(cVolume);
    }

    char diskSize[256];
    (void)snprintf(diskSize, 255, "%llu", total);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_TOTAL_DISK_SIZE", diskSize);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_DISK_DETECTION", NETDATA_WIN_DETECTION_METHOD);
}

// Host
static DWORD netdata_windows_get_current_build()
{
    HKEY hKey;
    char cBuild[64];
    if (!netdata_registry_get_string(
            cBuild, 63, HKEY_LOCAL_MACHINE, "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", "CurrentBuild"))
        return 0;

    errno = 0;

    DWORD version = strtol(cBuild, NULL, 10);
    if (errno == ERANGE)
        return 0;

    return version;
}

static void netdata_windows_discover_os_version(char *os, size_t length, DWORD build)
{
    char *commonName = {"Windows"};

    if (IsWindowsServer()) {
        (void)snprintf(os, length, "%s Server", commonName);
        return;
    }

#define ND_WIN_VER_LENGTH 16
    char version[ND_WIN_VER_LENGTH + 1];
    if (IsWindows10OrGreater()) {
        // https://learn.microsoft.com/en-us/windows/release-health/windows11-release-information
        (void)snprintf(version, ND_WIN_VER_LENGTH, (build < 22000) ? "10" : "11");
    } else if (IsWindows8Point1OrGreater()) {
        (void)snprintf(version, ND_WIN_VER_LENGTH, "8.1");
    } else if (IsWindows8OrGreater()) {
        (void)snprintf(version, ND_WIN_VER_LENGTH, "8");
    } else if (IsWindows7SP1OrGreater()) {
        (void)snprintf(version, ND_WIN_VER_LENGTH, "7 SP1");
    } else if (IsWindows7OrGreater()) {
        (void)snprintf(version, ND_WIN_VER_LENGTH, "7");
    } else if (IsWindowsVistaSP2OrGreater()) {
        (void)snprintf(version, ND_WIN_VER_LENGTH, "Vista SP2");
    } else if (IsWindowsVistaSP1OrGreater()) {
        (void)snprintf(version, ND_WIN_VER_LENGTH, "Vista SP1");
    } else if (IsWindowsVistaOrGreater()) {
        (void)snprintf(version, ND_WIN_VER_LENGTH, "Vista");
    }
    // We are not testing older, because it is not supported anymore by Microsoft

    (void)snprintf(os, length, "%s %s Client", commonName, version);
}

static void netdata_windows_os_version(char *out, DWORD length)
{
    if (netdata_registry_get_string(out,
                                    length,
                                    HKEY_LOCAL_MACHINE,
                                    "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
                                    "ProductName"))
        return;

    (void)snprintf(out, length, "%s", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);
}

static void netdata_windows_os_kernel_version(char *out, DWORD length, DWORD build)
{
    char version[8];
    if (!netdata_registry_get_string(version,
                                    7,
                                    HKEY_LOCAL_MACHINE,
                                    "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
                                    "CurrentVersion"))
        version[0] = '\0';

    (void)snprintf(out, length, "%s (build: %u)", version, build);
}

static void netdata_windows_host(struct rrdhost_system_info *systemInfo)
{
    char osVersion[4096];
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_NAME", "Microsoft Windows");

    DWORD build = netdata_windows_get_current_build();

    netdata_windows_discover_os_version(osVersion, 4095, build);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_ID", osVersion);

    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_HOST_OS_ID_LIKE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);

    netdata_windows_os_version(osVersion, 4095);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_VERSION", osVersion);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_VERSION_ID", osVersion);

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_DETECTION", NETDATA_WIN_DETECTION_METHOD);

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_KERNEL_NAME", "Windows");

    netdata_windows_os_kernel_version(osVersion, 4095, build);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_KERNEL_VERSION", osVersion);

    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_HOST_IS_K8S_NODE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_FALSE);
}

// Cloud
static void netdata_windows_cloud(struct rrdhost_system_info *systemInfo)
{
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_INSTANCE_CLOUD_TYPE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_INSTANCE_CLOUD_INSTANCE_REGION", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);
}

// Container
static void netdata_windows_container(struct rrdhost_system_info *systemInfo)
{
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_CONTAINER_OS_NAME", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_CONTAINER_OS_ID", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_CONTAINER_OS_ID_LIKE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_CONTAINER_OS_VERSION", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_CONTAINER_OS_VERSION_ID", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_CONTAINER_OS_DETECTION", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_set_system_info_variable(
        systemInfo, "NETDATA_CONTAINER_IS_OFFICIAL_IMAGE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_FALSE);
}

void netdata_windows_get_system_info(struct rrdhost_system_info *systemInfo)
{
    netdata_windows_cloud(systemInfo);
    netdata_windows_container(systemInfo);
    netdata_windows_host(systemInfo);
    netdata_windows_get_cpu(systemInfo);
    netdata_windows_get_mem(systemInfo);
    netdata_windows_get_total_disk_size(systemInfo);
}
#endif
