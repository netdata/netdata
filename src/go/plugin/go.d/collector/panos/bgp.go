// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const legacyBGPPeerCommand = "<show><routing><protocol><bgp><peer></peer></bgp></protocol></routing></show>"

const (
	noBGPReprobeInterval = 5 * time.Minute
	staleChartMaxMisses  = 3
	maxBGPPeerEntryDepth = 8
)

var advancedBGPPeerCommands = []string{
	"<show><advanced-routing><bgp><peer><details></details></peer></bgp></advanced-routing></show>",
	"<show><advanced-routing><bgp><peer><status></status></peer></bgp></advanced-routing></show>",
	"<show><advanced-routing><bgp><peer></peer></bgp></advanced-routing></show>",
}

type routingEngine string

const (
	routingEngineUnknown  routingEngine = ""
	routingEngineLegacy   routingEngine = "legacy"
	routingEngineAdvanced routingEngine = "advanced"
	routingEngineNone     routingEngine = "none"
)

type bgpPeer struct {
	VR             string
	PeerAddress    string
	LocalAddress   string
	RemoteAS       string
	PeerGroup      string
	State          string
	Uptime         int64
	MessagesIn     int64
	MessagesOut    int64
	UpdatesIn      int64
	UpdatesOut     int64
	Flaps          int64
	Established    int64
	PrefixCounters []bgpPrefixCounter
}

type bgpPrefixCounter struct {
	AFI                string
	SAFI               string
	IncomingTotal      int64
	IncomingAccepted   int64
	IncomingRejected   int64
	OutgoingAdvertised int64
}

type panosResponseMessage struct {
	Text  string   `xml:",chardata"`
	Lines []string `xml:"line"`
}

type panosBGPPeerEntry struct {
	PeerAttr          string `xml:"peer,attr"`
	NameAttr          string `xml:"name,attr"`
	PeerNameAttr      string `xml:"peer-name,attr"`
	PeerAddressAttr   string `xml:"peer-address,attr"`
	VRAttr            string `xml:"vr,attr"`
	VirtualRouterAttr string `xml:"virtual-router,attr"`
	LogicalRouterAttr string `xml:"logical-router,attr"`
	PeerGroupAttr     string `xml:"peer-group,attr"`
	RemoteASAttr      string `xml:"remote-as,attr"`

	Peer             string `xml:"peer"`
	PeerName         string `xml:"peer-name"`
	PeerAddress      string `xml:"peer-address"`
	VR               string `xml:"vr"`
	VirtualRouter    string `xml:"virtual-router"`
	LogicalRouter    string `xml:"logical-router"`
	PeerGroup        string `xml:"peer-group"`
	LocalAddress     string `xml:"local-address"`
	RemoteAS         string `xml:"remote-as"`
	PeerAS           string `xml:"peer-as"`
	PeerASNumber     string `xml:"peer-as-number"`
	Status           string `xml:"status"`
	State            string `xml:"state"`
	BGPState         string `xml:"bgp-state"`
	PeerState        string `xml:"peer-state"`
	SessionState     string `xml:"sess-state"`
	StatusDuration   string `xml:"status-duration"`
	Uptime           string `xml:"uptime"`
	UptimeSeconds    string `xml:"uptime-seconds"`
	MsgTotalIn       string `xml:"msg-total-in"`
	MsgTotalOut      string `xml:"msg-total-out"`
	MsgUpdateIn      string `xml:"msg-update-in"`
	MsgUpdateOut     string `xml:"msg-update-out"`
	StatusFlapCounts string `xml:"status-flap-counts"`
	FlapCount        string `xml:"flap-count"`
	EstablishedCount string `xml:"established-counts"`

	PrefixCounter struct {
		Entries []panosBGPPrefixEntry `xml:"entry"`
	} `xml:"prefix-counter"`
	Entries []panosBGPPeerEntry `xml:"entry"`
}

type panosBGPPrefixEntry struct {
	AFISAFIAttr string `xml:"afi-safi,attr"`
	NameAttr    string `xml:"name,attr"`
	AFIAttr     string `xml:"afi,attr"`
	SAFIAttr    string `xml:"safi,attr"`

	AFI                string `xml:"afi"`
	SAFI               string `xml:"safi"`
	IncomingTotal      string `xml:"incoming-total"`
	IncomingAccepted   string `xml:"incoming-accepted"`
	IncomingRejected   string `xml:"incoming-rejected"`
	OutgoingAdvertised string `xml:"outgoing-advertised"`
}

func (c *Collector) collectBGPPeers() ([]bgpPeer, error) {
	if c.apiClient == nil {
		return nil, errors.New("PAN-OS API client not initialized")
	}
	defer c.logSystemInfoOnce()

	if c.routingEngine == routingEngineNone && c.now().Sub(c.noBGPProbedAt) < noBGPReprobeInterval {
		c.Debugf("PAN-OS BGP peers not found on previous probe; skipping routing-engine probe until %s", c.noBGPProbedAt.Add(noBGPReprobeInterval).Format(time.RFC3339))
		return nil, nil
	}

	if c.bgpCommand != "" {
		peers, err := c.queryBGPPeers(c.bgpCommand)
		if err == nil {
			if len(peers) == 0 {
				c.warningOnce("bgp_empty_query", fmt.Sprintf("PAN-OS BGP query returned no peers (routing_engine=%s, command=%s)", c.routingEngine, bgpCommandName(c.bgpCommand)))
			} else {
				c.clearRuntimeLogState("bgp_empty_query", "bgp_query_failed", "bgp_no_peers")
			}
			return peers, nil
		}
		c.warningOnce("bgp_query_failed", fmt.Sprintf("PAN-OS BGP query failed (routing_engine=%s, command=%s), probing routing engine again: %v", c.routingEngine, bgpCommandName(c.bgpCommand), err))
		c.routingEngine = routingEngineUnknown
		c.bgpCommand = ""
	}

	return c.probeAndCollectBGPPeers()
}

func (c *Collector) probeAndCollectBGPPeers() ([]bgpPeer, error) {
	var errs []error
	var emptySuccess bool

	peers, err := c.queryBGPPeers(legacyBGPPeerCommand)
	if err == nil && len(peers) > 0 {
		c.routingEngine = routingEngineLegacy
		c.bgpCommand = legacyBGPPeerCommand
		c.clearRuntimeLogState("bgp_empty_query", "bgp_query_failed", "bgp_no_peers")
		c.Infof("detected PAN-OS legacy routing engine for BGP collection (command=%s)", bgpCommandName(c.bgpCommand))
		return peers, nil
	}
	if err == nil {
		emptySuccess = true
	} else {
		errs = append(errs, fmt.Errorf("%s: %w", bgpCommandName(legacyBGPPeerCommand), err))
	}

	for _, cmd := range c.advancedBGPCommands {
		peers, err = c.queryBGPPeers(cmd)
		if err == nil && len(peers) > 0 {
			c.routingEngine = routingEngineAdvanced
			c.bgpCommand = cmd
			c.clearRuntimeLogState("bgp_empty_query", "bgp_query_failed", "bgp_no_peers")
			c.Infof("detected PAN-OS Advanced Routing Engine for BGP collection (command=%s)", bgpCommandName(c.bgpCommand))
			return peers, nil
		}
		if err == nil {
			emptySuccess = true
		} else {
			errs = append(errs, fmt.Errorf("%s: %w", bgpCommandName(cmd), err))
		}
	}

	if emptySuccess {
		c.routingEngine = routingEngineNone
		c.noBGPProbedAt = c.now()
		c.infoOnce("bgp_no_peers", "connected to PAN-OS XML API, but no BGP peers were found by legacy or Advanced Routing Engine probes")
		return nil, nil
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return nil, nil
}

func (c *Collector) queryBGPPeers(cmd string) ([]bgpPeer, error) {
	body, err := c.apiClient.op(cmd)
	if err != nil {
		return nil, fmt.Errorf("%s API call: %w", bgpCommandName(cmd), err)
	}
	peers, err := parseBGPPeers(body)
	if err != nil {
		return nil, fmt.Errorf("%s response: %w", bgpCommandName(cmd), err)
	}
	return peers, nil
}

func parseBGPPeers(body []byte) ([]bgpPeer, error) {
	innerXML, err := decodePANOSResultInner(body, "PAN-OS BGP response")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(innerXML) == "" {
		return nil, nil
	}

	entries, err := decodeBGPPeerEntries(innerXML)
	if err != nil {
		return nil, err
	}

	peers := make([]bgpPeer, 0, len(entries))
	seen := make(map[string]bool)
	for _, entry := range entries {
		peer, ok, err := entry.toBGPPeer()
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		key := peer.VR + "\x00" + peer.PeerAddress
		if seen[key] {
			continue
		}
		seen[key] = true
		peers = append(peers, peer)
	}

	return peers, nil
}

func decodeBGPPeerEntries(innerXML string) ([]panosBGPPeerEntry, error) {
	decoder := xml.NewDecoder(strings.NewReader(innerXML))

	var entries []panosBGPPeerEntry
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parse PAN-OS BGP result: %w", err)
		}

		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "entry" {
			continue
		}

		var entry panosBGPPeerEntry
		if err := decoder.DecodeElement(&entry, &start); err != nil {
			return nil, fmt.Errorf("parse PAN-OS BGP peer entry: %w", err)
		}
		entries = appendFlattenedBGPPeerEntries(entries, entry, inheritedBGPPeerFields{}, 0)
	}

	return entries, nil
}

type inheritedBGPPeerFields struct {
	vr          string
	peerGroup   string
	localAddr   string
	remoteAS    string
	peerAddress string
}

func appendFlattenedBGPPeerEntries(entries []panosBGPPeerEntry, entry panosBGPPeerEntry, parent inheritedBGPPeerFields, depth int) []panosBGPPeerEntry {
	if depth > maxBGPPeerEntryDepth {
		return entries
	}
	entry.inherit(parent)
	entries = append(entries, entry)

	vr := entry.vr()
	if vr == "" && len(entry.Entries) > 0 {
		vr = entry.NameAttr
	}
	next := inheritedBGPPeerFields{
		vr:          vr,
		peerGroup:   entry.peerGroup(),
		localAddr:   entry.localAddress(),
		remoteAS:    entry.remoteAS(),
		peerAddress: entry.peerAddress(),
	}
	for _, child := range entry.Entries {
		entries = appendFlattenedBGPPeerEntries(entries, child, next, depth+1)
	}
	return entries
}

func (e *panosBGPPeerEntry) inherit(parent inheritedBGPPeerFields) {
	if e.vr() == "" {
		e.VR = parent.vr
	}
	if e.peerGroup() == "" {
		e.PeerGroup = parent.peerGroup
	}
	if e.localAddress() == "" {
		e.LocalAddress = parent.localAddr
	}
	if e.remoteAS() == "" {
		e.RemoteAS = parent.remoteAS
	}
	if e.peerAddress() == "" {
		e.PeerAddress = parent.peerAddress
	}
}

func (e panosBGPPeerEntry) toBGPPeer() (bgpPeer, bool, error) {
	peerAddr := normalizeAddress(e.peerAddress())
	if peerAddr == "" && e.hasPeerData() {
		peerAddr = normalizeAddress(e.peerName())
	}
	if peerAddr == "" {
		return bgpPeer{}, false, nil
	}

	uptime, err := parseRequiredPANOSDurationField("BGP peer "+peerAddr+" uptime", firstNonEmpty(e.StatusDuration, e.UptimeSeconds, e.Uptime))
	if err != nil {
		return bgpPeer{}, false, err
	}
	messagesIn, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" msg-total-in", e.MsgTotalIn)
	if err != nil {
		return bgpPeer{}, false, err
	}
	messagesOut, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" msg-total-out", e.MsgTotalOut)
	if err != nil {
		return bgpPeer{}, false, err
	}
	updatesIn, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" msg-update-in", e.MsgUpdateIn)
	if err != nil {
		return bgpPeer{}, false, err
	}
	updatesOut, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" msg-update-out", e.MsgUpdateOut)
	if err != nil {
		return bgpPeer{}, false, err
	}
	flaps, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" flap-count", firstNonEmpty(e.StatusFlapCounts, e.FlapCount))
	if err != nil {
		return bgpPeer{}, false, err
	}
	established, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" established-counts", e.EstablishedCount)
	if err != nil {
		return bgpPeer{}, false, err
	}
	prefixCounters, err := e.prefixCounters(peerAddr)
	if err != nil {
		return bgpPeer{}, false, err
	}

	peer := bgpPeer{
		VR:             firstNonEmpty(e.vr(), "default"),
		PeerAddress:    peerAddr,
		LocalAddress:   normalizeAddress(e.localAddress()),
		RemoteAS:       e.remoteAS(),
		PeerGroup:      e.peerGroup(),
		State:          normalizeBGPState(firstNonEmpty(e.Status, e.State, e.BGPState, e.PeerState, e.SessionState)),
		Uptime:         uptime,
		MessagesIn:     messagesIn,
		MessagesOut:    messagesOut,
		UpdatesIn:      updatesIn,
		UpdatesOut:     updatesOut,
		Flaps:          flaps,
		Established:    established,
		PrefixCounters: prefixCounters,
	}

	return peer, true, nil
}

func (e panosBGPPeerEntry) hasPeerData() bool {
	return firstNonEmpty(
		e.Status,
		e.State,
		e.BGPState,
		e.PeerState,
		e.SessionState,
		e.StatusDuration,
		e.Uptime,
		e.UptimeSeconds,
		e.MsgTotalIn,
		e.MsgTotalOut,
		e.MsgUpdateIn,
		e.MsgUpdateOut,
		e.StatusFlapCounts,
		e.FlapCount,
		e.EstablishedCount,
	) != "" || len(e.PrefixCounter.Entries) > 0
}

func (e panosBGPPeerEntry) vr() string {
	return firstNonEmpty(e.VR, e.VRAttr, e.VirtualRouter, e.VirtualRouterAttr, e.LogicalRouter, e.LogicalRouterAttr)
}

func (e panosBGPPeerEntry) peerName() string {
	return firstNonEmpty(e.PeerAddress, e.PeerAddressAttr, e.Peer, e.PeerAttr, e.PeerName, e.PeerNameAttr, e.NameAttr)
}

func (e panosBGPPeerEntry) peerAddress() string {
	return firstNonEmpty(e.PeerAddress, e.PeerAddressAttr, e.Peer, e.PeerAttr)
}

func (e panosBGPPeerEntry) localAddress() string {
	return e.LocalAddress
}

func (e panosBGPPeerEntry) remoteAS() string {
	return firstNonEmpty(e.RemoteAS, e.RemoteASAttr, e.PeerAS, e.PeerASNumber)
}

func (e panosBGPPeerEntry) peerGroup() string {
	return firstNonEmpty(e.PeerGroup, e.PeerGroupAttr)
}

func (e panosBGPPeerEntry) prefixCounters(peerAddr string) ([]bgpPrefixCounter, error) {
	counters := make([]bgpPrefixCounter, 0, len(e.PrefixCounter.Entries))
	for _, entry := range e.PrefixCounter.Entries {
		counter, err := entry.toBGPPrefixCounter(peerAddr)
		if err != nil {
			return nil, err
		}
		if counter.AFI == "" {
			counter.AFI = "unknown"
		}
		if counter.SAFI == "" {
			counter.SAFI = "unknown"
		}
		counters = append(counters, counter)
	}
	return counters, nil
}

func (e panosBGPPrefixEntry) toBGPPrefixCounter(peerAddr string) (bgpPrefixCounter, error) {
	afi, safi := normalizeAFISAFI(firstNonEmpty(e.AFISAFIAttr, e.NameAttr))
	if afi == "" {
		afi = normalizeAFI(firstNonEmpty(e.AFI, e.AFIAttr))
	}
	if safi == "" {
		safi = normalizeSAFI(firstNonEmpty(e.SAFI, e.SAFIAttr))
	}
	family := "unknown"
	if afi != "" && safi != "" {
		family = afi + "-" + safi
	} else if afi != "" {
		family = afi
	} else if safi != "" {
		family = safi
	}

	incomingTotal, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" "+family+" incoming-total", e.IncomingTotal)
	if err != nil {
		return bgpPrefixCounter{}, err
	}
	incomingAccepted, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" "+family+" incoming-accepted", e.IncomingAccepted)
	if err != nil {
		return bgpPrefixCounter{}, err
	}
	incomingRejected, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" "+family+" incoming-rejected", e.IncomingRejected)
	if err != nil {
		return bgpPrefixCounter{}, err
	}
	outgoingAdvertised, err := parseRequiredPANOSIntField("BGP peer "+peerAddr+" "+family+" outgoing-advertised", e.OutgoingAdvertised)
	if err != nil {
		return bgpPrefixCounter{}, err
	}

	return bgpPrefixCounter{
		AFI:                afi,
		SAFI:               safi,
		IncomingTotal:      incomingTotal,
		IncomingAccepted:   incomingAccepted,
		IncomingRejected:   incomingRejected,
		OutgoingAdvertised: outgoingAdvertised,
	}, nil
}

func normalizeBGPState(state string) string {
	v := strings.ToLower(strings.TrimSpace(state))
	v = stateNameReplacer.Replace(v)

	switch {
	case strings.Contains(v, "established"):
		return "established"
	case strings.Contains(v, "openconfirm"):
		return "openconfirm"
	case strings.Contains(v, "opensent"):
		return "opensent"
	case strings.Contains(v, "active"):
		return "active"
	case strings.Contains(v, "connect"):
		return "connect"
	case strings.Contains(v, "idle"):
		return "idle"
	default:
		return ""
	}
}

func normalizeAFISAFI(v string) (string, string) {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimPrefix(v, "bgpafi")
	v = strings.TrimPrefix(v, "afi-")
	v = strings.TrimPrefix(v, "afi_")
	v = strings.ReplaceAll(v, "_", "-")

	parts := strings.Split(v, "-")
	if len(parts) < 2 {
		return normalizeAFI(v), ""
	}
	return normalizeAFI(parts[0]), normalizeSAFI(strings.Join(parts[1:], "-"))
}

func normalizeAFI(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimPrefix(v, "bgpafi")
	switch v {
	case "ipv4", "ip":
		return "ipv4"
	case "ipv6":
		return "ipv6"
	default:
		return strings.ReplaceAll(v, " ", "_")
	}
}

func normalizeSAFI(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, " ", "_")
	return strings.ReplaceAll(v, "-", "_")
}

func normalizeAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(addr); err == nil {
		return strings.Trim(host, "[]")
	}
	if _, err := netip.ParseAddr(addr); err == nil {
		return addr
	}

	if strings.Count(addr, ":") == 1 {
		host, port, ok := strings.Cut(addr, ":")
		if ok && isDigits(port) {
			return host
		}
	}

	return strings.Trim(addr, "[]")
}

var digitsOnly = regexp.MustCompile(`^\d+$`)

func isDigits(v string) bool {
	return digitsOnly.MatchString(v)
}

func parsePANOSIntField(field, v string) (int64, error) {
	raw := strings.TrimSpace(v)
	v = strings.ReplaceAll(raw, ",", "")
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", field, raw)
	}
	return n, nil
}

func parseRequiredPANOSIntField(field, v string) (int64, error) {
	if strings.TrimSpace(v) == "" {
		return 0, fmt.Errorf("%s: missing integer", field)
	}
	return parsePANOSIntField(field, v)
}

func parsePANOSDurationField(field, v string) (int64, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return 0, nil
	}
	if isDigits(v) {
		return parsePANOSIntField(field, v)
	}

	var total int64
	matched := false
	for _, match := range durationPartRe.FindAllStringSubmatch(v, -1) {
		matched = true
		n, err := parsePANOSIntField(field, match[1])
		if err != nil {
			return 0, err
		}
		switch match[2] {
		case "day", "days", "d":
			total += n * int64((24 * time.Hour).Seconds())
		case "hour", "hours", "h":
			total += n * int64(time.Hour.Seconds())
		case "minute", "minutes", "min", "mins", "m":
			total += n * int64(time.Minute.Seconds())
		case "second", "seconds", "sec", "secs", "s":
			total += n
		}
	}

	timeMatch := durationClockRe.FindStringSubmatch(v)
	if len(timeMatch) == 4 {
		matched = true
		hour, err := parsePANOSIntField(field, timeMatch[1])
		if err != nil {
			return 0, err
		}
		minute, err := parsePANOSIntField(field, timeMatch[2])
		if err != nil {
			return 0, err
		}
		second, err := parsePANOSIntField(field, timeMatch[3])
		if err != nil {
			return 0, err
		}
		if minute >= 60 || second >= 60 {
			return 0, fmt.Errorf("%s: invalid duration %q", field, v)
		}
		total += hour*int64(time.Hour.Seconds()) +
			minute*int64(time.Minute.Seconds()) +
			second
	}

	remainder := durationPartRe.ReplaceAllString(v, "")
	remainder = durationClockRe.ReplaceAllString(remainder, "")
	remainder = strings.TrimSpace(strings.Trim(remainder, ","))
	if !matched || remainder != "" {
		return 0, fmt.Errorf("%s: invalid duration %q", field, v)
	}

	return total, nil
}

func parseRequiredPANOSDurationField(field, v string) (int64, error) {
	if strings.TrimSpace(v) == "" {
		return 0, fmt.Errorf("%s: missing duration", field)
	}
	return parsePANOSDurationField(field, v)
}

func (m panosResponseMessage) String() string {
	var lines []string
	if text := strings.TrimSpace(m.Text); text != "" {
		lines = append(lines, text)
	}
	for _, line := range m.Lines {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "; ")
}

type panosResponseError struct {
	code    string
	message string
}

func (e panosResponseError) Error() string {
	code := strings.TrimSpace(e.code)
	codeName := panosResponseCodeName(code)
	if e.message == "" {
		if code == "" {
			return "PAN-OS XML API response error"
		}
		if codeName != "" {
			return fmt.Sprintf("PAN-OS XML API response error code %s (%s)", code, codeName)
		}
		return fmt.Sprintf("PAN-OS XML API response error code %s", code)
	}
	if code == "" {
		return fmt.Sprintf("PAN-OS XML API response error: %s", e.message)
	}
	if codeName != "" && !strings.Contains(strings.ToLower(e.message), strings.ToLower(codeName)) {
		return fmt.Sprintf("PAN-OS XML API response error code %s (%s): %s", code, codeName, e.message)
	}
	return fmt.Sprintf("PAN-OS XML API response error code %s: %s", code, e.message)
}

func panosResponseCodeName(code string) string {
	switch strings.TrimSpace(code) {
	case "1":
		return "Unknown command"
	case "2", "3", "4", "5", "11":
		return "Internal error"
	case "6":
		return "Bad XPath"
	case "7":
		return "Object not found"
	case "8":
		return "Object not unique"
	case "10":
		return "Reference count not zero"
	case "12":
		return "Invalid object"
	case "14":
		return "Operation not possible"
	case "15":
		return "Operation denied"
	case "16":
		return "Unauthorized"
	case "17":
		return "Invalid command"
	case "18":
		return "Malformed command"
	case "22":
		return "Session timed out"
	case "400":
		return "Bad request"
	case "403":
		return "Forbidden"
	default:
		return ""
	}
}

func bgpCommandName(cmd string) string {
	switch cmd {
	case legacyBGPPeerCommand:
		return "legacy routing BGP peer query"
	case advancedBGPPeerCommands[0]:
		return "advanced routing BGP peer details query"
	case advancedBGPPeerCommands[1]:
		return "advanced routing BGP peer status query"
	case advancedBGPPeerCommands[2]:
		return "advanced routing BGP peer query"
	default:
		return "PAN-OS BGP query"
	}
}

var (
	stateNameReplacer = strings.NewReplacer(" ", "", "-", "", "_", "")
	durationPartRe    = regexp.MustCompile(`\b(\d+)\s*(days?|d|hours?|h|minutes?|mins?|min|m|seconds?|secs?|sec|s)\b`)
	durationClockRe   = regexp.MustCompile(`\b(\d{1,2}):(\d{2}):(\d{2})\b`)
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
