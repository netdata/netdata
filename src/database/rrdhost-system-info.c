// SPDX-License-Identifier: GPL-3.0-or-later

#define RRDHOST_SYSTEM_INFO_INTERNALS
#include "rrdhost-system-info.h"
#include "aclk/schema-wrappers/node_info.h"
#include "daemon/win_system-info.h"
#include "daemon/daemon-service.h"

// coverity[ +tainted_string_sanitize_content : arg-0 ]
static inline void coverity_remove_taint(char *s __maybe_unused) {
    // intentionally empty: only a marker for the Coverity taint sanitizer
    // (see the annotation above); it has no runtime effect.
}

static char *system_info_strdupz(const char *s) {
    return s ? strdupz(s) : NULL;
}

void rrdhost_system_info_swap(struct rrdhost_system_info *a, struct rrdhost_system_info *b) {
    if(a && b)
        SWAP(*a, *b);
}

// ----------------------------------------------------------------------------
// RRDHOST - set system info from environment variables
// system_info fields must be heap allocated or NULL
// The field table owns detection keys, label ownership and runtime merge semantics.
static const struct system_info_field {
    const char *key;
    const char *label;
    size_t offset;
    bool numeric;
    bool runtime;
} system_info_fields[] = {
    { "NETDATA_INSTANCE_CLOUD_TYPE", "_cloud_provider_type", offsetof(struct rrdhost_system_info, cloud_provider_type), false, false },
    { "NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE", "_cloud_instance_type", offsetof(struct rrdhost_system_info, cloud_instance_type), false, false },
    { "NETDATA_INSTANCE_CLOUD_INSTANCE_REGION", "_cloud_instance_region", offsetof(struct rrdhost_system_info, cloud_instance_region), false, false },
    { "NETDATA_HOST_OS_NAME", NULL, offsetof(struct rrdhost_system_info, host_os_name), false, false },
    { "NETDATA_HOST_OS_ID", NULL, offsetof(struct rrdhost_system_info, host_os_id), false, false },
    { "NETDATA_HOST_OS_ID_LIKE", NULL, offsetof(struct rrdhost_system_info, host_os_id_like), false, false },
    { "NETDATA_HOST_OS_VERSION", "_os_version", offsetof(struct rrdhost_system_info, host_os_version), false, false },
    { "NETDATA_HOST_OS_VERSION_ID", NULL, offsetof(struct rrdhost_system_info, host_os_version_id), false, false },
    { "NETDATA_HOST_OS_DETECTION", NULL, offsetof(struct rrdhost_system_info, host_os_detection), false, false },
    { "NETDATA_HOST_OS_LABEL_NAME", "_os_name", offsetof(struct rrdhost_system_info, host_os_label_name), false, false },
    { "NETDATA_HOST_OS_LABEL_VERSION", "_os_marketing_version", offsetof(struct rrdhost_system_info, host_os_label_version), false, false },
    { "NETDATA_HOST_OS_LABEL_RELEASE", "_os_release", offsetof(struct rrdhost_system_info, host_os_label_release), false, false },
    { "NETDATA_HOST_OS_LABEL_CODENAME", "_os_codename", offsetof(struct rrdhost_system_info, host_os_label_codename), false, false },
    { "NETDATA_HOST_OS_LABEL_EDITION", "_os_edition", offsetof(struct rrdhost_system_info, host_os_label_edition), false, false },
    { "NETDATA_HOST_OS_LABEL_BUILD", "_os_build", offsetof(struct rrdhost_system_info, host_os_label_build), false, false },
    { "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT", "_system_cores", offsetof(struct rrdhost_system_info, host_cores), true, true },
    { "NETDATA_SYSTEM_CPU_FREQ", "_system_cpu_freq", offsetof(struct rrdhost_system_info, host_cpu_freq), true, false },
    { "NETDATA_SYSTEM_CPU_MODEL", "_system_cpu_model", offsetof(struct rrdhost_system_info, host_cpu_model), false, false },
    { "NETDATA_SYSTEM_TOTAL_RAM", "_system_ram_total", offsetof(struct rrdhost_system_info, host_ram_total), true, true },
    { "NETDATA_SYSTEM_TOTAL_DISK_SIZE", "_system_disk_space", offsetof(struct rrdhost_system_info, host_disk_space), true, true },
    { "NETDATA_CONTAINER_OS_NAME", NULL, offsetof(struct rrdhost_system_info, container_os_name), false, false },
    { "NETDATA_CONTAINER_OS_ID", NULL, offsetof(struct rrdhost_system_info, container_os_id), false, false },
    { "NETDATA_CONTAINER_OS_ID_LIKE", NULL, offsetof(struct rrdhost_system_info, container_os_id_like), false, false },
    { "NETDATA_CONTAINER_OS_VERSION", NULL, offsetof(struct rrdhost_system_info, container_os_version), false, false },
    { "NETDATA_CONTAINER_OS_VERSION_ID", NULL, offsetof(struct rrdhost_system_info, container_os_version_id), false, false },
    { "NETDATA_CONTAINER_OS_DETECTION", NULL, offsetof(struct rrdhost_system_info, container_os_detection), false, false },
    { "NETDATA_SYSTEM_KERNEL_NAME", NULL, offsetof(struct rrdhost_system_info, kernel_name), false, false },
    { "NETDATA_SYSTEM_KERNEL_VERSION", "_kernel_version", offsetof(struct rrdhost_system_info, kernel_version), false, false },
    { "NETDATA_SYSTEM_ARCHITECTURE", "_architecture", offsetof(struct rrdhost_system_info, architecture), false, false },
    { "NETDATA_SYSTEM_VIRTUALIZATION", "_virtualization", offsetof(struct rrdhost_system_info, virtualization), false, false },
    { "NETDATA_SYSTEM_VIRT_DETECTION", "_virt_detection", offsetof(struct rrdhost_system_info, virt_detection), false, false },
    { "NETDATA_SYSTEM_CONTAINER", "_container", offsetof(struct rrdhost_system_info, container), false, false },
    { "NETDATA_SYSTEM_CONTAINER_DETECTION", "_container_detection", offsetof(struct rrdhost_system_info, container_detection), false, false },
    { "NETDATA_HOST_IS_K8S_NODE", "_is_k8s_node", offsetof(struct rrdhost_system_info, is_k8s_node), false, false },
    { NULL, "_install_type", offsetof(struct rrdhost_system_info, install_type), false, false },
    { NULL, "_prebuilt_arch", offsetof(struct rrdhost_system_info, prebuilt_arch), false, false },
    { NULL, "_prebuilt_dist", offsetof(struct rrdhost_system_info, prebuilt_dist), false, false },
    { "NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME", "_net_default_iface", offsetof(struct rrdhost_system_info, network_default_iface), false, true },
    { "NETDATA_SYSTEM_DEFAULT_INTERFACE_IP", "_net_default_iface_ip", offsetof(struct rrdhost_system_info, network_default_iface_ip), false, true },
    { "NETDATA_SYSTEM_DEFAULT_INTERFACE_DETECTION", "_net_default_iface_detection", offsetof(struct rrdhost_system_info, network_default_iface_detection), false, true },
    { NULL, "_hw_product_id", offsetof(struct rrdhost_system_info, hw_product_id), false, false },
    { NULL, "_hw_product_name", offsetof(struct rrdhost_system_info, hw_product_name), false, false },
    { NULL, "_hw_sys_vendor", offsetof(struct rrdhost_system_info, hw_sys_vendor), false, false },
    { NULL, "_hw_product_type", offsetof(struct rrdhost_system_info, hw_product_type), false, false },
};
_Static_assert(sizeof(system_info_fields) / sizeof(system_info_fields[0]) <= 64, "system-info validity bitmap exhausted");

static char **system_info_field_ptr(struct rrdhost_system_info *si, size_t i) {
    return (char **)((char *)si + system_info_fields[i].offset);
}

static bool system_info_replace(char **dst, const char *value) {
    if ((!*dst && !value) || (*dst && value && !strcmp(*dst, value)))
        return false;
    char *copy = system_info_strdupz(value);
    freez(*dst);
    *dst = copy;
    return true;
}

static int system_info_field_index(const char *key) {
    for (size_t i = 0; i < _countof(system_info_fields); i++)
        if (system_info_fields[i].key && !strcmp(key, system_info_fields[i].key))
            return (int)i;
    return -1;
}

int rrdhost_system_info_set_by_name(struct rrdhost_system_info *si, const char *name, const char *value) {
    if (!si || !name || !value)
        return 1;

    int i = system_info_field_index(name);
    if (i >= 0) {
        char **dst = system_info_field_ptr(si, i);
        if (system_info_fields[i].offset == offsetof(struct rrdhost_system_info, host_os_name)) {
            size_t size = strlen(value) * 2 + 1;
            char *clean = mallocz(size);
            text_sanitize((unsigned char *)clean, (const unsigned char *)value, size,
                          rrd_string_allowed_chars, true, "", NULL);
            system_info_replace(dst, clean);
            freez(clean);
        }
        else
            system_info_replace(dst, value);
        return 0;
    }

    // Recognized startup environment exports that have no stored system-info member.
    static const char *auxiliary[] = {
        "NETDATA_PROTOCOL_VERSION", "NETDATA_SYSTEM_CPU_VENDOR", "NETDATA_SYSTEM_CPU_DETECTION",
        "NETDATA_SYSTEM_RAM_DETECTION", "NETDATA_SYSTEM_DISK_DETECTION", "NETDATA_CONTAINER_IS_OFFICIAL_IMAGE",
    };
    for (size_t n = 0; n < _countof(auxiliary); n++)
        if (!strcmp(name, auxiliary[n]))
            return 0;
    return 1;
}

bool rrdhost_system_info_label_is_owned(const char *name) {
    for (size_t i = 0; i < _countof(system_info_fields); i++)
        if (system_info_fields[i].label && !strcmp(name, system_info_fields[i].label))
            return true;
    return false;
}

bool rrdhost_system_info_label_is_runtime(const char *name) {
    for (size_t i = 0; i < _countof(system_info_fields); i++)
        if (system_info_fields[i].runtime && !strcmp(name, system_info_fields[i].label))
            return true;
    return false;
}

bool rrdhost_system_info_update(struct rrdhost_system_info *dst, struct rrdhost_system_info *candidate) {
    bool changed = false;
    for (size_t i = 0; i < _countof(system_info_fields); i++)
        if (system_info_fields[i].runtime && (candidate->detected_fields & (UINT64_C(1) << i)))
            changed |= system_info_replace(system_info_field_ptr(dst, i), *system_info_field_ptr(candidate, i));
    return changed;
}

static bool system_info_update_from_labels(struct rrdhost_system_info *dst, RRDLABELS *old_labels,
                                           RRDLABELS *new_labels, bool runtime_only) {
    bool changed = false;
    for (size_t i = 0; i < _countof(system_info_fields); i++) {
        const char *label = system_info_fields[i].label;
        if (!label || (runtime_only && !system_info_fields[i].runtime))
            continue;
        char *value = NULL, *old = NULL;
        rrdlabels_get_value_strdup_or_null(new_labels, &value, label);
        if (old_labels)
            rrdlabels_get_value_strdup_or_null(old_labels, &old, label);
        // Re-apply even unchanged labels: a reconnect may have installed a fresh handshake snapshot.
        if (value || old) {
            changed |= system_info_replace(system_info_field_ptr(dst, i), value);
            if (!strcmp(label, "_os_name")) {
                char family[32];
                rrdlabels_get_value_strcpyz(new_labels, family, sizeof(family), "_os");
                changed |= system_info_replace(&dst->host_os_name,
                    value && !strcasecmp(family, "windows") ? "Microsoft Windows" : value);
            }
        }
        freez(value);
        freez(old);
    }
    return changed;
}

bool rrdhost_system_info_update_from_labels(struct rrdhost_system_info *dst, RRDLABELS *old_labels, RRDLABELS *new_labels) {
    return system_info_update_from_labels(dst, old_labels, new_labels, true);
}

bool rrdhost_system_info_update_all_from_labels(struct rrdhost_system_info *dst, RRDLABELS *old_labels, RRDLABELS *new_labels) {
    return system_info_update_from_labels(dst, old_labels, new_labels, false);
}

struct rrdhost_system_info *rrdhost_system_info_from_host_labels(RRDLABELS *labels) {
    struct rrdhost_system_info *info = rrdhost_system_info_create();
    info->hops = 1;
    rrdhost_system_info_update_all_from_labels(info, NULL, labels);
    char family[32];
    rrdlabels_get_value_strcpyz(labels, family, sizeof(family), "_os");
    if (!strcasecmp(family, "windows"))
        system_info_replace(&info->host_os_name, "Microsoft Windows");
    return info;
}

void rrdhost_system_info_to_rrdlabels(struct rrdhost_system_info *si, RRDLABELS *labels) {
    for (size_t i = 0; i < _countof(system_info_fields); i++) {
        const char *value = *system_info_field_ptr(si, i);
        if (system_info_fields[i].offset == offsetof(struct rrdhost_system_info, host_os_label_name) && !value)
            value = si->host_os_name;
        if (value && system_info_fields[i].label)
            rrdlabels_add(labels, system_info_fields[i].label, value, RRDLABEL_SRC_AUTO);
    }
}

// NULL is successful absence. Failed/unsupported probes never call this function.
bool rrdhost_system_info_detected_set(struct rrdhost_system_info *si, const char *key, const char *value) {
    int i = system_info_field_index(key);
    if (i < 0 || !system_info_fields[i].runtime)
        return false;
    if (value) {
        if (!*value || !strcmp(value, "unknown"))
            return false;
        if (system_info_fields[i].numeric) {
            uint64_t number = 0;
            for (const unsigned char *p = (const unsigned char *)value; *p; p++) {
                if (*p < '0' || *p > '9' || number > (UINT64_MAX - (*p - '0')) / 10)
                    return false;
                number = number * 10 + (*p - '0');
            }
            if (!strcmp(key, "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT") && number > UINT32_MAX)
                return false;
            if (!number && strcmp(key, "NETDATA_SYSTEM_TOTAL_DISK_SIZE"))
                return false;
        }
        rrdhost_system_info_set_by_name(si, key, value);
    }
    else
        system_info_replace(system_info_field_ptr(si, i), NULL);
    si->detected_fields |= UINT64_C(1) << i;
    return true;
}

// Every runtime key occurs exactly once, even when its probe failed.
static bool system_info_parse_runtime(struct rrdhost_system_info *si, char *output) {
    static const char header[] = "NETDATA_SYSTEM_INFO_V1\n";
    if (strncmp(output, header, sizeof(header) - 1))
        return false;
    char *line = output + sizeof(header) - 1;
    uint64_t seen = 0, expected = 0;
    char network_status = 0;
    for (size_t i = 0; i < _countof(system_info_fields); i++)
        if (system_info_fields[i].runtime)
            expected |= UINT64_C(1) << i;
    while (*line) {
        char *end = strchr(line, '\n');
        if (!end)
            return false;
        *end = '\0';
        if (!strcmp(line, "NETDATA_SYSTEM_INFO_END"))
            return !end[1] && seen == expected;
        if (!line[0] || line[1] != '\t')
            return false;
        char status = line[0];
        char *key = line + 2, *value = strchr(key, '\t');
        if (!value)
            return false;
        *value++ = '\0';
        int i = system_info_field_index(key);
        if (i < 0 || !system_info_fields[i].runtime || (seen & (UINT64_C(1) << i)) || strchr(value, '\r') || strchr(value, '\t'))
            return false;
        seen |= UINT64_C(1) << i;
        if (!strncmp(key, "NETDATA_SYSTEM_DEFAULT_INTERFACE_", sizeof("NETDATA_SYSTEM_DEFAULT_INTERFACE_") - 1)) {
            if (network_status && status != network_status)
                return false;
            network_status = status;
        }
        if (status == 'V') {
            if (!rrdhost_system_info_detected_set(si, key, value))
                return false;
        }
        else if (*value || (status != 'A' && status != 'F' && status != 'U'))
            return false;
        else if (status == 'A')
            rrdhost_system_info_detected_set(si, key, NULL);
        line = end + 1;
    }
    return false;
}

#if !defined(OS_WINDOWS)
static char *system_info_read_argv(const char **argv, bool (*cancelled)(void), unsigned timeout_ms);
static void system_info_parse_startup(struct rrdhost_system_info *system_info, char *output);
static bool system_info_test_cancelled(void) { return true; }
#endif

static BUFFER *system_info_test_response(const char *key, char status, const char *value) {
    BUFFER *b = buffer_create(0, NULL);
    buffer_strcat(b, "NETDATA_SYSTEM_INFO_V1\n");
    for (size_t i = 0; i < _countof(system_info_fields); i++) {
        if (!system_info_fields[i].runtime)
            continue;
        bool selected = key && !strcmp(key, system_info_fields[i].key);
        buffer_sprintf(b, "%c\t%s\t%s\n", selected ? status : 'F', system_info_fields[i].key,
                       selected && value ? value : "");
    }
    buffer_strcat(b, "NETDATA_SYSTEM_INFO_END\n");
    return b;
}

int rrdhost_system_info_unittest(void) {
    int errors = 0;
#define SI_CHECK(condition) do { if (!(condition)) { fprintf(stderr, "system-info test failed at %s:%d: %s\n", __FILE__, __LINE__, #condition); errors++; } } while (0)
    struct rrdhost_system_info *dst = rrdhost_system_info_create();
    struct rrdhost_system_info *candidate = rrdhost_system_info_create();
    rrdhost_system_info_set_by_name(dst, "NETDATA_SYSTEM_TOTAL_RAM", "1024");
    dst->hops = 7;
    dst->ml_enabled = true;
    dst->ml_capable = true;
    dst->mc_version = 3;
    dst->install_type = strdupz("fixture");
    dst->kernel_name = strdupz("Linux");
    BUFFER *b = system_info_test_response("NETDATA_SYSTEM_TOTAL_RAM", 'V', "2048");
    SI_CHECK(system_info_parse_runtime(candidate, (char *)buffer_tostring(b)));
    SI_CHECK(rrdhost_system_info_update(dst, candidate));
    SI_CHECK(!strcmp(dst->host_ram_total, "2048"));
    SI_CHECK(!rrdhost_system_info_update(dst, candidate));
    SI_CHECK(dst->hops == 7 && dst->ml_enabled && dst->ml_capable && dst->mc_version == 3);
    SI_CHECK(!strcmp(dst->install_type, "fixture") && !strcmp(dst->kernel_name, "Linux"));
    buffer_free(b);

    const char *invalid[] = { "-1", "1.5", "18446744073709551616", "0", "unknown", "" };
    for (size_t i = 0; i < _countof(invalid); i++) {
        b = system_info_test_response("NETDATA_SYSTEM_TOTAL_RAM", 'V', invalid[i]);
        SI_CHECK(!system_info_parse_runtime(candidate, (char *)buffer_tostring(b)));
        buffer_free(b);
    }
    candidate->detected_fields = 0;
    b = system_info_test_response("NETDATA_SYSTEM_TOTAL_RAM", 'F', NULL);
    SI_CHECK(system_info_parse_runtime(candidate, (char *)buffer_tostring(b)));
    SI_CHECK(!rrdhost_system_info_update(dst, candidate));
    SI_CHECK(!strcmp(dst->host_ram_total, "2048"));
    buffer_free(b);
    b = system_info_test_response("NETDATA_SYSTEM_TOTAL_RAM", 'A', NULL);
    SI_CHECK(system_info_parse_runtime(candidate, (char *)buffer_tostring(b)));
    SI_CHECK(rrdhost_system_info_update(dst, candidate) && !dst->host_ram_total);
    buffer_free(b);
    b = system_info_test_response("NETDATA_SYSTEM_TOTAL_RAM", 'F', "unexpected");
    SI_CHECK(!system_info_parse_runtime(candidate, (char *)buffer_tostring(b)));
    buffer_free(b);
    b = system_info_test_response("NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME", 'A', NULL);
    SI_CHECK(!system_info_parse_runtime(candidate, (char *)buffer_tostring(b))); // incoherent tuple
    buffer_free(b);
    char incomplete[] = "NETDATA_SYSTEM_INFO_V1\nF\tNETDATA_SYSTEM_TOTAL_RAM\t\n";
    char duplicate[] = "NETDATA_SYSTEM_INFO_V1\nF\tNETDATA_SYSTEM_TOTAL_RAM\t\nF\tNETDATA_SYSTEM_TOTAL_RAM\t\n";
    char malformed[] = "NETDATA_SYSTEM_INFO_V1\nPID123\n";
    char unknown[] = "NETDATA_SYSTEM_INFO_V1\nF\tUNRECOGNIZED\t\n";
    SI_CHECK(!system_info_parse_runtime(candidate, incomplete));
    SI_CHECK(!system_info_parse_runtime(candidate, duplicate));
    SI_CHECK(!system_info_parse_runtime(candidate, malformed));
    SI_CHECK(!system_info_parse_runtime(candidate, unknown));

    RRDLABELS *old = rrdlabels_create(), *fresh = rrdlabels_create();
    rrdlabels_add(old, "_system_ram_total", "4096", RRDLABEL_SRC_AUTO);
    rrdlabels_add(fresh, "_system_ram_total", "4096", RRDLABEL_SRC_AUTO);
    rrdhost_system_info_set_by_name(dst, "NETDATA_SYSTEM_TOTAL_RAM", "1024");
    rrdhost_system_info_set_by_name(dst, "NETDATA_SYSTEM_CPU_FREQ", "2000000000");
    SI_CHECK(rrdhost_system_info_update_from_labels(dst, old, fresh));
    SI_CHECK(!strcmp(dst->host_ram_total, "4096")); // unchanged labels override new handshake
    SI_CHECK(!rrdhost_system_info_update_from_labels(dst, old, fresh));
    SI_CHECK(!strcmp(dst->host_cpu_freq, "2000000000")); // never advertised
    SI_CHECK(dst->hops == 7 && dst->ml_enabled && !strcmp(dst->kernel_name, "Linux"));
    rrdlabels_destroy(fresh);
    fresh = rrdlabels_create();
    SI_CHECK(rrdhost_system_info_update_from_labels(dst, old, fresh));
    SI_CHECK(!dst->host_ram_total); // previously advertised then deleted
    SI_CHECK(!strcmp(dst->host_cpu_freq, "2000000000"));
    SI_CHECK(rrdhost_system_info_label_is_owned("_net_default_iface"));
    SI_CHECK(!rrdhost_system_info_label_is_owned("_stream_egress_iface"));
    SI_CHECK(!rrdhost_system_info_detected_set(candidate, "NETDATA_SYSTEM_CPU_FREQ", "3000000000"));
    SI_CHECK(!rrdhost_system_info_detected_set(candidate, "NETDATA_SYSTEM_CPU_MODEL", "new model"));
    SI_CHECK(!rrdhost_system_info_label_is_runtime("_os_name"));
    SI_CHECK(!rrdhost_system_info_label_is_runtime("_system_cpu_freq"));
    SI_CHECK(rrdhost_system_info_label_is_runtime("_system_cores"));
    b = system_info_test_response("NETDATA_SYSTEM_TOTAL_RAM", 'F', NULL);
    buffer_flush(b);
    buffer_strcat(b, "NETDATA_SYSTEM_INFO_V1\nV\tNETDATA_SYSTEM_CPU_FREQ\t3000000000\nNETDATA_SYSTEM_INFO_END\n");
    SI_CHECK(!system_info_parse_runtime(candidate, (char *)buffer_tostring(b)));
    buffer_free(b);
#if !defined(OS_WINDOWS)
    struct rrdhost_system_info *startup = rrdhost_system_info_create();
    const char *network_keys[] = {
        "NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME", "NETDATA_SYSTEM_DEFAULT_INTERFACE_IP",
        "NETDATA_SYSTEM_DEFAULT_INTERFACE_DETECTION",
    };
    char *saved_env[_countof(network_keys)];
    for (size_t i = 0; i < _countof(network_keys); i++)
        saved_env[i] = system_info_strdupz(getenv(network_keys[i]));
    struct rrdhost_system_info *absent = rrdhost_system_info_create();
    for (size_t i = 0; i < _countof(network_keys); i++)
        SI_CHECK(rrdhost_system_info_detected_set(absent, network_keys[i], NULL));
    // A route can exist while its interface is down or has no usable IPv4 address.
    const char *absence_methods[] = { "none", "route", "procfs", "iproute2" };
    for (size_t m = 0; m < _countof(absence_methods); m++) {
        char no_network[256];
        snprintfz(no_network, sizeof(no_network), "NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME=unknown\n"
                  "NETDATA_SYSTEM_DEFAULT_INTERFACE_IP=unknown\n"
                  "NETDATA_SYSTEM_DEFAULT_INTERFACE_DETECTION=%s\n", absence_methods[m]);
        system_info_parse_startup(startup, no_network);
        SI_CHECK(!startup->network_default_iface && !startup->network_default_iface_ip &&
                 !startup->network_default_iface_detection);
        for (size_t i = 0; i < _countof(network_keys); i++)
            SI_CHECK(getenv(network_keys[i]) && !strcmp(getenv(network_keys[i]), i == 2 ? absence_methods[m] : "unknown"));
        SI_CHECK(!rrdhost_system_info_update(startup, absent));
    }
    for (size_t i = 0; i < _countof(network_keys); i++) {
        if (saved_env[i]) nd_setenv(network_keys[i], saved_env[i], 1);
        else unsetenv(network_keys[i]);
        freez(saved_env[i]);
    }
    rrdhost_system_info_free(startup);
    rrdhost_system_info_free(absent);
    bool own_server = !netdata_main_spawn_server;
    SI_CHECK(netdata_main_spawn_server_init("system-info-test", 0, NULL));
    if (netdata_main_spawn_server) {
        b = system_info_test_response("NETDATA_SYSTEM_TOTAL_RAM", 'V', "8192");
        const char *valid_argv[] = { "/bin/sh", "-c", "printf '%s' \"$1\"", "sh", buffer_tostring(b), NULL };
        char *output = system_info_read_argv(valid_argv, NULL, 2000);
        SI_CHECK(output && system_info_parse_runtime(candidate, output));
        freez(output);
        const char *failure_argv[] = { "/bin/sh", "-c", "printf '%s' \"$1\"; exit 7", "sh", buffer_tostring(b), NULL };
        output = system_info_read_argv(failure_argv, NULL, 2000);
        SI_CHECK(!output);
        freez(output);
        buffer_free(b);
        const char *drain_argv[] = { "/bin/sh", "-c", "printf '%065535d' 0", NULL };
        output = system_info_read_argv(drain_argv, NULL, 2000);
        SI_CHECK(output && strlen(output) == 65535);
        freez(output);
        const char *overflow_argv[] = { "/bin/sh", "-c", "printf '%065536d' 0", NULL };
        output = system_info_read_argv(overflow_argv, NULL, 2000);
        SI_CHECK(!output);
        freez(output);
        const char *timeout_argv[] = { "/bin/sh", "-c", "sleep 10", NULL };
        usec_t started = now_monotonic_usec();
        output = system_info_read_argv(timeout_argv, NULL, 50);
        SI_CHECK(!output && now_monotonic_usec() - started < 3 * USEC_PER_SEC);
        freez(output);
        const char *descendant_argv[] = { "/bin/sh", "-c", "sleep 10 & exit 0", NULL };
        started = now_monotonic_usec();
        output = system_info_read_argv(descendant_argv, NULL, 50);
        SI_CHECK((!output || !*output) && now_monotonic_usec() - started < 3 * USEC_PER_SEC);
        freez(output);
        output = system_info_read_argv(timeout_argv, system_info_test_cancelled, 2000);
        SI_CHECK(!output);
        freez(output);
        const char *signal_argv[] = { "/bin/sh", "-c", "kill -TERM $$", NULL };
        output = system_info_read_argv(signal_argv, NULL, 2000);
        SI_CHECK(!output);
        freez(output);
    }
    if (own_server)
        netdata_main_spawn_server_cleanup();
#endif
    rrdlabels_destroy(old);
    rrdlabels_destroy(fresh);
    rrdhost_system_info_free(dst);
    rrdhost_system_info_free(candidate);
#undef SI_CHECK
    return errors;
}

#if !defined(OS_WINDOWS)
static void system_info_log_failure(const char *reason, int status) {
    nd_log_limit_static_global_var(limit, 60, 0);
    if (status >= 0)
        nd_log_limit(&limit, NDLS_DAEMON, NDLP_WARNING,
                     "SYSTEM INFO: detection failed: %s (exit status %d)", reason, status);
    else
        nd_log_limit(&limit, NDLS_DAEMON, NDLP_WARNING, "SYSTEM INFO: detection failed: %s", reason);
}

// Read through EOF, including buffered data delivered together with POLLHUP.
// The deadline starts after spawn; the owned group is reclaimed before its leader is reaped.
static char *system_info_read_argv(const char **argv, bool (*cancelled)(void), unsigned timeout_ms) {
    POPEN_INSTANCE *pi = spawn_popen_run_argv_group(argv);
    if (!pi) {
        system_info_log_failure("unable to spawn script", -1);
        return NULL;
    }
    const char *failure = "execution deadline exceeded";
    int exit_status = -1;
    bool was_cancelled = false;
    usec_t deadline = now_monotonic_usec() + (usec_t)timeout_ms * USEC_PER_MS;
    const size_t limit = 64 * 1024;
    char *output = mallocz(limit + 1);
    size_t used = 0;
    int fd = spawn_popen_read_fd(pi);
    bool eof = false, ok = fd >= 0;
    if (!ok)
        failure = "invalid script output descriptor";
    while (ok && !eof && used < limit && now_monotonic_usec() < deadline) {
        if (cancelled && cancelled()) {
            ok = false;
            was_cancelled = true;
            break;
        }
        struct pollfd pfd = { .fd = fd, .events = POLLIN };
        int rc = poll(&pfd, 1, 100);
        if (rc < 0) {
            if (errno == EINTR) continue;
            ok = false;
            failure = "polling script output failed";
        }
        else if (rc && (pfd.revents & (POLLIN | POLLHUP))) {
            ssize_t n = read(fd, output + used, limit - used);
            if (n > 0) used += (size_t)n;
            else if (!n) eof = true;
            else if (errno != EINTR) {
                ok = false;
                failure = "reading script output failed";
            }
        }
        else if (rc && (pfd.revents & (POLLERR | POLLNVAL))) {
            ok = false;
            failure = "script output pipe failed";
        }
    }
    if (used >= limit)
        failure = "script output reached the 64 KiB limit";
    else if (memchr(output, '\0', used))
        failure = "script output contains a NUL byte";
    ok = ok && eof && used < limit && !memchr(output, '\0', used);
    output[used] = '\0';
    while (ok && now_monotonic_usec() < deadline) {
        if (cancelled && cancelled()) {
            was_cancelled = true;
            break;
        }
        int code = 1;
        SPAWN_TIMEDWAIT_RESULT rc = spawn_popen_timedwait(pi, 100, &code);
        if (rc == SPAWN_TIMEDWAIT_EXITED) {
            pi = NULL;
            ok = code == 0;
            exit_status = code;
            failure = "script exited unsuccessfully";
            break;
        }
        if (rc == SPAWN_TIMEDWAIT_ERROR) {
            failure = "waiting for script exit failed";
            break;
        }
    }
    if (pi) {
        spawn_popen_kill(pi, 1000);
        ok = false;
    }
    if (!ok) {
        if (!was_cancelled)
            system_info_log_failure(failure, exit_status);
        freez(output);
        return NULL;
    }
    return output;
}
static void system_info_parse_startup(struct rrdhost_system_info *system_info, char *output) {
    char *cursor = output;
    char *line;
    while ((line = strsep(&cursor, "\n"))) {
        char *value = strchr(line, '=');
        if (!value)
            continue;
        *value++ = '\0';
        char *cr = strchr(value, '\r');
        if (cr) *cr = '\0';
        if (!*line || !*value)
            continue;
        coverity_remove_taint(line);
        coverity_remove_taint(value);
        if (!rrdhost_system_info_set_by_name(system_info, line, value))
            nd_setenv(line, value, 1);
    }

    // A known route can still lack a usable interface/address. Preserve legacy
    // environment values while keeping every unknown tuple identical to runtime absence.
    if (system_info->network_default_iface && !strcmp(system_info->network_default_iface, "unknown") &&
        system_info->network_default_iface_ip && !strcmp(system_info->network_default_iface_ip, "unknown")) {
        system_info_replace(&system_info->network_default_iface, NULL);
        system_info_replace(&system_info->network_default_iface_ip, NULL);
        system_info_replace(&system_info->network_default_iface_detection, NULL);
    }
}

static bool system_info_cancelled(void) {
    return nd_thread_signaled_to_cancel() || exit_initiated_get();
}

static char *system_info_read_script(bool runtime) {
    CLEAN_BUFFER *script = buffer_create(0, NULL);
    buffer_sprintf(script, "%s/system-info.sh", netdata_configured_primary_plugins_dir);
    const char *argv[] = { buffer_tostring(script), runtime ? "--runtime" : "--bounded", NULL };
    return system_info_read_argv(argv, runtime ? system_info_cancelled : NULL, 30000);
}

#endif

bool rrdhost_system_info_detect_runtime(struct rrdhost_system_info *candidate) {
    if (!candidate)
        return false;
    candidate->detected_fields = 0;
#if defined(OS_WINDOWS)
    netdata_windows_get_runtime_system_info(candidate);
    return true;
#else
    char *output = system_info_read_script(true);
    if (!output)
        return false;
    bool ok = system_info_parse_runtime(candidate, output);
    freez(output);
    if (!ok) {
        candidate->detected_fields = 0;
        system_info_log_failure("malformed or incomplete runtime response", -1);
    }
    return ok;
#endif
}

int rrdhost_system_info_detect(struct rrdhost_system_info *system_info) {
    if (!system_info) {
        netdata_log_error("SYSTEM INFO: System info structure is NULL.");
        return 1;
    }

    // Populate hardware product fields from the daemon status file when it is available/initialized.
    {
        const char *product_id = daemon_status_file_get_product_id();
        if (product_id && *product_id) {
            freez(system_info->hw_product_id);
            system_info->hw_product_id = strdupz(product_id);
        }

        const char *product_name = daemon_status_file_get_product_name();
        if (product_name && *product_name) {
            freez(system_info->hw_product_name);
            system_info->hw_product_name = strdupz(product_name);
        }

        const char *sys_vendor = daemon_status_file_get_sys_vendor();
        if (sys_vendor && *sys_vendor) {
            freez(system_info->hw_sys_vendor);
            system_info->hw_sys_vendor = strdupz(sys_vendor);
        }

        const char *product_type = daemon_status_file_get_product_type();
        if (product_type && *product_type) {
            freez(system_info->hw_product_type);
            system_info->hw_product_type = strdupz(product_type);
        }
    }

#if !defined(OS_WINDOWS)
    char *output = system_info_read_script(false);
    if (!output)
        return 1;
    system_info_parse_startup(system_info, output);
    freez(output);
    return 0;
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
        freez(system_info->host_os_label_name);
        freez(system_info->host_os_label_version);
        freez(system_info->host_os_label_release);
        freez(system_info->host_os_label_codename);
        freez(system_info->host_os_label_edition);
        freez(system_info->host_os_label_build);
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
        freez(system_info->network_default_iface);
        freez(system_info->network_default_iface_ip);
        freez(system_info->network_default_iface_detection);
        freez(system_info->hw_product_id);
        freez(system_info->hw_product_name);
        freez(system_info->hw_sys_vendor);
        freez(system_info->hw_product_type);
        freez(system_info);
    }
}

struct rrdhost_system_info *rrdhost_system_info_create(void) {
    struct rrdhost_system_info *system_info = callocz(1, sizeof(struct rrdhost_system_info));
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);
    return system_info;
}

struct rrdhost_system_info *rrdhost_system_info_dup(struct rrdhost_system_info *system_info) {
    if(unlikely(!system_info))
        return NULL;

    struct rrdhost_system_info *copy = rrdhost_system_info_create();

    copy->cloud_provider_type = system_info_strdupz(system_info->cloud_provider_type);
    copy->cloud_instance_type = system_info_strdupz(system_info->cloud_instance_type);
    copy->cloud_instance_region = system_info_strdupz(system_info->cloud_instance_region);
    copy->host_os_name = system_info_strdupz(system_info->host_os_name);
    copy->host_os_id = system_info_strdupz(system_info->host_os_id);
    copy->host_os_id_like = system_info_strdupz(system_info->host_os_id_like);
    copy->host_os_version = system_info_strdupz(system_info->host_os_version);
    copy->host_os_version_id = system_info_strdupz(system_info->host_os_version_id);
    copy->host_os_detection = system_info_strdupz(system_info->host_os_detection);
    copy->host_os_label_name = system_info_strdupz(system_info->host_os_label_name);
    copy->host_os_label_version = system_info_strdupz(system_info->host_os_label_version);
    copy->host_os_label_release = system_info_strdupz(system_info->host_os_label_release);
    copy->host_os_label_codename = system_info_strdupz(system_info->host_os_label_codename);
    copy->host_os_label_edition = system_info_strdupz(system_info->host_os_label_edition);
    copy->host_os_label_build = system_info_strdupz(system_info->host_os_label_build);
    copy->host_cores = system_info_strdupz(system_info->host_cores);
    copy->host_cpu_freq = system_info_strdupz(system_info->host_cpu_freq);
    copy->host_cpu_model = system_info_strdupz(system_info->host_cpu_model);
    copy->host_ram_total = system_info_strdupz(system_info->host_ram_total);
    copy->host_disk_space = system_info_strdupz(system_info->host_disk_space);
    copy->container_os_name = system_info_strdupz(system_info->container_os_name);
    copy->container_os_id = system_info_strdupz(system_info->container_os_id);
    copy->container_os_id_like = system_info_strdupz(system_info->container_os_id_like);
    copy->container_os_version = system_info_strdupz(system_info->container_os_version);
    copy->container_os_version_id = system_info_strdupz(system_info->container_os_version_id);
    copy->container_os_detection = system_info_strdupz(system_info->container_os_detection);
    copy->kernel_name = system_info_strdupz(system_info->kernel_name);
    copy->kernel_version = system_info_strdupz(system_info->kernel_version);
    copy->architecture = system_info_strdupz(system_info->architecture);
    copy->virtualization = system_info_strdupz(system_info->virtualization);
    copy->virt_detection = system_info_strdupz(system_info->virt_detection);
    copy->container = system_info_strdupz(system_info->container);
    copy->container_detection = system_info_strdupz(system_info->container_detection);
    copy->is_k8s_node = system_info_strdupz(system_info->is_k8s_node);
    copy->detected_fields = system_info->detected_fields;
    copy->hops = system_info->hops;
    copy->ml_capable = system_info->ml_capable;
    copy->ml_enabled = system_info->ml_enabled;
    copy->install_type = system_info_strdupz(system_info->install_type);
    copy->prebuilt_arch = system_info_strdupz(system_info->prebuilt_arch);
    copy->prebuilt_dist = system_info_strdupz(system_info->prebuilt_dist);
    copy->network_default_iface = system_info_strdupz(system_info->network_default_iface);
    copy->network_default_iface_ip = system_info_strdupz(system_info->network_default_iface_ip);
    copy->network_default_iface_detection = system_info_strdupz(system_info->network_default_iface_detection);
    copy->mc_version = system_info->mc_version;
    copy->hw_product_id = system_info_strdupz(system_info->hw_product_id);
    copy->hw_product_name = system_info_strdupz(system_info->hw_product_name);
    copy->hw_sys_vendor = system_info_strdupz(system_info->hw_sys_vendor);
    copy->hw_product_type = system_info_strdupz(system_info->hw_product_type);

    return copy;
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
    ret += cb("NETDATA_CONTAINER_OS_DETECTION", system_info->container_os_detection, uuid);
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
    ret += cb("NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME", system_info->network_default_iface, uuid);
    ret += cb("NETDATA_SYSTEM_DEFAULT_INTERFACE_IP", system_info->network_default_iface_ip, uuid);
    ret += cb("NETDATA_SYSTEM_DEFAULT_INTERFACE_DETECTION", system_info->network_default_iface_detection, uuid);
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

// the system-info columns of the netdata-streaming function, in column order;
// drives the column definitions, the row values and the group-by entries
static const struct {
    const char *key;
    const char *name;
    const char *group_by;           // NULL = no group-by entry
    RRDF_FIELD_FILTER filter;
    size_t offset;
} streaming_function_fields[] = {
    { "OSName",               "The name of the host's operating system",           "O/S Name",                  RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, host_os_name) },
    { "OSId",                 "The identifier of the host's operating system",     "O/S ID",                    RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, host_os_id) },
    { "OSIdLike",             "The ID-like string for the host's OS",              "O/S ID Like",               RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, host_os_id_like) },
    { "OSVersion",            "The version of the host's operating system",        "O/S Version",               RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, host_os_version) },
    { "OSVersionId",          "The version identifier of the host's OS",           "O/S Version ID",            RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, host_os_version_id) },
    { "OSDetection",          "Details about host OS detection",                   "O/S Detection",             RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, host_os_detection) },
    { "CPUCores",             "The number of CPU cores in the host",               "CPU Cores",                 RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, host_cores) },
    { "DiskSpace",            "The total disk space available on the host",        NULL,                        RRDF_FIELD_FILTER_NONE,        offsetof(struct rrdhost_system_info, host_disk_space) },
    { "CPUFreq",              "The CPU frequency of the host",                     NULL,                        RRDF_FIELD_FILTER_NONE,        offsetof(struct rrdhost_system_info, host_cpu_freq) },
    { "RAMTotal",             "The total RAM available on the host",               NULL,                        RRDF_FIELD_FILTER_NONE,        offsetof(struct rrdhost_system_info, host_ram_total) },
    { "ContainerOSName",      "The name of the container's operating system",      "Container O/S Name",        RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, container_os_name) },
    { "ContainerOSId",        "The identifier of the container's operating system", "Container O/S ID",         RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, container_os_id) },
    { "ContainerOSIdLike",    "The ID-like string for the container's OS",         "Container O/S ID Like",     RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, container_os_id_like) },
    { "ContainerOSVersion",   "The version of the container's OS",                 "Container O/S Version",     RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, container_os_version) },
    { "ContainerOSVersionId", "The version identifier of the container's OS",      "Container O/S Version ID",  RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, container_os_version_id) },
    { "ContainerOSDetection", "Details about container OS detection",              "Container O/S Detection",   RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, container_os_detection) },
    { "IsK8sNode",            "Whether this node is part of a Kubernetes cluster", "Kubernetes Nodes",          RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, is_k8s_node) },
    { "KernelName",           "The kernel name",                                   "Kernel Name",               RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, kernel_name) },
    { "KernelVersion",        "The kernel version",                                "Kernel Version",            RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, kernel_version) },
    { "Architecture",         "The system architecture",                           "Architecture",              RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, architecture) },
    { "Virtualization",       "The virtualization technology in use",              "Virtualization Technology", RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, virtualization) },
    { "VirtDetection",        "Details about virtualization detection",            "Virtualization Detection",  RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, virt_detection) },
    { "Container",            "Container type information",                        "Container",                 RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, container) },
    { "ContainerDetection",   "Details about container detection",                 "Container Detection",       RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, container_detection) },
    { "CloudProviderType",    "The type of cloud provider",                        "Cloud Provider Type",       RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, cloud_provider_type) },
    { "CloudInstanceType",    "The type of cloud instance",                        "Cloud Instance Type",       RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, cloud_instance_type) },
    { "CloudInstanceRegion",  "The region of the cloud instance",                  "Cloud Instance Region",     RRDF_FIELD_FILTER_MULTISELECT, offsetof(struct rrdhost_system_info, cloud_instance_region) },
};

size_t rrdhost_system_info_streaming_function_columns(BUFFER *wb, size_t field_id) {
    for(size_t i = 0; i < _countof(streaming_function_fields); i++)
        buffer_rrdf_table_add_field(wb, field_id++, streaming_function_fields[i].key, streaming_function_fields[i].name,
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, streaming_function_fields[i].filter,
                                    RRDF_FIELD_OPTS_NONE,
                                    NULL);

    return field_id;
}

void rrdhost_system_info_to_streaming_function_array(BUFFER *wb, struct rrdhost_system_info *system_info) {
    for(size_t i = 0; i < _countof(streaming_function_fields); i++) {
        const char *value = NULL;
        if(system_info)
            value = *(char *const *)((const char *)system_info + streaming_function_fields[i].offset);

        buffer_json_add_array_item_string(wb, value ? value : "");
    }
}

void rrdhost_system_info_streaming_function_group_by(BUFFER *wb) {
    for(size_t i = 0; i < _countof(streaming_function_fields); i++) {
        if(!streaming_function_fields[i].group_by)
            continue;

        buffer_json_member_add_object(wb, streaming_function_fields[i].key);
        {
            buffer_json_member_add_string(wb, "name", streaming_function_fields[i].group_by);
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, streaming_function_fields[i].key);
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
}

bool get_daemon_status_fields_from_system_info(DAEMON_STATUS_FILE *ds) {
    if(ds->read_system_info)
        return false;

    if(localhost)
        spinlock_lock(&localhost->rrdhost_update_lock);

    struct rrdhost_system_info *ri = (localhost && localhost->system_info) ? localhost->system_info : NULL;
    if(!ri) {
        if(localhost)
            spinlock_unlock(&localhost->rrdhost_update_lock);

        // nothing we can do, let it be
        return false;
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

    if(ri->cloud_provider_type && strcasecmp(ri->cloud_provider_type, "unknown") != 0)
        strncpyz(ds->cloud_provider_type, ri->cloud_provider_type, sizeof(ds->cloud_provider_type) - 1);

    if(ri->cloud_instance_type && strcasecmp(ri->cloud_instance_type, "unknown") != 0)
        strncpyz(ds->cloud_instance_type, ri->cloud_instance_type, sizeof(ds->cloud_instance_type) - 1);

    if(ri->cloud_instance_region && strcasecmp(ri->cloud_instance_region, "unknown") != 0)
        strncpyz(ds->cloud_instance_region, ri->cloud_instance_region, sizeof(ds->cloud_instance_region) - 1);

    ds->read_system_info = true;

    spinlock_unlock(&localhost->rrdhost_update_lock);

    return true;
}

bool localhost_is_docker() {
    bool ret = false;

    if (localhost) {
        spinlock_lock(&localhost->rrdhost_update_lock);

        if (localhost->system_info)
            ret = localhost->system_info->container && strcmp(localhost->system_info->container, "docker") == 0;

        spinlock_unlock(&localhost->rrdhost_update_lock);
    }

    return ret;
};
