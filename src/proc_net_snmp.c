#include "common.h"

#define RRD_TYPE_NET_SNMP           "ipv4"

struct netstat_columns {
    char *name;
    uint32_t hash;
    unsigned long long value;
    int multiplier;     // not needed everywhere
    char *label;        // not needed everywhere
    RRDDIM *rd;         // not needed everywhere
};

static struct netstat_columns ip_data[] = {
//    { "Forwarding", 0, 0, 1, NULL, NULL },
//    { "DefaultTTL", 0, 0, 1, NULL, NULL },
    { "InReceives", 0, 0, 1, NULL, NULL },
    { "InHdrErrors", 0, 0, 1, NULL, NULL },
    { "InAddrErrors", 0, 0, 1, NULL, NULL },
    { "ForwDatagrams", 0, 0, 1, NULL, NULL },
    { "InUnknownProtos", 0, 0, 1, NULL, NULL },
    { "InDiscards", 0, 0, 1, NULL, NULL },
    { "InDelivers", 0, 0, 1, NULL, NULL },
    { "OutRequests", 0, 0, 1, NULL, NULL },
    { "OutDiscards", 0, 0, 1, NULL, NULL },
    { "OutNoRoutes", 0, 0, 1, NULL, NULL },
//    { "ReasmTimeout", 0, 0, 1, NULL, NULL },
    { "ReasmReqds", 0, 0, 1, NULL, NULL },
    { "ReasmOKs", 0, 0, 1, NULL, NULL },
    { "ReasmFails", 0, 0, 1, NULL, NULL },
    { "FragOKs", 0, 0, 1, NULL, NULL },
    { "FragFails", 0, 0, 1, NULL, NULL },
    { "FragCreates", 0, 0, 1, NULL, NULL },
    { NULL, 0, 0, 0, NULL, NULL }
};

static struct netstat_columns icmp_data[] = {
    { "InMsgs", 0, 0, 1, NULL, NULL },
    { "OutMsgs", 0, 0, -1, NULL, NULL },
    { "InErrors", 0, 0, 1, NULL, NULL },
    { "OutErrors", 0, 0, -1, NULL, NULL },
    { "InCsumErrors", 0, 0, 1, NULL, NULL },

    // all these are available in icmpmsg
//    { "InDestUnreachs", 0, 0, 1, NULL, NULL },
//    { "OutDestUnreachs", 0, 0, -1, NULL, NULL },
//    { "InTimeExcds", 0, 0, 1, NULL, NULL },
//    { "OutTimeExcds", 0, 0, -1, NULL, NULL },
//    { "InParmProbs", 0, 0, 1, NULL, NULL },
//    { "OutParmProbs", 0, 0, -1, NULL, NULL },
//    { "InSrcQuenchs", 0, 0, 1, NULL, NULL },
//    { "OutSrcQuenchs", 0, 0, -1, NULL, NULL },
//    { "InRedirects", 0, 0, 1, NULL, NULL },
//    { "OutRedirects", 0, 0, -1, NULL, NULL },
//    { "InEchos", 0, 0, 1, NULL, NULL },
//    { "OutEchos", 0, 0, -1, NULL, NULL },
//    { "InEchoReps", 0, 0, 1, NULL, NULL },
//    { "OutEchoReps", 0, 0, -1, NULL, NULL },
//    { "InTimestamps", 0, 0, 1, NULL, NULL },
//    { "OutTimestamps", 0, 0, -1, NULL, NULL },
//    { "InTimestampReps", 0, 0, 1, NULL, NULL },
//    { "OutTimestampReps", 0, 0, -1, NULL, NULL },
//    { "InAddrMasks", 0, 0, 1, NULL, NULL },
//    { "OutAddrMasks", 0, 0, -1, NULL, NULL },
//    { "InAddrMaskReps", 0, 0, 1, NULL, NULL },
//    { "OutAddrMaskReps", 0, 0, -1, NULL, NULL },

    { NULL, 0, 0, 0, NULL, NULL }
};

static struct netstat_columns icmpmsg_data[] = {
    { "InType0", 0, 0, 1, "InEchoReps", NULL },
    { "OutType0", 0, 0, -1, "OutEchoReps", NULL },
//    { "InType1", 0, 0, 1, NULL, NULL },                   // unassigned
//    { "OutType1", 0, 0, -1, NULL, NULL },                 // unassigned
//    { "InType2", 0, 0, 1, NULL, NULL },                   // unassigned
//    { "OutType2", 0, 0, -1, NULL, NULL },                 // unassigned
    { "InType3", 0, 0, 1, "InDestUnreachs", NULL },
    { "OutType3", 0, 0, -1, "OutDestUnreachs", NULL },
//    { "InType4", 0, 0, 1, "InSrcQuenchs", NULL },         // deprecated
//    { "OutType4", 0, 0, -1, "OutSrcQuenchs", NULL },      // deprecated
    { "InType5", 0, 0, 1, "InRedirects", NULL },
    { "OutType5", 0, 0, -1, "OutRedirects", NULL },
//    { "InType6", 0, 0, 1, "InAlterHostAddr", NULL },      // deprecated
//    { "OutType6", 0, 0, -1, "OutAlterHostAddr", NULL },   // deprecated
//    { "InType7", 0, 0, 1, NULL, NULL },                   // unassigned
//    { "OutType7", 0, 0, -1, NULL, NULL },                 // unassigned
    { "InType8", 0, 0, 1, "InEchos", NULL },
    { "OutType8", 0, 0, -1, "OutEchos", NULL },
    { "InType9", 0, 0, 1, "InRouterAdvert", NULL },
    { "OutType9", 0, 0, -1, "OutRouterAdvert", NULL },
    { "InType10", 0, 0, 1, "InRouterSelect", NULL },
    { "OutType10", 0, 0, -1, "OutRouterSelect", NULL },
    { "InType11", 0, 0, 1, "InTimeExcds", NULL },
    { "OutType11", 0, 0, -1, "OutTimeExcds", NULL },
    { "InType12", 0, 0, 1, "InParmProbs", NULL },
    { "OutType12", 0, 0, -1, "OutParmProbs", NULL },
    { "InType13", 0, 0, 1, "InTimestamps", NULL },
    { "OutType13", 0, 0, -1, "OutTimestamps", NULL },
    { "InType14", 0, 0, 1, "InTimestampReps", NULL },
    { "OutType14", 0, 0, -1, "OutTimestampReps", NULL },
//    { "InType15", 0, 0, 1, "InInfos", NULL },             // deprecated
//    { "OutType15", 0, 0, -1, "OutInfos", NULL },          // deprecated
//    { "InType16", 0, 0, 1, "InInfoReps", NULL },          // deprecated
//    { "OutType16", 0, 0, -1, "OutInfoReps", NULL },       // deprecated
//    { "InType17", 0, 0, 1, "InAddrMasks", NULL },         // deprecated
//    { "OutType17", 0, 0, -1, "OutAddrMasks", NULL },      // deprecated
//    { "InType18", 0, 0, 1, "InAddrMaskReps", NULL },      // deprecated
//    { "OutType18", 0, 0, -1, "OutAddrMaskReps", NULL },   // deprecated
//    { "InType30", 0, 0, 1, "InTraceroute", NULL },        // deprecated
//    { "OutType30", 0, 0, -1, "OutTraceroute", NULL },     // deprecated
    { NULL, 0, 0, 0, NULL, NULL }
};

static struct netstat_columns tcp_data[] = {
//    { "RtoAlgorithm", 0, 0, 1, NULL, NULL },
//    { "RtoMin", 0, 0, 1, NULL, NULL },
//    { "RtoMax", 0, 0, 1, NULL, NULL },
//    { "MaxConn", 0, 0, 1, NULL, NULL },
    { "ActiveOpens", 0, 0, 1, NULL, NULL },
    { "PassiveOpens", 0, 0, 1, NULL, NULL },
    { "AttemptFails", 0, 0, 1, NULL, NULL },
    { "EstabResets", 0, 0, 1, NULL, NULL },
    { "CurrEstab", 0, 0, 1, NULL, NULL },
    { "InSegs", 0, 0, 1, NULL, NULL },
    { "OutSegs", 0, 0, 1, NULL, NULL },
    { "RetransSegs", 0, 0, 1, NULL, NULL },
    { "InErrs", 0, 0, 1, NULL, NULL },
    { "OutRsts", 0, 0, 1, NULL, NULL },
    { "InCsumErrors", 0, 0, 1, NULL, NULL },
    { NULL, 0, 0, 0, NULL, NULL }
};

static struct netstat_columns udp_data[] = {
    { "InDatagrams", 0, 0, 1, NULL, NULL },
    { "NoPorts", 0, 0, 1, NULL, NULL },
    { "InErrors", 0, 0, 1, NULL, NULL },
    { "OutDatagrams", 0, 0, 1, NULL, NULL },
    { "RcvbufErrors", 0, 0, 1, NULL, NULL },
    { "SndbufErrors", 0, 0, 1, NULL, NULL },
    { "InCsumErrors", 0, 0, 1, NULL, NULL },
    { "IgnoredMulti", 0, 0, 1, NULL, NULL },
    { NULL, 0, 0, 0, NULL, NULL }
};

static struct netstat_columns udplite_data[] = {
    { "InDatagrams", 0, 0, 1, NULL, NULL },
    { "NoPorts", 0, 0, 1, NULL, NULL },
    { "InErrors", 0, 0, 1, NULL, NULL },
    { "OutDatagrams", 0, 0, 1, NULL, NULL },
    { "RcvbufErrors", 0, 0, 1, NULL, NULL },
    { "SndbufErrors", 0, 0, 1, NULL, NULL },
    { "InCsumErrors", 0, 0, 1, NULL, NULL },
    { "IgnoredMulti", 0, 0, 1, NULL, NULL },
    { NULL, 0, 0, 0, NULL, NULL }
};

static void hash_array(struct netstat_columns *nc) {
    int i;

    for(i = 0; nc[i].name ;i++)
        nc[i].hash = simple_hash(nc[i].name);
}

static unsigned long long *netstat_columns_find(struct netstat_columns *nc, const char *name) {
    uint32_t i, hash = simple_hash(name);

    for(i = 0; nc[i].name ;i++)
        if(unlikely(nc[i].hash == hash && !strcmp(nc[i].name, name)))
            return &nc[i].value;

    fatal("Cannot find key '%s' in /proc/net/snmp internal array.", name);
}

static void parse_line_pair(procfile *ff, struct netstat_columns *nc, size_t header_line, size_t values_line) {
    size_t hwords = procfile_linewords(ff, header_line);
    size_t vwords = procfile_linewords(ff, values_line);
    size_t w, i;

    if(unlikely(vwords > hwords)) {
        error("File /proc/net/snmp on header line %zu has %zu words, but on value line %zu has %zu words.", header_line, hwords, values_line, vwords);
        vwords = hwords;
    }

    for(w = 1; w < vwords ;w++) {
        char *key = procfile_lineword(ff, header_line, w);
        uint32_t hash = simple_hash(key);

        for(i = 0 ; nc[i].name ;i++) {
            if(unlikely(hash == nc[i].hash && !strcmp(key, nc[i].name))) {
                nc[i].value = str2ull(procfile_lineword(ff, values_line, w));
                break;
            }
        }
    }
}

int do_proc_net_snmp(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1,
        do_tcp_sockets = -1, do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1,
        do_udp_packets = -1, do_udp_errors = -1, do_icmp_packets = -1, do_icmpmsg = -1, do_udplite_packets = -1;
    static uint32_t hash_ip = 0, hash_icmp = 0, hash_tcp = 0, hash_udp = 0, hash_icmpmsg = 0, hash_udplite = 0;

    //static unsigned long long *ip_Forwarding = NULL;
    //static unsigned long long *ip_DefaultTTL = NULL;
    static unsigned long long *ip_InReceives = NULL;
    static unsigned long long *ip_InHdrErrors = NULL;
    static unsigned long long *ip_InAddrErrors = NULL;
    static unsigned long long *ip_ForwDatagrams = NULL;
    static unsigned long long *ip_InUnknownProtos = NULL;
    static unsigned long long *ip_InDiscards = NULL;
    static unsigned long long *ip_InDelivers = NULL;
    static unsigned long long *ip_OutRequests = NULL;
    static unsigned long long *ip_OutDiscards = NULL;
    static unsigned long long *ip_OutNoRoutes = NULL;
    //static unsigned long long *ip_ReasmTimeout = NULL;
    static unsigned long long *ip_ReasmReqds = NULL;
    static unsigned long long *ip_ReasmOKs = NULL;
    static unsigned long long *ip_ReasmFails = NULL;
    static unsigned long long *ip_FragOKs = NULL;
    static unsigned long long *ip_FragFails = NULL;
    static unsigned long long *ip_FragCreates = NULL;

    static unsigned long long *icmp_InMsgs = NULL;
    static unsigned long long *icmp_OutMsgs = NULL;
    static unsigned long long *icmp_InErrors = NULL;
    static unsigned long long *icmp_OutErrors = NULL;
    static unsigned long long *icmp_InCsumErrors = NULL;

    //static unsigned long long *tcp_RtoAlgorithm = NULL;
    //static unsigned long long *tcp_RtoMin = NULL;
    //static unsigned long long *tcp_RtoMax = NULL;
    //static unsigned long long *tcp_MaxConn = NULL;
    static unsigned long long *tcp_ActiveOpens = NULL;
    static unsigned long long *tcp_PassiveOpens = NULL;
    static unsigned long long *tcp_AttemptFails = NULL;
    static unsigned long long *tcp_EstabResets = NULL;
    static unsigned long long *tcp_CurrEstab = NULL;
    static unsigned long long *tcp_InSegs = NULL;
    static unsigned long long *tcp_OutSegs = NULL;
    static unsigned long long *tcp_RetransSegs = NULL;
    static unsigned long long *tcp_InErrs = NULL;
    static unsigned long long *tcp_OutRsts = NULL;
    static unsigned long long *tcp_InCsumErrors = NULL;

    static unsigned long long *udp_InDatagrams = NULL;
    static unsigned long long *udp_NoPorts = NULL;
    static unsigned long long *udp_InErrors = NULL;
    static unsigned long long *udp_OutDatagrams = NULL;
    static unsigned long long *udp_RcvbufErrors = NULL;
    static unsigned long long *udp_SndbufErrors = NULL;
    static unsigned long long *udp_InCsumErrors = NULL;
    static unsigned long long *udp_IgnoredMulti = NULL;

    static unsigned long long *udplite_InDatagrams = NULL;
    static unsigned long long *udplite_NoPorts = NULL;
    static unsigned long long *udplite_InErrors = NULL;
    static unsigned long long *udplite_OutDatagrams = NULL;
    static unsigned long long *udplite_RcvbufErrors = NULL;
    static unsigned long long *udplite_SndbufErrors = NULL;
    static unsigned long long *udplite_InCsumErrors = NULL;
    static unsigned long long *udplite_IgnoredMulti = NULL;

    if(unlikely(do_ip_packets == -1)) {
        do_ip_packets       = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 packets", 1);
        do_ip_fragsout      = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 fragments sent", 1);
        do_ip_fragsin       = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 fragments assembly", 1);
        do_ip_errors        = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 errors", 1);
        do_tcp_sockets      = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP connections", 1);
        do_tcp_packets      = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP packets", 1);
        do_tcp_errors       = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP errors", 1);
        do_tcp_handshake    = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP handshake issues", 1);
        do_udp_packets      = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDP packets", 1);
        do_udp_errors       = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDP errors", 1);
        do_icmp_packets     = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 ICMP packets", 1);
        do_icmpmsg          = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 ICMP messages", 1);
        do_udplite_packets  = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDPLite packets", 1);

        hash_ip = simple_hash("Ip");
        hash_tcp = simple_hash("Tcp");
        hash_udp = simple_hash("Udp");
        hash_icmp = simple_hash("Icmp");
        hash_icmpmsg = simple_hash("IcmpMsg");
        hash_udplite = simple_hash("UdpLite");

        hash_array(ip_data);
        hash_array(tcp_data);
        hash_array(udp_data);
        hash_array(icmp_data);
        hash_array(icmpmsg_data);
        hash_array(udplite_data);

        //ip_Forwarding = netstat_columns_find(ip_data, "Forwarding");
        //ip_DefaultTTL = netstat_columns_find(ip_data, "DefaultTTL");
        ip_InReceives = netstat_columns_find(ip_data, "InReceives");
        ip_InHdrErrors = netstat_columns_find(ip_data, "InHdrErrors");
        ip_InAddrErrors = netstat_columns_find(ip_data, "InAddrErrors");
        ip_ForwDatagrams = netstat_columns_find(ip_data, "ForwDatagrams");
        ip_InUnknownProtos = netstat_columns_find(ip_data, "InUnknownProtos");
        ip_InDiscards = netstat_columns_find(ip_data, "InDiscards");
        ip_InDelivers = netstat_columns_find(ip_data, "InDelivers");
        ip_OutRequests = netstat_columns_find(ip_data, "OutRequests");
        ip_OutDiscards = netstat_columns_find(ip_data, "OutDiscards");
        ip_OutNoRoutes = netstat_columns_find(ip_data, "OutNoRoutes");
        //ip_ReasmTimeout = netstat_columns_find(ip_data, "ReasmTimeout");
        ip_ReasmReqds = netstat_columns_find(ip_data, "ReasmReqds");
        ip_ReasmOKs = netstat_columns_find(ip_data, "ReasmOKs");
        ip_ReasmFails = netstat_columns_find(ip_data, "ReasmFails");
        ip_FragOKs = netstat_columns_find(ip_data, "FragOKs");
        ip_FragFails = netstat_columns_find(ip_data, "FragFails");
        ip_FragCreates = netstat_columns_find(ip_data, "FragCreates");

        icmp_InMsgs = netstat_columns_find(icmp_data, "InMsgs");
        icmp_OutMsgs = netstat_columns_find(icmp_data, "OutMsgs");
        icmp_InErrors = netstat_columns_find(icmp_data, "InErrors");
        icmp_OutErrors = netstat_columns_find(icmp_data, "OutErrors");
        icmp_InCsumErrors = netstat_columns_find(icmp_data, "InCsumErrors");

        //tcp_RtoAlgorithm = netstat_columns_find(tcp_data, "RtoAlgorithm");
        //tcp_RtoMin = netstat_columns_find(tcp_data, "RtoMin");
        //tcp_RtoMax = netstat_columns_find(tcp_data, "RtoMax");
        //tcp_MaxConn = netstat_columns_find(tcp_data, "MaxConn");
        tcp_ActiveOpens = netstat_columns_find(tcp_data, "ActiveOpens");
        tcp_PassiveOpens = netstat_columns_find(tcp_data, "PassiveOpens");
        tcp_AttemptFails = netstat_columns_find(tcp_data, "AttemptFails");
        tcp_EstabResets = netstat_columns_find(tcp_data, "EstabResets");
        tcp_CurrEstab = netstat_columns_find(tcp_data, "CurrEstab");
        tcp_InSegs = netstat_columns_find(tcp_data, "InSegs");
        tcp_OutSegs = netstat_columns_find(tcp_data, "OutSegs");
        tcp_RetransSegs = netstat_columns_find(tcp_data, "RetransSegs");
        tcp_InErrs = netstat_columns_find(tcp_data, "InErrs");
        tcp_OutRsts = netstat_columns_find(tcp_data, "OutRsts");
        tcp_InCsumErrors = netstat_columns_find(tcp_data, "InCsumErrors");

        udp_InDatagrams = netstat_columns_find(udp_data, "InDatagrams");
        udp_NoPorts = netstat_columns_find(udp_data, "NoPorts");
        udp_InErrors = netstat_columns_find(udp_data, "InErrors");
        udp_OutDatagrams = netstat_columns_find(udp_data, "OutDatagrams");
        udp_RcvbufErrors = netstat_columns_find(udp_data, "RcvbufErrors");
        udp_SndbufErrors = netstat_columns_find(udp_data, "SndbufErrors");
        udp_InCsumErrors = netstat_columns_find(udp_data, "InCsumErrors");
        udp_IgnoredMulti = netstat_columns_find(udp_data, "IgnoredMulti");

        udplite_InDatagrams = netstat_columns_find(udplite_data, "InDatagrams");
        udplite_NoPorts = netstat_columns_find(udplite_data, "NoPorts");
        udplite_InErrors = netstat_columns_find(udplite_data, "InErrors");
        udplite_OutDatagrams = netstat_columns_find(udplite_data, "OutDatagrams");
        udplite_RcvbufErrors = netstat_columns_find(udplite_data, "RcvbufErrors");
        udplite_SndbufErrors = netstat_columns_find(udplite_data, "SndbufErrors");
        udplite_InCsumErrors = netstat_columns_find(udplite_data, "InCsumErrors");
        udplite_IgnoredMulti = netstat_columns_find(udplite_data, "IgnoredMulti");
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/snmp");
        ff = procfile_open(config_get("plugin:proc:/proc/net/snmp", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;
    size_t words;

    for(l = 0; l < lines ;l++) {
        char *key = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(key);

        if(unlikely(hash == hash_ip && strcmp(key, "Ip") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Ip") != 0) {
                error("Cannot read Ip line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp Ip line. Expected 3+ params, read %zu.", words);
                continue;
            }

            // see also http://net-snmp.sourceforge.net/docs/mibs/ip.html
            parse_line_pair(ff, ip_data, h, l);

            // --------------------------------------------------------------------

            if(do_ip_packets) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_InReceives = NULL,
                              *rd_OutRequests = NULL,
                              *rd_ForwDatagrams = NULL,
                              *rd_InDelivers = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "packets"
                            , NULL
                            , "packets"
                            , NULL
                            , "IPv4 Packets"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , 3000
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_InReceives    = rrddim_add(st, "InReceives",    "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutRequests   = rrddim_add(st, "OutRequests",   "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ForwDatagrams = rrddim_add(st, "ForwDatagrams", "forwarded", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_InDelivers    = rrddim_add(st, "InDelivers",    "delivered", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_OutRequests,   *ip_OutRequests);
                rrddim_set_by_pointer(st, rd_InReceives,    *ip_InReceives);
                rrddim_set_by_pointer(st, rd_ForwDatagrams, *ip_ForwDatagrams);
                rrddim_set_by_pointer(st, rd_InDelivers,    *ip_InDelivers);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ip_fragsout) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_FragOKs = NULL,
                              *rd_FragFails = NULL,
                              *rd_FragCreates = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "fragsout"
                            , NULL
                            , "fragments"
                            , NULL
                            , "IPv4 Fragments Sent"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , 3010
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_FragOKs     = rrddim_add(st, "FragOKs",     "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_FragFails   = rrddim_add(st, "FragFails",   "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_FragCreates = rrddim_add(st, "FragCreates", "created", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_FragOKs,     *ip_FragOKs);
                rrddim_set_by_pointer(st, rd_FragFails,   *ip_FragFails);
                rrddim_set_by_pointer(st, rd_FragCreates, *ip_FragCreates);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ip_fragsin) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_ReasmOKs = NULL,
                              *rd_ReasmFails = NULL,
                              *rd_ReasmReqds = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "fragsin"
                            , NULL
                            , "fragments"
                            , NULL
                            , "IPv4 Fragments Reassembly"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , 3011
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_ReasmOKs   = rrddim_add(st, "ReasmOKs",   "ok",      1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ReasmFails = rrddim_add(st, "ReasmFails", "failed", -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ReasmReqds = rrddim_add(st, "ReasmReqds", "all",     1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_ReasmOKs,   *ip_ReasmOKs);
                rrddim_set_by_pointer(st, rd_ReasmFails, *ip_ReasmFails);
                rrddim_set_by_pointer(st, rd_ReasmReqds, *ip_ReasmReqds);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ip_errors) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_InDiscards = NULL,
                              *rd_OutDiscards = NULL,
                              *rd_InHdrErrors = NULL,
                              *rd_OutNoRoutes = NULL,
                              *rd_InAddrErrors = NULL,
                              *rd_InUnknownProtos = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "errors"
                            , NULL
                            , "errors"
                            , NULL
                            , "IPv4 Errors"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , 3002
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_InDiscards      = rrddim_add(st, "InDiscards",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutDiscards     = rrddim_add(st, "OutDiscards",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                    rd_InHdrErrors     = rrddim_add(st, "InHdrErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutNoRoutes     = rrddim_add(st, "OutNoRoutes",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                    rd_InAddrErrors    = rrddim_add(st, "InAddrErrors",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_InUnknownProtos = rrddim_add(st, "InUnknownProtos", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_InDiscards,      *ip_InDiscards);
                rrddim_set_by_pointer(st, rd_OutDiscards,     *ip_OutDiscards);
                rrddim_set_by_pointer(st, rd_InHdrErrors,     *ip_InHdrErrors);
                rrddim_set_by_pointer(st, rd_InAddrErrors,    *ip_InAddrErrors);
                rrddim_set_by_pointer(st, rd_InUnknownProtos, *ip_InUnknownProtos);
                rrddim_set_by_pointer(st, rd_OutNoRoutes,     *ip_OutNoRoutes);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_icmp && strcmp(key, "Icmp") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Icmp") != 0) {
                error("Cannot read Icmp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp Icmp line. Expected 3+ params, read %zu.", words);
                continue;
            }

            parse_line_pair(ff, icmp_data, h, l);

            // --------------------------------------------------------------------

            if(do_icmp_packets) {
                {
                    static RRDSET *st_packets = NULL;
                    static RRDDIM *rd_InMsgs = NULL,
                                  *rd_OutMsgs = NULL;

                    if(unlikely(!st_packets)) {
                        st_packets = rrdset_create_localhost(
                                RRD_TYPE_NET_SNMP
                                , "icmp"
                                , NULL
                                , "icmp"
                                , NULL
                                , "IPv4 ICMP Packets"
                                , "packets/s"
                                , "proc"
                                , "net/snmp"
                                , 2602
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rd_InMsgs  = rrddim_add(st_packets, "InMsgs",  "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_OutMsgs = rrddim_add(st_packets, "OutMsgs", "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }
                    else rrdset_next(st_packets);

                    rrddim_set_by_pointer(st_packets, rd_InMsgs,  *icmp_InMsgs);
                    rrddim_set_by_pointer(st_packets, rd_OutMsgs, *icmp_OutMsgs);

                    rrdset_done(st_packets);
                }

                {
                    static RRDSET *st_errors = NULL;
                    static RRDDIM *rd_InErrors = NULL,
                                  *rd_OutErrors = NULL,
                                  *rd_InCsumErrors = NULL;

                    if(unlikely(!st_errors)) {
                        st_errors = rrdset_create_localhost(
                                RRD_TYPE_NET_SNMP
                                , "icmp_errors"
                                , NULL
                                , "icmp"
                                , NULL
                                , "IPv4 ICMP Errors"
                                , "packets/s"
                                , "proc"
                                , "net/snmp"
                                , 2603
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rd_InErrors     = rrddim_add(st_errors, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_OutErrors    = rrddim_add(st_errors, "OutErrors",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_InCsumErrors = rrddim_add(st_errors, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }
                    else rrdset_next(st_errors);

                    rrddim_set_by_pointer(st_errors, rd_InErrors,     *icmp_InErrors);
                    rrddim_set_by_pointer(st_errors, rd_OutErrors,    *icmp_OutErrors);
                    rrddim_set_by_pointer(st_errors, rd_InCsumErrors, *icmp_InCsumErrors);

                    rrdset_done(st_errors);
                }
            }
        }
        else if(unlikely(hash == hash_icmpmsg && strcmp(key, "IcmpMsg") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "IcmpMsg") != 0) {
                error("Cannot read IcmpMsg line from /proc/net/snmp.");
                break;
            }

            parse_line_pair(ff, icmpmsg_data, h, l);

            // --------------------------------------------------------------------

            if(do_icmpmsg) {
                int i;

                static RRDSET *st = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "icmpmsg"
                            , NULL
                            , "icmp"
                            , NULL
                            , "IPv4 ICMP Messages"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , 2604
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    for(i = 0; icmpmsg_data[i].name ;i++)
                        icmpmsg_data[i].rd = rrddim_add(st, icmpmsg_data[i].name, icmpmsg_data[i].label,  icmpmsg_data[i].multiplier, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                for(i = 0; icmpmsg_data[i].name ;i++)
                    rrddim_set_by_pointer(st, icmpmsg_data[i].rd, icmpmsg_data[i].value);

                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_tcp && strcmp(key, "Tcp") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Tcp") != 0) {
                error("Cannot read Tcp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp Tcp line. Expected 3+ params, read %zu.", words);
                continue;
            }

            parse_line_pair(ff, tcp_data, h, l);

            // --------------------------------------------------------------------

            // see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
            if(do_tcp_sockets) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_CurrEstab = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "tcpsock"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Connections"
                            , "active connections"
                            , "proc"
                            , "net/snmp"
                            , 2500
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_CurrEstab = rrddim_add(st, "CurrEstab", "connections", 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_CurrEstab, *tcp_CurrEstab);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_packets) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_InSegs = NULL,
                              *rd_OutSegs = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "tcppackets"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Packets"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , 2600
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_InSegs  = rrddim_add(st, "InSegs",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutSegs = rrddim_add(st, "OutSegs", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_InSegs,  *tcp_InSegs);
                rrddim_set_by_pointer(st, rd_OutSegs, *tcp_OutSegs);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_errors) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_InErrs = NULL,
                              *rd_InCsumErrors = NULL,
                              *rd_RetransSegs = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "tcperrors"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Errors"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , 2700
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_InErrs       = rrddim_add(st, "InErrs",       NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_RetransSegs  = rrddim_add(st, "RetransSegs",  NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_InErrs,       *tcp_InErrs);
                rrddim_set_by_pointer(st, rd_InCsumErrors, *tcp_InCsumErrors);
                rrddim_set_by_pointer(st, rd_RetransSegs,  *tcp_RetransSegs);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcp_handshake) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_EstabResets = NULL,
                              *rd_OutRsts = NULL,
                              *rd_ActiveOpens = NULL,
                              *rd_PassiveOpens = NULL,
                              *rd_AttemptFails = NULL,
                              *rd_TCPSynRetrans = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "tcphandshake"
                            , NULL
                            , "tcp"
                            , NULL
                            , "IPv4 TCP Handshake Issues"
                            , "events/s"
                            , "proc"
                            , "net/snmp"
                            , 2900
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_EstabResets   = rrddim_add(st, "EstabResets",   NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutRsts       = rrddim_add(st, "OutRsts",       NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_ActiveOpens   = rrddim_add(st, "ActiveOpens",   NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_PassiveOpens  = rrddim_add(st, "PassiveOpens",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_AttemptFails  = rrddim_add(st, "AttemptFails",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_TCPSynRetrans = rrddim_add(st, "TCPSynRetrans", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_EstabResets,   *tcp_EstabResets);
                rrddim_set_by_pointer(st, rd_OutRsts,       *tcp_OutRsts);
                rrddim_set_by_pointer(st, rd_ActiveOpens,   *tcp_ActiveOpens);
                rrddim_set_by_pointer(st, rd_PassiveOpens,  *tcp_PassiveOpens);
                rrddim_set_by_pointer(st, rd_AttemptFails,  *tcp_AttemptFails);
                rrddim_set_by_pointer(st, rd_TCPSynRetrans, tcpext_TCPSynRetrans);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_udp && strcmp(key, "Udp") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "Udp") != 0) {
                error("Cannot read Udp line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp Udp line. Expected 3+ params, read %zu.", words);
                continue;
            }

            parse_line_pair(ff, udp_data, h, l);

            // --------------------------------------------------------------------

            // see http://net-snmp.sourceforge.net/docs/mibs/udp.html
            if(do_udp_packets) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_InDatagrams = NULL,
                              *rd_OutDatagrams = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "udppackets"
                            , NULL
                            , "udp"
                            , NULL
                            , "IPv4 UDP Packets"
                            , "packets/s"
                            , "proc"
                            , "net/snmp"
                            , 2601
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rd_InDatagrams  = rrddim_add(st, "InDatagrams",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_OutDatagrams = rrddim_add(st, "OutDatagrams", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_InDatagrams,  *udp_InDatagrams);
                rrddim_set_by_pointer(st, rd_OutDatagrams, *udp_OutDatagrams);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_udp_errors) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_RcvbufErrors = NULL,
                              *rd_SndbufErrors = NULL,
                              *rd_InErrors = NULL,
                              *rd_NoPorts = NULL,
                              *rd_InCsumErrors = NULL,
                              *rd_IgnoredMulti = NULL;

                if(unlikely(!st)) {
                    st = rrdset_create_localhost(
                            RRD_TYPE_NET_SNMP
                            , "udperrors"
                            , NULL
                            , "udp"
                            , NULL
                            , "IPv4 UDP Errors"
                            , "events/s"
                            , "proc"
                            , "net/snmp"
                            , 2701
                            , update_every
                            , RRDSET_TYPE_LINE
                    );
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

                    rd_RcvbufErrors = rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_SndbufErrors = rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_InErrors     = rrddim_add(st, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_NoPorts      = rrddim_add(st, "NoPorts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_IgnoredMulti = rrddim_add(st, "IgnoredMulti", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set_by_pointer(st, rd_InErrors,     *udp_InErrors);
                rrddim_set_by_pointer(st, rd_NoPorts,      *udp_NoPorts);
                rrddim_set_by_pointer(st, rd_RcvbufErrors, *udp_RcvbufErrors);
                rrddim_set_by_pointer(st, rd_SndbufErrors, *udp_SndbufErrors);
                rrddim_set_by_pointer(st, rd_InCsumErrors, *udp_InCsumErrors);
                rrddim_set_by_pointer(st, rd_IgnoredMulti, *udp_IgnoredMulti);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_udplite && strcmp(key, "UdpLite") == 0)) {
            size_t h = l++;

            if(strcmp(procfile_lineword(ff, l, 0), "UdpLite") != 0) {
                error("Cannot read UdpLite line from /proc/net/snmp.");
                break;
            }

            words = procfile_linewords(ff, l);
            if(words < 3) {
                error("Cannot read /proc/net/snmp UdpLite line. Expected 3+ params, read %zu.", words);
                continue;
            }

            parse_line_pair(ff, udplite_data, h, l);

            // --------------------------------------------------------------------

            if(do_udplite_packets) {
                {
                    static RRDSET *st = NULL;
                    static RRDDIM *rd_InDatagrams = NULL,
                                  *rd_OutDatagrams = NULL;

                    if(unlikely(!st)) {
                        st = rrdset_create_localhost(
                                RRD_TYPE_NET_SNMP
                                , "udplite"
                                , NULL
                                , "udplite"
                                , NULL
                                , "IPv4 UDPLite Packets"
                                , "packets/s"
                                , "proc"
                                , "net/snmp"
                                , 2603
                                , update_every
                                , RRDSET_TYPE_LINE
                        );

                        rd_InDatagrams  = rrddim_add(st, "InDatagrams",  "received", 1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_OutDatagrams = rrddim_add(st, "OutDatagrams", "sent",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }
                    else rrdset_next(st);

                    rrddim_set_by_pointer(st, rd_InDatagrams,  *udplite_InDatagrams);
                    rrddim_set_by_pointer(st, rd_OutDatagrams, *udplite_OutDatagrams);
                    rrdset_done(st);
                }

                {
                    static RRDSET *st = NULL;
                    static RRDDIM *rd_RcvbufErrors = NULL,
                                  *rd_SndbufErrors = NULL,
                                  *rd_InErrors = NULL,
                                  *rd_NoPorts = NULL,
                                  *rd_InCsumErrors = NULL,
                                  *rd_IgnoredMulti = NULL;

                    if(unlikely(!st)) {
                        st = rrdset_create_localhost(
                                RRD_TYPE_NET_SNMP
                                , "udplite_errors"
                                , NULL
                                , "udplite"
                                , NULL
                                , "IPv4 UDPLite Errors"
                                , "packets/s"
                                , "proc"
                                , "net/snmp"
                                , 2604
                                , update_every
                                , RRDSET_TYPE_LINE);

                        rd_RcvbufErrors = rrddim_add(st, "RcvbufErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_SndbufErrors = rrddim_add(st, "SndbufErrors", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_InErrors     = rrddim_add(st, "InErrors",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_NoPorts      = rrddim_add(st, "NoPorts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_InCsumErrors = rrddim_add(st, "InCsumErrors", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                        rd_IgnoredMulti = rrddim_add(st, "IgnoredMulti", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                    }
                    else rrdset_next(st);

                    rrddim_set_by_pointer(st, rd_NoPorts,      *udplite_NoPorts);
                    rrddim_set_by_pointer(st, rd_InErrors,     *udplite_InErrors);
                    rrddim_set_by_pointer(st, rd_InCsumErrors, *udplite_InCsumErrors);
                    rrddim_set_by_pointer(st, rd_RcvbufErrors, *udplite_RcvbufErrors);
                    rrddim_set_by_pointer(st, rd_SndbufErrors, *udplite_SndbufErrors);
                    rrddim_set_by_pointer(st, rd_IgnoredMulti, *udplite_IgnoredMulti);
                    rrdset_done(st);
                }
            }
        }
    }

    return 0;
}

