// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"errors"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

type EventType uint8

const (
	EventError EventType = iota + 1
	EventDecoded
	EventListenerReadFailed
	EventListenerBufferDegraded
	EventDynamicEngineIDRegistered
	EventInformResponseFailed
	EventDiscoveryReportFailed
)

type ErrorKind string

const (
	ErrorMalformedPDU  ErrorKind = "malformed_pdu"
	ErrorAuthFailure   ErrorKind = "auth_failures"
	ErrorUSMFailure    ErrorKind = "usm_failures"
	ErrorUnknownEngine ErrorKind = "unknown_engine_id"
	ErrorDecodeFailed  ErrorKind = "decode_failed"
	ErrorDroppedPolicy ErrorKind = "dropped_allowlist"
	ErrorRateLimited   ErrorKind = "rate_limited"
)

type Event struct {
	Type      EventType
	ErrorKind ErrorKind
	Endpoint  Endpoint
	Requested int
	EngineID  string
	Username  string
	Err       error
}

type Reporter func(Event)

type Datagram struct {
	Data   []byte
	PeerIP net.IP
	Conn   *net.UDPConn
	Peer   *net.UDPAddr
}

type DecodeFailure struct {
	Data           []byte
	PeerIP         net.IP
	Conn           *net.UDPConn
	Peer           *net.UDPAddr
	Kind           ErrorKind
	Err            error
	SniffedVersion model.SnmpVersion
	VersionKnown   bool
	auditAdmitted  bool
}

type Result struct {
	PDU           *model.TrapPDU
	DecodeFailure *DecodeFailure
}

type Receiver struct {
	policy Policy
	report Reporter

	listener           *listener
	allowlist          *allowlist
	rateLimiter        *rateLimiter
	v3SecTable         *gosnmp.SnmpV3SecurityParametersTable
	engineIDs          map[string]struct{}
	engineBoots        *engineBoots
	localEngineID      *localEngineID
	dynamicEngineIDReg *dynamicEngineIDRegistry
	rollbackState      func()
}

func New(policy Policy, report Reporter) *Receiver {
	return &Receiver{
		policy:      policy,
		report:      report,
		allowlist:   newAllowlist(policy.sourceAllowlist, policy.communities),
		rateLimiter: newRateLimiter(policy.rateLimit.Enabled, policy.rateLimit.PerSourcePPS, policy.rateLimit.Mode),
	}
}

func (r *Receiver) Bind() ([]Event, error) {
	if r.listener != nil {
		return nil, nil
	}
	l, events, err := newListener(r.policy.listen, r.report)
	if err != nil {
		return nil, err
	}
	r.listener = l
	return events, nil
}

type preparationError struct {
	err    error
	config bool
}

func (e *preparationError) Error() string { return e.err.Error() }
func (e *preparationError) Unwrap() error { return e.err }

func IsConfigPreparationError(err error) bool {
	var target *preparationError
	return errors.As(err, &target) && target.config
}

func configPreparationError(err error) error  { return &preparationError{err: err, config: true} }
func startupPreparationError(err error) error { return &preparationError{err: err} }

func (r *Receiver) PrepareV3(stateRoot, jobName string) error {
	if !r.policy.V3Enabled() {
		return nil
	}

	paths := newEngineStatePaths(stateRoot, jobName)
	engineBootsExisted, err := engineStatePathExistsChecked(paths.engineBoots)
	if err != nil {
		return startupPreparationError(err)
	}
	localEngineIDExisted, err := engineStatePathExistsChecked(paths.localEngineID)
	if err != nil {
		return startupPreparationError(err)
	}
	engineStateDirExisted, err := engineStatePathExistsChecked(paths.dir)
	if err != nil {
		return startupPreparationError(err)
	}
	rollback := func() {
		cleanupCreatedEngineState(paths, !engineBootsExisted, !localEngineIDExisted, !engineStateDirExisted)
	}

	table, err := buildSnmpV3SecurityTable(r.policy.users, r.policy.dynamicEngineID)
	if err != nil {
		rollback()
		return configPreparationError(err)
	}
	engineIDs, err := buildEngineIDWhitelist(r.policy.engineIDWhitelist)
	if err != nil {
		rollback()
		return configPreparationError(err)
	}
	localEngineID, err := newLocalEngineID(paths, r.policy.localEngineID)
	if err != nil {
		rollback()
		return startupPreparationError(err)
	}
	if err := registerUSMUsersWithLocalEngineID(table, r.policy.users, localEngineID.bytes()); err != nil {
		rollback()
		return configPreparationError(err)
	}
	engineBoots, err := newEngineBoots(paths)
	if err != nil {
		rollback()
		return startupPreparationError(err)
	}

	r.v3SecTable = table
	r.engineIDs = engineIDs
	r.localEngineID = localEngineID
	r.engineBoots = engineBoots
	r.rollbackState = rollback
	if r.policy.dynamicEngineID && table != nil {
		known := make(map[dynamicEngineIDKey]struct{})
		for _, user := range r.policy.users {
			if user.EngineID == "" {
				continue
			}
			known[dynamicEngineIDKey{
				engineIDHex: strings.ToLower(strings.TrimSpace(user.EngineID)),
				username:    user.Username,
			}] = struct{}{}
		}
		r.dynamicEngineIDReg = newDynamicEngineIDRegistry(table, r.policy.dynamicEngineIDMax, known, r.policy.users)
	}
	return nil
}

func (r *Receiver) CommitPreparedState() {
	r.rollbackState = nil
}

func (r *Receiver) RollbackPreparedState() {
	if r.rollbackState != nil {
		r.rollbackState()
		r.rollbackState = nil
	}
}

func (r *Receiver) Start(handler func(Datagram)) {
	if r.listener != nil {
		r.listener.start(handler)
	}
}

func (r *Receiver) Close() {
	if r.listener != nil {
		r.listener.close()
		r.listener = nil
	}
}

func (r *Receiver) Sweep(now time.Time) {
	if r.rateLimiter != nil {
		r.rateLimiter.maybeSweep(now)
	}
}

func (r *Receiver) Ready() bool { return r.listener != nil }

func (r *Receiver) Process(datagram Datagram) Result {
	data := datagram.Data
	decodePeerIP := datagram.PeerIP
	rateLimitChecked := false

	if source, ok := packetSourceAddr(datagram.PeerIP, datagram.Peer); ok {
		decodePeerIP = net.IP(source.AsSlice())
		if r.allowlist != nil && !r.allowlist.allowedSource(source) {
			r.reportError(ErrorDroppedPolicy)
			return Result{}
		}
	} else if r.allowlist != nil {
		r.reportError(ErrorDroppedPolicy)
		return Result{}
	}

	sniffedVersion, versionKnown := sniffSNMPVersion(data)
	if versionKnown && !r.versionAllowed(sniffedVersion) {
		r.reportError(ErrorDroppedPolicy)
		return Result{}
	}

	trustedRelay := r.trustedRelaySource(decodePeerIP)
	packetContext, err := r.decodeTrapWithSharedTable(data, decodePeerIP, trustedRelay)
	if err != nil {
		kind := classifyDecodeError(err)
		if r.v3SecTable != nil {
			rawContext, rawErr := extractRawV3Context(data)
			if rawErr == nil && rawContext != nil {
				if r.policy.dynamicEngineID && !rawContext.reportable {
					retryContext, checked, dropped := r.tryDynamicRetry(data, decodePeerIP, datagram.Peer, rawContext, trustedRelay)
					rateLimitChecked = rateLimitChecked || checked
					if dropped {
						return Result{}
					}
					if retryContext != nil {
						packetContext = retryContext
						err = nil
					}
				} else if rawContext.discoveryProbe() && datagram.Conn != nil && datagram.Peer != nil {
					allowed, checked := r.allowRateLimitedPacket(datagram.Peer)
					rateLimitChecked = rateLimitChecked || checked
					if !allowed {
						return Result{}
					}
					r.sendDiscoveryReport(rawContext, datagram.Conn, datagram.Peer)
				}
			}
		}
		if err != nil {
			if shouldExtractEngineIDOnDecodeError(kind) {
				engineIDHex, ok, extractErr := extractSNMPv3EngineIDHex(data)
				if extractErr == nil && ok && !engineIDHexAllowed(engineIDHex, r.engineIDs) {
					kind = ErrorUnknownEngine
				}
			}
			r.reportError(kind)
			return Result{DecodeFailure: &DecodeFailure{
				Data:           data,
				PeerIP:         decodePeerIP,
				Conn:           datagram.Conn,
				Peer:           datagram.Peer,
				Kind:           kind,
				Err:            err,
				SniffedVersion: sniffedVersion,
				VersionKnown:   versionKnown,
				auditAdmitted:  rateLimitChecked,
			}}
		}
	}

	r.reportEvent(Event{Type: EventDecoded})
	pdu := packetContext.PDU
	if !r.versionAllowed(pdu.Version) {
		r.reportError(ErrorDroppedPolicy)
		return Result{}
	}
	if pdu.Version != model.SnmpVersionV3 && !r.communityAllowed(pdu.Community) {
		r.reportError(ErrorDroppedPolicy)
		return Result{}
	}
	if !r.ensureDynamicEngineIDRegistered(packetContext) {
		return Result{}
	}
	if packetContext.Packet != nil && pdu.Version == model.SnmpVersionV3 {
		if pdu.PduType == model.PduTypeInform {
			if !r.localEngineIDMatches(packetContext.Packet.SecurityParameters) {
				r.reportError(ErrorUnknownEngine)
				return Result{}
			}
		} else if !isEngineIDAllowed(packetContext.Packet.SecurityParameters, r.engineIDs) {
			r.reportError(ErrorUnknownEngine)
			return Result{}
		}
	}

	if pdu.PduType == model.PduTypeInform && packetContext.Packet != nil && datagram.Conn != nil && datagram.Peer != nil {
		var localEngineID []byte
		if r.localEngineID != nil {
			localEngineID = r.localEngineID.bytes()
		}
		if err := sendInformResponse(datagram.Conn, datagram.Peer, packetContext.Packet, r.engineBoots, localEngineID); err != nil {
			r.reportEvent(Event{Type: EventInformResponseFailed, Err: err})
		}
	}
	if !rateLimitChecked {
		allowed, _ := r.allowRateLimitedPacket(datagram.Peer)
		if !allowed {
			return Result{}
		}
	}
	return Result{PDU: packetContext.PDU}
}

func (r *Receiver) AdmitDecodeErrorAudit(failure *DecodeFailure) bool {
	if failure == nil {
		return false
	}
	if failure.auditAdmitted {
		return true
	}
	if r.rateLimiter == nil || failure.Peer == nil {
		return true
	}
	source, ok := udpPeerAddr(failure.Peer)
	if !ok {
		return true
	}
	allowed, mode := r.rateLimiter.allow(source)
	if allowed {
		return true
	}
	r.reportError(ErrorRateLimited)
	return mode != rateLimitModeDrop
}

func (r *Receiver) versionAllowed(version model.SnmpVersion) bool {
	if len(r.policy.versions) == 0 {
		return true
	}
	_, ok := r.policy.versions[version]
	return ok
}

func (r *Receiver) communityAllowed(community string) bool {
	return r.allowlist == nil || r.allowlist.allowedCommunity(community)
}

func (r *Receiver) trustedRelaySource(peerIP net.IP) bool {
	if len(r.policy.trustedRelays) == 0 || peerIP == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(peerIP)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range r.policy.trustedRelays {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func shouldExtractEngineIDOnDecodeError(kind ErrorKind) bool {
	switch kind {
	case ErrorAuthFailure, ErrorUSMFailure, ErrorUnknownEngine:
		return true
	default:
		return false
	}
}

func (r *Receiver) localEngineIDMatches(parameters gosnmp.SnmpV3SecurityParameters) bool {
	if r.localEngineID == nil || parameters == nil {
		return false
	}
	usm, ok := parameters.(*gosnmp.UsmSecurityParameters)
	return ok && r.localEngineID.equalRaw(usm.AuthoritativeEngineID)
}

func (r *Receiver) reportError(kind ErrorKind) {
	r.reportEvent(Event{Type: EventError, ErrorKind: kind})
}

func (r *Receiver) dynamicPairs() int {
	if r.dynamicEngineIDReg == nil {
		return 0
	}
	return r.dynamicEngineIDReg.size()
}

func (r *Receiver) reportEvent(event Event) {
	if r.report != nil {
		r.report(event)
	}
}

func udpPeerAddr(peer *net.UDPAddr) (netip.Addr, bool) {
	if peer == nil {
		return netip.Addr{}, false
	}
	addr, ok := netip.AddrFromSlice(peer.IP)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func packetSourceAddr(peerIP net.IP, peer *net.UDPAddr) (netip.Addr, bool) {
	if addr, ok := udpPeerAddr(peer); ok {
		return addr, true
	}
	if peerIP == nil {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(peerIP.String())
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}
