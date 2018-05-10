#include "common.h"

int do_proc_net_sctp_snmp(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;

    static int
            do_associations = -1,
            do_transitions = -1,
            do_packet_errors = -1,
            do_packets = -1,
            do_fragmentation = -1,
            do_chunk_types = -1;

    static ARL_BASE *arl_base = NULL;

    static unsigned long long SctpCurrEstab               = 0ULL;
    static unsigned long long SctpActiveEstabs            = 0ULL;
    static unsigned long long SctpPassiveEstabs           = 0ULL;
    static unsigned long long SctpAborteds                = 0ULL;
    static unsigned long long SctpShutdowns               = 0ULL;
    static unsigned long long SctpOutOfBlues              = 0ULL;
    static unsigned long long SctpChecksumErrors          = 0ULL;
    static unsigned long long SctpOutCtrlChunks           = 0ULL;
    static unsigned long long SctpOutOrderChunks          = 0ULL;
    static unsigned long long SctpOutUnorderChunks        = 0ULL;
    static unsigned long long SctpInCtrlChunks            = 0ULL;
    static unsigned long long SctpInOrderChunks           = 0ULL;
    static unsigned long long SctpInUnorderChunks         = 0ULL;
    static unsigned long long SctpFragUsrMsgs             = 0ULL;
    static unsigned long long SctpReasmUsrMsgs            = 0ULL;
    static unsigned long long SctpOutSCTPPacks            = 0ULL;
    static unsigned long long SctpInSCTPPacks             = 0ULL;
    static unsigned long long SctpT1InitExpireds          = 0ULL;
    static unsigned long long SctpT1CookieExpireds        = 0ULL;
    static unsigned long long SctpT2ShutdownExpireds      = 0ULL;
    static unsigned long long SctpT3RtxExpireds           = 0ULL;
    static unsigned long long SctpT4RtoExpireds           = 0ULL;
    static unsigned long long SctpT5ShutdownGuardExpireds = 0ULL;
    static unsigned long long SctpDelaySackExpireds       = 0ULL;
    static unsigned long long SctpAutocloseExpireds       = 0ULL;
    static unsigned long long SctpT3Retransmits           = 0ULL;
    static unsigned long long SctpPmtudRetransmits        = 0ULL;
    static unsigned long long SctpFastRetransmits         = 0ULL;
    static unsigned long long SctpInPktSoftirq            = 0ULL;
    static unsigned long long SctpInPktBacklog            = 0ULL;
    static unsigned long long SctpInPktDiscards           = 0ULL;
    static unsigned long long SctpInDataChunkDiscards     = 0ULL;

    if(unlikely(!arl_base)) {
        do_associations = config_get_boolean_ondemand("plugin:proc:/proc/net/sctp/snmp", "established associations", CONFIG_BOOLEAN_AUTO);
        do_transitions = config_get_boolean_ondemand("plugin:proc:/proc/net/sctp/snmp", "association transitions", CONFIG_BOOLEAN_AUTO);
        do_fragmentation = config_get_boolean_ondemand("plugin:proc:/proc/net/sctp/snmp", "fragmentation", CONFIG_BOOLEAN_AUTO);
        do_packets = config_get_boolean_ondemand("plugin:proc:/proc/net/sctp/snmp", "packets", CONFIG_BOOLEAN_AUTO);
        do_packet_errors = config_get_boolean_ondemand("plugin:proc:/proc/net/sctp/snmp", "packet errors", CONFIG_BOOLEAN_AUTO);
        do_chunk_types = config_get_boolean_ondemand("plugin:proc:/proc/net/sctp/snmp", "chunk types", CONFIG_BOOLEAN_AUTO);

        arl_base = arl_create("sctp", NULL, 60);
        arl_expect(arl_base, "SctpCurrEstab", &SctpCurrEstab);
        arl_expect(arl_base, "SctpActiveEstabs", &SctpActiveEstabs);
        arl_expect(arl_base, "SctpPassiveEstabs", &SctpPassiveEstabs);
        arl_expect(arl_base, "SctpAborteds", &SctpAborteds);
        arl_expect(arl_base, "SctpShutdowns", &SctpShutdowns);
        arl_expect(arl_base, "SctpOutOfBlues", &SctpOutOfBlues);
        arl_expect(arl_base, "SctpChecksumErrors", &SctpChecksumErrors);
        arl_expect(arl_base, "SctpOutCtrlChunks", &SctpOutCtrlChunks);
        arl_expect(arl_base, "SctpOutOrderChunks", &SctpOutOrderChunks);
        arl_expect(arl_base, "SctpOutUnorderChunks", &SctpOutUnorderChunks);
        arl_expect(arl_base, "SctpInCtrlChunks", &SctpInCtrlChunks);
        arl_expect(arl_base, "SctpInOrderChunks", &SctpInOrderChunks);
        arl_expect(arl_base, "SctpInUnorderChunks", &SctpInUnorderChunks);
        arl_expect(arl_base, "SctpFragUsrMsgs", &SctpFragUsrMsgs);
        arl_expect(arl_base, "SctpReasmUsrMsgs", &SctpReasmUsrMsgs);
        arl_expect(arl_base, "SctpOutSCTPPacks", &SctpOutSCTPPacks);
        arl_expect(arl_base, "SctpInSCTPPacks", &SctpInSCTPPacks);
        arl_expect(arl_base, "SctpT1InitExpireds", &SctpT1InitExpireds);
        arl_expect(arl_base, "SctpT1CookieExpireds", &SctpT1CookieExpireds);
        arl_expect(arl_base, "SctpT2ShutdownExpireds", &SctpT2ShutdownExpireds);
        arl_expect(arl_base, "SctpT3RtxExpireds", &SctpT3RtxExpireds);
        arl_expect(arl_base, "SctpT4RtoExpireds", &SctpT4RtoExpireds);
        arl_expect(arl_base, "SctpT5ShutdownGuardExpireds", &SctpT5ShutdownGuardExpireds);
        arl_expect(arl_base, "SctpDelaySackExpireds", &SctpDelaySackExpireds);
        arl_expect(arl_base, "SctpAutocloseExpireds", &SctpAutocloseExpireds);
        arl_expect(arl_base, "SctpT3Retransmits", &SctpT3Retransmits);
        arl_expect(arl_base, "SctpPmtudRetransmits", &SctpPmtudRetransmits);
        arl_expect(arl_base, "SctpFastRetransmits", &SctpFastRetransmits);
        arl_expect(arl_base, "SctpInPktSoftirq", &SctpInPktSoftirq);
        arl_expect(arl_base, "SctpInPktBacklog", &SctpInPktBacklog);
        arl_expect(arl_base, "SctpInPktDiscards", &SctpInPktDiscards);
        arl_expect(arl_base, "SctpInDataChunkDiscards", &SctpInDataChunkDiscards);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/sctp/snmp");
        ff = procfile_open(config_get("plugin:proc:/proc/net/sctp/snmp", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff))
            return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    arl_begin(arl_base);

    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 2)) {
            if(unlikely(words)) error("Cannot read /proc/net/sctp/snmp line %zu. Expected 2 params, read %zu.", l, words);
            continue;
        }

        if(unlikely(arl_check(arl_base,
                procfile_lineword(ff, l, 0),
                procfile_lineword(ff, l, 1)))) break;
    }

    // --------------------------------------------------------------------

    if(do_associations == CONFIG_BOOLEAN_YES || (do_associations == CONFIG_BOOLEAN_AUTO && SctpCurrEstab)) {
        do_associations = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_established = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "sctp"
                    , "established"
                    , NULL
                    , "associations"
                    , NULL
                    , "SCTP current total number of established associations"
                    , "associations"
                    , "proc"
                    , "net/sctp/snmp"
                    , NETDATA_CHART_PRIO_SCTP
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_established = rrddim_add(st, "SctpCurrEstab",  "established",  1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_established, SctpCurrEstab);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_transitions == CONFIG_BOOLEAN_YES || (do_transitions == CONFIG_BOOLEAN_AUTO && (SctpActiveEstabs || SctpPassiveEstabs || SctpAborteds || SctpShutdowns))) {
        do_transitions = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_active = NULL,
                      *rd_passive = NULL,
                      *rd_aborted = NULL,
                      *rd_shutdown = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "sctp"
                    , "transitions"
                    , NULL
                    , "transitions"
                    , NULL
                    , "SCTP Association Transitions"
                    , "transitions/s"
                    , "proc"
                    , "net/sctp/snmp"
                    , NETDATA_CHART_PRIO_SCTP + 10
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_active   = rrddim_add(st, "SctpActiveEstabs",  "active",    1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_passive  = rrddim_add(st, "SctpPassiveEstabs", "passive",   1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_aborted  = rrddim_add(st, "SctpAborteds",      "aborted",  -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_shutdown = rrddim_add(st, "SctpShutdowns",     "shutdown", -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_active,    SctpActiveEstabs);
        rrddim_set_by_pointer(st, rd_passive,   SctpPassiveEstabs);
        rrddim_set_by_pointer(st, rd_aborted,   SctpAborteds);
        rrddim_set_by_pointer(st, rd_shutdown,  SctpShutdowns);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_packets == CONFIG_BOOLEAN_YES || (do_packets == CONFIG_BOOLEAN_AUTO && (SctpInSCTPPacks || SctpOutSCTPPacks))) {
        do_packets = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_received = NULL,
                      *rd_sent = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "sctp"
                    , "packets"
                    , NULL
                    , "packets"
                    , NULL
                    , "SCTP Packets"
                    , "packets/s"
                    , "proc"
                    , "net/sctp/snmp"
                    , NETDATA_CHART_PRIO_SCTP + 20
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_received = rrddim_add(st, "SctpInSCTPPacks",  "received",  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_sent     = rrddim_add(st, "SctpOutSCTPPacks", "sent",     -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_received, SctpInSCTPPacks);
        rrddim_set_by_pointer(st, rd_sent,     SctpOutSCTPPacks);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_packet_errors == CONFIG_BOOLEAN_YES || (do_packet_errors == CONFIG_BOOLEAN_AUTO && (SctpOutOfBlues || SctpChecksumErrors))) {
        do_packet_errors = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM *rd_invalid = NULL,
                      *rd_csum = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "sctp"
                    , "packet_errors"
                    , NULL
                    , "packets"
                    , NULL
                    , "SCTP Packet Errors"
                    , "packets/s"
                    , "proc"
                    , "net/sctp/snmp"
                    , NETDATA_CHART_PRIO_SCTP + 30
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_invalid = rrddim_add(st, "SctpOutOfBlues",     "invalid",  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_csum    = rrddim_add(st, "SctpChecksumErrors", "checksum", 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_invalid, SctpOutOfBlues);
        rrddim_set_by_pointer(st, rd_csum,    SctpChecksumErrors);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_fragmentation == CONFIG_BOOLEAN_YES || (do_fragmentation == CONFIG_BOOLEAN_AUTO && (SctpFragUsrMsgs || SctpReasmUsrMsgs))) {
        do_fragmentation = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_fragmented = NULL,
                      *rd_reassembled = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "sctp"
                    , "fragmentation"
                    , NULL
                    , "fragmentation"
                    , NULL
                    , "SCTP Fragmentation"
                    , "packets/s"
                    , "proc"
                    , "net/sctp/snmp"
                    , NETDATA_CHART_PRIO_SCTP + 40
                    , update_every
                    , RRDSET_TYPE_LINE);
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_reassembled = rrddim_add(st, "SctpReasmUsrMsgs", "reassembled",  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_fragmented  = rrddim_add(st, "SctpFragUsrMsgs",  "fragmented",  -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_reassembled, SctpReasmUsrMsgs);
        rrddim_set_by_pointer(st, rd_fragmented,  SctpFragUsrMsgs);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_chunk_types == CONFIG_BOOLEAN_YES || (do_chunk_types == CONFIG_BOOLEAN_AUTO
        && (SctpInCtrlChunks || SctpInOrderChunks || SctpInUnorderChunks || SctpOutCtrlChunks || SctpOutOrderChunks || SctpOutUnorderChunks))) {
        do_chunk_types = CONFIG_BOOLEAN_YES;
        static RRDSET *st = NULL;
        static RRDDIM
                *rd_InCtrl     = NULL,
                *rd_InOrder    = NULL,
                *rd_InUnorder  = NULL,
                *rd_OutCtrl    = NULL,
                *rd_OutOrder   = NULL,
                *rd_OutUnorder = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "sctp"
                    , "chunks"
                    , NULL
                    , "chunks"
                    , NULL
                    , "SCTP Chunk Types"
                    , "chunks/s"
                    , "proc"
                    , "net/sctp/snmp"
                    , NETDATA_CHART_PRIO_SCTP + 50
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_InCtrl     = rrddim_add(st, "SctpInCtrlChunks",     "InCtrl",      1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InOrder    = rrddim_add(st, "SctpInOrderChunks",    "InOrder",     1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_InUnorder  = rrddim_add(st, "SctpInUnorderChunks",  "InUnorder",   1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutCtrl    = rrddim_add(st, "SctpOutCtrlChunks",    "OutCtrl",    -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutOrder   = rrddim_add(st, "SctpOutOrderChunks",   "OutOrder",   -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_OutUnorder = rrddim_add(st, "SctpOutUnorderChunks", "OutUnorder", -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_InCtrl,     SctpInCtrlChunks);
        rrddim_set_by_pointer(st, rd_InOrder,    SctpInOrderChunks);
        rrddim_set_by_pointer(st, rd_InUnorder,  SctpInUnorderChunks);
        rrddim_set_by_pointer(st, rd_OutCtrl,    SctpOutCtrlChunks);
        rrddim_set_by_pointer(st, rd_OutOrder,   SctpOutOrderChunks);
        rrddim_set_by_pointer(st, rd_OutUnorder, SctpOutUnorderChunks);
        rrdset_done(st);
    }

    return 0;
}

