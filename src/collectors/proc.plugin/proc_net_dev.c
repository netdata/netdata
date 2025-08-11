// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"
#include "proc_net_dev_renames.h"

#define PLUGIN_PROC_MODULE_NETDEV_NAME "/proc/net/dev"
#define CONFIG_SECTION_PLUGIN_PROC_NETDEV "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_NETDEV_NAME

#define RRDFUNCTIONS_NETDEV_HELP "Shows real-time network interface performance including traffic rates, packet counts, drops, and link status."

#define STATE_LENGTH_MAX 32

#define READ_RETRY_PERIOD 60 // seconds

time_t virtual_device_collect_delay_secs = 40;

enum {
    NETDEV_DUPLEX_UNKNOWN,
    NETDEV_DUPLEX_HALF,
    NETDEV_DUPLEX_FULL
};

static const char *get_duplex_string(int duplex) {
    switch (duplex) {
        case NETDEV_DUPLEX_FULL:
            return "full";
        case NETDEV_DUPLEX_HALF:
            return "half";
        default:
            return "unknown";
    }
}

enum {
    NETDEV_OPERSTATE_UNKNOWN,
    NETDEV_OPERSTATE_NOTPRESENT,
    NETDEV_OPERSTATE_DOWN,
    NETDEV_OPERSTATE_LOWERLAYERDOWN,
    NETDEV_OPERSTATE_TESTING,
    NETDEV_OPERSTATE_DORMANT,
    NETDEV_OPERSTATE_UP
};

static inline int get_operstate(char *operstate) {
    // As defined in https://www.kernel.org/doc/Documentation/ABI/testing/sysfs-class-net
    if (!strcmp(operstate, "up"))
        return NETDEV_OPERSTATE_UP;
    if (!strcmp(operstate, "down"))
        return NETDEV_OPERSTATE_DOWN;
    if (!strcmp(operstate, "notpresent"))
        return NETDEV_OPERSTATE_NOTPRESENT;
    if (!strcmp(operstate, "lowerlayerdown"))
        return NETDEV_OPERSTATE_LOWERLAYERDOWN;
    if (!strcmp(operstate, "testing"))
        return NETDEV_OPERSTATE_TESTING;
    if (!strcmp(operstate, "dormant"))
        return NETDEV_OPERSTATE_DORMANT;

    return NETDEV_OPERSTATE_UNKNOWN;
}

static const char *get_operstate_string(int operstate) {
    switch (operstate) {
        case NETDEV_OPERSTATE_UP:
            return "up";
        case NETDEV_OPERSTATE_DOWN:
            return "down";
        case NETDEV_OPERSTATE_NOTPRESENT:
            return "notpresent";
        case NETDEV_OPERSTATE_LOWERLAYERDOWN:
            return "lowerlayerdown";
        case NETDEV_OPERSTATE_TESTING:
            return "testing";
        case NETDEV_OPERSTATE_DORMANT:
            return "dormant";
        default:
            return "unknown";
    }
}

// ----------------------------------------------------------------------------
// netdev list

static struct netdev {
    char *name;
    uint32_t hash;
    size_t len;

    // flags
    bool virtual;
    bool configured;
    int enabled;
    bool updated;
    bool function_ready;

    time_t discover_time;
    
    int carrier_file_exists;
    time_t carrier_file_lost_time;

    int duplex_file_exists;
    time_t duplex_file_lost_time;

    int speed_file_exists;
    time_t speed_file_lost_time;

    int do_bandwidth;
    int do_packets;
    int do_errors;
    int do_drops;
    int do_fifo;
    int do_compressed;
    int do_events;
    int do_speed;
    int do_duplex;
    int do_operstate;
    int do_carrier;
    int do_mtu;

    const char *chart_type_net_bytes;
    const char *chart_type_net_packets;
    const char *chart_type_net_errors;
    const char *chart_type_net_fifo;
    const char *chart_type_net_events;
    const char *chart_type_net_drops;
    const char *chart_type_net_compressed;
    const char *chart_type_net_speed;
    const char *chart_type_net_duplex;
    const char *chart_type_net_operstate;
    const char *chart_type_net_carrier;
    const char *chart_type_net_mtu;

    const char *chart_id_net_bytes;
    const char *chart_id_net_packets;
    const char *chart_id_net_errors;
    const char *chart_id_net_fifo;
    const char *chart_id_net_events;
    const char *chart_id_net_drops;
    const char *chart_id_net_compressed;
    const char *chart_id_net_speed;
    const char *chart_id_net_duplex;
    const char *chart_id_net_operstate;
    const char *chart_id_net_carrier;
    const char *chart_id_net_mtu;

    const char *chart_ctx_net_bytes;
    const char *chart_ctx_net_packets;
    const char *chart_ctx_net_errors;
    const char *chart_ctx_net_fifo;
    const char *chart_ctx_net_events;
    const char *chart_ctx_net_drops;
    const char *chart_ctx_net_compressed;
    const char *chart_ctx_net_speed;
    const char *chart_ctx_net_duplex;
    const char *chart_ctx_net_operstate;
    const char *chart_ctx_net_carrier;
    const char *chart_ctx_net_mtu;

    const char *chart_family;

    RRDLABELS *chart_labels;

    int flipped;
    unsigned long priority;

    // data collected
    kernel_uint_t rbytes;
    kernel_uint_t rpackets;
    kernel_uint_t rerrors;
    kernel_uint_t rdrops;
    kernel_uint_t rfifo;
    kernel_uint_t rframe;
    kernel_uint_t rcompressed;
    kernel_uint_t rmulticast;

    kernel_uint_t tbytes;
    kernel_uint_t tpackets;
    kernel_uint_t terrors;
    kernel_uint_t tdrops;
    kernel_uint_t tfifo;
    kernel_uint_t tcollisions;
    kernel_uint_t tcarrier;
    kernel_uint_t tcompressed;
    kernel_uint_t speed;
    kernel_uint_t duplex;
    kernel_uint_t operstate;
    unsigned long long carrier;
    unsigned long long mtu;

    // charts
    RRDSET *st_bandwidth;
    RRDSET *st_packets;
    RRDSET *st_errors;
    RRDSET *st_drops;
    RRDSET *st_fifo;
    RRDSET *st_compressed;
    RRDSET *st_events;
    RRDSET *st_speed;
    RRDSET *st_duplex;
    RRDSET *st_operstate;
    RRDSET *st_carrier;
    RRDSET *st_mtu;

    // dimensions
    RRDDIM *rd_rbytes;
    RRDDIM *rd_rpackets;
    RRDDIM *rd_rerrors;
    RRDDIM *rd_rdrops;
    RRDDIM *rd_rfifo;
    RRDDIM *rd_rframe;
    RRDDIM *rd_rcompressed;
    RRDDIM *rd_rmulticast;

    RRDDIM *rd_tbytes;
    RRDDIM *rd_tpackets;
    RRDDIM *rd_terrors;
    RRDDIM *rd_tdrops;
    RRDDIM *rd_tfifo;
    RRDDIM *rd_tcollisions;
    RRDDIM *rd_tcarrier;
    RRDDIM *rd_tcompressed;

    RRDDIM *rd_speed;
    RRDDIM *rd_duplex_full;
    RRDDIM *rd_duplex_half;
    RRDDIM *rd_duplex_unknown;
    RRDDIM *rd_operstate_unknown;
    RRDDIM *rd_operstate_notpresent;
    RRDDIM *rd_operstate_down;
    RRDDIM *rd_operstate_lowerlayerdown;
    RRDDIM *rd_operstate_testing;
    RRDDIM *rd_operstate_dormant;
    RRDDIM *rd_operstate_up;
    RRDDIM *rd_carrier_up;
    RRDDIM *rd_carrier_down;
    RRDDIM *rd_mtu;

    char *filename_speed;
    const RRDVAR_ACQUIRED *chart_var_speed;

    char *filename_duplex;
    char *filename_operstate;
    char *filename_carrier;
    char *filename_mtu;

    const DICTIONARY_ITEM *cgroup_netdev_link;

    struct netdev *prev, *next;
} *netdev_root = NULL;

// ----------------------------------------------------------------------------

static void netdev_charts_release(struct netdev *d) {
    rrdvar_chart_variable_release(d->st_bandwidth, d->chart_var_speed);

    if(d->st_bandwidth) rrdset_is_obsolete___safe_from_collector_thread(d->st_bandwidth);
    if(d->st_packets) rrdset_is_obsolete___safe_from_collector_thread(d->st_packets);
    if(d->st_errors) rrdset_is_obsolete___safe_from_collector_thread(d->st_errors);
    if(d->st_drops) rrdset_is_obsolete___safe_from_collector_thread(d->st_drops);
    if(d->st_fifo) rrdset_is_obsolete___safe_from_collector_thread(d->st_fifo);
    if(d->st_compressed) rrdset_is_obsolete___safe_from_collector_thread(d->st_compressed);
    if(d->st_events) rrdset_is_obsolete___safe_from_collector_thread(d->st_events);
    if(d->st_speed) rrdset_is_obsolete___safe_from_collector_thread(d->st_speed);
    if(d->st_duplex) rrdset_is_obsolete___safe_from_collector_thread(d->st_duplex);
    if(d->st_operstate) rrdset_is_obsolete___safe_from_collector_thread(d->st_operstate);
    if(d->st_carrier) rrdset_is_obsolete___safe_from_collector_thread(d->st_carrier);
    if(d->st_mtu) rrdset_is_obsolete___safe_from_collector_thread(d->st_mtu);

    d->st_bandwidth   = NULL;
    d->st_compressed  = NULL;
    d->st_drops       = NULL;
    d->st_errors      = NULL;
    d->st_events      = NULL;
    d->st_fifo        = NULL;
    d->st_packets     = NULL;
    d->st_speed       = NULL;
    d->st_duplex      = NULL;
    d->st_operstate   = NULL;
    d->st_carrier     = NULL;
    d->st_mtu         = NULL;

    d->rd_rbytes      = NULL;
    d->rd_rpackets    = NULL;
    d->rd_rerrors     = NULL;
    d->rd_rdrops      = NULL;
    d->rd_rfifo       = NULL;
    d->rd_rframe      = NULL;
    d->rd_rcompressed = NULL;
    d->rd_rmulticast  = NULL;

    d->rd_tbytes      = NULL;
    d->rd_tpackets    = NULL;
    d->rd_terrors     = NULL;
    d->rd_tdrops      = NULL;
    d->rd_tfifo       = NULL;
    d->rd_tcollisions = NULL;
    d->rd_tcarrier    = NULL;
    d->rd_tcompressed = NULL;

    d->rd_speed       = NULL;
    d->rd_duplex_full = NULL;
    d->rd_duplex_half = NULL;
    d->rd_duplex_unknown = NULL;
    d->rd_carrier_up = NULL;
    d->rd_carrier_down = NULL;
    d->rd_mtu         = NULL;

    d->rd_operstate_unknown = NULL;
    d->rd_operstate_notpresent = NULL;
    d->rd_operstate_down = NULL;
    d->rd_operstate_lowerlayerdown = NULL;
    d->rd_operstate_testing = NULL;
    d->rd_operstate_dormant = NULL;
    d->rd_operstate_up = NULL;

    d->chart_var_speed     = NULL;
}

static void netdev_free_chart_strings(struct netdev *d) {
    freez_and_set_to_null(d->chart_type_net_bytes);
    freez_and_set_to_null(d->chart_type_net_compressed);
    freez_and_set_to_null(d->chart_type_net_drops);
    freez_and_set_to_null(d->chart_type_net_errors);
    freez_and_set_to_null(d->chart_type_net_events);
    freez_and_set_to_null(d->chart_type_net_fifo);
    freez_and_set_to_null(d->chart_type_net_packets);
    freez_and_set_to_null(d->chart_type_net_speed);
    freez_and_set_to_null(d->chart_type_net_duplex);
    freez_and_set_to_null(d->chart_type_net_operstate);
    freez_and_set_to_null(d->chart_type_net_carrier);
    freez_and_set_to_null(d->chart_type_net_mtu);

    freez_and_set_to_null(d->chart_id_net_bytes);
    freez_and_set_to_null(d->chart_id_net_compressed);
    freez_and_set_to_null(d->chart_id_net_drops);
    freez_and_set_to_null(d->chart_id_net_errors);
    freez_and_set_to_null(d->chart_id_net_events);
    freez_and_set_to_null(d->chart_id_net_fifo);
    freez_and_set_to_null(d->chart_id_net_packets);
    freez_and_set_to_null(d->chart_id_net_speed);
    freez_and_set_to_null(d->chart_id_net_duplex);
    freez_and_set_to_null(d->chart_id_net_operstate);
    freez_and_set_to_null(d->chart_id_net_carrier);
    freez_and_set_to_null(d->chart_id_net_mtu);

    freez_and_set_to_null(d->chart_ctx_net_bytes);
    freez_and_set_to_null(d->chart_ctx_net_compressed);
    freez_and_set_to_null(d->chart_ctx_net_drops);
    freez_and_set_to_null(d->chart_ctx_net_errors);
    freez_and_set_to_null(d->chart_ctx_net_events);
    freez_and_set_to_null(d->chart_ctx_net_fifo);
    freez_and_set_to_null(d->chart_ctx_net_packets);
    freez_and_set_to_null(d->chart_ctx_net_speed);
    freez_and_set_to_null(d->chart_ctx_net_duplex);
    freez_and_set_to_null(d->chart_ctx_net_operstate);
    freez_and_set_to_null(d->chart_ctx_net_carrier);
    freez_and_set_to_null(d->chart_ctx_net_mtu);

    freez_and_set_to_null(d->chart_family);
}

static void netdev_free(struct netdev *d) {
    netdev_charts_release(d);
    netdev_free_chart_strings(d);

    rrdlabels_destroy(d->chart_labels);
    d->chart_labels = NULL;

    cgroup_netdev_release(d->cgroup_netdev_link);
    d->cgroup_netdev_link = NULL;

    freez_and_set_to_null(d->name);
    freez_and_set_to_null(d->filename_speed);
    freez_and_set_to_null(d->filename_duplex);
    freez_and_set_to_null(d->filename_operstate);
    freez_and_set_to_null(d->filename_carrier);
    freez_and_set_to_null(d->filename_mtu);

    freez((void *)d);
}

static netdata_mutex_t netdev_mutex;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&netdev_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&netdev_mutex);
}

// ----------------------------------------------------------------------------

static inline void netdev_rename_unsafe(struct netdev *d, struct rename_task *r) {
    collector_info("CGROUP: renaming network interface '%s' as '%s' under '%s'", d->name, r->container_device, r->container_name);

    netdev_charts_release(d);
    netdev_free_chart_strings(d);

    cgroup_netdev_release(d->cgroup_netdev_link);
    d->cgroup_netdev_link = cgroup_netdev_dup(r->cgroup_netdev_link);
    d->discover_time = 0;

    char buffer[RRD_ID_LENGTH_MAX + 1];

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "cgroup_%s", r->container_name);
    d->chart_type_net_bytes      = strdupz(buffer);
    d->chart_type_net_compressed = strdupz(buffer);
    d->chart_type_net_drops      = strdupz(buffer);
    d->chart_type_net_errors     = strdupz(buffer);
    d->chart_type_net_events     = strdupz(buffer);
    d->chart_type_net_fifo       = strdupz(buffer);
    d->chart_type_net_packets    = strdupz(buffer);
    d->chart_type_net_speed      = strdupz(buffer);
    d->chart_type_net_duplex     = strdupz(buffer);
    d->chart_type_net_operstate  = strdupz(buffer);
    d->chart_type_net_carrier    = strdupz(buffer);
    d->chart_type_net_mtu        = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_%s", r->container_device);
    d->chart_id_net_bytes      = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_compressed_%s", r->container_device);
    d->chart_id_net_compressed = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_drops_%s", r->container_device);
    d->chart_id_net_drops      = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_errors_%s", r->container_device);
    d->chart_id_net_errors     = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_events_%s", r->container_device);
    d->chart_id_net_events     = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_fifo_%s", r->container_device);
    d->chart_id_net_fifo       = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_packets_%s", r->container_device);
    d->chart_id_net_packets    = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_speed_%s", r->container_device);
    d->chart_id_net_speed      = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_duplex_%s", r->container_device);
    d->chart_id_net_duplex     = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_operstate_%s", r->container_device);
    d->chart_id_net_operstate  = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_carrier_%s", r->container_device);
    d->chart_id_net_carrier    = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_mtu_%s", r->container_device);
    d->chart_id_net_mtu        = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_net", r->ctx_prefix);
    d->chart_ctx_net_bytes      = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_compressed", r->ctx_prefix);
    d->chart_ctx_net_compressed = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_drops", r->ctx_prefix);
    d->chart_ctx_net_drops      = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_errors", r->ctx_prefix);
    d->chart_ctx_net_errors     = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_events", r->ctx_prefix);
    d->chart_ctx_net_events     = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_fifo", r->ctx_prefix);
    d->chart_ctx_net_fifo       = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_packets", r->ctx_prefix);
    d->chart_ctx_net_packets    = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_speed", r->ctx_prefix);
    d->chart_ctx_net_speed      = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_duplex", r->ctx_prefix);
    d->chart_ctx_net_duplex     = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_operstate", r->ctx_prefix);
    d->chart_ctx_net_operstate  = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_carrier", r->ctx_prefix);
    d->chart_ctx_net_carrier    = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "%scgroup.net_mtu", r->ctx_prefix);
    d->chart_ctx_net_mtu        = strdupz(buffer);

    d->chart_family = strdupz("net");

    rrdlabels_copy(d->chart_labels, r->chart_labels);
    rrdlabels_add(d->chart_labels, "container_device", r->container_device, RRDLABEL_SRC_AUTO);

    d->priority = NETDATA_CHART_PRIO_CGROUP_NET_IFACE;
    d->flipped = 1;
}

static void netdev_rename_this_device(struct netdev *d) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(netdev_renames, d->name);
    if(item) {
        struct rename_task *r = dictionary_acquired_item_value(item);
        spinlock_lock(&r->spinlock);
        netdev_rename_unsafe(d, r);
        spinlock_unlock(&r->spinlock);
        dictionary_acquired_item_release(netdev_renames, item);
    }
}

// ----------------------------------------------------------------------------

static int netdev_function_net_interfaces(BUFFER *wb, const char *function __maybe_unused, BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_NETDEV_HELP);
    buffer_json_member_add_array(wb, "data");

    double max_traffic_rx = 0.0;
    double max_traffic_tx = 0.0;
    double max_traffic = 0.0;
    double max_packets_rx = 0.0;
    double max_packets_tx = 0.0;
    double max_mcast_rx = 0.0;
    double max_drops_rx = 0.0;
    double max_drops_tx = 0.0;

    netdata_mutex_lock(&netdev_mutex);

    RRDDIM *rd = NULL;

    for (struct netdev *d = netdev_root; d ; d = d->next) {
        if (unlikely(!d->function_ready))
            continue;

        buffer_json_add_array_item_array(wb);

        buffer_json_add_array_item_string(wb, d->name);

        buffer_json_add_array_item_string(wb, d->virtual ? "virtual" : "physical");
        buffer_json_add_array_item_string(wb, d->flipped ? "cgroup" : "host");
        buffer_json_add_array_item_string(wb, d->carrier == 1 ? "up" : "down");
        buffer_json_add_array_item_string(wb, get_operstate_string(d->operstate));
        buffer_json_add_array_item_string(wb, get_duplex_string(d->duplex));
        buffer_json_add_array_item_double(wb, d->speed > 0 ? d->speed : NAN);
        buffer_json_add_array_item_double(wb, d->mtu > 0 ? d->mtu : NAN);

        rd = d->flipped ? d->rd_tbytes : d->rd_rbytes;
        double traffic_rx = rrddim_get_last_stored_value(rd, &max_traffic_rx, 1000.0);
        rd = d->flipped ? d->rd_rbytes : d->rd_tbytes;
        double traffic_tx = rrddim_get_last_stored_value(rd, &max_traffic_tx, 1000.0);

        rd = d->flipped ? d->rd_tpackets : d->rd_rpackets;
        double packets_rx = rrddim_get_last_stored_value(rd, &max_packets_rx, 1000.0);
        rd = d->flipped ? d->rd_rpackets : d->rd_tpackets;
        double packets_tx = rrddim_get_last_stored_value(rd, &max_packets_tx, 1000.0);

        double mcast_rx = rrddim_get_last_stored_value(d->rd_rmulticast, &max_mcast_rx, 1000.0);

        rd = d->flipped ? d->rd_tdrops : d->rd_rdrops;
        double drops_rx = rrddim_get_last_stored_value(rd, &max_drops_rx, 1.0);
        rd = d->flipped ? d->rd_rdrops : d->rd_tdrops;
        double drops_tx = rrddim_get_last_stored_value(rd, &max_drops_tx, 1.0);

        // FIXME: "traffic" (total) is needed only for default_sorting
        // can be removed when default_sorting will accept multiple columns (sum)
        double traffic = NAN;
        if (!isnan(traffic_rx) && !isnan(traffic_tx)) {
            traffic = traffic_rx + traffic_tx;
            max_traffic = MAX(max_traffic, traffic);
        }


        buffer_json_add_array_item_double(wb, traffic_rx);
        buffer_json_add_array_item_double(wb, traffic_tx);
        buffer_json_add_array_item_double(wb, traffic);
        buffer_json_add_array_item_double(wb, packets_rx);
        buffer_json_add_array_item_double(wb, packets_tx);
        buffer_json_add_array_item_double(wb, mcast_rx);
        buffer_json_add_array_item_double(wb, drops_rx);
        buffer_json_add_array_item_double(wb, drops_tx);

        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "severity", drops_rx + drops_tx > 0 ? "warning" : "normal");
        }
        buffer_json_object_close(wb);

        buffer_json_array_close(wb);
    }

    netdata_mutex_unlock(&netdev_mutex);

    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        buffer_rrdf_table_add_field(wb, field_id++, "Interface", "Network Interface Name",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Type", "Network Interface Type",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "UsedBy", "Indicates whether the network interface is used by a cgroup or by the host system",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "PhState", "Current Physical State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OpState", "Current Operational State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Duplex", "Current Duplex Mode",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Speed", "Current Link Speed",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                0, "Mbit", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MTU", "Maximum Transmission Unit",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                0, "Octets", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "In", "Traffic Received",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Mbit", max_traffic_rx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Out", "Traffic Sent",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Mbit", max_traffic_tx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "Total", "Traffic Received and Sent",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Mbit", max_traffic, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "PktsIn", "Received Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Kpps", max_packets_rx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "PktsOut", "Sent Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Kpps", max_packets_tx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "McastIn", "Multicast Received Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Kpps", max_mcast_rx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DropsIn", "Dropped Inbound Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Drops", max_drops_rx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "DropsOut", "Dropped Outbound Packets",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Drops", max_drops_tx, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        buffer_rrdf_table_add_field(
                wb, field_id++,
                "rowOptions", "rowOptions",
                RRDF_FIELD_TYPE_NONE,
                RRDR_FIELD_VISUAL_ROW_OPTIONS,
                RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                RRDF_FIELD_SORT_FIXED,
                NULL,
                RRDF_FIELD_SUMMARY_COUNT,
                RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_DUMMY,
                NULL);
    }

    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Total");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "Traffic");
        {
            buffer_json_member_add_string(wb, "name", "Traffic");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "In");
                buffer_json_add_array_item_string(wb, "Out");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Packets");
        {
            buffer_json_member_add_string(wb, "name", "Packets");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "PktsIn");
                buffer_json_add_array_item_string(wb, "PktsOut");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Traffic");
        buffer_json_add_array_item_string(wb, "Interface");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Traffic");
        buffer_json_add_array_item_string(wb, "Type");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "Type");
        {
            buffer_json_member_add_string(wb, "name", "Type");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Type");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "UsedBy");
        {
            buffer_json_member_add_string(wb, "name", "UsedBy");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "UsedBy");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

// netdev data collection

static void netdev_cleanup() {
    struct netdev *d = netdev_root;
    while(d) {
        if(unlikely(!d->updated)) {
            struct netdev *next = d->next; // keep the next, to continue;

            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(netdev_root, d, prev, next);

            netdev_free(d);
            d = next;
            continue;
        }

        d->updated = false;
        d = d->next;
    }
}

static struct netdev *get_netdev(const char *name) {
    struct netdev *d;

    uint32_t hash = simple_hash(name);

    // search it, from the last position to the end
    for(d = netdev_root ; d ; d = d->next) {
        if(unlikely(hash == d->hash && !strcmp(name, d->name)))
            return d;
    }

    // create a new one
    d = callocz(1, sizeof(struct netdev));
    d->name = strdupz(name);
    d->hash = simple_hash(d->name);
    d->len = strlen(d->name);
    d->chart_labels = rrdlabels_create();
    d->function_ready = false;

    d->chart_type_net_bytes      = strdupz("net");
    d->chart_type_net_compressed = strdupz("net_compressed");
    d->chart_type_net_drops      = strdupz("net_drops");
    d->chart_type_net_errors     = strdupz("net_errors");
    d->chart_type_net_events     = strdupz("net_events");
    d->chart_type_net_fifo       = strdupz("net_fifo");
    d->chart_type_net_packets    = strdupz("net_packets");
    d->chart_type_net_speed      = strdupz("net_speed");
    d->chart_type_net_duplex     = strdupz("net_duplex");
    d->chart_type_net_operstate  = strdupz("net_operstate");
    d->chart_type_net_carrier    = strdupz("net_carrier");
    d->chart_type_net_mtu        = strdupz("net_mtu");

    d->chart_id_net_bytes      = strdupz(d->name);
    d->chart_id_net_compressed = strdupz(d->name);
    d->chart_id_net_drops      = strdupz(d->name);
    d->chart_id_net_errors     = strdupz(d->name);
    d->chart_id_net_events     = strdupz(d->name);
    d->chart_id_net_fifo       = strdupz(d->name);
    d->chart_id_net_packets    = strdupz(d->name);
    d->chart_id_net_speed      = strdupz(d->name);
    d->chart_id_net_duplex     = strdupz(d->name);
    d->chart_id_net_operstate  = strdupz(d->name);
    d->chart_id_net_carrier    = strdupz(d->name);
    d->chart_id_net_mtu        = strdupz(d->name);

    d->chart_ctx_net_bytes      = strdupz("net.net");
    d->chart_ctx_net_compressed = strdupz("net.compressed");
    d->chart_ctx_net_drops      = strdupz("net.drops");
    d->chart_ctx_net_errors     = strdupz("net.errors");
    d->chart_ctx_net_events     = strdupz("net.events");
    d->chart_ctx_net_fifo       = strdupz("net.fifo");
    d->chart_ctx_net_packets    = strdupz("net.packets");
    d->chart_ctx_net_speed      = strdupz("net.speed");
    d->chart_ctx_net_duplex     = strdupz("net.duplex");
    d->chart_ctx_net_operstate  = strdupz("net.operstate");
    d->chart_ctx_net_carrier    = strdupz("net.carrier");
    d->chart_ctx_net_mtu        = strdupz("net.mtu");

    d->chart_family = strdupz(d->name);
    d->priority = NETDATA_CHART_PRIO_FIRST_NET_IFACE;

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(netdev_root, d, prev, next);

    return d;
}

int do_proc_net_dev(int update_every, usec_t dt) {
    (void)dt;
    static SIMPLE_PATTERN *disabled_list = NULL;
    static SIMPLE_PATTERN *virtual_iface_no_delay = NULL;
    static procfile *ff = NULL;
    static int enable_new_interfaces = -1;
    static int do_bandwidth = -1, do_packets = -1, do_errors = -1, do_drops = -1, do_fifo = -1, do_compressed = -1,
               do_events = -1, do_speed = -1, do_duplex = -1, do_operstate = -1, do_carrier = -1, do_mtu = -1;
    static char *path_to_sys_devices_virtual_net = NULL, *path_to_sys_class_net_speed = NULL,
                *proc_net_dev_filename = NULL;
    static char *path_to_sys_class_net_duplex = NULL;
    static char *path_to_sys_class_net_operstate = NULL;
    static char *path_to_sys_class_net_carrier = NULL;
    static char *path_to_sys_class_net_mtu = NULL;

    if(unlikely(enable_new_interfaces == -1)) {
        char filename[FILENAME_MAX + 1];

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, (*netdata_configured_host_prefix)?"/proc/1/net/dev":"/proc/net/dev");
        proc_net_dev_filename = strdupz(filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/virtual/net/%s");
        path_to_sys_devices_virtual_net = strdupz(filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/speed");
        path_to_sys_class_net_speed = strdupz(filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/duplex");
        path_to_sys_class_net_duplex = strdupz(filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/operstate");
        path_to_sys_class_net_operstate = strdupz(filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/carrier");
        path_to_sys_class_net_carrier = strdupz(filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/mtu");
        path_to_sys_class_net_mtu = strdupz(filename);

        enable_new_interfaces = CONFIG_BOOLEAN_YES;

        do_bandwidth = do_packets = do_errors = do_drops = do_fifo = do_events = do_speed = do_duplex = do_operstate =
            do_carrier = do_mtu = CONFIG_BOOLEAN_YES;

        // only CSLIP, PPP
        do_compressed   = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_NETDEV, "compressed packets for all interfaces", CONFIG_BOOLEAN_NO);

        disabled_list = simple_pattern_create(
            inicfg_get(&netdata_config, 
                CONFIG_SECTION_PLUGIN_PROC_NETDEV,
                "disable by default interfaces matching",
                "lo fireqos* *-ifb fwpr* fwbr* fwln* ifb4*"),
            NULL,
            SIMPLE_PATTERN_EXACT,
            true);

        virtual_iface_no_delay = simple_pattern_create(
            " bond* "
            " vlan* "
            " vmbr* "
            " wg* "
            " vpn* "
            " tun* "
            " gre* "
            " docker* ",
            NULL,
            SIMPLE_PATTERN_EXACT,
            true);

        netdev_renames_init();
    }

    if(unlikely(!ff)) {
        ff = procfile_open(proc_net_dev_filename, " \t,|", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    kernel_uint_t system_rbytes = 0;
    kernel_uint_t system_tbytes = 0;

    time_t now = now_realtime_sec();

    size_t lines = procfile_lines(ff), l;
    for(l = 2; l < lines ;l++) {
        // require 17 words on each line
        if(unlikely(procfile_linewords(ff, l) < 17)) continue;

        char *name = procfile_lineword(ff, l, 0);
        size_t len = strlen(name);
        if(name[len - 1] == ':') name[len - 1] = '\0';

        struct netdev *d = get_netdev(name);
        d->updated = true;

        if(unlikely(!d->configured)) {
            // the first time we see this interface

            // remember we configured it
            d->configured = true;
            d->discover_time = now;

            d->enabled = enable_new_interfaces;

            if(d->enabled)
                d->enabled = !simple_pattern_matches(disabled_list, d->name);

            char buf[FILENAME_MAX + 1];
            snprintfz(buf, FILENAME_MAX, path_to_sys_devices_virtual_net, d->name);

            d->virtual = likely(access(buf, R_OK) == 0) ? true : false;

            // At least on Proxmox inside LXC: eth0 is virtual.
            // Virtual interfaces are not taken into account in system.net calculations
            if (inside_lxc_container && d->virtual && strncmp(d->name, "eth", 3) == 0)
                d->virtual = false;

            if (d->virtual)
                rrdlabels_add(d->chart_labels, "interface_type", "virtual", RRDLABEL_SRC_AUTO);
            else
                rrdlabels_add(d->chart_labels, "interface_type", "real", RRDLABEL_SRC_AUTO);

            rrdlabels_add(d->chart_labels, "device", name, RRDLABEL_SRC_AUTO);

            if(likely(!d->virtual)) {
                // set the filename to get the interface speed
                snprintfz(buf, FILENAME_MAX, path_to_sys_class_net_speed, d->name);
                d->filename_speed = strdupz(buf);

                snprintfz(buf, FILENAME_MAX, path_to_sys_class_net_duplex, d->name);
                d->filename_duplex = strdupz(buf);
            }

            snprintfz(buf, FILENAME_MAX, path_to_sys_class_net_operstate, d->name);
            d->filename_operstate = strdupz(buf);

            snprintfz(buf, FILENAME_MAX, path_to_sys_class_net_carrier, d->name);
            d->filename_carrier = strdupz(buf);

            snprintfz(buf, FILENAME_MAX, path_to_sys_class_net_mtu, d->name);
            d->filename_mtu = strdupz(buf);

            snprintfz(buf, FILENAME_MAX, "plugin:proc:/proc/net/dev:%s", d->name);

            if (inicfg_exists(&netdata_config, buf, "enabled"))
                d->enabled = inicfg_get_boolean_ondemand(&netdata_config, buf, "enabled", d->enabled);
            if (inicfg_exists(&netdata_config, buf, "virtual"))
                d->virtual = inicfg_get_boolean(&netdata_config, buf, "virtual", d->virtual);

            if(d->enabled == CONFIG_BOOLEAN_NO)
                continue;

            d->do_bandwidth = do_bandwidth;
            d->do_packets = do_packets;
            d->do_errors = do_errors;
            d->do_drops = do_drops;
            d->do_fifo = do_fifo;
            d->do_compressed = do_compressed;
            d->do_events = do_events;
            d->do_speed = do_speed;
            d->do_duplex = do_duplex;
            d->do_operstate = do_operstate;
            d->do_carrier = do_carrier;
            d->do_mtu = do_mtu;

            if (inicfg_exists(&netdata_config, buf, "bandwidth"))
                d->do_bandwidth = inicfg_get_boolean_ondemand(&netdata_config, buf, "bandwidth", do_bandwidth);
            if (inicfg_exists(&netdata_config, buf, "packets"))
                d->do_packets = inicfg_get_boolean_ondemand(&netdata_config, buf, "packets", do_packets);
            if (inicfg_exists(&netdata_config, buf, "errors"))
                d->do_errors = inicfg_get_boolean_ondemand(&netdata_config, buf, "errors", do_errors);
            if (inicfg_exists(&netdata_config, buf, "drops"))
                d->do_drops = inicfg_get_boolean_ondemand(&netdata_config, buf, "drops", do_drops);
            if (inicfg_exists(&netdata_config, buf, "fifo"))
                d->do_fifo = inicfg_get_boolean_ondemand(&netdata_config, buf, "fifo", do_fifo);
            if (inicfg_exists(&netdata_config, buf, "compressed"))
                d->do_compressed = inicfg_get_boolean_ondemand(&netdata_config, buf, "compressed", do_compressed);
            if (inicfg_exists(&netdata_config, buf, "events"))
                d->do_events = inicfg_get_boolean_ondemand(&netdata_config, buf, "events", do_events);
            if (inicfg_exists(&netdata_config, buf, "speed"))
                d->do_speed = inicfg_get_boolean_ondemand(&netdata_config, buf, "speed", do_speed);
            if (inicfg_exists(&netdata_config, buf, "duplex"))
                d->do_duplex = inicfg_get_boolean_ondemand(&netdata_config, buf, "duplex", do_duplex);
            if (inicfg_exists(&netdata_config, buf, "operstate"))
                d->do_operstate = inicfg_get_boolean_ondemand(&netdata_config, buf, "operstate", do_operstate);
            if (inicfg_exists(&netdata_config, buf, "carrier"))
                d->do_carrier = inicfg_get_boolean_ondemand(&netdata_config, buf, "carrier", do_carrier);
            if (inicfg_exists(&netdata_config, buf, "mtu"))
                d->do_mtu = inicfg_get_boolean_ondemand(&netdata_config, buf, "mtu", do_mtu);
        }

        if(unlikely(!d->enabled))
            continue;

        if(!d->cgroup_netdev_link)
            netdev_rename_this_device(d);

        // See https://github.com/netdata/netdata/issues/15206
        // This is necessary to prevent the creation of charts for virtual interfaces that will later be 
        // recreated as container interfaces (create container) or
        // rediscovered and recreated only to be deleted almost immediately (stop/remove container)
        if (d->virtual && !simple_pattern_matches(virtual_iface_no_delay, d->name) &&
            (now - d->discover_time < virtual_device_collect_delay_secs)) {
            continue;
        }

        if(likely(d->do_bandwidth != CONFIG_BOOLEAN_NO || !d->virtual)) {
            d->rbytes      = str2kernel_uint_t(procfile_lineword(ff, l, 1));
            d->tbytes      = str2kernel_uint_t(procfile_lineword(ff, l, 9));

            if(likely(!d->virtual)) {
                system_rbytes += d->rbytes;
                system_tbytes += d->tbytes;
            }
        }

        if(likely(d->do_packets != CONFIG_BOOLEAN_NO)) {
            d->rpackets    = str2kernel_uint_t(procfile_lineword(ff, l, 2));
            d->rmulticast  = str2kernel_uint_t(procfile_lineword(ff, l, 8));
            d->tpackets    = str2kernel_uint_t(procfile_lineword(ff, l, 10));
        }

        if(likely(d->do_errors != CONFIG_BOOLEAN_NO)) {
            d->rerrors     = str2kernel_uint_t(procfile_lineword(ff, l, 3));
            d->terrors     = str2kernel_uint_t(procfile_lineword(ff, l, 11));
        }

        if(likely(d->do_drops != CONFIG_BOOLEAN_NO)) {
            d->rdrops      = str2kernel_uint_t(procfile_lineword(ff, l, 4));
            d->tdrops      = str2kernel_uint_t(procfile_lineword(ff, l, 12));
        }

        if(likely(d->do_fifo != CONFIG_BOOLEAN_NO)) {
            d->rfifo       = str2kernel_uint_t(procfile_lineword(ff, l, 5));
            d->tfifo       = str2kernel_uint_t(procfile_lineword(ff, l, 13));
        }

        if(likely(d->do_compressed != CONFIG_BOOLEAN_NO)) {
            d->rcompressed = str2kernel_uint_t(procfile_lineword(ff, l, 7));
            d->tcompressed = str2kernel_uint_t(procfile_lineword(ff, l, 16));
        }

        if(likely(d->do_events != CONFIG_BOOLEAN_NO)) {
            d->rframe      = str2kernel_uint_t(procfile_lineword(ff, l, 6));
            d->tcollisions = str2kernel_uint_t(procfile_lineword(ff, l, 14));
            d->tcarrier    = str2kernel_uint_t(procfile_lineword(ff, l, 15));
        }

        if ((d->do_carrier != CONFIG_BOOLEAN_NO ||
             d->do_duplex != CONFIG_BOOLEAN_NO ||
             d->do_speed != CONFIG_BOOLEAN_NO) &&
             d->filename_carrier &&
            (d->carrier_file_exists ||
             now_monotonic_sec() - d->carrier_file_lost_time > READ_RETRY_PERIOD)) {
            if (read_single_number_file(d->filename_carrier, &d->carrier)) {
                if (d->carrier_file_exists)
                    collector_error(
                        "Cannot refresh interface %s carrier state by reading '%s'. Next update is in %d seconds.",
                        d->name,
                        d->filename_carrier,
                        READ_RETRY_PERIOD);
                d->carrier_file_exists = 0;
                d->carrier_file_lost_time = now_monotonic_sec();
            } else {
                d->carrier_file_exists = 1;
                d->carrier_file_lost_time = 0;
            }
        }

        if (d->do_duplex != CONFIG_BOOLEAN_NO &&
            d->filename_duplex &&
            (d->carrier || d->carrier_file_exists) &&
            (d->duplex_file_exists ||
             now_monotonic_sec() - d->duplex_file_lost_time > READ_RETRY_PERIOD)) {
            char buffer[STATE_LENGTH_MAX + 1];

            if (read_txt_file(d->filename_duplex, buffer, sizeof(buffer))) {
                if (d->duplex_file_exists)
                    collector_error("Cannot refresh interface %s duplex state by reading '%s'.", d->name, d->filename_duplex);
                d->duplex_file_exists = 0;
                d->duplex_file_lost_time = now_monotonic_sec();
                d->duplex = NETDEV_DUPLEX_UNKNOWN;
            } else {
                // values can be unknown, half or full -- just check the first letter for speed
                if (buffer[0] == 'f')
                    d->duplex = NETDEV_DUPLEX_FULL;
                else if (buffer[0] == 'h')
                    d->duplex = NETDEV_DUPLEX_HALF;
                else
                    d->duplex = NETDEV_DUPLEX_UNKNOWN;
                d->duplex_file_exists = 1;
                d->duplex_file_lost_time = 0;
            }
        } else {
            d->duplex = NETDEV_DUPLEX_UNKNOWN;
        }

        if(d->do_operstate != CONFIG_BOOLEAN_NO && d->filename_operstate) {
            char buffer[STATE_LENGTH_MAX + 1], *trimmed_buffer;

            if (read_txt_file(d->filename_operstate, buffer, sizeof(buffer))) {
                collector_error(
                    "Cannot refresh %s operstate by reading '%s'. Will not update its status anymore.",
                    d->name, d->filename_operstate);
                freez(d->filename_operstate);
                d->filename_operstate = NULL;
            } else {
                trimmed_buffer = trim(buffer);
                d->operstate = get_operstate(trimmed_buffer);
            }
        }

        if (d->do_mtu != CONFIG_BOOLEAN_NO && d->filename_mtu) {
            if (read_single_number_file(d->filename_mtu, &d->mtu)) {
                collector_error(
                    "Cannot refresh mtu for interface %s by reading '%s'. Stop updating it.", d->name, d->filename_mtu);
                freez(d->filename_mtu);
                d->filename_mtu = NULL;
            }
        }

        //collector_info("PROC_NET_DEV: %s speed %zu, bytes %zu/%zu, packets %zu/%zu/%zu, errors %zu/%zu, drops %zu/%zu, fifo %zu/%zu, compressed %zu/%zu, rframe %zu, tcollisions %zu, tcarrier %zu"
        //        , d->name, d->speed
        //        , d->rbytes, d->tbytes
        //        , d->rpackets, d->tpackets, d->rmulticast
        //        , d->rerrors, d->terrors
        //        , d->rdrops, d->tdrops
        //        , d->rfifo, d->tfifo
        //        , d->rcompressed, d->tcompressed
        //        , d->rframe, d->tcollisions, d->tcarrier
        //        );

        if (d->do_bandwidth == CONFIG_BOOLEAN_YES || d->do_bandwidth == CONFIG_BOOLEAN_AUTO) {
            if(unlikely(!d->st_bandwidth)) {

                d->st_bandwidth = rrdset_create_localhost(
                        d->chart_type_net_bytes
                        , d->chart_id_net_bytes
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_bytes
                        , "Bandwidth"
                        , "kilobits/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_update_rrdlabels(d->st_bandwidth, d->chart_labels);

                d->rd_rbytes = rrddim_add(d->st_bandwidth, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tbytes = rrddim_add(d->st_bandwidth, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

                    RRDDIM *td = d->rd_rbytes;
                    d->rd_rbytes = d->rd_tbytes;
                    d->rd_tbytes = td;
                }
            }

            rrddim_set_by_pointer(d->st_bandwidth, d->rd_rbytes, (collected_number)d->rbytes);
            rrddim_set_by_pointer(d->st_bandwidth, d->rd_tbytes, (collected_number)d->tbytes);
            rrdset_done(d->st_bandwidth);

            if(d->cgroup_netdev_link)
                cgroup_netdev_add_bandwidth(d->cgroup_netdev_link,
                                            d->flipped ? d->rd_tbytes->collector.last_stored_value : -d->rd_rbytes->collector.last_stored_value,
                                            d->flipped ? -d->rd_rbytes->collector.last_stored_value : d->rd_tbytes->collector.last_stored_value);

            if(unlikely(!d->chart_var_speed)) {
                d->chart_var_speed = rrdvar_chart_variable_add_and_acquire(d->st_bandwidth, "nic_speed_max");
                if(!d->chart_var_speed) {
                    collector_error(
                        "Cannot create interface %s chart variable 'nic_speed_max'. Will not update its speed anymore.",
                        d->name);
                }
                else {
                    rrdvar_chart_variable_set(d->st_bandwidth, d->chart_var_speed, NAN);
                }
            }

            // update the interface speed
            if(d->filename_speed) {
                if (d->filename_speed && d->chart_var_speed) {
                    int ret = 0;

                    if ((d->carrier || d->carrier_file_exists) &&
                        (d->speed_file_exists || now_monotonic_sec() - d->speed_file_lost_time > READ_RETRY_PERIOD)) {
                        ret = read_single_number_file(d->filename_speed, (unsigned long long *) &d->speed);
                    } else {
                        d->speed = 0; // TODO: this is wrong, shouldn't use 0 value, but NULL.
                    }

                    if(ret) {
                        if (d->speed_file_exists)
                            collector_error("Cannot refresh interface %s speed by reading '%s'.", d->name, d->filename_speed);
                        d->speed_file_exists = 0;
                        d->speed_file_lost_time = now_monotonic_sec();
                    }
                    else {
                        if(d->do_speed != CONFIG_BOOLEAN_NO) {
                            if(unlikely(!d->st_speed)) {
                                d->st_speed = rrdset_create_localhost(
                                        d->chart_type_net_speed
                                        , d->chart_id_net_speed
                                        , NULL
                                        , d->chart_family
                                        , d->chart_ctx_net_speed
                                        , "Interface Speed"
                                        , "kilobits/s"
                                        , PLUGIN_PROC_NAME
                                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                                        , d->priority + 7
                                        , update_every
                                        , RRDSET_TYPE_LINE
                                );

                                rrdset_update_rrdlabels(d->st_speed, d->chart_labels);

                                d->rd_speed = rrddim_add(d->st_speed, "speed",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
                            }

                            rrddim_set_by_pointer(d->st_speed, d->rd_speed, (collected_number)d->speed * KILOBITS_IN_A_MEGABIT);
                            rrdset_done(d->st_speed);
                        }

                        rrdvar_chart_variable_set(
                            d->st_bandwidth, d->chart_var_speed, (NETDATA_DOUBLE)d->speed * KILOBITS_IN_A_MEGABIT);

                        if (d->speed) {
                            d->speed_file_exists = 1;
                            d->speed_file_lost_time = 0;
                        }
                    }
                }
            }
        }

        if(d->do_duplex != CONFIG_BOOLEAN_NO && d->filename_duplex) {
            if(unlikely(!d->st_duplex)) {
                d->st_duplex = rrdset_create_localhost(
                        d->chart_type_net_duplex
                        , d->chart_id_net_duplex
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_duplex
                        , "Interface Duplex State"
                        , "state"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 8
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_duplex, d->chart_labels);

                d->rd_duplex_full = rrddim_add(d->st_duplex, "full", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                d->rd_duplex_half = rrddim_add(d->st_duplex, "half", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                d->rd_duplex_unknown = rrddim_add(d->st_duplex, "unknown", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(d->st_duplex, d->rd_duplex_full, (collected_number)(d->duplex == NETDEV_DUPLEX_FULL));
            rrddim_set_by_pointer(d->st_duplex, d->rd_duplex_half, (collected_number)(d->duplex == NETDEV_DUPLEX_HALF));
            rrddim_set_by_pointer(d->st_duplex, d->rd_duplex_unknown, (collected_number)(d->duplex == NETDEV_DUPLEX_UNKNOWN));
            rrdset_done(d->st_duplex);
        }

        if(d->do_operstate != CONFIG_BOOLEAN_NO && d->filename_operstate) {
            if(unlikely(!d->st_operstate)) {
                d->st_operstate = rrdset_create_localhost(
                        d->chart_type_net_operstate
                        , d->chart_id_net_operstate
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_operstate
                        , "Interface Operational State"
                        , "state"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 9
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_operstate, d->chart_labels);

                d->rd_operstate_up = rrddim_add(d->st_operstate, "up", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                d->rd_operstate_down = rrddim_add(d->st_operstate, "down", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                d->rd_operstate_notpresent = rrddim_add(d->st_operstate, "notpresent", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                d->rd_operstate_lowerlayerdown = rrddim_add(d->st_operstate, "lowerlayerdown", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                d->rd_operstate_testing = rrddim_add(d->st_operstate, "testing", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                d->rd_operstate_dormant = rrddim_add(d->st_operstate, "dormant", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                d->rd_operstate_unknown = rrddim_add(d->st_operstate, "unknown", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(d->st_operstate, d->rd_operstate_up, (collected_number)(d->operstate == NETDEV_OPERSTATE_UP));
            rrddim_set_by_pointer(d->st_operstate, d->rd_operstate_down, (collected_number)(d->operstate == NETDEV_OPERSTATE_DOWN));
            rrddim_set_by_pointer(d->st_operstate, d->rd_operstate_notpresent, (collected_number)(d->operstate == NETDEV_OPERSTATE_NOTPRESENT));
            rrddim_set_by_pointer(d->st_operstate, d->rd_operstate_lowerlayerdown, (collected_number)(d->operstate == NETDEV_OPERSTATE_LOWERLAYERDOWN));
            rrddim_set_by_pointer(d->st_operstate, d->rd_operstate_testing, (collected_number)(d->operstate == NETDEV_OPERSTATE_TESTING));
            rrddim_set_by_pointer(d->st_operstate, d->rd_operstate_dormant, (collected_number)(d->operstate == NETDEV_OPERSTATE_DORMANT));
            rrddim_set_by_pointer(d->st_operstate, d->rd_operstate_unknown, (collected_number)(d->operstate == NETDEV_OPERSTATE_UNKNOWN));
            rrdset_done(d->st_operstate);
        }

        if(d->do_carrier != CONFIG_BOOLEAN_NO && d->carrier_file_exists) {
            if(unlikely(!d->st_carrier)) {
                d->st_carrier = rrdset_create_localhost(
                        d->chart_type_net_carrier
                        , d->chart_id_net_carrier
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_carrier
                        , "Interface Physical Link State"
                        , "state"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 10
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_carrier, d->chart_labels);

                d->rd_carrier_up = rrddim_add(d->st_carrier, "up",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
                d->rd_carrier_down = rrddim_add(d->st_carrier, "down",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(d->st_carrier, d->rd_carrier_up, (collected_number)(d->carrier == 1));
            rrddim_set_by_pointer(d->st_carrier, d->rd_carrier_down, (collected_number)(d->carrier != 1));
            rrdset_done(d->st_carrier);
        }

        if(d->do_mtu != CONFIG_BOOLEAN_NO && d->filename_mtu) {
            if(unlikely(!d->st_mtu)) {
                d->st_mtu = rrdset_create_localhost(
                        d->chart_type_net_mtu
                        , d->chart_id_net_mtu
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_mtu
                        , "Interface MTU"
                        , "octets"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 11
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_mtu, d->chart_labels);

                d->rd_mtu = rrddim_add(d->st_mtu, "mtu",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(d->st_mtu, d->rd_mtu, (collected_number)d->mtu);
            rrdset_done(d->st_mtu);
        }

        if (d->do_packets == CONFIG_BOOLEAN_YES || d->do_packets == CONFIG_BOOLEAN_AUTO) {
            if(unlikely(!d->st_packets)) {

                d->st_packets = rrdset_create_localhost(
                        d->chart_type_net_packets
                        , d->chart_id_net_packets
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_packets
                        , "Packets"
                        , "packets/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 1
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_packets, d->chart_labels);

                d->rd_rpackets   = rrddim_add(d->st_packets, "received",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tpackets   = rrddim_add(d->st_packets, "sent",      NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_rmulticast = rrddim_add(d->st_packets, "multicast", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

                    RRDDIM *td = d->rd_rpackets;
                    d->rd_rpackets = d->rd_tpackets;
                    d->rd_tpackets = td;
                }
            }

            rrddim_set_by_pointer(d->st_packets, d->rd_rpackets, (collected_number)d->rpackets);
            rrddim_set_by_pointer(d->st_packets, d->rd_tpackets, (collected_number)d->tpackets);
            rrddim_set_by_pointer(d->st_packets, d->rd_rmulticast, (collected_number)d->rmulticast);
            rrdset_done(d->st_packets);
        }

        if (d->do_errors == CONFIG_BOOLEAN_YES || d->do_errors == CONFIG_BOOLEAN_AUTO) {
            if(unlikely(!d->st_errors)) {

                d->st_errors = rrdset_create_localhost(
                        d->chart_type_net_errors
                        , d->chart_id_net_errors
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_errors
                        , "Interface Errors"
                        , "errors/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 2
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_errors, d->chart_labels);

                d->rd_rerrors = rrddim_add(d->st_errors, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_terrors = rrddim_add(d->st_errors, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

                    RRDDIM *td = d->rd_rerrors;
                    d->rd_rerrors = d->rd_terrors;
                    d->rd_terrors = td;
                }
            }

            rrddim_set_by_pointer(d->st_errors, d->rd_rerrors, (collected_number)d->rerrors);
            rrddim_set_by_pointer(d->st_errors, d->rd_terrors, (collected_number)d->terrors);
            rrdset_done(d->st_errors);
        }

        if (d->do_drops == CONFIG_BOOLEAN_YES || d->do_drops == CONFIG_BOOLEAN_AUTO) {
            if(unlikely(!d->st_drops)) {

                d->st_drops = rrdset_create_localhost(
                        d->chart_type_net_drops
                        , d->chart_id_net_drops
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_drops
                        , "Interface Drops"
                        , "drops/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 3
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_drops, d->chart_labels);

                d->rd_rdrops = rrddim_add(d->st_drops, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tdrops = rrddim_add(d->st_drops, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

                    RRDDIM *td = d->rd_rdrops;
                    d->rd_rdrops = d->rd_tdrops;
                    d->rd_tdrops = td;
                }
            }

            rrddim_set_by_pointer(d->st_drops, d->rd_rdrops, (collected_number)d->rdrops);
            rrddim_set_by_pointer(d->st_drops, d->rd_tdrops, (collected_number)d->tdrops);
            rrdset_done(d->st_drops);
        }

        if (d->do_fifo == CONFIG_BOOLEAN_YES || d->do_fifo == CONFIG_BOOLEAN_AUTO) {
            if(unlikely(!d->st_fifo)) {

                d->st_fifo = rrdset_create_localhost(
                        d->chart_type_net_fifo
                        , d->chart_id_net_fifo
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_fifo
                        , "Interface FIFO Buffer Errors"
                        , "errors"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 4
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_fifo, d->chart_labels);

                d->rd_rfifo = rrddim_add(d->st_fifo, "receive",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tfifo = rrddim_add(d->st_fifo, "transmit", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

                    RRDDIM *td = d->rd_rfifo;
                    d->rd_rfifo = d->rd_tfifo;
                    d->rd_tfifo = td;
                }
            }

            rrddim_set_by_pointer(d->st_fifo, d->rd_rfifo, (collected_number)d->rfifo);
            rrddim_set_by_pointer(d->st_fifo, d->rd_tfifo, (collected_number)d->tfifo);
            rrdset_done(d->st_fifo);
        }

        if (d->do_compressed == CONFIG_BOOLEAN_YES || d->do_compressed == CONFIG_BOOLEAN_AUTO) {
            if(unlikely(!d->st_compressed)) {

                d->st_compressed = rrdset_create_localhost(
                        d->chart_type_net_compressed
                        , d->chart_id_net_compressed
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_compressed
                        , "Compressed Packets"
                        , "packets/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 5
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_compressed, d->chart_labels);

                d->rd_rcompressed = rrddim_add(d->st_compressed, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tcompressed = rrddim_add(d->st_compressed, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

                    RRDDIM *td = d->rd_rcompressed;
                    d->rd_rcompressed = d->rd_tcompressed;
                    d->rd_tcompressed = td;
                }
            }

            rrddim_set_by_pointer(d->st_compressed, d->rd_rcompressed, (collected_number)d->rcompressed);
            rrddim_set_by_pointer(d->st_compressed, d->rd_tcompressed, (collected_number)d->tcompressed);
            rrdset_done(d->st_compressed);
        }

        if (d->do_events == CONFIG_BOOLEAN_YES || d->do_events == CONFIG_BOOLEAN_AUTO) {
            if(unlikely(!d->st_events)) {

                d->st_events = rrdset_create_localhost(
                        d->chart_type_net_events
                        , d->chart_id_net_events
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_events
                        , "Network Interface Events"
                        , "events/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 6
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_update_rrdlabels(d->st_events, d->chart_labels);

                d->rd_rframe      = rrddim_add(d->st_events, "frames",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tcollisions = rrddim_add(d->st_events, "collisions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tcarrier    = rrddim_add(d->st_events, "carrier",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(d->st_events, d->rd_rframe,      (collected_number)d->rframe);
            rrddim_set_by_pointer(d->st_events, d->rd_tcollisions, (collected_number)d->tcollisions);
            rrddim_set_by_pointer(d->st_events, d->rd_tcarrier,    (collected_number)d->tcarrier);
            rrdset_done(d->st_events);
        }

        d->function_ready = true;
    }

    if (do_bandwidth == CONFIG_BOOLEAN_YES || do_bandwidth == CONFIG_BOOLEAN_AUTO) {
        do_bandwidth = CONFIG_BOOLEAN_YES;
        static RRDSET *st_system_net = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_system_net)) {
            st_system_net = rrdset_create_localhost(
                    "system"
                    , "net"
                    , NULL
                    , "network"
                    , NULL
                    , "Physical Network Interfaces Aggregated Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETDEV_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_NET
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_system_net, "InOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_system_net, "OutOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_system_net, rd_in,  (collected_number)system_rbytes);
        rrddim_set_by_pointer(st_system_net, rd_out, (collected_number)system_tbytes);

        rrdset_done(st_system_net);
    }

    netdev_cleanup();

    return 0;
}

static void netdev_main_cleanup(void *pptr) {
    if(CLEANUP_FUNCTION_GET_PTR(pptr) != (void *)0x01)
        return;

    netdata_mutex_lock(&netdev_mutex);
    while(netdev_root) {
        struct netdev *d = netdev_root;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(netdev_root, d, prev, next);
        netdev_free(d);
    }
    netdata_mutex_unlock(&netdev_mutex);

    worker_unregister();
}

void netdev_main(void *ptr_is_null __maybe_unused)
{
    CLEANUP_FUNCTION_REGISTER(netdev_main_cleanup) cleanup_ptr = (void *)0x01;

    worker_register("NETDEV");
    worker_register_job_name(0, "netdev");

    if (getenv("KUBERNETES_SERVICE_HOST") != NULL && getenv("KUBERNETES_SERVICE_PORT") != NULL)
        virtual_device_collect_delay_secs = 300;

    rrd_function_add_inline(localhost, NULL, "network-interfaces", 10,
                            RRDFUNCTIONS_PRIORITY_DEFAULT, RRDFUNCTIONS_VERSION_DEFAULT,
                            RRDFUNCTIONS_NETDEV_HELP,
                            "top", HTTP_ACCESS_ANONYMOUS_DATA,
                            netdev_function_net_interfaces);

    heartbeat_t hb;
    heartbeat_init(&hb, localhost->rrd_update_every * USEC_PER_SEC);

    while (service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb);

        if (unlikely(!service_running(SERVICE_COLLECTORS)))
            break;

        cgroup_netdev_reset_all();

        worker_is_busy(0);

        netdata_mutex_lock(&netdev_mutex);
        if (do_proc_net_dev(localhost->rrd_update_every, hb_dt))
            break;
        netdata_mutex_unlock(&netdev_mutex);
    }
}
