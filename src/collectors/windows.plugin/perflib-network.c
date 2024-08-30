// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// --------------------------------------------------------------------------------------------------------------------
// network protocols

struct network_protocol {
    const char *protocol;

    struct {
        COUNTER_DATA received;
        COUNTER_DATA sent;
        COUNTER_DATA delivered;
        COUNTER_DATA forwarded;
        RRDSET *st;
        RRDDIM *rd_received;
        RRDDIM *rd_sent;
        RRDDIM *rd_forwarded;
        RRDDIM *rd_delivered;
        const char *type;
        const char *id;
        const char *family;
        const char *context;
        const char *title;
        long priority;
    } packets;

} networks[] = {
    {
        .protocol = "IPv4",
        .packets = {
            .received = { .key = "Datagrams Received/sec" },
            .sent = { .key = "Datagrams Sent/sec" },
            .delivered = { .key = "Datagrams Received Delivered/sec" },
            .forwarded = { .key = "Datagrams Forwarded/sec" },
            .type = "ipv4",
            .id = "packets",
            .family = "packets",
            .context = "ipv4.packets",
            .title = "IPv4 Packets",
            .priority = NETDATA_CHART_PRIO_IPV4_PACKETS,
        },
    },
    {
        .protocol = "IPv6",
        .packets = {
            .received = { .key = "Datagrams Received/sec" },
            .sent = { .key = "Datagrams Sent/sec" },
            .delivered = { .key = "Datagrams Received Delivered/sec" },
            .forwarded = { .key = "Datagrams Forwarded/sec" },
            .type = "ipv6",
            .id = "packets",
            .family = "packets",
            .context = "ip6.packets",
            .title = "IPv6 Packets",
            .priority = NETDATA_CHART_PRIO_IPV6_PACKETS,
        },
    },
    {
        .protocol = "TCPv4",
        .packets = {
            .received = { .key = "Segments Received/sec" },
            .sent = { .key = "Segments Sent/sec" },
            .type = "ipv4",
            .id = "tcppackets",
            .family = "tcp",
            .context = "ipv4.tcppackets",
            .title = "IPv4 TCP Packets",
            .priority = NETDATA_CHART_PRIO_IPV4_TCP_PACKETS,
        },
    },
    {
        .protocol = "TCPv6",
        .packets = {
            .received = { .key = "Segments Received/sec" },
            .sent = { .key = "Segments Sent/sec" },
            .type = "ipv6",
            .id = "tcppackets",
            .family = "tcp6",
            .context = "ipv6.tcppackets",
            .title = "IPv6 TCP Packets",
            .priority = NETDATA_CHART_PRIO_IPV6_TCP_PACKETS,
        },
    },
    {
        .protocol = "UDPv4",
        .packets = {
            .received = { .key = "Datagrams Received/sec" },
            .sent = { .key = "Datagrams Sent/sec" },
            .type = "ipv4",
            .id = "udppackets",
            .family = "udp",
            .context = "ipv4.udppackets",
            .title = "IPv4 UDP Packets",
            .priority = NETDATA_CHART_PRIO_IPV4_UDP_PACKETS,
        },
    },
    {
        .protocol = "UDPv6",
        .packets = {
            .received = { .key = "Datagrams Received/sec" },
            .sent = { .key = "Datagrams Sent/sec" },
            .type = "ipv6",
            .id = "udppackets",
            .family = "udp6",
            .context = "ipv6.udppackets",
            .title = "IPv6 UDP Packets",
            .priority = NETDATA_CHART_PRIO_IPV6_UDP_PACKETS,
        },
    },
    {
        .protocol = "ICMP",
        .packets = {
            .received = { .key = "Messages Received/sec" },
            .sent = { .key = "Messages Sent/sec" },
            .type = "ipv4",
            .id = "icmp",
            .family = "icmp",
            .context = "ipv4.icmp",
            .title = "IPv4 ICMP Packets",
            .priority = NETDATA_CHART_PRIO_IPV4_ICMP_PACKETS,
        },
    },
    {
        .protocol = "ICMPv6",
        .packets = {
            .received = { .key = "Messages Received/sec" },
            .sent = { .key = "Messages Sent/sec" },
            .type = "ipv6",
            .id = "icmp",
            .family = "icmp6",
            .context = "ipv6.icmp",
            .title = "IPv6 ICMP Packets",
            .priority = NETDATA_CHART_PRIO_IPV6_ICMP_PACKETS,
        },
    },

    // terminator
    {
        .protocol = NULL,
    }
};

struct network_protocol tcp46 = {
    .packets = {
        .type = "ip",
        .id = "tcppackets",
        .family = "tcp",
        .context = "ip.tcppackets",
        .title = "TCP Packets",
        .priority = NETDATA_CHART_PRIO_IP_TCP_PACKETS,
    }
};

static void protocol_packets_chart_update(struct network_protocol *p, int update_every) {
    if(!p->packets.st) {
        p->packets.st = rrdset_create_localhost(
            p->packets.type
            , p->packets.id
            , NULL
            , p->packets.family
            , NULL
            , p->packets.title
            , "packets/s"
            , PLUGIN_WINDOWS_NAME
            , "PerflibNetwork"
            , p->packets.priority
            , update_every
            , RRDSET_TYPE_AREA
        );

        p->packets.rd_received  = rrddim_add(p->packets.st, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        p->packets.rd_sent      = rrddim_add(p->packets.st, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

        if(p->packets.forwarded.key)
            p->packets.rd_forwarded = rrddim_add(p->packets.st, "forwarded", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

        if(p->packets.delivered.key)
            p->packets.rd_delivered = rrddim_add(p->packets.st, "delivered", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    if(p->packets.received.updated)
        rrddim_set_by_pointer(p->packets.st, p->packets.rd_received,  (collected_number)p->packets.received.current.Data);

    if(p->packets.sent.updated)
        rrddim_set_by_pointer(p->packets.st, p->packets.rd_sent,      (collected_number)p->packets.sent.current.Data);

    if(p->packets.forwarded.key && p->packets.forwarded.updated)
        rrddim_set_by_pointer(p->packets.st, p->packets.rd_forwarded, (collected_number)p->packets.forwarded.current.Data);

    if(p->packets.delivered.key && p->packets.delivered.updated)
        rrddim_set_by_pointer(p->packets.st, p->packets.rd_delivered, (collected_number)p->packets.delivered.current.Data);

    rrdset_done(p->packets.st);
}

static bool do_network_protocol(PERF_DATA_BLOCK *pDataBlock, int update_every, struct network_protocol *p) {
    if(!p || !p->protocol) return false;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, p->protocol);
    if(!pObjectType) return false;

    size_t packets = 0;
    if(p->packets.received.key)
        packets += perflibGetObjectCounter(pDataBlock, pObjectType, &p->packets.received) ? 1 : 0;

    if(p->packets.sent.key)
        packets += perflibGetObjectCounter(pDataBlock, pObjectType, &p->packets.sent) ? 1 : 0;

    if(p->packets.delivered.key)
        packets += perflibGetObjectCounter(pDataBlock, pObjectType, &p->packets.delivered) ? 1 :0;

    if(p->packets.forwarded.key)
        packets += perflibGetObjectCounter(pDataBlock, pObjectType, &p->packets.forwarded) ? 1 : 0;

    if(packets)
        protocol_packets_chart_update(p, update_every);

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// network interfaces

struct network_interface {
    bool collected_metadata;

    struct {
        COUNTER_DATA received;
        COUNTER_DATA sent;

        RRDSET *st;
        RRDDIM *rd_received;
        RRDDIM *rd_sent;
    } packets;

    struct {
        COUNTER_DATA received;
        COUNTER_DATA sent;

        RRDSET *st;
        RRDDIM *rd_received;
        RRDDIM *rd_sent;
    } traffic;
};

static DICTIONARY *physical_interfaces = NULL, *virtual_interfaces = NULL;

static void network_interface_init(struct network_interface *ni) {
    ni->packets.received.key = "Packets Received/sec";
    ni->packets.sent.key = "Packets Sent/sec";

    ni->traffic.received.key = "Bytes Received/sec";
    ni->traffic.sent.key = "Bytes Sent/sec";
}

void dict_interface_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct network_interface *ni = value;
    network_interface_init(ni);
}

static void initialize(void) {
    physical_interfaces = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE |
                                                         DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct network_interface));

    virtual_interfaces = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE |
                                                        DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct network_interface));

    dictionary_register_insert_callback(physical_interfaces, dict_interface_insert_cb, NULL);
    dictionary_register_insert_callback(virtual_interfaces, dict_interface_insert_cb, NULL);
}

static void add_interface_labels(RRDSET *st, const char *name, bool physical) {
    rrdlabels_add(st->rrdlabels, "device", name, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "interface_type", physical ? "real" : "virtual", RRDLABEL_SRC_AUTO);
}

static bool is_physical_interface(const char *name) {
    void *d = dictionary_get(physical_interfaces, name);
    return d ? true : false;
}

static bool do_network_interface(PERF_DATA_BLOCK *pDataBlock, int update_every, bool physical) {
    DICTIONARY *dict = physical_interfaces;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, physical ? "Network Interface" : "Network Adapter");
    if(!pObjectType) return false;

    uint64_t total_received = 0, total_sent = 0;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if(!pi) break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        if(!physical && is_physical_interface(windows_shared_buffer))
            // this virtual interface is already reported as physical interface
            continue;

        struct network_interface *d = dictionary_set(dict, windows_shared_buffer, NULL, sizeof(*d));

        if(!d->collected_metadata) {
            // TODO - get metadata about the network interface
            d->collected_metadata = true;
        }

        if(perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->traffic.received) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->traffic.sent)) {

            if(d->traffic.received.current.Data == 0 && d->traffic.sent.current.Data == 0)
                // this interface has not received or sent any traffic
                continue;

            if (unlikely(!d->traffic.st)) {
                d->traffic.st = rrdset_create_localhost(
                    "net",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.net",
                    "Bandwidth",
                    "kilobits/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE,
                    update_every,
                    RRDSET_TYPE_AREA);

                rrdset_flag_set(d->traffic.st, RRDSET_FLAG_DETAIL);

                add_interface_labels(d->traffic.st, windows_shared_buffer, physical);

                d->traffic.rd_received = rrddim_add(d->traffic.st, "received", NULL, 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                d->traffic.rd_sent = rrddim_add(d->traffic.st, "sent", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            }

            total_received += d->traffic.received.current.Data;
            total_sent += d->traffic.sent.current.Data;

            rrddim_set_by_pointer(d->traffic.st, d->traffic.rd_received, (collected_number)d->traffic.received.current.Data);
            rrddim_set_by_pointer(d->traffic.st, d->traffic.rd_sent, (collected_number)d->traffic.sent.current.Data);
            rrdset_done(d->traffic.st);
        }

        if(perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->packets.received) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->packets.sent)) {

            if (unlikely(!d->packets.st)) {
                d->packets.st = rrdset_create_localhost(
                    "net_packets",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.packets",
                    "Packets",
                    "packets/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 1,
                    update_every,
                    RRDSET_TYPE_LINE);

                rrdset_flag_set(d->packets.st, RRDSET_FLAG_DETAIL);

                add_interface_labels(d->traffic.st, windows_shared_buffer, physical);

                d->packets.rd_received = rrddim_add(d->packets.st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->packets.rd_sent = rrddim_add(d->packets.st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(d->packets.st, d->packets.rd_received, (collected_number)d->packets.received.current.Data);
            rrddim_set_by_pointer(d->packets.st, d->packets.rd_sent, (collected_number)d->packets.sent.current.Data);
            rrdset_done(d->packets.st);
        }
    }

    if(physical) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL, *rd_sent = NULL;

        if (unlikely(!st)) {
            st = rrdset_create_localhost(
                "system",
                "net",
                NULL,
                "network",
                "system.net",
                "Physical Network Interfaces Aggregated Bandwidth",
                "kilobits/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibNetwork",
                NETDATA_CHART_PRIO_SYSTEM_NET,
                update_every,
                RRDSET_TYPE_AREA);

            rd_received = rrddim_add(st, "received", NULL, 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_sent = rrddim_add(st, "sent", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_received, (collected_number)total_received);
        rrddim_set_by_pointer(st, rd_sent, (collected_number)total_sent);
        rrdset_done(st);
    }

    return true;
}

int do_PerflibNetwork(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Network Interface");
    if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    do_network_interface(pDataBlock, update_every, true);
    do_network_interface(pDataBlock, update_every, false);

    struct network_protocol *tcp4 = NULL, *tcp6 = NULL;
    for(size_t i = 0; networks[i].protocol ;i++) {
        do_network_protocol(pDataBlock, update_every, &networks[i]);

        if(!tcp4 && strcmp(networks[i].protocol, "TCPv4") == 0)
            tcp4 = &networks[i];
        if(!tcp6 && strcmp(networks[i].protocol, "TCPv6") == 0)
            tcp6 = &networks[i];
    }

    if(tcp4 && tcp6) {
        tcp46.packets.received = tcp4->packets.received;
        tcp46.packets.sent = tcp4->packets.sent;
        tcp46.packets.received.current.Data += tcp6->packets.received.current.Data;
        tcp46.packets.sent.current.Data += tcp6->packets.sent.current.Data;
        protocol_packets_chart_update(&tcp46, update_every);
    }

    return 0;
}
