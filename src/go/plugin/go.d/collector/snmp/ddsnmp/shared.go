// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

var sharedMappings = struct {
	ifType      map[string]string
	ifTypeGroup map[string]string
}{
	ifType: map[string]string{
		"1":   "other",
		"2":   "regular1822",
		"3":   "hdh1822",
		"4":   "ddnX25",
		"5":   "rfc877x25",
		"6":   "ethernetCsmacd",
		"7":   "iso88023Csmacd",
		"8":   "iso88024TokenBus",
		"9":   "iso88025TokenRing",
		"10":  "iso88026Man",
		"11":  "starLan",
		"12":  "proteon10Mbit",
		"13":  "proteon80Mbit",
		"14":  "hyperchannel",
		"15":  "fddi",
		"16":  "lapb",
		"17":  "sdlc",
		"18":  "ds1",
		"19":  "e1",
		"20":  "basicISDN",
		"21":  "primaryISDN",
		"22":  "propPointToPointSerial",
		"23":  "ppp",
		"24":  "softwareLoopback",
		"25":  "eon",
		"26":  "ethernet3Mbit",
		"27":  "nsip",
		"28":  "slip",
		"29":  "ultra",
		"30":  "ds3",
		"31":  "sip",
		"32":  "frameRelay",
		"33":  "rs232",
		"34":  "para",
		"35":  "arcnet",
		"36":  "arcnetPlus",
		"37":  "atm",
		"38":  "miox25",
		"39":  "sonet",
		"40":  "x25ple",
		"41":  "iso88022llc",
		"42":  "localTalk",
		"43":  "smdsDxi",
		"44":  "frameRelayService",
		"45":  "v35",
		"46":  "hssi",
		"47":  "hippi",
		"48":  "modem",
		"49":  "aal5",
		"50":  "sonetPath",
		"51":  "sonetVT",
		"52":  "smdsIcip",
		"53":  "propVirtual",
		"54":  "propMultiplexor",
		"55":  "ieee80212",
		"56":  "fibreChannel",
		"57":  "hippiInterface",
		"58":  "frameRelayInterconnect",
		"59":  "aflane8023",
		"60":  "aflane8025",
		"61":  "cctEmul",
		"62":  "fastEther",
		"63":  "isdn",
		"64":  "v11",
		"65":  "v36",
		"66":  "g703at64k",
		"67":  "g703at2mb",
		"68":  "qllc",
		"69":  "fastEtherFX",
		"70":  "channel",
		"71":  "ieee80211",
		"72":  "ibm370parChan",
		"73":  "escon",
		"74":  "dlsw",
		"75":  "isdns",
		"76":  "isdnu",
		"77":  "lapd",
		"78":  "ipSwitch",
		"79":  "rsrb",
		"80":  "atmLogical",
		"81":  "ds0",
		"82":  "ds0Bundle",
		"83":  "bsc",
		"84":  "async",
		"85":  "cnr",
		"86":  "iso88025Dtr",
		"87":  "eplrs",
		"88":  "arap",
		"89":  "propCnls",
		"90":  "hostPad",
		"91":  "termPad",
		"92":  "frameRelayMPI",
		"93":  "x213",
		"94":  "adsl",
		"95":  "radsl",
		"96":  "sdsl",
		"97":  "vdsl",
		"98":  "iso88025CRFPInt",
		"99":  "myrinet",
		"100": "voiceEM",
		"101": "voiceFXO",
		"102": "voiceFXS",
		"103": "voiceEncap",
		"104": "voiceOverIp",
		"105": "atmDxi",
		"106": "atmFuni",
		"107": "atmIma",
		"108": "pppMultilinkBundle",
		"109": "ipOverCdlc",
		"110": "ipOverClaw",
		"111": "stackToStack",
		"112": "virtualIpAddress",
		"113": "mpc",
		"114": "ipOverAtm",
		"115": "iso88025Fiber",
		"116": "tdlc",
		"117": "gigabitEthernet",
		"118": "hdlc",
		"119": "lapf",
		"120": "v37",
		"121": "x25mlp",
		"122": "x25huntGroup",
		"123": "transpHdlc",
		"124": "interleave",
		"125": "fast",
		"126": "ip",
		"127": "docsCableMaclayer",
		"128": "docsCableDownstream",
		"129": "docsCableUpstream",
		"130": "a12MppSwitch",
		"131": "tunnel",
		"132": "coffee",
		"133": "ces",
		"134": "atmSubInterface",
		"135": "l2vlan",
		"136": "l3ipvlan",
		"137": "l3ipxvlan",
		"138": "digitalPowerline",
		"139": "mediaMailOverIp",
		"140": "dtm",
		"141": "dcn",
		"142": "ipForward",
		"143": "msdsl",
		"144": "ieee1394",
		"145": "if-gsn",
		"146": "dvbRccMacLayer",
		"147": "dvbRccDownstream",
		"148": "dvbRccUpstream",
		"149": "atmVirtual",
		"150": "mplsTunnel",
		"151": "srp",
		"152": "voiceOverAtm",
		"153": "voiceOverFrameRelay",
		"154": "idsl",
		"155": "compositeLink",
		"156": "ss7SigLink",
		"157": "propWirelessP2P",
		"158": "frForward",
		"159": "rfc1483",
		"160": "usb",
		"161": "ieee8023adLag",
		"162": "bgppolicyaccounting",
		"163": "frf16MfrBundle",
		"164": "h323Gatekeeper",
		"165": "h323Proxy",
		"166": "mpls",
		"167": "mfSigLink",
		"168": "hdsl2",
		"169": "shdsl",
		"170": "ds1FDL",
		"171": "pos",
		"172": "dvbAsiIn",
		"173": "dvbAsiOut",
		"174": "plc",
		"175": "nfas",
		"176": "tr008",
		"177": "gr303RDT",
		"178": "gr303IDT",
		"179": "isup",
		"180": "propDocsWirelessMaclayer",
		"181": "propDocsWirelessDownstream",
		"182": "propDocsWirelessUpstream",
		"183": "hiperlan2",
		"184": "propBWAp2Mp",
		"185": "sonetOverheadChannel",
		"186": "digitalWrapperOverheadChannel",
		"187": "aal2",
		"188": "radioMAC",
		"189": "atmRadio",
		"190": "imt",
		"191": "mvl",
		"192": "reachDSL",
		"193": "frDlciEndPt",
		"194": "atmVciEndPt",
		"195": "opticalChannel",
		"196": "opticalTransport",
		"197": "propAtm",
		"198": "voiceOverCable",
		"199": "infiniband",
		"200": "teLink",
		"201": "q2931",
		"202": "virtualTg",
		"203": "sipTg",
		"204": "sipSig",
		"205": "docsCableUpstreamChannel",
		"206": "econet",
		"207": "pon155",
		"208": "pon622",
		"209": "bridge",
		"210": "linegroup",
		"211": "voiceEMFGD",
		"212": "voiceFGDEANA",
		"213": "voiceDID",
		"214": "mpegTransport",
		"215": "sixToFour",
		"216": "gtp",
		"217": "pdnEtherLoop1",
		"218": "pdnEtherLoop2",
		"219": "opticalChannelGroup",
		"220": "homepna",
		"221": "gfp",
		"222": "ciscoISLvlan",
		"223": "actelisMetaLOOP",
		"224": "fcipLink",
		"225": "rpr",
		"226": "qam",
		"227": "lmp",
		"228": "cblVectaStar",
		"229": "docsCableMCmtsDownstream",
		"230": "adsl2",
		"231": "macSecControlledIF",
		"232": "macSecUncontrolledIF",
		"233": "aviciOpticalEther",
		"234": "atmbond",
		"235": "voiceFGDOS",
		"236": "mocaVersion1",
		"237": "ieee80216WMAN",
		"238": "adsl2plus",
		"239": "dvbRcsMacLayer",
		"240": "dvbTdm",
		"241": "dvbRcsTdma",
		"242": "x86Laps",
		"243": "wwanPP",
		"244": "wwanPP2",
		"245": "voiceEBS",
		"246": "ifPwType",
		"247": "ilan",
		"248": "pip",
		"249": "aluELP",
		"250": "gpon",
		"251": "vdsl2",
		"252": "capwapDot11Profile",
		"253": "capwapDot11Bss",
		"254": "capwapWtpVirtualRadio",
		"255": "bits",
		"256": "docsCableUpstreamRfPort",
		"257": "cableDownstreamRfPort",
		"258": "vmwareVirtualNic",
		"259": "ieee802154",
		"260": "otnOdu",
		"261": "otnOtu",
		"262": "ifVfiType",
		"263": "g9981",
		"264": "g9982",
		"265": "g9983",
		"266": "aluEpon",
		"267": "aluEponOnu",
		"268": "aluEponPhysicalUni",
		"269": "aluEponLogicalLink",
		"270": "aluGponOnu",
		"271": "aluGponPhysicalUni",
		"272": "vmwareNicTeam",
		"277": "docsOfdmDownstream",
		"278": "docsOfdmaUpstream",
		"279": "gfast",
		"280": "sdci",
		"281": "xboxWireless",
		"282": "fastdsl",
		"283": "docsCableScte55d1FwdOob",
		"284": "docsCableScte55d1RetOob",
		"285": "docsCableScte55d2DsOob",
		"286": "docsCableScte55d2UsOob",
		"287": "docsCableNdf",
		"288": "docsCableNdr",
		"289": "ptm",
		"290": "ghn",
		"291": "otnOtsi",
		"292": "otnOtuc",
		"293": "otnOduc",
		"294": "otnOtsig",
		"295": "microwaveCarrierTermination",
		"296": "microwaveRadioLinkTerminal",
		"297": "ieee8021axDrni",
		"298": "ax25",
		"299": "ieee19061nanocom",
		"300": "cpri",
		"301": "omni",
		"302": "roe",
		"303": "p2pOverLan",
	},
	ifTypeGroup: map[string]string{
		// Virtual / logical (includes tunnels, VLANs, bridges)
		"24":  "virtual", // softwareLoopback
		"25":  "virtual", // eon
		"31":  "virtual", // sip (SMDS Interface Protocol)
		"53":  "virtual", // propVirtual
		"78":  "virtual", // ipSwitch
		"109": "virtual", // ipOverCdlc
		"110": "virtual", // ipOverClaw
		"112": "virtual", // virtualIpAddress
		"126": "virtual", // ip
		"131": "virtual", // tunnel
		"135": "virtual", // l2vlan
		"136": "virtual", // l3ipvlan
		"137": "virtual", // l3ipxvlan
		"142": "virtual", // ipForward
		"150": "virtual", // mplsTunnel
		"202": "virtual", // virtualTg
		"209": "virtual", // bridge
		"215": "virtual", // sixToFour
		"222": "virtual", // ciscoISLvlan
		"246": "virtual", // ifPwType
		"258": "virtual", // vmwareVirtualNic
		"262": "virtual", // ifVfiType
		"272": "virtual", // vmwareNicTeam

		// Aggregation / bonding
		"82":  "aggregation", // ds0Bundle
		"108": "aggregation", // pppMultilinkBundle
		"155": "aggregation", // compositeLink
		"161": "aggregation", // ieee8023adLag
		"163": "aggregation", // frf16MfrBundle
		"210": "aggregation", // linegroup
		"297": "aggregation", // ieee8021axDrni

		// Ethernet
		"6":   "ethernet", // ethernetCsmacd
		"7":   "ethernet", // iso88023Csmacd
		"55":  "ethernet", // ieee80212
		"62":  "ethernet", // fastEther
		"69":  "ethernet", // fastEtherFX
		"117": "ethernet", // gigabitEthernet
		"233": "ethernet", // aviciOpticalEther
		"303": "ethernet", // p2pOverLan

		// Wireless (WiFi, WiMAX, cellular, point-to-point, microwave)
		"71":  "wireless", // ieee80211
		"157": "wireless", // propWirelessP2P
		"183": "wireless", // hiperlan2
		"188": "wireless", // radioMAC
		"216": "wireless", // gtp (GPRS Tunneling Protocol - mobile/cellular)
		"237": "wireless", // ieee80216WMAN
		"243": "wireless", // wwanPP
		"244": "wireless", // wwanPP2
		"252": "wireless", // capwapDot11Profile
		"253": "wireless", // capwapDot11Bss
		"254": "wireless", // capwapWtpVirtualRadio
		"281": "wireless", // xboxWireless
		"295": "wireless", // microwaveCarrierTermination
		"296": "wireless", // microwaveRadioLinkTerminal
		"299": "wireless", // ieee19061nanocom
		"300": "wireless", // cpri (Common Public Radio Interface - mobile fronthaul)

		// Optical transport (SONET, DWDM, OTN, metro rings)
		"39":  "optical", // sonet
		"50":  "optical", // sonetPath
		"51":  "optical", // sonetVT
		"151": "optical", // srp (Spatial Reuse Protocol - SONET rings)
		"171": "optical", // pos
		"185": "optical", // sonetOverheadChannel
		"186": "optical", // digitalWrapperOverheadChannel
		"195": "optical", // opticalChannel
		"196": "optical", // opticalTransport
		"219": "optical", // opticalChannelGroup
		"221": "optical", // gfp
		"225": "optical", // rpr (Resilient Packet Ring)
		"260": "optical", // otnOdu
		"261": "optical", // otnOtu
		"291": "optical", // otnOtsi
		"292": "optical", // otnOtuc
		"293": "optical", // otnOduc
		"294": "optical", // otnOtsig

		// DSL
		"94":  "dsl", // adsl
		"95":  "dsl", // radsl
		"96":  "dsl", // sdsl
		"97":  "dsl", // vdsl
		"143": "dsl", // msdsl
		"154": "dsl", // idsl
		"168": "dsl", // hdsl2
		"169": "dsl", // shdsl
		"192": "dsl", // reachDSL
		"230": "dsl", // adsl2
		"238": "dsl", // adsl2plus
		"251": "dsl", // vdsl2
		"263": "dsl", // g9981
		"264": "dsl", // g9982
		"265": "dsl", // g9983
		"279": "dsl", // gfast
		"282": "dsl", // fastdsl

		// Cable / DOCSIS / MoCA
		"127": "cable", // docsCableMaclayer
		"128": "cable", // docsCableDownstream
		"129": "cable", // docsCableUpstream
		"180": "cable", // propDocsWirelessMaclayer
		"181": "cable", // propDocsWirelessDownstream
		"182": "cable", // propDocsWirelessUpstream
		"205": "cable", // docsCableUpstreamChannel
		"228": "cable", // cblVectaStar
		"229": "cable", // docsCableMCmtsDownstream
		"236": "cable", // mocaVersion1
		"256": "cable", // docsCableUpstreamRfPort
		"257": "cable", // cableDownstreamRfPort
		"277": "cable", // docsOfdmDownstream
		"278": "cable", // docsOfdmaUpstream
		"283": "cable", // docsCableScte55d1FwdOob
		"284": "cable", // docsCableScte55d1RetOob
		"285": "cable", // docsCableScte55d2DsOob
		"286": "cable", // docsCableScte55d2UsOob
		"287": "cable", // docsCableNdf
		"288": "cable", // docsCableNdr

		// PON / fiber access
		"207": "pon", // pon155
		"208": "pon", // pon622
		"250": "pon", // gpon
		"266": "pon", // aluEpon
		"267": "pon", // aluEponOnu
		"268": "pon", // aluEponPhysicalUni
		"269": "pon", // aluEponLogicalLink
		"270": "pon", // aluGponOnu
		"271": "pon", // aluGponPhysicalUni

		// MPLS (label switching, traffic engineering)
		"166": "mpls", // mpls
		"200": "mpls", // teLink (Traffic Engineering)
		"227": "mpls", // lmp (Link Management Protocol - GMPLS)

		// Datacenter fabrics (Fibre Channel, InfiniBand)
		"56":  "datacenter", // fibreChannel
		"199": "datacenter", // infiniband
		"224": "datacenter", // fcipLink

		// Serial / PPP / HDLC / modem
		"16":  "serial", // lapb
		"17":  "serial", // sdlc
		"22":  "serial", // propPointToPointSerial
		"23":  "serial", // ppp
		"28":  "serial", // slip
		"33":  "serial", // rs232
		"45":  "serial", // v35
		"46":  "serial", // hssi
		"48":  "serial", // modem
		"64":  "serial", // v11
		"65":  "serial", // v36
		"77":  "serial", // lapd
		"84":  "serial", // async
		"88":  "serial", // arap
		"118": "serial", // hdlc
		"119": "serial", // lapf
		"120": "serial", // v37
		"123": "serial", // transpHdlc
		"125": "serial", // fast
		"298": "serial", // ax25

		// TDM / voice / telephony
		"18":  "tdm_voice", // ds1
		"19":  "tdm_voice", // e1
		"20":  "tdm_voice", // basicISDN
		"21":  "tdm_voice", // primaryISDN
		"30":  "tdm_voice", // ds3
		"63":  "tdm_voice", // isdn
		"66":  "tdm_voice", // g703at64k
		"67":  "tdm_voice", // g703at2mb
		"75":  "tdm_voice", // isdns
		"76":  "tdm_voice", // isdnu
		"81":  "tdm_voice", // ds0
		"100": "tdm_voice", // voiceEM
		"101": "tdm_voice", // voiceFXO
		"102": "tdm_voice", // voiceFXS
		"103": "tdm_voice", // voiceEncap
		"104": "tdm_voice", // voiceOverIp
		"133": "tdm_voice", // ces
		"156": "tdm_voice", // ss7SigLink
		"164": "tdm_voice", // h323Gatekeeper
		"165": "tdm_voice", // h323Proxy
		"167": "tdm_voice", // mfSigLink
		"170": "tdm_voice", // ds1FDL
		"175": "tdm_voice", // nfas
		"176": "tdm_voice", // tr008
		"177": "tdm_voice", // gr303RDT
		"178": "tdm_voice", // gr303IDT
		"179": "tdm_voice", // isup
		"198": "tdm_voice", // voiceOverCable
		"201": "tdm_voice", // q2931
		"203": "tdm_voice", // sipTg
		"204": "tdm_voice", // sipSig
		"211": "tdm_voice", // voiceEMFGD
		"212": "tdm_voice", // voiceFGDEANA
		"213": "tdm_voice", // voiceDID
		"235": "tdm_voice", // voiceFGDOS
		"245": "tdm_voice", // voiceEBS

		// ATM
		"37":  "atm", // atm
		"49":  "atm", // aal5
		"80":  "atm", // atmLogical
		"105": "atm", // atmDxi
		"106": "atm", // atmFuni
		"107": "atm", // atmIma
		"114": "atm", // ipOverAtm
		"134": "atm", // atmSubInterface
		"149": "atm", // atmVirtual
		"152": "atm", // voiceOverAtm
		"159": "atm", // rfc1483
		"187": "atm", // aal2
		"189": "atm", // atmRadio
		"194": "atm", // atmVciEndPt
		"197": "atm", // propAtm
		"234": "atm", // atmbond

		// Frame Relay
		"32":  "frame_relay", // frameRelay
		"44":  "frame_relay", // frameRelayService
		"58":  "frame_relay", // frameRelayInterconnect
		"92":  "frame_relay", // frameRelayMPI
		"153": "frame_relay", // voiceOverFrameRelay
		"158": "frame_relay", // frForward
		"193": "frame_relay", // frDlciEndPt

		// WAN (legacy X.25, etc.)
		"4":   "wan", // ddnX25
		"5":   "wan", // rfc877x25
		"27":  "wan", // nsip
		"38":  "wan", // miox25
		"40":  "wan", // x25ple
		"68":  "wan", // qllc
		"121": "wan", // x25mlp
		"122": "wan", // x25huntGroup

		// Legacy/obsolete (includes mainframe, protocol translation)
		"2":   "legacy", // regular1822
		"3":   "legacy", // hdh1822
		"8":   "legacy", // iso88024TokenBus
		"9":   "legacy", // iso88025TokenRing
		"10":  "legacy", // iso88026Man
		"11":  "legacy", // starLan
		"12":  "legacy", // proteon10Mbit
		"13":  "legacy", // proteon80Mbit
		"14":  "legacy", // hyperchannel
		"15":  "legacy", // fddi
		"26":  "legacy", // ethernet3Mbit
		"29":  "legacy", // ultra
		"35":  "legacy", // arcnet
		"36":  "legacy", // arcnetPlus
		"41":  "legacy", // iso88022llc
		"42":  "legacy", // localTalk
		"43":  "legacy", // smdsDxi
		"52":  "legacy", // smdsIcip
		"70":  "legacy", // channel (mainframe)
		"72":  "legacy", // ibm370parChan (mainframe)
		"73":  "legacy", // escon (mainframe)
		"74":  "legacy", // dlsw (protocol translation)
		"79":  "legacy", // rsrb (protocol translation)
		"98":  "legacy", // iso88025CRFPInt
		"99":  "legacy", // myrinet
		"111": "legacy", // stackToStack (protocol translation)
		"141": "legacy", // dcn (protocol translation)
		"206": "legacy", // econet
		"249": "legacy", // aluELP

		// Broadcast / media (satellite, DVB, MPEG)
		"139": "broadcast_media", // mediaMailOverIp
		"140": "broadcast_media", // dtm
		"146": "broadcast_media", // dvbRccMacLayer
		"147": "broadcast_media", // dvbRccDownstream
		"148": "broadcast_media", // dvbRccUpstream
		"172": "broadcast_media", // dvbAsiIn
		"173": "broadcast_media", // dvbAsiOut
		"214": "broadcast_media", // mpegTransport
		"239": "broadcast_media", // dvbRcsMacLayer
		"240": "broadcast_media", // dvbTdm
		"241": "broadcast_media", // dvbRcsTdma

		// Home networking (powerline, G.hn, ZigBee)
		"138": "home_network", // digitalPowerline
		"174": "home_network", // plc
		"220": "home_network", // homepna
		"259": "home_network", // ieee802154
		"290": "home_network", // ghn

		// Other/miscellaneous
		"1":   "other", // other
		"34":  "other", // para
		"47":  "other", // hippi
		"54":  "other", // propMultiplexor
		"57":  "other", // hippiInterface
		"59":  "other", // aflane8023
		"60":  "other", // aflane8025
		"61":  "other", // cctEmul
		"83":  "other", // bsc
		"85":  "other", // cnr
		"86":  "other", // iso88025Dtr
		"87":  "other", // eplrs
		"89":  "other", // propCnls
		"90":  "other", // hostPad
		"91":  "other", // termPad
		"93":  "other", // x213
		"113": "other", // mpc
		"115": "other", // iso88025Fiber
		"116": "other", // tdlc
		"124": "other", // interleave
		"130": "other", // a12MppSwitch
		"132": "other", // coffee
		"144": "other", // ieee1394
		"145": "other", // if-gsn
		"160": "other", // usb
		"162": "other", // bgppolicyaccounting
		"184": "other", // propBWAp2Mp
		"190": "other", // imt
		"191": "other", // mvl
		"217": "other", // pdnEtherLoop1
		"218": "other", // pdnEtherLoop2
		"223": "other", // actelisMetaLOOP
		"226": "other", // qam
		"231": "other", // macSecControlledIF
		"232": "other", // macSecUncontrolledIF
		"242": "other", // x86Laps
		"247": "other", // ilan
		"248": "other", // pip
		"255": "other", // bits
		"280": "other", // sdci
		"289": "other", // ptm
		"301": "other", // omni
		"302": "other", // roe
	},
}
