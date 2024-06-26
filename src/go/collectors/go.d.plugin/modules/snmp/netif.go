// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"strings"
)

const (
	oidIfIndex           = "1.3.6.1.2.1.2.2.1.1"
	oidIfDescr           = "1.3.6.1.2.1.2.2.1.2"
	oidIfType            = "1.3.6.1.2.1.2.2.1.3"
	oidIfMtu             = "1.3.6.1.2.1.2.2.1.4"
	oidIfSpeed           = "1.3.6.1.2.1.2.2.1.5"
	oidIfPhysAddress     = "1.3.6.1.2.1.2.2.1.6"
	oidIfAdminStatus     = "1.3.6.1.2.1.2.2.1.7"
	oidIfOperStatus      = "1.3.6.1.2.1.2.2.1.8"
	oidIfLastChange      = "1.3.6.1.2.1.2.2.1.9"
	oidIfInOctets        = "1.3.6.1.2.1.2.2.1.10"
	oidIfInUcastPkts     = "1.3.6.1.2.1.2.2.1.11"
	oidIfInNUcastPkts    = "1.3.6.1.2.1.2.2.1.12"
	oidIfInDiscards      = "1.3.6.1.2.1.2.2.1.13"
	oidIfInErrors        = "1.3.6.1.2.1.2.2.1.14"
	oidIfInUnknownProtos = "1.3.6.1.2.1.2.2.1.15"
	oidIfOutOctets       = "1.3.6.1.2.1.2.2.1.16"
	oidIfOutUcastPkts    = "1.3.6.1.2.1.2.2.1.17"
	oidIfOutNUcastPkts   = "1.3.6.1.2.1.2.2.1.18"
	oidIfOutDiscards     = "1.3.6.1.2.1.2.2.1.19"
	oidIfOutErrors       = "1.3.6.1.2.1.2.2.1.20"

	oidIfName               = "1.3.6.1.2.1.31.1.1.1.1"
	oidIfInMulticastPkts    = "1.3.6.1.2.1.31.1.1.1.2"
	oidIfInBroadcastPkts    = "1.3.6.1.2.1.31.1.1.1.3"
	oidIfOutMulticastPkts   = "1.3.6.1.2.1.31.1.1.1.4"
	oidIfOutBroadcastPkts   = "1.3.6.1.2.1.31.1.1.1.5"
	oidIfHCInOctets         = "1.3.6.1.2.1.31.1.1.1.6"
	oidIfHCInUcastPkts      = "1.3.6.1.2.1.31.1.1.1.7"
	oidIfHCInMulticastPkts  = "1.3.6.1.2.1.31.1.1.1.8"
	oidIfHCInBroadcastPkts  = "1.3.6.1.2.1.31.1.1.1.9"
	oidIfHCOutOctets        = "1.3.6.1.2.1.31.1.1.1.10"
	oidIfHCOutUcastPkts     = "1.3.6.1.2.1.31.1.1.1.11"
	oidIfHCOutMulticastPkts = "1.3.6.1.2.1.31.1.1.1.12"
	oidIfHCOutBroadcastPkts = "1.3.6.1.2.1.31.1.1.1.13"
	oidIfHighSpeed          = "1.3.6.1.2.1.31.1.1.1.15"
	oidIfAlias              = "1.3.6.1.2.1.31.1.1.1.18"
)

type netInterface struct {
	updated   bool
	hasCharts bool
	idx       string

	ifIndex int64
	ifDescr string
	ifType  int64
	ifMtu   int64
	ifSpeed int64
	//ifPhysAddress        string
	ifAdminStatus int64
	ifOperStatus  int64
	//ifLastChange         string
	ifInOctets           int64
	ifInUcastPkts        int64
	ifInNUcastPkts       int64
	ifInDiscards         int64
	ifInErrors           int64
	ifInUnknownProtos    int64
	ifOutOctets          int64
	ifOutUcastPkts       int64
	ifOutNUcastPkts      int64
	ifOutDiscards        int64
	ifOutErrors          int64
	ifName               string
	ifInMulticastPkts    int64
	ifInBroadcastPkts    int64
	ifOutMulticastPkts   int64
	ifOutBroadcastPkts   int64
	ifHCInOctets         int64
	ifHCInUcastPkts      int64
	ifHCInMulticastPkts  int64
	ifHCInBroadcastPkts  int64
	ifHCOutOctets        int64
	ifHCOutUcastPkts     int64
	ifHCOutMulticastPkts int64
	ifHCOutBroadcastPkts int64
	ifHighSpeed          int64
	ifAlias              string
}

func (n *netInterface) String() string {
	return fmt.Sprintf("iface index='%d',type='%s',name='%s',descr='%s',alias='%s'",
		n.ifIndex, ifTypeMapping[n.ifType], n.ifName, n.ifDescr, strings.ReplaceAll(n.ifAlias, "\n", "\\n"))
}

var ifAdminStatusMapping = map[int64]string{
	1: "up",
	2: "down",
	3: "testing",
}

var ifOperStatusMapping = map[int64]string{
	1: "up",
	2: "down",
	3: "testing",
	4: "unknown",
	5: "dormant",
	6: "notPresent",
	7: "lowerLayerDown",
}

var ifTypeMapping = map[int64]string{
	1:   "other",
	2:   "regular1822",
	3:   "hdh1822",
	4:   "ddnX25",
	5:   "rfc877x25",
	6:   "ethernetCsmacd",
	7:   "iso88023Csmacd",
	8:   "iso88024TokenBus",
	9:   "iso88025TokenRing",
	10:  "iso88026Man",
	11:  "starLan",
	12:  "proteon10Mbit",
	13:  "proteon80Mbit",
	14:  "hyperchannel",
	15:  "fddi",
	16:  "lapb",
	17:  "sdlc",
	18:  "ds1",
	19:  "e1",
	20:  "basicISDN",
	21:  "primaryISDN",
	22:  "propPointToPointSerial",
	23:  "ppp",
	24:  "softwareLoopback",
	25:  "eon",
	26:  "ethernet3Mbit",
	27:  "nsip",
	28:  "slip",
	29:  "ultra",
	30:  "ds3",
	31:  "sip",
	32:  "frameRelay",
	33:  "rs232",
	34:  "para",
	35:  "arcnet",
	36:  "arcnetPlus",
	37:  "atm",
	38:  "miox25",
	39:  "sonet",
	40:  "x25ple",
	41:  "iso88022llc",
	42:  "localTalk",
	43:  "smdsDxi",
	44:  "frameRelayService",
	45:  "v35",
	46:  "hssi",
	47:  "hippi",
	48:  "modem",
	49:  "aal5",
	50:  "sonetPath",
	51:  "sonetVT",
	52:  "smdsIcip",
	53:  "propVirtual",
	54:  "propMultiplexor",
	55:  "ieee80212",
	56:  "fibreChannel",
	57:  "hippiInterface",
	58:  "frameRelayInterconnect",
	59:  "aflane8023",
	60:  "aflane8025",
	61:  "cctEmul",
	62:  "fastEther",
	63:  "isdn",
	64:  "v11",
	65:  "v36",
	66:  "g703at64k",
	67:  "g703at2mb",
	68:  "qllc",
	69:  "fastEtherFX",
	70:  "channel",
	71:  "ieee80211",
	72:  "ibm370parChan",
	73:  "escon",
	74:  "dlsw",
	75:  "isdns",
	76:  "isdnu",
	77:  "lapd",
	78:  "ipSwitch",
	79:  "rsrb",
	80:  "atmLogical",
	81:  "ds0",
	82:  "ds0Bundle",
	83:  "bsc",
	84:  "async",
	85:  "cnr",
	86:  "iso88025Dtr",
	87:  "eplrs",
	88:  "arap",
	89:  "propCnls",
	90:  "hostPad",
	91:  "termPad",
	92:  "frameRelayMPI",
	93:  "x213",
	94:  "adsl",
	95:  "radsl",
	96:  "sdsl",
	97:  "vdsl",
	98:  "iso88025CRFPInt",
	99:  "myrinet",
	100: "voiceEM",
	101: "voiceFXO",
	102: "voiceFXS",
	103: "voiceEncap",
	104: "voiceOverIp",
	105: "atmDxi",
	106: "atmFuni",
	107: "atmIma",
	108: "pppMultilinkBundle",
	109: "ipOverCdlc",
	110: "ipOverClaw",
	111: "stackToStack",
	112: "virtualIpAddress",
	113: "mpc",
	114: "ipOverAtm",
	115: "iso88025Fiber",
	116: "tdlc",
	117: "gigabitEthernet",
	118: "hdlc",
	119: "lapf",
	120: "v37",
	121: "x25mlp",
	122: "x25huntGroup",
	123: "transpHdlc",
	124: "interleave",
	125: "fast",
	126: "ip",
	127: "docsCableMaclayer",
	128: "docsCableDownstream",
	129: "docsCableUpstream",
	130: "a12MppSwitch",
	131: "tunnel",
	132: "coffee",
	133: "ces",
	134: "atmSubInterface",
	135: "l2vlan",
	136: "l3ipvlan",
	137: "l3ipxvlan",
	138: "digitalPowerline",
	139: "mediaMailOverIp",
	140: "dtm",
	141: "dcn",
	142: "ipForward",
	143: "msdsl",
	144: "ieee1394",
	145: "if-gsn",
	146: "dvbRccMacLayer",
	147: "dvbRccDownstream",
	148: "dvbRccUpstream",
	149: "atmVirtual",
	150: "mplsTunnel",
	151: "srp",
	152: "voiceOverAtm",
	153: "voiceOverFrameRelay",
	154: "idsl",
	155: "compositeLink",
	156: "ss7SigLink",
	157: "propWirelessP2P",
	158: "frForward",
	159: "rfc1483",
	160: "usb",
	161: "ieee8023adLag",
	162: "bgppolicyaccounting",
	163: "frf16MfrBundle",
	164: "h323Gatekeeper",
	165: "h323Proxy",
	166: "mpls",
	167: "mfSigLink",
	168: "hdsl2",
	169: "shdsl",
	170: "ds1FDL",
	171: "pos",
	172: "dvbAsiIn",
	173: "dvbAsiOut",
	174: "plc",
	175: "nfas",
	176: "tr008",
	177: "gr303RDT",
	178: "gr303IDT",
	179: "isup",
	180: "propDocsWirelessMaclayer",
	181: "propDocsWirelessDownstream",
	182: "propDocsWirelessUpstream",
	183: "hiperlan2",
	184: "propBWAp2Mp",
	185: "sonetOverheadChannel",
	186: "digitalWrapperOverheadChannel",
	187: "aal2",
	188: "radioMAC",
	189: "atmRadio",
	190: "imt",
	191: "mvl",
	192: "reachDSL",
	193: "frDlciEndPt",
	194: "atmVciEndPt",
	195: "opticalChannel",
	196: "opticalTransport",
	197: "propAtm",
	198: "voiceOverCable",
	199: "infiniband",
	200: "teLink",
	201: "q2931",
	202: "virtualTg",
	203: "sipTg",
	204: "sipSig",
	205: "docsCableUpstreamChannel",
	206: "econet",
	207: "pon155",
	208: "pon622",
	209: "bridge",
	210: "linegroup",
	211: "voiceEMFGD",
	212: "voiceFGDEANA",
	213: "voiceDID",
	214: "mpegTransport",
	215: "sixToFour",
	216: "gtp",
	217: "pdnEtherLoop1",
	218: "pdnEtherLoop2",
	219: "opticalChannelGroup",
	220: "homepna",
	221: "gfp",
	222: "ciscoISLvlan",
	223: "actelisMetaLOOP",
	224: "fcipLink",
	225: "rpr",
	226: "qam",
	227: "lmp",
	228: "cblVectaStar",
	229: "docsCableMCmtsDownstream",
	230: "adsl2",
	231: "macSecControlledIF",
	232: "macSecUncontrolledIF",
	233: "aviciOpticalEther",
	234: "atmbond",
	235: "voiceFGDOS",
	236: "mocaVersion1",
	237: "ieee80216WMAN",
	238: "adsl2plus",
	239: "dvbRcsMacLayer",
	240: "dvbTdm",
	241: "dvbRcsTdma",
	242: "x86Laps",
	243: "wwanPP",
	244: "wwanPP2",
	245: "voiceEBS",
	246: "ifPwType",
	247: "ilan",
	248: "pip",
	249: "aluELP",
	250: "gpon",
	251: "vdsl2",
	252: "capwapDot11Profile",
	253: "capwapDot11Bss",
	254: "capwapWtpVirtualRadio",
	255: "bits",
	256: "docsCableUpstreamRfPort",
	257: "cableDownstreamRfPort",
	258: "vmwareVirtualNic",
	259: "ieee802154",
	260: "otnOdu",
	261: "otnOtu",
	262: "ifVfiType",
	263: "g9981",
	264: "g9982",
	265: "g9983",
	266: "aluEpon",
	267: "aluEponOnu",
	268: "aluEponPhysicalUni",
	269: "aluEponLogicalLink",
	270: "aluGponOnu",
	271: "aluGponPhysicalUni",
	272: "vmwareNicTeam",
	277: "docsOfdmDownstream",
	278: "docsOfdmaUpstream",
	279: "gfast",
	280: "sdci",
	281: "xboxWireless",
	282: "fastdsl",
	283: "docsCableScte55d1FwdOob",
	284: "docsCableScte55d1RetOob",
	285: "docsCableScte55d2DsOob",
	286: "docsCableScte55d2UsOob",
	287: "docsCableNdf",
	288: "docsCableNdr",
	289: "ptm",
	290: "ghn",
	291: "otnOtsi",
	292: "otnOtuc",
	293: "otnOduc",
	294: "otnOtsig",
	295: "microwaveCarrierTermination",
	296: "microwaveRadioLinkTerminal",
	297: "ieee8021axDrni",
	298: "ax25",
	299: "ieee19061nanocom",
	300: "cpri",
	301: "omni",
	302: "roe",
	303: "p2pOverLan",
}
