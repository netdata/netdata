// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/hex"
	"net"
	"strings"
	"sync"

	"github.com/gosnmp/gosnmp"
)

// dynamicEngineIDKey is a (engineID raw bytes, username) pair.
type dynamicEngineIDKey struct {
	engineIDHex string
	username    string
}

// dynamicEngineIDRegistry holds in-memory per-job dynamic engine ID state.
type dynamicEngineIDRegistry struct {
	mu      sync.Mutex
	tableMu sync.RWMutex
	table   *gosnmp.SnmpV3SecurityParametersTable
	max     int
	dynamic int
	known   map[dynamicEngineIDKey]struct{}
	users   []USMUserConfig
}

// newDynamicEngineIDRegistry creates a registry backed by the shared v3
// security table. The known map is pre-seeded with usm_users that have an
// explicit engine_id so configured pairs do not produce dynamic warnings.
func newDynamicEngineIDRegistry(table *gosnmp.SnmpV3SecurityParametersTable, max int, known map[dynamicEngineIDKey]struct{}, users []USMUserConfig) *dynamicEngineIDRegistry {
	if known == nil {
		known = make(map[dynamicEngineIDKey]struct{})
	}
	return &dynamicEngineIDRegistry{
		table: table,
		max:   max,
		known: known,
		users: users,
	}
}

// size returns the number of known pairs currently tracked by the registry.
func (r *dynamicEngineIDRegistry) size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.known)
}

// accept attempts to dynamically register and persist an (engineIDHex,
// username) pair. It returns (engineIDHex, false) when the cap is full or the
// username is not configured. It returns (engineIDHex, true, isNew) on
// success; isNew is true the first time this pair is accepted for the job
// lifetime.
func (r *dynamicEngineIDRegistry) accept(engineIDHex string, username string) (string, bool, bool) {
	engineIDHex = strings.ToLower(strings.TrimSpace(engineIDHex))
	rawEngineID, err := parseEngineIDHex(engineIDHex)
	if err != nil || username == "" {
		return "", false, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := dynamicEngineIDKey{engineIDHex: engineIDHex, username: username}
	if _, ok := r.known[key]; ok {
		return engineIDHex, true, false
	}

	if r.dynamic+1 > r.max {
		return "", false, false
	}

	persisted := false
	r.tableMu.Lock()
	defer r.tableMu.Unlock()
	for _, u := range r.users {
		if u.Username != username {
			continue
		}
		authProto := snmpV3AuthProto(strings.ToLower(u.AuthProto))
		privProto := snmpV3PrivProto(strings.ToLower(u.PrivProto))
		sp := &gosnmp.UsmSecurityParameters{
			UserName:                 u.Username,
			AuthenticationProtocol:   authProto,
			AuthenticationPassphrase: u.AuthKey,
			PrivacyProtocol:          privProto,
			PrivacyPassphrase:        u.PrivKey,
			AuthoritativeEngineID:    string(rawEngineID),
		}
		if err := r.table.Add(u.Username, sp); err != nil {
			return "", false, false
		}
		persisted = true
	}

	if !persisted {
		return "", false, false
	}

	r.known[key] = struct{}{}
	r.dynamic++
	return engineIDHex, true, true
}

func (c *Collector) decodeTrapWithSharedTable(data []byte, peerIP net.IP, trustedRelay bool) (*TrapPacketContext, error) {
	opts := DecodeOptions{TrustedRelay: trustedRelay}
	if c.dynamicEngineIDReg == nil {
		return DecodeTrapWithOptions(data, peerIP, c.v3SecTable, opts)
	}
	c.dynamicEngineIDReg.tableMu.RLock()
	defer c.dynamicEngineIDReg.tableMu.RUnlock()
	return DecodeTrapWithOptions(data, peerIP, c.v3SecTable, opts)
}

func (c *Collector) tryDynamicRetry(data []byte, peerIP net.IP, peer *net.UDPAddr, rawCtx *rawV3Context, trustedRelay bool) (*TrapPacketContext, bool, bool) {
	if c.dynamicEngineIDReg == nil || rawCtx.username == "" {
		return nil, false, false
	}
	allowed, checked := c.allowDynamicRetry(peer)
	if !allowed {
		return nil, checked, true
	}
	tempTable := c.buildDynamicTempTable(rawCtx.engineID, rawCtx.username)
	if tempTable == nil {
		return nil, checked, false
	}
	retryCtx, err := DecodeTrapWithOptions(data, peerIP, tempTable, DecodeOptions{TrustedRelay: trustedRelay})
	if err != nil {
		return nil, checked, false
	}
	if retryCtx.PDU.PduType == PduTypeInform {
		return nil, checked, false
	}
	if !c.registerDynamicEngineID(rawCtx.engineID, rawCtx.username) {
		return nil, checked, true
	}
	return retryCtx, checked, false
}

func (c *Collector) ensureDynamicEngineIDRegistered(pktCtx *TrapPacketContext) bool {
	if !c.dynamicEngineID || c.dynamicEngineIDReg == nil || pktCtx == nil || pktCtx.Packet == nil || pktCtx.PDU == nil {
		return true
	}
	if pktCtx.PDU.Version != SnmpVersionV3 || pktCtx.PDU.PduType == PduTypeInform {
		return true
	}
	usp, ok := pktCtx.Packet.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok || usp.UserName == "" || usp.AuthoritativeEngineID == "" {
		c.incTrapError("unknown_engine_id")
		return false
	}
	return c.registerDynamicEngineID(hex.EncodeToString([]byte(usp.AuthoritativeEngineID)), usp.UserName)
}

func (c *Collector) registerDynamicEngineID(engineIDHex, username string) bool {
	engineIDHex, accepted, isNew := c.dynamicEngineIDReg.accept(engineIDHex, username)
	if !accepted {
		c.incTrapError("unknown_engine_id")
		return false
	}
	if isNew {
		c.warnf("Dynamic SNMPv3 engine ID registered: engineID=%s username=%s. This sender was not in the static configuration. Every first-time dynamic (engineID, username) pair is accepted and logged once per job lifetime.",
			engineIDHex, username)
		c.incTrapError("unknown_engine_id")
	}
	return true
}

func (c *Collector) allowDynamicRetry(peer *net.UDPAddr) (bool, bool) {
	if c.rateLimiter == nil || peer == nil {
		return true, false
	}
	srcAddr, ok := udpPeerAddr(peer)
	if !ok {
		return true, false
	}
	allowed, mode := c.rateLimiter.Allow(srcAddr)
	if allowed {
		return true, true
	}
	c.incTrapError("rate_limited")
	return mode != rateLimitModeDrop, true
}

func (c *Collector) buildDynamicTempTable(engineIDHex, username string) *gosnmp.SnmpV3SecurityParametersTable {
	engineID, err := parseEngineIDHex(engineIDHex)
	if err != nil {
		return nil
	}
	hasUser := false
	tbl := gosnmp.NewSnmpV3SecurityParametersTable(trapDecodeLogger)
	for _, u := range c.USMUsers {
		if u.Username != username {
			continue
		}
		hasUser = true
		sp := &gosnmp.UsmSecurityParameters{
			UserName:                 u.Username,
			AuthenticationProtocol:   snmpV3AuthProto(strings.ToLower(u.AuthProto)),
			AuthenticationPassphrase: u.AuthKey,
			PrivacyProtocol:          snmpV3PrivProto(strings.ToLower(u.PrivProto)),
			PrivacyPassphrase:        u.PrivKey,
			AuthoritativeEngineID:    string(engineID),
		}
		if err := tbl.Add(u.Username, sp); err != nil {
			return nil
		}
	}
	if !hasUser {
		return nil
	}
	return tbl
}

func (c *Collector) sendDiscoveryReport(rawCtx *rawV3Context, conn *net.UDPConn, peer *net.UDPAddr) {
	var localEID []byte
	if c.localEngineID != nil {
		localEID = c.localEngineID.Bytes()
	}
	if err := sendDiscoveryReport(conn, peer, c.engineBoots, localEID, rawCtx.msgID); err != nil {
		c.warnf("SNMP trap INFORM discovery Report failed: %v", err)
		c.incTrapError("inform_response_failed")
	}
}
