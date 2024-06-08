#include "win_system-info.h"

#ifdef OS_WINDOWS
static inline void netdata_windows_cloud(struct rrdhost_system_info *systemInfo, char *defaultValue) {
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_INSTANCE_CLOUD_TYPE", defaultValue);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE", defaultValue);
    (void)rrdhost_set_system_info_variable(systemInfo, "NETDATA_INSTANCE_CLOUD_INSTANCE_REGION", defaultValue);
}

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
}
#endif
