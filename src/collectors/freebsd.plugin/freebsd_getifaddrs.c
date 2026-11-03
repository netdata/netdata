// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_freebsd.h"
#include "../common-contexts/network-interfaces-function.h"

#include <ifaddrs.h>
#include <net/if_media.h>
#include <sys/ioctl.h>
#include <sys/sockio.h>

struct cgroup_network_interface {
    char *name;
    uint32_t hash;
    size_t len;

    // flags
    int configured;
    int enabled;
    int updated;

    int do_bandwidth;
    int do_packets;
    int do_errors;
    int do_drops;
    int do_events;
    int do_speed;
    int do_duplex;
    int do_operstate;
    int do_carrier;
    int do_mtu;

    bool physical;
    int link_state;
    int operstate;
    int duplex;
    double speed_mbit;
    uint32_t mtu;

    // charts and dimensions

    RRDSET *st_bandwidth;
    RRDDIM *rd_bandwidth_in;
    RRDDIM *rd_bandwidth_out;

    RRDSET *st_packets;
    RRDDIM *rd_packets_in;
    RRDDIM *rd_packets_out;
    RRDDIM *rd_packets_m_in;
    RRDDIM *rd_packets_m_out;

    RRDSET *st_errors;
    RRDDIM *rd_errors_in;
    RRDDIM *rd_errors_out;

    RRDSET *st_drops;
    RRDDIM *rd_drops_in;
    RRDDIM *rd_drops_out;

    RRDSET *st_events;
    RRDDIM *rd_events_coll;

    RRDSET *st_speed;
    RRDDIM *rd_speed;

    RRDSET *st_duplex;
    RRDDIM *rd_duplex_full;
    RRDDIM *rd_duplex_half;
    RRDDIM *rd_duplex_unknown;

    RRDSET *st_operstate;
    RRDDIM *rd_operstate_up;
    RRDDIM *rd_operstate_down;
    RRDDIM *rd_operstate_unknown;

    RRDSET *st_carrier;
    RRDDIM *rd_carrier_up;
    RRDDIM *rd_carrier_down;

    RRDSET *st_mtu;
    RRDDIM *rd_mtu;

    const RRDVAR_ACQUIRED *chart_var_speed;

    struct cgroup_network_interface *next;
};

static struct cgroup_network_interface *network_interfaces_root = NULL, *network_interfaces_last_used = NULL;
static netdata_mutex_t network_interfaces_mutex;
static int network_interfaces_update_every = 1;
static int network_interfaces_media_fd = -1;

enum {
    FREEBSD_INTERFACE_UNKNOWN,
    FREEBSD_INTERFACE_DOWN,
    FREEBSD_INTERFACE_UP,
};

enum {
    FREEBSD_DUPLEX_UNKNOWN,
    FREEBSD_DUPLEX_HALF,
    FREEBSD_DUPLEX_FULL,
};

static const char *freebsd_interface_state(int state) {
    switch (state) {
        case FREEBSD_INTERFACE_UP: return "up";
        case FREEBSD_INTERFACE_DOWN: return "down";
        default: return "unknown";
    }
}

static const char *freebsd_interface_duplex(int duplex) {
    switch (duplex) {
        case FREEBSD_DUPLEX_FULL: return "full";
        case FREEBSD_DUPLEX_HALF: return "half";
        default: return "unknown";
    }
}

static void freebsd_interface_add_labels(RRDSET *st, const char *name, bool physical) {
    rrdlabels_add(st->rrdlabels, "device", name, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "interface_type", physical ? "real" : "unknown", RRDLABEL_SRC_AUTO);
}

static int freebsd_function_network_interfaces(BUFFER *wb, const char *function __maybe_unused,
                                                BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", network_interfaces_update_every);
    buffer_json_member_add_boolean(wb, "has_history", false);
    buffer_json_member_add_string(wb, "help", "Shows real-time network interface performance including traffic rates, packet counts, drops, and link status.");
    buffer_json_member_add_array(wb, "data");

    network_interfaces_function_maxima maxima = {0};
    netdata_mutex_lock(&network_interfaces_mutex);
    for (struct cgroup_network_interface *ifm = network_interfaces_root; ifm; ifm = ifm->next) {
        if (!ifm->enabled)
            continue;

        double traffic_rx = rrddim_get_last_stored_value(ifm->rd_bandwidth_in, &maxima.traffic_rx, 1000.0);
        double traffic_tx = rrddim_get_last_stored_value(ifm->rd_bandwidth_out, &maxima.traffic_tx, 1000.0);
        double traffic = NAN;
        if (!isnan(traffic_rx) && !isnan(traffic_tx)) {
            traffic = traffic_rx + traffic_tx;
            maxima.traffic = MAX(maxima.traffic, traffic);
        }
        double packets_rx = rrddim_get_last_stored_value(ifm->rd_packets_in, &maxima.packets_rx, 1000.0);
        double packets_tx = rrddim_get_last_stored_value(ifm->rd_packets_out, &maxima.packets_tx, 1000.0);
        double mcast_rx = rrddim_get_last_stored_value(ifm->rd_packets_m_in, &maxima.mcast_rx, 1000.0);
        double drops_rx = rrddim_get_last_stored_value(ifm->rd_drops_in, &maxima.drops_rx, 1.0);
        double drops_tx = rrddim_get_last_stored_value(ifm->rd_drops_out, &maxima.drops_tx, 1.0);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, ifm->name);
        buffer_json_add_array_item_string(wb, ifm->physical ? "physical" : "unknown");
        buffer_json_add_array_item_string(wb, "host");
        buffer_json_add_array_item_string(wb, freebsd_interface_state(ifm->link_state));
        buffer_json_add_array_item_string(wb, freebsd_interface_state(ifm->operstate));
        buffer_json_add_array_item_string(wb, freebsd_interface_duplex(ifm->duplex));
        buffer_json_add_array_item_double(wb, ifm->speed_mbit);
        buffer_json_add_array_item_double(wb, ifm->mtu ? ifm->mtu : NAN);
        buffer_json_add_array_item_double(wb, traffic_rx);
        buffer_json_add_array_item_double(wb, traffic_tx);
        buffer_json_add_array_item_double(wb, traffic);
        buffer_json_add_array_item_double(wb, packets_rx);
        buffer_json_add_array_item_double(wb, packets_tx);
        buffer_json_add_array_item_double(wb, mcast_rx);
        buffer_json_add_array_item_double(wb, drops_rx);
        buffer_json_add_array_item_double(wb, drops_tx);
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "severity", drops_rx + drops_tx > 0 ? "warning" : "normal");
        buffer_json_object_close(wb);
        buffer_json_array_close(wb);
    }
    netdata_mutex_unlock(&network_interfaces_mutex);

    buffer_json_array_close(wb);
    network_interfaces_function_schema(wb, &maxima);
    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + network_interfaces_update_every);
    buffer_json_finalize(wb);
    return HTTP_RESP_OK;
}

static void network_interface_free(struct cgroup_network_interface *ifm) {
    if (ifm->chart_var_speed)
        rrdvar_chart_variable_release(ifm->st_bandwidth, ifm->chart_var_speed);
    if (likely(ifm->st_bandwidth))
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_bandwidth);
    if (likely(ifm->st_packets))
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_packets);
    if (likely(ifm->st_errors))
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_errors);
    if (likely(ifm->st_drops))
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_drops);
    if (likely(ifm->st_events))
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_events);
    if (ifm->st_speed)
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_speed);
    if (ifm->st_duplex)
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_duplex);
    if (ifm->st_operstate)
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_operstate);
    if (ifm->st_carrier)
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_carrier);
    if (ifm->st_mtu)
        rrdset_is_obsolete___safe_from_collector_thread(ifm->st_mtu);

    freez(ifm->name);
    freez(ifm);
}

static void network_interfaces_cleanup() {
    struct cgroup_network_interface *ifm = network_interfaces_root, *last = NULL;
    while(ifm) {
        if (unlikely(!ifm->updated)) {
            // collector_info("Removing network interface '%s', linked after '%s'", ifm->name, last?last->name:"ROOT");

            if (network_interfaces_last_used == ifm)
                network_interfaces_last_used = last;

            struct cgroup_network_interface *t = ifm;

            if (ifm == network_interfaces_root || !last)
                network_interfaces_root = ifm = ifm->next;

            else
                last->next = ifm = ifm->next;

            t->next = NULL;
            network_interface_free(t);
        }
        else {
            last = ifm;
            ifm->updated = 0;
            ifm = ifm->next;
        }
    }
}

static struct cgroup_network_interface *get_network_interface(const char *name) {
    struct cgroup_network_interface *ifm;

    uint32_t hash = simple_hash(name);

    // search it, from the last position to the end
    for(ifm = network_interfaces_last_used ; ifm ; ifm = ifm->next) {
        if (unlikely(hash == ifm->hash && !strcmp(name, ifm->name))) {
            network_interfaces_last_used = ifm->next;
            return ifm;
        }
    }

    // search it from the beginning to the last position we used
    for(ifm = network_interfaces_root ; ifm != network_interfaces_last_used ; ifm = ifm->next) {
        if (unlikely(hash == ifm->hash && !strcmp(name, ifm->name))) {
            network_interfaces_last_used = ifm->next;
            return ifm;
        }
    }

    // create a new one
    ifm = callocz(1, sizeof(struct cgroup_network_interface));
    ifm->name = strdupz(name);
    ifm->hash = simple_hash(ifm->name);
    ifm->len = strlen(ifm->name);
    // link it to the end
    if (network_interfaces_root) {
        struct cgroup_network_interface *e;
        for(e = network_interfaces_root; e->next ; e = e->next) ;
        e->next = ifm;
    }
    else
        network_interfaces_root = ifm;

    return ifm;
}

// --------------------------------------------------------------------------------------------------------------------
// getifaddrs

int do_getifaddrs(int update_every, usec_t dt) {
    (void)dt;

#define DEFAULT_EXCLUDED_INTERFACES "lo*"
#define DEFAULT_PHYSICAL_INTERFACES "igb* ix* cxl* em* ixl* ixlv* bge* ixgbe* vtnet* vmx* re* igc* dwc*"
#define CONFIG_SECTION_GETIFADDRS "plugin:freebsd:getifaddrs"

    static int enable_new_interfaces = -1;
    static int do_bandwidth_ipv4 = -1, do_bandwidth_ipv6 = -1, do_bandwidth = -1, do_packets = -1, do_bandwidth_net = -1, do_packets_net = -1,
            do_errors = -1, do_drops = -1, do_events = -1, do_speed = -1, do_duplex = -1,
            do_operstate = -1, do_carrier = -1, do_mtu = -1;
    static SIMPLE_PATTERN *excluded_interfaces = NULL, *physical_interfaces = NULL;

    if (unlikely(enable_new_interfaces == -1)) {
        enable_new_interfaces = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS,
                                                              "enable new interfaces detected at runtime",
                                                              CONFIG_BOOLEAN_AUTO);

        do_bandwidth_net  = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "total bandwidth for physical interfaces",
                                                       CONFIG_BOOLEAN_AUTO);
        do_packets_net    = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "total packets for physical interfaces",
                                                       CONFIG_BOOLEAN_AUTO);
        do_bandwidth_ipv4 = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "total bandwidth for ipv4 interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_bandwidth_ipv6 = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "total bandwidth for ipv6 interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_bandwidth      = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "bandwidth for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_packets        = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "packets for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_errors         = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "errors for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_drops          = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "drops for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_events         = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "collisions for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_speed          = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "speed for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_duplex         = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "duplex for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_operstate      = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "operstate for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_carrier        = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "carrier for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_mtu            = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_GETIFADDRS, "mtu for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);

        excluded_interfaces = simple_pattern_create(
            inicfg_get(&netdata_config, CONFIG_SECTION_GETIFADDRS, "disable by default interfaces matching", DEFAULT_EXCLUDED_INTERFACES),
            NULL,
            SIMPLE_PATTERN_EXACT,
            true);
        physical_interfaces = simple_pattern_create(
            inicfg_get(&netdata_config, CONFIG_SECTION_GETIFADDRS, "set physical interfaces for system.net", DEFAULT_PHYSICAL_INTERFACES),
            NULL,
            SIMPLE_PATTERN_EXACT,
            true);

        // The Function reads collector-owned interface state from another thread.
        netdata_mutex_init(&network_interfaces_mutex);
        network_interfaces_update_every = update_every;
        nrpc_method_register_builtin(&(struct nrpc_builtin_desc) {
            .owner = rrdhost_nrpc_owner(localhost),
            .name = "network-interfaces",
            .help = "Shows real-time network interface performance including traffic rates, packet counts, drops, and link status.",
            .tags = "top",
            .timeout_s = 10,
            .priority = NRPC_PRIORITY_DEFAULT,
            .version = NRPC_VERSION_DEFAULT,
            .access = HTTP_ACCESS_ANONYMOUS_DATA,
            .handler = freebsd_function_network_interfaces,
        });
    }

    if (likely(do_bandwidth_ipv4 || do_bandwidth_ipv6 || do_bandwidth || do_packets || do_errors || do_bandwidth_net || do_packets_net ||
               do_drops || do_events || do_speed || do_duplex || do_operstate || do_carrier || do_mtu)) {
        struct ifaddrs *ifap;

        if (unlikely(getifaddrs(&ifap))) {
            collector_error("FREEBSD: getifaddrs() failed");
            netdata_mutex_lock(&network_interfaces_mutex);
            for (struct cgroup_network_interface *ifm = network_interfaces_root; ifm; ifm = ifm->next)
                ifm->updated = 0;
            network_interfaces_cleanup();
            netdata_mutex_unlock(&network_interfaces_mutex);
            if (network_interfaces_media_fd >= 0) {
                close(network_interfaces_media_fd);
                network_interfaces_media_fd = -1;
            }
            do_bandwidth_net = 0;
            collector_error("DISABLED: system.net chart");
            do_packets_net = 0;
            collector_error("DISABLED: system.packets chart");
            do_bandwidth_ipv4 = 0;
            collector_error("DISABLED: system.ipv4 chart");
            do_bandwidth_ipv6 = 0;
            collector_error("DISABLED: system.ipv6 chart");
            do_bandwidth = 0;
            collector_error("DISABLED: net.* charts");
            do_packets = 0;
            collector_error("DISABLED: net_packets.* charts");
            do_errors = 0;
            collector_error("DISABLED: net_errors.* charts");
            do_drops = 0;
            collector_error("DISABLED: net_drops.* charts");
            do_events = 0;
            collector_error("DISABLED: net_events.* charts");
            collector_error("DISABLED: getifaddrs module");
            return 1;
        } else {
#define IFA_DATA(s) (((struct if_data *)ifa->ifa_data)->ifi_ ## s)
            struct ifaddrs *ifa;
            struct iftot {
                u_long  ift_ibytes;
                u_long  ift_obytes;
                u_long  ift_ipackets;
                u_long  ift_opackets;
                u_long  ift_imcasts;
                u_long  ift_omcasts;
            } iftot = {0, 0, 0, 0, 0, 0};

            if (likely(do_bandwidth_net)) {

                iftot.ift_ibytes = iftot.ift_obytes = 0;
                for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                    if (!ifa->ifa_addr || !ifa->ifa_data || ifa->ifa_addr->sa_family != AF_LINK)
                        continue;
                    if (!simple_pattern_matches(physical_interfaces, ifa->ifa_name))
                        continue;
                    iftot.ift_ibytes += IFA_DATA(ibytes);
                    iftot.ift_obytes += IFA_DATA(obytes);
                }

                static RRDSET *st = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("system",
                                                 "net",
                                                 NULL,
                                                 "network",
                                                 NULL,
                                                 "Network Traffic",
                                                 "kilobits/s",
                                                 "freebsd.plugin",
                                                 "getifaddrs",
                                                 NETDATA_CHART_PRIO_SYSTEM_NET,
                                                 update_every,
                                                 RRDSET_TYPE_AREA
                                                 );

                    rd_in  = rrddim_add(st, "InOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st, "OutOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set_by_pointer(st, rd_in,  iftot.ift_ibytes);
                rrddim_set_by_pointer(st, rd_out, iftot.ift_obytes);
                rrdset_done(st);
            }

            if (likely(do_packets_net)) {
                iftot.ift_ipackets = iftot.ift_opackets = iftot.ift_imcasts = iftot.ift_omcasts = 0;

                for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                    if (!ifa->ifa_addr || !ifa->ifa_data || ifa->ifa_addr->sa_family != AF_LINK)
                        continue;
                    if (!simple_pattern_matches(physical_interfaces, ifa->ifa_name))
                        continue;
                    iftot.ift_ipackets += IFA_DATA(ipackets);
                    iftot.ift_opackets += IFA_DATA(opackets);
                    iftot.ift_imcasts += IFA_DATA(imcasts);
                    iftot.ift_omcasts += IFA_DATA(omcasts);
                }

                static RRDSET *st = NULL;
                static RRDDIM *rd_packets_in = NULL, *rd_packets_out = NULL, *rd_packets_m_in = NULL, *rd_packets_m_out = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("system",
                                                 "packets",
                                                 NULL,
                                                 "network",
                                                 NULL,
                                                 "Network Packets",
                                                 "packets/s",
                                                 "freebsd.plugin",
                                                 "getifaddrs",
                                                 NETDATA_CHART_PRIO_SYSTEM_PACKETS,
                                                 update_every,
                                                 RRDSET_TYPE_LINE
                                                 );

                    rd_packets_in    = rrddim_add(st, "received",           NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_packets_out   = rrddim_add(st, "sent",               NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_packets_m_in  = rrddim_add(st, "multicast_received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_packets_m_out = rrddim_add(st, "multicast_sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set_by_pointer(st, rd_packets_in,    iftot.ift_ipackets);
                rrddim_set_by_pointer(st, rd_packets_out,   iftot.ift_opackets);
                rrddim_set_by_pointer(st, rd_packets_m_in,  iftot.ift_imcasts);
                rrddim_set_by_pointer(st, rd_packets_m_out, iftot.ift_omcasts);
                rrdset_done(st);
            }

            if (likely(do_bandwidth_ipv4)) {
                iftot.ift_ibytes = iftot.ift_obytes = 0;
                for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                    if (!ifa->ifa_addr || !ifa->ifa_data || ifa->ifa_addr->sa_family != AF_INET)
                        continue;
                    iftot.ift_ibytes += IFA_DATA(ibytes);
                    iftot.ift_obytes += IFA_DATA(obytes);
                }

                static RRDSET *st = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("system",
                                                 "ipv4",
                                                 NULL,
                                                 "network",
                                                 NULL,
                                                 "IPv4 Bandwidth",
                                                 "kilobits/s",
                                                 "freebsd.plugin",
                                                 "getifaddrs",
                                                 NETDATA_CHART_PRIO_SYSTEM_IPV4,
                                                 update_every,
                                                 RRDSET_TYPE_AREA
                    );

                    rd_in  = rrddim_add(st, "InOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st, "OutOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set_by_pointer(st, rd_in,  iftot.ift_ibytes);
                rrddim_set_by_pointer(st, rd_out, iftot.ift_obytes);
                rrdset_done(st);
            }

            if (likely(do_bandwidth_ipv6)) {
                iftot.ift_ibytes = iftot.ift_obytes = 0;
                for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                    if (!ifa->ifa_addr || !ifa->ifa_data || ifa->ifa_addr->sa_family != AF_INET6)
                        continue;
                    iftot.ift_ibytes += IFA_DATA(ibytes);
                    iftot.ift_obytes += IFA_DATA(obytes);
                }

                static RRDSET *st = NULL;
                static RRDDIM *rd_in = NULL, *rd_out = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost("system",
                                                 "ipv6",
                                                 NULL,
                                                 "network",
                                                 NULL,
                                                 "IPv6 Bandwidth",
                                                 "kilobits/s",
                                                 "freebsd.plugin",
                                                 "getifaddrs",
                                                 NETDATA_CHART_PRIO_SYSTEM_IPV6,
                                                 update_every,
                                                 RRDSET_TYPE_AREA
                    );

                    rd_in  = rrddim_add(st, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                }

                rrddim_set_by_pointer(st, rd_in,  iftot.ift_ibytes);
                rrddim_set_by_pointer(st, rd_out, iftot.ift_obytes);
                rrdset_done(st);
            }

            netdata_mutex_lock(&network_interfaces_mutex);
            bool media_open_attempted = false;

            for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                if (!ifa->ifa_addr || !ifa->ifa_data || ifa->ifa_addr->sa_family != AF_LINK)
                    continue;

                struct cgroup_network_interface *ifm = get_network_interface(ifa->ifa_name);
                ifm->updated = 1;
                struct if_data *if_data = ifa->ifa_data;
                ifm->physical = simple_pattern_matches(physical_interfaces, ifa->ifa_name);
                ifm->link_state = if_data->ifi_link_state == LINK_STATE_UP ? FREEBSD_INTERFACE_UP :
                                  if_data->ifi_link_state == LINK_STATE_DOWN ? FREEBSD_INTERFACE_DOWN : FREEBSD_INTERFACE_UNKNOWN;
                ifm->operstate = !(ifa->ifa_flags & IFF_UP) ? FREEBSD_INTERFACE_DOWN : ifm->link_state;
                ifm->duplex = FREEBSD_DUPLEX_UNKNOWN;
                ifm->speed_mbit = NAN;
                ifm->mtu = if_data->ifi_mtu;

                if (unlikely(!ifm->configured)) {
                    char var_name[4096 + 1];

                    // this is the first time we see this network interface

                    // remember we configured it
                    ifm->configured = 1;

                    ifm->enabled = enable_new_interfaces;

                    if (likely(ifm->enabled))
                        ifm->enabled = !simple_pattern_matches(excluded_interfaces, ifa->ifa_name);

                    snprintfz(var_name, 4096, "%s:%s", CONFIG_SECTION_GETIFADDRS, ifa->ifa_name);
                    ifm->enabled = inicfg_get_boolean_ondemand(&netdata_config, var_name, "enabled", ifm->enabled);

                    if (unlikely(ifm->enabled == CONFIG_BOOLEAN_NO))
                        continue;

                    ifm->do_bandwidth = inicfg_get_boolean_ondemand(&netdata_config, var_name, "bandwidth", do_bandwidth);
                    ifm->do_packets   = inicfg_get_boolean_ondemand(&netdata_config, var_name, "packets",   do_packets);
                    ifm->do_errors    = inicfg_get_boolean_ondemand(&netdata_config, var_name, "errors",    do_errors);
                    ifm->do_drops     = inicfg_get_boolean_ondemand(&netdata_config, var_name, "drops",     do_drops);
                    ifm->do_events    = inicfg_get_boolean_ondemand(&netdata_config, var_name, "events",    do_events);
                    ifm->do_speed     = inicfg_get_boolean_ondemand(&netdata_config, var_name, "speed",     do_speed);
                    ifm->do_duplex    = inicfg_get_boolean_ondemand(&netdata_config, var_name, "duplex",    do_duplex);
                    ifm->do_operstate = inicfg_get_boolean_ondemand(&netdata_config, var_name, "operstate", do_operstate);
                    ifm->do_carrier   = inicfg_get_boolean_ondemand(&netdata_config, var_name, "carrier",   do_carrier);
                    ifm->do_mtu       = inicfg_get_boolean_ondemand(&netdata_config, var_name, "mtu",       do_mtu);
                }

                if (unlikely(!ifm->enabled))
                    continue;

                // The Function consumes media state even when the charts are disabled.
                if (network_interfaces_media_fd < 0 && !media_open_attempted) {
                    media_open_attempted = true;
                    network_interfaces_media_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
                }
                if (network_interfaces_media_fd >= 0) {
                    struct ifmediareq media = {0};
                    strncpyz(media.ifm_name, ifa->ifa_name, sizeof(media.ifm_name) - 1);
                    if (ioctl(network_interfaces_media_fd, SIOCGIFMEDIA, &media) == 0 &&
                        (media.ifm_status & IFM_AVALID) && (media.ifm_status & IFM_ACTIVE)) {
                        if (IFM_TYPE(media.ifm_active) == IFM_ETHER) {
                            if (media.ifm_active & IFM_FDX)
                                ifm->duplex = FREEBSD_DUPLEX_FULL;
                            else if (media.ifm_active & IFM_HDX)
                                ifm->duplex = FREEBSD_DUPLEX_HALF;
                        }
                        if (if_data->ifi_baudrate > 0)
                            ifm->speed_mbit = (double)if_data->ifi_baudrate / 1000000.0;
                    }
                }

                if (ifm->do_bandwidth == CONFIG_BOOLEAN_YES || ifm->do_bandwidth == CONFIG_BOOLEAN_AUTO) {
                    if (unlikely(!ifm->st_bandwidth)) {
                        ifm->st_bandwidth = rrdset_create_localhost("net",
                                                                    ifa->ifa_name,
                                                                    NULL,
                                                                    "traffik",
                                                                    "net.net",
                                                                    "Bandwidth",
                                                                    "kilobits/s",
                                                                    "freebsd.plugin",
                                                                    "getifaddrs",
                                                                    NETDATA_CHART_PRIO_FIRST_NET_IFACE,
                                                                    update_every,
                                                                    RRDSET_TYPE_AREA
                        );

                        ifm->rd_bandwidth_in  = rrddim_add(ifm->st_bandwidth, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_bandwidth_out = rrddim_add(ifm->st_bandwidth, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                        freebsd_interface_add_labels(ifm->st_bandwidth, ifa->ifa_name, ifm->physical);
                    }

                    rrddim_set_by_pointer(ifm->st_bandwidth, ifm->rd_bandwidth_in,  IFA_DATA(ibytes));
                    rrddim_set_by_pointer(ifm->st_bandwidth, ifm->rd_bandwidth_out, IFA_DATA(obytes));
                    rrdset_done(ifm->st_bandwidth);

                    if (!ifm->chart_var_speed)
                        ifm->chart_var_speed = rrdvar_chart_variable_add_and_acquire(ifm->st_bandwidth, "nic_speed_max");
                    if (ifm->chart_var_speed)
                        rrdvar_chart_variable_set(ifm->st_bandwidth, ifm->chart_var_speed,
                                                  isnan(ifm->speed_mbit) ? NAN : ifm->speed_mbit * 1000.0);
                }

                if (ifm->do_packets == CONFIG_BOOLEAN_YES || ifm->do_packets == CONFIG_BOOLEAN_AUTO) {
                    if (unlikely(!ifm->st_packets)) {
                        ifm->st_packets = rrdset_create_localhost("net_packets",
                                                                  ifa->ifa_name,
                                                                  NULL,
                                                                  "packets",
                                                                  "net.packets",
                                                                  "Packets",
                                                                  "packets/s",
                                                                  "freebsd.plugin",
                                                                  "getifaddrs",
                                                                  NETDATA_CHART_PRIO_FIRST_NET_PACKETS,
                                                                  update_every,
                                                                  RRDSET_TYPE_LINE
                        );

                        ifm->rd_packets_in    = rrddim_add(ifm->st_packets, "received",           NULL,  1, 1,
                                                           RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_packets_out   = rrddim_add(ifm->st_packets, "sent",               NULL, -1, 1,
                                                           RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_packets_m_in  = rrddim_add(ifm->st_packets, "multicast_received", NULL,  1, 1,
                                                           RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_packets_m_out = rrddim_add(ifm->st_packets, "multicast_sent",     NULL, -1, 1,
                                                           RRD_ALGORITHM_INCREMENTAL);
                        freebsd_interface_add_labels(ifm->st_packets, ifa->ifa_name, ifm->physical);
                    }

                    rrddim_set_by_pointer(ifm->st_packets, ifm->rd_packets_in,    IFA_DATA(ipackets));
                    rrddim_set_by_pointer(ifm->st_packets, ifm->rd_packets_out,   IFA_DATA(opackets));
                    rrddim_set_by_pointer(ifm->st_packets, ifm->rd_packets_m_in,  IFA_DATA(imcasts));
                    rrddim_set_by_pointer(ifm->st_packets, ifm->rd_packets_m_out, IFA_DATA(omcasts));
                    rrdset_done(ifm->st_packets);
                }

                if (ifm->do_errors == CONFIG_BOOLEAN_YES || ifm->do_errors == CONFIG_BOOLEAN_AUTO) {
                    if (unlikely(!ifm->st_errors)) {
                        ifm->st_errors = rrdset_create_localhost("net_errors",
                                                                 ifa->ifa_name,
                                                                 NULL,
                                                                 "errors",
                                                                 "net.errors",
                                                                 "Interface Errors",
                                                                 "errors/s",
                                                                 "freebsd.plugin",
                                                                 "getifaddrs",
                                                                 NETDATA_CHART_PRIO_FIRST_NET_ERRORS,
                                                                 update_every,
                                                                 RRDSET_TYPE_LINE
                        );

                        ifm->rd_errors_in  = rrddim_add(ifm->st_errors, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_errors_out = rrddim_add(ifm->st_errors, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                        freebsd_interface_add_labels(ifm->st_errors, ifa->ifa_name, ifm->physical);
                    }

                    rrddim_set_by_pointer(ifm->st_errors, ifm->rd_errors_in,  IFA_DATA(ierrors));
                    rrddim_set_by_pointer(ifm->st_errors, ifm->rd_errors_out, IFA_DATA(oerrors));
                    rrdset_done(ifm->st_errors);
                }

                if (ifm->do_drops == CONFIG_BOOLEAN_YES || ifm->do_drops == CONFIG_BOOLEAN_AUTO) {
                    if (unlikely(!ifm->st_drops)) {
                        ifm->st_drops = rrdset_create_localhost("net_drops",
                                                                ifa->ifa_name,
                                                                NULL,
                                                                "drops",
                                                                "net.drops",
                                                                "Interface Drops",
                                                                "drops/s",
                                                                "freebsd.plugin",
                                                                "getifaddrs",
                                                                NETDATA_CHART_PRIO_FIRST_NET_DROPS,
                                                                update_every,
                                                                RRDSET_TYPE_LINE
                        );

                        ifm->rd_drops_in  = rrddim_add(ifm->st_drops, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
#if __FreeBSD__ >= 11
                        ifm->rd_drops_out = rrddim_add(ifm->st_drops, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
#endif
                        freebsd_interface_add_labels(ifm->st_drops, ifa->ifa_name, ifm->physical);
                    }

                    rrddim_set_by_pointer(ifm->st_drops, ifm->rd_drops_in,  IFA_DATA(iqdrops));
#if __FreeBSD__ >= 11
                    rrddim_set_by_pointer(ifm->st_drops, ifm->rd_drops_out, IFA_DATA(oqdrops));
#endif
                    rrdset_done(ifm->st_drops);
                }

                if (ifm->do_events == CONFIG_BOOLEAN_YES || ifm->do_events == CONFIG_BOOLEAN_AUTO) {
                    if (unlikely(!ifm->st_events)) {
                        ifm->st_events = rrdset_create_localhost("net_events",
                                                                 ifa->ifa_name,
                                                                 NULL,
                                                                 "errors",
                                                                 "net.events",
                                                                 "Network Interface Events",
                                                                 "events/s",
                                                                 "freebsd.plugin",
                                                                 "getifaddrs",
                                                                 NETDATA_CHART_PRIO_FIRST_NET_EVENTS,
                                                                 update_every,
                                                                 RRDSET_TYPE_LINE
                        );

                        ifm->rd_events_coll = rrddim_add(ifm->st_events, "collisions", NULL, -1, 1,
                                                         RRD_ALGORITHM_INCREMENTAL);
                        freebsd_interface_add_labels(ifm->st_events, ifa->ifa_name, ifm->physical);
                    }

                    rrddim_set_by_pointer(ifm->st_events, ifm->rd_events_coll, IFA_DATA(collisions));
                    rrdset_done(ifm->st_events);
                }

                if ((ifm->do_speed == CONFIG_BOOLEAN_YES || ifm->do_speed == CONFIG_BOOLEAN_AUTO) &&
                    !isnan(ifm->speed_mbit)) {
                    if (!ifm->st_speed) {
                        ifm->st_speed = rrdset_create_localhost("net_speed", ifa->ifa_name, NULL, "net", "net.speed",
                                                                "Interface Speed", "kilobits/s", "freebsd.plugin", "getifaddrs",
                                                                NETDATA_CHART_PRIO_FIRST_NET_IFACE + 7, update_every, RRDSET_TYPE_LINE);
                        ifm->rd_speed = rrddim_add(ifm->st_speed, "speed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        freebsd_interface_add_labels(ifm->st_speed, ifa->ifa_name, ifm->physical);
                    }
                    rrddim_set_by_pointer(ifm->st_speed, ifm->rd_speed, (collected_number)(ifm->speed_mbit * 1000.0));
                    rrdset_done(ifm->st_speed);
                }

                if (ifm->do_duplex == CONFIG_BOOLEAN_YES || ifm->do_duplex == CONFIG_BOOLEAN_AUTO) {
                    if (!ifm->st_duplex) {
                        ifm->st_duplex = rrdset_create_localhost("net_duplex", ifa->ifa_name, NULL, "net", "net.duplex",
                                                                 "Interface Duplex State", "state", "freebsd.plugin", "getifaddrs",
                                                                 NETDATA_CHART_PRIO_FIRST_NET_IFACE + 8, update_every, RRDSET_TYPE_LINE);
                        ifm->rd_duplex_full = rrddim_add(ifm->st_duplex, "full", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        ifm->rd_duplex_half = rrddim_add(ifm->st_duplex, "half", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        ifm->rd_duplex_unknown = rrddim_add(ifm->st_duplex, "unknown", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        freebsd_interface_add_labels(ifm->st_duplex, ifa->ifa_name, ifm->physical);
                    }
                    rrddim_set_by_pointer(ifm->st_duplex, ifm->rd_duplex_full, ifm->duplex == FREEBSD_DUPLEX_FULL);
                    rrddim_set_by_pointer(ifm->st_duplex, ifm->rd_duplex_half, ifm->duplex == FREEBSD_DUPLEX_HALF);
                    rrddim_set_by_pointer(ifm->st_duplex, ifm->rd_duplex_unknown, ifm->duplex == FREEBSD_DUPLEX_UNKNOWN);
                    rrdset_done(ifm->st_duplex);
                }

                if (ifm->do_operstate == CONFIG_BOOLEAN_YES || ifm->do_operstate == CONFIG_BOOLEAN_AUTO) {
                    if (!ifm->st_operstate) {
                        ifm->st_operstate = rrdset_create_localhost("net_operstate", ifa->ifa_name, NULL, "net", "net.operstate",
                                                                    "Interface Operational State", "state", "freebsd.plugin", "getifaddrs",
                                                                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 9, update_every, RRDSET_TYPE_LINE);
                        ifm->rd_operstate_up = rrddim_add(ifm->st_operstate, "up", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        ifm->rd_operstate_down = rrddim_add(ifm->st_operstate, "down", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        ifm->rd_operstate_unknown = rrddim_add(ifm->st_operstate, "unknown", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        freebsd_interface_add_labels(ifm->st_operstate, ifa->ifa_name, ifm->physical);
                    }
                    rrddim_set_by_pointer(ifm->st_operstate, ifm->rd_operstate_up, ifm->operstate == FREEBSD_INTERFACE_UP);
                    rrddim_set_by_pointer(ifm->st_operstate, ifm->rd_operstate_down, ifm->operstate == FREEBSD_INTERFACE_DOWN);
                    rrddim_set_by_pointer(ifm->st_operstate, ifm->rd_operstate_unknown, ifm->operstate == FREEBSD_INTERFACE_UNKNOWN);
                    rrdset_done(ifm->st_operstate);
                }

                if ((ifm->do_carrier == CONFIG_BOOLEAN_YES || ifm->do_carrier == CONFIG_BOOLEAN_AUTO) &&
                    ifm->link_state != FREEBSD_INTERFACE_UNKNOWN) {
                    if (!ifm->st_carrier) {
                        ifm->st_carrier = rrdset_create_localhost("net_carrier", ifa->ifa_name, NULL, "net", "net.carrier",
                                                                  "Interface Physical Link State", "state", "freebsd.plugin", "getifaddrs",
                                                                  NETDATA_CHART_PRIO_FIRST_NET_IFACE + 10, update_every, RRDSET_TYPE_LINE);
                        ifm->rd_carrier_up = rrddim_add(ifm->st_carrier, "up", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        ifm->rd_carrier_down = rrddim_add(ifm->st_carrier, "down", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        freebsd_interface_add_labels(ifm->st_carrier, ifa->ifa_name, ifm->physical);
                    }
                    rrddim_set_by_pointer(ifm->st_carrier, ifm->rd_carrier_up, ifm->link_state == FREEBSD_INTERFACE_UP);
                    rrddim_set_by_pointer(ifm->st_carrier, ifm->rd_carrier_down, ifm->link_state == FREEBSD_INTERFACE_DOWN);
                    rrdset_done(ifm->st_carrier);
                }

                if ((ifm->do_mtu == CONFIG_BOOLEAN_YES || ifm->do_mtu == CONFIG_BOOLEAN_AUTO) && ifm->mtu > 0) {
                    if (!ifm->st_mtu) {
                        ifm->st_mtu = rrdset_create_localhost("net_mtu", ifa->ifa_name, NULL, "net", "net.mtu",
                                                              "Interface MTU", "octets", "freebsd.plugin", "getifaddrs",
                                                              NETDATA_CHART_PRIO_FIRST_NET_IFACE + 11, update_every, RRDSET_TYPE_LINE);
                        ifm->rd_mtu = rrddim_add(ifm->st_mtu, "mtu", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        freebsd_interface_add_labels(ifm->st_mtu, ifa->ifa_name, ifm->physical);
                    }
                    rrddim_set_by_pointer(ifm->st_mtu, ifm->rd_mtu, ifm->mtu);
                    rrdset_done(ifm->st_mtu);
                }
            }

            network_interfaces_cleanup();
            netdata_mutex_unlock(&network_interfaces_mutex);

            freeifaddrs(ifap);
        }
    } else {
        collector_error("DISABLED: getifaddrs module");
        return 1;
    }

    return 0;
}
