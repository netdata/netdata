// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import "errors"

type fanoutTrapWriter struct {
	primary   TrapWriter
	secondary TrapWriter
	metrics   *perJobMetrics
}

func newFanoutTrapWriter(primary, secondary TrapWriter, metrics *perJobMetrics) TrapWriter {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return &fanoutTrapWriter{
		primary:   primary,
		secondary: secondary,
		metrics:   metrics,
	}
}

func (w *fanoutTrapWriter) Write(entry *TrapEntry) error {
	primaryErr := w.primary.Write(entry)
	if err := w.secondary.Write(entry); err != nil {
		w.incOTLPExportFailed(1)
	}
	return primaryErr
}

func (w *fanoutTrapWriter) Flush() error {
	primaryErr := w.primary.Flush()
	secondaryErr := w.secondary.Flush()
	if secondaryErr != nil {
		w.incOTLPExportFailed(1)
	}
	return errors.Join(primaryErr, secondaryErr)
}

func (w *fanoutTrapWriter) Close() error {
	secondaryErr := w.secondary.Close()
	if secondaryErr != nil {
		w.incOTLPExportFailed(1)
	}
	return errors.Join(w.primary.Close(), secondaryErr)
}

func (w *fanoutTrapWriter) BinaryEncodedFields() uint64 {
	if binaryEncoded, ok := w.primary.(interface{ BinaryEncodedFields() uint64 }); ok {
		return binaryEncoded.BinaryEncodedFields()
	}
	return 0
}

func (w *fanoutTrapWriter) incOTLPExportFailed(n uint64) {
	if w.metrics != nil {
		w.metrics.addError("otlp_export_failed", n)
	}
}
