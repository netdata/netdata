// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, counter)                                                \
    do {                                                                                                               \
        if ((p)->packets.counter.key) {                                                                                \
            packets += perflibGetObjectCounter((pDataBlock), (pObjectType), &(p)->packets.counter) ? 1 : 0;            \
        }                                                                                                              \
    } while (0)

#define SET_DIM_IF_KEY_AND_UPDATED(p, field)                                                                           \
    do {                                                                                                               \
        if ((p)->packets.field.key && (p)->packets.field.updated) {                                                    \
            rrddim_set_by_pointer(                                                                                     \
                (p)->packets.st, (p)->packets.rd_##field, (collected_number)(p)->packets.field.current.Data);          \
        }                                                                                                              \
    } while (0)

#define ADD_RRD_DIM_IF_KEY(packet_field, id, name, multiplier, algorithm)                                              \
    do {                                                                                                               \
        if (p->packets.packet_field.key)                                                                               \
            p->packets.rd_##packet_field = rrddim_add(st, id, name, multiplier, 1, algorithm);                         \
    } while (0)

// --------------------------------------------------------------------------------------------------------------------
// network protocols

struct network_protocol {
    const char *protocol;

    struct {
        COUNTER_DATA received;
        COUNTER_DATA sent;
        COUNTER_DATA delivered;
        COUNTER_DATA forwarded;

        COUNTER_DATA InDiscards;
        COUNTER_DATA OutDiscards;
        COUNTER_DATA InHdrErrors;
        COUNTER_DATA InAddrErrors;
        COUNTER_DATA InUnknownProtos;
        COUNTER_DATA InTooBigErrors;
        COUNTER_DATA InTruncatedPkts;
        COUNTER_DATA InNoRoutes;
        COUNTER_DATA OutNoRoutes;

        COUNTER_DATA InEchoReps;
        COUNTER_DATA OutEchoReps;
        COUNTER_DATA InDestUnreachs;
        COUNTER_DATA OutDestUnreachs;
        COUNTER_DATA InRedirects;
        COUNTER_DATA OutRedirects;
        COUNTER_DATA InEchos;
        COUNTER_DATA OutEchos;
        COUNTER_DATA InRouterAdvert;
        COUNTER_DATA OutRouterAdvert;
        COUNTER_DATA InRouterSelect;
        COUNTER_DATA OutRouterSelect;
        COUNTER_DATA InTimeExcds;
        COUNTER_DATA OutTimeExcds;
        COUNTER_DATA InParmProbs;
        COUNTER_DATA OutParmProbs;
        COUNTER_DATA InTimestamps;
        COUNTER_DATA OutTimestamps;
        COUNTER_DATA InTimestampReps;
        COUNTER_DATA OutTimestampReps;

        RRDSET *st;
        RRDDIM *rd_received;
        RRDDIM *rd_sent;
        RRDDIM *rd_forwarded;
        RRDDIM *rd_delivered;

        RRDDIM *rd_InDiscards;
        RRDDIM *rd_OutDiscards;
        RRDDIM *rd_InHdrErrors;
        RRDDIM *rd_InAddrErrors;
        RRDDIM *rd_InUnknownProtos;
        RRDDIM *rd_InTooBigErrors;
        RRDDIM *rd_InTruncatedPkts;
        RRDDIM *rd_InNoRoutes;
        RRDDIM *rd_OutNoRoutes;

        RRDDIM *rd_InEchoReps;
        RRDDIM *rd_OutEchoReps;
        RRDDIM *rd_InDestUnreachs;
        RRDDIM *rd_OutDestUnreachs;
        RRDDIM *rd_InRedirects;
        RRDDIM *rd_OutRedirects;
        RRDDIM *rd_InEchos;
        RRDDIM *rd_OutEchos;
        RRDDIM *rd_InRouterAdvert;
        RRDDIM *rd_OutRouterAdvert;
        RRDDIM *rd_InRouterSelect;
        RRDDIM *rd_OutRouterSelect;
        RRDDIM *rd_InTimeExcds;
        RRDDIM *rd_OutTimeExcds;
        RRDDIM *rd_InParmProbs;
        RRDDIM *rd_OutParmProbs;
        RRDDIM *rd_InTimestamps;
        RRDDIM *rd_OutTimestamps;
        RRDDIM *rd_InTimestampReps;
        RRDDIM *rd_OutTimestampReps;

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
        .packets =
            {
                .received = {.key = "Datagrams Received/sec"},
                .sent = {.key = "Datagrams Sent/sec"},
                .delivered = {.key = "Datagrams Received Delivered/sec"},
                .forwarded = {.key = "Datagrams Forwarded/sec"},
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
        .packets =
            {
                .received = {.key = "Datagrams Received/sec"},
                .sent = {.key = "Datagrams Sent/sec"},
                .delivered = {.key = "Datagrams Received Delivered/sec"},
                .forwarded = {.key = "Datagrams Forwarded/sec"},
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
        .packets =
            {
                .received = {.key = "Segments Received/sec"},
                .sent = {.key = "Segments Sent/sec"},
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
        .packets =
            {
                .received = {.key = "Segments Received/sec"},
                .sent = {.key = "Segments Sent/sec"},
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
        .packets =
            {
                .received = {.key = "Datagrams Received/sec"},
                .sent = {.key = "Datagrams Sent/sec"},
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
        .packets =
            {
                .received = {.key = "Datagrams Received/sec"},
                .sent = {.key = "Datagrams Sent/sec"},
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
        .packets =
            {
                .received = {.key = "Messages Received/sec"},
                .sent = {.key = "Messages Sent/sec"},
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
        .packets =
            {
                .received = {.key = "Messages Received/sec"},
                .sent = {.key = "Messages Sent/sec"},
                .type = "ipv6",
                .id = "icmp",
                .family = "icmp6",
                .context = "ipv6.icmp",
                .title = "IPv6 ICMP Packets",
                .priority = NETDATA_CHART_PRIO_IPV6_ICMP_PACKETS,
            },
    },

    {
        .protocol = "IPv4",
        .packets =
            {
                .InDiscards = {.key = "Datagrams Received Discarded"},
                .OutDiscards = {.key = "Datagrams Outbound Discarded"},
                .OutNoRoutes = {.key = "Datagrams Outbound No Route"},
                .InAddrErrors = {.key = "Datagrams Received Address Errors"},
                .InHdrErrors = {.key = "Datagrams Received Header Errors"},
                .InUnknownProtos = {.key = "Datagrams Received Unknown Protocol"},
                .type = "ipv4",
                .id = "errors",
                .family = "errors",
                .context = "ipv4.errors",
                .title = "IPv4 errors",
                .priority = NETDATA_CHART_PRIO_IPV4_ERRORS,
            },
    },
    {
        .protocol = "IPv6",
        .packets =
            {
                .InDiscards = {.key = "Datagrams Received Discarded"},
                .OutDiscards = {.key = "Datagrams Outbound Discarded"},
                .OutNoRoutes = {.key = "Datagrams Outbound No Route"},
                .InAddrErrors = {.key = "Datagrams Received Address Errors"},
                .InHdrErrors = {.key = "Datagrams Received Header Errors"},
                .InUnknownProtos = {.key = "Datagrams Received Unknown Protocol"},
                .type = "ipv6",
                .id = "errors",
                .family = "errors",
                .context = "ipv6.errors",
                .title = "IPv6 errors",
                .priority = NETDATA_CHART_PRIO_IPV6_ERRORS,
            },
    },
    {
        .protocol = "ICMP",
        .packets =
            {
                .InEchoReps = {.key = "Received Echo Reply/sec"},
                .OutEchoReps = {.key = "Sent Echo Reply/sec"},
                .InDestUnreachs = {.key = "Received Dest. Unreachable"},
                .OutDestUnreachs = {.key = "Sent Destination Unreachable"},
                .InRedirects = {.key = "Received Redirect/sec"},
                .OutRedirects = {.key = "Sent Redirect/sec"},
                .InEchos = {.key = "Received Echo/sec"},
                .OutEchos = {.key = "Sent Echo/sec"},
                .InRouterAdvert = {.key = NULL},
                .OutRouterAdvert = {.key = NULL},
                .InRouterSelect = {.key = NULL},
                .OutRouterSelect = {.key = NULL},
                .InTimeExcds = {.key = "Received Time Exceeded"},
                .OutTimeExcds = {.key = "Sent Time Exceeded"},
                .InParmProbs = {.key = "Received Parameter Problem"},
                .OutParmProbs = {.key = "Sent Parameter Problem"},
                .InTimestamps = {.key = "Received Timestamp/sec"},
                .OutTimestamps = {.key = "Sent Timestamp/sec"},
                .InTimestampReps = {.key = "Received Timestamp Reply/sec"},
                .OutTimestampReps = {.key = "Sent Timestamp Reply/sec"},

                .type = "ipv4",
                .id = "icmpmsg",
                .family = "icmp",
                .context = "ipv4.icmpmsg",
                .title = "IPv4 ICMP Packets",
                .priority = NETDATA_CHART_PRIO_IPV4_ICMP_MESSAGES,
            },
    },
    {
        .protocol = "ICMPv6",
        .packets =
            {
                .InEchoReps = {.key = "Received Echo Reply/sec"},
                .OutEchoReps = {.key = "Sent Echo Reply/sec"},
                .InDestUnreachs = {.key = "Received Dest. Unreachable"},
                .OutDestUnreachs = {.key = "Sent Destination Unreachable"},
                .InRedirects = {.key = "Received Redirect/sec"},
                .OutRedirects = {.key = "Sent Redirect/sec"},
                .InEchos = {.key = "Received Echo/sec"},
                .OutEchos = {.key = "Sent Echo/sec"},
                .InRouterAdvert = {.key = NULL},
                .OutRouterAdvert = {.key = NULL},
                .InRouterSelect = {.key = NULL},
                .OutRouterSelect = {.key = NULL},
                .InTimeExcds = {.key = "Received Time Exceeded"},
                .OutTimeExcds = {.key = "Sent Time Exceeded"},
                .InParmProbs = {.key = "Received Parameter Problem"},
                .OutParmProbs = {.key = "Sent Parameter Problem"},
                .InTimestamps = {.key = "Received Timestamp/sec"},
                .OutTimestamps = {.key = "Sent Timestamp/sec"},
                .InTimestampReps = {.key = "Received Timestamp Reply/sec"},
                .OutTimestampReps = {.key = "Sent Timestamp Reply/sec"},

                .type = "ipv6",
                .id = "icmpmsg",
                .family = "icmp",
                .context = "ipv6.icmpmsg",
                .title = "IPv6 ICMP Packets",
                .priority = NETDATA_CHART_PRIO_IPV6_ICMP_MESSAGES,
            },
    },

    // terminator
    {
        .protocol = NULL,
    }};

struct network_protocol tcp46 = {
    .packets = {
        .type = "ip",
        .id = "tcppackets",
        .family = "tcp",
        .context = "ip.tcppackets",
        .title = "TCP Packets",
        .priority = NETDATA_CHART_PRIO_IP_TCP_PACKETS,
    }};

static void protocol_packets_chart_update(struct network_protocol *p, int update_every)
{
    if (!p->packets.st) {
        p->packets.st = rrdset_create_localhost(
            p->packets.type,
            p->packets.id,
            NULL,
            p->packets.family,
            NULL,
            p->packets.title,
            "packets/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibNetwork",
            p->packets.priority,
            update_every,
            RRDSET_TYPE_AREA);

        RRDSET *st = p->packets.st;

        ADD_RRD_DIM_IF_KEY(received, "received", NULL, 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(sent, "sent", NULL, -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(forwarded, "forwarded", NULL, -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(delivered, "delivered", NULL, 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InDiscards, "InDiscards", NULL, 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutDiscards, "OutDiscards", NULL, -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InHdrErrors, "InHdrErrors", NULL, 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InAddrErrors, "InAddrErrors", NULL, 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InUnknownProtos, "InUnknownProtos", NULL, 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InTooBigErrors, "InTooBigErrors", NULL, 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InTruncatedPkts, "InTruncatedPkts", NULL, 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InNoRoutes, "InNoRoutes", NULL, 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutNoRoutes, "OutNoRoutes", NULL, -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InEchoReps, "InType0", "InEchoReps", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutEchoReps, "OutType0", "OutEchoReps", -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InDestUnreachs, "InType3", "InDestUnreachs", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutDestUnreachs, "OutType3", "OutDestUnreachs", -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InRedirects, "InType5", "InRedirects", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutRedirects, "OutType5", "OutRedirects", -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InEchos, "InType8", "InEchos", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutEchos, "OutType8", "OutEchos", -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InRouterAdvert, "InType9", "InRouterAdvert", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutRouterAdvert, "OutType9", "OutRouterAdvert", -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InRouterSelect, "InType10", "InRouterSelect", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutRouterSelect, "OutType10", "OutRouterSelect", -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InTimeExcds, "InType11", "InTimeExcds", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutTimeExcds, "OutType11", "OutTimeExcds", -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InParmProbs, "InType12", "InParmProbs", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutParmProbs, "OutType12", "OutParmProbs", -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InTimestamps, "InType13", "InTimestamps", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutTimestamps, "OutType13", "OutTimestamps", -1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(InTimestampReps, "InType14", "InTimestampReps", 1, RRD_ALGORITHM_INCREMENTAL);
        ADD_RRD_DIM_IF_KEY(OutTimestampReps, "OutType14", "OutTimestampReps", -1, RRD_ALGORITHM_INCREMENTAL);
    }

    SET_DIM_IF_KEY_AND_UPDATED(p, received);
    SET_DIM_IF_KEY_AND_UPDATED(p, sent);

    SET_DIM_IF_KEY_AND_UPDATED(p, forwarded);
    SET_DIM_IF_KEY_AND_UPDATED(p, delivered);
    SET_DIM_IF_KEY_AND_UPDATED(p, InDiscards);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutDiscards);
    SET_DIM_IF_KEY_AND_UPDATED(p, InHdrErrors);
    SET_DIM_IF_KEY_AND_UPDATED(p, InAddrErrors);
    SET_DIM_IF_KEY_AND_UPDATED(p, InUnknownProtos);
    SET_DIM_IF_KEY_AND_UPDATED(p, InTooBigErrors);
    SET_DIM_IF_KEY_AND_UPDATED(p, InTruncatedPkts);
    SET_DIM_IF_KEY_AND_UPDATED(p, InNoRoutes);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutNoRoutes);
    SET_DIM_IF_KEY_AND_UPDATED(p, InEchoReps);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutEchoReps);
    SET_DIM_IF_KEY_AND_UPDATED(p, InDestUnreachs);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutDestUnreachs);
    SET_DIM_IF_KEY_AND_UPDATED(p, InRedirects);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutRedirects);
    SET_DIM_IF_KEY_AND_UPDATED(p, InEchos);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutEchos);
    SET_DIM_IF_KEY_AND_UPDATED(p, InRouterAdvert);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutRouterAdvert);
    SET_DIM_IF_KEY_AND_UPDATED(p, InRouterSelect);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutRouterSelect);
    SET_DIM_IF_KEY_AND_UPDATED(p, InTimeExcds);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutTimeExcds);
    SET_DIM_IF_KEY_AND_UPDATED(p, InParmProbs);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutParmProbs);
    SET_DIM_IF_KEY_AND_UPDATED(p, InTimestamps);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutTimestamps);
    SET_DIM_IF_KEY_AND_UPDATED(p, InTimestampReps);
    SET_DIM_IF_KEY_AND_UPDATED(p, OutTimestampReps);

    rrdset_done(p->packets.st);
}

static bool do_network_protocol(PERF_DATA_BLOCK *pDataBlock, int update_every, struct network_protocol *p)
{
    if (!p || !p->protocol)
        return false;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, p->protocol);
    if (!pObjectType)
        return false;

    size_t packets = 0;
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, received);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, sent);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, delivered);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, forwarded);

    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InDiscards);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutDiscards);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InHdrErrors);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InAddrErrors);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InUnknownProtos);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InTooBigErrors);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InTruncatedPkts);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InNoRoutes);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutNoRoutes);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InEchoReps);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutEchoReps);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InDestUnreachs);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutDestUnreachs);

    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InRedirects);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutRedirects);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InEchos);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutEchos);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InRouterAdvert);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutRouterAdvert);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InRouterSelect);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutRouterSelect);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InTimeExcds);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutTimeExcds);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InParmProbs);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutParmProbs);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InTimestamps);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutTimestamps);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, InTimestampReps);
    ADD_PACKET_IF_KEY(p, packets, pDataBlock, pObjectType, OutTimestampReps);

    if (packets)
        protocol_packets_chart_update(p, update_every);

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// network interfaces

struct network_interface {
    usec_t last_collected;
    bool collected_metadata;

    struct {
        COUNTER_DATA received;
        COUNTER_DATA sent;

        RRDSET *st;
        RRDDIM *rd_received;
        RRDDIM *rd_sent;
    } packets;

    struct {
        const RRDVAR_ACQUIRED *chart_var_speed;

        COUNTER_DATA received;
        COUNTER_DATA sent;

        RRDSET *st;
        RRDDIM *rd_received;
        RRDDIM *rd_sent;
    } traffic;

    struct {
        COUNTER_DATA current_bandwidth;
        RRDSET *st;
        RRDDIM *rd;
    } speed;

    struct {
        COUNTER_DATA received;
        COUNTER_DATA outbound;

        RRDSET *st;
        RRDDIM *rd_received;
        RRDDIM *rd_outbound;
    } discards;

    struct {
        COUNTER_DATA received;
        COUNTER_DATA outbound;

        RRDSET *st;
        RRDDIM *rd_received;
        RRDDIM *rd_outbound;
    } errors;

    struct {
        COUNTER_DATA length;
        RRDSET *st;
        RRDDIM *rd;
    } queue;

    struct {
        COUNTER_DATA connections;
        RRDSET *st;
        RRDDIM *rd;
    } chimney;

    struct {
        COUNTER_DATA connections;
        COUNTER_DATA packets;
        COUNTER_DATA exceptions;
        COUNTER_DATA average_packet_size;

        RRDSET *st_connections;
        RRDDIM *rd_connections;

        RRDSET *st_packets;
        RRDDIM *rd_packets;

        RRDSET *st_exceptions;
        RRDDIM *rd_exceptions;

        RRDSET *st_average_packet_size;
        RRDDIM *rd_average_packet_size;
    } rsc;
};

static DICTIONARY *physical_interfaces = NULL, *virtual_interfaces = NULL;

static void network_interface_init(struct network_interface *d)
{
    d->packets.received.key = "Packets Received/sec";
    d->packets.sent.key = "Packets Sent/sec";
    d->traffic.received.key = "Bytes Received/sec";
    d->traffic.sent.key = "Bytes Sent/sec";
    d->speed.current_bandwidth.key = "Current Bandwidth";
    d->discards.received.key = "Packets Received Discarded";
    d->discards.outbound.key = "Packets Outbound Discarded";
    d->errors.received.key = "Packets Received Errors";
    d->errors.outbound.key = "Packets Outbound Errors";
    d->queue.length.key = "Output Queue Length";
    d->chimney.connections.key = "Offloaded Connections";
    d->rsc.connections.key = "TCP Active RSC Connections";
    d->rsc.packets.key = "TCP RSC Coalesced Packets/sec";
    d->rsc.exceptions.key = "TCP RSC Exceptions/sec";
    d->rsc.average_packet_size.key = "TCP RSC Average Packet Size";
}

static void network_interface_cleanup(struct network_interface *d)
{
    rrdvar_chart_variable_release(d->traffic.st, d->traffic.chart_var_speed);
    d->traffic.chart_var_speed = NULL;
    rrdset_is_obsolete___safe_from_collector_thread(d->packets.st);
    rrdset_is_obsolete___safe_from_collector_thread(d->traffic.st);
    rrdset_is_obsolete___safe_from_collector_thread(d->speed.st);
    rrdset_is_obsolete___safe_from_collector_thread(d->discards.st);
    rrdset_is_obsolete___safe_from_collector_thread(d->errors.st);
    rrdset_is_obsolete___safe_from_collector_thread(d->queue.st);
    rrdset_is_obsolete___safe_from_collector_thread(d->chimney.st);
    rrdset_is_obsolete___safe_from_collector_thread(d->rsc.st_connections);
    rrdset_is_obsolete___safe_from_collector_thread(d->rsc.st_packets);
    rrdset_is_obsolete___safe_from_collector_thread(d->rsc.st_exceptions);
    rrdset_is_obsolete___safe_from_collector_thread(d->rsc.st_average_packet_size);
}

void dict_interface_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct network_interface *ni = value;
    network_interface_init(ni);
}

static void initialize(void)
{
    physical_interfaces = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct network_interface));

    virtual_interfaces = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct network_interface));

    dictionary_register_insert_callback(physical_interfaces, dict_interface_insert_cb, NULL);
    dictionary_register_insert_callback(virtual_interfaces, dict_interface_insert_cb, NULL);
}

static void add_interface_labels(RRDSET *st, const char *name, bool physical)
{
    rrdlabels_add(st->rrdlabels, "device", name, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "interface_type", physical ? "real" : "virtual", RRDLABEL_SRC_AUTO);
}

static bool is_physical_interface(const char *name)
{
    void *d = dictionary_get(physical_interfaces, name);
    return d ? true : false;
}

static bool do_network_interface(PERF_DATA_BLOCK *pDataBlock, int update_every, bool physical, usec_t now_ut)
{
    DICTIONARY *dict = physical ? physical_interfaces : virtual_interfaces;

    PERF_OBJECT_TYPE *pObjectType =
        perflibFindObjectTypeByName(pDataBlock, physical ? "Network Interface" : "Network Adapter");
    if (!pObjectType)
        return false;

    uint64_t total_received = 0, total_sent = 0;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        if (!physical && is_physical_interface(windows_shared_buffer))
            // this virtual interface is already reported as physical interface
            continue;

        struct network_interface *d = dictionary_set(dict, windows_shared_buffer, NULL, sizeof(*d));
        d->last_collected = now_ut;

        if (!d->collected_metadata) {
            // TODO - get metadata about the network interface
            d->collected_metadata = true;
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->traffic.received) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->traffic.sent)) {
            if (d->traffic.received.current.Data == 0 && d->traffic.sent.current.Data == 0)
                // this interface has not received or sent any traffic yet
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

                add_interface_labels(d->traffic.st, windows_shared_buffer, physical);

                d->traffic.rd_received =
                    rrddim_add(d->traffic.st, "received", NULL, 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                d->traffic.rd_sent =
                    rrddim_add(d->traffic.st, "sent", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);

                d->traffic.chart_var_speed = rrdvar_chart_variable_add_and_acquire(d->traffic.st, "nic_speed_max");
                rrdvar_chart_variable_set(d->traffic.st, d->traffic.chart_var_speed, NAN);
            }

            total_received += d->traffic.received.current.Data;
            total_sent += d->traffic.sent.current.Data;

            rrddim_set_by_pointer(
                d->traffic.st, d->traffic.rd_received, (collected_number)d->traffic.received.current.Data);
            rrddim_set_by_pointer(d->traffic.st, d->traffic.rd_sent, (collected_number)d->traffic.sent.current.Data);
            rrdset_done(d->traffic.st);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->packets.received) &&
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

                add_interface_labels(d->packets.st, windows_shared_buffer, physical);

                d->packets.rd_received = rrddim_add(d->packets.st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->packets.rd_sent = rrddim_add(d->packets.st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(
                d->packets.st, d->packets.rd_received, (collected_number)d->packets.received.current.Data);
            rrddim_set_by_pointer(d->packets.st, d->packets.rd_sent, (collected_number)d->packets.sent.current.Data);
            rrdset_done(d->packets.st);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->speed.current_bandwidth)) {
            if (unlikely(!d->speed.st)) {
                d->speed.st = rrdset_create_localhost(
                    "net_speed",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.speed",
                    "Interface Speed",
                    "kilobits/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 10,
                    update_every,
                    RRDSET_TYPE_LINE);

                add_interface_labels(d->speed.st, windows_shared_buffer, physical);

                d->speed.rd = rrddim_add(d->speed.st, "speed", NULL, 1, BITS_IN_A_KILOBIT, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(d->speed.st, d->speed.rd, (collected_number)d->speed.current_bandwidth.current.Data);
            rrdset_done(d->speed.st);

            rrdvar_chart_variable_set(
                d->traffic.st,
                d->traffic.chart_var_speed,
                (NETDATA_DOUBLE)d->speed.current_bandwidth.current.Data / BITS_IN_A_KILOBIT);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->errors.received) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->errors.outbound)) {
            if (unlikely(!d->errors.st)) {
                d->errors.st = rrdset_create_localhost(
                    "net_errors",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.errors",
                    "Interface Errors",
                    "errors/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 3,
                    update_every,
                    RRDSET_TYPE_LINE);

                add_interface_labels(d->errors.st, windows_shared_buffer, physical);

                d->errors.rd_received = rrddim_add(d->errors.st, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->errors.rd_outbound = rrddim_add(d->errors.st, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(
                d->errors.st, d->errors.rd_received, (collected_number)d->errors.received.current.Data);
            rrddim_set_by_pointer(
                d->errors.st, d->errors.rd_outbound, (collected_number)d->errors.outbound.current.Data);
            rrdset_done(d->errors.st);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->discards.received) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->discards.outbound)) {
            if (unlikely(!d->discards.st)) {
                d->discards.st = rrdset_create_localhost(
                    "net_drops",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.drops",
                    "Interface Drops",
                    "drops/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 4,
                    update_every,
                    RRDSET_TYPE_LINE);

                add_interface_labels(d->discards.st, windows_shared_buffer, physical);

                d->discards.rd_received = rrddim_add(d->discards.st, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->discards.rd_outbound =
                    rrddim_add(d->discards.st, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(
                d->discards.st, d->discards.rd_received, (collected_number)d->discards.received.current.Data);
            rrddim_set_by_pointer(
                d->discards.st, d->discards.rd_outbound, (collected_number)d->discards.outbound.current.Data);
            rrdset_done(d->discards.st);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->queue.length)) {
            if (unlikely(!d->queue.st)) {
                d->queue.st = rrdset_create_localhost(
                    "net_queue_length",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.queue_length",
                    "Interface Output Queue Length",
                    "packets",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 5,
                    update_every,
                    RRDSET_TYPE_LINE);

                add_interface_labels(d->queue.st, windows_shared_buffer, physical);

                d->queue.rd = rrddim_add(d->queue.st, "length", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(d->queue.st, d->queue.rd, (collected_number)d->queue.length.current.Data);
            rrdset_done(d->queue.st);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->rsc.connections)) {
            if (unlikely(!d->rsc.st_connections)) {
                d->rsc.st_connections = rrdset_create_localhost(
                    "net_rsc_connections",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.rsc_connections",
                    "Active TCP Connections Offloaded by RSC",
                    "connections",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 6,
                    update_every,
                    RRDSET_TYPE_LINE);

                add_interface_labels(d->rsc.st_connections, windows_shared_buffer, physical);

                d->rsc.rd_connections =
                    rrddim_add(d->rsc.st_connections, "connections", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(
                d->rsc.st_connections, d->rsc.rd_connections, (collected_number)d->rsc.connections.current.Data);
            rrdset_done(d->rsc.st_connections);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->rsc.packets)) {
            if (unlikely(!d->rsc.st_packets)) {
                d->rsc.st_packets = rrdset_create_localhost(
                    "net_rsc_packets",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.rsc_packets",
                    "TCP RSC Coalesced Packets",
                    "packets/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 7,
                    update_every,
                    RRDSET_TYPE_LINE);

                add_interface_labels(d->rsc.st_packets, windows_shared_buffer, physical);

                d->rsc.rd_packets = rrddim_add(d->rsc.st_packets, "packets", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(d->rsc.st_packets, d->rsc.rd_packets, (collected_number)d->rsc.packets.current.Data);
            rrdset_done(d->rsc.st_packets);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->rsc.exceptions)) {
            if (unlikely(!d->rsc.st_exceptions)) {
                d->rsc.st_exceptions = rrdset_create_localhost(
                    "net_rsc_exceptions",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.rsc_exceptions",
                    "TCP RSC Exceptions",
                    "exceptions/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 8,
                    update_every,
                    RRDSET_TYPE_LINE);

                add_interface_labels(d->rsc.st_exceptions, windows_shared_buffer, physical);

                d->rsc.rd_exceptions =
                    rrddim_add(d->rsc.st_exceptions, "exceptions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(
                d->rsc.st_exceptions, d->rsc.rd_exceptions, (collected_number)d->rsc.exceptions.current.Data);
            rrdset_done(d->rsc.st_exceptions);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->rsc.average_packet_size)) {
            if (unlikely(!d->rsc.st_average_packet_size)) {
                d->rsc.st_average_packet_size = rrdset_create_localhost(
                    "net_rsc_average_packet_size",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.rsc_average_packet_size",
                    "TCP RSC Average Packet Size",
                    "bytes",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 9,
                    update_every,
                    RRDSET_TYPE_LINE);

                add_interface_labels(d->rsc.st_average_packet_size, windows_shared_buffer, physical);

                d->rsc.rd_average_packet_size =
                    rrddim_add(d->rsc.st_average_packet_size, "average", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(
                d->rsc.st_average_packet_size,
                d->rsc.rd_average_packet_size,
                (collected_number)d->rsc.average_packet_size.current.Data);
            rrdset_done(d->rsc.st_average_packet_size);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->chimney.connections)) {
            if (unlikely(!d->chimney.st)) {
                d->chimney.st = rrdset_create_localhost(
                    "net_chimney_connections",
                    windows_shared_buffer,
                    NULL,
                    windows_shared_buffer,
                    "net.chimney_connections",
                    "Active TCP Connections Offloaded with Chimney",
                    "connections",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibNetwork",
                    NETDATA_CHART_PRIO_FIRST_NET_IFACE + 10,
                    update_every,
                    RRDSET_TYPE_LINE);

                add_interface_labels(d->chimney.st, windows_shared_buffer, physical);

                d->chimney.rd = rrddim_add(d->chimney.st, "connections", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(d->chimney.st, d->chimney.rd, (collected_number)d->chimney.connections.current.Data);
            rrdset_done(d->chimney.st);
        }
    }

    if (physical) {
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

    // cleanup
    {
        struct network_interface *d;
        dfe_start_write(dict, d)
        {
            if (d->last_collected < now_ut) {
                network_interface_cleanup(d);
                dictionary_del(dict, d_dfe.name);
            }
        }
        dfe_done(d);
        dictionary_garbage_collect(dict);
    }

    return true;
}

int do_PerflibNetwork(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Network Interface");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    usec_t now_ut = now_monotonic_usec();
    if (!do_network_interface(pDataBlock, update_every, true, now_ut))
        return -1;
    if (!do_network_interface(pDataBlock, update_every, false, now_ut))
        return -1;

    struct network_protocol *tcp4 = NULL, *tcp6 = NULL;
    for (size_t i = 0; networks[i].protocol; i++) {
        do_network_protocol(pDataBlock, update_every, &networks[i]);

        if (!tcp4 && strcmp(networks[i].protocol, "TCPv4") == 0)
            tcp4 = &networks[i];
        if (!tcp6 && strcmp(networks[i].protocol, "TCPv6") == 0)
            tcp6 = &networks[i];
    }

    if (tcp4 && tcp6) {
        tcp46.packets.received = tcp4->packets.received;
        tcp46.packets.sent = tcp4->packets.sent;
        tcp46.packets.received.current.Data += tcp6->packets.received.current.Data;
        tcp46.packets.sent.current.Data += tcp6->packets.sent.current.Data;
        protocol_packets_chart_update(&tcp46, update_every);
    }
    return 0;
}
