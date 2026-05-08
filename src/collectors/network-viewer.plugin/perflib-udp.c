// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/os/windows-api/windows_api.h"
#include "libnetdata/os/windows-perflib/perflib.h"

#define PLUGIN_NETWORK_VIEWER_NAME "network-viewer.plugin"

// Priority values continue after the TCP charts used by the same executable.
#define PRIO_UDP_IPV4_DATAGRAMS 21075
#define PRIO_UDP_IPV6_DATAGRAMS 21076

typedef struct {
    const char *af;
    const char *object_name;
    const char *type;
    const char *family;
    const char *title_prefix;
    const char *context_prefix;
    int datagrams_priority;

    COUNTER_DATA datagrams_no_port;
    COUNTER_DATA datagrams_received_errors;
    COUNTER_DATA datagrams_received;
    COUNTER_DATA datagrams_sent;

    bool datagrams_chart_created;
} UDP_FAMILY;

static UDP_FAMILY udp_ipv4 = {
    .af = "ipv4",
    .object_name = "UDPv4",
    .type = "ipv4",
    .family = "udp",
    .title_prefix = "IPv4",
    .context_prefix = "ipv4.udp",
    .datagrams_priority = PRIO_UDP_IPV4_DATAGRAMS,
};

static UDP_FAMILY udp_ipv6 = {
    .af = "ipv6",
    .object_name = "UDPv6",
    .type = "ipv6",
    .family = "udp6",
    .title_prefix = "IPv6",
    .context_prefix = "ipv6.udp",
    .datagrams_priority = PRIO_UDP_IPV6_DATAGRAMS,
};

static void initialize_udp_keys(UDP_FAMILY *udp)
{
    udp->datagrams_no_port.key = "Datagrams No Port/sec";
    udp->datagrams_received_errors.key = "Datagrams Received Errors";
    udp->datagrams_received.key = "Datagrams Received/sec";
    udp->datagrams_sent.key = "Datagrams Sent/sec";
}

static void initialize(void)
{
    initialize_udp_keys(&udp_ipv4);
    initialize_udp_keys(&udp_ipv6);
}

static void udp_create_datagrams_chart(UDP_FAMILY *udp, int update_every)
{
    if (udp->datagrams_chart_created)
        return;
    udp->datagrams_chart_created = true;

    char context[64];
    char title[64];
    snprintfz(context, sizeof(context), "%s.datagrams", udp->context_prefix);
    snprintfz(title, sizeof(title), "%s UDP Datagrams", udp->title_prefix);

    fprintf(stdout,
            "CHART %s.datagrams '' '%s' datagrams/s %s %s line %d %d '' '" PLUGIN_NETWORK_VIEWER_NAME "' PerflibUDP\n",
            udp->type, title, udp->family, context, udp->datagrams_priority, update_every);
    fprintf(stdout, "DIMENSION no_port '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION received_errors '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION received '' incremental 1 1\n");
    fprintf(stdout, "DIMENSION sent '' incremental 1 1\n");
}

static bool udp_collect_family(UDP_FAMILY *udp, int update_every, usec_t dt)
{
    DWORD id = RegistryFindIDByName(udp->object_name);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return false;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return false;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, udp->object_name);
    if (!pObjectType)
        return false;

    bool have_any = false;

    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_no_port);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_received_errors);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_received);
    have_any |= perflibGetObjectCounter(pDataBlock, pObjectType, &udp->datagrams_sent);

    udp_create_datagrams_chart(udp, update_every);

    fprintf(stdout, "BEGIN %s.datagrams %" PRIu64 "\n", udp->type, dt);
    fprintf(stdout, "SET no_port = %lld\n", (long long)udp->datagrams_no_port.current.Data);
    fprintf(stdout, "SET received_errors = %lld\n", (long long)udp->datagrams_received_errors.current.Data);
    fprintf(stdout, "SET received = %lld\n", (long long)udp->datagrams_received.current.Data);
    fprintf(stdout, "SET sent = %lld\n", (long long)udp->datagrams_sent.current.Data);
    fprintf(stdout, "END\n");

    return have_any;
}

int do_PerflibUDP(int update_every, usec_t dt)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    bool have_any = false;
    have_any |= udp_collect_family(&udp_ipv4, update_every, dt);
    have_any |= udp_collect_family(&udp_ipv6, update_every, dt);

    return have_any ? 0 : -1;
}
