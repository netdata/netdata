#include "common.h"


struct netstat_columns {
    char *name;
    uint32_t hash;
    unsigned long long value;
    int multiplier;     // not needed everywhere
    char *label;        // not needed everywhere
};

static struct netstat_columns tcpext_data[] = {
    { "SyncookiesSent", 0, 0, 1, NULL },
    { "SyncookiesRecv", 0, 0, 1, NULL },
    { "SyncookiesFailed", 0, 0, 1, NULL },
    { "EmbryonicRsts", 0, 0, 1, NULL },
    { "PruneCalled", 0, 0, 1, NULL },
    { "RcvPruned", 0, 0, 1, NULL },
    { "OfoPruned", 0, 0, 1, NULL },
    { "OutOfWindowIcmps", 0, 0, 1, NULL },
    { "LockDroppedIcmps", 0, 0, 1, NULL },
    { "ArpFilter", 0, 0, 1, NULL },
    { "TW", 0, 0, 1, NULL },
    { "TWRecycled", 0, 0, 1, NULL },
    { "TWKilled", 0, 0, 1, NULL },
    { "PAWSPassive", 0, 0, 1, NULL },
    { "PAWSActive", 0, 0, 1, NULL },
    { "PAWSEstab", 0, 0, 1, NULL },
    { "DelayedACKs", 0, 0, 1, NULL },
    { "DelayedACKLocked", 0, 0, 1, NULL },
    { "DelayedACKLost", 0, 0, 1, NULL },
    { "ListenOverflows", 0, 0, 1, NULL },
    { "ListenDrops", 0, 0, 1, NULL },
    { "TCPPrequeued", 0, 0, 1, NULL },
    { "TCPDirectCopyFromBacklog", 0, 0, 1, NULL },
    { "TCPDirectCopyFromPrequeue", 0, 0, 1, NULL },
    { "TCPPrequeueDropped", 0, 0, 1, NULL },
    { "TCPHPHits", 0, 0, 1, NULL },
    { "TCPHPHitsToUser", 0, 0, 1, NULL },
    { "TCPPureAcks", 0, 0, 1, NULL },
    { "TCPHPAcks", 0, 0, 1, NULL },
    { "TCPRenoRecovery", 0, 0, 1, NULL },
    { "TCPSackRecovery", 0, 0, 1, NULL },
    { "TCPSACKReneging", 0, 0, 1, NULL },
    { "TCPFACKReorder", 0, 0, 1, NULL },
    { "TCPSACKReorder", 0, 0, 1, NULL },
    { "TCPRenoReorder", 0, 0, 1, NULL },
    { "TCPTSReorder", 0, 0, 1, NULL },
    { "TCPFullUndo", 0, 0, 1, NULL },
    { "TCPPartialUndo", 0, 0, 1, NULL },
    { "TCPDSACKUndo", 0, 0, 1, NULL },
    { "TCPLossUndo", 0, 0, 1, NULL },
    { "TCPLostRetransmit", 0, 0, 1, NULL },
    { "TCPRenoFailures", 0, 0, 1, NULL },
    { "TCPSackFailures", 0, 0, 1, NULL },
    { "TCPLossFailures", 0, 0, 1, NULL },
    { "TCPFastRetrans", 0, 0, 1, NULL },
    { "TCPForwardRetrans", 0, 0, 1, NULL },
    { "TCPSlowStartRetrans", 0, 0, 1, NULL },
    { "TCPTimeouts", 0, 0, 1, NULL },
    { "TCPLossProbes", 0, 0, 1, NULL },
    { "TCPLossProbeRecovery", 0, 0, 1, NULL },
    { "TCPRenoRecoveryFail", 0, 0, 1, NULL },
    { "TCPSackRecoveryFail", 0, 0, 1, NULL },
    { "TCPSchedulerFailed", 0, 0, 1, NULL },
    { "TCPRcvCollapsed", 0, 0, 1, NULL },
    { "TCPDSACKOldSent", 0, 0, 1, NULL },
    { "TCPDSACKOfoSent", 0, 0, 1, NULL },
    { "TCPDSACKRecv", 0, 0, 1, NULL },
    { "TCPDSACKOfoRecv", 0, 0, 1, NULL },
    { "TCPAbortOnData", 0, 0, 1, NULL },
    { "TCPAbortOnClose", 0, 0, 1, NULL },
    { "TCPAbortOnMemory", 0, 0, 1, NULL },
    { "TCPAbortOnTimeout", 0, 0, 1, NULL },
    { "TCPAbortOnLinger", 0, 0, 1, NULL },
    { "TCPAbortFailed", 0, 0, 1, NULL },
    { "TCPMemoryPressures", 0, 0, 1, NULL },
    { "TCPSACKDiscard", 0, 0, 1, NULL },
    { "TCPDSACKIgnoredOld", 0, 0, 1, NULL },
    { "TCPDSACKIgnoredNoUndo", 0, 0, 1, NULL },
    { "TCPSpuriousRTOs", 0, 0, 1, NULL },
    { "TCPMD5NotFound", 0, 0, 1, NULL },
    { "TCPMD5Unexpected", 0, 0, 1, NULL },
    { "TCPSackShifted", 0, 0, 1, NULL },
    { "TCPSackMerged", 0, 0, 1, NULL },
    { "TCPSackShiftFallback", 0, 0, 1, NULL },
    { "TCPBacklogDrop", 0, 0, 1, NULL },
    { "TCPMinTTLDrop", 0, 0, 1, NULL },
    { "TCPDeferAcceptDrop", 0, 0, 1, NULL },
    { "IPReversePathFilter", 0, 0, 1, NULL },
    { "TCPTimeWaitOverflow", 0, 0, 1, NULL },
    { "TCPReqQFullDoCookies", 0, 0, 1, NULL },
    { "TCPReqQFullDrop", 0, 0, 1, NULL },
    { "TCPRetransFail", 0, 0, 1, NULL },
    { "TCPRcvCoalesce", 0, 0, 1, NULL },
    { "TCPOFOQueue", 0, 0, 1, NULL },
    { "TCPOFODrop", 0, 0, 1, NULL },
    { "TCPOFOMerge", 0, 0, 1, NULL },
    { "TCPChallengeACK", 0, 0, 1, NULL },
    { "TCPSYNChallenge", 0, 0, 1, NULL },
    { "TCPFastOpenActive", 0, 0, 1, NULL },
    { "TCPFastOpenActiveFail", 0, 0, 1, NULL },
    { "TCPFastOpenPassive", 0, 0, 1, NULL },
    { "TCPFastOpenPassiveFail", 0, 0, 1, NULL },
    { "TCPFastOpenListenOverflow", 0, 0, 1, NULL },
    { "TCPFastOpenCookieReqd", 0, 0, 1, NULL },
    { "TCPSpuriousRtxHostQueues", 0, 0, 1, NULL },
    { "BusyPollRxPackets", 0, 0, 1, NULL },
    { "TCPAutoCorking", 0, 0, 1, NULL },
    { "TCPFromZeroWindowAdv", 0, 0, 1, NULL },
    { "TCPToZeroWindowAdv", 0, 0, 1, NULL },
    { "TCPWantZeroWindowAdv", 0, 0, 1, NULL },
    { "TCPSynRetrans", 0, 0, 1, NULL },
    { "TCPOrigDataSent", 0, 0, 1, NULL },
    { "TCPHystartTrainDetect", 0, 0, 1, NULL },
    { "TCPHystartTrainCwnd", 0, 0, 1, NULL },
    { "TCPHystartDelayDetect", 0, 0, 1, NULL },
    { "TCPHystartDelayCwnd", 0, 0, 1, NULL },
    { "TCPACKSkippedSynRecv", 0, 0, 1, NULL },
    { "TCPACKSkippedPAWS", 0, 0, 1, NULL },
    { "TCPACKSkippedSeq", 0, 0, 1, NULL },
    { "TCPACKSkippedFinWait2", 0, 0, 1, NULL },
    { "TCPACKSkippedTimeWait", 0, 0, 1, NULL },
    { "TCPACKSkippedChallenge", 0, 0, 1, NULL },
    { "TCPWinProbe", 0, 0, 1, NULL },
    { "TCPKeepAlive", 0, 0, 1, NULL },
    { "TCPMTUPFail", 0, 0, 1, NULL },
    { "TCPMTUPSuccess", 0, 0, 1, NULL },
    { NULL, 0, 0, 0, NULL }
};

static struct netstat_columns ipext_data[] = {
    { "InNoRoutes", 0, 0, 1, NULL },
    { "InTruncatedPkts", 0, 0, 1, NULL },
    { "InMcastPkts", 0, 0, 1, NULL },
    { "OutMcastPkts", 0, 0, 1, NULL },
    { "InBcastPkts", 0, 0, 1, NULL },
    { "OutBcastPkts", 0, 0, 1, NULL },
    { "InOctets", 0, 0, 1, NULL },
    { "OutOctets", 0, 0, 1, NULL },
    { "InMcastOctets", 0, 0, 1, NULL },
    { "OutMcastOctets", 0, 0, 1, NULL },
    { "InBcastOctets", 0, 0, 1, NULL },
    { "OutBcastOctets", 0, 0, 1, NULL },
    { "InCsumErrors", 0, 0, 1, NULL },
    { "InNoECTPkts", 0, 0, 1, NULL },
    { "InECT1Pkts", 0, 0, 1, NULL },
    { "InECT0Pkts", 0, 0, 1, NULL },
    { "InCEPkts", 0, 0, 1, NULL },
    { NULL, 0, 0, 0, NULL }
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

    fatal("Cannot find key '%s' in /proc/net/netstat internal array.", name);
}

static void parse_line_pair(procfile *ff, struct netstat_columns *nc, uint32_t header_line, uint32_t values_line) {
    uint32_t hwords = procfile_linewords(ff, header_line);
    uint32_t vwords = procfile_linewords(ff, values_line);
    uint32_t w, i;

    if(unlikely(vwords > hwords)) {
        error("File /proc/net/netstat on header line %u has %u words, but on value line %u has %u words.", header_line, hwords, values_line, vwords);
        vwords = hwords;
    }

    for(w = 1; w < vwords ;w++) {
        char *key = procfile_lineword(ff, header_line, w);
        uint32_t hash = simple_hash(key);

        for(i = 0 ; nc[i].name ;i++) {
            if(unlikely(hash == nc[i].hash && !strcmp(key, nc[i].name))) {
                nc[i].value = strtoull(procfile_lineword(ff, values_line, w), NULL, 10);
                break;
            }
        }
    }
}


int do_proc_net_netstat(int update_every, usec_t dt) {
    (void)dt;

    static int do_bandwidth = -1, do_inerrors = -1, do_mcast = -1, do_bcast = -1, do_mcast_p = -1, do_bcast_p = -1, do_ecn = -1, \
        do_tcpext_reorder = -1, do_tcpext_syscookies = -1, do_tcpext_ofo = -1, do_tcpext_connaborts = -1, do_tcpext_memory = -1;
    static uint32_t hash_ipext = 0, hash_tcpext = 0;
    static procfile *ff = NULL;

    static unsigned long long *tcpext_TCPRenoReorder = NULL;
    static unsigned long long *tcpext_TCPFACKReorder = NULL;
    static unsigned long long *tcpext_TCPSACKReorder = NULL;
    static unsigned long long *tcpext_TCPTSReorder = NULL;

    static unsigned long long *tcpext_SyncookiesSent = NULL;
    static unsigned long long *tcpext_SyncookiesRecv = NULL;
    static unsigned long long *tcpext_SyncookiesFailed = NULL;

    static unsigned long long *tcpext_TCPOFOQueue = NULL; // Number of packets queued in OFO queue
    static unsigned long long *tcpext_TCPOFODrop = NULL;  // Number of packets meant to be queued in OFO but dropped because socket rcvbuf limit hit.
    static unsigned long long *tcpext_TCPOFOMerge = NULL; // Number of packets in OFO that were merged with other packets.
    static unsigned long long *tcpext_OfoPruned = NULL;   // packets dropped from out-of-order queue because of socket buffer overrun

    static unsigned long long *tcpext_TCPAbortOnData = NULL;    // connections reset due to unexpected data
    static unsigned long long *tcpext_TCPAbortOnClose = NULL;   // connections reset due to early user close
    static unsigned long long *tcpext_TCPAbortOnMemory = NULL;  // connections aborted due to memory pressure
    static unsigned long long *tcpext_TCPAbortOnTimeout = NULL; // connections aborted due to timeout
    static unsigned long long *tcpext_TCPAbortOnLinger = NULL;  // connections aborted after user close in linger timeout
    static unsigned long long *tcpext_TCPAbortFailed = NULL;    // times unable to send RST due to no memory

    static unsigned long long *tcpext_TCPMemoryPressures = NULL;

/*
    // connection rejects
    static unsigned long long *tcpext_PAWSActive = NULL;  // active connections rejected because of time stamp
    static unsigned long long *tcpext_PAWSPassive = NULL; // passive connections rejected because of time stamp

    static unsigned long long *tcpext_TCPTimeouts = NULL;

    static unsigned long long *tcpext_TCPDSACKUndo = NULL;
    static unsigned long long *tcpext_TCPDSACKOldSent = NULL;
    static unsigned long long *tcpext_TCPDSACKOfoSent = NULL;
    static unsigned long long *tcpext_TCPDSACKRecv = NULL;
    static unsigned long long *tcpext_TCPDSACKOfoRecv = NULL;
    static unsigned long long *tcpext_TCPDSACKIgnoredOld = NULL;
    static unsigned long long *tcpext_TCPDSACKIgnoredNoUndo = NULL;


    static unsigned long long *tcpext_EmbryonicRsts = NULL;

    static unsigned long long *tcpext_PruneCalled = NULL;
    static unsigned long long *tcpext_RcvPruned = NULL;
    static unsigned long long *tcpext_OutOfWindowIcmps = NULL;
    static unsigned long long *tcpext_LockDroppedIcmps = NULL;
    static unsigned long long *tcpext_ArpFilter = NULL;

    static unsigned long long *tcpext_TW = NULL;
    static unsigned long long *tcpext_TWRecycled = NULL;
    static unsigned long long *tcpext_TWKilled = NULL;

    static unsigned long long *tcpext_PAWSEstab = NULL;

    static unsigned long long *tcpext_DelayedACKs = NULL;
    static unsigned long long *tcpext_DelayedACKLocked = NULL;
    static unsigned long long *tcpext_DelayedACKLost = NULL;

    static unsigned long long *tcpext_ListenOverflows = NULL;
    static unsigned long long *tcpext_ListenDrops = NULL;

    static unsigned long long *tcpext_TCPPrequeued = NULL;

    static unsigned long long *tcpext_TCPDirectCopyFromBacklog = NULL;
    static unsigned long long *tcpext_TCPDirectCopyFromPrequeue = NULL;
    static unsigned long long *tcpext_TCPPrequeueDropped = NULL;

    static unsigned long long *tcpext_TCPHPHits = NULL;
    static unsigned long long *tcpext_TCPHPHitsToUser = NULL;
    static unsigned long long *tcpext_TCPHPAcks = NULL;

    static unsigned long long *tcpext_TCPPureAcks = NULL;
    static unsigned long long *tcpext_TCPRenoRecovery = NULL;

    static unsigned long long *tcpext_TCPSackRecovery = NULL;
    static unsigned long long *tcpext_TCPSackFailures = NULL;
    static unsigned long long *tcpext_TCPSACKReneging = NULL;
    static unsigned long long *tcpext_TCPSackRecoveryFail = NULL;
    static unsigned long long *tcpext_TCPSACKDiscard = NULL;
    static unsigned long long *tcpext_TCPSackShifted = NULL;
    static unsigned long long *tcpext_TCPSackMerged = NULL;
    static unsigned long long *tcpext_TCPSackShiftFallback = NULL;


    static unsigned long long *tcpext_TCPFullUndo = NULL;
    static unsigned long long *tcpext_TCPPartialUndo = NULL;

    static unsigned long long *tcpext_TCPLossUndo = NULL;
    static unsigned long long *tcpext_TCPLostRetransmit = NULL;

    static unsigned long long *tcpext_TCPRenoFailures = NULL;

    static unsigned long long *tcpext_TCPLossFailures = NULL;
    static unsigned long long *tcpext_TCPFastRetrans = NULL;
    static unsigned long long *tcpext_TCPForwardRetrans = NULL;
    static unsigned long long *tcpext_TCPSlowStartRetrans = NULL;
    static unsigned long long *tcpext_TCPLossProbes = NULL;
    static unsigned long long *tcpext_TCPLossProbeRecovery = NULL;
    static unsigned long long *tcpext_TCPRenoRecoveryFail = NULL;
    static unsigned long long *tcpext_TCPSchedulerFailed = NULL;
    static unsigned long long *tcpext_TCPRcvCollapsed = NULL;

    static unsigned long long *tcpext_TCPSpuriousRTOs = NULL;
    static unsigned long long *tcpext_TCPMD5NotFound = NULL;
    static unsigned long long *tcpext_TCPMD5Unexpected = NULL;

    static unsigned long long *tcpext_TCPBacklogDrop = NULL;
    static unsigned long long *tcpext_TCPMinTTLDrop = NULL;
    static unsigned long long *tcpext_TCPDeferAcceptDrop = NULL;
    static unsigned long long *tcpext_IPReversePathFilter = NULL;
    static unsigned long long *tcpext_TCPTimeWaitOverflow = NULL;
    static unsigned long long *tcpext_TCPReqQFullDoCookies = NULL;
    static unsigned long long *tcpext_TCPReqQFullDrop = NULL;
    static unsigned long long *tcpext_TCPRetransFail = NULL;
    static unsigned long long *tcpext_TCPRcvCoalesce = NULL;

    static unsigned long long *tcpext_TCPChallengeACK = NULL;
    static unsigned long long *tcpext_TCPSYNChallenge = NULL;

    static unsigned long long *tcpext_TCPFastOpenActive = NULL;
    static unsigned long long *tcpext_TCPFastOpenActiveFail = NULL;
    static unsigned long long *tcpext_TCPFastOpenPassive = NULL;
    static unsigned long long *tcpext_TCPFastOpenPassiveFail = NULL;
    static unsigned long long *tcpext_TCPFastOpenListenOverflow = NULL;
    static unsigned long long *tcpext_TCPFastOpenCookieReqd = NULL;

    static unsigned long long *tcpext_TCPSpuriousRtxHostQueues = NULL;
    static unsigned long long *tcpext_BusyPollRxPackets = NULL;
    static unsigned long long *tcpext_TCPAutoCorking = NULL;
    static unsigned long long *tcpext_TCPFromZeroWindowAdv = NULL;
    static unsigned long long *tcpext_TCPToZeroWindowAdv = NULL;
    static unsigned long long *tcpext_TCPWantZeroWindowAdv = NULL;
    static unsigned long long *tcpext_TCPSynRetrans = NULL;
    static unsigned long long *tcpext_TCPOrigDataSent = NULL;

    static unsigned long long *tcpext_TCPHystartTrainDetect = NULL;
    static unsigned long long *tcpext_TCPHystartTrainCwnd = NULL;
    static unsigned long long *tcpext_TCPHystartDelayDetect = NULL;
    static unsigned long long *tcpext_TCPHystartDelayCwnd = NULL;

    static unsigned long long *tcpext_TCPACKSkippedSynRecv = NULL;
    static unsigned long long *tcpext_TCPACKSkippedPAWS = NULL;
    static unsigned long long *tcpext_TCPACKSkippedSeq = NULL;
    static unsigned long long *tcpext_TCPACKSkippedFinWait2 = NULL;
    static unsigned long long *tcpext_TCPACKSkippedTimeWait = NULL;
    static unsigned long long *tcpext_TCPACKSkippedChallenge = NULL;

    static unsigned long long *tcpext_TCPWinProbe = NULL;
    static unsigned long long *tcpext_TCPKeepAlive = NULL;

    static unsigned long long *tcpext_TCPMTUPFail = NULL;
    static unsigned long long *tcpext_TCPMTUPSuccess = NULL;
*/

    static unsigned long long *ipext_InNoRoutes = NULL;
    static unsigned long long *ipext_InTruncatedPkts = NULL;
    static unsigned long long *ipext_InMcastPkts = NULL;
    static unsigned long long *ipext_OutMcastPkts = NULL;
    static unsigned long long *ipext_InBcastPkts = NULL;
    static unsigned long long *ipext_OutBcastPkts = NULL;
    static unsigned long long *ipext_InOctets = NULL;
    static unsigned long long *ipext_OutOctets = NULL;
    static unsigned long long *ipext_InMcastOctets = NULL;
    static unsigned long long *ipext_OutMcastOctets = NULL;
    static unsigned long long *ipext_InBcastOctets = NULL;
    static unsigned long long *ipext_OutBcastOctets = NULL;
    static unsigned long long *ipext_InCsumErrors = NULL;
    static unsigned long long *ipext_InNoECTPkts = NULL;
    static unsigned long long *ipext_InECT1Pkts = NULL;
    static unsigned long long *ipext_InECT0Pkts = NULL;
    static unsigned long long *ipext_InCEPkts = NULL;

    if(unlikely(do_bandwidth == -1)) {
        do_bandwidth = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "bandwidth", CONFIG_ONDEMAND_ONDEMAND);
        do_inerrors  = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "input errors", CONFIG_ONDEMAND_ONDEMAND);
        do_mcast     = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "multicast bandwidth", CONFIG_ONDEMAND_ONDEMAND);
        do_bcast     = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "broadcast bandwidth", CONFIG_ONDEMAND_ONDEMAND);
        do_mcast_p   = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "multicast packets", CONFIG_ONDEMAND_ONDEMAND);
        do_bcast_p   = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "broadcast packets", CONFIG_ONDEMAND_ONDEMAND);
        do_ecn       = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "ECN packets", CONFIG_ONDEMAND_ONDEMAND);

        do_tcpext_reorder    = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP reorders", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_syscookies = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP SYN cookies", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_ofo        = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP out-of-order queue", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_connaborts = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP connection aborts", CONFIG_ONDEMAND_ONDEMAND);
        do_tcpext_memory     = config_get_boolean_ondemand("plugin:proc:/proc/net/netstat", "TCP memory pressures", CONFIG_ONDEMAND_ONDEMAND);

        hash_ipext = simple_hash("IpExt");
        hash_tcpext = simple_hash("TcpExt");

        hash_array(ipext_data);
        hash_array(tcpext_data);

        // Reordering
        tcpext_TCPFACKReorder = netstat_columns_find(tcpext_data, "TCPFACKReorder");
        tcpext_TCPSACKReorder = netstat_columns_find(tcpext_data, "TCPSACKReorder");
        tcpext_TCPRenoReorder = netstat_columns_find(tcpext_data, "TCPRenoReorder");
        tcpext_TCPTSReorder = netstat_columns_find(tcpext_data, "TCPTSReorder");

        // SYN Cookies
        tcpext_SyncookiesSent = netstat_columns_find(tcpext_data, "SyncookiesSent");
        tcpext_SyncookiesRecv = netstat_columns_find(tcpext_data, "SyncookiesRecv");
        tcpext_SyncookiesFailed = netstat_columns_find(tcpext_data, "SyncookiesFailed");

        // Out Of Order Queue
        // http://www.spinics.net/lists/netdev/msg204696.html
        tcpext_TCPOFOQueue = netstat_columns_find(tcpext_data, "TCPOFOQueue"); // Number of packets queued in OFO queue
        tcpext_TCPOFODrop  = netstat_columns_find(tcpext_data, "TCPOFODrop");  // Number of packets meant to be queued in OFO but dropped because socket rcvbuf limit hit.
        tcpext_TCPOFOMerge = netstat_columns_find(tcpext_data, "TCPOFOMerge"); // Number of packets in OFO that were merged with other packets.
        tcpext_OfoPruned   = netstat_columns_find(tcpext_data, "OfoPruned");   // packets dropped from out-of-order queue because of socket buffer overrun

        // connection resets
        // https://github.com/ecki/net-tools/blob/bd8bceaed2311651710331a7f8990c3e31be9840/statistics.c
        tcpext_TCPAbortOnData    = netstat_columns_find(tcpext_data, "TCPAbortOnData");    // connections reset due to unexpected data
        tcpext_TCPAbortOnClose   = netstat_columns_find(tcpext_data, "TCPAbortOnClose");   // connections reset due to early user close
        tcpext_TCPAbortOnMemory  = netstat_columns_find(tcpext_data, "TCPAbortOnMemory");  // connections aborted due to memory pressure
        tcpext_TCPAbortOnTimeout = netstat_columns_find(tcpext_data, "TCPAbortOnTimeout"); // connections aborted due to timeout
        tcpext_TCPAbortOnLinger  = netstat_columns_find(tcpext_data, "TCPAbortOnLinger");  // connections aborted after user close in linger timeout
        tcpext_TCPAbortFailed    = netstat_columns_find(tcpext_data, "TCPAbortFailed");    // times unable to send RST due to no memory

        tcpext_TCPMemoryPressures = netstat_columns_find(tcpext_data, "TCPMemoryPressures");

        /*
        tcpext_EmbryonicRsts = netstat_columns_find(tcpext_data, "EmbryonicRsts");
        tcpext_PruneCalled = netstat_columns_find(tcpext_data, "PruneCalled");
        tcpext_RcvPruned = netstat_columns_find(tcpext_data, "RcvPruned");
        tcpext_OutOfWindowIcmps = netstat_columns_find(tcpext_data, "OutOfWindowIcmps");
        tcpext_LockDroppedIcmps = netstat_columns_find(tcpext_data, "LockDroppedIcmps");
        tcpext_ArpFilter = netstat_columns_find(tcpext_data, "ArpFilter");
        tcpext_TW = netstat_columns_find(tcpext_data, "TW");
        tcpext_TWRecycled = netstat_columns_find(tcpext_data, "TWRecycled");
        tcpext_TWKilled = netstat_columns_find(tcpext_data, "TWKilled");
        tcpext_PAWSPassive = netstat_columns_find(tcpext_data, "PAWSPassive");
        tcpext_PAWSActive = netstat_columns_find(tcpext_data, "PAWSActive");
        tcpext_PAWSEstab = netstat_columns_find(tcpext_data, "PAWSEstab");
        tcpext_DelayedACKs = netstat_columns_find(tcpext_data, "DelayedACKs");
        tcpext_DelayedACKLocked = netstat_columns_find(tcpext_data, "DelayedACKLocked");
        tcpext_DelayedACKLost = netstat_columns_find(tcpext_data, "DelayedACKLost");
        tcpext_ListenOverflows = netstat_columns_find(tcpext_data, "ListenOverflows");
        tcpext_ListenDrops = netstat_columns_find(tcpext_data, "ListenDrops");
        tcpext_TCPPrequeued = netstat_columns_find(tcpext_data, "TCPPrequeued");
        tcpext_TCPDirectCopyFromBacklog = netstat_columns_find(tcpext_data, "TCPDirectCopyFromBacklog");
        tcpext_TCPDirectCopyFromPrequeue = netstat_columns_find(tcpext_data, "TCPDirectCopyFromPrequeue");
        tcpext_TCPPrequeueDropped = netstat_columns_find(tcpext_data, "TCPPrequeueDropped");
        tcpext_TCPHPHits = netstat_columns_find(tcpext_data, "TCPHPHits");
        tcpext_TCPHPHitsToUser = netstat_columns_find(tcpext_data, "TCPHPHitsToUser");
        tcpext_TCPPureAcks = netstat_columns_find(tcpext_data, "TCPPureAcks");
        tcpext_TCPHPAcks = netstat_columns_find(tcpext_data, "TCPHPAcks");
        tcpext_TCPRenoRecovery = netstat_columns_find(tcpext_data, "TCPRenoRecovery");
        tcpext_TCPSackRecovery = netstat_columns_find(tcpext_data, "TCPSackRecovery");
        tcpext_TCPSACKReneging = netstat_columns_find(tcpext_data, "TCPSACKReneging");
        tcpext_TCPFullUndo = netstat_columns_find(tcpext_data, "TCPFullUndo");
        tcpext_TCPPartialUndo = netstat_columns_find(tcpext_data, "TCPPartialUndo");
        tcpext_TCPDSACKUndo = netstat_columns_find(tcpext_data, "TCPDSACKUndo");
        tcpext_TCPLossUndo = netstat_columns_find(tcpext_data, "TCPLossUndo");
        tcpext_TCPLostRetransmit = netstat_columns_find(tcpext_data, "TCPLostRetransmit");
        tcpext_TCPRenoFailures = netstat_columns_find(tcpext_data, "TCPRenoFailures");
        tcpext_TCPSackFailures = netstat_columns_find(tcpext_data, "TCPSackFailures");
        tcpext_TCPLossFailures = netstat_columns_find(tcpext_data, "TCPLossFailures");
        tcpext_TCPFastRetrans = netstat_columns_find(tcpext_data, "TCPFastRetrans");
        tcpext_TCPForwardRetrans = netstat_columns_find(tcpext_data, "TCPForwardRetrans");
        tcpext_TCPSlowStartRetrans = netstat_columns_find(tcpext_data, "TCPSlowStartRetrans");
        tcpext_TCPTimeouts = netstat_columns_find(tcpext_data, "TCPTimeouts");
        tcpext_TCPLossProbes = netstat_columns_find(tcpext_data, "TCPLossProbes");
        tcpext_TCPLossProbeRecovery = netstat_columns_find(tcpext_data, "TCPLossProbeRecovery");
        tcpext_TCPRenoRecoveryFail = netstat_columns_find(tcpext_data, "TCPRenoRecoveryFail");
        tcpext_TCPSackRecoveryFail = netstat_columns_find(tcpext_data, "TCPSackRecoveryFail");
        tcpext_TCPSchedulerFailed = netstat_columns_find(tcpext_data, "TCPSchedulerFailed");
        tcpext_TCPRcvCollapsed = netstat_columns_find(tcpext_data, "TCPRcvCollapsed");
        tcpext_TCPDSACKOldSent = netstat_columns_find(tcpext_data, "TCPDSACKOldSent");
        tcpext_TCPDSACKOfoSent = netstat_columns_find(tcpext_data, "TCPDSACKOfoSent");
        tcpext_TCPDSACKRecv = netstat_columns_find(tcpext_data, "TCPDSACKRecv");
        tcpext_TCPDSACKOfoRecv = netstat_columns_find(tcpext_data, "TCPDSACKOfoRecv");
        tcpext_TCPSACKDiscard = netstat_columns_find(tcpext_data, "TCPSACKDiscard");
        tcpext_TCPDSACKIgnoredOld = netstat_columns_find(tcpext_data, "TCPDSACKIgnoredOld");
        tcpext_TCPDSACKIgnoredNoUndo = netstat_columns_find(tcpext_data, "TCPDSACKIgnoredNoUndo");
        tcpext_TCPSpuriousRTOs = netstat_columns_find(tcpext_data, "TCPSpuriousRTOs");
        tcpext_TCPMD5NotFound = netstat_columns_find(tcpext_data, "TCPMD5NotFound");
        tcpext_TCPMD5Unexpected = netstat_columns_find(tcpext_data, "TCPMD5Unexpected");
        tcpext_TCPSackShifted = netstat_columns_find(tcpext_data, "TCPSackShifted");
        tcpext_TCPSackMerged = netstat_columns_find(tcpext_data, "TCPSackMerged");
        tcpext_TCPSackShiftFallback = netstat_columns_find(tcpext_data, "TCPSackShiftFallback");
        tcpext_TCPBacklogDrop = netstat_columns_find(tcpext_data, "TCPBacklogDrop");
        tcpext_TCPMinTTLDrop = netstat_columns_find(tcpext_data, "TCPMinTTLDrop");
        tcpext_TCPDeferAcceptDrop = netstat_columns_find(tcpext_data, "TCPDeferAcceptDrop");
        tcpext_IPReversePathFilter = netstat_columns_find(tcpext_data, "IPReversePathFilter");
        tcpext_TCPTimeWaitOverflow = netstat_columns_find(tcpext_data, "TCPTimeWaitOverflow");
        tcpext_TCPReqQFullDoCookies = netstat_columns_find(tcpext_data, "TCPReqQFullDoCookies");
        tcpext_TCPReqQFullDrop = netstat_columns_find(tcpext_data, "TCPReqQFullDrop");
        tcpext_TCPRetransFail = netstat_columns_find(tcpext_data, "TCPRetransFail");
        tcpext_TCPRcvCoalesce = netstat_columns_find(tcpext_data, "TCPRcvCoalesce");
        tcpext_TCPChallengeACK = netstat_columns_find(tcpext_data, "TCPChallengeACK");
        tcpext_TCPSYNChallenge = netstat_columns_find(tcpext_data, "TCPSYNChallenge");
        tcpext_TCPFastOpenActive = netstat_columns_find(tcpext_data, "TCPFastOpenActive");
        tcpext_TCPFastOpenActiveFail = netstat_columns_find(tcpext_data, "TCPFastOpenActiveFail");
        tcpext_TCPFastOpenPassive = netstat_columns_find(tcpext_data, "TCPFastOpenPassive");
        tcpext_TCPFastOpenPassiveFail = netstat_columns_find(tcpext_data, "TCPFastOpenPassiveFail");
        tcpext_TCPFastOpenListenOverflow = netstat_columns_find(tcpext_data, "TCPFastOpenListenOverflow");
        tcpext_TCPFastOpenCookieReqd = netstat_columns_find(tcpext_data, "TCPFastOpenCookieReqd");
        tcpext_TCPSpuriousRtxHostQueues = netstat_columns_find(tcpext_data, "TCPSpuriousRtxHostQueues");
        tcpext_BusyPollRxPackets = netstat_columns_find(tcpext_data, "BusyPollRxPackets");
        tcpext_TCPAutoCorking = netstat_columns_find(tcpext_data, "TCPAutoCorking");
        tcpext_TCPFromZeroWindowAdv = netstat_columns_find(tcpext_data, "TCPFromZeroWindowAdv");
        tcpext_TCPToZeroWindowAdv = netstat_columns_find(tcpext_data, "TCPToZeroWindowAdv");
        tcpext_TCPWantZeroWindowAdv = netstat_columns_find(tcpext_data, "TCPWantZeroWindowAdv");
        tcpext_TCPSynRetrans = netstat_columns_find(tcpext_data, "TCPSynRetrans");
        tcpext_TCPOrigDataSent = netstat_columns_find(tcpext_data, "TCPOrigDataSent");
        tcpext_TCPHystartTrainDetect = netstat_columns_find(tcpext_data, "TCPHystartTrainDetect");
        tcpext_TCPHystartTrainCwnd = netstat_columns_find(tcpext_data, "TCPHystartTrainCwnd");
        tcpext_TCPHystartDelayDetect = netstat_columns_find(tcpext_data, "TCPHystartDelayDetect");
        tcpext_TCPHystartDelayCwnd = netstat_columns_find(tcpext_data, "TCPHystartDelayCwnd");
        tcpext_TCPACKSkippedSynRecv = netstat_columns_find(tcpext_data, "TCPACKSkippedSynRecv");
        tcpext_TCPACKSkippedPAWS = netstat_columns_find(tcpext_data, "TCPACKSkippedPAWS");
        tcpext_TCPACKSkippedSeq = netstat_columns_find(tcpext_data, "TCPACKSkippedSeq");
        tcpext_TCPACKSkippedFinWait2 = netstat_columns_find(tcpext_data, "TCPACKSkippedFinWait2");
        tcpext_TCPACKSkippedTimeWait = netstat_columns_find(tcpext_data, "TCPACKSkippedTimeWait");
        tcpext_TCPACKSkippedChallenge = netstat_columns_find(tcpext_data, "TCPACKSkippedChallenge");
        tcpext_TCPWinProbe = netstat_columns_find(tcpext_data, "TCPWinProbe");
        tcpext_TCPKeepAlive = netstat_columns_find(tcpext_data, "TCPKeepAlive");
        tcpext_TCPMTUPFail = netstat_columns_find(tcpext_data, "TCPMTUPFail");
        tcpext_TCPMTUPSuccess = netstat_columns_find(tcpext_data, "TCPMTUPSuccess");
*/
        ipext_InNoRoutes = netstat_columns_find(ipext_data, "InNoRoutes");
        ipext_InTruncatedPkts = netstat_columns_find(ipext_data, "InTruncatedPkts");
        ipext_InMcastPkts = netstat_columns_find(ipext_data, "InMcastPkts");
        ipext_OutMcastPkts = netstat_columns_find(ipext_data, "OutMcastPkts");
        ipext_InBcastPkts = netstat_columns_find(ipext_data, "InBcastPkts");
        ipext_OutBcastPkts = netstat_columns_find(ipext_data, "OutBcastPkts");
        ipext_InOctets = netstat_columns_find(ipext_data, "InOctets");
        ipext_OutOctets = netstat_columns_find(ipext_data, "OutOctets");
        ipext_InMcastOctets = netstat_columns_find(ipext_data, "InMcastOctets");
        ipext_OutMcastOctets = netstat_columns_find(ipext_data, "OutMcastOctets");
        ipext_InBcastOctets = netstat_columns_find(ipext_data, "InBcastOctets");
        ipext_OutBcastOctets = netstat_columns_find(ipext_data, "OutBcastOctets");
        ipext_InCsumErrors = netstat_columns_find(ipext_data, "InCsumErrors");
        ipext_InNoECTPkts = netstat_columns_find(ipext_data, "InNoECTPkts");
        ipext_InECT1Pkts = netstat_columns_find(ipext_data, "InECT1Pkts");
        ipext_InECT0Pkts = netstat_columns_find(ipext_data, "InECT0Pkts");
        ipext_InCEPkts = netstat_columns_find(ipext_data, "InCEPkts");
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/net/netstat");
        ff = procfile_open(config_get("plugin:proc:/proc/net/netstat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    uint32_t lines = procfile_lines(ff), l;
    uint32_t words;

    for(l = 0; l < lines ;l++) {
        char *key = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(key);

        if(unlikely(hash == hash_ipext && strcmp(key, "IpExt") == 0)) {
            uint32_t h = l++;

            if(unlikely(strcmp(procfile_lineword(ff, l, 0), "IpExt") != 0)) {
                error("Cannot read IpExt line from /proc/net/netstat.");
                break;
            }
            words = procfile_linewords(ff, l);
            if(unlikely(words < 2)) {
                error("Cannot read /proc/net/netstat IpExt line. Expected 2+ params, read %u.", words);
                continue;
            }

            parse_line_pair(ff, ipext_data, h, l);

            RRDSET *st;

            // --------------------------------------------------------------------

            if(do_bandwidth == CONFIG_ONDEMAND_YES || (do_bandwidth == CONFIG_ONDEMAND_ONDEMAND && (*ipext_InOctets || *ipext_OutOctets))) {
                do_bandwidth = CONFIG_ONDEMAND_YES;
                st = rrdset_find("system.ipv4");
                if(unlikely(!st)) {
                    st = rrdset_create("system", "ipv4", NULL, "network", NULL, "IPv4 Bandwidth", "kilobits/s", 500, update_every, RRDSET_TYPE_AREA);

                    rrddim_add(st, "InOctets", "received", 8, 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutOctets", "sent", -8, 1024, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InOctets", *ipext_InOctets);
                rrddim_set(st, "OutOctets", *ipext_OutOctets);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_inerrors == CONFIG_ONDEMAND_YES || (do_inerrors == CONFIG_ONDEMAND_ONDEMAND && (*ipext_InNoRoutes || *ipext_InTruncatedPkts))) {
                do_inerrors = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.inerrors");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "inerrors", NULL, "errors", NULL, "IPv4 Input Errors", "packets/s", 4000, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InNoRoutes", "noroutes", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InTruncatedPkts", "truncated", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InCsumErrors", "checksum", 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InNoRoutes", *ipext_InNoRoutes);
                rrddim_set(st, "InTruncatedPkts", *ipext_InTruncatedPkts);
                rrddim_set(st, "InCsumErrors", *ipext_InCsumErrors);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_mcast == CONFIG_ONDEMAND_YES || (do_mcast == CONFIG_ONDEMAND_ONDEMAND && (*ipext_InMcastOctets || *ipext_OutMcastOctets))) {
                do_mcast = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.mcast");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "mcast", NULL, "multicast", NULL, "IPv4 Multicast Bandwidth", "kilobits/s", 9000, update_every, RRDSET_TYPE_AREA);
                    st->isdetail = 1;

                    rrddim_add(st, "InMcastOctets", "received", 8, 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutMcastOctets", "sent", -8, 1024, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InMcastOctets", *ipext_InMcastOctets);
                rrddim_set(st, "OutMcastOctets", *ipext_OutMcastOctets);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_bcast == CONFIG_ONDEMAND_YES || (do_bcast == CONFIG_ONDEMAND_ONDEMAND && (*ipext_InBcastOctets || *ipext_OutBcastOctets))) {
                do_bcast = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.bcast");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "bcast", NULL, "broadcast", NULL, "IPv4 Broadcast Bandwidth", "kilobits/s", 8000, update_every, RRDSET_TYPE_AREA);
                    st->isdetail = 1;

                    rrddim_add(st, "InBcastOctets", "received", 8, 1024, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutBcastOctets", "sent", -8, 1024, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InBcastOctets", *ipext_InBcastOctets);
                rrddim_set(st, "OutBcastOctets", *ipext_OutBcastOctets);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_mcast_p == CONFIG_ONDEMAND_YES || (do_mcast_p == CONFIG_ONDEMAND_ONDEMAND && (*ipext_InMcastPkts || *ipext_OutMcastPkts))) {
                do_mcast_p = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.mcastpkts");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "mcastpkts", NULL, "multicast", NULL, "IPv4 Multicast Packets", "packets/s", 8600, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InMcastPkts", "received", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutMcastPkts", "sent", -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InMcastPkts", *ipext_InMcastPkts);
                rrddim_set(st, "OutMcastPkts", *ipext_OutMcastPkts);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_bcast_p == CONFIG_ONDEMAND_YES || (do_bcast_p == CONFIG_ONDEMAND_ONDEMAND && (*ipext_InBcastPkts || *ipext_OutBcastPkts))) {
                do_bcast_p = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.bcastpkts");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "bcastpkts", NULL, "broadcast", NULL, "IPv4 Broadcast Packets", "packets/s", 8500, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InBcastPkts", "received", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OutBcastPkts", "sent", -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InBcastPkts", *ipext_InBcastPkts);
                rrddim_set(st, "OutBcastPkts", *ipext_OutBcastPkts);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_ecn == CONFIG_ONDEMAND_YES || (do_ecn == CONFIG_ONDEMAND_ONDEMAND && (*ipext_InCEPkts || *ipext_InECT0Pkts || *ipext_InECT1Pkts || *ipext_InNoECTPkts))) {
                do_ecn = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.ecnpkts");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "ecnpkts", NULL, "ecn", NULL, "IPv4 ECN Statistics", "packets/s", 8700, update_every, RRDSET_TYPE_LINE);
                    st->isdetail = 1;

                    rrddim_add(st, "InCEPkts", "CEP", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InNoECTPkts", "NoECTP", -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InECT0Pkts", "ECTP0", 1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "InECT1Pkts", "ECTP1", 1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "InCEPkts", *ipext_InCEPkts);
                rrddim_set(st, "InNoECTPkts", *ipext_InNoECTPkts);
                rrddim_set(st, "InECT0Pkts", *ipext_InECT0Pkts);
                rrddim_set(st, "InECT1Pkts", *ipext_InECT1Pkts);
                rrdset_done(st);
            }
        }
        else if(unlikely(hash == hash_tcpext && strcmp(key, "TcpExt") == 0)) {
            uint32_t h = l++;

            if(unlikely(strcmp(procfile_lineword(ff, l, 0), "TcpExt") != 0)) {
                error("Cannot read TcpExt line from /proc/net/netstat.");
                break;
            }
            words = procfile_linewords(ff, l);
            if(unlikely(words < 2)) {
                error("Cannot read /proc/net/netstat TcpExt line. Expected 2+ params, read %u.", words);
                continue;
            }

            parse_line_pair(ff, tcpext_data, h, l);

            RRDSET *st;

            // --------------------------------------------------------------------

            if(do_tcpext_memory == CONFIG_ONDEMAND_YES || (do_tcpext_memory == CONFIG_ONDEMAND_ONDEMAND && (*tcpext_TCPMemoryPressures))) {
                do_tcpext_memory = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.tcpmemorypressures");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpmemorypressures", NULL, "tcp", NULL, "TCP Memory Pressures", "events/s", 3000, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPMemoryPressures",   "pressures",  1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPMemoryPressures", *tcpext_TCPMemoryPressures);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_connaborts == CONFIG_ONDEMAND_YES || (do_tcpext_connaborts == CONFIG_ONDEMAND_ONDEMAND && (*tcpext_TCPAbortOnData || *tcpext_TCPAbortOnClose || *tcpext_TCPAbortOnMemory || *tcpext_TCPAbortOnTimeout || *tcpext_TCPAbortOnLinger || *tcpext_TCPAbortFailed))) {
                do_tcpext_connaborts = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.tcpconnaborts");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpconnaborts", NULL, "tcp", NULL, "TCP Connection Aborts", "connections/s", 3010, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPAbortOnData",    "baddata",     1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnClose",   "userclosed",  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnMemory",  "nomemory",    1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnTimeout", "timeout",     1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortOnLinger",  "linger",      1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPAbortFailed",    "failed",     -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPAbortOnData",    *tcpext_TCPAbortOnData);
                rrddim_set(st, "TCPAbortOnClose",   *tcpext_TCPAbortOnClose);
                rrddim_set(st, "TCPAbortOnMemory",  *tcpext_TCPAbortOnMemory);
                rrddim_set(st, "TCPAbortOnTimeout", *tcpext_TCPAbortOnTimeout);
                rrddim_set(st, "TCPAbortOnLinger",  *tcpext_TCPAbortOnLinger);
                rrddim_set(st, "TCPAbortFailed",    *tcpext_TCPAbortFailed);
                rrdset_done(st);
            }
            // --------------------------------------------------------------------

            if(do_tcpext_reorder == CONFIG_ONDEMAND_YES || (do_tcpext_reorder == CONFIG_ONDEMAND_ONDEMAND && (*tcpext_TCPRenoReorder || *tcpext_TCPFACKReorder || *tcpext_TCPSACKReorder || *tcpext_TCPTSReorder))) {
                do_tcpext_reorder = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.tcpreorders");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpreorders", NULL, "tcp", NULL, "TCP Reordered Packets by Detection Method", "packets/s", 3020, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPTSReorder",   "timestamp",   1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPSACKReorder", "sack",        1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPFACKReorder", "fack",        1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPRenoReorder", "reno",        1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPTSReorder",   *tcpext_TCPTSReorder);
                rrddim_set(st, "TCPSACKReorder", *tcpext_TCPSACKReorder);
                rrddim_set(st, "TCPFACKReorder", *tcpext_TCPFACKReorder);
                rrddim_set(st, "TCPRenoReorder", *tcpext_TCPRenoReorder);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_ofo == CONFIG_ONDEMAND_YES || (do_tcpext_ofo == CONFIG_ONDEMAND_ONDEMAND && (*tcpext_TCPOFOQueue || *tcpext_TCPOFODrop || *tcpext_TCPOFOMerge))) {
                do_tcpext_ofo = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.tcpofo");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpofo", NULL, "tcp", NULL, "TCP Out-Of-Order Queue", "packets/s", 3050, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "TCPOFOQueue", "inqueue",  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPOFODrop",  "dropped", -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "TCPOFOMerge", "merged",   1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "OfoPruned",   "pruned",  -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "TCPOFOQueue",   *tcpext_TCPOFOQueue);
                rrddim_set(st, "TCPOFODrop",    *tcpext_TCPOFODrop);
                rrddim_set(st, "TCPOFOMerge",   *tcpext_TCPOFOMerge);
                rrddim_set(st, "OfoPruned",     *tcpext_OfoPruned);
                rrdset_done(st);
            }

            // --------------------------------------------------------------------

            if(do_tcpext_syscookies == CONFIG_ONDEMAND_YES || (do_tcpext_syscookies == CONFIG_ONDEMAND_ONDEMAND && (*tcpext_SyncookiesSent || *tcpext_SyncookiesRecv || *tcpext_SyncookiesFailed))) {
                do_tcpext_syscookies = CONFIG_ONDEMAND_YES;
                st = rrdset_find("ipv4.tcpsyncookies");
                if(unlikely(!st)) {
                    st = rrdset_create("ipv4", "tcpsyncookies", NULL, "tcp", NULL, "TCP SYN Cookies", "packets/s", 3100, update_every, RRDSET_TYPE_LINE);

                    rrddim_add(st, "SyncookiesRecv",   "received",  1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesSent",   "sent",     -1, 1, RRDDIM_INCREMENTAL);
                    rrddim_add(st, "SyncookiesFailed", "failed",   -1, 1, RRDDIM_INCREMENTAL);
                }
                else rrdset_next(st);

                rrddim_set(st, "SyncookiesRecv",   *tcpext_SyncookiesRecv);
                rrddim_set(st, "SyncookiesSent",   *tcpext_SyncookiesSent);
                rrddim_set(st, "SyncookiesFailed", *tcpext_SyncookiesFailed);
                rrdset_done(st);
            }

        }
    }

    return 0;
}
