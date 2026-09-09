// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"encoding/hex"
	"net"
	"strings"
	"sync"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
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
	users   []USMUser
}

// newDynamicEngineIDRegistry creates a registry backed by the shared v3
// security table. The known map is pre-seeded with usm_users that have an
// explicit engine_id so configured pairs do not produce dynamic warnings.
func newDynamicEngineIDRegistry(table *gosnmp.SnmpV3SecurityParametersTable, max int, known map[dynamicEngineIDKey]struct{}, users []USMUser) *dynamicEngineIDRegistry {
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
		if err := r.table.Add(u.Username, newUSMSecurityParameters(u, rawEngineID)); err != nil {
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

func (r *Receiver) decodeTrapWithSharedTable(data []byte, peerIP net.IP, trustedRelay bool) (*packetContext, error) {
	opts := decodeOptions{trustedRelay: trustedRelay}
	if r.dynamicEngineIDReg == nil {
		return decodeTrapWithOptions(data, peerIP, r.v3SecTable, opts)
	}
	r.dynamicEngineIDReg.tableMu.RLock()
	defer r.dynamicEngineIDReg.tableMu.RUnlock()
	return decodeTrapWithOptions(data, peerIP, r.v3SecTable, opts)
}

func (r *Receiver) tryDynamicRetry(data []byte, peerIP net.IP, peer *net.UDPAddr, rawCtx *rawV3Context, trustedRelay bool) (*packetContext, bool, bool) {
	if r.dynamicEngineIDReg == nil || rawCtx.username == "" {
		return nil, false, false
	}
	allowed, checked := r.allowRateLimitedPacket(peer)
	if !allowed {
		return nil, checked, true
	}
	tempTable := r.buildDynamicTempTable(rawCtx.engineID, rawCtx.username)
	if tempTable == nil {
		return nil, checked, false
	}
	retryCtx, err := decodeTrapWithOptions(data, peerIP, tempTable, decodeOptions{trustedRelay: trustedRelay})
	if err != nil {
		return nil, checked, false
	}
	if retryCtx.PDU.PduType == model.PduTypeInform {
		return nil, checked, false
	}
	if !r.registerDynamicEngineID(rawCtx.engineID, rawCtx.username) {
		return nil, checked, true
	}
	return retryCtx, checked, false
}

func (r *Receiver) ensureDynamicEngineIDRegistered(pktCtx *packetContext) bool {
	if !r.policy.dynamicEngineID || r.dynamicEngineIDReg == nil || pktCtx == nil || pktCtx.Packet == nil || pktCtx.PDU == nil {
		return true
	}
	if pktCtx.PDU.Version != model.SnmpVersionV3 || pktCtx.PDU.PduType == model.PduTypeInform {
		return true
	}
	usp, ok := pktCtx.Packet.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok || usp.UserName == "" || usp.AuthoritativeEngineID == "" {
		r.reportError(ErrorUnknownEngine)
		return false
	}
	return r.registerDynamicEngineID(hex.EncodeToString([]byte(usp.AuthoritativeEngineID)), usp.UserName)
}

func (r *Receiver) registerDynamicEngineID(engineIDHex, username string) bool {
	engineIDHex, accepted, isNew := r.dynamicEngineIDReg.accept(engineIDHex, username)
	if !accepted {
		r.reportError(ErrorUnknownEngine)
		return false
	}
	if isNew {
		r.reportEvent(Event{Type: EventDynamicEngineIDRegistered, EngineID: engineIDHex, Username: username})
	}
	return true
}

func (r *Receiver) allowRateLimitedPacket(peer *net.UDPAddr) (bool, bool) {
	if r.rateLimiter == nil || peer == nil {
		return true, false
	}
	srcAddr, ok := udpPeerAddr(peer)
	if !ok {
		return true, false
	}
	allowed, mode := r.rateLimiter.allow(srcAddr)
	if allowed {
		return true, true
	}
	r.reportError(ErrorRateLimited)
	return mode != rateLimitModeDrop, true
}

func (r *Receiver) buildDynamicTempTable(engineIDHex, username string) *gosnmp.SnmpV3SecurityParametersTable {
	engineID, err := parseEngineIDHex(engineIDHex)
	if err != nil {
		return nil
	}
	hasUser := false
	tbl := gosnmp.NewSnmpV3SecurityParametersTable(trapDecodeLogger)
	for _, u := range r.policy.users {
		if u.Username != username {
			continue
		}
		hasUser = true
		if err := tbl.Add(u.Username, newUSMSecurityParameters(u, engineID)); err != nil {
			return nil
		}
	}
	if !hasUser {
		return nil
	}
	return tbl
}

func (r *Receiver) sendDiscoveryReport(rawCtx *rawV3Context, conn *net.UDPConn, peer *net.UDPAddr) {
	var localEID []byte
	if r.localEngineID != nil {
		localEID = r.localEngineID.bytes()
	}
	if err := sendDiscoveryReport(conn, peer, r.engineBoots, localEID, rawCtx.msgID); err != nil {
		r.reportEvent(Event{Type: EventDiscoveryReportFailed, Err: err})
	}
}
