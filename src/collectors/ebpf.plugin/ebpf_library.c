// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>
#include <stdlib.h>

#include "libnetdata/libnetdata.h"
#include "ebpf_library.h"
#include "ebpf.h"
#include "libbpf_api/ebpf.h"
#include "ebpf_process.h"
#include "ebpf_socket.h"
#include <ifaddrs.h>

extern char *ebpf_algorithms[];
extern uint32_t integration_with_collectors;
extern int running_on_kernel;
extern int isrh;
extern ebpf_network_viewer_options_t network_viewer_opt;
extern const char *btf_path;
extern ebpf_module_t ebpf_modules[];

void ebpf_parse_ports(const char *ptr);
void ebpf_parse_ips_unsafe(const char *ptr);
void ebpf_read_local_addresses_unsafe();

void read_collector_values(int *disable_cgroups, int update_every, netdata_ebpf_load_mode_t origin);
void ebpf_parse_service_name_section(struct config *cfg);

/*****************************************************************
 *
 *  DIMENSION WRITING FUNCTIONS
 *
 *****************************************************************/

void write_chart_dimension(char *dim, long long value)
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

    uint32_t i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->ncall);

        move = move->next;
        i++;
    }

    ebpf_write_end_chart();
}

void write_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end)
{
    ebpf_write_begin_chart(family, name, "");

    int i = 0;
    while (move && i < end) {
        write_chart_dimension(move->name, move->nerr);

        move = move->next;
        i++;
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
    char *id,
    char *suffix,
    char *title,
    char *units,
    char *family,
    char *charttype,
    char *context,
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
    static int priority = NETATA_EBPF_ORDER_STAT_ARAL_BEGIN;
    char *mem = {NETDATA_EBPF_STAT_DIMENSION_MEMORY};
    char *aral = {NETDATA_EBPF_STAT_DIMENSION_ARAL};

    snprintfz(em->memory_usage, NETDATA_EBPF_CHART_MEM_LENGTH - 1, "aral_%s_size", name);
    snprintfz(em->memory_allocations, NETDATA_EBPF_CHART_MEM_LENGTH - 1, "aral_%s_alloc", name);

    ebpf_write_chart_cmd(
        NETDATA_MONITORING_FAMILY,
        em->memory_usage,
        "",
        "Bytes allocated for ARAL.",
        "bytes",
        NETDATA_EBPF_FAMILY,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        "netdata.ebpf_aral_stat_size",
        priority++,
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
        priority++,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_PROCESS);

    ebpf_write_global_dimension(aral, aral, ebpf_algorithms[NETDATA_EBPF_ABSOLUTE_IDX]);

    return priority - 2;
}

void ebpf_statistic_obsolete_aral_chart(ebpf_module_t *em, int prio)
{
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
    char *mem = {NETDATA_EBPF_STAT_DIMENSION_MEMORY};
    char *aral = {NETDATA_EBPF_STAT_DIMENSION_ARAL};

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

void epbf_update_load_mode(const char *str, netdata_ebpf_load_mode_t origin)
{
    netdata_ebpf_load_mode_t load = epbf_convert_string_to_load_mode(str);

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
        return;
    } else if (!strcmp(integration, NETDATA_EBPF_IPC_INTEGRATION_SOCKET)) {
        integration_with_collectors = NETDATA_EBPF_INTEGRATION_SOCKET;
        return;
    }
    integration_with_collectors = NETDATA_EBPF_INTEGRATION_DISABLED;
}

void ebpf_parse_ipc_section()
{
    const char *integration = inicfg_get(
        &collector_config,
        NETDATA_EBPF_IPC_SECTION,
        NETDATA_EBPF_IPC_INTEGRATION,
        NETDATA_EBPF_IPC_INTEGRATION_DISABLED);
    ebpf_set_ipc_value(integration);

    ipc_sockets.default_bind_to = inicfg_get(
        &collector_config, NETDATA_EBPF_IPC_SECTION, NETDATA_EBPF_IPC_BIND_TO, NETDATA_EBPF_IPC_BIND_TO_DEFAULT);

    ipc_sockets.backlog =
        (int)inicfg_get_number(&collector_config, NETDATA_EBPF_IPC_SECTION, NETDATA_EBPF_IPC_BACKLOG, 20);
}

void ebpf_set_thread_mode(netdata_run_mode_t lmode)
{
    int i;
    for (i = 0; i < EBPF_MODULE_FUNCTION_IDX; i++) {
        ebpf_modules[i].mode = lmode;
    }
}

void ebpf_enable_specific_chart(struct ebpf_module *em, int disable_cgroup)
{
    em->enabled = NETDATA_THREAD_EBPF_RUNNING;

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

    epbf_update_load_mode(value, origin);

    ebpf_update_interval(update_every);

    ebpf_update_table_size();

    ebpf_update_lifetime();

    uint32_t enabled = inicfg_get_boolean(&collector_config, EBPF_GLOBAL_SECTION, "disable apps", CONFIG_BOOLEAN_NO);
    if (!enabled) {
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

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network viewer", CONFIG_BOOLEAN_NO);
    if (!enabled)
        enabled = inicfg_get_boolean(
            &collector_config,
            EBPF_PROGRAMS_SECTION,
            ebpf_modules[EBPF_MODULE_SOCKET_IDX].info.config_name,
            CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_cgroups);
    }

    enabled = inicfg_get_boolean(
        &collector_config, EBPF_PROGRAMS_SECTION, "network connection monitoring", CONFIG_BOOLEAN_YES);
    if (!enabled)
        enabled =
            inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "network connections", CONFIG_BOOLEAN_YES);

    network_viewer_opt.enabled = enabled;
    if (enabled) {
        if (!ebpf_modules[EBPF_MODULE_SOCKET_IDX].enabled)
            ebpf_enable_chart(EBPF_MODULE_SOCKET_IDX, *disable_cgroups);

        parse_network_viewer_section(&collector_config);
        ebpf_parse_service_name_section(&collector_config);
    }

    enabled = inicfg_get_boolean(&collector_config, EBPF_PROGRAMS_SECTION, "cachestat", CONFIG_BOOLEAN_NO);
    if (enabled) {
        ebpf_enable_chart(EBPF_MODULE_CACHESTAT_IDX, *disable_cgroups);
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

/**
 * Link hostname
 *
 * @param out is the output link list
 * @param in the hostname to add to list.
 */
static void ebpf_link_hostname(ebpf_network_viewer_hostname_list_t **out, ebpf_network_viewer_hostname_list_t *in)
{
    if (likely(*out)) {
        ebpf_network_viewer_hostname_list_t *move = *out;
        for (; move->next; move = move->next) {
            if (move->hash == in->hash && !strcmp(move->value, in->value)) {
                netdata_log_info("The hostname %s was already inserted, it will be ignored.", in->value);
                freez(in->value);
                simple_pattern_free(in->value_pattern);
                freez(in);
                return;
            }
        }

        move->next = in;
    } else {
        *out = in;
    }
#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info(
        "Adding value %s to %s hostname list used on network viewer",
        in->value,
        (*out == network_viewer_opt.included_hostnames) ? "included" : "excluded");
#endif
}

/**
 * Link Hostnames
 *
 * Parse the list of hostnames to create the link list.
 * This is not associated with the IP, because simple patterns like *example* cannot be resolved to IP.
 *
 * @param out is the output link list
 * @param parse is a pointer with the text to parser.
 */
static void ebpf_link_hostnames(const char *parse)
{
    // No value
    if (unlikely(!parse))
        return;

    while (likely(parse)) {
        // Find the first valid value
        while (isspace(*parse))
            parse++;

        // No valid value found
        if (unlikely(!*parse))
            return;

        // Find space that ends the list
        char *end = strchr(parse, ' ');
        if (end) {
            *end++ = '\0';
        }

        int neg = 0;
        if (*parse == '!') {
            neg++;
            parse++;
        }

        ebpf_network_viewer_hostname_list_t *hostname = callocz(1, sizeof(ebpf_network_viewer_hostname_list_t));
        hostname->value = strdupz(parse);
        hostname->hash = simple_hash(parse);
        hostname->value_pattern = simple_pattern_create(parse, NULL, SIMPLE_PATTERN_EXACT, true);

        ebpf_link_hostname(
            (!neg) ? &network_viewer_opt.included_hostnames : &network_viewer_opt.excluded_hostnames, hostname);

        parse = end;
    }
}

void parse_network_viewer_section(struct config *cfg)
{
    network_viewer_opt.hostname_resolution_enabled =
        inicfg_get_boolean(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_RESOLVE_HOSTNAME, CONFIG_BOOLEAN_NO);

    network_viewer_opt.service_resolution_enabled =
        inicfg_get_boolean(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_RESOLVE_SERVICE, CONFIG_BOOLEAN_YES);

    const char *value = inicfg_get(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_PORTS, NULL);
    ebpf_parse_ports(value);

    if (network_viewer_opt.hostname_resolution_enabled) {
        value = inicfg_get(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_HOSTNAMES, NULL);
        ebpf_link_hostnames(value);
    } else {
        netdata_log_info("Name resolution is disabled, collector will not parse \"hostnames\" list.");
    }

    value = inicfg_get(cfg, EBPF_NETWORK_VIEWER_SECTION, "ips", NULL);
    ebpf_parse_ips_unsafe(value);
}
