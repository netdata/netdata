// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"bytes"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/netdata/gosnmp"
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
	// EventReportFailed is a failed write of an SNMPv3 Report: the receiver's
	// answer to discovery, to an engine ID it has no user for, or to an
	// authenticated message outside its time window.
	EventReportFailed
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

// DecodeFailure is a datagram the receiver could not decode. SniffedVersion
// and EngineID come from its unauthenticated header (gosnmp.ReadEnvelope):
// none when the header does not parse; EngineID is hex, empty when the message
// names none.
type DecodeFailure struct {
	Data           []byte
	PeerIP         net.IP
	Conn           *net.UDPConn
	Peer           *net.UDPAddr
	Kind           ErrorKind
	Err            error
	SniffedVersion model.SnmpVersion
	VersionKnown   bool
	EngineID       string
	auditAdmitted  bool
}

type Result struct {
	PDU           *model.TrapPDU
	DecodeFailure *DecodeFailure
}

type Receiver struct {
	policy Policy
	report Reporter

	listener    *listener
	allowlist   *allowlist
	rateLimiter *rateLimiter
	// receiver decodes every datagram: SNMPv1 and SNMPv2c with no users, SNMPv3
	// with the users PrepareV3 registers.
	receiver      *gosnmp.Receiver
	engineIDs     [][]byte
	engineBoots   *engineBoots
	localEngineID *localEngineID
	rollbackState func()
}

func New(policy Policy, report Reporter) *Receiver {
	plain, _ := gosnmp.NewReceiver(gosnmp.ReceiverConfig{})
	return &Receiver{
		policy:      policy,
		report:      report,
		allowlist:   newAllowlist(policy.sourceAllowlist, policy.communities),
		rateLimiter: newRateLimiter(policy.rateLimit.Enabled, policy.rateLimit.PerSourcePPS, policy.rateLimit.Mode),
		receiver:    plain,
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

// PrepareV3 builds the job's SNMPv3 receiver: the configured users, registered
// with their engines and the local engine ID, and the local engine (its engine
// ID and boots). Configuration errors come before the engine boots are raised,
// so a bad configuration burns no boot.
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
	users, learnUsers, err := receiverUsers(r.policy.users, engineIDs, localEngineID.bytes(), r.policy.dynamicEngineID)
	if err != nil {
		rollback()
		return configPreparationError(err)
	}
	cfg := gosnmp.ReceiverConfig{Users: users}
	if r.policy.dynamicEngineID && len(learnUsers) > 0 {
		cfg.LearnUsers, cfg.MaxLearned = learnUsers, r.policy.dynamicEngineIDMax
	}
	// Without the local engine, the receiver checks the users only.
	if _, err := gosnmp.NewReceiver(cfg); err != nil {
		rollback()
		return configPreparationError(err)
	}
	engineBoots, err := newEngineBoots(paths)
	if err != nil {
		rollback()
		return startupPreparationError(err)
	}
	cfg.EngineID, cfg.EngineBoots = localEngineID.bytes(), uint32(engineBoots.bootsValue())
	receiver, err := gosnmp.NewReceiver(cfg)
	if err != nil {
		rollback()
		return startupPreparationError(err)
	}

	r.receiver = receiver
	r.engineIDs = engineIDs
	r.localEngineID = localEngineID
	r.engineBoots = engineBoots
	r.rollbackState = rollback
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

	// The unauthenticated header: the version for the policy and the decode
	// budget, the engine ID for the checks below and the decode-error entry.
	envelope, envelopeErr := gosnmp.ReadEnvelope(data)
	var sniffedVersion model.SnmpVersion
	versionKnown := false
	if envelopeErr == nil {
		sniffedVersion, envelopeErr = snmpVersion(envelope.Version)
		versionKnown = envelopeErr == nil
	}
	if versionKnown && !r.versionAllowed(sniffedVersion) {
		r.reportError(ErrorDroppedPolicy)
		return Result{}
	}
	engineID := envelope.SecurityParameters.AuthoritativeEngineID
	if r.policy.dynamicEngineID && versionKnown && sniffedVersion == model.SnmpVersionV3 &&
		len(engineID) > 0 && (isAllByte(engineID, 0x00) || isAllByte(engineID, 0xff)) {
		// Such engine IDs are never registered (parseEngineIDHex), so a job
		// learns none of them.
		r.reportError(ErrorUnknownEngine)
		return Result{}
	}

	trustedRelay := r.trustedRelaySource(decodePeerIP)
	packetContext, response, err := decodeTrapWithOptions(data, versionKnown && sniffedVersion == model.SnmpVersionV3, decodePeerIP, r.receiver, decodeOptions{trustedRelay: trustedRelay})
	if err != nil {
		// A Report answers a rejected SNMPv3 message: send it after one
		// rate-limit token, since its source can be forged and it can be larger
		// than the message.
		if response != nil && datagram.Conn != nil && datagram.Peer != nil {
			allowed, checked := r.allowRateLimitedPacket(datagram.Peer)
			rateLimitChecked = rateLimitChecked || checked
			if !allowed {
				return Result{}
			}
			if err := r.writeResponse(datagram, response); err != nil {
				r.reportEvent(Event{Type: EventReportFailed, Err: err})
			}
		}
		kind := classifyDecodeError(err)
		r.reportError(kind)
		failure := &DecodeFailure{
			Data:           data,
			PeerIP:         decodePeerIP,
			Conn:           datagram.Conn,
			Peer:           datagram.Peer,
			Kind:           kind,
			Err:            err,
			SniffedVersion: sniffedVersion,
			VersionKnown:   versionKnown,
			auditAdmitted:  rateLimitChecked,
		}
		if versionKnown && len(engineID) > 0 {
			failure.EngineID = hex.EncodeToString(engineID)
		}
		return Result{DecodeFailure: failure}
	}

	r.reportEvent(Event{Type: EventDecoded})
	pdu := packetContext.PDU
	msg := &packetContext.Notification.Message
	if !r.versionAllowed(pdu.Version) {
		r.reportError(ErrorDroppedPolicy)
		return Result{}
	}
	if pdu.Version != model.SnmpVersionV3 && !r.communityAllowed(pdu.Community) {
		r.reportError(ErrorDroppedPolicy)
		return Result{}
	}
	if pdu.Version == model.SnmpVersionV3 && pdu.PduType != model.PduTypeInform && r.trapNamesUnauthorizedLocalEngine(msg) {
		r.reportError(ErrorUnknownEngine)
		return Result{}
	}
	if packetContext.Notification.Learned {
		r.reportEvent(Event{
			Type:     EventDynamicEngineIDRegistered,
			EngineID: hex.EncodeToString(msg.SecurityParameters.AuthoritativeEngineID),
			Username: msg.SecurityParameters.UserName,
		})
	}

	// An accepted INFORM is acknowledged before the rate limit, so a rate-limited
	// INFORM is still acked while its content is dropped.
	if pdu.PduType == model.PduTypeInform && response != nil && datagram.Conn != nil && datagram.Peer != nil {
		if err := r.writeResponse(datagram, response); err != nil {
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

// trapNamesUnauthorizedLocalEngine reports a static job's trap that names the
// job's own engine ID, which the receiver decodes with the users registered
// there for informs: it is authorized only when that engine is whitelisted.
func (r *Receiver) trapNamesUnauthorizedLocalEngine(msg *gosnmp.Message) bool {
	if r.policy.dynamicEngineID || r.localEngineID == nil {
		return false
	}
	engineID := msg.SecurityParameters.AuthoritativeEngineID
	if !bytes.Equal(engineID, r.localEngineID.bytes()) {
		return false
	}
	for _, allowed := range r.engineIDs {
		if bytes.Equal(engineID, allowed) {
			return false
		}
	}
	return true
}

func (r *Receiver) writeResponse(datagram Datagram, response []byte) error {
	_, err := datagram.Conn.WriteToUDP(response, datagram.Peer)
	return err
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

func (r *Receiver) reportError(kind ErrorKind) {
	r.reportEvent(Event{Type: EventError, ErrorKind: kind})
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
