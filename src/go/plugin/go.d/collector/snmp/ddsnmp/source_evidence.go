// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// SourceOperation is one completed Handler call, including its internal retries.
// Ordinal identifies it within its acquisition context; ContextID qualifies that
// identity across normal polls. Wire packets and credentials are not retained.
// WALK terminal details hidden by the Handler are unknown.
type SourceOperation struct {
	ContextID     uint64            `json:"context_id,omitempty"`
	Ordinal       uint64            `json:"ordinal"`
	StartedAt     time.Time         `json:"started_at"`
	Method        string            `json:"method"`
	RequestedOIDs []string          `json:"requested_oids"`
	ElapsedNanos  int64             `json:"elapsed_ns"`
	Failure       snmputils.Failure `json:"failure"`
	// ResultPresent means the Handler returned a non-nil packet or WALK slice.
	// It does not establish whether a wire response or terminal PDU was received.
	ResultPresent bool        `json:"result_present"`
	PDUs          []SourcePDU `json:"pdus"`
}

type SourcePDU struct {
	OID   string      `json:"oid"`
	Type  uint8       `json:"type"`
	Value SourceValue `json:"value"`
}

// SourceValue preserves decoded values without JSON number rounding or UTF-8
// replacement. Floating-point text contains hexadecimal IEEE bits, not a decimal.
type SourceValue struct {
	Kind  string `json:"kind"`
	Text  string `json:"text,omitempty"`
	Bytes []byte `json:"bytes,omitempty"`
}

// SourceBinding records requested input, not a claim that it was processed.
// Operation is one-based within the owning collection context.
type SourceBinding struct {
	ContextID uint64 `json:"context_id,omitempty"`
	Operation uint64 `json:"operation"`
	OID       string `json:"oid"`
	Role      string `json:"role"`
}

// ProcessingEvent records an attempted field's non-success outcome. OID identifies
// the input or missing instance; it does not claim a particular occurrence among
// duplicate raw PDUs. Source operations preserve every occurrence in return order.
type ProcessingEvent struct {
	Stage    string `json:"stage,omitempty"`
	RowIndex string `json:"row_index,omitempty"`
	Field    string `json:"field"`
	OID      string `json:"oid,omitempty"`
	Reason   string `json:"reason"`
}

func ValidateSourceOperations(operations []*SourceOperation) error {
	for i, op := range operations {
		if op == nil {
			return fmt.Errorf("operation %d: missing source operation", i+1)
		}
		if op.Method != "get" && op.Method != "walk" && op.Method != "bulk_walk" {
			return fmt.Errorf("operation %d: invalid method", i+1)
		}
		if op.ElapsedNanos < 0 || len(op.RequestedOIDs) == 0 || (op.Method != "get" && len(op.RequestedOIDs) != 1) {
			return fmt.Errorf("operation %d: invalid request", i+1)
		}
		if !op.Failure.Valid() {
			return fmt.Errorf("operation %d: invalid failure", i+1)
		}
		if !op.ResultPresent && len(op.PDUs) != 0 {
			return fmt.Errorf("operation %d: PDUs without a returned result", i+1)
		}
		for _, pdu := range op.PDUs {
			v := pdu.Value
			valid := true
			switch v.Kind {
			case "null", "bytes_nil", "unavailable":
				valid = v.Text == "" && len(v.Bytes) == 0
			case "bytes", "string_bytes":
				valid = v.Text == ""
			case "string":
				valid = len(v.Bytes) == 0
			case "signed":
				n, err := strconv.ParseInt(v.Text, 10, 64)
				valid = err == nil && strconv.FormatInt(n, 10) == v.Text && len(v.Bytes) == 0
			case "unsigned":
				n, err := strconv.ParseUint(v.Text, 10, 64)
				valid = err == nil && strconv.FormatUint(n, 10) == v.Text && len(v.Bytes) == 0
			case "float32", "float64":
				bits := 64
				if v.Kind == "float32" {
					bits = 32
				}
				n, err := strconv.ParseUint(v.Text, 16, bits)
				valid = err == nil && strconv.FormatUint(n, 16) == v.Text && len(v.Bytes) == 0
			default:
				valid = false
			}
			if !valid {
				return fmt.Errorf("operation %d: invalid source value", i+1)
			}
		}
	}
	return nil
}

// SourceRequestIndex is temporary import-validation state. Building it once per
// context avoids rescanning a large GET batch for every consumer reference.
func SourceRequestIndex(operations []*SourceOperation) []map[string]struct{} {
	index := make([]map[string]struct{}, len(operations))
	for i, op := range operations {
		index[i] = make(map[string]struct{}, len(op.RequestedOIDs))
		for _, oid := range op.RequestedOIDs {
			index[i][strings.TrimPrefix(oid, ".")] = struct{}{}
		}
	}
	return index
}

func ValidateSourceBindings(bindings []SourceBinding, requests []map[string]struct{}) error {
	for _, binding := range bindings {
		if binding.Operation == 0 || binding.Operation > uint64(len(requests)) || (binding.Role != "primary" && binding.Role != "dependency") {
			return fmt.Errorf("invalid source binding")
		}
		if _, ok := requests[binding.Operation-1][binding.OID]; !ok {
			return fmt.Errorf("source binding does not reference a requested OID")
		}
	}
	return nil
}

// IsFailure distinguishes failed processing from ordinary omitted input/output.
// Rejection counts describe row acceptance and must not decide failure retention.
func (e ProcessingEvent) IsFailure() bool {
	failed, _ := processingReason(e.Reason)
	return failed
}

// Validation and classification share the same closed reason vocabulary.
func processingReason(reason string) (failed, valid bool) {
	switch reason {
	case "missing_input", "missing_dependency", "empty_date", "sentinel", "no_signals", "incomplete_identity":
		return false, true
	case "conversion", "pattern_mismatch", "extract_mismatch", "index_processing", "dependency_processing":
		return true, true
	default:
		return true, false
	}
}

func ValidateProcessingEvents(events []ProcessingEvent) error {
	for _, event := range events {
		if event.Stage != "" && event.Stage != "raw_text" {
			return fmt.Errorf("invalid processing stage")
		}
		if event.Field == "" {
			return fmt.Errorf("processing event has no field")
		}
		if _, valid := processingReason(event.Reason); !valid {
			return fmt.Errorf("invalid processing reason")
		}
	}
	return nil
}

// SourceOperationsShape describes retained logical content, not heap usage or
// encoded bytes. Physical operations are counted once, independently of consumers.
func SourceOperationsShape(operations []*SourceOperation) (records, logicalBytes uint64) {
	for _, op := range operations {
		records += uint64(1 + len(op.RequestedOIDs) + len(op.PDUs))
		logicalBytes += uint64(104+len(op.Method)) + snmputils.FailureLogicalBytes
		for _, oid := range op.RequestedOIDs {
			logicalBytes += uint64(16 + len(oid))
		}
		for _, pdu := range op.PDUs {
			logicalBytes += uint64(64 + len(pdu.OID) + len(pdu.Value.Kind) + len(pdu.Value.Text) + len(pdu.Value.Bytes))
		}
	}
	return records, logicalBytes
}
