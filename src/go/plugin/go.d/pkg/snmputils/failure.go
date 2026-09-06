// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"

	"github.com/gosnmp/gosnmp"
)

// Failure contains only allowlisted diagnostic fields, never an error message.
// Empty means no failure observed; unknown means a failure with no typed reason.
type Failure struct {
	Operation    string `json:"operation,omitempty"`
	Reason       string `json:"reason,omitempty"`
	PacketStatus uint8  `json:"packet_status,omitempty"`
	ErrorIndex   uint32 `json:"error_index,omitempty"`
}

// FailureLogicalBytes reserves scalar storage and the longest allowlisted labels.
const FailureLogicalBytes uint64 = 128

type classifiedFailure struct {
	cause   error
	failure Failure
}

func (e *classifiedFailure) Error() string { return e.cause.Error() }
func (e *classifiedFailure) Unwrap() error { return e.cause }

// WithFailure preserves operational error behavior while keeping available
// source detail through wrappers that would otherwise discard it.
func WithFailure(err error, operation, reason string) error {
	if err == nil {
		return nil
	}
	f := ClassifyFailure(err)
	if operation != "" {
		f.Operation = operation
	}
	if reason != "" {
		f.Reason = reason
	}
	return &classifiedFailure{cause: err, failure: f}
}

func WithPacketFailure(err error, operation string, packet *gosnmp.SnmpPacket) error {
	if err == nil {
		return nil
	}
	f := Failure{Operation: operation, Reason: "packet_error", PacketStatus: uint8(packet.Error), ErrorIndex: uint32(packet.ErrorIndex)}
	return &classifiedFailure{cause: err, failure: f}
}

func ClassifyFailure(err error) Failure {
	if err == nil {
		return Failure{}
	}
	var classified *classifiedFailure
	if errors.As(err, &classified) {
		return classified.failure
	}
	f := Failure{Reason: "unknown"}
	switch {
	case errors.Is(err, context.Canceled):
		f.Reason = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		f.Reason = "deadline"
	case errors.Is(err, net.ErrClosed):
		f.Reason = "closed"
	case errors.Is(err, syscall.ECONNREFUSED):
		f.Reason = "connection_refused"
	case errors.Is(err, syscall.ECONNRESET):
		f.Reason = "connection_reset"
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		f.Reason = "unreachable"
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		f.Reason = "permission"
	case errors.Is(err, io.EOF):
		f.Reason = "eof"
	case errors.Is(err, gosnmp.ErrWrongDigest):
		f.Reason = "wrong_digest"
	case errors.Is(err, gosnmp.ErrUnknownUsername):
		f.Reason = "unknown_user"
	case errors.Is(err, gosnmp.ErrDecryption):
		f.Reason = "decryption"
	case errors.Is(err, gosnmp.ErrUnknownSecurityLevel):
		f.Reason = "unsupported_security_level"
	case errors.Is(err, gosnmp.ErrUnknownSecurityModels):
		f.Reason = "unknown_security_model"
	case errors.Is(err, gosnmp.ErrInvalidMsgs):
		f.Reason = "invalid_message"
	case errors.Is(err, gosnmp.ErrUnknownPDUHandlers):
		f.Reason = "unknown_pdu_handler"
	case errors.Is(err, gosnmp.ErrUnknownReportPDU):
		f.Reason = "unknown_report"
	default:
		var dns *net.DNSError
		var network net.Error
		switch {
		case errors.As(err, &dns):
			f.Reason = "dns"
			if dns.IsNotFound {
				f.Reason = "dns_not_found"
			} else if dns.Timeout() {
				f.Reason = "timeout"
			}
		case errors.As(err, &network) && network.Timeout():
			f.Reason = "timeout"
		}
	}
	return f
}

func (f Failure) Valid() bool {
	switch f.Reason {
	case "missing_hostname",
		"invalid_vnode_id",
		"missing_v3_username",
		"invalid_snmp_version",
		"",
		"unknown",
		"cancelled",
		"deadline",
		"closed",
		"connection_refused",
		"connection_reset",
		"unreachable",
		"permission",
		"eof",
		"wrong_digest",
		"unknown_user",
		"decryption",
		"unsupported_security_level",
		"unknown_security_model",
		"invalid_message",
		"unknown_pdu_handler",
		"unknown_report",
		"dns",
		"dns_not_found",
		"timeout",
		"packet_error",
		"nil_response",
		"invalid_configuration",
		"no_profiles",
		"processing",
		"dependency",
		"panic":
	default:
		return false
	}
	switch f.Operation {
	case "max_repetitions",
		"",
		"configuration",
		"client",
		"connect",
		"system_identity",
		"profile_resolution",
		"metadata",
		"get",
		"walk",
		"prepare",
		"tables",
		"bgp",
		"licensing",
		"sys_uptime",
		"ping",
		"vlan_identifier":
	default:
		return false
	}
	if f.Reason == "" {
		return f == (Failure{})
	}
	if f.Reason == "packet_error" {
		return f.PacketStatus > 0
	}
	return f.PacketStatus == 0 && f.ErrorIndex == 0
}

// WithFailureDetail annotates an existing wrapper without changing its message
// or exposing a cause that the operational path deliberately did not wrap.
func WithFailureDetail(err error, f Failure) error {
	if err == nil {
		return nil
	}
	if !f.Valid() {
		f = Failure{Reason: "unknown"}
	}
	return &classifiedFailure{cause: err, failure: f}
}

// ClassifyGetFailure preserves the decoded response status, including vendor or
// malformed numeric values accepted by the SNMP decoder.
func ClassifyGetFailure(packet *gosnmp.SnmpPacket, err error) Failure {
	f := ClassifyFailure(err)
	if err == nil {
		if packet == nil {
			f.Reason = "nil_response"
		} else if packet.Error != gosnmp.NoError {
			f = Failure{Reason: "packet_error", PacketStatus: uint8(packet.Error), ErrorIndex: uint32(packet.ErrorIndex)}
		}
	}
	if f.Reason != "" {
		f.Operation = "get"
	}
	return f
}
