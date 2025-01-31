// SPDX-License-Identifier: GPL-3.0-or-later

// Heavily inspired from proc_net_dev.c
#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_INFINIBAND_NAME "/sys/class/infiniband"
#define CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND                                                                     \
    "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_INFINIBAND_NAME

// ib_device::name[IB_DEVICE_NAME_MAX(64)] + "-" + ib_device::phys_port_cnt[u8 = 3 chars]
#define IBNAME_MAX 68

// ----------------------------------------------------------------------------
// infiniband & omnipath standard counters

// I use macro as there's no single file acting as summary, but a lot of different files, so can't use helpers like
// procfile(). Also, omnipath generates other counters, that are not provided by infiniband
#define FOREACH_COUNTER(GEN, ...)              \
    FOREACH_COUNTER_BYTES(GEN,   __VA_ARGS__)  \
    FOREACH_COUNTER_PACKETS(GEN, __VA_ARGS__)  \
    FOREACH_COUNTER_ERRORS(GEN,  __VA_ARGS__)

#define FOREACH_COUNTER_BYTES(GEN, ...)                                                             \
    GEN(port_rcv_data,                   bytes,   "Received",                      1, __VA_ARGS__)  \
    GEN(port_xmit_data,                  bytes,   "Sent",                         -1, __VA_ARGS__)

#define FOREACH_COUNTER_PACKETS(GEN, ...)                                                           \
    GEN(port_rcv_packets,                packets, "Received",                      1, __VA_ARGS__)  \
    GEN(port_xmit_packets,               packets, "Sent",                         -1, __VA_ARGS__)  \
    GEN(multicast_rcv_packets,           packets, "Mcast rcvd",                    1, __VA_ARGS__)  \
    GEN(multicast_xmit_packets,          packets, "Mcast sent",                   -1, __VA_ARGS__)  \
    GEN(unicast_rcv_packets,             packets, "Ucast rcvd",                    1, __VA_ARGS__)  \
    GEN(unicast_xmit_packets,            packets, "Ucast sent",                   -1, __VA_ARGS__)

#define FOREACH_COUNTER_ERRORS(GEN, ...)                                                            \
    GEN(port_rcv_errors,                 errors,  "Pkts malformated",              1, __VA_ARGS__)  \
    GEN(port_rcv_constraint_errors,      errors,  "Pkts rcvd discarded ",          1, __VA_ARGS__)  \
    GEN(port_xmit_discards,              errors,  "Pkts sent discarded",           1, __VA_ARGS__)  \
    GEN(port_xmit_wait,                  errors,  "Tick Wait to send",             1, __VA_ARGS__)  \
    GEN(VL15_dropped,                    errors,  "Pkts missed resource",         1, __VA_ARGS__)  \
    GEN(excessive_buffer_overrun_errors, errors,  "Buffer overrun",                1, __VA_ARGS__)  \
    GEN(link_downed,                     errors,  "Link Downed",                   1, __VA_ARGS__)  \
    GEN(link_error_recovery,             errors,  "Link recovered",                1, __VA_ARGS__)  \
    GEN(local_link_integrity_errors,     errors,  "Link integrity err",            1, __VA_ARGS__)  \
    GEN(symbol_error,                    errors,  "Link minor errors",             1, __VA_ARGS__)  \
    GEN(port_rcv_remote_physical_errors, errors,  "Pkts rcvd with EBP",            1, __VA_ARGS__)  \
    GEN(port_rcv_switch_relay_errors,    errors,  "Pkts rcvd discarded by switch", 1, __VA_ARGS__)  \
    GEN(port_xmit_constraint_errors,     errors,  "Pkts sent discarded by switch", 1, __VA_ARGS__)

//
// Hardware Counters
//

// IMPORTANT: These are vendor-specific fields.
//  If you want to add a new vendor, search this for for 'VENDORS:' keyword and
//  add your definition as 'VENDOR-<key>:' where <key> if the string part that
//  is shown in /sys/class/infiniband/<key>X_Y
//  EG: for Mellanox, shown as mlx0_1, it's 'mlx'
//      for Intel, shown as hfi1_1, it's 'hfi'

// VENDORS: List of implemented hardware vendors
#define FOREACH_HWCOUNTER_NAME(GEN, ...) GEN(mlx, __VA_ARGS__)

// VENDOR-MLX: HW Counters for Mellanox ConnectX Devices
#define FOREACH_HWCOUNTER_MLX(GEN, ...)               \
    FOREACH_HWCOUNTER_MLX_PACKETS(GEN, __VA_ARGS__)   \
    FOREACH_HWCOUNTER_MLX_ERRORS(GEN,  __VA_ARGS__)

#define FOREACH_HWCOUNTER_MLX_PACKETS(GEN, ...)                                                   \
    GEN(np_cnp_sent,                hwpackets, "RoCEv2 Congestion sent",        1, __VA_ARGS__)   \
    GEN(np_ecn_marked_roce_packets, hwpackets, "RoCEv2 Congestion rcvd",       -1, __VA_ARGS__)   \
    GEN(rp_cnp_handled,             hwpackets, "IB Congestion handled",         1, __VA_ARGS__)   \
    GEN(rx_atomic_requests,         hwpackets, "ATOMIC req. rcvd",              1, __VA_ARGS__)   \
    GEN(rx_dct_connect,             hwpackets, "Connection req. rcvd",          1, __VA_ARGS__)   \
    GEN(rx_read_requests,           hwpackets, "Read req. rcvd",                1, __VA_ARGS__)   \
    GEN(rx_write_requests,          hwpackets, "Write req. rcvd",               1, __VA_ARGS__)   \
    GEN(roce_adp_retrans,           hwpackets, "RoCE retrans adaptive",         1, __VA_ARGS__)   \
    GEN(roce_adp_retrans_to,        hwpackets, "RoCE retrans timeout",          1, __VA_ARGS__)   \
    GEN(roce_slow_restart,          hwpackets, "RoCE slow restart",             1, __VA_ARGS__)   \
    GEN(roce_slow_restart_cnps,     hwpackets, "RoCE slow restart congestion",  1, __VA_ARGS__)   \
    GEN(roce_slow_restart_trans,    hwpackets, "RoCE slow restart count",       1, __VA_ARGS__)

#define FOREACH_HWCOUNTER_MLX_ERRORS(GEN, ...)                                                    \
    GEN(duplicate_request,          hwerrors, "Duplicated packets",            -1, __VA_ARGS__)   \
    GEN(implied_nak_seq_err,        hwerrors, "Pkt Seq Num gap",                1, __VA_ARGS__)   \
    GEN(local_ack_timeout_err,      hwerrors, "Ack timer expired",              1, __VA_ARGS__)   \
    GEN(out_of_buffer,              hwerrors, "Drop missing buffer",            1, __VA_ARGS__)   \
    GEN(out_of_sequence,            hwerrors, "Drop out of sequence",           1, __VA_ARGS__)   \
    GEN(packet_seq_err,             hwerrors, "NAK sequence rcvd",              1, __VA_ARGS__)   \
    GEN(req_cqe_error,              hwerrors, "CQE err Req",                    1, __VA_ARGS__)   \
    GEN(resp_cqe_error,             hwerrors, "CQE err Resp",                   1, __VA_ARGS__)   \
    GEN(req_cqe_flush_error,        hwerrors, "CQE Flushed err Req",            1, __VA_ARGS__)   \
    GEN(resp_cqe_flush_error,       hwerrors, "CQE Flushed err Resp",           1, __VA_ARGS__)   \
    GEN(req_remote_access_errors,   hwerrors, "Remote access err Req",          1, __VA_ARGS__)   \
    GEN(resp_remote_access_errors,  hwerrors, "Remote access err Resp",         1, __VA_ARGS__)   \
    GEN(req_remote_invalid_request, hwerrors, "Remote invalid req",             1, __VA_ARGS__)   \
    GEN(resp_local_length_error,    hwerrors, "Local length err Resp",          1, __VA_ARGS__)   \
    GEN(rnr_nak_retry_err,          hwerrors, "RNR NAK Packets",                1, __VA_ARGS__)   \
    GEN(rp_cnp_ignored,             hwerrors, "CNP Pkts ignored",               1, __VA_ARGS__)   \
    GEN(rx_icrc_encapsulated,       hwerrors, "RoCE ICRC Errors",               1, __VA_ARGS__)

// Common definitions used more than once
#define GEN_RRD_DIM_ADD(NAME, GRP, DESC, DIR, PORT)                                                                    \
    GEN_RRD_DIM_ADD_CUSTOM(NAME, GRP, DESC, DIR, PORT, 1, 1, RRD_ALGORITHM_INCREMENTAL)

#define GEN_RRD_DIM_ADD_CUSTOM(NAME, GRP, DESC, DIR, PORT, MULT, DIV, ALGO)                                            \
    PORT->rd_##NAME = rrddim_add(PORT->st_##GRP, DESC, NULL, DIR * MULT, DIV, ALGO);

#define GEN_RRD_DIM_ADD_HW(NAME, GRP, DESC, DIR, PORT, HW)                                                             \
    HW->rd_##NAME = rrddim_add(PORT->st_##GRP, DESC, NULL, DIR, 1, RRD_ALGORITHM_INCREMENTAL);

#define GEN_RRD_DIM_SETP(NAME, GRP, DESC, DIR, PORT)                                                                   \
    rrddim_set_by_pointer(PORT->st_##GRP, PORT->rd_##NAME, (collected_number)PORT->NAME);

#define GEN_RRD_DIM_SETP_HW(NAME, GRP, DESC, DIR, PORT, HW)                                                            \
    rrddim_set_by_pointer(PORT->st_##GRP, HW->rd_##NAME, (collected_number)HW->NAME);

// https://community.mellanox.com/s/article/understanding-mlx5-linux-counters-and-status-parameters
// https://community.mellanox.com/s/article/infiniband-port-counters
static struct ibport {
    char *name;
    char *counters_path;
    char *hwcounters_path;
    int len;

    // flags
    int configured;
    int enabled;
    int discovered;

    int do_bytes;
    int do_packets;
    int do_errors;
    int do_hwpackets;
    int do_hwerrors;

    const char *chart_type_bytes;
    const char *chart_type_packets;
    const char *chart_type_errors;
    const char *chart_type_hwpackets;
    const char *chart_type_hwerrors;

    const char *chart_id_bytes;
    const char *chart_id_packets;
    const char *chart_id_errors;
    const char *chart_id_hwpackets;
    const char *chart_id_hwerrors;

    const char *chart_family;

    unsigned long priority;

    // Port details using drivers/infiniband/core/sysfs.c :: rate_show()
    RRDDIM *rd_speed;
    uint64_t speed;
    uint64_t width;

// Stats from /$device/ports/$portid/counters
// as drivers/infiniband/hw/qib/qib_verbs.h
// All uint64 except vl15_dropped, local_link_integrity_errors, excessive_buffer_overrun_errors uint32
// Will generate 2 elements for each counter:
// - uint64_t to store the value
// - char*    to store the filename path
// - RRDDIM*  to store the RRD Dimension
#define GEN_DEF_COUNTER(NAME, ...)                                                                                     \
    uint64_t NAME;                                                                                                     \
    char *file_##NAME;                                                                                                 \
    RRDDIM *rd_##NAME;
    FOREACH_COUNTER(GEN_DEF_COUNTER)

// Vendor specific hwcounters from /$device/ports/$portid/hw_counters
// We will generate one struct pointer per vendor to avoid future casting
#define GEN_DEF_HWCOUNTER_PTR(VENDOR, ...) struct ibporthw_##VENDOR *hwcounters_##VENDOR;
    FOREACH_HWCOUNTER_NAME(GEN_DEF_HWCOUNTER_PTR)

    // Function pointer to the "infiniband_hwcounters_parse_<vendor>" function
    void (*hwcounters_parse)(struct ibport *);
    void (*hwcounters_dorrd)(struct ibport *);

    // charts and dim
    RRDSET *st_bytes;
    RRDSET *st_packets;
    RRDSET *st_errors;
    RRDSET *st_hwpackets;
    RRDSET *st_hwerrors;

    const RRDVAR_ACQUIRED *stv_speed;

    usec_t speed_last_collected_usec;

    struct ibport *next;
} *ibport_root = NULL, *ibport_last_used = NULL;

// VENDORS: reading / calculation functions
#define GEN_DEF_HWCOUNTER(NAME, ...)                                                                                   \
    uint64_t NAME;                                                                                                     \
    char *file_##NAME;                                                                                                 \
    RRDDIM *rd_##NAME;

#define GEN_DO_HWCOUNTER_READ(NAME, GRP, DESC, DIR, PORT, HW, ...)                                                     \
    if (HW->file_##NAME) {                                                                                             \
        if (read_single_number_file(HW->file_##NAME, (unsigned long long *)&HW->NAME)) {                               \
            collector_error("cannot read iface '%s' hwcounter '" #HW "'", PORT->name);                                           \
            HW->file_##NAME = NULL;                                                                                    \
        }                                                                                                              \
    }

// VENDOR-MLX: Mellanox
struct ibporthw_mlx {
    FOREACH_HWCOUNTER_MLX(GEN_DEF_HWCOUNTER)
};
void infiniband_hwcounters_parse_mlx(struct ibport *port)
{
    if (port->do_hwerrors != CONFIG_BOOLEAN_NO)
        FOREACH_HWCOUNTER_MLX_ERRORS(GEN_DO_HWCOUNTER_READ,  port, port->hwcounters_mlx)
    if (port->do_hwpackets != CONFIG_BOOLEAN_NO)
        FOREACH_HWCOUNTER_MLX_PACKETS(GEN_DO_HWCOUNTER_READ, port, port->hwcounters_mlx)
}
void infiniband_hwcounters_dorrd_mlx(struct ibport *port)
{
    if (port->do_hwerrors != CONFIG_BOOLEAN_NO) {
        FOREACH_HWCOUNTER_MLX_ERRORS(GEN_RRD_DIM_SETP_HW,    port, port->hwcounters_mlx)
        rrdset_done(port->st_hwerrors);
    }
    if (port->do_hwpackets != CONFIG_BOOLEAN_NO) {
        FOREACH_HWCOUNTER_MLX_PACKETS(GEN_RRD_DIM_SETP_HW,   port, port->hwcounters_mlx)
        rrdset_done(port->st_hwpackets);
    }
}

// ----------------------------------------------------------------------------

static struct ibport *get_ibport(const char *dev, const char *port)
{
    struct ibport *p;

    char name[IBNAME_MAX + 1];
    snprintfz(name, IBNAME_MAX, "%s-%s", dev, port);

    // search it, resuming from the last position in sequence
    for (p = ibport_last_used; p; p = p->next) {
        if (unlikely(!strcmp(name, p->name))) {
            ibport_last_used = p->next;
            return p;
        }
    }

    // new round, from the beginning to the last position used this time
    for (p = ibport_root; p != ibport_last_used; p = p->next) {
        if (unlikely(!strcmp(name, p->name))) {
            ibport_last_used = p->next;
            return p;
        }
    }

    // create a new one
    p = callocz(1, sizeof(struct ibport));
    p->name = strdupz(name);
    p->len  = strlen(p->name);

    p->chart_type_bytes     = strdupz("infiniband_cnt_bytes");
    p->chart_type_packets   = strdupz("infiniband_cnt_packets");
    p->chart_type_errors    = strdupz("infiniband_cnt_errors");
    p->chart_type_hwpackets = strdupz("infiniband_hwc_packets");
    p->chart_type_hwerrors  = strdupz("infiniband_hwc_errors");

    char buffer[RRD_ID_LENGTH_MAX + 1];
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "ib_cntbytes_%s",     p->name);
    p->chart_id_bytes = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "ib_cntpackets_%s",   p->name);
    p->chart_id_packets = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "ib_cnterrors_%s",    p->name);
    p->chart_id_errors = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "ib_hwcntpackets_%s", p->name);
    p->chart_id_hwpackets = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "ib_hwcnterrors_%s",  p->name);
    p->chart_id_hwerrors = strdupz(buffer);

    p->chart_family = strdupz(p->name);
    p->priority = NETDATA_CHART_PRIO_INFINIBAND;

    // Link current ibport to last one in the list
    if (ibport_root) {
        struct ibport *t;
        for (t = ibport_root; t->next; t = t->next)
            ;
        t->next = p;
    } else
        ibport_root = p;

    return p;
}

int do_sys_class_infiniband(int update_every, usec_t dt)
{
    (void)dt;
    static SIMPLE_PATTERN *disabled_list = NULL;
    static int initialized = 0;
    static int enable_new_ports = -1, enable_only_active = CONFIG_BOOLEAN_YES;
    static int do_bytes = -1, do_packets = -1, do_errors = -1, do_hwpackets = -1, do_hwerrors = -1;
    static const char *sys_class_infiniband_dirname = NULL;

    static long long int dt_to_refresh_ports = 0, last_refresh_ports_usec = 0;

    if (unlikely(enable_new_ports == -1)) {
        char dirname[FILENAME_MAX + 1];

        snprintfz(dirname, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/infiniband");
        sys_class_infiniband_dirname =
            inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "dirname to monitor", dirname);

        do_bytes     = inicfg_get_boolean_ondemand(
            &netdata_config, CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "bandwidth counters",        CONFIG_BOOLEAN_YES);
        do_packets   = inicfg_get_boolean_ondemand(
            &netdata_config, CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "packets counters",          CONFIG_BOOLEAN_YES);
        do_errors    = inicfg_get_boolean_ondemand(
            &netdata_config, CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "errors counters",           CONFIG_BOOLEAN_YES);
        do_hwpackets = inicfg_get_boolean_ondemand(
            &netdata_config, CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "hardware packets counters", CONFIG_BOOLEAN_AUTO);
        do_hwerrors  = inicfg_get_boolean_ondemand(
            &netdata_config, CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "hardware errors counters",  CONFIG_BOOLEAN_AUTO);

        enable_only_active = inicfg_get_boolean_ondemand(
            &netdata_config, CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "monitor only active ports", CONFIG_BOOLEAN_AUTO);
        disabled_list = simple_pattern_create(
                inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "disable by default interfaces matching", ""),
                NULL,
                SIMPLE_PATTERN_EXACT, true);

        dt_to_refresh_ports =
            inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "refresh ports state every", 30) *
            USEC_PER_SEC;
        if (dt_to_refresh_ports < 0)
            dt_to_refresh_ports = 0;
    }

    // init listing of /sys/class/infiniband/ (or rediscovery)
    if (unlikely(!initialized) || unlikely(last_refresh_ports_usec >= dt_to_refresh_ports)) {
        // If folder does not exists, return 1 to disable
        DIR *devices_dir = opendir(sys_class_infiniband_dirname);
        if (unlikely(!devices_dir))
            return 1;

        // Work on all device available
        struct dirent *dev_dent;
        while ((dev_dent = readdir(devices_dir))) {
            // Skip special folders
            if (!strcmp(dev_dent->d_name, "..") || !strcmp(dev_dent->d_name, "."))
                continue;

            // /sys/class/infiniband/<dev>/ports
            char ports_dirname[FILENAME_MAX + 1];
            snprintfz(ports_dirname, FILENAME_MAX, "%s/%s/%s", sys_class_infiniband_dirname, dev_dent->d_name, "ports");

            DIR *ports_dir = opendir(ports_dirname);
            if (unlikely(!ports_dir))
                continue;

            struct dirent *port_dent;
            while ((port_dent = readdir(ports_dir))) {
                // Skip special folders
                if (!strcmp(port_dent->d_name, "..") || !strcmp(port_dent->d_name, "."))
                    continue;

                char buffer[FILENAME_MAX + 1];

                // Check if counters are available (mandatory)
                // /sys/class/infiniband/<device>/ports/<port>/counters
                char counters_dirname[FILENAME_MAX + 1];
                snprintfz(counters_dirname, FILENAME_MAX, "%s/%s/%s", ports_dirname, port_dent->d_name, "counters");
                DIR *counters_dir = opendir(counters_dirname);
                // Standard counters are mandatory
                if (!counters_dir)
                    continue;
                closedir(counters_dir);

                // Hardware Counters are optional, used later
                char hwcounters_dirname[FILENAME_MAX + 1];
                snprintfz(
                    hwcounters_dirname, FILENAME_MAX, "%s/%s/%s", ports_dirname, port_dent->d_name, "hw_counters");

                // Get new or existing ibport
                struct ibport *p = get_ibport(dev_dent->d_name, port_dent->d_name);
                if (!p)
                    continue;

                // Prepare configuration
                if (!p->configured) {
                    p->configured = 1;

                    // Enable by default, will be filtered out later
                    p->enabled = 1;

                    p->counters_path   = strdupz(counters_dirname);
                    p->hwcounters_path = strdupz(hwcounters_dirname);

                    snprintfz(buffer, FILENAME_MAX, "plugin:proc:/sys/class/infiniband:%s", p->name);

                    // Standard counters
                    p->do_bytes   = inicfg_get_boolean_ondemand(&netdata_config, buffer, "bytes",   do_bytes);
                    p->do_packets = inicfg_get_boolean_ondemand(&netdata_config, buffer, "packets", do_packets);
                    p->do_errors  = inicfg_get_boolean_ondemand(&netdata_config, buffer, "errors",  do_errors);

// Gen filename allocation and concatenation
#define GEN_DO_COUNTER_NAME(NAME, GRP, DESC, DIR, PORT, ...)                                                           \
    PORT->file_##NAME = callocz(1, strlen(PORT->counters_path) + sizeof(#NAME) + 3);                                   \
    strcat(PORT->file_##NAME, PORT->counters_path);                                                                    \
    strcat(PORT->file_##NAME, "/" #NAME);
                    FOREACH_COUNTER(GEN_DO_COUNTER_NAME, p)

                    // Check HW Counters vendor dependent
                    DIR *hwcounters_dir = opendir(hwcounters_dirname);
                    if (hwcounters_dir) {
                        // By default set standard
                        p->do_hwpackets = inicfg_get_boolean_ondemand(&netdata_config, buffer, "hwpackets", do_hwpackets);
                        p->do_hwerrors  = inicfg_get_boolean_ondemand(&netdata_config, buffer, "hwerrors",  do_hwerrors);

// VENDORS: Set your own

// Allocate the chars to the filenames
#define GEN_DO_HWCOUNTER_NAME(NAME, GRP, DESC, DIR, PORT, HW, ...)                                                     \
    HW->file_##NAME = callocz(1, strlen(PORT->hwcounters_path) + sizeof(#NAME) + 3);                                   \
    strcat(HW->file_##NAME, PORT->hwcounters_path);                                                                    \
    strcat(HW->file_##NAME, "/" #NAME);

                        // VENDOR-MLX: Mellanox
                        if (strncmp(dev_dent->d_name, "mlx", 3) == 0) {
                            // Allocate the vendor specific struct
                            p->hwcounters_mlx = callocz(1, sizeof(struct ibporthw_mlx));

                            FOREACH_HWCOUNTER_MLX(GEN_DO_HWCOUNTER_NAME, p, p->hwcounters_mlx)

                            // Set the function pointer for hwcounter parsing
                            p->hwcounters_parse = &infiniband_hwcounters_parse_mlx;
                            p->hwcounters_dorrd = &infiniband_hwcounters_dorrd_mlx;
                        }

                        // VENDOR: Unknown
                        else {
                            p->do_hwpackets = CONFIG_BOOLEAN_NO;
                            p->do_hwerrors = CONFIG_BOOLEAN_NO;
                        }
                        closedir(hwcounters_dir);
                    }
                }

                // Check port state to keep activation
                if (enable_only_active) {
                    snprintfz(buffer, FILENAME_MAX, "%s/%s/%s", ports_dirname, port_dent->d_name, "state");
                    unsigned long long active;
                    // File is "1: DOWN" or "4: ACTIVE", but str2ull will stop on first non-decimal char
                    read_single_number_file(buffer, &active);

                    // Want "IB_PORT_ACTIVE" == "4", as defined by drivers/infiniband/core/sysfs.c::state_show()
                    if (active == 4)
                        p->enabled = 1;
                    else
                        p->enabled = 0;
                }

                if (p->enabled)
                    p->enabled = !simple_pattern_matches(disabled_list, p->name);

                // Check / Update the link speed & width frm "rate" file
                // Sample output: "100 Gb/sec (4X EDR)"
                snprintfz(buffer, FILENAME_MAX, "%s/%s/%s", ports_dirname, port_dent->d_name, "rate");
                char buffer_rate[65];
                p->width = 4;
                if (read_txt_file(buffer, buffer_rate, sizeof(buffer_rate))) {
                    collector_error("Unable to read '%s'", buffer);
                } else {
                    char *buffer_width = strstr(buffer_rate, "(");
                    if (buffer_width) {
                        buffer_width++;
                        // str2ull will stop on first non-decimal value
                        p->speed = str2ull(buffer_rate, NULL);
                        p->width = str2ull(buffer_width, NULL);
                    }
                }

                if (!p->discovered)
                    collector_info("Infiniband card %s port %s at speed %" PRIu64 " width %" PRIu64 "",
                                   dev_dent->d_name,
                                   port_dent->d_name,
                                   p->speed,
                                   p->width);

                p->discovered = 1;
            }
            closedir(ports_dir);
        }
        closedir(devices_dir);

        initialized = 1;
        last_refresh_ports_usec = 0;
    }
    last_refresh_ports_usec += dt;

    // Update all ports values
    struct ibport *port;
    for (port = ibport_root; port; port = port->next) {
        if (!port->enabled)
            continue;
    //
    // Read values from system to struct
    //

//  counter from file and place it in ibport struct
#define GEN_DO_COUNTER_READ(NAME, GRP, DESC, DIR, PORT, ...)                                                           \
    if (PORT->file_##NAME) {                                                                                           \
        if (read_single_number_file(PORT->file_##NAME, (unsigned long long *)&PORT->NAME)) {                           \
            collector_error("cannot read iface '%s' counter '" #NAME "'", PORT->name);                                           \
            PORT->file_##NAME = NULL;                                                                                  \
        }                                                                                                              \
    }

        // Update charts
        if (port->do_bytes != CONFIG_BOOLEAN_NO) {
            // Read values from sysfs
            FOREACH_COUNTER_BYTES(GEN_DO_COUNTER_READ, port)

            // First creation of RRD Set (charts)
            if (unlikely(!port->st_bytes)) {
                port->st_bytes = rrdset_create_localhost(
                    "Infiniband",
                    port->chart_id_bytes,
                    NULL,
                    port->chart_family,
                    "ib.bytes",
                    "Bandwidth usage",
                    "kilobits/s",
                    PLUGIN_PROC_NAME,
                    PLUGIN_PROC_MODULE_INFINIBAND_NAME,
                    port->priority + 1,
                    update_every,
                    RRDSET_TYPE_AREA);

                // On this chart, we want to have a KB/s so the dashboard will autoscale it
                // The reported values are also per-lane, so we must multiply it by the width
                // x4 lanes multiplier as per Documentation/ABI/stable/sysfs-class-infiniband
                FOREACH_COUNTER_BYTES(GEN_RRD_DIM_ADD_CUSTOM, port, port->width * 8, 1000, RRD_ALGORITHM_INCREMENTAL)

                port->stv_speed = rrdvar_chart_variable_add_and_acquire(port->st_bytes, "link_speed");
            }

            // Link read values to dimensions
            FOREACH_COUNTER_BYTES(GEN_RRD_DIM_SETP, port)

            // For link speed set only variable
            rrdvar_chart_variable_set(port->st_bytes, port->stv_speed, port->speed);

            rrdset_done(port->st_bytes);
        }

        if (port->do_packets != CONFIG_BOOLEAN_NO) {
            // Read values from sysfs
            FOREACH_COUNTER_PACKETS(GEN_DO_COUNTER_READ, port)

            // First creation of RRD Set (charts)
            if (unlikely(!port->st_packets)) {
                port->st_packets = rrdset_create_localhost(
                    "Infiniband",
                    port->chart_id_packets,
                    NULL,
                    port->chart_family,
                    "ib.packets",
                    "Packets Statistics",
                    "packets/s",
                    PLUGIN_PROC_NAME,
                    PLUGIN_PROC_MODULE_INFINIBAND_NAME,
                    port->priority + 2,
                    update_every,
                    RRDSET_TYPE_AREA);

                FOREACH_COUNTER_PACKETS(GEN_RRD_DIM_ADD, port)
            }

            // Link read values to dimensions
            FOREACH_COUNTER_PACKETS(GEN_RRD_DIM_SETP, port)
            rrdset_done(port->st_packets);
        }

        if (port->do_errors != CONFIG_BOOLEAN_NO) {
            // Read values from sysfs
            FOREACH_COUNTER_ERRORS(GEN_DO_COUNTER_READ, port)

            // First creation of RRD Set (charts)
            if (unlikely(!port->st_errors)) {
                port->st_errors = rrdset_create_localhost(
                    "Infiniband",
                    port->chart_id_errors,
                    NULL,
                    port->chart_family,
                    "ib.errors",
                    "Error Counters",
                    "errors/s",
                    PLUGIN_PROC_NAME,
                    PLUGIN_PROC_MODULE_INFINIBAND_NAME,
                    port->priority + 3,
                    update_every,
                    RRDSET_TYPE_LINE);

                FOREACH_COUNTER_ERRORS(GEN_RRD_DIM_ADD, port)
            }

            // Link read values to dimensions
            FOREACH_COUNTER_ERRORS(GEN_RRD_DIM_SETP, port)
            rrdset_done(port->st_errors);
        }

        //
        // HW Counters
        //

        // Call the function for parsing and setting hwcounters
        if (port->hwcounters_parse && port->hwcounters_dorrd) {
            // Read all values (done by vendor-specific function)
            (*port->hwcounters_parse)(port);

            if (port->do_hwerrors != CONFIG_BOOLEAN_NO) {
                // First creation of RRD Set (charts)
                if (unlikely(!port->st_hwerrors)) {
                    port->st_hwerrors = rrdset_create_localhost(
                        "Infiniband",
                        port->chart_id_hwerrors,
                        NULL,
                        port->chart_family,
                        "ib.hwerrors",
                        "Hardware Errors",
                        "errors/s",
                        PLUGIN_PROC_NAME,
                        PLUGIN_PROC_MODULE_INFINIBAND_NAME,
                        port->priority + 4,
                        update_every,
                        RRDSET_TYPE_LINE);

                    // VENDORS: Set your selection

                    // VENDOR: Mellanox
                    if (strncmp(port->name, "mlx", 3) == 0) {
                        FOREACH_HWCOUNTER_MLX_ERRORS(GEN_RRD_DIM_ADD_HW, port, port->hwcounters_mlx)
                    }

                    // Unknown vendor, should not happen
                    else {
                        collector_error(
                            "Unmanaged vendor for '%s', do_hwerrors should have been set to no. Please report this bug",
                            port->name);
                        port->do_hwerrors = CONFIG_BOOLEAN_NO;
                    }
                }
            }

            if (port->do_hwpackets != CONFIG_BOOLEAN_NO) {
                // First creation of RRD Set (charts)
                if (unlikely(!port->st_hwpackets)) {
                    port->st_hwpackets = rrdset_create_localhost(
                        "Infiniband",
                        port->chart_id_hwpackets,
                        NULL,
                        port->chart_family,
                        "ib.hwpackets",
                        "Hardware Packets Statistics",
                        "packets/s",
                        PLUGIN_PROC_NAME,
                        PLUGIN_PROC_MODULE_INFINIBAND_NAME,
                        port->priority + 5,
                        update_every,
                        RRDSET_TYPE_LINE);

                    // VENDORS: Set your selection

                    // VENDOR: Mellanox
                    if (strncmp(port->name, "mlx", 3) == 0) {
                        FOREACH_HWCOUNTER_MLX_PACKETS(GEN_RRD_DIM_ADD_HW, port, port->hwcounters_mlx)
                    }

                    // Unknown vendor, should not happen
                    else {
                        collector_error(
                            "Unmanaged vendor for '%s', do_hwpackets should have been set to no. Please report this bug",
                            port->name);
                        port->do_hwpackets = CONFIG_BOOLEAN_NO;
                    }
                }
            }

            // Update values to rrd (done by vendor-specific function)
            (*port->hwcounters_dorrd)(port);
        }
    }

    return 0;
}
