// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSourceEvidence(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate  func(*SourceOperation, *SourceBinding, *ProcessingEvent)
		invalid bool
	}{
		"valid partial response":  {},
		"unknown method":          {mutate: func(o *SourceOperation, _ *SourceBinding, _ *ProcessingEvent) { o.Method = "packet" }, invalid: true},
		"negative elapsed":        {mutate: func(o *SourceOperation, _ *SourceBinding, _ *ProcessingEvent) { o.ElapsedNanos = -1 }, invalid: true},
		"absent result with PDUs": {mutate: func(o *SourceOperation, _ *SourceBinding, _ *ProcessingEvent) { o.ResultPresent = false }, invalid: true},
		"unsigned overflow": {mutate: func(o *SourceOperation, _ *SourceBinding, _ *ProcessingEvent) {
			o.PDUs[0].Value = SourceValue{Kind: "unsigned", Text: "18446744073709551616"}
		}, invalid: true},
		"float32 overflow": {mutate: func(o *SourceOperation, _ *SourceBinding, _ *ProcessingEvent) {
			o.PDUs[0].Value = SourceValue{Kind: "float32", Text: "100000000"}
		}, invalid: true},
		"mixed representations":     {mutate: func(o *SourceOperation, _ *SourceBinding, _ *ProcessingEvent) { o.PDUs[0].Value.Bytes = []byte{1} }, invalid: true},
		"operation zero":            {mutate: func(_ *SourceOperation, b *SourceBinding, _ *ProcessingEvent) { b.Operation = 0 }, invalid: true},
		"operation outside context": {mutate: func(_ *SourceOperation, b *SourceBinding, _ *ProcessingEvent) { b.Operation = 2 }, invalid: true},
		"unrequested root":          {mutate: func(_ *SourceOperation, b *SourceBinding, _ *ProcessingEvent) { b.OID = "1.9" }, invalid: true},
		"invalid role":              {mutate: func(_ *SourceOperation, b *SourceBinding, _ *ProcessingEvent) { b.Role = "accepted" }, invalid: true},
		"unknown processing reason": {mutate: func(_ *SourceOperation, _ *SourceBinding, e *ProcessingEvent) { e.Reason = "arbitrary error" }, invalid: true},
		"unknown processing stage":  {mutate: func(_ *SourceOperation, _ *SourceBinding, e *ProcessingEvent) { e.Stage = "unknown" }, invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			operation := SourceOperation{Method: "bulk_walk", RequestedOIDs: []string{".1.2.3"}, ResultPresent: true, PDUs: []SourcePDU{{OID: "1.2.3.1", Type: 2, Value: SourceValue{Kind: "signed", Text: "42"}}}}
			binding := SourceBinding{Operation: 1, OID: "1.2.3", Role: "primary"}
			event := ProcessingEvent{RowIndex: "1", Field: "state", OID: "1.2.3.1", Reason: "conversion"}
			if tc.mutate != nil {
				tc.mutate(&operation, &binding, &event)
			}
			operations := []SourceOperation{operation}
			err := ValidateSourceOperations(operations)
			if err == nil {
				err = ValidateSourceBindings([]SourceBinding{binding}, SourceRequestIndex(operations))
			}
			if err == nil {
				err = ValidateProcessingEvents([]ProcessingEvent{event})
			}
			if tc.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
