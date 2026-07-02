// SPDX-License-Identifier: GPL-3.0-or-later

#include "win_system-info.h"
#include "database/rrdhost-system-info.h"
#include "libnetdata/os/windows-api/windows_api.h"
#include "libnetdata/os/windows-wmi/windows-wmi.h"
#include "libnetdata/os/windows-wmi/windows-wmi-GetSystemInfo.h"
#include "daemon/status-file-dmi.h"
#include "daemon/status-file-product.h"
#include "libnetdata/os/os-windows-wrappers.h"
#include "libnetdata/os/setenv.h"

#ifdef OS_WINDOWS

static void netdata_windows_ip(struct rrdhost_system_info *systemInfo)
{
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_DEFAULT_INTERFACE_DETECTION", "WINAPI");

    char *ptr = netdata_win_local_interface();
    if (ptr)
        (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME", ptr);

    ptr = netdata_win_local_ip();
    if (ptr)
        (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_DEFAULT_INTERFACE_IP", ptr);
}

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
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", cpuData);

    char *arch = netdata_windows_arch(sysInfo.wProcessorArchitecture);
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_ARCHITECTURE", arch);
}

static void netdata_windows_cpu_vendor_model(struct rrdhost_system_info *systemInfo,
                                                    HKEY lKey,
                                                    char *variable,
                                                    char *key)
{
    char cpuData[256];
    long ret = netdata_registry_get_string_from_open_key(cpuData, 255, lKey, key);
    (void)rrdhost_system_info_set_by_name(systemInfo,
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

    (void)rrdhost_system_info_set_by_name(systemInfo,
                                           "NETDATA_SYSTEM_CPU_FREQ",
                                           (!cpuFreq) ? NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN : cpuData);

    netdata_windows_cpu_vendor_model(systemInfo, lKey, "NETDATA_SYSTEM_CPU_VENDOR", "VendorIdentifier");
    netdata_windows_cpu_vendor_model(systemInfo, lKey, "NETDATA_SYSTEM_CPU_MODEL", "ProcessorNameString");
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_CPU_DETECTION", NETDATA_WIN_DETECTION_METHOD);
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
    // The amount of physically installed RAM, in kilobytes.
    if (!GetPhysicallyInstalledSystemMemory(&size))
        size = 0;
    else
        (void)snprintf(memSize, 255, "%llu", size * 1024); // to bytes

    (void)rrdhost_system_info_set_by_name(systemInfo,
                                           "NETDATA_SYSTEM_TOTAL_RAM",
                                           (!size) ? NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN : memSize);
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_RAM_DETECTION", NETDATA_WIN_DETECTION_METHOD);
}

static ULONGLONG netdata_windows_get_disk_size(char *cVolume)
{
    HANDLE disk = CreateFile(cVolume, GENERIC_READ, FILE_SHARE_VALID_FLAGS, 0, OPEN_EXISTING, 0, 0);
    if (disk == INVALID_HANDLE_VALUE)
        return 0;

    GET_LENGTH_INFORMATION length;
    DWORD ret;

    if (!DeviceIoControl(disk, IOCTL_DISK_GET_LENGTH_INFO, 0, 0, &length, sizeof(length), &ret, 0)) {
        CloseHandle(disk);
        return 0;
    }

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
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_TOTAL_DISK_SIZE", diskSize);
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_DISK_DETECTION", NETDATA_WIN_DETECTION_METHOD);
}

// Host
static DWORD netdata_windows_get_current_build()
{
    char cBuild[64];
    if (!netdata_registry_get_string(
            cBuild, 63, HKEY_LOCAL_MACHINE, "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", "CurrentBuild"))
        return 0;

    errno_clear();

    DWORD version = strtol(cBuild, NULL, 10);
    if (errno == ERANGE)
        return 0;

    return version;
}

static void netdata_windows_discover_os_version(char *os, size_t length, DWORD build)
{
    char versionName[256];
    if (!netdata_registry_get_string(versionName,
                                    255,
                                    HKEY_LOCAL_MACHINE,
                                    "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
                                    "DisplayVersion"))
    {
        (void)snprintf(os, length, "Microsoft Windows");
        return;
    }

    if (IsWindowsServer()) {
        (void)snprintf(os, length, "Microsoft Windows Version %s", versionName);
        return;
    }

#define ND_WIN_VER_LENGTH 16
    char version[ND_WIN_VER_LENGTH + 1] = NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN;
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

    (void)snprintf(os, length, "Microsoft Windows Version %s, Build %d", version, build);
}

static void netdata_windows_os_kernel_version(char *out, DWORD length, DWORD build)
{
    DWORD major, minor;
    if (!netdata_registry_get_dword(&major,
                                    HKEY_LOCAL_MACHINE,
                                    "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
                                    "CurrentMajorVersionNumber"))
        major = 0;

    if (!netdata_registry_get_dword(&minor,
                                    HKEY_LOCAL_MACHINE,
                                    "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
                                    "CurrentMinorVersionNumber"))
        minor = 0;

    (void)snprintf(out, length, "Windows %u.%u.%u Build: %u", major, minor, build, build);
}

static char *netdata_windows_get_edition(void)
{
    static char edition[256] = {0};
    
    // Try to read EditionID first, which is more precise
    if (netdata_registry_get_string(edition, sizeof(edition)-1, HKEY_LOCAL_MACHINE, 
                                   "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", 
                                   "EditionID")) {
        return edition;
    }
    
    // If EditionID fails, try ProductName
    if (netdata_registry_get_string(edition, sizeof(edition)-1, HKEY_LOCAL_MACHINE, 
                                   "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", 
                                   "ProductName")) {
        return edition;
    }
    
    // Return unknown if both methods fail
    return NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN;
}

static char *netdata_windows_get_os_id_like(DWORD build)
{
    static char id_like[256];
    char *edition = netdata_windows_get_edition();
    const char *base_id = "";
    
    if (IsWindowsServer()) {
        // Windows Server versions based on build numbers
        if (build >= 25000)
            base_id = "Windows-Server-2025";
        else if (build >= 20348)
            base_id = "Windows-Server-2022";
        else if (build >= 17763)
            base_id = "Windows-Server-2019";
        else if (build >= 14393)
            base_id = "Windows-Server-2016";
        else if (build >= 9600)
            base_id = "Windows-Server-2012R2";
        else if (build >= 9200)
            base_id = "Windows-Server-2012";
        else if (build >= 7601)
            base_id = "Windows-Server-2008R2";
        else
            base_id = "Windows-Server";
    } else {
        // Windows client versions
        if (build >= 22000)
            base_id = "Windows-11";
        else if (build >= 10240)
            base_id = "Windows-10";
        else if (build >= 9600)
            base_id = "Windows-8.1";
        else if (build >= 9200)
            base_id = "Windows-8";
        else if (build >= 7601)
            base_id = "Windows-7";
        else
            base_id = "Windows";
    }
    
    // If we have a valid edition, append it to the ID_LIKE with a dash
    if (strcmp(edition, NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN) != 0) {
        snprintf(id_like, sizeof(id_like), "%s-%s", base_id, edition);
    } else {
        strcpy(id_like, base_id);
    }
    
    return id_like;
}

static void netdata_windows_host(struct rrdhost_system_info *systemInfo)
{
    char osVersion[4096];
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_HOST_OS_NAME", "Microsoft Windows");

    DWORD build = netdata_windows_get_current_build();

    netdata_windows_discover_os_version(osVersion, 4095, build);
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_HOST_OS_ID", osVersion);

    char *id_like = netdata_windows_get_os_id_like(build);
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_HOST_OS_ID_LIKE", id_like);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_HOST_OS_VERSION", osVersion);
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_HOST_OS_VERSION_ID", osVersion);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_HOST_OS_DETECTION", NETDATA_WIN_DETECTION_METHOD);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_KERNEL_NAME", "Windows");

    netdata_windows_os_kernel_version(osVersion, 4095, build);
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_KERNEL_VERSION", osVersion);

    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_HOST_IS_K8S_NODE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_FALSE);
}

// Cloud
static void netdata_windows_cloud(struct rrdhost_system_info *systemInfo)
{
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_INSTANCE_CLOUD_TYPE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_INSTANCE_CLOUD_INSTANCE_REGION", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);
}

// Container
static void netdata_windows_container(struct rrdhost_system_info *systemInfo)
{
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_CONTAINER_OS_NAME", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_CONTAINER_OS_ID", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_CONTAINER_OS_ID_LIKE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_CONTAINER_OS_VERSION", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_CONTAINER_OS_VERSION_ID", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_CONTAINER_OS_DETECTION", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_CONTAINER_IS_OFFICIAL_IMAGE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_FALSE);
}

static void netdata_windows_install_type(struct rrdhost_system_info *systemInfo)
{
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_INSTALL_TYPE", "netdata-installer.exe");
}

static bool netdata_windows_str_contains_ci(const char *haystack, const char *needle) {
    if(!haystack || !needle || !*needle) return false;
    size_t nlen = strlen(needle);
    for(const char *p = haystack; *p; p++) {
        if(strncasecmp(p, needle, nlen) == 0)
            return true;
    }
    return false;
}

const char *netdata_windows_normalize_virt_string(const char *raw) {
    if(!raw || !*raw) return NETDATA_WIN_VIRT_BARE_METAL;

    if(netdata_windows_str_contains_ci(raw, "vmware")) return NETDATA_WIN_VIRT_VMWARE;
    if(netdata_windows_str_contains_ci(raw, "virtualbox")) return NETDATA_WIN_VIRT_ORACLE;
    if(netdata_windows_str_contains_ci(raw, "innotek") || netdata_windows_str_contains_ci(raw, "oracle corp")) return NETDATA_WIN_VIRT_ORACLE;
    if(netdata_windows_str_contains_ci(raw, "parallels")) return NETDATA_WIN_VIRT_PARALLELS;
    if(netdata_windows_str_contains_ci(raw, "qemu")) return NETDATA_WIN_VIRT_QEMU;
    if(netdata_windows_str_contains_ci(raw, "kvm")) return NETDATA_WIN_VIRT_KVM;
    if(netdata_windows_str_contains_ci(raw, "xen") || netdata_windows_str_contains_ci(raw, "domu")) return NETDATA_WIN_VIRT_XEN;
    if(netdata_windows_str_contains_ci(raw, "amazon")) return NETDATA_WIN_VIRT_AMAZON;
    if(netdata_windows_str_contains_ci(raw, "digitalocean")) return NETDATA_WIN_VIRT_DIGITALOCEAN;
    if(netdata_windows_str_contains_ci(raw, "virtual machine") || netdata_windows_str_contains_ci(raw, "hyper-v")) return NETDATA_WIN_VIRT_MICROSOFT;

    return NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN;
}

static bool netdata_windows_is_unknown_virt_result(const char *virt) {
    return virt && strcmp(virt, NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN) == 0;
}

const char *netdata_windows_resolve_virt_detection(const char *wmi, const char *smbios, const char *registry) {
    const char *probes[] = { wmi, smbios, registry };
    const char *unknown = NULL;

    for(size_t i = 0; i < sizeof(probes) / sizeof(probes[0]); i++) {
        const char *probe = probes[i];
        if(!probe)
            continue;

        if(netdata_windows_is_unknown_virt_result(probe)) {
            unknown = probe;
            continue;
        }

        return probe;
    }

    if(unknown)
        return unknown;

    return NETDATA_WIN_VIRT_BARE_METAL;
}

static const char *netdata_windows_detect_via_wmi(void) {
    Win32ComputerSystemInfo cs;
    if(!GetWin32ComputerSystemInfo(&cs) || !cs.Populated)
        return NULL;

    if(cs.Model[0]) {
        const char *m = netdata_windows_normalize_virt_string(cs.Model);
        if(strcmp(m, NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN) != 0)
            return m;
    }

    if(cs.Manufacturer[0]) {
        const char *m = netdata_windows_normalize_virt_string(cs.Manufacturer);
        if(strcmp(m, NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN) != 0)
            return m;
    }

    return NULL;
}

static const char *netdata_windows_detect_via_smbios(void) {
    DMI_INFO dmi;
    dmi_info_init(&dmi);
    os_dmi_info_get(&dmi);

    if(!dmi_is_virtual_machine(&dmi))
        return NULL;

    char buf[128];
    buf[0] = '\0';

    if(dmi.product.name[0]) {
        snprintf(buf, sizeof(buf), "%s", dmi.product.name);
    }
    else if(dmi.product.family[0]) {
        snprintf(buf, sizeof(buf), "%s", dmi.product.family);
    }
    else if(dmi.sys.vendor[0]) {
        snprintf(buf, sizeof(buf), "%s", dmi.sys.vendor);
    }
    else if(dmi.board.name[0]) {
        snprintf(buf, sizeof(buf), "%s", dmi.board.name);
    }
    else {
        return NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN;
    }

    const char *m = netdata_windows_normalize_virt_string(buf);
    if(strcmp(m, NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN) == 0)
        return NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN;

    return m;
}

static const char *netdata_windows_detect_via_registry(void) {
    if(netdata_registry_key_exists(HKEY_LOCAL_MACHINE, "SOFTWARE\\VMware, Inc.\\VMware Tools"))
        return NETDATA_WIN_VIRT_VMWARE;

    if(netdata_registry_key_exists(HKEY_LOCAL_MACHINE, "SOFTWARE\\Oracle\\VirtualBox Guest Additions"))
        return NETDATA_WIN_VIRT_ORACLE;

    if(netdata_registry_key_exists(HKEY_LOCAL_MACHINE, "SOFTWARE\\Parallels\\Parallels Tools"))
        return NETDATA_WIN_VIRT_PARALLELS;

    return NULL;
}

static const char *netdata_windows_detect_virt(void) {
    const char *wmi = netdata_windows_detect_via_wmi();
    const char *smbios = netdata_windows_detect_via_smbios();
    const char *registry = netdata_windows_detect_via_registry();

    return netdata_windows_resolve_virt_detection(wmi, smbios, registry);
}

static void netdata_windows_detect_virtualization(struct rrdhost_system_info *systemInfo) {
    const char *virt = netdata_windows_detect_virt();

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_VIRTUALIZATION", virt);
    nd_setenv("NETDATA_SYSTEM_VIRTUALIZATION", virt, 1);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_VIRT_DETECTION", NETDATA_WIN_DETECTION_METHOD);
    nd_setenv("NETDATA_SYSTEM_VIRT_DETECTION", NETDATA_WIN_DETECTION_METHOD, 1);
}

static const char *netdata_windows_detect_container(void) {
    char *k_host = getenv("KUBERNETES_SERVICE_HOST");
    char *k_port = getenv("KUBERNETES_SERVICE_PORT");
    if(k_host && *k_host && k_port && *k_port)
        return NETDATA_WIN_CONTAINER_KUBERNETES;

    Win32OperatingSystemInfo os;
    if(GetWin32OperatingSystemInfo(&os) && os.Populated && os.Caption[0]) {
        if(netdata_windows_str_contains_ci(os.Caption, "container"))
            return NETDATA_WIN_CONTAINER_WINDOWS;
    }

    return NETDATA_WIN_CONTAINER_NONE;
}

static void netdata_windows_detect_container_state(struct rrdhost_system_info *systemInfo) {
    const char *container = netdata_windows_detect_container();

    const char *container_detection;
    if(strcmp(container, NETDATA_WIN_CONTAINER_KUBERNETES) == 0)
        container_detection = NETDATA_WIN_CONTAINER_KUBERNETES_DETECT;
    else if(strcmp(container, NETDATA_WIN_CONTAINER_WINDOWS) == 0)
        container_detection = NETDATA_WIN_CONTAINER_WINDOWS_DETECT;
    else
        container_detection = NETDATA_WIN_CONTAINER_NONE;

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_CONTAINER", container);
    nd_setenv("NETDATA_SYSTEM_CONTAINER", container, 1);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_CONTAINER_DETECTION", container_detection);
    nd_setenv("NETDATA_SYSTEM_CONTAINER_DETECTION", container_detection, 1);
}

void netdata_windows_get_system_info(struct rrdhost_system_info *systemInfo)
{
    netdata_windows_cloud(systemInfo);
    netdata_windows_container(systemInfo);
    netdata_windows_host(systemInfo);
    netdata_windows_get_cpu(systemInfo);
    netdata_windows_detect_virtualization(systemInfo);
    netdata_windows_detect_container_state(systemInfo);
    netdata_windows_get_mem(systemInfo);
    netdata_windows_get_total_disk_size(systemInfo);
    netdata_windows_install_type(systemInfo);
    netdata_windows_ip(systemInfo);
}
#endif
