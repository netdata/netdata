// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"
#include "cgroup-lookup-resolver.h"
#include "collectors/common-cgroups/cgroup-path.h"

#if defined(OS_LINUX) && defined(ENABLE_CGROUPS_LOOKUP_SERVER)

typedef struct cgroup_lookup_resolver_prefix_cache {
    char *raw_prefix;
    char *canonical_prefix;
} CGROUP_LOOKUP_RESOLVER_PREFIX_CACHE;

typedef struct cgroup_lookup_resolver_state {
    netdata_mutex_t mutex;
    bool initialized;
    uint64_t generation;
    CGROUP_LOOKUP_RESOLVER_PREFIX_CACHE *positive;
    size_t positive_count;
    size_t positive_capacity;
    char **negative;
    size_t negative_count;
    size_t negative_capacity;
#ifdef NETDATA_INTERNAL_CHECKS
    size_t suffix_scans;
#endif
} CGROUP_LOOKUP_RESOLVER_STATE;

static CGROUP_LOOKUP_RESOLVER_STATE resolver_state;

static void cgroup_lookup_resolver_clear_locked(void)
{
    for(size_t i = 0; i < resolver_state.positive_count; i++) {
        freez(resolver_state.positive[i].raw_prefix);
        freez(resolver_state.positive[i].canonical_prefix);
    }
    freez(resolver_state.positive);
    resolver_state.positive = NULL;
    resolver_state.positive_count = 0;
    resolver_state.positive_capacity = 0;

    for(size_t i = 0; i < resolver_state.negative_count; i++)
        freez(resolver_state.negative[i]);
    freez(resolver_state.negative);
    resolver_state.negative = NULL;
    resolver_state.negative_count = 0;
    resolver_state.negative_capacity = 0;
#ifdef NETDATA_INTERNAL_CHECKS
    resolver_state.suffix_scans = 0;
#endif
}

void cgroup_lookup_resolver_init(void)
{
    if(resolver_state.initialized)
        return;

    netdata_mutex_init(&resolver_state.mutex);
    resolver_state.initialized = true;
}

void cgroup_lookup_resolver_cleanup(void)
{
    if(!resolver_state.initialized)
        return;

    netdata_mutex_lock(&resolver_state.mutex);
    cgroup_lookup_resolver_clear_locked();
    netdata_mutex_unlock(&resolver_state.mutex);

    netdata_mutex_destroy(&resolver_state.mutex);
    resolver_state = (CGROUP_LOOKUP_RESOLVER_STATE){ 0 };
}

#ifdef NETDATA_INTERNAL_CHECKS
size_t cgroup_lookup_resolver_suffix_scans_for_testing(void)
{
    if(!resolver_state.initialized)
        return 0;

    netdata_mutex_lock(&resolver_state.mutex);
    size_t scans = resolver_state.suffix_scans;
    netdata_mutex_unlock(&resolver_state.mutex);
    return scans;
}
#endif

static bool cgroup_lookup_path_has_parent_component(const char *path)
{
    if(!path)
        return false;

    const char *p = path;
    while(*p) {
        while(*p == '/')
            p++;

        const char *start = p;
        while(*p && *p != '/')
            p++;

        size_t len = (size_t)(p - start);
        if(len == 2 && start[0] == '.' && start[1] == '.')
            return true;
    }

    return false;
}

static bool cgroup_lookup_resolver_parse(
    const char *path,
    char **raw_prefix,
    char **suffix,
    size_t *prefix_len)
{
    if(raw_prefix)
        *raw_prefix = NULL;
    if(suffix)
        *suffix = NULL;
    if(prefix_len)
        *prefix_len = 0;

    if(!cgroup_path_is_namespace_relative(path))
        return false;

    size_t i = 0;
    while(path[i] == '/')
        i++;

    size_t first_component_start = 0;
    size_t first_component_end = 0;
    bool found = false;

    while(path[i]) {
        size_t start = i;
        while(path[i] && path[i] != '/')
            i++;
        size_t end = i;

        if(end - start == 2 && path[start] == '.' && path[start + 1] == '.') {
            while(path[i] == '/')
                i++;
            continue;
        }

        first_component_start = start;
        first_component_end = end;
        found = true;
        break;
    }

    if(!found || first_component_end == first_component_start)
        return false;

    size_t pfx_len = first_component_end;
    char *pfx = mallocz(pfx_len + 1);
    memcpy(pfx, path, pfx_len);
    pfx[pfx_len] = '\0';

    char *sfx = strdupz(path + first_component_start);
    if(cgroup_lookup_path_has_parent_component(sfx)) {
        freez(pfx);
        freez(sfx);
        return false;
    }

    if(raw_prefix)
        *raw_prefix = pfx;
    else
        freez(pfx);

    if(suffix)
        *suffix = sfx;
    else
        freez(sfx);

    if(prefix_len)
        *prefix_len = pfx_len;

    return true;
}

static const CGROUP_LOOKUP_RESOLVER_PREFIX_CACHE *cgroup_lookup_resolver_positive_get_locked(const char *raw_prefix)
{
    for(size_t i = 0; i < resolver_state.positive_count; i++) {
        if(strcmp(resolver_state.positive[i].raw_prefix, raw_prefix) == 0)
            return &resolver_state.positive[i];
    }

    return NULL;
}

static bool cgroup_lookup_resolver_negative_has_locked(const char *path)
{
    for(size_t i = 0; i < resolver_state.negative_count; i++) {
        if(strcmp(resolver_state.negative[i], path) == 0)
            return true;
    }

    return false;
}

static void cgroup_lookup_resolver_positive_add_locked(const char *raw_prefix, const char *canonical_prefix)
{
    if(cgroup_lookup_resolver_positive_get_locked(raw_prefix))
        return;

    if(resolver_state.positive_count == resolver_state.positive_capacity) {
        resolver_state.positive_capacity = resolver_state.positive_capacity ? resolver_state.positive_capacity * 2 : 8;
        resolver_state.positive = reallocz(
            resolver_state.positive,
            resolver_state.positive_capacity * sizeof(*resolver_state.positive));
    }

    CGROUP_LOOKUP_RESOLVER_PREFIX_CACHE *entry = &resolver_state.positive[resolver_state.positive_count++];
    entry->raw_prefix = strdupz(raw_prefix);
    entry->canonical_prefix = strdupz(canonical_prefix);
}

static void cgroup_lookup_resolver_negative_add_locked(const char *path)
{
    if(cgroup_lookup_resolver_negative_has_locked(path))
        return;

    if(resolver_state.negative_count == resolver_state.negative_capacity) {
        resolver_state.negative_capacity = resolver_state.negative_capacity ? resolver_state.negative_capacity * 2 : 16;
        resolver_state.negative = reallocz(
            resolver_state.negative,
            resolver_state.negative_capacity * sizeof(*resolver_state.negative));
    }

    resolver_state.negative[resolver_state.negative_count++] = strdupz(path);
}

static bool cgroup_lookup_clean_join(const char *base, const char *relative, char *dst, size_t dst_size)
{
    if(!base || !relative || !dst || dst_size == 0)
        return false;

    size_t base_len = strlen(base);
    size_t relative_len = strlen(relative);
    bool add_separator = base_len && relative_len && base[base_len - 1] != '/' && relative[0] != '/';
    char *combined = mallocz(base_len + relative_len + (add_separator ? 2 : 1));
    snprintfz(
        combined,
        base_len + relative_len + (add_separator ? 2 : 1),
        "%s%s%s",
        base,
        add_separator ? "/" : "",
        relative);

    const char **components = callocz(base_len + relative_len + 2, sizeof(*components));
    size_t *component_lens = callocz(base_len + relative_len + 2, sizeof(*component_lens));
    size_t component_count = 0;

    bool ok = true;
    char *p = combined;
    while(*p) {
        while(*p == '/')
            p++;

        char *start = p;
        while(*p && *p != '/')
            p++;

        size_t len = (size_t)(p - start);
        if(len == 0 || (len == 1 && start[0] == '.'))
            continue;

        if(len == 2 && start[0] == '.' && start[1] == '.') {
            if(component_count == 0) {
                ok = false;
                break;
            }
            component_count--;
            continue;
        }

        components[component_count] = start;
        component_lens[component_count] = len;
        component_count++;
    }

    if(ok) {
        if(dst_size < 2)
            ok = false;
        else {
            size_t used = 0;
            dst[used++] = '/';
            for(size_t i = 0; i < component_count; i++) {
                if(i) {
                    if(used + 1 >= dst_size) {
                        ok = false;
                        break;
                    }
                    dst[used++] = '/';
                }

                if(used + component_lens[i] >= dst_size) {
                    ok = false;
                    break;
                }
                memcpy(&dst[used], components[i], component_lens[i]);
                used += component_lens[i];
            }
            dst[used] = '\0';
        }
    }

    freez(component_lens);
    freez(components);
    freez(combined);
    return ok;
}

static bool cgroup_lookup_prefix_has_parent_component(const char *path)
{
    if(!path)
        return true;

    const char *p = path;
    while(*p) {
        while(*p == '/')
            p++;

        const char *start = p;
        while(*p && *p != '/')
            p++;

        size_t len = (size_t)(p - start);
        if(len == 2 && start[0] == '.' && start[1] == '.')
            return true;
    }

    return false;
}

static bool cgroup_lookup_build_host_proc_self_cgroup_path(char *dst, size_t dst_size)
{
    static const char proc_self_cgroup[] = "/proc/self/cgroup";

    if(!dst || dst_size == 0 || !netdata_configured_host_prefix || !*netdata_configured_host_prefix)
        return false;

    const char *prefix = netdata_configured_host_prefix;
    if(prefix[0] != '/' || cgroup_lookup_prefix_has_parent_component(prefix))
        return false;

    size_t prefix_len = strlen(prefix);
    size_t child_len = sizeof(proc_self_cgroup) - 1;
    bool prefix_has_trailing_slash = prefix[prefix_len - 1] == '/';
    size_t child_offset = prefix_has_trailing_slash ? 1 : 0;
    if(prefix_len + child_len - child_offset >= dst_size)
        return false;

    memcpy(dst, prefix, prefix_len);
    memcpy(dst + prefix_len, proc_self_cgroup + child_offset, child_len + 1 - child_offset);
    return true;
}

static bool cgroup_lookup_stat_under_base(const char *base, const char *cgroup_id, struct stat *st)
{
    if(!base || !*base || !cgroup_id || !st)
        return false;

    char path[FILENAME_MAX + 1];
    snprintfz(path, sizeof(path) - 1, "%s%s", base, cgroup_id);
    return stat(path, st) == 0;
}

static bool cgroup_lookup_strip_host_prefix(const char *base, char *dst, size_t dst_size)
{
    if(!base || !dst || dst_size == 0 || !netdata_configured_host_prefix || !*netdata_configured_host_prefix)
        return false;

    size_t prefix_len = strlen(netdata_configured_host_prefix);
    if(strncmp(base, netdata_configured_host_prefix, prefix_len) != 0)
        return false;
    if(base[prefix_len] && base[prefix_len] != '/')
        return false;

    const char *stripped = base + prefix_len;
    if(!*stripped)
        stripped = "/";

    if(strlen(stripped) >= dst_size)
        return false;

    strncpyz(dst, stripped, dst_size - 1);
    return true;
}

static bool cgroup_lookup_stat_plugin_visible_base(const char *canonical_base, const char *cgroup_id, struct stat *st)
{
    char plugin_base[FILENAME_MAX + 1];

    if(cgroup_lookup_strip_host_prefix(canonical_base, plugin_base, sizeof(plugin_base)) &&
       cgroup_lookup_stat_under_base(plugin_base, cgroup_id, st))
        return true;

    if(netdata_configured_host_prefix && *netdata_configured_host_prefix)
        return false;

    return cgroup_lookup_stat_under_base(canonical_base, cgroup_id, st);
}

static bool cgroup_lookup_stat_self_cgroup_dir(const char *cgroup_id, struct stat *st)
{
    if(cgroup_use_unified_cgroups && cgroup_unified_base) {
        if(cgroup_lookup_stat_plugin_visible_base(cgroup_unified_base, cgroup_id, st))
            return true;

        return false;
    }

    if(cgroup_cpuacct_base) {
        if(cgroup_lookup_stat_plugin_visible_base(cgroup_cpuacct_base, cgroup_id, st))
            return true;
    }

    if(cgroup_cpuset_base) {
        if(cgroup_lookup_stat_plugin_visible_base(cgroup_cpuset_base, cgroup_id, st))
            return true;
    }

    if(cgroup_blkio_base) {
        if(cgroup_lookup_stat_plugin_visible_base(cgroup_blkio_base, cgroup_id, st))
            return true;
    }

    if(cgroup_memory_base) {
        if(cgroup_lookup_stat_plugin_visible_base(cgroup_memory_base, cgroup_id, st))
            return true;
    }

    return false;
}

static bool cgroup_lookup_read_proc_self_cgroup(const char *proc_path, char *dst, size_t dst_size)
{
    char content[8192];

    if(read_txt_file(proc_path, content, sizeof(content)) != 0)
        return false;

    bool ok = cgroup_path_parse_proc_pid_cgroup_content(content, dst, dst_size);
    return ok;
}

static bool cgroup_lookup_path_has_suffix_component_boundary(const char *path, const char *suffix)
{
    if(!path || !suffix)
        return false;

    size_t path_len = strlen(path);
    size_t suffix_len = strlen(suffix);
    if(suffix_len == 1 && suffix[0] == '/')
        return true;

    if(path_len <= suffix_len)
        return false;

    const char *tail = path + path_len - suffix_len;
    if(memcmp(tail, suffix, suffix_len) != 0)
        return false;

    return suffix[0] == '/' || tail[-1] == '/';
}

static bool cgroup_lookup_namespace_base_from_self_path(
    const CGROUP_SNAPSHOT_STORE *store,
    const char *self_cgroup,
    char *base,
    size_t base_size)
{
    if(!store || !self_cgroup || !base || base_size < 2)
        return false;

    if(strcmp(self_cgroup, "/") == 0)
        return false;

    struct stat st;
    if(!cgroup_lookup_stat_self_cgroup_dir(self_cgroup, &st))
        return false;

    const CGROUP_SNAPSHOT_ENTRY *entry =
        cgroup_snapshot_store_find_unique_identity(store, st.st_dev, st.st_ino);
    if(!entry)
        return false;

    const char *canonical_self = string2str(entry->id);
    if(!cgroup_lookup_path_has_suffix_component_boundary(canonical_self, self_cgroup))
        return false;

    size_t canonical_len = string_strlen(entry->id);
    size_t self_len = strlen(self_cgroup);
    size_t base_len = canonical_len - self_len;
    if(base_len == 0 || base_len >= base_size)
        return false;

    memcpy(base, canonical_self, base_len);
    base[base_len] = '\0';
    return true;
}

static const CGROUP_SNAPSHOT_ENTRY *cgroup_lookup_resolve_with_self_path(
    const CGROUP_SNAPSHOT_STORE *store,
    const char *self_cgroup,
    const char *path)
{
    char base[FILENAME_MAX + 1];
    if(!cgroup_lookup_namespace_base_from_self_path(store, self_cgroup, base, sizeof(base)))
        return NULL;

    char candidate[FILENAME_MAX + 1];
    if(!cgroup_lookup_clean_join(base, path, candidate, sizeof(candidate)))
        return NULL;

    return cgroup_snapshot_store_find(store, candidate, strlen(candidate));
}

static const CGROUP_SNAPSHOT_ENTRY *cgroup_lookup_resolve_with_self_views(
    const CGROUP_SNAPSHOT_STORE *store,
    const char *path)
{
    char proc_path[FILENAME_MAX + 1];
    char self_cgroup[FILENAME_MAX + 1];

    if(netdata_configured_host_prefix && *netdata_configured_host_prefix) {
        if(!cgroup_lookup_build_host_proc_self_cgroup_path(proc_path, sizeof(proc_path)))
            return NULL;
    }
    else
        strncpyz(proc_path, "/proc/self/cgroup", sizeof(proc_path) - 1);

    if(cgroup_lookup_read_proc_self_cgroup(proc_path, self_cgroup, sizeof(self_cgroup))) {
        const CGROUP_SNAPSHOT_ENTRY *entry = cgroup_lookup_resolve_with_self_path(store, self_cgroup, path);
        if(entry)
            return entry;
    }

    return NULL;
}

static bool cgroup_lookup_replacement_prefix(
    const CGROUP_SNAPSHOT_ENTRY *entry,
    const char *suffix,
    char *dst,
    size_t dst_size)
{
    if(!entry || !entry->id || !suffix || !dst || !dst_size)
        return false;

    const char *id = string2str(entry->id);
    size_t id_len = string_strlen(entry->id);
    size_t suffix_len = strlen(suffix);
    if(id_len < suffix_len)
        return false;

    size_t suffix_offset = id_len - suffix_len;
    const char *slash = strchr(suffix, '/');
    size_t first_component_len = slash ? (size_t)(slash - suffix) : suffix_len;
    size_t prefix_len = suffix_offset + first_component_len;
    if(prefix_len >= dst_size)
        return false;

    memcpy(dst, id, prefix_len);
    dst[prefix_len] = '\0';
    return true;
}

static const CGROUP_SNAPSHOT_ENTRY *cgroup_lookup_resolver_apply_positive(
    const CGROUP_SNAPSHOT_STORE *store,
    const CGROUP_LOOKUP_RESOLVER_PREFIX_CACHE *cache,
    const char *path,
    size_t raw_prefix_len)
{
    char candidate[FILENAME_MAX + 1];
    const char *remainder = path + raw_prefix_len;
    size_t replacement_len = strlen(cache->canonical_prefix);
    size_t remainder_len = strlen(remainder);

    if(replacement_len + remainder_len >= sizeof(candidate))
        return NULL;

    memcpy(candidate, cache->canonical_prefix, replacement_len);
    memcpy(candidate + replacement_len, remainder, remainder_len + 1);

    if(cgroup_lookup_path_has_parent_component(candidate))
        return NULL;

    return cgroup_snapshot_store_find(store, candidate, strlen(candidate));
}

const CGROUP_SNAPSHOT_ENTRY *cgroup_lookup_resolver_resolve(
    const CGROUP_SNAPSHOT_STORE *store,
    uint64_t generation,
    const char *path,
    size_t path_len)
{
    if(!store || !generation || !path || !path_len || !resolver_state.initialized)
        return NULL;

    char *pathz = mallocz(path_len + 1);
    memcpy(pathz, path, path_len);
    pathz[path_len] = '\0';

    char *raw_prefix = NULL;
    char *suffix = NULL;
    size_t raw_prefix_len = 0;
    if(!cgroup_lookup_resolver_parse(pathz, &raw_prefix, &suffix, &raw_prefix_len)) {
        freez(pathz);
        return NULL;
    }

    const CGROUP_SNAPSHOT_ENTRY *resolved = NULL;

    // Lock order is snapshot read lock first (held by caller), resolver mutex second.
    netdata_mutex_lock(&resolver_state.mutex);
    if(resolver_state.generation != generation) {
        cgroup_lookup_resolver_clear_locked();
        resolver_state.generation = generation;
    }

    if(cgroup_lookup_resolver_negative_has_locked(pathz))
        goto done;

    const CGROUP_LOOKUP_RESOLVER_PREFIX_CACHE *positive =
        cgroup_lookup_resolver_positive_get_locked(raw_prefix);
    if(positive) {
        resolved = cgroup_lookup_resolver_apply_positive(store, positive, pathz, raw_prefix_len);
        if(!resolved)
            cgroup_lookup_resolver_negative_add_locked(pathz);
        goto done;
    }

    resolved = cgroup_lookup_resolve_with_self_views(store, pathz);
    if(!resolved) {
#ifdef NETDATA_INTERNAL_CHECKS
        resolver_state.suffix_scans++;
#endif
        bool duplicate = false;
        resolved = cgroup_snapshot_store_find_unique_suffix(store, suffix, strlen(suffix), &duplicate);
        if(duplicate)
            resolved = NULL;
    }

    if(resolved) {
        char canonical_prefix[FILENAME_MAX + 1];
        if(cgroup_lookup_replacement_prefix(resolved, suffix, canonical_prefix, sizeof(canonical_prefix)))
            cgroup_lookup_resolver_positive_add_locked(raw_prefix, canonical_prefix);
    }
    else
        cgroup_lookup_resolver_negative_add_locked(pathz);

done:
    netdata_mutex_unlock(&resolver_state.mutex);

    freez(suffix);
    freez(raw_prefix);
    freez(pathz);
    return resolved;
}

#endif
