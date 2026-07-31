// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>
#include <stdlib.h>
#include <pthread.h>

#include "libnetdata/libnetdata.h"
#include "ebpf_library.h"
#include "../ebpf.h"
#include "../ebpf_process.h"
#include <ifaddrs.h>

/*****************************************************************
 *
 *  DIMENSION WRITING FUNCTIONS
 *
 *****************************************************************/

void write_chart_dimension(const char *dim, long long value)
{
    printf("SET %s = %lld\n", dim, value);
}

void ebpf_write_global_dimension(char *name, char *id, char *algorithm)
{
    printf("DIMENSION %s %s %s 1 1\n", name, id, algorithm);
}

void ebpf_create_global_dimension(void *ptr, int end)
{
    netdata_publish_syscall_t *move = ptr;

    int i = 0;
    while (move && i < end) {
        ebpf_write_global_dimension(move->name, move->dimension, move->algorithm);

        move = move->next;
        i++;
    }
}

/*****************************************************************
 *
 *  CHART WRITING FUNCTIONS
 *
 *****************************************************************/

void write_count_chart(char *name, char *family, netdata_publish_syscall_t *move, uint32_t end)
{
    ebpf_write_begin_chart(family, name, "");

    uint32_t i;
    for (i = 0; move && i < end; i++) {
        write_chart_dimension(move->name, move->ncall);
        move = move->next;
    }

    ebpf_write_end_chart();
}

void write_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end)
{
    ebpf_write_begin_chart(family, name, "");

    int i;
    for (i = 0; move && i < end; i++) {
        write_chart_dimension(move->name, move->nerr);
        move = move->next;
    }

    ebpf_write_end_chart();
}

void ebpf_one_dimension_write_charts(char *family, char *chart, char *dim, long long v1)
{
    ebpf_write_begin_chart(family, chart, "");

    write_chart_dimension(dim, v1);

    ebpf_write_end_chart();
}

void write_io_chart(char *chart, char *family, char *dwrite, long long vwrite, char *dread, long long vread)
{
    ebpf_write_begin_chart(family, chart, "");

    write_chart_dimension(dwrite, vwrite);
    write_chart_dimension(dread, vread);

    ebpf_write_end_chart();
}

void write_histogram_chart(char *family, char *name, const uint64_t *hist, char **dimensions, uint32_t end)
{
    ebpf_write_begin_chart(family, name, "");

    uint32_t i;
    for (i = 0; i < end; i++) {
        write_chart_dimension(dimensions[i], (long long)hist[i]);
    }

    ebpf_write_end_chart();

    fflush(stdout);
}

/*****************************************************************
 *
 *  CHART CREATION FUNCTIONS
 *
 *****************************************************************/

void ebpf_write_chart_cmd(
    char *type,
    char *id,
    char *suffix,
    char *title,
    char *units,
    char *family,
    char *charttype,
    char *context,
    int order,
    int update_every,
    char *module)
{
    printf(
        "CHART %s.%s%s '' '%s' '%s' '%s' '%s' '%s' %d %d '' 'ebpf.plugin' '%s'\n",
        type,
        id,
        suffix,
        title,
        units,
        (family) ? family : "",
        (context) ? context : "",
        (charttype) ? charttype : "",
        order,
        update_every,
        module);
}

void ebpf_write_chart_obsolete(
    char *type,
    const char *id,
    char *suffix,
    char *title,
    char *units,
    char *family,
    char *charttype,
    const char *context,
    int order,
    int update_every)
{
    printf(
        "CHART %s.%s%s '' '%s' '%s' '%s' '%s' '%s' %d %d 'obsolete'\n",
        type,
        id,
        suffix,
        title,
        units,
        (family) ? family : "",
        (context) ? context : "",
        (charttype) ? charttype : "",
        order,
        update_every);
}

void ebpf_create_chart(
    char *type,
    char *id,
    char *title,
    char *units,
    char *family,
    char *context,
    char *charttype,
    int order,
    void (*ncd)(void *, int),
    void *move,
    int end,
    int update_every,
    char *module)
{
    ebpf_write_chart_cmd(type, id, "", title, units, family, charttype, context, order, update_every, module);

    if (ncd) {
        ncd(move, end);
    }
}

/*****************************************************************
 *
 *  ARAL STATISTIC CHARTS
 *
 *****************************************************************/

int ebpf_statistic_create_aral_chart(char *name, ebpf_module_t *em)
{
    static int priority = NETDATA_EBPF_ORDER_STAT_ARAL_BEGIN;
    static netdata_mutex_t priority_mutex;
    static int priority_mutex_initialized = 0;

    if (!priority_mutex_initialized) {
        netdata_mutex_init(&priority_mutex);
        priority_mutex_initialized = 1;
    }

    char *mem = NETDATA_EBPF_STAT_DIMENSION_MEMORY;
    char *aral = NETDATA_EBPF_STAT_DIMENSION_ARAL;

    snprintfz(em->memory_usage, NETDATA_EBPF_CHART_MEM_LENGTH - 1, "aral_%s_size", name);
    snprintfz(em->memory_allocations, NETDATA_EBPF_CHART_MEM_LENGTH - 1, "aral_%s_alloc", name);

    netdata_mutex_lock(&priority_mutex);
    int ret_priority = priority;
    priority += 2;
    netdata_mutex_unlock(&priority_mutex);

    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        em->memory_usage,
        "",
        "Bytes allocated for ARAL.",
        "bytes",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_size",
        ret_priority,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(mem, mem, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        em->memory_allocations,
        "",
        "Calls to allocate memory.",
        "calls",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_alloc",
        ret_priority + 1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(aral, aral, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    return ret_priority;
}

void ebpf_statistic_obsolete_aral_chart(ebpf_module_t *em, int prio)
{
    ebpf_write_chart_obsolete(
        NETDATA_MONITORING_FAMILY,
        em->memory_usage,
        "",
        "Bytes allocated for ARAL.",
        "bytes",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_size",
        prio++,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_MONITORING_FAMILY,
        em->memory_allocations,
        "",
        "Calls to allocate memory.",
        "calls",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_alloc",
        prio++,
        em->update_every);
}

void ebpf_send_data_aral_chart(ARAL *memory, ebpf_module_t *em)
{
    if (!memory)
        return;

    char *mem = NETDATA_EBPF_STAT_DIMENSION_MEMORY;
    char *aral = NETDATA_EBPF_STAT_DIMENSION_ARAL;

    struct aral_statistics *stats = aral_get_statistics(memory);

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, em->memory_usage, "");
    write_chart_dimension(mem, (long long)stats->structures.allocated_bytes);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(NETDATA_MONITORING_FAMILY, em->memory_allocations, "");
    write_chart_dimension(aral, (long long)stats->structures.allocations);
    ebpf_write_end_chart();
}

/*****************************************************************
 *
 *  CONFIG FILE PARSER FUNCTIONS
 *
 *****************************************************************/

void ebpf_how_to_load(const char *ptr)
{
    if (!strcasecmp(ptr, EBPF_CFG_LOAD_MODE_RETURN))
        ebpf_set_thread_mode(MODE_RETURN);
    else if (!strcasecmp(ptr, EBPF_CFG_LOAD_MODE_DEFAULT))
        ebpf_set_thread_mode(MODE_ENTRY);
    else
        netdata_log_error("the option %s for \"ebpf load mode\" is not a valid option.", ptr);
}

void ebpf_set_apps_mode(netdata_apps_integration_flags_t value)
{
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_modules[i].apps_charts = value;
    }
}

void ebpf_update_interval(int update_every)
{
    int i;

    int value = (int)inicfg_get_number(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_UPDATE_EVERY, update_every);

    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
        ebpf_modules[i].update_every = value;
    }
}

void ebpf_update_table_size()
{
    uint32_t value = (uint32_t)inicfg_get_number(
        &collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_PID_SIZE, ND_EBPF_DEFAULT_PID_SIZE);
    for (int i = 0; ebpf_modules[i].info.thread_name; i++) {
        ebpf_modules[i].pid_map_size = value;
    }
}

void ebpf_update_lifetime()
{
    uint32_t value =
        (uint32_t)inicfg_get_number(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_LIFETIME, EBPF_DEFAULT_LIFETIME);

    for (int i = 0; ebpf_modules[i].info.thread_name; i++) {
        ebpf_modules[i].lifetime = value;
    }
}

void ebpf_set_load_mode(netdata_ebpf_load_mode_t load, netdata_ebpf_load_mode_t origin)
{
    int i;
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
        ebpf_modules[i].load &= ~NETDATA_EBPF_LOAD_METHODS;
        ebpf_modules[i].load |= load | origin;
    }
}

void ebpf_update_load_mode(const char *str, netdata_ebpf_load_mode_t origin)
{
    netdata_ebpf_load_mode_t load = epbf_convert_string_to_load_mode(str);

    ebpf_set_load_mode(load, origin);
}

void ebpf_update_object_flavor(const char *str, netdata_ebpf_load_mode_t origin)
{
    netdata_ebpf_load_mode_t load = ebpf_convert_object_flavor_to_load_mode(str);

    ebpf_set_load_mode(load, origin);
}

void ebpf_update_map_per_core()
{
    int value = inicfg_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_MAPS_PER_CORE, CONFIG_BOOLEAN_YES);

    for (int i = 0; ebpf_modules[i].info.thread_name; i++) {
        ebpf_modules[i].maps_per_core = value;
    }
}

void ebpf_set_ipc_value(const char *integration)
{
    if (!strcmp(integration, NETDATA_EBPF_IPC_INTEGRATION_SHM)) {
        integration_with_collectors = NETDATA_EBPF_INTEGRATION_SHM;
    } else if (!strcmp(integration, NETDATA_EBPF_IPC_INTEGRATION_DISABLED)) {
        integration_with_collectors = NETDATA_EBPF_INTEGRATION_DISABLED;
    } else {
        netdata_log_error("unsupported [ipc] integration '%s'; using 'disabled'", integration);
        integration_with_collectors = NETDATA_EBPF_INTEGRATION_DISABLED;
    }
}

void ebpf_parse_ipc_section()
{
    const char *integration = inicfg_get(
        &collector_config,
        NETDATA_EBPF_IPC_SECTION,
        NETDATA_EBPF_IPC_INTEGRATION,
        NETDATA_EBPF_IPC_INTEGRATION_DISABLED);
    ebpf_set_ipc_value(integration);
}

void ebpf_set_thread_mode(netdata_run_mode_t lmode)
{
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_modules[i].mode = lmode;
    }
}

void ebpf_enable_specific_chart(ebpf_module_t *em, int disable_cgroup)
{
    ebpf_module_enabled_set(em, NETDATA_THREAD_EBPF_RUNNING);

    if (!disable_cgroup) {
        em->cgroup_charts = CONFIG_BOOLEAN_YES;
    }

    em->global_charts = CONFIG_BOOLEAN_YES;
}

void ebpf_enable_chart(int idx, int disable_cgroup)
{
    int i;
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
        if (i == idx) {
            ebpf_enable_specific_chart(&ebpf_modules[i], disable_cgroup);
            break;
        }
    }
}

int ebpf_load_collector_config(char *path, int *disable_cgroups, int update_every)
{
    char lpath[4096];
    netdata_ebpf_load_mode_t origin;

    snprintf(lpath, 4095, "%s/%s", path, NETDATA_EBPF_CONFIG_FILE);
    if (!inicfg_load(&collector_config, lpath, 0, NULL)) {
        snprintf(lpath, 4095, "%s/%s", path, NETDATA_EBPF_OLD_CONFIG_FILE);
        if (!inicfg_load(&collector_config, lpath, 0, NULL)) {
            return -1;
        }
        origin = EBPF_LOADED_FROM_STOCK;
    } else
        origin = EBPF_LOADED_FROM_USER;

    read_collector_values(disable_cgroups, update_every, origin);
    ebpf_parse_ipc_section();

    return 0;
}

void ebpf_load_thread_config()
{
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_update_module(&ebpf_modules[i], default_btf, running_on_kernel, isrh);
    }
}

void read_collector_values(int *disable_cgroups, int update_every, netdata_ebpf_load_mode_t origin)
{
    const char *value;
    if (inicfg_exists(&collector_config, EBPF_GLOBAL_SECTION, "load"))
        value = inicfg_get(&collector_config, EBPF_GLOBAL_SECTION, "load", EBPF_CFG_LOAD_MODE_DEFAULT);
    else
        value = inicfg_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_LOAD_MODE, EBPF_CFG_LOAD_MODE_DEFAULT);

    ebpf_how_to_load(value);

    btf_path = inicfg_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_PROGRAM_PATH, EBPF_DEFAULT_BTF_PATH);

#ifdef LIBBPF_MAJOR_VERSION
    default_btf = ebpf_load_btf_file(btf_path, EBPF_DEFAULT_BTF_FILE);
#endif

    value = inicfg_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_TYPE_FORMAT, EBPF_CFG_DEFAULT_PROGRAM);
    netdata_ebpf_load_mode_t fmt_load = epbf_convert_string_to_load_mode(value);
    ebpf_set_load_mode(fmt_load, origin);

    /* Apply object flavor only when type format is auto/default (PLAY_DICE).
     * An explicit "legacy" or "co-re" in ebpf type format must not be silently
     * overridden by the stock "ebpf object flavor = buffer" shipped in ebpf.d.conf. */
    value = inicfg_get(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_OBJECT_FLAVOR, NULL);
    if (value && !(fmt_load & (EBPF_LOAD_LEGACY | EBPF_LOAD_CORE)))
        ebpf_update_object_flavor(value, origin);

    ebpf_update_interval(update_every);

    ebpf_update_table_size();

    ebpf_update_lifetime();

    uint32_t enabled = inicfg_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, "disable apps", CONFIG_BOOLEAN_NO);
    if (!enabled) {
        // `application` is a positive option, but the legacy `disable apps`
        // setting is negative. Preserve the original compatibility semantics.
        enabled = inicfg_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_APPLICATION, CONFIG_BOOLEAN_YES);
        enabled = (enabled == CONFIG_BOOLEAN_NO) ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;
    }

    ebpf_set_apps_mode(!enabled);

    enabled = inicfg_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, EBPF_CFG_CGROUP, CONFIG_BOOLEAN_NO);
    *disable_cgroups = (enabled == CONFIG_BOOLEAN_NO) ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;

    ebpf_update_map_per_core();

    enabled = inicfg_get_boolean(
        &collector_config,
        EBPF_PROGRAMS_SECTION,
        ebpf_modules[EBPF_MODULE_PROCESS_IDX].info.config_name,
        CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_PROCESS_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "sync", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SYNC_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "dcstat", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DCSTAT_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "swap", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SWAP_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "vfs", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_VFS_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "filesystem", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_FILESYSTEM_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "disk", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_DISK_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "mount", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_MOUNT_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "fd", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_FD_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "hardirq", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_HARDIRQ_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "softirq", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOFTIRQ_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "oomkill", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_OOMKILL_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "shm", CONFIG_BOOLEAN_YES);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SHM_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "mdflush", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_MDFLUSH_IDX, *disable_cgroups);
    }
}
/*****************************************************************
 *
 *  FUNCTIONS TO DEFINE OPTIONS
 *
 *****************************************************************/

/**
 * Define labels used to generate charts
 *
 * @param is   structure with information about number of calls made for a function.
 * @param pio  structure used to generate charts.
 * @param dim  a pointer for the dimensions name
 * @param name a pointer for the tensor with the name of the functions.
 * @param algorithm a vector with the algorithms used to make the charts
 * @param end  the number of elements in the previous 4 arguments.
 */
void ebpf_global_labels(
    netdata_syscall_stat_t *is,
    netdata_publish_syscall_t *pio,
    char **dim,
    char **name,
    int *algorithm,
    int end)
{
    int i;

    netdata_syscall_stat_t *prev = NULL;
    netdata_publish_syscall_t *publish_prev = NULL;
    for (i = 0; i < end; i++) {
        if (prev) {
            prev->next = &is[i];
        }
        prev = &is[i];

        pio[i].dimension = dim[i];
        pio[i].name = name[i];
        pio[i].algorithm = ebpf_algorithms[algorithm[i]];
        if (publish_prev) {
            publish_prev->next = &pio[i];
        }
        publish_prev = &pio[i];
    }
}

/**
 * Disable all Global charts
 *
 * Disable charts
 */
void disable_all_global_charts()
{
    int i;
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
        ebpf_module_enabled_set(&ebpf_modules[i], NETDATA_THREAD_EBPF_NOT_RUNNING);
        ebpf_modules[i].global_charts = 0;
    }
}

/**
 * Disable Cgroups
 *
 * Disable charts for apps loading only global charts.
 */
void ebpf_disable_cgroups()
{
    int i;
    for (i = 0; ebpf_modules[i].info.thread_name; i++) {
        ebpf_modules[i].cgroup_charts = 0;
    }
}

/**
 * Update Disabled Plugins
 *
 * This function calls ebpf_update_stats to update statistics for collector.
 *
 * @param em  a pointer to `struct ebpf_module`
 */
void ebpf_update_disabled_plugin_stats(ebpf_module_t *em)
{
    netdata_mutex_lock(&lock);
    ebpf_update_stats(&plugin_statistics, em);
    netdata_mutex_unlock(&lock);
}

/**
 * Print help on standard error for user knows how to use the collector.
 */
void ebpf_print_help()
{
    fprintf(
        stderr,
        "\n"
        " Netdata ebpf.plugin %s\n"
        " Copyright 2018-2025 Netdata Inc.\n"
        " Released under GNU General Public License v3 or later.\n"
        "\n"
        " This eBPF.plugin is a data collector plugin for netdata.\n"
        "\n"
        " This plugin only accepts long options with one or two dashes. The available command line options are:\n"
        "\n"
        " SECONDS               Set the data collection frequency.\n"
        "\n"
        " [-]-help              Show this help.\n"
        "\n"
        " [-]-version           Show software version.\n"
        "\n"
        " [-]-global            Disable charts per application and cgroup.\n"
        "\n"
        " [-]-all               Enable all chart groups (global, apps, and cgroup), unless -g is also given.\n"
        "\n"
        " [-]-dcstat            Enable charts related to directory cache.\n"
        "\n"
        " [-]-disk              Enable charts related to disk monitoring.\n"
        "\n"
        " [-]-filesystem        Enable chart related to filesystem run time.\n"
        "\n"
        " [-]-hardirq           Enable chart related to hard IRQ latency.\n"
        "\n"
        " [-]-mdflush           Enable charts related to multi-device flush.\n"
        "\n"
        " [-]-mount             Enable charts related to mount monitoring.\n"
        "\n"
        " [-]-oomkill           Enable chart related to OOM kill tracking.\n"
        "\n"
        " [-]-process           Enable charts related to process run time.\n"
        "\n"
        " [-]-return            Run the collector in return mode.\n"
        "\n"
        " [-]-shm               Enable chart related to shared memory tracking.\n"
        "\n"
        " [-]-softirq           Enable chart related to soft IRQ latency.\n"
        "\n"
        " [-]-sync              Enable chart related to sync run time.\n"
        "\n"
        " [-]-swap              Enable chart related to swap run time.\n"
        "\n"
        " [-]-vfs               Enable chart related to vfs run time.\n"
        "\n"
        " [-]-legacy            Load legacy eBPF programs.\n"
        "\n"
        " [-]-core              Use CO-RE when available(Working in progress).\n"
        "\n",
        NETDATA_VERSION);
}

/*****************************************************************
 *
 *  TRACEPOINT MANAGEMENT FUNCTIONS
 *
 *****************************************************************/

/**
 * Enable a tracepoint.
 *
 * @return 0 on success, -1 on error.
 */
int ebpf_enable_tracepoint(ebpf_tracepoint_t *tp)
{
    int test = ebpf_is_tracepoint_enabled(tp->class, tp->event);

    // err?
    if (test == -1) {
        return -1;
    }
    // disabled?
    else if (test == 0) {
        // enable it then.
        if (ebpf_enable_tracing_values(tp->class, tp->event)) {
            return -1;
        }
    }

    // enabled now or already was.
    tp->enabled = true;

    return 0;
}

/**
 * Disable a tracepoint if it's enabled.
 *
 * @return 0 on success, -1 on error.
 */
int ebpf_disable_tracepoint(ebpf_tracepoint_t *tp)
{
    int test = ebpf_is_tracepoint_enabled(tp->class, tp->event);

    // err?
    if (test == -1) {
        return -1;
    }
    // enabled?
    else if (test == 1) {
        // disable it then.
        if (ebpf_disable_tracing_values(tp->class, tp->event)) {
            return -1;
        }
    }

    // disable now or already was.
    tp->enabled = false;

    return 0;
}

/**
 * Enable multiple tracepoints on a list of tracepoints which end when the
 * class is NULL.
 *
 * @return the number of successful enables.
 */
uint32_t ebpf_enable_tracepoints(ebpf_tracepoint_t *tps)
{
    uint32_t cnt = 0;
    for (int i = 0; tps[i].class != NULL; i++) {
        if (ebpf_enable_tracepoint(&tps[i]) == -1) {
            netdata_log_error("Failed to enable tracepoint %s:%s", tps[i].class, tps[i].event);
        } else {
            cnt++;
        }
    }
    return cnt;
}
/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

/**
 * Create apps for module
 *
 * @param em     the module main structure.
 * @param root   a pointer for the targets.
 */
void ebpf_create_apps_for_module(ebpf_module_t *em, ebpf_target_t *root)
{
    if (ebpf_module_enabled_get(em) < NETDATA_THREAD_EBPF_STOPPING && em->apps_charts && em->functions.apps_routine)
        em->functions.apps_routine(em, root);
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param root a pointer for the targets.
 */
void ebpf_create_apps_charts(ebpf_target_t *root)
{
    struct ebpf_target *w;
    int newly_added = 0;

    for (w = root; w; w = w->next) {
        if (w->target)
            continue;

        if (unlikely(w->processes && (debug_enabled || w->debug_enabled))) {
            struct ebpf_pid_on_target *pid_on_target;

            fprintf(
                stderr,
                "ebpf.plugin: target '%s' has aggregated %u process%s:",
                w->name,
                w->processes,
                (w->processes == 1) ? "" : "es");

            for (pid_on_target = w->root_pid; pid_on_target; pid_on_target = pid_on_target->next) {
                fprintf(stderr, " %d", pid_on_target->pid);
            }

            fputc('\n', stderr);
        }

        if (!w->exposed && w->processes) {
            newly_added++;
            w->exposed = 1;
            if (debug_enabled || w->debug_enabled)
                debug_log_int("%s just added - regenerating charts.", w->name);
        }
    }

    if (newly_added) {
        int i;
        for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
            if (!(collect_pids & (1 << i)))
                continue;

            ebpf_module_t *current = &ebpf_modules[i];
            ebpf_create_apps_for_module(current, root);
        }
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO READ GLOBAL HASH TABLES
 *
 *****************************************************************/

/**
 * Read Global Table Stats
 *
 * Read data from a BPF map (map_fd) into stats[], accumulating per-core
 * values when maps_per_core is set.
 *
 * @param stats         output vector
 * @param values        per-core read buffer
 * @param map_fd        BPF map file descriptor
 * @param maps_per_core non-zero when per-CPU map must be aggregated
 * @param begin         first key to read
 * @param end           one-past-last key (exclusive)
 */
void ebpf_read_global_table_stats(
    netdata_idx_t *stats,
    netdata_idx_t *values,
    int map_fd,
    int maps_per_core,
    uint32_t begin,
    uint32_t end)
{
    uint32_t idx;
    int before = (maps_per_core) ? ebpf_nprocs : 1;

    for (idx = begin; idx < end; idx++) {
        /* Zero the slot first: a failed lookup must publish 0, not stale or
         * uninitialized data (callers may pass stack-allocated arrays). */
        stats[idx - begin] = 0;
        if (!bpf_map_lookup_elem(map_fd, &idx, values)) {
            netdata_idx_t total = 0;
            int i;
            for (i = 0; i < before; i++)
                total += values[i];

            stats[idx - begin] = total;
        }
    }
}
