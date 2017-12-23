#include "common.h"

#include <ifaddrs.h>

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

    struct cgroup_network_interface *next;
};

static struct cgroup_network_interface *network_interfaces_root = NULL, *network_interfaces_last_used = NULL;

static size_t network_interfaces_added = 0, network_interfaces_found = 0;

static void network_interface_free(struct cgroup_network_interface *ifm) {
    if (likely(ifm->st_bandwidth))
        rrdset_is_obsolete(ifm->st_bandwidth);
    if (likely(ifm->st_packets))
        rrdset_is_obsolete(ifm->st_packets);
    if (likely(ifm->st_errors))
        rrdset_is_obsolete(ifm->st_errors);
    if (likely(ifm->st_drops))
        rrdset_is_obsolete(ifm->st_drops);
    if (likely(ifm->st_events))
        rrdset_is_obsolete(ifm->st_events);

    network_interfaces_added--;
    freez(ifm->name);
    freez(ifm);
}

static void network_interfaces_cleanup() {
    if (likely(network_interfaces_found == network_interfaces_added)) return;

    struct cgroup_network_interface *ifm = network_interfaces_root, *last = NULL;
    while(ifm) {
        if (unlikely(!ifm->updated)) {
            // info("Removing network interface '%s', linked after '%s'", ifm->name, last?last->name:"ROOT");

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
    network_interfaces_added++;

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

#define DELAULT_EXLUDED_INTERFACES "lo*"
#define CONFIG_SECTION_GETIFADDRS "plugin:freebsd:getifaddrs"

    static int enable_new_interfaces = -1;
    static int do_bandwidth_ipv4 = -1, do_bandwidth_ipv6 = -1, do_bandwidth = -1, do_packets = -1,
            do_errors = -1, do_drops = -1, do_events = -1;
    static SIMPLE_PATTERN *excluded_interfaces = NULL;

    if (unlikely(enable_new_interfaces == -1)) {
        enable_new_interfaces = config_get_boolean_ondemand(CONFIG_SECTION_GETIFADDRS,
                                                              "enable new interfaces detected at runtime",
                                                              CONFIG_BOOLEAN_AUTO);

        do_bandwidth_ipv4 = config_get_boolean_ondemand(CONFIG_SECTION_GETIFADDRS, "total bandwidth for ipv4 interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_bandwidth_ipv6 = config_get_boolean_ondemand(CONFIG_SECTION_GETIFADDRS, "total bandwidth for ipv6 interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_bandwidth      = config_get_boolean_ondemand(CONFIG_SECTION_GETIFADDRS, "bandwidth for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_packets        = config_get_boolean_ondemand(CONFIG_SECTION_GETIFADDRS, "packets for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_errors         = config_get_boolean_ondemand(CONFIG_SECTION_GETIFADDRS, "errors for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_drops          = config_get_boolean_ondemand(CONFIG_SECTION_GETIFADDRS, "drops for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);
        do_events         = config_get_boolean_ondemand(CONFIG_SECTION_GETIFADDRS, "collisions for all interfaces",
                                                        CONFIG_BOOLEAN_AUTO);

        excluded_interfaces = simple_pattern_create(
                config_get(CONFIG_SECTION_GETIFADDRS, "disable by default interfaces matching", DELAULT_EXLUDED_INTERFACES)
                , NULL
                , SIMPLE_PATTERN_EXACT
        );
    }

    if (likely(do_bandwidth_ipv4 || do_bandwidth_ipv6 || do_bandwidth || do_packets || do_errors ||
               do_drops || do_events)) {
        struct ifaddrs *ifap;

        if (unlikely(getifaddrs(&ifap))) {
            error("FREEBSD: getifaddrs() failed");
            do_bandwidth_ipv4 = 0;
            error("DISABLED: system.ipv4 chart");
            do_bandwidth_ipv6 = 0;
            error("DISABLED: system.ipv6 chart");
            do_bandwidth = 0;
            error("DISABLED: net.* charts");
            do_packets = 0;
            error("DISABLED: net_packets.* charts");
            do_errors = 0;
            error("DISABLED: net_errors.* charts");
            do_drops = 0;
            error("DISABLED: net_drops.* charts");
            do_events = 0;
            error("DISABLED: net_events.* charts");
            error("DISABLED: getifaddrs module");
            return 1;
        } else {
#define IFA_DATA(s) (((struct if_data *)ifa->ifa_data)->ifi_ ## s)
            struct ifaddrs *ifa;
            struct iftot {
                u_long  ift_ibytes;
                u_long  ift_obytes;
            } iftot = {0, 0};

            // --------------------------------------------------------------------

            if (likely(do_bandwidth_ipv4)) {
                iftot.ift_ibytes = iftot.ift_obytes = 0;
                for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                    if (ifa->ifa_addr->sa_family != AF_INET)
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
                                                 "freebsd",
                                                 "getifaddrs",
                                                 500,
                                                 update_every,
                                                 RRDSET_TYPE_AREA
                    );

                    rd_in  = rrddim_add(st, "InOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st, "OutOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in,  iftot.ift_ibytes);
                rrddim_set_by_pointer(st, rd_out, iftot.ift_obytes);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if (likely(do_bandwidth_ipv6)) {
                iftot.ift_ibytes = iftot.ift_obytes = 0;
                for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                    if (ifa->ifa_addr->sa_family != AF_INET6)
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
                                                 "freebsd",
                                                 "getifaddrs",
                                                 500,
                                                 update_every,
                                                 RRDSET_TYPE_AREA
                    );

                    rd_in  = rrddim_add(st, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    rd_out = rrddim_add(st, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                } else
                    rrdset_next(st);

                rrddim_set_by_pointer(st, rd_in,  iftot.ift_ibytes);
                rrddim_set_by_pointer(st, rd_out, iftot.ift_obytes);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            network_interfaces_found = 0;

            for (ifa = ifap; ifa; ifa = ifa->ifa_next) {
                if (ifa->ifa_addr->sa_family != AF_LINK)
                    continue;

                struct cgroup_network_interface *ifm = get_network_interface(ifa->ifa_name);
                ifm->updated = 1;
                network_interfaces_found++;

                if (unlikely(!ifm->configured)) {
                    char var_name[4096 + 1];

                    // this is the first time we see this network interface

                    // remember we configured it
                    ifm->configured = 1;

                    ifm->enabled = enable_new_interfaces;

                    if (likely(ifm->enabled))
                        ifm->enabled = !simple_pattern_matches(excluded_interfaces, ifa->ifa_name);

                    snprintfz(var_name, 4096, "%s:%s", CONFIG_SECTION_GETIFADDRS, ifa->ifa_name);
                    ifm->enabled = config_get_boolean_ondemand(var_name, "enabled", ifm->enabled);

                    if (unlikely(ifm->enabled == CONFIG_BOOLEAN_NO))
                        continue;

                    ifm->do_bandwidth = config_get_boolean_ondemand(var_name, "bandwidth", do_bandwidth);
                    ifm->do_packets   = config_get_boolean_ondemand(var_name, "packets",   do_packets);
                    ifm->do_errors    = config_get_boolean_ondemand(var_name, "errors",    do_errors);
                    ifm->do_drops     = config_get_boolean_ondemand(var_name, "drops",     do_drops);
                    ifm->do_events    = config_get_boolean_ondemand(var_name, "events",    do_events);
                }

                if (unlikely(!ifm->enabled))
                    continue;

                // --------------------------------------------------------------------

                if (ifm->do_bandwidth == CONFIG_BOOLEAN_YES || (ifm->do_bandwidth == CONFIG_BOOLEAN_AUTO &&
                                                                (IFA_DATA(ibytes) || IFA_DATA(obytes)))) {
                    if (unlikely(!ifm->st_bandwidth)) {
                        ifm->st_bandwidth = rrdset_create_localhost("net",
                                                                    ifa->ifa_name,
                                                                    NULL,
                                                                    ifa->ifa_name,
                                                                    "net.net",
                                                                    "Bandwidth",
                                                                    "kilobits/s",
                                                                    "freebsd",
                                                                    "getifaddrs",
                                                                    7000,
                                                                    update_every,
                                                                    RRDSET_TYPE_AREA
                        );

                        ifm->rd_bandwidth_in  = rrddim_add(ifm->st_bandwidth, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_bandwidth_out = rrddim_add(ifm->st_bandwidth, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                    } else
                        rrdset_next(ifm->st_bandwidth);

                    rrddim_set_by_pointer(ifm->st_bandwidth, ifm->rd_bandwidth_in,  IFA_DATA(ibytes));
                    rrddim_set_by_pointer(ifm->st_bandwidth, ifm->rd_bandwidth_out, IFA_DATA(obytes));
                    rrdset_done(ifm->st_bandwidth);
                }

                // --------------------------------------------------------------------

                if (ifm->do_packets == CONFIG_BOOLEAN_YES || (ifm->do_packets == CONFIG_BOOLEAN_AUTO &&
                                                              (IFA_DATA(ipackets) || IFA_DATA(opackets) || IFA_DATA(imcasts) || IFA_DATA(omcasts)))) {
                    if (unlikely(!ifm->st_packets)) {
                        ifm->st_packets = rrdset_create_localhost("net_packets",
                                                                  ifa->ifa_name,
                                                                  NULL,
                                                                  ifa->ifa_name,
                                                                  "net.packets",
                                                                  "Packets",
                                                                  "packets/s",
                                                                  "freebsd",
                                                                  "getifaddrs",
                                                                  7001,
                                                                  update_every,
                                                                  RRDSET_TYPE_LINE
                        );

                        rrdset_flag_set(ifm->st_packets, RRDSET_FLAG_DETAIL);

                        ifm->rd_packets_in    = rrddim_add(ifm->st_packets, "received",           NULL,  1, 1,
                                                           RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_packets_out   = rrddim_add(ifm->st_packets, "sent",               NULL, -1, 1,
                                                           RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_packets_m_in  = rrddim_add(ifm->st_packets, "multicast_received", NULL,  1, 1,
                                                           RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_packets_m_out = rrddim_add(ifm->st_packets, "multicast_sent",     NULL, -1, 1,
                                                           RRD_ALGORITHM_INCREMENTAL);
                    } else
                        rrdset_next(ifm->st_packets);

                    rrddim_set_by_pointer(ifm->st_packets, ifm->rd_packets_in,    IFA_DATA(ipackets));
                    rrddim_set_by_pointer(ifm->st_packets, ifm->rd_packets_out,   IFA_DATA(opackets));
                    rrddim_set_by_pointer(ifm->st_packets, ifm->rd_packets_m_in,  IFA_DATA(imcasts));
                    rrddim_set_by_pointer(ifm->st_packets, ifm->rd_packets_m_out, IFA_DATA(omcasts));
                    rrdset_done(ifm->st_packets);
                }

                // --------------------------------------------------------------------

                if (ifm->do_errors == CONFIG_BOOLEAN_YES || (ifm->do_errors == CONFIG_BOOLEAN_AUTO &&
                                                             (IFA_DATA(ierrors) || IFA_DATA(oerrors)))) {
                    if (unlikely(!ifm->st_errors)) {
                        ifm->st_errors = rrdset_create_localhost("net_errors",
                                                                 ifa->ifa_name,
                                                                 NULL,
                                                                 ifa->ifa_name,
                                                                 "net.errors",
                                                                 "Interface Errors",
                                                                 "errors/s",
                                                                 "freebsd",
                                                                 "getifaddrs",
                                                                 7002,
                                                                 update_every,
                                                                 RRDSET_TYPE_LINE
                        );

                        rrdset_flag_set(ifm->st_errors, RRDSET_FLAG_DETAIL);

                        ifm->rd_errors_in  = rrddim_add(ifm->st_errors, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        ifm->rd_errors_out = rrddim_add(ifm->st_errors, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    } else
                        rrdset_next(ifm->st_errors);

                    rrddim_set_by_pointer(ifm->st_errors, ifm->rd_errors_in,  IFA_DATA(ierrors));
                    rrddim_set_by_pointer(ifm->st_errors, ifm->rd_errors_out, IFA_DATA(oerrors));
                    rrdset_done(ifm->st_errors);
                }
                // --------------------------------------------------------------------

                if (ifm->do_drops == CONFIG_BOOLEAN_YES || (ifm->do_drops == CONFIG_BOOLEAN_AUTO &&
                                                            (IFA_DATA(iqdrops)
                                                             #if __FreeBSD__ >= 11
                                                             || IFA_DATA(oqdrops)
#endif
                ))) {
                    if (unlikely(!ifm->st_drops)) {
                        ifm->st_drops = rrdset_create_localhost("net_drops",
                                                                ifa->ifa_name,
                                                                NULL,
                                                                ifa->ifa_name,
                                                                "net.drops",
                                                                "Interface Drops",
                                                                "drops/s",
                                                                "freebsd",
                                                                "getifaddrs",
                                                                7003,
                                                                update_every,
                                                                RRDSET_TYPE_LINE
                        );

                        rrdset_flag_set(ifm->st_drops, RRDSET_FLAG_DETAIL);

                        ifm->rd_drops_in  = rrddim_add(ifm->st_drops, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
#if __FreeBSD__ >= 11
                        ifm->rd_drops_out = rrddim_add(ifm->st_drops, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
#endif
                    } else
                        rrdset_next(ifm->st_drops);

                    rrddim_set_by_pointer(ifm->st_drops, ifm->rd_drops_in,  IFA_DATA(iqdrops));
#if __FreeBSD__ >= 11
                    rrddim_set_by_pointer(ifm->st_drops, ifm->rd_drops_out, IFA_DATA(oqdrops));
#endif
                    rrdset_done(ifm->st_drops);
                }

                // --------------------------------------------------------------------

                if (ifm->do_events == CONFIG_BOOLEAN_YES || (ifm->do_events == CONFIG_BOOLEAN_AUTO &&
                                                             IFA_DATA(collisions))) {
                    if (unlikely(!ifm->st_events)) {
                        ifm->st_events = rrdset_create_localhost("net_events",
                                                                 ifa->ifa_name,
                                                                 NULL,
                                                                 ifa->ifa_name,
                                                                 "net.events",
                                                                 "Network Interface Events",
                                                                 "events/s",
                                                                 "freebsd",
                                                                 "getifaddrs",
                                                                 7006,
                                                                 update_every,
                                                                 RRDSET_TYPE_LINE
                        );

                        rrdset_flag_set(ifm->st_events, RRDSET_FLAG_DETAIL);

                        ifm->rd_events_coll = rrddim_add(ifm->st_events, "collisions", NULL, -1, 1,
                                                         RRD_ALGORITHM_INCREMENTAL);
                    } else
                        rrdset_next(ifm->st_events);

                    rrddim_set_by_pointer(ifm->st_events, ifm->rd_events_coll, IFA_DATA(collisions));
                    rrdset_done(ifm->st_events);
                }
            }

            freeifaddrs(ifap);
        }
    } else {
        error("DISABLED: getifaddrs module");
        return 1;
    }

    network_interfaces_cleanup();

    return 0;
}
