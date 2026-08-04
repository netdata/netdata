// SPDX-License-Identifier: GPL-3.0-or-later

#include "win_system-info.h"
#include "database/rrdhost-system-info.h"
#include "libnetdata/os/windows-api/windows_api.h"
#include "libnetdata/os/windows-wmi/windows-wmi.h"
#include "libnetdata/os/windows-wmi/windows-wmi-GetSystemInfo.h"
#include "daemon/status-file-dmi.h"
#include "daemon/status-file-product.h"
#include "libnetdata/environment/environment.h"
#include "libnetdata/os/os-windows-wrappers.h"

#ifdef OS_WINDOWS

static void netdata_windows_publish_environment(const char *name, const char *value) {
    if(nd_environment_set(name, value, true) != 0)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "WINDOWS SYSTEM INFO: cannot publish child environment variable '%s': %s",
               name, strerror(errno));
}

typedef struct netdata_windows_os_info {
    char name[256];
    char id[4096];
    char id_like[256];
    char version[4096];
    char version_id[4096];
    char detection[64];
} NETDATA_WINDOWS_OS_INFO;

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

    // GetSystemInfo() cannot fail, so CPU info (arch, logical count) is always gathered here.
    // Record the detection method unconditionally, independent of the optional registry probe
    // (freq/vendor/model) that may fail. Not stored in the struct; consumed from the environment
    // by anonymous-statistics.sh (mirrors the Linux system-info.sh dispatch, and the RAM/disk
    // detection pattern in this file).
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_CPU_DETECTION", NETDATA_WIN_DETECTION_METHOD);
    netdata_windows_publish_environment("NETDATA_SYSTEM_CPU_DETECTION", NETDATA_WIN_DETECTION_METHOD);
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
    // Not stored in the struct; consumed from the environment by anonymous-statistics.sh.
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_RAM_DETECTION", NETDATA_WIN_DETECTION_METHOD);
    netdata_windows_publish_environment("NETDATA_SYSTEM_RAM_DETECTION", NETDATA_WIN_DETECTION_METHOD);
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

    // Not stored in the struct; consumed from the environment by anonymous-statistics.sh.
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_DISK_DETECTION", NETDATA_WIN_DETECTION_METHOD);
    netdata_windows_publish_environment("NETDATA_SYSTEM_DISK_DETECTION", NETDATA_WIN_DETECTION_METHOD);
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

static void netdata_windows_set_os_fields(struct rrdhost_system_info *systemInfo,
                                          const char *prefix,
                                          const char *name,
                                          const char *id,
                                          const char *id_like,
                                          const char *version,
                                          const char *version_id,
                                          const char *detection)
{
    char key[64];

    snprintf(key, sizeof(key), "%s_NAME", prefix);
    (void)rrdhost_system_info_set_by_name(systemInfo, key, name);

    snprintf(key, sizeof(key), "%s_ID", prefix);
    (void)rrdhost_system_info_set_by_name(systemInfo, key, id);

    snprintf(key, sizeof(key), "%s_ID_LIKE", prefix);
    (void)rrdhost_system_info_set_by_name(systemInfo, key, id_like);

    snprintf(key, sizeof(key), "%s_VERSION", prefix);
    (void)rrdhost_system_info_set_by_name(systemInfo, key, version);

    snprintf(key, sizeof(key), "%s_VERSION_ID", prefix);
    (void)rrdhost_system_info_set_by_name(systemInfo, key, version_id);

    snprintf(key, sizeof(key), "%s_DETECTION", prefix);
    (void)rrdhost_system_info_set_by_name(systemInfo, key, detection);
}

static void netdata_windows_get_local_os_info(NETDATA_WINDOWS_OS_INFO *info)
{
    memset(info, 0, sizeof(*info));

    snprintf(info->name, sizeof(info->name), "%s", "Microsoft Windows");

    DWORD build = netdata_windows_get_current_build();
    netdata_windows_discover_os_version(info->id, sizeof(info->id), build);
    snprintf(info->version, sizeof(info->version), "%s", info->id);
    snprintf(info->version_id, sizeof(info->version_id), "%s", info->id);
    snprintf(info->id_like, sizeof(info->id_like), "%s", netdata_windows_get_os_id_like(build));
    snprintf(info->detection, sizeof(info->detection), "%s", NETDATA_WIN_DETECTION_METHOD);
}

static void netdata_windows_set_local_kernel_info(struct rrdhost_system_info *systemInfo)
{
    char kernelVersion[4096];
    DWORD build = netdata_windows_get_current_build();

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_KERNEL_NAME", "Windows");
    netdata_windows_os_kernel_version(kernelVersion, sizeof(kernelVersion), build);
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_KERNEL_VERSION", kernelVersion);
}

static void netdata_windows_set_host_os_info(struct rrdhost_system_info *systemInfo, const NETDATA_WINDOWS_OS_INFO *info)
{
    netdata_windows_set_os_fields(systemInfo,
                                  "NETDATA_HOST_OS",
                                  info->name,
                                  info->id,
                                  info->id_like,
                                  info->version,
                                  info->version_id,
                                  info->detection);
}

static void netdata_windows_set_host_os_unknown(struct rrdhost_system_info *systemInfo)
{
    netdata_windows_set_os_fields(systemInfo,
                                  "NETDATA_HOST_OS",
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN);
}

static void netdata_windows_set_container_os_info(struct rrdhost_system_info *systemInfo, const NETDATA_WINDOWS_OS_INFO *info)
{
    netdata_windows_set_os_fields(systemInfo,
                                  "NETDATA_CONTAINER_OS",
                                  info->name,
                                  info->id,
                                  info->id_like,
                                  info->version,
                                  info->version_id,
                                  info->detection);
}

static void netdata_windows_set_container_os_none(struct rrdhost_system_info *systemInfo)
{
    netdata_windows_set_os_fields(systemInfo,
                                  "NETDATA_CONTAINER_OS",
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE,
                                  NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE);
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
static void netdata_windows_container(struct rrdhost_system_info *systemInfo, const char *container)
{
    NETDATA_WINDOWS_OS_INFO info;
    netdata_windows_get_local_os_info(&info);

    if(strcmp(container, NETDATA_WIN_CONTAINER_NONE) == 0) {
        netdata_windows_set_host_os_info(systemInfo, &info);
        netdata_windows_set_container_os_none(systemInfo);
    }
    else {
        netdata_windows_set_host_os_unknown(systemInfo);
        netdata_windows_set_container_os_info(systemInfo, &info);
    }

    netdata_windows_set_local_kernel_info(systemInfo);
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_HOST_IS_K8S_NODE", NETDATA_DEFAULT_SYSTEM_INFO_VALUE_FALSE);

    // rrdhost_system_info_set_by_name() recognizes but does not store this key (it has no
    // struct field); its only consumer is anonymous-statistics.sh, which reads it from the
    // managed environment. Mirror the Linux system-info.sh publication so the flag is not
    // silently dropped on Windows.
    CLEAN_CHAR_P *official_image_env = nd_environment_get_dup("NETDATA_OFFICIAL_IMAGE");
    const char *official_image = official_image_env && *official_image_env ?
                                     official_image_env : NETDATA_DEFAULT_SYSTEM_INFO_VALUE_FALSE;
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_CONTAINER_IS_OFFICIAL_IMAGE", official_image);
    netdata_windows_publish_environment("NETDATA_CONTAINER_IS_OFFICIAL_IMAGE", official_image);
}

static void netdata_windows_install_type(struct rrdhost_system_info *systemInfo)
{
    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_INSTALL_TYPE", "netdata-installer.exe");
}

static bool netdata_windows_str_contains_ci(const char *haystack, const char *needle) {
    return haystack && needle && *needle && strcasestr(haystack, needle) != NULL;
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
    if(netdata_windows_str_contains_ci(raw, "virtual machine") ||
       netdata_windows_str_contains_ci(raw, "hyper-v") ||
       netdata_windows_str_contains_ci(raw, "microsoft hv")) return NETDATA_WIN_VIRT_MICROSOFT;

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
    netdata_windows_publish_environment("NETDATA_SYSTEM_VIRTUALIZATION", virt);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_VIRT_DETECTION", NETDATA_WIN_DETECTION_METHOD);
    netdata_windows_publish_environment("NETDATA_SYSTEM_VIRT_DETECTION", NETDATA_WIN_DETECTION_METHOD);
}

const char *netdata_windows_container_from_env(const char *k_host, const char *k_port) {
    if(k_host && *k_host && k_port && *k_port)
        return NETDATA_WIN_CONTAINER_KUBERNETES;

    return NULL;
}

const char *netdata_windows_container_detection_method(const char *container) {
    if(container && strcmp(container, NETDATA_WIN_CONTAINER_KUBERNETES) == 0)
        return NETDATA_WIN_CONTAINER_KUBERNETES_DETECT;

    if(container && strcmp(container, NETDATA_WIN_CONTAINER_WINDOWS) == 0)
        return NETDATA_WIN_CONTAINER_WINDOWS_DETECT;

    return NETDATA_WIN_CONTAINER_NONE;
}

static const char *netdata_windows_detect_container(void) {
    CLEAN_CHAR_P *kubernetes_service_host = nd_environment_get_dup("KUBERNETES_SERVICE_HOST");
    CLEAN_CHAR_P *kubernetes_service_port = nd_environment_get_dup("KUBERNETES_SERVICE_PORT");
    const char *from_env = netdata_windows_container_from_env(
        kubernetes_service_host, kubernetes_service_port);
    if(from_env)
        return from_env;

    // Windows container base images (servercore/nanoserver) report the normal OS edition string
    // in Win32_OperatingSystem.Caption (e.g. "Microsoft Windows Server 2022 Datacenter"), never
    // the word "container", so a caption match cannot detect them. The reliable marker is the
    // ContainerType value under HKLM\SYSTEM\CurrentControlSet\Control, which the host populates
    // only inside a Windows container; its mere presence identifies the container.
    unsigned int container_type;
    if(netdata_registry_get_dword(&container_type, HKEY_LOCAL_MACHINE,
                                  "SYSTEM\\CurrentControlSet\\Control", "ContainerType"))
        return NETDATA_WIN_CONTAINER_WINDOWS;

    return NETDATA_WIN_CONTAINER_NONE;
}

static const char *netdata_windows_detect_container_state(struct rrdhost_system_info *systemInfo) {
    const char *container = netdata_windows_detect_container();
    const char *container_detection = netdata_windows_container_detection_method(container);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_CONTAINER", container);
    netdata_windows_publish_environment("NETDATA_SYSTEM_CONTAINER", container);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_CONTAINER_DETECTION", container_detection);
    netdata_windows_publish_environment("NETDATA_SYSTEM_CONTAINER_DETECTION", container_detection);

    return container;
}

void netdata_windows_get_system_info(struct rrdhost_system_info *systemInfo)
{
    const char *container;

    netdata_windows_cloud(systemInfo);
    netdata_windows_get_cpu(systemInfo);
    netdata_windows_detect_virtualization(systemInfo);
    container = netdata_windows_detect_container_state(systemInfo);
    netdata_windows_container(systemInfo, container);
    netdata_windows_get_mem(systemInfo);
    netdata_windows_get_total_disk_size(systemInfo);
    netdata_windows_install_type(systemInfo);
    netdata_windows_ip(systemInfo);
}
#endif
