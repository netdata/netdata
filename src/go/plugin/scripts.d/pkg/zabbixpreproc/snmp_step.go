package zabbixpreproc

import (
	"context"
	"encoding/hex"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// snmpWalkToValue extracts value from SNMP walk output.
// Format modes:
// 0 = unchanged (return raw value as-is)
// 1 = UTF-8 (decode hex-string to UTF-8 string)
// 2 = MAC (format hex-string as MAC address with colons)
// 3 = BITS (convert BITS to integer)
func snmpWalkToValue(value Value, params string) (Value, error) {
	lines := strings.Split(params, "\n")
	if len(lines) < 2 {
		return Value{}, fmt.Errorf("snmp walk requires oid and format mode parameters")
	}

	targetOID := strings.TrimSpace(lines[0])
	formatMode := 0
	if len(lines) >= 2 {
		var err error
		formatMode, err = strconv.Atoi(strings.TrimSpace(lines[1]))
		if err != nil {
			return Value{}, fmt.Errorf("invalid format mode: %v", err)
		}
	}

	// Parse SNMP walk output
	extractedValue, err := parseSNMPWalk(value.Data, targetOID)
	if err != nil {
		return Value{}, err
	}

	// Apply format conversion (trim float zeros for WALK_VALUE)
	result, err := formatSNMPValue(extractedValue, formatMode, true)
	if err != nil {
		return Value{}, err
	}

	return Value{Data: result, Type: ValueTypeStr}, nil
}

// snmpEntry represents a parsed SNMP entry
type snmpEntry struct {
	oid       string
	valueType string
	value     string
}

// Common MIB-to-OID mappings for offline operation
// Covers ~95% of real-world SNMP monitoring use cases
// Falls back to snmptranslate for rare MIBs (if installed)
//
// NOTE: This map is effectively read-only (never modified after initialization).
// It is safe for concurrent access as all operations are reads (lookups).
// Go maps are safe for concurrent reads without synchronization.
var mibToOID = map[string]string{
	// ============================================================================
	// SNMPv2-MIB (System group) - RFC 3418
	// ============================================================================
	"SNMPv2-MIB::sysDescr":        ".1.3.6.1.2.1.1.1",
	"SNMPv2-MIB::sysObjectID":     ".1.3.6.1.2.1.1.2",
	"SNMPv2-MIB::sysUpTime":       ".1.3.6.1.2.1.1.3",
	"SNMPv2-MIB::sysContact":      ".1.3.6.1.2.1.1.4",
	"SNMPv2-MIB::sysName":         ".1.3.6.1.2.1.1.5",
	"SNMPv2-MIB::sysLocation":     ".1.3.6.1.2.1.1.6",
	"SNMPv2-MIB::sysServices":     ".1.3.6.1.2.1.1.7",
	"SNMPv2-MIB::sysORLastChange": ".1.3.6.1.2.1.1.8",
	"SNMPv2-MIB::sysORIndex":      ".1.3.6.1.2.1.1.9.1.1",
	"SNMPv2-MIB::sysORID":         ".1.3.6.1.2.1.1.9.1.2",
	"SNMPv2-MIB::sysORDescr":      ".1.3.6.1.2.1.1.9.1.3",
	"SNMPv2-MIB::sysORUpTime":     ".1.3.6.1.2.1.1.9.1.4",

	// SNMPv2-MIB SNMP group
	"SNMPv2-MIB::snmpInPkts":              ".1.3.6.1.2.1.11.1",
	"SNMPv2-MIB::snmpOutPkts":             ".1.3.6.1.2.1.11.2",
	"SNMPv2-MIB::snmpInBadVersions":       ".1.3.6.1.2.1.11.3",
	"SNMPv2-MIB::snmpInBadCommunityNames": ".1.3.6.1.2.1.11.4",
	"SNMPv2-MIB::snmpInBadCommunityUses":  ".1.3.6.1.2.1.11.5",
	"SNMPv2-MIB::snmpInASNParseErrs":      ".1.3.6.1.2.1.11.6",
	"SNMPv2-MIB::snmpInTooBigs":           ".1.3.6.1.2.1.11.8",
	"SNMPv2-MIB::snmpInNoSuchNames":       ".1.3.6.1.2.1.11.9",
	"SNMPv2-MIB::snmpInBadValues":         ".1.3.6.1.2.1.11.10",
	"SNMPv2-MIB::snmpInReadOnlys":         ".1.3.6.1.2.1.11.11",
	"SNMPv2-MIB::snmpInGenErrs":           ".1.3.6.1.2.1.11.12",
	"SNMPv2-MIB::snmpInTotalReqVars":      ".1.3.6.1.2.1.11.13",
	"SNMPv2-MIB::snmpInTotalSetVars":      ".1.3.6.1.2.1.11.14",
	"SNMPv2-MIB::snmpInGetRequests":       ".1.3.6.1.2.1.11.15",
	"SNMPv2-MIB::snmpInGetNexts":          ".1.3.6.1.2.1.11.16",
	"SNMPv2-MIB::snmpInSetRequests":       ".1.3.6.1.2.1.11.17",
	"SNMPv2-MIB::snmpInGetResponses":      ".1.3.6.1.2.1.11.18",
	"SNMPv2-MIB::snmpInTraps":             ".1.3.6.1.2.1.11.19",
	"SNMPv2-MIB::snmpOutTooBigs":          ".1.3.6.1.2.1.11.20",
	"SNMPv2-MIB::snmpOutNoSuchNames":      ".1.3.6.1.2.1.11.21",
	"SNMPv2-MIB::snmpOutBadValues":        ".1.3.6.1.2.1.11.22",
	"SNMPv2-MIB::snmpOutGenErrs":          ".1.3.6.1.2.1.11.24",
	"SNMPv2-MIB::snmpOutGetRequests":      ".1.3.6.1.2.1.11.25",
	"SNMPv2-MIB::snmpOutGetNexts":         ".1.3.6.1.2.1.11.26",
	"SNMPv2-MIB::snmpOutSetRequests":      ".1.3.6.1.2.1.11.27",
	"SNMPv2-MIB::snmpOutGetResponses":     ".1.3.6.1.2.1.11.28",
	"SNMPv2-MIB::snmpOutTraps":            ".1.3.6.1.2.1.11.29",
	"SNMPv2-MIB::snmpEnableAuthenTraps":   ".1.3.6.1.2.1.11.30",
	"SNMPv2-MIB::snmpSilentDrops":         ".1.3.6.1.2.1.11.31",
	"SNMPv2-MIB::snmpProxyDrops":          ".1.3.6.1.2.1.11.32",

	// ============================================================================
	// IF-MIB (Interfaces) - RFC 2863
	// ============================================================================
	// Interface counters (.1.3.6.1.2.1.2.x)
	"IF-MIB::ifNumber": ".1.3.6.1.2.1.2.1",

	// Legacy interfaces table (.1.3.6.1.2.1.2.2.1.x)
	"IF-MIB::ifIndex":           ".1.3.6.1.2.1.2.2.1.1",
	"IF-MIB::ifDescr":           ".1.3.6.1.2.1.2.2.1.2",
	"IF-MIB::ifType":            ".1.3.6.1.2.1.2.2.1.3",
	"IF-MIB::ifMtu":             ".1.3.6.1.2.1.2.2.1.4",
	"IF-MIB::ifSpeed":           ".1.3.6.1.2.1.2.2.1.5",
	"IF-MIB::ifPhysAddress":     ".1.3.6.1.2.1.2.2.1.6",
	"IF-MIB::ifAdminStatus":     ".1.3.6.1.2.1.2.2.1.7",
	"IF-MIB::ifOperStatus":      ".1.3.6.1.2.1.2.2.1.8",
	"IF-MIB::ifLastChange":      ".1.3.6.1.2.1.2.2.1.9",
	"IF-MIB::ifInOctets":        ".1.3.6.1.2.1.2.2.1.10",
	"IF-MIB::ifInUcastPkts":     ".1.3.6.1.2.1.2.2.1.11",
	"IF-MIB::ifInNUcastPkts":    ".1.3.6.1.2.1.2.2.1.12",
	"IF-MIB::ifInDiscards":      ".1.3.6.1.2.1.2.2.1.13",
	"IF-MIB::ifInErrors":        ".1.3.6.1.2.1.2.2.1.14",
	"IF-MIB::ifInUnknownProtos": ".1.3.6.1.2.1.2.2.1.15",
	"IF-MIB::ifOutOctets":       ".1.3.6.1.2.1.2.2.1.16",
	"IF-MIB::ifOutUcastPkts":    ".1.3.6.1.2.1.2.2.1.17",
	"IF-MIB::ifOutNUcastPkts":   ".1.3.6.1.2.1.2.2.1.18",
	"IF-MIB::ifOutDiscards":     ".1.3.6.1.2.1.2.2.1.19",
	"IF-MIB::ifOutErrors":       ".1.3.6.1.2.1.2.2.1.20",
	"IF-MIB::ifOutQLen":         ".1.3.6.1.2.1.2.2.1.21",
	"IF-MIB::ifSpecific":        ".1.3.6.1.2.1.2.2.1.22",

	// IF-MIB Extended table (.1.3.6.1.2.1.31.1.1.1.x)
	"IF-MIB::ifName":                     ".1.3.6.1.2.1.31.1.1.1.1",
	"IF-MIB::ifInMulticastPkts":          ".1.3.6.1.2.1.31.1.1.1.2",
	"IF-MIB::ifInBroadcastPkts":          ".1.3.6.1.2.1.31.1.1.1.3",
	"IF-MIB::ifOutMulticastPkts":         ".1.3.6.1.2.1.31.1.1.1.4",
	"IF-MIB::ifOutBroadcastPkts":         ".1.3.6.1.2.1.31.1.1.1.5",
	"IF-MIB::ifHCInOctets":               ".1.3.6.1.2.1.31.1.1.1.6",
	"IF-MIB::ifHCInUcastPkts":            ".1.3.6.1.2.1.31.1.1.1.7",
	"IF-MIB::ifHCInMulticastPkts":        ".1.3.6.1.2.1.31.1.1.1.8",
	"IF-MIB::ifHCInBroadcastPkts":        ".1.3.6.1.2.1.31.1.1.1.9",
	"IF-MIB::ifHCOutOctets":              ".1.3.6.1.2.1.31.1.1.1.10",
	"IF-MIB::ifHCOutUcastPkts":           ".1.3.6.1.2.1.31.1.1.1.11",
	"IF-MIB::ifHCOutMulticastPkts":       ".1.3.6.1.2.1.31.1.1.1.12",
	"IF-MIB::ifHCOutBroadcastPkts":       ".1.3.6.1.2.1.31.1.1.1.13",
	"IF-MIB::ifLinkUpDownTrapEnable":     ".1.3.6.1.2.1.31.1.1.1.14",
	"IF-MIB::ifHighSpeed":                ".1.3.6.1.2.1.31.1.1.1.15",
	"IF-MIB::ifPromiscuousMode":          ".1.3.6.1.2.1.31.1.1.1.16",
	"IF-MIB::ifConnectorPresent":         ".1.3.6.1.2.1.31.1.1.1.17",
	"IF-MIB::ifAlias":                    ".1.3.6.1.2.1.31.1.1.1.18",
	"IF-MIB::ifCounterDiscontinuityTime": ".1.3.6.1.2.1.31.1.1.1.19",

	// IF-MIB Stack table
	"IF-MIB::ifStackHigherLayer": ".1.3.6.1.2.1.31.1.2.1.1",
	"IF-MIB::ifStackLowerLayer":  ".1.3.6.1.2.1.31.1.2.1.2",
	"IF-MIB::ifStackStatus":      ".1.3.6.1.2.1.31.1.2.1.3",

	// ============================================================================
	// IP-MIB - RFC 4293
	// ============================================================================
	"IP-MIB::ipForwarding":      ".1.3.6.1.2.1.4.1",
	"IP-MIB::ipDefaultTTL":      ".1.3.6.1.2.1.4.2",
	"IP-MIB::ipInReceives":      ".1.3.6.1.2.1.4.3",
	"IP-MIB::ipInHdrErrors":     ".1.3.6.1.2.1.4.4",
	"IP-MIB::ipInAddrErrors":    ".1.3.6.1.2.1.4.5",
	"IP-MIB::ipForwDatagrams":   ".1.3.6.1.2.1.4.6",
	"IP-MIB::ipInUnknownProtos": ".1.3.6.1.2.1.4.7",
	"IP-MIB::ipInDiscards":      ".1.3.6.1.2.1.4.8",
	"IP-MIB::ipInDelivers":      ".1.3.6.1.2.1.4.9",
	"IP-MIB::ipOutRequests":     ".1.3.6.1.2.1.4.10",
	"IP-MIB::ipOutDiscards":     ".1.3.6.1.2.1.4.11",
	"IP-MIB::ipOutNoRoutes":     ".1.3.6.1.2.1.4.12",
	"IP-MIB::ipReasmTimeout":    ".1.3.6.1.2.1.4.13",
	"IP-MIB::ipReasmReqds":      ".1.3.6.1.2.1.4.14",
	"IP-MIB::ipReasmOKs":        ".1.3.6.1.2.1.4.15",
	"IP-MIB::ipReasmFails":      ".1.3.6.1.2.1.4.16",
	"IP-MIB::ipFragOKs":         ".1.3.6.1.2.1.4.17",
	"IP-MIB::ipFragFails":       ".1.3.6.1.2.1.4.18",
	"IP-MIB::ipFragCreates":     ".1.3.6.1.2.1.4.19",

	// IP Address table
	"IP-MIB::ipAdEntAddr":         ".1.3.6.1.2.1.4.20.1.1",
	"IP-MIB::ipAdEntIfIndex":      ".1.3.6.1.2.1.4.20.1.2",
	"IP-MIB::ipAdEntNetMask":      ".1.3.6.1.2.1.4.20.1.3",
	"IP-MIB::ipAdEntBcastAddr":    ".1.3.6.1.2.1.4.20.1.4",
	"IP-MIB::ipAdEntReasmMaxSize": ".1.3.6.1.2.1.4.20.1.5",

	// IP Route table
	"IP-MIB::ipRouteDest":    ".1.3.6.1.2.1.4.21.1.1",
	"IP-MIB::ipRouteIfIndex": ".1.3.6.1.2.1.4.21.1.2",
	"IP-MIB::ipRouteMetric1": ".1.3.6.1.2.1.4.21.1.3",
	"IP-MIB::ipRouteMetric2": ".1.3.6.1.2.1.4.21.1.4",
	"IP-MIB::ipRouteMetric3": ".1.3.6.1.2.1.4.21.1.5",
	"IP-MIB::ipRouteMetric4": ".1.3.6.1.2.1.4.21.1.6",
	"IP-MIB::ipRouteNextHop": ".1.3.6.1.2.1.4.21.1.7",
	"IP-MIB::ipRouteType":    ".1.3.6.1.2.1.4.21.1.8",
	"IP-MIB::ipRouteProto":   ".1.3.6.1.2.1.4.21.1.9",
	"IP-MIB::ipRouteAge":     ".1.3.6.1.2.1.4.21.1.10",
	"IP-MIB::ipRouteMask":    ".1.3.6.1.2.1.4.21.1.11",
	"IP-MIB::ipRouteMetric5": ".1.3.6.1.2.1.4.21.1.12",
	"IP-MIB::ipRouteInfo":    ".1.3.6.1.2.1.4.21.1.13",

	// IP Net-to-Media table
	"IP-MIB::ipNetToMediaIfIndex":     ".1.3.6.1.2.1.4.22.1.1",
	"IP-MIB::ipNetToMediaPhysAddress": ".1.3.6.1.2.1.4.22.1.2",
	"IP-MIB::ipNetToMediaNetAddress":  ".1.3.6.1.2.1.4.22.1.3",
	"IP-MIB::ipNetToMediaType":        ".1.3.6.1.2.1.4.22.1.4",

	// IP Forwarding table (newer)
	"IP-MIB::inetCidrRouteNumber": ".1.3.6.1.2.1.4.24.6",

	// ============================================================================
	// ICMP-MIB - RFC 4293
	// ============================================================================
	"IP-MIB::icmpInMsgs":           ".1.3.6.1.2.1.5.1",
	"IP-MIB::icmpInErrors":         ".1.3.6.1.2.1.5.2",
	"IP-MIB::icmpInDestUnreachs":   ".1.3.6.1.2.1.5.3",
	"IP-MIB::icmpInTimeExcds":      ".1.3.6.1.2.1.5.4",
	"IP-MIB::icmpInParmProbs":      ".1.3.6.1.2.1.5.5",
	"IP-MIB::icmpInSrcQuenchs":     ".1.3.6.1.2.1.5.6",
	"IP-MIB::icmpInRedirects":      ".1.3.6.1.2.1.5.7",
	"IP-MIB::icmpInEchos":          ".1.3.6.1.2.1.5.8",
	"IP-MIB::icmpInEchoReps":       ".1.3.6.1.2.1.5.9",
	"IP-MIB::icmpInTimestamps":     ".1.3.6.1.2.1.5.10",
	"IP-MIB::icmpInTimestampReps":  ".1.3.6.1.2.1.5.11",
	"IP-MIB::icmpInAddrMasks":      ".1.3.6.1.2.1.5.12",
	"IP-MIB::icmpInAddrMaskReps":   ".1.3.6.1.2.1.5.13",
	"IP-MIB::icmpOutMsgs":          ".1.3.6.1.2.1.5.14",
	"IP-MIB::icmpOutErrors":        ".1.3.6.1.2.1.5.15",
	"IP-MIB::icmpOutDestUnreachs":  ".1.3.6.1.2.1.5.16",
	"IP-MIB::icmpOutTimeExcds":     ".1.3.6.1.2.1.5.17",
	"IP-MIB::icmpOutParmProbs":     ".1.3.6.1.2.1.5.18",
	"IP-MIB::icmpOutSrcQuenchs":    ".1.3.6.1.2.1.5.19",
	"IP-MIB::icmpOutRedirects":     ".1.3.6.1.2.1.5.20",
	"IP-MIB::icmpOutEchos":         ".1.3.6.1.2.1.5.21",
	"IP-MIB::icmpOutEchoReps":      ".1.3.6.1.2.1.5.22",
	"IP-MIB::icmpOutTimestamps":    ".1.3.6.1.2.1.5.23",
	"IP-MIB::icmpOutTimestampReps": ".1.3.6.1.2.1.5.24",
	"IP-MIB::icmpOutAddrMasks":     ".1.3.6.1.2.1.5.25",
	"IP-MIB::icmpOutAddrMaskReps":  ".1.3.6.1.2.1.5.26",

	// ============================================================================
	// TCP-MIB - RFC 4022
	// ============================================================================
	"TCP-MIB::tcpRtoAlgorithm": ".1.3.6.1.2.1.6.1",
	"TCP-MIB::tcpRtoMin":       ".1.3.6.1.2.1.6.2",
	"TCP-MIB::tcpRtoMax":       ".1.3.6.1.2.1.6.3",
	"TCP-MIB::tcpMaxConn":      ".1.3.6.1.2.1.6.4",
	"TCP-MIB::tcpActiveOpens":  ".1.3.6.1.2.1.6.5",
	"TCP-MIB::tcpPassiveOpens": ".1.3.6.1.2.1.6.6",
	"TCP-MIB::tcpAttemptFails": ".1.3.6.1.2.1.6.7",
	"TCP-MIB::tcpEstabResets":  ".1.3.6.1.2.1.6.8",
	"TCP-MIB::tcpCurrEstab":    ".1.3.6.1.2.1.6.9",
	"TCP-MIB::tcpInSegs":       ".1.3.6.1.2.1.6.10",
	"TCP-MIB::tcpOutSegs":      ".1.3.6.1.2.1.6.11",
	"TCP-MIB::tcpRetransSegs":  ".1.3.6.1.2.1.6.12",
	"TCP-MIB::tcpInErrs":       ".1.3.6.1.2.1.6.14",
	"TCP-MIB::tcpOutRsts":      ".1.3.6.1.2.1.6.15",
	"TCP-MIB::tcpHCInSegs":     ".1.3.6.1.2.1.6.17",
	"TCP-MIB::tcpHCOutSegs":    ".1.3.6.1.2.1.6.18",

	// TCP Connection table
	"TCP-MIB::tcpConnState":        ".1.3.6.1.2.1.6.13.1.1",
	"TCP-MIB::tcpConnLocalAddress": ".1.3.6.1.2.1.6.13.1.2",
	"TCP-MIB::tcpConnLocalPort":    ".1.3.6.1.2.1.6.13.1.3",
	"TCP-MIB::tcpConnRemAddress":   ".1.3.6.1.2.1.6.13.1.4",
	"TCP-MIB::tcpConnRemPort":      ".1.3.6.1.2.1.6.13.1.5",

	// ============================================================================
	// UDP-MIB - RFC 4113
	// ============================================================================
	"UDP-MIB::udpInDatagrams":    ".1.3.6.1.2.1.7.1",
	"UDP-MIB::udpNoPorts":        ".1.3.6.1.2.1.7.2",
	"UDP-MIB::udpInErrors":       ".1.3.6.1.2.1.7.3",
	"UDP-MIB::udpOutDatagrams":   ".1.3.6.1.2.1.7.4",
	"UDP-MIB::udpHCInDatagrams":  ".1.3.6.1.2.1.7.8",
	"UDP-MIB::udpHCOutDatagrams": ".1.3.6.1.2.1.7.9",

	// UDP Listener table
	"UDP-MIB::udpLocalAddress": ".1.3.6.1.2.1.7.5.1.1",
	"UDP-MIB::udpLocalPort":    ".1.3.6.1.2.1.7.5.1.2",

	// ============================================================================
	// HOST-RESOURCES-MIB - RFC 2790
	// ============================================================================
	// System group (.1.3.6.1.2.1.25.1.x)
	"HOST-RESOURCES-MIB::hrSystemUptime":                ".1.3.6.1.2.1.25.1.1",
	"HOST-RESOURCES-MIB::hrSystemDate":                  ".1.3.6.1.2.1.25.1.2",
	"HOST-RESOURCES-MIB::hrSystemInitialLoadDevice":     ".1.3.6.1.2.1.25.1.3",
	"HOST-RESOURCES-MIB::hrSystemInitialLoadParameters": ".1.3.6.1.2.1.25.1.4",
	"HOST-RESOURCES-MIB::hrSystemNumUsers":              ".1.3.6.1.2.1.25.1.5",
	"HOST-RESOURCES-MIB::hrSystemProcesses":             ".1.3.6.1.2.1.25.1.6",
	"HOST-RESOURCES-MIB::hrSystemMaxProcesses":          ".1.3.6.1.2.1.25.1.7",

	// Storage group (.1.3.6.1.2.1.25.2.x)
	"HOST-RESOURCES-MIB::hrMemorySize":                ".1.3.6.1.2.1.25.2.2",
	"HOST-RESOURCES-MIB::hrStorageIndex":              ".1.3.6.1.2.1.25.2.3.1.1",
	"HOST-RESOURCES-MIB::hrStorageType":               ".1.3.6.1.2.1.25.2.3.1.2",
	"HOST-RESOURCES-MIB::hrStorageDescr":              ".1.3.6.1.2.1.25.2.3.1.3",
	"HOST-RESOURCES-MIB::hrStorageAllocationUnits":    ".1.3.6.1.2.1.25.2.3.1.4",
	"HOST-RESOURCES-MIB::hrStorageSize":               ".1.3.6.1.2.1.25.2.3.1.5",
	"HOST-RESOURCES-MIB::hrStorageUsed":               ".1.3.6.1.2.1.25.2.3.1.6",
	"HOST-RESOURCES-MIB::hrStorageAllocationFailures": ".1.3.6.1.2.1.25.2.3.1.7",

	// Device group (.1.3.6.1.2.1.25.3.x)
	"HOST-RESOURCES-MIB::hrDeviceIndex":               ".1.3.6.1.2.1.25.3.2.1.1",
	"HOST-RESOURCES-MIB::hrDeviceType":                ".1.3.6.1.2.1.25.3.2.1.2",
	"HOST-RESOURCES-MIB::hrDeviceDescr":               ".1.3.6.1.2.1.25.3.2.1.3",
	"HOST-RESOURCES-MIB::hrDeviceID":                  ".1.3.6.1.2.1.25.3.2.1.4",
	"HOST-RESOURCES-MIB::hrDeviceStatus":              ".1.3.6.1.2.1.25.3.2.1.5",
	"HOST-RESOURCES-MIB::hrDeviceErrors":              ".1.3.6.1.2.1.25.3.2.1.6",
	"HOST-RESOURCES-MIB::hrProcessorFrwID":            ".1.3.6.1.2.1.25.3.3.1.1",
	"HOST-RESOURCES-MIB::hrProcessorLoad":             ".1.3.6.1.2.1.25.3.3.1.2",
	"HOST-RESOURCES-MIB::hrNetworkIfIndex":            ".1.3.6.1.2.1.25.3.4.1.1",
	"HOST-RESOURCES-MIB::hrPrinterStatus":             ".1.3.6.1.2.1.25.3.5.1.1",
	"HOST-RESOURCES-MIB::hrPrinterDetectedErrorState": ".1.3.6.1.2.1.25.3.5.1.2",
	"HOST-RESOURCES-MIB::hrDiskStorageAccess":         ".1.3.6.1.2.1.25.3.6.1.1",
	"HOST-RESOURCES-MIB::hrDiskStorageMedia":          ".1.3.6.1.2.1.25.3.6.1.2",
	"HOST-RESOURCES-MIB::hrDiskStorageRemoveble":      ".1.3.6.1.2.1.25.3.6.1.3",
	"HOST-RESOURCES-MIB::hrDiskStorageCapacity":       ".1.3.6.1.2.1.25.3.6.1.4",
	"HOST-RESOURCES-MIB::hrPartitionIndex":            ".1.3.6.1.2.1.25.3.7.1.1",
	"HOST-RESOURCES-MIB::hrPartitionLabel":            ".1.3.6.1.2.1.25.3.7.1.2",
	"HOST-RESOURCES-MIB::hrPartitionID":               ".1.3.6.1.2.1.25.3.7.1.3",
	"HOST-RESOURCES-MIB::hrPartitionSize":             ".1.3.6.1.2.1.25.3.7.1.4",
	"HOST-RESOURCES-MIB::hrPartitionFSIndex":          ".1.3.6.1.2.1.25.3.7.1.5",
	"HOST-RESOURCES-MIB::hrFSIndex":                   ".1.3.6.1.2.1.25.3.8.1.1",
	"HOST-RESOURCES-MIB::hrFSMountPoint":              ".1.3.6.1.2.1.25.3.8.1.2",
	"HOST-RESOURCES-MIB::hrFSRemoteMountPoint":        ".1.3.6.1.2.1.25.3.8.1.3",
	"HOST-RESOURCES-MIB::hrFSType":                    ".1.3.6.1.2.1.25.3.8.1.4",
	"HOST-RESOURCES-MIB::hrFSAccess":                  ".1.3.6.1.2.1.25.3.8.1.5",
	"HOST-RESOURCES-MIB::hrFSBootable":                ".1.3.6.1.2.1.25.3.8.1.6",
	"HOST-RESOURCES-MIB::hrFSStorageIndex":            ".1.3.6.1.2.1.25.3.8.1.7",
	"HOST-RESOURCES-MIB::hrFSLastFullBackupDate":      ".1.3.6.1.2.1.25.3.8.1.8",
	"HOST-RESOURCES-MIB::hrFSLastPartialBackupDate":   ".1.3.6.1.2.1.25.3.8.1.9",

	// Running Software group (.1.3.6.1.2.1.25.4.x)
	"HOST-RESOURCES-MIB::hrSWOSIndex":       ".1.3.6.1.2.1.25.4.1",
	"HOST-RESOURCES-MIB::hrSWRunIndex":      ".1.3.6.1.2.1.25.4.2.1.1",
	"HOST-RESOURCES-MIB::hrSWRunName":       ".1.3.6.1.2.1.25.4.2.1.2",
	"HOST-RESOURCES-MIB::hrSWRunID":         ".1.3.6.1.2.1.25.4.2.1.3",
	"HOST-RESOURCES-MIB::hrSWRunPath":       ".1.3.6.1.2.1.25.4.2.1.4",
	"HOST-RESOURCES-MIB::hrSWRunParameters": ".1.3.6.1.2.1.25.4.2.1.5",
	"HOST-RESOURCES-MIB::hrSWRunType":       ".1.3.6.1.2.1.25.4.2.1.6",
	"HOST-RESOURCES-MIB::hrSWRunStatus":     ".1.3.6.1.2.1.25.4.2.1.7",

	// Running Software Performance group
	"HOST-RESOURCES-MIB::hrSWRunPerfCPU": ".1.3.6.1.2.1.25.5.1.1.1",
	"HOST-RESOURCES-MIB::hrSWRunPerfMem": ".1.3.6.1.2.1.25.5.1.1.2",

	// Installed Software group (.1.3.6.1.2.1.25.6.x)
	"HOST-RESOURCES-MIB::hrSWInstalledLastChange":     ".1.3.6.1.2.1.25.6.1",
	"HOST-RESOURCES-MIB::hrSWInstalledLastUpdateTime": ".1.3.6.1.2.1.25.6.2",
	"HOST-RESOURCES-MIB::hrSWInstalledIndex":          ".1.3.6.1.2.1.25.6.3.1.1",
	"HOST-RESOURCES-MIB::hrSWInstalledName":           ".1.3.6.1.2.1.25.6.3.1.2",
	"HOST-RESOURCES-MIB::hrSWInstalledID":             ".1.3.6.1.2.1.25.6.3.1.3",
	"HOST-RESOURCES-MIB::hrSWInstalledType":           ".1.3.6.1.2.1.25.6.3.1.4",
	"HOST-RESOURCES-MIB::hrSWInstalledDate":           ".1.3.6.1.2.1.25.6.3.1.5",

	// ============================================================================
	// ENTITY-MIB - RFC 6933
	// ============================================================================
	"ENTITY-MIB::entPhysicalIndex":        ".1.3.6.1.2.1.47.1.1.1.1.1",
	"ENTITY-MIB::entPhysicalDescr":        ".1.3.6.1.2.1.47.1.1.1.1.2",
	"ENTITY-MIB::entPhysicalVendorType":   ".1.3.6.1.2.1.47.1.1.1.1.3",
	"ENTITY-MIB::entPhysicalContainedIn":  ".1.3.6.1.2.1.47.1.1.1.1.4",
	"ENTITY-MIB::entPhysicalClass":        ".1.3.6.1.2.1.47.1.1.1.1.5",
	"ENTITY-MIB::entPhysicalParentRelPos": ".1.3.6.1.2.1.47.1.1.1.1.6",
	"ENTITY-MIB::entPhysicalName":         ".1.3.6.1.2.1.47.1.1.1.1.7",
	"ENTITY-MIB::entPhysicalHardwareRev":  ".1.3.6.1.2.1.47.1.1.1.1.8",
	"ENTITY-MIB::entPhysicalFirmwareRev":  ".1.3.6.1.2.1.47.1.1.1.1.9",
	"ENTITY-MIB::entPhysicalSoftwareRev":  ".1.3.6.1.2.1.47.1.1.1.1.10",
	"ENTITY-MIB::entPhysicalSerialNum":    ".1.3.6.1.2.1.47.1.1.1.1.11",
	"ENTITY-MIB::entPhysicalMfgName":      ".1.3.6.1.2.1.47.1.1.1.1.12",
	"ENTITY-MIB::entPhysicalModelName":    ".1.3.6.1.2.1.47.1.1.1.1.13",
	"ENTITY-MIB::entPhysicalAlias":        ".1.3.6.1.2.1.47.1.1.1.1.14",
	"ENTITY-MIB::entPhysicalAssetID":      ".1.3.6.1.2.1.47.1.1.1.1.15",
	"ENTITY-MIB::entPhysicalIsFRU":        ".1.3.6.1.2.1.47.1.1.1.1.16",
	"ENTITY-MIB::entPhysicalMfgDate":      ".1.3.6.1.2.1.47.1.1.1.1.17",
	"ENTITY-MIB::entPhysicalUris":         ".1.3.6.1.2.1.47.1.1.1.1.18",

	// ============================================================================
	// EtherLike-MIB - RFC 3635
	// ============================================================================
	"EtherLike-MIB::dot3StatsIndex":                     ".1.3.6.1.2.1.10.7.2.1.1",
	"EtherLike-MIB::dot3StatsAlignmentErrors":           ".1.3.6.1.2.1.10.7.2.1.2",
	"EtherLike-MIB::dot3StatsFCSErrors":                 ".1.3.6.1.2.1.10.7.2.1.3",
	"EtherLike-MIB::dot3StatsSingleCollisionFrames":     ".1.3.6.1.2.1.10.7.2.1.4",
	"EtherLike-MIB::dot3StatsMultipleCollisionFrames":   ".1.3.6.1.2.1.10.7.2.1.5",
	"EtherLike-MIB::dot3StatsSQETestErrors":             ".1.3.6.1.2.1.10.7.2.1.6",
	"EtherLike-MIB::dot3StatsDeferredTransmissions":     ".1.3.6.1.2.1.10.7.2.1.7",
	"EtherLike-MIB::dot3StatsLateCollisions":            ".1.3.6.1.2.1.10.7.2.1.8",
	"EtherLike-MIB::dot3StatsExcessiveCollisions":       ".1.3.6.1.2.1.10.7.2.1.9",
	"EtherLike-MIB::dot3StatsInternalMacTransmitErrors": ".1.3.6.1.2.1.10.7.2.1.10",
	"EtherLike-MIB::dot3StatsCarrierSenseErrors":        ".1.3.6.1.2.1.10.7.2.1.11",
	"EtherLike-MIB::dot3StatsFrameTooLongs":             ".1.3.6.1.2.1.10.7.2.1.13",
	"EtherLike-MIB::dot3StatsInternalMacReceiveErrors":  ".1.3.6.1.2.1.10.7.2.1.16",
	"EtherLike-MIB::dot3StatsSymbolErrors":              ".1.3.6.1.2.1.10.7.2.1.18",
	"EtherLike-MIB::dot3StatsDuplexStatus":              ".1.3.6.1.2.1.10.7.2.1.19",

	// ============================================================================
	// UCD-SNMP-MIB (Linux/Unix systems) - NET-SNMP
	// ============================================================================
	// Memory statistics
	"UCD-SNMP-MIB::memIndex":        ".1.3.6.1.4.1.2021.4.1",
	"UCD-SNMP-MIB::memErrorName":    ".1.3.6.1.4.1.2021.4.2",
	"UCD-SNMP-MIB::memTotalSwap":    ".1.3.6.1.4.1.2021.4.3",
	"UCD-SNMP-MIB::memAvailSwap":    ".1.3.6.1.4.1.2021.4.4",
	"UCD-SNMP-MIB::memTotalReal":    ".1.3.6.1.4.1.2021.4.5",
	"UCD-SNMP-MIB::memAvailReal":    ".1.3.6.1.4.1.2021.4.6",
	"UCD-SNMP-MIB::memTotalFree":    ".1.3.6.1.4.1.2021.4.11",
	"UCD-SNMP-MIB::memMinimumSwap":  ".1.3.6.1.4.1.2021.4.12",
	"UCD-SNMP-MIB::memShared":       ".1.3.6.1.4.1.2021.4.13",
	"UCD-SNMP-MIB::memBuffer":       ".1.3.6.1.4.1.2021.4.14",
	"UCD-SNMP-MIB::memCached":       ".1.3.6.1.4.1.2021.4.15",
	"UCD-SNMP-MIB::memSwapError":    ".1.3.6.1.4.1.2021.4.100",
	"UCD-SNMP-MIB::memSwapErrorMsg": ".1.3.6.1.4.1.2021.4.101",

	// CPU statistics
	"UCD-SNMP-MIB::ssIndex":           ".1.3.6.1.4.1.2021.11.1",
	"UCD-SNMP-MIB::ssErrorName":       ".1.3.6.1.4.1.2021.11.2",
	"UCD-SNMP-MIB::ssSwapIn":          ".1.3.6.1.4.1.2021.11.3",
	"UCD-SNMP-MIB::ssSwapOut":         ".1.3.6.1.4.1.2021.11.4",
	"UCD-SNMP-MIB::ssIOSent":          ".1.3.6.1.4.1.2021.11.5",
	"UCD-SNMP-MIB::ssIOReceive":       ".1.3.6.1.4.1.2021.11.6",
	"UCD-SNMP-MIB::ssSysInterrupts":   ".1.3.6.1.4.1.2021.11.7",
	"UCD-SNMP-MIB::ssSysContext":      ".1.3.6.1.4.1.2021.11.8",
	"UCD-SNMP-MIB::ssCpuUser":         ".1.3.6.1.4.1.2021.11.9",
	"UCD-SNMP-MIB::ssCpuSystem":       ".1.3.6.1.4.1.2021.11.10",
	"UCD-SNMP-MIB::ssCpuIdle":         ".1.3.6.1.4.1.2021.11.11",
	"UCD-SNMP-MIB::ssCpuRawUser":      ".1.3.6.1.4.1.2021.11.50",
	"UCD-SNMP-MIB::ssCpuRawNice":      ".1.3.6.1.4.1.2021.11.51",
	"UCD-SNMP-MIB::ssCpuRawSystem":    ".1.3.6.1.4.1.2021.11.52",
	"UCD-SNMP-MIB::ssCpuRawIdle":      ".1.3.6.1.4.1.2021.11.53",
	"UCD-SNMP-MIB::ssCpuRawWait":      ".1.3.6.1.4.1.2021.11.54",
	"UCD-SNMP-MIB::ssCpuRawKernel":    ".1.3.6.1.4.1.2021.11.55",
	"UCD-SNMP-MIB::ssCpuRawInterrupt": ".1.3.6.1.4.1.2021.11.56",
	"UCD-SNMP-MIB::ssIORawSent":       ".1.3.6.1.4.1.2021.11.57",
	"UCD-SNMP-MIB::ssIORawReceived":   ".1.3.6.1.4.1.2021.11.58",
	"UCD-SNMP-MIB::ssRawInterrupts":   ".1.3.6.1.4.1.2021.11.59",
	"UCD-SNMP-MIB::ssRawContexts":     ".1.3.6.1.4.1.2021.11.60",
	"UCD-SNMP-MIB::ssCpuRawSoftIRQ":   ".1.3.6.1.4.1.2021.11.61",
	"UCD-SNMP-MIB::ssCpuRawSteal":     ".1.3.6.1.4.1.2021.11.64",
	"UCD-SNMP-MIB::ssCpuRawGuest":     ".1.3.6.1.4.1.2021.11.65",
	"UCD-SNMP-MIB::ssCpuRawGuestNice": ".1.3.6.1.4.1.2021.11.66",

	// Load average
	"UCD-SNMP-MIB::laIndex":      ".1.3.6.1.4.1.2021.10.1.1",
	"UCD-SNMP-MIB::laNames":      ".1.3.6.1.4.1.2021.10.1.2",
	"UCD-SNMP-MIB::laLoad":       ".1.3.6.1.4.1.2021.10.1.3",
	"UCD-SNMP-MIB::laConfig":     ".1.3.6.1.4.1.2021.10.1.4",
	"UCD-SNMP-MIB::laLoadInt":    ".1.3.6.1.4.1.2021.10.1.5",
	"UCD-SNMP-MIB::laLoadFloat":  ".1.3.6.1.4.1.2021.10.1.6",
	"UCD-SNMP-MIB::laErrorFlag":  ".1.3.6.1.4.1.2021.10.1.100",
	"UCD-SNMP-MIB::laErrMessage": ".1.3.6.1.4.1.2021.10.1.101",

	// Disk I/O statistics
	"UCD-SNMP-MIB::diskIOIndex":     ".1.3.6.1.4.1.2021.13.15.1.1.1",
	"UCD-SNMP-MIB::diskIODevice":    ".1.3.6.1.4.1.2021.13.15.1.1.2",
	"UCD-SNMP-MIB::diskIONRead":     ".1.3.6.1.4.1.2021.13.15.1.1.3",
	"UCD-SNMP-MIB::diskIONWritten":  ".1.3.6.1.4.1.2021.13.15.1.1.4",
	"UCD-SNMP-MIB::diskIOReads":     ".1.3.6.1.4.1.2021.13.15.1.1.5",
	"UCD-SNMP-MIB::diskIOWrites":    ".1.3.6.1.4.1.2021.13.15.1.1.6",
	"UCD-SNMP-MIB::diskIONReadX":    ".1.3.6.1.4.1.2021.13.15.1.1.12",
	"UCD-SNMP-MIB::diskIONWrittenX": ".1.3.6.1.4.1.2021.13.15.1.1.13",

	// ============================================================================
	// UPS-MIB - RFC 1628
	// ============================================================================
	"UPS-MIB::upsIdentManufacturer":         ".1.3.6.1.2.1.33.1.1.1",
	"UPS-MIB::upsIdentModel":                ".1.3.6.1.2.1.33.1.1.2",
	"UPS-MIB::upsIdentUPSSoftwareVersion":   ".1.3.6.1.2.1.33.1.1.3",
	"UPS-MIB::upsIdentAgentSoftwareVersion": ".1.3.6.1.2.1.33.1.1.4",
	"UPS-MIB::upsIdentName":                 ".1.3.6.1.2.1.33.1.1.5",
	"UPS-MIB::upsIdentAttachedDevices":      ".1.3.6.1.2.1.33.1.1.6",
	"UPS-MIB::upsBatteryStatus":             ".1.3.6.1.2.1.33.1.2.1",
	"UPS-MIB::upsSecondsOnBattery":          ".1.3.6.1.2.1.33.1.2.2",
	"UPS-MIB::upsEstimatedMinutesRemaining": ".1.3.6.1.2.1.33.1.2.3",
	"UPS-MIB::upsEstimatedChargeRemaining":  ".1.3.6.1.2.1.33.1.2.4",
	"UPS-MIB::upsBatteryVoltage":            ".1.3.6.1.2.1.33.1.2.5",
	"UPS-MIB::upsBatteryCurrent":            ".1.3.6.1.2.1.33.1.2.6",
	"UPS-MIB::upsBatteryTemperature":        ".1.3.6.1.2.1.33.1.2.7",
	"UPS-MIB::upsInputLineBads":             ".1.3.6.1.2.1.33.1.3.1",
	"UPS-MIB::upsInputNumLines":             ".1.3.6.1.2.1.33.1.3.2",
	"UPS-MIB::upsInputLineIndex":            ".1.3.6.1.2.1.33.1.3.3.1.1",
	"UPS-MIB::upsInputFrequency":            ".1.3.6.1.2.1.33.1.3.3.1.2",
	"UPS-MIB::upsInputVoltage":              ".1.3.6.1.2.1.33.1.3.3.1.3",
	"UPS-MIB::upsInputCurrent":              ".1.3.6.1.2.1.33.1.3.3.1.4",
	"UPS-MIB::upsInputTruePower":            ".1.3.6.1.2.1.33.1.3.3.1.5",
	"UPS-MIB::upsOutputSource":              ".1.3.6.1.2.1.33.1.4.1",
	"UPS-MIB::upsOutputFrequency":           ".1.3.6.1.2.1.33.1.4.2",
	"UPS-MIB::upsOutputNumLines":            ".1.3.6.1.2.1.33.1.4.3",
	"UPS-MIB::upsOutputLineIndex":           ".1.3.6.1.2.1.33.1.4.4.1.1",
	"UPS-MIB::upsOutputVoltage":             ".1.3.6.1.2.1.33.1.4.4.1.2",
	"UPS-MIB::upsOutputCurrent":             ".1.3.6.1.2.1.33.1.4.4.1.3",
	"UPS-MIB::upsOutputPower":               ".1.3.6.1.2.1.33.1.4.4.1.4",
	"UPS-MIB::upsOutputPercentLoad":         ".1.3.6.1.2.1.33.1.4.4.1.5",
	"UPS-MIB::upsBypassFrequency":           ".1.3.6.1.2.1.33.1.5.1",
	"UPS-MIB::upsBypassNumLines":            ".1.3.6.1.2.1.33.1.5.2",
	"UPS-MIB::upsAlarmsPresent":             ".1.3.6.1.2.1.33.1.6.1",
	"UPS-MIB::upsTestId":                    ".1.3.6.1.2.1.33.1.7.1",
	"UPS-MIB::upsTestSpinLock":              ".1.3.6.1.2.1.33.1.7.2",
	"UPS-MIB::upsTestResultsSummary":        ".1.3.6.1.2.1.33.1.7.3",
	"UPS-MIB::upsTestResultsDetail":         ".1.3.6.1.2.1.33.1.7.4",
	"UPS-MIB::upsTestStartTime":             ".1.3.6.1.2.1.33.1.7.5",
	"UPS-MIB::upsTestElapsedTime":           ".1.3.6.1.2.1.33.1.7.6",

	// ============================================================================
	// LLDP-MIB - IEEE 802.1AB
	// ============================================================================
	"LLDP-MIB::lldpLocChassisIdSubtype": ".1.0.8802.1.1.2.1.3.1",
	"LLDP-MIB::lldpLocChassisId":        ".1.0.8802.1.1.2.1.3.2",
	"LLDP-MIB::lldpLocSysName":          ".1.0.8802.1.1.2.1.3.3",
	"LLDP-MIB::lldpLocSysDesc":          ".1.0.8802.1.1.2.1.3.4",
	"LLDP-MIB::lldpLocSysCapSupported":  ".1.0.8802.1.1.2.1.3.5",
	"LLDP-MIB::lldpLocSysCapEnabled":    ".1.0.8802.1.1.2.1.3.6",
	"LLDP-MIB::lldpRemChassisIdSubtype": ".1.0.8802.1.1.2.1.4.1.1.4",
	"LLDP-MIB::lldpRemChassisId":        ".1.0.8802.1.1.2.1.4.1.1.5",
	"LLDP-MIB::lldpRemPortIdSubtype":    ".1.0.8802.1.1.2.1.4.1.1.6",
	"LLDP-MIB::lldpRemPortId":           ".1.0.8802.1.1.2.1.4.1.1.7",
	"LLDP-MIB::lldpRemPortDesc":         ".1.0.8802.1.1.2.1.4.1.1.8",
	"LLDP-MIB::lldpRemSysName":          ".1.0.8802.1.1.2.1.4.1.1.9",
	"LLDP-MIB::lldpRemSysDesc":          ".1.0.8802.1.1.2.1.4.1.1.10",
	"LLDP-MIB::lldpRemSysCapSupported":  ".1.0.8802.1.1.2.1.4.1.1.11",
	"LLDP-MIB::lldpRemSysCapEnabled":    ".1.0.8802.1.1.2.1.4.1.1.12",

	// ============================================================================
	// Printer-MIB - RFC 3805
	// ============================================================================
	"Printer-MIB::prtGeneralConfigChanges":        ".1.3.6.1.2.1.43.5.1.1.1",
	"Printer-MIB::prtGeneralCurrentLocalization":  ".1.3.6.1.2.1.43.5.1.1.2",
	"Printer-MIB::prtGeneralReset":                ".1.3.6.1.2.1.43.5.1.1.3",
	"Printer-MIB::prtGeneralCurrentOperator":      ".1.3.6.1.2.1.43.5.1.1.4",
	"Printer-MIB::prtGeneralServicePerson":        ".1.3.6.1.2.1.43.5.1.1.5",
	"Printer-MIB::prtInputDefaultIndex":           ".1.3.6.1.2.1.43.5.1.1.6",
	"Printer-MIB::prtOutputDefaultIndex":          ".1.3.6.1.2.1.43.5.1.1.7",
	"Printer-MIB::prtMarkerDefaultIndex":          ".1.3.6.1.2.1.43.5.1.1.8",
	"Printer-MIB::prtMediaPathDefaultIndex":       ".1.3.6.1.2.1.43.5.1.1.9",
	"Printer-MIB::prtConsoleLocalization":         ".1.3.6.1.2.1.43.5.1.1.10",
	"Printer-MIB::prtConsoleNumberOfDisplayLines": ".1.3.6.1.2.1.43.5.1.1.11",
	"Printer-MIB::prtConsoleNumberOfDisplayChars": ".1.3.6.1.2.1.43.5.1.1.12",
	"Printer-MIB::prtConsoleDisable":              ".1.3.6.1.2.1.43.5.1.1.13",
	"Printer-MIB::prtCoverIndex":                  ".1.3.6.1.2.1.43.6.1.1.1",
	"Printer-MIB::prtCoverDescription":            ".1.3.6.1.2.1.43.6.1.1.2",
	"Printer-MIB::prtCoverStatus":                 ".1.3.6.1.2.1.43.6.1.1.3",
	"Printer-MIB::prtMarkerSuppliesIndex":         ".1.3.6.1.2.1.43.11.1.1.1",
	"Printer-MIB::prtMarkerSuppliesMarkerIndex":   ".1.3.6.1.2.1.43.11.1.1.2",
	"Printer-MIB::prtMarkerSuppliesColorantIndex": ".1.3.6.1.2.1.43.11.1.1.3",
	"Printer-MIB::prtMarkerSuppliesClass":         ".1.3.6.1.2.1.43.11.1.1.4",
	"Printer-MIB::prtMarkerSuppliesType":          ".1.3.6.1.2.1.43.11.1.1.5",
	"Printer-MIB::prtMarkerSuppliesDescription":   ".1.3.6.1.2.1.43.11.1.1.6",
	"Printer-MIB::prtMarkerSuppliesSupplyUnit":    ".1.3.6.1.2.1.43.11.1.1.7",
	"Printer-MIB::prtMarkerSuppliesMaxCapacity":   ".1.3.6.1.2.1.43.11.1.1.8",
	"Printer-MIB::prtMarkerSuppliesLevel":         ".1.3.6.1.2.1.43.11.1.1.9",
	"Printer-MIB::prtAlertIndex":                  ".1.3.6.1.2.1.43.18.1.1.1",
	"Printer-MIB::prtAlertSeverityLevel":          ".1.3.6.1.2.1.43.18.1.1.2",
	"Printer-MIB::prtAlertTrainingLevel":          ".1.3.6.1.2.1.43.18.1.1.3",
	"Printer-MIB::prtAlertGroup":                  ".1.3.6.1.2.1.43.18.1.1.4",
	"Printer-MIB::prtAlertGroupIndex":             ".1.3.6.1.2.1.43.18.1.1.5",
	"Printer-MIB::prtAlertLocation":               ".1.3.6.1.2.1.43.18.1.1.6",
	"Printer-MIB::prtAlertCode":                   ".1.3.6.1.2.1.43.18.1.1.7",
	"Printer-MIB::prtAlertDescription":            ".1.3.6.1.2.1.43.18.1.1.8",
	"Printer-MIB::prtAlertTime":                   ".1.3.6.1.2.1.43.18.1.1.9",

	// ============================================================================
	// BGP4-MIB - RFC 4273
	// ============================================================================
	"BGP4-MIB::bgpVersion":                           ".1.3.6.1.2.1.15.1",
	"BGP4-MIB::bgpLocalAs":                           ".1.3.6.1.2.1.15.2",
	"BGP4-MIB::bgpPeerIdentifier":                    ".1.3.6.1.2.1.15.3.1.1",
	"BGP4-MIB::bgpPeerState":                         ".1.3.6.1.2.1.15.3.1.2",
	"BGP4-MIB::bgpPeerAdminStatus":                   ".1.3.6.1.2.1.15.3.1.3",
	"BGP4-MIB::bgpPeerNegotiatedVersion":             ".1.3.6.1.2.1.15.3.1.4",
	"BGP4-MIB::bgpPeerLocalAddr":                     ".1.3.6.1.2.1.15.3.1.5",
	"BGP4-MIB::bgpPeerLocalPort":                     ".1.3.6.1.2.1.15.3.1.6",
	"BGP4-MIB::bgpPeerRemoteAddr":                    ".1.3.6.1.2.1.15.3.1.7",
	"BGP4-MIB::bgpPeerRemotePort":                    ".1.3.6.1.2.1.15.3.1.8",
	"BGP4-MIB::bgpPeerRemoteAs":                      ".1.3.6.1.2.1.15.3.1.9",
	"BGP4-MIB::bgpPeerInUpdates":                     ".1.3.6.1.2.1.15.3.1.10",
	"BGP4-MIB::bgpPeerOutUpdates":                    ".1.3.6.1.2.1.15.3.1.11",
	"BGP4-MIB::bgpPeerInTotalMessages":               ".1.3.6.1.2.1.15.3.1.12",
	"BGP4-MIB::bgpPeerOutTotalMessages":              ".1.3.6.1.2.1.15.3.1.13",
	"BGP4-MIB::bgpPeerLastError":                     ".1.3.6.1.2.1.15.3.1.14",
	"BGP4-MIB::bgpPeerFsmEstablishedTransitions":     ".1.3.6.1.2.1.15.3.1.15",
	"BGP4-MIB::bgpPeerFsmEstablishedTime":            ".1.3.6.1.2.1.15.3.1.16",
	"BGP4-MIB::bgpPeerConnectRetryInterval":          ".1.3.6.1.2.1.15.3.1.17",
	"BGP4-MIB::bgpPeerHoldTime":                      ".1.3.6.1.2.1.15.3.1.18",
	"BGP4-MIB::bgpPeerKeepAlive":                     ".1.3.6.1.2.1.15.3.1.19",
	"BGP4-MIB::bgpPeerHoldTimeConfigured":            ".1.3.6.1.2.1.15.3.1.20",
	"BGP4-MIB::bgpPeerKeepAliveConfigured":           ".1.3.6.1.2.1.15.3.1.21",
	"BGP4-MIB::bgpPeerMinASOriginationInterval":      ".1.3.6.1.2.1.15.3.1.22",
	"BGP4-MIB::bgpPeerMinRouteAdvertisementInterval": ".1.3.6.1.2.1.15.3.1.23",
	"BGP4-MIB::bgpPeerInUpdateElapsedTime":           ".1.3.6.1.2.1.15.3.1.24",

	// ============================================================================
	// OSPF-MIB - RFC 4750
	// ============================================================================
	"OSPF-MIB::ospfRouterId":             ".1.3.6.1.2.1.14.1.1",
	"OSPF-MIB::ospfAdminStat":            ".1.3.6.1.2.1.14.1.2",
	"OSPF-MIB::ospfVersionNumber":        ".1.3.6.1.2.1.14.1.3",
	"OSPF-MIB::ospfAreaBdrRtrStatus":     ".1.3.6.1.2.1.14.1.4",
	"OSPF-MIB::ospfASBdrRtrStatus":       ".1.3.6.1.2.1.14.1.5",
	"OSPF-MIB::ospfExternLsaCount":       ".1.3.6.1.2.1.14.1.6",
	"OSPF-MIB::ospfExternLsaCksumSum":    ".1.3.6.1.2.1.14.1.7",
	"OSPF-MIB::ospfTOSSupport":           ".1.3.6.1.2.1.14.1.8",
	"OSPF-MIB::ospfOriginateNewLsas":     ".1.3.6.1.2.1.14.1.9",
	"OSPF-MIB::ospfRxNewLsas":            ".1.3.6.1.2.1.14.1.10",
	"OSPF-MIB::ospfExtLsdbLimit":         ".1.3.6.1.2.1.14.1.11",
	"OSPF-MIB::ospfMulticastExtensions":  ".1.3.6.1.2.1.14.1.12",
	"OSPF-MIB::ospfExitOverflowInterval": ".1.3.6.1.2.1.14.1.13",
	"OSPF-MIB::ospfDemandExtensions":     ".1.3.6.1.2.1.14.1.14",

	// ============================================================================
	// DISMAN-EVENT-MIB - RFC 2981
	// ============================================================================
	"DISMAN-EVENT-MIB::sysUpTimeInstance": ".1.3.6.1.2.1.1.3.0",
}

// Translation cache for snmptranslate results
// Thread-safe cache to avoid repeated exec calls
var (
	mibTranslationCache   = make(map[string]string)
	mibTranslationCacheMu sync.RWMutex
	mibCacheMaxSize       = 1000 // Reasonable limit for production
)

// translateMIB translates MIB name to numeric OID
// Strategy: hardcoded map → cache → snmptranslate (optional) → error
// validateMIBName checks if a MIB name contains only safe characters.
// This prevents command injection when passing to snmptranslate.
// Valid MIB names: alphanumeric, dots, colons, hyphens, underscores.
func validateMIBName(mibName string) error {
	if mibName == "" {
		return fmt.Errorf("empty MIB name")
	}

	// Max length check (reasonable limit for MIB names)
	if len(mibName) > 256 {
		return fmt.Errorf("MIB name too long")
	}

	// Check each character for safety
	for i, ch := range mibName {
		if (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '.' || ch == ':' || ch == '-' || ch == '_' {
			continue
		}
		return fmt.Errorf("invalid character '%c' at position %d in MIB name", ch, i)
	}

	return nil
}

func translateMIB(mibName string) (string, error) {
	// 1. Check if already numeric OID (passthrough)
	if strings.HasPrefix(mibName, ".") || (len(mibName) > 0 && mibName[0] >= '0' && mibName[0] <= '9') {
		return mibName, nil
	}

	// 2. Validate MIB name BEFORE any cache operations (defense in depth)
	if err := validateMIBName(mibName); err != nil {
		return "", fmt.Errorf("invalid MIB name: %w", err)
	}

	// 3. Check hardcoded map (covers ~90% of use cases)
	if oid, found := mibToOID[mibName]; found {
		return oid, nil
	}

	// 4. Check translation cache (from previous snmptranslate calls)
	mibTranslationCacheMu.RLock()
	if oid, found := mibTranslationCache[mibName]; found {
		mibTranslationCacheMu.RUnlock()
		return oid, nil
	}
	mibTranslationCacheMu.RUnlock()

	// 5. Try snmptranslate (optional - fails gracefully if not installed)
	// Use timeout to prevent hanging on slow/stuck snmptranslate
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "snmptranslate", "-On", mibName)
	output, err := cmd.Output()
	if err == nil {
		oid := strings.TrimSpace(string(output))
		// Validate OID format before caching
		if strings.HasPrefix(oid, ".") {
			// Cache the result (with size limit)
			mibTranslationCacheMu.Lock()
			if len(mibTranslationCache) < mibCacheMaxSize {
				mibTranslationCache[mibName] = oid
			}
			mibTranslationCacheMu.Unlock()
			return oid, nil
		}
	}

	// 5. Unknown MIB - fail with helpful error message
	return "", fmt.Errorf("unknown MIB: %s (install net-snmp or use numeric OIDs)", mibName)
}

// parseSNMPWalkAll parses all SNMP entries from snmpwalk output.
// Returns a slice of all parsed entries, preserving order.
// This handles the same complex formats as parseSNMPWalk but returns all entries.
func parseSNMPWalkAll(data string, logger Logger) ([]snmpEntry, error) {
	lines := strings.Split(data, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var entries []snmpEntry

	for i := 0; i < len(lines); {
		line := lines[i]

		// Normalize textual OIDs to numeric format (e.g., IF-MIB::ifDescr.1 -> .1.3.6.1.2.1.2.2.1.2.1)
		line = normalizeTextualOIDLine(line)

		// Check if this is a new entry (starts with OID)
		if !strings.HasPrefix(strings.TrimSpace(line), ".") {
			if strings.TrimSpace(line) != "" {
				logger.Debug("skipping non-OID line in snmp walk output", "line", line, "lineNumber", i+1)
			}
			i++
			continue
		}

		// Parse line using helper
		oid, valueType, valueStr, ok := parseSNMPLine(line)
		if !ok {
			logger.Debug("skipping malformed snmp line", "line", line, "lineNumber", i+1)
			i++
			continue
		}

		// Handle "Wrong Type" warnings - extract actual type and value
		if strings.HasPrefix(valueType, "Wrong Type") {
			actualTypeParts := strings.SplitN(valueStr, ":", 2)
			if len(actualTypeParts) == 2 {
				valueType = strings.TrimSpace(actualTypeParts[0])
				valueStr = strings.TrimLeft(actualTypeParts[1], " ")
			} else {
				return nil, fmt.Errorf("cannot parse snmp walk output")
			}
		}

		// Collect multiline values using helper
		finalValue, consumed := collectMultilineValue(valueType, valueStr, lines, i)

		entries = append(entries, snmpEntry{
			oid:       oid,
			valueType: valueType,
			value:     finalValue,
		})

		i += consumed + 1
	}

	return entries, nil
}

// parseSNMPWalk parses SNMP walk text output and extracts value for target OID.
//
// This custom parser processes text output from the snmpwalk command-line tool
// (not binary SNMP protocol data). It handles complex SNMP data formats including:
//   - Multiple value types: STRING, INTEGER, Hex-STRING, Timeticks, Counter32, etc.
//   - Multiline quoted strings with proper quote tracking
//   - Multiline hex strings with continuation lines
//   - MIB name to numeric OID translation
//   - "Wrong Type" warnings with type/value extraction
//   - NULL and empty string handling
//
// Example SNMP walk format:
//
//	.1.3.6.1.2.1.1.1.0 = STRING: "Linux server 5.4.0"
//	.1.3.6.1.2.1.1.3.0 = Timeticks: (12345) 0:02:03.45
//
// Parameters:
//   - data: Text output from snmpwalk command (one or more OID=value lines)
//   - targetOID: OID to extract (supports MIB names like "SNMPv2-MIB::sysDescr.0")
//
// Returns:
//   - snmpEntry: Parsed entry containing OID, value type, and value
//   - error: Parsing error if format is invalid or target OID not found
//
// This function cannot use gosnmp library because it parses command-line tool output,
// not binary SNMP protocol data. No standard library exists for this text format.
// normalizeOID adds a leading dot to numeric OIDs if missing
func normalizeOID(oid string) string {
	if !strings.HasPrefix(oid, ".") && len(oid) > 0 && oid[0] >= '0' && oid[0] <= '9' {
		return "." + oid
	}
	return oid
}

// normalizeTextualOIDLine converts textual OID lines to numeric OID format.
// Input:  "IF-MIB::ifDescr.1 = STRING: \"eth0\""
// Output: ".1.3.6.1.2.1.2.2.1.2.1 = STRING: \"eth0\""
//
// Returns original line unchanged if:
// - Not a textual OID format (no "::" before "=")
// - MIB name is not in the built-in translation table (translateMIB fails)
//
// IMPORTANT: Unknown MIBs (vendor-specific, custom, or not in hardcoded map) are
// returned as-is. Since the parser expects lines starting with ".", these untranslated
// textual OID lines will be silently skipped. For full textual OID support, either:
// - Use numeric OIDs in snmpwalk output (snmpwalk -On)
// - Extend translateMIB() with additional MIB mappings
// - Use snmptranslate externally before passing to this library
func normalizeTextualOIDLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return line
	}

	// Check for textual OID format (contains :: before =)
	eqIdx := strings.Index(trimmed, "=")
	if eqIdx == -1 {
		return line
	}

	oidPart := strings.TrimSpace(trimmed[:eqIdx])

	// Check if OID contains :: (textual format)
	if !strings.Contains(oidPart, "::") {
		return line
	}

	// Extract MIB name and instance suffix
	// Format: MIB-NAME::objectName.instance or MIB-NAME::objectName
	// Split on :: to get MIB::object
	parts := strings.SplitN(oidPart, "::", 2)
	if len(parts) != 2 {
		return line
	}

	mibPrefix := parts[0]
	objectAndInstance := parts[1]

	// Split object name from instance (e.g., "ifDescr.1" -> "ifDescr", "1")
	var objectName, instanceSuffix string
	dotIdx := strings.Index(objectAndInstance, ".")
	if dotIdx == -1 {
		objectName = objectAndInstance
		instanceSuffix = ""
	} else {
		objectName = objectAndInstance[:dotIdx]
		instanceSuffix = objectAndInstance[dotIdx:] // Includes the dot
	}

	// Construct full MIB name for translation
	fullMIBName := mibPrefix + "::" + objectName

	// Try to translate
	numericOID, err := translateMIB(fullMIBName)
	if err != nil {
		// Can't translate - return original line (will be skipped later)
		return line
	}

	// Reconstruct line with numeric OID
	newOID := numericOID + instanceSuffix
	rest := trimmed[eqIdx:]
	return newOID + " " + rest
}

// parseSNMPLine parses a single SNMP walk output line into OID, type, and value
func parseSNMPLine(line string) (oid, valueType, value string, ok bool) {
	// Split on = to get OID and rest
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", "", false
	}

	oid = strings.TrimSpace(parts[0])
	rest := strings.TrimSpace(parts[1])

	// Extract type and value (format: "TYPE: value")
	typeParts := strings.SplitN(rest, ":", 2)

	if len(typeParts) == 2 {
		// Format: "TYPE: value"
		valueType = strings.TrimSpace(typeParts[0])
		value = strings.TrimLeft(typeParts[1], " ")
		return oid, valueType, value, true
	}

	// No colon - could be "NULL", empty quotes "", or raw value
	trimmed := strings.TrimSpace(typeParts[0])
	if trimmed == "NULL" {
		return oid, "NULL", "", true
	}
	if trimmed == "\"\"" {
		return oid, "EMPTY", "", true
	}
	// Arbitrary value without type
	return oid, "", trimmed, true
}

// isMultilineValue determines if an SNMP value requires multiline processing
func isMultilineValue(valueType, value string) bool {
	if valueType == "Hex-STRING" {
		return true
	}
	if valueType == "STRING" {
		// Quoted string without closing quote, or unquoted string
		if strings.HasPrefix(value, "\"") && !strings.HasSuffix(strings.TrimRight(value, " \t"), "\"") {
			return true
		}
		if !strings.HasPrefix(value, "\"") {
			return true
		}
	}
	return false
}

// appendContinuationLine adds a continuation line to a multiline SNMP value
func appendContinuationLine(entry *snmpEntry, line string) {
	if entry.valueType == "Hex-STRING" {
		// For Hex-STRING, append with space separator (trimmed)
		entry.value += " " + strings.TrimSpace(line)
	} else {
		// For STRING, append with newline preserved
		entry.value += "\n" + line
	}
}

// isStringClosed checks if a multiline STRING value is now complete
func isStringClosed(valueType, line string) bool {
	if valueType != "STRING" {
		return false
	}
	trimmed := strings.TrimRight(line, " \t")
	// Closed if ends with unescaped quote
	return strings.HasSuffix(trimmed, "\"") && !strings.HasSuffix(trimmed, "\\\"")
}

// collectMultilineValue collects continuation lines for multiline SNMP values
// Returns the complete value and the number of continuation lines consumed
func collectMultilineValue(valueType, initialValue string, lines []string, startIdx int) (string, int) {
	value := initialValue
	consumed := 0

	if valueType == "STRING" && strings.HasPrefix(initialValue, "\"") {
		trimmed := strings.TrimRight(initialValue, " \t")
		isMultiline := (trimmed == "\"") || (!strings.HasSuffix(trimmed, "\""))

		if isMultiline {
			// Multiline quoted string
			for startIdx+consumed+1 < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[startIdx+consumed+1]), ".") {
				consumed++
				value += "\n" + lines[startIdx+consumed]
				if isStringClosed(valueType, lines[startIdx+consumed]) {
					break
				}
			}
		}
	} else if valueType == "Hex-STRING" {
		// Multiline hex-string - continuation lines are indented hex bytes
		for startIdx+consumed+1 < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[startIdx+consumed+1]), ".") {
			consumed++
			value += " " + strings.TrimSpace(lines[startIdx+consumed])
		}
	}

	return value, consumed
}

func parseSNMPWalk(data string, targetOID string) (snmpEntry, error) {
	// Translate MIB name to numeric OID (if needed)
	translatedOID, err := translateMIB(targetOID)
	if err != nil {
		return snmpEntry{}, err
	}
	targetOID = normalizeOID(translatedOID)

	// Split lines and remove trailing empty line (from YAML | formatting)
	lines := strings.Split(data, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var current snmpEntry
	inMultiline := false
	wentMultiline := false // Track if we actually appended continuation lines

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Normalize textual OIDs to numeric format (e.g., IF-MIB::ifDescr.1 -> .1.3.6.1.2.1.2.2.1.2.1)
		line = normalizeTextualOIDLine(line)

		// Check if this is a new entry (starts with OID)
		if strings.HasPrefix(strings.TrimSpace(line), ".") {
			// If we were in multiline and found our target, check if it's complete
			if inMultiline && current.oid == targetOID {
				// Check for broken quoting - if we started with a quote but never closed it
				if current.valueType == "STRING" && strings.HasPrefix(strings.TrimSpace(current.value), "\"") {
					// Still in quoted mode but hit new OID - broken quoting
					return snmpEntry{}, fmt.Errorf("malformed quoted string: missing closing quote")
				}
				// Only preserve whitespace for STRING if we actually went multiline
				if current.valueType != "STRING" || !wentMultiline {
					current.value = strings.TrimSpace(current.value)
				}
				return current, nil
			}

			// Parse new entry using helper
			oid, valueType, valueStr, ok := parseSNMPLine(line)
			if !ok {
				continue
			}

			current = snmpEntry{
				oid:       oid,
				valueType: valueType,
				value:     valueStr,
			}

			// Check if this is a multiline value using helper
			inMultiline = isMultilineValue(valueType, valueStr)

			// If not multiline and this is our target, we're done
			if !inMultiline && oid == targetOID {
				return current, nil
			}
		} else if inMultiline {
			// Continuation line
			wentMultiline = true
			appendContinuationLine(&current, line)

			// Check if STRING is now closed using helper
			if isStringClosed(current.valueType, line) {
				inMultiline = false
				if current.oid == targetOID {
					// Don't trim - preserve exact formatting
					return current, nil
				}
			}
		}
	}

	// Check if last entry matches (for cases where file ends without newline)
	if current.oid == targetOID {
		// Only preserve whitespace for STRING if we actually went multiline
		if current.valueType != "STRING" || !wentMultiline {
			current.value = strings.TrimSpace(current.value)
		}
		return current, nil
	}

	return snmpEntry{}, fmt.Errorf("OID not found: %s", targetOID)
}

// formatSNMPValue applies format conversion to SNMP value
// trimFloatZeros: whether to trim trailing zeros from Opaque floats (true for WALK_VALUE, false for WALK_TO_JSON)
func formatSNMPValue(entry snmpEntry, formatMode int, trimFloatZeros bool) (string, error) {
	switch entry.valueType {
	case "STRING":
		return formatSTRING(entry.value, formatMode)
	case "Hex-STRING":
		return formatHexSTRING(entry.value, formatMode)
	case "INTEGER":
		return strings.TrimSpace(entry.value), nil
	case "BITS":
		if formatMode == 3 {
			return convertBITSToInteger(entry.value)
		}
		return strings.TrimSpace(entry.value), nil
	case "IpAddress", "Counter32", "Gauge32", "Counter64", "Timeticks":
		// Return value as-is for these types
		return strings.TrimSpace(entry.value), nil
	case "Opaque":
		// Opaque can have wrapped types - parse them
		return formatOpaque(entry.value, trimFloatZeros), nil
	case "OID":
		return strings.TrimSpace(entry.value), nil
	case "NULL":
		// NULL type returns "NULL" as a string
		return "NULL", nil
	case "EMPTY":
		// Empty quotes - return empty string
		return "", nil
	case "":
		// No type specified - return value as-is (arbitrary number)
		return strings.TrimSpace(entry.value), nil
	default:
		// Unknown type - return value as-is
		return strings.TrimSpace(entry.value), nil
	}
}

// formatSTRING handles STRING formatting
func formatSTRING(value string, formatMode int) (string, error) {
	// Check if it's a quoted string (trim only for quote detection)
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") && len(trimmed) >= 2 {
		// Unescape the quoted string
		unquoted := trimmed[1 : len(trimmed)-1]
		// Replace escape sequences
		unquoted = strings.ReplaceAll(unquoted, "\\\"", "\"")
		unquoted = strings.ReplaceAll(unquoted, "\\\\", "\\")
		return unquoted, nil
	}
	// Unquoted string - return as-is (preserve whitespace)
	return value, nil
}

// formatHexSTRING handles Hex-STRING formatting
func formatHexSTRING(value string, formatMode int) (string, error) {
	// Clean up the hex string (remove extra spaces)
	value = strings.TrimSpace(value)

	switch formatMode {
	case 0:
		// Unchanged - normalize spaces
		parts := strings.Fields(value)
		return strings.Join(parts, " "), nil

	case 1:
		// UTF-8 conversion
		bytes, err := parseHexBytes(value)
		if err != nil {
			return "", err
		}
		// Remove null terminator if present
		if len(bytes) > 0 && bytes[len(bytes)-1] == 0 {
			bytes = bytes[:len(bytes)-1]
		}
		// Convert to string and replace invalid UTF-8 bytes with '?'
		result := string(bytes)
		if !utf8.ValidString(result) {
			// Iterate over bytes and decode runes manually to properly handle
			// both invalid bytes and legitimate U+FFFD characters
			var fixed strings.Builder
			for i := 0; i < len(bytes); {
				r, size := utf8.DecodeRune(bytes[i:])
				// If DecodeRune returns (RuneError, 1), it's an invalid byte
				// If it returns (RuneError, 3), it's a valid encoding of U+FFFD
				if r == utf8.RuneError && size == 1 {
					fixed.WriteByte('?')
					i++
				} else {
					fixed.WriteRune(r)
					i += size
				}
			}
			return fixed.String(), nil
		}
		return result, nil

	case 2:
		// MAC address format
		bytes, err := parseHexBytes(value)
		if err != nil {
			return "", err
		}
		// Format as colon-separated hex
		var parts []string
		for _, b := range bytes {
			parts = append(parts, fmt.Sprintf("%02X", b))
		}
		return strings.Join(parts, ":"), nil

	default:
		// Unknown format mode - return unchanged
		parts := strings.Fields(value)
		return strings.Join(parts, " "), nil
	}
}

// parseHexBytes parses hex string like "74 65 73 74" into bytes
func parseHexBytes(hexStr string) ([]byte, error) {
	parts := strings.Fields(hexStr)
	var bytes []byte
	for _, part := range parts {
		b, err := hex.DecodeString(part)
		if err != nil {
			return nil, fmt.Errorf("invalid hex string: %v", err)
		}
		bytes = append(bytes, b...)
	}
	return bytes, nil
}

// convertBITSToInteger converts BITS hex string to integer (little-endian)
func convertBITSToInteger(value string) (string, error) {
	bytes, err := parseHexBytes(value)
	if err != nil {
		return "", err
	}

	// BITS uses little-endian byte order
	// Reverse the bytes
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}

	// Convert bytes to integer (big-endian after reversal)
	var result uint64
	for _, b := range bytes {
		result = (result << 8) | uint64(b)
	}

	return fmt.Sprintf("%d", result), nil
}

// formatOpaque handles Opaque wrapped types
func formatOpaque(value string, trimFloatZeros bool) string {
	// Opaque can have wrapped types like "Float: 0.460000" or "STRING: \"hello\""
	// Extract the wrapped type and value
	parts := strings.SplitN(value, ":", 2)
	if len(parts) == 2 {
		wrappedType := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// For STRING wrapped type, strip quotes
		if wrappedType == "STRING" && len(val) >= 2 && strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") {
			val = val[1 : len(val)-1]
		}

		// For Float wrapped type, optionally trim trailing zeros
		if wrappedType == "Float" && trimFloatZeros && strings.Contains(val, ".") {
			val = strings.TrimRight(val, "0")
			val = strings.TrimRight(val, ".")
		}

		// For other types (Unsigned32, etc.), return value as-is
		return val
	}
	return strings.TrimSpace(value)
}

// snmpGetValue formats raw SNMP hex-string value.
// Format modes:
// 0 = unchanged (return raw hex as-is with normalized spaces)
// 1 = UTF-8 (decode hex-string to UTF-8 string)
// 2 = MAC (format hex-string as MAC address with colons)
// 3 = BITS (convert BITS to integer)
func snmpGetValue(value Value, params string) (Value, error) {
	lines := strings.Split(params, "\n")
	if len(lines) < 1 {
		return Value{}, fmt.Errorf("snmp get value requires format mode parameter")
	}

	formatMode := 0
	if len(lines) >= 1 && strings.TrimSpace(lines[0]) != "" {
		var err error
		formatMode, err = strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil {
			return Value{}, fmt.Errorf("invalid format mode: %v", err)
		}
	}

	// For BITS mode, convert directly
	if formatMode == 3 {
		result, err := convertBITSToInteger(value.Data)
		if err != nil {
			return Value{}, err
		}
		return Value{Data: result, Type: ValueTypeStr}, nil
	}

	// Apply format conversion to hex-string input
	result, err := formatHexSTRING(value.Data, formatMode)
	if err != nil {
		return Value{}, err
	}

	return Value{Data: result, Type: ValueTypeStr}, nil
}

// snmpWalkToJSON converts SNMP walk output to JSON discovery format.
// Params format (one entry per 3 lines):
//
//	MACRO_NAME
//	OID_PREFIX
//	FORMAT_MODE
func snmpWalkToJSON(value Value, params string, logger Logger) (Value, error) {
	// Parse params into macro definitions
	type macroDef struct {
		name       string
		oidPrefix  string
		formatMode int
	}

	var macros []macroDef
	lines := strings.Split(params, "\n")

	// Validate params format: must be triplets (macro, oid, format)
	// Exception: empty params (all whitespace) is valid and returns []
	trimmedParams := strings.TrimSpace(params)
	if trimmedParams != "" && len(lines)%3 != 0 {
		return Value{}, fmt.Errorf("snmp walk to json requires parameters in triplets (macro, oid, format)")
	}

	for i := 0; i+2 < len(lines); i += 3 {
		name := strings.TrimSpace(lines[i])
		oidPrefix := strings.TrimSpace(lines[i+1])
		formatModeStr := strings.TrimSpace(lines[i+2])

		if name == "" || oidPrefix == "" {
			continue
		}

		formatMode := 0
		if formatModeStr != "" {
			var err error
			formatMode, err = strconv.Atoi(formatModeStr)
			if err != nil {
				return Value{}, fmt.Errorf("invalid format mode: %v", err)
			}
		}

		// Translate MIB name to numeric OID (supports textual OIDs like IF-MIB::ifDescr)
		translatedOID, err := translateMIB(oidPrefix)
		if err != nil {
			return Value{}, err
		}
		oidPrefix = normalizeOID(translatedOID)

		macros = append(macros, macroDef{
			name:       name,
			oidPrefix:  oidPrefix,
			formatMode: formatMode,
		})
	}

	// If no macros defined, return empty array
	if len(macros) == 0 {
		return Value{Data: "[]", Type: ValueTypeStr}, nil
	}

	// Parse SNMP walk data using shared parser
	entries, err := parseSNMPWalkAll(value.Data, logger)
	if err != nil {
		return Value{}, err
	}

	// Group entries by index (last OID component)
	type indexData struct {
		index  string
		values map[string]string
	}
	indices := make(map[string]*indexData)

	// Process each parsed entry
	for _, entry := range entries {
		// Check which macro this OID matches
		for _, macro := range macros {
			// Build match prefix - add dot only if not already present
			matchPrefix := macro.oidPrefix
			if !strings.HasSuffix(matchPrefix, ".") {
				matchPrefix += "."
			}

			if strings.HasPrefix(entry.oid, matchPrefix) {
				// Extract index (everything after the prefix)
				index := strings.TrimPrefix(entry.oid, matchPrefix)

				// Get or create index data
				if indices[index] == nil {
					indices[index] = &indexData{
						index:  index,
						values: make(map[string]string),
					}
				}

				// Format the value (preserve float zeros for WALK_TO_JSON)
				formatted, err := formatSNMPValue(entry, macro.formatMode, false)
				if err != nil {
					return Value{}, err
				}

				indices[index].values[macro.name] = formatted
				break
			}
		}
	}

	// If no data found for any macro, check if it's due to bad data or just empty input
	if len(indices) == 0 {
		// If we found valid SNMP entries but none matched our macros, that's an error
		// If we found no valid entries at all (empty or bad data), return error only for bad data
		if len(entries) > 0 {
			// Found valid SNMP entries but none matched - this is an error
			return Value{}, fmt.Errorf("cannot convert snmp walk to json")
		}
		// No valid entries found - could be empty input (ok) or bad data (error)
		// Distinguish by checking if input has non-empty, non-whitespace content
		trimmedData := strings.TrimSpace(value.Data)
		if trimmedData != "" {
			// Non-empty input but no valid SNMP entries - bad data
			return Value{}, fmt.Errorf("cannot convert snmp walk to json")
		}
		// Empty input - return empty array
		return Value{Data: "[]", Type: ValueTypeStr}, nil
	}

	// Convert to JSON array, sorted by index in descending order
	var indexList []string
	for idx := range indices {
		indexList = append(indexList, idx)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(indexList)))

	// Build JSON manually to preserve field order: {#SNMPINDEX} first, then macros in order
	var jsonParts []string
	for _, idx := range indexList {
		data := indices[idx]

		// Start with {#SNMPINDEX}
		var fields []string
		fields = append(fields, fmt.Sprintf("\"{#SNMPINDEX}\":%q", data.index))

		// Add macros in definition order
		for _, macro := range macros {
			if val, ok := data.values[macro.name]; ok {
				// Handle NULL values as JSON null
				if val == "NULL" {
					fields = append(fields, fmt.Sprintf("%q:null", macro.name))
				} else {
					fields = append(fields, fmt.Sprintf("%q:%q", macro.name, val))
				}
			}
		}

		jsonParts = append(jsonParts, "{"+strings.Join(fields, ",")+"}")
	}

	return Value{Data: "[" + strings.Join(jsonParts, ",") + "]", Type: ValueTypeStr}, nil
}

// snmpWalkToJSONMulti converts SNMP walk data to multiple discovery metrics
// Each discovery item becomes a separate Result.Metric with index label
func snmpWalkToJSONMulti(value Value, params string, logger Logger) (Result, error) {
	// Parse params into macro definitions
	type macroDef struct {
		name       string
		oidPrefix  string
		formatMode int
	}

	var macros []macroDef
	lines := strings.Split(params, "\n")

	// Validate params format: must be triplets (macro, oid, format)
	// Exception: empty params (all whitespace) is valid and returns []
	trimmedParams := strings.TrimSpace(params)
	if trimmedParams != "" && len(lines)%3 != 0 {
		err := fmt.Errorf("snmp walk to json requires parameters in triplets (macro, oid, format)")
		return Result{Error: err}, err
	}

	for i := 0; i+2 < len(lines); i += 3 {
		name := strings.TrimSpace(lines[i])
		oidPrefix := strings.TrimSpace(lines[i+1])
		formatModeStr := strings.TrimSpace(lines[i+2])

		if name == "" || oidPrefix == "" {
			continue
		}

		formatMode := 0
		if formatModeStr != "" {
			var err error
			formatMode, err = strconv.Atoi(formatModeStr)
			if err != nil {
				err = fmt.Errorf("invalid format mode: %v", err)
				return Result{Error: err}, err
			}
		}

		// Translate MIB name to numeric OID (supports textual OIDs like IF-MIB::ifDescr)
		translatedOID, err := translateMIB(oidPrefix)
		if err != nil {
			return Result{Error: err}, err
		}
		oidPrefix = normalizeOID(translatedOID)

		macros = append(macros, macroDef{
			name:       name,
			oidPrefix:  oidPrefix,
			formatMode: formatMode,
		})
	}

	// If no macros defined, return empty metrics array
	if len(macros) == 0 {
		return Result{Metrics: []Metric{}}, nil
	}

	// Parse SNMP walk data using shared parser
	entries, err := parseSNMPWalkAll(value.Data, logger)
	if err != nil {
		return Result{Error: err}, err
	}

	// Group entries by index (last OID component)
	type indexData struct {
		index  string
		values map[string]string
	}
	indices := make(map[string]*indexData)

	// Process each parsed entry
	for _, entry := range entries {
		// Check which macro this OID matches
		for _, macro := range macros {
			// Build match prefix - add dot only if not already present
			matchPrefix := macro.oidPrefix
			if !strings.HasSuffix(matchPrefix, ".") {
				matchPrefix += "."
			}

			if strings.HasPrefix(entry.oid, matchPrefix) {
				// Extract index (everything after the prefix)
				index := strings.TrimPrefix(entry.oid, matchPrefix)

				// Get or create index data
				if indices[index] == nil {
					indices[index] = &indexData{
						index:  index,
						values: make(map[string]string),
					}
				}

				// Format the value (preserve float zeros for WALK_TO_JSON)
				formatted, err := formatSNMPValue(entry, macro.formatMode, false)
				if err != nil {
					return Result{Error: err}, err
				}

				indices[index].values[macro.name] = formatted
				break
			}
		}
	}

	// If no data found for any macro, check if it's due to bad data or just empty input
	if len(indices) == 0 {
		// If we found valid SNMP entries but none matched our macros, that's an error
		// If we found no valid entries at all (empty or bad data), return error only for bad data
		if len(entries) > 0 {
			// Found valid SNMP entries but none matched - this is an error
			err := fmt.Errorf("cannot convert snmp walk to json")
			return Result{Error: err}, err
		}
		// No valid entries found - could be empty input (ok) or bad data (error)
		// Distinguish by checking if input has non-empty, non-whitespace content
		trimmedData := strings.TrimSpace(value.Data)
		if trimmedData != "" {
			// Non-empty input but no valid SNMP entries - bad data
			err := fmt.Errorf("cannot convert snmp walk to json")
			return Result{Error: err}, err
		}
		// Empty input - return empty metrics array
		return Result{Metrics: []Metric{}}, nil
	}

	// Convert to multiple metrics, sorted by index in descending order
	var indexList []string
	for idx := range indices {
		indexList = append(indexList, idx)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(indexList)))

	// Build Result with multiple Metric objects
	metrics := make([]Metric, 0, len(indexList))
	for _, idx := range indexList {
		data := indices[idx]

		// Build JSON object for this discovery item
		// Start with {#SNMPINDEX}
		var fields []string
		fields = append(fields, fmt.Sprintf("\"{#SNMPINDEX}\":%q", data.index))

		// Add macros in definition order
		for _, macro := range macros {
			if val, ok := data.values[macro.name]; ok {
				// Handle NULL values as JSON null
				if val == "NULL" {
					fields = append(fields, fmt.Sprintf("%q:null", macro.name))
				} else {
					fields = append(fields, fmt.Sprintf("%q:%q", macro.name, val))
				}
			}
		}

		jsonObj := "{" + strings.Join(fields, ",") + "}"

		// Create metric with index label
		metrics = append(metrics, Metric{
			Name:   "snmp_discovery",
			Value:  jsonObj,
			Type:   ValueTypeStr,
			Labels: map[string]string{"index": data.index},
		})
	}

	return Result{Metrics: metrics}, nil
}
