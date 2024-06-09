#include "win_system-info.h"

#ifdef OS_WINDOWS

#include "win_system-info.h"

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
        (void)snprintf(os, ND_WIN_VER_LENGTH, "10");
    } else if (IsWindows8Point1OrGreater()) {
        (void)snprintf(os, ND_WIN_VER_LENGTH, "8.1");
    } else if (IsWindows8OrGreater()) {
        (void)snprintf(os, ND_WIN_VER_LENGTH, "8");
    } else if (IsWindows7SP1OrGreater()) {
        (void)snprintf(os, ND_WIN_VER_LENGTH, "7 SP1");
    } else if (IsWindows7OrGreater()) {
        (void)snprintf(os, ND_WIN_VER_LENGTH, "7");
    } else if (IsWindowsVistaSP2OrGreater()) {
        (void)snprintf(os, ND_WIN_VER_LENGTH, "Vista SP2");
    } else if (IsWindowsVistaSP1OrGreater()) {
        (void)snprintf(os, ND_WIN_VER_LENGTH, "Vista SP1");
    } else if (IsWindowsVistaOrGreater()) {
        (void)snprintf(os, ND_WIN_VER_LENGTH, "Vista");
    }
	// We are not testing older, because it is not supported anymore by Microsoft

    (void)snprintf(os, length, "%s Client %s", commonName, version);
}

static inline void netdata_windows_host(struct rrdhost_system_info *systemInfo, char *defaultValue) {
	char temp[4096];
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_NAME", "Windows");

    netdata_windows_discover_os_version(temp, 4095);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_ID", temp);

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_ID_LIKE", defaultValue);

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_VERSION", temp);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_VERSION_ID", temp);

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_HOST_OS_DETECTION", "Windows API");

    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_SYSTEM_KERNEL_NAME", defaultValue);
}

// Cloud
static inline void netdata_windows_cloud(struct rrdhost_system_info *systemInfo, char *defaultValue) {
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_INSTANCE_CLOUD_TYPE", defaultValue);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE", defaultValue);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_INSTANCE_CLOUD_INSTANCE_REGION", defaultValue);
}

// Container
static inline void netdata_windows_container(struct rrdhost_system_info *systemInfo, char *defaultValue) {
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_CONTAINER_OS_NAME", defaultValue);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_CONTAINER_OS_ID", defaultValue);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_CONTAINER_OS_ID_LIKE", defaultValue);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_CONTAINER_OS_VERSION", defaultValue);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_CONTAINER_OS_VERSION_ID", defaultValue);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_CONTAINER_OS_DETECTION", defaultValue);
}

void netdata_windows_get_system_info(struct rrdhost_system_info *systemInfo) {
    static char *unknowValue = { "unknown" };

    netdata_windows_cloud(systemInfo, unknowValue);
    netdata_windows_container(systemInfo, unknowValue);
    netdata_windows_host(systemInfo, unknowValue);

}
#endif
