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

typedef struct netdata_windows_os_info {
    char name[256];
    char id[4096];
    char id_like[256];
    char version[4096];
    char version_id[4096];
    char detection[64];
    NETDATA_WINDOWS_OS_LABELS labels;
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
    nd_setenv("NETDATA_SYSTEM_CPU_DETECTION", NETDATA_WIN_DETECTION_METHOD, 1);
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
    nd_setenv("NETDATA_SYSTEM_RAM_DETECTION", NETDATA_WIN_DETECTION_METHOD, 1);
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
    nd_setenv("NETDATA_SYSTEM_DISK_DETECTION", NETDATA_WIN_DETECTION_METHOD, 1);
}

// Host
static DWORD netdata_windows_get_current_build()
{
    char cBuild[64] = { 0 };
    if (!netdata_registry_get_string(
            cBuild, 63, HKEY_LOCAL_MACHINE, "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", "CurrentBuild"))
        return 0;

    errno_clear();

    DWORD version = strtol(cBuild, NULL, 10);
    if (errno == ERANGE)
        return 0;

    return version;
}

static bool netdata_windows_get_update_revision(DWORD *ubr)
{
    return netdata_registry_get_dword(ubr,
                                      HKEY_LOCAL_MACHINE,
                                      "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion",
                                      "UBR");
}

static const char *netdata_windows_server_version_from_build(DWORD build)
{
    if (build >= 26100)
        return "2025";
    if (build >= 20348)
        return "2022";
    if (build >= 17763)
        return "2019";
    if (build >= 14393)
        return "2016";
    if (build >= 9600)
        return "2012R2";
    if (build >= 9200)
        return "2012";
    if (build >= 7601)
        return "2008R2";
    return "";
}

static const char *netdata_windows_client_version_from_build(DWORD build)
{
    if (build >= 22000)
        return "11";
    if (build >= 10240)
        return "10";
    if (build >= 9600)
        return "8.1";
    if (build >= 9200)
        return "8";
    if (build >= 7601)
        return "7";
    return "";
}

static const char *netdata_windows_without_microsoft_prefix(const char *product_name)
{
    if (product_name && !strncasecmp(product_name,
                                     NETDATA_WINDOWS_MICROSOFT_PREFIX,
                                     sizeof(NETDATA_WINDOWS_MICROSOFT_PREFIX) - 1))
        return product_name + (sizeof(NETDATA_WINDOWS_MICROSOFT_PREFIX) - 1);

    return product_name;
}

static void netdata_windows_product_edition(char *edition, size_t length, const char *product_name, bool is_server)
{
    if (!product_name || !*product_name || !length)
        return;

    const char *name = netdata_windows_without_microsoft_prefix(product_name);
    const char *prefix = is_server
                              ? NETDATA_WINDOWS_OS_PREFIX_SERVER
                              : NETDATA_WINDOWS_OS_PREFIX_CLIENT;
    size_t prefix_length = is_server
                               ? sizeof(NETDATA_WINDOWS_OS_PREFIX_SERVER) - 1
                               : sizeof(NETDATA_WINDOWS_OS_PREFIX_CLIENT) - 1;
    if (strncasecmp(name, prefix, prefix_length) ||
        (name[prefix_length] && name[prefix_length] != ' '))
        return;

    name += prefix_length;
    while (*name == ' ')
        name++;

    while (*name >= '0' && *name <= '9')
        name++;
    while (*name == '.' || *name == ' ')
        name++;

    if (*name)
        snprintf(edition, length, "%s", name);
}

void netdata_windows_parse_os_labels(NETDATA_WINDOWS_OS_LABELS *labels, const char *product_name,
                                     const char *display_version, const char *edition_id, DWORD build,
                                     DWORD ubr, bool has_ubr, bool is_server)
{
    memset(labels, 0, sizeof(*labels));

    snprintf(labels->name, sizeof(labels->name), "%s",
             is_server ? NETDATA_WINDOWS_OS_PREFIX_SERVER : NETDATA_WINDOWS_OS_PREFIX_CLIENT);

    const char *version_source = is_server ? netdata_windows_server_version_from_build(build)
                                           : netdata_windows_client_version_from_build(build);
    if (is_server) {
        const char *name = netdata_windows_without_microsoft_prefix(product_name);
        if (name && !strncasecmp(name,
                                 NETDATA_WINDOWS_OS_PREFIX_SERVER_NAME,
                                 sizeof(NETDATA_WINDOWS_OS_PREFIX_SERVER_NAME) - 1)) {
            const char *digits = name + (sizeof(NETDATA_WINDOWS_OS_PREFIX_SERVER_NAME) - 1);
            size_t version_length = strspn(digits, "0123456789");
            if (version_length) {
                snprintf(labels->version, sizeof(labels->version), "%.*s", (int)version_length, digits);
                version_source = NULL; // override the build-derived fallback below
            }
        }
    }

    if (!labels->version[0] && version_source)
        snprintf(labels->version, sizeof(labels->version), "%s", version_source);

    if (display_version && *display_version)
        snprintf(labels->release, sizeof(labels->release), "%s", display_version);

    netdata_windows_product_edition(labels->edition, sizeof(labels->edition), product_name, is_server);
    if (!labels->edition[0] && edition_id && *edition_id)
        snprintf(labels->edition, sizeof(labels->edition), "%s", edition_id);

    if (build && has_ubr)
        snprintf(labels->build, sizeof(labels->build), "%u.%u", build, ubr);
}

void netdata_windows_format_os_version(char *out, size_t length, const char *product_name, DWORD build, bool is_server)
{
    if (!length)
        return;

    if (!product_name || !*product_name) {
        (void)snprintf(out, length, "Microsoft Windows");
        return;
    }

    const char *name = product_name;
    if (!strncasecmp(name,
                     NETDATA_WINDOWS_MICROSOFT_PREFIX,
                     sizeof(NETDATA_WINDOWS_MICROSOFT_PREFIX) - 1))
        name += (sizeof(NETDATA_WINDOWS_MICROSOFT_PREFIX) - 1);

    size_t windows_version_length = sizeof("Windows 10") - 1;
    if (!is_server && build >= 10240 &&
        (!strncasecmp(name, "Windows 10", windows_version_length) ||
         !strncasecmp(name, "Windows 11", windows_version_length)) &&
        (name[windows_version_length] == '\0' || name[windows_version_length] == ' ')) {
        (void)snprintf(out, length, "Microsoft Windows %s%s", build >= 22000 ? "11" : "10", name + windows_version_length);
        return;
    }

    (void)snprintf(out, length, "Microsoft %s", name);
}

static void netdata_windows_discover_os_version(char *os, size_t length, DWORD build,
                                                const char *product_name, const char *display_version)
{
    if (product_name && *product_name) {
        netdata_windows_format_os_version(os, length, product_name, build, IsWindowsServer());
        return;
    }

    if (!display_version || !*display_version) {
        (void)snprintf(os, length, "Microsoft Windows");
        return;
    }

    if (IsWindowsServer()) {
        (void)snprintf(os, length, "Microsoft Windows Version %s", display_version);
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

void netdata_windows_format_os_id_like(char *out, size_t length, DWORD build, const char *edition_id,
                                       bool is_server)
{
    if (!length)
        return;

    const char *version = is_server ? netdata_windows_server_version_from_build(build)
                                    : netdata_windows_client_version_from_build(build);
    char base_id[64];

    if (*version) {
        (void)snprintf(base_id, sizeof(base_id), "%s-%s",
                       is_server ? "Windows-Server" : NETDATA_WINDOWS_OS_PREFIX_CLIENT,
                       version);
    } else {
        (void)snprintf(base_id, sizeof(base_id), "%s",
                       is_server ? "Windows-Server" : NETDATA_WINDOWS_OS_PREFIX_CLIENT);
    }

    if (edition_id && *edition_id)
        (void)snprintf(out, length, "%s-%s", base_id, edition_id);
    else
        (void)snprintf(out, length, "%s", base_id);
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

    char product_name[256] = {0};
    char display_version[64] = {0};
    char edition_id[256] = {0};
    DWORD ubr = 0;
    DWORD build = netdata_windows_get_current_build();
    bool has_ubr = netdata_windows_get_update_revision(&ubr);
    (void)netdata_registry_get_string(product_name, sizeof(product_name) - 1, HKEY_LOCAL_MACHINE,
                                      "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", "ProductName");
    (void)netdata_registry_get_string(display_version, sizeof(display_version) - 1, HKEY_LOCAL_MACHINE,
                                      "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", "DisplayVersion");
    (void)netdata_registry_get_string(edition_id, sizeof(edition_id) - 1, HKEY_LOCAL_MACHINE,
                                      "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion", "EditionID");

    netdata_windows_discover_os_version(info->id, sizeof(info->id), build, product_name, display_version);
    snprintf(info->version, sizeof(info->version), "%s", info->id);
    snprintf(info->version_id, sizeof(info->version_id), "%s", info->id);
    netdata_windows_format_os_id_like(info->id_like, sizeof(info->id_like), build, edition_id, IsWindowsServer());
    snprintf(info->detection, sizeof(info->detection), "%s", NETDATA_WIN_DETECTION_METHOD);
    netdata_windows_parse_os_labels(&info->labels, product_name, display_version, edition_id,
                                    build, ubr, has_ubr, IsWindowsServer());
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

    const struct {
        const char *key;
        const char *value;
    } label_fields[] = {
        { "NETDATA_HOST_OS_LABEL_NAME",    info->labels.name    },
        { "NETDATA_HOST_OS_LABEL_VERSION", info->labels.version },
        { "NETDATA_HOST_OS_LABEL_RELEASE", info->labels.release },
        { "NETDATA_HOST_OS_LABEL_EDITION", info->labels.edition },
        { "NETDATA_HOST_OS_LABEL_BUILD",   info->labels.build   },
    };
    for (size_t i = 0; i < sizeof(label_fields) / sizeof(label_fields[0]); i++) {
        if (label_fields[i].value[0])
            (void)rrdhost_system_info_set_by_name(systemInfo, label_fields[i].key, label_fields[i].value);
    }
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

static const char *netdata_windows_container_is_official_image(void)
{
    const char *official = getenv("NETDATA_OFFICIAL_IMAGE");
    if(official && *official)
        return official;

    return NETDATA_DEFAULT_SYSTEM_INFO_VALUE_FALSE;
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
    // environment. On Linux the system-info.sh dispatch loop nd_setenv()s it — mirror that
    // here so the flag is not silently dropped on Windows.
    const char *official_image = netdata_windows_container_is_official_image();
    (void)rrdhost_system_info_set_by_name(
        systemInfo, "NETDATA_CONTAINER_IS_OFFICIAL_IMAGE", official_image);
    nd_setenv("NETDATA_CONTAINER_IS_OFFICIAL_IMAGE", official_image, 1);
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
    nd_setenv("NETDATA_SYSTEM_VIRTUALIZATION", virt, 1);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_VIRT_DETECTION", NETDATA_WIN_DETECTION_METHOD);
    nd_setenv("NETDATA_SYSTEM_VIRT_DETECTION", NETDATA_WIN_DETECTION_METHOD, 1);
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
    const char *from_env = netdata_windows_container_from_env(
        getenv("KUBERNETES_SERVICE_HOST"), getenv("KUBERNETES_SERVICE_PORT"));
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
    nd_setenv("NETDATA_SYSTEM_CONTAINER", container, 1);

    (void)rrdhost_system_info_set_by_name(systemInfo, "NETDATA_SYSTEM_CONTAINER_DETECTION", container_detection);
    nd_setenv("NETDATA_SYSTEM_CONTAINER_DETECTION", container_detection, 1);

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
