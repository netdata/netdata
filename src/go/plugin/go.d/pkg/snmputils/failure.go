// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"

	"github.com/netdata/gosnmp"
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
		if reason != "packet_error" {
			f.PacketStatus, f.ErrorIndex = 0, 0
		}
	}
	return WithFailureDetail(err, f)
}

// WithPacketFailure annotates err with the error-status and index of the
// agent's answer, status.
func WithPacketFailure(err error, operation string, status *gosnmp.StatusError) error {
	if err == nil {
		return nil
	}
	if status == nil {
		return WithFailure(err, operation, "")
	}
	return WithFailureDetail(err, packetFailure(operation, status))
}

func packetFailure(operation string, status *gosnmp.StatusError) Failure {
	return Failure{Operation: operation, Reason: "packet_error", PacketStatus: uint8(status.Status), ErrorIndex: uint32(status.Index)}
}

func ClassifyFailure(err error) Failure {
	if err == nil {
		return Failure{}
	}
	var classified *classifiedFailure
	if errors.As(err, &classified) {
		return classified.failure
	}
	var status *gosnmp.StatusError
	if errors.As(err, &status) {
		return packetFailure("", status)
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
	case errors.Is(err, gosnmp.ErrNotInTimeWindow):
		f.Reason = "not_in_time_window"
	case errors.Is(err, gosnmp.ErrUnknownEngineID):
		f.Reason = "unknown_engine_id"
	case errors.Is(err, gosnmp.ErrASNParse):
		f.Reason = "parse_error"
	case errors.Is(err, gosnmp.ErrBadVersion):
		f.Reason = "bad_version"
	case errors.Is(err, gosnmp.ErrUnmatchedReply):
		f.Reason = "unmatched_reply"
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
		"not_in_time_window",
		"unknown_engine_id",
		"parse_error",
		"bad_version",
		"unmatched_reply",
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

// ClassifyGetFailure classifies a Get's outcome: the answer's error-status,
// including vendor or malformed numeric values accepted by the SNMP decoder,
// comes in a *gosnmp.StatusError, and a client fake may answer nothing.
func ClassifyGetFailure(packet *gosnmp.Message, err error) Failure {
	f := ClassifyFailure(err)
	if err == nil && packet == nil {
		f.Reason = "nil_response"
	}
	if f.Reason != "" {
		f.Operation = "get"
	}
	return f
}
