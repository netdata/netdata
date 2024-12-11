// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

// Docs: https://nsd.docs.nlnetlabs.nl/en/latest/manpages/nsd-control.html?highlight=elapsed#statistics-counters
// Source: https://github.com/NLnetLabs/nsd/blob/b4a5ccd2235a1f8f71f7c640390e409bf123c963/remote.c#L2735

// https://github.com/NLnetLabs/nsd/blob/b4a5ccd2235a1f8f71f7c640390e409bf123c963/remote.c#L2737
var answerRcodes = []string{
	"NOERROR",
	"FORMERR",
	"SERVFAIL",
	"NXDOMAIN",
	"NOTIMP",
	"REFUSED",
	"YXDOMAIN",
	"YXRRSET",
	"NXRRSET",
	"NOTAUTH",
	"NOTZONE",
	"RCODE11",
	"RCODE12",
	"RCODE13",
	"RCODE14",
	"RCODE15",
	"BADVERS",
}

// https://github.com/NLnetLabs/nsd/blob/b4a5ccd2235a1f8f71f7c640390e409bf123c963/remote.c#L2706
var queryOpcodes = []string{
	"QUERY",
	"IQUERY",
	"STATUS",
	"NOTIFY",
	"UPDATE",
	"OTHER",
}

// https://github.com/NLnetLabs/nsd/blob/b4a5ccd2235a1f8f71f7c640390e409bf123c963/dns.c#L27
var queryClasses = []string{
	"IN",
	"CS",
	"CH",
	"HS",
}

// https://github.com/NLnetLabs/nsd/blob/b4a5ccd2235a1f8f71f7c640390e409bf123c963/dns.c#L35
var queryTypes = []string{
	"A",
	"NS",
	"MD",
	"MF",
	"CNAME",
	"SOA",
	"MB",
	"MG",
	"MR",
	"NULL",
	"WKS",
	"PTR",
	"HINFO",
	"MINFO",
	"MX",
	"TXT",
	"RP",
	"AFSDB",
	"X25",
	"ISDN",
	"RT",
	"NSAP",
	"SIG",
	"KEY",
	"PX",
	"AAAA",
	"LOC",
	"NXT",
	"SRV",
	"NAPTR",
	"KX",
	"CERT",
	"DNAME",
	"OPT",
	"APL",
	"DS",
	"SSHFP",
	"IPSECKEY",
	"RRSIG",
	"NSEC",
	"DNSKEY",
	"DHCID",
	"NSEC3",
	"NSEC3PARAM",
	"TLSA",
	"SMIMEA",
	"CDS",
	"CDNSKEY",
	"OPENPGPKEY",
	"CSYNC",
	"ZONEMD",
	"SVCB",
	"HTTPS",
	"SPF",
	"NID",
	"L32",
	"L64",
	"LP",
	"EUI48",
	"EUI64",
	"URI",
	"CAA",
	"AVC",
	"DLV",
	"TYPE252",
	"TYPE255",
}

var queryTypeNumberMap = map[string]string{
	"TYPE251": "IXFR",
	"TYPE252": "AXFR",
	"TYPE253": "MAILB",
	"TYPE254": "MAILA",
	"TYPE255": "ANY",
}
