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

func (c *Collector) writeDecodeErrorEntry(data []byte, peerIP net.IP, conn *net.UDPConn, peer *net.UDPAddr, kind string, decodeErr error, sniffedVersion SnmpVersion, versionKnown bool) {
	if c.trapWriter == nil {
		return
	}

	if c.rateLimiter != nil && peer != nil {
		srcAddr, ok := udpPeerAddr(peer)
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

	entry := newDecodeErrorEntry(c.jobName, data, peerIP, conn, peer, kind, decodeErr, sniffedVersion, versionKnown)
	if err := c.trapWriter.Write(entry); err != nil {
		c.incTrapError(c.trapWriteFailureDim())
	}
}

func newDecodeErrorEntry(jobName string, data []byte, peerIP net.IP, conn *net.UDPConn, peer *net.UDPAddr, kind string, decodeErr error, sniffedVersion SnmpVersion, versionKnown bool) *TrapEntry {
	now := time.Now().UnixMicro()
	sourceIP, sourcePeer, sourcePort := decodeErrorSource(peerIP, peer)
	listener := decodeErrorListener(conn)
	errText := sanitizeDecodeError(decodeErr)
	packetHash := sha256.Sum256(data)

	info := &DecodeErrorInfo{
		Kind:          kind,
		Error:         errText,
		PacketSize:    len(data),
		PacketSHA256:  hex.EncodeToString(packetHash[:]),
		SourceUDPPort: sourcePort,
		Listener:      listener,
	}
	if versionKnown {
		info.SnmpVersion = string(sniffedVersion)
	}
	if engineID, ok := decodeErrorEngineID(data); ok {
		info.EngineID = engineID
	}

	messageSource := sourcePeer
	if messageSource == "" {
		messageSource = sourceIP
	}
	message := fmt.Sprintf("SNMP trap decode failed from %s: %s: %s", messageSource, kind, errText)
	if len(message) > maxMessageLen {
		message = truncateUTF8(message, maxMessageLen-3) + "..."
	}

	return &TrapEntry{
		JobName:               jobName,
		ReportType:            ReportTypeDecodeError,
		ReceivedRealtimeUsec:  now,
		ReceivedMonotonicUsec: monotonicUsec(),
		Category:              decodeErrorCategory(kind),
		Severity:              "warning",
		Message:               message,
		SourceIP:              sourceIP,
		SourceUDPPeer:         sourcePeer,
		SnmpVersion:           sniffedVersion,
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
