// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
)

const maxDecodeErrorLen = 256

func (j *Job) writeDecodeErrorEntry(failure *receiver.DecodeFailure, packetSequence uint64) {
	entry := newDecodeErrorEntry(j.policy.jobName, failure, packetSequence, j.monotonicUsecWith(j.journalHost))
	if err := j.writer.Write(entry); err != nil {
		j.telemetry.Error(j.trapWriteFailureDim())
	}
}

func newDecodeErrorEntry(jobName string, failure *receiver.DecodeFailure, packetSequence uint64, monotonicUsec int64) *model.TrapEntry {
	now := time.Now().UnixMicro()
	sourceIP, sourcePeer, sourcePort := decodeErrorSource(failure.PeerIP, failure.Peer)
	listener := decodeErrorListener(failure.Conn)
	errText := sanitizeDecodeError(failure.Err)
	packetHash := sha256.Sum256(failure.Data)

	info := &model.DecodeErrorInfo{
		Kind:          string(failure.Kind),
		Error:         errText,
		PacketSize:    len(failure.Data),
		PacketSHA256:  hex.EncodeToString(packetHash[:]),
		SourceUDPPort: sourcePort,
		Listener:      listener,
	}
	if failure.VersionKnown {
		info.SnmpVersion = string(failure.SniffedVersion)
	}
	if engineID, ok := receiver.ExtractEngineID(failure.Data); ok {
		info.EngineID = engineID
	}

	messageSource := sourcePeer
	if messageSource == "" {
		messageSource = sourceIP
	}
	message := fmt.Sprintf("SNMP trap decode failed from %s: %s: %s", messageSource, failure.Kind, errText)
	if len(message) > catalog.MaxMessageLen {
		message = catalog.TruncateUTF8(message, catalog.MaxMessageLen-3) + "..."
	}

	return &model.TrapEntry{
		JobName:               jobName,
		ReportType:            model.ReportTypeDecodeError,
		ReceivedRealtimeUsec:  now,
		ReceivedMonotonicUsec: monotonicUsec,
		Category:              decodeErrorCategory(failure.Kind),
		Severity:              "warning",
		Message:               message,
		SourceIP:              sourceIP,
		SourceUDPPeer:         sourcePeer,
		SnmpVersion:           failure.SniffedVersion,
		PacketSequence:        packetSequence,
		DecodeError:           info,
	}
}

func decodeErrorSource(peerIP net.IP, peer *net.UDPAddr) (sourceIP, sourcePeer string, sourcePort int) {
	if peerIP != nil {
		sourceIP = peerIP.String()
	}
	if peer != nil {
		if sourceIP == "" && peer.IP != nil {
			sourceIP = peer.IP.String()
		}
		sourcePort = peer.Port
	}
	if sourcePeer == "" {
		sourcePeer = sourceIP
	}
	return sourceIP, sourcePeer, sourcePort
}

func decodeErrorListener(conn *net.UDPConn) string {
	if conn == nil || conn.LocalAddr() == nil {
		return ""
	}
	return conn.LocalAddr().String()
}

func decodeErrorCategory(kind receiver.ErrorKind) model.Category {
	switch kind {
	case receiver.ErrorAuthFailure, receiver.ErrorUSMFailure, receiver.ErrorUnknownEngine:
		return "auth"
	default:
		return "diagnostic"
	}
}

func sanitizeDecodeError(err error) string {
	if err == nil {
		return "unknown decode error"
	}
	s := strings.TrimSpace(err.Error())
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		s = "unknown decode error"
	}
	if len(s) > maxDecodeErrorLen {
		return catalog.TruncateUTF8(s, maxDecodeErrorLen-3) + "..."
	}
	return s
}
