// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-labels.h"
#include "rrdhost.h"
#include "streaming/stream.h"

// Gate hot-path events before they touch label storage.
// The slow path re-reads the receiver count before writing.
static SPINLOCK is_parent_label_commit_spinlock = SPINLOCK_INITIALIZER;
static uint32_t is_parent_label_cached_state = 0;

static uint32_t rrdhost_is_parent_state_from_count(uint32_t count) {
    return count ? 1 : 0;
}

static bool rrdhost_update_is_parent_label_for_state(RRDLABELS *labels, uint32_t state) {
    if (!labels)
        return false;

    return rrdlabels_add_changed(labels, "_is_parent", state ? "true" : "false", RRDLABEL_SRC_AUTO);
}

static uint32_t rrdhost_read_is_parent_desired_state(uint32_t (*count_reader)(void)) {
    return rrdhost_is_parent_state_from_count(count_reader());
}

static bool rrdhost_is_parent_label_claim_transition(uint32_t desired) {
    uint32_t cached = __atomic_load_n(&is_parent_label_cached_state, __ATOMIC_RELAXED);

    while (cached != desired) {
        if (__atomic_compare_exchange_n(
                &is_parent_label_cached_state, &cached, desired, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
            return true;
    }

    return false;
}

static bool rrdhost_update_is_parent_label(RRDLABELS *labels, uint32_t (*count_reader)(void), bool force) {
    if (!labels || !count_reader)
        return false;

    uint32_t desired = rrdhost_read_is_parent_desired_state(count_reader);

    if (!force && !rrdhost_is_parent_label_claim_transition(desired))
        return false;

    spinlock_lock(&is_parent_label_commit_spinlock);
    desired = rrdhost_read_is_parent_desired_state(count_reader);
    __atomic_store_n(&is_parent_label_cached_state, desired, __ATOMIC_RELAXED);
    bool changed = rrdhost_update_is_parent_label_for_state(labels, desired);
    spinlock_unlock(&is_parent_label_commit_spinlock);

    return changed;
}

// The complete reaction to a change of a host's label set: persist it, re-match alerts, refresh the
// Cloud node info and forward the set to our parent. Call it when the set changed, or may have (a reload).
// The node info request is immediate: label changes are rare, and a debounced request would postpone
// an immediate one already pending (e.g. the one queued when a child connects).
void rrdhost_labels_changed(RRDHOST *host) {
    rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE | RRDHOST_FLAG_PENDING_LABEL_RECHECK);
    aclk_queue_node_info(host, true);
    stream_send_host_labels(host);
}

// Structured metadata can change while its sanitized label representation stays identical.
void rrdhost_system_info_changed(RRDHOST *host) {
    aclk_queue_node_info(host, true);
    stream_send_host_labels(host);
}

void rrdhost_set_is_parent_label(void) {
    if (!localhost || !localhost->rrdlabels)
        return;

    if (rrdhost_update_is_parent_label(localhost->rrdlabels, stream_receivers_currently_connected, false))
        rrdhost_labels_changed(localhost);
}

// expand ${VAR} and ${VAR:-default} patterns in src, writing result to dst
static void env_expand_labels_value(const char *src, char *dst, size_t dst_size) {
    if(!src || !dst || dst_size < 1) return;

    const char *s = src;
    char *d = dst;
    char *end = dst + dst_size - 1;

    while(*s && d < end) {
        if(s[0] == '$' && s[1] == '{') {
            const char *closing = strchr(s + 2, '}');
            if(!closing) {
                // no closing brace — copy rest literally
                while(*s && d < end)
                    *d++ = *s++;
                break;
            }

            size_t content_len = closing - (s + 2);
            char *var_name = mallocz(content_len + 1);
            memcpy(var_name, s + 2, content_len);
            var_name[content_len] = '\0';

            // check for :- default separator
            char *default_val = NULL;
            char *sep = strstr(var_name, ":-");
            if(sep) {
                *sep = '\0';
                default_val = sep + 2;
            }

            const char *env_val = getenv(var_name);
            const char *resolved;

            if(env_val && *env_val)
                resolved = env_val;
            else if(default_val)
                resolved = default_val;
            else {
                nd_log(NDLS_DAEMON, NDLP_WARNING,
                       "RRDLABEL: environment variable '%s' is not set and no default provided", var_name);
                resolved = "";
            }

            size_t rlen = strlen(resolved);
            size_t available = end - d;
            size_t to_copy = rlen < available ? rlen : available;
            memcpy(d, resolved, to_copy);
            d += to_copy;

            freez(var_name);
            s = closing + 1;
        }
        else {
            *d++ = *s++;
        }
    }

    *d = '\0';
}

// check if value contains any ${...} pattern worth expanding
static bool value_has_env_variables(const char *value) {
    const char *p = value;
    while((p = strchr(p, '$')) != NULL) {
        if(p[1] == '{') return true;
        p++;
    }
    return false;
}

static bool config_label_cb(void *data, const char *name, const char *value) {
    RRDLABELS *labels = data;
    if(value_has_env_variables(value)) {
        char expanded[RRDLABELS_MAX_VALUE_LENGTH + 1];
        env_expand_labels_value(value, expanded, sizeof(expanded));
        rrdlabels_add(labels, name, expanded, RRDLABEL_SRC_CONFIG);
    }
    else
        rrdlabels_add(labels, name, value, RRDLABEL_SRC_CONFIG);

    return true;
}

static void rrdhost_load_config_labels(RRDLABELS *labels) {
    netdata_conf_reload_section(CONFIG_SECTION_HOST_LABEL);
    inicfg_foreach_value_in_section(&netdata_config, CONFIG_SECTION_HOST_LABEL, config_label_cb, labels);
}

// Returns true if the kubernetes labels script ran to a clean exit. A false
// return tells the caller the refresh was best-effort: callers must preserve
// the previously-loaded k8s labels (see reload_host_labels()) so a transient
// script failure does not silently delete them.
static bool rrdhost_load_kubernetes_labels(RRDLABELS *labels) {
    char label_script[sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("get-kubernetes-labels.sh") + 2)];
    sprintf(label_script, "%s/%s", netdata_configured_primary_plugins_dir, "get-kubernetes-labels.sh");

    if (unlikely(access(label_script, R_OK) != 0)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Kubernetes pod label fetching script %s not found.",
               label_script);

        return false;
    }

    POPEN_INSTANCE *instance = spawn_popen_run(label_script);
    if(!instance) return false;

    FILE *child_stdout = spawn_popen_stdout(instance);
    if(unlikely(!child_stdout)) {
        spawn_popen_kill(instance, 0);
        return false;
    }

    char buffer[1000 + 1];
    while (fgets(buffer, 1000, child_stdout) != NULL)
        rrdlabels_add_pair(labels, buffer, RRDLABEL_SRC_AUTO|RRDLABEL_SRC_K8S);

    // Non-zero exit code means that all the script output is error messages. We've shown already any message that didn't include a ':'
    // Here we'll inform with an ERROR that the script failed, show whatever (if anything) was added to the list of labels, free the memory and set the return to null
    int rc = spawn_popen_wait(instance);
    if(rc) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "%s exited abnormally. Failed to get kubernetes labels.",
               label_script);
        return false;
    }

    return true;
}

// Caller holds rrdhost_update_lock and is_parent_label_commit_spinlock.
static void rrdhost_load_auto_labels(RRDHOST *host, RRDLABELS *labels) {
    rrdhost_system_info_to_rrdlabels(host->system_info, labels);

    // The source should be CONF, but when it is set, these labels are exported by default ('send configured labels' in exporting.conf).
    // Their export seems to break exporting to Graphite, see https://github.com/netdata/netdata/issues/14084.

    int is_ephemeral = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_GLOBAL, "is ephemeral node", CONFIG_BOOLEAN_NO);
    rrdlabels_add(labels, HOST_LABEL_IS_EPHEMERAL, is_ephemeral ? "true" : "false", RRDLABEL_SRC_CONFIG);

    int has_unstable_connection = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_GLOBAL, "has unstable connection", CONFIG_BOOLEAN_NO);
    rrdlabels_add(labels, "_has_unstable_connection", has_unstable_connection ? "true" : "false", RRDLABEL_SRC_AUTO);

    uint32_t parent_state = rrdhost_is_parent_state_from_count(stream_receivers_currently_connected());
    __atomic_store_n(&is_parent_label_cached_state, parent_state, __ATOMIC_RELAXED);
    (void)rrdhost_update_is_parent_label_for_state(labels, parent_state);

    rrdlabels_add(labels, "_hostname", string2str(host->hostname), RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_os", string2str(host->os), RRDLABEL_SRC_AUTO);

    if (host->stream.snd.destination)
        rrdlabels_add(labels, "_streams_to", string2str(host->stream.snd.destination), RRDLABEL_SRC_AUTO);

    rrdlabels_add(labels, "_timezone", string2str(host->timezone), RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_abbrev_timezone", string2str(host->abbrev_timezone), RRDLABEL_SRC_AUTO);
}

static bool system_info_owns_label(const char *name, RRDLABEL_SRC source __maybe_unused, void *data __maybe_unused) {
    return rrdhost_system_info_label_is_runtime(name);
}

static bool reload_owns_label(const char *name, RRDLABEL_SRC source, void *data) {
    bool k8s_loaded = *(bool *)data;
    if (source & RRDLABEL_SRC_K8S)
        return k8s_loaded;
    return (source & RRDLABEL_SRC_CONFIG) || rrdhost_system_info_label_is_owned(name) ||
        !strcmp(name, "_is_parent") || !strcmp(name, "_has_unstable_connection") ||
        !strcmp(name, "_hostname") || !strcmp(name, "_os") || !strcmp(name, "_streams_to") ||
        !strcmp(name, "_timezone") || !strcmp(name, "_abbrev_timezone");
}

// Commit staged external inputs against the current cache under the publication lock.
static void rrdhost_commit_reloaded_labels(RRDHOST *host, RRDLABELS *labels,
                                         RRDLABELS *k8s_labels, bool k8s_loaded) {
    RRDLABELS *aclk_labels = rrdlabels_create();
    add_aclk_host_labels(aclk_labels);
    if (k8s_loaded)
        rrdlabels_copy(labels, k8s_labels);
    spinlock_lock(&host->rrdhost_update_lock);
    spinlock_lock(&is_parent_label_commit_spinlock);
    rrdhost_load_auto_labels(host, labels);
    (void)rrdlabels_replace_subset(host->rrdlabels, labels, reload_owns_label, &k8s_loaded);
    // ACLK owns these values even when configuration or Kubernetes supplies the same keys.
    rrdlabels_copy(host->rrdlabels, aclk_labels);
    spinlock_unlock(&is_parent_label_commit_spinlock);
    spinlock_unlock(&host->rrdhost_update_lock);
    rrdlabels_destroy(aclk_labels);
}

void reload_host_labels(void) {
    RRDLABELS *labels = rrdlabels_create();
    RRDLABELS *k8s_labels = rrdlabels_create();
    rrdhost_load_config_labels(labels);
    bool k8s_loaded = rrdhost_load_kubernetes_labels(k8s_labels);

    // Scripts and configuration reads happen before taking locks. Derive automatic labels from
    // the current cache at commit time, so a slow reload cannot roll back a periodic refresh.
    rrdhost_commit_reloaded_labels(localhost, labels, k8s_labels, k8s_loaded);
    rrdlabels_destroy(k8s_labels);
    rrdlabels_destroy(labels);

    // Explicit reloads also refresh ACLK labels; keep their existing unconditional notification.
    rrdhost_labels_changed(localhost);
}

struct rrdhost_system_info *rrdhost_system_info_labels_snapshot(RRDHOST *host, RRDLABELS **labels) {
    *labels = rrdlabels_create();
    spinlock_lock(&host->rrdhost_update_lock);
    struct rrdhost_system_info *info = rrdhost_system_info_dup(host->system_info);
    rrdlabels_copy(*labels, host->rrdlabels);
    spinlock_unlock(&host->rrdhost_update_lock);
    return info;
}

bool rrdhost_refresh_system_info(RRDHOST *host, struct rrdhost_system_info *candidate) {
    RRDLABELS *labels = rrdlabels_create();
    spinlock_lock(&host->rrdhost_update_lock);
    bool info_changed = rrdhost_system_info_update(host->system_info, candidate);
    rrdhost_system_info_to_rrdlabels(host->system_info, labels);
    bool labels_changed = rrdlabels_replace_subset(host->rrdlabels, labels, system_info_owns_label, NULL);
    if (info_changed)
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_UPDATE);
    spinlock_unlock(&host->rrdhost_update_lock);
    rrdlabels_destroy(labels);

    if (labels_changed)
        rrdhost_labels_changed(host);
    else if (info_changed)
        rrdhost_system_info_changed(host);
    return info_changed || labels_changed;
}

// ----------------------------------------------------------------------------
// unit tests

static int env_expand_unittest_check(const char *src, const char *expected, const char *test_name) {
    char buf[RRDLABELS_MAX_VALUE_LENGTH + 1];
    env_expand_labels_value(src, buf, sizeof(buf));

    int err = strcmp(buf, expected) != 0;
    fprintf(stderr, "  env_expand(%s): %s, expected '%s', got '%s'\n",
            test_name, err ? "FAILED" : "OK", expected, buf);
    return err;
}

static uint32_t is_parent_label_unittest_receiver_count = 0;

static uint32_t is_parent_label_unittest_count_reader(void) {
    return is_parent_label_unittest_receiver_count;
}

static void is_parent_label_unittest_set_cached(uint32_t state) {
    __atomic_store_n(&is_parent_label_cached_state, rrdhost_is_parent_state_from_count(state), __ATOMIC_RELAXED);
}

static int is_parent_label_unittest_check(
    RRDLABELS *labels, uint32_t count, bool expected_changed, const char *expected_value, const char *test_name) {
    is_parent_label_unittest_receiver_count = count;
    bool changed = rrdhost_update_is_parent_label(labels, is_parent_label_unittest_count_reader, false);

    char value[RRDLABELS_MAX_VALUE_LENGTH + 1];
    rrdlabels_get_value_strcpyz(labels, value, sizeof(value), "_is_parent");

    int err = (changed != expected_changed) || strcmp(value, expected_value) != 0;
    fprintf(stderr, "  _is_parent(%s): %s, expected changed=%s value='%s', got changed=%s value='%s'\n",
            test_name,
            err ? "FAILED" : "OK",
            expected_changed ? "true" : "false",
            expected_value,
            changed ? "true" : "false",
            value);

    return err;
}

static int is_parent_label_unittest_check_force(
    RRDLABELS *labels, uint32_t count, bool expected_changed, const char *expected_value, const char *test_name) {
    is_parent_label_unittest_receiver_count = count;
    bool changed = rrdhost_update_is_parent_label(labels, is_parent_label_unittest_count_reader, true);

    char value[RRDLABELS_MAX_VALUE_LENGTH + 1];
    rrdlabels_get_value_strcpyz(labels, value, sizeof(value), "_is_parent");

    int err = (changed != expected_changed) || strcmp(value, expected_value) != 0;
    fprintf(stderr, "  _is_parent(%s): %s, expected force changed=%s value='%s', got changed=%s value='%s'\n",
            test_name,
            err ? "FAILED" : "OK",
            expected_changed ? "true" : "false",
            expected_value,
            changed ? "true" : "false",
            value);

    return err;
}

static int is_parent_label_unittest_check_refreshed_nonzero_count(RRDLABELS *labels) {
    is_parent_label_unittest_set_cached(0);
    is_parent_label_unittest_receiver_count = 1;
    bool changed = rrdhost_update_is_parent_label(labels, is_parent_label_unittest_count_reader, false);

    char value[RRDLABELS_MAX_VALUE_LENGTH + 1];
    rrdlabels_get_value_strcpyz(labels, value, sizeof(value), "_is_parent");

    int err = changed || strcmp(value, "true") != 0;
    fprintf(stderr,
            "  _is_parent(refreshed non-zero receiver count after pending false transition): %s, "
            "cached state reset to false, current count=%u expected changed=false value='true', "
            "got changed=%s value='%s'\n",
            err ? "FAILED" : "OK",
            is_parent_label_unittest_receiver_count,
            changed ? "true" : "false",
            value);

    return err;
}

static int is_parent_label_unittest(void) {
    int errors = 0;

    RRDLABELS *labels = rrdlabels_create();

    is_parent_label_unittest_set_cached(1);
    errors += is_parent_label_unittest_check(labels, 0, true, "false", "initial zero receivers");
    errors += is_parent_label_unittest_check(labels, 0, false, "false", "unchanged zero receivers");
    errors += is_parent_label_unittest_check(labels, 2, true, "true", "two receivers after stale false");
    errors += is_parent_label_unittest_check(labels, 1, false, "true", "one receiver after already true");
    errors += is_parent_label_unittest_check_force(labels, 1, false, "true", "forced reload while already true");
    errors += is_parent_label_unittest_check_force(labels, 0, true, "false", "forced reload from true to false");
    errors += is_parent_label_unittest_check_force(labels, 2, true, "true", "forced reload from false to true");
    errors += is_parent_label_unittest_check_refreshed_nonzero_count(labels);
    errors += is_parent_label_unittest_check(labels, 0, true, "false", "last receiver disconnected");

    rrdlabels_destroy(labels);

    return errors;
}

static int os_metadata_labels_unittest(void) {
    struct rrdhost_system_info *system_info = rrdhost_system_info_create();
    RRDLABELS *labels = rrdlabels_create();
    int errors = 0;
    char value[RRDLABELS_MAX_VALUE_LENGTH + 1];

    (void)rrdhost_system_info_set_by_name(system_info, "NETDATA_HOST_OS_LABEL_NAME", "Ubuntu");
    (void)rrdhost_system_info_set_by_name(system_info, "NETDATA_HOST_OS_LABEL_VERSION", "24.04");
    (void)rrdhost_system_info_set_by_name(system_info, "NETDATA_HOST_OS_LABEL_RELEASE", "24.04");
    (void)rrdhost_system_info_set_by_name(system_info, "NETDATA_HOST_OS_LABEL_CODENAME", "noble");
    (void)rrdhost_system_info_set_by_name(system_info, "NETDATA_HOST_OS_VERSION", "Ubuntu 24.04.3 LTS");
    rrdhost_system_info_to_rrdlabels(system_info, labels);

    const struct {
        const char *name;
        const char *expected;
    } cases[] = {
        { "_os_name", "Ubuntu" },
        { "_os_version", "Ubuntu 24.04.3 LTS" },
        { "_os_marketing_version", "24.04" },
        { "_os_release", "24.04" },
        { "_os_codename", "noble" },
    };

    for(size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        rrdlabels_get_value_strcpyz(labels, value, sizeof(value), cases[i].name);
        int err = strcmp(value, cases[i].expected) != 0;
        fprintf(stderr, "  os metadata %s: %s, expected '%s', got '%s'\n", cases[i].name,
                err ? "FAILED" : "OK", cases[i].expected, value);
        errors += err;
    }

    rrdlabels_destroy(labels);
    rrdhost_system_info_free(system_info);
    return errors;
}

static int streamed_windows_system_info_unittest(void) {
    RRDLABELS *labels = rrdlabels_create();
    rrdlabels_add(labels, "_os", "windows", RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_os_name", "Windows", RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_os_version", "Microsoft Windows 11 Home", RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_os_marketing_version", "11", RRDLABEL_SRC_AUTO);

    struct rrdhost_system_info *system_info = rrdhost_system_info_from_host_labels(labels);
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    rrdhost_system_info_to_json_v1(wb, system_info);

    RRDLABELS *roundtrip = rrdlabels_create();
    rrdhost_system_info_to_rrdlabels(system_info, roundtrip);
    char version[RRDLABELS_MAX_VALUE_LENGTH + 1];
    char marketing_version[RRDLABELS_MAX_VALUE_LENGTH + 1];
    rrdlabels_get_value_strcpyz(roundtrip, version, sizeof(version), "_os_version");
    rrdlabels_get_value_strcpyz(roundtrip, marketing_version, sizeof(marketing_version), "_os_marketing_version");

    int err = !strstr(buffer_tostring(wb), "Microsoft Windows") ||
              !strstr(buffer_tostring(wb), "Microsoft Windows 11 Home") ||
              strcmp(version, "Microsoft Windows 11 Home") || strcmp(marketing_version, "11");
    fprintf(stderr, "  streamed Windows OS labels and public metadata: %s\n", err ? "FAILED" : "OK");

    rrdlabels_destroy(roundtrip);
    rrdhost_system_info_free(system_info);
    rrdlabels_destroy(labels);
    return err;
}

static bool refresh_test_label(RRDLABELS *labels, const char *key, const char *expected) {
    char value[RRDLABELS_MAX_VALUE_LENGTH + 1];
    rrdlabels_get_value_strcpyz(labels, value, sizeof(value), key);
    return expected ? !strcmp(value, expected) : !rrdlabels_exist(labels, key);
}

struct refresh_snapshot_test {
    RRDHOST *host;
    bool ready;
    bool done;
    int errors;
    size_t snapshots;
};

static void refresh_snapshot_test_reader(void *arg) {
    struct refresh_snapshot_test *test = arg;
    __atomic_store_n(&test->ready, true, __ATOMIC_RELEASE);
    do {
        RRDLABELS *labels;
        struct rrdhost_system_info *info = rrdhost_system_info_labels_snapshot(test->host, &labels);
        RRDLABELS *info_labels = rrdlabels_create();
        rrdhost_system_info_to_rrdlabels(info, info_labels);
        char ram[32], iface[32], ip[64];
        rrdlabels_get_value_strcpyz(info_labels, ram, sizeof(ram), "_system_ram_total");
        rrdlabels_get_value_strcpyz(info_labels, iface, sizeof(iface), "_net_default_iface");
        rrdlabels_get_value_strcpyz(info_labels, ip, sizeof(ip), "_net_default_iface_ip");
        bool generation_a = !strcmp(ram, "2048");
        test->errors += (!generation_a && strcmp(ram, "4096")) ||
            !refresh_test_label(labels, "_system_ram_total", ram) ||
            !refresh_test_label(labels, "_net_default_iface", iface) ||
            !refresh_test_label(labels, "_net_default_iface_ip", ip) ||
            strcmp(iface, generation_a ? "eth1" : "eth2") ||
            strcmp(ip, generation_a ? "192.0.2.1" : "192.0.2.2");
        test->snapshots++;
        rrdlabels_destroy(info_labels);
        rrdlabels_destroy(labels);
        rrdhost_system_info_free(info);
    } while (!__atomic_load_n(&test->done, __ATOMIC_ACQUIRE));
}

static int system_info_publication_unittest(void) {
    int errors = 0;
#define REFRESH_CHECK(expr) do { if (!(expr)) { \
    fprintf(stderr, "  system-info publication FAILED at line %d: %s\n", __LINE__, #expr); errors++; \
} } while (0)
    RRDHOST host = { 0 };
    spinlock_init(&host.rrdhost_update_lock);
    host.system_info = rrdhost_system_info_create();
    host.rrdlabels = rrdlabels_create();
    rrdhost_system_info_hops_set(host.system_info, 7);
    rrdhost_system_info_ml_capable_set(host.system_info, true);
    rrdhost_system_info_ml_enabled_set(host.system_info, true);
    rrdhost_system_info_mc_version_set(host.system_info, 3);
    rrdhost_system_info_set_by_name(host.system_info, "NETDATA_HOST_OS_ID", "preserved-os-id");
    rrdhost_system_info_set_by_name(host.system_info, "NETDATA_SYSTEM_TOTAL_RAM", "1024");
    rrdlabels_add(host.rrdlabels, "custom", "keep", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(host.rrdlabels, "_os_name", "startup label", RRDLABEL_SRC_AUTO);
    rrdlabels_add(host.rrdlabels, "k8s", "keep", RRDLABEL_SRC_K8S);
    rrdlabels_add(host.rrdlabels, "_stream_egress_iface", "wan0", RRDLABEL_SRC_AUTO);

    struct rrdhost_system_info *candidate = rrdhost_system_info_create();
    REFRESH_CHECK(rrdhost_system_info_detected_set(candidate, "NETDATA_SYSTEM_TOTAL_RAM", "2048"));
    REFRESH_CHECK(rrdhost_system_info_detected_set(candidate, "NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME", "eth1"));
    REFRESH_CHECK(rrdhost_system_info_detected_set(candidate, "NETDATA_SYSTEM_DEFAULT_INTERFACE_IP", "192.0.2.1"));
    REFRESH_CHECK(rrdhost_refresh_system_info(&host, candidate));
    REFRESH_CHECK(rrdhost_flag_check(&host, RRDHOST_FLAG_METADATA_INFO));
    REFRESH_CHECK(rrdhost_flag_check(&host, RRDHOST_FLAG_METADATA_LABELS));
    REFRESH_CHECK(refresh_test_label(host.rrdlabels, "_system_ram_total", "2048"));
    REFRESH_CHECK(refresh_test_label(host.rrdlabels, "_net_default_iface", "eth1"));

    RRDHOST_FLAGS flags = RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_LABELS |
        RRDHOST_FLAG_METADATA_UPDATE | RRDHOST_FLAG_PENDING_LABEL_RECHECK;
    rrdhost_flag_clear(&host, flags);
    uint32_t version = rrdlabels_version(host.rrdlabels);
    REFRESH_CHECK(!rrdhost_refresh_system_info(&host, candidate));
    REFRESH_CHECK(!rrdhost_flag_check(&host, flags));
    REFRESH_CHECK(rrdlabels_version(host.rrdlabels) == version);

    struct rrdhost_system_info *other = rrdhost_system_info_create();
    REFRESH_CHECK(rrdhost_system_info_detected_set(other, "NETDATA_SYSTEM_TOTAL_RAM", "4096"));
    REFRESH_CHECK(rrdhost_system_info_detected_set(other, "NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME", "eth2"));
    REFRESH_CHECK(rrdhost_system_info_detected_set(other, "NETDATA_SYSTEM_DEFAULT_INTERFACE_IP", "192.0.2.2"));
    struct refresh_snapshot_test test = { .host = &host };
    ND_THREAD *reader = nd_thread_create("SI-SNAPSHOT", NETDATA_THREAD_OPTION_DONT_LOG,
                                         refresh_snapshot_test_reader, &test);
    REFRESH_CHECK(reader);
    if (reader) {
        while (!__atomic_load_n(&test.ready, __ATOMIC_ACQUIRE))
            sleep_usec(100);
        for (size_t i = 0; i < 1000; i++)
            rrdhost_refresh_system_info(&host, i % 2 ? candidate : other);
        __atomic_store_n(&test.done, true, __ATOMIC_RELEASE);
        nd_thread_join(reader);
        REFRESH_CHECK(test.snapshots > 0 && test.errors == 0);
    }
    rrdhost_system_info_free(other);

    REFRESH_CHECK(rrdhost_system_info_detected_set(candidate, "NETDATA_SYSTEM_DEFAULT_INTERFACE_NAME", NULL));
    REFRESH_CHECK(rrdhost_system_info_detected_set(candidate, "NETDATA_SYSTEM_DEFAULT_INTERFACE_IP", NULL));
    REFRESH_CHECK(rrdhost_refresh_system_info(&host, candidate));
    REFRESH_CHECK(refresh_test_label(host.rrdlabels, "_net_default_iface", NULL));
    REFRESH_CHECK(refresh_test_label(host.rrdlabels, "_net_default_iface_ip", NULL));
    REFRESH_CHECK(refresh_test_label(host.rrdlabels, "custom", "keep"));
    REFRESH_CHECK(refresh_test_label(host.rrdlabels, "_os_name", "startup label"));
    REFRESH_CHECK(refresh_test_label(host.rrdlabels, "k8s", "keep"));
    REFRESH_CHECK(refresh_test_label(host.rrdlabels, "_stream_egress_iface", "wan0"));
    REFRESH_CHECK(rrdhost_system_info_hops(host.system_info) == 7);
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    rrdhost_system_info_to_url_encode_stream(wb, host.system_info);
    REFRESH_CHECK(strstr(buffer_tostring(wb), "NETDATA_SYSTEM_OS_ID=preserved-os-id"));
    REFRESH_CHECK(strstr(buffer_tostring(wb), "&ml_capable=1&ml_enabled=1&mc_version=3"));
    rrdhost_system_info_free(candidate);
    rrdhost_system_info_free(host.system_info);
    rrdlabels_destroy(host.rrdlabels);
    fprintf(stderr, "  system-info publication and concurrent snapshots: %s\n", errors ? "FAILED" : "OK");
#undef REFRESH_CHECK
    return errors;
}

static int reloaded_labels_unittest(void) {
    int errors = 0;
#define RELOAD_CHECK(expr) do { if (!(expr)) { \
    fprintf(stderr, "  reload labels FAILED at line %d: %s\n", __LINE__, #expr); errors++; \
} } while (0)
    RRDHOST host = { 0 };
    spinlock_init(&host.rrdhost_update_lock);
    host.system_info = rrdhost_system_info_create();
    host.rrdlabels = rrdlabels_create();
    host.hostname = string_strdupz("reload-test");
    host.os = string_strdupz("linux");
    host.timezone = string_strdupz("UTC");
    host.abbrev_timezone = string_strdupz("UTC");
    rrdhost_system_info_set_by_name(host.system_info, "NETDATA_SYSTEM_TOTAL_RAM", "1024");
    rrdhost_system_info_set_by_name(host.system_info, "NETDATA_HOST_OS_NAME", "startup-os");
    rrdlabels_add(host.rrdlabels, "old-config", "remove", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(host.rrdlabels, "k8s", "keep", RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S);

    // Stage inputs, then publish a refresh and independent label writes before reload commits.
    RRDLABELS *config = rrdlabels_create();
    RRDLABELS *k8s = rrdlabels_create();
    rrdlabels_add(config, "new-config", "new", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(config, "_aclk_available", "false", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(config, "_mqtt_version", "wrong", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(k8s, "k8s", "partial-failed-output", RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S);
    struct rrdhost_system_info *candidate = rrdhost_system_info_create();
    rrdhost_system_info_detected_set(candidate, "NETDATA_SYSTEM_TOTAL_RAM", "2048");
    RELOAD_CHECK(rrdhost_refresh_system_info(&host, candidate));
    rrdlabels_add(host.rrdlabels, "_stream_egress_iface", "wan1", RRDLABEL_SRC_AUTO);
    rrdlabels_add(host.rrdlabels, "_aclk_test", "live", RRDLABEL_SRC_AUTO);
    rrdhost_commit_reloaded_labels(&host, config, k8s, false);
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "_system_ram_total", "2048"));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "_os_name", "startup-os"));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "old-config", NULL));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "new-config", "new"));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "k8s", "keep"));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "_stream_egress_iface", "wan1"));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "_aclk_test", "live"));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "_aclk_available", "true"));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "_mqtt_version", "5"));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "_is_parent",
        stream_receivers_currently_connected() ? "true" : "false"));
    rrdlabels_destroy(config);
    rrdlabels_destroy(k8s);

    // A successful empty Kubernetes response removes the old labels.
    config = rrdlabels_create();
    k8s = rrdlabels_create();
    rrdhost_commit_reloaded_labels(&host, config, k8s, true);
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "k8s", NULL));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "new-config", NULL));
    RELOAD_CHECK(refresh_test_label(host.rrdlabels, "_system_ram_total", "2048"));
    rrdlabels_destroy(config);
    rrdlabels_destroy(k8s);
    rrdhost_system_info_free(candidate);
    rrdhost_system_info_free(host.system_info);
    rrdlabels_destroy(host.rrdlabels);
    string_freez(host.hostname);
    string_freez(host.os);
    string_freez(host.timezone);
    string_freez(host.abbrev_timezone);
    fprintf(stderr, "  reload staging/refresh interleaving and ownership: %s\n", errors ? "FAILED" : "OK");
#undef RELOAD_CHECK
    return errors;
}

int pluginsd_system_info_unittest(void);

int rrdhost_labels_unittest(void) {
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);
    int errors = 0;

    // --- set up test env vars ---
    setenv("ND_TEST_VAR", "hello", 1);
    setenv("ND_TEST_DC", "us-east", 1);
    setenv("ND_TEST_RACK", "rack42", 1);
    setenv("ND_TEST_EMPTY", "", 1);
    unsetenv("ND_TEST_UNSET");

    // no variables — pass through unchanged
    errors += env_expand_unittest_check("plain value", "plain value", "plain text");
    errors += env_expand_unittest_check("", "", "empty string");

    // basic variable expansion
    errors += env_expand_unittest_check("${ND_TEST_VAR}", "hello", "${VAR} set");
    errors += env_expand_unittest_check("prefix-${ND_TEST_VAR}", "prefix-hello", "prefix + ${VAR}");
    errors += env_expand_unittest_check("${ND_TEST_VAR}-suffix", "hello-suffix", "${VAR} + suffix");
    errors += env_expand_unittest_check("pre-${ND_TEST_VAR}-post", "pre-hello-post", "prefix + ${VAR} + suffix");

    // multiple variables
    errors += env_expand_unittest_check("${ND_TEST_DC}-${ND_TEST_RACK}", "us-east-rack42", "two vars adjacent");
    errors += env_expand_unittest_check("${ND_TEST_DC}/${ND_TEST_RACK}/${ND_TEST_VAR}", "us-east/rack42/hello", "three vars");
    errors += env_expand_unittest_check("dc=${ND_TEST_DC} rack=${ND_TEST_RACK}", "dc=us-east rack=rack42", "vars with literal labels");

    // default values — variable is set (default ignored)
    errors += env_expand_unittest_check("${ND_TEST_VAR:-fallback}", "hello", "default ignored when var set");
    errors += env_expand_unittest_check("${ND_TEST_DC:-other}", "us-east", "default ignored when var set (2)");

    // default values — variable is unset
    errors += env_expand_unittest_check("${ND_TEST_UNSET:-fallback}", "fallback", "default used when var unset");
    errors += env_expand_unittest_check("pre-${ND_TEST_UNSET:-fallback}-post", "pre-fallback-post", "default with surrounding text");

    // default values — variable is empty (treated same as unset)
    errors += env_expand_unittest_check("${ND_TEST_EMPTY:-fallback}", "fallback", "default used when var empty");

    // unset variable, no default — empty string
    errors += env_expand_unittest_check("${ND_TEST_UNSET}", "", "unset var no default = empty");
    errors += env_expand_unittest_check("pre-${ND_TEST_UNSET}-post", "pre--post", "unset var no default with text");

    // empty default — should resolve to empty string
    errors += env_expand_unittest_check("${ND_TEST_UNSET:-}", "", "empty default");
    errors += env_expand_unittest_check("pre-${ND_TEST_UNSET:-}-post", "pre--post", "empty default with text");

    // malformed syntax — no closing brace, copy literally
    errors += env_expand_unittest_check("${ND_TEST_UNCLOSED", "${ND_TEST_UNCLOSED", "no closing brace");
    errors += env_expand_unittest_check("pre-${ND_TEST_UNCLOSED", "pre-${ND_TEST_UNCLOSED", "no closing brace with prefix");

    // dollar sign not followed by brace — literal
    errors += env_expand_unittest_check("$notavar", "$notavar", "$ without {");
    errors += env_expand_unittest_check("price is $5", "price is $5", "$ with digit");
    errors += env_expand_unittest_check("$$", "$$", "double dollar");
    errors += env_expand_unittest_check("$", "$", "lone dollar at end");

    // empty variable name: ${} — getenv("") returns NULL, no default → empty
    errors += env_expand_unittest_check("${}", "", "empty var name");
    errors += env_expand_unittest_check("${:-fallback}", "fallback", "empty var name with default");

    // default containing :- (only first :- is the separator)
    errors += env_expand_unittest_check("${ND_TEST_UNSET:-a:-b}", "a:-b", "default containing :-");

    // no recursive expansion — env value containing ${...} is NOT re-expanded
    setenv("ND_TEST_NESTED", "${ND_TEST_VAR}", 1);
    errors += env_expand_unittest_check("${ND_TEST_NESTED}", "${ND_TEST_VAR}", "no recursive expansion");

    // default containing ${...} is NOT re-expanded
    errors += env_expand_unittest_check("${ND_TEST_UNSET:-${ND_TEST_VAR}}", "${ND_TEST_VAR}", "no expansion in default");

    // buffer overflow protection — expand into small buffer
    {
        char tiny[8];
        env_expand_labels_value("${ND_TEST_DC}", tiny, sizeof(tiny));
        // "us-east" is 7 chars, buffer is 8 (7+null) — should fit exactly
        int err = strcmp(tiny, "us-east") != 0;
        fprintf(stderr, "  env_expand(small buffer exact fit): %s, expected 'us-east', got '%s'\n",
                err ? "FAILED" : "OK", tiny);
        errors += err;
    }
    {
        char tiny[5];
        env_expand_labels_value("${ND_TEST_DC}", tiny, sizeof(tiny));
        // "us-east" is 7 chars, buffer is 5 (4+null) — should truncate to "us-e"
        int err = strcmp(tiny, "us-e") != 0;
        fprintf(stderr, "  env_expand(small buffer truncation): %s, expected 'us-e', got '%s'\n",
                err ? "FAILED" : "OK", tiny);
        errors += err;
    }
    {
        char tiny[5];
        env_expand_labels_value("abcdefghij", tiny, sizeof(tiny));
        // plain text truncation — should truncate to "abcd"
        int err = strcmp(tiny, "abcd") != 0;
        fprintf(stderr, "  env_expand(plain text truncation): %s, expected 'abcd', got '%s'\n",
                err ? "FAILED" : "OK", tiny);
        errors += err;
    }

    // value_has_env_variables() tests
    {
        struct {
            const char *input;
            bool expected;
        } detect_tests[] = {
            { "plain",                    false },
            { "",                         false },
            { "$notvar",                  false },
            { "$",                        false },
            { "${VAR}",                   true  },
            { "pre${VAR}post",            true  },
            { "${A}${B}",                 true  },
            { "$${}",                     true  },  // second $ starts ${
            { "${",                       true  },  // has ${ even without closing }
            { NULL, false }
        };
        for(int i = 0; detect_tests[i].input; i++) {
            bool result = value_has_env_variables(detect_tests[i].input);
            int err = result != detect_tests[i].expected;
            fprintf(stderr, "  value_has_env_variables('%s'): %s, expected %s, got %s\n",
                    detect_tests[i].input, err ? "FAILED" : "OK",
                    detect_tests[i].expected ? "true" : "false",
                    result ? "true" : "false");
            errors += err;
        }
    }

    // --- cleanup test env vars ---
    unsetenv("ND_TEST_VAR");
    unsetenv("ND_TEST_DC");
    unsetenv("ND_TEST_RACK");
    unsetenv("ND_TEST_EMPTY");
    unsetenv("ND_TEST_NESTED");

    errors += is_parent_label_unittest();
    errors += os_metadata_labels_unittest();
    errors += streamed_windows_system_info_unittest();
    errors += system_info_publication_unittest();
    errors += reloaded_labels_unittest();
    errors += pluginsd_system_info_unittest();

    fprintf(stderr, "%s: %d errors\n", __FUNCTION__, errors);
    return errors;
}
