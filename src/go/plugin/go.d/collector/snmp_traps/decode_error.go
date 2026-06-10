// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"
)

const maxDecodeErrorLen = 256

type decodeErrorRecord struct {
	data           []byte
	peerIP         net.IP
	conn           *net.UDPConn
	peer           *net.UDPAddr
	kind           string
	err            error
	sniffedVersion SnmpVersion
	versionKnown   bool
}

func (c *Collector) writeDecodeErrorEntry(rec decodeErrorRecord) {
	if c.trapWriter == nil {
		return
	}

	if c.rateLimiter != nil && rec.peer != nil {
		srcAddr, ok := udpPeerAddr(rec.peer)
		if ok {
			allowed, mode := c.rateLimiter.Allow(srcAddr)
			if !allowed {
				c.incTrapError("rate_limited")
				if mode == rateLimitModeDrop {
					return
				}
			}
		}
	}

	entry := newDecodeErrorEntry(c.jobName, rec)
	if err := c.trapWriter.Write(entry); err != nil {
		c.incTrapError(c.trapWriteFailureDim())
	}
}

func newDecodeErrorEntry(jobName string, rec decodeErrorRecord) *TrapEntry {
	now := time.Now().UnixMicro()
	sourceIP, sourcePeer, sourcePort := decodeErrorSource(rec.peerIP, rec.peer)
	listener := decodeErrorListener(rec.conn)
	errText := sanitizeDecodeError(rec.err)
	packetHash := sha256.Sum256(rec.data)

	info := &DecodeErrorInfo{
		Kind:          rec.kind,
		Error:         errText,
		PacketSize:    len(rec.data),
		PacketSHA256:  hex.EncodeToString(packetHash[:]),
		SourceUDPPort: sourcePort,
		Listener:      listener,
	}
	if rec.versionKnown {
		info.SnmpVersion = string(rec.sniffedVersion)
	}
	if engineID, ok := decodeErrorEngineID(rec.data); ok {
		info.EngineID = engineID
	}

	messageSource := sourcePeer
	if messageSource == "" {
		messageSource = sourceIP
	}
	message := fmt.Sprintf("SNMP trap decode failed from %s: %s: %s", messageSource, rec.kind, errText)
	if len(message) > maxMessageLen {
		message = truncateUTF8(message, maxMessageLen-3) + "..."
	}

	return &TrapEntry{
		JobName:               jobName,
		ReportType:            ReportTypeDecodeError,
		ReceivedRealtimeUsec:  now,
		ReceivedMonotonicUsec: monotonicUsec(),
		Category:              decodeErrorCategory(rec.kind),
		Severity:              "warning",
		Message:               message,
		SourceIP:              sourceIP,
		SourceUDPPeer:         sourcePeer,
		SnmpVersion:           rec.sniffedVersion,
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

func decodeErrorEngineID(data []byte) (string, bool) {
	engineID, ok, err := extractSNMPv3EngineIDHex(data)
	if err != nil || !ok || engineID == "" {
		return "", false
	}
	return engineID, true
}

func decodeErrorCategory(kind string) Category {
	switch kind {
	case "auth_failures", "usm_failures", "unknown_engine_id":
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
		return truncateUTF8(s, maxDecodeErrorLen-3) + "..."
	}
	return s
}
