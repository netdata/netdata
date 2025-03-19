// SPDX-License-Identifier: GPL-3.0-or-later

#define RRDHOST_SYSTEM_INFO_INTERNALS
#include "rrdhost-system-info.h"
#include "aclk/schema-wrappers/node_info.h"
#include "daemon/win_system-info.h"

// coverity[ +tainted_string_sanitize_content : arg-0 ]
static inline void coverity_remove_taint(char *s __maybe_unused) { }

void rrdhost_system_info_swap(struct rrdhost_system_info *a, struct rrdhost_system_info *b) {
    if(a && b)
        SWAP(*a, *b);
}

// ----------------------------------------------------------------------------
// RRDHOST - set system info from environment variables
// system_info fields must be heap allocated or NULL
int rrdhost_system_info_set_by_name(struct rrdhost_system_info *system_info, char *name, char *value) {
    int res = 0;

    if (!strcmp(name, "NETDATA_PROTOCOL_VERSION"))
        return res;

    else if(!strcmp(name, "NETDATA_INSTANCE_CLOUD_TYPE")){
        freez(system_info->cloud_provider_type);
        system_info->cloud_provider_type = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE")){
        freez(system_info->cloud_instance_type);
        system_info->cloud_instance_type = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_INSTANCE_CLOUD_INSTANCE_REGION")){
        freez(system_info->cloud_instance_region);
        system_info->cloud_instance_region = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_NAME")){
        freez(system_info->container_os_name);
        system_info->container_os_name = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_ID")){
        freez(system_info->container_os_id);
        system_info->container_os_id = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_ID_LIKE")){
        freez(system_info->container_os_id_like);
        system_info->container_os_id_like = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_VERSION")){
        freez(system_info->container_os_version);
        system_info->container_os_version = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_VERSION_ID")){
        freez(system_info->container_os_version_id);
        system_info->container_os_version_id = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_DETECTION")){
        freez(system_info->container_os_detection);
        system_info->container_os_detection = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_NAME")){
        freez(system_info->host_os_name);
        system_info->host_os_name = strdupz(value);
        json_fix_string(system_info->host_os_name);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_ID")){
        freez(system_info->host_os_id);
        system_info->host_os_id = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_ID_LIKE")){
        freez(system_info->host_os_id_like);
        system_info->host_os_id_like = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_VERSION")){
        freez(system_info->host_os_version);
        system_info->host_os_version = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_VERSION_ID")){
        freez(system_info->host_os_version_id);
        system_info->host_os_version_id = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_DETECTION")){
        freez(system_info->host_os_detection);
        system_info->host_os_detection = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_KERNEL_NAME")){
        freez(system_info->kernel_name);
        system_info->kernel_name = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT")){
        freez(system_info->host_cores);
        system_info->host_cores = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_CPU_FREQ")){
        freez(system_info->host_cpu_freq);
        system_info->host_cpu_freq = strdupz(value);
    }
    else if (!strcmp(name, "NETDATA_SYSTEM_CPU_MODEL")){
        freez(system_info->host_cpu_model);
        system_info->host_cpu_model = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_TOTAL_RAM")){
        freez(system_info->host_ram_total);
        system_info->host_ram_total = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_TOTAL_DISK_SIZE")){
        freez(system_info->host_disk_space);
        system_info->host_disk_space = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_KERNEL_VERSION")){
        freez(system_info->kernel_version);
        system_info->kernel_version = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_ARCHITECTURE")){
        freez(system_info->architecture);
        system_info->architecture = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_VIRTUALIZATION")){
        freez(system_info->virtualization);
        system_info->virtualization = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_VIRT_DETECTION")){
        freez(system_info->virt_detection);
        system_info->virt_detection = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_CONTAINER")){
        freez(system_info->container);
        system_info->container = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_CONTAINER_DETECTION")){
        freez(system_info->container_detection);
        system_info->container_detection = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_IS_K8S_NODE")){
        freez(system_info->is_k8s_node);
        system_info->is_k8s_node = strdupz(value);
    }
    else if (!strcmp(name, "NETDATA_SYSTEM_CPU_VENDOR"))
        return res;
    else if (!strcmp(name, "NETDATA_SYSTEM_CPU_DETECTION"))
        return res;
    else if (!strcmp(name, "NETDATA_SYSTEM_RAM_DETECTION"))
        return res;
    else if (!strcmp(name, "NETDATA_SYSTEM_DISK_DETECTION"))
        return res;
    else if (!strcmp(name, "NETDATA_CONTAINER_IS_OFFICIAL_IMAGE"))
        return res;
    else {
        res = 1;
    }

    return res;
}

struct rrdhost_system_info *rrdhost_system_info_from_host_labels(RRDLABELS *labels) {
    struct rrdhost_system_info *info = rrdhost_system_info_create();
    info->hops = 1;

    rrdlabels_get_value_strdup_or_null(labels, &info->cloud_provider_type, "_cloud_provider_type");
    rrdlabels_get_value_strdup_or_null(labels, &info->cloud_instance_type, "_cloud_instance_type");
    rrdlabels_get_value_strdup_or_null(labels, &info->cloud_instance_region, "_cloud_instance_region");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_os_name, "_os_name");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_os_version, "_os_version");
    rrdlabels_get_value_strdup_or_null(labels, &info->kernel_version, "_kernel_version");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_cores, "_system_cores");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_cpu_freq, "_system_cpu_freq");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_cpu_model, "_system_cpu_model");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_ram_total, "_system_ram_total");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_disk_space, "_system_disk_space");
    rrdlabels_get_value_strdup_or_null(labels, &info->architecture, "_architecture");
    rrdlabels_get_value_strdup_or_null(labels, &info->virtualization, "_virtualization");
    rrdlabels_get_value_strdup_or_null(labels, &info->container, "_container");
    rrdlabels_get_value_strdup_or_null(labels, &info->container_detection, "_container_detection");
    rrdlabels_get_value_strdup_or_null(labels, &info->virt_detection, "_virt_detection");
    rrdlabels_get_value_strdup_or_null(labels, &info->is_k8s_node, "_is_k8s_node");
    rrdlabels_get_value_strdup_or_null(labels, &info->install_type, "_install_type");
    rrdlabels_get_value_strdup_or_null(labels, &info->prebuilt_arch, "_prebuilt_arch");
    rrdlabels_get_value_strdup_or_null(labels, &info->prebuilt_dist, "_prebuilt_dist");

    return info;
}

void rrdhost_system_info_to_rrdlabels(struct rrdhost_system_info *system_info, RRDLABELS *labels) {
    if (system_info->cloud_provider_type)
        rrdlabels_add(labels, "_cloud_provider_type", system_info->cloud_provider_type, RRDLABEL_SRC_AUTO);

    if (system_info->cloud_instance_type)
        rrdlabels_add(labels, "_cloud_instance_type", system_info->cloud_instance_type, RRDLABEL_SRC_AUTO);

    if (system_info->cloud_instance_region)
        rrdlabels_add(labels, "_cloud_instance_region", system_info->cloud_instance_region, RRDLABEL_SRC_AUTO);

    if (system_info->host_os_name)
        rrdlabels_add(labels, "_os_name", system_info->host_os_name, RRDLABEL_SRC_AUTO);

    if (system_info->host_os_version)
        rrdlabels_add(labels, "_os_version", system_info->host_os_version, RRDLABEL_SRC_AUTO);

    if (system_info->kernel_version)
        rrdlabels_add(labels, "_kernel_version", system_info->kernel_version, RRDLABEL_SRC_AUTO);

    if (system_info->host_cores)
        rrdlabels_add(labels, "_system_cores", system_info->host_cores, RRDLABEL_SRC_AUTO);

    if (system_info->host_cpu_freq)
        rrdlabels_add(labels, "_system_cpu_freq", system_info->host_cpu_freq, RRDLABEL_SRC_AUTO);

    if (system_info->host_cpu_model)
        rrdlabels_add(labels, "_system_cpu_model", system_info->host_cpu_model, RRDLABEL_SRC_AUTO);

    if (system_info->host_ram_total)
        rrdlabels_add(labels, "_system_ram_total", system_info->host_ram_total, RRDLABEL_SRC_AUTO);

    if (system_info->host_disk_space)
        rrdlabels_add(labels, "_system_disk_space", system_info->host_disk_space, RRDLABEL_SRC_AUTO);

    if (system_info->architecture)
        rrdlabels_add(labels, "_architecture", system_info->architecture, RRDLABEL_SRC_AUTO);

    if (system_info->virtualization)
        rrdlabels_add(labels, "_virtualization", system_info->virtualization, RRDLABEL_SRC_AUTO);

    if (system_info->container)
        rrdlabels_add(labels, "_container", system_info->container, RRDLABEL_SRC_AUTO);

    if (system_info->container_detection)
        rrdlabels_add(labels, "_container_detection", system_info->container_detection, RRDLABEL_SRC_AUTO);

    if (system_info->virt_detection)
        rrdlabels_add(labels, "_virt_detection", system_info->virt_detection, RRDLABEL_SRC_AUTO);

    if (system_info->is_k8s_node)
        rrdlabels_add(labels, "_is_k8s_node", system_info->is_k8s_node, RRDLABEL_SRC_AUTO);

    if (system_info->install_type)
        rrdlabels_add(labels, "_install_type", system_info->install_type, RRDLABEL_SRC_AUTO);

    if (system_info->prebuilt_arch)
        rrdlabels_add(labels, "_prebuilt_arch", system_info->prebuilt_arch, RRDLABEL_SRC_AUTO);

    if (system_info->prebuilt_dist)
        rrdlabels_add(labels, "_prebuilt_dist", system_info->prebuilt_dist, RRDLABEL_SRC_AUTO);
}

int rrdhost_system_info_detect(struct rrdhost_system_info *system_info) {
#if !defined(OS_WINDOWS)
    if (unlikely(!system_info)) {
        netdata_log_error("SYSTEM INFO: System info structure is NULL.");
        return 1;
    }

    CLEAN_BUFFER *script = buffer_create(0, NULL);
    buffer_sprintf(script, "%s/system-info.sh", netdata_configured_primary_plugins_dir);

    POPEN_INSTANCE *instance = NULL;
    int ret = 1;

    // Check if script exists and is readable
    if (unlikely(access(buffer_tostring(script), R_OK) != 0)) {
        netdata_log_error("SYSTEM INFO: System info script %s not found or not readable.",
                          buffer_tostring(script));
        goto cleanup;
    }

    // Run the script
    instance = spawn_popen_run(buffer_tostring(script));
    if (unlikely(!instance)) {
        netdata_log_error("SYSTEM INFO: Failed to execute system info script %s.",
                          buffer_tostring(script));
        goto cleanup;
    }

    char line[1024];
    FILE *fp = spawn_popen_stdout(instance);
    if (unlikely(!fp)) {
        netdata_log_error("SYSTEM INFO: Failed to get stdout from system info script.");
        goto cleanup;
    }

    // Process each line from the script output
    while (fgets(line, sizeof(line) - 1, fp) != NULL) {
        // Ensure null-termination
        line[sizeof(line) - 1] = '\0';

        // Find the equals sign separator
        char *value = strchr(line, '=');
        if (unlikely(!value)) {
            // Skip lines without an equal sign
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "SYSTEM INFO: Skipping malformed line from system-info.sh (no '=' found): '%s'",
                   line);
            continue;
        }

        // Split the name and value
        *value = '\0';
        value++;

        // Trim any trailing newline from the value
        char *end = strchr(value, '\n');
        if (end) *end = '\0';

        // Remove any carriage return that might be present (especially for macOS)
        end = strchr(value, '\r');
        if (end) *end = '\0';

        // Validate name and value
        if (unlikely(!*line || !*value)) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "SYSTEM INFO: Skipping empty name or value from system-info.sh: '%s=%s'",
                   line, value);
            continue;
        }

        // Process the name-value pair
        coverity_remove_taint(line);
        coverity_remove_taint(value);

        if (unlikely(rrdhost_system_info_set_by_name(system_info, line, value))) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "SYSTEM INFO: Unexpected variable '%s=%s'",
                   line, value);
        } else {
            // Only set as environment variable if it was successfully processed
            nd_setenv(line, value, 1);
        }
    }

    // Everything succeeded
    ret = 0;

cleanup:
    // Clean up resources
    if (instance)
        spawn_popen_wait(instance);

    return ret;
#else
    netdata_windows_get_system_info(system_info);
    return 0;
#endif
}

void rrdhost_system_info_free(struct rrdhost_system_info *system_info) {
    if(likely(system_info)) {
        __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);

        freez(system_info->cloud_provider_type);
        freez(system_info->cloud_instance_type);
        freez(system_info->cloud_instance_region);
        freez(system_info->host_os_name);
        freez(system_info->host_os_id);
        freez(system_info->host_os_id_like);
        freez(system_info->host_os_version);
        freez(system_info->host_os_version_id);
        freez(system_info->host_os_detection);
        freez(system_info->host_cores);
        freez(system_info->host_cpu_freq);
        freez(system_info->host_cpu_model);
        freez(system_info->host_ram_total);
        freez(system_info->host_disk_space);
        freez(system_info->container_os_name);
        freez(system_info->container_os_id);
        freez(system_info->container_os_id_like);
        freez(system_info->container_os_version);
        freez(system_info->container_os_version_id);
        freez(system_info->container_os_detection);
        freez(system_info->kernel_name);
        freez(system_info->kernel_version);
        freez(system_info->architecture);
        freez(system_info->virtualization);
        freez(system_info->virt_detection);
        freez(system_info->container);
        freez(system_info->container_detection);
        freez(system_info->is_k8s_node);
        freez(system_info->install_type);
        freez(system_info->prebuilt_arch);
        freez(system_info->prebuilt_dist);
        freez(system_info);
    }
}

struct rrdhost_system_info *rrdhost_system_info_create(void) {
    struct rrdhost_system_info *system_info = callocz(1, sizeof(struct rrdhost_system_info));
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);
    return system_info;
}

const char *rrdhost_system_info_install_type(struct rrdhost_system_info *si) {
    return si->install_type;
}

const char *rrdhost_system_info_prebuilt_dist(struct rrdhost_system_info *si) {
    return si->prebuilt_dist;
}

int16_t rrdhost_system_info_hops(struct rrdhost_system_info *si) {
    if(!si) return 0;
    return si->hops;
}

void rrdhost_system_info_hops_set(struct rrdhost_system_info *si, int16_t hops) {
    si->hops = hops;
}

void rrdhost_system_info_to_json_v1(BUFFER *wb, struct rrdhost_system_info *system_info) {
    if(!system_info) return;
    
    buffer_json_member_add_string_or_empty(wb, "os_name", system_info->host_os_name);
    buffer_json_member_add_string_or_empty(wb, "os_id", system_info->host_os_id);
    buffer_json_member_add_string_or_empty(wb, "os_id_like", system_info->host_os_id_like);
    buffer_json_member_add_string_or_empty(wb, "os_version", system_info->host_os_version);
    buffer_json_member_add_string_or_empty(wb, "os_version_id", system_info->host_os_version_id);
    buffer_json_member_add_string_or_empty(wb, "os_detection", system_info->host_os_detection);
    buffer_json_member_add_string_or_empty(wb, "cores_total", system_info->host_cores);
    buffer_json_member_add_string_or_empty(wb, "total_disk_space", system_info->host_disk_space);
    buffer_json_member_add_string_or_empty(wb, "cpu_freq", system_info->host_cpu_freq);
    buffer_json_member_add_string_or_empty(wb, "ram_total", system_info->host_ram_total);
    
    buffer_json_member_add_string_or_omit(wb, "container_os_name", system_info->container_os_name);
    buffer_json_member_add_string_or_omit(wb, "container_os_id", system_info->container_os_id);
    buffer_json_member_add_string_or_omit(wb, "container_os_id_like", system_info->container_os_id_like);
    buffer_json_member_add_string_or_omit(wb, "container_os_version", system_info->container_os_version);
    buffer_json_member_add_string_or_omit(wb, "container_os_version_id", system_info->container_os_version_id);
    buffer_json_member_add_string_or_omit(wb, "container_os_detection", system_info->container_os_detection);
    buffer_json_member_add_string_or_omit(wb, "is_k8s_node", system_info->is_k8s_node);

    buffer_json_member_add_string_or_empty(wb, "kernel_name", system_info->kernel_name);
    buffer_json_member_add_string_or_empty(wb, "kernel_version", system_info->kernel_version);
    buffer_json_member_add_string_or_empty(wb, "architecture", system_info->architecture);
    buffer_json_member_add_string_or_empty(wb, "virtualization", system_info->virtualization);
    buffer_json_member_add_string_or_empty(wb, "virt_detection", system_info->virt_detection);
    buffer_json_member_add_string_or_empty(wb, "container", system_info->container);
    buffer_json_member_add_string_or_empty(wb, "container_detection", system_info->container_detection);

    buffer_json_member_add_string_or_omit(wb, "cloud_provider_type", system_info->cloud_provider_type);
    buffer_json_member_add_string_or_omit(wb, "cloud_instance_type", system_info->cloud_instance_type);
    buffer_json_member_add_string_or_omit(wb, "cloud_instance_region", system_info->cloud_instance_region);
}

void rrdhost_system_info_to_json_v2(BUFFER *wb, struct rrdhost_system_info *system_info) {
    if(!system_info) return;

    buffer_json_member_add_object(wb, "hw");
    {
        buffer_json_member_add_string_or_empty(wb, "architecture", system_info->architecture);
        buffer_json_member_add_string_or_empty(wb, "cpu_frequency", system_info->host_cpu_freq);
        buffer_json_member_add_string_or_empty(wb, "cpus", system_info->host_cores);
        buffer_json_member_add_string_or_empty(wb, "memory", system_info->host_ram_total);
        buffer_json_member_add_string_or_empty(wb, "disk_space", system_info->host_disk_space);
        buffer_json_member_add_string_or_empty(wb, "virtualization", system_info->virtualization);
        buffer_json_member_add_string_or_empty(wb, "container", system_info->container);
    }
    buffer_json_object_close(wb);
    
    buffer_json_member_add_object(wb, "os");
    {
        buffer_json_member_add_string_or_empty(wb, "id", system_info->host_os_id);
        buffer_json_member_add_string_or_empty(wb, "nm", system_info->host_os_name);
        buffer_json_member_add_string_or_empty(wb, "v", system_info->host_os_version);
        buffer_json_member_add_object(wb, "kernel");
        buffer_json_member_add_string_or_empty(wb, "nm", system_info->kernel_name);
        buffer_json_member_add_string_or_empty(wb, "v", system_info->kernel_version);
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb);
}

void rrdhost_system_info_ml_capable_set(struct rrdhost_system_info *system_info, bool capable) {
    system_info->ml_capable = capable;
}

void rrdhost_system_info_ml_enabled_set(struct rrdhost_system_info *system_info, bool enabled) {
    system_info->ml_enabled = enabled;
}

void rrdhost_system_info_mc_version_set(struct rrdhost_system_info *system_info, int version) {
    system_info->mc_version = version;
}

int rrdhost_system_info_foreach(struct rrdhost_system_info *system_info, add_host_sysinfo_key_value_t cb, nd_uuid_t *uuid) {
    int ret = 0;

    ret += cb("NETDATA_CONTAINER_OS_NAME", system_info->container_os_name, uuid);
    ret += cb("NETDATA_CONTAINER_OS_ID", system_info->container_os_id, uuid);
    ret += cb("NETDATA_CONTAINER_OS_ID_LIKE", system_info->container_os_id_like, uuid);
    ret += cb("NETDATA_CONTAINER_OS_VERSION", system_info->container_os_version, uuid);
    ret += cb("NETDATA_CONTAINER_OS_VERSION_ID", system_info->container_os_version_id, uuid);
    ret += cb("NETDATA_CONTAINER_OS_DETECTION", system_info->host_os_detection, uuid);
    ret += cb("NETDATA_HOST_OS_NAME", system_info->host_os_name, uuid);
    ret += cb("NETDATA_HOST_OS_ID", system_info->host_os_id, uuid);
    ret += cb("NETDATA_HOST_OS_ID_LIKE", system_info->host_os_id_like, uuid);
    ret += cb("NETDATA_HOST_OS_VERSION", system_info->host_os_version, uuid);
    ret += cb("NETDATA_HOST_OS_VERSION_ID", system_info->host_os_version_id, uuid);
    ret += cb("NETDATA_HOST_OS_DETECTION", system_info->host_os_detection, uuid);
    ret += cb("NETDATA_SYSTEM_KERNEL_NAME", system_info->kernel_name, uuid);
    ret += cb("NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", system_info->host_cores, uuid);
    ret += cb("NETDATA_SYSTEM_CPU_FREQ", system_info->host_cpu_freq, uuid);
    ret += cb("NETDATA_SYSTEM_TOTAL_RAM", system_info->host_ram_total, uuid);
    ret += cb("NETDATA_SYSTEM_TOTAL_DISK_SIZE", system_info->host_disk_space, uuid);
    ret += cb("NETDATA_SYSTEM_KERNEL_VERSION", system_info->kernel_version, uuid);
    ret += cb("NETDATA_SYSTEM_ARCHITECTURE", system_info->architecture, uuid);
    ret += cb("NETDATA_SYSTEM_VIRTUALIZATION", system_info->virtualization, uuid);
    ret += cb("NETDATA_SYSTEM_VIRT_DETECTION", system_info->virt_detection, uuid);
    ret += cb("NETDATA_SYSTEM_CONTAINER", system_info->container, uuid);
    ret += cb("NETDATA_SYSTEM_CONTAINER_DETECTION", system_info->container_detection, uuid);
    ret += cb("NETDATA_HOST_IS_K8S_NODE", system_info->is_k8s_node, uuid);

    return ret;
}

void rrdhost_system_info_to_url_encode_stream(BUFFER *wb, struct rrdhost_system_info *system_info) {
    buffer_sprintf(wb, "&ml_capable=%d", system_info->ml_capable ? 1 : 0);
    buffer_sprintf(wb, "&ml_enabled=%d", system_info->ml_enabled ? 1 : 0);
    buffer_sprintf(wb, "&mc_version=%d", system_info->mc_version);
    buffer_key_value_urlencode(wb, "&NETDATA_INSTANCE_CLOUD_TYPE", system_info->cloud_provider_type);
    buffer_key_value_urlencode(wb, "&NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE", system_info->cloud_instance_type);
    buffer_key_value_urlencode(wb, "&NETDATA_INSTANCE_CLOUD_INSTANCE_REGION", system_info->cloud_instance_region);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_OS_NAME", system_info->host_os_name);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_OS_ID", system_info->host_os_id);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_OS_ID_LIKE", system_info->host_os_id_like);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_OS_VERSION", system_info->host_os_version);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_OS_VERSION_ID", system_info->host_os_version_id);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_OS_DETECTION", system_info->host_os_detection);
    buffer_key_value_urlencode(wb, "&NETDATA_HOST_IS_K8S_NODE", system_info->is_k8s_node);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_KERNEL_NAME", system_info->kernel_name);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_KERNEL_VERSION", system_info->kernel_version);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_ARCHITECTURE", system_info->architecture);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_VIRTUALIZATION", system_info->virtualization);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_VIRT_DETECTION", system_info->virt_detection);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_CONTAINER", system_info->container);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_CONTAINER_DETECTION", system_info->container_detection);
    buffer_key_value_urlencode(wb, "&NETDATA_CONTAINER_OS_NAME", system_info->container_os_name);
    buffer_key_value_urlencode(wb, "&NETDATA_CONTAINER_OS_ID", system_info->container_os_id);
    buffer_key_value_urlencode(wb, "&NETDATA_CONTAINER_OS_ID_LIKE", system_info->container_os_id_like);
    buffer_key_value_urlencode(wb, "&NETDATA_CONTAINER_OS_VERSION", system_info->container_os_version);
    buffer_key_value_urlencode(wb, "&NETDATA_CONTAINER_OS_VERSION_ID", system_info->container_os_version_id);
    buffer_key_value_urlencode(wb, "&NETDATA_CONTAINER_OS_DETECTION", system_info->container_os_detection);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", system_info->host_cores);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_CPU_FREQ", system_info->host_cpu_freq);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_TOTAL_RAM", system_info->host_ram_total);
    buffer_key_value_urlencode(wb, "&NETDATA_SYSTEM_TOTAL_DISK_SIZE", system_info->host_disk_space);
}

void rrdhost_system_info_to_node_info(struct rrdhost_system_info *system_info, struct update_node_info *node_info) {
    node_info->data.os_name = system_info->host_os_name;
    node_info->data.os_version = system_info->host_os_version;
    node_info->data.kernel_name = system_info->kernel_name;
    node_info->data.kernel_version = system_info->kernel_version;
    node_info->data.architecture = system_info->architecture;
    node_info->data.cpus = system_info->host_cores ? str2uint32_t(system_info->host_cores, NULL) : 0;
    node_info->data.cpu_frequency = system_info->host_cpu_freq ? system_info->host_cpu_freq : "0";
    node_info->data.memory = system_info->host_ram_total ? system_info->host_ram_total : "0";
    node_info->data.disk_space = system_info->host_disk_space ? system_info->host_disk_space : "0";
    node_info->data.virtualization_type = system_info->virtualization ? system_info->virtualization : "unknown";
    node_info->data.container_type = system_info->container ? system_info->container : "unknown";
    node_info->data.ml_info.ml_capable = system_info->ml_capable;
    node_info->data.ml_info.ml_enabled = system_info->ml_enabled;
}

void rrdhost_system_info_to_streaming_function_array(BUFFER *wb, struct rrdhost_system_info *system_info) {
    if(system_info) {
        buffer_json_add_array_item_string(wb, system_info->host_os_name ? system_info->host_os_name : "");
        buffer_json_add_array_item_string(wb, system_info->host_os_id ? system_info->host_os_id : "");
        buffer_json_add_array_item_string(wb, system_info->host_os_id_like ? system_info->host_os_id_like : "");
        buffer_json_add_array_item_string(wb, system_info->host_os_version ? system_info->host_os_version : "");
        buffer_json_add_array_item_string(wb, system_info->host_os_version_id ? system_info->host_os_version_id : "");
        buffer_json_add_array_item_string(wb, system_info->host_os_detection ? system_info->host_os_detection : "");
        buffer_json_add_array_item_string(wb, system_info->host_cores ? system_info->host_cores : "");
        buffer_json_add_array_item_string(wb, system_info->host_disk_space ? system_info->host_disk_space : "");
        buffer_json_add_array_item_string(wb, system_info->host_cpu_freq ? system_info->host_cpu_freq : "");
        buffer_json_add_array_item_string(wb, system_info->host_ram_total ? system_info->host_ram_total : "");
        buffer_json_add_array_item_string(wb, system_info->container_os_name ? system_info->container_os_name : "");
        buffer_json_add_array_item_string(wb, system_info->container_os_id ? system_info->container_os_id : "");
        buffer_json_add_array_item_string(wb, system_info->container_os_id_like ? system_info->container_os_id_like : "");
        buffer_json_add_array_item_string(wb, system_info->container_os_version ? system_info->container_os_version : "");
        buffer_json_add_array_item_string(wb, system_info->container_os_version_id ? system_info->container_os_version_id : "");
        buffer_json_add_array_item_string(wb, system_info->container_os_detection ? system_info->container_os_detection : "");
        buffer_json_add_array_item_string(wb, system_info->is_k8s_node ? system_info->is_k8s_node : "");
        buffer_json_add_array_item_string(wb, system_info->kernel_name ? system_info->kernel_name : "");
        buffer_json_add_array_item_string(wb, system_info->kernel_version ? system_info->kernel_version : "");
        buffer_json_add_array_item_string(wb, system_info->architecture ? system_info->architecture : "");
        buffer_json_add_array_item_string(wb, system_info->virtualization ? system_info->virtualization : "");
        buffer_json_add_array_item_string(wb, system_info->virt_detection ? system_info->virt_detection : "");
        buffer_json_add_array_item_string(wb, system_info->container ? system_info->container : "");
        buffer_json_add_array_item_string(wb, system_info->container_detection ? system_info->container_detection : "");
        buffer_json_add_array_item_string(wb, system_info->cloud_provider_type ? system_info->cloud_provider_type : "");
        buffer_json_add_array_item_string(wb, system_info->cloud_instance_type ? system_info->cloud_instance_type : "");
        buffer_json_add_array_item_string(wb, system_info->cloud_instance_region ? system_info->cloud_instance_region : "");
    }
    else {
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
        buffer_json_add_array_item_string(wb, "");
    }
}

void get_daemon_status_fields_from_system_info(DAEMON_STATUS_FILE *ds) {
    if(ds->read_system_info) return;

    struct rrdhost_system_info *ri = (localhost && localhost->system_info) ? localhost->system_info : NULL;
    if(!ri) {
        // nothing we can do, let it be
        return;
    }

    if(ri->architecture)
        strncpyz(ds->architecture, ri->architecture, sizeof(ds->architecture) - 1);

    if(ri->virtualization)
        strncpyz(ds->virtualization, ri->virtualization, sizeof(ds->virtualization) - 1);

    if(ri->container)
        strncpyz(ds->container, ri->container, sizeof(ds->container) - 1);

    if(ri->kernel_version)
        strncpyz(ds->kernel_version, ri->kernel_version, sizeof(ds->kernel_version) - 1);

    if(ri->host_os_name)
        strncpyz(ds->os_name, ri->host_os_name, sizeof(ds->os_name) - 1);

    if(ri->host_os_version)
        strncpyz(ds->os_version, ri->host_os_version, sizeof(ds->os_version) - 1);

    if(ri->host_os_id)
        strncpyz(ds->os_id, ri->host_os_id, sizeof(ds->os_id) - 1);

    if(ri->host_os_id_like)
        strncpyz(ds->os_id_like, ri->host_os_id_like, sizeof(ds->os_id_like) - 1);

    if(ri->is_k8s_node) {
        if (strcmp(ri->is_k8s_node, "true") == 0)
            ds->kubernetes = true;
        else
            ds->kubernetes = false;
    }

    if(ri->cloud_provider_type && strcmp(ri->cloud_provider_type, "unknown") != 0)
        strncpyz(ds->cloud_provider_type, ri->cloud_provider_type, sizeof(ds->cloud_provider_type) - 1);

    if(ri->cloud_instance_type && strcmp(ri->cloud_instance_type, "unknown") != 0)
        strncpyz(ds->cloud_instance_type, ri->cloud_instance_type, sizeof(ds->cloud_instance_type) - 1);

    if(ri->cloud_instance_region && strcmp(ri->cloud_instance_region, "unknown") != 0)
        strncpyz(ds->cloud_instance_region, ri->cloud_instance_region, sizeof(ds->cloud_instance_region) - 1);

    ds->read_system_info = true;
}
