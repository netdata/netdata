// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define PAGEFILE_OBJECT_NAME "Paging File"
#define PAGEFILE_CHART_TYPE "mem"
#define PAGEFILE_CHART_ID_PREFIX "pagefile_"
#define PAGEFILE_CHART_ID_SUFFIX "usage"

// NT object-manager prefix some hosts report on instance names; split to avoid trigraph warnings.
#define PAGEFILE_NT_PREFIX "\\?" "?\\"

struct paging_file {
    // Presence marker for eviction: the file was enumerated in this cycle.
    usec_t last_collected;
    // Deduplication marker: a valid sample was charted in this cycle.
    usec_t last_sampled;

    RRDSET *st;
    RRDDIM *rd_free;
    RRDDIM *rd_used;

    COUNTER_DATA usage;
};

static DICTIONARY *paging_files = NULL;

static void paging_file_obsolete_chart(struct paging_file *pf)
{
    rrdset_is_obsolete___safe_from_collector_thread(pf->st);
    pf->st = NULL;
    pf->rd_free = NULL;
    pf->rd_used = NULL;
}

static void paging_file_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    paging_file_obsolete_chart(value);
}

static void paging_file_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct paging_file *pf = value;

    // PERF_RAW_FRACTION: the value is pages in use, its base is the file size in pages.
    pf->usage.key = "% Usage";
}

static void initialize(void)
{
    paging_files = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct paging_file));

    dictionary_register_insert_callback(paging_files, paging_file_insert_cb, NULL);
    dictionary_register_delete_callback(paging_files, paging_file_delete_cb, NULL);
}

// The path is truncated to fit the ID limit; the hash of the full path keeps truncated IDs unique.
static void paging_file_chart_id(char *dst, size_t size, const char *path)
{
// Length of a string literal, without its NUL terminator.
#define PAGEFILE_LITERAL_LENGTH(s) (sizeof(s) - 1)
// One '_' between path and hash, one between hash and suffix.
#define PAGEFILE_ID_SEPARATOR_LENGTH 1
// A 64-bit hash printed as zero-padded hex.
#define PAGEFILE_HASH_HEX_DIGITS 16
// The full ID is "<type>.<id>": the type and its '.' share the ID limit.
#define PAGEFILE_TYPE_SEPARATOR_LENGTH 1
// RRD_ID_LENGTH_MAX counts the NUL terminator.
#define PAGEFILE_NUL_LENGTH 1

    const size_t path_length = strnlen(path, sizeof(windows_shared_buffer));
    XXH64_hash_t hash = XXH3_64bits(path, path_length);

    // Everything in the chart ID except the path itself.
    const size_t fixed_length = PAGEFILE_LITERAL_LENGTH(PAGEFILE_CHART_ID_PREFIX) + PAGEFILE_ID_SEPARATOR_LENGTH +
                                PAGEFILE_HASH_HEX_DIGITS + PAGEFILE_ID_SEPARATOR_LENGTH +
                                PAGEFILE_LITERAL_LENGTH(PAGEFILE_CHART_ID_SUFFIX);
    const size_t type_prefix_length =
        PAGEFILE_LITERAL_LENGTH(PAGEFILE_CHART_TYPE) + PAGEFILE_TYPE_SEPARATOR_LENGTH;

    size_t chart_id_limit = RRD_ID_LENGTH_MAX - PAGEFILE_NUL_LENGTH - type_prefix_length;
    if (chart_id_limit >= size)
        chart_id_limit = size - PAGEFILE_NUL_LENGTH;
    size_t visible_path_length = chart_id_limit > fixed_length ? chart_id_limit - fixed_length : 0;
    if (visible_path_length > path_length)
        visible_path_length = path_length;

    snprintfz(
        dst,
        size,
        PAGEFILE_CHART_ID_PREFIX "%.*s_%0*llx_" PAGEFILE_CHART_ID_SUFFIX,
        (int)visible_path_length,
        path,
        PAGEFILE_HASH_HEX_DIGITS,
        (unsigned long long)hash);
    netdata_fix_chart_name(dst);

#undef PAGEFILE_NUL_LENGTH
#undef PAGEFILE_TYPE_SEPARATOR_LENGTH
#undef PAGEFILE_HASH_HEX_DIGITS
#undef PAGEFILE_ID_SEPARATOR_LENGTH
#undef PAGEFILE_LITERAL_LENGTH
}

static void paging_file_chart(struct paging_file *pf, const char *path, uint64_t used_bytes, uint64_t free_bytes, int update_every)
{
    if (unlikely(!pf->st)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        paging_file_chart_id(id, sizeof(id), path);

        pf->st = rrdset_create_localhost(
            PAGEFILE_CHART_TYPE,
            id,
            NULL,
            "pagefile",
            "mem.pagefile_usage",
            "Paging file usage",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibPagingFile",
            NETDATA_CHART_PRIO_MEM_SWAP + 1,
            update_every,
            RRDSET_TYPE_STACKED);

        pf->rd_free = rrddim_add(pf->st, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        pf->rd_used = rrddim_add(pf->st, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(pf->st->rrdlabels, "pagefile", path, RRDLABEL_SRC_AUTO);
    }

    // A file removed and re-added reuses the chart that was marked obsolete.
    rrdset_isnot_obsolete___safe_from_collector_thread(pf->st);
    rrddim_set_by_pointer(pf->st, pf->rd_free, (collected_number)free_bytes);
    rrddim_set_by_pointer(pf->st, pf->rd_used, (collected_number)used_bytes);
    rrdset_done(pf->st);
}

static void paging_files_evict(usec_t now_ut, bool all)
{
    struct paging_file *pf;
    dfe_start_write(paging_files, pf)
    {
        if (all || pf->last_collected != now_ut)
            dictionary_del(paging_files, pf_dfe.name);
    }
    dfe_done(pf);
    dictionary_garbage_collect(paging_files);
}

static void do_paging_files(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    usec_t now_ut = now_monotonic_usec();

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, PAGEFILE_OBJECT_NAME);
    if (!pObjectType) {
        // The query succeeded without the object, so no paging file remains.
        paging_files_evict(now_ut, true);
        return;
    }

    const uint64_t page_size = os_get_system_page_size();
    bool complete = true;
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi) {
            complete = false;
            break;
        }

        // Without a name the file cannot be matched, so removals cannot be proven this cycle.
        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))) {
            complete = false;
            continue;
        }

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        const char *path = windows_shared_buffer;
        if (strncmp(path, PAGEFILE_NT_PREFIX, sizeof(PAGEFILE_NT_PREFIX) - 1) == 0)
            path += sizeof(PAGEFILE_NT_PREFIX) - 1;

        if (!*path) {
            complete = false;
            continue;
        }

        struct paging_file *pf = dictionary_set(paging_files, path, NULL, sizeof(*pf));
        if (!pf) {
            complete = false;
            continue;
        }

        pf->last_collected = now_ut;

        // Two instances normalizing to the same path: keep the first valid sample of this cycle, so an
        // invalid first instance does not hide a valid second one.
        if (pf->last_sampled == now_ut)
            continue;

        // An unreadable or invalid sample leaves a gap instead of reporting zero.
        if (!perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &pf->usage) ||
            pf->usage.current.CounterType != PERF_RAW_FRACTION)
            continue;

        // Both values are 32-bit page counts, so the byte products fit in 64 bits.
        const uint64_t used_pages = pf->usage.current.Data;
        const uint64_t size_pages = (uint64_t)pf->usage.current.Time;
        if (!size_pages || used_pages > size_pages)
            continue;

        pf->last_sampled = now_ut;
        paging_file_chart(
            pf, path, used_pages * page_size, (size_pages - used_pages) * page_size, update_every);
    }

    if (complete)
        paging_files_evict(now_ut, false);
}

int do_PerflibPagingFile(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    // Returning nonzero disables the module for good; keep probing instead.
    DWORD id = RegistryFindIDByName(PAGEFILE_OBJECT_NAME);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return 0;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return 0;

    do_paging_files(pDataBlock, update_every);

    return 0;
}

void do_PerflibPagingFile_cleanup(void)
{
    if (paging_files) {
        dictionary_destroy(paging_files);
        paging_files = NULL;
    }
}
