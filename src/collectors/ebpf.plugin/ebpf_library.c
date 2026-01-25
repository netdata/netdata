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

/*****************************************************************
 *
 *  IP PARSING FUNCTIONS
 *
 *****************************************************************/

/**
 * Netmask
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param prefix create the netmask based in the CIDR value.
 *
 * @return
 */
static inline in_addr_t ebpf_netmask(int prefix)
{
    if (prefix == 0)
        return (~((in_addr_t)-1));
    else
        return (in_addr_t)(~((1 << (32 - prefix)) - 1));
}

/**
 * Broadcast
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param addr is the ip address
 * @param prefix is the CIDR value.
 *
 * @return It returns the last address of the range
 */
static inline in_addr_t ebpf_broadcast(in_addr_t addr, int prefix)
{
    return (addr | ~ebpf_netmask(prefix));
}

/**
 * Network
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param addr is the ip address
 * @param prefix is the CIDR value.
 *
 * @return It returns the first address of the range.
 */
static inline in_addr_t ebpf_ipv4_network(in_addr_t addr, int prefix)
{
    return (addr & ebpf_netmask(prefix));
}

/**
 * Calculate ipv6 first address
 *
 * @param out the address to store the first address.
 * @param in the address used to do the math.
 * @param prefix number of bits used to calculate the address
 */
static void get_ipv6_first_addr(union netdata_ip_t *out, union netdata_ip_t *in, uint64_t prefix)
{
    uint64_t mask, tmp;
    uint64_t ret[2];

    memcpy(ret, in->addr32, sizeof(union netdata_ip_t));

    if (prefix == 128) {
        memcpy(out->addr32, in->addr32, sizeof(union netdata_ip_t));
        return;
    } else if (!prefix) {
        ret[0] = ret[1] = 0;
        memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
        return;
    } else if (prefix <= 64) {
        ret[1] = 0ULL;

        tmp = be64toh(ret[0]);
        mask = 0xFFFFFFFFFFFFFFFFULL << (64 - prefix);
        tmp &= mask;
        ret[0] = htobe64(tmp);
    } else {
        mask = 0xFFFFFFFFFFFFFFFFULL << (128 - prefix);
        tmp = be64toh(ret[1]);
        tmp &= mask;
        ret[1] = htobe64(tmp);
    }

    memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
}

/**
 * Get IPV6 Last Address
 *
 * @param out the address to store the last address.
 * @param in the address used to do the math.
 * @param prefix number of bits used to calculate the address
 */
static void get_ipv6_last_addr(union netdata_ip_t *out, union netdata_ip_t *in, uint64_t prefix)
{
    uint64_t mask, tmp;
    uint64_t ret[2];
    memcpy(ret, in->addr32, sizeof(union netdata_ip_t));

    if (prefix == 128) {
        memcpy(out->addr32, in->addr32, sizeof(union netdata_ip_t));
        return;
    } else if (!prefix) {
        ret[0] = ret[1] = 0xFFFFFFFFFFFFFFFF;
        memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
        return;
    } else if (prefix <= 64) {
        ret[1] = 0xFFFFFFFFFFFFFFFFULL;

        tmp = be64toh(ret[0]);
        mask = 0xFFFFFFFFFFFFFFFFULL << (64 - prefix);
        tmp |= ~mask;
        ret[0] = htobe64(tmp);
    } else {
        mask = 0xFFFFFFFFFFFFFFFFULL << (128 - prefix);
        tmp = be64toh(ret[1]);
        tmp |= ~mask;
        ret[1] = htobe64(tmp);
    }

    memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
}

/**
 * IP to network long
 *
 * @param dst the vector to store the result
 * @param ip the source ip given by our users.
 * @param domain the ip domain (IPV4 or IPV6)
 * @param source the original string
 *
 * @return it returns 0 on success and -1 otherwise.
 */
static inline int ebpf_ip2nl(uint8_t *dst, const char *ip, int domain, char *source)
{
    if (inet_pton(domain, ip, dst) <= 0) {
        netdata_log_error("The address specified (%s) is invalid ", source);
        return -1;
    }

    return 0;
}

/**
 * Clean IP structure
 *
 * Clean the allocated list.
 *
 * @param clean the list that will be cleaned
 */
void ebpf_clean_ip_structure(ebpf_network_viewer_ip_list_t **clean)
{
    ebpf_network_viewer_ip_list_t *move = *clean;
    while (move) {
        ebpf_network_viewer_ip_list_t *next = move->next;
        freez(move->value);
        freez(move);

        move = next;
    }
    *clean = NULL;
}

/**
 * Clean port structure
 *
 * Clean the allocated list.
 *
 * @param clean the list that will be cleaned
 */
void ebpf_clean_port_structure(ebpf_network_viewer_port_list_t **clean)
{
    ebpf_network_viewer_port_list_t *move = *clean;
    while (move) {
        ebpf_network_viewer_port_list_t *next = move->next;
        freez(move->value);
        freez(move);

        move = next;
    }
    *clean = NULL;
}

/**
 * Parse IP List
 *
 * Parse IP list and link it.
 *
 * @param out a pointer to store the link list
 * @param ip the value given as parameter
 */
static void ebpf_parse_ip_list_unsafe(void **out, const char *ip)
{
    ebpf_network_viewer_ip_list_t **list = (ebpf_network_viewer_ip_list_t **)out;

    char *ipdup = strdupz(ip);
    union netdata_ip_t first = {};
    union netdata_ip_t last = {};
    const char *is_ipv6;
    if (*ip == '*' && *(ip + 1) == '\0') {
        memset(first.addr8, 0, sizeof(first.addr8));
        memset(last.addr8, 0xFF, sizeof(last.addr8));

        is_ipv6 = ip;

        ebpf_clean_ip_structure(list);
        goto storethisip;
    }

    char *enddup = strdupz(ip);
    char *end = enddup;
    // Move while I cannot find a separator
    while (*end && *end != '/' && *end != '-')
        end++;

    // We will use only the classic IPV6 for while, but we could consider the base 85 in a near future
    // https://tools.ietf.org/html/rfc1924
    is_ipv6 = strchr(ip, ':');

    int select;
    if (*end && !is_ipv6) { // IPV4 range
        select = (*end == '/') ? 0 : 1;
        *end++ = '\0';
        if (*end == '!') {
            netdata_log_info("The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
            goto cleanipdup;
        }

        if (!select) { // CIDR
            select = ebpf_ip2nl(first.addr8, ip, AF_INET, ipdup);
            if (select)
                goto cleanipdup;

            select = (int)str2i(end);
            if (select < NETDATA_MINIMUM_IPV4_CIDR || select > NETDATA_MAXIMUM_IPV4_CIDR) {
                netdata_log_info("The specified CIDR %s is not valid, the IP %s will be ignored.", end, ip);
                goto cleanipdup;
            }

            last.addr32[0] = htonl(ebpf_broadcast(ntohl(first.addr32[0]), select));
            // This was added to remove
            // https://app.codacy.com/manual/netdata/netdata/pullRequest?prid=5810941&bid=19021977
            UNUSED(last.addr32[0]);

            uint32_t ipv4_test = htonl(ebpf_ipv4_network(ntohl(first.addr32[0]), select));
            if (first.addr32[0] != ipv4_test) {
                first.addr32[0] = ipv4_test;
                struct in_addr ipv4_convert;
                ipv4_convert.s_addr = ipv4_test;
                char ipv4_msg[INET_ADDRSTRLEN];
                if (inet_ntop(AF_INET, &ipv4_convert, ipv4_msg, INET_ADDRSTRLEN))
                    netdata_log_info("The network value of CIDR %s was updated for %s .", ipdup, ipv4_msg);
            }
        } else { // Range
            select = ebpf_ip2nl(first.addr8, ip, AF_INET, ipdup);
            if (select)
                goto cleanipdup;

            select = ebpf_ip2nl(last.addr8, end, AF_INET, ipdup);
            if (select)
                goto cleanipdup;
        }

        if (htonl(first.addr32[0]) > htonl(last.addr32[0])) {
            netdata_log_info(
                "The specified range %s is invalid, the second address is smallest than the first, it will be ignored.",
                ipdup);
            goto cleanipdup;
        }
    } else if (is_ipv6) { // IPV6
        if (!*end) {      // Unique
            select = ebpf_ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            memcpy(last.addr8, first.addr8, sizeof(first.addr8));
        } else if (*end == '-') {
            *end++ = 0x00;
            if (*end == '!') {
                netdata_log_info(
                    "The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
                goto cleanipdup;
            }

            select = ebpf_ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            select = ebpf_ip2nl(last.addr8, end, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;
        } else { // CIDR
            *end++ = 0x00;
            if (*end == '!') {
                netdata_log_info(
                    "The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
                goto cleanipdup;
            }

            select = str2i(end);
            if (select < 0 || select > 128) {
                netdata_log_info("The CIDR %s is not valid, the address %s will be ignored.", end, ip);
                goto cleanipdup;
            }

            uint64_t prefix = (uint64_t)select;
            select = ebpf_ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            get_ipv6_last_addr(&last, &first, prefix);

            union netdata_ip_t ipv6_test;
            get_ipv6_first_addr(&ipv6_test, &first, prefix);

            if (memcmp(first.addr8, ipv6_test.addr8, sizeof(union netdata_ip_t)) != 0) {
                memcpy(first.addr8, ipv6_test.addr8, sizeof(union netdata_ip_t));

                struct in6_addr ipv6_convert;
                memcpy(ipv6_convert.s6_addr, ipv6_test.addr8, sizeof(union netdata_ip_t));

                char ipv6_msg[INET6_ADDRSTRLEN];
                if (inet_ntop(AF_INET6, &ipv6_convert, ipv6_msg, INET6_ADDRSTRLEN))
                    netdata_log_info("The network value of CIDR %s was updated for %s .", ipdup, ipv6_msg);
            }
        }

        if ((be64toh(*(uint64_t *)&first.addr64[1]) > be64toh(*(uint64_t *)&last.addr64[1]) &&
             !memcmp(first.addr64, last.addr64, sizeof(uint64_t))) ||
            (be64toh(*(uint64_t *)&first.addr64) > be64toh(*(uint64_t *)&last.addr64))) {
            netdata_log_info(
                "The specified range %s is invalid, the second address is smallest than the first, it will be ignored.",
                ipdup);
            goto cleanipdup;
        }
    } else { // Unique ip
        select = ebpf_ip2nl(first.addr8, ip, AF_INET, ipdup);
        if (select)
            goto cleanipdup;

        memcpy(last.addr8, first.addr8, sizeof(first.addr8));
    }

    ebpf_network_viewer_ip_list_t *store;

storethisip:
    store = callocz(1, sizeof(ebpf_network_viewer_ip_list_t));
    store->value = ipdup;
    store->hash = simple_hash(ipdup);
    store->ver = (uint8_t)(!is_ipv6) ? AF_INET : AF_INET6;
    memcpy(store->first.addr8, first.addr8, sizeof(first.addr8));
    memcpy(store->last.addr8, last.addr8, sizeof(last.addr8));

    ebpf_fill_ip_list_unsafe(list, store, "socket");
    return;

cleanipdup:
    freez(ipdup);
    freez(enddup);
}

/**
 * Check if the ip is inside a IP range
 *
 * @param rfirst    the first ip address of the range
 * @param rlast     the last ip address of the range
 * @param cmpfirst  the first ip to compare
 * @param cmplast   the last ip to compare
 * @param family    the IP family
 *
 * @return It returns 1 if the IP is inside the range and 0 otherwise
 */
static int ebpf_is_ip_inside_range(
    union netdata_ip_t *rfirst,
    union netdata_ip_t *rlast,
    union netdata_ip_t *cmpfirst,
    union netdata_ip_t *cmplast,
    int family)
{
    if (family == AF_INET) {
        if ((rfirst->addr32[0] <= cmpfirst->addr32[0]) && (rlast->addr32[0] >= cmplast->addr32[0]))
            return 1;
    } else {
        if (memcmp(rfirst->addr8, cmpfirst->addr8, sizeof(union netdata_ip_t)) <= 0 &&
            memcmp(rlast->addr8, cmplast->addr8, sizeof(union netdata_ip_t)) >= 0) {
            return 1;
        }
    }
    return 0;
}

/**
 * Fill IP list
 *
 * @param out a pointer to link list.
 * @param in the structure that will be linked.
 * @param table the modified table.
 */
void ebpf_fill_ip_list_unsafe(
    ebpf_network_viewer_ip_list_t **out,
    ebpf_network_viewer_ip_list_t *in,
    char *table __maybe_unused)
{
    if (in->ver == AF_INET) { // It is simpler to compare using host order
        in->first.addr32[0] = ntohl(in->first.addr32[0]);
        in->last.addr32[0] = ntohl(in->last.addr32[0]);
    }
    if (likely(*out)) {
        ebpf_network_viewer_ip_list_t *move = *out, *store = *out;
        while (move) {
            if (in->ver == move->ver &&
                ebpf_is_ip_inside_range(&move->first, &move->last, &in->first, &in->last, in->ver)) {
#ifdef NETDATA_DEV_MODE
                netdata_log_info(
                    "The range/value (%s) is inside the range/value (%s) already inserted, it will be ignored.",
                    in->value,
                    move->value);
#endif
                freez(in->value);
                freez(in);
                return;
            }
            store = move;
            move = move->next;
        }

        store->next = in;
    } else {
        *out = in;
    }

#ifdef NETDATA_DEV_MODE
    char first[256], last[512];
    if (in->ver == AF_INET) {
        netdata_log_info(
            "Adding values %s: (%u - %u) to %s IP list \"%s\" used on network viewer",
            in->value,
            in->first.addr32[0],
            in->last.addr32[0],
            (*out == network_viewer_opt.included_ips) ? "included" : "excluded",
            table);
    } else {
        if (inet_ntop(AF_INET6, in->first.addr8, first, INET6_ADDRSTRLEN) &&
            inet_ntop(AF_INET6, in->last.addr8, last, INET6_ADDRSTRLEN))
            netdata_log_info(
                "Adding values %s - %s to %s IP list \"%s\" used on network viewer",
                first,
                last,
                (*out == network_viewer_opt.included_ips) ? "included" : "excluded",
                table);
    }
#endif
}

/**
 * Parse IP Range
 *
 * Parse the IP ranges given and create Network Viewer IP Structure
 *
 * @param ptr  is a pointer with the text to parse.
 */
void ebpf_parse_ips_unsafe(const char *ptr)
{
    // No value
    if (unlikely(!ptr))
        return;

    while (likely(ptr)) {
        // Move forward until next valid character
        while (isspace(*ptr))
            ptr++;

        // No valid value found
        if (unlikely(!*ptr))
            return;

        // Find space that ends the list
        char *end = strchr(ptr, ' ');
        if (end) {
            *end++ = '\0';
        }

        int neg = 0;
        if (*ptr == '!') {
            neg++;
            ptr++;
        }

        if (isascii(*ptr)) { // Parse port
            ebpf_parse_ip_list_unsafe(
                (!neg) ? (void **)&network_viewer_opt.included_ips : (void **)&network_viewer_opt.excluded_ips, ptr);
        }

        ptr = end;
    }
}
