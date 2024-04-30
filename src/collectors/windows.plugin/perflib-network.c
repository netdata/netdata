// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

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

static void initialize(void) {
    ;
}

static bool do_network_interface(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    ;
    return true;
}

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

int do_PerflibNetwork(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Network Interface");
    if(id == REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    do_network_interface(pDataBlock, update_every);

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
