#include "win_system-info.h"

#ifdef OS_WINDOWS

#include "win_system-info.h"

// Hardware
char *netdata_windows_arch(DWORD value)
{
    switch(value) {
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
            return NETDATA_DEFAULT_VALUE_SYSTEM_INFO;
    }
}

DWORD netdata_windows_cpu_frequency()
{
    HKEY hKey;
    long ret = RegOpenKeyEx(HKEY_LOCAL_MACHINE,
                            "HARDWARE\\DESCRIPTION\\System\\CentralProcessor\\0",
                            0,
                            KEY_READ,
                            &hKey);
    if (ret != ERROR_SUCCESS)
        return 0;

    DWORD length = 260, freq = 260;
    ret = RegQueryValueEx(hKey, "~MHz", NULL, NULL, (LPBYTE) &freq, &length);
    if (ret != ERROR_SUCCESS)
        freq = 0;

    RegCloseKey(hKey);
    freq *= 1000000;

    return freq;
}

void netdata_windows_get_cpu(struct rrdhost_system_info *systemInfo)
{
    SYSTEM_INFO sysInfo;
    GetSystemInfo(&sysInfo);

    char cpuData[256];
    (void)snprintf(cpuData, 255, "%d", sysInfo.dwNumberOfProcessors);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", cpuData);

    ULONGLONG cpuFreq = netdata_windows_cpu_frequency();
    if (cpuFreq)
        (void)snprintf(cpuData, 255, "%lu", (unsigned long) cpuFreq);

    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_SYSTEM_CPU_FREQ",
                                           (!cpuFreq) ? NETDATA_DEFAULT_VALUE_SYSTEM_INFO : cpuData);

    char *arch = netdata_windows_arch(sysInfo.wProcessorArchitecture);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_ARCHITECTURE", arch);
}

void netdata_windows_get_mem(struct rrdhost_system_info *systemInfo)
{
    ULONGLONG size;
    char memSize[256];
    if (!GetPhysicallyInstalledSystemMemory(&size))
        size = 0;
    else
        (void)snprintf(memSize, 255, "%llu", size);

    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_SYSTEM_TOTAL_RAM",
                                           (!size) ? NETDATA_DEFAULT_VALUE_SYSTEM_INFO : memSize);
}

ULONGLONG inline netdata_windows_get_disk_size(char *cVolume)
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

void netdata_windows_get_total_disk_size(struct rrdhost_system_info *systemInfo)
{
    ULONGLONG total = 0;
    char cVolume[8];
    snprintf(cVolume, 7, "\\\\.\\C:");

    DWORD lDrives = GetLogicalDrives();
    if (!lDrives) {
        return 0;
    }

    int i;
#define ND_POSSIBLE_VOLUMES 26
    for (i = 0 ; i < ND_POSSIBLE_VOLUMES; i++) {
        if (!(lDrives & 1<<i))
            continue;

        cVolume[4] = 'A' + i;
        total += netdata_windows_get_disk_size(cVolume);
    }

    char diskSize[256];
    (void)snprintf(diskSize, 255, "%llu", total);
    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_SYSTEM_TOTAL_DISK_SIZE",
                                           diskSize);
}

// Host
void netdata_windows_discover_os_version(char *os, size_t length) {
    char *commonName = { "Windows" };
    if (IsWindowsServer()) {
        (void)snprintf(os, length, "%s Server", commonName);
        return;
    }

#define ND_WIN_VER_LENGTH 16
    char version[ND_WIN_VER_LENGTH + 1];
    if (IsWindows10OrGreater()) {
        (void)snprintf(version, ND_WIN_VER_LENGTH, "10");
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

static inline void netdata_windows_host(struct rrdhost_system_info *systemInfo) {
	char osVersion[4096];
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_NAME", "Windows");

    netdata_windows_discover_os_version(osVersion, 4095);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_ID", osVersion);

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_ID_LIKE", NETDATA_DEFAULT_VALUE_SYSTEM_INFO);

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_VERSION", osVersion);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_VERSION_ID", osVersion);

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_DETECTION", "Windows API");

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_KERNEL_NAME", NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
}

// Cloud
static inline void netdata_windows_cloud(struct rrdhost_system_info *systemInfo) {
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_INSTANCE_CLOUD_TYPE", NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE",
                                           NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_INSTANCE_CLOUD_INSTANCE_REGION",
                                           NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
}

// Container
static inline void netdata_windows_container(struct rrdhost_system_info *systemInfo) {
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_CONTAINER_OS_NAME", NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_CONTAINER_OS_ID", NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_CONTAINER_OS_ID_LIKE",
                                           NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_CONTAINER_OS_VERSION",
                                           NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_CONTAINER_OS_VERSION_ID",
                                           NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
    (void)rrdhost_set_system_info_variable(systemInfo,
                                           "NETDATA_CONTAINER_OS_DETECTION",
                                           NETDATA_DEFAULT_VALUE_SYSTEM_INFO);
}

void netdata_windows_get_system_info(struct rrdhost_system_info *systemInfo) {
    netdata_windows_cloud(systemInfo);
    netdata_windows_container(systemInfo);
    netdata_windows_host(systemInfo);
    netdata_windows_get_cpu(systemInfo);
    netdata_windows_get_mem(systemInfo);
    netdata_windows_get_total_disk_size(systemInfo);
}
#endif
